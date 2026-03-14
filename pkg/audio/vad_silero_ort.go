package audio

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"voicebot/pkg/silero"
)

func init() {
	RegisterVAD(VADTypeSilero, NewSileroORTVADDetector)
}

// contextLen is the number of samples to keep from the previous frame for context continuity.
// This matches the value in pkg/silero/detector.go.
const contextLen = 64

// Default paths for Silero VAD
const (
	// DefaultSileroVADModelPath is the default path to the Silero VAD ONNX model
	DefaultSileroVADModelPath = "../../models/silero_vad.onnx"
	// DefaultONNXRuntimeLibPathDarwinVAD is the default ONNX runtime library path for macOS
	DefaultONNXRuntimeLibPathDarwinVAD = "/opt/homebrew/lib/libonnxruntime.dylib"
	// DefaultONNXRuntimeLibPathLinuxVAD is the default ONNX runtime library path for Linux
	DefaultONNXRuntimeLibPathLinuxVAD = "../../models/libonnxruntime.so"
)

// SileroORTVADDetector implements VAD using pkg/silero (pure Go + onnxruntime_go).
//
// Notes:
// - This implementation is stream-oriented: it calls Detector.Infer() on fixed windows (512@16k / 256@8k).
// - It reuses the existing voiceserver hysteresis/state-machine logic (positive/negative thresholds + frame counters).
// - It maintains context from the previous frame to ensure continuity across frame boundaries.
type SileroORTVADDetector struct {
	VADDetectorOption

	det      *silero.Detector
	duration time.Duration
	state    VADState

	frameDuration  time.Duration
	frameBuffer    *RingBuffer[byte]
	chunkSize      int
	speakingFrames int
	silenceFrames  int

	samplesBuffer  []float32
	pcmFrameBuffer []byte

	// context management for cross-frame continuity
	ctx         [contextLen]float32
	frameCount  int // number of frames processed, used to determine if context should be prepended
	inferBuffer []float32

	// Pre-computed constants to avoid recalculation per frame
	bytesPerFrame       int
	bytesPerMillisecond int
	frameDurationMs     int
}

func NewSileroORTVADDetector(opt VADDetectorOption) VADDetector {
	v := &SileroORTVADDetector{
		VADDetectorOption: opt,
		state:             VADStateBeginTransition,
		chunkSize:         512,
	}

	v.frameDuration = ParseFrameDuration(opt.FrameDuration)

	if v.SampleRate == 8000 {
		v.chunkSize = 256
	}

	bytesPerSample := v.BitDepth / 8
	bytesPerFrame := v.chunkSize * bytesPerSample
	vadFrameDurationMs := (v.chunkSize * 1000) / v.SampleRate // 32ms@16k, 32ms@8k
	bytesPerMillisecond := GetSampleSize(v.SampleRate, v.BitDepth, v.Channels)

	v.frameBuffer = NewRingBuffer[byte](bytesPerFrame * 3)
	v.samplesBuffer = make([]float32, v.chunkSize)
	v.pcmFrameBuffer = make([]byte, bytesPerFrame)
	v.inferBuffer = make([]float32, v.chunkSize+contextLen) // pre-allocate for context + samples

	// Store pre-computed constants
	v.bytesPerFrame = bytesPerFrame
	v.bytesPerMillisecond = bytesPerMillisecond
	v.frameDurationMs = bytesPerFrame / bytesPerMillisecond

	v.VADDetectorOption = NormalizeVADDetectorOption(opt, vadFrameDurationMs)

	modelPath := resolveSileroVADModelPath()
	sharedLibPath := resolveOnnxRuntimeLibPathVAD()
	cfg := silero.DetectorConfig{
		ModelPath:         modelPath,
		SharedLibraryPath: sharedLibPath,
		SampleRate:        v.SampleRate,
		Threshold:         0.5,
	}

	// Use session pool for better memory efficiency and concurrent performance
	// All detectors share the same pool (default size: 4 sessions)
	d, err := silero.NewDetector(cfg)
	if err != nil {
		panic(fmt.Sprintf("failed to initialize silerovad-onnx model: %v", err))
	}
	v.det = d

	slog.Info("silerovad-onnx detector initialized with session pool", "poolSize", silero.DefaultPoolSize)

	return v
}

func (v *SileroORTVADDetector) String() string {
	return fmt.Sprintf(
		"SileroORTVADDetector{positiveSpeechThreshold:%f, negativeSpeechThreshold:%f, redemptionFrames:%d, minSpeechFrames:%d}",
		v.PositiveSpeechThreshold, v.NegativeSpeechThreshold, v.RedemptionFrames, v.MinSpeechFrames,
	)
}

