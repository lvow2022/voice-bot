package device

import (
	"fmt"
	"sync"

	"github.com/gen2brain/malgo"
)

type DeviceConfig struct {
	SampleRate int
	Channels   int
	PeriodMs   int
}

// MalgoDevice malgo 音频设备实现
type MalgoDevice struct {
	config  DeviceConfig
	ctx     *malgo.AllocatedContext
	device  *malgo.Device
	buffer  []byte
	bufSize int
	bufMu   sync.RWMutex
	once    sync.Once
}

func NewMalgoDevice(config DeviceConfig) (*MalgoDevice, error) {
	ctx, err := malgo.InitContext(nil, malgo.ContextConfig{}, nil)
	if err != nil {
		return nil, fmt.Errorf("init malgo context: %w", err)
	}

	if config.PeriodMs == 0 {
		config.PeriodMs = 20
	}
	frameSize := config.SampleRate * config.Channels * 2 * config.PeriodMs / 1000

	return &MalgoDevice{
		config: config,
		ctx:    ctx,
		buffer: make([]byte, frameSize),
	}, nil
}

func (d *MalgoDevice) Start() error {
	if err := d.initDevice(); err != nil {
		return err
	}
	if err := d.device.Start(); err != nil {
		return fmt.Errorf("start device: %w", err)
	}
	return nil
}

func (d *MalgoDevice) initDevice() error {
	config := malgo.DefaultDeviceConfig(malgo.Playback)
	config.Playback.Format = malgo.FormatS16
	config.Playback.Channels = uint32(d.config.Channels)
	config.SampleRate = uint32(d.config.SampleRate)
	config.PeriodSizeInMilliseconds = uint32(d.config.PeriodMs)

	callbacks := malgo.DeviceCallbacks{
		Data: func(output, _ []byte, framecount uint32) {
			d.bufMu.RLock()
			n := d.bufSize
			if n > len(output) {
				n = len(output)
			}
			copy(output, d.buffer[:n])
			d.bufMu.RUnlock()

			for i := n; i < len(output); i++ {
				output[i] = 0
			}
		},
	}

	device, err := malgo.InitDevice(d.ctx.Context, config, callbacks)
	if err != nil {
		return fmt.Errorf("init device: %w", err)
	}

	d.device = device
	return nil
}

func (d *MalgoDevice) Stop() error {
	if d.device != nil {
		return d.device.Stop()
	}
	return nil
}

func (d *MalgoDevice) Close() error {
	d.once.Do(func() {
		if d.device != nil {
			d.device.Uninit()
		}
		if d.ctx != nil {
			d.ctx.Uninit()
		}
	})
	return nil
}

func (d *MalgoDevice) Write(data []byte) error {
	d.bufMu.Lock()
	n := copy(d.buffer, data)
	d.bufSize = n
	d.bufMu.Unlock()
	return nil
}
