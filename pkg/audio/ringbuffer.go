package audio

import (
	"bytes"
	"io"
	"sync"
)

type AudioRingBuffer struct {
	buf    *bytes.Buffer
	maxLen int
	mtx    sync.RWMutex
}

func NewAudioRingBuffer(maxLen int) *AudioRingBuffer {
	return &AudioRingBuffer{
		maxLen: maxLen,
		buf:    bytes.NewBuffer(nil),
	}
}

func (r *AudioRingBuffer) Len() int {
	r.mtx.Lock()
	defer r.mtx.Unlock()
	return r.buf.Len()
}

func (r *AudioRingBuffer) Discard() {
	r.mtx.Lock()
	defer r.mtx.Unlock()
	r.buf.Reset()
}

func (r *AudioRingBuffer) Write(p []byte) (n int, err error) {
	r.mtx.Lock()
	defer r.mtx.Unlock()
	return r.buf.Write(p)
}

func (r *AudioRingBuffer) ReadAtLeast(p []byte) (n int, err error) {
	// Read and discard the oldest data
	r.mtx.RLock()
	defer r.mtx.RUnlock()
	return io.ReadAtLeast(r.buf, p, len(p))
}

func (r *AudioRingBuffer) Read(p []byte) (n int, err error) {
	r.mtx.RLock()
	defer r.mtx.RUnlock()
	return r.buf.Read(p)
}

type RingBuffer[T any] struct {
	buf   []T
	size  int
	write int
	read  int
	count int
	mtx   sync.Mutex
}

func NewRingBuffer[T any](size int) *RingBuffer[T] {
	return &RingBuffer[T]{
		buf:  make([]T, size),
		size: size,
	}
}

func (r *RingBuffer[T]) Write(data []T) {
	r.mtx.Lock()
	defer r.mtx.Unlock()

	for _, v := range data {
		r.buf[r.write] = v
		r.write = (r.write + 1) % r.size

		if r.count < r.size {
			r.count++
		} else {
			r.read = (r.read + 1) % r.size
		}
	}
}

func (r *RingBuffer[T]) Read(dst []T) int {
	r.mtx.Lock()
	defer r.mtx.Unlock()

	n := len(dst)
	if n > r.count {
		n = r.count
	}

	for i := 0; i < n; i++ {
		dst[i] = r.buf[r.read]
		r.read = (r.read + 1) % r.size
	}

	r.count -= n
	return n
}

func (r *RingBuffer[T]) Peek(dst []T) int {
	r.mtx.Lock()
	defer r.mtx.Unlock()

	n := len(dst)
	if n > r.count {
		n = r.count
	}

	readPos := r.read
	for i := 0; i < n; i++ {
		dst[i] = r.buf[readPos]
		readPos = (readPos + 1) % r.size
	}

	return n
}

func (r *RingBuffer[T]) Len() int {
	r.mtx.Lock()
	defer r.mtx.Unlock()
	return r.count
}

func (r *RingBuffer[T]) Cap() int {
	return r.size
}

func (r *RingBuffer[T]) Reset() {
	r.mtx.Lock()
	defer r.mtx.Unlock()
	r.write = 0
	r.read = 0
	r.count = 0
}
