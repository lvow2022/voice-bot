package tts

import (
	"context"

	"voicebot/pkg/stream"
)

// Engine TTS 引擎
type Engine interface {
	NewSession(ctx context.Context, output stream.Stream) (Session, error)
	Close() error
}

// Session TTS 会话
type Session interface {
	SendText(text string, options map[string]any) error
	Done() <-chan struct{} // session 结束时关闭
	Close() error
}
