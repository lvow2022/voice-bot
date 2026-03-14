package audio

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"voicebot/pkg/voicechain"
)

func TestNewRecorder(t *testing.T) {
	fileName := filepath.Join(t.TempDir(), "test.wav")
	file, err := NewRecordFile(fileName)
	assert.Nil(t, err)
	assert.NotNil(t, file)
	defer file.Close()

	codecOpt := voicechain.CodecOption{
		SampleRate:    16000,
		BitDepth:      16,
		FrameDuration: "20ms",
	}

	recorder := NewRecorder(fileName, file, codecOpt)
	assert.NotNil(t, recorder)

	recorder.WriteMono([]byte("test"))
	recorder.WriteStereo([]byte("test"))

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	recorder.Start(ctx)

	<-ctx.Done()
	recorder.Close()
	if ctx.Err() == context.DeadlineExceeded {
		t.Log("Test timed out after 3 seconds")
	} else {
		t.Errorf("Test was cancelled: %v", ctx.Err())
	}
}

// BenchmarkRecorder_30Seconds 模拟写入 30 秒数据并测量性能
func BenchmarkRecorder_30Seconds(b *testing.B) {

	// 设置测试参数
	sampleRate := 16000
	bitDepth := 16
	frameDuration := 20 * time.Millisecond
	duration := 120 * time.Second
	concurrency := 100 // 并发数

	// 计算每帧的字节数
	bytesPerFrame := sampleRate * bitDepth / 8 * int(frameDuration.Milliseconds()) / 1000

	// 打印初始内存状态
	var m1, m2 runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&m1)

	b.ResetTimer()
	b.ReportAllocs()

	// 只运行一次，但并发 100 个 recorder
	b.StopTimer()

	// 生成测试数据（模拟音频数据）
	testData := make([]byte, bytesPerFrame)
	for j := range testData {
		testData[j] = byte(j % 256)
	}

	var wg sync.WaitGroup
	startTime := time.Now()

	b.StartTimer()

	// 启动 100 个并发的 recorder
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			fileName := filepath.Join(b.TempDir(), fmt.Sprintf("recorder_%d.wav", id))
			file, err := NewRecordFile(fileName)
			if err != nil {
				b.Errorf("Failed to create file for recorder %d: %v", id, err)
				return
			}
			defer file.Close()

			codecOpt := voicechain.CodecOption{
				SampleRate:    sampleRate,
				BitDepth:      bitDepth,
				FrameDuration: frameDuration.String(),
			}

			recorder := NewRecorder(fileName, file, codecOpt)

			// 启动 recorder
			ctx, cancel := context.WithTimeout(context.Background(), duration+1*time.Second)
			defer cancel()
			go recorder.Start(ctx)

			// 模拟写入 30 秒数据
			writeInterval := frameDuration
			ticker := time.NewTicker(writeInterval)
			defer ticker.Stop()

			writeCount := 0
			for time.Since(startTime) < duration {
				select {
				case <-ticker.C:
					recorder.WriteMono(testData)
					recorder.WriteStereo(testData)
					writeCount++
				case <-ctx.Done():
					break
				}
			}

			// 等待 recorder 完成
			time.Sleep(100 * time.Millisecond)
			recorder.Close()
		}(i)
	}

	// 等待所有 recorder 完成
	wg.Wait()
	b.StopTimer()

	// 打印最终内存状态
	runtime.ReadMemStats(&m2)

	b.Logf("\n=== Memory Statistics (100 Concurrent Recorders) ===")
	b.Logf("Concurrency: %d", concurrency)
	b.Logf("Duration: %v", time.Since(startTime))
	b.Logf("Allocs: %d allocations", m2.Mallocs-m1.Mallocs)
	b.Logf("TotalAlloc: %d bytes (%.2f MB)", m2.TotalAlloc-m1.TotalAlloc, float64(m2.TotalAlloc-m1.TotalAlloc)/1024/1024)
	b.Logf("Sys: %d bytes (%.2f MB)", m2.Sys-m1.Sys, float64(m2.Sys-m1.Sys)/1024/1024)
	b.Logf("HeapAlloc: %d bytes (%.2f MB)", m2.HeapAlloc-m1.HeapAlloc, float64(m2.HeapAlloc-m1.HeapAlloc)/1024/1024)
	b.Logf("HeapSys: %d bytes (%.2f MB)", m2.HeapSys-m1.HeapSys, float64(m2.HeapSys-m1.HeapSys)/1024/1024)
	b.Logf("HeapInuse: %d bytes (%.2f MB)", m2.HeapInuse-m1.HeapInuse, float64(m2.HeapInuse-m1.HeapInuse)/1024/1024)
	b.Logf("HeapIdle: %d bytes (%.2f MB)", m2.HeapIdle-m1.HeapIdle, float64(m2.HeapIdle-m1.HeapIdle)/1024/1024)
	b.Logf("NumGC: %d GC cycles", m2.NumGC-m1.NumGC)
}

