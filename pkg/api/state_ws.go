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

const (
	eventsWriteBufSize = 64
	eventsMaxReadSize  = 64 * 1024 // 64 KB
)

// eventClient wraps a WebSocket connection with a buffered write channel
// to avoid concurrent writes on gorilla/websocket connections.
type eventClient struct {
	conn   *websocket.Conn
	filter *clientFilter
	send   chan []byte
	done   chan struct{}
}

func newEventClient(conn *websocket.Conn, filter *clientFilter) *eventClient {
	return &eventClient{
		conn:   conn,
		filter: filter,
		send:   make(chan []byte, eventsWriteBufSize),
		done:   make(chan struct{}),
	}
}

// writePump drains the send channel and writes messages sequentially.
func (ec *eventClient) writePump() {
	defer ec.conn.Close()
	for {
		select {
		case msg, ok := <-ec.send:
			if !ok {
				return
			}
			if err := ec.conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				return
			}
		case <-ec.done:
			return
		}
	}
}

func (ec *eventClient) close() {
	select {
	case <-ec.done:
	default:
		close(ec.done)
	}
}

// EventHub broadcasts state change events to connected WebSocket subscribers.
type EventHub struct {
	mu      sync.RWMutex
	clients map[*websocket.Conn]*eventClient
}

// NewEventHub creates a new event hub.
func NewEventHub() *EventHub {
	return &EventHub{
		clients: make(map[*websocket.Conn]*eventClient),
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

	for _, client := range eh.clients {
		if !client.filter.matches(eventType, data) {
			continue
		}
		select {
		case client.send <- payload:
		default:
			logger.DebugCF("api", "Event broadcast dropped (client buffer full)", nil)
		}
	}
}

func (eh *EventHub) addClient(conn *websocket.Conn, filter *clientFilter) *eventClient {
	ec := newEventClient(conn, filter)
	eh.mu.Lock()
	eh.clients[conn] = ec
	eh.mu.Unlock()
	go ec.writePump()
	return ec
}

func (eh *EventHub) removeClient(conn *websocket.Conn) {
	eh.mu.Lock()
	ec, ok := eh.clients[conn]
	delete(eh.clients, conn)
	eh.mu.Unlock()
	if ok {
		ec.close()
	}
}

func newEventsUpgrader(allowOrigins []string) websocket.Upgrader {
	return websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			origin := r.Header.Get("Origin")
			if origin == "" {
				return true // non-browser clients (curl, CLI, etc.)
			}
			if len(allowOrigins) == 0 {
				return false // deny browser requests when no origins configured
			}
			for _, allowed := range allowOrigins {
				if allowed == "*" || allowed == origin {
					return true
				}
			}
			return false
		},
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
	}
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
	upgrader := newEventsUpgrader(h.allowOrigins)
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		logger.ErrorCF("api", "Events WebSocket upgrade failed", map[string]any{
			"error": err.Error(),
		})
		return
	}
	conn.SetReadLimit(eventsMaxReadSize)

	filter := parseClientFilter(r)
	ec := h.eventHub.addClient(conn, filter)
	defer func() {
		h.eventHub.removeClient(conn)
		ec.close()
	}()

	// Read loop — keeps the connection alive and detects disconnections.
	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			break
		}
	}
}
