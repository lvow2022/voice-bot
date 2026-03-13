package conversation

// 重新导出 voicechain 的类型，方便使用
import "voicebot/pkg/voicechain"

// TurnState 轮次状态
type TurnState = voicechain.TurnState

// AgentPhase Agent 阶段
type AgentPhase = voicechain.AgentPhase

// AudioEventType 音频事件类型
type AudioEventType = voicechain.AudioEventType

// ConversationEvent 对话事件类型
type ConversationEvent = voicechain.ConversationEvent

// AgentCommand 命令
type AgentCommand = voicechain.AgentCommand

// AudioEvent 音频事件
type AudioEvent = voicechain.AudioEvent

// ConversationState 对话状态
type ConversationState = voicechain.ConversationState

// 常量别名
const (
	UserTurn       = voicechain.UserTurn
	AgentTurn      = voicechain.AgentTurn
	AgentIdle      = voicechain.AgentIdle
	AgentProcessing = voicechain.AgentProcessing
	AgentSpeaking  = voicechain.AgentSpeaking

	VADStart   = voicechain.VADStart
	VADStop    = voicechain.VADStop
	ASRPartial = voicechain.ASRPartial
	ASRFinal   = voicechain.ASRFinal

	EventIgnore      = voicechain.EventIgnore
	EventBackchannel = voicechain.EventBackchannel
	EventInterrupt   = voicechain.EventInterrupt
	EventNewTurn     = voicechain.EventNewTurn

	CmdNone          = voicechain.CmdNone
	CmdStartAgent    = voicechain.CmdStartAgent
	CmdStopPlayback  = voicechain.CmdStopPlayback
	CmdCancelAgent   = voicechain.CmdCancelAgent
	CmdCommitAgent   = voicechain.CmdCommitAgent
	CmdPausePlayback = voicechain.CmdPausePlayback
)
