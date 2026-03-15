// Package pipeline provides voicechain pipeline filters for audio processing.
package pipeline

import (
	"voicebot/pkg/audio"
)

// AudioFilterOption configures the audio filter for voicechain sessions.
type AudioFilterOption struct {
	// Audio processing options
	audio.AudioProcessOption

	// Session behavior options
	SilentDuration string `json:"silentDuration" yaml:"silent_duration" default:"5s"` // 静音超时时间
	DTMFDuration   string `json:"dtmfDuration" yaml:"dtmf_duration" default:"400ms"`  // DTMF 按键去抖时间
}

// DefaultAudioFilterOption returns an AudioFilterOption with sensible defaults.
func DefaultAudioFilterOption() AudioFilterOption {
	return AudioFilterOption{
		AudioProcessOption: audio.DefaultAudioProcessOption(),
		SilentDuration:     "5s",
		DTMFDuration:       "400ms",
	}
}

// WithAudioFilter creates a voicechain filter that processes audio frames.
// It combines VAD, noise suppression, AGC, and DTMF detection.
// func WithAudioFilter(s *voicechain.Session, opt AudioFilterOption) voicechain.FilterFunc {

// }
