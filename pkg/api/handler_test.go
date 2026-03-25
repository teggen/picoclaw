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
		apiToken:  "test-token",
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

func TestHandler_RequireAuth_NoToken(t *testing.T) {
	// When no token is configured, all requests pass through.
	h := &Handler{}
	handler := h.requireAuth(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/test", nil)
	handler(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 when no token configured, got %d", w.Code)
	}
}

func TestHandler_RequireAuth_ValidBearer(t *testing.T) {
	h := &Handler{apiToken: "secret-token"}
	handler := h.requireAuth(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/test", nil)
	r.Header.Set("Authorization", "Bearer secret-token")
	handler(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 with valid bearer token, got %d", w.Code)
	}
}

func TestHandler_RequireAuth_ValidQueryParam(t *testing.T) {
	h := &Handler{apiToken: "secret-token"}
	handler := h.requireAuth(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/test?token=secret-token", nil)
	handler(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 with valid query token, got %d", w.Code)
	}
}

func TestHandler_RequireAuth_InvalidToken(t *testing.T) {
	h := &Handler{apiToken: "secret-token"}
	handler := h.requireAuth(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/test", nil)
	r.Header.Set("Authorization", "Bearer wrong-token")
	handler(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 with invalid token, got %d", w.Code)
	}
}

func TestHandler_RequireAuth_MissingToken(t *testing.T) {
	h := &Handler{apiToken: "secret-token"}
	handler := h.requireAuth(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/test", nil)
	handler(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 with missing token, got %d", w.Code)
	}
}
