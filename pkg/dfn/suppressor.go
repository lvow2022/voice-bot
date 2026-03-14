package dfn

import (
	"encoding/binary"
	"fmt"
	"math"
)

// Suppressor implements noise suppression using DFN ONNX model
// It uses a shared SessionPool for efficient resource utilization
type Suppressor struct {
	pool            *SessionPool
	states          []float32
	attenLimDb      float32
	hopSize         int
	fftSize         int
	ModelSampleRate int

	// Pre-allocated buffers to reduce memory allocations
	samplesBuffer  []float32
	paddedBuffer   []float32
	enhancedBuffer []float32
	bytesBuffer    []byte
	frameBuffer    []float32 // buffer for single frame inference result
}

// SuppressorConfig holds configuration for DFN noise suppressor
type SuppressorConfig struct {
	ModelPath       string
	ModelSampleRate int
	HopSize         int
	FFTSize         int
	AttenLimDB      float32
	PoolSize        int // optional, defaults to DefaultPoolSize
}

// DefaultSuppressorConfig returns default configuration
func DefaultSuppressorConfig() SuppressorConfig {
	modelPath := ResolveModelPath()
	return SuppressorConfig{
		ModelPath:       modelPath,
		ModelSampleRate: DefaultSampleRate,
		HopSize:         DefaultHopSize,
		FFTSize:         DefaultFFTSize,
		AttenLimDB:      0.0,
		PoolSize:        DefaultPoolSize,
	}
}

// NewSuppressor creates a new DFN-based noise suppressor
func NewSuppressor(config SuppressorConfig) (*Suppressor, error) {
	// Build dfn.Config
	dfnConfig := Config{
		ModelPath:       config.ModelPath,
		ModelSampleRate: config.ModelSampleRate,
		HopSize:         config.HopSize,
		FFTSize:         config.FFTSize,
		AttenLimDB:      config.AttenLimDB,
	}

	// Use default values if not set
	if dfnConfig.ModelPath == "" {
		dfnConfig.ModelPath = ResolveModelPath()
	}
	if dfnConfig.ModelSampleRate == 0 {
		dfnConfig.ModelSampleRate = DefaultSampleRate
	}
	if dfnConfig.HopSize == 0 {
		dfnConfig.HopSize = DefaultHopSize
	}
	if dfnConfig.FFTSize == 0 {
		dfnConfig.FFTSize = DefaultFFTSize
	}

	poolSize := config.PoolSize
	if poolSize <= 0 {
		poolSize = DefaultPoolSize
	}

	// Get or create global session pool
	pool, err := GetGlobalPool(dfnConfig, poolSize)
	if err != nil {
		return nil, fmt.Errorf("failed to get session pool: %w", err)
	}

	// Each instance has its own states
	states := make([]float32, StateLen)

	return &Suppressor{
		pool:            pool,
		states:          states,
		attenLimDb:      config.AttenLimDB,
		hopSize:         dfnConfig.HopSize,
		fftSize:         dfnConfig.FFTSize,
		ModelSampleRate: dfnConfig.ModelSampleRate,
		frameBuffer:     make([]float32, dfnConfig.HopSize),
	}, nil
}

// Process applies noise suppression to PCM audio data (16-bit little-endian)
// sampleRate: the sample rate of the input audio
// Returns: enhanced PCM audio data
func (s *Suppressor) Process(audioData []byte, sampleRate int) ([]byte, error) {
	var err error

	// Convert byte data to float32 samples, reusing pre-allocated buffer
	s.samplesBuffer, err = bytesToFloat32(audioData, s.samplesBuffer)
	if err != nil {
		return nil, fmt.Errorf("failed to convert bytes to float32: %w", err)
	}

	// Resample if necessary
	if sampleRate != s.ModelSampleRate {
		// Resample input to model sample rate
		s.samplesBuffer = resampleFloat32(s.samplesBuffer, sampleRate, s.ModelSampleRate)
	}

	// Apply noise suppression
	enhancedSamples, err := s.ProcessSamples(s.samplesBuffer)
	if err != nil {
		return nil, fmt.Errorf("failed to process samples: %w", err)
	}

	// Resample back to original sample rate if necessary
	if sampleRate != s.ModelSampleRate {
		enhancedSamples = resampleFloat32(enhancedSamples, s.ModelSampleRate, sampleRate)
	}

	// Convert back to bytes, reusing pre-allocated buffer
	s.bytesBuffer, err = float32ToBytes(enhancedSamples, s.bytesBuffer)
	if err != nil {
		return nil, fmt.Errorf("failed to convert float32 to bytes: %w", err)
	}

	return s.bytesBuffer, nil
}

