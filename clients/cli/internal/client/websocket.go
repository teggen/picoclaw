package client

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// ChatConn is a WebSocket connection for interactive chat with Bubble Tea integration.
type ChatConn struct {
	conn    *websocket.Conn
	msgs    chan APIMessage
	done    chan struct{}
	writeMu sync.Mutex
	closed  bool
	mu      sync.Mutex

	// Reconnection config.
	baseURL   string
	sessionID string
}

// DialChat connects to the chat WebSocket at the given gateway URL.
func DialChat(baseURL, sessionID string) (*ChatConn, error) {
	conn, sid, err := dialChatWS(baseURL, sessionID)
	if err != nil {
		return nil, err
	}

	c := &ChatConn{
		conn:      conn,
		msgs:      make(chan APIMessage, 64),
		done:      make(chan struct{}),
		baseURL:   baseURL,
		sessionID: sid,
	}
	go c.readLoop()
	return c, nil
}

func dialChatWS(baseURL, sessionID string) (*websocket.Conn, string, error) {
	wsURL := httpToWS(baseURL) + "/api/v1/chat/ws"
	if sessionID != "" {
		wsURL += "?session_id=" + url.QueryEscape(sessionID)
	}

	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}
	conn, _, err := dialer.Dial(wsURL, http.Header{})
	if err != nil {
		return nil, "", fmt.Errorf("cannot connect to gateway WebSocket at %s — is the gateway running?", baseURL)
	}
	return conn, sessionID, nil
}

// Messages returns the channel that delivers incoming WebSocket messages.
func (c *ChatConn) Messages() <-chan APIMessage {
	return c.msgs
}

// SessionID returns the session ID for this connection.
func (c *ChatConn) SessionID() string {
	return c.sessionID
}

// Send sends a chat message to the gateway.
func (c *ChatConn) Send(content string) error {
	msg := APIMessage{
		Type:      TypeMessageSend,
		Timestamp: time.Now().UnixMilli(),
		Payload:   map[string]any{"content": content},
	}
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	return c.conn.WriteJSON(msg)
}

// Close closes the WebSocket connection.
func (c *ChatConn) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return nil
	}
	c.closed = true
	close(c.done)
	return c.conn.Close()
}

func (c *ChatConn) readLoop() {
	defer close(c.msgs)
	for {
		var msg APIMessage
		if err := c.conn.ReadJSON(&msg); err != nil {
			c.mu.Lock()
			closed := c.closed
			c.mu.Unlock()
			if closed {
				return
			}
			// Try to reconnect.
			if c.reconnect() {
				continue
			}
			return
		}
		// Capture session ID from first server message.
		if c.sessionID == "" && msg.SessionID != "" {
			c.sessionID = msg.SessionID
		}
		select {
		case c.msgs <- msg:
		case <-c.done:
			return
		}
	}
}

func (c *ChatConn) reconnect() bool {
	backoff := time.Second
	maxBackoff := 30 * time.Second
	attempts := 5

	for i := range attempts {
		select {
		case <-c.done:
			return false
		case <-time.After(backoff):
		}

		conn, _, err := dialChatWS(c.baseURL, c.sessionID)
		if err == nil {
			c.writeMu.Lock()
			c.conn = conn
			c.writeMu.Unlock()
			// Signal reconnection.
			select {
			case c.msgs <- APIMessage{Type: "reconnected"}:
			default:
			}
			return true
		}

		if i < attempts-1 {
			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
		}
	}
	return false
}

// EventFilter specifies server-side filtering for the event stream.
type EventFilter struct {
	Type    string // event type pattern (e.g. "tool.*", "session.*")
	Session string // session key filter
	Errors  bool   // only error events
}

// EventConn is a WebSocket connection for the event stream.
type EventConn struct {
	conn    *websocket.Conn
	msgs    chan EventMessage
	done    chan struct{}
	writeMu sync.Mutex
	closed  bool
	mu      sync.Mutex

	// Reconnection config.
	baseURL string
	wsURL   string
}

// DialEvents connects to the events WebSocket with optional filtering.
func DialEvents(baseURL string, filter ...EventFilter) (*EventConn, error) {
	wsURL := buildEventsWSURL(baseURL, filter...)
	conn, err := dialEventsWS(wsURL)
	if err != nil {
		return nil, fmt.Errorf("cannot connect to gateway events at %s — is the gateway running?", baseURL)
	}

	c := &EventConn{
		conn:    conn,
		msgs:    make(chan EventMessage, 64),
		done:    make(chan struct{}),
		baseURL: baseURL,
		wsURL:   wsURL,
	}
	go c.readLoop()
	return c, nil
}

func buildEventsWSURL(baseURL string, filter ...EventFilter) string {
	wsURL := httpToWS(baseURL) + "/api/v1/events/ws"
	if len(filter) > 0 {
		f := filter[0]
		params := url.Values{}
		if f.Type != "" {
			params.Set("type", f.Type)
		}
		if f.Session != "" {
			params.Set("session", f.Session)
		}
		if f.Errors {
			params.Set("status", "error")
		}
		if len(params) > 0 {
			wsURL += "?" + params.Encode()
		}
	}
	return wsURL
}

func dialEventsWS(wsURL string) (*websocket.Conn, error) {
	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}
	conn, _, err := dialer.Dial(wsURL, http.Header{})
	return conn, err
}

// Messages returns the channel that delivers incoming events.
func (c *EventConn) Messages() <-chan EventMessage {
	return c.msgs
}

// Close closes the event WebSocket connection.
func (c *EventConn) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return nil
	}
	c.closed = true
	close(c.done)
	return c.conn.Close()
}

func (c *EventConn) readLoop() {
	defer close(c.msgs)
	for {
		_, data, err := c.conn.ReadMessage()
		if err != nil {
			c.mu.Lock()
			closed := c.closed
			c.mu.Unlock()
			if closed {
				return
			}
			// Try to reconnect.
			if c.reconnect() {
				continue
			}
			return
		}
		var msg EventMessage
		if json.Unmarshal(data, &msg) != nil {
			continue
		}
		select {
		case c.msgs <- msg:
		case <-c.done:
			return
		}
	}
}

func (c *EventConn) reconnect() bool {
	backoff := time.Second
	maxBackoff := 30 * time.Second
	attempts := 5

	// Signal disconnection to the TUI.
	select {
	case c.msgs <- EventMessage{Type: "disconnected"}:
	default:
	}

	for i := range attempts {
		select {
		case <-c.done:
			return false
		case <-time.After(backoff):
		}

		conn, err := dialEventsWS(c.wsURL)
		if err == nil {
			c.writeMu.Lock()
			c.conn = conn
			c.writeMu.Unlock()
			// Signal reconnection.
			select {
			case c.msgs <- EventMessage{Type: "reconnected"}:
			default:
			}
			return true
		}

		if i < attempts-1 {
			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
		}
	}
	return false
}

func httpToWS(baseURL string) string {
	s := strings.Replace(baseURL, "https://", "wss://", 1)
	s = strings.Replace(s, "http://", "ws://", 1)
	return s
}
