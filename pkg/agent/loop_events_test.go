package agent

import (
	"sync"
	"testing"

	"github.com/sipeed/picoclaw/pkg/events"
)

type recordingBroadcaster struct {
	mu     sync.Mutex
	events []struct {
		eventType string
		data      any
	}
}

func (r *recordingBroadcaster) Broadcast(eventType string, data any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, struct {
		eventType string
		data      any
	}{eventType, data})
}

func (r *recordingBroadcaster) getEvents() []struct {
	eventType string
	data      any
} {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := make([]struct {
		eventType string
		data      any
	}, len(r.events))
	copy(cp, r.events)
	return cp
}

func TestEmitEvent_NilEventBus(t *testing.T) {
	al, _, _, _, cleanup := newTestAgentLoop(t)
	defer cleanup()

	// Should not panic with nil eventBus.
	al.events.emitEvent(EventKindTurnStart, EventMeta{AgentID: "test"}, nil)
}

func TestSetEventBroadcaster_Replaces(t *testing.T) {
	al, _, _, _, cleanup := newTestAgentLoop(t)
	defer cleanup()

	rec1 := &recordingBroadcaster{}
	rec2 := &recordingBroadcaster{}

	al.SetEventBroadcaster(rec1)
	_ = rec1 // broadcaster is set but emitEvent now uses EventBus, not broadcaster

	al.SetEventBroadcaster(rec2)
	_ = rec2

	// Verify the broadcaster field is updated
	al.events.mu.RLock()
	b := al.events.eventBroadcaster
	al.events.mu.RUnlock()
	if b != rec2 {
		t.Error("expected eventBroadcaster to be rec2")
	}
}

// TestEventBroadcasterInterface verifies the events.Broadcaster interface constants exist.
func TestEventBroadcasterInterface(t *testing.T) {
	constants := []string{
		events.SessionStarted,
		events.SessionCleared,
		events.TurnStarted,
		events.TurnCompleted,
		events.TurnError,
		events.ToolCallStarted,
		events.ToolCallCompleted,
	}
	for _, c := range constants {
		if c == "" {
			t.Error("event constant should not be empty")
		}
	}
}

func TestEmitEvent_SubscriberReceivesEvent(t *testing.T) {
	al, _, _, _, cleanup := newTestAgentLoop(t)
	defer cleanup()

	sub := al.SubscribeEvents(4)
	defer al.UnsubscribeEvents(sub.ID)

	meta := EventMeta{AgentID: "a1", SessionKey: "s1", TurnID: "t1"}
	payload := TurnStartPayload{Channel: "test", UserMessage: "hello", MediaCount: 0}
	al.events.emitEvent(EventKindTurnStart, meta, payload)

	select {
	case evt := <-sub.C:
		if evt.Kind != EventKindTurnStart {
			t.Errorf("Kind = %v, want EventKindTurnStart", evt.Kind)
		}
		if evt.Meta.AgentID != "a1" {
			t.Errorf("AgentID = %q, want %q", evt.Meta.AgentID, "a1")
		}
		p, ok := evt.Payload.(TurnStartPayload)
		if !ok {
			t.Fatalf("payload type = %T, want TurnStartPayload", evt.Payload)
		}
		if p.Channel != "test" {
			t.Errorf("Channel = %q, want %q", p.Channel, "test")
		}
	default:
		t.Fatal("subscriber did not receive event")
	}
}

func TestEmitEvent_MultipleSubscribers(t *testing.T) {
	al, _, _, _, cleanup := newTestAgentLoop(t)
	defer cleanup()

	sub1 := al.SubscribeEvents(4)
	sub2 := al.SubscribeEvents(4)
	defer al.UnsubscribeEvents(sub1.ID)
	defer al.UnsubscribeEvents(sub2.ID)

	al.events.emitEvent(EventKindTurnEnd, EventMeta{AgentID: "a"}, TurnEndPayload{Status: TurnEndStatusCompleted})

	for i, sub := range []EventSubscription{sub1, sub2} {
		select {
		case evt := <-sub.C:
			if evt.Kind != EventKindTurnEnd {
				t.Errorf("sub%d: Kind = %v, want EventKindTurnEnd", i+1, evt.Kind)
			}
		default:
			t.Errorf("sub%d: did not receive event", i+1)
		}
	}
}

func TestNewTurnEventScope_SequentialIDs(t *testing.T) {
	al, _, _, _, cleanup := newTestAgentLoop(t)
	defer cleanup()

	scope1 := al.events.newTurnEventScope("agent-1", "session-a")
	scope2 := al.events.newTurnEventScope("agent-1", "session-a")
	scope3 := al.events.newTurnEventScope("agent-2", "session-b")

	if scope1.turnID == scope2.turnID {
		t.Errorf("expected unique turn IDs, both are %q", scope1.turnID)
	}
	if scope1.agentID != "agent-1" {
		t.Errorf("agentID = %q, want %q", scope1.agentID, "agent-1")
	}
	if scope1.sessionKey != "session-a" {
		t.Errorf("sessionKey = %q, want %q", scope1.sessionKey, "session-a")
	}
	if scope3.agentID != "agent-2" {
		t.Errorf("scope3 agentID = %q, want %q", scope3.agentID, "agent-2")
	}
}

