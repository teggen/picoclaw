package list

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sipeed/picoclaw/clients/cli/internal/client"
)

func TestListChannels(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]client.ChannelInfo{
			{Name: "api", Enabled: true},
			{Name: "telegram", Enabled: true},
		})
	}))
	defer srv.Close()

	c := client.New(srv.URL, 0)
	err := listChannels(c, false)
	require.NoError(t, err)
}

func TestListChannelsJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]client.ChannelInfo{
			{Name: "api", Enabled: true},
		})
	}))
	defer srv.Close()

	c := client.New(srv.URL, 0)
	err := listChannels(c, true)
	require.NoError(t, err)
}

func TestListAgents(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]client.AgentInfo{
			{ID: "main", Name: "Main", Model: "claude-sonnet", MaxIterations: 10, MaxTokens: 4096},
		})
	}))
	defer srv.Close()

	c := client.New(srv.URL, 0)
	err := listAgents(c, false)
	require.NoError(t, err)
}

func TestListTools(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]client.ToolInfo{
			{Name: "shell", Description: "Execute shell commands"},
		})
	}))
	defer srv.Close()

	c := client.New(srv.URL, 0)
	err := listTools(c, false)
	require.NoError(t, err)
}

func TestRenderTable(t *testing.T) {
	headers := []string{"#", "NAME"}
	rows := [][]string{{"1", "test"}, {"2", "demo"}}
	result := renderTable(headers, rows)
	assert.Contains(t, result, "NAME")
	assert.Contains(t, result, "test")
	assert.Contains(t, result, "demo")
}
