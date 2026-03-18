package client

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var testUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

func TestDialChat(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/chat/ws", r.URL.Path)

		conn, err := testUpgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer conn.Close()

		// Send a test message.
		msg := APIMessage{
			Type:      TypeMessageCreate,
			SessionID: "test-session",
			Payload:   map[string]any{"content": "Hello!"},
		}
		conn.WriteJSON(msg)

		// Keep connection alive briefly.
		time.Sleep(100 * time.Millisecond)
	}))
	defer srv.Close()

	wsURL := strings.Replace(srv.URL, "http://", "http://", 1)
	conn, err := DialChat(wsURL, "test-session")
	require.NoError(t, err)
	defer conn.Close()

	// Should receive the test message.
	select {
	case msg := <-conn.Messages():
		assert.Equal(t, TypeMessageCreate, msg.Type)
		assert.Equal(t, "test-session", msg.SessionID)
		content, _ := msg.Payload["content"].(string)
		assert.Equal(t, "Hello!", content)
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for message")
	}
}

func TestChatConnSend(t *testing.T) {
	received := make(chan APIMessage, 1)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := testUpgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer conn.Close()

		// Read message from client.
		_, data, err := conn.ReadMessage()
		if err != nil {
			return
		}
		var msg APIMessage
		json.Unmarshal(data, &msg)
		received <- msg
	}))
	defer srv.Close()

	wsURL := strings.Replace(srv.URL, "http://", "http://", 1)
	conn, err := DialChat(wsURL, "")
	require.NoError(t, err)
	defer conn.Close()

	err = conn.Send("test message")
	require.NoError(t, err)

	select {
	case msg := <-received:
		assert.Equal(t, TypeMessageSend, msg.Type)
		content, _ := msg.Payload["content"].(string)
		assert.Equal(t, "test message", content)
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for sent message")
	}
}

func TestDialEvents(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/events/ws", r.URL.Path)

		conn, err := testUpgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer conn.Close()

		event := EventMessage{
			Type:      "config.updated",
			Timestamp: time.Now().UnixMilli(),
		}
		data, _ := json.Marshal(event)
		conn.WriteMessage(websocket.TextMessage, data)

		time.Sleep(100 * time.Millisecond)
	}))
	defer srv.Close()

	wsURL := strings.Replace(srv.URL, "http://", "http://", 1)
	conn, err := DialEvents(wsURL)
	require.NoError(t, err)
	defer conn.Close()

	select {
	case msg := <-conn.Messages():
		assert.Equal(t, "config.updated", msg.Type)
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for event")
	}
}

func TestHttpToWS(t *testing.T) {
	assert.Equal(t, "ws://localhost:8080", httpToWS("http://localhost:8080"))
	assert.Equal(t, "wss://example.com", httpToWS("https://example.com"))
}

func TestDialChatConnectionError(t *testing.T) {
	_, err := DialChat("http://localhost:1", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot connect")
}

func TestDialEventsConnectionError(t *testing.T) {
	_, err := DialEvents("http://localhost:1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot connect")
}
