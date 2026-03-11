package tts

import (
	"context"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"

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

// MinimaxOption provider 配置选项
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

	mu         sync.Mutex
	activeSess *MinimaxSession // 当前活跃的 session
	closeOnce  sync.Once
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

	p := &MinimaxEngine{
		opt:    opt,
		conn:   conn,
		ctx:    ctx,
		cancel: cancel,
	}

	if err := p.startTask(); err != nil {
		_ = conn.Close()
		cancel()
		return nil, fmt.Errorf("start task: %w", err)
	}

	return p, nil
}

func parseMinimaxOption(cfg EngineConfig) MinimaxOption {
	opts := cfg.Options

	opt := MinimaxOption{
		// 优先使用顶层配置字段
		APIKey:     cfg.APIKey,
		VoiceID:    cfg.VoiceID,
		Model:      cfg.Model,
		SpeedRatio: cfg.Speed,
		SampleRate: cfg.SampleRate,
		// Minimax 特有配置从 Options 获取
		Emotion:       getString(opts, "emotion"),
		LanguageBoost: getString(opts, "languageBoost"),
		Format:        getString(opts, "format"),
		Volume:        getFloat64(opts, "volume"),
		Pitch:         getFloat64(opts, "pitch"),
		Channels:      getInt(opts, "channels"),
	}

	// 默认值
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

func (p *MinimaxEngine) startTask() error {
	req := MinimaxTaskStartRequest{
		Event:         MinimaxEventTaskStart,
		Model:         p.opt.Model,
		LanguageBoost: p.opt.LanguageBoost,
	}

	req.VoiceSetting.VoiceID = p.opt.VoiceID
	req.VoiceSetting.Speed = p.opt.SpeedRatio
	req.VoiceSetting.Volume = p.opt.Volume
	req.VoiceSetting.Pitch = p.opt.Pitch
	req.VoiceSetting.Emotion = p.opt.Emotion

	req.AudioSetting.SampleRate = p.opt.SampleRate
	req.AudioSetting.Format = p.opt.Format
	req.AudioSetting.Channel = p.opt.Channels

	return p.conn.SendTextJSON(req)
}

func (p *MinimaxEngine) NewSession(ctx context.Context) (Session, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.activeSess != nil {
		return nil, errors.New("minimax: session already active")
	}

	sessCtx, sessCancel := context.WithCancel(ctx)

	sess := &MinimaxSession{
		provider: p,
		stream:   newAudioStream(),
		ctx:      sessCtx,
		cancel:   sessCancel,
	}
	p.activeSess = sess

	go sess.recvLoop()

	return sess, nil
}

func (p *MinimaxEngine) Close() error {
	p.closeOnce.Do(func() {
		p.cancel()
		if p.conn != nil {
			_ = p.conn.Close()
		}
	})
	return nil
}

// ============ Session ============

type MinimaxSession struct {
	provider *MinimaxEngine
	stream   *audioStream
	ctx      context.Context
	cancel   context.CancelFunc
}

func (s *MinimaxSession) SendText(text string, _ map[string]any) error {
	return s.provider.conn.SendTextJSON(MinimaxTaskContinueRequest{
		Event: MinimaxEventTaskContinue,
		Text:  text,
	})
}

func (s *MinimaxSession) RecvAudio() AudioStream {
	return s.stream
}

func (s *MinimaxSession) setErr(err error) {
	s.stream.err = err
}

func (s *MinimaxSession) recvLoop() {
	defer func() {
		s.stream.Close()
		s.provider.clearSession(s)
	}()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-s.provider.ctx.Done():
			s.setErr(s.provider.ctx.Err())
			return
		default:
			rawMsg, err := s.provider.conn.Recv()
			if err != nil {
				s.setErr(err)
				return
			}

			if err = s.dispatch(rawMsg); err != nil {
				if err != io.EOF {
					s.setErr(err)
				}

				return
			}
		}
	}
}

func (s *MinimaxSession) dispatch(rawMsg []byte) error {
	var msg MinimaxMessage
	if err := json.Unmarshal(rawMsg, &msg); err != nil {
		return fmt.Errorf("unmarshal: %w", err)
	}

	switch msg.Event {
	case MinimaxEventTaskStarted:
		// 任务已启动

	case MinimaxEventTaskContinued:
		if msg.Data.Audio != "" {
			audio, err := hex.DecodeString(msg.Data.Audio)
			if err != nil {
				return fmt.Errorf("decode hex: %w", err)
			}
			select {
			case s.stream.ch <- AudioFrame{Data: audio, Final: msg.IsFinal}:
				// 发送成功
			case <-s.ctx.Done():
				return s.ctx.Err()
			}
		}

		if msg.IsFinal {
			return io.EOF
		}

	case MinimaxEventTaskFinished:
		return nil // 正常结束

	case MinimaxEventTaskFailed:
		return fmt.Errorf("task failed: %s", msg.BaseResp.StatusMsg)
	}

	return nil
}

func (p *MinimaxEngine) clearSession(sess *MinimaxSession) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.activeSess == sess {
		p.activeSess = nil
	}
}
