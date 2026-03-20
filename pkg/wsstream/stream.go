package wsstream

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/gorilla/websocket"
)

// ============ Errors ============

var (
	ErrStreamClosed   = errors.New("stream closed")
	ErrNotConnected   = errors.New("not connected")
	ErrSendTimeout    = errors.New("send timeout")
	ErrAlreadyClosed  = errors.New("already closed")
)

// ============ StreamEvent ============

// StreamEvent 统一的流事件
type StreamEvent struct {
	Type string // "delta" | "final" | "error"

	// 数据字段（根据 Type 不同使用不同字段）
	Text  string  // 文本数据
	Audio []byte  // 音频数据

	// 错误信息
	Err error
}

// ============ Codec ============

// Codec 编解码器接口
type Codec[Req any] interface {
	// Encode 编码请求
	Encode(req Req) ([]byte, error)

	// Decode 解码响应
	Decode(data []byte) (StreamEvent, error)

	// MessageType 返回 WebSocket 消息类型（TextMessage 或 BinaryMessage）
	MessageType() int
}

// ============ Conn ============

// Conn WebSocket 连接抽象
type Conn interface {
	Read() (messageType int, data []byte, err error)
	Write(messageType int, data []byte) error
	Close() error
}

// WSConn WebSocket 连接实现
type WSConn struct {
	conn *websocket.Conn
}

func NewWSConn(conn *websocket.Conn) *WSConn {
	return &WSConn{conn: conn}
}

func (c *WSConn) Read() (int, []byte, error) {
	return c.conn.ReadMessage()
}

func (c *WSConn) Write(messageType int, data []byte) error {
	return c.conn.WriteMessage(messageType, data)
}

func (c *WSConn) Close() error {
	return c.conn.Close()
}

// ============ WSStream ============

// WSStream 泛型 WebSocket 双向流
type WSStream[Req any] struct {
	conn  Conn
	codec Codec[Req]

	// 接收通道
	recvCh chan StreamEvent

	// 发送通道
	sendCh   chan Req
	sendDone chan struct{}

	// 生命周期
	ctx    context.Context
	cancel context.CancelFunc

	// 状态
	closed    bool
	closeOnce sync.Once
	wg        sync.WaitGroup

	// 接收错误
	recvErr error
}

// NewWSStream 创建 WebSocket 流
func NewWSStream[Req any](conn Conn, codec Codec[Req], opts ...Option) *WSStream[Req] {
	options := DefaultOptions()
	for _, opt := range opts {
		opt(&options)
	}

	ctx, cancel := context.WithCancel(context.Background())

	s := &WSStream[Req]{
		conn:      conn,
		codec:     codec,
		recvCh:    make(chan StreamEvent, options.RecvBufferSize),
		sendCh:    make(chan Req, options.SendBufferSize),
		sendDone:  make(chan struct{}),
		ctx:       ctx,
		cancel:    cancel,
	}

	// 启动读写循环
	s.wg.Add(2)
	go s.readLoop()
	go s.writeLoop()

	return s
}

// Send 发送请求
func (s *WSStream[Req]) Send(ctx context.Context, req Req) error {
	select {
	case s.sendCh <- req:
		return nil
	case <-s.ctx.Done():
		return ErrStreamClosed
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Recv 返回接收通道
func (s *WSStream[Req]) Recv() <-chan StreamEvent {
	return s.recvCh
}

// Close 关闭流
func (s *WSStream[Req]) Close() error {
	var err error
	s.closeOnce.Do(func() {
		s.closed = true
		s.cancel()

		// 关闭发送通道
		close(s.sendCh)

		// 等待写循环结束
		s.wg.Wait()

		// 关闭连接
		if s.conn != nil {
			err = s.conn.Close()
		}

		// 关闭接收通道
		close(s.recvCh)
	})
	return err
}

// Err 返回流错误
func (s *WSStream[Req]) Err() error {
	return s.recvErr
}

// Done 返回流结束信号
func (s *WSStream[Req]) Done() <-chan struct{} {
	return s.ctx.Done()
}

// readLoop 读取循环
func (s *WSStream[Req]) readLoop() {
	defer s.wg.Done()
	defer s.Close()

	for {
		select {
		case <-s.ctx.Done():
			s.setRecvErr(s.ctx.Err())
			return
		default:
		}

		_, data, err := s.conn.Read()
		if err != nil {
			s.setRecvErr(fmt.Errorf("network: %w", err))
			s.recvCh <- StreamEvent{
				Type: "error",
				Err:  fmt.Errorf("network: %w", err),
			}
			return
		}

		evt, err := s.codec.Decode(data)
		if err != nil {
			// 解码错误，继续读取
			s.recvCh <- StreamEvent{
				Type: "error",
				Err:  fmt.Errorf("decode: %w", err),
			}
			continue
		}

		s.recvCh <- evt

		// 收到 final 事件，结束流
		if evt.Type == "final" {
			return
		}
	}
}

// writeLoop 写入循环
func (s *WSStream[Req]) writeLoop() {
	defer s.wg.Done()
	defer close(s.sendDone)

	messageType := s.codec.MessageType()

	for {
		select {
		case <-s.ctx.Done():
			return
		case req, ok := <-s.sendCh:
			if !ok {
				// channel 关闭，结束写入
				return
			}

			data, err := s.codec.Encode(req)
			if err != nil {
				s.recvCh <- StreamEvent{
					Type: "error",
					Err:  fmt.Errorf("encode: %w", err),
				}
				continue
			}

			if err := s.conn.Write(messageType, data); err != nil {
				s.recvCh <- StreamEvent{
					Type: "error",
					Err:  fmt.Errorf("write: %w", err),
				}
				return
			}
		}
	}
}

// setRecvErr 设置接收错误
func (s *WSStream[Req]) setRecvErr(err error) {
	if err != nil && s.recvErr == nil {
		s.recvErr = err
	}
}
