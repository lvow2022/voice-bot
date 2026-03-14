package audio

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewAudioRingBuffer(t *testing.T) {
	rb := NewAudioRingBuffer(1024)
	assert.Equal(t, 0, rb.Len())
}

func TestAudioRingBuffer_Discard(t *testing.T) {
	rb := NewAudioRingBuffer(1024)
	rb.Discard()
	assert.Equal(t, 0, rb.Len())
}

func TestAudioRingBuffer_Read(t *testing.T) {
	rb := NewAudioRingBuffer(1024)
	val := []byte{1, 2, 3, 4, 5}
	rb.Write(val)
	buf := make([]byte, 5)
	rb.Read(buf)
	assert.Equal(t, val, buf)
}

func TestAudioRingBuffer_ReadAtLeast(t *testing.T) {
	rb := NewAudioRingBuffer(5)
	rb.Write([]byte{1, 2})

	buf := make([]byte, 3)
	_, err := rb.ReadAtLeast(buf)
	assert.NotNil(t, err)
}

func TestAudioRingBuffer_Write(t *testing.T) {
	rb := NewAudioRingBuffer(5)
	rb.Write([]byte{1, 2, 3})
	assert.Equal(t, 3, rb.Len())
	rb.Write([]byte{4, 5, 6})
	buf := make([]byte, 5)
	rb.Read(buf)
	assert.Equal(t, []byte{1, 2, 3, 4, 5}, buf)
}
