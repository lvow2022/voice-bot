package stream

import (
	"fmt"
)

// ============ Stream AOP 使用示例 ============

// ExampleStreamAOP_Basic 基础 AOP 使用示例
func ExampleStreamAOP_Basic() {
	// 创建流
	stream := NewTtsStream(64 * 1024)

	// 添加处理器（Gin 风格）
	stream.Use(
		NewLoggerProcessor("input"),               // 日志记录
		NewSimpleResamplerProcessor(48000, 16000), // 48k -> 16k
		NewSimpleVolumeProcessor(0.8),             // 音量 80%
	)

	// Push 数据（自动通过处理器链）
	ttsData48k := make([]byte, 9600) // 模拟 48kHz 数据
	stream.Push(ttsData48k, false)
	stream.Push(make([]byte, 9600), false)
	stream.Push(nil, true) // EOF

	// Pull 数据（已经是处理后的 16kHz 数据）
	for {
		buf := make([]byte, 3200) // 16kHz buffer
		n, err := stream.Pull(buf)
		if err == ErrStreamEnded {
			fmt.Println("Stream ended")
			break
		}
		if n > 0 {
			fmt.Printf("Pulled %d bytes (16kHz)\n", n)
		}
	}
}

// ExampleStreamAOP_ProcessorChain 使用处理器链
func ExampleStreamAOP_ProcessorChain() {
	stream := NewTtsStream(64 * 1024)

	// 创建处理器链
	chain := NewStreamProcessorChain().
		Add(NewLoggerProcessor("step1")).
		Add(NewSimpleResamplerProcessor(48000, 16000)).
		Add(NewSimpleVolumeProcessor(0.8)).
		Add(NewLoggerProcessor("final"))

	// 添加整个链
	stream.Use(chain)

	// 使用...
}

// ExampleStreamAOP_CustomProcessor 自定义处理器
func ExampleStreamAOP_CustomProcessor() {
	stream := NewTtsStream(64 * 1024)

	// 使用函数式处理器
	stream.Use(
		ProcessorFunc(func(data []byte) ([]byte, error) {
			// 自定义逻辑：检测静音
			if len(data) == 0 {
				return data, nil
			}
			fmt.Printf("[Custom] Processing %d bytes\n", len(data))
			return data, nil
		}),
	)

	// 使用...
}

// ExampleStreamAOP_DynamicVolume 动态调整音量
func ExampleStreamAOP_DynamicVolume() {
	stream := NewTtsStream(64 * 1024)

	// 创建可控制的音量处理器
	volume := NewSimpleVolumeProcessor(1.0)
	stream.Use(volume)

	// 推送数据
	stream.Push(make([]byte, 3200), false)

	// 动态调整音量
	volume.SetVolume(0.5) // 降低到 50%
	stream.Push(make([]byte, 3200), false)

	volume.SetVolume(0.8) // 提高到 80%
	stream.Push(make([]byte, 3200), false)

	stream.Push(nil, true)
}

// ExampleStreamAOP_TTSScenario TTS 实际场景
func ExampleStreamAOP_TTSScenario() {
	// TTS 引擎产生 48kHz 数据
	// 播放器需要 16kHz 数据

	stream := NewTtsStream(64 * 1024)

	// 配置处理管道：48k -> 16k -> 音量调整
	stream.Use(
		NewSimpleResamplerProcessor(48000, 16000),
		NewSimpleVolumeProcessor(0.9),
	)

	// TTS 引程推送数据
	go func() {
		// 模拟 TTS 生成 48kHz 数据
		for i := 0; i < 10; i++ {
			ttsChunk := make([]byte, 9600) // 100ms @ 48kHz
			stream.Push(ttsChunk, false)
		}
		stream.Push(nil, true) // TTS 完成
	}()

	// 播放器读取 16kHz 数据
	for {
		buf := make([]byte, 3200) // 100ms @ 16kHz
		n, err := stream.Pull(buf)
		if err == ErrStreamEnded {
			fmt.Println("Playback finished")
			break
		}
		if n > 0 {
			// player.Play(buf[:n])
			fmt.Printf("Playing %d bytes\n", n)
		}
	}
}
