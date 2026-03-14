package audio

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	gornnoise "github.com/shenjinti/go-rnnoise"
	"voicebot/pkg/codecs"
	"voicebot/pkg/dfn"
	"github.com/youpy/go-wav"
)

// TestRNNoise_Offline_PCM 测试 RNNoise 降噪算法（离线 PCM 文件）
func TestRNNoise_Offline_PCM(t *testing.T) {
	file, err := os.Open("../../testdata/通话记录-4708-user-pcm.pcm") // 16bit PCM, mono
	if err != nil {
		log.Fatalf("Failed to open file: %v", err)
	}
	defer file.Close()

	ns := gornnoise.NewRNNoise()
	defer func() {
		// RNNoise 没有显式的 Close 方法，这里可以添加清理逻辑
	}()

	sampleRate := 16000
	//frameSize := gornnoise.GetFrameSize() * 2 // RNNoise 需要 48kHz，帧大小 * 2 bytes
	buffer := make([]byte, GetSampleSize(sampleRate, 16, 1)*20) // 20ms at 16kHz

	// 创建输出文件
	outputFile, err := os.Create("../../testdata/rnnoise_output.pcm")
	if err != nil {
		log.Fatalf("Failed to create output file: %v", err)
	}
	defer outputFile.Close()

	totalProcessed := 0
	for {
		n, err := file.Read(buffer)
		if err == io.EOF {
			break
		}
		if n < len(buffer) {
			break
		}

		// RNNoise 需要 48kHz，先重采样
		payload48k, err := codecs.ResamplePCM(buffer[:n], sampleRate, 48000)
		if err != nil {
			t.Errorf("Failed to resample: %v", err)
			continue
		}

		// 处理音频
		var output []byte
		fsize := gornnoise.GetFrameSize() * 2
		for i := 0; i < len(payload48k); i += fsize {
			end := i + fsize
			if end > len(payload48k) {
				end = len(payload48k)
			}
			buf := payload48k[i:end]
			buf = ns.Process(buf)
			output = append(output, buf...)
		}

		// 重采样回原始采样率
		output16k, err := codecs.ResamplePCM(output, 48000, sampleRate)
		if err != nil {
			t.Errorf("Failed to resample back: %v", err)
			continue
		}

		// 写入输出文件
		outputFile.Write(output16k)
		totalProcessed += len(output16k)

		fmt.Printf("[RNNoise] Processed %d bytes\n", len(output16k))
	}

	fmt.Printf("[RNNoise] Total processed: %d bytes\n", totalProcessed)
}

// TestONNXNoiseSuppressor_Offline_PCM 测试 ONNX 降噪算法（离线 PCM 文件）
func TestONNXNoiseSuppressor_Offline_PCM(t *testing.T) {
	file, err := os.Open("../../testdata/通话记录-4708-user-pcm.pcm") // 16bit PCM, mono
	if err != nil {
		log.Fatalf("Failed to open file: %v", err)
	}
	defer file.Close()

	config := dfn.DefaultSuppressorConfig()
	ns, err := dfn.NewSuppressor(config)
	if err != nil {
		t.Fatalf("Failed to create ONNX noise suppressor: %v", err)
	}
	defer ns.Destroy()

	sampleRate := 16000
	frameSize := GetSampleSize(sampleRate, 16, 1) * 20 // 20ms
	buffer := make([]byte, frameSize)

	// 创建输出文件
	outputFile, err := os.Create("../../testdata/onnx_output.pcm")
	if err != nil {
		log.Fatalf("Failed to create output file: %v", err)
	}
	defer outputFile.Close()

	totalProcessed := 0
	for {
		n, err := file.Read(buffer)
		if err == io.EOF {
			break
		}
		if n < frameSize {
			break
		}

		// 处理音频
		output, err := ns.Process(buffer[:n], sampleRate)
		if err != nil {
			t.Errorf("Failed to process: %v", err)
			continue
		}

		// 写入输出文件
		outputFile.Write(output)
		totalProcessed += len(output)

		fmt.Printf("[ONNX] Processed %d bytes -> %d bytes\n", n, len(output))
	}

	fmt.Printf("[ONNX] Total processed: %d bytes\n", totalProcessed)
}

