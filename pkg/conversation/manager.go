package conversation

import (
	"log/slog"

	"voicebot/pkg/voicechain"
)

// ConversationManager 对话管理器 - 主运行时控制器
//
// ConversationManager 的核心职责：
//  1. 管理 Turn / Phase 状态
//     • TurnUser / TurnAgent
//     • PhaseGenerating / PhaseSpeaking
//  2. 处理用户输入事件
//     • VAD / ASR partial / ASR final
//     • 判断 backchannel / interrupt / new turn
//  3. 生成控制命令
//     • CmdInvokeAgent / CmdPausePlayback / CmdStopPlayback / CmdCancelAgent
//
// ConversationManager 内部模块职责：
//   - TurnManager: 管理 Turn 和 Phase 状态
//   - BackchannelChecker: 判断用户输入类型（backchannel / interrupt / ignore）
//   - CommandGenerator: 生成 AgentCommand 控制 Agent / Playback
//   - EventQueue: 顺序管理事件，保证处理一致性
//   - ContextManager: 维护历史上下文 / 已播放内容
type ConversationManager struct {
	// 核心组件
	turnManager        *TurnManager
	backchannelChecker *BackchannelChecker
	commandGenerator   *CommandGenerator
	eventQueue         *EventQueue
	contextManager     *ContextManager

	// 可选的状态变更回调
	onStateChange func(oldState, newState State)
	onCommand     func(cmd AgentCommand)

	// 日志
	logger *slog.Logger
}

// Option 配置选项
type Option func(*ConversationManager)

// WithLogger 设置日志
func WithLogger(logger *slog.Logger) Option {
	return func(m *ConversationManager) {
		m.logger = logger
	}
}

// WithOnStateChange 设置状态变更回调
func WithOnStateChange(callback func(oldState, newState State)) Option {
	return func(m *ConversationManager) {
		m.onStateChange = callback
	}
}

// WithOnCommand 设置命令回调
func WithOnCommand(callback func(cmd AgentCommand)) Option {
	return func(m *ConversationManager) {
		m.onCommand = callback
	}
}

// WithBackchannelChecker 设置自定义 BackchannelChecker
func WithBackchannelChecker(checker *BackchannelChecker) Option {
	return func(m *ConversationManager) {
		m.backchannelChecker = checker
	}
}

// NewConversationManager 创建新的对话管理器
func NewConversationManager(opts ...Option) *ConversationManager {
	m := &ConversationManager{
		turnManager:        NewTurnManager(),
		backchannelChecker: NewBackchannelChecker(),
		commandGenerator:   NewCommandGenerator(),
		eventQueue:         NewEventQueue(),
		contextManager:     NewContextManager(),
		logger:             slog.Default().With("component", "conversation_manager"),
	}

	for _, opt := range opts {
		opt(m)
	}

	// 设置事件队列的处理器
	m.eventQueue.SetHandler(m)

	return m
}

// HandleAudioEvent 处理音频事件，返回 Agent 命令
// 实现 EventHandler 接口
func (m *ConversationManager) HandleAudioEvent(event AudioEvent) AgentCommand {
	m.logger.Debug("handle audio event",
		"type", event.Type,
		"text", event.Text,
	)

	var cmd AgentCommand

	switch event.Type {
	case VADStart:
		cmd = m.handleVADStart(event)
	case VADStop:
		cmd = m.handleVADStop(event)
	case ASRPartial:
		cmd = m.handleASRPartial(event)
	case ASRFinal:
		cmd = m.handleASRFinal(event)
	}

	if cmd != CmdNone && m.onCommand != nil {
		m.onCommand(cmd)
	}

	return cmd
}

// HandleSystemEvent 处理系统事件
// 实现 EventHandler 接口
func (m *ConversationManager) HandleSystemEvent(event Event) AgentCommand {
	m.logger.Debug("handle system event", "type", event.Type)

	var cmd AgentCommand

	switch event.Type {
	case voicechain.StateAgentGenerating:
		m.turnManager.HandleAgentStart()
		m.notifyStateChange()
	case voicechain.StateAgentSpeaking:
		m.turnManager.HandleAgentSpeak()
		m.notifyStateChange()
	case voicechain.StatePlaybackDone:
		cmd = m.handlePlaybackFinished()
	}

	if cmd != CmdNone && m.onCommand != nil {
		m.onCommand(cmd)
	}

	return cmd
}

// HandlePlaybackFinished 处理播放完成
// 实现 EventHandler 接口
func (m *ConversationManager) HandlePlaybackFinished() AgentCommand {
	return m.handlePlaybackFinished()
}

