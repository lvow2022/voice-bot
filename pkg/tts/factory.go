package tts

import (
	"errors"
)

// EngineConfig provider 配置
type EngineConfig struct {
	Name       string
	Model      string
	URL        string
	APIKey     string
	VoiceID    string
	Speed      float64
	SampleRate int
	Options    map[string]any // 扩展配置
}

// CreateEngine 根据配置创建引擎
func CreateEngine(cfg EngineConfig) (Engine, error) {
	switch cfg.Name {
	case "minimax":
		return NewMinimaxEngine(cfg)
	default:
		return nil, errors.New("unknown engine: " + cfg.Name)
	}
}

// RegisteredEngines 返回支持的引擎名称列表
func RegisteredEngines() []string {
	return []string{"minimax"}
}
