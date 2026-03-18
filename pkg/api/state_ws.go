package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"github.com/sipeed/picoclaw/pkg/logger"
)

// clientFilter controls which events a WebSocket client receives.
type clientFilter struct {
	typePrefix string // e.g. "tool.call." — only events with this prefix; "" = all
	session    string // specific session key; "" = all
	errorOnly  bool   // only .error events or isError=true results
}

// matches returns true if the event passes this filter.
func (f *clientFilter) matches(eventType string, data any) bool {
	if f.typePrefix != "" && !strings.HasPrefix(eventType, f.typePrefix) {
		return false
	}
	if f.session != "" {
		if m, ok := data.(map[string]any); ok {
			if s, ok := m["session"].(string); ok && s != f.session {
				return false
			}
		}
	}
	if f.errorOnly {
		isError := strings.HasSuffix(eventType, ".error")
		if m, ok := data.(map[string]any); ok {
			if ie, ok := m["isError"].(bool); ok && ie {
				isError = true
			}
		}
		if !isError {
			return false
		}
	}
	return true
}

// EventHub broadcasts state change events to connected WebSocket subscribers.
type EventHub struct {
	mu      sync.RWMutex
	clients map[*websocket.Conn]*clientFilter
}

// NewEventHub creates a new event hub.
func NewEventHub() *EventHub {
	return &EventHub{
		clients: make(map[*websocket.Conn]*clientFilter),
	}
}

// Broadcast sends an event to all connected clients whose filter matches.
func (eh *EventHub) Broadcast(eventType string, data any) {
	eh.mu.RLock()
	defer eh.mu.RUnlock()

	payload, _ := json.Marshal(map[string]any{
		"type":      eventType,
		"data":      data,
		"timestamp": time.Now().UnixMilli(),
	})

	for conn, filter := range eh.clients {
		if !filter.matches(eventType, data) {
			continue
		}
		if err := conn.WriteMessage(websocket.TextMessage, payload); err != nil {
			logger.DebugCF("api", "Event broadcast write failed", map[string]any{
				"error": err.Error(),
			})
		}
	}
}

func (eh *EventHub) addClient(conn *websocket.Conn, filter *clientFilter) {
	eh.mu.Lock()
	defer eh.mu.Unlock()
	eh.clients[conn] = filter
}

func (eh *EventHub) removeClient(conn *websocket.Conn) {
	eh.mu.Lock()
	defer eh.mu.Unlock()
	delete(eh.clients, conn)
}

var eventsUpgrader = websocket.Upgrader{
	CheckOrigin:     func(r *http.Request) bool { return true },
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

// parseClientFilter extracts filter parameters from query string.
func parseClientFilter(r *http.Request) *clientFilter {
	f := &clientFilter{}
	if t := r.URL.Query().Get("type"); t != "" {
		prefix := strings.TrimRight(t, "*")
		prefix = strings.TrimRight(prefix, ".")
		if prefix != "" {
			f.typePrefix = prefix + "."
		}
	}
	f.session = r.URL.Query().Get("session")
	if r.URL.Query().Get("status") == "error" {
		f.errorOnly = true
	}
	return f
}

// handleEventsWS upgrades to WebSocket and streams real-time state change events.
// GET /api/v1/events/ws
func (h *Handler) handleEventsWS(w http.ResponseWriter, r *http.Request) {
	conn, err := eventsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		logger.ErrorCF("api", "Events WebSocket upgrade failed", map[string]any{
			"error": err.Error(),
		})
		return
	}

	filter := parseClientFilter(r)
	h.eventHub.addClient(conn, filter)
	defer func() {
		h.eventHub.removeClient(conn)
		conn.Close()
	}()

	// Read loop — keeps the connection alive and detects disconnections.
	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			break
		}
	}
}
