package tools

import "context"

// Stub implementations for removed tools

// NewI2CTool creates a stub I2C tool.
func NewI2CTool() Tool {
	return &stubTool{name: "i2c", desc: "I2C tool (disabled)"}
}

// NewSPITool creates a stub SPI tool.
func NewSPITool() Tool {
	return &stubTool{name: "spi", desc: "SPI tool (disabled)"}
}

// NewMessageTool creates a stub message tool.
func NewMessageTool() *messageTool {
	return &messageTool{}
}

type messageTool struct{}

func (t *messageTool) Name() string        { return "message" }
func (t *messageTool) Description() string { return "Message tool (disabled)" }
func (t *messageTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"channel": map[string]any{"type": "string"},
			"chat_id": map[string]any{"type": "string"},
			"content": map[string]any{"type": "string"},
		},
	}
}
func (t *messageTool) Execute(ctx context.Context, args map[string]any) *ToolResult {
	return ErrorResult("message tool is disabled")
}
func (t *messageTool) SetSendCallback(cb func(channel, chatID, content string) error) {}

// NewSendFileTool creates a stub send file tool.
func NewSendFileTool(workspace string, restrict bool, maxSize int64, store interface{}) Tool {
	return &stubTool{name: "send_file", desc: "Send file tool (disabled)"}
}

// NewFindSkillsTool creates a stub find skills tool.
func NewFindSkillsTool(mgr interface{}, cache interface{}) Tool {
	return &stubTool{name: "find_skills", desc: "Find skills tool (disabled)"}
}

type stubTool struct {
	name string
	desc string
}

func (t *stubTool) Name() string        { return t.name }
func (t *stubTool) Description() string { return t.desc }
func (t *stubTool) Parameters() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{}}
}
func (t *stubTool) Execute(ctx context.Context, args map[string]any) *ToolResult {
	return ErrorResult(t.name + " tool is disabled")
}
