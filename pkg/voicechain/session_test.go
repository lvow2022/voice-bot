package voicechain

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSessionCreate(t *testing.T) {
	session := NewSession()
	assert.NotNil(t, session)
	assert.NotEmpty(t, session.ID)
	assert.Empty(t, session.eventHandles)
	assert.Empty(t, session.errors)
	assert.Nil(t, session.encoder)
	assert.Nil(t, session.decoder)
	assert.Empty(t, session.inputs)
	assert.Empty(t, session.outputs)
}

func TestSessionID(t *testing.T) {
	session := NewSession()
	assert.NotEmpty(t, session.ID)

	session.SetID("custom-id")
	assert.Equal(t, "custom-id", session.ID)
}

func TestSessionValue(t *testing.T) {
	session := NewSession()

	// Set and Get
	session.Set("key1", "value1")
	val, ok := session.Get("key1")
	assert.True(t, ok)
	assert.Equal(t, "value1", val)

	// Get non-existent
	_, ok = session.Get("nonexistent")
	assert.False(t, ok)

	// Overwrite
	session.Set("key1", "value2")
	val, ok = session.Get("key1")
	assert.True(t, ok)
	assert.Equal(t, "value2", val)

	// Delete
	session.Delete("key1")
	_, ok = session.Get("key1")
	assert.False(t, ok)
}

func TestSessionCodec(t *testing.T) {
	session := NewSession()
	session.SampleRate = 8000

	codec := session.Codec()
	assert.Equal(t, "pcm", codec.Codec)
	assert.Equal(t, 8000, codec.SampleRate)
	assert.Equal(t, 1, codec.Channels)
	assert.Equal(t, 16, codec.BitDepth)
}

func TestSessionInputOutput(t *testing.T) {
	session := NewSession()

	input := &DummyTransport{}
	output := &DummyTransport{}

	session.Input(input)
	session.Output(output)

	assert.Len(t, session.inputs, 1)
	assert.Len(t, session.outputs, 1)
}

func TestSessionIsValid(t *testing.T) {
	session := NewSession()

	// No input/output
	err := session.IsValid()
	assert.Equal(t, ErrNotInputTransport, err)

	// Only input
	session.Input(&DummyTransport{})
	err = session.IsValid()
	assert.Equal(t, ErrNotOutputTransport, err)

	// Both input and output
	session.Output(&DummyTransport{})
	err = session.IsValid()
	assert.Nil(t, err)
}

func TestSessionPipeline(t *testing.T) {
	session := NewSession()

	called := false
	session.Pipeline(func(h SessionHandler, data SessionData) {
		called = true
	})

	assert.Len(t, session.handles, 1)
	assert.False(t, called) // Not called yet, just registered
}

func TestSessionOn(t *testing.T) {
	session := NewSession()

	called := false
	session.On("custom_event", func(event Event) {
		called = true
	})

	assert.Len(t, session.eventHandles["custom_event"], 1)
	assert.False(t, called) // Not called yet, just registered
}

func TestSessionError(t *testing.T) {
	session := NewSession()

	called := false
	session.Error(func(sender any, err error) {
		called = true
	})

	assert.Len(t, session.errors, 1)

	// Trigger error
	session.CauseError(session, assert.AnError)
	assert.True(t, called)
}

func TestSessionString(t *testing.T) {
	session := NewSession()
	session.SetID("test-session")
	s := session.String()
	assert.Contains(t, s, "test-session")
}

func TestSessionContext(t *testing.T) {
	session := NewSession()

	ctx := session.GetContext()
	assert.NotNil(t, ctx)
	assert.Nil(t, ctx.Err())

	session.Close()

	// Context should be canceled
	assert.Error(t, ctx.Err())
}
