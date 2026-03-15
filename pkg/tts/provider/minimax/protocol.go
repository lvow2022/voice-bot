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

// ============ Event Types ============

const (
	EventTaskStart     = "task_start"
	EventTaskStarted   = "task_started"
	EventTaskContinue  = "task_continue"
	EventTaskContinued = "task_continued"
	EventTaskFinish    = "task_finish"
	EventTaskFinished  = "task_finished"
	EventTaskFailed    = "task_failed"
)

// ============ Protocol Messages ============

// Message WebSocket 消息
type Message struct {
	Event    string `json:"event,omitempty"`
	TraceID  string `json:"trace_id,omitempty"`
	BaseResp struct {
		StatusCode int    `json:"status_code"`
		StatusMsg  string `json:"status_msg"`
	} `json:"base_resp"`
	Data struct {
		Audio string `json:"audio,omitempty"`
	} `json:"data,omitempty"`
	IsFinal bool `json:"is_final,omitempty"`
}

// TaskStartRequest 任务开始请求
type TaskStartRequest struct {
	Event         string `json:"event"`
	Model         string `json:"model"`
	LanguageBoost string `json:"language_boost,omitempty"`
	VoiceSetting  struct {
		VoiceID string  `json:"voice_id,omitempty"`
		Speed   float64 `json:"speed"`
		Volume  float64 `json:"vol"`
		Pitch   float64 `json:"pitch"`
		Emotion string  `json:"emotion"`
	} `json:"voice_setting"`
	AudioSetting struct {
		SampleRate int    `json:"sample_rate"`
		Format     string `json:"format"`
		Channel    int    `json:"channel"`
	} `json:"audio_setting"`
}

// TaskContinueRequest 任务继续请求
type TaskContinueRequest struct {
	Event string `json:"event"`
	Text  string `json:"text"`
}

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

func getInt(m map[string]any, key string) int {
	if m == nil {
		return 0
	}
	if v, ok := m[key].(int); ok {
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
