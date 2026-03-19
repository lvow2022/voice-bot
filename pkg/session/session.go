// Package session provides session management (stub for voicebot).
package session

import (
	"context"
	"sync"

	"voicebot/pkg/providers"
)

// SessionStore stores session data with full history support.
type SessionStore interface {
	AddMessage(sessionKey, role, content string)
	GetHistory(sessionKey string) []providers.Message
	GetSummary(sessionKey string) string
	Save(sessionKey string)
	AddFullMessage(sessionKey string, msg providers.Message)
	SetHistory(sessionKey string, history []providers.Message)
	SetSummary(sessionKey, summary string)
	TruncateHistory(sessionKey string, keepLast int)
	Clear(sessionKey string)
	Close() error
}

// SimpleSessionStore is a minimal session store interface.
type SimpleSessionStore interface {
	AddMessage(sessionKey, role, content string)
	GetMessages(sessionKey string) []Message
	Clear(sessionKey string)
	Close() error
}

// Message represents a session message.
type Message struct {
	Role    string
	Content string
}

// SessionManager manages sessions in memory.
type SessionManager struct {
	mu       sync.RWMutex
	sessions map[string][]Message
	dir      string
}

// NewSessionManager creates a new session manager.
func NewSessionManager(dir string) *SessionManager {
	return &SessionManager{
		sessions: make(map[string][]Message),
		dir:      dir,
	}
}

// AddMessage adds a message to a session.
func (sm *SessionManager) AddMessage(sessionKey, role, content string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.sessions[sessionKey] = append(sm.sessions[sessionKey], Message{
		Role:    role,
		Content: content,
	})
}

// GetHistory returns the history as providers.Message slice.
func (sm *SessionManager) GetHistory(sessionKey string) []providers.Message {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	msgs := sm.sessions[sessionKey]
	result := make([]providers.Message, len(msgs))
	for i, m := range msgs {
		result[i] = providers.Message{Role: m.Role, Content: m.Content}
	}
	return result
}

// GetSummary returns an empty summary (memory store doesn't support summaries).
func (sm *SessionManager) GetSummary(sessionKey string) string {
	return ""
}

// Save is a no-op for memory store.
func (sm *SessionManager) Save(sessionKey string) {}

// AddFullMessage adds a full message to the session.
func (sm *SessionManager) AddFullMessage(sessionKey string, msg providers.Message) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.sessions[sessionKey] = append(sm.sessions[sessionKey], Message{
		Role:    msg.Role,
		Content: msg.Content,
	})
}

// GetMessages gets all messages for a session.
func (sm *SessionManager) GetMessages(sessionKey string) []Message {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.sessions[sessionKey]
}

// Clear clears a session.
func (sm *SessionManager) Clear(sessionKey string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	delete(sm.sessions, sessionKey)
}

// Close closes the session manager.
func (sm *SessionManager) Close() error {
	return nil
}

// SetHistory sets the session history (replaces existing).
func (sm *SessionManager) SetHistory(sessionKey string, history []providers.Message) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	msgs := make([]Message, len(history))
	for i, m := range history {
		msgs[i] = Message{Role: m.Role, Content: m.Content}
	}
	sm.sessions[sessionKey] = msgs
}

// SetSummary sets the session summary (no-op for memory store).
func (sm *SessionManager) SetSummary(sessionKey, summary string) {}

// TruncateHistory truncates the session history.
func (sm *SessionManager) TruncateHistory(sessionKey string, keepLast int) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	msgs := sm.sessions[sessionKey]
	if keepLast <= 0 || keepLast >= len(msgs) {
		return
	}
	sm.sessions[sessionKey] = msgs[len(msgs)-keepLast:]
}

// JSONLBackend wraps a memory store to provide SessionStore interface.
type JSONLBackend struct {
	store interface {
		AddMessage(ctx context.Context, sessionKey, role, content string) error
		GetHistory(ctx context.Context, sessionKey string) ([]providers.Message, error)
		GetSummary(ctx context.Context, sessionKey string) (string, error)
		AddFullMessage(ctx context.Context, sessionKey string, msg providers.Message) error
		SetHistory(ctx context.Context, sessionKey string, history []providers.Message) error
		SetSummary(ctx context.Context, sessionKey, summary string) error
		TruncateHistory(ctx context.Context, sessionKey string, keepLast int) error
		Close() error
	}
}

// NewJSONLBackend creates a new JSONL backend.
func NewJSONLBackend(store interface{}) *JSONLBackend {
	// Type assertion to get the expected interface
	type jsonlStore interface {
		AddMessage(ctx context.Context, sessionKey, role, content string) error
		GetHistory(ctx context.Context, sessionKey string) ([]providers.Message, error)
		GetSummary(ctx context.Context, sessionKey string) (string, error)
		AddFullMessage(ctx context.Context, sessionKey string, msg providers.Message) error
		SetHistory(ctx context.Context, sessionKey string, history []providers.Message) error
		SetSummary(ctx context.Context, sessionKey, summary string) error
		TruncateHistory(ctx context.Context, sessionKey string, keepLast int) error
		Close() error
	}
	return &JSONLBackend{store: store.(jsonlStore)}
}

// AddMessage adds a message.
func (b *JSONLBackend) AddMessage(sessionKey, role, content string) {
	b.store.AddMessage(context.Background(), sessionKey, role, content)
}

// GetHistory returns the session history.
func (b *JSONLBackend) GetHistory(sessionKey string) []providers.Message {
	msgs, _ := b.store.GetHistory(context.Background(), sessionKey)
	return msgs
}

// GetSummary returns the session summary.
func (b *JSONLBackend) GetSummary(sessionKey string) string {
	summary, _ := b.store.GetSummary(context.Background(), sessionKey)
	return summary
}

// Save is a no-op (JSONL store auto-saves).
func (b *JSONLBackend) Save(sessionKey string) {}

// AddFullMessage adds a full message.
func (b *JSONLBackend) AddFullMessage(sessionKey string, msg providers.Message) {
	b.store.AddFullMessage(context.Background(), sessionKey, msg)
}

// SetHistory sets the session history.
func (b *JSONLBackend) SetHistory(sessionKey string, history []providers.Message) {
	b.store.SetHistory(context.Background(), sessionKey, history)
}

// SetSummary sets the session summary.
func (b *JSONLBackend) SetSummary(sessionKey, summary string) {
	b.store.SetSummary(context.Background(), sessionKey, summary)
}

// TruncateHistory truncates the session history.
func (b *JSONLBackend) TruncateHistory(sessionKey string, keepLast int) {
	b.store.TruncateHistory(context.Background(), sessionKey, keepLast)
}

// GetMessages is not supported by JSONLBackend.
func (b *JSONLBackend) GetMessages(sessionKey string) []Message {
	return nil
}

// Clear clears a session (not implemented for JSONL).
func (b *JSONLBackend) Clear(sessionKey string) {}

// Close closes the store.
func (b *JSONLBackend) Close() error {
	return b.store.Close()
}
