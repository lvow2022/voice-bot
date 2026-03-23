// PicoClaw - Ultra-lightweight personal AI agent
// Inspired by and based on nanobot: https://github.com/HKUDS/nanobot
// License: MIT
//
// Copyright (c) 2026 PicoClaw contributors

package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"voicebot/pkg/commands"
	"voicebot/pkg/config"
	"voicebot/pkg/constants"
	"voicebot/pkg/logger"
	"voicebot/pkg/providers"
	"voicebot/pkg/routing"
	"voicebot/pkg/skills"
	"voicebot/pkg/state"
	"voicebot/pkg/tools"
	"voicebot/pkg/utils"
	"voicebot/pkg/voice"
)

// Local message types (bus package removed)
type InboundMessage struct {
	Channel    string
	ChatID     string
	SenderID   string
	Content    string
	Media      []string
	MediaScope string
	ReplyTo    string
	MessageID  string
	SessionKey string
	Peer       *Peer
	Sender     *SenderInfo
	Metadata   map[string]any
}

type OutboundMessage struct {
	Channel          string
	ChatID           string
	Content          string
	ReplyTo          string
	ReplyToMessageID string
}

type Peer struct {
	ID       string
	Name     string
	Username string
	Kind     string
}

type SenderInfo struct {
	ID       string
	Name     string
	Username string
}

type AgentLoop struct {
	cfg            *config.Config
	registry       *AgentRegistry
	state          *state.Manager
	running        atomic.Bool
	summarizing    sync.Map
	fallback       *providers.FallbackChain
	transcriber    voice.Transcriber
	cmdRegistry    *commands.Registry
	mcp            mcpRuntime
	mu             sync.RWMutex
	// Track active requests for safe provider cleanup
	activeRequests sync.WaitGroup
}

// processOptions configures how a message is processed
type processOptions struct {
	SessionKey      string   // Session identifier for history/context
	Channel         string   // Target channel for tool execution
	ChatID          string   // Target chat ID for tool execution
	UserMessage     string   // User message content (may include prefix)
	Media           []string // media:// refs from inbound message
	DefaultResponse string   // Response when LLM returns empty
	EnableSummary   bool     // Whether to trigger summarization
	SendResponse    bool     // Whether to send response via bus
	NoHistory       bool     // If true, don't load session history (for heartbeat)
}

const (
	defaultResponse           = "I've completed processing but have no response to give. Increase `max_tool_iterations` in config.json."
	sessionKeyAgentPrefix     = "agent:"
	metadataKeyAccountID      = "account_id"
	metadataKeyGuildID        = "guild_id"
	metadataKeyTeamID         = "team_id"
	metadataKeyParentPeerKind = "parent_peer_kind"
	metadataKeyParentPeerID   = "parent_peer_id"
)

func NewAgentLoop(
	cfg *config.Config,
	provider providers.LLMProvider,
) *AgentLoop {
	registry := NewAgentRegistry(cfg, provider)

	// Register shared tools to all agents
	registerSharedTools(cfg, registry, provider)

	// Set up shared fallback chain
	cooldown := providers.NewCooldownTracker()
	fallbackChain := providers.NewFallbackChain(cooldown)

	// Create state manager using default agent's workspace for channel recording
	defaultAgent := registry.GetDefaultAgent()
	var stateManager *state.Manager
	if defaultAgent != nil {
		stateManager = state.NewManager(defaultAgent.Workspace)
	}

	al := &AgentLoop{
		cfg:         cfg,
		registry:    registry,
		state:       stateManager,
		summarizing: sync.Map{},
		fallback:    fallbackChain,
		cmdRegistry: commands.NewRegistry(commands.BuiltinDefinitions()),
	}

	return al
}

// registerSharedTools registers tools that are shared across all agents (web, message, spawn).
func registerSharedTools(
	cfg *config.Config,
	registry *AgentRegistry,
	provider providers.LLMProvider,
) {
	for _, agentID := range registry.ListAgentIDs() {
		agent, ok := registry.GetAgent(agentID)
		if !ok {
			continue
		}

		if cfg.Tools.IsToolEnabled("web") {
			searchTool, err := tools.NewWebSearchTool(tools.WebSearchToolOptions{
				BraveAPIKeys:         config.MergeAPIKeys(cfg.Tools.Web.Brave.APIKey, cfg.Tools.Web.Brave.APIKeys),
				BraveMaxResults:      cfg.Tools.Web.Brave.MaxResults,
				BraveEnabled:         cfg.Tools.Web.Brave.Enabled,
				TavilyAPIKeys:        config.MergeAPIKeys(cfg.Tools.Web.Tavily.APIKey, cfg.Tools.Web.Tavily.APIKeys),
				TavilyBaseURL:        cfg.Tools.Web.Tavily.BaseURL,
				TavilyMaxResults:     cfg.Tools.Web.Tavily.MaxResults,
				TavilyEnabled:        cfg.Tools.Web.Tavily.Enabled,
				DuckDuckGoMaxResults: cfg.Tools.Web.DuckDuckGo.MaxResults,
				DuckDuckGoEnabled:    cfg.Tools.Web.DuckDuckGo.Enabled,
				PerplexityAPIKeys: config.MergeAPIKeys(
					cfg.Tools.Web.Perplexity.APIKey,
					cfg.Tools.Web.Perplexity.APIKeys,
				),
				PerplexityMaxResults: cfg.Tools.Web.Perplexity.MaxResults,
				PerplexityEnabled:    cfg.Tools.Web.Perplexity.Enabled,
				SearXNGBaseURL:       cfg.Tools.Web.SearXNG.BaseURL,
				SearXNGMaxResults:    cfg.Tools.Web.SearXNG.MaxResults,
				SearXNGEnabled:       cfg.Tools.Web.SearXNG.Enabled,
				GLMSearchAPIKey:      cfg.Tools.Web.GLMSearch.APIKey,
				GLMSearchBaseURL:     cfg.Tools.Web.GLMSearch.BaseURL,
				GLMSearchEngine:      cfg.Tools.Web.GLMSearch.SearchEngine,
				GLMSearchMaxResults:  cfg.Tools.Web.GLMSearch.MaxResults,
				GLMSearchEnabled:     cfg.Tools.Web.GLMSearch.Enabled,
				Proxy:                cfg.Tools.Web.Proxy,
			})
			if err != nil {
				logger.ErrorCF("agent", "Failed to create web search tool", map[string]any{"error": err.Error()})
			} else if searchTool != nil {
				agent.Tools.Register(searchTool)
			}
		}
		if cfg.Tools.IsToolEnabled("web_fetch") {
			fetchTool, err := tools.NewWebFetchToolWithProxy(50000, cfg.Tools.Web.Proxy, cfg.Tools.Web.FetchLimitBytes)
			if err != nil {
				logger.ErrorCF("agent", "Failed to create web fetch tool", map[string]any{"error": err.Error()})
			} else {
				agent.Tools.Register(fetchTool)
			}
		}

		// Hardware tools (I2C, SPI) - Linux only, returns error on other platforms
		if cfg.Tools.IsToolEnabled("i2c") {
			agent.Tools.Register(tools.NewI2CTool())
		}
		if cfg.Tools.IsToolEnabled("spi") {
			agent.Tools.Register(tools.NewSPITool())
		}

		// Message tool (bus removed - callback not set)
		if cfg.Tools.IsToolEnabled("message") {
			messageTool := tools.NewMessageTool()
			// Note: SetSendCallback removed - bus no longer available
			agent.Tools.Register(messageTool)
		}

		// Send file tool (outbound media via MediaStore — store injected later by SetMediaStore)
		if cfg.Tools.IsToolEnabled("send_file") {
			sendFileTool := tools.NewSendFileTool(
				agent.Workspace,
				cfg.Agents.Defaults.RestrictToWorkspace,
				int64(cfg.Agents.Defaults.GetMaxMediaSize()),
				nil,
			)
			agent.Tools.Register(sendFileTool)
		}

		// Skill discovery and installation tools
		skills_enabled := cfg.Tools.IsToolEnabled("skills")
		find_skills_enable := cfg.Tools.IsToolEnabled("find_skills")
		if skills_enabled && find_skills_enable {
			registryMgr := skills.NewRegistryManagerFromConfig(skills.RegistryConfig{
				MaxConcurrentSearches: cfg.Tools.Skills.MaxConcurrentSearches,
				ClawHub:               skills.ClawHubConfig(cfg.Tools.Skills.Registries.ClawHub),
			})

			searchCache := skills.NewSearchCache(
				cfg.Tools.Skills.SearchCache.MaxSize,
				time.Duration(cfg.Tools.Skills.SearchCache.TTLSeconds)*time.Second,
			)
			agent.Tools.Register(tools.NewFindSkillsTool(registryMgr, searchCache))
		}

		// Spawn tool with allowlist checker
		if cfg.Tools.IsToolEnabled("spawn") {
			if cfg.Tools.IsToolEnabled("subagent") {
				subagentManager := tools.NewSubagentManager(provider, agent.Model, agent.Workspace)
				subagentManager.SetLLMOptions(agent.MaxTokens, agent.Temperature)
				spawnTool := tools.NewSpawnTool(subagentManager)
				currentAgentID := agentID
				spawnTool.SetAllowlistChecker(func(targetAgentID string) bool {
					return registry.CanSpawnSubagent(currentAgentID, targetAgentID)
				})
				agent.Tools.Register(spawnTool)
			} else {
				logger.WarnCF("agent", "spawn tool requires subagent to be enabled", nil)
			}
		}
	}
}

