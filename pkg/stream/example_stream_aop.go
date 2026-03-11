package stream

import (
	"fmt"
)

// ============ 过滤器链使用示例 ============

// ExampleFilterChain_Basic 基础使用示例
func ExampleFilterChain_Basic() {
	// 创建流
	stream := NewAudioStream(64 * 1024)

	// 添加过滤器到输入端（Push 时执行）
	stream.PushFilter(
		NewLogFilter("input"),        // 日志记录
		NewResampleFilter(48000, 16000), // 48k -> 16k
		NewVolumeFilter(0.8),         // 音量 80%
	)

	// Push 数据（自动通过过滤器链）
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

// ExampleFilterChain_Chain 使用过滤器链
func ExampleFilterChain_Chain() {
	stream := NewAudioStream(64 * 1024)

	// 创建过滤器链
	chain := NewFilterChain(
		NewLogFilter("step1"),
		NewResampleFilter(48000, 16000),
		NewVolumeFilter(0.8),
		NewLogFilter("final"),
	)

	// 添加整个链
	stream.PushFilter(chain)

	// 使用...
}

// ExampleFilterChain_Custom 自定义过滤器
func ExampleFilterChain_Custom() {
	stream := NewAudioStream(64 * 1024)

	// 使用函数式过滤器
	stream.PushFilter(
		FilterFunc(func(data []byte) ([]byte, error) {
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

// ExampleFilterChain_DynamicVolume 动态调整音量
func ExampleFilterChain_DynamicVolume() {
	stream := NewAudioStream(64 * 1024)

	// 创建可控制的音量过滤器
	volume := NewVolumeFilter(1.0)
	stream.PushFilter(volume)

	// 推送数据
	stream.Push(make([]byte, 3200), false)

	// 动态调整音量
	volume.SetVolume(0.5) // 降低到 50%
	stream.Push(make([]byte, 3200), false)

	volume.SetVolume(0.8) // 提高到 80%
	stream.Push(make([]byte, 3200), false)

	stream.Push(nil, true)
}

// ExampleFilterChain_Bidirectional 双向过滤器示例
func ExampleFilterChain_Bidirectional() {
	stream := NewAudioStream(64 * 1024)

	// 输入端：TTS 48kHz → 16kHz
	stream.PushFilter(
		NewResampleFilter(48000, 16000),
	)

	// 输出端：播放前调整音量
	stream.PullFilter(
		NewVolumeFilter(0.9),
		NewLogFilter("output"),
	)

	// 数据流：
	// Push(48k) → [ResampleFilter] → Buffer → [VolumeFilter → LogFilter] → Pull(16k)
}

// ExampleFilterChain_TTSScenario TTS 实际场景
func ExampleFilterChain_TTSScenario() {
	// TTS 引擎产生 48kHz 数据
	// 播放器需要 16kHz 数据

	stream := NewAudioStream(64 * 1024)

	// 配置处理管道：48k -> 16k -> 音量调整
	stream.PushFilter(
		NewResampleFilter(48000, 16000),
		NewVolumeFilter(0.9),
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
