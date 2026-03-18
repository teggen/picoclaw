package chat

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sipeed/picoclaw/clients/cli/internal/client"
)

var testUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

func TestNewChatModel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/status":
			w.Write([]byte(`{"status":"running"}`))
		case "/api/v1/chat/ws":
			conn, err := testUpgrader.Upgrade(w, r, nil)
			if err != nil {
				return
			}
			defer conn.Close()
			time.Sleep(200 * time.Millisecond)
		}
	}))
	defer srv.Close()

	c := client.New(srv.URL, 5*time.Second)
	conn, err := client.DialChat(srv.URL, "test-session")
	require.NoError(t, err)
	defer conn.Close()

	model := newChatModel(conn, c, "test-session")
	assert.Equal(t, "test-session", model.sessionID)
	assert.Empty(t, model.messages)
	assert.False(t, model.typing)
}

func TestHandleCommand(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := testUpgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		time.Sleep(500 * time.Millisecond)
	}))
	defer srv.Close()

	c := client.New(srv.URL, 5*time.Second)
	conn, err := client.DialChat(srv.URL, "")
	require.NoError(t, err)
	defer conn.Close()

	model := newChatModel(conn, c, "test-session")

	// Test /session command.
	result, _ := model.handleCommand("/session")
	m := result.(chatModel)
	require.Len(t, m.messages, 1)
	assert.Contains(t, m.messages[0].Content, "Session ID: test-session")

	// Test /help command.
	result, _ = m.handleCommand("/help")
	m = result.(chatModel)
	assert.Len(t, m.messages, 2)
	assert.Contains(t, m.messages[1].Content, "Commands:")

	// Test /clear command.
	result, _ = m.handleCommand("/clear")
	m = result.(chatModel)
	assert.Empty(t, m.messages)

	// Test unknown command.
	result, _ = m.handleCommand("/unknown")
	m = result.(chatModel)
	assert.Len(t, m.messages, 1)
	assert.Contains(t, m.messages[0].Content, "Unknown command")
}

func TestMessageHistory(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := testUpgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		time.Sleep(500 * time.Millisecond)
	}))
	defer srv.Close()

	c := client.New(srv.URL, 5*time.Second)
	conn, err := client.DialChat(srv.URL, "")
	require.NoError(t, err)
	defer conn.Close()

	m := newChatModel(conn, c, "test-session")

	upKey := tea.KeyPressMsg{Code: tea.KeyUp}
	downKey := tea.KeyPressMsg{Code: tea.KeyDown}
	enterKey := tea.KeyPressMsg{Code: tea.KeyEnter}

	// Up with no history should be a no-op.
	result, _ := m.Update(upKey)
	m = result.(chatModel)
	assert.Equal(t, "", m.textarea.Value())

	// Send three messages via Enter key.
	msgs := []string{"hello", "world", "foo"}
	for _, msg := range msgs {
		m.textarea.SetValue(msg)
		result, _ = m.Update(enterKey)
		m = result.(chatModel)
	}
	assert.Equal(t, msgs, m.sentHistory)
	assert.Equal(t, 3, m.historyIdx)

	// Type something in progress, then navigate up.
	m.textarea.SetValue("in-progress")

	// Up → should show "foo" (last sent).
	result, _ = m.Update(upKey)
	m = result.(chatModel)
	assert.Equal(t, "foo", m.textarea.Value())
	assert.Equal(t, "in-progress", m.savedInput)

	// Up → should show "world".
	result, _ = m.Update(upKey)
	m = result.(chatModel)
	assert.Equal(t, "world", m.textarea.Value())

	// Up → should show "hello".
	result, _ = m.Update(upKey)
	m = result.(chatModel)
	assert.Equal(t, "hello", m.textarea.Value())

	// Up again → should stay at "hello" (boundary).
	result, _ = m.Update(upKey)
	m = result.(chatModel)
	assert.Equal(t, "hello", m.textarea.Value())
	assert.Equal(t, 0, m.historyIdx)

	// Down → should show "world".
	result, _ = m.Update(downKey)
	m = result.(chatModel)
	assert.Equal(t, "world", m.textarea.Value())

	// Down → should show "foo".
	result, _ = m.Update(downKey)
	m = result.(chatModel)
	assert.Equal(t, "foo", m.textarea.Value())

	// Down → should restore saved input.
	result, _ = m.Update(downKey)
	m = result.(chatModel)
	assert.Equal(t, "in-progress", m.textarea.Value())
	assert.Equal(t, 3, m.historyIdx)

	// Down again → should be a no-op, stays at saved input.
	result, _ = m.Update(downKey)
	m = result.(chatModel)
	assert.Equal(t, "in-progress", m.textarea.Value())
}

func TestWsMessageMsg(t *testing.T) {
	msg := wsMessageMsg(client.APIMessage{
		Type:    client.TypeMessageCreate,
		Payload: map[string]any{"content": "Hello!"},
	})
	apiMsg := client.APIMessage(msg)
	assert.Equal(t, client.TypeMessageCreate, apiMsg.Type)

	content, ok := apiMsg.Payload["content"].(string)
	assert.True(t, ok)
	assert.Equal(t, "Hello!", content)
}

func TestWaitForMessage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := testUpgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()

		msg := client.APIMessage{
			Type:    client.TypeMessageCreate,
			Payload: map[string]any{"content": "response"},
		}
		data, _ := json.Marshal(msg)
		conn.WriteMessage(websocket.TextMessage, data)
		time.Sleep(100 * time.Millisecond)
	}))
	defer srv.Close()

	chatConn, err := client.DialChat(srv.URL, "")
	require.NoError(t, err)
	defer chatConn.Close()

	cmd := waitForMessage(chatConn)
	result := cmd()
	wsMsg, ok := result.(wsMessageMsg)
	assert.True(t, ok)
	assert.Equal(t, client.TypeMessageCreate, wsMsg.Type)
}
