// Package codecs provides audio codec utilities including resampling.
package codecs

import (
	"encoding/binary"
	"fmt"
	"math"
)

// ResamplePCM resamples 16-bit PCM audio data from one sample rate to another.
// Uses linear interpolation for simplicity and reasonable quality.
func ResamplePCM(pcm []byte, fromRate, toRate int) ([]byte, error) {
	if len(pcm)%2 != 0 {
		return nil, fmt.Errorf("pcm data length must be even for 16-bit audio")
	}

	if fromRate == toRate {
		return pcm, nil
	}

	// Convert bytes to int16 samples
	numSamples := len(pcm) / 2
	samples := make([]int16, numSamples)
	for i := 0; i < numSamples; i++ {
		samples[i] = int16(binary.LittleEndian.Uint16(pcm[i*2:]))
	}

	// Resample using linear interpolation
	ratio := float64(fromRate) / float64(toRate)
	newLen := int(math.Ceil(float64(len(samples)) / ratio))
	result := make([]int16, newLen)

	for i := 0; i < newLen; i++ {
		srcPos := float64(i) * ratio
		srcIdx := int(srcPos)
		frac := float32(srcPos - float64(srcIdx))

		if srcIdx+1 < len(samples) {
			result[i] = int16(float32(samples[srcIdx])*(1-frac) + float32(samples[srcIdx+1])*frac)
		} else if srcIdx < len(samples) {
			result[i] = samples[srcIdx]
		}
	}

	// Convert back to bytes
	output := make([]byte, len(result)*2)
	for i, sample := range result {
		binary.LittleEndian.PutUint16(output[i*2:], uint16(sample))
	}

	return output, nil
}
