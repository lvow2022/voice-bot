package stream

// ============ 音频元数据 ============

// AudioFormat 音频格式
type AudioFormat string

const (
	AudioFormatPCM  AudioFormat = "pcm"  // 16-bit signed little-endian
	AudioFormatOPUS AudioFormat = "opus" // opus 编码
	AudioFormatMP3  AudioFormat = "mp3"  // mp3 编码
)

// Streamer 流接口
type Streamer interface {
	Pull(sample []byte) (n int, err error)
	Push(data []byte, eof bool) error
	Meta() StreamMeta
}

// StreamMeta 流元数据（类似文件元信息）
type StreamMeta struct {
	Name       string // 流名称
	Format     AudioFormat
	SampleRate int
	Source     string // 来源（如：TTS引擎、文件路径等）
}