func (v *SileroORTVADDetector) Process(frame []byte, handler VADHandler) error {
	// Validate input frame length
	frameLength := len(frame)
	if frameLength%v.bytesPerMillisecond != 0 {
		return fmt.Errorf("frame length %d is not a multiple of %d", frameLength, v.bytesPerMillisecond)
	}

	v.frameBuffer.Write(frame)

	// Use pre-computed constants
	bytesPerFrame := v.bytesPerFrame

	for v.frameBuffer.Len() >= bytesPerFrame {
		// Read directly without redundant Peek
		n := v.frameBuffer.Read(v.pcmFrameBuffer)
		if n < bytesPerFrame {
			break
		}

		pcmFrame := v.pcmFrameBuffer[:bytesPerFrame]
		v.duration += time.Duration(v.frameDurationMs) * time.Millisecond

		floatSamples, err := BytesToFloat32(pcmFrame, v.samplesBuffer)
		if err != nil {
			return err
		}

		// Build input with context: always use windowSize + contextLen length
		// First frame: pad with zeros at the beginning, then samples
		// Subsequent frames: prepend contextLen samples from previous frame
		if v.frameCount > 0 {
			// Prepend context from previous frame
			copy(v.inferBuffer[:contextLen], v.ctx[:])
		} else {
			// First frame: zero-initialize context portion
			for i := 0; i < contextLen; i++ {
				v.inferBuffer[i] = 0
			}
		}
		copy(v.inferBuffer[contextLen:], floatSamples)

		// Save context for next iteration (last contextLen samples of current frame)
		if len(floatSamples) >= contextLen {
			copy(v.ctx[:], floatSamples[len(floatSamples)-contextLen:])
		} else {
			copy(v.ctx[:], floatSamples)
		}
		v.frameCount++

		speechProb, err := v.det.InferRaw(v.inferBuffer[:contextLen+len(floatSamples)])
		if err != nil {
			return err
		}

		switch v.state {
		case VADStateBeginTransition, VADStateInactivityTransition:
			if speechProb >= v.PositiveSpeechThreshold {
				v.speakingFrames++
			} else {
				v.speakingFrames = 0
			}

			if v.speakingFrames >= v.MinSpeechFrames {
				v.state = VADStateActivityTransition
				v.speakingFrames = 0
				v.silenceFrames = 0
				handler(v.duration, true, false)
			}

		case VADStateActivityTransition:
			if speechProb >= v.NegativeSpeechThreshold {
				v.silenceFrames = 0
			} else {
				v.silenceFrames++
			}

			if v.silenceFrames >= v.RedemptionFrames {
				v.state = VADStateInactivityTransition
				v.silenceFrames = 0
				handler(v.duration, false, true)
			}
		}
	}

	return nil
}

func (v *SileroORTVADDetector) Close() error {
	if v.det != nil {
		_ = v.det.Reset()
		_ = v.det.Destroy()
		v.det = nil
	}
	if v.frameBuffer != nil {
		v.frameBuffer.Reset()
	}
	return nil
}

// resolveSileroVADModelPath finds the Silero VAD ONNX model path
func resolveSileroVADModelPath() string {
	modelName := "silero_vad.onnx"
	candidates := []string{
		filepath.Join("models", modelName),
		filepath.Join("..", "..", "models", modelName),
	}

	// Add path relative to this source file
	if _, thisFile, _, ok := runtime.Caller(0); ok {
		repoRoot := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", ".."))
		candidates = append([]string{filepath.Join(repoRoot, "models", modelName)}, candidates...)
	}

	for _, p := range candidates {
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p
		}
	}

	return filepath.Join("models", modelName)
}

// resolveOnnxRuntimeLibPathVAD returns the ONNX runtime shared library path based on platform
func resolveOnnxRuntimeLibPathVAD() string {
	switch runtime.GOOS {
	case "darwin":
		return DefaultONNXRuntimeLibPathDarwinVAD
	default:
		// For Linux, try to resolve the library path relative to the source file
		libName := "libonnxruntime.so"
		candidates := []string{
			filepath.Join("models", libName),
			filepath.Join("..", "..", "models", libName),
		}

		// Add path relative to this source file
		if _, thisFile, _, ok := runtime.Caller(0); ok {
			repoRoot := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", ".."))
			candidates = append([]string{filepath.Join(repoRoot, "models", libName)}, candidates...)
		}

		for _, p := range candidates {
			if st, err := os.Stat(p); err == nil && !st.IsDir() {
				return p
			}
		}

		// Fallback to default path
		return DefaultONNXRuntimeLibPathLinuxVAD
	}
}
