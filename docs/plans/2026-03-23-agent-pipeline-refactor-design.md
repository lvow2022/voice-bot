# AgentPipeline Refactor: Reuse AgentLoop Logic

## Overview

Refactor `pkg/pipeline/agent_pipeline.go` to reuse `AgentLoop` logic instead of directly calling `AgentInstance.Provider.Chat()`. This enables full agent capabilities (tools, sessions, context, fallback) in voice conversations with streaming TTS integration.

## Design Decisions

| Decision | Choice |
|----------|--------|
| Capability level | Full AgentLoop (tools, sessions, context, fallback, summarization) |
| Tool execution | Keep current logic unchanged |
| AgentLoop lifecycle | Per-session (one per WebSocket connection) |
| TTS integration | Provider-level streaming, chunks feed directly to TTS |
| Session key format | `voice:<connectionID>` |

## Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                    Voice WebSocket Session                       │
├─────────────────────────────────────────────────────────────────┤
│  AudioProcessPipeline          AgentPipeline (refactored)        │
│  ┌──────────────────┐          ┌──────────────────────────────┐ │
│  │ ASR → TextFrame  │ ──────▶  │         AgentLoop            │ │
│  └──────────────────┘          │  ┌────────────────────────┐  │ │
│                                │  │ ProcessDirectStream()  │  │ │
│                                │  │   ↓                    │  │ │
│                                │  │ Provider.ChatStream()  │  │ │
│                                │  │   ↓                    │  │ │
│                                │  │ OnChunk → Scheduler    │  │ │
│                                │  └────────────────────────┘  │ │
│                                └──────────────────────────────┘ │
└─────────────────────────────────────────────────────────────────┘
```

## Component Changes

### 1. AgentLoop Streaming Extension

**File:** `pkg/agent/loop.go`

```go
// StreamCallbacks 流式响应回调
type StreamCallbacks struct {
    OnChunk func(chunk string)      // 每个 token/chunk 调用
    OnDone  func(fullText string)   // 完成时调用
}

// ProcessDirectStream 流式处理消息
func (al *AgentLoop) ProcessDirectStream(
    ctx context.Context,
    content, sessionKey, channel, chatID string,
    callbacks StreamCallbacks,
) (string, error)
```

**Streaming flow:**
1. `ProcessDirectStream()` calls `Provider.ChatStream()` instead of `Provider.Chat()`
2. For each chunk → `callbacks.OnChunk(chunk)` → `scheduler.Feed(chunk)`
3. On completion → `callbacks.OnDone(fullText)` → `scheduler.Flush()`
4. Tool calls execute synchronously, then continue streaming

### 2. AgentPipeline Refactoring

**File:** `pkg/pipeline/agent_pipeline.go`

```go
type AgentPipelineOptions struct {
    AgentLoop    *agent.AgentLoop     // 优先使用
    TTSSession   *tts.TtsSession
    StreamPlayer *stream.StreamPlayer
    SpeechConfig speech.Config

    // 可选：仅在 AgentLoop 为 nil 时使用
    AgentInstance *agent.AgentInstance
    Config        *config.Config
}

type agentProcessor struct {
    opts       AgentPipelineOptions
    agentLoop  *agent.AgentLoop
    scheduler  *speech.Scheduler
    ctx        context.Context
    cancel     context.CancelFunc
}
```

**Lifecycle:**
- `OnBegin()` - Create `AgentLoop` if not provided; start `Scheduler`
- `OnExecute()` - Call `agentLoop.ProcessDirectStream()` with streaming callbacks
- `OnEnd()` - Close `AgentLoop` and `Scheduler`

### 3. PipelineBuilder Changes

**File:** `pkg/server/pipeline_builder.go`

- Accept `AgentLoop` or creation parameters (`Config`, `Provider`)
- Pass to `AgentPipeline` instead of just `AgentInstance`

## Data Flow

```
TextFrame (ASR final)
    │
    ▼
AgentLoop.ProcessDirectStream(ctx, text, sessionKey, channel, chatID, callbacks)
    │
    ├── BuildMessages (history + summary + system prompt)
    │
    ├── Provider.ChatStream(messages, tools, model, opts)
    │       │
    │       ├── OnChunk(chunk) → scheduler.Feed(chunk)  [实时 TTS]
    │       │
    │       └── ToolCall? → ExecuteTool() → continue streaming
    │
    └── OnDone(fullText) → scheduler.Flush() → WaitPlayback()
```

## Error Handling

| Error Type | Handling |
|------------|----------|
| Context canceled (user hangup) | `scheduler.Reset()`, return immediately |
| Provider timeout | Fallback chain tries next model |
| All providers fail | Emit error event, TTS speaks fallback message |
| Tool execution error | Return error as tool result, LLM decides next step |
| Context window exceeded | `forceCompression()`, retry with shorter history |

## Implementation Files

| File | Action | Changes |
|------|--------|---------|
| `pkg/agent/loop.go` | Modify | Add `StreamCallbacks`, `ProcessDirectStream()` |
| `pkg/providers/interface.go` | Modify | Add `ChatStream()` if not exists |
| `pkg/pipeline/agent_pipeline.go` | Refactor | Use `AgentLoop`, streaming integration |
| `pkg/server/pipeline_builder.go` | Modify | Pass `AgentLoop` or creation params |

## Backward Compatibility

- `ProcessDirect()` remains unchanged
- `AgentPipelineOptions.AgentInstance` still works (creates internal AgentLoop)
- Existing code paths unaffected
