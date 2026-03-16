package pipeline

import (
	"context"
	"time"

	"voicebot/pkg/conversation"
	"voicebot/pkg/voicechain"
)

// ConversationPipeline 将 ConversationManager 包装为 voicechain pipeline
// 基于 Executor 实现，处理 ASR 事件帧，输出 CommandFrame
type ConversationPipeline struct {
	manager     *conversation.ConversationManager
	executor    voicechain.Executor[conversationRequest]
	eventMapper func(voicechain.Frame) (conversation.AudioEvent, bool)
}

// conversationRequest 对话处理请求
type conversationRequest struct {
	event    conversation.AudioEvent
	isSystem bool
	sysType  voicechain.SystemEventType
}

// ConversationOption 配置选项
type ConversationOption func(*ConversationPipeline)

// WithEventMapper 设置自定义事件映射器
func WithEventMapper(mapper func(voicechain.Frame) (conversation.AudioEvent, bool)) ConversationOption {
	return func(p *ConversationPipeline) {
		p.eventMapper = mapper
	}
}

// WithConversationManager 设置自定义 ConversationManager
func WithConversationManager(mgr *conversation.ConversationManager) ConversationOption {
	return func(p *ConversationPipeline) {
		p.manager = mgr
	}
}

// WithBackchannelWords 设置附和词列表
func WithBackchannelWords(words []string) ConversationOption {
	return func(p *ConversationPipeline) {
		for _, w := range words {
			p.manager.GetBackchannelChecker().AddBackchannelWord(w)
		}
	}
}

// WithInterruptWords 设置打断词列表
func WithInterruptWords(words []string) ConversationOption {
	return func(p *ConversationPipeline) {
		for _, w := range words {
			p.manager.GetBackchannelChecker().AddInterruptWord(w)
		}
	}
}

// NewConversationPipeline 创建对话管道
func NewConversationPipeline(opts ...ConversationOption) *ConversationPipeline {
	p := &ConversationPipeline{
		manager:     conversation.NewConversationManager(),
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
func (p *ConversationPipeline) GetManager() *conversation.ConversationManager {
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
		cmd := p.manager.HandleSystemEvent(conversation.SystemEvent{
			Type: convertSystemEventType(req.Req.sysType),
			Data: nil,
		})
		if cmd != conversation.CmdNone {
			h.EmitFrame(p, &voicechain.CommandFrame{
				Command:   cmd,
				Timestamp: time.Now(),
			})
		}
		return nil
	}

	// 处理音频事件
	cmd := p.manager.HandleAudioEvent(req.Req.event)
	if cmd != conversation.CmdNone {
		h.EmitFrame(p, &voicechain.CommandFrame{
			Command:   cmd,
			Text:      req.Req.event.Text,
			Timestamp: time.Now(),
		})
	}

	return nil
}

// convertSystemEventType 将 voicechain.SystemEventType 转换为 conversation.SystemEventType
func convertSystemEventType(t voicechain.SystemEventType) conversation.SystemEventType {
	switch t {
	case voicechain.SystemEventAgentStart:
		return conversation.SystemEventAgentStart
	case voicechain.SystemEventAgentSpeak:
		return conversation.SystemEventAgentSpeak
	case voicechain.SystemEventPlaybackFinished:
		return conversation.SystemEventPlaybackFinished
	default:
		return conversation.SystemEventAgentStart
	}
}

// DefaultEventMapper 默认的事件映射器
// 将 TextFrame 映射为 AudioEvent
func DefaultEventMapper(frame voicechain.Frame) (conversation.AudioEvent, bool) {
	switch f := frame.(type) {
	case *voicechain.TextFrame:
		if f.IsTranscribed {
			if f.IsPartial {
				return conversation.AudioEvent{Type: conversation.ASRPartial, Text: f.Text}, true
			}
			if f.IsEnd {
				return conversation.AudioEvent{Type: conversation.ASRFinal, Text: f.Text}, true
			}
		}
	}
	return conversation.AudioEvent{}, false
}
