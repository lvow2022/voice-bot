package asr

import (
	"errors"

	"voicebot/pkg/asr/provider/volc"
	"voicebot/pkg/asr/types"
)

// CreateProvider 根据配置创建 provider
func CreateProvider(cfg types.ProviderConfig) (types.Provider, error) {
	switch cfg.Name {
	case "volcano":
		return volc.NewVolcanoAdapter(cfg)
	default:
		return nil, errors.New("unknown provider: " + cfg.Name)
	}
}

// RegisteredProviders 返回支持的 provider 名称列表
func RegisteredProviders() []string {
	return []string{"volcano"}
}
