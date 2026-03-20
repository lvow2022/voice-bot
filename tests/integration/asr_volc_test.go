package integration

import (
	"context"
	"os"
	"testing"
	"time"

	"voicebot/pkg/asr"
	"voicebot/pkg/asr/types"
)

func TestASRVolcano(t *testing.T) {
	apiKey := os.Getenv("VOLCANO_ASR_API_KEY")
	appID := os.Getenv("VOLCANO_ASR_APP_ID")
	resourceID := os.Getenv("VOLCANO_ASR_RESOURCE_ID")

	if apiKey == "" || appID == "" || resourceID == "" {
		t.Skip("VOLCANO_ASR_API_KEY, VOLCANO_ASR_APP_ID, VOLCANO_ASR_RESOURCE_ID not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 1. 创建 Provider
	provider, err := asr.CreateProvider(types.ProviderConfig{
		Name:       "volcano",
		APIKey:     apiKey,
		AppID:      appID,
		ResourceID: resourceID,
		SampleRate: 16000,
		Format:     "pcm",
	})
	if err != nil {
		t.Fatalf("CreateProvider: %v", err)
	}

	// 2. 连接获取 WSStream
	stream, err := provider.Connect(ctx, types.SessionOptions{
		SampleRate: 16000,
		Format:     "pcm",
		Language:   "zh-CN",
	})
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer stream.Close()

	// 3. 启动接收协程
	done := make(chan struct{})
	var texts []string
	go func() {
		defer close(done)
		for evt := range stream.Recv() {
			// 处理错误
			if err, ok := evt.(error); ok {
				t.Logf("Recv error: %v", err)
				return
			}

			// 类型断言
			asrEvt, ok := evt.(types.AsrEvent)
			if !ok {
				continue
			}

			t.Logf("Event: type=%s text=%q isFinal=%v", asrEvt.Type, asrEvt.Text, asrEvt.IsFinal())

			if asrEvt.Text != "" {
				texts = append(texts, asrEvt.Text)
			}

			if asrEvt.Type == types.EventFinal || asrEvt.Type == types.EventError {
				return
			}
		}
	}()

	// 4. 发送测试音频
	audioData, err := os.ReadFile("../../../testdata/s16_16k_1c_zh.wav")
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	// 跳过 wav header (44 bytes)
	pcmData := audioData[44:]

	// 分批发送
	chunkSize := 3200 // 100ms @ 16kHz 16bit mono
	for i := 0; i < len(pcmData); i += chunkSize {
		end := i + chunkSize
		if end > len(pcmData) {
			end = len(pcmData)
		}

		isLast := end >= len(pcmData)
		if err := stream.Send(ctx, types.AsrRequest{
			Audio:  pcmData[i:end],
			IsLast: isLast,
		}); err != nil {
			t.Fatalf("Send: %v", err)
		}

		if isLast {
			break
		}

		time.Sleep(50 * time.Millisecond)
	}

	// 5. 等待接收完成
	<-done

	if len(texts) == 0 {
		t.Error("No text recognized")
	} else {
		t.Logf("Recognized: %v", texts)
	}
}
