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
	stream   *stream.TtsStream
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
func (c *TTSController) Synthesize(ctx context.Context, text string, maxSize int) (*stream.TtsStream, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// 创建 session
	session, err := c.provider.NewSession(ctx)
	if err != nil {
		return nil, err
	}
	c.session = session

	// 创建 stream
	strm := stream.NewTtsStream(maxSize)
	c.stream = strm

	// 发送文本
	if err := session.SendText(text, nil); err != nil {
		return nil, err
	}

	// 启动 pump 协程
	go c.pumpAudio(ctx, session, strm)

	return strm, nil
}

// pumpAudio 从 session 读取音频并写入 stream
func (c *TTSController) pumpAudio(ctx context.Context, sess tts.Session, strm *stream.TtsStream) {
	defer func() {
		strm.Push(nil, true) // EOF
		c.mu.Lock()
		c.session = nil
		c.stream = nil
		c.mu.Unlock()
	}()

	audioStream := sess.RecvAudio()
	defer audioStream.Close()

	for audioStream.Next() {
		select {
		case <-ctx.Done():
			return
		case <-c.ctx.Done():
			return
		default:
			frame := audioStream.Frame()
			if len(frame.Data) > 0 {
				_ = strm.Push(frame.Data, frame.Final)
			}
			if frame.Final {
				return
			}
		}
	}

	if audioStream.Error() != nil {
		// todo log session error
	}
}

// Cancel 取消当前合成
func (c *TTSController) Cancel() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.session = nil
	if c.stream != nil {
		c.stream.Push(nil, true)
		c.stream = nil
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
