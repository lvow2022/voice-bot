package pipeline

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"time"

	"voicebot/pkg/agent"
	"voicebot/pkg/providers"
	"voicebot/pkg/speech"
	"voicebot/pkg/stream"
	"voicebot/pkg/tts"
	"voicebot/pkg/voicechain"
)

// AgentPipelineOptions Agent Pipeline 配置选项
type AgentPipelineOptions struct {
	AgentInstance *agent.AgentInstance  // 复用现有 agent
	TTSSession    *tts.TtsSession       // TTS 会话
	StreamPlayer  *stream.StreamPlayer  // 音频播放器
	SpeechConfig  speech.Config         // Scheduler 配置
}

// agentProcessor Agent 处理器
type agentProcessor struct {
	opts AgentPipelineOptions

	// 运行时组件
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

// OnExecute 执行 Agent 处理
func (p *agentProcessor) OnExecute(ctx context.Context, h voicechain.SessionHandler, req voicechain.FrameRequest[string]) error {
	userText := req.Req

	// 1. 发送 Agent 生成中事件
	h.EmitEvent(p, voicechain.Event{Type: voicechain.StateAgentGenerating, Payload: userText})
	slog.Debug("agent: generating response", "input_len", len(userText))

	// 2. 调用 LLM
	agentInstance := p.opts.AgentInstance
	if agentInstance == nil {
		slog.Error("agent: AgentInstance is nil")
		return nil
	}

	// 构建消息
	messages := []providers.Message{
		{Role: "user", Content: userText},
	}

	// 调用 LLM Provider
	response, err := agentInstance.Provider.Chat(ctx, messages, nil, agentInstance.Model, map[string]any{
		"max_tokens":  agentInstance.MaxTokens,
		"temperature": agentInstance.Temperature,
	})
	if err != nil {
		slog.Error("agent: LLM call failed", "error", err)
		return err
	}

	// 3. 发送 Agent 开始说话事件
	h.EmitEvent(p, voicechain.Event{Type: voicechain.StateAgentSpeaking})
	slog.Debug("agent: response received", "content_len", len(response.Content))

	// 4. 将响应喂入 Scheduler 进行 TTS
	if response.Content != "" {
		// 逐字符喂入 Scheduler（模拟流式）
		for _, char := range response.Content {
			p.scheduler.Feed(string(char))
		}
		p.scheduler.Flush()

		// 等待播放完成
		for p.scheduler.IsPlaying() {
			select {
			case <-ctx.Done():
				p.scheduler.Reset()
				return ctx.Err()
			case <-time.After(100 * time.Millisecond):
			}
		}
	}

	// 5. 发送播放完成事件
	h.EmitEvent(p, voicechain.Event{Type: voicechain.StatePlaybackDone, Payload: response.Content})
	slog.Debug("agent: playback done")

	return nil
}
