package audio

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestParseFrameDuration(t *testing.T) {
	sampleRate := 8000
	bitDepth := 16
	channels := 1
	assert.Equal(t, 16, GetSampleSize(sampleRate, bitDepth, channels))

	timeDuration := ParseFrameDuration("20ms")
	assert.Equal(t, time.Duration(20000000), timeDuration)

	timeDuration1 := ParseFrameDuration("5ns")
	assert.Equal(t, time.Duration(20000000), timeDuration1)
}
