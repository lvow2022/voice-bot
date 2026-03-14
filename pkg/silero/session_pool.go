package silero

import (
	"fmt"
	"sync"

	ort "github.com/yalue/onnxruntime_go"
)

// pooledSession represents a reusable ONNX session with pre-allocated tensors
type pooledSession struct {
	session        *ort.AdvancedSession
	inputTensor    *ort.Tensor[float32]
	stateTensor    *ort.Tensor[float32]
	srTensor       *ort.Tensor[int64]
	outputTensor   *ort.Tensor[float32]
	stateOutTensor *ort.Tensor[float32]
	windowSize     int
	sampleRate     int
}

// SessionPool manages a pool of reusable ONNX sessions
type SessionPool struct {
	mu          sync.Mutex
	sessions    chan *pooledSession
	cfg         DetectorConfig
	poolSize    int
	initialized bool
}

// DefaultPoolSize is the default number of sessions in the pool
const DefaultPoolSize = 8

// globalPool is the global session pool instance
var (
	globalPool     *SessionPool
	globalPoolOnce sync.Once
	globalPoolMu   sync.Mutex
)

// GetGlobalPool returns the global session pool, initializing it if necessary
func GetGlobalPool(cfg DetectorConfig, poolSize int) (*SessionPool, error) {
	globalPoolMu.Lock()
	defer globalPoolMu.Unlock()

	if globalPool != nil && globalPool.initialized {
		return globalPool, nil
	}

	var initErr error
	globalPoolOnce.Do(func() {
		globalPool = &SessionPool{
			cfg:      cfg,
			poolSize: poolSize,
		}
		initErr = globalPool.init()
	})

	if initErr != nil {
		return nil, initErr
	}

	return globalPool, nil
}

// NewSessionPool creates a new session pool with the specified size
func NewSessionPool(cfg DetectorConfig, poolSize int) (*SessionPool, error) {
	if poolSize <= 0 {
		poolSize = DefaultPoolSize
	}

	pool := &SessionPool{
		cfg:      cfg,
		poolSize: poolSize,
	}

	if err := pool.init(); err != nil {
		return nil, err
	}

	return pool, nil
}

func (p *SessionPool) init() error {
	if err := p.cfg.IsValid(); err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}

	if err := ensureOrtEnv(p.cfg); err != nil {
		return err
	}

	p.sessions = make(chan *pooledSession, p.poolSize)

	// Pre-create all sessions
	for i := 0; i < p.poolSize; i++ {
		sess, err := p.createSession()
		if err != nil {
			// Cleanup already created sessions
			p.Close()
			return fmt.Errorf("failed to create session %d: %w", i, err)
		}
		p.sessions <- sess
	}

	p.initialized = true
	return nil
}

