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
	ErrStreamClosed  = errors.New("stream closed")
	ErrNotConnected  = errors.New("not connected")
	ErrSendTimeout   = errors.New("send timeout")
	ErrAlreadyClosed = errors.New("already closed")
)

// ============ Codec ============

// Codec 编解码器接口
type Codec interface {
	// Encode 编码请求（内部做类型断言）
	Encode(req any) ([]byte, error)

	// Decode 解码响应（返回具体事件类型）
	Decode(data []byte) (any, error)

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

// WSStream WebSocket 双向流（传输层，数据类型为 any）
type WSStream struct {
	conn  Conn
	codec Codec

	// 接收通道
	recvCh chan any

	// 发送通道
	sendCh   chan any
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
func NewWSStream(conn Conn, codec Codec, opts ...Option) *WSStream {
	options := DefaultOptions()
	for _, opt := range opts {
		opt(&options)
	}

	ctx, cancel := context.WithCancel(context.Background())

	s := &WSStream{
		conn:      conn,
		codec:     codec,
		recvCh:    make(chan any, options.RecvBufferSize),
		sendCh:    make(chan any, options.SendBufferSize),
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
func (s *WSStream) Send(ctx context.Context, req any) error {
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
func (s *WSStream) Recv() <-chan any {
	return s.recvCh
}

// Close 关闭流
func (s *WSStream) Close() error {
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
func (s *WSStream) Err() error {
	return s.recvErr
}

// Done 返回流结束信号
func (s *WSStream) Done() <-chan struct{} {
	return s.ctx.Done()
}

// readLoop 读取循环
func (s *WSStream) readLoop() {
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
			s.recvCh <- fmt.Errorf("network: %w", err)
			return
		}

		evt, err := s.codec.Decode(data)
		if err != nil {
			// 解码错误，继续读取
			s.recvCh <- fmt.Errorf("decode: %w", err)
			continue
		}

		// 跳过空事件
		if evt == nil {
			continue
		}

		s.recvCh <- evt

		// 检查是否是 final 事件（通过反射或接口判断）
		if isFinal(evt) {
			return
		}
	}
}

// writeLoop 写入循环
func (s *WSStream) writeLoop() {
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
				s.recvCh <- fmt.Errorf("encode: %w", err)
				continue
			}

			if err := s.conn.Write(messageType, data); err != nil {
				s.recvCh <- fmt.Errorf("write: %w", err)
				return
			}
		}
	}
}

// setRecvErr 设置接收错误
func (s *WSStream) setRecvErr(err error) {
	if err != nil && s.recvErr == nil {
		s.recvErr = err
	}
}

// ============ Final Event Detection ============

// FinalEvent final 事件接口
type FinalEvent interface {
	IsFinal() bool
}

// isFinal 检查是否是 final 事件
func isFinal(evt any) bool {
	if fe, ok := evt.(FinalEvent); ok {
		return fe.IsFinal()
	}
	return false
}
