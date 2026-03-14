package audio

import (
	"fmt"
	"log/slog"
	"strings"
	"time"

	gornnoise "github.com/shenjinti/go-rnnoise"
	"voicebot/pkg/codecs"
	"voicebot/pkg/dfn"
)

// AudioProcess combines multiple audio processors: VAD, noise suppression, AGC, and DTMF.
type AudioProcess struct {
	vad     VADDetector
	ns      *gornnoise.RNNoise
	nsDFN   *dfn.Suppressor
	agc     *AGCProcessor
	dtmf    *DTMFDetector
	vadType VADType
	nsType  NoiseSuppressionType
}

// AudioProcessOption configures the AudioProcess.
type AudioProcessOption struct {
	VADEnabled                 bool                 `json:"vadEnabled" yaml:"vad_enabled" default:"true"`
	VADType                    VADType              `json:"vadType" yaml:"vad_type" default:"silero"`
	VADOption                  VADDetectorOption    `json:"vadOption" yaml:"vad_option"`
	NoiseSuppressionEnabled    bool                 `json:"noiseSuppressionEnabled" yaml:"noise_suppression_enabled" default:"true"`
	NoiseSuppressionType       NoiseSuppressionType `json:"noiseSuppressionType" yaml:"noise_suppression_type" default:"dfn"`
	NoiseSuppressionIntensity  int                  `json:"noiseSuppressionIntensity" yaml:"noise_suppression_intensity" default:"5"` // range(1,30), higher for stronger suppression
	NoiseSuppressionONNXConfig dfn.SuppressorConfig `json:"noiseSuppressionONNXConfig" yaml:"noise_suppression_onnx_config"`
	AGCEnabled                 bool                 `json:"agcEnabled" yaml:"agc_enabled" default:"false"`
	AGCConfig                  AGCConfig            `json:"agcConfig" yaml:"agc_config"`
	DTMFDetectionEnabled       bool                 `json:"dtmfDetectionEnabled" yaml:"dtmf_detection_enabled" default:"true"`
	DtmfThreshold              float64              `json:"dtmfThreshold" yaml:"dtmf_threshold" default:"0.09"`
}

// DefaultAudioProcessOption returns an AudioProcessOption with sensible defaults.
func DefaultAudioProcessOption() AudioProcessOption {
	return AudioProcessOption{
		VADEnabled:                 true,
		VADType:                    VADTypeSilero,
		VADOption:                  DefaultVADDetectorOption(),
		NoiseSuppressionEnabled:    true,
		NoiseSuppressionType:       NoiseSuppressionTypeONNX,
		NoiseSuppressionIntensity:  5,
		NoiseSuppressionONNXConfig: dfn.DefaultSuppressorConfig(),
		AGCEnabled:                 false,
		AGCConfig:                  DefaultAGCConfig(),
		DTMFDetectionEnabled:       true,
		DtmfThreshold:              0.09,
	}
}

// NewAudioProcess creates a new AudioProcess with the given options.
func NewAudioProcess(opt AudioProcessOption) *AudioProcess {
	ap := &AudioProcess{
		vadType: opt.VADType,
		nsType:  opt.NoiseSuppressionType,
	}

	if opt.VADEnabled {
		ap.vad = CreateVadProcessor(opt.VADType, opt.VADOption)
	}

	if opt.NoiseSuppressionEnabled {
		switch opt.NoiseSuppressionType {
		case NoiseSuppressionTypeRNNoise:
			ap.ns = gornnoise.NewRNNoise()
		case NoiseSuppressionTypeONNX:
			var err error
			onnxConfig := dfn.DefaultSuppressorConfig()
			// set NoiseSuppressionIntensity (AttenLimDB)
			if opt.NoiseSuppressionIntensity >= 30 {
				opt.NoiseSuppressionIntensity = 0 // max noise suppression
			}
			onnxConfig.AttenLimDB = float32(opt.NoiseSuppressionIntensity)

			ap.nsDFN, err = dfn.NewSuppressor(onnxConfig)
			if err != nil {
				slog.Warn("Initializing ONNX noise suppressor failed", "error", err.Error())
			} else {
				slog.Info("Initializing ONNX noise suppressor success", "intensity", opt.NoiseSuppressionIntensity)
			}
		default:
			// default NoiseSuppressionType rnnoise
			ap.nsType = NoiseSuppressionTypeRNNoise
			ap.ns = gornnoise.NewRNNoise()
		}
	}

	if opt.DTMFDetectionEnabled {
		ap.dtmf = NewDTMFDetector(opt.DtmfThreshold, opt.VADOption.SampleRate)
	}

	if opt.AGCEnabled {
		var err error
		ap.agc, err = NewAGCProcessor(uint32(opt.VADOption.SampleRate), &opt.AGCConfig)
		if err != nil {
			slog.Warn("Initializing AGC processor failed", "error", err.Error())
		} else {
			slog.Info("Initializing AGC processor success")
		}
	}

	return ap
}

