package conversation

import (
	"context"
	"sync"

	"voicebot/pkg/voicechain"
)

// EventQueue 管理事件顺序，保证 ConversationManager 内部处理是顺序一致的
type EventQueue struct {
	mu     sync.Mutex
	events []queueEvent
	cond   *sync.Cond
	closed bool

	// 处理器
	handler EventHandler
}

// queueEvent 内部队列事件
type queueEvent struct {
	audioEvent   *AudioEvent
	systemEvent  *voicechain.Event
	playbackDone bool
}

// EventHandler 事件处理器接口
type EventHandler interface {
	HandleAudioEvent(event AudioEvent) AgentCommand
	HandleSystemEvent(event voicechain.Event) AgentCommand
	HandlePlaybackFinished() AgentCommand
}

// NewEventQueue 创建新的事件队列
func NewEventQueue() *EventQueue {
	q := &EventQueue{
		events: make([]queueEvent, 0, 16),
	}
	q.cond = sync.NewCond(&q.mu)
	return q
}

// SetHandler 设置事件处理器
func (q *EventQueue) SetHandler(handler EventHandler) {
	q.handler = handler
}

// PushAudio 推入音频事件
func (q *EventQueue) PushAudio(event AudioEvent) {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.closed {
		return
	}

	q.events = append(q.events, queueEvent{audioEvent: &event})
	q.cond.Signal()
}

// PushSystem 推入系统事件
func (q *EventQueue) PushSystem(event voicechain.Event) {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.closed {
		return
	}

	q.events = append(q.events, queueEvent{systemEvent: &event})
	q.cond.Signal()
}

// PushPlaybackDone 推入播放完成事件
func (q *EventQueue) PushPlaybackDone() {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.closed {
		return
	}

	q.events = append(q.events, queueEvent{playbackDone: true})
	q.cond.Signal()
}

// Run 运行事件循环
func (q *EventQueue) Run(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			event := q.pop()
			if event == nil {
				continue
			}

			if q.handler != nil {
				if event.audioEvent != nil {
					q.handler.HandleAudioEvent(*event.audioEvent)
				} else if event.systemEvent != nil {
					q.handler.HandleSystemEvent(*event.systemEvent)
				} else if event.playbackDone {
					q.handler.HandlePlaybackFinished()
				}
			}
		}
	}
}

// pop 弹出事件（阻塞直到有事件）
func (q *EventQueue) pop() *queueEvent {
	q.mu.Lock()
	defer q.mu.Unlock()

	for len(q.events) == 0 && !q.closed {
		q.cond.Wait()
	}

	if q.closed {
		return nil
	}

	event := q.events[0]
	q.events = q.events[1:]
	return &event
}

// Close 关闭队列
func (q *EventQueue) Close() {
	q.mu.Lock()
	defer q.mu.Unlock()

	q.closed = true
	q.cond.Broadcast()
}

// Len 获取队列长度
func (q *EventQueue) Len() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.events)
}