func (al *AgentLoop) Run(ctx context.Context) error {
	al.running.Store(true)

	if err := al.ensureMCPInitialized(ctx); err != nil {
		return err
	}

	for al.running.Load() {
		select {
		case <-ctx.Done():
			return nil
		default:
			// Bus removed - no messages to consume
			// Sleep briefly to avoid busy loop
			time.Sleep(100 * time.Millisecond)
		}
	}

	return nil
}

func (al *AgentLoop) Stop() {
	al.running.Store(false)
}

// Close releases resources held by agent session stores. Call after Stop.
func (al *AgentLoop) Close() {
	mcpManager := al.mcp.takeManager()

	if mcpManager != nil {
		if err := mcpManager.Close(); err != nil {
			logger.ErrorCF("agent", "Failed to close MCP manager",
				map[string]any{
					"error": err.Error(),
				})
		}
	}

	al.GetRegistry().Close()
}

func (al *AgentLoop) RegisterTool(tool tools.Tool) {
	registry := al.GetRegistry()
	for _, agentID := range registry.ListAgentIDs() {
		if agent, ok := registry.GetAgent(agentID); ok {
			agent.Tools.Register(tool)
		}
	}
}

// ReloadProviderAndConfig atomically swaps the provider and config with proper synchronization.
// It uses a context to allow timeout control from the caller.
// Returns an error if the reload fails or context is canceled.
func (al *AgentLoop) ReloadProviderAndConfig(
	ctx context.Context,
	provider providers.LLMProvider,
	cfg *config.Config,
) error {
	// Validate inputs
	if provider == nil {
		return fmt.Errorf("provider cannot be nil")
	}
	if cfg == nil {
		return fmt.Errorf("config cannot be nil")
	}

	// Create new registry with updated config and provider
	// Wrap in defer/recover to handle any panics gracefully
	var registry *AgentRegistry
	var panicErr error
	done := make(chan struct{}, 1)

	go func() {
		defer func() {
			if r := recover(); r != nil {
				panicErr = fmt.Errorf("panic during registry creation: %v", r)
				logger.ErrorCF("agent", "Panic during registry creation",
					map[string]any{"panic": r})
			}
			close(done)
		}()

		registry = NewAgentRegistry(cfg, provider)
	}()

	// Wait for completion or context cancellation
	select {
	case <-done:
		if registry == nil {
			if panicErr != nil {
				return fmt.Errorf("registry creation failed: %w", panicErr)
			}
			return fmt.Errorf("registry creation failed (nil result)")
		}
	case <-ctx.Done():
		return fmt.Errorf("context canceled during registry creation: %w", ctx.Err())
	}

	// Check context again before proceeding
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context canceled after registry creation: %w", err)
	}

	// Ensure shared tools are re-registered on the new registry
	registerSharedTools(cfg, registry, provider)

	// Atomically swap the config and registry under write lock
	// This ensures readers see a consistent pair
	al.mu.Lock()
	oldRegistry := al.registry

	// Store new values
	al.cfg = cfg
	al.registry = registry

	// Also update fallback chain with new config
	al.fallback = providers.NewFallbackChain(providers.NewCooldownTracker())

	al.mu.Unlock()

	// Close old provider after releasing the lock
	// This prevents blocking readers while closing
	if oldProvider, ok := extractProvider(oldRegistry); ok {
		if stateful, ok := oldProvider.(providers.StatefulProvider); ok {
			// Give in-flight requests a moment to complete
			// Use a reasonable timeout that balances cleanup vs resource usage
			select {
			case <-time.After(100 * time.Millisecond):
				stateful.Close()
			case <-ctx.Done():
				// Context canceled, close immediately but log warning
				logger.WarnCF("agent", "Context canceled during provider cleanup, forcing close",
					map[string]any{"error": ctx.Err()})
				stateful.Close()
			}
		}
	}

	logger.InfoCF("agent", "Provider and config reloaded successfully",
		map[string]any{
			"model": cfg.Agents.Defaults.GetModelName(),
		})

	return nil
}

// GetRegistry returns the current registry (thread-safe)
func (al *AgentLoop) GetRegistry() *AgentRegistry {
	al.mu.RLock()
	defer al.mu.RUnlock()
	return al.registry
}

// GetConfig returns the current config (thread-safe)
func (al *AgentLoop) GetConfig() *config.Config {
	al.mu.RLock()
	defer al.mu.RUnlock()
	return al.cfg
}

// SetTranscriber injects a voice transcriber for agent-level audio transcription.
func (al *AgentLoop) SetTranscriber(t voice.Transcriber) {
	al.transcriber = t
}

var audioAnnotationRe = regexp.MustCompile(`\[(voice|audio)(?::[^\]]*)?\]`)

// transcribeAudioInMessage resolves audio media refs, transcribes them, and
// replaces audio annotations in msg.Content with the transcribed text.
// Returns the (possibly modified) message and true if audio was transcribed.
// Feature removed: mediaStore no longer available.
func (al *AgentLoop) transcribeAudioInMessage(ctx context.Context, msg InboundMessage) (InboundMessage, bool) {
	// No-op: mediaStore removed
	return msg, false
}

// sendTranscriptionFeedback sends feedback to the user with the result of
// audio transcription if the option is enabled.
// Feature removed: channelManager no longer available.
func (al *AgentLoop) sendTranscriptionFeedback(
	ctx context.Context,
	channel, chatID, messageID string,
	validTexts []string,
) {
	// No-op: channelManager removed
}

// inferMediaType determines the media type ("image", "audio", "video", "file")
// from a filename and MIME content type.
func inferMediaType(filename, contentType string) string {
	ct := strings.ToLower(contentType)
	fn := strings.ToLower(filename)

	if strings.HasPrefix(ct, "image/") {
		return "image"
	}
	if strings.HasPrefix(ct, "audio/") || ct == "application/ogg" {
		return "audio"
	}
	if strings.HasPrefix(ct, "video/") {
		return "video"
	}

	// Fallback: infer from extension
	ext := filepath.Ext(fn)
	switch ext {
	case ".jpg", ".jpeg", ".png", ".gif", ".webp", ".bmp", ".svg":
		return "image"
	case ".mp3", ".wav", ".ogg", ".m4a", ".flac", ".aac", ".wma", ".opus":
		return "audio"
	case ".mp4", ".avi", ".mov", ".webm", ".mkv":
		return "video"
	}

	return "file"
}

