package voicechain

import (
	"context"
	"fmt"
	"time"
)

// PipelineHandler 管道处理器
type PipelineHandler struct {
	s       *Session
	handler HandleFunc
	next    SessionHandler
	inject  FilterFunc
}

func (p *PipelineHandler) String() string {
	isFinal := p.handler == nil
	hasNext := p.next != nil
	return fmt.Sprintf("PipelineHandler{isFinal:%v hasNext: %v}", isFinal, hasNext)
}

func (p *PipelineHandler) GetContext() context.Context {
	return p.s.GetContext()
}

func (p *PipelineHandler) GetSession() *Session {
	return p.s
}

func (p *PipelineHandler) InjectFrame(f FilterFunc) {
	p.inject = f
}

func (p *PipelineHandler) CauseError(sender any, err error) {
	p.s.CauseError(sender, err)
}

func (p *PipelineHandler) EmitEvent(sender any, event Event) {
	p.s.EmitEvent(sender, event)
}

func (p *PipelineHandler) EmitFrame(sender any, frame Frame) {
	if p.inject != nil {
		discard, err := p.inject(frame)
		if err != nil {
			p.CauseError(p, err)
			return
		}
		if discard {
			return
		}
	}
	if p.handler == nil {
		p.s.putFrame(DirectionOutput, frame)
		return
	}
	p.handler(p.next, SessionData{Type: SessionDataFrame, Frame: frame})
}

func (p *PipelineHandler) AddMetric(key string, duration time.Duration) {
	p.s.AddMetric(key, duration)
}

func (p *PipelineHandler) SendToOutput(_ any, frame Frame) {
	p.s.putFrame(DirectionOutput, frame)
}
