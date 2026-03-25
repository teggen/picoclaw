// turn_test.go contains unit tests for turn state management.
package agent

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sipeed/picoclaw/pkg/bus"
	"github.com/sipeed/picoclaw/pkg/providers"
	"github.com/sipeed/picoclaw/pkg/tools"
)

// newTestTurnState creates a turnState with minimal dependencies for unit testing.
func newTestTurnState(opts processOptions) *turnState {
	agent := &AgentInstance{
		ID: "test-agent",
	}
	scope := turnEventScope{
		agentID:    agent.ID,
		sessionKey: opts.SessionKey,
		turnID:     "turn-001",
	}
	return newTurnState(agent, opts, scope)
}

func TestTurnState_NewTurnState(t *testing.T) {
	tests := []struct {
		name        string
		opts        processOptions
		wantPhase   TurnPhase
		wantChannel string
		wantChatID  string
		wantMedia   []string
	}{
		{
			name: "basic creation",
			opts: processOptions{
				SessionKey:  "sess-1",
				Channel:     "telegram",
				ChatID:      "chat-42",
				UserMessage: "hello",
				Media:       []string{"media://abc"},
			},
			wantPhase:   TurnPhaseSetup,
			wantChannel: "telegram",
			wantChatID:  "chat-42",
			wantMedia:   []string{"media://abc"},
		},
		{
			name:        "empty options",
			opts:        processOptions{},
			wantPhase:   TurnPhaseSetup,
			wantChannel: "",
			wantChatID:  "",
			wantMedia:   nil,
		},
		{
			name: "nil media produces nil copy",
			opts: processOptions{
				SessionKey:  "sess-2",
				UserMessage: "test",
			},
			wantPhase: TurnPhaseSetup,
			wantMedia: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ts := newTestTurnState(tt.opts)

			assert.Equal(t, tt.wantPhase, ts.phase)
			assert.Equal(t, tt.wantChannel, ts.channel)
			assert.Equal(t, tt.wantChatID, ts.chatID)
			assert.Equal(t, tt.opts.UserMessage, ts.userMessage)
			assert.Equal(t, "test-agent", ts.agentID)
			assert.Equal(t, "turn-001", ts.turnID)
			assert.Equal(t, tt.opts.SessionKey, ts.sessionKey)
			assert.False(t, ts.startedAt.IsZero(), "startedAt should be set")
			if tt.wantMedia == nil {
				assert.Nil(t, ts.wantMedia())
			} else {
				assert.Equal(t, tt.wantMedia, ts.media)
			}
		})
	}
}

// wantMedia is a helper to avoid a method call on nil; returns ts.media.
func (ts *turnState) wantMedia() []string {
	return ts.media
}

func TestTurnState_MediaCopyIsolation(t *testing.T) {
	opts := processOptions{
		Media: []string{"media://one", "media://two"},
	}
	ts := newTestTurnState(opts)

	// Mutating the original slice should not affect the turnState copy.
	opts.Media[0] = "mutated"
	assert.Equal(t, "media://one", ts.media[0], "turnState media should be a defensive copy")
}

func TestTurnState_PhaseTransitions(t *testing.T) {
	ts := newTestTurnState(processOptions{})

	phases := []TurnPhase{
		TurnPhaseRunning,
		TurnPhaseTools,
		TurnPhaseFinalizing,
		TurnPhaseCompleted,
	}
	for _, p := range phases {
		ts.setPhase(p)
		snap := ts.snapshot()
		assert.Equal(t, p, snap.Phase)
	}
}

func TestTurnState_IterationGetSet(t *testing.T) {
	ts := newTestTurnState(processOptions{})

	assert.Equal(t, 0, ts.currentIteration())

	ts.setIteration(5)
	assert.Equal(t, 5, ts.currentIteration())

	ts.setIteration(0)
	assert.Equal(t, 0, ts.currentIteration())
}

func TestTurnState_FinalContent(t *testing.T) {
	ts := newTestTurnState(processOptions{})

	assert.Equal(t, 0, ts.finalContentLen())

	ts.setFinalContent("hello world")
	assert.Equal(t, len("hello world"), ts.finalContentLen())

	ts.setFinalContent("")
	assert.Equal(t, 0, ts.finalContentLen())
}