// TestONNXNoiseSuppressor_Offline_WAV 测试 ONNX 降噪算法（离线 WAV 文件，输出 WAV）
func TestONNXNoiseSuppressor_Offline_WAV(t *testing.T) {
	file, err := os.Open("../../testdata/s16_16k_1c_zh.wav")
	if err != nil {
		log.Fatalf("Failed to open file: %v", err)
	}
	defer file.Close()

	// 读取 WAV 文件
	wavReader := wav.NewReader(file)
	format, err := wavReader.Format()
	if err != nil {
		log.Fatalf("Failed to read WAV format: %v", err)
	}

	sampleRate := int(format.SampleRate)
	fmt.Printf("[ONNX] Input WAV: SampleRate=%d, Channels=%d, BitsPerSample=%d\n",
		sampleRate, format.NumChannels, format.BitsPerSample)

	// 先读取所有 PCM 数据
	allInputPCM, err := io.ReadAll(wavReader)
	if err != nil {
		t.Fatalf("Failed to read WAV data: %v", err)
	}
	fmt.Printf("[ONNX] Input PCM bytes: %d\n", len(allInputPCM))

	config := dfn.DefaultSuppressorConfig()
	ns, err := dfn.NewSuppressor(config)
	if err != nil {
		t.Fatalf("Failed to create ONNX noise suppressor: %v", err)
	}
	defer ns.Destroy()

	frameSize := GetSampleSize(sampleRate, 16, 1) * 20 // 20ms

	// 收集所有处理后的 PCM 数据
	var allOutputPCM []byte
	framesProcessed := 0

	for i := 0; i < len(allInputPCM); i += frameSize {
		end := i + frameSize
		if end > len(allInputPCM) {
			end = len(allInputPCM)
		}

		frame := allInputPCM[i:end]
		if len(frame) == 0 {
			break
		}

		// 处理音频
		output, err := ns.Process(frame, sampleRate)
		if err != nil {
			t.Errorf("Failed to process frame %d: %v", framesProcessed, err)
			continue
		}

		allOutputPCM = append(allOutputPCM, output...)
		framesProcessed++
	}

	fmt.Printf("[ONNX] Frames processed: %d\n", framesProcessed)

	// 计算总样本数
	totalSamples := uint32(len(allOutputPCM) / 2)
	fmt.Printf("[ONNX] Total PCM bytes: %d, Total samples: %d\n", len(allOutputPCM), totalSamples)

	// 创建输出 WAV 文件（现在知道正确的样本数）
	outputFile, err := os.Create("../../testdata/onnx_output.wav")
	if err != nil {
		log.Fatalf("Failed to create output file: %v", err)
	}
	defer outputFile.Close()

	// 使用正确的样本数创建 WAV Writer
	wavWriter := wav.NewWriter(outputFile, totalSamples, 1, uint32(sampleRate), 16)

	// 将 PCM 数据转换为 wav.Sample 格式并写入
	samples := make([]wav.Sample, totalSamples)
	for i := uint32(0); i < totalSamples; i++ {
		sampleValue := int16(allOutputPCM[i*2]) | int16(allOutputPCM[i*2+1])<<8
		samples[i].Values[0] = int(sampleValue)
	}

	if err := wavWriter.WriteSamples(samples); err != nil {
		t.Fatalf("Failed to write samples: %v", err)
	}

	fmt.Printf("[ONNX] WAV file written successfully: %d samples (%.2f seconds)\n",
		totalSamples, float64(totalSamples)/float64(sampleRate))
}

