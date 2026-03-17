package conversation

// TurnManager 管理当前 Turn 和 Phase 状态
type TurnManager struct {
	state State
}

// NewTurnManager 创建新的 TurnManager
func NewTurnManager() *TurnManager {
	return &TurnManager{
		state: State{
			Turn:  TurnUser,
			Phase: PhaseIdle,
		},
	}
}

// GetState 获取当前状态
func (t *TurnManager) GetState() State {
	return t.state
}

// SetState 设置状态
func (t *TurnManager) SetState(state State) {
	t.state = state
}

// SetTurn 设置轮次
func (t *TurnManager) SetTurn(turn Turn) {
	t.state.Turn = turn
}

// SetPhase 设置 Agent 阶段
func (t *TurnManager) SetPhase(phase Phase) {
	t.state.Phase = phase
}

// IsUserTurn 是否用户轮次
func (t *TurnManager) IsUserTurn() bool {
	return t.state.Turn == TurnUser
}

// IsAgentTurn 是否 Agent 轮次
func (t *TurnManager) IsAgentTurn() bool {
	return t.state.Turn == TurnAgent
}

// IsAgentGenerating Agent 是否在生成中
func (t *TurnManager) IsAgentGenerating() bool {
	return t.state.Turn == TurnAgent && t.state.Phase == PhaseGenerating
}

// IsAgentSpeaking Agent 是否在说话
func (t *TurnManager) IsAgentSpeaking() bool {
	return t.state.Turn == TurnAgent && t.state.Phase == PhaseSpeaking
}

// HandleVADStart 处理 VAD_START 事件
// 用户开始说话，切换到 UserTurn
func (t *TurnManager) HandleVADStart() {
	t.state.Turn = TurnUser
}

// HandleVADStop 处理 VAD_STOP 事件
// 用户停止说话（状态不变，等待 ASR_FINAL）
func (t *TurnManager) HandleVADStop() {
	// 状态不变，等待 ASR_FINAL 决定下一步
}

// HandleASRFinal 处理 ASR_FINAL 事件
// 注意：只有在 UserTurn 时才更新状态
// 在 AgentTurn 时，状态由外部命令（如 StopPlayback）控制
func (t *TurnManager) HandleASRFinal(event Semantic) State {
	// 只有在用户轮次时才更新状态
	if t.state.Turn == TurnUser {
		switch event {
		case SemanticNewTurn, SemanticInterrupt:
			// 新轮次或打断，状态保持 UserTurn，等待外部启动 Agent
			t.state.Turn = TurnUser
		case SemanticBackchannel:
			// 附和词，状态不变
		default:
			// SemanticIgnore，状态不变
		}
	}
	// Agent 轮次时不改变状态，等待外部命令（如 StopPlayback）来改变
	return t.state
}

// HandleAgentStart 处理 Agent 开始生成
func (t *TurnManager) HandleAgentStart() {
	t.state.Turn = TurnAgent
	t.state.Phase = PhaseGenerating
}

// HandleAgentSpeak 处理 Agent 开始说话
func (t *TurnManager) HandleAgentSpeak() {
	t.state.Phase = PhaseSpeaking
}

// HandlePlaybackFinished 处理播放完成
// Agent 播放完成，切换回 UserTurn
func (t *TurnManager) HandlePlaybackFinished() {
	t.state.Turn = TurnUser
	t.state.Phase = PhaseIdle
}
