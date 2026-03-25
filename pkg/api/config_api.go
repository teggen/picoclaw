package api

import (
	"encoding/json"
	"io"
	"net/http"
	"os"

	"github.com/sipeed/picoclaw/pkg/config"
	"github.com/sipeed/picoclaw/pkg/events"
	"github.com/sipeed/picoclaw/pkg/logger"
)

// handleGetConfig returns the current configuration.
// GET /api/v1/config
func (h *Handler) handleGetConfig(w http.ResponseWriter, r *http.Request) {
	cfg, err := config.LoadConfig(h.configPath)
	if err != nil {
		http.Error(w, `{"error":"failed to load config"}`, http.StatusInternalServerError)
		return
	}
	writeJSON(w, cfg)
}

// handlePutConfig replaces the entire configuration.
// PUT /api/v1/config
func (h *Handler) handlePutConfig(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		http.Error(w, `{"error":"failed to read request body"}`, http.StatusBadRequest)
		return
	}

	// Validate it's valid JSON and a valid config.
	var cfg config.Config
	if err := json.Unmarshal(body, &cfg); err != nil {
		http.Error(w, `{"error":"invalid JSON"}`, http.StatusBadRequest)
		return
	}

	// Write to disk — the config watcher will detect the change and trigger a hot reload.
	if err := os.WriteFile(h.configPath, body, 0o644); err != nil {
		http.Error(w, `{"error":"failed to write config"}`, http.StatusInternalServerError)
		return
	}

	logger.InfoC("api", "Config replaced via PUT /api/v1/config")
	h.eventHub.Broadcast(events.ConfigUpdated, nil)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	writeJSON(w, map[string]string{"status": "ok"})
}

// handlePatchConfig partially updates the configuration using JSON merge patch.
// PATCH /api/v1/config
func (h *Handler) handlePatchConfig(w http.ResponseWriter, r *http.Request) {
	// Read current config.
	currentData, err := os.ReadFile(h.configPath)
	if err != nil {
		http.Error(w, `{"error":"failed to read current config"}`, http.StatusInternalServerError)
		return
	}

	// Parse current config as a generic map.
	var current map[string]any
	if err := json.Unmarshal(currentData, &current); err != nil {
		http.Error(w, `{"error":"failed to parse current config"}`, http.StatusInternalServerError)
		return
	}

	// Read patch.
	patchBody, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		http.Error(w, `{"error":"failed to read request body"}`, http.StatusBadRequest)
		return
	}

	var patch map[string]any
	if err := json.Unmarshal(patchBody, &patch); err != nil {
		http.Error(w, `{"error":"invalid JSON"}`, http.StatusBadRequest)
		return
	}

	// Apply merge patch.
	merged := mergeMaps(current, patch)

	// Validate by unmarshaling into Config.
	mergedData, err := json.MarshalIndent(merged, "", "  ")
	if err != nil {
		http.Error(w, `{"error":"failed to marshal merged config"}`, http.StatusInternalServerError)
		return
	}

	var cfg config.Config
	if err := json.Unmarshal(mergedData, &cfg); err != nil {
		http.Error(w, `{"error":"merged config is invalid"}`, http.StatusBadRequest)
		return
	}

	// Write to disk.
	if err := os.WriteFile(h.configPath, mergedData, 0o644); err != nil {
		http.Error(w, `{"error":"failed to write config"}`, http.StatusInternalServerError)
		return
	}

	logger.InfoC("api", "Config patched via PATCH /api/v1/config")
	h.eventHub.Broadcast(events.ConfigUpdated, nil)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	writeJSON(w, map[string]string{"status": "ok"})
}

// mergeMaps performs a recursive JSON merge patch (RFC 7396).
func mergeMaps(base, patch map[string]any) map[string]any {
	result := make(map[string]any, len(base))
	for k, v := range base {
		result[k] = v
	}
	for k, v := range patch {
		if v == nil {
			delete(result, k)
			continue
		}
		if patchMap, ok := v.(map[string]any); ok {
			if baseMap, ok := result[k].(map[string]any); ok {
				result[k] = mergeMaps(baseMap, patchMap)
				continue
			}
		}
		result[k] = v
	}
	return result
}
