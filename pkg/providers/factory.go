package providers

import (
	"fmt"
	"strings"

	"voicebot/pkg/config"
)

const defaultAnthropicAPIBase = "https://api.anthropic.com/v1"

// ExtractProtocol extracts the protocol prefix and model identifier from a model string.
// If no prefix is specified, it defaults to "openai".
// Examples:
//   - "openai/gpt-4o" -> ("openai", "gpt-4o")
//   - "anthropic/claude-sonnet-4.6" -> ("anthropic", "claude-sonnet-4.6")
//   - "gpt-4o" -> ("openai", "gpt-4o")  // default protocol
func ExtractProtocol(model string) (protocol, modelID string) {
	model = strings.TrimSpace(model)
	protocol, modelID, found := strings.Cut(model, "/")
	if !found {
		return "openai", model
	}
	return protocol, modelID
}

// ProviderConfig contains the configuration needed to create a provider.
type ProviderConfig struct {
	APIKey   string
	APIBase  string
	Proxy    string
	Model    string
	Protocol string
}

// ResolveProviderConfig resolves provider configuration from the main config.
// Returns the provider configuration for the specified model.
func ResolveProviderConfig(cfg *config.Config, model string) (*ProviderConfig, error) {
	if model == "" {
		model = cfg.Agents.Defaults.GetModelName()
	}
	if model == "" {
		return nil, fmt.Errorf("no model configured")
	}

	protocol, modelID := ExtractProtocol(model)
	providerName := strings.ToLower(cfg.Agents.Defaults.Provider)

	// Try explicit provider configuration first
	var pCfg *ProviderConfig

	switch providerName {
	case "openai", "gpt", "":
		pCfg = resolveOpenAIConfig(cfg, modelID)
	case "anthropic", "claude":
		pCfg = resolveAnthropicConfig(cfg, modelID)
	case "deepseek":
		pCfg = resolveDeepSeekConfig(cfg, modelID)
	case "zhipu", "glm":
		pCfg = resolveZhipuConfig(cfg, modelID)
	case "ollama":
		pCfg = resolveOllamaConfig(cfg, modelID)
	case "vllm":
		pCfg = resolveVLLMConfig(cfg, modelID)
	default:
		// Try to infer from model name
		pCfg = inferProviderFromModel(cfg, model, modelID, protocol)
	}

	if pCfg == nil {
		return nil, fmt.Errorf("no provider configuration for model: %s", model)
	}

	pCfg.Protocol = protocol
	return pCfg, nil
}

func resolveOpenAIConfig(cfg *config.Config, modelID string) *ProviderConfig {
	if cfg.Providers.OpenAI.APIKey == "" {
		return nil
	}
	apiBase := cfg.Providers.OpenAI.APIBase
	if apiBase == "" {
		apiBase = "https://api.openai.com/v1"
	}
	return &ProviderConfig{
		APIKey:  cfg.Providers.OpenAI.APIKey,
		APIBase: apiBase,
		Proxy:   cfg.Providers.OpenAI.Proxy,
		Model:   modelID,
	}
}

func resolveAnthropicConfig(cfg *config.Config, modelID string) *ProviderConfig {
	if cfg.Providers.Anthropic.APIKey == "" {
		return nil
	}
	apiBase := cfg.Providers.Anthropic.APIBase
	if apiBase == "" {
		apiBase = defaultAnthropicAPIBase
	}
	return &ProviderConfig{
		APIKey:  cfg.Providers.Anthropic.APIKey,
		APIBase: apiBase,
		Proxy:   cfg.Providers.Anthropic.Proxy,
		Model:   modelID,
	}
}

func resolveDeepSeekConfig(cfg *config.Config, modelID string) *ProviderConfig {
	if cfg.Providers.DeepSeek.APIKey == "" {
		return nil
	}
	apiBase := cfg.Providers.DeepSeek.APIBase
	if apiBase == "" {
		apiBase = "https://api.deepseek.com/v1"
	}
	if modelID == "" {
		modelID = "deepseek-chat"
	}
	return &ProviderConfig{
		APIKey:  cfg.Providers.DeepSeek.APIKey,
		APIBase: apiBase,
		Proxy:   cfg.Providers.DeepSeek.Proxy,
		Model:   modelID,
	}
}

func resolveZhipuConfig(cfg *config.Config, modelID string) *ProviderConfig {
	if cfg.Providers.Zhipu.APIKey == "" {
		return nil
	}
	apiBase := cfg.Providers.Zhipu.APIBase
	if apiBase == "" {
		apiBase = "https://open.bigmodel.cn/api/paas/v4"
	}
	if modelID == "" {
		modelID = "glm-4"
	}
	return &ProviderConfig{
		APIKey:  cfg.Providers.Zhipu.APIKey,
		APIBase: apiBase,
		Proxy:   cfg.Providers.Zhipu.Proxy,
		Model:   modelID,
	}
}

func resolveOllamaConfig(cfg *config.Config, modelID string) *ProviderConfig {
	apiBase := cfg.Providers.Ollama.APIBase
	if apiBase == "" {
		apiBase = "http://localhost:11434/v1"
	}
	return &ProviderConfig{
		APIKey:  cfg.Providers.Ollama.APIKey,
		APIBase: apiBase,
		Proxy:   cfg.Providers.Ollama.Proxy,
		Model:   modelID,
	}
}

func resolveVLLMConfig(cfg *config.Config, modelID string) *ProviderConfig {
	if cfg.Providers.VLLM.APIBase == "" {
		return nil
	}
	return &ProviderConfig{
		APIKey:  cfg.Providers.VLLM.APIKey,
		APIBase: cfg.Providers.VLLM.APIBase,
		Proxy:   cfg.Providers.VLLM.Proxy,
		Model:   modelID,
	}
}

func inferProviderFromModel(cfg *config.Config, fullModel, modelID, protocol string) *ProviderConfig {
	lowerModel := strings.ToLower(fullModel)

	switch {
	case strings.Contains(lowerModel, "gpt") || strings.HasPrefix(fullModel, "openai/"):
		return resolveOpenAIConfig(cfg, modelID)
	case strings.Contains(lowerModel, "claude") || strings.HasPrefix(fullModel, "anthropic/"):
		return resolveAnthropicConfig(cfg, modelID)
	case strings.Contains(lowerModel, "deepseek"):
		return resolveDeepSeekConfig(cfg, modelID)
	case strings.Contains(lowerModel, "glm") || strings.Contains(lowerModel, "zhipu"):
		return resolveZhipuConfig(cfg, modelID)
	case strings.HasPrefix(fullModel, "ollama/"):
		return resolveOllamaConfig(cfg, modelID)
	case strings.Contains(lowerModel, "ollama"):
		return resolveOllamaConfig(cfg, modelID)
	default:
		// Default to OpenAI-compatible
		if cfg.Providers.OpenAI.APIKey != "" {
			return resolveOpenAIConfig(cfg, modelID)
		}
	}

	return nil
}
