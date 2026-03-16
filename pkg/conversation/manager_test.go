package conversation

import (
	"testing"

	"voicebot/pkg/voicechain"
)

func TestTurnManager(t *testing.T) {
	tm := NewTurnManager()

	// 初始状态应该是 UserTurn, AgentIdle
	if tm.GetState().Turn != UserTurn {
		t.Errorf("initial turn should be UserTurn, got %v", tm.GetState().Turn)
	}
	if tm.GetState().Phase != AgentIdle {
		t.Errorf("initial phase should be AgentIdle, got %v", tm.GetState().Phase)
	}

	// HandleVADStart
	tm.HandleVADStart()
	if !tm.IsUserTurn() {
		t.Error("should be UserTurn after VADStart")
	}

	// HandleAgentStart
	tm.HandleAgentStart()
	if !tm.IsAgentTurn() {
		t.Error("should be AgentTurn after AgentStart")
	}
	if !tm.IsAgentProcessing() {
		t.Error("should be AgentProcessing after AgentStart")
	}

	// HandleAgentSpeak
	tm.HandleAgentSpeak()
	if !tm.IsAgentSpeaking() {
		t.Error("should be AgentSpeaking after AgentSpeak")
	}

	// HandlePlaybackFinished
	tm.HandlePlaybackFinished()
	if !tm.IsUserTurn() {
		t.Error("should be UserTurn after PlaybackFinished")
	}
}

func TestBackchannelChecker(t *testing.T) {
	checker := NewBackchannelChecker()

	tests := []struct {
		text     string
		want     BackchannelType
	}{
		{"嗯", BackchannelACK},
		{"对", BackchannelACK},
		{"好的", BackchannelACK},
		{"等一下", BackchannelInterrupt},
		{"不用了", BackchannelInterrupt},
		{"帮我查天气", BackchannelNewTurn},
		{"", BackchannelIgnore},
	}

	for _, tt := range tests {
		t.Run(tt.text, func(t *testing.T) {
			got := checker.CheckInput(tt.text, 0, 0)
			if got != tt.want {
				t.Errorf("CheckInput(%q) = %v, want %v", tt.text, got, tt.want)
			}
		})
	}
}

func TestCommandGenerator_UserTurn(t *testing.T) {
	cg := NewCommandGenerator()
	state := ConversationState{Turn: UserTurn, Phase: AgentIdle}

	tests := []struct {
		event ConversationEvent
		want  AgentCommand
	}{
		{EventNewTurn, CmdStartAgent},
		{EventBackchannel, CmdNone},
		{EventInterrupt, CmdNone},
		{EventIgnore, CmdNone},
	}

	for _, tt := range tests {
		t.Run(tt.event.String(), func(t *testing.T) {
			got := cg.Generate(state, tt.event)
			if got != tt.want {
				t.Errorf("Generate(UserTurn, %v) = %v, want %v", tt.event, got, tt.want)
			}
		})
	}
}

func TestCommandGenerator_AgentProcessing(t *testing.T) {
	cg := NewCommandGenerator()
	state := ConversationState{Turn: AgentTurn, Phase: AgentProcessing}

	tests := []struct {
		event ConversationEvent
		want  AgentCommand
	}{
		{EventNewTurn, CmdCancelAgent},
		{EventInterrupt, CmdCancelAgent},
		{EventBackchannel, CmdNone},
		{EventIgnore, CmdNone},
	}

	for _, tt := range tests {
		t.Run(tt.event.String(), func(t *testing.T) {
			got := cg.Generate(state, tt.event)
			if got != tt.want {
				t.Errorf("Generate(AgentProcessing, %v) = %v, want %v", tt.event, got, tt.want)
			}
		})
	}
}

func TestCommandGenerator_AgentSpeaking(t *testing.T) {
	cg := NewCommandGenerator()
	state := ConversationState{Turn: AgentTurn, Phase: AgentSpeaking}

	tests := []struct {
		event ConversationEvent
		want  AgentCommand
	}{
		{EventInterrupt, CmdStopPlayback},
		{EventNewTurn, CmdStopPlayback},
		{EventBackchannel, CmdNone},
		{EventIgnore, CmdNone},
	}

	for _, tt := range tests {
		t.Run(tt.event.String(), func(t *testing.T) {
			got := cg.Generate(state, tt.event)
			if got != tt.want {
				t.Errorf("Generate(AgentSpeaking, %v) = %v, want %v", tt.event, got, tt.want)
			}
		})
	}
}

