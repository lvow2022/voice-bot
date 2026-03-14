// Package pipeline provides voicechain pipeline filters for audio processing.
package pipeline

import (
	"log/slog"
	"strings"
	"time"

	gonanoid "github.com/matoous/go-nanoid"
	"voicebot/pkg/audio"
	"voicebot/pkg/voicechain"
)

// AudioFilterOption configures the audio filter for voicechain sessions.
type AudioFilterOption struct {
	// Audio processing options
	audio.AudioProcessOption

	// Session behavior options
	SilentDuration string `json:"silentDuration" yaml:"silent_duration" default:"5s"` // 静音超时时间
	DTMFDuration   string `json:"dtmfDuration" yaml:"dtmf_duration" default:"400ms"`    // DTMF 按键去抖时间
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
func WithAudioFilter(s *voicechain.Session, opt AudioFilterOption) voicechain.FilterFunc {
	ap := audio.NewAudioProcess(opt.AudioProcessOption)
	lastSilence := time.Now()
	var lastSpeaking time.Time
	silentDuration, _ := time.ParseDuration(opt.SilentDuration)
	hasSpeaker := false
	hasSilence := false
	dialogID := ""

	lastKeyReceived := time.Now()
	lastKey := ""
	dtmfDuration, _ := time.ParseDuration(opt.DTMFDuration)

	dtmfHandler := func(sender, digit string) {
		if lastKey == digit && time.Since(lastKeyReceived) < dtmfDuration {
			return
		}
		digit = strings.ToUpper(strings.TrimSpace(digit))
		if !strings.Contains("0123456789*#ABCD", digit) {
			return
		}
		lastKey = digit
		lastKeyReceived = time.Now()
		slog.Info("dtmf handle", "id", s.ID, "digit", digit, "from", sender, "sessionID", s.ID)
		s.EmitState(ap, voicechain.DTMF, digit)
		s.EmitFrame(ap, &voicechain.DtmfFrame{Event: digit})
	}

	// Cleanup resources when session ends
	if ap.HasVAD() {
		s.On(voicechain.End, func(event voicechain.StateEvent) {
			ap.Close()
		})
	}

	return func(frame voicechain.Frame) (bool, error) {
		switch frame := frame.(type) {
		case *voicechain.DtmfFrame:
			dtmfHandler("frame", frame.Event)
			return true, nil
		case *voicechain.AudioFrame:
			if len(frame.Payload) == 0 {
				return true, nil // discard empty frame
			}

			vadHandler := func(duration time.Duration, speaking bool, silence bool) {
				slog.Debug("vad handle", "duration", duration, "speaking", speaking, "silence", silence, "sessionID", s.ID)

				if speaking {
					hasSpeaker = true
					hasSilence = false
					dialogID, _ = gonanoid.Nanoid()
					lastSpeaking = time.Now()
					frame.IsSilence = false
					s.EmitState(ap, voicechain.StartSpeaking, dialogID)
				}

				if silence {
					frame.IsSilence = true
					hasSilence = true
					lastSilence = time.Now()
					if hasSpeaker {
						s.EmitState(ap, voicechain.StartSilence, dialogID, lastSpeaking, lastSilence)
						if dialogID != "" {
							s.EmitCallMetric(dialogID, &voicechain.VadMetric{
								StartSilenceAt:   lastSilence,
								StartSpeakingAt: lastSpeaking,
								DialogID:         dialogID,
							})
						}
						dialogID = ""
					}
				}
			}

			ap.Process(s.SampleRate, frame.Payload, vadHandler, dtmfHandler)

			if hasSilence && time.Since(lastSilence) > silentDuration {
				lastSilence = time.Now()
				s.EmitState(ap, voicechain.Silence)
			}
		}
		return false, nil
	}
}
