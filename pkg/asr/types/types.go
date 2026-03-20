package types

import (
	"context"
	"time"

	"voicebot/pkg/websocket"
)

// Provider 协议适配器接口，负责建立连接并返回 WSStream
type Provider interface {
	// Connect 建立连接，返回 WSStream 供调用方使用
	Connect(ctx context.Context, opts SessionOptions) (*websocket.WSStream, error)
}

// SessionOptions 会话配置
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

// AsrEvent 识别事件
type AsrEvent struct {
	Type       AsrEventType
	Text       string
	Confidence float32
	Err        error
}

// IsFinal 实现 websocket.FinalEvent 接口
func (e AsrEvent) IsFinal() bool {
	return e.Type == EventFinal
}

// AsrEventType 事件类型
type AsrEventType int

const (
	EventPartial AsrEventType = iota
	EventFinal
	EventError
	EventReconnecting
	EventReconnected
)

func (t AsrEventType) String() string {
	switch t {
	case EventPartial:
		return "partial"
	case EventFinal:
		return "final"
	case EventError:
		return "error"
	case EventReconnecting:
		return "reconnecting"
	case EventReconnected:
		return "reconnected"
	default:
		return "unknown"
	}
}

// AsrRequest ASR 请求
type AsrRequest struct {
	Audio  []byte
	IsLast bool // half-close 标记
}

// ProviderType provider 类型
type ProviderType string

const (
	ProviderVolcano ProviderType = "volcano"
)

// ProviderConfig provider 配置
type ProviderConfig struct {
	Name       string         // provider 名称
	URL        string         // 服务地址
	APIKey     string         // API Key
	AppID      string         // 应用 ID
	ResourceID string         // 资源 ID
	SampleRate int            // 采样率
	Format     string         // 音频格式
	Options    map[string]any // 扩展配置
}

// RetryConfig 重试配置
type RetryConfig struct {
	MaxAttempts  int           // 最大重试次数
	InitialDelay time.Duration // 初始重试延迟
	MaxDelay     time.Duration // 最大重试延迟
	Multiplier   float64       // 延迟倍数
}

// DefaultRetryConfig 默认重试配置
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxAttempts:  3,
		InitialDelay: 100 * time.Millisecond,
		MaxDelay:     5 * time.Second,
		Multiplier:   2.0,
	}
}

// ClientConfig 客户端配置
type ClientConfig struct {
	Primary  ProviderConfig  // 主 provider
	Fallback *ProviderConfig // 备用 provider (可选)
	Session  SessionOptions  // 会话配置
	Retry    RetryConfig     // 重试配置

	// 框架层配置
	EventBufferSize int           // 事件通道缓冲区大小
	WriteBufferSize int           // 写缓冲区大小
	ConnectTimeout  time.Duration // 连接超时
}

// DefaultClientConfig 返回默认客户端配置
func DefaultClientConfig() ClientConfig {
	return ClientConfig{
		Session:         DefaultSessionOptions(),
		Retry:           DefaultRetryConfig(),
		EventBufferSize: 100,
		WriteBufferSize: 100,
		ConnectTimeout:  10 * time.Second,
	}
}
