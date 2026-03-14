package silero

import (
	"fmt"
	"log/slog"
	"sync"
)

const (
	stateLen   = 2 * 1 * 128
	contextLen = 64
)

type LogLevel int

const (
	LevelVerbose LogLevel = iota + 1
	LogLevelInfo
	LogLevelWarn
	LogLevelError
	LogLevelFatal
)

type DetectorConfig struct {
	// The path to the ONNX Silero VAD model file to load.
	ModelPath string
	// The sampling rate of the input audio samples. Supported values are 8000 and 16000.
	SampleRate int
	// The probability threshold above which we detect speech. A good default is 0.5.
	Threshold float32
	// The duration of silence to wait for each speech segment before separating it.
	MinSilenceDurationMs int
	// The padding to add to speech segments to avoid aggressive cutting.
	SpeechPadMs int
	// The loglevel for the onnx environment, by default it is set to LogLevelWarn.
	LogLevel LogLevel

	// Path to the onnxruntime shared library (e.g. libonnxruntime.dylib / libonnxruntime.so).
	// If non-empty, this path will be used directly (no environment variable lookup).
	// If empty, we fall back to best-effort auto-detection on common install paths.
	SharedLibraryPath string
}

func (c DetectorConfig) IsValid() error {
	if c.ModelPath == "" {
		return fmt.Errorf("invalid ModelPath: should not be empty")
	}

	if c.SampleRate != 8000 && c.SampleRate != 16000 {
		return fmt.Errorf("invalid SampleRate: valid values are 8000 and 16000")
	}

	if c.Threshold <= 0 || c.Threshold >= 1 {
		return fmt.Errorf("invalid Threshold: should be in range (0, 1)")
	}

	if c.MinSilenceDurationMs < 0 {
		return fmt.Errorf("invalid MinSilenceDurationMs: should be a positive number")
	}

	if c.SpeechPadMs < 0 {
		return fmt.Errorf("invalid SpeechPadMs: should be a positive number")
	}

	return nil
}

type Detector struct {
	cfg DetectorConfig

	state [stateLen]float32
	ctx   [contextLen]float32

	mu sync.Mutex

	currSample int
	triggered  bool
	tempEnd    int

	// Session pool for inference (shared across detectors)
	pool *SessionPool

	// Window size for this detector (512@16k, 256@8k)
	windowSize int
}

// NewDetector creates a new detector using the global session pool.
// The pool is initialized on first use with DefaultPoolSize sessions.
func NewDetector(cfg DetectorConfig) (*Detector, error) {
	return NewDetectorWithPoolSize(cfg, DefaultPoolSize)
}

// NewDetectorWithPoolSize creates a new detector with a specified pool size.
// All detectors with the same config share the same pool.
func NewDetectorWithPoolSize(cfg DetectorConfig, poolSize int) (*Detector, error) {
	if err := cfg.IsValid(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	// Get or create the global session pool
	pool, err := GetGlobalPool(cfg, poolSize)
	if err != nil {
		return nil, fmt.Errorf("failed to get session pool: %w", err)
	}

	// Determine window size based on sample rate
	windowSize := 512
	if cfg.SampleRate == 8000 {
		windowSize = 256
	}

	return &Detector{
		cfg:        cfg,
		pool:       pool,
		windowSize: windowSize,
	}, nil
}

// Segment contains timing information of a speech segment.
type Segment struct {
	// The relative timestamp in seconds of when a speech segment begins.
	SpeechStartAt float64
	// The relative timestamp in seconds of when a speech segment ends.
	SpeechEndAt float64
}

var frameCount = 0

func (sd *Detector) Detect(pcm []float32) ([]Segment, error) {
	if sd == nil {
		return nil, fmt.Errorf("invalid nil detector")
	}

	windowSize := 512
	if sd.cfg.SampleRate == 8000 {
		windowSize = 256
	}

	if len(pcm) < windowSize {
		return nil, fmt.Errorf("not enough samples")
	}

	slog.Debug("starting speech detection", slog.Int("samplesLen", len(pcm)))

	minSilenceSamples := sd.cfg.MinSilenceDurationMs * sd.cfg.SampleRate / 1000
	speechPadSamples := sd.cfg.SpeechPadMs * sd.cfg.SampleRate / 1000

	var segments []Segment
	for i := 0; i < len(pcm)-windowSize; i += windowSize {
		frameCount++
		speechProb, err := sd.Infer(pcm[i : i+windowSize])
		if err != nil {
			return nil, fmt.Errorf("infer failed: %w", err)
		}
		fmt.Printf("frame:%d speechProb: %v\n", frameCount, speechProb)
		sd.currSample += windowSize

		if speechProb >= sd.cfg.Threshold && sd.tempEnd != 0 {
			sd.tempEnd = 0
		}

		if speechProb >= sd.cfg.Threshold && !sd.triggered {
			sd.triggered = true
			speechStartAt := (float64(sd.currSample-windowSize-speechPadSamples) / float64(sd.cfg.SampleRate))

			// We clamp at zero since due to padding the starting position could be negative.
			if speechStartAt < 0 {
				speechStartAt = 0
			}

			slog.Debug("speech start", slog.Float64("startAt", speechStartAt))
			segments = append(segments, Segment{
				SpeechStartAt: speechStartAt,
			})
		}

		if speechProb < (sd.cfg.Threshold-0.15) && sd.triggered {
			if sd.tempEnd == 0 {
				sd.tempEnd = sd.currSample
			}

			// Not enough silence yet to split, we continue.
			if sd.currSample-sd.tempEnd < minSilenceSamples {
				continue
			}

			speechEndAt := (float64(sd.tempEnd+speechPadSamples) / float64(sd.cfg.SampleRate))
			sd.tempEnd = 0
			sd.triggered = false
			slog.Debug("speech end", slog.Float64("endAt", speechEndAt))

			if len(segments) < 1 {
				return nil, fmt.Errorf("unexpected speech end")
			}

			segments[len(segments)-1].SpeechEndAt = speechEndAt
		}
	}

	slog.Debug("speech detection done", slog.Int("segmentsLen", len(segments)))

	return segments, nil
}

func (sd *Detector) Reset() error {
	if sd == nil {
		return fmt.Errorf("invalid nil detector")
	}

	sd.currSample = 0
	sd.triggered = false
	sd.tempEnd = 0
	for i := 0; i < stateLen; i++ {
		sd.state[i] = 0
	}
	for i := 0; i < contextLen; i++ {
		sd.ctx[i] = 0
	}

	return nil
}

func (sd *Detector) SetThreshold(value float32) {
	sd.cfg.Threshold = value
}

func (sd *Detector) Destroy() error {
	if sd == nil {
		return fmt.Errorf("invalid nil detector")
	}

	// Detector no longer owns session resources.
	// The pool manages session lifecycle.
	// Just reset the detector state.
	sd.pool = nil

	return nil
}
