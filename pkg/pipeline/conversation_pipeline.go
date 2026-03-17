package pipeline

import (
	"voicebot/pkg/conversation"
	"voicebot/pkg/voicechain"
)

// ConversationPipeline 将 ConversationManager 包装为 voicechain pipeline
// 同步处理帧，直接返回命令
type ConversationPipeline struct {
	manager *conversation.ConversationManager
}

// ConversationOption 配置选项
type ConversationOption func(*ConversationPipeline)

// WithConversationManager 设置自定义 ConversationManager
func WithConversationManager(mgr *conversation.ConversationManager) ConversationOption {
	return func(p *ConversationPipeline) {
		p.manager = mgr
	}
}

// WithConversationBackchannelWords 设置附和词列表
func WithConversationBackchannelWords(words []string) ConversationOption {
	return func(p *ConversationPipeline) {
		for _, w := range words {
			p.manager.AddBackchannelWord(w)
		}
	}
}

// WithConversationInterruptWords 设置打断词列表
func WithConversationInterruptWords(words []string) ConversationOption {
	return func(p *ConversationPipeline) {
		for _, w := range words {
			p.manager.AddInterruptWord(w)
		}
	}
}

// NewConversationPipeline 创建对话管道
func NewConversationPipeline(opts ...ConversationOption) *ConversationPipeline {
	p := &ConversationPipeline{
		manager: conversation.NewConversationManager(),
	}

	for _, opt := range opts {
		opt(p)
	}

	return p
}

// HandleFunc 返回 voicechain HandleFunc
func (p *ConversationPipeline) HandleFunc() voicechain.HandleFunc {
	return func(h voicechain.SessionHandler, data voicechain.SessionData) {
		var event conversation.Event
		var text string
		var ok bool

		// 根据 SessionData 类型映射事件
		switch data.Type {
		case voicechain.SessionDataState:
			// 状态事件
			event, ok = mapStateEvent(data.Event.Type)

		case voicechain.SessionDataFrame:
			// 帧事件
			event, text, ok = mapFrame(data.Frame)
		}

		if !ok {
			return
		}

		// 处理事件
		cmd := p.manager.HandleEvent(event, text)

		// 广播命令
		if !cmd.IsNone() {
			h.EmitEvent(p, voicechain.Event{
				Type:    string(cmd),
				Payload: text,
			})
		}
	}
}

// GetManager 获取内部 ConversationManager
func (p *ConversationPipeline) GetManager() *conversation.ConversationManager {
	return p.manager
}

// mapStateEvent 将 voicechain 状态事件映射为 conversation.Event
func mapStateEvent(eventType string) (conversation.Event, bool) {
	switch eventType {
	case voicechain.StateVADSpeaking:
		return conversation.EventVADStart, true
	case voicechain.StateVADSilence:
		return conversation.EventVADStop, true
	case voicechain.StateAgentGenerating:
		return conversation.EventAgentGenerating, true
	case voicechain.StateAgentSpeaking:
		return conversation.EventAgentSpeaking, true
	case voicechain.StatePlaybackDone:
		return conversation.EventPlaybackDone, true
	default:
		return 0, false
	}
}

// mapFrame 将 voicechain.Frame 映射为 conversation.Event
func mapFrame(frame voicechain.Frame) (conversation.Event, string, bool) {
	switch f := frame.(type) {
	case *voicechain.TextFrame:
		// ASR 最终结果
		if f.IsTranscribed && f.IsEnd {
			return conversation.EventASRFinal, f.Text, true
		}
	}

	return 0, "", false
}
