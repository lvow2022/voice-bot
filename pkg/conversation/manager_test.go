package conversation

import (
	"testing"

	"voicebot/pkg/voicechain"
)

func TestTurnManager(t *testing.T) {
	tm := NewTurnManager()

	// 初始状态应该是 TurnUser, PhaseIdle
	if tm.GetState().Turn != TurnUser {
		t.Errorf("initial turn should be TurnUser, got %v", tm.GetState().Turn)
	}
	if tm.GetState().Phase != PhaseIdle {
		t.Errorf("initial phase should be PhaseIdle, got %v", tm.GetState().Phase)
	}

	// HandleVADStart
	tm.HandleVADStart()
	if !tm.IsUserTurn() {
		t.Error("should be TurnUser after VADStart")
	}

	// HandleAgentStart
	tm.HandleAgentStart()
	if !tm.IsAgentTurn() {
		t.Error("should be TurnAgent after AgentStart")
	}
	if !tm.IsAgentGenerating() {
		t.Error("should be PhaseGenerating after AgentStart")
	}

	// HandleAgentSpeak
	tm.HandleAgentSpeak()
	if !tm.IsAgentSpeaking() {
		t.Error("should be PhaseSpeaking after AgentSpeak")
	}

	// HandlePlaybackFinished
	tm.HandlePlaybackFinished()
	if !tm.IsUserTurn() {
		t.Error("should be TurnUser after PlaybackFinished")
	}
}

func TestBackchannelChecker(t *testing.T) {
	checker := NewBackchannelChecker()

	tests := []struct {
		text string
		want BackchannelType
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
	state := State{Turn: TurnUser, Phase: PhaseIdle}

	tests := []struct {
		event Semantic
		want  AgentCommand
	}{
		{SemanticNewTurn, CmdInvokeAgentGenerate},
		{SemanticBackchannel, CmdNone},
		{SemanticInterrupt, CmdNone},
		{SemanticIgnore, CmdNone},
	}

	for _, tt := range tests {
		t.Run(tt.event.String(), func(t *testing.T) {
			got := cg.Generate(state, tt.event)
			if got != tt.want {
				t.Errorf("Generate(TurnUser, %v) = %v, want %v", tt.event, got, tt.want)
			}
		})
	}
}

func TestCommandGenerator_AgentGenerating(t *testing.T) {
	cg := NewCommandGenerator()
	state := State{Turn: TurnAgent, Phase: PhaseGenerating}

	tests := []struct {
		event Semantic
		want  AgentCommand
	}{
		{SemanticNewTurn, CmdCancelAgentGenerate},
		{SemanticInterrupt, CmdCancelAgentGenerate},
		{SemanticBackchannel, CmdNone},
		{SemanticIgnore, CmdNone},
	}

	for _, tt := range tests {
		t.Run(tt.event.String(), func(t *testing.T) {
			got := cg.Generate(state, tt.event)
			if got != tt.want {
				t.Errorf("Generate(PhaseGenerating, %v) = %v, want %v", tt.event, got, tt.want)
			}
		})
	}
}

func TestCommandGenerator_AgentSpeaking(t *testing.T) {
	cg := NewCommandGenerator()
	state := State{Turn: TurnAgent, Phase: PhaseSpeaking}

	tests := []struct {
		event Semantic
		want  AgentCommand
	}{
		{SemanticInterrupt, CmdStopAgentPlayback},
		{SemanticNewTurn, CmdStopAgentPlayback},
		{SemanticBackchannel, CmdNone},
		{SemanticIgnore, CmdNone},
	}

	for _, tt := range tests {
		t.Run(tt.event.String(), func(t *testing.T) {
			got := cg.Generate(state, tt.event)
			if got != tt.want {
				t.Errorf("Generate(PhaseSpeaking, %v) = %v, want %v", tt.event, got, tt.want)
			}
		})
	}
}

func TestConversationManager_FullFlow(t *testing.T) {
	var stateChanges []string
	var commands []AgentCommand

	mgr := NewConversationManager(
		WithOnStateChange(func(old, new State) {
			stateChanges = append(stateChanges, old.String()+" -> "+new.String())
		}),
		WithOnCommand(func(cmd AgentCommand) {
			commands = append(commands, cmd)
		}),
	)

	// 1. 用户说话: "帮我查天气" -> InvokeAgentGenerate
	cmd := mgr.HandleAudioEvent(AudioEvent{Type: ASRFinal, Text: "帮我查天气"})
	if cmd != CmdInvokeAgentGenerate {
		t.Errorf("expected CmdInvokeAgentGenerate, got %v", cmd)
	}

	// 2. 模拟 Agent 开始生成
	mgr.HandleSystemEvent(Event{Type: voicechain.StateAgentGenerating})
	if mgr.turnManager.IsAgentGenerating() {
		// OK
	} else {
		t.Error("should be PhaseGenerating")
	}

	// 3. 模拟 Agent 开始说话
	mgr.HandleSystemEvent(Event{Type: voicechain.StateAgentSpeaking})
	if mgr.turnManager.IsAgentSpeaking() {
		// OK
	} else {
		t.Error("should be PhaseSpeaking")
	}

	// 4. 用户打断: "等一下" -> StopAgentPlayback
	cmd = mgr.HandleAudioEvent(AudioEvent{Type: ASRFinal, Text: "等一下"})
	if cmd != CmdStopAgentPlayback {
		t.Errorf("expected CmdStopAgentPlayback, got %v", cmd)
	}

	// 5. 播放完成
	cmd = mgr.HandlePlaybackFinished()
	if cmd != CmdNone {
		t.Errorf("expected CmdNone (playback finished is internal), got %v", cmd)
	}
	if mgr.GetState().Turn != TurnUser {
		t.Error("should be TurnUser after playback finished")
	}

	t.Logf("state changes: %v", stateChanges)
	t.Logf("commands: %v", commands)
}

func TestConversationManager_VADPause(t *testing.T) {
	cg := NewCommandGenerator(WithPauseOnVADStart(true))
	state := State{Turn: TurnAgent, Phase: PhaseSpeaking}

	cmd := cg.GenerateForVAD(state, VADStart)
	if cmd != CmdPauseAgentPlayback {
		t.Errorf("expected CmdPauseAgentPlayback, got %v", cmd)
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
	q.PushSystem(voicechain.Event{Type: voicechain.StateAgentGenerating})
	q.PushPlaybackDone()

	if q.Len() != 4 {
		t.Errorf("expected queue length 4, got %d", q.Len())
	}
}
