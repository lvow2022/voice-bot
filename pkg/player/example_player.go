package player

import (
	"fmt"
	"time"

	"voicebot/pkg/stream"
)

// ============ Player 使用示例 ============

// ExamplePlayer_Basic 基础使用示例
func ExamplePlayer_Basic() {
	// 创建播放器（16kHz, 单声道）
	player, err := NewPlayer(16000, 1)
	if err != nil {
		fmt.Printf("Failed to create player: %v\n", err)
		return
	}

	// 启动播放器（后台运行）
	go player.Run()

	// 创建并配置音频流
	audioStream := stream.NewAudioStream(64 * 1024)
	audioStream.PushFilter(
		stream.NewLogFilter("tts"),
		stream.NewResampleFilter(48000, 16000),
		stream.NewVolumeFilter(0.8),
	)

	// 推送数据
	go func() {
		for i := 0; i < 10; i++ {
			data := make([]byte, 3200) // 100ms @ 16kHz
			audioStream.Push(data, false)
			time.Sleep(100 * time.Millisecond)
		}
		audioStream.Push(nil, true) // EOF
	}()

	// 开始播放
	player.Start(audioStream)

	// 等待播放完成
	time.Sleep(2 * time.Second)

	// 停止播放器
	player.Stop()
}

// ExamplePlayer_Queue 队列播放示例
func ExamplePlayer_Queue() {
	player, _ := NewPlayer(16000, 1)
	go player.Run()

	// 创建多个流（排队播放）
	for i := 1; i <= 3; i++ {
		audioStream := stream.NewAudioStream(64 * 1024)
		audioStream.PushFilter(stream.NewLogFilter(fmt.Sprintf("stream%d", i)))

		// 推送数据
		go func(idx int, s *stream.AudioStream) {
			for j := 0; j < 5; j++ {
				s.Push(make([]byte, 3200), false)
				time.Sleep(100 * time.Millisecond)
			}
			s.Push(nil, true)
		}(i, audioStream)

		// 添加到队列（会排队播放）
		player.Start(audioStream)
		fmt.Printf("[Main] Added stream %d to queue (queue length: %d)\n",
			i, player.QueueLength())
	}

	// 观察队列状态
	time.Sleep(500 * time.Millisecond)
	fmt.Printf("[Main] Current state: %s, Queue: %d\n",
		player.State(), player.QueueLength())

	time.Sleep(3 * time.Second)
	player.Stop()
}

// ExamplePlayer_Control 播放控制示例
func ExamplePlayer_Control() {
	player, _ := NewPlayer(16000, 1)
	go player.Run()

	audioStream := stream.NewAudioStream(64 * 1024)
	audioStream.PushFilter(stream.NewResampleFilter(48000, 16000))

	// 推送较长的数据
	go func() {
		for i := 0; i < 50; i++ {
			audioStream.Push(make([]byte, 3200), false)
			time.Sleep(100 * time.Millisecond)
		}
		audioStream.Push(nil, true)
	}()

	player.Start(audioStream)

	// 演示播放控制
	time.Sleep(1 * time.Second)
	fmt.Println("[Main] Pausing...")
	player.Pause()

	time.Sleep(1 * time.Second)
	fmt.Println("[Main] Resuming...")
	player.Resume()

	time.Sleep(1 * time.Second)
	fmt.Println("[Main] Skipping...")
	player.Next() // 跳过当前

	time.Sleep(2 * time.Second)
	player.Stop()
}

// ExamplePlayer_TTSScenario TTS 实际场景
func ExamplePlayer_TTSScenario() {
	// TTS 引擎 → Player → 扬声器

	player, _ := NewPlayer(16000, 1)

	go player.Run()

	// TTS 生成文本音频
	ttsStreams := []string{"Hello", "World", "How", "Are", "You"}

	for _, text := range ttsStreams {
		// 创建 TTS 流（48kHz → 16kHz）
		audioStream := stream.NewAudioStream(64 * 1024)

		audioStream.PushFilter(
			stream.NewResampleFilter(48000, 16000),
			stream.NewVolumeFilter(0.9),
		)

		// 创建可播放流并设置命令回调（AOP）
		playable := NewPlayable(audioStream)
		playable.OnPlay(func() {
			fmt.Printf("[Stream:%s] Started playing\n", text)
		})
		playable.OnPause(func() {
			fmt.Printf("[Stream:%s] Paused\n", text)
		})
		playable.OnResume(func() {
			fmt.Printf("[Stream:%s] Resumed\n", text)
		})
		playable.OnStop(func() {
			fmt.Printf("[Stream:%s] Stopped\n", text)
		})

		// 模拟 TTS 生成数据
		go func(s *stream.AudioStream, txt string) {
			fmt.Printf("[TTS] Generating: %s\n", txt)
			for i := 0; i < 10; i++ {
				data := make([]byte, 9600) // 100ms @ 48kHz
				s.Push(data, false)
				time.Sleep(100 * time.Millisecond)
			}
			s.Push(nil, true)
			fmt.Printf("[TTS] Finished: %s\n", txt)
		}(audioStream, text)

		// 添加到播放队列
		player.StartPlayable(playable)
		fmt.Printf("[Main] Added to queue: %s (queue: %d)\n",
			text, player.QueueLength())
	}

	// 等待所有播放完成
	for player.QueueLength() > 0 || player.IsPlaying() {
		fmt.Printf("[Main] State: %s, Queue: %d\n",
			player.State(), player.QueueLength())
		time.Sleep(500 * time.Millisecond)
	}

	player.Stop()
	fmt.Println("[Main] All finished")
}

// ExamplePlayer_RealtimeControl 实时控制示例
func ExamplePlayer_RealtimeControl() {
	player, _ := NewPlayer(16000, 1)
	go player.Run()

	audioStream := stream.NewAudioStream(64 * 1024)
	go func() {
		for i := 0; i < 100; i++ {
			audioStream.Push(make([]byte, 3200), false)
			time.Sleep(100 * time.Millisecond)
		}
		audioStream.Push(nil, true)
	}()

	player.Start(audioStream)

	// 模拟用户交互
	commands := []struct {
		delay time.Duration
		cmd   string
	}{
		{2 * time.Second, "pause"},
		{2 * time.Second, "resume"},
		{2 * time.Second, "pause"},
		{2 * time.Second, "resume"},
		{2 * time.Second, "stop"},
	}

	for _, c := range commands {
		time.Sleep(c.delay)
		fmt.Printf("[Main] Sending command: %s\n", c.cmd)
		switch c.cmd {
		case "pause":
			player.Pause()
		case "resume":
			player.Resume()
		case "stop":
			player.Stop()
		}
		fmt.Printf("[Main] State: %s\n", player.State())
	}
}
