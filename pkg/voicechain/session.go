package voicechain

import (
	"context"
	"fmt"
	"log/slog"
	"reflect"
	"runtime/debug"
	"strings"
	"sync"
	"time"
)

// SessionHandler 会话处理器接口
type SessionHandler interface {
	GetContext() context.Context
	GetSession() *Session
	CauseError(sender any, err error)
	EmitEvent(sender any, event Event)
	EmitFrame(sender any, frame Frame)
	SendToOutput(sender any, frame Frame)
	AddMetric(key string, duration time.Duration)
	InjectFrame(f FilterFunc)
}

// FilterFunc 帧过滤函数
type FilterFunc func(frame Frame) (bool, error)

// EventFunc 事件处理函数
type EventFunc func(event Event)

// ErrorFunc 错误处理函数
type ErrorFunc func(sender any, err error)

// EncodeFunc 编码函数
type EncodeFunc func(frame Frame) ([]Frame, error)

// HandleFunc 数据处理函数
type HandleFunc func(h SessionHandler, data SessionData)

// Session 会话
type Session struct {
	ctx          context.Context
	cancel       context.CancelFunc
	encoder      EncodeFunc
	decoder      EncodeFunc
	values       sync.Map
	eventHandles map[string][]EventFunc
	handles      []HandleFunc
	pipeline     []*PipelineHandler
	errors       []ErrorFunc
	inputs       []*TransportLayer
	outputs      []*TransportLayer
	dataChan     chan *SessionData

	ID         string    `json:"id"`
	Running    bool      `json:"running"`
	QueueSize  int       `json:"queueSize"`
	SampleRate int       `json:"sampleRate"`
	StartAt    time.Time `json:"startAt"`
}

// NewSession 创建新会话
func NewSession() *Session {
	return &Session{
		ID:           "session-" + time.Now().Format("20060102150405"),
		values:       sync.Map{},
		eventHandles: make(map[string][]EventFunc),
		SampleRate:   16000,
		Running:      false,
		QueueSize:    128,
	}
}

// SetID 设置会话ID
func (s *Session) SetID(id string) *Session {
	s.ID = id
	return s
}

func (s *Session) String() string {
	return fmt.Sprintf("Session{ID: %s, Running: %t}", s.ID, s.Running)
}

// Get 获取值
func (s *Session) Get(key string) (val any, ok bool) {
	return s.values.Load(key)
}

// Set 设置值
func (s *Session) Set(key string, val any) {
	s.values.Store(key, val)
}

// Delete 删除值
func (s *Session) Delete(key string) {
	s.values.Delete(key)
}

// GetContext 获取上下文
func (s *Session) GetContext() context.Context {
	if s.ctx == nil {
		s.Context(context.Background())
	}
	return s.ctx
}

// GetSession 实现 SessionHandler
func (s *Session) GetSession() *Session {
	return s
}

// InjectFrame 注入帧过滤器
func (s *Session) InjectFrame(_ FilterFunc) {}

// Context 设置上下文
func (s *Session) Context(parent context.Context) *Session {
	s.ctx, s.cancel = context.WithCancel(parent)
	return s
}

// Encode 设置编码器
func (s *Session) Encode(enc EncodeFunc) *Session {
	s.encoder = enc
	return s
}

// Decode 设置解码器
func (s *Session) Decode(dec EncodeFunc) *Session {
	s.decoder = dec
	return s
}

// Input 添加输入传输
func (s *Session) Input(rx Transport, filterFuncs ...FilterFunc) *Session {
	tl := &TransportLayer{
		session:   s,
		transport: rx,
		filters:   filterFuncs,
	}
	rx.Attach(s)
	s.inputs = append(s.inputs, tl)
	return s
}

// Output 添加输出传输
func (s *Session) Output(tx Transport, filterFuncs ...FilterFunc) *Session {
	queueSize := s.QueueSize
	if queueSize == 0 {
		queueSize = 128
	}
	tl := &TransportLayer{
		txqueue:   make(chan Frame, queueSize),
		session:   s,
		transport: tx,
		filters:   filterFuncs,
	}
	slog.Debug("output transport", "sessionID", s.ID, "tx", tx, "queue", queueSize)
	tx.Attach(s)
	s.outputs = append(s.outputs, tl)
	return s
}

// Error 设置错误处理
func (s *Session) Error(handles ...ErrorFunc) *Session {
	s.errors = append(s.errors, handles...)
	return s
}

// On 设置事件处理
func (s *Session) On(eventType string, handles ...EventFunc) *Session {
	s.eventHandles[eventType] = append(s.eventHandles[eventType], handles...)
	return s
}

// Pipeline 设置处理管道
func (s *Session) Pipeline(handles ...HandleFunc) *Session {
	s.handles = append(s.handles, handles...)
	return s
}

// buildPipeline 构建管道链
func (s *Session) buildPipeline() {
	for idx := range s.handles {
		handle := s.handles[idx]
		pipeline := &PipelineHandler{
			s:       s,
			handler: handle,
			next:    nil,
		}
		if idx > 0 {
			prev := s.pipeline[idx-1]
			prev.next = pipeline
		}
		s.pipeline = append(s.pipeline, pipeline)
	}
	// 添加最后一个处理节点
	if len(s.pipeline) > 0 {
		lastHandler := &PipelineHandler{
			s:       s,
			handler: nil,
			next:    nil,
		}
		s.pipeline[len(s.pipeline)-1].next = lastHandler
		s.pipeline = append(s.pipeline, lastHandler)
	}
}

