// Package channels provides multi-channel management (stub for voicebot).
package channels

import (
	"context"
)

// Manager manages multiple communication channels.
type Manager struct {
	channels map[string]*Channel
}

// Channel represents a communication channel.
type Channel struct {
	name             string
	reasoningChannel string
}

// NewManager creates a new channel manager.
func NewManager() *Manager {
	return &Manager{
		channels: make(map[string]*Channel),
	}
}

// SendMessage sends a message to a channel.
func (m *Manager) SendMessage(ctx context.Context, msg interface{}) error {
	return nil
}

// SendPlaceholder sends a placeholder message.
func (m *Manager) SendPlaceholder(ctx context.Context, channel, chatID string) {}

// GetChannel returns a channel by name.
func (m *Manager) GetChannel(name string) (*Channel, bool) {
	ch, ok := m.channels[name]
	return ch, ok
}

// GetEnabledChannels returns all enabled channel names.
func (m *Manager) GetEnabledChannels() []string {
	var names []string
	for name := range m.channels {
		names = append(names, name)
	}
	return names
}

// ReasoningChannelID returns the reasoning channel ID.
func (c *Channel) ReasoningChannelID() string {
	return c.reasoningChannel
}

// IsRunning returns whether the channel is running.
func (c *Channel) IsRunning() bool {
	return true
}

// IsAllowed returns whether a sender is allowed.
func (c *Channel) IsAllowed(senderID string) bool {
	return true
}

// IsAllowedSender returns whether a sender is allowed.
func (c *Channel) IsAllowedSender(sender SenderInfo) bool {
	return true
}

// Send sends a message.
func (c *Channel) Send(ctx context.Context, msg interface{}) error {
	return nil
}

// SenderInfo contains sender information.
type SenderInfo struct {
	ID    string
	Name  string
	IsBot bool
}
