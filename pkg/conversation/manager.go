package conversation

import "log/slog"

// ConversationManager 对话管理器 - 主控制器
//
// 核心职责：
//  1. 持有当前状态
//  2. 组合 Interpreter + Policy
//  3. 统一处理所有事件，返回命令
//  4. 负责状态转换
//
// 数据流：
//
//	(Event, Text) → HandleEvent() → [Interpreter] → [Policy] → Command
//	                                  ↓
//	                             [State Transition]
type ConversationManager struct {
	state       State
	interpreter Interpreter
	policy      Policy
	logger      *slog.Logger

	// 可选回调
	onStateChange func(oldState, newState State)
	onCommand     func(cmd Command, text string)
}

// Option 配置选项
type Option func(*ConversationManager)

// WithInterpreter 设置语义解释器
func WithInterpreter(i Interpreter) Option {
	return func(m *ConversationManager) { m.interpreter = i }
}

// WithPolicy 设置策略决策器
func WithPolicy(p Policy) Option {
	return func(m *ConversationManager) { m.policy = p }
}

// WithManagerLogger 设置日志
func WithManagerLogger(logger *slog.Logger) Option {
	return func(m *ConversationManager) { m.logger = logger }
}

// WithOnStateChange 设置状态变更回调
func WithOnStateChange(callback func(oldState, newState State)) Option {
	return func(m *ConversationManager) { m.onStateChange = callback }
}

// WithOnCommand 设置命令回调
func WithOnCommand(callback func(cmd Command, text string)) Option {
	return func(m *ConversationManager) { m.onCommand = callback }
}

// NewConversationManager 创建对话管理器
func NewConversationManager(opts ...Option) *ConversationManager {
	m := &ConversationManager{
		state:       StateUserTurn,
		interpreter: NewDefaultInterpreter(),
		policy:      NewDefaultPolicy(),
		logger:      slog.Default().With("component", "conversation_manager"),
	}

	for _, opt := range opts {
		opt(m)
	}

	return m
}

// HandleEvent 统一事件处理入口
// event: 事件类型
// text: 文本内容（仅 ASRFinal 需要使用）
// 返回: 命令
func (m *ConversationManager) HandleEvent(event Event, text string) Command {
	m.logger.Debug("handle event",
		"event", event.String(),
		"text", text,
		"state", m.state.String(),
	)

	// 1. 解释语义（仅 ASRFinal 需要文本）
	semantic := SemanticIgnore
	if event == EventASRFinal {
		semantic = m.interpreter.Interpret(text)
		m.logger.Debug("semantic interpreted",
			"text", text,
			"semantic", semantic.String(),
		)
	}

	// 2. 策略决策
	cmd := m.policy.Decide(m.state, event, semantic)

	// 3. 状态转换
	oldState := m.state
	m.applyTransition(event)

	// 4. 通知状态变更
	if oldState != m.state && m.onStateChange != nil {
		m.onStateChange(oldState, m.state)
	}

	// 5. 记录命令
	if !cmd.IsNone() {
		m.logger.Info("command generated",
			"state", oldState.String(),
			"event", event.String(),
			"semantic", semantic.String(),
			"command", cmd.String(),
		)

		if m.onCommand != nil {
			m.onCommand(cmd, text)
		}
	}

	return cmd
}

// applyTransition 状态转换（集中管理）
func (m *ConversationManager) applyTransition(event Event) {
	switch m.state {
	case StateUserTurn:
		// UserTurn → AgentGenerating
		if event == EventAgentGenerating {
			m.state = StateAgentGenerating
			m.logger.Debug("state transition", "from", "UserTurn", "to", "AgentGenerating")
		}

	case StateAgentGenerating:
		// AgentGenerating → AgentSpeaking
		if event == EventAgentSpeaking {
			m.state = StateAgentSpeaking
			m.logger.Debug("state transition", "from", "AgentGenerating", "to", "AgentSpeaking")
		}
		// AgentGenerating → UserTurn (生成被取消)
		if event == EventPlaybackDone {
			m.state = StateUserTurn
			m.logger.Debug("state transition", "from", "AgentGenerating", "to", "UserTurn")
		}

	case StateAgentSpeaking:
		// AgentSpeaking → UserTurn
		if event == EventPlaybackDone {
			m.state = StateUserTurn
			m.logger.Debug("state transition", "from", "AgentSpeaking", "to", "UserTurn")
		}
	}
}

// GetState 获取当前状态
func (m *ConversationManager) GetState() State {
	return m.state
}

// SetState 设置状态（外部强制设置）
func (m *ConversationManager) SetState(state State) {
	oldState := m.state
	m.state = state
	if oldState != state && m.onStateChange != nil {
		m.onStateChange(oldState, state)
	}
}

// GetInterpreter 获取语义解释器（用于配置）
func (m *ConversationManager) GetInterpreter() Interpreter {
	return m.interpreter
}

// AddBackchannelWord 添加附和词（便捷方法）
func (m *ConversationManager) AddBackchannelWord(word string) {
	if di, ok := m.interpreter.(*DefaultInterpreter); ok {
		di.AddBackchannelWord(word)
	}
}

// AddInterruptWord 添加打断词（便捷方法）
func (m *ConversationManager) AddInterruptWord(word string) {
	if di, ok := m.interpreter.(*DefaultInterpreter); ok {
		di.AddInterruptWord(word)
	}
}
