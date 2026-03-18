package events

import "testing"

type mockBroadcaster struct {
	calls []struct {
		eventType string
		data      any
	}
}

func (m *mockBroadcaster) Broadcast(eventType string, data any) {
	m.calls = append(m.calls, struct {
		eventType string
		data      any
	}{eventType, data})
}

func TestBroadcasterInterface(t *testing.T) {
	var b Broadcaster = &mockBroadcaster{}
	b.Broadcast(TurnStarted, map[string]any{"session": "test"})

	mock := b.(*mockBroadcaster)
	if len(mock.calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(mock.calls))
	}
	if mock.calls[0].eventType != TurnStarted {
		t.Fatalf("expected %s, got %s", TurnStarted, mock.calls[0].eventType)
	}
}

func TestConstants(t *testing.T) {
	constants := []string{
		SessionStarted, SessionCleared, SessionSummarized,
		TurnStarted, TurnCompleted, TurnError,
		ToolCallStarted, ToolCallCompleted, ConfigUpdated,
	}
	for _, c := range constants {
		if c == "" {
			t.Fatal("event constant must not be empty")
		}
	}
}
