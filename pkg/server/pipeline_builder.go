package server

import (
	"context"
	"log/slog"

	"voicebot/pkg/agent"
	asrtypes "voicebot/pkg/asr/types"
	"voicebot/pkg/audio"
	"voicebot/pkg/device"
	"voicebot/pkg/pipeline"
	"voicebot/pkg/providers"
	"voicebot/pkg/speech"
	"voicebot/pkg/stream"
	"voicebot/pkg/tts"
	ttstypes "voicebot/pkg/tts/types"
	"voicebot/pkg/voicechain"
)

// PipelineBuilder 构建 voicechain pipeline
type PipelineBuilder struct {
	llm *LLMConfig
	asr *asrtypes.ProviderConfig
	tts *ttstypes.ProviderConfig
	vad *VADConfig
}

// NewPipelineBuilder 创建 PipelineBuilder
func NewPipelineBuilder(llm *LLMConfig, asr *asrtypes.ProviderConfig, tts *ttstypes.ProviderConfig, vad *VADConfig) *PipelineBuilder {
	return &PipelineBuilder{
		llm: llm,
		asr: asr,
		tts: tts,
		vad: vad,
	}
}

// transportOutput 适配 voicechain.Transport 到 stream.Output 接口
type transportOutput struct {
	transport voicechain.Transport
}

func (t *transportOutput) Write(data []byte) error {
	_, err := t.transport.Send(context.Background(), &voicechain.AudioFrame{
		Payload: data,
	})
	return err
}

// Build 构建完整的 voicechain pipeline
// agentLoop: Per-session AgentLoop (caller should create one per voice connection)
// connectionID: Unique identifier for this voice connection (used as session key prefix)
func (b *PipelineBuilder) Build(
	ctx context.Context,
	agentLoop *agent.AgentLoop,
	connectionID string,
	outputTransport voicechain.Transport,
) ([]voicechain.HandleFunc, error) {

	// 1. 创建 TTS 客户端和会话
	ttsClientCfg := ttstypes.ClientConfig{
		Primary: *b.tts,
		Session: ttstypes.DefaultSessionOptions(),
	}
	ttsClient, err := tts.NewClient(ttsClientCfg)
	if err != nil {
		return nil, err
	}

	ttsSession, err := ttsClient.NewSession(ctx)
	if err != nil {
		ttsClient.Close()
		return nil, err
	}

	// 2. 创建音频播放器
	output := &transportOutput{transport: outputTransport}
	player, err := stream.NewPlayer(output, device.DeviceConfig{
		SampleRate: 16000, // 使用默认采样率
		Channels:   1,
		PeriodMs:   20,
	})
	if err != nil {
		ttsSession.Close()
		ttsClient.Close()
		return nil, err
	}

	// 3. 创建 Scheduler 配置
	speechConfig := speech.DefaultConfig

	// 4. 构建 AudioProcessPipeline
	audioProcessHandler := pipeline.NewAudioProcessPipeline(pipeline.AudioProcessOptions{
		ASR:       *b.asr,
		Session:   asrtypes.DefaultSessionOptions(),
		VADType:   audio.VADType(b.vad.Type),
		VADOption: b.vad.Option,
	})

	// 5. 构建 AgentPipeline (使用 AgentLoop)
	sessionKey := "voice:" + connectionID
	agentHandler := pipeline.NewAgentPipeline(pipeline.AgentPipelineOptions{
		AgentLoop:    agentLoop,
		SessionKey:   sessionKey,
		TTSSession:   ttsSession,
		StreamPlayer: player,
		SpeechConfig: speechConfig,
	})

	slog.Debug("pipeline built",
		"connectionID", connectionID,
		"sessionKey", sessionKey,
		"asrProvider", b.asr.Name,
		"ttsProvider", b.tts.Name,
		"vadType", b.vad.Type)

	return []voicechain.HandleFunc{
		audioProcessHandler,
		agentHandler,
	}, nil
}

// CreateLLMProvider 创建 LLM Provider
func (b *PipelineBuilder) CreateLLMProvider() providers.LLMProvider {
	return providers.NewHTTPProvider(b.llm.APIKey, b.llm.APIBase, b.llm.Proxy)
}
