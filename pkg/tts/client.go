package tts

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"

	"voicebot/pkg/tts/types"
)

// Errors
var (
	ErrClientClosed = errors.New("tts client is closed")
)

// TtsClient TTS 客户端，负责创建 session 和 provider fallback
type TtsClient struct {
	config types.ClientConfig

	mu        sync.RWMutex
	session   *TtsSession
	closeOnce sync.Once
	closed    atomic.Bool

	// 当前使用的 provider
	currentProvider string

	// metrics
	totalSessions int64
	fallbackCount int64
}

// NewClient 创建 TTS 客户端
func NewClient(config types.ClientConfig) (*TtsClient, error) {
	return &TtsClient{
		config:          config,
		currentProvider: config.Primary.Name,
	}, nil
}

// NewSession 创建新的 TTS 会话
func (c *TtsClient) NewSession(ctx context.Context) (*TtsSession, error) {
	c.mu.Lock()
	if c.isClosed() {
		c.mu.Unlock()
		return nil, ErrClientClosed
	}

	if c.session != nil {
		_ = c.session.Close()
		c.session = nil
	}
	c.mu.Unlock()

	session, err := c.createSessionWithFallback(ctx)
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
func (c *TtsClient) createSessionWithFallback(ctx context.Context) (*TtsSession, error) {
	providers := c.getProviders()

	var lastErr error
	for i, providerCfg := range providers {
		if i > 0 {
			atomic.AddInt64(&c.fallbackCount, 1)
			slog.Warn("tts switching to fallback provider", "provider", providerCfg.Name)
		}

		c.mu.Lock()
		c.currentProvider = providerCfg.Name
		c.mu.Unlock()

		provider, err := CreateProvider(providerCfg)
		if err != nil {
			lastErr = err
			continue
		}

		session, err := NewTtsSession(ctx, provider, c.config.Session)
		if err != nil {
			_ = provider.Close()
			lastErr = err
			slog.Error("tts provider failed", "provider", providerCfg.Name, "error", err)
			continue
		}

		return session, nil
	}

	return nil, fmt.Errorf("all tts providers failed: %w", lastErr)
}

// getProviders 获取 provider 列表
func (c *TtsClient) getProviders() []types.ProviderConfig {
	providers := []types.ProviderConfig{c.config.Primary}
	if c.config.Fallback != nil {
		providers = append(providers, *c.config.Fallback)
	}
	return providers
}

// CurrentProvider 返回当前使用的 provider
func (c *TtsClient) CurrentProvider() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.currentProvider
}

// Metrics 返回客户端指标
func (c *TtsClient) Metrics() map[string]int64 {
	return map[string]int64{
		"total_sessions": atomic.LoadInt64(&c.totalSessions),
		"fallback_count": atomic.LoadInt64(&c.fallbackCount),
	}
}

// Close 关闭客户端
func (c *TtsClient) Close() error {
	c.closeOnce.Do(func() {
		c.closed.Store(true)
		c.mu.Lock()
		defer c.mu.Unlock()

		if c.session != nil {
			_ = c.session.Close()
		}
	})
	return nil
}

// isClosed 检查客户端是否已关闭
func (c *TtsClient) isClosed() bool {
	return c.closed.Load()
}
