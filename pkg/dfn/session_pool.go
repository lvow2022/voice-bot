package dfn

import (
	"fmt"
	"sync"

	ort "github.com/yalue/onnxruntime_go"
)

var (
	onnxInitOnce  sync.Once
	onnxInitError error
)

// SessionPool manages a pool of reusable ONNX sessions for DFN noise suppression
type SessionPool struct {
	mu          sync.Mutex
	sessions    chan *pooledSession
	cfg         Config
	poolSize    int
	initialized bool
}

// globalPool is the global session pool instance
var (
	globalPool     *SessionPool
	globalPoolOnce sync.Once
	globalPoolMu   sync.Mutex
)

// GetGlobalPool returns the global session pool, initializing it if necessary
func GetGlobalPool(cfg Config, poolSize int) (*SessionPool, error) {
	globalPoolMu.Lock()
	defer globalPoolMu.Unlock()

	if globalPool != nil && globalPool.initialized {
		return globalPool, nil
	}

	var initErr error
	globalPoolOnce.Do(func() {
		if poolSize <= 0 {
			poolSize = DefaultPoolSize
		}
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
func NewSessionPool(cfg Config, poolSize int) (*SessionPool, error) {
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
	if err := p.cfg.Validate(); err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}

	if err := ensureOrtEnv(); err != nil {
		return err
	}

	p.sessions = make(chan *pooledSession, p.poolSize)

	// Pre-create all sessions
	for i := 0; i < p.poolSize; i++ {
		sess, err := newPooledSession(p.cfg)
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

// ensureOrtEnv initializes the ONNX runtime environment if not already initialized
func ensureOrtEnv() error {
	// If already initialized by another package (e.g., silero), skip initialization
	if ort.IsInitialized() {
		return nil
	}

	onnxInitOnce.Do(func() {
		// Double-check after acquiring the once lock
		if ort.IsInitialized() {
			return
		}
		libPath := ResolveOnnxRuntimeLibPath()
		ort.SetSharedLibraryPath(libPath)
		onnxInitError = ort.InitializeEnvironment()
	})

	return onnxInitError
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

// Close destroys all sessions in the pool
func (p *SessionPool) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.sessions == nil {
		return nil
	}

	close(p.sessions)
	for sess := range p.sessions {
		sess.destroy()
	}

	p.initialized = false
	return nil
}

// Size returns the pool size
func (p *SessionPool) Size() int {
	return p.poolSize
}

// Available returns the number of available sessions
func (p *SessionPool) Available() int {
	return len(p.sessions)
}

// Config returns the pool configuration
func (p *SessionPool) Config() Config {
	return p.cfg
}