func TestONNXNoiseSuppressor_Offline_PCM_Concurrent(t *testing.T) {
	const (
		concurrency = 100

		pcmPath    = "../../testdata/通话记录-4708-user-pcm.pcm"
		sampleRate = 16000
		bitDepth   = 16
		channels   = 1
	)

	frameSize := GetSampleSize(sampleRate, bitDepth, channels) * 20 // 20ms

	t.Logf("start stress test: concurrency=%d", concurrency)

	// =========================
	// Load PCM once
	// =========================
	pcmData, err := os.ReadFile(pcmPath)
	if err != nil {
		t.Fatalf("failed to read pcm file: %v", err)
	}

	totalFrames := len(pcmData) / frameSize
	t.Logf("pcm loaded: bytes=%d frames=%d", len(pcmData), totalFrames)

	// =========================
	// Store outputs per worker
	// =========================
	outputs := make([][]byte, concurrency)

	startAll := time.Now()

	var wg sync.WaitGroup
	wg.Add(concurrency)

	for i := 0; i < concurrency; i++ {
		time.Sleep(1 * time.Millisecond)

		go func(workerID int) {
			defer wg.Done()

			start := time.Now()

			config := dfn.DefaultSuppressorConfig()
			ns, err := dfn.NewSuppressor(config)
			if err != nil {
				t.Errorf("[worker-%d] create NS failed: %v", workerID, err)
				return
			}
			defer ns.Destroy()

			var out bytes.Buffer
			out.Grow(len(pcmData))

			for frameID := 0; frameID < totalFrames; frameID++ {
				offset := frameID * frameSize
				frame := pcmData[offset : offset+frameSize]

				enhanced, err := ns.Process(frame, sampleRate)
				if err != nil {
					t.Errorf("[worker-%d] process error frame=%d: %v", workerID, frameID, err)
					return
				}

				out.Write(enhanced)
			}

			outputs[workerID] = out.Bytes()

			elapsed := time.Since(start)

			fmt.Printf(
				"[worker-%03d] done | cost=%v | goroutines=%d | out_bytes=%d\n",
				workerID,
				elapsed,
				runtime.NumGoroutine(),
				len(outputs[workerID]),
			)

		}(i)
	}

	wg.Wait()

	totalCost := time.Since(startAll)

	// =========================
	// Compare outputs
	// =========================
	ref := outputs[0]
	for i := 1; i < concurrency; i++ {
		if !bytes.Equal(ref, outputs[i]) {
			// find first mismatch
			minLen := min(len(ref), len(outputs[i]))
			for j := 0; j < minLen; j++ {
				if ref[j] != outputs[i][j] {
					t.Fatalf(
						"output mismatch worker=%d offset=%d ref=%d got=%d",
						i, j, ref[j], outputs[i][j],
					)
				}
			}
			t.Fatalf("output length mismatch worker=%d ref=%d got=%d",
				i, len(ref), len(outputs[i]))
		}
	}

	fmt.Println("=================================")
	fmt.Println("ALL WORKERS DONE")
	fmt.Println("concurrency :", concurrency)
	fmt.Println("total cost  :", totalCost)
	fmt.Println("avg / worker:", totalCost/time.Duration(concurrency))
	fmt.Println("goroutines  :", runtime.NumGoroutine())
	fmt.Println("outputs     : ALL MATCH ✅")
	fmt.Println("=================================")
}

