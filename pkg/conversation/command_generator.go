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
func (g *CommandGenerator) Generate(state ConversationState, event ConversationEvent) AgentCommand {
	// 根据状态和事件决定命令
	switch state.Turn {
	case UserTurn:
		return g.generateForUserTurn(state, event)
	case AgentTurn:
		return g.generateForAgentTurn(state, event)
	}
	return CmdNone
}

// GenerateForVAD 处理 VAD 事件
func (g *CommandGenerator) GenerateForVAD(state ConversationState, eventType AudioEventType) AgentCommand {
	if eventType == VADStart && state.Turn == AgentTurn && state.Phase == AgentSpeaking {
		if g.pauseOnVADStart {
			return CmdPausePlayback
		}
	}
	return CmdNone
}

// generateForUserTurn 用户轮次的命令生成
func (g *CommandGenerator) generateForUserTurn(_ ConversationState, event ConversationEvent) AgentCommand {
	switch event {
	case EventNewTurn:
		// 新轮次，启动 Agent
		return CmdStartAgent
	case EventBackchannel, EventInterrupt, EventIgnore:
		// 用户轮次的附和词/打断词/忽略，不做处理
		return CmdNone
	}
	return CmdNone
}

// generateForAgentTurn Agent 轮次的命令生成
func (g *CommandGenerator) generateForAgentTurn(state ConversationState, event ConversationEvent) AgentCommand {
	switch state.Phase {
	case AgentProcessing:
		return g.generateForProcessing(event)
	case AgentSpeaking:
		return g.generateForSpeaking(event)
	case AgentIdle:
		// Agent 空闲时不处理
		return CmdNone
	}
	return CmdNone
}

// generateForProcessing AgentProcessing 阶段的命令生成
func (g *CommandGenerator) generateForProcessing(event ConversationEvent) AgentCommand {
	switch event {
	case EventNewTurn, EventInterrupt:
		// 新轮次或打断，取消当前 Agent 处理
		return CmdCancelAgent
	case EventBackchannel, EventIgnore:
		// 附和词或忽略，不做处理
		return CmdNone
	}
	return CmdNone
}

// generateForSpeaking AgentSpeaking 阶段的命令生成
func (g *CommandGenerator) generateForSpeaking(event ConversationEvent) AgentCommand {
	switch event {
	case EventInterrupt:
		// 打断，停止播放
		return CmdStopPlayback
	case EventNewTurn:
		// 新轮次，也停止播放（可能需要取消 Agent）
		return CmdStopPlayback
	case EventBackchannel, EventIgnore:
		// 附和词或忽略，恢复播放（如果暂停了）
		// 注意：这里返回 CmdNone，由上层决定是否恢复
		return CmdNone
	}
	return CmdNone
}
