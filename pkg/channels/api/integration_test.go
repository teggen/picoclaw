package api

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/sipeed/picoclaw/pkg/agent"
	"github.com/sipeed/picoclaw/pkg/bus"
	"github.com/sipeed/picoclaw/pkg/config"
	"github.com/sipeed/picoclaw/pkg/providers"
)

// mockLLMProvider returns a fixed response for every Chat call.
type mockLLMProvider struct {
	response string
}

func (m *mockLLMProvider) Chat(
	_ context.Context,
	_ []providers.Message,
	_ []providers.ToolDefinition,
	_ string,
	_ map[string]any,
) (*providers.LLMResponse, error) {
	return &providers.LLMResponse{
		Content:   m.response,
		ToolCalls: []providers.ToolCall{},
	}, nil
}

func (m *mockLLMProvider) GetDefaultModel() string { return "mock" }

// testHarness wires up bus + agent loop + API channel + httptest server.
type testHarness struct {
	bus       *bus.MessageBus
	agentLoop *agent.AgentLoop
	channel   *APIChannel
	server    *httptest.Server
	cancel    context.CancelFunc
	workspace string
}

func newTestHarness(t *testing.T, response string) *testHarness {
	t.Helper()

	tmpDir := t.TempDir()

	msgBus := bus.NewMessageBus()
	provider := &mockLLMProvider{response: response}

	cfg := &config.Config{
		Agents: config.AgentsConfig{
			Defaults: config.AgentDefaults{
				Workspace:         tmpDir,
				Model:             "mock",
				MaxTokens:         4096,
				MaxToolIterations: 1,
			},
		},
		Channels: config.ChannelsConfig{
			API: config.APIConfig{
				Enabled:        true,
				MaxConnections: 100,
				PingInterval:   30,
				ReadTimeout:    60,
			},
		},
	}

	al := agent.NewAgentLoop(cfg, msgBus, provider)

	ch, err := NewAPIChannel(cfg.Channels.API, msgBus)
	if err != nil {
		t.Fatalf("NewAPIChannel: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	// Start the API channel.
	if err := ch.Start(ctx); err != nil {
		cancel()
		t.Fatalf("channel Start: %v", err)
	}

	// Run the agent loop in a goroutine — it consumes from the bus.
	go al.Run(ctx)

	// Create httptest server using the channel's ServeHTTP.
	mux := http.NewServeMux()
	mux.Handle(ch.WebhookPath(), ch)
	server := httptest.NewServer(mux)

	// Start a goroutine that dispatches outbound messages from bus to channel.
	go func() {
		for {
			msg, ok := msgBus.SubscribeOutbound(ctx)
			if !ok {
				return
			}
			ch.Send(ctx, msg)
		}
	}()

	return &testHarness{
		bus:       msgBus,
		agentLoop: al,
		channel:   ch,
		server:    server,
		cancel:    cancel,
		workspace: tmpDir,
	}
}

func (h *testHarness) close() {
	h.cancel()
	h.server.Close()
	h.agentLoop.Stop()
	h.channel.Stop(context.Background())
	h.bus.Close()
}

func (h *testHarness) wsURL(sessionID string) string {
	base := "ws" + strings.TrimPrefix(h.server.URL, "http")
	url := base + "/api/v1/chat/ws"
	if sessionID != "" {
		url += "?session_id=" + sessionID
	}
	return url
}

// dialWS connects a WebSocket client to the test server.
func (h *testHarness) dialWS(t *testing.T, sessionID string) *websocket.Conn {
	t.Helper()
	ws, _, err := websocket.DefaultDialer.Dial(h.wsURL(sessionID), nil)
	if err != nil {
		t.Fatalf("WebSocket dial failed: %v", err)
	}
	// Give the connection time to register in the channel.
	time.Sleep(50 * time.Millisecond)
	return ws
}

// readWSMessage reads the next JSON message from a WebSocket with a timeout.
func readWSMessage(t *testing.T, ws *websocket.Conn, timeout time.Duration) APIMessage {
	t.Helper()
	ws.SetReadDeadline(time.Now().Add(timeout))
	_, raw, err := ws.ReadMessage()
	if err != nil {
		t.Fatalf("WebSocket ReadMessage: %v", err)
	}
	var msg APIMessage
	if err := json.Unmarshal(raw, &msg); err != nil {
		t.Fatalf("Unmarshal APIMessage: %v", err)
	}
	return msg
}

func TestIntegration_WebSocket_ChatRoundTrip(t *testing.T) {
	const expected = "Hello from mock LLM!"
	h := newTestHarness(t, expected)
	defer h.close()

	ws := h.dialWS(t, "ws-roundtrip")
	defer ws.Close()

	// Send a message via WebSocket.
	sendMsg := APIMessage{
		Type:      TypeMessageSend,
		SessionID: "ws-roundtrip",
		Payload:   map[string]any{"content": "Hi there"},
	}
	if err := ws.WriteJSON(sendMsg); err != nil {
		t.Fatalf("WriteJSON: %v", err)
	}

	// Read the response — should be a message.create with the mock response.
	msg := readWSMessage(t, ws, 10*time.Second)
	if msg.Type != TypeMessageCreate {
		t.Fatalf("expected type %q, got %q", TypeMessageCreate, msg.Type)
	}
	content, _ := msg.Payload["content"].(string)
	if content != expected {
		t.Fatalf("expected content %q, got %q", expected, content)
	}
}

func TestIntegration_WebSocket_SessionPersistence(t *testing.T) {
	const expected = "Mock reply"
	h := newTestHarness(t, expected)
	defer h.close()

	ws := h.dialWS(t, "ws-persist")
	defer ws.Close()

	// Send first message.
	msg1 := APIMessage{
		Type:      TypeMessageSend,
		SessionID: "ws-persist",
		Payload:   map[string]any{"content": "First message"},
	}
	if err := ws.WriteJSON(msg1); err != nil {
		t.Fatalf("WriteJSON msg1: %v", err)
	}
	resp1 := readWSMessage(t, ws, 10*time.Second)
	if resp1.Type != TypeMessageCreate {
		t.Fatalf("msg1: expected %q, got %q", TypeMessageCreate, resp1.Type)
	}

	// Send second message on the same session.
	msg2 := APIMessage{
		Type:      TypeMessageSend,
		SessionID: "ws-persist",
		Payload:   map[string]any{"content": "Second message"},
	}
	if err := ws.WriteJSON(msg2); err != nil {
		t.Fatalf("WriteJSON msg2: %v", err)
	}
	resp2 := readWSMessage(t, ws, 10*time.Second)
	if resp2.Type != TypeMessageCreate {
		t.Fatalf("msg2: expected %q, got %q", TypeMessageCreate, resp2.Type)
	}

	content, _ := resp2.Payload["content"].(string)
	if content != expected {
		t.Fatalf("msg2: expected content %q, got %q", expected, content)
	}
}

func TestIntegration_REST_ChatSync(t *testing.T) {
	const expected = "Sync response from mock"
	h := newTestHarness(t, expected)
	defer h.close()

	body := strings.NewReader(`{"message":"Hello sync","session_id":"rest-sync-1"}`)
	resp, err := http.Post(h.server.URL+"/api/v1/chat", "application/json", body)
	if err != nil {
		t.Fatalf("POST /chat: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("Decode response: %v", err)
	}

	if result["response"] != expected {
		t.Fatalf("expected response %q, got %q", expected, result["response"])
	}
	if result["session_id"] != "rest-sync-1" {
		t.Fatalf("expected session_id %q, got %q", "rest-sync-1", result["session_id"])
	}
}

func TestIntegration_REST_ChatStream_SSE(t *testing.T) {
	const expected = "Streamed mock response"
	h := newTestHarness(t, expected)
	defer h.close()

	body := strings.NewReader(`{"message":"Hello stream","session_id":"rest-stream-1"}`)
	resp, err := http.Post(h.server.URL+"/api/v1/chat/stream", "application/json", body)
	if err != nil {
		t.Fatalf("POST /chat/stream: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Fatalf("expected Content-Type text/event-stream, got %q", ct)
	}

	// Read SSE events.
	scanner := bufio.NewScanner(resp.Body)
	var foundDone bool
	deadline := time.After(10 * time.Second)

	for {
		select {
		case <-deadline:
			t.Fatal("timed out waiting for SSE done event")
		default:
		}

		if !scanner.Scan() {
			break
		}
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		data := strings.TrimPrefix(line, "data: ")
		var event map[string]any
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			t.Fatalf("Unmarshal SSE event: %v", err)
		}

		if event["type"] == "done" {
			foundDone = true
			if event["content"] != expected {
				t.Fatalf("expected content %q, got %q", expected, event["content"])
			}
			if event["session_id"] != "rest-stream-1" {
				t.Fatalf("expected session_id %q, got %q", "rest-stream-1", event["session_id"])
			}
			break
		}
	}

	if !foundDone {
		t.Fatal("never received SSE done event")
	}
}

func TestIntegration_WebSocket_MultipleClients(t *testing.T) {
	const expected = "Multi-client response"
	h := newTestHarness(t, expected)
	defer h.close()

	ws1 := h.dialWS(t, "multi-1")
	defer ws1.Close()

	ws2 := h.dialWS(t, "multi-2")
	defer ws2.Close()

	// Send message from client 1.
	send1 := APIMessage{
		Type:      TypeMessageSend,
		SessionID: "multi-1",
		Payload:   map[string]any{"content": "From client 1"},
	}
	if err := ws1.WriteJSON(send1); err != nil {
		t.Fatalf("ws1 WriteJSON: %v", err)
	}

	// Send message from client 2.
	send2 := APIMessage{
		Type:      TypeMessageSend,
		SessionID: "multi-2",
		Payload:   map[string]any{"content": "From client 2"},
	}
	if err := ws2.WriteJSON(send2); err != nil {
		t.Fatalf("ws2 WriteJSON: %v", err)
	}

	// Both clients should receive their own responses.
	resp1 := readWSMessage(t, ws1, 10*time.Second)
	if resp1.Type != TypeMessageCreate {
		t.Fatalf("ws1: expected %q, got %q", TypeMessageCreate, resp1.Type)
	}
	content1, _ := resp1.Payload["content"].(string)
	if content1 != expected {
		t.Fatalf("ws1: expected content %q, got %q", expected, content1)
	}

	resp2 := readWSMessage(t, ws2, 10*time.Second)
	if resp2.Type != TypeMessageCreate {
		t.Fatalf("ws2: expected %q, got %q", TypeMessageCreate, resp2.Type)
	}
	content2, _ := resp2.Payload["content"].(string)
	if content2 != expected {
		t.Fatalf("ws2: expected content %q, got %q", expected, content2)
	}
}