// TestRNNoise_Offline_PCM_Concurrent 并发测试 RNNoise 降噪算法
func TestRNNoise_Offline_PCM_Concurrent(t *testing.T) {
	const (
		// ====== 压测参数 ======
		maxGoroutines = 10 // 固定并发上限
		totalJobs     = 10 // 总任务数
		interval      = 20 * time.Millisecond

		// ====== PCM 参数 ======
		pcmPath    = "../../testdata/通话记录-4708-user-pcm.pcm"
		sampleRate = 16000
		bitDepth   = 16
		channels   = 1
	)

	frameSize := GetSampleSize(sampleRate, bitDepth, channels) * 20 // 20ms

	t.Logf("start test: maxGoroutines=%d totalJobs=%d interval=%s",
		maxGoroutines, totalJobs, interval)

	// =========================
	// Job Channel
	// =========================
	jobs := make(chan int)

	// =========================
	// Worker Pool
	// =========================
	var wg sync.WaitGroup
	wg.Add(maxGoroutines)

	for i := 0; i < maxGoroutines; i++ {
		go func(workerID int) {
			defer wg.Done()

			for jobID := range jobs {
				start := time.Now()

				file, err := os.Open(pcmPath)
				if err != nil {
					t.Errorf("[worker-%d] open failed: %v", workerID, err)
					continue
				}

				// 每个 worker 创建自己的 RNNoise 实例
				ns := gornnoise.NewRNNoise()

				buffer := make([]byte, frameSize)
				for {
					n, err := file.Read(buffer)
					if err == io.EOF || n < frameSize {
						break
					}
					if err != nil {
						t.Errorf("[worker-%d] read error: %v", workerID, err)
						break
					}

					// RNNoise 需要 48kHz，先重采样
					payload48k, err := codecs.ResamplePCM(buffer[:n], sampleRate, 48000)
					if err != nil {
						t.Errorf("[worker-%d] resample error: %v", workerID, err)
						break
					}

					// 处理音频
					var output []byte
					fsize := gornnoise.GetFrameSize() * 2
					for i := 0; i < len(payload48k); i += fsize {
						end := i + fsize
						if end > len(payload48k) {
							end = len(payload48k)
						}
						buf := payload48k[i:end]
						buf = ns.Process(buf)
						output = append(output, buf...)
					}

					// 重采样回原始采样率
					_, err = codecs.ResamplePCM(output, 48000, sampleRate)
					if err != nil {
						t.Errorf("[worker-%d] resample back error: %v", workerID, err)
						break
					}
				}

				file.Close()

				elapsed := time.Since(start)

				// 打印关键观测点
				fmt.Printf(
					"[worker-%02d] job-%04d done | cost=%v | goroutines=%d\n",
					workerID,
					jobID,
					elapsed,
					runtime.NumGoroutine(),
				)
			}
		}(i)
	}

	// =========================
	// 定时投喂任务（20ms）
	// =========================
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	startAll := time.Now()

	for i := 0; i < totalJobs; i++ {
		<-ticker.C
		jobs <- i
	}

	close(jobs)
	wg.Wait()

	totalCost := time.Since(startAll)

	fmt.Println("=================================")
	fmt.Println("ALL JOBS DONE")
	fmt.Println("total jobs :", totalJobs)
	fmt.Println("total cost :", totalCost)
	fmt.Println("avg / job  :", totalCost/time.Duration(totalJobs))
	fmt.Println("goroutines :", runtime.NumGoroutine())
	fmt.Println("=================================")

	fmt.Println(">>> GC done, block for inspection")
	select {}
}

// TestNoiseSuppression_Online 在线测试降噪算法（需要 PortAudio）
// pre: brew install portaudio
//func TestNoiseSuppression_Online(t *testing.T) {
//	const (
//		sampleRate    = 16000
//		frameMs       = 20
//		channels      = 1
//		bitsPerSample = 16
//	)
//
//	samplesPerFrame := sampleRate * frameMs / 1000
//
//	// 初始化 PortAudio
//	err := portaudio.Initialize()
//	if err != nil {
//		log.Fatalf("Failed to initialize PortAudio: %v", err)
//	}
//	defer portaudio.Terminate()
//
//	// 分配采样缓冲区
//	buffer := make([]int16, samplesPerFrame)
//
//	// 打开输入流
//	stream, err := portaudio.OpenDefaultStream(channels, 0, float64(sampleRate), len(buffer), buffer)
//	if err != nil {
//		log.Fatalf("Failed to open stream: %v", err)
//	}
//	defer stream.Close()
//
//	// 使用 ONNX 降噪
//	config := DefaultONNXNoiseSuppressorConfig()
//	ns, err := NewONNXNoiseSuppressor(config)
//	if err != nil {
//		log.Fatalf("Failed to create ONNX noise suppressor: %v", err)
//	}
//	defer ns.Destroy()
//
//	// 创建输出 WAV 文件
//	outputFile, err := os.Create("../../testdata/online_ns_output.wav")
//	if err != nil {
//		log.Fatalf("Failed to create output file: %v", err)
//	}
//	defer outputFile.Close()
//
//	wavWriter := wav.NewWriter(outputFile, 0, 1, uint32(sampleRate), 16)
//	totalSamples := uint32(0)
//
//	err = stream.Start()
//	if err != nil {
//		log.Fatalf("Failed to start stream: %v", err)
//	}
//
//	fmt.Println("🎤 正在监听麦克风并降噪（按 Ctrl+C 退出）...")
//	for {
//		if err := stream.Read(); err != nil {
//			log.Printf("Read error: %v", err)
//			continue
//		}
//
//		// 转换为字节
//		byteData := int16SliceToBytes(buffer)
//
//		// 降噪处理
//		processed, err := ns.Process(byteData, sampleRate)
//		if err != nil {
//			log.Printf("Process error: %v", err)
//			continue
//		}
//
//		// 转换为 wav.Sample 并写入
//		numSamples := len(processed) / 2
//		samples := make([]wav.Sample, numSamples)
//		for i := 0; i < numSamples; i++ {
//			sampleValue := int16(processed[i*2]) | int16(processed[i*2+1])<<8
//			samples[i].Values[0] = int(sampleValue)
//		}
//
//		wavWriter.WriteSamples(samples)
//		totalSamples += uint32(numSamples)
//
//		fmt.Printf("[NoiseSuppression] Processed %d samples\n", numSamples)
//	}
//}

