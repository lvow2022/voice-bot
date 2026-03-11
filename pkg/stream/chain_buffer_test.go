package stream

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestChainBuffer_BasicOperations 测试基本操作
func TestChainBuffer_BasicOperations(t *testing.T) {
	cb := NewChainBuffer()

	// Write 数据
	n, err := cb.Write([]byte{1, 2, 3})
	assert.NoError(t, err)
	assert.Equal(t, 3, n)
	assert.Equal(t, 3, cb.Len())

	n, err = cb.Write([]byte{4, 5, 6})
	assert.NoError(t, err)
	assert.Equal(t, 3, n)
	assert.Equal(t, 6, cb.Len())

	// Read 数据
	dst := make([]byte, 10)
	n, err = cb.Read(dst)
	assert.NoError(t, err)
	assert.Equal(t, 6, n)
	assert.Equal(t, []byte{1, 2, 3, 4, 5, 6}, dst[:n])
	assert.Equal(t, 0, cb.Len())
}

// TestChainBuffer_ReadInChunks 测试分块读取
func TestChainBuffer_ReadInChunks(t *testing.T) {
	cb := NewChainBuffer()

	// 追加多个数据块
	cb.Write([]byte{1, 2, 3})
	cb.Write([]byte{4, 5, 6})
	cb.Write([]byte{7, 8, 9})

	// 分块读取
	dst1 := make([]byte, 2)
	n1, _ := cb.Read(dst1)
	assert.Equal(t, []byte{1, 2}, dst1[:n1])

	dst2 := make([]byte, 2)
	n2, _ := cb.Read(dst2)
	assert.Equal(t, []byte{3, 4}, dst2[:n2])

	dst3 := make([]byte, 10)
	n3, _ := cb.Read(dst3)
	assert.Equal(t, []byte{5, 6, 7, 8, 9}, dst3[:n3])
}

// TestChainBuffer_EmptyBuffer 测试空缓冲区
func TestChainBuffer_EmptyBuffer(t *testing.T) {
	cb := NewChainBuffer()

	dst := make([]byte, 10)
	n, err := cb.Read(dst)
	assert.NoError(t, err)
	assert.Equal(t, 0, n)
}

// TestChainBuffer_Reset 测试重置
func TestChainBuffer_Reset(t *testing.T) {
	cb := NewChainBuffer()

	cb.Write([]byte{1, 2, 3})
	cb.Write([]byte{4, 5, 6})
	assert.Equal(t, 6, cb.Len())

	// 读取一部分
	dst := make([]byte, 2)
	cb.Read(dst)
	assert.Equal(t, 4, cb.Len())

	// 重置
	cb.Reset()
	assert.Equal(t, 0, cb.Len())

	// 追加新数据
	cb.Write([]byte{7, 8, 9})
	assert.Equal(t, 3, cb.Len())

	// 读取应该只有新数据
	dst2 := make([]byte, 10)
	n, _ := cb.Read(dst2)
	assert.Equal(t, []byte{7, 8, 9}, dst2[:n])
}

// TestChainBuffer_Peek 测试 Peek 操作
func TestChainBuffer_Peek(t *testing.T) {
	cb := NewChainBuffer()

	cb.Write([]byte{1, 2, 3})
	cb.Write([]byte{4, 5, 6})

	// Peek 不会消耗数据
	data := cb.Peek(5)
	assert.Equal(t, []byte{1, 2, 3, 4, 5}, data)
	assert.Equal(t, 6, cb.Len())

	// 再次 Peek 应该返回相同的数据
	data2 := cb.Peek(5)
	assert.Equal(t, []byte{1, 2, 3, 4, 5}, data2)

	// Peek 超过可用长度
	data3 := cb.Peek(10)
	assert.Equal(t, []byte{1, 2, 3, 4, 5, 6}, data3)
}

// TestChainBuffer_LargeData 测试大数据
func TestChainBuffer_LargeData(t *testing.T) {
	cb := NewChainBuffer()

	// 追加多个大数据块
	for i := 0; i < 100; i++ {
		data := make([]byte, 1024)
		data[0] = byte(i)
		cb.Write(data)
	}

	assert.Equal(t, 102400, cb.Len())

	// 读取所有数据
	dst := make([]byte, 102400)
	totalRead := 0

	for totalRead < 102400 {
		n, err := cb.Read(dst[totalRead:])
		require.NoError(t, err)
		totalRead += n
	}

	assert.Equal(t, 102400, totalRead)
	assert.Equal(t, 0, cb.Len())
}

