package voicechain

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"runtime/debug"
	"sync"
	"time"
)

const (
	DirectionInput      = "rx"
	DirectionOutput     = "tx"
	DirectionMiddleware = "middleware"
)

// CodecOption 编解码选项
type CodecOption struct {
	Codec         string `json:"codec" form:"codec" default:"pcm"`
	SampleRate    int    `json:"sampleRate" form:"sample_rate" default:"16000"`
	Channels      int    `json:"channels" form:"channels" default:"1"`
	BitDepth      int    `json:"bitDepth" form:"bit_depth" default:"16"`
	FrameDuration string `json:"frameDuration" form:"frame_duration"`
	PayloadType   uint8  `json:"payloadType" form:"payload_type"`
}

// DefaultCodecOption 返回默认编解码选项
func DefaultCodecOption() CodecOption {
	return CodecOption{
		Codec:         "pcm",
		SampleRate:    16000,
		Channels:      1,
		BitDepth:      16,
		FrameDuration: "",
	}
}

func (c CodecOption) String() string {
	return fmt.Sprintf("CodecOption{Codec: %s, SampleRate: %d, Channels: %d, BitDepth: %d}",
		c.Codec, c.SampleRate, c.Channels, c.BitDepth)
}

// Transport 传输接口
type Transport interface {
	io.Closer
	String() string
	Attach(s *Session)
	Next(ctx context.Context) (Frame, error)
	Send(ctx context.Context, frame Frame) (int, error)
	Codec() CodecOption
}

// TransportLayer 传输层
type TransportLayer struct {
	session             *Session
	txqueue             chan Frame
	transport           Transport
	filters             []FilterFunc
	mtx                 sync.Mutex
	incomingClosedChan  chan struct{}
	outcomingClosedChan chan struct{}
}

// DummyTransport 虚拟传输（用于测试）
type DummyTransport struct {
	Pos     int
	Inputs  []Frame
	Outputs bytes.Buffer
}

func (t *DummyTransport) Next(_ context.Context) (Frame, error) {
	if t.Pos >= len(t.Inputs) {
		return nil, io.EOF
	}
	frame := t.Inputs[t.Pos]
	t.Pos++
	return frame, nil
}

func (t *DummyTransport) Send(_ context.Context, frame Frame) (int, error) {
	body := frame.Body()
	if len(body) == 0 {
		return 0, nil
	}
	return t.Outputs.Write(body)
}

func (t *DummyTransport) Attach(_ *Session) {}

func (t *DummyTransport) String() string {
	return fmt.Sprintf("DummyTransport{Inputs: %d, Pos:%d, Outputs: %d}", len(t.Inputs), t.Pos, t.Outputs.Len())
}

func (t *DummyTransport) Close() error { return nil }

func (t *DummyTransport) Codec() CodecOption { return DefaultCodecOption() }

func (tl *TransportLayer) String() string {
	return fmt.Sprintf("TransportLayer{Session: %s, Transport: %s}", tl.session, tl.transport)
}

func (tl *TransportLayer) processIncoming() {
	slog.Debug("transport layer incoming loop started", "sessionID", tl.session.ID, "transport", tl.transport)
	tl.incomingClosedChan = make(chan struct{}, 1)
	defer func() {
		if r := recover(); r != nil {
			slog.Error("transport layer incoming loop panic", "sessionID", tl.session.ID, "transport", tl.transport, "error", r, "stacktrace", string(debug.Stack()))
		}
		tl.incomingClosedChan <- struct{}{}
	}()

	transport := tl.transport
incomingLoop:
	for tl.session.ctx.Err() == nil {
		frame, err := transport.Next(tl.session.ctx)
		if err != nil {
			if err != io.EOF {
				tl.session.CauseError(tl, err)
			} else {
				tl.session.EmitState(tl, StateSessionHangup)
			}
			break incomingLoop
		}
		if frame == nil {
			continue
		}

		var frames []Frame
		if tl.session.decoder != nil {
			frames, err = tl.session.decoder(frame)
			if err != nil {
				tl.session.CauseError(tl, err)
				break incomingLoop
			}
		} else {
			frames = []Frame{frame}
		}

		for _, frame := range frames {
			if frame == nil {
				continue
			}
			discard := false
			for _, f := range tl.filters {
				discard, err = f(frame)
				if err != nil {
					tl.session.CauseError(tl, err)
					break incomingLoop
				}
				if discard {
					break
				}
			}
			if !discard {
				tl.session.EmitFrame(tl, frame)
			}
		}
	}
	slog.Debug("transport layer incoming loop ended", "sessionID", tl.session.ID, "transport", transport)
}

func (tl *TransportLayer) processOutgoing() {
	if tl.txqueue == nil {
		panic("txqueue is nil, transport layer is not initialized")
	}
	tl.outcomingClosedChan = make(chan struct{}, 1)
	defer func() {
		if r := recover(); r != nil {
			slog.Error("transport layer outgoing loop panic", "sessionID", tl.session.ID, "transport", tl.transport, "error", r, "stacktrace", string(debug.Stack()))
		}
		tl.outcomingClosedChan <- struct{}{}
	}()

	slog.Debug("transport layer outgoing loop started", "sessionID", tl.session.ID, "transport", tl.transport)
outgoingLoop:
	for {
		var frame Frame
		var ok bool
		var err error
		var discard = false
		select {
		case <-tl.session.ctx.Done():
			slog.Debug("transport layer outgoing loop canceled", "sessionID", tl.session.ID, "transport", tl.transport)
			break outgoingLoop
		case frame, ok = <-tl.txqueue:
			if !ok || frame == nil {
				slog.Debug("transport layer outgoing queue closed", "sessionID", tl.session.ID, "transport", tl.transport)
				break outgoingLoop
			}
		}

		for _, f := range tl.filters {
			discard, err = f(frame)
			if discard {
				break
			}
			if err != nil {
				tl.session.CauseError(tl, err)
				break outgoingLoop
			}
		}

		if discard {
			continue
		}

		var frames []Frame
		if tl.session.encoder != nil {
			frames, err = tl.session.encoder(frame)
			if err != nil {
				tl.session.CauseError(tl, err)
				break outgoingLoop
			}
		} else {
			frames = []Frame{frame}
		}

		for _, frame := range frames {
			tl.transport.Send(tl.session.ctx, frame)
		}
	}
	slog.Debug("transport layer outgoing loop ended", "sessionID", tl.session.ID)
}

func (tl *TransportLayer) waitForIncomingLoopStop() {
	select {
	case <-tl.incomingClosedChan:
	case <-time.After(5 * time.Second):
	}
}

func (tl *TransportLayer) waitForOutcomingLoopStop() {
	select {
	case <-tl.outcomingClosedChan:
	case <-time.After(5 * time.Second):
	}
}

func (tl *TransportLayer) cleanup() {
	tl.mtx.Lock()
	defer tl.mtx.Unlock()
	if tl.transport == nil {
		return
	}

	tl.transport.Close()
	tl.waitForIncomingLoopStop()
	tl.waitForOutcomingLoopStop()

	tl.transport = nil

	for _, f := range tl.filters {
		_, _ = f(&CloseFrame{Reason: "transport cleanup"})
	}

	slog.Debug("transport layer cleaned up", "sessionID", tl.session.ID)
}

func (tl *TransportLayer) trySendFrame(frame Frame) {
	tl.mtx.Lock()
	defer tl.mtx.Unlock()

	if tl.txqueue == nil || tl.transport == nil {
		return
	}
	select {
	case tl.txqueue <- frame:
	default:
		slog.Warn("frame dropped", "sessionID", tl.session.ID, "frame", frame)
	}
}
