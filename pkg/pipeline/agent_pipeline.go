package pipeline

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"time"

	"voicebot/pkg/agent"
	"voicebot/pkg/speech"
	"voicebot/pkg/stream"
	"voicebot/pkg/tts"
	"voicebot/pkg/voicechain"
)

// AgentPipelineOptions Agent Pipeline 配置选项
type AgentPipelineOptions struct {
	AgentLoop    *agent.AgentLoop   // Per-session AgentLoop
	SessionKey   string             // Session key for this voice connection
	TTSSession   *tts.TtsSession    // TTS 会话
	StreamPlayer *stream.StreamPlayer // 音频播放器
	SpeechConfig speech.Config      // Scheduler 配置
}

// agentProcessor Agent 处理器
type agentProcessor struct {
	opts      AgentPipelineOptions
	scheduler *speech.Scheduler

	// 生命周期控制
	wg     sync.WaitGroup
	ctx    context.Context
	cancel context.CancelFunc
}

// NewAgentPipeline 创建 Agent Pipeline
func NewAgentPipeline(opts AgentPipelineOptions) voicechain.HandleFunc {
	// 使用 string 作为请求类型（LLM 文本输入）
	executor := voicechain.NewExecutor[string](32)

	p := &agentProcessor{opts: opts}

	executor.OnBegin = func(h voicechain.SessionHandler) error {
		return p.OnBegin(h)
	}

	executor.OnEnd = func(h voicechain.SessionHandler) error {
		return p.OnEnd(h)
	}

	executor.OnBuildRequest = func(h voicechain.SessionHandler, frame voicechain.Frame) (*voicechain.FrameRequest[string], error) {
		return p.OnBuildRequest(h, frame)
	}

	executor.OnExecute = func(ctx context.Context, h voicechain.SessionHandler, req voicechain.FrameRequest[string]) error {
		return p.OnExecute(ctx, h, req)
	}

	return executor.HandleSessionData
}

// OnBegin 会话开始时初始化
func (p *agentProcessor) OnBegin(h voicechain.SessionHandler) error {
	p.ctx, p.cancel = context.WithCancel(h.GetContext())

	// 创建 Scheduler
	if p.opts.SpeechConfig.MaxWaiting == 0 {
		p.opts.SpeechConfig = speech.DefaultConfig
	}

	p.scheduler = speech.NewScheduler(p.opts.TTSSession, p.opts.StreamPlayer, p.opts.SpeechConfig)
	p.scheduler.Run()

	slog.Debug("agent pipeline started")
	return nil
}

// OnEnd 会话结束时清理
func (p *agentProcessor) OnEnd(_ voicechain.SessionHandler) error {
	if p.cancel != nil {
		p.cancel()
	}

	if p.scheduler != nil {
		_ = p.scheduler.Close()
	}

	p.wg.Wait()

	slog.Debug("agent pipeline stopped")
	return nil
}

// OnBuildRequest 构建请求（从 TextFrame 提取文本）
func (p *agentProcessor) OnBuildRequest(_ voicechain.SessionHandler, frame voicechain.Frame) (*voicechain.FrameRequest[string], error) {
	textFrame, ok := frame.(*voicechain.TextFrame)
	if !ok {
		return nil, nil
	}

	// 只处理 ASR Final 结果
	if !textFrame.IsTranscribed || !textFrame.IsEnd {
		return nil, nil
	}

	text := strings.TrimSpace(textFrame.Text)
	if text == "" {
		return nil, nil
	}

	return &voicechain.FrameRequest[string]{
		Req: text,
	}, nil
}

// OnExecute 执行 Agent 处理（使用 AgentLoop 流式处理）
func (p *agentProcessor) OnExecute(ctx context.Context, h voicechain.SessionHandler, req voicechain.FrameRequest[string]) error {
	userText := req.Req

	// 1. 发送 Agent 生成中事件
	h.EmitEvent(p, voicechain.Event{Type: voicechain.StateAgentGenerating, Payload: userText})
	slog.Debug("agent: generating response", "input_len", len(userText))

	// 2. 调用 AgentLoop 流式处理
	agentLoop := p.opts.AgentLoop
	if agentLoop == nil {
		slog.Error("agent: AgentLoop is nil")
		return nil
	}

	// 3. 创建流式回调，将 chunks 直接喂入 TTS Scheduler
	var fullContent strings.Builder
	callbacks := agent.StreamCallbacks{
		OnChunk: func(chunk string) {
			if chunk == "" {
				return
			}
			fullContent.WriteString(chunk)
			// 直接喂入 TTS Scheduler
			p.scheduler.Feed(chunk)
		},
	}

	// 4. 调用 ProcessDirectStream
	response, err := agentLoop.ProcessDirectStream(
		ctx,
		userText,
		p.opts.SessionKey,
		"voice",
		"voice_session",
		callbacks,
	)
	if err != nil {
		slog.Error("agent: ProcessDirectStream failed", "error", err)
		return err
	}

	// 5. 发送 Agent 开始说话事件
	h.EmitEvent(p, voicechain.Event{Type: voicechain.StateAgentSpeaking})
	slog.Debug("agent: response received", "content_len", len(response))

	// 6. Flush TTS 并等待播放完成
	p.scheduler.Flush()

	for p.scheduler.IsPlaying() {
		select {
		case <-ctx.Done():
			p.scheduler.Reset()
			return ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}

	// 7. 发送播放完成事件
	h.EmitEvent(p, voicechain.Event{Type: voicechain.StatePlaybackDone, Payload: response})
	slog.Debug("agent: playback done")

	return nil
}
