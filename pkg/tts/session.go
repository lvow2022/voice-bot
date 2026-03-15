package tts

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"sync"
	"sync/atomic"
	"voicebot/pkg/stream"
	"voicebot/pkg/tts/types"
)

var (
	ErrSessionClosed  = errors.New("tts session closed")
	ErrSendBufferFull = errors.New("tts send buffer full")
)

type TtsSession struct {
	provider types.Provider
	opts     types.SessionOptions

	writeCh chan string
	readCh  chan *types.TtsEvent

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	closeOnce sync.Once
	closed    atomic.Bool

	// error
	errOnce sync.Once
	err     error

	synthDone chan struct{}
}

func NewTtsSession(
	ctx context.Context,
	provider types.Provider,
	opts types.SessionOptions,
) (*TtsSession, error) {

	ctx, cancel := context.WithCancel(ctx)

	s := &TtsSession{
		provider: provider,
		opts:     opts,
		writeCh:  make(chan string, 32),
		readCh:   make(chan *types.TtsEvent, 32),
		ctx:      ctx,
		cancel:   cancel,
	}

	if err := provider.Connect(ctx, opts); err != nil {
		cancel()
		return nil, err
	}

	s.wg.Add(2)

	go s.readLoop()
	go s.writeLoop()

	slog.Debug("asr session started")

	return s, nil
}

func (s *TtsSession) Send(text string) error {
	if s.closed.Load() {
		return ErrSessionClosed
	}

	select {
	case <-s.ctx.Done():
		return s.Err()
	case s.writeCh <- text:
		return nil
	default:
		return ErrSendBufferFull
	}
}

func (s *TtsSession) Recv() (*types.TtsEvent, error) {

	select {

	case <-s.ctx.Done():
		return nil, s.Err()

	case ev := <-s.readCh:
		return ev, nil
	}
}

func (s *TtsSession) Close() error {

	s.closeOnce.Do(func() {

		s.closed.Store(true)

		s.cancel()

		// 先关闭 provider（打断 socket read）
		if err := s.provider.Close(); err != nil {
			slog.Error("provider close failed", "err", err)
		}

		s.wg.Wait()

		close(s.readCh)

	})

	return nil
}

func (s *TtsSession) readLoop() {
	defer s.wg.Done()
loop:
	for {
		event, err := s.provider.RecvEvent()
		if err != nil {
			// if isNetError
			if s.tryReconnect(err) {
				continue
			}

			s.setErr(err)
			break loop
		}

		select {
		case <-s.ctx.Done():
			s.setErr(s.ctx.Err())
			break loop
		case s.readCh <- event:
		}
	}
}

func (s *TtsSession) writeLoop() {
	defer s.wg.Done()
loop:
	for {
		select {
		case <-s.ctx.Done():
			s.setErr(s.ctx.Err())
			break loop
		case data := <-s.writeCh:
			if err := s.provider.SendText(data, nil); err != nil {
				s.setErr(err)
				break loop
			}
		}
	}
}

func (s *TtsSession) Err() error {
	return s.err
}

func (s *TtsSession) setErr(err error) {
	if err == nil {
		return
	}
	s.errOnce.Do(func() {
		s.err = err
	})
}

func (s *TtsSession) AsyncSynthesize(text string, stream *stream.AudioStream) error {
	s.synthDone = make(chan struct{})
	if err := s.Send(text); err != nil {
		return err
	}

	go func() {
		defer func() {
			close(s.synthDone)
		}()

		for {
			event, err := s.Recv()
			if err != nil {
				return
			}

			switch event.Type {
			case types.EventAudioChunk:
				if len(event.Data) > 0 {
					if err := stream.Push(event.Data, false); err != nil {
						return
					}
				}
			case types.EventCompleted:
				if err := stream.Push(nil, true); err != nil {
					return
				}

				return
			case types.EventError:

			}
		}
	}()

	return nil
}

func (s *TtsSession) SynthesizeDone() chan struct{} {
	return s.synthDone
}

func (s *TtsSession) tryReconnect(err error) bool {
	if s.closed.Load() {
		return false
	}

	if !isReconnectable(err) {
		return false
	}

	slog.Warn("tts reconnecting", "err", err)

	if err := s.provider.Connect(s.ctx, s.opts); err != nil {
		slog.Error("tts reconnect failed", "err", err)
		return false
	}

	slog.Info("tts reconnected")
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

	// 连接重置/关闭可重连
	if errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, io.EOF) {
		return true
	}

	// WebSocket 关闭错误 (gorilla/websocket)
	if errors.Is(err, errors.New("websocket: close 1006 (abnormal closure): unexpected EOF")) {
		return true
	}

	return false
}
