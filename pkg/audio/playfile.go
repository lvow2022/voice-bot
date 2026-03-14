package audio

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/hajimehoshi/go-mp3"
	"github.com/youpy/go-wav"
	"voicebot/pkg/codecs"
	"voicebot/pkg/voicechain"
)

var ErrUnsupportedAudioFormat = errors.New("unsupported audio format")
var ErrUnsupportedSchema = errors.New("unsupported schema")

var UserAgent = "voicefoxai/voiceserver"

type AudioStreamFormat struct {
	SampleRate    int
	BitDepth      int
	Channels      int
	FrameDuration time.Duration
}

type AudioPlayer struct {
	cancel context.CancelFunc

	Stream        io.Reader
	Format        AudioStreamFormat
	ToOutput      bool
	IsSynthesized bool
	PlayID        string
	Sequence      int
}

func PlayFile(filename string, frameDuration time.Duration, toOutput bool) (voicechain.HandleFunc, error) {
	ap, err := LoadFromFile(filename, frameDuration, toOutput)
	if err != nil {
		return nil, err
	}
	return func(h voicechain.SessionHandler, data voicechain.SessionData) {
		if data.Type == voicechain.SessionDataState {
			ap.handleState(h, data.State)
			return
		}
	}, nil
}

func LoadFromFile(filename string, frameDuration time.Duration, toOutput bool) (*AudioPlayer, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}

	ap := &AudioPlayer{
		Format: AudioStreamFormat{
			SampleRate:    16000,
			Channels:      1,
			BitDepth:      16,
			FrameDuration: frameDuration,
		},
		ToOutput: toOutput,
		PlayID:   filename,
	}
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".pcm", ".raw":
		ap.Stream = file
	case ".wav":
		w := wav.NewReader(file)
		f, err := w.Format()
		if err != nil {
			panic(err)
		}
		ap.Stream = w
		ap.Format.Channels = int(f.NumChannels)
		ap.Format.SampleRate = int(f.SampleRate)
		ap.Format.BitDepth = int(f.BitsPerSample)
	case ".mp3":
		m, err := mp3.NewDecoder(file)
		if err != nil {
			panic(err)
		}
		ap.Stream = m
		ap.Format.SampleRate = int(m.SampleRate())
	default:
		panic("file format not supported, only support [mp3, wav]")
	}
	return ap, nil
}

func (f AudioStreamFormat) GetFrameSize(frameDuration time.Duration) int {
	return GetSampleSize(f.SampleRate, f.BitDepth, f.Channels) * int(frameDuration.Milliseconds())
}

func (ap *AudioPlayer) String() string {
	return fmt.Sprintf("AudioPlayer{samplerate: %d, channels: %d, bitdepth: %d, frameDuration: %s, isSynthesized: %t}",
		ap.Format.SampleRate, ap.Format.Channels, ap.Format.BitDepth, ap.Format.FrameDuration, ap.IsSynthesized)
}

func (ap *AudioPlayer) handleState(h voicechain.SessionHandler, state voicechain.StateEvent) {
	switch state.State {
	case voicechain.Hangup, voicechain.End:
		ap.Close()
	case voicechain.Begin:
		go ap.StartPlay(h, h.GetContext())
	case voicechain.Interruption:
		ap.Close()
	}
}

func (ap *AudioPlayer) Close() error {
	slog.Info("AudioPlayer: close", "ap", ap, "playID", ap.PlayID)
	if ap.cancel != nil {
		ap.cancel()
		ap.cancel = nil
	}
	return nil
}

func (ap *AudioPlayer) checkResample(targetSamplerate int) {
	if ap.Format.SampleRate == targetSamplerate {
		return
	}

	st := time.Now()
	buf, err := io.ReadAll(ap.Stream)
	if err != nil {
		return
	}

	buf, err = codecs.ResamplePCM(buf, ap.Format.SampleRate, targetSamplerate)
	if err != nil {
		return
	}
	ap.Stream = bytes.NewReader(buf)
	slog.Info("AudioPlayer: resample audio",
		"ap", ap,
		"targetSampleRate", targetSamplerate,
		"duration", time.Since(st),
		"size", len(buf))
	ap.Format.SampleRate = targetSamplerate
}

