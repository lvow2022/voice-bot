package stream

import (
	"errors"
	"sync"
)

// ============ 错误定义 ============

var (
	ErrBufferFull  = errors.New("buffer full")
	ErrStreamEnded = errors.New("stream ended")
)

// ============ AudioStream 基础流实现 ============

type AudioStream struct {
	buffer      *ChainBuffer // 数据缓冲区
	mu          sync.RWMutex // 读写锁
	maxSize     int          // 最大容量（0=无限制）
	eof         bool         // EOF 标记
	pushFilters []Filter     // 输入端过滤器链（Push 时执行）
	pullFilters []Filter     // 输出端过滤器链（Pull 时执行）
	meta        StreamMeta   // 流元数据
}

// NewAudioStream 创建基础流
func NewAudioStream(maxSize int) *AudioStream {
	return &AudioStream{
		buffer:      NewChainBuffer(),
		maxSize:     maxSize,
		pushFilters: make([]Filter, 0),
		pullFilters: make([]Filter, 0),
	}
}

// PushFilter 添加过滤器到输入端（Push 时执行）
func (s *AudioStream) PushFilter(filters ...Filter) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pushFilters = append(s.pushFilters, filters...)
}

// PullFilter 添加过滤器到输出端（Pull 时执行）
func (s *AudioStream) PullFilter(filters ...Filter) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pullFilters = append(s.pullFilters, filters...)
}

// Push 推送数据到流（实现 Stream 接口）
func (s *AudioStream) Push(data []byte, eof bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 检查缓冲区是否已满
	if s.maxSize > 0 && s.buffer.Len() >= s.maxSize {
		return ErrBufferFull
	}

	// 通过输入端过滤器链处理数据
	processed, err := s.applyPushFilters(data)
	if err != nil {
		return err
	}

	// 写入处理后的数据
	if len(processed) > 0 {
		if _, err := s.buffer.Write(processed); err != nil {
			return err
		}
	}

	// EOF 标记
	if eof {
		s.eof = true
	}

	return nil
}

// Pull 从流拉取数据（实现 Stream 接口）
//
// 返回值组合：
//   - (0, nil)           缓冲区为空，流未结束，产生静音帧
//   - (n, nil)           成功读取 n 字节数据（包括 EOF 后的剩余数据）
//   - (0, ErrStreamEnded)  EOF 且缓冲区为空，流结束
func (s *AudioStream) Pull(sample []byte) (n int, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 先检查 EOF 且缓冲区为空，直接返回结束
	if s.eof && s.buffer.Len() == 0 {
		return 0, ErrStreamEnded
	}

	// 读取数据（无论 EOF 状态）
	n, _ = s.buffer.Read(sample)

	// 通过输出端过滤器链处理数据
	if n > 0 {
		processed, err := s.applyPullFilters(sample[:n])
		if err != nil {
			return 0, err
		}
		n = copy(sample, processed)
	}

	return n, nil
}

// applyPushFilters 应用输入端过滤器链
func (s *AudioStream) applyPushFilters(data []byte) ([]byte, error) {
	result := data
	for _, f := range s.pushFilters {
		var err error
		result, err = f.Filter(result)
		if err != nil {
			return nil, err
		}
	}
	return result, nil
}

// applyPullFilters 应用输出端过滤器链
func (s *AudioStream) applyPullFilters(data []byte) ([]byte, error) {
	result := data
	for _, f := range s.pullFilters {
		var err error
		result, err = f.Filter(result)
		if err != nil {
			return nil, err
		}
	}
	return result, nil
}

// IsEOF 检查流是否已标记 EOF
func (s *AudioStream) IsEOF() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.eof
}

// Len 返回缓冲区长度
func (s *AudioStream) Len() int {
	return s.buffer.Len()
}

// Meta 返回流元数据（实现 Stream 接口）
func (s *AudioStream) Meta() StreamMeta {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.meta
}

// Close 关闭流（实现 Stream 接口）
func (s *AudioStream) Close() error {
	s.Push(nil, true) // 标记 EOF
	return nil
}
