package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/sipeed/picoclaw/pkg/bus"
	"github.com/sipeed/picoclaw/pkg/config"
)

func newTestChannel(t *testing.T) (*APIChannel, *bus.MessageBus) {
	t.Helper()
	msgBus := bus.NewMessageBus()
	ch, err := NewAPIChannel(config.APIConfig{
		Enabled:        true,
		MaxConnections: 10,
		PingInterval:   30,
		ReadTimeout:    60,
	}, msgBus)
	if err != nil {
		t.Fatal(err)
	}
	return ch, msgBus
}

func TestAPIChannel_Lifecycle(t *testing.T) {
	ch, _ := newTestChannel(t)

	if ch.IsRunning() {
		t.Fatal("channel should not be running before Start")
	}

	if err := ch.Start(context.Background()); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	if !ch.IsRunning() {
		t.Fatal("channel should be running after Start")
	}

	if err := ch.Stop(context.Background()); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}
	if ch.IsRunning() {
		t.Fatal("channel should not be running after Stop")
	}
}

func TestAPIChannel_Name(t *testing.T) {
	ch, _ := newTestChannel(t)
	if ch.Name() != "api" {
		t.Fatalf("expected name 'api', got %q", ch.Name())
	}
}

func TestAPIChannel_WebhookPath(t *testing.T) {
	ch, _ := newTestChannel(t)
	if ch.WebhookPath() != "/api/v1/" {
		t.Fatalf("expected path '/api/v1/', got %q", ch.WebhookPath())
	}
}

func TestAPIChannel_SendNotRunning(t *testing.T) {
	ch, _ := newTestChannel(t)
	err := ch.Send(context.Background(), bus.OutboundMessage{
		Channel: "api",
		ChatID:  "api:test-session",
		Content: "hello",
	})
	if err == nil {
		t.Fatal("expected error when sending to non-running channel")
	}
}

func TestAPIChannel_ServeHTTP_NotFound(t *testing.T) {
	ch, _ := newTestChannel(t)
	ch.Start(context.Background())
	defer ch.Stop(context.Background())

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/api/v1/unknown", nil)
	ch.ServeHTTP(w, r)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestAPIChannel_ChatSync_EmptyMessage(t *testing.T) {
	ch, _ := newTestChannel(t)
	ch.Start(context.Background())
	defer ch.Stop(context.Background())

	w := httptest.NewRecorder()
	body := strings.NewReader(`{"message":""}`)
	r := httptest.NewRequest("POST", "/api/v1/chat", body)
	r.Header.Set("Content-Type", "application/json")
	ch.ServeHTTP(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for empty message, got %d", w.Code)
	}
}

func TestAPIChannel_ChatSync_InvalidJSON(t *testing.T) {
	ch, _ := newTestChannel(t)
	ch.Start(context.Background())
	defer ch.Stop(context.Background())

	w := httptest.NewRecorder()
	body := strings.NewReader(`not json`)
	r := httptest.NewRequest("POST", "/api/v1/chat", body)
	r.Header.Set("Content-Type", "application/json")
	ch.ServeHTTP(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid JSON, got %d", w.Code)
	}
}

func TestAPIChannel_ChatSync_MethodNotAllowed(t *testing.T) {
	ch, _ := newTestChannel(t)
	ch.Start(context.Background())
	defer ch.Stop(context.Background())

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/api/v1/chat", nil)
	ch.ServeHTTP(w, r)
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}

func TestAPIChannel_CORS(t *testing.T) {
	ch, _ := newTestChannel(t)
	ch.config.AllowOrigins = []string{"http://example.com"}
	ch.Start(context.Background())
	defer ch.Stop(context.Background())

	w := httptest.NewRecorder()
	r := httptest.NewRequest("OPTIONS", "/api/v1/chat", nil)
	r.Header.Set("Origin", "http://example.com")
	ch.ServeHTTP(w, r)
	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204 for OPTIONS, got %d", w.Code)
	}
	if w.Header().Get("Access-Control-Allow-Origin") != "http://example.com" {
		t.Fatal("missing CORS header")
	}
}

func TestAPIChannel_SendToWebSocket(t *testing.T) {
	ch, _ := newTestChannel(t)
	ch.Start(context.Background())
	defer ch.Stop(context.Background())

	// Create a test server with the channel as handler.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.URL.Path = "/api/v1" + r.URL.Path
		ch.ServeHTTP(w, r)
	}))
	defer server.Close()

	// Connect WebSocket.
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/chat/ws?session_id=test123"
	ws, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("WebSocket dial failed: %v", err)
	}
	defer ws.Close()

	// Give connection time to register.
	time.Sleep(50 * time.Millisecond)

	// Send an outbound message through the channel.
	err = ch.Send(context.Background(), bus.OutboundMessage{
		Channel: "api",
		ChatID:  "api:test123",
		Content: "hello from agent",
	})
	if err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	// Read the message from WebSocket.
	ws.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, rawMsg, err := ws.ReadMessage()
	if err != nil {
		t.Fatalf("ReadMessage failed: %v", err)
	}

	var msg APIMessage
	if err := json.Unmarshal(rawMsg, &msg); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	if msg.Type != TypeMessageCreate {
		t.Fatalf("expected type %q, got %q", TypeMessageCreate, msg.Type)
	}
	content, _ := msg.Payload["content"].(string)
	if content != "hello from agent" {
		t.Fatalf("expected content 'hello from agent', got %q", content)
	}
}

func TestAPIChannel_WebSocket_Ping(t *testing.T) {
	ch, _ := newTestChannel(t)
	ch.Start(context.Background())
	defer ch.Stop(context.Background())

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.URL.Path = "/api/v1" + r.URL.Path
		ch.ServeHTTP(w, r)
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/chat/ws"
	ws, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("WebSocket dial failed: %v", err)
	}
	defer ws.Close()

	// Send a ping message.
	pingMsg := APIMessage{Type: TypePing, ID: "ping-1"}
	if err := ws.WriteJSON(pingMsg); err != nil {
		t.Fatalf("Write ping failed: %v", err)
	}

	// Read pong.
	ws.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, rawMsg, err := ws.ReadMessage()
	if err != nil {
		t.Fatalf("ReadMessage failed: %v", err)
	}

	var resp APIMessage
	json.Unmarshal(rawMsg, &resp)
	if resp.Type != TypePong {
		t.Fatalf("expected pong, got %q", resp.Type)
	}
	if resp.ID != "ping-1" {
		t.Fatalf("expected ID 'ping-1', got %q", resp.ID)
	}
}

func TestAPIChannel_MaxConnections(t *testing.T) {
	ch, _ := newTestChannel(t)
	ch.config.MaxConnections = 1
	ch.Start(context.Background())
	defer ch.Stop(context.Background())

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.URL.Path = "/api/v1" + r.URL.Path
		ch.ServeHTTP(w, r)
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/chat/ws"

	// First connection should succeed.
	ws1, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("First dial failed: %v", err)
	}
	defer ws1.Close()

	time.Sleep(50 * time.Millisecond)

	// Second connection should be rejected.
	_, resp, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err == nil {
		t.Fatal("expected second connection to be rejected")
	}
	if resp != nil && resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", resp.StatusCode)
	}
}
