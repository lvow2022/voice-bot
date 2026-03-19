// Package voice provides voice transcription (stub for voicebot).
package voice

import "context"

// Transcriber transcribes audio to text.
type Transcriber interface {
	Transcribe(ctx context.Context, audioPath string) (string, error)
}

// MockTranscriber is a mock transcriber for testing.
type MockTranscriber struct{}

// NewMockTranscriber creates a new mock transcriber.
func NewMockTranscriber() *MockTranscriber {
	return &MockTranscriber{}
}

// Transcribe transcribes audio.
func (t *MockTranscriber) Transcribe(ctx context.Context, audioPath string) (string, error) {
	return "", nil
}
