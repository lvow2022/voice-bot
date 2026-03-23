package server

import (
	"context"
	"log/slog"

	"voicebot/pkg/agent"
	"voicebot/pkg/asr"
	asrtypes "voicebot/pkg/asr/types"
	"voicebot/pkg/audio"
	"voicebot/pkg/device"
	"voicebot/pkg/pipeline"
	"voicebot/pkg/speech"
	"voicebot/pkg/stream"
	"voicebot/pkg/tts"
	ttstypes "voicebot/pkg/tts/types"
	"voicebot/pkg/voicechain"
)

// PipelineBuilder 构建 voicechain pipeline
type PipelineBuilder struct {
	cfg *PipelineConfig
}

// NewPipelineBuilder 创建 PipelineBuilder
func NewPipelineBuilder(cfg *PipelineConfig) *PipelineBuilder {
	if cfg == nil {
		cfg = DefaultPipelineConfig()
	}
	return &PipelineBuilder{cfg: cfg}
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
func (b *PipelineBuilder) Build(
	ctx context.Context,
	agentInstance *agent.AgentInstance,
	asrOpts asrtypes.SessionOptions,
	ttsOpts ttstypes.SessionOptions,
	outputTransport voicechain.Transport,
) ([]voicechain.HandleFunc, error) {

	// 1. 创建 TTS 客户端和会话
	ttsClient, err := tts.NewClient(b.cfg.TTS)
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
		SampleRate: ttsOpts.SampleRate,
		Channels:   ttsOpts.Channels,
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
		ASR:       b.cfg.ASR,
		Session:   asrOpts,
		VADType:   audio.VADType(b.cfg.VADType),
		VADOption: b.cfg.VADOption,
	})

	// 5. 构建 AgentPipeline
	agentHandler := pipeline.NewAgentPipeline(pipeline.AgentPipelineOptions{
		AgentInstance: agentInstance,
		TTSSession:    ttsSession,
		StreamPlayer:  player,
		SpeechConfig:  speechConfig,
	})

	// 6. 构建 ConversationPipeline
	conversationHandler := pipeline.NewConversationPipeline().HandleFunc()

	slog.Debug("pipeline built",
		"agentID", agentInstance.ID,
		"asrProvider", b.cfg.ASR.Name,
		"ttsProvider", b.cfg.TTS.Primary.Name,
		"vadType", b.cfg.VADType)

	return []voicechain.HandleFunc{
		audioProcessHandler,
		agentHandler,
		conversationHandler,
	}, nil
}

// CreateASRProvider 创建 ASR Provider
func (b *PipelineBuilder) CreateASRProvider() (asrtypes.Provider, error) {
	return asr.CreateProvider(b.cfg.ASR)
}

// CreateTTSClient 创建 TTS 客户端
func (b *PipelineBuilder) CreateTTSClient() (*tts.TtsClient, error) {
	return tts.NewClient(b.cfg.TTS)
}

// CreateVAD 创建 VAD 检测器
func (b *PipelineBuilder) CreateVAD() (audio.VADDetector, error) {
	return audio.GetVAD(audio.VADType(b.cfg.VADType), b.cfg.VADOption)
}
