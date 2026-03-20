package asr

import (
	"errors"

	"voicebot/pkg/asr/types"
	wsvolc "voicebot/pkg/websocket/provider/volc"
)

// CreateProvider 根据配置创建 provider
func CreateProvider(cfg types.ProviderConfig) (types.Provider, error) {
	switch cfg.Name {
	case "volcano":
		return wsvolc.NewProvider(cfg)
	default:
		return nil, errors.New("unknown provider: " + cfg.Name)
	}
}

// RegisteredProviders 返回支持的 provider 名称列表
func RegisteredProviders() []string {
	return []string{"volcano"}
}
