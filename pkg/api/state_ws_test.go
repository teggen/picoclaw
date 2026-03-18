package api

import (
	"testing"
)

func TestEventHub_NewEventHub(t *testing.T) {
	eh := NewEventHub()
	if eh == nil {
		t.Fatal("expected non-nil EventHub")
	}
	if len(eh.clients) != 0 {
		t.Fatal("expected empty clients")
	}
}

func TestEventHub_Broadcast_NoClients(t *testing.T) {
	eh := NewEventHub()
	// Should not panic with no clients.
	eh.Broadcast("test.event", map[string]string{"key": "value"})
}
