package conversation

import (
	"sync"
	"time"
)

// ContextManager 维护当前对话上下文（历史对话、已播放的句子等）
type ContextManager struct {
	mu sync.RWMutex

	// 对话历史
	history []HistoryEntry

	// 当前 Agent 回复（正在生成/播放）
	currentAgentReply string
	playedContent     string
	playStartTime     time.Time
}

// HistoryEntry 历史记录条目
type HistoryEntry struct {
	Role      string    // "user" or "agent"
	Content   string    // 文本内容
	Timestamp time.Time // 时间戳
}

// NewContextManager 创建新的 ContextManager
func NewContextManager() *ContextManager {
	return &ContextManager{
		history: make([]HistoryEntry, 0),
	}
}

// AddUserMessage 添加用户消息到历史
func (c *ContextManager) AddUserMessage(content string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.history = append(c.history, HistoryEntry{
		Role:      "user",
		Content:   content,
		Timestamp: time.Now(),
	})
}

// AddAgentMessage 添加 Agent 消息到历史
func (c *ContextManager) AddAgentMessage(content string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.history = append(c.history, HistoryEntry{
		Role:      "agent",
		Content:   content,
		Timestamp: time.Now(),
	})
}

// SetCurrentAgentReply 设置当前 Agent 回复
func (c *ContextManager) SetCurrentAgentReply(content string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.currentAgentReply = content
	c.playedContent = ""
	c.playStartTime = time.Now()
}

// AppendPlayedContent 追加已播放内容
func (c *ContextManager) AppendPlayedContent(content string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.playedContent += content
}

// CommitPlayedContent 提交已播放内容到历史
// 返回已播放的内容
func (c *ContextManager) CommitPlayedContent() string {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.playedContent == "" {
		return ""
	}

	c.history = append(c.history, HistoryEntry{
		Role:      "agent",
		Content:   c.playedContent,
		Timestamp: time.Now(),
	})

	played := c.playedContent
	c.playedContent = ""
	c.currentAgentReply = ""
	return played
}

// GetHistory 获取对话历史
func (c *ContextManager) GetHistory() []HistoryEntry {
	c.mu.RLock()
	defer c.mu.RUnlock()

	result := make([]HistoryEntry, len(c.history))
	copy(result, c.history)
	return result
}

// GetCurrentAgentReply 获取当前 Agent 回复
func (c *ContextManager) GetCurrentAgentReply() string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.currentAgentReply
}

// GetPlayedContent 获取已播放内容
func (c *ContextManager) GetPlayedContent() string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.playedContent
}

// GetPlayedDuration 获取已播放时长
func (c *ContextManager) GetPlayedDuration() time.Duration {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.playStartTime.IsZero() {
		return 0
	}
	return time.Since(c.playStartTime)
}

// Clear 清空上下文
func (c *ContextManager) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.history = make([]HistoryEntry, 0)
	c.currentAgentReply = ""
	c.playedContent = ""
}

// GetLastUserMessage 获取最后一条用户消息
func (c *ContextManager) GetLastUserMessage() string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	for i := len(c.history) - 1; i >= 0; i-- {
		if c.history[i].Role == "user" {
			return c.history[i].Content
		}
	}
	return ""
}
