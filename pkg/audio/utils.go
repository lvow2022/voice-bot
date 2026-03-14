package audio

import (
	"fmt"
	"time"
)

// GetSampleSize returns the size of an audio sample in bytes per millisecond.
func GetSampleSize(sampleRate, bitDepth, channels int) int {
	return sampleRate * bitDepth / 1000 / 8
}

// ParseFrameDuration parses a duration string and validates it's within acceptable range.
func ParseFrameDuration(d string) time.Duration {
	duration, err := time.ParseDuration(d)
	if err != nil || duration == 0 {
		return 0
	}
	if duration < 10*time.Millisecond || duration > 300*time.Millisecond {
		return 20 * time.Millisecond
	}
	return duration
}

// BytesToFloat32 converts 16-bit PCM bytes to float32 samples (-1.0 to 1.0).
// Reuses buffer if it has sufficient capacity.
func BytesToFloat32(pcm []byte, buffer []float32) ([]float32, error) {
	if len(pcm)%2 != 0 {
		return nil, fmt.Errorf("pcm data length must be even for 16-bit audio")
	}

	sampleCount := len(pcm) / 2
	if cap(buffer) < sampleCount {
		buffer = make([]float32, sampleCount)
	} else {
		buffer = buffer[:sampleCount]
	}

	for i := 0; i < sampleCount; i++ {
		sample := int16(pcm[i*2]) | (int16(pcm[i*2+1]) << 8)
		buffer[i] = float32(sample) / 32768.0
	}

	return buffer, nil
}

// Float32ToBytes converts float32 samples to 16-bit PCM bytes.
// Reuses buffer if it has sufficient capacity.
// Samples are clamped to [-1.0, 1.0].
func Float32ToBytes(samples []float32, buffer []byte) ([]byte, error) {
	byteCount := len(samples) * 2
	if cap(buffer) < byteCount {
		buffer = make([]byte, byteCount)
	} else {
		buffer = buffer[:byteCount]
	}

	for i, sample := range samples {
		// Clamp to [-1.0, 1.0]
		if sample > 1.0 {
			sample = 1.0
		} else if sample < -1.0 {
			sample = -1.0
		}

		intSample := int16(sample * 32767.0)
		buffer[i*2] = byte(intSample)
		buffer[i*2+1] = byte(intSample >> 8)
	}

	return buffer, nil
}

// BytesToInt16 converts 16-bit PCM bytes to int16 samples.
// Reuses buffer if it has sufficient capacity.
func BytesToInt16(pcm []byte, buffer []int16) ([]int16, error) {
	if len(pcm)%2 != 0 {
		return nil, fmt.Errorf("pcm data length must be even for 16-bit audio")
	}

	sampleCount := len(pcm) / 2
	if cap(buffer) < sampleCount {
		buffer = make([]int16, sampleCount)
	} else {
		buffer = buffer[:sampleCount]
	}

	for i := 0; i < sampleCount; i++ {
		buffer[i] = int16(pcm[i*2]) | int16(pcm[i*2+1])<<8
	}

	return buffer, nil
}

// Int16ToBytes converts int16 samples to 16-bit PCM bytes.
// Reuses buffer if it has sufficient capacity.
func Int16ToBytes(samples []int16, buffer []byte) []byte {
	byteCount := len(samples) * 2
	if cap(buffer) < byteCount {
		buffer = make([]byte, byteCount)
	} else {
		buffer = buffer[:byteCount]
	}

	for i, sample := range samples {
		buffer[i*2] = byte(sample)
		buffer[i*2+1] = byte(sample >> 8)
	}

	return buffer
}
