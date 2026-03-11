# Real‑Time Voice Agent Speech System Design

Author: ChatGPT\
Date: 2026-03-09

------------------------------------------------------------------------

# 1. Overview

This document describes a **complete architecture for a real‑time Voice
Agent speech output system**, focusing on the design of the following
components:

-   Speech Scheduler
-   Sentence Splitter
-   TTS Provider
-   TTS Session
-   Audio Stream
-   Audio Player

The design goal is to support:

-   Low‑latency streaming speech
-   Controlled TTS generation
-   Interruptible playback
-   Stable resource usage
-   Flexible provider integration

This architecture is suitable for:

-   Voice assistants
-   AI companions
-   Realtime LLM voice agents
-   Interactive robotics
-   Conversational interfaces

------------------------------------------------------------------------

# 2. High Level Architecture

    User Speech
        │
        ▼
    ASR (Speech Recognition)
        │
        ▼
    LLM (Text Generation)
        │
        ▼
    Speech Scheduler
        │
        ├── Sentence Splitter
        ├── Text Queue
        ├── TTS Controller
        │       │
        │       ├── Provider
        │       └── Session
        │
        ├── Stream Manager
        │
        └── Audio Player
                │
                ▼
             Speaker

The **Speech Scheduler** orchestrates the entire speech pipeline.

------------------------------------------------------------------------

# 3. Design Goals

## 3.1 Low Latency

The system should start playing audio **before the entire response is
generated**.

LLM → partial sentence → TTS → stream → playback

## 3.2 Interruptibility

Users must be able to interrupt the AI at any moment.

Interrupt sources:

-   Voice activity detection
-   ASR partial results
-   UI events

Interrupt behavior:

-   stop playback
-   cancel synthesis
-   clear queues

## 3.3 Resource Control

Uncontrolled systems can accumulate dozens of queued TTS jobs.

Scheduler limits:

-   synthesis concurrency
-   audio backlog
-   sentence window

## 3.4 Modular Provider Support

The architecture supports multiple TTS providers.

Examples:

-   OpenAI TTS
-   Azure Speech
-   ElevenLabs
-   Local models
-   Edge TTS engines

------------------------------------------------------------------------

# 4. Core Component: Speech Scheduler

The **Speech Scheduler** is the orchestrator of the speech system.

Responsibilities:

-   receive text from LLM
-   split sentences
-   manage synthesis queue
-   control TTS concurrency
-   handle interruptions
-   schedule audio playback

Conceptually:

    SpeechScheduler = Speech Orchestrator

------------------------------------------------------------------------

# 5. Scheduler Inputs

The scheduler receives two main types of events.

## 5.1 Text Input

Text arrives from LLM generation.

Example API:

    SubmitText(text string)

Source:

-   token streaming
-   completed sentence
-   system messages

Example:

    你好
    你好，我可以帮你查询天气。

## 5.2 Interrupt Event

Triggered when the user starts speaking.

API:

    Interrupt()

Interrupt behavior:

    player.stop()
    cancel active TTS session
    clear waiting streams
    clear text queue

------------------------------------------------------------------------

# 6. Scheduler Internal Modules

The scheduler consists of the following modules.

    SpeechScheduler
    │
    ├── Sentence Splitter
    ├── Text Queue
    ├── TTS Controller
    │       │
    │       ├── Provider
    │       └── Session
    │
    ├── Stream Manager
    │
    └── Player

------------------------------------------------------------------------

# 7. Sentence Splitter

LLM token streams produce incomplete fragments.

Example stream:

    你好
    你好，我
    你好，我可以
    你好，我可以帮

The splitter converts token streams into **stable sentences**.

Rules may include:

-   punctuation
-   minimum characters
-   pause timing
-   token thresholds

Example output:

    你好，
    你好，我可以帮你查询天气。

Configuration example:

    min_chars = 6
    max_chars = 120
    flush_timeout = 500ms

------------------------------------------------------------------------

# 8. Text Queue

The scheduler maintains a **bounded text queue**.

Purpose:

-   avoid uncontrolled backlog
-   control latency
-   allow cancellation

Example queue state:

    [playing]
    [waiting]
    [pending]

Recommended window:

    max_sentences = 2 or 3

Example:

    Playing  : sentence1
    Waiting  : sentence2
    Pending  : sentence3

------------------------------------------------------------------------

# 9. TTS Provider Design

A provider represents a speech engine.

Interface example:

    type TTSProvider interface {
        CreateSession() (TTSSession, error)
    }

Responsibilities:

-   authenticate service
-   configure voice
-   create synthesis sessions