func (ap *AudioPlayer) StartPlay(h voicechain.SessionHandler, ctx context.Context) {
	ticker := time.NewTicker(ap.Format.FrameDuration)
	defer ticker.Stop()
	playCtx, cancel := context.WithCancel(ctx)
	ap.cancel = cancel
	defer cancel()

	targetSampleRate := h.GetSession().Codec().SampleRate
	ap.checkResample(targetSampleRate)

	frameSize := ap.Format.GetFrameSize(ap.Format.FrameDuration)
	frame := make([]byte, frameSize)
	startTime := time.Now()

	slog.Info("AudioPlayer: start play",
		"sessionID", h.GetSession().ID,
		"ap", ap,
		"targetSampleRate", targetSampleRate,
		"frameSize", frameSize,
		"frameDuration", ap.Format.FrameDuration,
		"playID", ap.PlayID)
	firstFrame := true
	h.EmitState(ap, voicechain.StartPlay, ap.PlayID)

	for {
		select {
		case <-ticker.C:
			n, err := ap.Stream.Read(frame)
			if err != nil {
				if err == io.EOF {
					slog.Info("AudioPlayer: play completed",
						"sessionID", h.GetSession().ID,
						"ap", ap,
						"duration", time.Since(startTime),
						"playID", ap.PlayID)
					h.EmitState(ap, voicechain.StopPlay, "play.file", voicechain.PlayStateData{}, time.Since(startTime).String(), ap.PlayID, ap.Sequence)
				}
				return
			}
			audioFrame := voicechain.AudioFrame{
				Payload:       frame[:n],
				IsSynthesized: ap.IsSynthesized,
				IsFirstFrame:  firstFrame,
			}
			firstFrame = false
			if ap.ToOutput {
				h.SendToOutput(ap, &audioFrame)
			} else {
				h.EmitFrame(ap, &audioFrame)
			}
		case <-playCtx.Done():
			h.EmitState(ap, voicechain.StopPlay, "play.file", voicechain.PlayStateData{}, time.Since(startTime).String(), ap.PlayID, ap.Sequence)
			return
		}
	}
}

func (p *AudioPlayer) LoadFromStream(ctx context.Context, message string) error {
	u, err := url.Parse(message)
	if err != nil {
		return err
	}
	schema := strings.ToLower(u.Scheme)
	ext := strings.ToLower(filepath.Ext(u.Path))

	switch ext {
	case ".mp3", ".wav", ".pcm":
	default:
		return ErrUnsupportedAudioFormat
	}
	switch schema {
	case "http", "https":
		p.Stream, p.Format, err = p.getHTTPAudioStream(ctx, message, ext)
	case "file":
		p.Stream, p.Format, err = p.getFileAudioStream(strings.TrimPrefix(message, "file://"), ext)
	default:
		return ErrUnsupportedSchema
	}
	return err
}

func (p *AudioPlayer) getFileAudioStream(filename, ext string) (io.Reader, AudioStreamFormat, error) {
	slog.Info("AudioPlayer: get stream from file", "filename", filename, "ext", ext, "p", p)
	body, err := os.ReadFile(filename)
	if err != nil {
		return nil, AudioStreamFormat{}, err
	}
	return p.getAudioStream(body, ext)
}

func (p *AudioPlayer) getHTTPAudioStream(ctx context.Context, url, ext string) (stream io.Reader, format AudioStreamFormat, err error) {
	slog.Info("AudioPlayer: get stream from http", "url", url, "ext", ext)
	cacheKey := MediaCache().BuildKey(url) + ext
	body, err := MediaCache().Get(cacheKey)
	if err == nil {
		return p.getAudioStream(body, ext)
	}

	st := time.Now()
	var req *http.Request
	req, err = http.NewRequest("GET", url, nil)
	if err != nil {
		slog.Error("say: http request failed", "url", url, "error", err)
		return
	}
	req.Header.Set("User-Agent", UserAgent)
	var resp *http.Response
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		slog.Error("say: http request failed", "url", url, "error", err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		err = fmt.Errorf("http request failed: %s %s", resp.Status, url)
		slog.Error("say: http request failed", "url", url, "status", resp.Status)
		return
	}

	body, err = io.ReadAll(resp.Body)
	if err != nil {
		slog.Error("say: read http response failed", "url", url, "error", err)
		return
	}
	slog.Info("say: http request success", "url", url, "filesize", len(body), "duration", time.Since(st))
	MediaCache().Store(cacheKey, body)
	return p.getAudioStream(body, ext)
}

func (p *AudioPlayer) getAudioStream(body []byte, ext string) (stream io.Reader, format AudioStreamFormat, err error) {
	switch ext {
	case ".mp3":
		m, err := mp3.NewDecoder(bytes.NewReader(body))
		if err != nil {
			return nil, AudioStreamFormat{}, err
		}
		format = AudioStreamFormat{
			SampleRate: m.SampleRate(),
			BitDepth:   16,
			Channels:   1,
		}
		stream = m
	case ".wav":
		w := wav.NewReader(bytes.NewReader(body))
		wf, err := w.Format()
		if err != nil {
			return nil, AudioStreamFormat{}, err
		}
		format = AudioStreamFormat{
			SampleRate: int(wf.SampleRate),
			BitDepth:   int(wf.BitsPerSample),
			Channels:   int(wf.NumChannels),
		}
		stream = w
	default:
		format = AudioStreamFormat{
			SampleRate: 16000,
			BitDepth:   16,
			Channels:   1,
		}
		stream = bytes.NewReader(body)
	}
	return
}