// TestChainBuffer_ZeroLengthWrite 测试写入空数据
func TestChainBuffer_ZeroLengthWrite(t *testing.T) {
	cb := NewChainBuffer()

	n, err := cb.Write([]byte{})
	assert.NoError(t, err)
	assert.Equal(t, 0, n)
	assert.Equal(t, 0, cb.Len())

	n, err = cb.Write(nil)
	assert.NoError(t, err)
	assert.Equal(t, 0, n)
	assert.Equal(t, 0, cb.Len())
}

// TestChainBuffer_ZeroLengthRead 测试读取到零长度缓冲区
func TestChainBuffer_ZeroLengthRead(t *testing.T) {
	cb := NewChainBuffer()

	cb.Write([]byte{1, 2, 3})

	dst := make([]byte, 0)
	n, err := cb.Read(dst)
	assert.NoError(t, err)
	assert.Equal(t, 0, n)

	// 数据应该还在
	assert.Equal(t, 3, cb.Len())
}

// Benchmark benchmarks for ChainBuffer

// BenchmarkChainBuffer_SmallChunks benchmarks small chunk writes
func BenchmarkChainBuffer_SmallChunks(b *testing.B) {
	cb := NewChainBuffer()
	chunk := make([]byte, 320)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		cb.Write(chunk)
	}
}

// BenchmarkChainBuffer_MediumChunks benchmarks medium chunk writes
func BenchmarkChainBuffer_MediumChunks(b *testing.B) {
	cb := NewChainBuffer()
	chunk := make([]byte, 1600)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		cb.Write(chunk)
	}
}

// BenchmarkChainBuffer_LargeChunks benchmarks large chunk writes
func BenchmarkChainBuffer_LargeChunks(b *testing.B) {
	cb := NewChainBuffer()
	chunk := make([]byte, 8000)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		cb.Write(chunk)
	}
}

// BenchmarkChainBuffer_WriteAndRead benchmarks write followed by read
func BenchmarkChainBuffer_WriteAndRead(b *testing.B) {
	chunk := make([]byte, 320)
	dst := make([]byte, 320)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		cb := NewChainBuffer()
		for j := 0; j < 10; j++ {
			cb.Write(chunk)
			cb.Read(dst)
		}
	}
}

// BenchmarkChainBuffer_ManySmallWrites benchmarks many small writes
func BenchmarkChainBuffer_ManySmallWrites(b *testing.B) {
	smallChunk := make([]byte, 32)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		cb := NewChainBuffer()
		for j := 0; j < 100; j++ {
			cb.Write(smallChunk)
		}
	}
}

// BenchmarkChainBuffer_Parallel benchmarks concurrent operations
func BenchmarkChainBuffer_Parallel(b *testing.B) {
	chunk := make([]byte, 320)

	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		cb := NewChainBuffer()
		for pb.Next() {
			cb.Write(chunk)
		}
	})
}

// BenchmarkChainBuffer_ReadPerformance benchmarks read performance
func BenchmarkChainBuffer_ReadPerformance(b *testing.B) {
	chunk := make([]byte, 320)
	dst := make([]byte, 320*10)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		cb := NewChainBuffer()
		// 追加 10 个 chunk
		for j := 0; j < 10; j++ {
			cb.Write(chunk)
		}
		// 读取所有数据
		total := 0
		for total < 320*10 {
			n, _ := cb.Read(dst[total:])
			total += n
		}
	}
}

// BenchmarkChainBuffer_SequentialAccess benchmarks realistic streaming usage
func BenchmarkChainBuffer_SequentialAccess(b *testing.B) {
	chunk := make([]byte, 320)
	dst := make([]byte, 320)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		cb := NewChainBuffer()
		// 模拟 TTS 流：write 一下，read 一下
		for j := 0; j < 100; j++ {
			cb.Write(chunk)
			cb.Read(dst)
		}
	}
}

// BenchmarkChainBuffer_Peek benchmarks peek operation
func BenchmarkChainBuffer_Peek(b *testing.B) {
	chunk := make([]byte, 320)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		cb := NewChainBuffer()
		for j := 0; j < 10; j++ {
			cb.Write(chunk)
		}
		// Peek 多次
		for j := 0; j < 10; j++ {
			_ = cb.Peek(320)
		}
	}
}
