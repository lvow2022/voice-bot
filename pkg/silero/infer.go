package silero

import (
	"fmt"
)

// Infer performs inference with context management for batch/offline processing.
// It prepends context from the previous frame (after the first frame) to maintain
// continuity across frames. Use this method when processing audio in sequence
// via the Detect() method.
func (sd *Detector) Infer(samples []float32) (float32, error) {
	if sd == nil {
		return 0, fmt.Errorf("invalid nil detector")
	}
	if len(samples) == 0 {
		return 0, fmt.Errorf("empty samples")
	}

	// Build input with context
	inputSize := sd.windowSize + contextLen
	inputSamples := make([]float32, inputSize)

	// After first frame, prepend contextLen samples from previous frame
	if sd.currSample > 0 {
		// Copy context to the beginning
		copy(inputSamples[:contextLen], sd.ctx[:])
		// Copy samples after context
		copy(inputSamples[contextLen:], samples)
	} else {
		// First frame: pad with zeros at the beginning, then samples
		// (inputSamples is already zero-initialized)
		copy(inputSamples[contextLen:], samples)
	}

	// Save context for next iteration (last contextLen samples of current frame)
	if len(samples) >= contextLen {
		copy(sd.ctx[:], samples[len(samples)-contextLen:])
	} else {
		copy(sd.ctx[:], samples)
	}

	return sd.infer(inputSamples)
}

// InferRaw performs inference on raw samples without any context management.
// The caller is responsible for preparing the input (including any context prepending).
// Use this method when you manage context externally in streaming scenarios.
func (sd *Detector) InferRaw(samples []float32) (float32, error) {
	if sd == nil {
		return 0, fmt.Errorf("invalid nil detector")
	}
	if len(samples) == 0 {
		return 0, fmt.Errorf("empty samples")
	}

	return sd.infer(samples)
}

// infer is the internal inference implementation.
// It uses the session pool to perform inference.
func (sd *Detector) infer(inputSamples []float32) (float32, error) {
	if sd.pool == nil {
		return 0, fmt.Errorf("session pool is nil")
	}

	// Use the pool to run inference
	// The pool handles session acquisition, inference, and release
	return sd.pool.RunInference(inputSamples, &sd.state)
}
