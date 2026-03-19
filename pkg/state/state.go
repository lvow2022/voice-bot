// Package state provides state management (stub for voicebot).
package state

import "sync"

// Manager manages agent state.
type Manager struct {
	mu           sync.RWMutex
	lastChannel  string
	lastChatID   string
	workspace    string
}

// NewManager creates a new state manager.
func NewManager(workspace string) *Manager {
	return &Manager{
		workspace: workspace,
	}
}

// SetLastChannel sets the last active channel.
func (m *Manager) SetLastChannel(channel string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lastChannel = channel
	return nil
}

// GetLastChannel gets the last active channel.
func (m *Manager) GetLastChannel() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.lastChannel
}

// SetLastChatID sets the last active chat ID.
func (m *Manager) SetLastChatID(chatID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lastChatID = chatID
	return nil
}

// GetLastChatID gets the last active chat ID.
func (m *Manager) GetLastChatID() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.lastChatID
}

// RecordLastChannel records the last channel (alias for SetLastChannel).
func (m *Manager) RecordLastChannel(channel string) error {
	return m.SetLastChannel(channel)
}

// RecordLastChatID records the last chat ID (alias for SetLastChatID).
func (m *Manager) RecordLastChatID(chatID string) error {
	return m.SetLastChatID(chatID)
}
