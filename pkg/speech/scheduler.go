package speech

import (
	"context"
	"log"
	"sync"
	"time"

	"voicebot/pkg/player"
	"voicebot/pkg/stream"
	"voicebot/pkg/tts"
)

// Scheduler 语音调度器
type Scheduler struct {
	config Config

	// 组件
	splitter *SentenceSplitter
	engine   tts.Engine
	player   *player.Player

	// 待合成队列
	mu              sync.Mutex
	pendingSentence []string

	// 生命周期
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewScheduler 创建调度器
func NewScheduler(engine tts.Engine, config Config) (*Scheduler, error) {
	ctx, cancel := context.WithCancel(context.Background())

	pl, err := player.NewPlayer(config.SampleRate, config.Channels)
	if err != nil {
		cancel()
		return nil, err
	}

	return &Scheduler{
		config:          config,
		splitter:        NewSentenceSplitter(config.MinSentenceLen, config.MaxSentenceLen),
		engine:          engine,
		player:          pl,
		pendingSentence: make([]string, 0, config.WindowSize()),
		ctx:             ctx,
		cancel:          cancel,
	}, nil
}

// Run 启动调度器
func (s *Scheduler) Run() {
	s.player.Run()

	s.wg.Add(1)
	go s.loop()
}

// loop 主循环：定时检查并合成句子
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

// synthesizeNext 合成下一个待处理的句子
func (s *Scheduler) synthesizeNext() {
	// 等待 player 队列有空位
	if s.player.QueueLength() >= s.config.MaxWaiting {
		return
	}

	// 取出句子
	s.mu.Lock()
	if len(s.pendingSentence) == 0 {
		s.mu.Unlock()
		return
	}
	sentence := s.pendingSentence[0]
	s.pendingSentence = s.pendingSentence[1:]
	s.mu.Unlock()

	// 创建音频流
	audioStream := stream.NewAudioStream(0)
	audioStream.PushFilter(s.config.Filters...)

	// 创建 TTS session
	sess, err := s.engine.NewSession(s.ctx, audioStream)
	if err != nil {
		log.Printf("[Scheduler] 创建 session 失败: %v", err)
		return
	}
	defer sess.Close()

	// 加入播放队列
	s.player.Start(audioStream)

	// 发送文本进行合成
	if err := sess.SendText(sentence, nil); err != nil {
		log.Printf("[Scheduler] 发送文本失败: %v", err)
		return
	}

	// 等待合成完成
	<-sess.Done()
}

// Feed 输入 token，自动分句
func (s *Scheduler) Feed(token string) {
	if sentence := s.splitter.Write(token); sentence != "" {
		s.mu.Lock()
		s.pendingSentence = append(s.pendingSentence, sentence)
		s.mu.Unlock()
	}
}

// Flush 刷新分句器缓冲区
func (s *Scheduler) Flush() {
	if sentence := s.splitter.Flush(); sentence != "" {
		s.mu.Lock()
		s.pendingSentence = append(s.pendingSentence, sentence)
		s.mu.Unlock()
	}
}

// Reset 重置调度器状态，停止播放并清空所有队列
func (s *Scheduler) Reset() {
	s.player.StopAndClear()

	s.mu.Lock()
	s.pendingSentence = s.pendingSentence[:0]
	s.mu.Unlock()

	s.splitter.Reset()
}

// Close 关闭调度器
func (s *Scheduler) Close() error {
	s.cancel()
	s.engine.Close()
	s.player.Shutdown()
	s.wg.Wait()
	return nil
}

// IsPlaying 是否正在播放
func (s *Scheduler) IsPlaying() bool {
	return s.player.IsPlaying()
}
