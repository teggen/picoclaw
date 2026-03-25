package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestEventHub_NewEventHub(t *testing.T) {
	eh := NewEventHub()
	if eh == nil {
		t.Fatal("expected non-nil EventHub")
	}
}

func TestNewEventsUpgrader_DenyByDefault(t *testing.T) {
	// With no allowed origins, browser requests (with Origin header) should be denied.
	upgrader := newEventsUpgrader(nil)
	r := httptest.NewRequest(http.MethodGet, "/ws", nil)
	r.Header.Set("Origin", "http://evil.com")
	if upgrader.CheckOrigin(r) {
		t.Fatal("expected deny when no origins configured")
	}
}

func TestNewEventsUpgrader_AllowConfigured(t *testing.T) {
	upgrader := newEventsUpgrader([]string{"http://localhost:3000"})
	r := httptest.NewRequest(http.MethodGet, "/ws", nil)
	r.Header.Set("Origin", "http://localhost:3000")
	if !upgrader.CheckOrigin(r) {
		t.Fatal("expected allow for configured origin")
	}
}

func TestNewEventsUpgrader_AllowNonBrowser(t *testing.T) {
	// Requests without Origin header (non-browser) should be allowed.
	upgrader := newEventsUpgrader(nil)
	r := httptest.NewRequest(http.MethodGet, "/ws", nil)
	if !upgrader.CheckOrigin(r) {
		t.Fatal("expected allow for non-browser client (no Origin header)")
	}
}

func TestEventHub_Broadcast_NoClients(t *testing.T) {
	eh := NewEventHub()
	// Should not panic with no clients.
	eh.Broadcast("test.event", map[string]string{"key": "value"})
}

func TestClientFilter_Matches_NoFilter(t *testing.T) {
	f := &clientFilter{}
	if !f.matches("turn.started", nil) {
		t.Fatal("empty filter should match all events")
	}
	if !f.matches("tool.call.completed", map[string]any{"tool": "shell"}) {
		t.Fatal("empty filter should match all events")
	}
}

func TestClientFilter_Matches_TypePrefix(t *testing.T) {
	f := &clientFilter{typePrefix: "tool.call."}

	if !f.matches("tool.call.started", nil) {
		t.Fatal("should match tool.call.started")
	}
	if !f.matches("tool.call.completed", nil) {
		t.Fatal("should match tool.call.completed")
	}
	if f.matches("turn.started", nil) {
		t.Fatal("should not match turn.started")
	}
	if f.matches("config.updated", nil) {
		t.Fatal("should not match config.updated")
	}
}

func TestClientFilter_Matches_Session(t *testing.T) {
	f := &clientFilter{session: "tg:123"}

	if !f.matches("turn.started", map[string]any{"session": "tg:123"}) {
		t.Fatal("should match matching session")
	}
	if f.matches("turn.started", map[string]any{"session": "discord:456"}) {
		t.Fatal("should not match different session")
	}
	// Events without session data should pass (e.g. config.updated).
	if !f.matches("config.updated", nil) {
		t.Fatal("should pass events without session data")
	}
	if !f.matches("config.updated", map[string]any{"other": "field"}) {
		t.Fatal("should pass events without session field in data")
	}
}

func TestClientFilter_Matches_ErrorOnly(t *testing.T) {
	f := &clientFilter{errorOnly: true}

	if !f.matches("turn.error", nil) {
		t.Fatal("should match .error suffix")
	}
	if !f.matches("tool.call.completed", map[string]any{"isError": true}) {
		t.Fatal("should match isError=true")
	}
	if f.matches("turn.started", nil) {
		t.Fatal("should not match non-error event")
	}
	if f.matches("tool.call.completed", map[string]any{"isError": false}) {
		t.Fatal("should not match isError=false")
	}
}

func TestClientFilter_Matches_Combined(t *testing.T) {
	f := &clientFilter{
		typePrefix: "turn.",
		session:    "tg:123",
		errorOnly:  true,
	}

	if !f.matches("turn.error", map[string]any{"session": "tg:123"}) {
		t.Fatal("should match all criteria")
	}
	if f.matches("tool.call.started", map[string]any{"session": "tg:123"}) {
		t.Fatal("should not match wrong type prefix")
	}
	if f.matches("turn.error", map[string]any{"session": "discord:456"}) {
		t.Fatal("should not match wrong session")
	}
	if f.matches("turn.started", map[string]any{"session": "tg:123"}) {
		t.Fatal("should not match non-error")
	}
}

func TestParseClientFilter(t *testing.T) {
	tests := []struct {
		name       string
		query      string
		wantPrefix string
		wantSess   string
		wantErrors bool
	}{
		{"empty", "/api/v1/events/ws", "", "", false},
		{"type with star", "/api/v1/events/ws?type=tool.*", "tool.", "", false},
		{"type with dot star", "/api/v1/events/ws?type=session.*", "session.", "", false},
		{"session", "/api/v1/events/ws?session=tg:123", "", "tg:123", false},
		{"errors", "/api/v1/events/ws?status=error", "", "", true},
		{"combined", "/api/v1/events/ws?type=tool.*&session=abc&status=error", "tool.", "abc", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.query, nil)
			f := parseClientFilter(req)
			if f.typePrefix != tt.wantPrefix {
				t.Errorf("typePrefix = %q, want %q", f.typePrefix, tt.wantPrefix)
			}
			if f.session != tt.wantSess {
				t.Errorf("session = %q, want %q", f.session, tt.wantSess)
			}
			if f.errorOnly != tt.wantErrors {
				t.Errorf("errorOnly = %v, want %v", f.errorOnly, tt.wantErrors)
			}
		})
	}
}
