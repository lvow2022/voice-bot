package speech

import (
	"context"
	"log"
	"sync"
	"time"

	"voicebot/pkg/stream"
	"voicebot/pkg/tts"
)

// Scheduler 语音调度器
type Scheduler struct {
	config Config

	// 组件
	splitter *SentenceSplitter
	engine   tts.Engine
	player   *stream.Player

	// 队列
	queueMu sync.Mutex
	queue   []string

	// 控制
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	// 状态
	mu      sync.Mutex
	running bool
}

// NewScheduler 创建调度器
func NewScheduler(provider tts.Engine, config Config) (*Scheduler, error) {
	ctx, cancel := context.WithCancel(context.Background())

	// 创建播放器
	player, err := stream.NewPlayer(config.SampleRate, config.Channels)
	if err != nil {
		cancel()
		return nil, err
	}

	s := &Scheduler{
		config:   config,
		splitter: NewSentenceSplitter(config.MinSentenceLen, config.MaxSentenceLen),
		engine:   provider,
		player:   player,
		queue:    make([]string, 0, config.WindowSize()),
		ctx:      ctx,
		cancel:   cancel,
	}

	return s, nil
}

// Run 启动调度器
func (s *Scheduler) Run() {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return
	}
	s.running = true
	s.mu.Unlock()

	// 启动播放器
	s.player.Run()

	// 启动工作循环
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.workLoop()
	}()
}

// workLoop 工作循环 - 定期检查 player 队列，补充合成任务
func (s *Scheduler) workLoop() {
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.trySynthesize()
		}
	}
}

// Feed 输入 token，自动分句并入队
func (s *Scheduler) Feed(token string) {
	if sentence := s.splitter.Write(token); sentence != "" {
		s.queueMu.Lock()
		s.queue = append(s.queue, sentence)
		s.queueMu.Unlock()
	}
}

// Flush 刷新分句器缓冲区
func (s *Scheduler) Flush() {
	if sentence := s.splitter.Flush(); sentence != "" {
		s.queueMu.Lock()
		s.queue = append(s.queue, sentence)
		s.queueMu.Unlock()
	}
}

// trySynthesize 尝试开始合成
func (s *Scheduler) trySynthesize() {
	// 检查 player 队列是否有空位
	if s.player.QueueLength() >= s.config.MaxWaiting+1 {
		return
	}

	// 从队列取出句子
	s.queueMu.Lock()
	if len(s.queue) == 0 {
		s.queueMu.Unlock()
		return
	}
	sentence := s.queue[0]
	s.queue = s.queue[1:]
	s.queueMu.Unlock()

	// 开始合成
	go s.synthesize(sentence)
}

// synthesize 执行合成
func (s *Scheduler) synthesize(sentence string) {
	// Step 1: 创建 stream
	strm := stream.NewTtsStream(s.config.MaxStreamSize)

	// Step 2: 加入 player 队列
	s.player.Start(strm)

	// Step 3: 创建 session
	sess, err := s.engine.NewSession(s.ctx)
	if err != nil {
		log.Printf("[Scheduler] Create session failed: %v", err)
		strm.Push(nil, true)
		return
	}

	// Step 4: 启动 audio pump
	go s.pumpAudio(sess, strm)

	// Step 5: 发送文本
	if err := sess.SendText(sentence, nil); err != nil {
		log.Printf("[Scheduler] SendText failed: %v", err)
		strm.Push(nil, true)
		return
	}
}

// pumpAudio 将音频数据从 session 拉取到 stream
func (s *Scheduler) pumpAudio(sess tts.Session, strm *stream.TtsStream) {
	defer strm.Push(nil, true) // EOF

	audioStream := sess.RecvAudio()
	defer audioStream.Close()

	for audioStream.Next() {
		select {
		case <-s.ctx.Done():
			return
		default:
			frame := audioStream.Frame()
			if len(frame.Data) > 0 {
				if err := strm.Push(frame.Data, frame.Final); err != nil {
					log.Printf("[Scheduler] Push error: %v", err)
					return
				}
			}
			if frame.Final {
				return
			}
		}
	}

	if err := audioStream.Error(); err != nil {
		log.Printf("[Scheduler] Stream error: %v", err)
	}
}

// Interrupt 中断当前播放
func (s *Scheduler) Interrupt() {
	s.player.StopAndClear()
	s.queueMu.Lock()
	s.queue = s.queue[:0]
	s.queueMu.Unlock()
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

// QueueLength 待合成队列长度
func (s *Scheduler) QueueLength() int {
	s.queueMu.Lock()
	defer s.queueMu.Unlock()
	return len(s.queue)
}
