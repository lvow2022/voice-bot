package voicechain

import (
	"fmt"
	"strings"
	"time"
)

// ========== Event 定义 ==========

// Event 统一事件（控制指令和状态通知）
type Event struct {
	Type    string  // 事件类型常量
	Payload any     // 可选载荷
}

func (e *Event) String() string {
	return fmt.Sprintf("Event{Type: %s}", e.Type)
}

// ========== State Events（状态事件）==========

const (
	// Audio - VAD
	StateVADSpeaking = "state.vad.speaking" // 用户开始说话
	StateVADSilence  = "state.vad.silence"  // 用户停止说话

	// Audio - ASR
	StateASRPartial = "state.asr.partial" // ASR 部分结果
	StateASRFinal   = "state.asr.final"   // ASR 最终结果

	// Lifecycle
	StateSessionBegin  = "state.session.begin"  // 会话开始
	StateSessionEnd    = "state.session.end"    // 会话结束
	StateSessionHangup = "state.session.hangup" // 会话挂断

	// Agent
	StateAgentGenerating = "state.agent.generating" // Agent 正在生成（LLM 推理中）
	StateAgentSpeaking   = "state.agent.speaking"   // Agent 正在说话（TTS 播放中）
	StatePlaybackDone    = "state.playback.done"    // 播放完成

	// Legacy aliases for backward compatibility
	Begin  = StateSessionBegin
	End    = StateSessionEnd
	Hangup = StateSessionHangup
)

// ========== Command Events（命令事件）==========

const (
	// Agent Generate - LLM 生成相关
	CmdInvokeAgentGenerate = "cmd.agent.generate.invoke" // 调用 Agent 生成
	CmdCancelAgentGenerate = "cmd.agent.generate.cancel" // 取消 Agent 生成

	// Agent Playback - TTS 播放相关
	CmdStopAgentPlayback  = "cmd.agent.playback.stop"  // 停止 Agent 播放
	CmdPauseAgentPlayback = "cmd.agent.playback.pause" // 暂停 Agent 播放
)

// ========== 辅助函数 ==========

// IsState 判断是否为状态事件
func IsState(eventType string) bool {
	return strings.HasPrefix(eventType, "state.")
}

// IsCommand 判断是否为命令事件
func IsCommand(eventType string) bool {
	return strings.HasPrefix(eventType, "cmd.")
}

// ========== 数据类型常量 ==========

const (
	SessionDataState  = "state"
	SessionDataFrame  = "frame"
	SessionDataMetric = "metric"
)

// SessionData 会话数据
type SessionData struct {
	CreatedAt time.Time
	Sender    any
	Type      string
	Event     Event  // 统一使用 Event
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