Providers should be **stateless factories**.

Examples:

-   cloud providers
-   local models
-   embedded engines

------------------------------------------------------------------------

# 10. TTS Session

A **TTS Session** represents one synthesis job.

Example interface:

    type TTSSession interface {
        Start(text string) (AudioStream, error)
        Cancel()
    }

Responsibilities:

-   send text to engine
-   receive audio chunks
-   manage synthesis lifecycle

A session typically maps to:

    1 sentence → 1 session

------------------------------------------------------------------------

# 11. Audio Stream

The TTS session outputs an **audio stream**.

    AudioStream
        │
        ├── chunk1
        ├── chunk2
        ├── chunk3
        └── ...

Stream interface:

    type AudioStream interface {
        ReadChunk() ([]byte, error)
        Close()
    }

Features:

-   streaming playback
-   partial buffering
-   cancellation support

------------------------------------------------------------------------

# 12. Stream Manager

The stream manager coordinates audio streams.

Responsibilities:

-   manage playing stream
-   manage waiting stream
-   discard cancelled streams

Example states:

    Playing Stream
    Waiting Stream
    Synthesizing Stream

Recommended limit:

    max_audio_window = 2

This prevents excessive audio backlog.

------------------------------------------------------------------------

# 13. Audio Player

The player consumes audio streams and outputs audio to the speaker.

    AudioStream → Player → Speaker

Interface example:

    type Player interface {
        Play(stream AudioStream)
        Stop()
    }

Responsibilities:

-   decode audio chunks
-   buffer audio
-   send to hardware output

------------------------------------------------------------------------

# 14. Scheduler State Machine

Typical runtime state:

    playing        = 1
    synthesizing   = 1
    waiting        = 1

Example flow:

    Sentence1 → playing
    Sentence2 → waiting
    Sentence3 → pending

When playing finishes:

    waiting → playing
    pending → synthesizing

------------------------------------------------------------------------

# 15. Interrupt Handling

Interrupt events must immediately stop speech.

Steps:

1.  Stop player
2.  Cancel active synthesis
3.  Clear audio streams
4.  Clear text queue

After interrupt:

    system ready for next response

------------------------------------------------------------------------

# 16. Concurrency Control

Recommended limits:

    max_synthesis = 1
    max_playing = 1
    max_waiting = 1

Result:

    max sentences in system = 3

Benefits:

-   predictable latency
-   bounded memory
-   stable provider load

------------------------------------------------------------------------

# 17. Optional Advanced Features

## 17.1 Text Merge

Merge tokens within a short window.

Example:

    merge_window = 400ms

Reduces excessive short sentences.

## 17.2 Priority Messages

Example priority:

    System alert
    User response
    Background speech

Scheduler can preempt playback.

## 17.3 Provider Failover

If provider fails:

    fallback → secondary provider

## 17.4 Voice Switching

Session configuration may include:

    voice
    speed
    emotion
    language

------------------------------------------------------------------------

# 18. Example Go Interfaces

Example simplified interfaces.

    type SpeechScheduler interface {
        SubmitText(text string)
        Interrupt()
        Close()
    }

Internal structure:

    type Scheduler struct {
        splitter SentenceSplitter
        queue TextQueue
        provider TTSProvider
        player Player
    }

------------------------------------------------------------------------

# 19. Recommended Runtime Limits

    Sentence window: 2–3
    Synthesis concurrency: 1
    Playback streams: 1
    Queue size: 3–5

This configuration provides good performance for real‑time agents.

------------------------------------------------------------------------

# 20. Summary

This architecture introduces a **Speech Scheduler** as the central
orchestrator of speech synthesis and playback.

Key principles:

-   bounded audio window
-   streaming synthesis
-   fast interruption
-   modular providers
-   controlled concurrency

Final pipeline:

    LLM
     │
     ▼
    Sentence Splitter
     │
     ▼
    Speech Scheduler
     │
     ├── Provider
     ├── Session
     ├── Stream
     └── Player
     │
     ▼
    Speaker

This design enables **stable, low‑latency, interruptible voice AI
systems** suitable for modern real‑time conversational agents.

# 接口设计
``` go
type TTSProvider interface {
    NewSession(ctx context.Context) (TTSSession, error)

}

type TTSSession interface {
    SendText(text) error
    RecvAudio() []byte, error
}

func pumpAudio(session TTSSession, stream AudioStream) {
	defer stream.Close()

	for {
		data, err := session.RecvAudio()

		if err != nil {
			if err == io.EOF {
				return
			}

			stream.Error(err)
			return
		}

		stream.Push(data)
	}
}
```