package player

import (
	"fmt"
	"sync"
	"time"

	"github.com/gen2brain/malgo"
	"voicebot/pkg/stream"
)

// ============ Player 状态 ============

// State 播放器状态
type State int

const (
	StateIdle    State = iota // 空闲
	StatePlaying              // 播放中
	StatePaused               // 暂停
	StateError                // 错误
)

func (s State) String() string {
	switch s {
	case StateIdle:
		return "idle"
	case StatePlaying:
		return "playing"
	case StatePaused:
		return "paused"
	case StateError:
		return "error"
	default:
		return "unknown"
	}
}

// ============ PlayablePipe（对 Stream 的状态封装）============

// PipeState 管道状态
type PipeState int

const (
	PipeStatePlaying PipeState = iota // 播放中
	PipeStatePaused                   // 暂停
	PipeStateStopped                  // 停止
)

func (s PipeState) String() string {
	switch s {
	case PipeStatePlaying:
		return "playing"
	case PipeStatePaused:
		return "paused"
	case PipeStateStopped:
		return "stopped"
	default:
		return "unknown"
	}
}

// PlayablePipe 对 Stream 进行状态封装
type PlayablePipe struct {
	pipe             stream.Stream // 原始管道
	state            PipeState     // 管道状态
	onPlayCallback   func()        // 开始播放回调
	onPauseCallback  func()        // 暂停回调
	onResumeCallback func()        // 恢复回调
	onStopCallback   func()        // 停止回调
	mu               sync.RWMutex
}

// newPlayablePipe 创建可播放管道
func newPlayablePipe(pipe stream.Stream) *PlayablePipe {
	return &PlayablePipe{
		pipe:  pipe,
		state: PipeStatePlaying,
	}
}

// NewPlayable 创建可播放管道（导出版本，允许设置回调）
func NewPlayable(pipe stream.Stream) *PlayablePipe {
	return newPlayablePipe(pipe)
}

// OnPlay 设置开始播放回调
func (s *PlayablePipe) OnPlay(callback func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onPlayCallback = callback
}

// OnPause 设置暂停回调
func (s *PlayablePipe) OnPause(callback func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onPauseCallback = callback
}

// OnResume 设置恢复回调
func (s *PlayablePipe) OnResume(callback func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onResumeCallback = callback
}

// OnStop 设置停止回调
func (s *PlayablePipe) OnStop(callback func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onStopCallback = callback
}

// Pull 实现 Stream 接口，根据状态决定行为
func (s *PlayablePipe) Pull(buf []byte) (int, error) {
	s.mu.RLock()
	state := s.state
	s.mu.RUnlock()

	switch state {
	case PipeStatePaused:
		return 0, nil // 暂停：返回静音（0 字节）
	case PipeStateStopped:
		return 0, stream.ErrStreamEnded // 停止：返回结束
	default:
		return s.pipe.Pull(buf) // 播放：正常读取
	}
}

// Push 实现 Stream 接口
func (s *PlayablePipe) Push(data []byte, eof bool) error {
	return s.pipe.Push(data, eof)
}

// Play 开始播放（触发 onPlay 回调）
func (s *PlayablePipe) Play() {
	s.mu.RLock()
	callback := s.onPlayCallback
	s.mu.RUnlock()

	if callback != nil {
		callback()
	}
}

// Pause 暂停管道
func (s *PlayablePipe) Pause() {
	s.mu.Lock()
	s.state = PipeStatePaused
	callback := s.onPauseCallback
	s.mu.Unlock()

	if callback != nil {
		callback()
	}
}

// Resume 恢复管道
func (s *PlayablePipe) Resume() {
	s.mu.Lock()
	s.state = PipeStatePlaying
	callback := s.onResumeCallback
	s.mu.Unlock()

	if callback != nil {
		callback()
	}
}

// Stop 停止管道
func (s *PlayablePipe) Stop() {
	s.mu.Lock()
	s.state = PipeStateStopped
	callback := s.onStopCallback
	s.mu.Unlock()

	if callback != nil {
		callback()
	}
}

// State 获取管道状态
func (s *PlayablePipe) State() PipeState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.state
}

// ============ Player 播放器 ============

// Player 音频播放器
type Player struct {
	// ============ 输入 ============
	queue chan *PlayablePipe

	// ============ malgo（只初始化一次）============
	ctx        *malgo.AllocatedContext
	device     *malgo.Device
	deviceInit bool
	sampleRate int
	channels   int
	frameSize  int

	// ============ 共享缓冲区 ============
	buffer  []byte
	bufSize int
	bufMu   sync.RWMutex

	// ============ 当前流管理 ============
	currentPipe *PlayablePipe
	currentMu   sync.RWMutex

	// ============ 生命周期 ============
	done           chan struct{}
	wg             sync.WaitGroup
	once           sync.Once
	shutdownCalled bool
}

