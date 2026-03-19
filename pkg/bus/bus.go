// Package bus provides a minimal message bus for agent communication.
package bus

import (
	"context"
	"errors"
)

// ErrBusClosed is returned when the bus is closed.
var ErrBusClosed = errors.New("bus closed")

// Peer represents a message sender.
type Peer struct {
	ID       string
	Name     string
	Username string
	Kind     string // "user", "bot", etc.
}

// SenderInfo contains information about the message sender.
type SenderInfo struct {
	ID       string
	Name     string
	Username string
}

// Metadata contains additional message metadata.
type Metadata map[string]any

// InboundMessage represents an incoming message.
type InboundMessage struct {
	Channel     string
	ChatID      string
	SenderID    string
	Content     string
	Media       []string
	MediaScope  string
	ReplyTo     string
	MessageID   string      // ID of the original message (for replies)
	SessionKey  string      // Optional session key override
	Peer        *Peer       // Sender peer info
	Sender      *SenderInfo // Sender info
	Metadata    Metadata    // Additional metadata
}

// OutboundMessage represents an outgoing message.
type OutboundMessage struct {
	Channel          string
	ChatID           string
	Content          string
	ReplyTo          string
	ReplyToMessageID string // For compatibility
}

// OutboundMediaMessage represents an outgoing message with media.
type OutboundMediaMessage struct {
	Channel string
	ChatID  string
	Parts   []MediaPart
}

// MediaPart represents a media attachment.
type MediaPart struct {
	Type        string
	Data        []byte
	Filename    string
	ContentType string
	Ref         string // Reference to stored media
}

// MessageBus is a minimal message bus implementation.
type MessageBus struct {
	inbound  chan InboundMessage
	outbound chan OutboundMessage
}

// NewMessageBus creates a new message bus.
func NewMessageBus() *MessageBus {
	return &MessageBus{
		inbound:  make(chan InboundMessage, 100),
		outbound: make(chan OutboundMessage, 100),
	}
}

// PublishInbound publishes an inbound message.
func (b *MessageBus) PublishInbound(ctx context.Context, msg InboundMessage) error {
	select {
	case b.inbound <- msg:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// ConsumeInbound consumes an inbound message.
func (b *MessageBus) ConsumeInbound(ctx context.Context) (InboundMessage, bool) {
	select {
	case msg := <-b.inbound:
		return msg, true
	case <-ctx.Done():
		return InboundMessage{}, false
	}
}

// PublishOutbound publishes an outbound message.
func (b *MessageBus) PublishOutbound(ctx context.Context, msg OutboundMessage) error {
	select {
	case b.outbound <- msg:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// PublishOutboundMedia publishes an outbound media message.
func (b *MessageBus) PublishOutboundMedia(ctx context.Context, msg OutboundMediaMessage) error {
	// Simplified: just ignore for now
	return nil
}

// ConsumeOutbound consumes an outbound message.
func (b *MessageBus) ConsumeOutbound(ctx context.Context) (OutboundMessage, bool) {
	select {
	case msg := <-b.outbound:
		return msg, true
	case <-ctx.Done():
		return OutboundMessage{}, false
	}
}

// SubscribeOutbound returns a channel for outbound messages and a cancel function.
func (b *MessageBus) SubscribeOutbound(ctx context.Context) (<-chan OutboundMessage, func()) {
	return b.outbound, func() {}
}

// Close closes the message bus.
func (b *MessageBus) Close() {
	close(b.inbound)
	close(b.outbound)
}