// RecordLastChannel records the last active channel for this workspace.
// This uses the atomic state save mechanism to prevent data loss on crash.
func (al *AgentLoop) RecordLastChannel(channel string) error {
	if al.state == nil {
		return nil
	}
	return al.state.SetLastChannel(channel)
}

// RecordLastChatID records the last active chat ID for this workspace.
// This uses the atomic state save mechanism to prevent data loss on crash.
func (al *AgentLoop) RecordLastChatID(chatID string) error {
	if al.state == nil {
		return nil
	}
	return al.state.SetLastChatID(chatID)
}

func (al *AgentLoop) ProcessDirect(
	ctx context.Context,
	content, sessionKey string,
) (string, error) {
	return al.ProcessDirectWithChannel(ctx, content, sessionKey, "cli", "direct")
}

func (al *AgentLoop) ProcessDirectWithChannel(
	ctx context.Context,
	content, sessionKey, channel, chatID string,
) (string, error) {
	if err := al.ensureMCPInitialized(ctx); err != nil {
		return "", err
	}

	msg := InboundMessage{
		Channel:    channel,
		SenderID:   "cron",
		ChatID:     chatID,
		Content:    content,
		SessionKey: sessionKey,
	}

	return al.processMessage(ctx, msg)
}

// ProcessHeartbeat processes a heartbeat request without session history.
// Each heartbeat is independent and doesn't accumulate context.
func (al *AgentLoop) ProcessHeartbeat(
	ctx context.Context,
	content, channel, chatID string,
) (string, error) {
	agent := al.GetRegistry().GetDefaultAgent()
	if agent == nil {
		return "", fmt.Errorf("no default agent for heartbeat")
	}
	return al.runAgentLoop(ctx, agent, processOptions{
		SessionKey:      "heartbeat",
		Channel:         channel,
		ChatID:          chatID,
		UserMessage:     content,
		DefaultResponse: defaultResponse,
		EnableSummary:   false,
		SendResponse:    false,
		NoHistory:       true, // Don't load session history for heartbeat
	})
}

// StreamCallbacks handles streaming response chunks for voice pipeline
type StreamCallbacks struct {
	OnChunk func(chunk string) // Called for each text chunk
}

// ProcessDirectStream processes a message with streaming response.
// If the provider supports streaming (implements StreamCapable), it streams chunks via callbacks.
// Otherwise, it falls back to non-streaming and calls OnChunk once with the full response.
func (al *AgentLoop) ProcessDirectStream(
	ctx context.Context,
	content, sessionKey, channel, chatID string,
	callbacks StreamCallbacks,
) (string, error) {
	if err := al.ensureMCPInitialized(ctx); err != nil {
		return "", err
	}

	agent := al.GetRegistry().GetDefaultAgent()
	if agent == nil {
		return "", fmt.Errorf("no default agent available")
	}

	// Check if provider supports streaming
	streamCapable, ok := agent.Provider.(providers.StreamCapable)
	if !ok {
		// Fallback to non-streaming
		response, err := al.ProcessDirectWithChannel(ctx, content, sessionKey, channel, chatID)
		if err != nil {
			return "", err
		}
		if callbacks.OnChunk != nil {
			callbacks.OnChunk(response)
		}
		return response, nil
	}

	// Use streaming path
	opts := processOptions{
		SessionKey:      sessionKey,
		Channel:         channel,
		ChatID:          chatID,
		UserMessage:     content,
		DefaultResponse: defaultResponse,
		EnableSummary:   true,
		SendResponse:    false,
	}

	return al.runAgentLoopStream(ctx, agent, opts, streamCapable, callbacks)
}

// runAgentLoopStream is the streaming variant of runAgentLoop
func (al *AgentLoop) runAgentLoopStream(
	ctx context.Context,
	agent *AgentInstance,
	opts processOptions,
	streamCapable providers.StreamCapable,
	callbacks StreamCallbacks,
) (string, error) {
	// 1. Build messages
	var history []providers.Message
	var summary string
	if !opts.NoHistory {
		history = agent.Sessions.GetHistory(opts.SessionKey)
		summary = agent.Sessions.GetSummary(opts.SessionKey)
	}
	messages := agent.ContextBuilder.BuildMessages(
		history,
		summary,
		opts.UserMessage,
		opts.Media,
		opts.Channel,
		opts.ChatID,
	)

	// 2. Save user message to session
	agent.Sessions.AddMessage(opts.SessionKey, "user", opts.UserMessage)

	// 3. Run streaming LLM iteration loop
	finalContent, _, err := al.runLLMIterationStream(ctx, agent, messages, opts, streamCapable, callbacks)
	if err != nil {
		return "", err
	}

	// 4. Handle empty response
	if finalContent == "" {
		finalContent = opts.DefaultResponse
	}

	// 5. Save final assistant message to session
	agent.Sessions.AddMessage(opts.SessionKey, "assistant", finalContent)
	agent.Sessions.Save(opts.SessionKey)

	// 6. Optional: summarization
	if opts.EnableSummary {
		al.maybeSummarize(agent, opts.SessionKey, opts.Channel, opts.ChatID)
	}

	return finalContent, nil
}

// runLLMIterationStream executes the LLM call loop with streaming
func (al *AgentLoop) runLLMIterationStream(
	ctx context.Context,
	agent *AgentInstance,
	messages []providers.Message,
	opts processOptions,
	streamCapable providers.StreamCapable,
	callbacks StreamCallbacks,
) (string, int, error) {
	iteration := 0
	var finalContent string

	activeCandidates, activeModel := al.selectCandidates(agent, opts.UserMessage, messages)

	for iteration < agent.MaxIterations {
		iteration++

		logger.DebugCF("agent", "LLM iteration (streaming)",
			map[string]any{
				"agent_id":  agent.ID,
				"iteration": iteration,
				"max":       agent.MaxIterations,
			})

		providerToolDefs := agent.Tools.ToProviderDefs()

		llmOpts := map[string]any{
			"max_tokens":       agent.MaxTokens,
			"temperature":      agent.Temperature,
			"prompt_cache_key": agent.ID,
		}

		// Build streaming callbacks adapter
		streamCallbacks := providers.StreamCallbacks{
			OnChunk: func(chunk providers.StreamChunk) {
				if callbacks.OnChunk != nil && chunk.Content != "" {
					callbacks.OnChunk(chunk.Content)
				}
			},
		}

		// Call LLM with streaming
		var response *providers.LLMResponse
		var err error

		al.activeRequests.Add(1)
		func() {
			defer al.activeRequests.Done()

			if len(activeCandidates) > 1 && al.fallback != nil {
				// For fallback with streaming, use first candidate only
				// (streaming fallback is complex and not commonly needed)
				response, err = streamCapable.ChatStream(
					ctx, messages, providerToolDefs, activeModel, llmOpts, streamCallbacks,
				)
			} else {
				response, err = streamCapable.ChatStream(
					ctx, messages, providerToolDefs, activeModel, llmOpts, streamCallbacks,
				)
			}
		}()

		if err != nil {
			logger.ErrorCF("agent", "LLM streaming call failed",
				map[string]any{
					"agent_id":  agent.ID,
					"iteration": iteration,
					"model":     activeModel,
					"error":     err.Error(),
				})
			return "", iteration, fmt.Errorf("LLM streaming call failed: %w", err)
		}

		logger.DebugCF("agent", "LLM streaming response",
			map[string]any{
				"agent_id":      agent.ID,
				"iteration":     iteration,
				"content_chars": len(response.Content),
				"tool_calls":    len(response.ToolCalls),
			})

		// Check if no tool calls
		if len(response.ToolCalls) == 0 {
			finalContent = response.Content
			break
		}

		// Handle tool calls (same as non-streaming)
		normalizedToolCalls := make([]providers.ToolCall, 0, len(response.ToolCalls))
		for _, tc := range response.ToolCalls {
			normalizedToolCalls = append(normalizedToolCalls, providers.NormalizeToolCall(tc))
		}

		// Build assistant message with tool calls
		assistantMsg := providers.Message{
			Role:    "assistant",
			Content: response.Content,
		}
		for _, tc := range normalizedToolCalls {
			argumentsJSON, _ := json.Marshal(tc.Arguments)
			assistantMsg.ToolCalls = append(assistantMsg.ToolCalls, providers.ToolCall{
				ID:   tc.ID,
				Type: "function",
				Name: tc.Name,
				Function: &providers.FunctionCall{
					Name:      tc.Name,
					Arguments: string(argumentsJSON),
				},
			})
		}
		messages = append(messages, assistantMsg)
		agent.Sessions.AddFullMessage(opts.SessionKey, assistantMsg)

		// Execute tool calls in parallel
		type indexedResult struct {
			result *tools.ToolResult
			tc     providers.ToolCall
		}
		results := make([]indexedResult, len(normalizedToolCalls))
		var wg sync.WaitGroup

		for i, tc := range normalizedToolCalls {
			results[i].tc = tc
			wg.Add(1)
			go func(idx int, tc providers.ToolCall) {
				defer wg.Done()
				results[idx].result = agent.Tools.ExecuteWithContext(
					ctx, tc.Name, tc.Arguments, opts.Channel, opts.ChatID, nil,
				)
			}(i, tc)
		}
		wg.Wait()

		// Process results
		for _, r := range results {
			contentForLLM := r.result.ForLLM
			if contentForLLM == "" && r.result.Err != nil {
				contentForLLM = r.result.Err.Error()
			}
			toolResultMsg := providers.Message{
				Role:       "tool",
				Content:    contentForLLM,
				ToolCallID: r.tc.ID,
			}
			messages = append(messages, toolResultMsg)
			agent.Sessions.AddFullMessage(opts.SessionKey, toolResultMsg)
		}

		agent.Tools.TickTTL()
	}

	return finalContent, iteration, nil
}

