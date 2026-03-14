package audio

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestAudioProcess(t *testing.T) {
	opt := DefaultAudioProcessOption()
	file, err := os.ReadFile("../../testdata/dtmf/concat.wav")
	if err != nil {
		t.Fatal(err)
	}

	ap := NewAudioProcess(opt)
	ap.dtmf = NewDTMFDetector(0.09, 16000)

	vadHandler := func(duration time.Duration, speaking bool, silence bool) {
		t.Log("vadHandler", duration, speaking, silence)
	}
	dtmfHandler := func(sender, digit string) {
		t.Logf("dtmfHandler %s %s", sender, digit)
	}

	size := GetSampleSize(16000, 16, 1) * 20
	for i := 0; i < len(file); i += size {
		end := i + size
		if end > len(file) {
			end = len(file)
		}
		payload := file[i:end]
		ap.Process(16000, payload, vadHandler, dtmfHandler)
	}

	ap.String()
}

func TestAudioProcessWithSilero(t *testing.T) {
	// This is just a simple compilation test to ensure things compile properly
	// The actual functionality is tested in vad_silero_ort_test.go

	opt := DefaultAudioProcessOption()
	opt.VADType = VADTypeSilero

	// Create the audio process - this should initialize successfully
	ap := NewAudioProcess(opt)
	assert.NotNil(t, ap)
	assert.True(t, ap.HasVAD())

	// Clean up
	ap.Close()
}

func TestAudioProcess_NoVAD(t *testing.T) {
	opt := DefaultAudioProcessOption()
	opt.VADEnabled = false

	ap := NewAudioProcess(opt)
	assert.NotNil(t, ap)
	assert.False(t, ap.HasVAD())

	// Process should still work without VAD
	payload := make([]byte, 640) // 20ms at 16kHz
	result, err := ap.Process(16000, payload, nil, nil)
	assert.NoError(t, err)
	assert.NotNil(t, result)

	ap.Close()
}
