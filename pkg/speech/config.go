package speech

import "voicebot/pkg/stream"

// Config 调度器配置
type Config struct {
	// 队列配置
	MaxWaiting int // player 最大等待播放数 (default: 1)

	// 分句器配置
	MinSentenceLen int // 最小句子长度 (default: 6)
	MaxSentenceLen int // 最大句子长度 (default: 120)

	// 音频配置
	SampleRate int // 采样率 (default: 16000)
	Channels   int // 声道数 (default: 1)

	// Stream 配置
	MaxStreamSize int // 单个流最大缓冲 (default: 1MB)

	// 过滤器
	Filters []stream.Filter // 输入端过滤器（Push 时执行）
}

// DefaultConfig 默认配置
var DefaultConfig = Config{
	MaxWaiting:     1,
	MinSentenceLen:  6,
	MaxSentenceLen:  120,
	SampleRate:     16000,
	Channels:       1,
	MaxStreamSize:  1024 * 1024, // 1MB
}

// WindowSize 返回队列大小
func (c Config) WindowSize() int {
	return c.MaxWaiting + 2 // 预留一些空间
}
