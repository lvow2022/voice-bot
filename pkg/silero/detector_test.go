package silero

import (
	"encoding/binary"
	"fmt"
	"os"
	"testing"
)

// loadPCM reads a raw PCM file (16-bit signed, little-endian) and converts it to float32 samples.
func loadPCM(path string) ([]float32, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	numSamples := len(data) / 2
	samples := make([]float32, numSamples)
	for i := 0; i < numSamples; i++ {
		sample := int16(binary.LittleEndian.Uint16(data[i*2 : i*2+2]))
		samples[i] = float32(sample) / 32768.0
	}
	return samples, nil
}

func TestDetector_Detect(t *testing.T) {
	// Path to the ONNX model
	modelPath := "../../models/silero_vad.onnx"
	sharedLibPath := "/opt/homebrew/lib/libonnxruntime.dylib"
	pcmPath := "../../testdata/16k_zh.pcm"

	// Create detector
	cfg := DetectorConfig{
		ModelPath:            modelPath,
		SampleRate:           16000,
		Threshold:            0.5,
		MinSilenceDurationMs: 300,
		SpeechPadMs:          30,
		LogLevel:             LogLevelWarn,
		SharedLibraryPath:    sharedLibPath,
	}

	detector, err := NewDetector(cfg)
	if err != nil {
		t.Fatalf("Failed to create detector: %v", err)
	}
	defer detector.Destroy()

	// Load PCM audio
	samples, err := loadPCM(pcmPath)
	if err != nil {
		t.Fatalf("Failed to load PCM: %v", err)
	}

	fmt.Printf("Loaded %d samples (%.2f seconds)\n", len(samples), float64(len(samples))/16000.0)

	// Run VAD detection
	segments, err := detector.Detect(samples)
	if err != nil {
		t.Fatalf("Failed to detect: %v", err)
	}

	// Print results
	fmt.Printf("\n=== VAD Detection Results ===\n")
	fmt.Printf("Total segments found: %d\n\n", len(segments))

	for i, seg := range segments {
		fmt.Printf("Segment %d:\n", i+1)
		fmt.Printf("  Start: %.3f s\n", seg.SpeechStartAt)
		if seg.SpeechEndAt > 0 {
			fmt.Printf("  End:   %.3f s\n", seg.SpeechEndAt)
			fmt.Printf("  Duration: %.3f s\n", seg.SpeechEndAt-seg.SpeechStartAt)
		} else {
			fmt.Printf("  End:   (ongoing)\n")
		}
		fmt.Println()
	}
}
