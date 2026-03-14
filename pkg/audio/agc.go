package audio

import (
	"errors"
	"github.com/crazysunsir/webrtcagc/go"
	"unsafe"
)

// AgcMode
type AgcMode int16

const (
	ModeUnchanged       AgcMode = 0
	ModeAdaptiveAnalog  AgcMode = 1
	ModeAdaptiveDigital AgcMode = 2
	ModeFixedDigital    AgcMode = 3
)

// AGC Config
type AGCConfig struct {
	CompressionGaindB int16
	TargetLevelDbfs   int16
	LimiterEnable     bool
	Mode              AgcMode
}

// DefaultAGCConfig
func DefaultAGCConfig() AGCConfig {
	return AGCConfig{
		CompressionGaindB: 25,
		TargetLevelDbfs:   3,
		LimiterEnable:     true,
		Mode:              ModeAdaptiveDigital,
	}
}

type AGCProcessor struct {
	instance   *agc.AGC
	sampleRate uint32
	config     AGCConfig
}

func NewAGCProcessor(sampleRate uint32, config *AGCConfig) (*AGCProcessor, error) {
	cfg := DefaultAGCConfig()
	if config != nil {
		cfg = *config
	}

	agcConfig := &agc.Config{
		CompressionGaindB: cfg.CompressionGaindB,
		TargetLevelDbfs:   cfg.TargetLevelDbfs,
		LimiterEnable:     cfg.LimiterEnable,
		Mode:              agc.AgcMode(cfg.Mode),
	}

	instance, err := agc.NewAGC(sampleRate, agcConfig)
	if err != nil {
		return nil, err
	}

	return &AGCProcessor{
		instance:   instance,
		sampleRate: sampleRate,
		config:     cfg,
	}, nil
}

func (a *AGCProcessor) Close() error {
	if a.instance != nil {
		a.instance.Close()
		a.instance = nil
	}
	return nil
}

func (a *AGCProcessor) Process(audioData []byte) error {
	if a.instance == nil {
		return errors.New("AGC instance is nil")
	}

	if len(audioData)%2 != 0 {
		return errors.New("audio data length must be even for 16-bit samples")
	}
	if len(audioData) == 0 {
		return nil
	}

	samples := unsafe.Slice(
		(*int16)(unsafe.Pointer(&audioData[0])),
		len(audioData)/2,
	)

	err := a.instance.Process(samples)
	if err != nil {
		return err
	}

	return nil
}