func (al *AgentLoop) processMessage(ctx context.Context, msg InboundMessage) (string, error) {
	// Add message preview to log (show full content for error messages)
	var logContent string
	if strings.Contains(msg.Content, "Error:") || strings.Contains(msg.Content, "error") {
		logContent = msg.Content // Full content for errors
	} else {
		logContent = utils.Truncate(msg.Content, 80)
	}
	logger.InfoCF(
		"agent",
		fmt.Sprintf("Processing message from %s:%s: %s", msg.Channel, msg.SenderID, logContent),
		map[string]any{
			"channel":     msg.Channel,
			"chat_id":     msg.ChatID,
			"sender_id":   msg.SenderID,
			"session_key": msg.SessionKey,
		},
	)

	var _ bool // hadAudio - no longer used after channelManager removal
	msg, _ = al.transcribeAudioInMessage(ctx, msg)

	// Placeholder feature removed (channelManager no longer available)

	// Route system messages to processSystemMessage
	if msg.Channel == "system" {
		return al.processSystemMessage(ctx, msg)
	}

	route, agent, routeErr := al.resolveMessageRoute(msg)
	if routeErr != nil {
		return "", routeErr
	}

	// Reset message-tool state for this round so we don't skip publishing due to a previous round.
	if tool, ok := agent.Tools.Get("message"); ok {
		if resetter, ok := tool.(interface{ ResetSentInRound() }); ok {
			resetter.ResetSentInRound()
		}
	}

	// Resolve session key from route, while preserving explicit agent-scoped keys.
	scopeKey := resolveScopeKey(route, msg.SessionKey)
	sessionKey := scopeKey

	logger.InfoCF("agent", "Routed message",
		map[string]any{
			"agent_id":      agent.ID,
			"scope_key":     scopeKey,
			"session_key":   sessionKey,
			"matched_by":    route.MatchedBy,
			"route_agent":   route.AgentID,
			"route_channel": route.Channel,
		})

	opts := processOptions{
		SessionKey:      sessionKey,
		Channel:         msg.Channel,
		ChatID:          msg.ChatID,
		UserMessage:     msg.Content,
		Media:           msg.Media,
		DefaultResponse: defaultResponse,
		EnableSummary:   true,
		SendResponse:    false,
	}

	// context-dependent commands check their own Runtime fields and report
	// "unavailable" when the required capability is nil.
	if response, handled := al.handleCommand(ctx, msg, agent, &opts); handled {
		return response, nil
	}

	return al.runAgentLoop(ctx, agent, opts)
}

func (al *AgentLoop) resolveMessageRoute(msg InboundMessage) (routing.ResolvedRoute, *AgentInstance, error) {
	registry := al.GetRegistry()
	route := registry.ResolveRoute(routing.RouteInput{
		Channel:    msg.Channel,
		AccountID:  inboundMetadata(msg, metadataKeyAccountID),
		Peer:       extractPeer(msg),
		ParentPeer: extractParentPeer(msg),
		GuildID:    inboundMetadata(msg, metadataKeyGuildID),
		TeamID:     inboundMetadata(msg, metadataKeyTeamID),
	})

	agent, ok := registry.GetAgent(route.AgentID)
	if !ok {
		agent = registry.GetDefaultAgent()
	}
	if agent == nil {
		return routing.ResolvedRoute{}, nil, fmt.Errorf("no agent available for route (agent_id=%s)", route.AgentID)
	}

	return route, agent, nil
}

func resolveScopeKey(route routing.ResolvedRoute, msgSessionKey string) string {
	if msgSessionKey != "" && strings.HasPrefix(msgSessionKey, sessionKeyAgentPrefix) {
		return msgSessionKey
	}
	return route.SessionKey
}

func (al *AgentLoop) processSystemMessage(
	ctx context.Context,
	msg InboundMessage,
) (string, error) {
	if msg.Channel != "system" {
		return "", fmt.Errorf(
			"processSystemMessage called with non-system message channel: %s",
			msg.Channel,
		)
	}

	logger.InfoCF("agent", "Processing system message",
		map[string]any{
			"sender_id": msg.SenderID,
			"chat_id":   msg.ChatID,
		})

	// Parse origin channel from chat_id (format: "channel:chat_id")
	var originChannel, originChatID string
	if idx := strings.Index(msg.ChatID, ":"); idx > 0 {
		originChannel = msg.ChatID[:idx]
		originChatID = msg.ChatID[idx+1:]
	} else {
		originChannel = "cli"
		originChatID = msg.ChatID
	}

	// Extract subagent result from message content
	// Format: "Task 'label' completed.\n\nResult:\n<actual content>"
	content := msg.Content
	if idx := strings.Index(content, "Result:\n"); idx >= 0 {
		content = content[idx+8:] // Extract just the result part
	}

	// Skip internal channels - only log, don't send to user
	if constants.IsInternalChannel(originChannel) {
		logger.InfoCF("agent", "Subagent completed (internal channel)",
			map[string]any{
				"sender_id":   msg.SenderID,
				"content_len": len(content),
				"channel":     originChannel,
			})
		return "", nil
	}

	// Use default agent for system messages
	agent := al.GetRegistry().GetDefaultAgent()
	if agent == nil {
		return "", fmt.Errorf("no default agent for system message")
	}

	// Use the origin session for context
	sessionKey := routing.BuildAgentMainSessionKey(agent.ID)

	return al.runAgentLoop(ctx, agent, processOptions{
		SessionKey:      sessionKey,
		Channel:         originChannel,
		ChatID:          originChatID,
		UserMessage:     fmt.Sprintf("[System: %s] %s", msg.SenderID, msg.Content),
		DefaultResponse: "Background task completed.",
		EnableSummary:   false,
		SendResponse:    true,
	})
}

