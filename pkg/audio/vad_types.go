package audio

import (
	"fmt"
	"io"
	"log/slog"
	"sync"
	"time"
)

// VADType defines different VAD implementations
type VADType string

const (
	// VADTypeSilero uses the Silero-based VAD with ONNX runtime
	VADTypeSilero VADType = "silero"
)

// VADState represents the state of VAD detection
type VADState int

const (
	VADStateBeginTransition     VADState = iota // 初始状态，等待语音开始
	VADStateInactivityTransition               // 语音结束后的静音状态
	VADStateActivityTransition                  // 检测到语音活动状态
)

// VADHandler is a callback function for VAD events
// duration: 累计处理的音频时长
// speaking: 是否检测到语音开始
// silence: 是否检测到语音结束
type VADHandler func(duration time.Duration, speaking bool, silence bool)

// VADDetector interface defines the contract for VAD detectors
type VADDetector interface {
	io.Closer
	fmt.Stringer
	Process(frame []byte, handler VADHandler) error
}

// VADDetectorOption configures the VAD detector
type VADDetectorOption struct {
	// 音频格式参数
	SampleRate    int    `json:"sampleRate" default:"16000"`    // 采样率 (8000 或 16000)
	BitDepth      int    `json:"bitDepth" default:"16"`         // 位深度 (16)
	Channels      int    `json:"channels" default:"1"`          // 声道数 (1)
	FrameDuration string `json:"frameDuration" default:"20ms"`  // 输入帧时长

	// 语音检测阈值
	PositiveSpeechThreshold float32 `json:"positiveSpeechThreshold" default:"0.5"` // 语音开始阈值 (0.0-1.0)
	NegativeSpeechThreshold float32 `json:"negativeSpeechThreshold" default:"0.4"` // 语音结束阈值 (0.0-1.0)

	// 语音帧数配置（推荐使用 SpeechStartMs/SpeechEndMs）
	SpeechStartMs   int `json:"speechStartMs"`   // 触发语音需要的毫秒数（新参数，优先）
	SpeechEndMs     int `json:"speechEndMs"`     // 结束语音需要的毫秒数（新参数，优先）
	MinSpeechFrames int `json:"minSpeechFrames"` // 触发语音需要的帧数（旧参数）
	RedemptionFrames int `json:"redemptionFrames"` // 结束语音需要的帧数（旧参数）
}

// NormalizeVADDetectorOption normalizes VADDetectorOption with frame duration calculation
// Priority:
// 1. If SpeechStartMs/SpeechEndMs are set (>0), calculate MinSpeechFrames/RedemptionFrames
// 2. Otherwise, use MinSpeechFrames/RedemptionFrames directly
func NormalizeVADDetectorOption(opt VADDetectorOption, vadFrameDurationMs int) VADDetectorOption {
	var frameDurationMs int
	if vadFrameDurationMs > 0 {
		frameDurationMs = vadFrameDurationMs
	} else {
		frameDuration := ParseFrameDuration(opt.FrameDuration)
		frameDurationMs = int(frameDuration.Milliseconds())
	}

	// New params take priority
	if opt.SpeechStartMs > 0 && opt.SpeechEndMs > 0 && frameDurationMs > 0 {
		opt.MinSpeechFrames = opt.SpeechStartMs / frameDurationMs
		opt.RedemptionFrames = opt.SpeechEndMs / frameDurationMs
	}

	// Set defaults if still not set
	if opt.MinSpeechFrames <= 0 {
		opt.MinSpeechFrames = 5
	}
	if opt.RedemptionFrames <= 0 {
		opt.RedemptionFrames = 10
	}

	return opt
}

// DefaultVADDetectorOption returns a VADDetectorOption with sensible defaults
func DefaultVADDetectorOption() VADDetectorOption {
	return VADDetectorOption{
		SampleRate:    16000,
		BitDepth:      16,
		Channels:      1,
		FrameDuration: "20ms",

		PositiveSpeechThreshold: 0.5,
		NegativeSpeechThreshold: 0.4,

		SpeechStartMs:   160, // ~5 frames at 32ms
		SpeechEndMs:     320, // ~10 frames at 32ms
		MinSpeechFrames: 5,
		RedemptionFrames: 10,
	}
}

// VADFactory creates a VADDetector from options
type VADFactory func(opt VADDetectorOption) VADDetector

var (
	vadRegistry   = make(map[VADType]VADFactory)
	vadRegistryMu sync.RWMutex
)

// RegisterVAD registers a VAD factory for a given type
func RegisterVAD(vadType VADType, factory VADFactory) {
	vadRegistryMu.Lock()
	defer vadRegistryMu.Unlock()
	vadRegistry[vadType] = factory
}

// GetVAD returns a VAD detector for the given type
func GetVAD(vadType VADType, opt VADDetectorOption) (VADDetector, error) {
	vadRegistryMu.RLock()
	defer vadRegistryMu.RUnlock()
	factory, ok := vadRegistry[vadType]
	if !ok {
		return nil, fmt.Errorf("VAD type %s not registered", vadType)
	}
	return factory(opt), nil
}

// CreateVadProcessor creates a VAD detector for the given type
func CreateVadProcessor(vadType VADType, opt VADDetectorOption) VADDetector {
	vad, err := GetVAD(vadType, opt)
	if err != nil {
		slog.Warn("VAD processor creation failed", "type", vadType, "error", err)
		return nil
	}
	slog.Info("VAD processor created", "type", vadType)
	return vad
}
