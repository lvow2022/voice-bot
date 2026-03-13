package conversation

// TurnManager 管理当前 Turn 和 Phase 状态
type TurnManager struct {
	state ConversationState
}

// NewTurnManager 创建新的 TurnManager
func NewTurnManager() *TurnManager {
	return &TurnManager{
		state: ConversationState{
			Turn:  UserTurn,
			Phase: AgentIdle,
		},
	}
}

// GetState 获取当前状态
func (t *TurnManager) GetState() ConversationState {
	return t.state
}

// SetState 设置状态
func (t *TurnManager) SetState(state ConversationState) {
	t.state = state
}

// SetTurn 设置轮次
func (t *TurnManager) SetTurn(turn TurnState) {
	t.state.Turn = turn
}

// SetPhase 设置 Agent 阶段
func (t *TurnManager) SetPhase(phase AgentPhase) {
	t.state.Phase = phase
}

// IsUserTurn 是否用户轮次
func (t *TurnManager) IsUserTurn() bool {
	return t.state.Turn == UserTurn
}

// IsAgentTurn 是否 Agent 轮次
func (t *TurnManager) IsAgentTurn() bool {
	return t.state.Turn == AgentTurn
}

// IsAgentProcessing Agent 是否在处理中
func (t *TurnManager) IsAgentProcessing() bool {
	return t.state.Turn == AgentTurn && t.state.Phase == AgentProcessing
}

// IsAgentSpeaking Agent 是否在说话
func (t *TurnManager) IsAgentSpeaking() bool {
	return t.state.Turn == AgentTurn && t.state.Phase == AgentSpeaking
}

// HandleVADStart 处理 VAD_START 事件
// 用户开始说话，切换到 UserTurn
func (t *TurnManager) HandleVADStart() {
	t.state.Turn = UserTurn
}

// HandleVADStop 处理 VAD_STOP 事件
// 用户停止说话（状态不变，等待 ASR_FINAL）
func (t *TurnManager) HandleVADStop() {
	// 状态不变，等待 ASR_FINAL 决定下一步
}

// HandleASRFinal 处理 ASR_FINAL 事件
// 注意：只有在 UserTurn 时才更新状态
// 在 AgentTurn 时，状态由外部命令（如 StopPlayback）控制
func (t *TurnManager) HandleASRFinal(event ConversationEvent) ConversationState {
	// 只有在用户轮次时才更新状态
	if t.state.Turn == UserTurn {
		switch event {
		case EventNewTurn, EventInterrupt:
			// 新轮次或打断，状态保持 UserTurn，等待外部启动 Agent
			t.state.Turn = UserTurn
		case EventBackchannel:
			// 附和词，状态不变
		default:
			// EventIgnore，状态不变
		}
	}
	// Agent 轮次时不改变状态，等待外部命令（如 StopPlayback）来改变
	return t.state
}

// HandleAgentStart 处理 Agent 开始处理
func (t *TurnManager) HandleAgentStart() {
	t.state.Turn = AgentTurn
	t.state.Phase = AgentProcessing
}

// HandleAgentSpeak 处理 Agent 开始说话
func (t *TurnManager) HandleAgentSpeak() {
	t.state.Phase = AgentSpeaking
}

// HandlePlaybackFinished 处理播放完成
// Agent 播放完成，切换回 UserTurn
func (t *TurnManager) HandlePlaybackFinished() {
	t.state.Turn = UserTurn
	t.state.Phase = AgentIdle
}