// runAgentLoop is the core message processing logic.
func (al *AgentLoop) runAgentLoop(
	ctx context.Context,
	agent *AgentInstance,
	opts processOptions,
) (string, error) {
	// 0. Record last channel for heartbeat notifications (skip internal channels and cli)
	if opts.Channel != "" && opts.ChatID != "" {
		if !constants.IsInternalChannel(opts.Channel) {
			channelKey := fmt.Sprintf("%s:%s", opts.Channel, opts.ChatID)
			if err := al.RecordLastChannel(channelKey); err != nil {
				logger.WarnCF(
					"agent",
					"Failed to record last channel",
					map[string]any{"error": err.Error()},
				)
			}
		}
	}

	// 1. Build messages (skip history for heartbeat)
	var history []providers.Message
	var summary string
	if !opts.NoHistory {
		history = agent.Sessions.GetHistory(opts.SessionKey)
		summary = agent.Sessions.GetSummary(opts.SessionKey)
	}
	messages := agent.ContextBuilder.BuildMessages(
		history,
		summary,
		opts.UserMessage,
		opts.Media,
		opts.Channel,
		opts.ChatID,
	)

	// Resolve media:// refs to base64 data URLs (streaming)
	// Feature removed: mediaStore no longer available
	// cfg := al.GetConfig()
	// maxMediaSize := cfg.Agents.Defaults.GetMaxMediaSize()
	// messages = resolveMediaRefs(messages, al.mediaStore, maxMediaSize)

	// 2. Save user message to session
	agent.Sessions.AddMessage(opts.SessionKey, "user", opts.UserMessage)

	// 3. Run LLM iteration loop
	finalContent, iteration, err := al.runLLMIteration(ctx, agent, messages, opts)
	if err != nil {
		return "", err
	}

	// If last tool had ForUser content and we already sent it, we might not need to send final response
	// This is controlled by the tool's Silent flag and ForUser content

	// 4. Handle empty response
	if finalContent == "" {
		finalContent = opts.DefaultResponse
	}

	// 5. Save final assistant message to session
	agent.Sessions.AddMessage(opts.SessionKey, "assistant", finalContent)
	agent.Sessions.Save(opts.SessionKey)

	// 6. Optional: summarization
	if opts.EnableSummary {
		al.maybeSummarize(agent, opts.SessionKey, opts.Channel, opts.ChatID)
	}

	// 7. Optional: send response via bus (removed - bus no longer available)
	// if opts.SendResponse { ... }

	// 8. Log response
	responsePreview := utils.Truncate(finalContent, 120)
	logger.InfoCF("agent", fmt.Sprintf("Response: %s", responsePreview),
		map[string]any{
			"agent_id":     agent.ID,
			"session_key":  opts.SessionKey,
			"iterations":   iteration,
			"final_length": len(finalContent),
		})

	return finalContent, nil
}

func (al *AgentLoop) targetReasoningChannelID(channelName string) (chatID string) {
	// Feature removed: channelManager no longer available
	return ""
}

func (al *AgentLoop) handleReasoning(
	ctx context.Context,
	reasoningContent, channelName, channelID string,
) {
	// Feature removed: bus no longer available
	// Reasoning output is no longer published
}