func TestTurnEventScope_Meta(t *testing.T) {
	scope := turnEventScope{
		agentID:    "a1",
		sessionKey: "s1",
		turnID:     "a1-turn-5",
	}
	meta := scope.meta(3, "llm", "turn.llm.request")
	if meta.AgentID != "a1" {
		t.Errorf("AgentID = %q, want %q", meta.AgentID, "a1")
	}
	if meta.TurnID != "a1-turn-5" {
		t.Errorf("TurnID = %q, want %q", meta.TurnID, "a1-turn-5")
	}
	if meta.SessionKey != "s1" {
		t.Errorf("SessionKey = %q, want %q", meta.SessionKey, "s1")
	}
	if meta.Iteration != 3 {
		t.Errorf("Iteration = %d, want 3", meta.Iteration)
	}
	if meta.Source != "llm" {
		t.Errorf("Source = %q, want %q", meta.Source, "llm")
	}
	if meta.TracePath != "turn.llm.request" {
		t.Errorf("TracePath = %q, want %q", meta.TracePath, "turn.llm.request")
	}
}

func TestCloneEventArguments_Nil(t *testing.T) {
	got := cloneEventArguments(nil)
	if got != nil {
		t.Errorf("cloneEventArguments(nil) = %v, want nil", got)
	}
}

func TestCloneEventArguments_Empty(t *testing.T) {
	got := cloneEventArguments(map[string]any{})
	if got != nil {
		t.Errorf("cloneEventArguments(empty) = %v, want nil", got)
	}
}

func TestCloneEventArguments_CopiesEntries(t *testing.T) {
	original := map[string]any{"key": "value", "num": 42}
	cloned := cloneEventArguments(original)

	if cloned["key"] != "value" {
		t.Errorf("cloned[key] = %v, want %q", cloned["key"], "value")
	}
	if cloned["num"] != 42 {
		t.Errorf("cloned[num] = %v, want 42", cloned["num"])
	}

	// Mutation of original should not affect clone.
	original["key"] = "changed"
	if cloned["key"] != "value" {
		t.Error("clone was affected by mutation of original")
	}
}

func TestHookAbortError_DefaultReason(t *testing.T) {
	al, _, _, _, cleanup := newTestAgentLoop(t)
	defer cleanup()

	ts := &turnState{}
	ts.scope = turnEventScope{agentID: "a1", sessionKey: "s1", turnID: "t1"}
	err := al.hookAbortError(ts, "before_llm", HookDecision{Action: HookActionAbortTurn})

	if err == nil {
		t.Fatal("expected error")
	}
	if !contains(err.Error(), "hook aborted turn during before_llm") {
		t.Errorf("error = %q, want contains 'hook aborted turn during before_llm'", err.Error())
	}
	if !contains(err.Error(), "hook requested turn abort") {
		t.Errorf("error = %q, want default reason", err.Error())
	}
}

func TestHookAbortError_CustomReason(t *testing.T) {
	al, _, _, _, cleanup := newTestAgentLoop(t)
	defer cleanup()

	ts := &turnState{}
	ts.scope = turnEventScope{agentID: "a1", sessionKey: "s1", turnID: "t1"}
	err := al.hookAbortError(ts, "after_tool", HookDecision{Action: HookActionAbortTurn, Reason: "rate limited"})

	if err == nil {
		t.Fatal("expected error")
	}
	if !contains(err.Error(), "rate limited") {
		t.Errorf("error = %q, want contains 'rate limited'", err.Error())
	}
}

func TestHookDeniedToolContent_EmptyReason(t *testing.T) {
	got := hookDeniedToolContent("Tool denied", "")
	if got != "Tool denied" {
		t.Errorf("got %q, want %q", got, "Tool denied")
	}
}

func TestHookDeniedToolContent_WithReason(t *testing.T) {
	got := hookDeniedToolContent("Tool denied", "not allowed")
	if got != "Tool denied: not allowed" {
		t.Errorf("got %q, want %q", got, "Tool denied: not allowed")
	}
}

func TestSubscribeEvents_NilLoop(t *testing.T) {
	var al *AgentLoop
	sub := al.SubscribeEvents(4)
	// Channel should be closed for nil loop.
	_, ok := <-sub.C
	if ok {
		t.Error("expected closed channel for nil AgentLoop")
	}
}

func TestUnsubscribeEvents_NilLoop(t *testing.T) {
	var al *AgentLoop
	// Should not panic.
	al.UnsubscribeEvents(999)
}

func TestEventDrops_NilLoop(t *testing.T) {
	var al *AgentLoop
	drops := al.EventDrops(EventKindTurnStart)
	if drops != 0 {
		t.Errorf("EventDrops on nil loop = %d, want 0", drops)
	}
}

func TestMountHook_NilLoop(t *testing.T) {
	var al *AgentLoop
	err := al.MountHook(HookRegistration{Name: "test"})
	if err == nil {
		t.Error("expected error for nil loop")
	}
}

func TestUnmountHook_NilLoop(t *testing.T) {
	var al *AgentLoop
	// Should not panic.
	al.UnmountHook("test")
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
