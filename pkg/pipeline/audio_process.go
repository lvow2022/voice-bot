package pipeline

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"voicebot/pkg/asr"
	"voicebot/pkg/asr/types"
	"voicebot/pkg/audio"
	"voicebot/pkg/voicechain"
)

// AudioProcessOptions 音频处理 pipeline 配置选项
type AudioProcessOptions struct {
	ASR   types.ClientConfig      // ASR 客户端配置
	Audio audio.AudioProcessOption // 音频处理配置
}

// asrRequest ASR 请求
type asrRequest struct {
	data []byte
}

// audioProcessManager 音频处理管理器
type audioProcessManager struct {
	opts AudioProcessOptions

	// 运行时组件
	audioProc  *audio.AudioProcess
	asrClient  *asr.AsrClient
	asrSession *asr.AsrSession

	// VAD 状态
	mu       sync.Mutex
	speaking bool

	// 控制生命周期
	wg     sync.WaitGroup
	ctx    context.Context
	cancel context.CancelFunc
}

// NewAudioProcessPipeline 创建音频处理 pipeline
func NewAudioProcessPipeline(opts AudioProcessOptions) voicechain.HandleFunc {
	executor := voicechain.NewExecutor[asrRequest](128)
	executor.Async = true

	mgr := &audioProcessManager{opts: opts}

	executor.OnBegin = func(h voicechain.SessionHandler) error {
		return mgr.OnBegin(h)
	}

	executor.OnEnd = func(h voicechain.SessionHandler) error {
		return mgr.OnEnd(h)
	}

	executor.OnBuildRequest = func(h voicechain.SessionHandler, frame voicechain.Frame) (*voicechain.FrameRequest[asrRequest], error) {
		return mgr.OnBuildRequest(h, frame)
	}

	executor.OnExecute = func(ctx context.Context, h voicechain.SessionHandler, req voicechain.FrameRequest[asrRequest]) error {
		return mgr.OnExecute(ctx, h, req)
	}

	return executor.HandleSessionData
}

// OnBegin 会话开始时初始化
func (m *audioProcessManager) OnBegin(h voicechain.SessionHandler) error {
	m.ctx, m.cancel = context.WithCancel(h.GetContext())

	// 1. 初始化音频处理器（VAD + 降噪）
	m.audioProc = audio.NewAudioProcess(m.opts.Audio)

	// 2. 创建 ASR 客户端
	client, err := asr.NewClient(m.opts.ASR)
	if err != nil {
		return err
	}
	m.asrClient = client

	// 3. 创建 ASR session
	session, err := m.asrClient.NewSession(m.ctx, m.opts.ASR.Session)
	if err != nil {
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

// OnBuildRequest 构建请求（VAD 过滤）
func (m *audioProcessManager) OnBuildRequest(h voicechain.SessionHandler, frame voicechain.Frame) (*voicechain.FrameRequest[asrRequest], error) {
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

	// 音频处理（降噪、VAD）
	processed, err := m.audioProc.Process(
		m.opts.Audio.VADOption.SampleRate,
		payload,
		m.createVADHandler(h),
		nil, // DTMF handler
	)
	if err != nil {
		return nil, err
	}

	// 检查是否有语音
	m.mu.Lock()
	hasSpeech := m.speaking
	m.mu.Unlock()

	if !hasSpeech {
		return nil, nil // 静音，跳过
	}

	return &voicechain.FrameRequest[asrRequest]{
		Req: asrRequest{data: processed},
	}, nil
}

// OnExecute 发送音频到 ASR
func (m *audioProcessManager) OnExecute(_ context.Context, _ voicechain.SessionHandler, req voicechain.FrameRequest[asrRequest]) error {
	if m.asrSession == nil {
		return nil
	}

	return m.asrSession.Send(types.AudioFrame{
		Data:      req.Req.data,
		Timestamp: time.Now().UnixNano(),
	})
}

// OnEnd 会话结束时清理
func (m *audioProcessManager) OnEnd(_ voicechain.SessionHandler) error {
	if m.cancel != nil {
		m.cancel()
	}
	m.wg.Wait()

	if m.audioProc != nil {
		m.audioProc.Close()
	}
	if m.asrClient != nil {
		_ = m.asrClient.Close()
	}

	slog.Debug("audio process pipeline stopped")
	return nil
}

// createVADHandler 创建 VAD 事件处理器
func (m *audioProcessManager) createVADHandler(h voicechain.SessionHandler) audio.VADHandler {
	return func(duration time.Duration, speaking bool, silence bool) {
		m.mu.Lock()
		defer m.mu.Unlock()

		if speaking && !m.speaking {
			// 语音开始
			m.speaking = true
			h.EmitState(m, voicechain.StateVADStart)
			slog.Debug("vad: speech start", "duration", duration)
		}

		if silence && m.speaking {
			// 语音结束
			m.speaking = false
			h.EmitState(m, voicechain.StateVADStop)
			slog.Debug("vad: speech end", "duration", duration)
		}
	}
}

// recvLoop 接收 ASR 结果
func (m *audioProcessManager) recvLoop(h voicechain.SessionHandler) {
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
				h.EmitState(m, voicechain.StateASRPartial, event.Text)
			case types.EventFinal:
				h.EmitState(m, voicechain.StateASRFinal, event.Text)
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
