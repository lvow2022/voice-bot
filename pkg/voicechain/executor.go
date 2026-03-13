package voicechain

import (
	"context"
	"log/slog"
	"time"
)

// FrameRequest 帧请求
type FrameRequest[R any] struct {
	h         SessionHandler
	Interrupt bool
	Req       R
}

// Executor 异步执行器
type Executor[T any] struct {
	OnBegin        func(h SessionHandler) error
	OnEnd          func(h SessionHandler) error
	OnState        func(h SessionHandler, event StateEvent) error
	OnBuildRequest func(h SessionHandler, frame Frame) (*FrameRequest[T], error)
	OnExecute      func(ctx context.Context, h SessionHandler, req FrameRequest[T]) error

	ReqChanSize    int
	ExecuteTimeout time.Duration
	MaxTimeout     time.Duration
	Async          bool

	reqChan        chan *FrameRequest[T]
	currentContext context.Context
	cancelFunc     context.CancelFunc
}

// NewExecutor 创建执行器
func NewExecutor[T any](chanSize int) Executor[T] {
	return Executor[T]{
		ReqChanSize: chanSize,
		MaxTimeout:  1 * time.Minute,
		Async:       true,
	}
}

// Cleanup 清理资源
func (m *Executor[T]) Cleanup() {
	if m.reqChan != nil {
		m.reqChan = nil
	}
}

// Interrupt 中断当前执行
func (m *Executor[T]) Interrupt() {
	if m.cancelFunc != nil {
		m.cancelFunc()
		m.cancelFunc = nil
	}
}

// HandleSessionData 处理会话数据
func (m *Executor[T]) HandleSessionData(h SessionHandler, data SessionData) {
	if m.OnExecute == nil {
		panic("OnExecute is not set")
	}
	switch data.Type {
	case SessionDataFrame:
		m.HandleFrame(h, data.Frame)
	case SessionDataState:
		m.HandleState(h, data.State)
	}
}

// HandleState 处理状态事件
func (m *Executor[T]) HandleState(h SessionHandler, event StateEvent) {
	switch event.State {
	case StateSessionBegin:
		if m.Async {
			reqChanSize := m.ReqChanSize
			if reqChanSize == 0 {
				reqChanSize = 1
			}
			m.reqChan = make(chan *FrameRequest[T], reqChanSize)
			go m.start(h.GetContext())
		}
		if m.OnBegin != nil {
			if err := m.OnBegin(h); err != nil {
				h.CauseError(m, err)
			}
		}
	case StateSessionEnd:
		m.Interrupt()
		m.Cleanup()
		if m.OnEnd != nil {
			if err := m.OnEnd(h); err != nil {
				h.CauseError(m, err)
			}
		}
	}
	if m.OnState != nil {
		if err := m.OnState(h, event); err != nil {
			h.CauseError(m, err)
		}
	}
}

// HandleFrame 处理帧
func (m *Executor[T]) HandleFrame(h SessionHandler, frame Frame) {
	if m.OnBuildRequest == nil {
		panic("OnBuildRequest is not set")
	}
	req, err := m.OnBuildRequest(h, frame)
	if err != nil {
		h.CauseError(m, err)
		return
	}
	if req == nil {
		return
	}
	req.h = h
	if m.Async {
		if m.reqChan != nil {
			m.reqChan <- req
		} else {
			slog.Warn("reqChan is nil", "frame", frame)
		}
	} else {
		m.doExecute(h.GetContext(), *req)
	}
}

func (m *Executor[T]) start(parent context.Context) {
	ctx, cancel := context.WithCancel(parent)
	defer func() {
		m.Interrupt()
		m.Cleanup()
		cancel()
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case req := <-m.reqChan:
			if req == nil {
				continue
			}
			if req.Interrupt {
				m.Interrupt()
			} else if m.currentContext != nil {
				select {
				case <-ctx.Done():
					return
				case <-m.currentContext.Done():
					break
				}
			}
			m.doExecute(ctx, *req)
		}
	}
}

func (m *Executor[T]) doExecute(ctx context.Context, req FrameRequest[T]) {
	timeout := m.MaxTimeout
	if m.ExecuteTimeout > 0 {
		timeout = m.ExecuteTimeout
	}
	m.currentContext, m.cancelFunc = context.WithTimeout(ctx, timeout)
	defer m.cancelFunc()

	err := m.OnExecute(m.currentContext, req.h, req)
	if err != nil {
		slog.Error("executor error", "handler", req.h, "error", err)
		req.h.CauseError(m, err)
	}
}
