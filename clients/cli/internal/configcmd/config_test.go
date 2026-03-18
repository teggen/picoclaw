package configcmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sipeed/picoclaw/clients/cli/internal/client"
)

func TestGetConfig(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/config", r.URL.Path)
		w.Write([]byte(`{"agents":{"defaults":{"model":"claude-sonnet"}}}`))
	}))
	defer srv.Close()

	c := client.New(srv.URL, 0)
	raw, err := c.GetConfig()
	require.NoError(t, err)
	assert.True(t, json.Valid(raw))
}

func TestPutConfig(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPut, r.Method)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := client.New(srv.URL, 0)
	err := c.PutConfig(json.RawMessage(`{"key":"value"}`))
	assert.NoError(t, err)
}

func TestPatchConfig(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPatch, r.Method)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := client.New(srv.URL, 0)
	err := c.PatchConfig(json.RawMessage(`{"key":"patched"}`))
	assert.NoError(t, err)
}

func TestColorizeJSON(t *testing.T) {
	input := `{"key": "value", "num": 42, "bool": true, "null": null}`
	result := colorizeJSON(input)
	// Should contain the original values (wrapped in ANSI codes).
	assert.Contains(t, result, "key")
	assert.Contains(t, result, "value")
	assert.Contains(t, result, "42")
	assert.Contains(t, result, "true")
	assert.Contains(t, result, "null")
}
