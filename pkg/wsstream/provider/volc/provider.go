package volc

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/gorilla/websocket"

	"voicebot/pkg/asr/types"
	"voicebot/pkg/wsstream"
)

// ============ Provider ============

// Provider 火山引擎 ASR Provider（基于 wsstream)
type Provider struct {
	cfg    Config
	codec  *Codec
	stream *wsstream.WSStream
}

// NewProvider 创建 Provider
func NewProvider(cfg types.ProviderConfig) (*Provider, error) {
	config := ParseConfig(cfg)

	return &Provider{
		cfg: config,
	}, nil
}

// Connect 连接
func (p *Provider) Connect(ctx context.Context, opts types.SessionOptions) error {
	// 更新配置
	if opts.SampleRate > 0 {
		p.cfg.SampleRate = opts.SampleRate
	}
	if opts.Format != "" {
		p.cfg.Format = opts.Format
	}

	// 生成 connect ID
	connectID := generateUUID()

	// 建立 WebSocket 连接
	headers := http.Header{}
	headers.Set("X-Api-App-Key", p.cfg.AppID)
	headers.Set("X-Api-Access-Key", p.cfg.APIKey)
	headers.Set("X-Api-Resource-Id", p.cfg.ResourceID)
	headers.Set("X-Api-Connect-Id", connectID)

	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}

	conn, _, err := dialer.DialContext(ctx, volcanoWSURL, headers)
	if err != nil {
		return fmt.Errorf("dial volcano asr: %w", err)
	}

	// 创建 codec
	p.codec = NewCodec(p.cfg)

	// 发送 full client request
	fullReq, err := p.codec.EncodeFullClientRequest()
	if err != nil {
		_ = conn.Close()
		return fmt.Errorf("encode full client request: %w", err)
	}

	if err := conn.WriteMessage(websocket.BinaryMessage, fullReq); err != nil {
		_ = conn.Close()
		return fmt.Errorf("write full client request: %w", err)
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

// SendAudio 发送音频
func (p *Provider) SendAudio(data []byte, isLast bool) error {
	if p.stream == nil {
		return fmt.Errorf("not connected")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	return p.stream.Send(ctx, AsrRequest{
		Audio:  data,
		IsLast: isLast,
	})
}

// RecvEvent 接收事件
func (p *Provider) RecvEvent() (*types.AsrEvent, error) {
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
			return &types.AsrEvent{
				Type: types.EventError,
				Text: err.Error(),
			}, nil
		}

		// 类型断言为 AsrEvent
		asrEvt, ok := evt.(AsrEvent)
		if !ok {
			return nil, nil
		}

		// 转换为 types.AsrEvent
		switch asrEvt.Type {
		case AsrEventPartial:
			return &types.AsrEvent{
				Type:       types.EventPartial,
				Text:       asrEvt.Text,
				Confidence: float32(asrEvt.Confidence),
				IsFinal:    false,
			}, nil

		case AsrEventFinal:
			return &types.AsrEvent{
				Type:       types.EventFinal,
				Text:       asrEvt.Text,
				Confidence: float32(asrEvt.Confidence),
				IsFinal:    true,
			}, nil

		case AsrEventError:
			return &types.AsrEvent{
				Type: types.EventError,
				Text: asrEvt.Err.Error(),
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

// generateUUID 生成 UUID
func generateUUID() string {
	return fmt.Sprintf("%d-%d", time.Now().UnixNano(), time.Now().Nanosecond())
}
