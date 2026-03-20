package minimax

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
)

// ============ Constants ============

const (
	EventTaskStart     = "task_start"
	EventTaskStarted   = "task_started"
	EventTaskContinue  = "task_continue"
	EventTaskContinued = "task_continued"
	EventTaskFinish    = "task_finish"
	EventTaskFinished  = "task_finished"
	EventTaskFailed    = "task_failed"
)

// ============ Request & Event ============

// TTSRequest TTS 请求
type TTSRequest struct {
	Text   string
	IsLast bool // half-close 标记
}

// TtsEventType TTS 事件类型
type TtsEventType int

const (
	TtsEventAudio TtsEventType = iota
	TtsEventFinal
	TtsEventError
)

func (t TtsEventType) String() string {
	switch t {
	case TtsEventAudio:
		return "audio"
	case TtsEventFinal:
		return "final"
	case TtsEventError:
		return "error"
	default:
		return "unknown"
	}
}

// TtsEvent TTS 事件
type TtsEvent struct {
	Type  TtsEventType
	Audio []byte
	Err   error
}

// IsFinal 实现 FinalEvent 接口
func (e TtsEvent) IsFinal() bool {
	return e.Type == TtsEventFinal
}

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

// TaskFinishRequest 任务结束请求
type TaskFinishRequest struct {
	Event string `json:"event"`
}

// ============ Codec ============

// Codec Minimax TTS 编解码器
type Codec struct {
	cfg Config
}

// NewCodec 创建编解码器
func NewCodec(cfg Config) *Codec {
	return &Codec{cfg: cfg}
}

// Encode 编码请求（类型断言）
func (c *Codec) Encode(req any) ([]byte, error) {
	ttsReq, ok := req.(TTSRequest)
	if !ok {
		return nil, fmt.Errorf("expected TTSRequest, got %T", req)
	}

	// Half-close: 发送 finish 事件
	if ttsReq.IsLast {
		return json.Marshal(TaskFinishRequest{
			Event: EventTaskFinish,
		})
	}

	// 普通文本请求
	return json.Marshal(TaskContinueRequest{
		Event: EventTaskContinue,
		Text:  ttsReq.Text,
	})
}

// Decode 解码响应
func (c *Codec) Decode(data []byte) (any, error) {
	var msg Message
	if err := json.Unmarshal(data, &msg); err != nil {
		return nil, fmt.Errorf("unmarshal: %w", err)
	}

	switch msg.Event {
	case EventTaskContinued:
		if msg.Data.Audio != "" {
			audio, err := hex.DecodeString(msg.Data.Audio)
			if err != nil {
				return TtsEvent{
					Type: TtsEventError,
					Err:  fmt.Errorf("decode hex audio: %w", err),
				}, nil
			}

			if len(audio) == 0 {
				return nil, nil // 忽略空音频
			}

			return TtsEvent{
				Type:  TtsEventAudio,
				Audio: audio,
			}, nil
		}

	case EventTaskFinished:
		return TtsEvent{
			Type: TtsEventFinal,
		}, nil

	case EventTaskFailed:
		return TtsEvent{
			Type: TtsEventError,
			Err:  fmt.Errorf("task failed: %s", msg.BaseResp.StatusMsg),
		}, nil
	}

	// 忽略其他事件
	return nil, nil
}

// MessageType 返回消息类型
func (c *Codec) MessageType() int {
	return 1 // websocket.TextMessage
}

// EncodeStart 编码启动请求
func (c *Codec) EncodeStart() ([]byte, error) {
	req := TaskStartRequest{
		Event:         EventTaskStart,
		Model:         c.cfg.Model,
		LanguageBoost: c.cfg.LanguageBoost,
	}
	req.VoiceSetting.VoiceID = c.cfg.VoiceID
	req.VoiceSetting.Speed = c.cfg.SpeedRatio
	req.VoiceSetting.Volume = c.cfg.Volume
	req.VoiceSetting.Pitch = c.cfg.Pitch
	req.VoiceSetting.Emotion = c.cfg.Emotion
	req.AudioSetting.SampleRate = c.cfg.SampleRate
	req.AudioSetting.Format = c.cfg.Format
	req.AudioSetting.Channel = c.cfg.Channels

	return json.Marshal(req)
}