// runLLMIteration executes the LLM call loop with tool handling.
func (al *AgentLoop) runLLMIteration(
	ctx context.Context,
	agent *AgentInstance,
	messages []providers.Message,
	opts processOptions,
) (string, int, error) {
	iteration := 0
	var finalContent string

	// Determine effective model tier for this conversation turn.
	// selectCandidates evaluates routing once and the decision is sticky for
	// all tool-follow-up iterations within the same turn so that a multi-step
	// tool chain doesn't switch models mid-way through.
	activeCandidates, activeModel := al.selectCandidates(agent, opts.UserMessage, messages)

	for iteration < agent.MaxIterations {
		iteration++

		logger.DebugCF("agent", "LLM iteration",
			map[string]any{
				"agent_id":  agent.ID,
				"iteration": iteration,
				"max":       agent.MaxIterations,
			})

		// Build tool definitions
		providerToolDefs := agent.Tools.ToProviderDefs()

		// Log LLM request details
		logger.DebugCF("agent", "LLM request",
			map[string]any{
				"agent_id":          agent.ID,
				"iteration":         iteration,
				"model":             activeModel,
				"messages_count":    len(messages),
				"tools_count":       len(providerToolDefs),
				"max_tokens":        agent.MaxTokens,
				"temperature":       agent.Temperature,
				"system_prompt_len": len(messages[0].Content),
			})

		// Log full messages (detailed)
		logger.DebugCF("agent", "Full LLM request",
			map[string]any{
				"iteration":     iteration,
				"messages_json": formatMessagesForLog(messages),
				"tools_json":    formatToolsForLog(providerToolDefs),
			})

		// Call LLM with fallback chain if multiple candidates are configured.
		var response *providers.LLMResponse
		var err error

		llmOpts := map[string]any{
			"max_tokens":       agent.MaxTokens,
			"temperature":      agent.Temperature,
			"prompt_cache_key": agent.ID,
		}
		// parseThinkingLevel guarantees ThinkingOff for empty/unknown values,
		// so checking != ThinkingOff is sufficient.
		if agent.ThinkingLevel != ThinkingOff {
			if tc, ok := agent.Provider.(providers.ThinkingCapable); ok && tc.SupportsThinking() {
				llmOpts["thinking_level"] = string(agent.ThinkingLevel)
			} else {
				logger.WarnCF("agent", "thinking_level is set but current provider does not support it, ignoring",
					map[string]any{"agent_id": agent.ID, "thinking_level": string(agent.ThinkingLevel)})
			}
		}

		callLLM := func() (*providers.LLMResponse, error) {
			al.activeRequests.Add(1)
			defer al.activeRequests.Done()

			if len(activeCandidates) > 1 && al.fallback != nil {
				fbResult, fbErr := al.fallback.Execute(
					ctx,
					activeCandidates,
					func(ctx context.Context, provider, model string) (*providers.LLMResponse, error) {
						return agent.Provider.Chat(ctx, messages, providerToolDefs, model, llmOpts)
					},
				)
				if fbErr != nil {
					return nil, fbErr
				}
				if fbResult.Provider != "" && len(fbResult.Attempts) > 0 {
					logger.InfoCF(
						"agent",
						fmt.Sprintf("Fallback: succeeded with %s/%s after %d attempts",
							fbResult.Provider, fbResult.Model, len(fbResult.Attempts)+1),
						map[string]any{"agent_id": agent.ID, "iteration": iteration},
					)
				}
				return fbResult.Response, nil
			}
			return agent.Provider.Chat(ctx, messages, providerToolDefs, activeModel, llmOpts)
		}

		// Retry loop for context/token errors
		maxRetries := 2
		for retry := 0; retry <= maxRetries; retry++ {
			response, err = callLLM()
			if err == nil {
				break
			}

			errMsg := strings.ToLower(err.Error())

			// Check if this is a network/HTTP timeout — not a context window error.
			isTimeoutError := errors.Is(err, context.DeadlineExceeded) ||
				strings.Contains(errMsg, "deadline exceeded") ||
				strings.Contains(errMsg, "client.timeout") ||
				strings.Contains(errMsg, "timed out") ||
				strings.Contains(errMsg, "timeout exceeded")

			// Detect real context window / token limit errors, excluding network timeouts.
			isContextError := !isTimeoutError && (strings.Contains(errMsg, "context_length_exceeded") ||
				strings.Contains(errMsg, "context window") ||
				strings.Contains(errMsg, "maximum context length") ||
				strings.Contains(errMsg, "token limit") ||
				strings.Contains(errMsg, "too many tokens") ||
				strings.Contains(errMsg, "max_tokens") ||
				strings.Contains(errMsg, "invalidparameter") ||
				strings.Contains(errMsg, "prompt is too long") ||
				strings.Contains(errMsg, "request too large"))

			if isTimeoutError && retry < maxRetries {
				backoff := time.Duration(retry+1) * 5 * time.Second
				logger.WarnCF("agent", "Timeout error, retrying after backoff", map[string]any{
					"error":   err.Error(),
					"retry":   retry,
					"backoff": backoff.String(),
				})
				time.Sleep(backoff)
				continue
			}

			if isContextError && retry < maxRetries {
				logger.WarnCF(
					"agent",
					"Context window error detected, attempting compression",
					map[string]any{
						"error": err.Error(),
						"retry": retry,
					},
				)

				if retry == 0 && !constants.IsInternalChannel(opts.Channel) {
					// Bus removed - cannot publish context exceeded message
				}

				al.forceCompression(agent, opts.SessionKey)
				newHistory := agent.Sessions.GetHistory(opts.SessionKey)
				newSummary := agent.Sessions.GetSummary(opts.SessionKey)
				messages = agent.ContextBuilder.BuildMessages(
					newHistory, newSummary, "",
					nil, opts.Channel, opts.ChatID,
				)
				continue
			}
			break
		}

		if err != nil {
			logger.ErrorCF("agent", "LLM call failed",
				map[string]any{
					"agent_id":  agent.ID,
					"iteration": iteration,
					"model":     activeModel,
					"error":     err.Error(),
				})
			return "", iteration, fmt.Errorf("LLM call failed after retries: %w", err)
		}

		go al.handleReasoning(
			ctx,
			response.Reasoning,
			opts.Channel,
			al.targetReasoningChannelID(opts.Channel),
		)

		logger.DebugCF("agent", "LLM response",
			map[string]any{
				"agent_id":       agent.ID,
				"iteration":      iteration,
				"content_chars":  len(response.Content),
				"tool_calls":     len(response.ToolCalls),
				"reasoning":      response.Reasoning,
				"target_channel": al.targetReasoningChannelID(opts.Channel),
				"channel":        opts.Channel,
			})
		// Check if no tool calls - then check reasoning content if any
		if len(response.ToolCalls) == 0 {
			finalContent = response.Content
			if finalContent == "" && response.ReasoningContent != "" {
				finalContent = response.ReasoningContent
			}
			logger.InfoCF("agent", "LLM response without tool calls (direct answer)",
				map[string]any{
					"agent_id":      agent.ID,
					"iteration":     iteration,
					"content_chars": len(finalContent),
				})
			break
		}

		normalizedToolCalls := make([]providers.ToolCall, 0, len(response.ToolCalls))
		for _, tc := range response.ToolCalls {
			normalizedToolCalls = append(normalizedToolCalls, providers.NormalizeToolCall(tc))
		}

		// Log tool calls
		toolNames := make([]string, 0, len(normalizedToolCalls))
		for _, tc := range normalizedToolCalls {
			toolNames = append(toolNames, tc.Name)
		}
		logger.InfoCF("agent", "LLM requested tool calls",
			map[string]any{
				"agent_id":  agent.ID,
				"tools":     toolNames,
				"count":     len(normalizedToolCalls),
				"iteration": iteration,
			})

		// Build assistant message with tool calls
		assistantMsg := providers.Message{
			Role:             "assistant",
			Content:          response.Content,
			ReasoningContent: response.ReasoningContent,
		}
		for _, tc := range normalizedToolCalls {
			argumentsJSON, _ := json.Marshal(tc.Arguments)
			// Copy ExtraContent to ensure thought_signature is persisted for Gemini 3
			extraContent := tc.ExtraContent
			thoughtSignature := ""
			if tc.Function != nil {
				thoughtSignature = tc.Function.ThoughtSignature
			}

			assistantMsg.ToolCalls = append(assistantMsg.ToolCalls, providers.ToolCall{
				ID:   tc.ID,
				Type: "function",
				Name: tc.Name,
				Function: &providers.FunctionCall{
					Name:             tc.Name,
					Arguments:        string(argumentsJSON),
					ThoughtSignature: thoughtSignature,
				},
				ExtraContent:     extraContent,
				ThoughtSignature: thoughtSignature,
			})
		}
		messages = append(messages, assistantMsg)

		// Save assistant message with tool calls to session
		agent.Sessions.AddFullMessage(opts.SessionKey, assistantMsg)

		// Execute tool calls in parallel
		type indexedAgentResult struct {
			result *tools.ToolResult
			tc     providers.ToolCall
		}

		agentResults := make([]indexedAgentResult, len(normalizedToolCalls))
		var wg sync.WaitGroup

		for i, tc := range normalizedToolCalls {
			agentResults[i].tc = tc

			wg.Add(1)
			go func(idx int, tc providers.ToolCall) {
				defer wg.Done()

				argsJSON, _ := json.Marshal(tc.Arguments)
				argsPreview := utils.Truncate(string(argsJSON), 200)
				logger.InfoCF("agent", fmt.Sprintf("Tool call: %s(%s)", tc.Name, argsPreview),
					map[string]any{
						"agent_id":  agent.ID,
						"tool":      tc.Name,
						"iteration": iteration,
					})

				// Create async callback for tools that implement AsyncExecutor.
				// When the background work completes, this publishes the result
				// as an inbound system message so processSystemMessage routes it
				// back to the user via the normal agent loop.
				asyncCallback := func(_ context.Context, result *tools.ToolResult) {
					// Send ForUser content directly to the user (immediate feedback),
					// mirroring the synchronous tool execution path.
					if !result.Silent && result.ForUser != "" {
						// Bus removed - cannot publish async tool result
					_ = result.ForUser
					}

					// Determine content for the agent loop (ForLLM or error).
					content := result.ForLLM
					if content == "" && result.Err != nil {
						content = result.Err.Error()
					}
					if content == "" {
						return
					}

					logger.InfoCF("agent", "Async tool completed, publishing result",
						map[string]any{
							"tool":        tc.Name,
							"content_len": len(content),
							"channel":     opts.Channel,
						})

					// Bus removed - cannot publish async result back to inbound
					_ = content
				}

				toolResult := agent.Tools.ExecuteWithContext(
					ctx,
					tc.Name,
					tc.Arguments,
					opts.Channel,
					opts.ChatID,
					asyncCallback,
				)
				agentResults[idx].result = toolResult
			}(i, tc)
		}
		wg.Wait()

		// Process results in original order (send to user, save to session)
		for _, r := range agentResults {
			// Send ForUser content to user immediately if not Silent (bus removed)
			if !r.result.Silent && r.result.ForUser != "" && opts.SendResponse {
				// Bus removed - cannot send tool result to user
				logger.DebugCF("agent", "Tool result (not sent - bus removed)",
					map[string]any{
						"tool":        r.tc.Name,
						"content_len": len(r.result.ForUser),
					})
			}

			// If tool returned media refs, publish them as outbound media (bus removed)
			if len(r.result.Media) > 0 {
				// Bus removed - cannot publish media
			}

			// Determine content for LLM based on tool result
			contentForLLM := r.result.ForLLM
			if contentForLLM == "" && r.result.Err != nil {
				contentForLLM = r.result.Err.Error()
			}

			toolResultMsg := providers.Message{
				Role:       "tool",
				Content:    contentForLLM,
				ToolCallID: r.tc.ID,
			}
			messages = append(messages, toolResultMsg)

			// Save tool result message to session
			agent.Sessions.AddFullMessage(opts.SessionKey, toolResultMsg)
		}

		// Tick down TTL of discovered tools after processing tool results.
		// Only reached when tool calls were made (the loop continues);
		// the break on no-tool-call responses skips this.
		// NOTE: This is safe because processMessage is sequential per agent.
		// If per-agent concurrency is added, TTL consistency between
		// ToProviderDefs and Get must be re-evaluated.
		agent.Tools.TickTTL()
		logger.DebugCF("agent", "TTL tick after tool execution", map[string]any{
			"agent_id": agent.ID, "iteration": iteration,
		})
	}

	return finalContent, iteration, nil
}

// selectCandidates returns the model candidates and resolved model name to use
// for a conversation turn.
func (al *AgentLoop) selectCandidates(
	agent *AgentInstance,
	userMsg string,
	history []providers.Message,
) (candidates []providers.FallbackCandidate, model string) {
	return agent.Candidates, agent.Model
}

