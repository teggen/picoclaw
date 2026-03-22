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
	al.emitEvent(EventKindTurnStart, EventMeta{AgentID: "test"}, nil)
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
	al.mu.RLock()
	b := al.eventBroadcaster
	al.mu.RUnlock()
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
