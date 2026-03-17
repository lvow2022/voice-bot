package pipeline

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"voicebot/pkg/asr"
	"voicebot/pkg/asr/types"
	"voicebot/pkg/audio"
	"voicebot/pkg/voicechain"
)

// AudioProcessOptions 音频处理 pipeline 配置选项
type AudioProcessOptions struct {
	ASR       types.ClientConfig      // ASR 客户端配置
	VADType   audio.VADType           // VAD 类型
	VADOption audio.VADDetectorOption // VAD 配置
}

// audioProcesser 音频处理管理器
type audioProcesser struct {
	opts AudioProcessOptions

	// 运行时组件
	vad        audio.VADDetector
	asrClient  *asr.AsrClient
	asrSession *asr.AsrSession

	// VAD 状态
	speaking atomic.Bool

	// 控制生命周期
	wg     sync.WaitGroup
	ctx    context.Context
	cancel context.CancelFunc
}

// NewAudioProcessPipeline 创建音频处理 pipeline
func NewAudioProcessPipeline(opts AudioProcessOptions) voicechain.HandleFunc {
	executor := voicechain.NewExecutor[[]byte](128)
	executor.Async = true

	mgr := &audioProcesser{opts: opts}

	executor.OnBegin = func(h voicechain.SessionHandler) error {
		return mgr.OnBegin(h)
	}

	executor.OnEnd = func(h voicechain.SessionHandler) error {
		return mgr.OnEnd(h)
	}

	executor.OnBuildRequest = func(h voicechain.SessionHandler, frame voicechain.Frame) (*voicechain.FrameRequest[[]byte], error) {
		return mgr.OnBuildRequest(h, frame)
	}

	executor.OnExecute = func(ctx context.Context, h voicechain.SessionHandler, req voicechain.FrameRequest[[]byte]) error {
		return mgr.OnExecute(ctx, h, req)
	}

	return executor.HandleSessionData
}

// OnBegin 会话开始时初始化
func (m *audioProcesser) OnBegin(h voicechain.SessionHandler) error {
	m.ctx, m.cancel = context.WithCancel(h.GetContext())

	// 1. 初始化 VAD 检测器
	vad, err := audio.GetVAD(m.opts.VADType, m.opts.VADOption)
	if err != nil {
		return err
	}
	m.vad = vad

	// 2. 创建 ASR 客户端
	client, err := asr.NewClient(m.opts.ASR)
	if err != nil {
		m.vad.Close()
		return err
	}
	m.asrClient = client

	// 3. 创建 ASR session
	session, err := m.asrClient.NewSession(m.ctx, m.opts.ASR.Session)
	if err != nil {
		m.vad.Close()
		_ = m.asrClient.Close()
		return err
	}
	m.asrSession = session

	// 4. 启动 ASR 结果接收循环
	m.wg.Add(1)
	go m.recvLoop(h)

	slog.Debug("audio process pipeline started")
	return nil
}

// OnBuildRequest 构建请求（仅提取音频数据）
func (m *audioProcesser) OnBuildRequest(_ voicechain.SessionHandler, frame voicechain.Frame) (*voicechain.FrameRequest[[]byte], error) {
	// 获取音频数据
	var payload []byte
	switch f := frame.(type) {
	case *voicechain.AudioFrame:
		payload = f.Payload
	default:
		return nil, nil
	}

	if len(payload) == 0 {
		return nil, nil
	}

	return &voicechain.FrameRequest[[]byte]{
		Req: payload,
	}, nil
}

// OnExecute 执行音频处理（VAD 检测 + ASR 发送）
func (m *audioProcesser) OnExecute(ctx context.Context, h voicechain.SessionHandler, req voicechain.FrameRequest[[]byte]) error {
	// VAD 检测
	if m.vad != nil {
		if err := m.vad.Process(req.Req, m.createVADHandler(h)); err != nil {
			return err
		}
	}

	// 检查是否有语音
	if !m.speaking.Load() {
		return nil // 静音，不发送到 ASR
	}

	// 发送到 ASR
	if m.asrSession == nil {
		return nil
	}

	return m.asrSession.Send(types.AudioFrame{
		Data:      req.Req,
		Timestamp: time.Now().UnixNano(),
	})
}

// OnEnd 会话结束时清理
func (m *audioProcesser) OnEnd(_ voicechain.SessionHandler) error {
	if m.cancel != nil {
		m.cancel()
	}
	m.wg.Wait()

	if m.vad != nil {
		m.vad.Close()
	}
	if m.asrClient != nil {
		_ = m.asrClient.Close()
	}

	slog.Debug("audio process pipeline stopped")
	return nil
}

// createVADHandler 创建 VAD 事件处理器
func (m *audioProcesser) createVADHandler(h voicechain.SessionHandler) audio.VADHandler {
	return func(duration time.Duration, speaking bool, silence bool) {
		if speaking && !m.speaking.Load() {
			// 语音开始
			m.speaking.Store(true)
			h.EmitEvent(m, voicechain.Event{Type: voicechain.StateVADSpeaking})
			slog.Debug("vad: speech start", "duration", duration)
		}

		if silence && m.speaking.Load() {
			// 语音结束
			m.speaking.Store(false)
			h.EmitEvent(m, voicechain.Event{Type: voicechain.StateVADSilence})
			slog.Debug("vad: speech end", "duration", duration)
		}
	}
}

// recvLoop 接收 ASR 结果
func (m *audioProcesser) recvLoop(h voicechain.SessionHandler) {
	defer m.wg.Done()

	for {
		select {
		case <-m.ctx.Done():
			return
		default:
			event, err := m.asrSession.Recv()
			if err != nil {
				if m.ctx.Err() != nil {
					return // 正常关闭
				}
				slog.Error("asr recv error", "error", err)
				return
			}

			if event == nil {
				continue
			}

			// 根据事件类型发送状态
			switch event.Type {
			case types.EventPartial:
				h.EmitEvent(m, voicechain.Event{Type: voicechain.StateASRPartial, Payload: event.Text})
			case types.EventFinal:
				h.EmitEvent(m, voicechain.Event{Type: voicechain.StateASRFinal, Payload: event.Text})
			}

			// 发送帧给下游 pipeline
			h.EmitFrame(m, &voicechain.TextFrame{
				Text:          event.Text,
				IsTranscribed: true,
				IsPartial:     !event.IsFinal,
				IsEnd:         event.IsFinal,
			})
		}
	}
}
