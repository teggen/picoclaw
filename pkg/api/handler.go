package api

import (
	"net/http"
	"time"

	"github.com/sipeed/picoclaw/pkg/agent"
	"github.com/sipeed/picoclaw/pkg/channels"
)

// Handler serves the PicoClaw REST API for state monitoring, configuration, and docs.
type Handler struct {
	agentLoop  *agent.AgentLoop
	chanMgr    *channels.Manager
	configPath string
	eventHub   *EventHub
	startTime  time.Time
}

// NewHandler creates a new API handler.
func NewHandler(agentLoop *agent.AgentLoop, chanMgr *channels.Manager, configPath string) *Handler {
	return &Handler{
		agentLoop:  agentLoop,
		chanMgr:    chanMgr,
		configPath: configPath,
		eventHub:   NewEventHub(),
		startTime:  time.Now(),
	}
}

// RegisterRoutes registers all API handler routes on the given mux.
// This is designed to be passed as an extra registrar to Manager.SetupHTTPServer.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	// State monitoring endpoints
	mux.HandleFunc("GET /api/v1/status", h.withJSON(h.handleStatus))
	mux.HandleFunc("GET /api/v1/channels", h.withJSON(h.handleChannels))
	mux.HandleFunc("GET /api/v1/agents", h.withJSON(h.handleAgents))
	mux.HandleFunc("GET /api/v1/agents/{id}", h.withJSON(h.handleAgentByID))
	mux.HandleFunc("GET /api/v1/tools", h.withJSON(h.handleTools))
	mux.HandleFunc("GET /api/v1/sessions", h.withJSON(h.handleListSessions))
	mux.HandleFunc("GET /api/v1/sessions/{id}", h.withJSON(h.handleGetSession))
	mux.HandleFunc("DELETE /api/v1/sessions/{id}", h.handleDeleteSession)

	// Configuration endpoints
	mux.HandleFunc("GET /api/v1/config", h.withJSON(h.handleGetConfig))
	mux.HandleFunc("PUT /api/v1/config", h.handlePutConfig)
	mux.HandleFunc("PATCH /api/v1/config", h.handlePatchConfig)

	// Events WebSocket
	mux.HandleFunc("GET /api/v1/events/ws", h.handleEventsWS)

	// Documentation endpoints
	mux.HandleFunc("GET /api/v1/openapi.yaml", h.handleOpenAPISpec)
	mux.Handle("GET /api/v1/docs/", http.StripPrefix("/api/v1/docs/", h.swaggerFileServer()))
	mux.HandleFunc("GET /api/v1/docs", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/api/v1/docs/", http.StatusMovedPermanently)
	})
}

// withJSON is middleware that sets Content-Type: application/json.
func (h *Handler) withJSON(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		next(w, r)
	}
}
