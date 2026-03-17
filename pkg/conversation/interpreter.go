package conversation

import "strings"

// Interpreter 语义解释器接口
// 将 ASR 文本解释为语义类型
type Interpreter interface {
	Interpret(text string) Semantic
}

// DefaultInterpreter 默认语义解释器
// 基于关键词匹配判断附和词/打断词
type DefaultInterpreter struct {
	backchannelWords map[string]bool
	interruptWords   map[string]bool
}

// InterpreterOption 配置选项
type InterpreterOption func(*DefaultInterpreter)

// WithBackchannelWords 设置附和词列表
func WithBackchannelWords(words []string) InterpreterOption {
	return func(i *DefaultInterpreter) {
		for _, w := range words {
			i.backchannelWords[w] = true
		}
	}
}

// WithInterruptWords 设置打断词列表
func WithInterruptWords(words []string) InterpreterOption {
	return func(i *DefaultInterpreter) {
		for _, w := range words {
			i.interruptWords[w] = true
		}
	}
}

// NewDefaultInterpreter 创建默认语义解释器
func NewDefaultInterpreter(opts ...InterpreterOption) *DefaultInterpreter {
	i := &DefaultInterpreter{
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
	}

	for _, opt := range opts {
		opt(i)
	}

	return i
}

// Interpret 解释文本语义
func (i *DefaultInterpreter) Interpret(text string) Semantic {
	text = strings.TrimSpace(text)
	if text == "" {
		return SemanticIgnore
	}

	// 优先检查打断词
	if i.interruptWords[text] {
		return SemanticInterrupt
	}

	// 检查附和词
	if i.backchannelWords[text] {
		return SemanticBackchannel
	}

	// 其他情况视为新轮次
	return SemanticNewTurn
}

// AddBackchannelWord 添加附和词
func (i *DefaultInterpreter) AddBackchannelWord(word string) {
	i.backchannelWords[word] = true
}

// AddInterruptWord 添加打断词
func (i *DefaultInterpreter) AddInterruptWord(word string) {
	i.interruptWords[word] = true
}

// IsBackchannelWord 检查是否是附和词
func (i *DefaultInterpreter) IsBackchannelWord(text string) bool {
	return i.backchannelWords[strings.TrimSpace(text)]
}

// IsInterruptWord 检查是否是打断词
func (i *DefaultInterpreter) IsInterruptWord(text string) bool {
	return i.interruptWords[strings.TrimSpace(text)]
}
