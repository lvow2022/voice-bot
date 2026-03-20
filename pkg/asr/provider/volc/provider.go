package volc

import (
	"context"
	"fmt"
	"net/http"
	"time"

	gorilla "github.com/gorilla/websocket"

	"voicebot/pkg/asr/types"
	"voicebot/pkg/websocket"
)

// ============ Provider ============

// Provider 火山引擎 ASR Provider（基于 websocket)
type Provider struct {
	cfg Config
}

// NewProvider 创建 Provider
func NewProvider(cfg types.ProviderConfig) (*Provider, error) {
	config := ParseConfig(cfg)

	return &Provider{
		cfg: config,
	}, nil
}

// Connect 建立连接，返回 WSStream 供调用方使用
func (p *Provider) Connect(ctx context.Context, opts types.SessionOptions) (*websocket.WSStream, error) {
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

	dialer := gorilla.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}

	conn, _, err := dialer.DialContext(ctx, volcanoWSURL, headers)
	if err != nil {
		return nil, fmt.Errorf("dial volcano asr: %w", err)
	}

	// 创建 codec
	codec := NewCodec(p.cfg)

	// 发送 full client request
	fullReq, err := codec.EncodeFullClientRequest()
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("encode full client request: %w", err)
	}

	if err := conn.WriteMessage(gorilla.BinaryMessage, fullReq); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("write full client request: %w", err)
	}

	// 创建流并返回
	stream := websocket.NewWSStream(
		websocket.NewWSConn(conn),
		codec,
		websocket.WithSendBufferSize(128),
		websocket.WithRecvBufferSize(128),
	)

	return stream, nil
}

// generateUUID 生成 UUID
func generateUUID() string {
	return fmt.Sprintf("%d-%d", time.Now().UnixNano(), time.Now().Nanosecond())
}
