package voicechain

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestExecutorHandleState(t *testing.T) {
	exec := NewExecutor[int](3)
	session := NewSession()

	beginCalled := false
	endCalled := false

	exec.OnBegin = func(h SessionHandler) error {
		beginCalled = true
		return nil
	}
	exec.OnEnd = func(h SessionHandler) error {
		endCalled = true
		return nil
	}

	// Test StateSessionBegin
	exec.HandleState(session, StateEvent{State: StateSessionBegin})
	assert.True(t, beginCalled)
	assert.NotNil(t, exec.reqChan)

	// Test StateSessionEnd
	exec.HandleState(session, StateEvent{State: StateSessionEnd})
	assert.True(t, endCalled)
}

func TestExecutorHandleFrame(t *testing.T) {
	exec := NewExecutor[int](3)
	session := NewSession()

	exec.OnBuildRequest = func(h SessionHandler, frame Frame) (*FrameRequest[int], error) {
		return &FrameRequest[int]{
			Req:       1,
			Interrupt: false,
		}, nil
	}

	executed := false
	exec.OnExecute = func(ctx context.Context, h SessionHandler, req FrameRequest[int]) error {
		executed = true
		assert.Equal(t, 1, req.Req)
		return nil
	}

	// Initialize the executor
	exec.HandleState(session, StateEvent{State: StateSessionBegin})

	// Handle frame synchronously
	exec.Async = false
	exec.HandleFrame(session, &AudioFrame{Payload: []byte{1, 2, 3}})
	assert.True(t, executed)
}

func TestExecutorAsync(t *testing.T) {
	exec := NewExecutor[string](1)
	session := NewSession()

	exec.OnBuildRequest = func(h SessionHandler, frame Frame) (*FrameRequest[string], error) {
		return &FrameRequest[string]{
			Req:       "test",
			Interrupt: false,
		}, nil
	}

	done := make(chan struct{})
	exec.OnExecute = func(ctx context.Context, h SessionHandler, req FrameRequest[string]) error {
		assert.Equal(t, "test", req.Req)
		close(done)
		return nil
	}

	// Initialize and start async executor
	exec.HandleState(session, StateEvent{State: StateSessionBegin})

	// Send frame
	exec.HandleFrame(session, &AudioFrame{Payload: []byte{1, 2, 3}})

	select {
	case <-done:
		// Success
	case <-time.After(1 * time.Second):
		assert.Fail(t, "timeout waiting for execution")
	}

	exec.HandleState(session, StateEvent{State: StateSessionEnd})
}

func TestExecutorInterrupt(t *testing.T) {
	exec := NewExecutor[int](1)
	session := NewSession()

	ctx, cancel := context.WithCancel(session.GetContext())
	defer cancel()

	// Initialize
	exec.HandleState(session, StateEvent{State: StateSessionBegin})

	// Create a context that we can cancel
	exec.currentContext, exec.cancelFunc = context.WithCancel(ctx)

	// Interrupt should cancel the context
	exec.Interrupt()
	assert.Nil(t, exec.cancelFunc)
}