// NewPlayer 创建播放器
func NewPlayer(sampleRate, channels int) (*Player, error) {
	ctx, err := malgo.InitContext(nil, malgo.ContextConfig{}, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to init malgo: %w", err)
	}

	frameSize := sampleRate * channels * 2 / 50 // 20ms @ 16-bit

	return &Player{
		queue:      make(chan *PlayablePipe, 10),
		ctx:        ctx,
		sampleRate: sampleRate,
		channels:   channels,
		frameSize:  frameSize,
		buffer:     make([]byte, frameSize),
		done:       make(chan struct{}),
	}, nil
}

// Run 启动播放器（后台运行）
func (p *Player) Run() {
	p.wg.Add(1)
	go p.run()
}

// run 主循环
func (p *Player) run() {
	defer p.wg.Done()

	// 初始化并启动设备
	if err := p.initAndStartDevice(); err != nil {
		return
	}
	defer p.stopDevice()

	// 调度循环
	for {
		select {
		case <-p.done:
			return
		case playable, ok := <-p.queue:
			if !ok {
				return
			}
			p.playStream(playable)
		}
	}
}

// initAndStartDevice 初始化并启动音频设备
func (p *Player) initAndStartDevice() error {
	if err := p.initDevice(); err != nil {
		fmt.Printf("[Player] Failed to init device: %v\n", err)
		return err
	}

	if err := p.device.Start(); err != nil {
		fmt.Printf("[Player] Failed to start device: %v\n", err)
		return err
	}

	fmt.Printf("[Player] Device started (sampleRate=%d, channels=%d, frameSize=%d)\n",
		p.sampleRate, p.channels, p.frameSize)
	return nil
}

// stopDevice 停止音频设备
func (p *Player) stopDevice() {
	if p.device != nil {
		p.device.Stop()
		p.device.Uninit()
		p.deviceInit = false
		fmt.Println("[Player] Device stopped")
	}
}

// initDevice 初始化 malgo 设备（只调用一次）
func (p *Player) initDevice() error {
	p.currentMu.Lock()
	defer p.currentMu.Unlock()

	if p.deviceInit {
		return nil
	}

	deviceConfig := malgo.DefaultDeviceConfig(malgo.Playback)
	deviceConfig.Playback.Format = malgo.FormatS16
	deviceConfig.Playback.Channels = uint32(p.channels)
	deviceConfig.SampleRate = uint32(p.sampleRate)
	deviceConfig.PeriodSizeInMilliseconds = 20 // 20ms

	// 创建回调
	deviceCallbacks := malgo.DeviceCallbacks{
		Data: p.newDataSent(),
	}

	// 创建设备
	device, err := malgo.InitDevice(p.ctx.Context, deviceConfig, deviceCallbacks)
	if err != nil {
		return err
	}

	p.device = device
	p.deviceInit = true
	return nil
}

// playStream 播放单个流
func (p *Player) playStream(playable *PlayablePipe) {
	// 设置当前流
	p.setCurrentStream(playable)
	defer p.clearCurrentStream()

	// 触发开始播放回调
	playable.Play()

	// 喂数据循环
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-p.done:
			playable.Stop()
			return
		case <-ticker.C:
			if playable.State() == PipeStateStopped {
				return
			}
			if err := p.feedStream(playable); err != nil {
				return
			}
		}
	}
}

// setCurrentStream 设置当前播放流
func (p *Player) setCurrentStream(playable *PlayablePipe) {
	p.currentMu.Lock()
	p.currentPipe = playable
	p.currentMu.Unlock()
}

// clearCurrentStream 清除当前播放流
func (p *Player) clearCurrentStream() {
	p.currentMu.Lock()
	p.currentPipe = nil
	p.currentMu.Unlock()
}

// feedStream 从流拉取数据到缓冲区
func (p *Player) feedStream(playable *PlayablePipe) error {
	p.bufMu.Lock()
	n, err := playable.Pull(p.buffer)
	p.bufSize = n
	p.bufMu.Unlock()

	if err == stream.ErrStreamEnded {
		return err
	}
	if err != nil {
		fmt.Printf("[Player] Stream error: %v\n", err)
		return err
	}
	return nil
}

