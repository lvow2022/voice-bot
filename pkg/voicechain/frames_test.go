package voicechain

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTextFrames(t *testing.T) {
	tf := TextFrame{
		Text:           "hello world",
		IsTranscribed:  false,
		IsLLMGenerated: false,
	}
	assert.NotNil(t, tf.Body())
	assert.NotNil(t, tf.String())
}

func TestAudioFrame(t *testing.T) {
	af := AudioFrame{
		Payload: []byte{0x11, 0x22, 0x33, 0x44, 0x55, 0x66},
	}
	assert.NotNil(t, af.Body())
	assert.NotNil(t, af.String())
}

func TestFunctionFrame(t *testing.T) {
	ff := FunctionFrame{
		Name:   "say",
		Params: []string{"hello world"},
	}
	assert.NotNil(t, ff.Body())
	assert.NotNil(t, ff.String())
}

func TestInterruptFrame(t *testing.T) {
	rf := InterruptFrame{
		Sender: "test",
	}
	assert.Nil(t, rf.Body())
	assert.NotNil(t, rf.String())
}