// maybeSummarize triggers summarization if the session history exceeds thresholds.
func (al *AgentLoop) maybeSummarize(agent *AgentInstance, sessionKey, channel, chatID string) {
	newHistory := agent.Sessions.GetHistory(sessionKey)
	tokenEstimate := al.estimateTokens(newHistory)
	threshold := agent.ContextWindow * agent.SummarizeTokenPercent / 100

	if len(newHistory) > agent.SummarizeMessageThreshold || tokenEstimate > threshold {
		summarizeKey := agent.ID + ":" + sessionKey
		if _, loading := al.summarizing.LoadOrStore(summarizeKey, true); !loading {
			go func() {
				defer al.summarizing.Delete(summarizeKey)
				logger.Debug("Memory threshold reached. Optimizing conversation history...")
				al.summarizeSession(agent, sessionKey)
			}()
		}
	}
}

// forceCompression aggressively reduces context when the limit is hit.
// It drops the oldest 50% of messages (keeping system prompt and last user message).
func (al *AgentLoop) forceCompression(agent *AgentInstance, sessionKey string) {
	history := agent.Sessions.GetHistory(sessionKey)
	if len(history) <= 4 {
		return
	}

	// Keep system prompt (usually [0]) and the very last message (user's trigger)
	// We want to drop the oldest half of the *conversation*
	// Assuming [0] is system, [1:] is conversation
	conversation := history[1 : len(history)-1]
	if len(conversation) == 0 {
		return
	}

	// Helper to find the mid-point of the conversation
	mid := len(conversation) / 2

	// New history structure:
	// 1. System Prompt (with compression note appended)
	// 2. Second half of conversation
	// 3. Last message

	droppedCount := mid
	keptConversation := conversation[mid:]

	newHistory := make([]providers.Message, 0, 1+len(keptConversation)+1)

	// Append compression note to the original system prompt instead of adding a new system message
	// This avoids having two consecutive system messages which some APIs (like Zhipu) reject
	compressionNote := fmt.Sprintf(
		"\n\n[System Note: Emergency compression dropped %d oldest messages due to context limit]",
		droppedCount,
	)
	enhancedSystemPrompt := history[0]
	enhancedSystemPrompt.Content = enhancedSystemPrompt.Content + compressionNote
	newHistory = append(newHistory, enhancedSystemPrompt)

	newHistory = append(newHistory, keptConversation...)
	newHistory = append(newHistory, history[len(history)-1]) // Last message

	// Update session
	agent.Sessions.SetHistory(sessionKey, newHistory)
	agent.Sessions.Save(sessionKey)

	logger.WarnCF("agent", "Forced compression executed", map[string]any{
		"session_key":  sessionKey,
		"dropped_msgs": droppedCount,
		"new_count":    len(newHistory),
	})
}

// GetStartupInfo returns information about loaded tools and skills for logging.
func (al *AgentLoop) GetStartupInfo() map[string]any {
	info := make(map[string]any)

	registry := al.GetRegistry()
	agent := registry.GetDefaultAgent()
	if agent == nil {
		return info
	}

	// Tools info
	toolsList := agent.Tools.List()
	info["tools"] = map[string]any{
		"count": len(toolsList),
		"names": toolsList,
	}

	// Skills info
	info["skills"] = agent.ContextBuilder.GetSkillsInfo()

	// Agents info
	info["agents"] = map[string]any{
		"count": len(registry.ListAgentIDs()),
		"ids":   registry.ListAgentIDs(),
	}

	return info
}

// formatMessagesForLog formats messages for logging
func formatMessagesForLog(messages []providers.Message) string {
	if len(messages) == 0 {
		return "[]"
	}

	var sb strings.Builder
	sb.WriteString("[\n")
	for i, msg := range messages {
		fmt.Fprintf(&sb, "  [%d] Role: %s\n", i, msg.Role)
		if len(msg.ToolCalls) > 0 {
			sb.WriteString("  ToolCalls:\n")
			for _, tc := range msg.ToolCalls {
				fmt.Fprintf(&sb, "    - ID: %s, Type: %s, Name: %s\n", tc.ID, tc.Type, tc.Name)
				if tc.Function != nil {
					fmt.Fprintf(
						&sb,
						"      Arguments: %s\n",
						utils.Truncate(tc.Function.Arguments, 200),
					)
				}
			}
		}
		if msg.Content != "" {
			content := utils.Truncate(msg.Content, 200)
			fmt.Fprintf(&sb, "  Content: %s\n", content)
		}
		if msg.ToolCallID != "" {
			fmt.Fprintf(&sb, "  ToolCallID: %s\n", msg.ToolCallID)
		}
		sb.WriteString("\n")
	}
	sb.WriteString("]")
	return sb.String()
}

// formatToolsForLog formats tool definitions for logging
func formatToolsForLog(toolDefs []providers.ToolDefinition) string {
	if len(toolDefs) == 0 {
		return "[]"
	}

	var sb strings.Builder
	sb.WriteString("[\n")
	for i, tool := range toolDefs {
		fmt.Fprintf(&sb, "  [%d] Type: %s, Name: %s\n", i, tool.Type, tool.Function.Name)
		fmt.Fprintf(&sb, "      Description: %s\n", tool.Function.Description)
		if len(tool.Function.Parameters) > 0 {
			fmt.Fprintf(
				&sb,
				"      Parameters: %s\n",
				utils.Truncate(fmt.Sprintf("%v", tool.Function.Parameters), 200),
			)
		}
	}
	sb.WriteString("]")
	return sb.String()
}

// summarizeSession summarizes the conversation history for a session.
func (al *AgentLoop) summarizeSession(agent *AgentInstance, sessionKey string) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	history := agent.Sessions.GetHistory(sessionKey)
	summary := agent.Sessions.GetSummary(sessionKey)

	// Keep last 4 messages for continuity
	if len(history) <= 4 {
		return
	}

	toSummarize := history[:len(history)-4]

	// Oversized Message Guard
	maxMessageTokens := agent.ContextWindow / 2
	validMessages := make([]providers.Message, 0)
	omitted := false

	for _, m := range toSummarize {
		if m.Role != "user" && m.Role != "assistant" {
			continue
		}
		msgTokens := len(m.Content) / 2
		if msgTokens > maxMessageTokens {
			omitted = true
			continue
		}
		validMessages = append(validMessages, m)
	}

	if len(validMessages) == 0 {
		return
	}

	const (
		maxSummarizationMessages = 10
		llmMaxRetries            = 3
		llmTemperature           = 0.3
		fallbackMaxContentLength = 200
	)

	// Multi-Part Summarization
	var finalSummary string
	if len(validMessages) > maxSummarizationMessages {
		mid := len(validMessages) / 2

		mid = al.findNearestUserMessage(validMessages, mid)

		part1 := validMessages[:mid]
		part2 := validMessages[mid:]

		s1, _ := al.summarizeBatch(ctx, agent, part1, "")
		s2, _ := al.summarizeBatch(ctx, agent, part2, "")

		mergePrompt := fmt.Sprintf(
			"Merge these two conversation summaries into one cohesive summary:\n\n1: %s\n\n2: %s",
			s1,
			s2,
		)

		resp, err := al.retryLLMCall(ctx, agent, mergePrompt, llmMaxRetries)
		if err == nil && resp.Content != "" {
			finalSummary = resp.Content
		} else {
			finalSummary = s1 + " " + s2
		}
	} else {
		finalSummary, _ = al.summarizeBatch(ctx, agent, validMessages, summary)
	}

	if omitted && finalSummary != "" {
		finalSummary += "\n[Note: Some oversized messages were omitted from this summary for efficiency.]"
	}

	if finalSummary != "" {
		agent.Sessions.SetSummary(sessionKey, finalSummary)
		agent.Sessions.TruncateHistory(sessionKey, 4)
		agent.Sessions.Save(sessionKey)
	}
}

// findNearestUserMessage finds the nearest user message to the given index.
// It searches backward first, then forward if no user message is found.
func (al *AgentLoop) findNearestUserMessage(messages []providers.Message, mid int) int {
	originalMid := mid

	for mid > 0 && messages[mid].Role != "user" {
		mid--
	}

	if messages[mid].Role == "user" {
		return mid
	}

	mid = originalMid
	for mid < len(messages) && messages[mid].Role != "user" {
		mid++
	}

	if mid < len(messages) {
		return mid
	}

	return originalMid
}

