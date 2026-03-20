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

	client, err := asr.NewClient(types.ClientConfig{
		Primary: types.ProviderConfig{
			Name:       "volcano",
			APIKey:     apiKey,
			AppID:      appID,
			ResourceID: resourceID,
			SampleRate: 16000,
			Format:     "pcm",
		},
		Session: types.DefaultSessionOptions(),
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer client.Close()

	session, err := client.NewSession(ctx, types.SessionOptions{
		SampleRate: 16000,
		Format:     "pcm",
		Language:   "zh-CN",
	})
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	defer session.Close()

	// 发送测试音频
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
		if err := session.Send(types.AudioFrame{
			Data: pcmData[i:end],
		}); err != nil {
			t.Fatalf("Send: %v", err)
		}

		if isLast {
			break
		}

		time.Sleep(50 * time.Millisecond)
	}

	// 接收识别结果
	var texts []string
	for {
		event, err := session.Recv()
		if err != nil {
			t.Logf("Recv error: %v", err)
			break
		}

		t.Logf("Event: type=%d text=%q isFinal=%v", event.Type, event.Text, event.IsFinal())

		if event.Text != "" {
			texts = append(texts, event.Text)
		}

		if event.Type == types.EventFinal || event.Type == types.EventError {
			break
		}
	}

	if len(texts) == 0 {
		t.Error("No text recognized")
	} else {
		t.Logf("Recognized: %v", texts)
	}
}
