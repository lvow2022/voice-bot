package asr

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"
	"voicebot/pkg/asr/types"
)

// AsrSession ASR 会话，管理连接生命周期和断线重连
type AsrSession struct {
	provider types.Provider
	opts     types.SessionOptions
	config   types.ClientConfig

	events     chan types.AsrEvent
	eventsOnce sync.Once

	writeCh chan []byte
	closeCh chan struct{}

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	closed atomic.Bool
	mu     sync.Mutex

	// 重连状态
	reconnecting atomic.Bool
	retryCount   int64

	// metrics
	startTime   time.Time
	bytesSent   int64
	bytesRecv   int64
	audioFrames int64
}

// newASRSession 创建新的 ASR 会话
func newASRSession(ctx context.Context, provider types.Provider, opts types.SessionOptions, config types.ClientConfig) (*AsrSession, error) {
	ctx, cancel := context.WithCancel(ctx)

	s := &AsrSession{
		provider:  provider,
		opts:      opts,
		config:    config,
		events:    make(chan types.AsrEvent, config.EventBufferSize),
		writeCh:   make(chan []byte, config.WriteBufferSize),
		closeCh:   make(chan struct{}),
		ctx:       ctx,
		cancel:    cancel,
		startTime: time.Now(),
	}

	// 连接 provider
	if err := s.provider.Connect(ctx, opts); err != nil {
		cancel()
		return nil, err
	}

	// 启动读写循环
	s.wg.Add(2)
	go s.readLoop()
	go s.writeLoop()

	slog.Debug("asr session started")
	return s, nil
}

// SendAudio 发送音频帧
func (s *AsrSession) SendAudio(frame types.AudioFrame) error {
	if s.closed.Load() {
		return ErrSessionClosed
	}

	// 如果正在重连，等待重连完成
	if s.reconnecting.Load() {
		return ErrReconnecting
	}

	select {
	case s.writeCh <- frame.Data:
		atomic.AddInt64(&s.bytesSent, int64(len(frame.Data)))
		atomic.AddInt64(&s.audioFrames, 1)
		return nil
	case <-s.ctx.Done():
		return s.ctx.Err()
	}
}

// Events 返回识别事件通道
func (s *AsrSession) Events() <-chan types.AsrEvent {
	return s.events
}

// Close 关闭会话
func (s *AsrSession) Close() error {
	if s.closed.Swap(true) {
		return nil
	}

	close(s.closeCh)
	s.cancel()
	s.wg.Wait()

	s.eventsOnce.Do(func() {
		close(s.events)
	})

	if err := s.provider.Close(); err != nil {
		slog.Error("close provider failed", "error", err)
	}

	duration := time.Since(s.startTime)
	slog.Debug("asr session closed",
		"duration", duration,
		"bytes_sent", atomic.LoadInt64(&s.bytesSent),
		"bytes_recv", atomic.LoadInt64(&s.bytesRecv),
		"audio_frames", atomic.LoadInt64(&s.audioFrames),
		"retry_count", atomic.LoadInt64(&s.retryCount),
	)

	return nil
}

// readLoop 读取循环
func (s *AsrSession) readLoop() {
	defer s.wg.Done()
	defer s.eventsOnce.Do(func() { close(s.events) })

	for {
		select {
		case <-s.ctx.Done():
			return
		default:
			event, err := s.provider.RecvEvent()
			if err != nil {
				if s.closed.Load() {
					return
				}

				slog.Error("asr read error", "error", err)

				// 尝试重连
				if s.handleDisconnect(err) {
					continue
				}
				return
			}

			if event != nil {
				s.emitEvent(*event)
			}
		}
	}
}

// writeLoop 写入循环
func (s *AsrSession) writeLoop() {
	defer s.wg.Done()

	for {
		select {
		case <-s.ctx.Done():
			return
		case data := <-s.writeCh:
			if s.reconnecting.Load() {
				continue // 重连期间丢弃数据
			}

			if err := s.provider.SendAudio(data, false); err != nil {
				if s.closed.Load() {
					return
				}

				slog.Error("asr write error", "error", err)
				s.handleDisconnect(err)
			}
		case <-s.closeCh:
			_ = s.provider.SendAudio([]byte{}, true)
			return
		}
	}
}

// handleDisconnect 处理断线，返回 true 表示重连成功
func (s *AsrSession) handleDisconnect(err error) bool {
	if s.closed.Load() {
		return false
	}

	// 标记正在重连
	if !s.reconnecting.CompareAndSwap(false, true) {
		return false // 已在重连中
	}
	defer s.reconnecting.Store(false)

	slog.Warn("connection lost, attempting reconnect", "error", err)

	// 发送重连事件
	s.emitEvent(types.AsrEvent{
		Type: types.EventReconnecting,
		Text: "connection lost, reconnecting",
	})

	retry := s.config.Retry
	for attempt := 1; attempt <= retry.MaxAttempts; attempt++ {
		atomic.AddInt64(&s.retryCount, 1)

		// 检查是否已关闭
		if s.closed.Load() {
			return false
		}

		// 关闭旧连接
		_ = s.provider.Close()

		// 重新连接
		reconnectErr := s.provider.Connect(s.ctx, s.opts)
		if reconnectErr == nil {
			slog.Info("reconnect successful", "attempt", attempt)
			s.emitEvent(types.AsrEvent{
				Type: types.EventReconnected,
				Text: "reconnect successful",
			})
			return true
		}

		slog.Warn("reconnect failed",
			"attempt", attempt,
			"max_attempts", retry.MaxAttempts,
			"error", reconnectErr,
		)

		// 等待退避时间
		if attempt < retry.MaxAttempts {
			delay := s.calculateBackoff(attempt)
			select {
			case <-s.ctx.Done():
				return false
			case <-time.After(delay):
			}
		}
	}

	// 重连失败
	slog.Error("reconnect failed after all attempts", "attempts", retry.MaxAttempts)
	s.emitEvent(types.AsrEvent{
		Type: types.EventError,
		Text: "reconnect failed: " + err.Error(),
	})

	return false
}

// calculateBackoff 计算指数退避延迟
func (s *AsrSession) calculateBackoff(attempt int) time.Duration {
	retry := s.config.Retry
	delay := float64(retry.InitialDelay)
	for i := 1; i < attempt; i++ {
		delay *= retry.Multiplier
	}
	if delay > float64(retry.MaxDelay) {
		delay = float64(retry.MaxDelay)
	}
	return time.Duration(delay)
}

// emitEvent 发送事件到通道
func (s *AsrSession) emitEvent(event types.AsrEvent) {
	select {
	case s.events <- event:
	default:
		slog.Warn("asr events channel full, dropping event", "type", event.Type)
	}
}

// IsReconnecting 返回是否正在重连
func (s *AsrSession) IsReconnecting() bool {
	return s.reconnecting.Load()
}

// Metrics 返回会话指标
func (s *AsrSession) Metrics() map[string]int64 {
	return map[string]int64{
		"bytes_sent":   atomic.LoadInt64(&s.bytesSent),
		"bytes_recv":   atomic.LoadInt64(&s.bytesRecv),
		"audio_frames": atomic.LoadInt64(&s.audioFrames),
		"retry_count":  atomic.LoadInt64(&s.retryCount),
	}
}

// Errors
var (
	ErrSessionClosed = &SessionError{Code: "SESSION_CLOSED", Message: "session is closed"}
	ErrReconnecting  = &SessionError{Code: "RECONNECTING", Message: "session is reconnecting"}
)

// SessionError 会话错误
type SessionError struct {
	Code    string
	Message string
}

func (e *SessionError) Error() string {
	return e.Message
}
