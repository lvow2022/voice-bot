package audio

import (
	"fmt"
	"io"
	"log"
	"os"
	"sync"
	"testing"
	"time"
)

func TestSileroVADOrt_Offline_PCM(t *testing.T) {
	file, err := os.Open("../../testdata/16k_zh.pcm") // 16bit PCM, mono
	if err != nil {
		log.Fatalf("Failed to open file: %v", err)
	}
	defer file.Close()

	vad := NewSileroORTVADDetector(VADDetectorOption{
		SampleRate: 16000,
		Channels:   1,
		BitDepth:   16,

		PositiveSpeechThreshold: 0.5,
		NegativeSpeechThreshold: 0.4,
		MinSpeechFrames:         5,
		RedemptionFrames:        10,
	})

	frameSize := GetSampleSize(16000, 16, 1) * 20 // 20ms
	buffer := make([]byte, frameSize)

	for {
		n, err := file.Read(buffer)
		if err == io.EOF {
			break
		}
		if n < frameSize {
			break
		}

		handler := func(duration time.Duration, speaking bool, silence bool) {
			if speaking {
				fmt.Printf("[sileroVAD] %s Speech Start \n", duration)
			}

			if silence {
				fmt.Printf("[sileroVAD] %s Speech End \n", duration)
			}
		}

		_ = vad.Process(buffer[:n], handler)
	}
}

// VADEvent represents a speech start or end event
type VADEvent struct {
	Duration time.Duration
	Speaking bool
	Silence  bool
}

// TestSileroVADOrt_Concurrent tests that 10 VAD instances processing the same PCM file
// concurrently produce identical results, validating the session pool implementation.
func TestSileroVADOrt_Concurrent(t *testing.T) {
	// Read the entire PCM file into memory
	pcmData, err := os.ReadFile("../../testdata/16k_zh.pcm")
	if err != nil {
		t.Fatalf("Failed to read PCM file: %v", err)
	}

	const numVADs = 1000
	frameSize := GetSampleSize(16000, 16, 1) * 20 // 20ms

	// Results from each VAD instance
	results := make([][]VADEvent, numVADs)
	var wg sync.WaitGroup
	var mu sync.Mutex

	t.Logf("Starting concurrent test with %d VAD instances, PCM size: %d bytes", numVADs, len(pcmData))
	start := time.Now()
	// Run all	 VADs concurrently
	for i := 0; i < numVADs; i++ {
		wg.Add(1)
		go func(vadIndex int) {
			defer wg.Done()

			// Create a new VAD instance
			vad := NewSileroORTVADDetector(VADDetectorOption{
				SampleRate: 16000,
				Channels:   1,
				BitDepth:   16,

				PositiveSpeechThreshold: 0.5,
				NegativeSpeechThreshold: 0.4,
				MinSpeechFrames:         5,
				RedemptionFrames:        10,
			})
			defer vad.Close()

			var events []VADEvent

			// Process the PCM data frame by frame
			for offset := 0; offset+frameSize <= len(pcmData); offset += frameSize {
				frame := pcmData[offset : offset+frameSize]

				handler := func(duration time.Duration, speaking bool, silence bool) {
					events = append(events, VADEvent{
						Duration: duration,
						Speaking: speaking,
						Silence:  silence,
					})
				}

				if err := vad.Process(frame, handler); err != nil {
					t.Errorf("VAD %d: Process error: %v", vadIndex, err)
					return
				}
			}

			// Store results
			mu.Lock()
			results[vadIndex] = events
			mu.Unlock()

			t.Logf("VAD %d completed: %d events", vadIndex, len(events))
		}(i)
	}

	// Wait for all VADs to complete
	wg.Wait()

	// Verify all results are identical
	t.Log("Verifying results... spendtime", time.Since(start))

	// Use the first VAD's result as reference
	reference := results[0]
	t.Logf("Reference (VAD 0) has %d events:", len(reference))
	for i, event := range reference {
		eventType := "unknown"
		if event.Speaking {
			eventType = "Speech Start"
		} else if event.Silence {
			eventType = "Speech End"
		}
		t.Logf("  Event %d: %s at %s", i, eventType, event.Duration)
	}

	// Compare all other VADs with the reference
	allMatch := true
	for i := 1; i < numVADs; i++ {
		if len(results[i]) != len(reference) {
			t.Errorf("VAD %d has %d events, expected %d (same as VAD 0)", i, len(results[i]), len(reference))
			allMatch = false
			continue
		}

		for j, event := range results[i] {
			if event.Duration != reference[j].Duration ||
				event.Speaking != reference[j].Speaking ||
				event.Silence != reference[j].Silence {
				t.Errorf("VAD %d event %d mismatch: got {%s, speaking=%v, silence=%v}, expected {%s, speaking=%v, silence=%v}",
					i, j,
					event.Duration, event.Speaking, event.Silence,
					reference[j].Duration, reference[j].Speaking, reference[j].Silence)
				allMatch = false
			}
		}
	}

	if allMatch {
		t.Logf("SUCCESS: All %d VAD instances produced identical results (%d events each)", numVADs, len(reference))
	} else {
		t.Fatalf("FAILED: VAD instances produced different results")
	}
}
