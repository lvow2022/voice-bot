package server

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/gorilla/websocket"

	asrtypes "voicebot/pkg/asr/types"
	"voicebot/pkg/agent"
	ttstypes "voicebot/pkg/tts/types"
	"voicebot/pkg/voicechain"
)

// Server WebSocket 服务
type Server struct {
	config     *ServerConfig
	registry   *agent.AgentRegistry
	sessionMgr *SessionManager
	upgrader   websocket.Upgrader
	httpServer *http.Server
}

// New 创建 WebSocket 服务
func New(registry *agent.AgentRegistry, config *ServerConfig) *Server {
	if config == nil {
		config = DefaultServerConfig()
	}
	return &Server{
		config:     config,
		registry:   registry,
		sessionMgr: NewSessionManager(config.TokenTTL),
		upgrader: websocket.Upgrader{
			ReadBufferSize:  4096,
			WriteBufferSize: 4096,
			CheckOrigin: func(r *http.Request) bool {
				return true // 允许所有来源，生产环境应限制
			},
		},
	}
}

// Start 启动服务
func (s *Server) Start() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/session", s.handleCreateSession)
	mux.HandleFunc("/ws", s.handleWebSocket)
	mux.HandleFunc("/health", s.handleHealth)

	s.httpServer = &http.Server{
		Addr:    s.config.Addr,
		Handler: mux,
	}

	slog.Info("websocket server starting", "addr", s.config.Addr)
	return s.httpServer.ListenAndServe()
}

// Shutdown 优雅关闭
func (s *Server) Shutdown(ctx context.Context) error {
	if s.httpServer != nil {
		return s.httpServer.Shutdown(ctx)
	}
	return nil
}

// handleCreateSession 处理 POST /session
func (s *Server) handleCreateSession(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "only POST allowed")
		return
	}

	var req InitRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid_request", "invalid JSON body")
		return
	}

	// 验证 agent
	agentID := req.Agent
	if agentID == "" {
		agentID = "main"
	}
	if _, ok := s.registry.GetAgent(agentID); !ok {
		s.writeError(w, http.StatusBadRequest, "agent_not_found", "agent '"+req.Agent+"' not found")
		return
	}

	// 设置默认值
	asrOpts := req.ASR
	if asrOpts.SampleRate == 0 {
		asrOpts = asrtypes.DefaultSessionOptions()
	}
	ttsOpts := req.TTS
	if ttsOpts.SampleRate == 0 {
		ttsOpts = ttstypes.DefaultSessionOptions()
	}

	// 创建 token
	token := generateToken()
	ps := s.sessionMgr.Create(agentID, asrOpts, ttsOpts)
	s.sessionMgr.Store(token, ps)

	resp := InitResponse{
		Token:     token,
		ExpiresAt: ps.ExpiresAt.Unix(),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// handleWebSocket 处理 GET /ws?token=xxx
func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if token == "" {
		s.writeError(w, http.StatusBadRequest, "missing_token", "token parameter required")
		return
	}

	// 消费 token
	ps, err := s.sessionMgr.Consume(token)
	if err != nil {
		switch err {
		case ErrInvalidToken:
			s.writeError(w, http.StatusUnauthorized, "invalid_token", "token is invalid or expired")
		case ErrTokenUsed:
			s.writeError(w, http.StatusConflict, "token_used", "token already used")
		default:
			s.writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		}
		return
	}

	// 升级 WebSocket
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.Error("websocket upgrade failed", "error", err)
		return
	}

	// 获取 agent
	agentInstance, ok := s.registry.GetAgent(ps.AgentID)
	if !ok {
		slog.Error("agent not found after token validation", "agentID", ps.AgentID)
		conn.Close()
		return
	}
	_ = agentInstance // TODO: 用于绑定 pipeline

	// 创建 transport
	transport := NewWSTransport(conn, ps.ASR.SampleRate)

	// 创建 voicechain session
	session := voicechain.NewSession().
		SetID(generateSessionID()).
		Context(r.Context()).
		Input(transport).
		Output(transport)

	// TODO: 绑定 pipeline (需要根据 agent 配置创建)

	// 发送就绪消息
	if err := transport.SendReady(); err != nil {
		slog.Error("failed to send ready message", "error", err)
		conn.Close()
		return
	}

	// 启动发送循环
	transport.StartSendLoop(session.GetContext())

	slog.Info("websocket session started",
		"sessionID", session.ID,
		"agentID", ps.AgentID)

	// 阻塞直到会话结束
	if err := session.Serve(); err != nil {
		slog.Debug("session ended", "sessionID", session.ID, "error", err)
	}

	transport.Close()
	slog.Info("websocket session ended", "sessionID", session.ID)
}

// handleHealth 健康检查
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// writeError 写入错误响应
func (s *Server) writeError(w http.ResponseWriter, statusCode int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(ErrorResponse{
		Error:   code,
		Message: message,
	})
}