// IsValid 验证会话
func (s *Session) IsValid() error {
	if len(s.inputs) == 0 {
		return ErrNotInputTransport
	}
	if len(s.outputs) == 0 {
		return ErrNotOutputTransport
	}
	return nil
}

// Serve 启动会话（阻塞）
func (s *Session) Serve() error {
	s.StartAt = time.Now()
	s.Running = true

	defer func() {
		if err := recover(); err != nil {
			slog.Error("session panic", "error", err, "stacktrace", string(debug.Stack()))
			return
		}
		s.Running = false
		slog.Info("session stopped", "sessionID", s.ID)
		s.cleanup()
		s.EmitEvent(s, Event{Type: StateSessionEnd})
	}()

	s.dataChan = make(chan *SessionData, s.QueueSize)
	s.buildPipeline()

	// 启动输入/输出处理
	for _, tl := range s.inputs {
		go tl.processIncoming()
	}
	for _, tl := range s.outputs {
		go tl.processOutgoing()
	}

	s.EmitEvent(s, Event{Type: StateSessionBegin})
	slog.Info("session started", "sessionID", s.ID)

	// 主循环
serveLoop:
	for {
		select {
		case <-s.ctx.Done():
			break serveLoop
		case data := <-s.dataChan:
			s.processData(data)
		}
	}

	return nil
}

// Close 关闭会话
func (s *Session) Close() error {
	if s.cancel != nil {
		s.cancel()
	}
	return nil
}

// Codec 获取编解码选项
func (s *Session) Codec() CodecOption {
	return CodecOption{
		Codec:      "pcm",
		SampleRate: s.SampleRate,
		Channels:   1,
		BitDepth:   16,
	}
}

// cleanup 清理资源
func (s *Session) cleanup() {
	for _, tl := range s.inputs {
		tl.cleanup()
	}
	for _, tl := range s.outputs {
		tl.cleanup()
	}
	s.dataChan = nil
}

// putFrame 发送帧到输出
func (s *Session) putFrame(direction string, frame Frame) {
	tls := s.inputs
	if direction == DirectionOutput {
		tls = s.outputs
	}
	for _, tl := range tls {
		tl.trySendFrame(frame)
	}
}

func senderAsString(sender any) string {
	if sender == nil {
		return ""
	}
	if str, ok := sender.(string); ok {
		return str
	}
	n := reflect.TypeOf(sender).String()
	if end := strings.LastIndex(n, "."); end != -1 {
		n = n[end+1:]
	}
	return n
}

// CauseError 触发错误
func (s *Session) CauseError(sender any, err error) {
	sender = senderAsString(sender)
	slog.Error("cause error", "sessionID", s.ID, "sender", sender, "error", err)

	for _, handle := range s.errors {
		handle(sender, err)
	}
}

// EmitEvent 发送事件
func (s *Session) EmitEvent(sender any, event Event) {
	sender = senderAsString(sender)

	slog.Debug("emit event", "sender", sender, "type", event.Type, "sessionID", s.ID)

	data := &SessionData{
		CreatedAt: time.Now(),
		Sender:    sender,
		Type:      SessionDataState,
		Event:     event,
	}

	if s.dataChan == nil {
		s.processData(data)
		return
	}
	s.dataChan <- data
}

// EmitFrame 发送帧事件
func (s *Session) EmitFrame(sender any, frame Frame) {
	data := &SessionData{
		CreatedAt: time.Now(),
		Sender:    sender,
		Type:      SessionDataFrame,
		Frame:     frame,
	}

	if s.dataChan == nil {
		s.processData(data)
		return
	}
	s.dataChan <- data
}

// SendToOutput 发送帧到输出
func (s *Session) SendToOutput(_ any, frame Frame) {
	s.putFrame(DirectionOutput, frame)
}

// AddMetric 添加指标
func (s *Session) AddMetric(key string, duration time.Duration) {
	slog.Debug("add metric", "sessionID", s.ID, "key", key, "duration", duration)
}

// EmitCallMetric 发送通话指标
func (s *Session) EmitCallMetric(dialogID string, metric any) {
	slog.Debug("emit call metric", "sessionID", s.ID, "dialogID", dialogID, "metric", metric)
}

// processData 处理数据
func (s *Session) processData(data *SessionData) {
	switch data.Type {
	case SessionDataState:
		// 处理特定事件类型
		if handles, ok := s.eventHandles[data.Event.Type]; ok {
			for _, handle := range handles {
				callHandleWithEvent(s, handle, data.Event)
			}
		}
		// 处理通配符事件
		if handles, ok := s.eventHandles["*"]; ok {
			for _, handle := range handles {
				callHandleWithEvent(s, handle, data.Event)
			}
		}
		// 传递给管道
		for _, pipeline := range s.pipeline {
			if pipeline.handler != nil {
				callHandleWithSessionData(s, pipeline.next, pipeline.handler, *data)
			}
		}

	case SessionDataFrame:
		if s.pipeline == nil {
			slog.Error("session: no pipeline", "sessionID", s.ID, "data", data)
			return
		}
		s.pipeline[0].EmitFrame(data.Sender, data.Frame)
	}
}

func callHandleWithEvent(s *Session, handle EventFunc, event Event) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("event panic", "sessionID", s.ID, "event", event, "error", r, "stacktrace", string(debug.Stack()))
		}
	}()
	handle(event)
}

func callHandleWithSessionData(s *Session, h SessionHandler, handle HandleFunc, data SessionData) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("handle panic", "sessionID", s.ID, "data", data, "error", r, "stacktrace", string(debug.Stack()))
		}
	}()
	handle(h, data)
}