func TestConversationManager_FullFlow(t *testing.T) {
	var stateChanges []string
	var commands []AgentCommand

	mgr := NewConversationManager(
		WithOnStateChange(func(old, new ConversationState) {
			stateChanges = append(stateChanges, old.String()+" -> "+new.String())
		}),
		WithOnCommand(func(cmd AgentCommand) {
			commands = append(commands, cmd)
		}),
	)

	// 1. 用户说话: "帮我查天气" -> StartLLM
	cmd := mgr.HandleAudioEvent(AudioEvent{Type: ASRFinal, Text: "帮我查天气"})
	if cmd != CmdStartAgent {
		t.Errorf("expected CmdStartAgent, got %v", cmd)
	}

	// 2. 模拟 Agent 开始处理
	mgr.HandleSystemEvent(voicechain.SystemEvent{Type: voicechain.SystemEventAgentStart})
	if mgr.turnManager.IsAgentProcessing() {
		// OK
	} else {
		t.Error("should be AgentProcessing")
	}

	// 3. 模拟 Agent 开始说话
	mgr.HandleSystemEvent(voicechain.SystemEvent{Type: voicechain.SystemEventAgentSpeak})
	if mgr.turnManager.IsAgentSpeaking() {
		// OK
	} else {
		t.Error("should be AgentSpeaking")
	}

	// 4. 用户打断: "等一下" -> StopPlayback
	cmd = mgr.HandleAudioEvent(AudioEvent{Type: ASRFinal, Text: "等一下"})
	if cmd != CmdStopPlayback {
		t.Errorf("expected CmdStopPlayback, got %v", cmd)
	}

	// 5. 播放完成
	cmd = mgr.HandlePlaybackFinished()
	if cmd != CmdCommitAgent {
		t.Errorf("expected CmdCommitAgent, got %v", cmd)
	}
	if mgr.GetState().Turn != UserTurn {
		t.Error("should be UserTurn after playback finished")
	}

	t.Logf("state changes: %v", stateChanges)
	t.Logf("commands: %v", commands)
}

func TestConversationManager_VADPause(t *testing.T) {
	cg := NewCommandGenerator(WithPauseOnVADStart(true))
	state := ConversationState{Turn: AgentTurn, Phase: AgentSpeaking}

	cmd := cg.GenerateForVAD(state, VADStart)
	if cmd != CmdPausePlayback {
		t.Errorf("expected CmdPausePlayback, got %v", cmd)
	}
}

func TestContextManager(t *testing.T) {
	cm := NewContextManager()

	// 添加用户消息
	cm.AddUserMessage("帮我查天气")
	history := cm.GetHistory()
	if len(history) != 1 {
		t.Fatalf("expected 1 history entry, got %d", len(history))
	}
	if history[0].Role != "user" || history[0].Content != "帮我查天气" {
		t.Errorf("unexpected history entry: %+v", history[0])
	}

	// 设置当前 Agent 回复
	cm.SetCurrentAgentReply("好的，我来帮您查天气")
	if cm.GetCurrentAgentReply() != "好的，我来帮您查天气" {
		t.Errorf("unexpected current reply: %s", cm.GetCurrentAgentReply())
	}

	// 追加已播放内容
	cm.AppendPlayedContent("好的，")
	cm.AppendPlayedContent("我来帮您查天气")

	// 提交
	committed := cm.CommitPlayedContent()
	if committed != "好的，我来帮您查天气" {
		t.Errorf("unexpected committed content: %s", committed)
	}

	// 历史应该有 2 条
	history = cm.GetHistory()
	if len(history) != 2 {
		t.Errorf("expected 2 history entries, got %d", len(history))
	}
}

func TestEventQueue(t *testing.T) {
	q := NewEventQueue()
	defer q.Close()

	// 推入事件
	q.PushAudio(AudioEvent{Type: VADStart})
	q.PushAudio(AudioEvent{Type: ASRFinal, Text: "test"})
	q.PushSystem(voicechain.SystemEvent{Type: voicechain.SystemEventAgentStart})
	q.PushPlaybackDone()

	if q.Len() != 4 {
		t.Errorf("expected queue length 4, got %d", q.Len())
	}
}
