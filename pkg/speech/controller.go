package speech

import (
	"context"
	"sync"

	"voicebot/pkg/stream"
	"voicebot/pkg/tts"
)

// TTSController TTS 控制器
type TTSController struct {
	provider tts.Engine
	session  tts.Session
	mu       sync.Mutex
	ctx      context.Context
	cancel   context.CancelFunc
}

// NewTTSController 创建 TTS 控制器
func NewTTSController(provider tts.Engine) *TTSController {
	ctx, cancel := context.WithCancel(context.Background())
	return &TTSController{
		provider: provider,
		ctx:      ctx,
		cancel:   cancel,
	}
}

// Synthesize 合成语音
func (c *TTSController) Synthesize(ctx context.Context, text string, filters ...stream.Filter) (stream.Stream, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// 创建 stream 并配置过滤器
	audioStream := stream.NewAudioStream(0)
	audioStream.PushFilter(filters...)

	// 创建 session，注入 stream
	session, err := c.provider.NewSession(ctx, audioStream)
	if err != nil {
		return nil, err
	}
	c.session = session

	// 发送文本
	if err := session.SendText(text, nil); err != nil {
		session.Close()
		c.session = nil
		return nil, err
	}

	return audioStream, nil
}

// Cancel 取消当前合成
func (c *TTSController) Cancel() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.session != nil {
		c.session.Close()
		c.session = nil
	}
}

// Close 关闭控制器
func (c *TTSController) Close() error {
	c.Cancel()
	c.cancel()
	return c.provider.Close()
}

// IsBusy 是否正在合成
func (c *TTSController) IsBusy() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.session != nil
}
