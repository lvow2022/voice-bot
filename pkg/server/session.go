package server

import (
	"sync"
	"time"

	asrtypes "voicebot/pkg/asr/types"
	ttstypes "voicebot/pkg/tts/types"
)

// PendingSession 待连接的会话
type PendingSession struct {
	AgentID   string
	ASR       asrtypes.SessionOptions
	TTS       ttstypes.SessionOptions
	CreatedAt time.Time
	ExpiresAt time.Time
	Used      bool
}

// SessionManager 管理待连接的会话
type SessionManager struct {
	pending sync.Map // token -> *PendingSession
	ttl     time.Duration
}

// NewSessionManager 创建会话管理器
func NewSessionManager(ttl time.Duration) *SessionManager {
	if ttl == 0 {
		ttl = 5 * time.Minute
	}
	sm := &SessionManager{
		ttl: ttl,
	}
	go sm.cleanup()
	return sm
}

// Create 创建新的待连接会话
func (sm *SessionManager) Create(agentID string, asr asrtypes.SessionOptions, tts ttstypes.SessionOptions) *PendingSession {
	now := time.Now()
	return &PendingSession{
		AgentID:   agentID,
		ASR:       asr,
		TTS:       tts,
		CreatedAt: now,
		ExpiresAt: now.Add(sm.ttl),
		Used:      false,
	}
}

// Store 存储待连接会话
func (sm *SessionManager) Store(token string, session *PendingSession) {
	sm.pending.Store(token, session)
}

// Consume 消费 token，返回会话配置并标记为已使用
func (sm *SessionManager) Consume(token string) (*PendingSession, error) {
	val, ok := sm.pending.Load(token)
	if !ok {
		return nil, ErrInvalidToken
	}

	ps := val.(*PendingSession)

	if ps.Used {
		return nil, ErrTokenUsed
	}

	if time.Now().After(ps.ExpiresAt) {
		sm.pending.Delete(token)
		return nil, ErrInvalidToken
	}

	ps.Used = true
	sm.pending.Delete(token)
	return ps, nil
}

// cleanup 定期清理过期会话
func (sm *SessionManager) cleanup() {
	ticker := time.NewTicker(1 * time.Minute)
	for range ticker.C {
		now := time.Now()
		sm.pending.Range(func(key, value any) bool {
			ps := value.(*PendingSession)
			if now.After(ps.ExpiresAt) {
				sm.pending.Delete(key)
			}
			return true
		})
	}
}
