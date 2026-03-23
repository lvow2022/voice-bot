package server

import (
	"context"
	"io"
	"log/slog"
	"sync"

	"github.com/gorilla/websocket"
	"voicebot/pkg/voicechain"
)

// WSTransport 实现 voicechain.Transport 接口
type WSTransport struct {
	conn      *websocket.Conn
	codec     voicechain.CodecOption
	sendQueue chan voicechain.Frame

	mu      sync.Mutex
	closed  bool
	closeCh chan struct{}
}

// NewWSTransport 创建 WebSocket Transport
func NewWSTransport(conn *websocket.Conn, sampleRate int) *WSTransport {
	if sampleRate == 0 {
		sampleRate = 16000
	}
	return &WSTransport{
		conn: conn,
		codec: voicechain.CodecOption{
			Codec:      "pcm",
			SampleRate: sampleRate,
			Channels:   1,
			BitDepth:   16,
		},
		sendQueue: make(chan voicechain.Frame, 128),
		closeCh:   make(chan struct{}),
	}
}

// Attach 实现 Transport 接口
func (t *WSTransport) Attach(_ *voicechain.Session) {}

// String 实现 Transport 接口
func (t *WSTransport) String() string {
	return "WSTransport"
}

// Codec 实现 Transport 接口
func (t *WSTransport) Codec() voicechain.CodecOption {
	return t.codec
}

// Next 实现 Transport 接口，从 WebSocket 读取消息
func (t *WSTransport) Next(ctx context.Context) (voicechain.Frame, error) {
	for {
		select {
		case <-ctx.Done():
			return nil, io.EOF
		case <-t.closeCh:
			return nil, io.EOF
		default:
		}

		messageType, data, err := t.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				slog.Debug("websocket read error", "error", err)
			}
			return nil, io.EOF
		}

		switch messageType {
		case websocket.BinaryMessage:
			// 音频数据
			return &voicechain.AudioFrame{
				Payload: data,
			}, nil

		case websocket.TextMessage:
			// 控制消息
			msg, err := DecodeClientMessage(data)
			if err != nil {
				slog.Warn("invalid client message", "error", err)
				continue
			}

			switch msg.Type {
			case "interrupt":
				return &voicechain.InterruptFrame{}, nil
			case "dtmf":
				return &voicechain.DtmfFrame{Event: msg.Key}, nil
			}
		}
	}
}

// Send 实现 Transport 接口，发送消息到 WebSocket
func (t *WSTransport) Send(_ context.Context, frame voicechain.Frame) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.closed {
		return 0, io.EOF
	}

	select {
	case t.sendQueue <- frame:
		return 1, nil
	default:
		// 队列满，丢弃帧
		slog.Warn("send queue full, frame dropped")
		return 0, nil
	}
}

// StartSendLoop 启动发送循环
func (t *WSTransport) StartSendLoop(ctx context.Context) {
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.closeCh:
				return
			case frame := <-t.sendQueue:
				if err := t.writeFrame(frame); err != nil {
					slog.Debug("websocket write error", "error", err)
					return
				}
			}
		}
	}()
}

// writeFrame 写入帧到 WebSocket
func (t *WSTransport) writeFrame(frame voicechain.Frame) error {
	switch f := frame.(type) {
	case *voicechain.AudioFrame:
		return t.conn.WriteMessage(websocket.BinaryMessage, f.Payload)

	case *voicechain.TextFrame:
		msg := FrameToMessage(f)
		if msg == nil {
			return nil
		}
		data, err := EncodeServerMessage(msg)
		if err != nil {
			return err
		}
		return t.conn.WriteMessage(websocket.TextMessage, data)

	case *voicechain.CloseFrame:
		msg := &ServerMessage{Type: "state", State: "closed", Payload: f.Reason}
		data, _ := EncodeServerMessage(msg)
		t.conn.WriteMessage(websocket.TextMessage, data)
		return t.conn.WriteMessage(websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, f.Reason))

	default:
		msg := FrameToMessage(frame)
		if msg == nil {
			return nil
		}
		data, err := EncodeServerMessage(msg)
		if err != nil {
			return err
		}
		return t.conn.WriteMessage(websocket.TextMessage, data)
	}
}

// SendReady 发送就绪消息
func (t *WSTransport) SendReady() error {
	msg := &ServerMessage{Type: "ready"}
	data, err := EncodeServerMessage(msg)
	if err != nil {
		return err
	}
	return t.conn.WriteMessage(websocket.TextMessage, data)
}

// SendError 发送错误消息
func (t *WSTransport) SendError(code, message string) error {
	msg := &ServerMessage{Type: "error", Code: code, Message: message}
	data, err := EncodeServerMessage(msg)
	if err != nil {
		return err
	}
	return t.conn.WriteMessage(websocket.TextMessage, data)
}

// Close 实现 io.Closer 接口
func (t *WSTransport) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.closed {
		return nil
	}
	t.closed = true
	close(t.closeCh)

	return t.conn.Close()
}
