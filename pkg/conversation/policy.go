package conversation

// Policy 策略决策器接口
// 根据当前状态 + 事件 + 语义，决定输出什么命令
type Policy interface {
	Decide(state State, event Event, semantic Semantic) Command
}

// DefaultPolicy 默认策略实现
type DefaultPolicy struct {
	pauseOnVADStart bool // VADStart 时是否暂停播放
}

// PolicyOption 配置选项
type PolicyOption func(*DefaultPolicy)

// WithPauseOnVADStart 设置 VADStart 时是否暂停播放
func WithPauseOnVADStart(pause bool) PolicyOption {
	return func(p *DefaultPolicy) {
		p.pauseOnVADStart = pause
	}
}

// NewDefaultPolicy 创建默认策略
func NewDefaultPolicy(opts ...PolicyOption) *DefaultPolicy {
	p := &DefaultPolicy{
		pauseOnVADStart: true,
	}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

// Decide 根据状态 + 事件 + 语义决定命令
func (p *DefaultPolicy) Decide(state State, event Event, semantic Semantic) Command {
	switch state {
	case StateUserTurn:
		return p.decideForUserTurn(event, semantic)
	case StateAgentGenerating:
		return p.decideForAgentGenerating(event, semantic)
	case StateAgentSpeaking:
		return p.decideForAgentSpeaking(event, semantic)
	}
	return CmdNone
}

// decideForUserTurn 用户轮次的策略
func (p *DefaultPolicy) decideForUserTurn(event Event, semantic Semantic) Command {
	// ASRFinal + NewTurn → 启动 Agent
	if event == EventASRFinal && semantic == SemanticNewTurn {
		return CmdInvokeAgent
	}
	return CmdNone
}

// decideForAgentGenerating Agent 生成中的策略
func (p *DefaultPolicy) decideForAgentGenerating(event Event, semantic Semantic) Command {
	// ASRFinal + Interrupt/NewTurn → 取消生成
	if event == EventASRFinal {
		if semantic == SemanticInterrupt || semantic == SemanticNewTurn {
			return CmdCancelAgent
		}
	}
	return CmdNone
}

// decideForAgentSpeaking Agent 播放中的策略
func (p *DefaultPolicy) decideForAgentSpeaking(event Event, semantic Semantic) Command {
	// VADStart → 暂停播放（可配置）
	if event == EventVADStart && p.pauseOnVADStart {
		return CmdPausePlayback
	}

	// ASRFinal + Interrupt/NewTurn → 停止播放
	if event == EventASRFinal {
		if semantic == SemanticInterrupt || semantic == SemanticNewTurn {
			return CmdStopPlayback
		}
	}

	return CmdNone
}