func TestTurnState_Snapshot(t *testing.T) {
	opts := processOptions{
		SessionKey:  "sess-snap",
		Channel:     "discord",
		ChatID:      "ch-99",
		UserMessage: "snapshot test",
	}
	ts := newTestTurnState(opts)
	ts.setPhase(TurnPhaseRunning)
	ts.setIteration(3)
	ts.mu.Lock()
	ts.depth = 2
	ts.parentTurnID = "parent-001"
	ts.childTurnIDs = []string{"child-a", "child-b"}
	ts.mu.Unlock()

	snap := ts.snapshot()

	assert.Equal(t, "turn-001", snap.TurnID)
	assert.Equal(t, "test-agent", snap.AgentID)
	assert.Equal(t, "sess-snap", snap.SessionKey)
	assert.Equal(t, "discord", snap.Channel)
	assert.Equal(t, "ch-99", snap.ChatID)
	assert.Equal(t, "snapshot test", snap.UserMessage)
	assert.Equal(t, TurnPhaseRunning, snap.Phase)
	assert.Equal(t, 3, snap.Iteration)
	assert.Equal(t, 2, snap.Depth)
	assert.Equal(t, "parent-001", snap.ParentTurnID)
	assert.Equal(t, []string{"child-a", "child-b"}, snap.ChildTurnIDs)
}

func TestTurnState_SnapshotChildIDsCopy(t *testing.T) {
	ts := newTestTurnState(processOptions{})
	ts.mu.Lock()
	ts.childTurnIDs = []string{"c1", "c2"}
	ts.mu.Unlock()

	snap := ts.snapshot()
	snap.ChildTurnIDs[0] = "mutated"

	ts.mu.RLock()
	assert.Equal(t, "c1", ts.childTurnIDs[0], "snapshot should return a defensive copy")
	ts.mu.RUnlock()
}

func TestTurnState_GracefulInterrupt(t *testing.T) {
	ts := newTestTurnState(processOptions{})

	// Initially no interrupt requested.
	requested, hint := ts.gracefulInterruptRequested()
	assert.False(t, requested)
	assert.Empty(t, hint)

	// Request graceful interrupt.
	ok := ts.requestGracefulInterrupt("user asked to stop")
	assert.True(t, ok)

	requested, hint = ts.gracefulInterruptRequested()
	assert.True(t, requested)
	assert.Equal(t, "user asked to stop", hint)

	// After marking terminal used, it should report not requested.
	ts.markGracefulTerminalUsed()
	requested, _ = ts.gracefulInterruptRequested()
	assert.False(t, requested)
}

func TestTurnState_GracefulInterruptBlockedByHardAbort(t *testing.T) {
	ts := newTestTurnState(processOptions{})

	// Hard abort first.
	ts.requestHardAbort()

	// Graceful interrupt should be rejected.
	ok := ts.requestGracefulInterrupt("too late")
	assert.False(t, ok)
}

func TestTurnState_HardAbort(t *testing.T) {
	ts := newTestTurnState(processOptions{})

	assert.False(t, ts.hardAbortRequested())

	// First hard abort returns true.
	ok := ts.requestHardAbort()
	assert.True(t, ok)
	assert.True(t, ts.hardAbortRequested())

	// Second hard abort returns false (idempotent).
	ok = ts.requestHardAbort()
	assert.False(t, ok)
}

func TestTurnState_HardAbortCallsCancelFuncs(t *testing.T) {
	ts := newTestTurnState(processOptions{})

	providerCancelled := false
	turnCancelled := false

	ts.setProviderCancel(func() { providerCancelled = true })
	ts.setTurnCancel(func() { turnCancelled = true })

	ts.requestHardAbort()

	assert.True(t, providerCancelled, "provider cancel should be called on hard abort")
	assert.True(t, turnCancelled, "turn cancel should be called on hard abort")
}

func TestTurnState_ClearProviderCancel(t *testing.T) {
	ts := newTestTurnState(processOptions{})

	called := false
	cancel := func() { called = true }
	ts.setProviderCancel(cancel)
	ts.clearProviderCancel(cancel)

	// After clearing, hard abort should not call the provider cancel.
	ts.requestHardAbort()
	assert.False(t, called, "cleared provider cancel should not be invoked")
}

func TestTurnState_EventMeta(t *testing.T) {
	opts := processOptions{SessionKey: "sess-meta"}
	ts := newTestTurnState(opts)
	ts.setIteration(7)

	meta := ts.eventMeta("test-source", "agent/turn")

	assert.Equal(t, "test-agent", meta.AgentID)
	assert.Equal(t, "turn-001", meta.TurnID)
	assert.Equal(t, "sess-meta", meta.SessionKey)
	assert.Equal(t, 7, meta.Iteration)
	assert.Equal(t, "test-source", meta.Source)
	assert.Equal(t, "agent/turn", meta.TracePath)
}

