package api

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"github.com/sipeed/picoclaw/pkg/logger"
)

// EventHub broadcasts state change events to connected WebSocket subscribers.
type EventHub struct {
	mu      sync.RWMutex
	clients map[*websocket.Conn]struct{}
}

// NewEventHub creates a new event hub.
func NewEventHub() *EventHub {
	return &EventHub{
		clients: make(map[*websocket.Conn]struct{}),
	}
}

// Broadcast sends an event to all connected clients.
func (eh *EventHub) Broadcast(eventType string, data any) {
	eh.mu.RLock()
	defer eh.mu.RUnlock()

	payload, _ := json.Marshal(map[string]any{
		"type":      eventType,
		"data":      data,
		"timestamp": time.Now().UnixMilli(),
	})

	for conn := range eh.clients {
		if err := conn.WriteMessage(websocket.TextMessage, payload); err != nil {
			logger.DebugCF("api", "Event broadcast write failed", map[string]any{
				"error": err.Error(),
			})
		}
	}
}

func (eh *EventHub) addClient(conn *websocket.Conn) {
	eh.mu.Lock()
	defer eh.mu.Unlock()
	eh.clients[conn] = struct{}{}
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

	h.eventHub.addClient(conn)
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
