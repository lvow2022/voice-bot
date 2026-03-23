package server

import (
	asrtypes "voicebot/pkg/asr/types"
	"voicebot/pkg/audio"
	ttstypes "voicebot/pkg/tts/types"
)

// PipelineConfig Pipeline 配置
type PipelineConfig struct {
	// ASR 配置
	ASR asrtypes.ProviderConfig `json:"asr"`

	// TTS 配置
	TTS ttstypes.ClientConfig `json:"tts"`

	// VAD 配置
	VADType   string                `json:"vadType"`   // "silero"
	VADOption audio.VADDetectorOption `json:"vadOption"`
}

// DefaultPipelineConfig 返回默认 Pipeline 配置
func DefaultPipelineConfig() *PipelineConfig {
	return &PipelineConfig{
		ASR: asrtypes.ProviderConfig{
			Name:       "volcano",
			SampleRate: 16000,
			Format:     "pcm",
		},
		TTS: ttstypes.ClientConfig{
			Primary: ttstypes.ProviderConfig{
				Name: "minimax",
			},
			Session: ttstypes.DefaultSessionOptions(),
		},
		VADType:   string(audio.VADTypeSilero),
		VADOption: audio.DefaultVADDetectorOption(),
	}
}
