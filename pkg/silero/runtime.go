package silero

import (
	"fmt"
	"sync"

	ort "github.com/yalue/onnxruntime_go"
)

var ortEnvOnce sync.Once
var ortEnvErr error

func logLevelToOrtEnvOption(l LogLevel) ort.EnvironmentOption {
	switch l {
	case LevelVerbose:
		return ort.WithLogLevelVerbose()
	case LogLevelInfo:
		return ort.WithLogLevelInfo()
	case LogLevelWarn:
		return ort.WithLogLevelWarning()
	case LogLevelError:
		return ort.WithLogLevelError()
	case LogLevelFatal:
		return ort.WithLogLevelFatal()
	default:
		return ort.WithLogLevelWarning()
	}
}

func ensureOrtEnv(cfg DetectorConfig) error {
	// If already initialized by another package (e.g., noise_suppressor_onnx), skip initialization
	if ort.IsInitialized() {
		return nil
	}

	ortEnvOnce.Do(func() {
		// Double-check after acquiring the once lock
		if ort.IsInitialized() {
			return
		}

		libPath := cfg.SharedLibraryPath
		if libPath == "" {
			ortEnvErr = fmt.Errorf("shared library path is empty")
			return
		}
		ort.SetSharedLibraryPath(libPath)
		ortEnvErr = ort.InitializeEnvironment(logLevelToOrtEnvOption(cfg.LogLevel))
		if ortEnvErr != nil {
			ortEnvErr = fmt.Errorf("failed to initialize onnxruntime_go (shared_library=%q): %w\n\n"+
				"Hint: The onnxruntime version you loaded is too old or mismatched (e.g., ORT 1.18.x only supports API<=18), "+
				"but github.com/yalue/onnxruntime_go v1.22.0 requires ORT 1.22.x (API=22). "+
				"Please install a matching version of the onnxruntime shared library and specify it via DetectorConfig.SharedLibraryPath.",
				libPath, ortEnvErr)
		}
	})
	return ortEnvErr
}
