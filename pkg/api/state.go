package api

import (
	"bufio"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/sipeed/picoclaw/pkg/config"
	"github.com/sipeed/picoclaw/pkg/providers"
)

// handleStatus returns gateway status information.
// GET /api/v1/status
func (h *Handler) handleStatus(w http.ResponseWriter, r *http.Request) {
	info := h.agentLoop.GetStartupInfo()
	cfg := h.agentLoop.GetConfig()

	modelName := cfg.Agents.Defaults.ModelName
	if modelName == "" {
		modelName = cfg.Agents.Defaults.Model
	}

	json.NewEncoder(w).Encode(map[string]any{
		"status":   "running",
		"uptime":   time.Since(h.startTime).String(),
		"model":    modelName,
		"channels": h.chanMgr.GetEnabledChannels(),
		"tools":    info["tools"],
		"agents":   info["agents"],
		"skills":   info["skills"],
	})
}

// handleChannels returns enabled channels and their state.
// GET /api/v1/channels
func (h *Handler) handleChannels(w http.ResponseWriter, r *http.Request) {
	names := h.chanMgr.GetEnabledChannels()
	result := make([]map[string]any, 0, len(names))
	for _, name := range names {
		result = append(result, map[string]any{
			"name":    name,
			"enabled": true,
		})
	}
	json.NewEncoder(w).Encode(result)
}

// handleAgents returns registered agents.
// GET /api/v1/agents
func (h *Handler) handleAgents(w http.ResponseWriter, r *http.Request) {
	registry := h.agentLoop.GetRegistry()
	ids := registry.ListAgentIDs()

	agents := make([]map[string]any, 0, len(ids))
	for _, id := range ids {
		a, ok := registry.GetAgent(id)
		if !ok {
			continue
		}
		agents = append(agents, map[string]any{
			"id":             a.ID,
			"name":           a.Name,
			"model":          a.Model,
			"max_iterations": a.MaxIterations,
			"max_tokens":     a.MaxTokens,
		})
	}
	json.NewEncoder(w).Encode(agents)
}

// handleAgentByID returns a specific agent.
// GET /api/v1/agents/{id}
func (h *Handler) handleAgentByID(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		http.Error(w, `{"error":"missing agent id"}`, http.StatusBadRequest)
		return
	}

	registry := h.agentLoop.GetRegistry()
	a, ok := registry.GetAgent(id)
	if !ok {
		http.Error(w, `{"error":"agent not found"}`, http.StatusNotFound)
		return
	}

	toolNames := a.Tools.List()
	json.NewEncoder(w).Encode(map[string]any{
		"id":             a.ID,
		"name":           a.Name,
		"model":          a.Model,
		"max_iterations": a.MaxIterations,
		"max_tokens":     a.MaxTokens,
		"temperature":    a.Temperature,
		"tools":          toolNames,
	})
}

// handleTools returns the live tool registry.
// GET /api/v1/tools
func (h *Handler) handleTools(w http.ResponseWriter, r *http.Request) {
	registry := h.agentLoop.GetRegistry()
	agent := registry.GetDefaultAgent()
	if agent == nil {
		json.NewEncoder(w).Encode([]any{})
		return
	}

	defs := agent.Tools.ToProviderDefs()
	result := make([]map[string]any, 0, len(defs))
	for _, def := range defs {
		result = append(result, map[string]any{
			"name":        def.Function.Name,
			"description": def.Function.Description,
			"parameters":  def.Function.Parameters,
		})
	}
	json.NewEncoder(w).Encode(result)
}

// Session types and helpers, mirroring web/backend/api/session.go patterns.

const (
	apiSessionPrefix          = "agent:main:api:direct:api:"
	sanitizedAPISessionPrefix = "agent_main_api_direct_api_"
	maxSessionJSONLLineSize   = 10 * 1024 * 1024
	maxSessionTitleRunes      = 60
)

type sessionFile struct {
	Key      string              `json:"key"`
	Messages []providers.Message `json:"messages"`
	Summary  string              `json:"summary,omitempty"`
	Created  time.Time           `json:"created"`
	Updated  time.Time           `json:"updated"`
}

