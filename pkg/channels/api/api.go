package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"

	"github.com/sipeed/picoclaw/pkg/bus"
	"github.com/sipeed/picoclaw/pkg/channels"
	"github.com/sipeed/picoclaw/pkg/config"
	"github.com/sipeed/picoclaw/pkg/identity"
	"github.com/sipeed/picoclaw/pkg/logger"
)

// apiConn represents a single WebSocket connection.
type apiConn struct {
	id        string
	conn      *websocket.Conn
	sessionID string
	writeMu   sync.Mutex
	closed    atomic.Bool
}

func (ac *apiConn) writeJSON(v any) error {
	if ac.closed.Load() {
		return fmt.Errorf("connection closed")
	}
	ac.writeMu.Lock()
	defer ac.writeMu.Unlock()
	return ac.conn.WriteJSON(v)
}

func (ac *apiConn) close() {
	if ac.closed.CompareAndSwap(false, true) {
		ac.conn.Close()
	}
}

// pendingResponse is a channel used to collect the response for a synchronous REST or SSE request.
type pendingResponse struct {
	ch   chan string // receives deltas (SSE) or final response (sync)
	done chan struct{}
}

// APIChannel implements the API channel for REST and WebSocket chat.
type APIChannel struct {
	*channels.BaseChannel
	config      config.APIConfig
	upgrader    websocket.Upgrader
	connections sync.Map // connID → *apiConn
	connCount   atomic.Int32
	pending     sync.Map // chatID → *pendingResponse
	ctx         context.Context
	cancel      context.CancelFunc
}

// NewAPIChannel creates a new API channel.
func NewAPIChannel(cfg config.APIConfig, messageBus *bus.MessageBus) (*APIChannel, error) {
	base := channels.NewBaseChannel("api", cfg, messageBus, cfg.AllowFrom)

	allowOrigins := cfg.AllowOrigins
	checkOrigin := func(r *http.Request) bool {
		if len(allowOrigins) == 0 {
			return true
		}
		origin := r.Header.Get("Origin")
		for _, allowed := range allowOrigins {
			if allowed == "*" || allowed == origin {
				return true
			}
		}
		return false
	}

	return &APIChannel{
		BaseChannel: base,
		config:      cfg,
		upgrader: websocket.Upgrader{
			CheckOrigin:     checkOrigin,
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
		},
	}, nil
}

// Start implements Channel.
func (c *APIChannel) Start(ctx context.Context) error {
	logger.InfoC("api", "Starting API channel")
	c.ctx, c.cancel = context.WithCancel(ctx)
	c.SetRunning(true)
	logger.InfoC("api", "API channel started")
	return nil
}

// Stop implements Channel.
func (c *APIChannel) Stop(ctx context.Context) error {
	logger.InfoC("api", "Stopping API channel")
	c.SetRunning(false)

	c.connections.Range(func(key, value any) bool {
		if ac, ok := value.(*apiConn); ok {
			ac.close()
		}
		c.connections.Delete(key)
		return true
	})

	if c.cancel != nil {
		c.cancel()
	}

	logger.InfoC("api", "API channel stopped")
	return nil
}

// WebhookPath implements channels.WebhookHandler.
func (c *APIChannel) WebhookPath() string { return "/api/v1/" }

// ServeHTTP implements http.Handler — dispatches all /api/v1/chat* routes.
func (c *APIChannel) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/v1")

	// Apply CORS headers
	c.setCORSHeaders(w, r)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	switch {
	case path == "/chat/ws" || path == "/chat/ws/":
		c.handleWebSocket(w, r)
	case path == "/chat/stream" || path == "/chat/stream/":
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		c.handleChatStream(w, r)
	case path == "/chat" || path == "/chat/":
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		c.handleChatSync(w, r)
	default:
		http.NotFound(w, r)
	}
}

