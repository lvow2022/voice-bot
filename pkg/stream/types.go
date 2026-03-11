package stream

// ============ 音频元数据 ============

// AudioFormat 音频格式
type AudioFormat string

const (
	AudioFormatPCM  AudioFormat = "pcm"  // 16-bit signed little-endian
	AudioFormatOPUS AudioFormat = "opus" // opus 编码
	AudioFormatMP3  AudioFormat = "mp3"  // mp3 编码
)

// Stream 音频管道接口
type Stream interface {
	Pull(sample []byte) (n int, err error)
	Push(data []byte, eof bool) error
}

// StreamMeta 流元数据（类似文件元信息）
type StreamMeta struct {
	Name       string     // 流名称
	Format     AudioFormat
	SampleRate int
	Source     string // 来源（如：TTS引擎、文件路径等）
}

// ============ 过滤器链 ============

// Filter 数据过滤器（线性处理）
type Filter interface {
	Filter(data []byte) ([]byte, error)
}

// FilterFunc 函数式过滤器
type FilterFunc func(data []byte) ([]byte, error)

func (f FilterFunc) Filter(data []byte) ([]byte, error) {
	return f(data)
}

// PushFilter 输入端过滤器（Push 时执行）
type PushFilter interface {
	Filter
}

// PullFilter 输出端过滤器（Pull 时执行）
type PullFilter interface {
	Filter
}
