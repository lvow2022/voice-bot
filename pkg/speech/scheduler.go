package speech

import (
	"context"
	"sync"
	"time"

	"voicebot/pkg/stream"
	"voicebot/pkg/tts"
)

type Scheduler struct {
	config Config

	splitter   *SentenceSplitter
	ttsSession *tts.TtsSession
	player     *stream.StreamPlayer

	mu              sync.Mutex
	pendingSentence []string

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

func NewScheduler(ttsSession *tts.TtsSession, player *stream.StreamPlayer, config Config) *Scheduler {
	ctx, cancel := context.WithCancel(context.Background())

	return &Scheduler{
		config:          config,
		splitter:        NewSentenceSplitter(config.MinSentenceLen, config.MaxSentenceLen),
		ttsSession:      ttsSession,
		player:          player,
		pendingSentence: make([]string, 0, config.WindowSize()),
		ctx:             ctx,
		cancel:          cancel,
	}
}

func (s *Scheduler) Run() {
	s.player.Run()

	s.wg.Add(1)
	go s.loop()
}

func (s *Scheduler) loop() {
	defer s.wg.Done()

	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.synthesizeNext()
		}
	}
}

func (s *Scheduler) synthesizeNext() {
	if s.player.QueueLength() >= s.config.MaxWaiting {
		return
	}

	s.mu.Lock()
	if len(s.pendingSentence) == 0 {
		s.mu.Unlock()
		return
	}

	sentence := s.pendingSentence[0]
	s.pendingSentence = s.pendingSentence[1:]
	s.mu.Unlock()

	audioStream := stream.NewAudioStream(0)
	audioStream.PushFilter(s.config.Filters...)
	s.player.Enqueue(audioStream)

	err := s.ttsSession.AsyncSynthesize(sentence, audioStream)
	if err != nil {
		_ = audioStream.Push(nil, false)
		return
	}

	<-s.ttsSession.SynthesizeDone()
}

func (s *Scheduler) Feed(token string) {
	if sentence := s.splitter.Write(token); sentence != "" {
		s.mu.Lock()
		s.pendingSentence = append(s.pendingSentence, sentence)
		s.mu.Unlock()
	}
}

func (s *Scheduler) Flush() {
	if sentence := s.splitter.Flush(); sentence != "" {
		s.mu.Lock()
		s.pendingSentence = append(s.pendingSentence, sentence)
		s.mu.Unlock()
	}
}

func (s *Scheduler) Reset() {
	s.player.StopAndClear()

	s.mu.Lock()
	s.pendingSentence = s.pendingSentence[:0]
	s.mu.Unlock()

	s.splitter.Reset()
}

func (s *Scheduler) Close() error {
	s.cancel()
	s.player.Close()
	_ = s.ttsSession.Close()
	s.wg.Wait()
	return nil
}

func (s *Scheduler) IsPlaying() bool {
	return s.player.IsPlaying()
}
