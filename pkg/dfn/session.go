package dfn

import (
	"fmt"

	ort "github.com/yalue/onnxruntime_go"
)

// pooledSession represents a reusable ONNX session with pre-allocated tensors
type pooledSession struct {
	session       *ort.AdvancedSession
	inputTensors  []ort.Value // input_frame, states, atten_lim_db
	outputTensors []ort.Value // enhanced_audio_frame, new_states, lsnr
	hopSize       int
}

// newPooledSession creates a new pooled session with pre-allocated tensors
func newPooledSession(cfg Config) (*pooledSession, error) {
	// Define input and output names (from DFN model)
	inputNames := []string{"input_frame", "states", "atten_lim_db"}
	outputNames := []string{"enhanced_audio_frame", "new_states", "lsnr"}

	// Initialize states and attenuation limit tensors
	states := make([]float32, StateLen)
	attenLimDb := []float32{cfg.AttenLimDB}

	// Create input tensors
	inputFrameTensor, err := ort.NewEmptyTensor[float32](ort.NewShape(int64(cfg.HopSize)))
	if err != nil {
		return nil, fmt.Errorf("failed to create input frame tensor: %w", err)
	}

	statesTensor, err := ort.NewTensor(ort.NewShape(StateLen), states)
	if err != nil {
		inputFrameTensor.Destroy()
		return nil, fmt.Errorf("failed to create states tensor: %w", err)
	}

	attenLimDbTensor, err := ort.NewTensor(ort.NewShape(1), attenLimDb)
	if err != nil {
		inputFrameTensor.Destroy()
		statesTensor.Destroy()
		return nil, fmt.Errorf("failed to create atten_lim_db tensor: %w", err)
	}

	// Create output tensors
	enhancedFrameTensor, err := ort.NewEmptyTensor[float32](ort.NewShape(int64(cfg.HopSize)))
	if err != nil {
		inputFrameTensor.Destroy()
		statesTensor.Destroy()
		attenLimDbTensor.Destroy()
		return nil, fmt.Errorf("failed to create enhanced frame tensor: %w", err)
	}

	newStatesTensor, err := ort.NewEmptyTensor[float32](ort.NewShape(StateLen))
	if err != nil {
		inputFrameTensor.Destroy()
		statesTensor.Destroy()
		attenLimDbTensor.Destroy()
		enhancedFrameTensor.Destroy()
		return nil, fmt.Errorf("failed to create new states tensor: %w", err)
	}

	lsnrTensor, err := ort.NewEmptyTensor[float32](ort.NewShape(1))
	if err != nil {
		inputFrameTensor.Destroy()
		statesTensor.Destroy()
		attenLimDbTensor.Destroy()
		enhancedFrameTensor.Destroy()
		newStatesTensor.Destroy()
		return nil, fmt.Errorf("failed to create lsnr tensor: %w", err)
	}

	// Create session inputs and outputs
	inputs := []ort.Value{inputFrameTensor, statesTensor, attenLimDbTensor}
	outputs := []ort.Value{enhancedFrameTensor, newStatesTensor, lsnrTensor}

	// Create session options
	sessionOptions, err := ort.NewSessionOptions()
	if err != nil {
		destroyTensors(inputs, outputs)
		return nil, fmt.Errorf("failed to create session options: %w", err)
	}
	defer sessionOptions.Destroy()

	// Set session options for optimal performance
	sessionOptions.SetIntraOpNumThreads(1)
	sessionOptions.SetInterOpNumThreads(1)
	sessionOptions.SetGraphOptimizationLevel(ort.GraphOptimizationLevelEnableAll)

	// Create session
	session, err := ort.NewAdvancedSession(
		cfg.ModelPath,
		inputNames,
		outputNames,
		inputs,
		outputs,
		sessionOptions,
	)
	if err != nil {
		destroyTensors(inputs, outputs)
		return nil, fmt.Errorf("failed to create ONNX session: %w", err)
	}

	return &pooledSession{
		session:       session,
		inputTensors:  inputs,
		outputTensors: outputs,
		hopSize:       cfg.HopSize,
	}, nil
}

// destroy cleans up the session and its tensors
func (s *pooledSession) destroy() {
	if s == nil {
		return
	}

	if s.session != nil {
		_ = s.session.Destroy()
		s.session = nil
	}

	destroyTensors(s.inputTensors, s.outputTensors)
	s.inputTensors = nil
	s.outputTensors = nil
}

// destroyTensors is a helper to destroy input and output tensors
func destroyTensors(inputs, outputs []ort.Value) {
	for _, tensor := range inputs {
		if tensor != nil {
			tensor.Destroy()
		}
	}
	for _, tensor := range outputs {
		if tensor != nil {
			tensor.Destroy()
		}
	}
}
