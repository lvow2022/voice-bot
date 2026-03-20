package minimax

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"time"

	"github.com/gorilla/websocket"

	"voicebot/pkg/tts/types"
	"voicebot/pkg/wsstream"
)

// ============ Provider ============

// Provider Minimax TTS Provider（基于 wsstream）
type Provider struct {
	cfg    Config
	codec  *Codec
	stream *wsstream.WSStream
}

// NewProvider 创建 Provider
func NewProvider(cfg types.ProviderConfig) (*Provider, error) {
	config := ParseConfig(cfg)

	if config.APIKey == "" {
		return nil, fmt.Errorf("api key required")
	}

	return &Provider{
		cfg: config,
	}, nil
}

// Connect 连接并发送 task_start
func (p *Provider) Connect(ctx context.Context, opts types.SessionOptions) error {
	// 更新配置
	if opts.SampleRate > 0 {
		p.cfg.SampleRate = opts.SampleRate
	}
	if opts.Format != "" {
		p.cfg.Format = opts.Format
	}
	if opts.Channels > 0 {
		p.cfg.Channels = opts.Channels
	}

	// 建立 WebSocket 连接
	h := http.Header{}
	h.Set("Authorization", fmt.Sprintf("Bearer %s", p.cfg.APIKey))

	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
		TLSClientConfig:  &tls.Config{InsecureSkipVerify: true},
	}

	conn, _, err := dialer.DialContext(ctx, p.cfg.URL, h)
	if err != nil {
		return fmt.Errorf("dial websocket: %w", err)
	}

	// 创建 codec
	p.codec = NewCodec(p.cfg)

	// 发送 task_start
	startData, err := p.codec.EncodeStart()
	if err != nil {
		_ = conn.Close()
		return fmt.Errorf("encode start: %w", err)
	}

	if err := conn.WriteMessage(websocket.TextMessage, startData); err != nil {
		_ = conn.Close()
		return fmt.Errorf("write start: %w", err)
	}

	// 创建流
	p.stream = wsstream.NewWSStream(
		wsstream.NewWSConn(conn),
		p.codec,
		wsstream.WithSendBufferSize(128),
		wsstream.WithRecvBufferSize(128),
	)

	return nil
}

// SendText 发送文本
func (p *Provider) SendText(text string, _ map[string]any) error {
	if p.stream == nil {
		return fmt.Errorf("not connected")
	}

	return p.stream.Send(context.Background(), TTSRequest{
		Text:   text,
		IsLast: false,
	})
}

// RecvEvent 接收事件
func (p *Provider) RecvEvent() (*types.TtsEvent, error) {
	if p.stream == nil {
		return nil, fmt.Errorf("not connected")
	}

	select {
	case <-p.stream.Done():
		if err := p.stream.Err(); err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("stream closed")
	case evt, ok := <-p.stream.Recv():
		if !ok {
			return nil, fmt.Errorf("stream closed")
		}

		// 处理错误类型
		if err, ok := evt.(error); ok {
			return &types.TtsEvent{
				Type: types.EventError,
				Err:  err,
			}, nil
		}

		// 类型断言为 TtsEvent
		ttsEvt, ok := evt.(TtsEvent)
		if !ok {
			return nil, nil
		}

		// 转换为 types.TtsEvent
		switch ttsEvt.Type {
		case TtsEventAudio:
			return &types.TtsEvent{
				Type: types.EventAudioChunk,
				Data: ttsEvt.Audio,
			}, nil

		case TtsEventFinal:
			return &types.TtsEvent{
				Type: types.EventCompleted,
			}, nil

		case TtsEventError:
			return &types.TtsEvent{
				Type: types.EventError,
				Err:  ttsEvt.Err,
			}, nil

		default:
			return nil, nil
		}
	}
}

// Close 关闭连接
func (p *Provider) Close() error {
	if p.stream == nil {
		return nil
	}
	return p.stream.Close()
}

// SendEOF 发送结束标记（half-close）
func (p *Provider) SendEOF() error {
	if p.stream == nil {
		return fmt.Errorf("not connected")
	}

	return p.stream.Send(context.Background(), TTSRequest{
		IsLast: true,
	})
}
