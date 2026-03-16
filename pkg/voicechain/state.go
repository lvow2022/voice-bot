package voicechain

import (
	"fmt"
	"time"
)

// ========== 会话生命周期状态 ==========

const (
	// StateSessionBegin 会话开始
	StateSessionBegin = "session.begin"
	// StateSessionEnd 会话结束
	StateSessionEnd = "session.end"
	// StateSessionHangup 会话挂断
	StateSessionHangup = "session.hangup"

	// Legacy aliases for backward compatibility
	Begin  = StateSessionBegin
	End    = StateSessionEnd
	Hangup = StateSessionHangup
)

// ========== Turn 状态 ==========

type TurnState int

const (
	UserTurn TurnState = iota
	AgentTurn
)

func (s TurnState) String() string {
	switch s {
	case UserTurn:
		return "User"
	case AgentTurn:
		return "Agent"
	default:
		return "Unknown"
	}
}

// ========== Agent 阶段 ==========

type AgentPhase int

const (
	AgentIdle AgentPhase = iota
	AgentProcessing
	AgentSpeaking
)

func (p AgentPhase) String() string {
	switch p {
	case AgentIdle:
		return "Idle"
	case AgentProcessing:
		return "Processing"
	case AgentSpeaking:
		return "Speaking"
	default:
		return "Unknown"
	}
}

// ========== 音频事件类型 ==========

type AudioEventType int

const (
	VADStart AudioEventType = iota
	VADStop
	ASRPartial
	ASRFinal
)

func (t AudioEventType) String() string {
	switch t {
	case VADStart:
		return "VADStart"
	case VADStop:
		return "VADStop"
	case ASRPartial:
		return "ASRPartial"
	case ASRFinal:
		return "ASRFinal"
	default:
		return "Unknown"
	}
}

// StateString 返回用于 EmitState 的状态字符串
func (t AudioEventType) StateString() string {
	switch t {
	case VADStart:
		return StateVADStart
	case VADStop:
		return StateVADStop
	case ASRPartial:
		return StateASRPartial
	case ASRFinal:
		return StateASRFinal
	default:
		return ""
	}
}

// ========== 对话事件类型 ==========

type ConversationEvent int

const (
	EventIgnore      ConversationEvent = iota // 噪音，忽略
	EventBackchannel                          // 附和词 "嗯", "对", "好的"
	EventInterrupt                            // 打断 "等一下", "不用了"
	EventNewTurn                              // 新轮次 "帮我查天气"
)

func (e ConversationEvent) String() string {
	switch e {
	case EventIgnore:
		return "Ignore"
	case EventBackchannel:
		return "Backchannel"
	case EventInterrupt:
		return "Interrupt"
	case EventNewTurn:
		return "NewTurn"
	default:
		return "Unknown"
	}
}

// ========== Agent 命令 ==========

type AgentCommand int

const (
	CmdNone          AgentCommand = iota // 无动作
	CmdStartAgent                        // 开始 LLM chat + TTS synthesize
	CmdStopPlayback                      // 停止当前播放
	CmdCancelAgent                       // 取消当前 LLM chat / TTS synthesize
	CmdCommitAgent                       // 提交已播放内容到上下文
	CmdPausePlayback                     // 暂停播放（等待 backchannel 判定）
)

func (c AgentCommand) String() string {
	switch c {
	case CmdNone:
		return "None"
	case CmdStartAgent:
		return "StartAgent"
	case CmdStopPlayback:
		return "StopPlayback"
	case CmdCancelAgent:
		return "CancelAgent"
	case CmdCommitAgent:
		return "CommitAgent"
	case CmdPausePlayback:
		return "PausePlayback"
	default:
		return "Unknown"
	}
}

// ========== 数据类型常量 ==========

const (
	SessionDataState  = "state"
	SessionDataFrame  = "frame"
	SessionDataMetric = "metric"
)

// ========== 事件和数据结构 ==========

// AudioEvent 由 AudioProcess (VAD + ASR) 产生的音频事件
type AudioEvent struct {
	Type AudioEventType
	Text string
}

// SystemEvent 系统事件，实现 Frame 接口
type SystemEvent struct {
	Type    SystemEventType
	Payload any
}

// Body 实现 Frame 接口
func (e *SystemEvent) Body() []byte {
	return nil
}

// String 实现 Frame 接口
func (e *SystemEvent) String() string {
	return fmt.Sprintf("SystemEvent{Type: %s}", e.Type)
}

// ========== 音频处理状态（用于 EmitState）==========

const (
	StateVADStart   = "audio.vad.start"
	StateVADStop    = "audio.vad.stop"
	StateASRPartial = "audio.asr.partial"
	StateASRFinal   = "audio.asr.final"
)

// ConversationState 对话状态
type ConversationState struct {
	Turn  TurnState
	Phase AgentPhase
}

// String 返回状态的可读表示
func (s *ConversationState) String() string {
	return s.Turn.String() + ":" + s.Phase.String()
}

// StateEvent 状态事件
type StateEvent struct {
	State  string `json:"state"`
	Params []any  `json:"params,omitempty"`
}

// SessionData 会话数据
type SessionData struct {
	CreatedAt time.Time
	Sender    any
	Type      string
	State     StateEvent
	Frame     Frame
	Duration  *time.Duration
}

// TranscribingData 转录数据
type TranscribingData struct {
	SenderName string        `json:"senderName"`
	Duration   time.Duration `json:"duration"`
	Result     any           `json:"result"`
	Direction  string        `json:"direction"`
	DialogID   string        `json:"dialogID"`
	Timestamp  time.Time     `json:"timestamp"`
}

// CompletedData 完成数据
type CompletedData struct {
	SenderName string        `json:"senderName"`
	Duration   time.Duration `json:"duration"`
	Result     any           `json:"result"`
	DialogID   string        `json:"dialogID"`
	Timestamp  time.Time     `json:"timestamp"`
}
