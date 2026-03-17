package voicechain

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPipelineHandlerGetContext(t *testing.T) {
	session := NewSession()
	handler := &PipelineHandler{s: session}

	ctx := handler.GetContext()
	assert.NotNil(t, ctx)
}

func TestPipelineHandlerGetSession(t *testing.T) {
	session := NewSession()
	handler := &PipelineHandler{s: session}

	s := handler.GetSession()
	assert.Equal(t, session, s)
}

func TestPipelineHandlerCauseError(t *testing.T) {
	session := NewSession()
	handler := &PipelineHandler{s: session}

	called := false
	session.Error(func(sender any, err error) {
		called = true
	})

	handler.CauseError(handler, assert.AnError)
	assert.True(t, called)
}

func TestPipelineHandlerEmitEvent(t *testing.T) {
	session := NewSession()
	handler := &PipelineHandler{s: session}

	called := false
	session.On("test_state", func(event Event) {
		called = true
	})

	handler.EmitEvent(handler, Event{Type: "test_state"})
	assert.True(t, called)
}

func TestPipelineHandlerSendToOutput(t *testing.T) {
	session := NewSession()
	output := &DummyTransport{}
	session.Output(output)

	frame := &TextFrame{Text: "test"}

	// Create a handler and send directly
	handler := &PipelineHandler{s: session}
	handler.SendToOutput(handler, frame)
	// Just verify it doesn't panic
}

func TestPipelineHandlerAddMetric(t *testing.T) {
	session := NewSession()
	handler := &PipelineHandler{s: session}

	// Just verify it doesn't panic
	handler.AddMetric("test_metric", 1000000)
}

func TestPipelineHandlerString(t *testing.T) {
	session := NewSession()

	// Final handler (no next)
	h1 := &PipelineHandler{s: session, handler: nil, next: nil}
	s1 := h1.String()
	assert.Contains(t, s1, "isFinal:true")

	// Handler with next
	h2 := &PipelineHandler{s: session, handler: func(h SessionHandler, data SessionData) {}, next: h1}
	s2 := h2.String()
	assert.Contains(t, s2, "hasNext: true")
}