func TestTurnState_InterruptHintMessage(t *testing.T) {
	tests := []struct {
		name     string
		hint     string
		wantSub  string
		wantRole string
	}{
		{
			name:     "without hint",
			hint:     "",
			wantSub:  "Interrupt requested",
			wantRole: "user",
		},
		{
			name:     "with hint",
			hint:     "user wants summary",
			wantSub:  "Interrupt hint: user wants summary",
			wantRole: "user",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ts := newTestTurnState(processOptions{})
			ts.requestGracefulInterrupt(tt.hint)

			msg := ts.interruptHintMessage()
			assert.Equal(t, tt.wantRole, msg.Role)
			assert.Contains(t, msg.Content, tt.wantSub)
		})
	}
}

func TestTurnState_CaptureAndRestorePoint(t *testing.T) {
	ts := newTestTurnState(processOptions{})

	history := []providers.Message{
		{Role: "user", Content: "hi"},
		{Role: "assistant", Content: "hello"},
	}
	ts.captureRestorePoint(history, "summary-text")

	// Mutate original to verify defensive copy.
	history[0].Content = "mutated"

	ts.mu.RLock()
	assert.Equal(t, "hi", ts.restorePointHistory[0].Content)
	assert.Equal(t, "summary-text", ts.restorePointSummary)
	ts.mu.RUnlock()
}

func TestTurnState_RecordPersistedMessage(t *testing.T) {
	ts := newTestTurnState(processOptions{})

	ts.recordPersistedMessage(providers.Message{Role: "user", Content: "m1"})
	ts.recordPersistedMessage(providers.Message{Role: "assistant", Content: "m2"})

	ts.mu.RLock()
	require.Len(t, ts.persistedMessages, 2)
	assert.Equal(t, "m1", ts.persistedMessages[0].Content)
	assert.Equal(t, "m2", ts.persistedMessages[1].Content)
	ts.mu.RUnlock()
}

func TestTurnState_FinishAndFinishedChan(t *testing.T) {
	ts := newTestTurnState(processOptions{})
	ts.pendingResults = make(chan *tools.ToolResult, 1)

	ch := ts.Finished()
	require.NotNil(t, ch)

	// Should not be closed yet.
	select {
	case <-ch:
		t.Fatal("finishedChan should not be closed before Finish()")
	default:
	}

	ts.Finish(false)

	// Should be closed now.
	select {
	case <-ch:
		// OK
	case <-time.After(time.Second):
		t.Fatal("finishedChan should be closed after Finish()")
	}

	assert.True(t, ts.isFinished.Load())
}

func TestTurnState_FinishIdempotent(t *testing.T) {
	ts := newTestTurnState(processOptions{})

	// Call Finish multiple times; should not panic.
	ts.Finish(false)
	ts.Finish(false)
	ts.Finish(true)

	assert.True(t, ts.isFinished.Load())
}

func TestTurnState_IsParentEnded(t *testing.T) {
	parent := newTestTurnState(processOptions{})
	child := newTestTurnState(processOptions{})
	child.parentTurnState = parent

	assert.False(t, child.IsParentEnded())

	// Finishing a root turn (parentTurnState == nil) sets parentEnded.
	parent.Finish(false)
	assert.True(t, child.IsParentEnded())
}

func TestTurnState_IsParentEnded_NilParent(t *testing.T) {
	ts := newTestTurnState(processOptions{})
	assert.False(t, ts.IsParentEnded(), "root turn should return false")
}

func TestTurnState_LastFinishReason(t *testing.T) {
	ts := newTestTurnState(processOptions{})

	assert.Empty(t, ts.GetLastFinishReason())

	ts.SetLastFinishReason("stop")
	assert.Equal(t, "stop", ts.GetLastFinishReason())

	ts.SetLastFinishReason("length")
	assert.Equal(t, "length", ts.GetLastFinishReason())
}

func TestTurnState_LastUsage(t *testing.T) {
	ts := newTestTurnState(processOptions{})

	assert.Nil(t, ts.GetLastUsage())

	usage := &providers.UsageInfo{
		PromptTokens:     100,
		CompletionTokens: 50,
	}
	ts.SetLastUsage(usage)
	got := ts.GetLastUsage()
	require.NotNil(t, got)
	assert.Equal(t, 100, got.PromptTokens)
	assert.Equal(t, 50, got.CompletionTokens)

	// Set nil usage.
	ts.SetLastUsage(nil)
	assert.Nil(t, ts.GetLastUsage())
}

