package tts

import "context"

const (
	ErrSessionClosed = "Session closed"
	ErrSessionEOF    = "Session eof"
)

// AudioFrame 音频帧
type AudioFrame struct {
	Data  []byte
	Final bool
}

// AudioStream 音频流迭代器
type AudioStream interface {
	Next() bool
	Frame() AudioFrame
	Error() error
	Close() error
}

// Provider TTS 提供者
type Engine interface {
	NewSession(ctx context.Context) (Session, error)
	Close() error
}

// Session TTS 会话
type Session interface {
	SendText(text string, options map[string]any) error
	RecvAudio() AudioStream
}