// newDataSent 创建 malgo 数据回调
func (p *Player) newDataSent() func(pOutputSamples, pInputSamples []byte, framecount uint32) {
	return func(pOutputSamples, pInputSamples []byte, framecount uint32) {
		// 检查当前流状态
		p.currentMu.RLock()
		stream := p.currentPipe
		p.currentMu.RUnlock()

		isPaused := stream != nil && stream.State() == PipeStatePaused

		p.bufMu.RLock()
		defer p.bufMu.RUnlock()

		// 暂停或无数据时填充静音
		if isPaused || p.bufSize == 0 {
			for i := range pOutputSamples {
				pOutputSamples[i] = 0
			}
			return
		}

		// 复制数据到输出缓冲区
		n := p.bufSize
		if n > len(pOutputSamples) {
			n = len(pOutputSamples)
		}
		copy(pOutputSamples, p.buffer[:n])

		// 如果数据不足，填充静音
		for i := n; i < len(pOutputSamples); i++ {
			pOutputSamples[i] = 0
		}
	}
}

// Start 添加流到播放队列
func (p *Player) Start(streamer stream.Stream) {
	playable := newPlayablePipe(streamer)
	select {
	case p.queue <- playable:
		fmt.Println("[Player] Stream added to queue")
	case <-time.After(1 * time.Second):
		fmt.Println("[Player] Queue full, stream dropped")
	}
}

// StartPlayable 添加已配置的可播放流到队列（用于设置回调）
func (p *Player) StartPlayable(playable *PlayablePipe) {
	select {
	case p.queue <- playable:
		fmt.Println("[Player] Playable stream added to queue")
	case <-time.After(1 * time.Second):
		fmt.Println("[Player] Queue full, playable stream dropped")
	}
}

// Pause 暂停播放
func (p *Player) Pause() {
	p.currentMu.RLock()
	stream := p.currentPipe
	p.currentMu.RUnlock()
	if stream != nil {
		stream.Pause()
	}
}

// Resume 恢复播放
func (p *Player) Resume() {
	p.currentMu.RLock()
	stream := p.currentPipe
	p.currentMu.RUnlock()
	if stream != nil {
		stream.Resume()
	}
}

// Stop 停止当前播放的流，继续播放队列中的下一个
func (p *Player) Stop() {
	p.currentMu.RLock()
	stream := p.currentPipe
	p.currentMu.RUnlock()
	if stream != nil {
		stream.Stop()
	}
}

// StopAndClear 停止当前流并清空播放队列
func (p *Player) StopAndClear() {
	// 停止当前
	p.Stop()

	// 清空队列
	p.clearQueue()
}

// Shutdown 停止整个 Player（释放资源）
func (p *Player) Shutdown() {
	p.once.Do(func() {
		close(p.done)
	})

	// 清空队列
	p.clearQueue()

	// 等待 goroutine 退出
	p.wg.Wait()

	// 清理资源
	if p.device != nil {
		p.device.Stop()
		p.device.Uninit()
	}
	p.ctx.Uninit()

	fmt.Println("[Player] Shutdown complete")
}

// Next 跳过当前，播放下一个
func (p *Player) Next() {
	p.currentMu.RLock()
	stream := p.currentPipe
	p.currentMu.RUnlock()
	if stream != nil {
		stream.Stop()
	}
}

// State 获取当前状态（派生自 currentPipe.State()）
func (p *Player) State() State {
	p.currentMu.RLock()
	stream := p.currentPipe
	p.currentMu.RUnlock()

	if stream == nil {
		return StateIdle
	}
	switch stream.State() {
	case PipeStatePlaying:
		return StatePlaying
	case PipeStatePaused:
		return StatePaused
	default:
		return StateIdle
	}
}

// QueueLength 返回队列长度
func (p *Player) QueueLength() int {
	return len(p.queue)
}

// IsPlaying 是否正在播放
func (p *Player) IsPlaying() bool {
	return p.State() == StatePlaying
}

// IsPaused 是否暂停
func (p *Player) IsPaused() bool {
	return p.State() == StatePaused
}

// IsStopped 是否停止
func (p *Player) IsStopped() bool {
	return p.State() == StateIdle
}

// clearQueue 清空队列
func (p *Player) clearQueue() {
	cleared := 0
	for {
		select {
		case playable := <-p.queue:
			// 触发流的停止回调
			playable.Stop()
			cleared++
		default:
			goto done
		}
	}
done:
	if cleared > 0 {
		fmt.Printf("[Player] Cleared %d streams from queue\n", cleared)
	}
}

// CurrentStream 获取当前播放的流
func (p *Player) CurrentStream() stream.Stream {
	p.currentMu.RLock()
	defer p.currentMu.RUnlock()
	if p.currentPipe != nil {
		return p.currentPipe.pipe
	}
	return nil
}