// Send implements Channel — routes outbound messages to WebSocket connections or pending REST responses.
func (c *APIChannel) Send(ctx context.Context, msg bus.OutboundMessage) error {
	if !c.IsRunning() {
		return channels.ErrNotRunning
	}

	sessionID := strings.TrimPrefix(msg.ChatID, "api:")

	logger.DebugCF("api", "Send called", map[string]any{
		"chat_id":    msg.ChatID,
		"session_id": sessionID,
		"content_len": len(msg.Content),
	})

	// Check if there's a pending REST/SSE response for this chat.
	if val, ok := c.pending.Load(msg.ChatID); ok {
		pr := val.(*pendingResponse)
		select {
		case pr.ch <- msg.Content:
			logger.DebugCF("api", "Delivered to pending response", map[string]any{
				"chat_id": msg.ChatID,
			})
		default:
			logger.WarnCF("api", "Pending response channel full, dropping", map[string]any{
				"chat_id": msg.ChatID,
			})
		}
		return nil
	}

	logger.DebugCF("api", "No pending response, broadcasting to WebSocket", map[string]any{
		"chat_id": msg.ChatID,
	})

	// Otherwise broadcast to WebSocket connections.
	outMsg := newMessage(TypeMessageCreate, map[string]any{
		"content": msg.Content,
	})
	return c.broadcastToSession(sessionID, outMsg)
}

// EditMessage implements channels.MessageEditor.
func (c *APIChannel) EditMessage(ctx context.Context, chatID string, messageID string, content string) error {
	sessionID := strings.TrimPrefix(chatID, "api:")
	outMsg := newMessage(TypeMessageUpdate, map[string]any{
		"message_id": messageID,
		"content":    content,
	})
	return c.broadcastToSession(sessionID, outMsg)
}

// StartTyping implements channels.TypingCapable.
func (c *APIChannel) StartTyping(ctx context.Context, chatID string) (func(), error) {
	sessionID := strings.TrimPrefix(chatID, "api:")
	startMsg := newMessage(TypeTypingStart, nil)
	if err := c.broadcastToSession(sessionID, startMsg); err != nil {
		return func() {}, err
	}
	return func() {
		stopMsg := newMessage(TypeTypingStop, nil)
		c.broadcastToSession(sessionID, stopMsg)
	}, nil
}

// broadcastToSession sends a message to all WebSocket connections with a matching session.
func (c *APIChannel) broadcastToSession(sessionID string, msg APIMessage) error {
	msg.SessionID = sessionID

	var sent bool
	c.connections.Range(func(key, value any) bool {
		ac, ok := value.(*apiConn)
		if !ok {
			return true
		}
		if ac.sessionID == sessionID {
			if err := ac.writeJSON(msg); err != nil {
				logger.DebugCF("api", "Write to connection failed", map[string]any{
					"conn_id": ac.id,
					"error":   err.Error(),
				})
			} else {
				sent = true
			}
		}
		return true
	})

	if !sent {
		return fmt.Errorf("no active connections for session %s: %w", sessionID, channels.ErrSendFailed)
	}
	return nil
}

// setCORSHeaders sets CORS headers based on config.
func (c *APIChannel) setCORSHeaders(w http.ResponseWriter, r *http.Request) {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return
	}

	allowed := false
	if len(c.config.AllowOrigins) == 0 {
		allowed = true
	} else {
		for _, o := range c.config.AllowOrigins {
			if o == "*" || o == origin {
				allowed = true
				break
			}
		}
	}

	if allowed {
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		w.Header().Set("Access-Control-Max-Age", "86400")
	}
}

// chatRequest is the body for POST /api/v1/chat and /api/v1/chat/stream.
type chatRequest struct {
	Message   string `json:"message"`
	SessionID string `json:"session_id,omitempty"`
}

