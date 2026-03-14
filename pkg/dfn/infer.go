package dfn

import (
	"fmt"

	ort "github.com/yalue/onnxruntime_go"
)

// RunInference performs inference and writes results to provided buffers
// This is a zero-allocation method using caller-provided buffers
// frame: input audio frame (hopSize samples)
// states: caller-managed state (StateLen floats), will be updated with new states
// attenLimDb: attenuation limit in dB
// enhancedOut: output buffer for enhanced audio (must be at least hopSize)
func (p *SessionPool) RunInference(
	frame []float32,
	states []float32,
	attenLimDb float32,
	enhancedOut []float32,
) error {
	if len(states) != StateLen {
		return fmt.Errorf("invalid states length: got %d, want %d", len(states), StateLen)
	}

	sess := p.Acquire()
	defer p.Release(sess)

	if len(enhancedOut) < sess.hopSize {
		return fmt.Errorf("enhancedOut buffer too small: got %d, want at least %d", len(enhancedOut), sess.hopSize)
	}

	// Update input tensors with new data
	inputFrameTensor := sess.inputTensors[0].(*ort.Tensor[float32])
	inputData := inputFrameTensor.GetData()
	copy(inputData, frame)

	statesTensor := sess.inputTensors[1].(*ort.Tensor[float32])
	statesData := statesTensor.GetData()
	copy(statesData, states)

	attenLimDbTensor := sess.inputTensors[2].(*ort.Tensor[float32])
	attenLimDbData := attenLimDbTensor.GetData()
	attenLimDbData[0] = attenLimDb

	// Run inference
	if err := sess.session.Run(); err != nil {
		return fmt.Errorf("failed to run inference: %w", err)
	}

	// Extract results
	enhancedFrameTensor := sess.outputTensors[0].(*ort.Tensor[float32])
	enhancedFrame := enhancedFrameTensor.GetData()

	newStatesTensor := sess.outputTensors[1].(*ort.Tensor[float32])
	newStates := newStatesTensor.GetData()

	// Update caller's states and output buffer
	copy(states, newStates)
	copy(enhancedOut, enhancedFrame)

	return nil
}
