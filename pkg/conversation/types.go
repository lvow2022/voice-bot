package conversation

import "voicebot/pkg/voicechain"

// ========== 公共类型（跨包使用）==========

// Event 统一事件
type Event = voicechain.Event

// ========== 内部类型 ==========

// AudioEvent 音频事件（输入）
type AudioEvent struct {
	Type string // VADStart, VADStop, ASRPartial, ASRFinal
	Text string
}

// AgentCommand 命令（输出）- 使用字符串类型
type AgentCommand string

const (
	CmdNone                AgentCommand = ""                       // 无动作
	CmdInvokeAgentGenerate AgentCommand = AgentCommand(voicechain.CmdInvokeAgentGenerate)
	CmdCancelAgentGenerate AgentCommand = AgentCommand(voicechain.CmdCancelAgentGenerate)
	CmdStopAgentPlayback   AgentCommand = AgentCommand(voicechain.CmdStopAgentPlayback)
	CmdPauseAgentPlayback  AgentCommand = AgentCommand(voicechain.CmdPauseAgentPlayback)
)

func (c AgentCommand) String() string {
	return string(c)
}

// ========== 事件类型常量 ==========

const (
	// Audio events
	VADStart   = voicechain.StateVADSpeaking
	VADStop    = voicechain.StateVADSilence
	ASRPartial = voicechain.StateASRPartial
	ASRFinal   = voicechain.StateASRFinal
)

// ========== 辅助函数 ==========

// IsCommand 判断是否为命令事件
var IsCommand = voicechain.IsCommand

// IsState 判断是否为状态事件
var IsState = voicechain.IsState