// handleChatSync handles POST /api/v1/chat — synchronous chat.
func (c *APIChannel) handleChatSync(w http.ResponseWriter, r *http.Request) {
	if !c.IsRunning() {
		http.Error(w, "channel not running", http.StatusServiceUnavailable)
		return
	}

	var req chatRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if strings.TrimSpace(req.Message) == "" {
		http.Error(w, "message is required", http.StatusBadRequest)
		return
	}

	sessionID := req.SessionID
	if sessionID == "" {
		sessionID = uuid.New().String()
	}

	chatID := "api:" + sessionID

	// Register a pending response.
	pr := &pendingResponse{
		ch:   make(chan string, 1),
		done: make(chan struct{}),
	}
	c.pending.Store(chatID, pr)
	defer c.pending.Delete(chatID)

	logger.DebugCF("api", "Chat sync: publishing message", map[string]any{
		"chat_id":    chatID,
		"session_id": sessionID,
	})

	// Publish inbound message through the bus.
	c.publishMessage(sessionID, req.Message)

	// Extend the server's write deadline for this long-running request.
	rc := http.NewResponseController(w)
	rc.SetWriteDeadline(time.Now().Add(120 * time.Second))

	// Wait for the response with a timeout.
	timeout := 120 * time.Second
	select {
	case response := <-pr.ch:
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"response":   response,
			"session_id": sessionID,
		})
	case <-time.After(timeout):
		http.Error(w, "request timeout", http.StatusGatewayTimeout)
	case <-r.Context().Done():
		return
	}
}

// handleChatStream handles POST /api/v1/chat/stream — SSE streaming.
func (c *APIChannel) handleChatStream(w http.ResponseWriter, r *http.Request) {
	if !c.IsRunning() {
		http.Error(w, "channel not running", http.StatusServiceUnavailable)
		return
	}

	var req chatRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if strings.TrimSpace(req.Message) == "" {
		http.Error(w, "message is required", http.StatusBadRequest)
		return
	}

	sessionID := req.SessionID
	if sessionID == "" {
		sessionID = uuid.New().String()
	}

	chatID := "api:" + sessionID

	// Register a pending response for SSE.
	pr := &pendingResponse{
		ch:   make(chan string, 64),
		done: make(chan struct{}),
	}
	c.pending.Store(chatID, pr)
	defer c.pending.Delete(chatID)

	// Extend the server's write deadline for this long-running request.
	rc := http.NewResponseController(w)
	rc.SetWriteDeadline(time.Now().Add(120 * time.Second))

	// Set SSE headers.
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	// Publish inbound message through the bus.
	c.publishMessage(sessionID, req.Message)

	// Stream response chunks as SSE events.
	timeout := 120 * time.Second
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	for {
		select {
		case content := <-pr.ch:
			data, _ := json.Marshal(map[string]any{
				"type":       "done",
				"content":    content,
				"session_id": sessionID,
			})
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
			return
		case <-timer.C:
			data, _ := json.Marshal(map[string]any{
				"type":  "error",
				"error": "timeout",
			})
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
			return
		case <-r.Context().Done():
			return
		}
	}
}

// handleWebSocket upgrades the HTTP connection and manages the WebSocket lifecycle.
func (c *APIChannel) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	if !c.IsRunning() {
		http.Error(w, "channel not running", http.StatusServiceUnavailable)
		return
	}

	maxConns := c.config.MaxConnections
	if maxConns <= 0 {
		maxConns = 100
	}
	if int(c.connCount.Load()) >= maxConns {
		http.Error(w, "too many connections", http.StatusServiceUnavailable)
		return
	}

	conn, err := c.upgrader.Upgrade(w, r, nil)
	if err != nil {
		logger.ErrorCF("api", "WebSocket upgrade failed", map[string]any{
			"error": err.Error(),
		})
		return
	}

	sessionID := r.URL.Query().Get("session_id")
	if sessionID == "" {
		sessionID = uuid.New().String()
	}

	ac := &apiConn{
		id:        uuid.New().String(),
		conn:      conn,
		sessionID: sessionID,
	}

	c.connections.Store(ac.id, ac)
	c.connCount.Add(1)

	logger.InfoCF("api", "WebSocket client connected", map[string]any{
		"conn_id":    ac.id,
		"session_id": sessionID,
	})

	go c.readLoop(ac)
}

