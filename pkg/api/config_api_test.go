package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestHandleGetConfig(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")

	cfg := map[string]any{
		"version": 1,
		"gateway": map[string]any{"host": "0.0.0.0", "port": 8080},
	}
	data, _ := json.Marshal(cfg)
	os.WriteFile(configPath, data, 0o644)

	h := &Handler{
		configPath: configPath,
		eventHub:   NewEventHub(),
		startTime:  time.Now(),
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/api/v1/config", nil)
	h.withJSON(h.handleGetConfig)(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var result map[string]any
	json.Unmarshal(w.Body.Bytes(), &result)
	if _, ok := result["gateway"]; !ok {
		t.Fatal("expected gateway in config response")
	}
}

func TestHandlePutConfig(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")

	// Write initial config.
	initial := `{"version":1,"gateway":{"host":"0.0.0.0","port":8080}}`
	os.WriteFile(configPath, []byte(initial), 0o644)

	h := &Handler{
		configPath: configPath,
		eventHub:   NewEventHub(),
		startTime:  time.Now(),
	}

	// PUT new config.
	newCfg := `{"version":1,"gateway":{"host":"0.0.0.0","port":9090}}`
	w := httptest.NewRecorder()
	r := httptest.NewRequest("PUT", "/api/v1/config", strings.NewReader(newCfg))
	r.Header.Set("Content-Type", "application/json")
	h.handlePutConfig(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Verify file was written.
	data, _ := os.ReadFile(configPath)
	if !strings.Contains(string(data), "9090") {
		t.Fatal("config file was not updated with new port")
	}
}

func TestHandlePutConfig_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	os.WriteFile(configPath, []byte(`{}`), 0o644)

	h := &Handler{
		configPath: configPath,
		eventHub:   NewEventHub(),
		startTime:  time.Now(),
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest("PUT", "/api/v1/config", strings.NewReader("not json"))
	h.handlePutConfig(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid JSON, got %d", w.Code)
	}
}

func TestHandlePatchConfig(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")

	initial := `{"version":1,"gateway":{"host":"0.0.0.0","port":8080},"channels":{}}`
	os.WriteFile(configPath, []byte(initial), 0o644)

	h := &Handler{
		configPath: configPath,
		eventHub:   NewEventHub(),
		startTime:  time.Now(),
	}

	patch := `{"gateway":{"port":9090}}`
	w := httptest.NewRecorder()
	r := httptest.NewRequest("PATCH", "/api/v1/config", strings.NewReader(patch))
	r.Header.Set("Content-Type", "application/json")
	h.handlePatchConfig(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Verify the merged config.
	data, _ := os.ReadFile(configPath)
	var result map[string]any
	json.Unmarshal(data, &result)

	gw, _ := result["gateway"].(map[string]any)
	if gw["host"] != "0.0.0.0" {
		t.Fatal("host should be preserved after patch")
	}
	if gw["port"] != float64(9090) {
		t.Fatal("port should be updated to 9090")
	}
}

func TestMergeMaps(t *testing.T) {
	base := map[string]any{
		"a": "1",
		"b": map[string]any{"c": "2", "d": "3"},
		"e": "4",
	}
	patch := map[string]any{
		"a": "updated",
		"b": map[string]any{"c": "updated"},
		"e": nil, // delete
	}
	result := mergeMaps(base, patch)

	if result["a"] != "updated" {
		t.Fatal("a should be updated")
	}
	b := result["b"].(map[string]any)
	if b["c"] != "updated" {
		t.Fatal("b.c should be updated")
	}
	if b["d"] != "3" {
		t.Fatal("b.d should be preserved")
	}
	if _, ok := result["e"]; ok {
		t.Fatal("e should be deleted")
	}
}
