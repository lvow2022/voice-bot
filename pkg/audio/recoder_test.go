package audio

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"voicebot/pkg/voicechain"
)

func TestRecorder(t *testing.T) {
	file, err := NewRecordFile("../../testdata/s16_16k_1c_zh.wav")
	assert.Nil(t, err)

	codecOpt := voicechain.DefaultCodecOption()
	recorder := NewRecorder("../../testdata/s16_16k_1c_zh.wav", file, codecOpt)

	recorder.WriteMono([]byte{0x02, 0x01, 0x04, 0x03, 0x06, 0x05, 0x08, 0x07})
	recorder.WriteStereo([]byte{0x02, 0x01, 0x06, 0x05, 0x04, 0x03, 0x08, 0x07})

	ctx, cancel := context.WithTimeout(context.Background(), 10)
	defer cancel()
	recorder.Start(ctx)

	recorder.Close()
}