// retryLLMCall calls the LLM with retry logic.
func (al *AgentLoop) retryLLMCall(
	ctx context.Context,
	agent *AgentInstance,
	prompt string,
	maxRetries int,
) (*providers.LLMResponse, error) {
	const (
		llmTemperature = 0.3
	)

	var resp *providers.LLMResponse
	var err error

	for attempt := 0; attempt < maxRetries; attempt++ {
		al.activeRequests.Add(1)
		resp, err = func() (*providers.LLMResponse, error) {
			defer al.activeRequests.Done()
			return agent.Provider.Chat(
				ctx,
				[]providers.Message{{Role: "user", Content: prompt}},
				nil,
				agent.Model,
				map[string]any{
					"max_tokens":       agent.MaxTokens,
					"temperature":      llmTemperature,
					"prompt_cache_key": agent.ID,
				},
			)
		}()

		if err == nil && resp != nil && resp.Content != "" {
			return resp, nil
		}
		if attempt < maxRetries-1 {
			time.Sleep(time.Duration(attempt+1) * 100 * time.Millisecond)
		}
	}

	return resp, err
}

// summarizeBatch summarizes a batch of messages.
func (al *AgentLoop) summarizeBatch(
	ctx context.Context,
	agent *AgentInstance,
	batch []providers.Message,
	existingSummary string,
) (string, error) {
	const (
		llmMaxRetries             = 3
		llmTemperature            = 0.3
		fallbackMinContentLength  = 200
		fallbackMaxContentPercent = 10
	)

	var sb strings.Builder
	sb.WriteString(
		"Provide a concise summary of this conversation segment, preserving core context and key points.\n",
	)
	if existingSummary != "" {
		sb.WriteString("Existing context: ")
		sb.WriteString(existingSummary)
		sb.WriteString("\n")
	}
	sb.WriteString("\nCONVERSATION:\n")
	for _, m := range batch {
		fmt.Fprintf(&sb, "%s: %s\n", m.Role, m.Content)
	}
	prompt := sb.String()

	response, err := al.retryLLMCall(ctx, agent, prompt, llmMaxRetries)
	if err == nil && response.Content != "" {
		return strings.TrimSpace(response.Content), nil
	}

	var fallback strings.Builder
	fallback.WriteString("Conversation summary: ")
	for i, m := range batch {
		if i > 0 {
			fallback.WriteString(" | ")
		}
		content := strings.TrimSpace(m.Content)
		runes := []rune(content)
		if len(runes) == 0 {
			fallback.WriteString(fmt.Sprintf("%s: ", m.Role))
			continue
		}

		keepLength := len(runes) * fallbackMaxContentPercent / 100
		if keepLength < fallbackMinContentLength {
			keepLength = fallbackMinContentLength
		}

		if keepLength > len(runes) {
			keepLength = len(runes)
		}

		content = string(runes[:keepLength])
		if keepLength < len(runes) {
			content += "..."
		}
		fallback.WriteString(fmt.Sprintf("%s: %s", m.Role, content))
	}
	return fallback.String(), nil
}

// estimateTokens estimates the number of tokens in a message list.
// Uses a safe heuristic of 2.5 characters per token to account for CJK and other
// overheads better than the previous 3 chars/token.
func (al *AgentLoop) estimateTokens(messages []providers.Message) int {
	totalChars := 0
	for _, m := range messages {
		totalChars += utf8.RuneCountInString(m.Content)
	}
	// 2.5 chars per token = totalChars * 2 / 5
	return totalChars * 2 / 5
}

func (al *AgentLoop) handleCommand(
	ctx context.Context,
	msg InboundMessage,
	agent *AgentInstance,
	opts *processOptions,
) (string, bool) {
	if !commands.HasCommandPrefix(msg.Content) {
		return "", false
	}

	if al.cmdRegistry == nil {
		return "", false
	}

	rt := al.buildCommandsRuntime(agent, opts)
	executor := commands.NewExecutor(al.cmdRegistry, rt)

	var commandReply string
	result := executor.Execute(ctx, commands.Request{
		Channel:  msg.Channel,
		ChatID:   msg.ChatID,
		SenderID: msg.SenderID,
		Text:     msg.Content,
		Reply: func(text string) error {
			commandReply = text
			return nil
		},
	})

	switch result.Outcome {
	case commands.OutcomeHandled:
		if result.Err != nil {
			return mapCommandError(result), true
		}
		if commandReply != "" {
			return commandReply, true
		}
		return "", true
	default: // OutcomePassthrough — let the message fall through to LLM
		return "", false
	}
}

func (al *AgentLoop) buildCommandsRuntime(agent *AgentInstance, opts *processOptions) *commands.Runtime {
	registry := al.GetRegistry()
	cfg := al.GetConfig()
	rt := &commands.Runtime{
		Config:          cfg,
		ListAgentIDs:    registry.ListAgentIDs,
		ListDefinitions: al.cmdRegistry.Definitions(),
		// Channel management features removed
		GetEnabledChannels: func() []string { return nil },
		SwitchChannel: func(value string) error {
			return fmt.Errorf("channel switching not supported")
		},
	}
	if agent != nil {
		rt.GetModelInfo = func() (string, string) {
			return agent.Model, cfg.Agents.Defaults.Provider
		}
		rt.SwitchModel = func(value string) (string, error) {
			oldModel := agent.Model
			agent.Model = value
			return oldModel, nil
		}

		rt.ClearHistory = func() error {
			if opts == nil {
				return fmt.Errorf("process options not available")
			}
			if agent.Sessions == nil {
				return fmt.Errorf("sessions not initialized for agent")
			}

			agent.Sessions.SetHistory(opts.SessionKey, make([]providers.Message, 0))
			agent.Sessions.SetSummary(opts.SessionKey, "")
			agent.Sessions.Save(opts.SessionKey)
			return nil
		}
	}
	return rt
}

func mapCommandError(result commands.ExecuteResult) string {
	if result.Command == "" {
		return fmt.Sprintf("Failed to execute command: %v", result.Err)
	}
	return fmt.Sprintf("Failed to execute /%s: %v", result.Command, result.Err)
}

// extractPeer extracts the routing peer from the inbound message's structured Peer field.
func extractPeer(msg InboundMessage) *routing.RoutePeer {
	if msg.Peer.Kind == "" {
		return nil
	}
	peerID := msg.Peer.ID
	if peerID == "" {
		if msg.Peer.Kind == "direct" {
			peerID = msg.SenderID
		} else {
			peerID = msg.ChatID
		}
	}
	return &routing.RoutePeer{Kind: msg.Peer.Kind, ID: peerID}
}

func inboundMetadata(msg InboundMessage, key string) string {
	if msg.Metadata == nil {
		return ""
	}
	if val, ok := msg.Metadata[key]; ok {
		if str, ok := val.(string); ok {
			return str
		}
	}
	return ""
}

// extractParentPeer extracts the parent peer (reply-to) from inbound message metadata.
func extractParentPeer(msg InboundMessage) *routing.RoutePeer {
	parentKind := inboundMetadata(msg, metadataKeyParentPeerKind)
	parentID := inboundMetadata(msg, metadataKeyParentPeerID)
	if parentKind == "" || parentID == "" {
		return nil
	}
	return &routing.RoutePeer{Kind: parentKind, ID: parentID}
}

// Helper to extract provider from registry for cleanup
func extractProvider(registry *AgentRegistry) (providers.LLMProvider, bool) {
	if registry == nil {
		return nil, false
	}
	// Get any agent to access the provider
	defaultAgent := registry.GetDefaultAgent()
	if defaultAgent == nil {
		return nil, false
	}
	return defaultAgent.Provider, true
}

// resolveMediaRefs resolves media:// refs in messages to base64 data URLs.
// Feature removed: mediaStore no longer available.
func resolveMediaRefs(messages []providers.Message, store interface{}, maxSize int) []providers.Message {
	// No-op: mediaStore removed
	return messages
}
