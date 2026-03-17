package conversation

// CommandGenerator 根据 Turn/Phase 状态 + Backchannel 判定生成 AgentCommand
type CommandGenerator struct {
	// 可配置的策略
	pauseOnVADStart bool // VAD_START 时是否暂停播放
}

// CommandGeneratorOption 配置选项
type CommandGeneratorOption func(*CommandGenerator)

// WithPauseOnVADStart 设置 VAD_START 时是否暂停播放
func WithPauseOnVADStart(pause bool) CommandGeneratorOption {
	return func(g *CommandGenerator) {
		g.pauseOnVADStart = pause
	}
}

// NewCommandGenerator 创建新的 CommandGenerator
func NewCommandGenerator(opts ...CommandGeneratorOption) *CommandGenerator {
	g := &CommandGenerator{
		pauseOnVADStart: true,
	}
	for _, opt := range opts {
		opt(g)
	}
	return g
}

// Generate 根据 state + event 生成 AgentCommand
func (g *CommandGenerator) Generate(state State, event Semantic) AgentCommand {
	// 根据状态和事件决定命令
	switch state.Turn {
	case TurnUser:
		return g.generateForUserTurn(state, event)
	case TurnAgent:
		return g.generateForAgentTurn(state, event)
	}
	return CmdNone
}

// GenerateForVAD 处理 VAD 事件
func (g *CommandGenerator) GenerateForVAD(state State, eventType string) AgentCommand {
	if eventType == VADStart && state.Turn == TurnAgent && state.Phase == PhaseSpeaking {
		if g.pauseOnVADStart {
			return CmdPauseAgentPlayback
		}
	}
	return CmdNone
}

// generateForUserTurn 用户轮次的命令生成
func (g *CommandGenerator) generateForUserTurn(_ State, event Semantic) AgentCommand {
	switch event {
	case SemanticNewTurn:
		// 新轮次，调用 Agent 处理
		return CmdInvokeAgentGenerate
	case SemanticBackchannel, SemanticInterrupt, SemanticIgnore:
		// 用户轮次的附和词/打断词/忽略，不做处理
		return CmdNone
	}
	return CmdNone
}

// generateForAgentTurn Agent 轮次的命令生成
func (g *CommandGenerator) generateForAgentTurn(state State, event Semantic) AgentCommand {
	switch state.Phase {
	case PhaseGenerating:
		return g.generateForGenerating(event)
	case PhaseSpeaking:
		return g.generateForSpeaking(event)
	case PhaseIdle:
		// Agent 空闲时不处理
		return CmdNone
	}
	return CmdNone
}

// generateForGenerating AgentGenerating 阶段的命令生成
func (g *CommandGenerator) generateForGenerating(event Semantic) AgentCommand {
	switch event {
	case SemanticNewTurn, SemanticInterrupt:
		// 新轮次或打断，取消当前 Agent 生成
		return CmdCancelAgentGenerate
	case SemanticBackchannel, SemanticIgnore:
		// 附和词或忽略，不做处理
		return CmdNone
	}
	return CmdNone
}

// generateForSpeaking AgentSpeaking 阶段的命令生成
func (g *CommandGenerator) generateForSpeaking(event Semantic) AgentCommand {
	switch event {
	case SemanticInterrupt:
		// 打断，停止播放
		return CmdStopAgentPlayback
	case SemanticNewTurn:
		// 新轮次，也停止播放（可能需要取消 Agent）
		return CmdStopAgentPlayback
	case SemanticBackchannel, SemanticIgnore:
		// 附和词或忽略，恢复播放（如果暂停了）
		// 注意：这里返回 CmdNone，由上层决定是否恢复
		return CmdNone
	}
	return CmdNone
}