// TestNoiseSuppression_Compare 对比测试两种降噪算法
func TestNoiseSuppression_Compare(t *testing.T) {
	inputFile := "../../testdata/通话记录-4708-user-pcm.pcm"
	sampleRate := 16000
	frameSize := GetSampleSize(sampleRate, 16, 1) * 20 // 20ms

	file, err := os.Open(inputFile)
	if err != nil {
		t.Fatalf("Failed to open file: %v", err)
	}
	defer file.Close()

	// 创建输出目录
	outputDir := "../../testdata/ns_comparison"
	os.MkdirAll(outputDir, 0755)

	// 测试 ONNX
	fmt.Println("=== Testing ONNX Noise Suppressor ===")
	onnxConfig := dfn.DefaultSuppressorConfig()
	onnxNS, err := dfn.NewSuppressor(onnxConfig)
	if err != nil {
		t.Fatalf("Failed to create ONNX NS: %v", err)
	}
	defer onnxNS.Destroy()

	file.Seek(0, 0)
	onnxOutput, err := os.Create(filepath.Join(outputDir, "onnx_output.pcm"))
	if err != nil {
		t.Fatalf("Failed to create ONNX output: %v", err)
	}
	defer onnxOutput.Close()

	buffer := make([]byte, frameSize)
	onnxTotal := 0
	for {
		n, err := file.Read(buffer)
		if err == io.EOF || n < frameSize {
			break
		}

		output, err := onnxNS.Process(buffer[:n], sampleRate)
		if err != nil {
			t.Errorf("ONNX process error: %v", err)
			continue
		}

		onnxOutput.Write(output)
		onnxTotal += len(output)
	}
	fmt.Printf("ONNX: Processed %d bytes\n", onnxTotal)

	// 测试 RNNoise
	fmt.Println("=== Testing RNNoise ===")
	rnnoiseNS := gornnoise.NewRNNoise()

	file.Seek(0, 0)
	rnnoiseOutput, err := os.Create(filepath.Join(outputDir, "rnnoise_output.pcm"))
	if err != nil {
		t.Fatalf("Failed to create RNNoise output: %v", err)
	}
	defer rnnoiseOutput.Close()

	rnnoiseTotal := 0
	for {
		n, err := file.Read(buffer)
		if err == io.EOF || n < frameSize {
			break
		}

		// RNNoise 需要 48kHz
		payload48k, err := codecs.ResamplePCM(buffer[:n], sampleRate, 48000)
		if err != nil {
			t.Errorf("Resample error: %v", err)
			continue
		}

		var output []byte
		fsize := gornnoise.GetFrameSize() * 2
		for i := 0; i < len(payload48k); i += fsize {
			end := i + fsize
			if end > len(payload48k) {
				end = len(payload48k)
			}
			buf := payload48k[i:end]
			buf = rnnoiseNS.Process(buf)
			output = append(output, buf...)
		}

		output16k, err := codecs.ResamplePCM(output, 48000, sampleRate)
		if err != nil {
			t.Errorf("Resample back error: %v", err)
			continue
		}

		rnnoiseOutput.Write(output16k)
		rnnoiseTotal += len(output16k)
	}
	fmt.Printf("RNNoise: Processed %d bytes\n", rnnoiseTotal)

	fmt.Println("=== Comparison Complete ===")
	fmt.Printf("ONNX output: %s\n", filepath.Join(outputDir, "onnx_output.pcm"))
	fmt.Printf("RNNoise output: %s\n", filepath.Join(outputDir, "rnnoise_output.pcm"))
}
