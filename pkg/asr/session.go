package asr

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"voicebot/pkg/asr/types"
	wsvolc "voicebot/pkg/websocket/provider/volc"
	"voicebot/pkg/websocket"
)

var (
	ErrSessionClosed  = errors.New("asr session closed")
	ErrSendBufferFull = errors.New("asr send buffer full")
)

type AsrSession struct {
	provider types.Provider
	stream   *websocket.WSStream
	opts     types.SessionOptions

	writeCh chan []byte
	readCh  chan *types.AsrEvent

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	closeOnce sync.Once
	closed    atomic.Bool

	// error
	errOnce sync.Once
	err     error

	// metrics
	startTime   time.Time
	bytesSent   int64
	bytesRecv   int64
	audioFrames int64
}

func NewASRSession(
	ctx context.Context,
	provider types.Provider,
	opts types.SessionOptions,
) (*AsrSession, error) {

	ctx, cancel := context.WithCancel(ctx)

	s := &AsrSession{
		provider:  provider,
		opts:      opts,
		writeCh:   make(chan []byte, 32),
		readCh:    make(chan *types.AsrEvent, 32),
		ctx:       ctx,
		cancel:    cancel,
		startTime: time.Now(),
	}

	// 连接并获取 WSStream
	stream, err := provider.Connect(ctx, opts)
	if err != nil {
		cancel()
		return nil, err
	}
	s.stream = stream

	s.wg.Add(2)

	go s.readLoop()
	go s.writeLoop()

	slog.Debug("asr session started")

	return s, nil
}

func (s *AsrSession) Send(frame types.AudioFrame) error {

	if s.closed.Load() {
		return ErrSessionClosed
	}

	select {

	case <-s.ctx.Done():
		return s.Err()

	case s.writeCh <- frame.Data:
		return nil

	default:
		return ErrSendBufferFull
	}
}

func (s *AsrSession) Recv() (*types.AsrEvent, error) {

	select {

	case <-s.ctx.Done():
		return nil, s.Err()

	case ev := <-s.readCh:
		return ev, nil
	}
}

func (s *AsrSession) Close() error {

	s.closeOnce.Do(func() {

		s.closed.Store(true)

		s.cancel()

		// 关闭 WSStream
		if s.stream != nil {
			_ = s.stream.Close()
		}

		s.wg.Wait()

		close(s.readCh)

		duration := time.Since(s.startTime)

		slog.Debug("asr session closed",
			"duration", duration,
			"bytes_sent", atomic.LoadInt64(&s.bytesSent),
			"bytes_recv", atomic.LoadInt64(&s.bytesRecv),
			"audio_frames", atomic.LoadInt64(&s.audioFrames),
		)
	})

	return nil
}

// readLoop 读取循环
func (s *AsrSession) readLoop() {
	defer s.wg.Done()
loop:
	for {
		select {
		case <-s.ctx.Done():
			s.setErr(s.ctx.Err())
			break loop
		case evt, ok := <-s.stream.Recv():
			if !ok {
				s.setErr(errors.New("stream closed"))
				break loop
			}

			// 处理错误类型
			if err, ok := evt.(error); ok {
				if s.tryReconnect(err) {
					continue
				}
				s.setErr(err)
				break loop
			}

			// 类型断言为 volc.AsrEvent
			asrEvt, ok := evt.(wsvolc.AsrEvent)
			if !ok {
				continue
			}

			// 转换为 types.AsrEvent
			var event *types.AsrEvent
			switch asrEvt.Type {
			case wsvolc.AsrEventPartial:
				event = &types.AsrEvent{
					Type:       types.EventPartial,
					Text:       asrEvt.Text,
					Confidence: float32(asrEvt.Confidence),
				}

			case wsvolc.AsrEventFinal:
				event = &types.AsrEvent{
					Type:       types.EventFinal,
					Text:       asrEvt.Text,
					Confidence: float32(asrEvt.Confidence),
				}

			case wsvolc.AsrEventError:
				event = &types.AsrEvent{
					Type: types.EventError,
					Err:  asrEvt.Err,
				}

			default:
				continue
			}

			select {
			case <-s.ctx.Done():
				s.setErr(s.ctx.Err())
				break loop
			case s.readCh <- event:
			}

			// 如果是 final 事件，结束
			if asrEvt.Type == wsvolc.AsrEventFinal {
				break loop
			}
		}
	}
}

// writeLoop 写入循环
func (s *AsrSession) writeLoop() {
	defer s.wg.Done()
loop:
	for {
		select {
		case <-s.ctx.Done():
			s.setErr(s.ctx.Err())
			break loop
		case data := <-s.writeCh:
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			err := s.stream.Send(ctx, wsvolc.AsrRequest{
				Audio:  data,
				IsLast: false,
			})
			cancel()
			if err != nil {
				if s.tryReconnect(err) {
					continue
				}
				s.setErr(err)
				break loop
			}
		}
	}
}

func (s *AsrSession) Err() error {
	return s.err
}

func (s *AsrSession) setErr(err error) {
	if err == nil {
		return
	}
	s.errOnce.Do(func() {
		s.err = err
	})
}

// tryReconnect 尝试重连
func (s *AsrSession) tryReconnect(err error) bool {
	if s.closed.Load() {
		return false
	}

	if !isReconnectable(err) {
		return false
	}

	slog.Warn("asr reconnecting", "err", err)

	// 关闭旧 stream
	if s.stream != nil {
		_ = s.stream.Close()
	}

	// 重新连接
	stream, connErr := s.provider.Connect(s.ctx, s.opts)
	if connErr != nil {
		slog.Error("asr reconnect failed", "err", connErr)
		return false
	}
	s.stream = stream

	slog.Info("asr reconnected")
	return true
}

// isReconnectable 判断错误是否可重连
func isReconnectable(err error) bool {
	if err == nil {
		return false
	}

	// 上下文取消不重连
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}

	// 网络错误可重连
	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}

	// WebSocket 流关闭
	if errors.Is(err, websocket.ErrStreamClosed) {
		return true
	}

	return false
}
