package client

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveGatewayURL(t *testing.T) {
	t.Run("flag takes priority", func(t *testing.T) {
		assert.Equal(t, "http://custom:9090", ResolveGatewayURL("http://custom:9090"))
	})

	t.Run("env fallback", func(t *testing.T) {
		t.Setenv("PICOCLAW_GATEWAY_URL", "http://env:8080")
		assert.Equal(t, "http://env:8080", ResolveGatewayURL(""))
	})

	t.Run("default", func(t *testing.T) {
		assert.Equal(t, "http://localhost:8080", ResolveGatewayURL(""))
	})
}

func TestClientGetStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/status", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"status":   "running",
			"uptime":   "1h30m",
			"model":    "test-model",
			"channels": []string{"api", "telegram"},
			"tools":    3,
			"agents":   1,
			"skills":   "2/5",
		})
	}))
	defer srv.Close()

	c := New(srv.URL, 5*time.Second)
	status, err := c.GetStatus()
	require.NoError(t, err)
	assert.Equal(t, "running", status.Status)
	assert.Equal(t, "1h30m", status.Uptime)
	assert.Equal(t, "test-model", status.Model)
	assert.Equal(t, []string{"api", "telegram"}, status.Channels)
}

func TestClientListChannels(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/channels", r.URL.Path)
		json.NewEncoder(w).Encode([]ChannelInfo{
			{Name: "api", Enabled: true},
			{Name: "telegram", Enabled: true},
		})
	}))
	defer srv.Close()

	c := New(srv.URL, 5*time.Second)
	channels, err := c.ListChannels()
	require.NoError(t, err)
	assert.Len(t, channels, 2)
	assert.Equal(t, "api", channels[0].Name)
}

func TestClientListAgents(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]AgentInfo{
			{ID: "main", Name: "Main Agent", Model: "claude-sonnet", MaxIterations: 10, MaxTokens: 4096},
		})
	}))
	defer srv.Close()

	c := New(srv.URL, 5*time.Second)
	agents, err := c.ListAgents()
	require.NoError(t, err)
	assert.Len(t, agents, 1)
	assert.Equal(t, "main", agents[0].ID)
}

func TestClientListTools(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]ToolInfo{
			{Name: "shell", Description: "Execute shell commands"},
		})
	}))
	defer srv.Close()

	c := New(srv.URL, 5*time.Second)
	tools, err := c.ListTools()
	require.NoError(t, err)
	assert.Len(t, tools, 1)
	assert.Equal(t, "shell", tools[0].Name)
}

func TestClientListSessions(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]SessionListItem{
			{ID: "abc123", Title: "Test Session", MessageCount: 5, Updated: "2026-01-01T00:00:00Z"},
		})
	}))
	defer srv.Close()

	c := New(srv.URL, 5*time.Second)
	sessions, err := c.ListSessions()
	require.NoError(t, err)
	assert.Len(t, sessions, 1)
	assert.Equal(t, "abc123", sessions[0].ID)
}

func TestClientGetSession(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(SessionDetail{
			ID: "abc123",
			Messages: []SessionMessage{
				{Role: "user", Content: "Hello"},
				{Role: "assistant", Content: "Hi there!"},
			},
			Created: "2026-01-01T00:00:00Z",
			Updated: "2026-01-01T00:01:00Z",
		})
	}))
	defer srv.Close()

	c := New(srv.URL, 5*time.Second)
	sess, err := c.GetSession("abc123")
	require.NoError(t, err)
	assert.Equal(t, "abc123", sess.ID)
	assert.Len(t, sess.Messages, 2)
}

func TestClientDeleteSession(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodDelete, r.Method)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	c := New(srv.URL, 5*time.Second)
	err := c.DeleteSession("abc123")
	assert.NoError(t, err)
}

func TestClientGetConfig(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"key":"value"}`))
	}))
	defer srv.Close()

	c := New(srv.URL, 5*time.Second)
	raw, err := c.GetConfig()
	require.NoError(t, err)
	assert.JSONEq(t, `{"key":"value"}`, string(raw))
}

func TestClientPutConfig(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPut, r.Method)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := New(srv.URL, 5*time.Second)
	err := c.PutConfig(json.RawMessage(`{"key":"value"}`))
	assert.NoError(t, err)
}

func TestClientPatchConfig(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPatch, r.Method)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := New(srv.URL, 5*time.Second)
	err := c.PatchConfig(json.RawMessage(`{"key":"patched"}`))
	assert.NoError(t, err)
}

func TestClientHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"error":"not found"}`))
	}))
	defer srv.Close()

	c := New(srv.URL, 5*time.Second)
	_, err := c.GetStatus()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestClientConnectionError(t *testing.T) {
	c := New("http://localhost:1", 1*time.Second)
	_, err := c.GetStatus()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot connect to gateway")
}
