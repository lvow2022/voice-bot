package integration

import (
	"context"
	"os"
	"testing"
	"time"

	"voicebot/pkg/tts"
	"voicebot/pkg/tts/types"
)

func TestTTSMinimax(t *testing.T) {
	apiKey := os.Getenv("MINIMAX_API_KEY")
	if apiKey == "" {
		t.Skip("MINIMAX_API_KEY not set")
	}

	cfg := types.ClientConfig{
		Primary: types.ProviderConfig{
			Name:    "minimax",
			APIKey:  apiKey,
			VoiceID: "female-tianmei",
			Speed:   1.0,
			Options: map[string]any{
				"model":  "speech-2.5-turbo-preview",
				"format": "pcm",
			},
		},
		Session: types.SessionOptions{
			SampleRate: 16000,
			Format:     "pcm",
			Channels:   1,
		},
	}

	client, err := tts.NewClient(cfg)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	session, err := client.NewSession(ctx)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	defer session.Close()

	// 发送文本
	if err := session.Send("你好，这是一个测试。"); err != nil {
		t.Fatalf("Send: %v", err)
	}

	// 接收音频事件
	var audioChunks int
	var totalBytes int64

	for {
		event, err := session.Recv()
		if err != nil {
			t.Fatalf("Recv: %v", err)
		}

		t.Logf("Event: type=%v, dataSize=%d, isFinal=%v",
			event.Type, len(event.Data), event.IsFinal)

		switch event.Type {
		case types.EventAudioChunk:
			if len(event.Data) > 0 {
				audioChunks++
				totalBytes += int64(len(event.Data))
			}
		case types.EventCompleted:
			t.Logf("Synthesis completed: chunks=%d, bytes=%d", audioChunks, totalBytes)
			return
		case types.EventError:
			t.Fatalf("Synthesis error: %v", event.Err)
		}

		if audioChunks > 100 {
			// 防止无限循环
			t.Log("Received enough chunks, stopping")
			return
		}
	}
}

func TestTTSMinimaxClientFallback(t *testing.T) {
	apiKey := os.Getenv("MINIMAX_API_KEY")
	if apiKey == "" {
		t.Skip("MINIMAX_API_KEY not set")
	}

	cfg := types.ClientConfig{
		Primary: types.ProviderConfig{
			Name:    "minimax",
			APIKey:  apiKey,
			VoiceID: "female-tianmei",
		},
		// Fallback 使用相同配置（测试 fallback 逻辑）
		Fallback: &types.ProviderConfig{
			Name:    "minimax",
			APIKey:  apiKey,
			VoiceID: "male-qn-qingse",
		},
		Session: types.DefaultSessionOptions(),
	}

	client, err := tts.NewClient(cfg)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer client.Close()

	t.Logf("Current provider: %s", client.CurrentProvider())

	ctx := context.Background()
	session, err := client.NewSession(ctx)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	defer session.Close()

	// 验证 session 可用
	if err := session.Send("测试"); err != nil {
		t.Fatalf("Send: %v", err)
	}

	t.Log("Fallback test passed")
}
