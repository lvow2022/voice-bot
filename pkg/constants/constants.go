// Package constants provides shared constants for voicebot.
package constants

import "strings"

// Internal channel prefixes
var internalChannelPrefixes = []string{
	"internal:",
	"subagent:",
	"spawn:",
	"mcp:",
}

// IsInternalChannel returns true if the channel is an internal channel.
// Internal channels are used for agent-to-agent communication and should
// not trigger user-facing behaviors like message persistence.
func IsInternalChannel(channel string) bool {
	if channel == "" {
		return false
	}
	for _, prefix := range internalChannelPrefixes {
		if strings.HasPrefix(channel, prefix) {
			return true
		}
	}
	return false
}