func (p *SessionPool) createSession() (*pooledSession, error) {
	opts, err := ort.NewSessionOptions()
	if err != nil {
		return nil, fmt.Errorf("create session options failed: %w", err)
	}
	defer opts.Destroy()

	_ = opts.SetIntraOpNumThreads(1)
	_ = opts.SetInterOpNumThreads(1)
	_ = opts.SetGraphOptimizationLevel(ort.GraphOptimizationLevelEnableAll)

	switch p.cfg.LogLevel {
	case LevelVerbose:
		_ = opts.SetLogSeverityLevel(ort.LoggingLevelVerbose)
	case LogLevelInfo:
		_ = opts.SetLogSeverityLevel(ort.LoggingLevelInfo)
	case LogLevelWarn:
		_ = opts.SetLogSeverityLevel(ort.LoggingLevelWarning)
	case LogLevelError:
		_ = opts.SetLogSeverityLevel(ort.LoggingLevelError)
	case LogLevelFatal:
		_ = opts.SetLogSeverityLevel(ort.LoggingLevelFatal)
	default:
		_ = opts.SetLogSeverityLevel(ort.LoggingLevelWarning)
	}

	windowSize := 512
	if p.cfg.SampleRate == 8000 {
		windowSize = 256
	}

	inputSize := windowSize + contextLen

	// Pre-allocate input tensors
	inputData := make([]float32, inputSize)
	inputTensor, err := ort.NewTensor(ort.NewShape(1, int64(inputSize)), inputData)
	if err != nil {
		return nil, fmt.Errorf("create input tensor failed: %w", err)
	}

	stateData := make([]float32, stateLen)
	stateTensor, err := ort.NewTensor(ort.NewShape(2, 1, 128), stateData)
	if err != nil {
		inputTensor.Destroy()
		return nil, fmt.Errorf("create state tensor failed: %w", err)
	}

	srData := []int64{int64(p.cfg.SampleRate)}
	srTensor, err := ort.NewTensor(ort.NewShape(1), srData)
	if err != nil {
		inputTensor.Destroy()
		stateTensor.Destroy()
		return nil, fmt.Errorf("create sr tensor failed: %w", err)
	}

	outputTensor, err := ort.NewEmptyTensor[float32](ort.NewShape(1, 1))
	if err != nil {
		inputTensor.Destroy()
		stateTensor.Destroy()
		srTensor.Destroy()
		return nil, fmt.Errorf("create output tensor failed: %w", err)
	}

	stateOutTensor, err := ort.NewEmptyTensor[float32](ort.NewShape(2, 1, 128))
	if err != nil {
		inputTensor.Destroy()
		stateTensor.Destroy()
		srTensor.Destroy()
		outputTensor.Destroy()
		return nil, fmt.Errorf("create state output tensor failed: %w", err)
	}

	inputs := []ort.Value{inputTensor, stateTensor, srTensor}
	outputs := []ort.Value{outputTensor, stateOutTensor}

	session, err := ort.NewAdvancedSession(
		p.cfg.ModelPath,
		[]string{"input", "state", "sr"},
		[]string{"output", "stateN"},
		inputs,
		outputs,
		opts,
	)
	if err != nil {
		inputTensor.Destroy()
		stateTensor.Destroy()
		srTensor.Destroy()
		outputTensor.Destroy()
		stateOutTensor.Destroy()
		return nil, fmt.Errorf("create onnxruntime session failed: %w", err)
	}

	return &pooledSession{
		session:        session,
		inputTensor:    inputTensor,
		stateTensor:    stateTensor,
		srTensor:       srTensor,
		outputTensor:   outputTensor,
		stateOutTensor: stateOutTensor,
		windowSize:     windowSize,
		sampleRate:     p.cfg.SampleRate,
	}, nil
}

// Acquire gets a session from the pool (blocks if none available)
func (p *SessionPool) Acquire() *pooledSession {
	return <-p.sessions
}

// Release returns a session to the pool
func (p *SessionPool) Release(sess *pooledSession) {
	if sess != nil {
		p.sessions <- sess
	}
}

// RunInference performs inference using a pooled session
// This is the main method for detector to use
func (p *SessionPool) RunInference(
	inputSamples []float32,
	state *[stateLen]float32,
) (prob float32, err error) {
	sess := p.Acquire()
	defer p.Release(sess)

	// Copy input samples to session's input tensor
	inputData := sess.inputTensor.GetData()
	copy(inputData, inputSamples)

	// Copy detector's state to session's state tensor
	stateData := sess.stateTensor.GetData()
	copy(stateData, state[:])

	// Run inference
	if err := sess.session.Run(); err != nil {
		return 0, fmt.Errorf("onnxruntime run failed: %w", err)
	}

	// Read output probability
	outputData := sess.outputTensor.GetData()
	if len(outputData) == 0 {
		return 0, fmt.Errorf("unexpected empty output")
	}

	// Update detector's state from session's output
	stateOutData := sess.stateOutTensor.GetData()
	if len(stateOutData) != stateLen {
		return 0, fmt.Errorf("unexpected state output size: got %d, want %d", len(stateOutData), stateLen)
	}
	copy(state[:], stateOutData)

	return outputData[0], nil
}

// Close destroys all sessions in the pool
func (p *SessionPool) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.sessions == nil {
		return nil
	}

	close(p.sessions)
	for sess := range p.sessions {
		p.destroySession(sess)
	}

	p.initialized = false
	return nil
}

func (p *SessionPool) destroySession(sess *pooledSession) {
	if sess == nil {
		return
	}

	if sess.session != nil {
		_ = sess.session.Destroy()
	}
	if sess.inputTensor != nil {
		sess.inputTensor.Destroy()
	}
	if sess.stateTensor != nil {
		sess.stateTensor.Destroy()
	}
	if sess.srTensor != nil {
		sess.srTensor.Destroy()
	}
	if sess.outputTensor != nil {
		sess.outputTensor.Destroy()
	}
	if sess.stateOutTensor != nil {
		sess.stateOutTensor.Destroy()
	}
}

// Size returns the pool size
func (p *SessionPool) Size() int {
	return p.poolSize
}

// Available returns the number of available sessions
func (p *SessionPool) Available() int {
	return len(p.sessions)
}
