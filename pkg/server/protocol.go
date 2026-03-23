package server

import (
	"encoding/json"

	asrtypes "voicebot/pkg/asr/types"
	ttstypes "voicebot/pkg/tts/types"
	"voicebot/pkg/voicechain"
)

// ClientMessage 客户端发送的消息
type ClientMessage struct {
	Type string `json:"type"` // interrupt, dtmf
	Key  string `json:"key"`  // DTMF 按键
}

// ServerMessage 服务端发送的消息
type ServerMessage struct {
	Type      string `json:"type"`            // asr, llm, state, error, ready
	Text      string `json:"text,omitempty"`  // 文本内容
	IsFinal   bool   `json:"isFinal,omitempty"`
	IsPartial bool   `json:"isPartial,omitempty"`
	State     string `json:"state,omitempty"`   // 状态类型
	Payload   string `json:"payload,omitempty"` // 状态附加信息
	Code      string `json:"code,omitempty"`    // 错误码
	Message   string `json:"message,omitempty"` // 错误消息
}

// InitRequest 会话初始化请求
type InitRequest struct {
	Agent string                  `json:"agent"`
	ASR   asrtypes.SessionOptions `json:"asr,omitempty"`
	TTS   ttstypes.SessionOptions `json:"tts,omitempty"`
}

// InitResponse 会话初始化响应
type InitResponse struct {
	Token     string `json:"token"`
	ExpiresAt int64  `json:"expiresAt"` // Unix timestamp
}

// DecodeClientMessage 解码客户端文本消息
func DecodeClientMessage(data []byte) (*ClientMessage, error) {
	var msg ClientMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		return nil, err
	}
	return &msg, nil
}

// EncodeServerMessage 编码服务端消息为 JSON
func EncodeServerMessage(msg *ServerMessage) ([]byte, error) {
	return json.Marshal(msg)
}

// FrameToMessage 将 voicechain.Frame 转换为 ServerMessage
func FrameToMessage(frame voicechain.Frame) *ServerMessage {
	switch f := frame.(type) {
	case *voicechain.TextFrame:
		msgType := "asr"
		if f.IsLLMGenerated {
			msgType = "llm"
		}
		return &ServerMessage{
			Type:      msgType,
			Text:      f.Text,
			IsFinal:   f.IsEnd,
			IsPartial: f.IsPartial,
		}
	case *voicechain.CloseFrame:
		return &ServerMessage{
			Type:    "state",
			State:   "closed",
			Payload: f.Reason,
		}
	}
	return nil
}
