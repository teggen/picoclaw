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

func TestEmitEvent_NilBroadcaster(t *testing.T) {
	al, _, _, _, cleanup := newTestAgentLoop(t)
	defer cleanup()

	// Should not panic with nil broadcaster.
	al.emitEvent(events.TurnStarted, map[string]any{"session": "test"})
}

func TestEmitEvent_WithBroadcaster(t *testing.T) {
	al, _, _, _, cleanup := newTestAgentLoop(t)
	defer cleanup()

	rec := &recordingBroadcaster{}
	al.SetEventBroadcaster(rec)

	al.emitEvent(events.TurnStarted, map[string]any{"session": "s1", "agent": "default"})
	al.emitEvent(events.ToolCallStarted, map[string]any{"session": "s1", "tool": "shell"})
	al.emitEvent(
		events.ToolCallCompleted,
		map[string]any{"session": "s1", "tool": "shell", "duration": 1.5, "isError": false},
	)
	al.emitEvent(events.TurnCompleted, map[string]any{"session": "s1", "iterations": 3})

	evts := rec.getEvents()
	if len(evts) != 4 {
		t.Fatalf("expected 4 events, got %d", len(evts))
	}

	expected := []string{
		events.TurnStarted,
		events.ToolCallStarted,
		events.ToolCallCompleted,
		events.TurnCompleted,
	}
	for i, want := range expected {
		if evts[i].eventType != want {
			t.Errorf("event[%d] type = %q, want %q", i, evts[i].eventType, want)
		}
	}
}

func TestSetEventBroadcaster_Replaces(t *testing.T) {
	al, _, _, _, cleanup := newTestAgentLoop(t)
	defer cleanup()

	rec1 := &recordingBroadcaster{}
	rec2 := &recordingBroadcaster{}

	al.SetEventBroadcaster(rec1)
	al.emitEvent(events.TurnStarted, nil)

	al.SetEventBroadcaster(rec2)
	al.emitEvent(events.TurnCompleted, nil)

	if len(rec1.getEvents()) != 1 {
		t.Fatalf("rec1 should have 1 event, got %d", len(rec1.getEvents()))
	}
	if len(rec2.getEvents()) != 1 {
		t.Fatalf("rec2 should have 1 event, got %d", len(rec2.getEvents()))
	}
}