// String returns a string representation of the AudioProcess.
func (ap *AudioProcess) String() string {
	if ap.ns != nil {
		return fmt.Sprintf("AudioProcess{VAD: %v, NS: {FrameSize: %d, Type: %s}}", ap.vad, gornnoise.GetFrameSize(), ap.nsType)
	}
	if ap.nsDFN != nil {
		return fmt.Sprintf("AudioProcess{VAD: %v, NS: {Type: %s}}", ap.vad, ap.nsType)
	}
	return fmt.Sprintf("AudioProcess{VAD: %v, NS: disabled}", ap.vad)
}

// HasVAD returns true if VAD is enabled.
func (ap *AudioProcess) HasVAD() bool {
	return ap.vad != nil
}

// HasNoiseSuppression returns true if noise suppression is enabled.
func (ap *AudioProcess) HasNoiseSuppression() bool {
	return ap.ns != nil || ap.nsDFN != nil
}

// Close releases all resources.
func (ap *AudioProcess) Close() {
	if ap.vad != nil {
		ap.vad.Close()
		ap.vad = nil
	}
	if ap.nsDFN != nil {
		ap.nsDFN.Destroy()
		ap.nsDFN = nil
	}
	if ap.agc != nil {
		ap.agc.Close()
		ap.agc = nil
	}
}

// Process processes an audio frame.
// sampleRate: the sample rate of the audio (e.g., 16000)
// payload: 16-bit PCM audio data (will be modified in place for noise suppression and AGC)
// vadHandler: callback for VAD events (may be nil)
// dtmfHandler: callback for DTMF events (may be nil)
// Returns the processed payload (may be the same slice or a new one).
func (ap *AudioProcess) Process(sampleRate int, payload []byte, vadHandler VADHandler, dtmfHandler DTMFHandler) ([]byte, error) {
	if len(payload) == 0 {
		return payload, nil
	}

	var err error

	// DTMF detection
	if ap.dtmf != nil && dtmfHandler != nil {
		ap.dtmf.Process(payload, dtmfHandler)
	}

	// Noise suppression
	if ap.ns != nil {
		// RNNoise requires 48kHz
		payload48k, err := codecs.ResamplePCM(payload, sampleRate, 48000)
		if err != nil {
			return payload, err
		}
		var output []byte
		fsize := gornnoise.GetFrameSize() * 2
		for i := 0; i < len(payload48k); i += fsize {
			end := i + fsize
			if end > len(payload48k) {
				end = len(payload48k)
			}
			buf := payload48k[i:end]
			buf = ap.ns.Process(buf)
			output = append(output, buf...)
		}
		payload, err = codecs.ResamplePCM(output, 48000, sampleRate)
		if err != nil {
			return payload, err
		}
	} else if ap.nsDFN != nil {
		var output []byte
		fsize := 320 // 10ms at 16kHz
		for i := 0; i < len(payload); i += fsize {
			end := i + fsize
			if end > len(payload) {
				end = len(payload)
			}
			buf := payload[i:end]
			buf, err = ap.nsDFN.Process(buf, sampleRate)
			output = append(output, buf...)
		}
		payload = output
		if err != nil {
			return payload, fmt.Errorf("ONNX noise suppression failed: %w", err)
		}
	}

	// AGC
	if ap.agc != nil {
		err = ap.agc.Process(payload)
		if err != nil {
			slog.Warn("AGC processing failed", "error", err.Error())
		}
	}

	// VAD
	if ap.vad != nil && vadHandler != nil {
		err = ap.vad.Process(payload, vadHandler)
	}

	return payload, err
}

// DTMFHandler is a callback function for DTMF events.
// sender: the source of the DTMF event (e.g., "detector", "frame")
// digit: the detected DTMF digit (0-9, *, #, A-D)
// Note: This type is also defined in dtmf.go. Both definitions are kept for backward compatibility.
// Deprecated: Use the DTMFHandler from dtmf.go instead.

// NormalizeDTMFHandler creates a DTMF handler with debouncing.
// It filters out repeated digits within the specified duration.
func NormalizeDTMFHandler(handler DTMFHandler, debounceDuration time.Duration) DTMFHandler {
	var lastKeyReceived time.Time
	var lastKey string

	return func(sender, digit string) {
		if lastKey == digit && time.Since(lastKeyReceived) < debounceDuration {
			return
		}
		digit = strings.ToUpper(strings.TrimSpace(digit))
		if !strings.Contains("0123456789*#ABCD", digit) {
			return
		}
		lastKey = digit
		lastKeyReceived = time.Now()
		handler(sender, digit)
	}
}
