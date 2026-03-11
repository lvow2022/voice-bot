package tts

import (
	"context"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"

	"voicebot/pkg/stream"
	ws "voicebot/pkg/websocket"
)

// ============ Constants ============

const (
	MinimaxWebSocketURL         = "wss://api.minimaxi.com/ws/v1/t2a_v2"
	MinimaxSpeech25HDPreview    = "speech-2.5-hd-preview"
	MinimaxSpeech25TurboPreview = "speech-2.5-turbo-preview"
)

const (
	MinimaxEventTaskStart     = "task_start"
	MinimaxEventTaskStarted   = "task_started"
	MinimaxEventTaskContinue  = "task_continue"
	MinimaxEventTaskContinued = "task_continued"
	MinimaxEventTaskFinish    = "task_finish"
	MinimaxEventTaskFinished  = "task_finished"
	MinimaxEventTaskFailed    = "task_failed"
)

// ============ Types ============

type MinimaxMessage struct {
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

type MinimaxTaskStartRequest struct {
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

type MinimaxTaskContinueRequest struct {
	Event string `json:"event"`
	Text  string `json:"text"`
}

// MinimaxOption 配置选项
type MinimaxOption struct {
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
}

// ============ Engine ============

type MinimaxEngine struct {
	opt    MinimaxOption
	conn   ws.Client
	ctx    context.Context
	cancel context.CancelFunc

	mu          sync.Mutex
	currentSess *MinimaxSession
	closeOnce   sync.Once
}

func NewMinimaxEngine(cfg EngineConfig) (Engine, error) {
	opt := parseMinimaxOption(cfg)

	if opt.APIKey == "" {
		return nil, errors.New("minimax: apiKey required")
	}

	ctx, cancel := context.WithCancel(context.Background())

	h := http.Header{}
	h.Set("Authorization", fmt.Sprintf("Bearer %s", opt.APIKey))

	wsURL := firstNonEmpty(cfg.URL, MinimaxWebSocketURL)

	conn, err := ws.NewClient(ctx, ws.Config{
		URL:       wsURL,
		Headers:   h,
		TLSConfig: &tls.Config{InsecureSkipVerify: true},
	})
	if err != nil {
		cancel()
		return nil, fmt.Errorf("connect websocket: %w", err)
	}

	e := &MinimaxEngine{
		opt:    opt,
		conn:   conn,
		ctx:    ctx,
		cancel: cancel,
	}

	if err := e.startTask(); err != nil {
		_ = conn.Close()
		cancel()
		return nil, fmt.Errorf("start task: %w", err)
	}

	// 启动接收循环
	go e.recvLoop()

	return e, nil
}

func parseMinimaxOption(cfg EngineConfig) MinimaxOption {
	opts := cfg.Options

	opt := MinimaxOption{
		APIKey:        cfg.APIKey,
		VoiceID:       cfg.VoiceID,
		Model:         cfg.Model,
		SpeedRatio:    cfg.Speed,
		SampleRate:    cfg.SampleRate,
		Emotion:       getString(opts, "emotion"),
		LanguageBoost: getString(opts, "languageBoost"),
		Format:        getString(opts, "format"),
		Volume:        getFloat64(opts, "volume"),
		Pitch:         getFloat64(opts, "pitch"),
		Channels:      getInt(opts, "channels"),
	}

	opt.Model = firstNonEmpty(opt.Model, MinimaxSpeech25TurboPreview)
	opt.Format = firstNonEmpty(opt.Format, "pcm")
	if opt.SampleRate == 0 {
		opt.SampleRate = 16000
	}
	if opt.Channels == 0 {
		opt.Channels = 1
	}
	if opt.SpeedRatio == 0 {
		opt.SpeedRatio = 1.0
	}
	if opt.Volume == 0 {
		opt.Volume = 1.0
	}

	return opt
}

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

func (e *MinimaxEngine) startTask() error {
	req := MinimaxTaskStartRequest{
		Event:         MinimaxEventTaskStart,
		Model:         e.opt.Model,
		LanguageBoost: e.opt.LanguageBoost,
	}

	req.VoiceSetting.VoiceID = e.opt.VoiceID
	req.VoiceSetting.Speed = e.opt.SpeedRatio
	req.VoiceSetting.Volume = e.opt.Volume
	req.VoiceSetting.Pitch = e.opt.Pitch
	req.VoiceSetting.Emotion = e.opt.Emotion

	req.AudioSetting.SampleRate = e.opt.SampleRate
	req.AudioSetting.Format = e.opt.Format
	req.AudioSetting.Channel = e.opt.Channels

	return e.conn.SendTextJSON(req)
}

// NewSession 创建新 session（同一时间只能有一个）
func (e *MinimaxEngine) NewSession(ctx context.Context, output stream.Stream) (Session, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.currentSess != nil {
		return nil, errors.New("minimax: session already active")
	}

	sess := &MinimaxSession{
		engine: e,
		output: output,
		done:   make(chan struct{}),
	}
	e.currentSess = sess

	return sess, nil
}

// Close 关闭引擎
func (e *MinimaxEngine) Close() error {
	e.closeOnce.Do(func() {
		e.cancel()
		if e.conn != nil {
			_ = e.conn.Close()
		}
	})
	return nil
}

// recvLoop 接收循环
func (e *MinimaxEngine) recvLoop() {
	for {
		select {
		case <-e.ctx.Done():
			return
		default:
			rawMsg, err := e.conn.Recv()
			if err != nil {
				return
			}
			e.handleMessage(rawMsg)
		}
	}
}

// handleMessage 处理消息
func (e *MinimaxEngine) handleMessage(rawMsg []byte) {
	var msg MinimaxMessage
	if err := json.Unmarshal(rawMsg, &msg); err != nil {
		return
	}

	e.mu.Lock()
	sess := e.currentSess
	e.mu.Unlock()

	if sess == nil {
		return
	}

	switch msg.Event {
	case MinimaxEventTaskContinued:
		if msg.Data.Audio != "" {
			audio, err := hex.DecodeString(msg.Data.Audio)
			if err != nil {
				return
			}
			if len(audio) > 0 {
				if pusher, ok := sess.output.(interface{ Push([]byte, bool) error }); ok {
					pusher.Push(audio, msg.IsFinal)
				}
			}
		}

		if msg.IsFinal {
			e.finishSession(sess)
		}

	case MinimaxEventTaskFailed:
		e.finishSession(sess)
	}
}

// finishSession 结束 session
func (e *MinimaxEngine) finishSession(sess *MinimaxSession) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.currentSess == sess {
		// 发送 EOF
		if pusher, ok := sess.output.(interface{ Push([]byte, bool) error }); ok {
			pusher.Push(nil, true)
		}
		close(sess.done)
		e.currentSess = nil
	}
}

// ============ Session ============

type MinimaxSession struct {
	engine *MinimaxEngine
	output stream.Stream
	done   chan struct{}
}

func (s *MinimaxSession) SendText(text string, _ map[string]any) error {
	return s.engine.conn.SendTextJSON(MinimaxTaskContinueRequest{
		Event: MinimaxEventTaskContinue,
		Text:  text,
	})
}

func (s *MinimaxSession) Done() <-chan struct{} {
	return s.done
}

func (s *MinimaxSession) Close() error {
	s.engine.finishSession(s)
	return nil
}