// readLoop reads messages from a WebSocket connection.
func (c *APIChannel) readLoop(ac *apiConn) {
	defer func() {
		ac.close()
		c.connections.Delete(ac.id)
		c.connCount.Add(-1)
		logger.InfoCF("api", "WebSocket client disconnected", map[string]any{
			"conn_id":    ac.id,
			"session_id": ac.sessionID,
		})
	}()

	readTimeout := time.Duration(c.config.ReadTimeout) * time.Second
	if readTimeout <= 0 {
		readTimeout = 60 * time.Second
	}

	_ = ac.conn.SetReadDeadline(time.Now().Add(readTimeout))
	ac.conn.SetPongHandler(func(appData string) error {
		_ = ac.conn.SetReadDeadline(time.Now().Add(readTimeout))
		return nil
	})

	pingInterval := time.Duration(c.config.PingInterval) * time.Second
	if pingInterval <= 0 {
		pingInterval = 30 * time.Second
	}
	go c.pingLoop(ac, pingInterval)

	for {
		select {
		case <-c.ctx.Done():
			return
		default:
		}

		_, rawMsg, err := ac.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				logger.DebugCF("api", "WebSocket read error", map[string]any{
					"conn_id": ac.id,
					"error":   err.Error(),
				})
			}
			return
		}

		_ = ac.conn.SetReadDeadline(time.Now().Add(readTimeout))

		var msg APIMessage
		if err := json.Unmarshal(rawMsg, &msg); err != nil {
			errMsg := newError("invalid_message", "failed to parse message")
			ac.writeJSON(errMsg)
			continue
		}

		c.handleMessage(ac, msg)
	}
}

// pingLoop sends periodic ping frames.
func (c *APIChannel) pingLoop(ac *apiConn, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-c.ctx.Done():
			return
		case <-ticker.C:
			if ac.closed.Load() {
				return
			}
			ac.writeMu.Lock()
			err := ac.conn.WriteMessage(websocket.PingMessage, nil)
			ac.writeMu.Unlock()
			if err != nil {
				return
			}
		}
	}
}

// handleMessage processes an inbound API Protocol message.
func (c *APIChannel) handleMessage(ac *apiConn, msg APIMessage) {
	switch msg.Type {
	case TypePing:
		pong := newMessage(TypePong, nil)
		pong.ID = msg.ID
		ac.writeJSON(pong)

	case TypeMessageSend:
		c.handleMessageSend(ac, msg)

	default:
		errMsg := newError("unknown_type", fmt.Sprintf("unknown message type: %s", msg.Type))
		ac.writeJSON(errMsg)
	}
}

// handleMessageSend processes an inbound message.send from a WebSocket client.
func (c *APIChannel) handleMessageSend(ac *apiConn, msg APIMessage) {
	content, _ := msg.Payload["content"].(string)
	if strings.TrimSpace(content) == "" {
		errMsg := newError("empty_content", "message content is empty")
		ac.writeJSON(errMsg)
		return
	}

	sessionID := msg.SessionID
	if sessionID == "" {
		sessionID = ac.sessionID
	}

	c.publishMessage(sessionID, content)
}

// publishMessage publishes a message through the bus using BaseChannel.HandleMessage.
func (c *APIChannel) publishMessage(sessionID, content string) {
	chatID := "api:" + sessionID
	senderID := "api-user"
	peer := bus.Peer{Kind: "direct", ID: "api:" + sessionID}

	metadata := map[string]string{
		"platform":   "api",
		"session_id": sessionID,
	}

	sender := bus.SenderInfo{
		Platform:    "api",
		PlatformID:  senderID,
		CanonicalID: identity.BuildCanonicalID("api", senderID),
	}

	c.HandleMessage(c.ctx, peer, "", senderID, chatID, content, nil, metadata, sender)
}

// truncate truncates a string to maxLen runes.
func truncate(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "..."
}