// TestRecorder_MemoryUsage 测试内存使用情况，模拟写入 30 秒数据
func TestRecorder_MemoryUsage(t *testing.T) {

	// 设置测试参数
	sampleRate := 16000
	bitDepth := 16
	frameDuration := 20 * time.Millisecond
	duration := 30 * time.Second

	// 计算每帧的字节数
	bytesPerFrame := sampleRate * bitDepth / 8 * int(frameDuration.Milliseconds()) / 1000

	// 打印初始内存状态
	var m1, m2 runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&m1)
	t.Logf("=== Initial Memory State ===")
	t.Logf("HeapAlloc: %d bytes (%.2f MB)", m1.HeapAlloc, float64(m1.HeapAlloc)/1024/1024)
	t.Logf("HeapSys: %d bytes (%.2f MB)", m1.HeapSys, float64(m1.HeapSys)/1024/1024)

	fileName := filepath.Join(t.TempDir(), "memory_test.wav")
	file, err := NewRecordFile(fileName)
	assert.Nil(t, err)
	defer file.Close()

	codecOpt := voicechain.CodecOption{
		SampleRate:    sampleRate,
		BitDepth:      bitDepth,
		FrameDuration: frameDuration.String(),
	}

	recorder := NewRecorder(fileName, file, codecOpt)

	// 启动 recorder
	ctx, cancel := context.WithTimeout(context.Background(), duration+1*time.Second)
	defer cancel()
	go recorder.Start(ctx)

	// 模拟写入 30 秒数据
	startTime := time.Now()
	writeInterval := frameDuration
	ticker := time.NewTicker(writeInterval)
	defer ticker.Stop()

	// 生成测试数据（模拟音频数据）
	testData := make([]byte, bytesPerFrame)
	for i := range testData {
		// 生成简单的测试数据
		testData[i] = byte(i % 256)
	}

	writeCount := 0
	checkInterval := 5 * time.Second
	lastCheckTime := time.Now()

	for time.Since(startTime) < duration {
		select {
		case <-ticker.C:
			recorder.WriteMono(testData)
			recorder.WriteStereo(testData)
			writeCount++

			// 每 5 秒打印一次内存状态
			if time.Since(lastCheckTime) >= checkInterval {
				var m runtime.MemStats
				runtime.ReadMemStats(&m)
				t.Logf("After %v: HeapAlloc=%.2f MB, HeapSys=%.2f MB, Writes=%d",
					time.Since(startTime),
					float64(m.HeapAlloc)/1024/1024,
					float64(m.HeapSys)/1024/1024,
					writeCount)
				lastCheckTime = time.Now()
			}
		case <-ctx.Done():
			break
		}
	}

	// 等待 recorder 完成
	time.Sleep(100 * time.Millisecond)
	recorder.Close()

	// 清理测试文件
	if err := os.Remove(fileName); err == nil {
		t.Logf("Cleaned up test file: %s", fileName)
	}

	// 打印最终内存状态
	runtime.GC()
	runtime.ReadMemStats(&m2)
	t.Logf("\n=== Final Memory State ===")
	t.Logf("Total writes: %d frames", writeCount)
	t.Logf("Allocs: %d allocations", m2.Mallocs-m1.Mallocs)
	t.Logf("TotalAlloc: %d bytes (%.2f MB)", m2.TotalAlloc-m1.TotalAlloc, float64(m2.TotalAlloc-m1.TotalAlloc)/1024/1024)
	t.Logf("HeapAlloc: %d bytes (%.2f MB)", m2.HeapAlloc-m1.HeapAlloc, float64(m2.HeapAlloc-m1.HeapAlloc)/1024/1024)
	t.Logf("HeapSys: %d bytes (%.2f MB)", m2.HeapSys-m1.HeapSys, float64(m2.HeapSys-m1.HeapSys)/1024/1024)
	t.Logf("HeapInuse: %d bytes (%.2f MB)", m2.HeapInuse-m1.HeapInuse, float64(m2.HeapInuse-m1.HeapInuse)/1024/1024)
	t.Logf("HeapIdle: %d bytes (%.2f MB)", m2.HeapIdle-m1.HeapIdle, float64(m2.HeapIdle-m1.HeapIdle)/1024/1024)
	t.Logf("NumGC: %d GC cycles", m2.NumGC-m1.NumGC)
}
