package conversation

import (
	"testing"
)

func TestConversationManager_StateTransitions(t *testing.T) {
	mgr := NewConversationManager()

	// 初始状态应该是 StateUserTurn
	if mgr.GetState() != StateUserTurn {
		t.Errorf("initial state should be StateUserTurn, got %v", mgr.GetState())
	}

	// AgentGenerating → StateAgentGenerating
	mgr.HandleEvent(EventAgentGenerating, "")
	if mgr.GetState() != StateAgentGenerating {
		t.Errorf("state should be StateAgentGenerating, got %v", mgr.GetState())
	}

	// AgentSpeaking → StateAgentSpeaking
	mgr.HandleEvent(EventAgentSpeaking, "")
	if mgr.GetState() != StateAgentSpeaking {
		t.Errorf("state should be StateAgentSpeaking, got %v", mgr.GetState())
	}

	// PlaybackDone → StateUserTurn
	mgr.HandleEvent(EventPlaybackDone, "")
	if mgr.GetState() != StateUserTurn {
		t.Errorf("state should be StateUserTurn, got %v", mgr.GetState())
	}
}

func TestInterpreter_SemanticTypes(t *testing.T) {
	i := NewDefaultInterpreter()

	tests := []struct {
		text string
		want Semantic
	}{
		{"嗯", SemanticBackchannel},
		{"对", SemanticBackchannel},
		{"好的", SemanticBackchannel},
		{"等一下", SemanticInterrupt},
		{"不用了", SemanticInterrupt},
		{"帮我查天气", SemanticNewTurn},
		{"", SemanticIgnore},
	}

	for _, tt := range tests {
		t.Run(tt.text, func(t *testing.T) {
			got := i.Interpret(tt.text)
			if got != tt.want {
				t.Errorf("Interpret(%q) = %v, want %v", tt.text, got, tt.want)
			}
		})
	}
}

func TestPolicy_UserTurn(t *testing.T) {
	p := NewDefaultPolicy()

	tests := []struct {
		event    Event
		semantic Semantic
		want     Command
	}{
		{EventASRFinal, SemanticNewTurn, CmdInvokeAgent},
		{EventASRFinal, SemanticBackchannel, CmdNone},
		{EventASRFinal, SemanticInterrupt, CmdNone},
		{EventASRFinal, SemanticIgnore, CmdNone},
	}

	for _, tt := range tests {
		t.Run(tt.semantic.String(), func(t *testing.T) {
			got := p.Decide(StateUserTurn, tt.event, tt.semantic)
			if got != tt.want {
				t.Errorf("Decide(UserTurn, %v, %v) = %v, want %v", tt.event, tt.semantic, got, tt.want)
			}
		})
	}
}

func TestPolicy_AgentGenerating(t *testing.T) {
	p := NewDefaultPolicy()

	tests := []struct {
		event    Event
		semantic Semantic
		want     Command
	}{
		{EventASRFinal, SemanticNewTurn, CmdCancelAgent},
		{EventASRFinal, SemanticInterrupt, CmdCancelAgent},
		{EventASRFinal, SemanticBackchannel, CmdNone},
		{EventASRFinal, SemanticIgnore, CmdNone},
	}

	for _, tt := range tests {
		t.Run(tt.semantic.String(), func(t *testing.T) {
			got := p.Decide(StateAgentGenerating, tt.event, tt.semantic)
			if got != tt.want {
				t.Errorf("Decide(AgentGenerating, %v, %v) = %v, want %v", tt.event, tt.semantic, got, tt.want)
			}
		})
	}
}

func TestPolicy_AgentSpeaking(t *testing.T) {
	p := NewDefaultPolicy()

	tests := []struct {
		event    Event
		semantic Semantic
		want     Command
	}{
		{EventASRFinal, SemanticInterrupt, CmdStopPlayback},
		{EventASRFinal, SemanticNewTurn, CmdStopPlayback},
		{EventASRFinal, SemanticBackchannel, CmdNone},
		{EventASRFinal, SemanticIgnore, CmdNone},
		{EventVADStart, SemanticIgnore, CmdPausePlayback}, // VADStart 暂停播放
	}

	for _, tt := range tests {
		t.Run(tt.event.String()+"_"+tt.semantic.String(), func(t *testing.T) {
			got := p.Decide(StateAgentSpeaking, tt.event, tt.semantic)
			if got != tt.want {
				t.Errorf("Decide(AgentSpeaking, %v, %v) = %v, want %v", tt.event, tt.semantic, got, tt.want)
			}
		})
	}
}

func TestConversationManager_FullFlow(t *testing.T) {
	var stateChanges []string
	var commands []Command

	mgr := NewConversationManager(
		WithOnStateChange(func(old, new State) {
			stateChanges = append(stateChanges, old.String()+" -> "+new.String())
		}),
		WithOnCommand(func(cmd Command, text string) {
			commands = append(commands, cmd)
		}),
	)

	// 1. 用户说话: "帮我查天气" -> InvokeAgent
	cmd := mgr.HandleEvent(EventASRFinal, "帮我查天气")
	if cmd != CmdInvokeAgent {
		t.Errorf("expected CmdInvokeAgent, got %v", cmd)
	}

	// 2. 模拟 Agent 开始生成
	mgr.HandleEvent(EventAgentGenerating, "")
	if mgr.GetState() != StateAgentGenerating {
		t.Errorf("state should be StateAgentGenerating, got %v", mgr.GetState())
	}

	// 3. 模拟 Agent 开始说话
	mgr.HandleEvent(EventAgentSpeaking, "")
	if mgr.GetState() != StateAgentSpeaking {
		t.Errorf("state should be StateAgentSpeaking, got %v", mgr.GetState())
	}

	// 4. 用户打断: "等一下" -> StopPlayback
	cmd = mgr.HandleEvent(EventASRFinal, "等一下")
	if cmd != CmdStopPlayback {
		t.Errorf("expected CmdStopPlayback, got %v", cmd)
	}

	// 5. 播放完成
	mgr.HandleEvent(EventPlaybackDone, "")
	if mgr.GetState() != StateUserTurn {
		t.Errorf("state should be StateUserTurn, got %v", mgr.GetState())
	}

	t.Logf("state changes: %v", stateChanges)
	t.Logf("commands: %v", commands)
}

func TestConversationManager_VADPause(t *testing.T) {
	mgr := NewConversationManager()

	// 设置为 AgentSpeaking 状态
	mgr.SetState(StateAgentSpeaking)

	// VADStart 应该返回暂停命令
	cmd := mgr.HandleEvent(EventVADStart, "")
	if cmd != CmdPausePlayback {
		t.Errorf("expected CmdPausePlayback, got %v", cmd)
	}
}

func TestPolicy_NoPauseOnVADStart(t *testing.T) {
	p := NewDefaultPolicy(WithPauseOnVADStart(false))

	// VADStart 不应该返回暂停命令
	got := p.Decide(StateAgentSpeaking, EventVADStart, SemanticIgnore)
	if got != CmdNone {
		t.Errorf("Decide with pauseOnVADStart=false should return CmdNone, got %v", got)
	}
}
