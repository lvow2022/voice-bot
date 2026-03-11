package speech

import (
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"
)

// SentenceSplitter 分句器 - 将 token 流转换为完整句子（同步版本）
type SentenceSplitter struct {
	buffer strings.Builder
	minLen int
	maxLen int
	mu     sync.Mutex
}

// NewSentenceSplitter 创建分句器
func NewSentenceSplitter(minLen, maxLen int) *SentenceSplitter {
	return &SentenceSplitter{
		minLen: minLen,
		maxLen: maxLen,
	}
}

// Write 写入 token，返回完整句子（如果有）
func (s *SentenceSplitter) Write(token string) string {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.buffer.WriteString(token)

	// 检查是否有完整句子
	if s.buffer.Len() >= s.minLen {
		if sentence := s.tryExtractSentence(); sentence != "" {
			return sentence
		}
	}

	// 超过最大长度，强制切分
	if s.buffer.Len() >= s.maxLen {
		sentence := s.buffer.String()
		s.buffer.Reset()
		return sentence
	}

	return ""
}

// tryExtractSentence 尝试提取完整句子
func (s *SentenceSplitter) tryExtractSentence() string {
	if s.buffer.Len() == 0 {
		return ""
	}

	text := s.buffer.String()

	// 查找句子边界
	idx := s.findSentenceEnd(text)
	if idx > 0 {
		sentence := text[:idx]
		remaining := strings.TrimLeft(text[idx:], " \t\n")
		s.buffer.Reset()
		s.buffer.WriteString(remaining)
		return sentence
	}

	return ""
}

// findSentenceEnd 查找句子结束位置
func (s *SentenceSplitter) findSentenceEnd(text string) int {
	endPuncts := ".!?。！？…\n"

	for i, r := range text {
		if strings.ContainsRune(endPuncts, r) {
			// 跳过省略号
			if r == '…' {
				continue
			}

			// 跳过连续的标点和空格
			j := i + utf8.RuneLen(r)
			for j < len(text) {
				nextRune, size := utf8.DecodeRuneInString(text[j:])
				if nextRune == utf8.RuneError || size == 0 {
					break
				}
				if unicode.IsPunct(nextRune) || unicode.IsSpace(nextRune) {
					j += size
				} else {
					break
				}
			}
			return j
		}
	}

	return -1
}

// Flush 刷新缓冲区，返回剩余内容
func (s *SentenceSplitter) Flush() string {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.buffer.Len() == 0 {
		return ""
	}

	text := strings.TrimSpace(s.buffer.String())
	s.buffer.Reset()
	return text
}

// Reset 重置分句器
func (s *SentenceSplitter) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.buffer.Reset()
}