func TestTurnState_ContextHelpers(t *testing.T) {
	ts := newTestTurnState(processOptions{SessionKey: "ctx-sess"})

	// Store in context and retrieve.
	ctx := withTurnState(context.Background(), ts)
	got := turnStateFromContext(ctx)
	require.NotNil(t, got)
	assert.Equal(t, "ctx-sess", got.sessionKey)

	// Exported variant.
	got2 := TurnStateFromContext(ctx)
	require.NotNil(t, got2)
	assert.Equal(t, ts, got2)
}

func TestTurnState_ContextHelpers_NilContext(t *testing.T) {
	// Empty context returns nil.
	got := turnStateFromContext(context.Background())
	assert.Nil(t, got)

	got2 := TurnStateFromContext(context.Background())
	assert.Nil(t, got2)
}

func TestTurnResult_Fields(t *testing.T) {
	tr := turnResult{
		finalContent: "done",
		status:       TurnEndStatusCompleted,
		followUps: []bus.InboundMessage{
			{Content: "follow-up-1"},
		},
	}

	assert.Equal(t, "done", tr.finalContent)
	assert.Equal(t, TurnEndStatusCompleted, tr.status)
	require.Len(t, tr.followUps, 1)
	assert.Equal(t, "follow-up-1", tr.followUps[0].Content)
}

func TestTurnResult_EmptyFields(t *testing.T) {
	tr := turnResult{}

	assert.Empty(t, tr.finalContent)
	assert.Empty(t, string(tr.status))
	assert.Nil(t, tr.followUps)
}

func TestActiveTurnInfo_Fields(t *testing.T) {
	now := time.Now()
	info := ActiveTurnInfo{
		TurnID:       "t-1",
		AgentID:      "a-1",
		SessionKey:   "s-1",
		Channel:      "slack",
		ChatID:       "ch-1",
		UserMessage:  "hello",
		Phase:        TurnPhaseRunning,
		Iteration:    2,
		StartedAt:    now,
		Depth:        1,
		ParentTurnID: "t-0",
		ChildTurnIDs: []string{"t-2"},
	}

	assert.Equal(t, "t-1", info.TurnID)
	assert.Equal(t, "a-1", info.AgentID)
	assert.Equal(t, "slack", info.Channel)
	assert.Equal(t, TurnPhaseRunning, info.Phase)
	assert.Equal(t, 2, info.Iteration)
	assert.Equal(t, now, info.StartedAt)
	assert.Equal(t, 1, info.Depth)
	assert.Equal(t, "t-0", info.ParentTurnID)
	assert.Equal(t, []string{"t-2"}, info.ChildTurnIDs)
}

func TestTurnPhaseConstants(t *testing.T) {
	// Verify all phase constants have expected string values.
	phases := map[TurnPhase]string{
		TurnPhaseSetup:      "setup",
		TurnPhaseRunning:    "running",
		TurnPhaseTools:      "tools",
		TurnPhaseFinalizing: "finalizing",
		TurnPhaseCompleted:  "completed",
		TurnPhaseAborted:    "aborted",
	}
	for phase, want := range phases {
		assert.Equal(t, want, string(phase))
	}
}

func TestMatchingTurnMessageTail(t *testing.T) {
	tests := []struct {
		name      string
		history   []providers.Message
		persisted []providers.Message
		want      int
	}{
		{
			name:      "empty slices",
			history:   nil,
			persisted: nil,
			want:      0,
		},
		{
			name:      "no match",
			history:   []providers.Message{{Role: "user", Content: "a"}},
			persisted: []providers.Message{{Role: "user", Content: "b"}},
			want:      0,
		},
		{
			name: "full tail match",
			history: []providers.Message{
				{Role: "user", Content: "old"},
				{Role: "user", Content: "a"},
				{Role: "assistant", Content: "b"},
			},
			persisted: []providers.Message{
				{Role: "user", Content: "a"},
				{Role: "assistant", Content: "b"},
			},
			want: 2,
		},
		{
			name: "partial tail match",
			history: []providers.Message{
				{Role: "user", Content: "a"},
				{Role: "assistant", Content: "b"},
				{Role: "user", Content: "c"},
			},
			persisted: []providers.Message{
				{Role: "assistant", Content: "x"},
				{Role: "user", Content: "c"},
			},
			want: 1,
		},
		{
			name:      "persisted longer than history",
			history:   []providers.Message{{Role: "user", Content: "a"}},
			persisted: []providers.Message{{Role: "user", Content: "x"}, {Role: "user", Content: "a"}},
			want:      1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := matchingTurnMessageTail(tt.history, tt.persisted)
			assert.Equal(t, tt.want, got)
		})
	}
}
