package api

import (
	"crypto/subtle"
	"net/http"
	"strings"
	"time"

	"github.com/sipeed/picoclaw/pkg/agent"
	"github.com/sipeed/picoclaw/pkg/channels"
)

// Handler serves the PicoClaw REST API for state monitoring, configuration, and docs.
type Handler struct {
	agentLoop    *agent.AgentLoop
	chanMgr      *channels.Manager
	configPath   string
	eventHub     *EventHub
	startTime    time.Time
	apiToken     string
	allowOrigins []string
}

// NewHandler creates a new API handler.
func NewHandler(
	agentLoop *agent.AgentLoop,
	chanMgr *channels.Manager,
	configPath, apiToken string,
	allowOrigins ...string,
) *Handler {
	h := &Handler{
		agentLoop:    agentLoop,
		chanMgr:      chanMgr,
		configPath:   configPath,
		eventHub:     NewEventHub(),
		startTime:    time.Now(),
		apiToken:     apiToken,
		allowOrigins: allowOrigins,
	}
	agentLoop.SetEventBroadcaster(h.eventHub)
	return h
}

// RegisterRoutes registers all API handler routes on the given mux.
// This is designed to be passed as an extra registrar to Manager.SetupHTTPServer.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	// State monitoring endpoints (read-only, auth required)
	mux.HandleFunc("GET /api/v1/status", h.requireAuth(h.withJSON(h.handleStatus)))
	mux.HandleFunc("GET /api/v1/channels", h.requireAuth(h.withJSON(h.handleChannels)))
	mux.HandleFunc("GET /api/v1/agents", h.requireAuth(h.withJSON(h.handleAgents)))
	mux.HandleFunc("GET /api/v1/agents/{id}", h.requireAuth(h.withJSON(h.handleAgentByID)))
	mux.HandleFunc("GET /api/v1/tools", h.requireAuth(h.withJSON(h.handleTools)))
	mux.HandleFunc("GET /api/v1/sessions", h.requireAuth(h.withJSON(h.handleListSessions)))
	mux.HandleFunc("GET /api/v1/sessions/{id}", h.requireAuth(h.withJSON(h.handleGetSession)))
	mux.HandleFunc("DELETE /api/v1/sessions/{id}", h.requireAuth(h.handleDeleteSession))

	// Configuration endpoints (auth required)
	mux.HandleFunc("GET /api/v1/config", h.requireAuth(h.withJSON(h.handleGetConfig)))
	mux.HandleFunc("PUT /api/v1/config", h.requireAuth(h.handlePutConfig))
	mux.HandleFunc("PATCH /api/v1/config", h.requireAuth(h.handlePatchConfig))

	// Events WebSocket (auth required via query param)
	mux.HandleFunc("GET /api/v1/events/ws", h.requireAuth(h.handleEventsWS))

	// Documentation endpoints (public — no auth)
	mux.HandleFunc("GET /api/v1/openapi.yaml", h.handleOpenAPISpec)
	mux.Handle("GET /api/v1/docs/", http.StripPrefix("/api/v1/docs/", h.swaggerFileServer()))
	mux.HandleFunc("GET /api/v1/docs", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/api/v1/docs/", http.StatusMovedPermanently)
	})
}

// requireAuth is middleware that checks for a valid API token.
// If no token is configured, all requests are allowed (backwards compatible).
func (h *Handler) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if h.apiToken == "" {
			next(w, r)
			return
		}

		token := ""
		if auth := r.Header.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
			token = strings.TrimPrefix(auth, "Bearer ")
		}
		if token == "" {
			token = r.URL.Query().Get("token")
		}

		if subtle.ConstantTimeCompare([]byte(token), []byte(h.apiToken)) != 1 {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}

// withJSON is middleware that sets Content-Type: application/json.
func (h *Handler) withJSON(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		next(w, r)
	}
}
