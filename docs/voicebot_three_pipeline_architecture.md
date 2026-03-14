# VoiceBot Architecture: Three Pipelines + Event Bus

## 1. Overview

This document summarizes a scalable **VoiceBot runtime architecture**
based on:

-   **Three independent pipelines**
-   **An event-driven communication model**
-   **A centralized Session context**

The goal of this architecture is to:

-   Reduce coupling between modules
-   Support real-time voice interaction
-   Allow flexible extension (skills, multiple agents, streaming
    LLM/TTS)
-   Support interruption and backchannel handling

------------------------------------------------------------------------

# 2. Core Architecture

The system can be abstracted as:

    AudioProcessor  →  ConversationManager  →  Agent
            \              |                 /
             \             |                /
                    Event Bus

Each pipeline is responsible for a specific stage of the voice
interaction.

------------------------------------------------------------------------

# 3. Pipeline 1: AudioProcessor

## Responsibilities

The **AudioProcessor** handles all low-level audio processing.

### Components

-   VAD (Voice Activity Detection)
-   ASR (Speech Recognition)
-   Audio stream processing

### Generated Events

The AudioProcessor publishes events to the EventBus:

    VAD_START
    VAD_END
    ASR_PARTIAL
    ASR_FINAL

These events represent **user speech activity and transcription**.

------------------------------------------------------------------------

# 4. Pipeline 2: ConversationManager

## Responsibilities

The **ConversationManager** is the brain of the dialogue flow.

It manages:

-   Turn-taking
-   Agent/User state
-   Backchannel detection
-   Interruption handling
-   Agent command generation

### Turn State

    UserTurn
    AgentTurn

### Agent Phase

    AgentProcessing   (LLM + TTS synthesis)
    AgentSpeaking     (TTS playback)

### Events Consumed

From EventBus:

    VAD_START
    ASR_PARTIAL
    ASR_FINAL
    PlaybackFinished

### Commands Produced

    CmdStartAgent
    CmdPausePlayback
    CmdStopPlayback
    CmdCancelAgent

These commands are sent to the Agent pipeline through the EventBus.

------------------------------------------------------------------------

# 5. Pipeline 3: Agent

The **Agent pipeline** handles response generation and playback.

## AgentInstance

Responsibilities:

-   Skill routing
-   Provider routing (LLM / TTS providers)
-   System prompt generation
-   Dynamic prompt variables
-   Interaction with Session context

## AgentLoop

The AgentLoop runs one **iteration of the agent execution cycle**:

    1. Fetch context from Session
    2. Call LLM
    3. Generate response text
    4. Run TTS synthesis
    5. Start audio playback

AgentLoop responds to commands from ConversationManager.

------------------------------------------------------------------------

# 6. Session Layer (Global Context)

The **Session layer maintains conversation context**.

Both ConversationManager and Agent read from Session.

### Session Responsibilities

-   Store dialogue history
-   Track played content
-   Provide recent turns for backchannel detection
-   Provide full context for LLM prompts

### Example Interface

``` go
type Session interface {
    GetRecentTurns(n int) []Turn
    GetFullContext() []Turn
    AppendUserTurn(text string)
    CommitAgentContent(text string)
}
```

------------------------------------------------------------------------

# 7. Event Bus

The EventBus connects all pipelines.

    AudioProcessor
    ConversationManager
    AgentLoop
    PlaybackController
    Session

All communication is done through events.

### Example Events

Audio events:

    VAD_START
    VAD_END
    ASR_PARTIAL
    ASR_FINAL

Conversation commands:

    CmdStartAgent
    CmdPausePlayback
    CmdStopPlayback
    CmdCancelAgent

Playback events:

    PlaybackStarted
    PlaybackPaused
    PlaybackFinished

------------------------------------------------------------------------

# 8. Backchannel Handling

Backchannel refers to short feedback signals from the user:

Examples:

    嗯
    对
    好
    yeah
    uh-huh

ConversationManager uses:

-   Recent dialogue turns
-   ASR text
-   Speech duration
-   Timing signals

To classify input as:

    BACKCHANNEL_ACK
    INTERRUPT
    NEW_TURN
    IGNORE_NOISE

------------------------------------------------------------------------

# 9. Playback Control

PlaybackController manages:

    pause
    resume
    stop

Typical flow:

    AgentSpeaking
          ↓
    User starts speaking
          ↓
    CmdPausePlayback
          ↓
    Backchannel decision
          ↓
    Resume OR Stop

------------------------------------------------------------------------

# 10. Advantages of This Architecture

### Clear separation of concerns

    Audio → Dialogue Control → Agent Execution

### Event-driven decoupling

Modules communicate through events rather than direct calls.

### Natural conversation

Supports:

-   barge-in
-   backchannel
-   interruption

### Extensibility

Easy to add:

-   skills
-   multiple agents
-   streaming LLM
-   streaming TTS

------------------------------------------------------------------------

# 11. Final Architecture Summary

                    +-------------------+
                    |       Session      |
                    |  Context Manager   |
                    +----------+--------+
                               |
                               |
    +--------------+    +------+-------+    +-------------+
    | AudioProcessor| → |ConversationMgr| → |    Agent     |
    +--------------+    +------+-------+    +-------------+
                              |
                              |
                          EventBus

Three pipelines + event bus form a **clean, scalable VoiceBot runtime
architecture**.
