package stream

import (
	"context"
	"fmt"
	"sync"
	"time"

	"voicebot/pkg/device"
)

type State int

const (
	StateIdle State = iota
	StatePlaying
	StatePaused
)

func (s State) String() string {
	switch s {
	case StateIdle:
		return "idle"
	case StatePlaying:
		return "playing"
	case StatePaused:
		return "paused"
	default:
		return "unknown"
	}
}

type Output interface {
	Write(data []byte) error
}

type OutputFunc func(data []byte) error

func (f OutputFunc) Write(data []byte) error {
	return f(data)
}

type StreamPlayer struct {
	config  device.DeviceConfig
	output  Output
	queue   chan Stream
	current Stream
	buffer  []byte
	state   State
	paused  bool
	mu      sync.RWMutex
	ctxMain context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup
	once    sync.Once
}

func NewPlayer(output Output, config device.DeviceConfig) (*StreamPlayer, error) {
	ctxMain, cancel := context.WithCancel(context.Background())

	if config.PeriodMs == 0 {
		config.PeriodMs = 20
	}
	frameSize := config.SampleRate * config.Channels * 2 * config.PeriodMs / 1000

	return &StreamPlayer{
		config:  config,
		output:  output,
		buffer:  make([]byte, frameSize),
		queue:   make(chan Stream, 10),
		ctxMain: ctxMain,
		cancel:  cancel,
	}, nil
}

func (p *StreamPlayer) Run() {
	p.wg.Add(1)
	go p.run()
}

func (p *StreamPlayer) run() {
	defer p.wg.Done()

	ticker := time.NewTicker(time.Duration(p.config.PeriodMs) * time.Millisecond)
	defer ticker.Stop()

	var current Stream

	for {
		select {
		case <-p.ctxMain.Done():
			return
		case s, ok := <-p.queue:
			if !ok {
				return
			}
			current = s
			p.mu.Lock()
			p.current = s
			p.state = StatePlaying
			p.paused = false
			p.mu.Unlock()

		case <-ticker.C:
			if current == nil {
				continue
			}

			p.mu.RLock()
			paused := p.paused
			p.mu.RUnlock()

			if paused {
				continue
			}

			n, err := current.Pull(p.buffer)
			if err == ErrStreamEnded {
				current = nil
				p.mu.Lock()
				p.current = nil
				p.state = StateIdle
				p.mu.Unlock()
				continue
			}
			if err != nil {
				fmt.Printf("[Player] pull error: %v\n", err)
				current = nil
				p.mu.Lock()
				p.current = nil
				p.state = StateIdle
				p.mu.Unlock()
				continue
			}

			if n > 0 {
				if err := p.output.Write(p.buffer[:n]); err != nil {
					fmt.Printf("[Player] write error: %v\n", err)
				}
			}
		}
	}
}

func (p *StreamPlayer) Enqueue(s Stream) {
	select {
	case p.queue <- s:
	default:
		fmt.Println("[Player] queue full, stream dropped")
	}
}

func (p *StreamPlayer) Pause() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.paused = true
	p.state = StatePaused
}

func (p *StreamPlayer) Resume() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.paused = false
	if p.current != nil {
		p.state = StatePlaying
	}
}

func (p *StreamPlayer) Stop() {
	p.mu.Lock()
	p.current = nil
	p.state = StateIdle
	p.mu.Unlock()
}

func (p *StreamPlayer) StopAndClear() {
	p.Stop()
	p.clearQueue()
}

func (p *StreamPlayer) clearQueue() {
	cleared := 0
	for {
		select {
		case <-p.queue:
			cleared++
		default:
			if cleared > 0 {
				fmt.Printf("[Player] cleared %d streams\n", cleared)
			}
			return
		}
	}
}

func (p *StreamPlayer) Close() {
	p.once.Do(func() {
		p.cancel()
		p.clearQueue()
	})
	p.wg.Wait()
}

func (p *StreamPlayer) State() State {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.state
}

func (p *StreamPlayer) QueueLength() int {
	return len(p.queue)
}

func (p *StreamPlayer) IsPlaying() bool {
	return p.State() == StatePlaying
}

func (p *StreamPlayer) IsPaused() bool {
	return p.State() == StatePaused
}