// ProcessSamples applies noise suppression to float32 audio samples
// samples: normalized float32 samples in range [-1.0, 1.0]
// Returns: enhanced float32 samples
func (s *Suppressor) ProcessSamples(samples []float32) ([]float32, error) {
	if len(samples) == 0 {
		return samples, nil
	}

	// Pad audio to make it divisible by hop_size
	origLen := len(samples)
	hopSizeDivisiblePaddingSize := (s.hopSize - origLen%s.hopSize) % s.hopSize
	origLen += hopSizeDivisiblePaddingSize

	// Reuse or expand paddedBuffer
	paddedLen := origLen + s.fftSize
	if cap(s.paddedBuffer) < paddedLen {
		s.paddedBuffer = make([]float32, paddedLen)
	} else {
		s.paddedBuffer = s.paddedBuffer[:paddedLen]
		// Clear the buffer (zero out padding area)
		for i := len(samples); i < paddedLen; i++ {
			s.paddedBuffer[i] = 0
		}
	}
	copy(s.paddedBuffer, samples)

	// Calculate expected output size and pre-allocate enhancedBuffer
	numFrames := origLen / s.hopSize
	expectedOutputLen := numFrames * s.hopSize
	if cap(s.enhancedBuffer) < expectedOutputLen {
		s.enhancedBuffer = make([]float32, 0, expectedOutputLen)
	} else {
		s.enhancedBuffer = s.enhancedBuffer[:0]
	}

	// Process in chunks using the pool
	for i := 0; i < origLen; i += s.hopSize {
		end := i + s.hopSize
		if end > len(s.paddedBuffer) {
			break
		}

		frame := s.paddedBuffer[i:end]

		// Use zero-allocation inference
		err := s.pool.RunInference(frame, s.states, s.attenLimDb, s.frameBuffer)
		if err != nil {
			return nil, err
		}

		s.enhancedBuffer = append(s.enhancedBuffer, s.frameBuffer...)
	}

	// Remove padding
	d := s.fftSize - s.hopSize
	if len(s.enhancedBuffer) > d+origLen {
		return s.enhancedBuffer[d : d+origLen], nil
	}

	return s.enhancedBuffer, nil
}

// Reset resets the internal states for a new audio stream
func (s *Suppressor) Reset() {
	for i := range s.states {
		s.states[i] = 0
	}
}

// Destroy cleans up resources
// Note: The session pool is shared and will not be destroyed
func (s *Suppressor) Destroy() {
	// Clear references but don't destroy the shared pool
	s.pool = nil
	s.states = nil
	s.samplesBuffer = nil
	s.paddedBuffer = nil
	s.enhancedBuffer = nil
	s.bytesBuffer = nil
	s.frameBuffer = nil
}

// bytesToFloat32 converts PCM bytes (16-bit little-endian) to normalized float32 samples
func bytesToFloat32(data []byte, buffer []float32) ([]float32, error) {
	if len(data)%2 != 0 {
		return nil, fmt.Errorf("invalid PCM data length: %d (must be even)", len(data))
	}

	numSamples := len(data) / 2
	if cap(buffer) < numSamples {
		buffer = make([]float32, numSamples)
	} else {
		buffer = buffer[:numSamples]
	}

	for i := 0; i < numSamples; i++ {
		sample := int16(binary.LittleEndian.Uint16(data[i*2:]))
		buffer[i] = float32(sample) / 32768.0
	}

	return buffer, nil
}

// float32ToBytes converts normalized float32 samples to PCM bytes (16-bit little-endian)
func float32ToBytes(samples []float32, buffer []byte) ([]byte, error) {
	numBytes := len(samples) * 2
	if cap(buffer) < numBytes {
		buffer = make([]byte, numBytes)
	} else {
		buffer = buffer[:numBytes]
	}

	for i, sample := range samples {
		// Clamp to [-1.0, 1.0]
		if sample > 1.0 {
			sample = 1.0
		} else if sample < -1.0 {
			sample = -1.0
		}
		intSample := int16(sample * 32767.0)
		binary.LittleEndian.PutUint16(buffer[i*2:], uint16(intSample))
	}

	return buffer, nil
}

// resampleFloat32 performs simple linear interpolation resampling
func resampleFloat32(samples []float32, fromRate, toRate int) []float32 {
	if fromRate == toRate {
		return samples
	}

	ratio := float64(fromRate) / float64(toRate)
	newLen := int(math.Ceil(float64(len(samples)) / ratio))
	result := make([]float32, newLen)

	for i := 0; i < newLen; i++ {
		srcPos := float64(i) * ratio
		srcIdx := int(srcPos)
		frac := float32(srcPos - float64(srcIdx))

		if srcIdx+1 < len(samples) {
			result[i] = samples[srcIdx]*(1-frac) + samples[srcIdx+1]*frac
		} else if srcIdx < len(samples) {
			result[i] = samples[srcIdx]
		}
	}

	return result
}