// handleVADStart 处理 VAD_START 事件
func (m *ConversationManager) handleVADStart(_ AudioEvent) AgentCommand {
	state := m.turnManager.GetState()

	// 如果 Agent 正在说话，可能需要暂停
	if m.turnManager.IsAgentSpeaking() {
		cmd := m.commandGenerator.GenerateForVAD(state, VADStart)
		if cmd == CmdPauseAgentPlayback {
			m.logger.Info("pause playback on VAD start")
		}
		return cmd
	}

	// 更新状态为 UserTurn
	oldState := m.turnManager.GetState()
	m.turnManager.HandleVADStart()
	m.notifyStateChangeWith(oldState)

	return CmdNone
}

// handleVADStop 处理 VAD_STOP 事件
func (m *ConversationManager) handleVADStop(_ AudioEvent) AgentCommand {
	m.turnManager.HandleVADStop()
	return CmdNone
}

// handleASRPartial 处理 ASR_PARTIAL 事件
func (m *ConversationManager) handleASRPartial(_ AudioEvent) AgentCommand {
	// ASR partial 通常不触发命令，仅用于 UI 显示
	return CmdNone
}

// handleASRFinal 处理 ASR_FINAL 事件
func (m *ConversationManager) handleASRFinal(event AudioEvent) AgentCommand {
	// 使用 BackchannelChecker 判断事件类型
	convEvent := m.backchannelChecker.Check(event)

	m.logger.Debug("backchannel check result",
		"text", event.Text,
		"event", convEvent.String(),
	)

	// 记录用户消息到上下文
	if convEvent == SemanticNewTurn || convEvent == SemanticInterrupt {
		m.contextManager.AddUserMessage(event.Text)
	}

	// 更新 TurnManager 状态
	oldState := m.turnManager.GetState()
	m.turnManager.HandleASRFinal(convEvent)
	m.notifyStateChangeWith(oldState)

	// 使用 CommandGenerator 生成命令
	state := m.turnManager.GetState()
	cmd := m.commandGenerator.Generate(state, convEvent)

	m.logger.Info("command generated",
		"state", state.String(),
		"event", convEvent.String(),
		"command", cmd.String(),
	)

	return cmd
}

// handlePlaybackFinished 处理播放完成
func (m *ConversationManager) handlePlaybackFinished() AgentCommand {
	oldState := m.turnManager.GetState()
	m.turnManager.HandlePlaybackFinished()
	m.notifyStateChangeWith(oldState)

	// 提交已播放内容到上下文（内部处理，不作为命令返回）
	m.contextManager.CommitPlayedContent()

	m.logger.Info("playback finished, turn to user")

	return CmdNone
}

// notifyStateChange 通知状态变更
func (m *ConversationManager) notifyStateChange() {
	m.notifyStateChangeWith(m.turnManager.GetState())
}

// notifyStateChangeWith 通知状态变更（带旧状态）
func (m *ConversationManager) notifyStateChangeWith(oldState State) {
	newState := m.turnManager.GetState()
	if m.onStateChange != nil && oldState != newState {
		m.onStateChange(oldState, newState)
	}
}

// ========== 公共 API ==========

// PushEvent 推入音频事件（异步）
func (m *ConversationManager) PushEvent(event AudioEvent) {
	m.eventQueue.PushAudio(event)
}

// PushSystemEvent 推入系统事件（异步）
func (m *ConversationManager) PushSystemEvent(event Event) {
	m.eventQueue.PushSystem(event)
}

// PushPlaybackDone 推入播放完成事件（异步）
func (m *ConversationManager) PushPlaybackDone() {
	m.eventQueue.PushPlaybackDone()
}

// GetState 获取当前状态
func (m *ConversationManager) GetState() State {
	return m.turnManager.GetState()
}

// SetState 设置状态（外部强制设置）
func (m *ConversationManager) SetState(state State) {
	oldState := m.turnManager.GetState()
	m.turnManager.SetState(state)
	m.notifyStateChangeWith(oldState)
}

// GetContextManager 获取上下文管理器
func (m *ConversationManager) GetContextManager() *ContextManager {
	return m.contextManager
}

// GetBackchannelChecker 获取附和词检查器（用于配置）
func (m *ConversationManager) GetBackchannelChecker() *BackchannelChecker {
	return m.backchannelChecker
}

// Close 关闭管理器
func (m *ConversationManager) Close() {
	m.eventQueue.Close()
}
