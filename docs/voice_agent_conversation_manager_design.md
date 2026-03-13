# Voice Agent ConversationManager (Go Design)

## Overview

This document describes a **clean architecture for a voice agent
conversation controller**. The design focuses on solving common issues
in voice pipelines such as:

-   VAD false triggers
-   Background speech
-   Backchannel detection ("嗯", "对", "好的")
-   User interruption during TTS playback

The system is divided into three high‑level modules:

    AudioProcess  ->  ConversationManager  ->  Agent(LLM + TTS)

Where:

-   **AudioProcess**
    -   Handles VAD and ASR
    -   Produces audio events
-   **ConversationManager**
    -   Maintains dialogue state
    -   Interprets events
    -   Controls turn‑taking
-   **Agent**
    -   Handles LLM response generation
    -   TTS synthesis and playback

------------------------------------------------------------------------

# Core Architecture

    AudioProcess
         │
         ▼
    AudioEvent
         │
         ▼
    ConversationManager
         │
         ├── EventInterpreter
         │        │
         │        ▼
         │   ConversationEvent
         │
         └── TurnPolicy
                  │
                  ▼
            AgentCommand

ConversationManager consists of three main components:

    ConversationManager
     ├── State
     ├── EventInterpreter
     └── TurnPolicy

------------------------------------------------------------------------

# Data Structures

## Turn State

    type TurnState int

    const (
        TurnUser TurnState = iota
        TurnAgent
    )

## Agent Phase

Agent has two internal phases:

    type AgentPhase int

    const (
        AgentProcessing AgentPhase = iota
        AgentSpeaking
    )

Meaning:

  Phase        Description
  ------------ -----------------------------
  Processing   LLM + TTS synthesis running
  Speaking     TTS audio currently playing

------------------------------------------------------------------------

# Audio Events

Produced by **AudioProcess (VAD + ASR)**

    type AudioEventType int

    const (
        VADStart AudioEventType = iota
        VADStop
        ASRPartial
        ASRFinal
    )

Event structure:

    type AudioEvent struct {
        Type AudioEventType
        Text string
    }

------------------------------------------------------------------------

# Conversation Events

ConversationManager converts raw audio events into semantic events.

    type ConversationEvent int

    const (
        EventIgnore ConversationEvent = iota
        EventBackchannel
        EventInterrupt
        EventNewTurn
    )

Meaning:

  Event         Example
  ------------- --------------------
  Ignore        noise
  Backchannel   "嗯", "对", "好的"
  Interrupt     "等一下"
  NewTurn       "帮我查天气"

------------------------------------------------------------------------

# Agent Commands

Commands sent to the Agent module.

    type AgentCommand int

    const (
        CmdNone AgentCommand = iota
        CmdStartLLM
        CmdStopTTS
        CmdCancelAgent
    )

------------------------------------------------------------------------

# Conversation State

    type ConversationState struct {
        Turn  TurnState
        Phase AgentPhase
    }

Example:

    User speaking
    Turn = TurnUser

    Agent speaking
    Turn = TurnAgent
    Phase = AgentSpeaking

------------------------------------------------------------------------

# Event Interpreter

Responsible for converting ASR results into conversation events.

    type EventInterpreter struct{}

    func (e *EventInterpreter) Interpret(ev AudioEvent) ConversationEvent {

        if ev.Type != ASRFinal {
            return EventIgnore
        }

        text := ev.Text

        switch text {

        case "嗯", "对", "好的":
            return EventBackchannel

        case "等一下", "不用了":
            return EventInterrupt
        }

        return EventNewTurn
    }

In production systems this logic can be replaced by:

-   rules
-   classifier
-   LLM

------------------------------------------------------------------------

# Turn Policy

Decides what to do based on:

    State + ConversationEvent

    type TurnPolicy struct{}

    func (p *TurnPolicy) Decide(
        state *ConversationState,
        event ConversationEvent,
    ) AgentCommand {

        switch state.Turn {

        case TurnAgent:

            if state.Phase == AgentSpeaking {

                if event == EventInterrupt {
                    return CmdStopTTS
                }

                if event == EventBackchannel {
                    return CmdNone
                }
            }

            if state.Phase == AgentProcessing {

                if event == EventNewTurn {
                    return CmdCancelAgent
                }
            }

        case TurnUser:

            if event == EventNewTurn {
                return CmdStartLLM
            }
        }

        return CmdNone
    }

------------------------------------------------------------------------

# Conversation Manager

Main runtime controller.

    type ConversationManager struct {

        state        ConversationState
        interpreter  *EventInterpreter
        policy       *TurnPolicy
    }

Constructor:

    func NewConversationManager() *ConversationManager {

        return &ConversationManager{
            state: ConversationState{
                Turn: TurnUser,
            },
            interpreter: &EventInterpreter{},
            policy:      &TurnPolicy{},
        }
    }

Event handling:

    func (m *ConversationManager) HandleAudioEvent(
        ev AudioEvent,
    ) AgentCommand {

        convEvent := m.interpreter.Interpret(ev)

        cmd := m.policy.Decide(&m.state, convEvent)

        return cmd
    }

------------------------------------------------------------------------

# Full Interaction Flow

    User Speech
         │
         ▼
    VAD + ASR
         │
         ▼
    AudioEvent
         │
         ▼
    ConversationManager
         │
         ▼
    AgentCommand
         │
         ▼
    Agent (LLM + TTS)

Example timeline:

    User: 帮我查天气
    → NEW_TURN
    → StartLLM

    Agent speaking...

    User: 嗯
    → BACKCHANNEL
    → ignore

    User: 等一下
    → INTERRUPT
    → stop_tts

------------------------------------------------------------------------

# Advantages of This Architecture

### 1 Clean separation

    AudioProcess      → speech processing
    ConversationMgr   → dialogue control
    Agent             → response generation

------------------------------------------------------------------------

### 2 Easy to evolve

You can independently improve:

-   backchannel detection
-   interrupt policy
-   turn switching rules

------------------------------------------------------------------------

### 3 Compatible with pipeline + event bus

ConversationManager can run as:

-   a **pipeline stage**
-   or an **event bus consumer**

------------------------------------------------------------------------

# Suggested Production Improvements

### Backchannel detection

Use:

-   keyword lists
-   duration filters
-   small classifier

------------------------------------------------------------------------

### Interrupt confidence

Combine:

-   VAD duration
-   ASR confidence
-   semantic check

------------------------------------------------------------------------

### Partial ASR support

Early interrupt detection can use:

    ASRPartial

instead of waiting for final transcription.

------------------------------------------------------------------------

# Summary

A robust voice agent architecture should separate:

    Audio Processing
    Conversation Control
    Agent Generation

The **ConversationManager** acts as the brain of dialogue control:

    State Machine
    + Event Interpreter
    + Turn Policy

This design avoids unnecessary complexity while still supporting:

-   backchannels
-   interruptions
-   turn‑taking
-   streaming speech interfaces
