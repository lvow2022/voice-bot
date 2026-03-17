// Package pipeline provides voicechain pipeline filters for audio processing.
package pipeline

import (
	"voicebot/pkg/audio"
	"voicebot/pkg/dfn"
)

// NoiseSuppressionOption 降噪处理器配置
type NoiseSuppressionOption struct {
	// Enabled 是否启用降噪
	Enabled bool `json:"enabled" yaml:"enabled" default:"true"`

	// Type 降噪类型：rnnoise 或 dfn(onnx)
	Type audio.NoiseSuppressionType `json:"type" yaml:"type" default:"dfn"`

	// Intensity 降噪强度 (1-30), 值越大降噪越强
	Intensity int `json:"intensity" yaml:"intensity" default:"5"`

	// DFNConfig ONNX 降噪器配置（仅 Type=dfn 时生效）
	DFNConfig dfn.SuppressorConfig `json:"dfnConfig" yaml:"dfn_config"`
}

// DefaultNoiseSuppressionOption 返回默认降噪配置
func DefaultNoiseSuppressionOption() NoiseSuppressionOption {
	return NoiseSuppressionOption{
		Enabled:   true,
		Type:      audio.NoiseSuppressionTypeONNX,
		Intensity: 5,
		DFNConfig: dfn.DefaultSuppressorConfig(),
	}
}
