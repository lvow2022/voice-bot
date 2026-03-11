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

// ============ TtsStream 基础流实现 ============

type TtsStream struct {
	buffer     *ChainBuffer // 数据缓冲区
	mu         sync.RWMutex // 读写锁
	maxSize    int          // 最大容量（0=无限制）
	eof        bool         // EOF 标记
	processors []Processor  // 处理器链
	meta       StreamMeta   // 流元数据
}

// NewTtsStream 创建基础流
func NewTtsStream(maxSize int) *TtsStream {
	return &TtsStream{
		buffer:     NewChainBuffer(),
		maxSize:    maxSize,
		processors: make([]Processor, 0),
	}
}

// Use 添加处理器到处理器链（类似 gin.Use()）
func (s *TtsStream) Use(processors ...Processor) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.processors = append(s.processors, processors...)
}

// Push 推送数据到流（实现 Streamer 接口）
func (s *TtsStream) Push(data []byte, eof bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 检查缓冲区是否已满
	if s.maxSize > 0 && s.buffer.Len() >= s.maxSize {
		return ErrBufferFull
	}

	// 通过处理器链处理数据
	processed, err := s.processData(data)
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

// processData 执行处理器链
func (s *TtsStream) processData(data []byte) ([]byte, error) {
	result := data
	for _, p := range s.processors {
		var err error
		result, err = p.Process(result)
		if err != nil {
			return nil, err
		}
	}
	return result, nil
}

// Pull 从流拉取数据（实现 Streamer 接口）
//
// 返回值组合：
//   - (0, nil)           缓冲区为空，流未结束，产生静音帧
//   - (n, nil)           成功读取 n 字节数据（包括 EOF 后的剩余数据）
//   - (0, ErrStreamEnded)  EOF 且缓冲区为空，流结束
func (s *TtsStream) Pull(sample []byte) (n int, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 先检查 EOF 且缓冲区为空，直接返回结束
	if s.eof && s.buffer.Len() == 0 {
		return 0, ErrStreamEnded
	}

	// 读取数据（无论 EOF 状态）
	n, _ = s.buffer.Read(sample)
	return n, nil
}

// IsEOF 检查流是否已标记 EOF
func (s *TtsStream) IsEOF() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.eof
}

// Len 返回缓冲区长度
func (s *TtsStream) Len() int {
	return s.buffer.Len()
}

// Meta 返回流元数据（实现 Streamer 接口）
func (s *TtsStream) Meta() StreamMeta {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.meta
}
