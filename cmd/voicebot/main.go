package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"voicebot/pkg/agent"
	"voicebot/pkg/config"
	asrtypes "voicebot/pkg/asr/types"
	"voicebot/pkg/audio"
	"voicebot/pkg/providers"
	ttstypes "voicebot/pkg/tts/types"
	"voicebot/pkg/server"
)

var (
	configPath = flag.String("config", "", "Path to config file (default: ~/.voicebot/config.json)")
	addr       = flag.String("addr", ":8080", "Server listen address")
)

func main() {
	flag.Parse()

	// 1. 加载配置
	cfg, err := loadConfig(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// 2. 创建 LLM Provider
	provider := createProvider()

	// 3. 创建 Agent Registry
	registry := agent.NewAgentRegistry(cfg, provider)

	// 4. 创建 Pipeline 配置
	pipelineCfg := buildPipelineConfig()

	// 5. 创建 Server
	srv := server.New(registry, &server.ServerConfig{
		Addr: *addr,
	}, pipelineCfg)

	// 6. 启动服务（在 goroutine 中）
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		slog.Info("Starting voicebot server", "addr", *addr)
		if err := srv.Start(); err != nil {
			log.Fatalf("Server error: %v", err)
		}
	}()

	// 7. 等待中断信号
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.Info("Shutting down server...")
	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("Server shutdown error: %v", err)
	}

	registry.Close()
	slog.Info("Server stopped")
}

func loadConfig(path string) (*config.Config, error) {
	if path == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("failed to get home directory: %w", err)
		}
		path = home + "/.voicebot/config.json"
	}

	cfg, err := config.LoadConfig(path)
	if err != nil {
		return nil, fmt.Errorf("failed to load config from %s: %w", path, err)
	}

	slog.Info("Config loaded", "path", path)
	return cfg, nil
}

func createProvider() providers.LLMProvider {
	// 从环境变量获取 API 配置
	apiKey := os.Getenv("LLM_API_KEY")
	if apiKey == "" {
		log.Fatal("LLM_API_KEY environment variable is required")
	}

	apiBase := os.Getenv("LLM_API_BASE")
	if apiBase == "" {
		apiBase = "https://api.openai.com/v1"
	}

	proxy := os.Getenv("HTTP_PROXY")
	if proxy == "" {
		proxy = os.Getenv("HTTPS_PROXY")
	}

	provider := providers.NewHTTPProvider(apiKey, apiBase, proxy)

	slog.Info("LLM provider created", "base", apiBase)

	return provider
}

func buildPipelineConfig() *server.PipelineConfig {
	// 从环境变量或默认值构建 Pipeline 配置
	asrProvider := os.Getenv("ASR_PROVIDER")
	if asrProvider == "" {
		asrProvider = "volcano"
	}

	ttsProvider := os.Getenv("TTS_PROVIDER")
	if ttsProvider == "" {
		ttsProvider = "minimax"
	}

	ttsAPIKey := os.Getenv("TTS_API_KEY")
	ttsVoiceID := os.Getenv("TTS_VOICE_ID")

	asrAPIKey := os.Getenv("ASR_API_KEY")
	asrAppID := os.Getenv("ASR_APP_ID")

	return &server.PipelineConfig{
		ASR: asrtypes.ProviderConfig{
			Name:       asrProvider,
			APIKey:     asrAPIKey,
			AppID:      asrAppID,
			SampleRate: 16000,
			Format:     "pcm",
		},
		TTS: ttstypes.ClientConfig{
			Primary: ttstypes.ProviderConfig{
				Name:    ttsProvider,
				APIKey:  ttsAPIKey,
				VoiceID: ttsVoiceID,
			},
			Session: ttstypes.DefaultSessionOptions(),
		},
		VADType:   string(audio.VADTypeSilero),
		VADOption: audio.DefaultVADDetectorOption(),
	}
}
