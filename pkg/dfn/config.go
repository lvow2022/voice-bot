package dfn

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// Constants for DFN model
const (
	// DefaultPoolSize is the default number of sessions in the pool
	DefaultPoolSize = 16

	// DefaultSampleRate is the default sample rate for the model
	DefaultSampleRate = 16000

	// DefaultHopSize is the default hop size (10ms at 16kHz)
	DefaultHopSize = 160

	// DefaultFFTSize is the default FFT size
	DefaultFFTSize = 320

	// StateLen is the size of the model state
	StateLen = 36984

	// DefaultONNXRuntimeLibPathDarwin is the default ONNX runtime library path for macOS
	DefaultONNXRuntimeLibPathDarwin = "/opt/homebrew/lib/libonnxruntime.dylib"

	// DefaultONNXRuntimeLibPathLinux is the default ONNX runtime library path for Linux
	DefaultONNXRuntimeLibPathLinux = "../../models/libonnxruntime.so"
)

// Config holds configuration for DFN noise suppressor
type Config struct {
	ModelPath       string
	ModelSampleRate int
	HopSize         int
	FFTSize         int
	AttenLimDB      float32
}

// DefaultConfig returns default configuration
func DefaultConfig() Config {
	modelPath := ResolveModelPath()
	return Config{
		ModelPath:       modelPath,
		ModelSampleRate: DefaultSampleRate,
		HopSize:         DefaultHopSize,
		FFTSize:         DefaultFFTSize,
		AttenLimDB:      0.0,
	}
}

// Validate checks if the configuration is valid
func (c Config) Validate() error {
	if c.ModelPath == "" {
		return fmt.Errorf("model path is required")
	}

	if _, err := os.Stat(c.ModelPath); os.IsNotExist(err) {
		return fmt.Errorf("model file not found: %s", c.ModelPath)
	}

	if c.ModelSampleRate <= 0 {
		return fmt.Errorf("invalid sample rate: %d", c.ModelSampleRate)
	}

	if c.HopSize <= 0 {
		return fmt.Errorf("invalid hop size: %d", c.HopSize)
	}

	if c.FFTSize <= 0 {
		return fmt.Errorf("invalid FFT size: %d", c.FFTSize)
	}

	return nil
}

// ResolveModelPath finds the denoiser ONNX model path
func ResolveModelPath() string {
	modelName := "denoiser_model_10s_16k.onnx"
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

// ResolveOnnxRuntimeLibPath returns the ONNX runtime shared library path based on platform
func ResolveOnnxRuntimeLibPath() string {
	switch runtime.GOOS {
	case "darwin":
		return DefaultONNXRuntimeLibPathDarwin
	default:
		return DefaultONNXRuntimeLibPathLinux
	}
}
