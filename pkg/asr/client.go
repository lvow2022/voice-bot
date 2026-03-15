package asr

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
	"voicebot/pkg/asr/types"
)

// AsrClient ASR 客户端，负责创建 session 和 provider fallback
type AsrClient struct {
	config types.ClientConfig

	mu      sync.RWMutex
	session *AsrSession
	closed  bool

	// 当前使用的 provider
	currentProvider string

	// metrics
	totalSessions int64
	fallbackCount int64
}

// NewClient 创建 ASR 客户端
func NewClient(config types.ClientConfig) (*AsrClient, error) {
	return &AsrClient{
		config:          config,
		currentProvider: config.Primary.Name,
	}, nil
}

// NewSession 创建新的 ASR 会话
func (c *AsrClient) NewSession(ctx context.Context, opts types.SessionOptions) (*AsrSession, error) {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil, ErrClientClosed
	}

	// 如果已有活跃 session，先关闭
	if c.session != nil {
		_ = c.session.Close()
		c.session = nil
	}
	c.mu.Unlock()

	// 合并配置
	mergedOpts := mergeSessionOptions(c.config.Session, opts)

	// 尝试创建 session（带 fallback）
	session, err := c.createSessionWithFallback(ctx, mergedOpts)
	if err != nil {
		return nil, err
	}

	c.mu.Lock()
	c.session = session
	c.totalSessions++
	c.mu.Unlock()

	return session, nil
}

// createSessionWithFallback 带 fallback 的 session 创建
func (c *AsrClient) createSessionWithFallback(ctx context.Context, opts types.SessionOptions) (*AsrSession, error) {
	providers := c.getProviders()

	var lastErr error
	for i, providerCfg := range providers {
		isFallback := i > 0
		if isFallback {
			atomic.AddInt64(&c.fallbackCount, 1)
			slog.Warn("switching to fallback provider", "provider", providerCfg.Name)
		}

		c.mu.Lock()
		c.currentProvider = providerCfg.Name
		c.mu.Unlock()

		// 创建 provider
		provider, err := CreateProvider(providerCfg)
		if err != nil {
			lastErr = err
			continue
		}

		// 创建 session
		session, err := NewASRSession(ctx, provider, opts)
		if err != nil {
			_ = provider.Close()
			lastErr = err
			slog.Error("provider connection failed",
				"provider", providerCfg.Name,
				"error", err,
			)
			continue
		}

		return session, nil
	}

	return nil, &ClientError{
		Code:    "ALL_PROVIDERS_FAILED",
		Message: "all providers failed: " + lastErr.Error(),
	}
}

// getProviders 获取 provider 列表（主 + 备用）
func (c *AsrClient) getProviders() []types.ProviderConfig {
	providers := []types.ProviderConfig{c.config.Primary}
	if c.config.Fallback != nil {
		providers = append(providers, *c.config.Fallback)
	}
	return providers
}

// CurrentProvider 返回当前使用的 provider
func (c *AsrClient) CurrentProvider() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.currentProvider
}

// Metrics 返回客户端指标
func (c *AsrClient) Metrics() map[string]int64 {
	return map[string]int64{
		"total_sessions": atomic.LoadInt64(&c.totalSessions),
		"fallback_count": atomic.LoadInt64(&c.fallbackCount),
	}
}

// Close 关闭客户端
func (c *AsrClient) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return nil
	}
	c.closed = true

	if c.session != nil {
		return c.session.Close()
	}
	return nil
}

// mergeSessionOptions 合并会话配置
func mergeSessionOptions(base, override types.SessionOptions) types.SessionOptions {
	result := base
	if override.SampleRate > 0 {
		result.SampleRate = override.SampleRate
	}
	if override.Format != "" {
		result.Format = override.Format
	}
	if override.Channels > 0 {
		result.Channels = override.Channels
	}
	result.EnableITN = override.EnableITN
	result.EnablePunc = override.EnablePunc
	if override.Language != "" {
		result.Language = override.Language
	}
	return result
}

// Errors
var (
	ErrClientClosed = &ClientError{Code: "CLIENT_CLOSED", Message: "client is closed"}
)

// ClientError 客户端错误
type ClientError struct {
	Code    string
	Message string
}

func (e *ClientError) Error() string {
	return e.Message
}
