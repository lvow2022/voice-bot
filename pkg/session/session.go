// Package session provides session management (stub for voicebot).
package session

import (
	"sync"
)

// SessionStore stores session data.
type SessionStore interface {
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

// JSONLBackend is a JSONL-based session store.
type JSONLBackend struct {
	store SessionStore
}

// NewJSONLBackend creates a new JSONL backend.
func NewJSONLBackend(store SessionStore) *JSONLBackend {
	return &JSONLBackend{store: store}
}

// AddMessage adds a message.
func (b *JSONLBackend) AddMessage(sessionKey, role, content string) {
	b.store.AddMessage(sessionKey, role, content)
}

// GetMessages gets messages.
func (b *JSONLBackend) GetMessages(sessionKey string) []Message {
	return b.store.GetMessages(sessionKey)
}

// Clear clears a session.
func (b *JSONLBackend) Clear(sessionKey string) {
	b.store.Clear(sessionKey)
}

// Close closes the store.
func (b *JSONLBackend) Close() error {
	return b.store.Close()
}
