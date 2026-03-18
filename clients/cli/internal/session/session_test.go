package session

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sipeed/picoclaw/clients/cli/internal/client"
)

func TestRunListSessions(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]client.SessionListItem{
			{ID: "abc123", Title: "Test", MessageCount: 3, Updated: "2026-01-01T00:00:00Z"},
		})
	}))
	defer srv.Close()

	c := client.New(srv.URL, 0)
	sessions, err := c.ListSessions()
	require.NoError(t, err)
	assert.Len(t, sessions, 1)
	assert.Equal(t, "abc123", sessions[0].ID)
}

func TestRunShowSession(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(client.SessionDetail{
			ID: "abc123",
			Messages: []client.SessionMessage{
				{Role: "user", Content: "Hello"},
				{Role: "assistant", Content: "Hi!"},
			},
			Created: "2026-01-01T00:00:00Z",
			Updated: "2026-01-01T00:01:00Z",
		})
	}))
	defer srv.Close()

	c := client.New(srv.URL, 0)
	sess, err := c.GetSession("abc123")
	require.NoError(t, err)
	assert.Equal(t, "abc123", sess.ID)
	assert.Len(t, sess.Messages, 2)
}

func TestRunDeleteSession(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodDelete, r.Method)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	c := client.New(srv.URL, 0)
	err := c.DeleteSession("abc123")
	assert.NoError(t, err)
}
