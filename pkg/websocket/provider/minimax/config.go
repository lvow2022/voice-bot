package minimax

import "voicebot/pkg/tts/types"

// ============ Constants ============

const (
	DefaultWebSocketURL      = "wss://api.minimaxi.com/ws/v1/t2a_v2"
	Speech25HDPreview        = "speech-2.5-hd-preview"
	Speech25TurboPreview     = "speech-2.5-turbo-preview"
	DefaultSampleRate        = 16000
	DefaultChannels          = 1
	DefaultSpeed             = 1.0
	DefaultVolume            = 1.0
)

// ============ Config ============

// Config Provider 配置
type Config struct {
	Model         string
	APIKey        string
	VoiceID       string
	SpeedRatio    float64
	Volume        float64
	Pitch         float64
	Emotion       string
	LanguageBoost string
	SampleRate    int
	Format        string
	Channels      int
	URL           string
}

// ParseConfig 从 ProviderConfig 解析配置
func ParseConfig(cfg types.ProviderConfig) Config {
	opts := cfg.Options

	c := Config{
		APIKey:     cfg.APIKey,
		VoiceID:    cfg.VoiceID,
		Model:      cfg.Model,
		SpeedRatio: cfg.Speed,
		Emotion:    getString(opts, "emotion"),
		Format:     getString(opts, "format"),
		Volume:     getFloat64(opts, "volume"),
		Pitch:      getFloat64(opts, "pitch"),
		URL:        cfg.URL,
	}

	// 设置默认值
	c.Model = firstNonEmpty(c.Model, Speech25TurboPreview)
	c.Format = firstNonEmpty(c.Format, "pcm")
	c.URL = firstNonEmpty(c.URL, DefaultWebSocketURL)
	if c.SampleRate == 0 {
		c.SampleRate = DefaultSampleRate
	}
	if c.Channels == 0 {
		c.Channels = DefaultChannels
	}
	if c.SpeedRatio == 0 {
		c.SpeedRatio = DefaultSpeed
	}
	if c.Volume == 0 {
		c.Volume = DefaultVolume
	}

	return c
}

// ============ Helper Functions ============

func getString(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

func getFloat64(m map[string]any, key string) float64 {
	if m == nil {
		return 0
	}
	if v, ok := m[key].(float64); ok {
		return v
	}
	return 0
}

func firstNonEmpty[T comparable](vals ...T) T {
	var zero T
	for _, v := range vals {
		if v != zero {
			return v
		}
	}
	return zero
}
