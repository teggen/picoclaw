package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHandler_RegisterRoutes(t *testing.T) {
	mux := http.NewServeMux()
	h := &Handler{
		eventHub:  NewEventHub(),
		startTime: time.Now(),
	}
	// Should not panic.
	h.RegisterRoutes(mux)
}

func TestHandler_WithJSON(t *testing.T) {
	h := &Handler{}
	handler := h.withJSON(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"ok":true}`))
	})

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/test", nil)
	handler(w, r)

	if w.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("expected Content-Type application/json, got %q", w.Header().Get("Content-Type"))
	}
}
