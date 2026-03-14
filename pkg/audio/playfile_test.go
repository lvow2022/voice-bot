package audio

import (
	"context"
	"io"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"voicebot/pkg/voicechain"
)

func TestAudioPlayer(t *testing.T) {
	session := voicechain.NewSession()
	p, err := PlayFile("../../testdata/s16_16k_1c_zh.wav", 200*time.Millisecond, true)
	assert.Nil(t, err)
	session.Pipeline(p)

	beginTime := time.Now()
	session.On(voicechain.StartPlay, func(event voicechain.StateEvent) {
		duration := time.Since(beginTime)
		assert.LessOrEqual(t, duration.Seconds(), 3.0, "Playback time exceeds file duration")
		err := session.Close()
		assert.Nil(t, err)
	})

	err = session.Serve()
	assert.Nil(t, err)
}
func TestLoadFromStream(t *testing.T) {
	audioPlayer, err := LoadFromFile("../../testdata/s16_16k_1c_zh.wav", 200*time.Millisecond, true)
	assert.Nil(t, err)

	ctx := context.Background()
	err = audioPlayer.LoadFromStream(ctx, "http://localhost:8080/testdata/s16_16k_1c_zh.wav")
	assert.NotNil(t, err)

	ap, err := LoadFromFile("../../testdata/s16_16k_1c_zh.mp3", 200*time.Millisecond, true)
	assert.Nil(t, err)

	err = audioPlayer.LoadFromStream(ctx, "http://localhost:8080/testdata/s16_16k_1c_zh.mp3")
	assert.NotNil(t, err)

	err = ap.Close()
	assert.Nil(t, err)

	ap.checkResample(8000)

}

func TestGetAudioStream(t *testing.T) {
	audioPlayer, err := LoadFromFile("../../testdata/s16_16k_1c_zh.wav", 200*time.Millisecond, true)
	assert.Nil(t, err)

	file, err := os.Open("../../testdata/s16_16k_1c_zh.wav")
	assert.Nil(t, err)
	data, err := io.ReadAll(file)
	assert.Nil(t, err)
	_, _, err = audioPlayer.getAudioStream(data, ".wav")
	assert.Nil(t, err)

	file, err = os.Open("../../testdata/s16_16k_1c_zh.mp3")
	assert.Nil(t, err)
	data, err = io.ReadAll(file)
	assert.Nil(t, err)
	_, _, err = audioPlayer.getAudioStream(data, ".mp3")
	assert.Nil(t, err)

	audioPlayer.getAudioStream(data, ".ogg")
}
