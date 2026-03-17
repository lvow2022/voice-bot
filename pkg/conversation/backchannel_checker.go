package conversation

import (
	"strings"
	"time"
)

// BackchannelType 附和词/打断词类型
type BackchannelType int

const (
	BackchannelIgnore BackchannelType = iota // 噪音，忽略
	BackchannelACK                            // 附和词 "嗯", "对", "好的"
	BackchannelInterrupt                      // 打断 "等一下", "不用了"
	BackchannelNewTurn                        // 新轮次
)

func (t BackchannelType) String() string {
	switch t {
	case BackchannelACK:
		return "ACK"
	case BackchannelInterrupt:
		return "Interrupt"
	case BackchannelNewTurn:
		return "NewTurn"
	default:
		return "Ignore"
	}
}

// BackchannelChecker 判断用户输入是否属于 backchannel / interrupt / new turn
type BackchannelChecker struct {
	// 配置
	backchannelWords map[string]bool // 附和词列表
	interruptWords   map[string]bool // 打断词列表
	minSpeechDur     time.Duration   // 最小语音时长（过滤噪音）
}

// BackchannelOption 配置选项
type BackchannelOption func(*BackchannelChecker)

// WithBackchannelWords 设置附和词列表
func WithBackchannelWords(words []string) BackchannelOption {
	return func(c *BackchannelChecker) {
		for _, w := range words {
			c.backchannelWords[w] = true
		}
	}
}

// WithInterruptWords 设置打断词列表
func WithInterruptWords(words []string) BackchannelOption {
	return func(c *BackchannelChecker) {
		for _, w := range words {
			c.interruptWords[w] = true
		}
	}
}

// WithMinSpeechDuration 设置最小语音时长
func WithMinSpeechDuration(d time.Duration) BackchannelOption {
	return func(c *BackchannelChecker) {
		c.minSpeechDur = d
	}
}

// NewBackchannelChecker 创建新的 BackchannelChecker
func NewBackchannelChecker(opts ...BackchannelOption) *BackchannelChecker {
	c := &BackchannelChecker{
		backchannelWords: map[string]bool{
			"嗯":   true,
			"对":   true,
			"好的":  true,
			"是的":  true,
			"啊":   true,
			"哦":   true,
			"行":   true,
			"可以":  true,
			"明白了": true,
			"知道":  true,
		},
		interruptWords: map[string]bool{
			"等一下":  true,
			"等一等":  true,
			"不用了":  true,
			"停":    true,
			"停下":   true,
			"不要":   true,
			"别说了":  true,
			"闭嘴":   true,
			"够了":   true,
			"算了":   true,
			"取消":   true,
			"打断一下": true,
		},
		minSpeechDur: 200 * time.Millisecond,
	}

	for _, opt := range opts {
		opt(c)
	}

	return c
}

// CheckInput 检查用户输入，返回类型
// 输入：ASR text + speech duration + VAD gap
// 输出：backchannel 类型
func (c *BackchannelChecker) CheckInput(text string, speechDur, vadGap time.Duration) BackchannelType {
	// 空文本，忽略
	text = strings.TrimSpace(text)
	if text == "" {
		return BackchannelIgnore
	}

	// 语音时长太短，可能是噪音
	if speechDur > 0 && speechDur < c.minSpeechDur {
		return BackchannelIgnore
	}

	// 检查打断词
	if c.interruptWords[text] {
		return BackchannelInterrupt
	}

	// 检查附和词
	if c.backchannelWords[text] {
		return BackchannelACK
	}

	// 其他情况，视为新轮次
	return BackchannelNewTurn
}

// Check 从 AudioEvent 判断 Semantic
// 简化版：仅使用文本判断
func (c *BackchannelChecker) Check(event AudioEvent) Semantic {
	if event.Type != ASRFinal {
		return SemanticIgnore
	}

	switch c.CheckInput(event.Text, 0, 0) {
	case BackchannelACK:
		return SemanticBackchannel
	case BackchannelInterrupt:
		return SemanticInterrupt
	case BackchannelNewTurn:
		return SemanticNewTurn
	default:
		return SemanticIgnore
	}
}

// AddBackchannelWord 添加附和词
func (c *BackchannelChecker) AddBackchannelWord(word string) {
	c.backchannelWords[word] = true
}

// AddInterruptWord 添加打断词
func (c *BackchannelChecker) AddInterruptWord(word string) {
	c.interruptWords[word] = true
}

// IsBackchannelWord 检查是否是附和词
func (c *BackchannelChecker) IsBackchannelWord(text string) bool {
	return c.backchannelWords[strings.TrimSpace(text)]
}

// IsInterruptWord 检查是否是打断词
func (c *BackchannelChecker) IsInterruptWord(text string) bool {
	return c.interruptWords[strings.TrimSpace(text)]
}
