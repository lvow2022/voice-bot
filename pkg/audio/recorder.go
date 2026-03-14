package audio

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path"
	"sync"
	"time"

	"github.com/youpy/go-wav"
	"voicebot/pkg/voicechain"
)

func NewRecordFile(fileName string) (*os.File, error) {
	baseDir := path.Dir(fileName)
	if _, err := os.Stat(baseDir); os.IsNotExist(err) {
		if err := os.MkdirAll(baseDir, os.ModePerm); err != nil {
			return nil, err
		}
	}
	file, err := os.Create(fileName)
	if err != nil {
		return nil, err
	}
	return file, nil
}

type Recorder struct {
	fileName       string
	writer         io.WriteSeeker
	wavWriter      *wav.Writer
	cancel         context.CancelFunc
	mtx            sync.Mutex
	mono           *RingBuffer[uint16]
	stereo         *RingBuffer[uint16]
	samples        []wav.Sample // 复用 samples slice 以减少分配
	writeBuffer    []uint16     // 复用 write buffer 以减少分配
	codecOpt       voicechain.CodecOption
	monoWrites     int64
	stereoWrites   int64
	lastMonoTime   time.Time
	lastStereoTime time.Time
}

func NewRecorder(fileName string, w io.WriteSeeker, codecOpt voicechain.CodecOption) *Recorder {
	return &Recorder{
		fileName:  fileName,
		writer:    w,
		wavWriter: wav.NewWriter(w, 0, 2, uint32(codecOpt.SampleRate), uint16(codecOpt.BitDepth)),
		codecOpt:  codecOpt,
		mono:      NewRingBuffer[uint16](4096),
		stereo:    NewRingBuffer[uint16](4096),
	}
}
func (r *Recorder) WriteMono(data []byte) {
	r.mtx.Lock()
	defer r.mtx.Unlock()
	r.monoWrites++
	r.lastMonoTime = time.Now()

	// 复用 write buffer，避免每次分配
	// 使用 (len(data) + 1) / 2 向上取整，确保当 data 长度为奇数时也能容纳所有索引
	expectedLen := (len(data) + 1) / 2
	if cap(r.writeBuffer) < expectedLen {
		r.writeBuffer = make([]uint16, expectedLen)
	} else {
		r.writeBuffer = r.writeBuffer[:expectedLen]
	}

	// 将 byte 数据转换为 uint16 slice
	for i := 0; i < len(data); i += 2 {
		var value = uint16(data[i])
		if i+1 < len(data) {
			value |= uint16(data[i+1]) << 8
		}
		r.writeBuffer[i/2] = value
	}
	r.mono.Write(r.writeBuffer)
}

func (r *Recorder) WriteStereo(data []byte) {
	r.mtx.Lock()
	defer r.mtx.Unlock()
	r.stereoWrites++
	r.lastStereoTime = time.Now()

	// 复用 write buffer，避免每次分配
	// 使用 (len(data) + 1) / 2 向上取整，确保当 data 长度为奇数时也能容纳所有索引
	expectedLen := (len(data) + 1) / 2
	if cap(r.writeBuffer) < expectedLen {
		r.writeBuffer = make([]uint16, expectedLen)
	} else {
		r.writeBuffer = r.writeBuffer[:expectedLen]
	}

	// 将 byte 数据转换为 uint16 slice
	for i := 0; i < len(data); i += 2 {
		var value = uint16(data[i])
		if i+1 < len(data) {
			value |= uint16(data[i+1]) << 8
		}
		r.writeBuffer[i/2] = value
	}
	r.stereo.Write(r.writeBuffer)
}

func (r *Recorder) Close() {
	r.cancel()
}

func (r *Recorder) Start(parent context.Context) {
	ctx, cancel := context.WithCancel(parent)
	r.cancel = cancel
	duration := ParseFrameDuration(r.codecOpt.FrameDuration)
	frameSize := GetSampleSize(r.codecOpt.SampleRate, r.codecOpt.BitDepth, 1) * int(duration.Milliseconds())

	ticker := time.NewTicker(duration)

	var numSamples uint32
	slog.Info("recorder: started",
		"filename", r.fileName,
		"codecOpt", r.codecOpt,
		"frameSize", frameSize,
		"duration", duration)
	defer func() {
		ticker.Stop()
		r.writer.Seek(0, io.SeekStart)
		wav.NewWriter(r.writer, numSamples, 2, uint32(r.codecOpt.SampleRate), uint16(r.codecOpt.BitDepth))
		if closer, ok := r.writer.(io.Closer); ok {
			closer.Close()
		}
		slog.Info("recorder: closed", "filename", r.fileName, "numSamples", numSamples)
	}()
	frameSize /= 2 // 2 bytes per sample

	// 预分配 samples slice 以避免每次 ticker 都分配
	if cap(r.samples) < frameSize {
		r.samples = make([]wav.Sample, frameSize)
	} else {
		r.samples = r.samples[:frameSize]
	}

	// 预分配临时 buffer 用于从 ringbuffer 读取数据
	monoBuffer := make([]uint16, frameSize)
	stereoBuffer := make([]uint16, frameSize)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.mtx.Lock()
			// 从 ringbuffer 读取数据
			monoCount := r.mono.Read(monoBuffer)
			stereoCount := r.stereo.Read(stereoBuffer)

			// 填充 samples
			for i := 0; i < frameSize; i++ {
				if i < monoCount {
					r.samples[i].Values[0] = int(monoBuffer[i])
				} else {
					r.samples[i].Values[0] = 0
				}
				if i < stereoCount {
					r.samples[i].Values[1] = int(stereoBuffer[i])
				} else {
					r.samples[i].Values[1] = 0
				}
			}
			numSamples += uint32(frameSize)
			r.mtx.Unlock()
			r.wavWriter.WriteSamples(r.samples)
		}
	}
}
