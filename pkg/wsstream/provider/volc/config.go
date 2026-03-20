package volc

import (
	"voicebot/pkg/asr/types"
)

// Config 火山引擎 ASR 配置
type Config struct {
	AppID      string
	APIKey     string
	ResourceID string
	SampleRate int
	Format     string
	Options    map[string]any
}

// ParseConfig 从 ProviderConfig 解析配置
func ParseConfig(cfg types.ProviderConfig) Config {
	return Config{
		AppID:      cfg.AppID,
		APIKey:     cfg.APIKey,
		ResourceID: cfg.ResourceID,
		SampleRate: cfg.SampleRate,
		Format:     cfg.Format,
		Options:    cfg.Options,
	}
}
