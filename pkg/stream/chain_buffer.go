package stream

import (
	"sync"
)

// chunk 表示链表中的一个数据块
type chunk struct {
	data []byte
	next *chunk
}

// ChainBuffer 是一个基于链表的零拷贝缓冲区实现
// 适用于大量频繁 Write 操作的场景，避免数据复制
// 实现了 io.Reader 和 io.Writer 接口
type ChainBuffer struct {
	mu     sync.Mutex
	head   *chunk // 链表头部
	tail   *chunk // 链表尾部
	offset int    // 当前读取位置在 head.data 中的偏移
	length int    // 缓冲区总长度（字节）
}

// NewChainBuffer 创建一个新的链表缓冲区
func NewChainBuffer() *ChainBuffer {
	return &ChainBuffer{}
}

// Write 写入数据到缓冲区（实现 io.Writer 接口）
// 零拷贝：调用者不应该在写入后修改 p 的内容
func (cb *ChainBuffer) Write(p []byte) (n int, err error) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	if len(p) == 0 {
		return 0, nil
	}

	// 创建新的 chunk
	newChunk := &chunk{
		data: p,
		next: nil,
	}

	// 添加到链表尾部
	if cb.tail == nil {
		// 第一个 chunk
		cb.head = newChunk
		cb.tail = newChunk
	} else {
		cb.tail.next = newChunk
		cb.tail = newChunk
	}

	cb.length += len(p)
	return len(p), nil
}

// Read 从缓冲区读取数据
func (cb *ChainBuffer) Read(dst []byte) (n int, err error) {
	if len(dst) == 0 {
		return 0, nil
	}

	cb.mu.Lock()
	defer cb.mu.Unlock()

	// 缓冲区为空
	if cb.head == nil {
		return 0, nil
	}

	totalRead := 0
	remaining := len(dst)

	// 从链表中读取数据
	for remaining > 0 && cb.head != nil {
		available := len(cb.head.data) - cb.offset

		if available == 0 {
			// 当前 chunk 已读完，移动到下一个
			cb.head = cb.head.next
			cb.offset = 0
			if cb.head == nil {
				cb.tail = nil
			}
			continue
		}

		// 计算本次读取的字节数
		toRead := available
		if toRead > remaining {
			toRead = remaining
		}

		// 复制数据到目标缓冲区
		copy(dst[totalRead:], cb.head.data[cb.offset:cb.offset+toRead])
		totalRead += toRead
		remaining -= toRead
		cb.offset += toRead
		cb.length -= toRead

		// 如果当前 chunk 读完，移动到下一个
		if cb.offset >= len(cb.head.data) {
			cb.head = cb.head.next
			cb.offset = 0
			if cb.head == nil {
				cb.tail = nil
			}
		}
	}

	return totalRead, nil
}

// Len 返回缓冲区中可读数据的字节数
func (cb *ChainBuffer) Len() int {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	return cb.length
}

// Reset 清空缓冲区
func (cb *ChainBuffer) Reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.head = nil
	cb.tail = nil
	cb.offset = 0
	cb.length = 0
}

// Peek 查看缓冲区的前 n 个字节但不移除它们
func (cb *ChainBuffer) Peek(n int) []byte {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	if n <= 0 || cb.head == nil {
		return nil
	}

	result := make([]byte, 0, n)
	remaining := n
	current := cb.head
	offset := cb.offset

	for remaining > 0 && current != nil {
		available := len(current.data) - offset
		if available == 0 {
			current = current.next
			offset = 0
			continue
		}

		toPeek := available
		if toPeek > remaining {
			toPeek = remaining
		}

		result = append(result, current.data[offset:offset+toPeek]...)
		remaining -= toPeek
		offset += toPeek

		if offset >= len(current.data) {
			current = current.next
			offset = 0
		}
	}

	return result
}
