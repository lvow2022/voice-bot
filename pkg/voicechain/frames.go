package voicechain

import (
	"encoding/json"
	"fmt"
	"time"
)

type Frame interface {
	fmt.Stringer
	Body() []byte
}

type TextFrame struct {
	PlayID         string    `json:"id,omitempty"`
	Text           string    `json:"text"`
	IsTranscribed  bool      `json:"isTranscribed"`
	IsLLMGenerated bool      `json:"isLLMGenerated"`
	IsPartial      bool      `json:"isPartial"`
	IsEnd          bool      `json:"isEnd"`
	Sequence       int       `json:"sequence"`
	StartAt        time.Time `json:"startAt"`
}

type AudioFrame struct {
	PlayID        string `json:"id,omitempty"`
	Sequence      int    `json:"sequence"`
	Payload       []byte `json:"payload"`
	IsFirstFrame  bool   `json:"isFirstFrame,omitempty"`
	IsEndFrame    bool   `json:"isEndFrame,omitempty"`
	IsSynthesized bool   `json:"isSynthesized,omitempty"`
	IsSilence     bool   `json:"isSilence,omitempty"`
	SourceText    string `json:"sourceText,omitempty"`
}

type FunctionFrame struct {
	Name   string   `json:"name"`
	Params []string `json:"params,omitempty"`
}

type InterruptFrame struct {
	Sender any `json:"sender"`
}

type DtmfFrame struct {
	Event  string
	Volume int
}

type CloseFrame struct {
	Reason string `json:"reason"`
}

func (f *CloseFrame) Body() []byte {
	return nil
}

func (f *CloseFrame) String() string {
	return fmt.Sprintf("CloseFrame{Reason: %s}", f.Reason)
}

func (f *DtmfFrame) Body() []byte {
	return nil
}

func (f *DtmfFrame) String() string {
	return fmt.Sprintf("DtmfFrame{Event: %s}", f.Event)
}

func (f *InterruptFrame) Body() []byte {
	return nil
}

func (f *InterruptFrame) String() string {
	return fmt.Sprintf("InterruptFrame{Sender: %v}", f.Sender)
}

func (f *FunctionFrame) Body() []byte {
	data, _ := json.Marshal(f)
	return data
}

func (f *FunctionFrame) String() string {
	return fmt.Sprintf("FunctionFrame{Name: %s, Params: %v}", f.Name, f.Params)
}

// Body implements Frame.
func (t *TextFrame) Body() []byte {
	return []byte(t.Text)
}

// String implements Frame.
func (t *TextFrame) String() string {
	source := "user"
	if t.IsTranscribed {
		source = "Transcribed"
	}
	if t.IsLLMGenerated {
		source = "LLMGenerated"
	}
	return fmt.Sprintf("TextFrame{Text: %q, Source: %s, IsPartial: %t, IsEnd: %t, Sequence: %d}}",
		t.Text, source, t.IsPartial, t.IsEnd, t.Sequence)
}

// Body implements Frame.
func (d *AudioFrame) Body() []byte {
	return d.Payload
}

// String implements Frame.
func (d *AudioFrame) String() string {
	return fmt.Sprintf("AudioFrame{Payload: %d bytes, IsFirstFrame: %t, IsSynthesized: %t, IsSilence: %t}",
		len(d.Payload), d.IsFirstFrame, d.IsSynthesized, d.IsSilence)
}

// ========== Conversation Pipeline Frames ==========

// SystemEventType 系统事件类型
type SystemEventType int

const (
	SystemEventAgentStart        SystemEventType = iota // Agent 开始处理
	SystemEventAgentSpeak                              // Agent 开始说话
	SystemEventPlaybackFinished                        // 播放完成
)

func (t SystemEventType) String() string {
	switch t {
	case SystemEventAgentStart:
		return "AgentStart"
	case SystemEventAgentSpeak:
		return "AgentSpeak"
	case SystemEventPlaybackFinished:
		return "PlaybackFinished"
	default:
		return "Unknown"
	}
}

// SystemEventFrame 系统事件帧，用于传递 Agent 状态
type SystemEventFrame struct {
	Type    SystemEventType `json:"type"`
	Payload any             `json:"payload,omitempty"`
}

func (f *SystemEventFrame) Body() []byte {
	return nil
}

func (f *SystemEventFrame) String() string {
	return fmt.Sprintf("SystemEventFrame{Type: %s}", f.Type)
}

// CommandFrame 控制命令帧，封装 AgentCommand
type CommandFrame struct {
	Command   AgentCommand `json:"command"`
	Text      string       `json:"text,omitempty"`      // 关联的文本（如用户输入）
	Timestamp time.Time    `json:"timestamp"`
}

func (f *CommandFrame) Body() []byte {
	return nil
}

func (f *CommandFrame) String() string {
	return fmt.Sprintf("CommandFrame{Command: %s, Text: %q}", f.Command, f.Text)
}
