package conversation

import (
	"context"
	"time"

	"voicebot/pkg/voicechain"
)

// ConversationPipeline 将 ConversationManager 包装为 voicechain pipeline
// 基于 Executor 实现，处理 ASR 事件帧，输出 CommandFrame
type ConversationPipeline struct {
	manager     *ConversationManager
	executor    voicechain.Executor[conversationRequest]
	eventMapper func(voicechain.Frame) (AudioEvent, bool)
}

// conversationRequest 对话处理请求
type conversationRequest struct {
	event    AudioEvent
	isSystem bool
	sysType  voicechain.SystemEventType
}

// PipelineOption 配置选项
type PipelineOption func(*ConversationPipeline)

// WithEventMapper 设置自定义事件映射器
func WithEventMapper(mapper func(voicechain.Frame) (AudioEvent, bool)) PipelineOption {
	return func(p *ConversationPipeline) {
		p.eventMapper = mapper
	}
}

// WithConversationManager 设置自定义 ConversationManager
func WithConversationManager(mgr *ConversationManager) PipelineOption {
	return func(p *ConversationPipeline) {
		p.manager = mgr
	}
}

// PipelineBackchannelWords 设置附和词列表
func PipelineBackchannelWords(words []string) PipelineOption {
	return func(p *ConversationPipeline) {
		for _, w := range words {
			p.manager.GetBackchannelChecker().AddBackchannelWord(w)
		}
	}
}

// PipelineInterruptWords 设置打断词列表
func PipelineInterruptWords(words []string) PipelineOption {
	return func(p *ConversationPipeline) {
		for _, w := range words {
			p.manager.GetBackchannelChecker().AddInterruptWord(w)
		}
	}
}

// NewConversationPipeline 创建对话管道
func NewConversationPipeline(opts ...PipelineOption) *ConversationPipeline {
	p := &ConversationPipeline{
		manager:     NewConversationManager(),
		eventMapper: DefaultEventMapper,
	}

	p.executor = voicechain.NewExecutor[conversationRequest](16)
	p.executor.Async = true
	p.executor.OnBuildRequest = p.buildRequest
	p.executor.OnExecute = p.execute

	for _, opt := range opts {
		opt(p)
	}

	return p
}

// HandleFunc 返回 voicechain HandleFunc
func (p *ConversationPipeline) HandleFunc() voicechain.HandleFunc {
	return p.executor.HandleSessionData
}

// GetManager 获取内部 ConversationManager
func (p *ConversationPipeline) GetManager() *ConversationManager {
	return p.manager
}

// buildRequest 构建帧请求（同步，在主线程中执行）
func (p *ConversationPipeline) buildRequest(_ voicechain.SessionHandler, frame voicechain.Frame) (*voicechain.FrameRequest[conversationRequest], error) {
	// 处理系统事件帧
	if sysFrame, ok := frame.(*voicechain.SystemEventFrame); ok {
		return &voicechain.FrameRequest[conversationRequest]{
			Req: conversationRequest{
				isSystem: true,
				sysType:  sysFrame.Type,
			},
		}, nil
	}

	// 处理音频事件帧
	event, ok := p.eventMapper(frame)
	if !ok {
		return nil, nil // 不处理的帧返回 nil
	}

	return &voicechain.FrameRequest[conversationRequest]{
		Req: conversationRequest{
			event:    event,
			isSystem: false,
		},
	}, nil
}

// execute 执行处理逻辑（异步，在 goroutine 中执行）
func (p *ConversationPipeline) execute(_ context.Context, h voicechain.SessionHandler, req voicechain.FrameRequest[conversationRequest]) error {
	if req.Req.isSystem {
		// 处理系统事件
		cmd := p.manager.HandleSystemEvent(SystemEvent{
			Type: convertSystemEventType(req.Req.sysType),
			Data: nil,
		})
		if cmd != CmdNone {
			h.EmitFrame(p, &voicechain.CommandFrame{
				Command:   cmd,
				Timestamp: time.Now(),
			})
		}
		return nil
	}

	// 处理音频事件
	cmd := p.manager.HandleAudioEvent(req.Req.event)
	if cmd != CmdNone {
		h.EmitFrame(p, &voicechain.CommandFrame{
			Command:   cmd,
			Text:      req.Req.event.Text,
			Timestamp: time.Now(),
		})
	}

	return nil
}

// convertSystemEventType 将 voicechain.SystemEventType 转换为 conversation.SystemEventType
func convertSystemEventType(t voicechain.SystemEventType) SystemEventType {
	switch t {
	case voicechain.SystemEventAgentStart:
		return SystemEventAgentStart
	case voicechain.SystemEventAgentSpeak:
		return SystemEventAgentSpeak
	case voicechain.SystemEventPlaybackFinished:
		return SystemEventPlaybackFinished
	default:
		return SystemEventAgentStart
	}
}

// DefaultEventMapper 默认的事件映射器
// 将 TextFrame 映射为 AudioEvent
func DefaultEventMapper(frame voicechain.Frame) (AudioEvent, bool) {
	switch f := frame.(type) {
	case *voicechain.TextFrame:
		if f.IsTranscribed {
			if f.IsPartial {
				return AudioEvent{Type: ASRPartial, Text: f.Text}, true
			}
			if f.IsEnd {
				return AudioEvent{Type: ASRFinal, Text: f.Text}, true
			}
		}
	}
	return AudioEvent{}, false
}
