package server

import "errors"

var (
	// ErrInvalidToken token 无效或已过期
	ErrInvalidToken = errors.New("invalid or expired token")
	// ErrTokenUsed token 已被使用
	ErrTokenUsed = errors.New("token already used")
	// ErrAgentNotFound agent 不存在
	ErrAgentNotFound = errors.New("agent not found")
)

// ErrorResponse HTTP 错误响应
type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}
