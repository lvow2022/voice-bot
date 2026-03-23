package server

import (
	"crypto/rand"
	"encoding/hex"
	"time"
)

// randomString 生成随机字符串
func randomString(n int) string {
	bytes := make([]byte, (n+1)/2)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)[:n]
}

// generateToken 生成会话 token
func generateToken() string {
	return "sess_" + randomString(16)
}

// generateSessionID 生成会话 ID
func generateSessionID() string {
	return "ws-" + time.Now().Format("20060102150405") + "-" + randomString(8)
}
