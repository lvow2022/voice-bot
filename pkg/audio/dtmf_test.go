package audio

import (
	"os"
	"testing"
)

func TestNewDTMFDetector(t *testing.T) {
	//t.Skip("tmp skip dtmf")
	dt := NewDTMFDetector(0.09, 16000)
	file, err := os.ReadFile("../../testdata/dtmf/concat.wav")
	if err != nil {
		t.Fatal(err)
	}
	size := GetSampleSize(16000, 16, 1) * 20
	for i := 0; i < len(file); i += size {
		end := i + size
		if end > len(file) {
			end = len(file)
		}
		dt.Process(file[i:end], func(sender, digit string) {
			t.Logf("Detected digit: %s", digit)
		})
	}

}
