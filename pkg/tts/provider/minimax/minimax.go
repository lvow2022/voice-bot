package minimax

import (
	"context"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	"voicebot/pkg/tts/types"
)

// ============ Errors ============

var (
	ErrNotConnected   = errors.New("minimax: not connected")
	ErrAlreadyClosed  = errors.New("minimax: already closed")
	ErrAPIKeyRequired = errors.New("minimax: apiKey required")
)

// ============ Provider ============

// MinimaxProvider 实现 types.Provider 接口
type MinimaxProvider struct {
	cfg  Config
	conn *websocket.Conn

	ctx    context.Context
	cancel context.CancelFunc

	// 读写通道
	recvCh   chan *types.TtsEvent
	sendCh   chan []byte
	recvErr  error
	recvOnce sync.Once

	// 状态
	closeOnce sync.Once
	connected atomic.Bool
	wg        sync.WaitGroup

	// metrics
	bytesRecv int64
}

// NewProvider 创建 Minimax Provider
func NewProvider(cfg types.ProviderConfig) (*MinimaxProvider, error) {
	config := ParseConfig(cfg)

	if config.APIKey == "" {
		return nil, ErrAPIKeyRequired
	}

	ctx, cancel := context.WithCancel(context.Background())

	p := &MinimaxProvider{
		cfg:     config,
		ctx:     ctx,
		cancel:  cancel,
		recvCh:  make(chan *types.TtsEvent, 128),
		sendCh:  make(chan []byte, 128),
	}

	// 建立 WebSocket 连接
	if err := p.connect(); err != nil {
		cancel()
		return nil, err
	}

	return p, nil
}

// connect 建立 WebSocket 连接并发送 task_start
func (p *MinimaxProvider) connect() error {
	h := http.Header{}
	h.Set("Authorization", fmt.Sprintf("Bearer %s", p.cfg.APIKey))

	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
		TLSClientConfig:  &tls.Config{InsecureSkipVerify: true},
	}

	conn, _, err := dialer.DialContext(p.ctx, p.cfg.URL, h)
	if err != nil {
		return fmt.Errorf("connect websocket: %w", err)
	}
	p.conn = conn

	// 发送 task_start
	if err := p.sendTaskStart(); err != nil {
		_ = conn.Close()
		return fmt.Errorf("start task: %w", err)
	}

	p.connected.Store(true)

	// 启动读写循环
	p.wg.Add(2)
	go p.readLoop()
	go p.writeLoop()

	return nil
}

// sendTaskStart 发送任务开始请求
func (p *MinimaxProvider) sendTaskStart() error {
	req := TaskStartRequest{
		Event:         EventTaskStart,
		Model:         p.cfg.Model,
		LanguageBoost: p.cfg.LanguageBoost,
	}
	req.VoiceSetting.VoiceID = p.cfg.VoiceID
	req.VoiceSetting.Speed = p.cfg.SpeedRatio
	req.VoiceSetting.Volume = p.cfg.Volume
	req.VoiceSetting.Pitch = p.cfg.Pitch
	req.VoiceSetting.Emotion = p.cfg.Emotion
	req.AudioSetting.SampleRate = p.cfg.SampleRate
	req.AudioSetting.Format = p.cfg.Format
	req.AudioSetting.Channel = p.cfg.Channels

	data, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshal task_start: %w", err)
	}

	return p.conn.WriteMessage(websocket.TextMessage, data)
}

// Connect 实现 Provider.Connect（已由 NewProvider 完成，此方法用于重连）
func (p *MinimaxProvider) Connect(ctx context.Context, opts types.SessionOptions) error {
	if p.connected.Load() {
		return nil
	}
	return p.connect()
}

// SendText 实现 Provider.SendText
func (p *MinimaxProvider) SendText(text string, _ map[string]any) error {
	if !p.connected.Load() {
		return ErrNotConnected
	}

	req := TaskContinueRequest{
		Event: EventTaskContinue,
		Text:  text,
	}

	data, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshal task_continue: %w", err)
	}

	select {
	case p.sendCh <- data:
		return nil
	case <-p.ctx.Done():
		return p.ctx.Err()
	}
}

// RecvEvent 实现 Provider.RecvEvent
func (p *MinimaxProvider) RecvEvent() (*types.TtsEvent, error) {
	select {
	case <-p.ctx.Done():
		if p.recvErr != nil {
			return nil, p.recvErr
		}
		return nil, p.ctx.Err()
	case ev := <-p.recvCh:
		return ev, nil
	}
}

// Close 实现 Provider.Close
func (p *MinimaxProvider) Close() error {
	p.closeOnce.Do(func() {
		p.connected.Store(false)
		p.cancel()

		if p.conn != nil {
			_ = p.conn.Close()
		}

		p.wg.Wait()
		close(p.recvCh)
		close(p.sendCh)
	})
	return nil
}

// readLoop 读取循环
func (p *MinimaxProvider) readLoop() {
	defer p.wg.Done()
	defer p.Close()

	for {
		select {
		case <-p.ctx.Done():
			p.setRecvErr(p.ctx.Err())
			return
		default:
			_, rawMsg, err := p.conn.ReadMessage()
			if err != nil {
				p.setRecvErr(err)
				return
			}

			ev := p.handleMessage(rawMsg)
			if ev == nil {
				continue
			}

			select {
			case <-p.ctx.Done():
				p.setRecvErr(p.ctx.Err())
				return
			case p.recvCh <- ev:
				if ev.Type == types.EventCompleted || ev.Type == types.EventError {
					return
				}
			}
		}
	}
}

// writeLoop 写入循环
func (p *MinimaxProvider) writeLoop() {
	defer p.wg.Done()
	defer p.Close()

	for {
		select {
		case <-p.ctx.Done():
			return
		case data := <-p.sendCh:
			if err := p.conn.WriteMessage(websocket.TextMessage, data); err != nil {
				p.setRecvErr(err)
				return
			}
		}
	}
}

// handleMessage 处理消息
func (p *MinimaxProvider) handleMessage(rawMsg []byte) *types.TtsEvent {
	var msg Message
	if err := json.Unmarshal(rawMsg, &msg); err != nil {
		return nil
	}

	switch msg.Event {
	case EventTaskContinued:
		if msg.Data.Audio != "" {
			audio, err := hex.DecodeString(msg.Data.Audio)
			if err != nil || len(audio) == 0 {
				return nil
			}
			atomic.AddInt64(&p.bytesRecv, int64(len(audio)))

			ev := &types.TtsEvent{
				Type:    types.EventAudioChunk,
				Data:    audio,
				IsFinal: msg.IsFinal,
			}

			if msg.IsFinal {
				// 发送完成事件后紧接着发送完成信号
				go func() {
					select {
					case <-p.ctx.Done():
					case p.recvCh <- &types.TtsEvent{Type: types.EventCompleted}:
					}
				}()
			}

			return ev
		}

	case EventTaskFailed:
		return &types.TtsEvent{
			Type: types.EventError,
			Err:  fmt.Errorf("task failed: %s", msg.BaseResp.StatusMsg),
		}

	case EventTaskFinished:
		return &types.TtsEvent{Type: types.EventCompleted}
	}

	return nil
}

// setRecvErr 设置接收错误
func (p *MinimaxProvider) setRecvErr(err error) {
	if err == nil {
		return
	}
	p.recvOnce.Do(func() {
		p.recvErr = err
	})
}

// Metrics 返回指标
func (p *MinimaxProvider) Metrics() map[string]int64 {
	return map[string]int64{
		"bytes_recv": atomic.LoadInt64(&p.bytesRecv),
	}
}