type sessionMetaFile struct {
	Key       string    `json:"key"`
	Summary   string    `json:"summary"`
	Skip      int       `json:"skip"`
	Count     int       `json:"count"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type sessionListItem struct {
	ID           string `json:"id"`
	Title        string `json:"title"`
	Preview      string `json:"preview"`
	MessageCount int    `json:"message_count"`
	Created      string `json:"created"`
	Updated      string `json:"updated"`
}

func (h *Handler) sessionsDir() (string, error) {
	cfg, err := config.LoadConfig(h.configPath)
	if err != nil {
		return "", err
	}

	workspace := cfg.Agents.Defaults.Workspace
	if workspace == "" {
		home, _ := os.UserHomeDir()
		workspace = filepath.Join(home, ".picoclaw", "workspace")
	}

	if len(workspace) > 0 && workspace[0] == '~' {
		home, _ := os.UserHomeDir()
		if len(workspace) > 1 && workspace[1] == '/' {
			workspace = home + workspace[1:]
		} else {
			workspace = home
		}
	}

	return filepath.Join(workspace, "sessions"), nil
}

func sanitizeSessionKey(key string) string {
	return strings.ReplaceAll(key, ":", "_")
}

func extractAPISessionID(key string) (string, bool) {
	if strings.HasPrefix(key, apiSessionPrefix) {
		return strings.TrimPrefix(key, apiSessionPrefix), true
	}
	return "", false
}

func extractAPISessionIDFromSanitizedKey(key string) (string, bool) {
	if strings.HasPrefix(key, sanitizedAPISessionPrefix) {
		return strings.TrimPrefix(key, sanitizedAPISessionPrefix), true
	}
	return "", false
}

func (h *Handler) readSessionMeta(path, sessionKey string) (sessionMetaFile, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return sessionMetaFile{Key: sessionKey}, nil
	}
	if err != nil {
		return sessionMetaFile{}, err
	}
	var meta sessionMetaFile
	if err := json.Unmarshal(data, &meta); err != nil {
		return sessionMetaFile{}, err
	}
	if meta.Key == "" {
		meta.Key = sessionKey
	}
	return meta, nil
}

func (h *Handler) readSessionMessages(path string, skip int) ([]providers.Message, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	msgs := make([]providers.Message, 0)
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), maxSessionJSONLLineSize)

	seen := 0
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		seen++
		if seen <= skip {
			continue
		}
		var msg providers.Message
		if err := json.Unmarshal(line, &msg); err != nil {
			continue
		}
		msgs = append(msgs, msg)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return msgs, nil
}

func (h *Handler) readJSONLSession(dir, sessionID string) (sessionFile, error) {
	sessionKey := apiSessionPrefix + sessionID
	base := filepath.Join(dir, sanitizeSessionKey(sessionKey))
	jsonlPath := base + ".jsonl"
	metaPath := base + ".meta.json"

	meta, err := h.readSessionMeta(metaPath, sessionKey)
	if err != nil {
		return sessionFile{}, err
	}

	messages, err := h.readSessionMessages(jsonlPath, meta.Skip)
	if err != nil {
		return sessionFile{}, err
	}

	updated := meta.UpdatedAt
	created := meta.CreatedAt
	if created.IsZero() || updated.IsZero() {
		if info, statErr := os.Stat(jsonlPath); statErr == nil {
			if created.IsZero() {
				created = info.ModTime()
			}
			if updated.IsZero() {
				updated = info.ModTime()
			}
		}
	}

	return sessionFile{
		Key:      meta.Key,
		Messages: messages,
		Summary:  meta.Summary,
		Created:  created,
		Updated:  updated,
	}, nil
}

func isEmptySession(sess sessionFile) bool {
	return len(sess.Messages) == 0 && strings.TrimSpace(sess.Summary) == ""
}

func truncateRunes(s string, maxLen int) string {
	if maxLen <= 0 {
		return ""
	}
	runes := []rune(strings.TrimSpace(s))
	if len(runes) <= maxLen {
		return string(runes)
	}
	return string(runes[:maxLen]) + "..."
}

func buildSessionListItem(sessionID string, sess sessionFile) sessionListItem {
	preview := ""
	for _, msg := range sess.Messages {
		if msg.Role == "user" && strings.TrimSpace(msg.Content) != "" {
			preview = msg.Content
			break
		}
	}
	title := strings.TrimSpace(sess.Summary)
	if title == "" {
		title = preview
	}

	title = truncateRunes(title, maxSessionTitleRunes)
	preview = truncateRunes(preview, maxSessionTitleRunes)

	if preview == "" {
		preview = "(empty)"
	}
	if title == "" {
		title = preview
	}

	validMessageCount := 0
	for _, msg := range sess.Messages {
		if (msg.Role == "user" || msg.Role == "assistant") && strings.TrimSpace(msg.Content) != "" {
			validMessageCount++
		}
	}

	return sessionListItem{
		ID:           sessionID,
		Title:        title,
		Preview:      preview,
		MessageCount: validMessageCount,
		Created:      sess.Created.Format(time.RFC3339),
		Updated:      sess.Updated.Format(time.RFC3339),
	}
}

// handleListSessions returns a list of API session summaries.
// GET /api/v1/sessions
func (h *Handler) handleListSessions(w http.ResponseWriter, r *http.Request) {
	dir, err := h.sessionsDir()
	if err != nil {
		http.Error(w, `{"error":"failed to resolve sessions directory"}`, http.StatusInternalServerError)
		return
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		json.NewEncoder(w).Encode([]sessionListItem{})
		return
	}

	items := []sessionListItem{}
	seen := make(map[string]struct{})

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		var (
			sessionID string
			sess      sessionFile
			loadErr   error
			ok        bool
		)

		switch {
		case strings.HasSuffix(name, ".jsonl"):
			sessionID, ok = extractAPISessionIDFromSanitizedKey(strings.TrimSuffix(name, ".jsonl"))
			if !ok {
				continue
			}
			sess, loadErr = h.readJSONLSession(dir, sessionID)
			if loadErr == nil && isEmptySession(sess) {
				continue
			}
		case strings.HasSuffix(name, ".meta.json"):
			continue
		default:
			continue
		}

		if loadErr != nil {
			continue
		}
		if _, exists := seen[sessionID]; exists {
			continue
		}

		seen[sessionID] = struct{}{}
		items = append(items, buildSessionListItem(sessionID, sess))
	}

	sort.Slice(items, func(i, j int) bool {
		return items[i].Updated > items[j].Updated
	})

	// Pagination
	offsetStr := r.URL.Query().Get("offset")
	limitStr := r.URL.Query().Get("limit")

	offset := 0
	limit := 20
	if val, err := strconv.Atoi(offsetStr); err == nil && val >= 0 {
		offset = val
	}
	if val, err := strconv.Atoi(limitStr); err == nil && val > 0 {
		limit = val
	}

	totalItems := len(items)
	end := offset + limit
	if offset >= totalItems {
		items = []sessionListItem{}
	} else {
		if end > totalItems {
			end = totalItems
		}
		items = items[offset:end]
	}

	json.NewEncoder(w).Encode(items)
}

// handleGetSession returns the full message history for a specific session.
// GET /api/v1/sessions/{id}
func (h *Handler) handleGetSession(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("id")
	if sessionID == "" {
		http.Error(w, `{"error":"missing session id"}`, http.StatusBadRequest)
		return
	}

	dir, err := h.sessionsDir()
	if err != nil {
		http.Error(w, `{"error":"failed to resolve sessions directory"}`, http.StatusInternalServerError)
		return
	}

	sess, err := h.readJSONLSession(dir, sessionID)
	if err != nil || isEmptySession(sess) {
		if err == nil || errors.Is(err, os.ErrNotExist) {
			http.Error(w, `{"error":"session not found"}`, http.StatusNotFound)
		} else {
			http.Error(w, `{"error":"failed to parse session"}`, http.StatusInternalServerError)
		}
		return
	}

	type chatMessage struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}

	messages := make([]chatMessage, 0, len(sess.Messages))
	for _, msg := range sess.Messages {
		if (msg.Role == "user" || msg.Role == "assistant") && strings.TrimSpace(msg.Content) != "" {
			messages = append(messages, chatMessage{
				Role:    msg.Role,
				Content: msg.Content,
			})
		}
	}

	json.NewEncoder(w).Encode(map[string]any{
		"id":       sessionID,
		"messages": messages,
		"summary":  sess.Summary,
		"created":  sess.Created.Format(time.RFC3339),
		"updated":  sess.Updated.Format(time.RFC3339),
	})
}

// handleDeleteSession deletes a specific session.
// DELETE /api/v1/sessions/{id}
func (h *Handler) handleDeleteSession(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("id")
	if sessionID == "" {
		http.Error(w, `{"error":"missing session id"}`, http.StatusBadRequest)
		return
	}

	dir, err := h.sessionsDir()
	if err != nil {
		http.Error(w, `{"error":"failed to resolve sessions directory"}`, http.StatusInternalServerError)
		return
	}

	base := filepath.Join(dir, sanitizeSessionKey(apiSessionPrefix+sessionID))
	jsonlPath := base + ".jsonl"
	metaPath := base + ".meta.json"

	removed := false
	for _, path := range []string{jsonlPath, metaPath} {
		if err := os.Remove(path); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			http.Error(w, `{"error":"failed to delete session"}`, http.StatusInternalServerError)
			return
		}
		removed = true
	}

	if !removed {
		http.Error(w, `{"error":"session not found"}`, http.StatusNotFound)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
