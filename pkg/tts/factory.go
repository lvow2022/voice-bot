package tts

import (
	"errors"

	"voicebot/pkg/tts/provider/minimax"
	"voicebot/pkg/tts/types"
)

// CreateProvider 根据配置创建 Provider
func CreateProvider(cfg types.EngineConfig) (types.Provider, error) {
	switch cfg.Name {
	case "minimax":
		return minimax.NewProvider(cfg)
	default:
		return nil, errors.New("unknown provider: " + cfg.Name)
	}
}

// RegisteredProviders 返回支持的 Provider 名称列表
func RegisteredProviders() []string {
	return []string{"minimax"}
}
