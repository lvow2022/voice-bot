package conversation

// State 对话状态（扁平化三状态模型）
type State int

const (
	StateUserTurn       State = iota // 用户轮次
	StateAgentGenerating             // Agent 生成中（LLM）
	StateAgentSpeaking               // Agent 播放中（TTS）
)

func (s State) String() string {
	switch s {
	case StateUserTurn:
		return "UserTurn"
	case StateAgentGenerating:
		return "AgentGenerating"
	case StateAgentSpeaking:
		return "AgentSpeaking"
	default:
		return "Unknown"
	}
}

// Event 输入事件类型
type Event int

const (
	EventVADStart Event = iota // 用户开始说话
	EventVADStop               // 用户停止说话
	EventASRFinal              // ASR 最终结果
	EventAgentGenerating       // Agent 开始生成
	EventAgentSpeaking         // Agent 开始播放
	EventPlaybackDone          // 播放完成
)

func (e Event) String() string {
	switch e {
	case EventVADStart:
		return "VADStart"
	case EventVADStop:
		return "VADStop"
	case EventASRFinal:
		return "ASRFinal"
	case EventAgentGenerating:
		return "AgentGenerating"
	case EventAgentSpeaking:
		return "AgentSpeaking"
	case EventPlaybackDone:
		return "PlaybackDone"
	default:
		return "Unknown"
	}
}

// Semantic 语义类型（由 Interpreter 解释得出）
type Semantic int

const (
	SemanticIgnore      Semantic = iota // 噪音/无效
	SemanticBackchannel                 // 附和词 "嗯", "对"
	SemanticInterrupt                   // 打断 "等一下", "不用了"
	SemanticNewTurn                     // 新轮次 "帮我查天气"
)

func (s Semantic) String() string {
	switch s {
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

// Command 输出命令
type Command string

const (
	CmdNone          Command = ""                              // 无动作
	CmdInvokeAgent   Command = "cmd.agent.generate.invoke"     // 启动 Agent 生成
	CmdCancelAgent   Command = "cmd.agent.generate.cancel"     // 取消 Agent 生成
	CmdStopPlayback  Command = "cmd.agent.playback.stop"      // 停止播放
	CmdPausePlayback Command = "cmd.agent.playback.pause"     // 暂停播放
)

func (c Command) String() string {
	if c == "" {
		return "None"
	}
	return string(c)
}

// IsNone 判断是否为空命令
func (c Command) IsNone() bool {
	return c == CmdNone
}
