package server

import "time"

// ServerConfig WebSocket 服务配置
type ServerConfig struct {
	Addr         string        // 监听地址，如 ":8080"
	ReadTimeout  time.Duration // WebSocket 读超时
	WriteTimeout time.Duration // WebSocket 写超时
	TokenTTL     time.Duration // Token 有效期，默认 5 分钟
}

// DefaultServerConfig 返回默认配置
func DefaultServerConfig() *ServerConfig {
	return &ServerConfig{
		Addr:         ":8080",
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		TokenTTL:     5 * time.Minute,
	}
}
