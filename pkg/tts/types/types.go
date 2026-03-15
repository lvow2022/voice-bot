package types

import (
	"context"
)

// Engine TTS 引擎
//type Engine interface {
//	NewSession(ctx context.Context, output stream.Stream) (Session, error)
//	Close() error
//}
//
//// Session TTS 会话
//type Session interface {
//	SendText(text string, options map[string]any) error
//	Done() <-chan struct{} // session 结束时关闭
//	Close() error
//}

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

type Provider interface {
	Connect(ctx context.Context, opts SessionOptions) error
	SendText(text string, options map[string]any) error
	RecvEvent() (*TtsEvent, error)
	Close() error
}

type SessionOptions struct {
	SampleRate int    `json:"sampleRate" yaml:"sample_rate"`
	Format     string `json:"format" yaml:"format"`
	Channels   int    `json:"channels" yaml:"channels"`
	EnableITN  bool   `json:"enableItn" yaml:"enable_itn"`
	EnablePunc bool   `json:"enablePunc" yaml:"enable_punc"`
	Language   string `json:"language" yaml:"language"`
}

// DefaultSessionOptions 返回默认会话配置
func DefaultSessionOptions() SessionOptions {
	return SessionOptions{
		SampleRate: 16000,
		Format:     "pcm",
		Channels:   1,
		EnableITN:  true,
		EnablePunc: true,
		Language:   "zh-CN",
	}
}

type TtsEventType int

const (
	EventAudioChunk TtsEventType = iota
	EventCompleted
	EventError
)

type TtsEvent struct {
	Type    TtsEventType
	Data    []byte // 音频块
	IsFinal bool
	Err     error
}

// ProviderConfig provider 配置
type ProviderConfig struct {
	Name    string         // provider 名称
	Model   string         // 模型名称
	URL     string         // 服务地址 (可选，使用默认)
	APIKey  string         // API Key
	VoiceID string         // 音色 ID
	Speed   float64        // 语速
	Options map[string]any // 扩展配置
}

// ClientConfig 客户端配置
type ClientConfig struct {
	Primary  ProviderConfig  // 主 provider
	Fallback *ProviderConfig // 备用 provider (可选)
	Session  SessionOptions  // 会话配置
}
