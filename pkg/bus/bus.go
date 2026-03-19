// Package bus provides a minimal message bus for agent communication.
package bus

import (
	"context"
)

// InboundMessage represents an incoming message.
type InboundMessage struct {
	Channel     string
	ChatID      string
	SenderID    string
	Content     string
	Media       []string
	MediaScope  string
	ReplyTo     string
}

// OutboundMessage represents an outgoing message.
type OutboundMessage struct {
	Channel   string
	ChatID    string
	Content   string
	ReplyTo   string
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
