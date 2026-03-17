package conversation

// ========== 内部状态类型 ==========

// Turn 轮次
type Turn int

const (
	TurnUser Turn = iota
	TurnAgent
)

func (t Turn) String() string {
	switch t {
	case TurnUser:
		return "User"
	case TurnAgent:
		return "Agent"
	default:
		return "Unknown"
	}
}

// Phase Agent 阶段
type Phase int

const (
	PhaseIdle Phase = iota
	PhaseGenerating // 生成中（LLM 推理）
	PhaseSpeaking   // 说话中（TTS 播放）
)

func (p Phase) String() string {
	switch p {
	case PhaseIdle:
		return "Idle"
	case PhaseGenerating:
		return "Generating"
	case PhaseSpeaking:
		return "Speaking"
	default:
		return "Unknown"
	}
}

// State 对话状态
type State struct {
	Turn  Turn
	Phase Phase
}

func (s *State) String() string {
	return s.Turn.String() + ":" + s.Phase.String()
}

// ========== 内部语义事件 ==========

// Semantic 语义事件（由 BackchannelChecker 从 ASR 结果解释得出）
type Semantic int

const (
	SemanticIgnore      Semantic = iota // 噪音/无效
	SemanticBackchannel                 // 附和词 "嗯", "对", "好的"
	SemanticInterrupt                   // 打断 "等一下", "不用了"
	SemanticNewTurn                     // 新轮次 "帮我查天气"
)

func (e Semantic) String() string {
	switch e {
	case SemanticIgnore:
		return "Ignore"
	case SemanticBackchannel:
		return "Backchannel"
	case SemanticInterrupt:
		return "Interrupt"
	case SemanticNewTurn:
		return "NewTurn"
	default:
		return "Unknown"
	}
}
