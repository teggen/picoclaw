package tools

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// mockCodingBackend is a test double for CodingAgentBackend.
type mockCodingBackend struct {
	name      string
	available bool
	result    *CodingAgentResult
	err       error
}

func (m *mockCodingBackend) Name() string      { return m.name }
func (m *mockCodingBackend) Available() bool    { return m.available }

func (m *mockCodingBackend) Execute(_ context.Context, _ CodingAgentExecOpts) (*CodingAgentResult, error) {
	return m.result, m.err
}

func TestCodingAgentTool_Name(t *testing.T) {
	tool := NewCodingAgentTool(&mockCodingBackend{name: "test"}, "/tmp", CodingAgentToolConfig{TimeoutSeconds: 60})
	if tool.Name() != "coding_agent" {
		t.Errorf("expected name 'coding_agent', got %q", tool.Name())
	}
}

func TestCodingAgentTool_Parameters(t *testing.T) {
	tool := NewCodingAgentTool(&mockCodingBackend{name: "test"}, "/tmp", CodingAgentToolConfig{TimeoutSeconds: 60})
	params := tool.Parameters()

	props, ok := params["properties"].(map[string]any)
	if !ok {
		t.Fatal("expected properties in parameters")
	}

	if _, ok := props["task"]; !ok {
		t.Error("expected 'task' parameter")
	}
	if _, ok := props["working_dir"]; !ok {
		t.Error("expected 'working_dir' parameter")
	}
	if _, ok := props["model"]; !ok {
		t.Error("expected 'model' parameter")
	}

	required, ok := params["required"].([]string)
	if !ok {
		t.Fatal("expected required in parameters")
	}
	if len(required) != 1 || required[0] != "task" {
		t.Errorf("expected required=['task'], got %v", required)
	}
}

func TestCodingAgentTool_EmptyTask(t *testing.T) {
	tool := NewCodingAgentTool(&mockCodingBackend{name: "test"}, "/tmp", CodingAgentToolConfig{TimeoutSeconds: 60})
	result := tool.Execute(context.Background(), map[string]any{})
	if !result.IsError {
		t.Error("expected error for empty task")
	}
	if !strings.Contains(result.ForLLM, "task is required") {
		t.Errorf("expected 'task is required' in error, got %q", result.ForLLM)
	}
}

func TestCodingAgentTool_WhitespaceTask(t *testing.T) {
	tool := NewCodingAgentTool(&mockCodingBackend{name: "test"}, "/tmp", CodingAgentToolConfig{TimeoutSeconds: 60})
	result := tool.Execute(context.Background(), map[string]any{"task": "   "})
	if !result.IsError {
		t.Error("expected error for whitespace-only task")
	}
}

func TestCodingAgentTool_SuccessfulExecution(t *testing.T) {
	backend := &mockCodingBackend{
		name:      "test_backend",
		available: true,
		result: &CodingAgentResult{
			Summary:       "Added rate limiting",
			FilesModified: []string{"pkg/middleware/ratelimit.go"},
			DurationMS:    5000,
			CostUSD:       0.05,
			TokensUsed:    1000,
			SessionID:     "test-session",
			RawOutput:     "Created rate limiting middleware",
		},
	}

	tool := NewCodingAgentTool(backend, "/tmp", CodingAgentToolConfig{TimeoutSeconds: 60})
	result := tool.Execute(context.Background(), map[string]any{"task": "add rate limiting"})

	if result.IsError {
		t.Errorf("unexpected error: %s", result.ForLLM)
	}
	if !strings.Contains(result.ForLLM, `backend="test_backend"`) {
		t.Error("expected backend name in result")
	}
	if !strings.Contains(result.ForLLM, `success="true"`) {
		t.Error("expected success=true in result")
	}
	if !strings.Contains(result.ForLLM, "<summary>Added rate limiting</summary>") {
		t.Errorf("expected summary in result, got %s", result.ForLLM)
	}
	if !strings.Contains(result.ForLLM, "ratelimit.go") {
		t.Error("expected file path in result")
	}
	if !strings.Contains(result.ForLLM, "<duration_ms>5000</duration_ms>") {
		t.Error("expected duration in result")
	}
	if !strings.Contains(result.ForLLM, "<session_id>test-session</session_id>") {
		t.Error("expected session_id in result")
	}
	if !strings.Contains(result.ForUser, "Added rate limiting") {
		t.Error("expected summary in ForUser")
	}
	if !strings.Contains(result.ForUser, "1 files modified") {
		t.Error("expected files count in ForUser")
	}
}

func TestCodingAgentTool_BackendError(t *testing.T) {
	backend := &mockCodingBackend{
		name:      "test_backend",
		available: true,
		err:       fmt.Errorf("connection refused"),
	}

	tool := NewCodingAgentTool(backend, "/tmp", CodingAgentToolConfig{TimeoutSeconds: 60})
	result := tool.Execute(context.Background(), map[string]any{"task": "do something"})

	if !result.IsError {
		t.Error("expected error result")
	}
	if !strings.Contains(result.ForLLM, `success="false"`) {
		t.Error("expected success=false in result")
	}
	if !strings.Contains(result.ForLLM, "connection refused") {
		t.Error("expected error message in result")
	}
}

func TestCodingAgentTool_BackendResultError(t *testing.T) {
	backend := &mockCodingBackend{
		name:      "test_backend",
		available: true,
		result: &CodingAgentResult{
			IsError:      true,
			ErrorMessage: "syntax error in file",
			RawOutput:    "some partial output",
		},
	}

	tool := NewCodingAgentTool(backend, "/tmp", CodingAgentToolConfig{TimeoutSeconds: 60})
	result := tool.Execute(context.Background(), map[string]any{"task": "fix bug"})

	if !result.IsError {
		t.Error("expected error result")
	}
	if !strings.Contains(result.ForLLM, "syntax error in file") {
		t.Error("expected error message in result")
	}
	if !strings.Contains(result.ForLLM, "<partial_output>") {
		t.Error("expected partial output in result")
	}
}

func TestCodingAgentTool_Timeout(t *testing.T) {
	// Use a cancelled context to simulate timeout
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel()
	time.Sleep(5 * time.Millisecond) // let it expire

	backend := &mockCodingBackend{
		name:      "test_backend",
		available: true,
		err:       context.DeadlineExceeded,
	}

	tool := NewCodingAgentTool(backend, "/tmp", CodingAgentToolConfig{})
	result := tool.Execute(ctx, map[string]any{"task": "slow task"})

	if !result.IsError {
		t.Error("expected error for timeout")
	}
	if !strings.Contains(result.ForLLM, "timed out") {
		t.Errorf("expected timeout message, got %s", result.ForLLM)
	}
}

func TestCodingAgentTool_SessionID(t *testing.T) {
	var capturedOpts CodingAgentExecOpts
	backend := &mockCodingBackend{
		name:      "test_backend",
		available: true,
		result:    &CodingAgentResult{Summary: "done"},
	}

	// Wrap to capture opts
	wrappedBackend := &capturingBackend{
		CodingAgentBackend: backend,
		captured:           &capturedOpts,
	}

	tool := NewCodingAgentTool(wrappedBackend, "/tmp", CodingAgentToolConfig{TimeoutSeconds: 60, SessionContinuity: true})

	ctx := WithToolContext(context.Background(), "telegram", "chat123")
	tool.Execute(ctx, map[string]any{"task": "do something"})

	// Session ID must be a valid UUID format: xxxxxxxx-xxxx-5xxx-yxxx-xxxxxxxxxxxx
	if len(capturedOpts.SessionID) != 36 {
		t.Errorf("expected UUID length 36, got %d (%q)", len(capturedOpts.SessionID), capturedOpts.SessionID)
	}
	parts := strings.Split(capturedOpts.SessionID, "-")
	if len(parts) != 5 {
		t.Errorf("expected 5 UUID parts, got %d", len(parts))
	}
	// Check version nibble is 5
	if len(parts) >= 3 && len(parts[2]) >= 1 && parts[2][0] != '5' {
		t.Errorf("expected UUID version 5, got %q", parts[2])
	}
}

func TestCodingAgentTool_NoSessionIDWhenDisabled(t *testing.T) {
	var capturedOpts CodingAgentExecOpts
	backend := &mockCodingBackend{
		name:      "test_backend",
		available: true,
		result:    &CodingAgentResult{Summary: "done"},
	}

	wrappedBackend := &capturingBackend{
		CodingAgentBackend: backend,
		captured:           &capturedOpts,
	}

	tool := NewCodingAgentTool(wrappedBackend, "/tmp", CodingAgentToolConfig{TimeoutSeconds: 60})

	ctx := WithToolContext(context.Background(), "telegram", "chat123")
	tool.Execute(ctx, map[string]any{"task": "do something"})

	if capturedOpts.SessionID != "" {
		t.Errorf("expected empty session ID when disabled, got %q", capturedOpts.SessionID)
	}
}

func TestCodingAgentTool_XMLEscape(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"hello", "hello"},
		{"<script>", "&lt;script&gt;"},
		{"a & b", "a &amp; b"},
		{`say "hi"`, "say &quot;hi&quot;"},
	}
	for _, tt := range tests {
		got := xmlEscape(tt.input)
		if got != tt.expected {
			t.Errorf("xmlEscape(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestCodingAgentTool_TruncateString(t *testing.T) {
	short := "hello"
	if truncateString(short, 100) != short {
		t.Error("should not truncate short string")
	}

	long := strings.Repeat("a", 200)
	truncated := truncateString(long, 100)
	if !strings.HasPrefix(truncated, strings.Repeat("a", 100)) {
		t.Error("truncated string should start with first 100 chars")
	}
	if !strings.Contains(truncated, "truncated") {
		t.Error("truncated string should contain truncation indicator")
	}
}

func TestNewCodingAgentBackendFromConfig(t *testing.T) {
	// Default / claude_code
	b := NewCodingAgentBackendFromConfig(CodingAgentToolConfig{Backend: "claude_code"})
	if b == nil {
		t.Fatal("expected non-nil backend for claude_code")
	}
	if b.Name() != "claude_code" {
		t.Errorf("expected name 'claude_code', got %q", b.Name())
	}

	// Empty defaults to claude_code
	b = NewCodingAgentBackendFromConfig(CodingAgentToolConfig{})
	if b == nil {
		t.Fatal("expected non-nil backend for empty config")
	}

	// Unknown backend
	b = NewCodingAgentBackendFromConfig(CodingAgentToolConfig{Backend: "unknown"})
	if b != nil {
		t.Error("expected nil backend for unknown")
	}
}

func TestDeterministicUUID(t *testing.T) {
	uuid1 := deterministicUUID("telegram", "chat123")
	uuid2 := deterministicUUID("telegram", "chat123")
	uuid3 := deterministicUUID("discord", "chat123")

	// Same inputs produce same UUID
	if uuid1 != uuid2 {
		t.Errorf("expected deterministic UUID, got %q and %q", uuid1, uuid2)
	}

	// Different inputs produce different UUIDs
	if uuid1 == uuid3 {
		t.Error("expected different UUIDs for different inputs")
	}

	// Valid UUID format
	if len(uuid1) != 36 {
		t.Errorf("expected UUID length 36, got %d", len(uuid1))
	}
	parts := strings.Split(uuid1, "-")
	if len(parts) != 5 {
		t.Errorf("expected 5 UUID parts, got %d", len(parts))
	}
	if len(parts[0]) != 8 || len(parts[1]) != 4 || len(parts[2]) != 4 || len(parts[3]) != 4 || len(parts[4]) != 12 {
		t.Errorf("UUID part lengths wrong: %v", parts)
	}
	// Version nibble should be 5
	if parts[2][0] != '5' {
		t.Errorf("expected version 5, got %c", parts[2][0])
	}
	// Variant nibble should be 8, 9, a, or b
	v := parts[3][0]
	if v != '8' && v != '9' && v != 'a' && v != 'b' {
		t.Errorf("expected variant 8/9/a/b, got %c", v)
	}
}

func TestExtractSummary(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"", ""},
		{"single line", "single line"},
		{"first line\nsecond line", "first line"},
		{strings.Repeat("a", 250), strings.Repeat("a", 200) + "..."},
	}
	for _, tt := range tests {
		got := extractSummary(tt.input)
		if got != tt.expected {
			t.Errorf("extractSummary(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestExtractFilePaths(t *testing.T) {
	text := `Modified pkg/tools/coding_agent.go and pkg/tools/coding_agent_test.go.
Also updated README.md but not "just a word".`

	paths := extractFilePaths(text)
	if len(paths) != 2 {
		t.Errorf("expected 2 paths, got %d: %v", len(paths), paths)
	}
}

// capturingBackend wraps a CodingAgentBackend and captures the execution opts.
type capturingBackend struct {
	CodingAgentBackend
	captured *CodingAgentExecOpts
}

func (c *capturingBackend) Execute(ctx context.Context, opts CodingAgentExecOpts) (*CodingAgentResult, error) {
	*c.captured = opts
	return c.CodingAgentBackend.Execute(ctx, opts)
}

func TestCodingAgentTool_EffortFlag(t *testing.T) {
	var capturedOpts CodingAgentExecOpts
	backend := &capturingBackend{
		CodingAgentBackend: &mockCodingBackend{name: "test", available: true, result: &CodingAgentResult{Summary: "done"}},
		captured:           &capturedOpts,
	}

	tool := NewCodingAgentTool(backend, "/tmp", CodingAgentToolConfig{TimeoutSeconds: 60, Effort: "high"})
	tool.Execute(context.Background(), map[string]any{"task": "do something"})

	if capturedOpts.Effort != "high" {
		t.Errorf("expected effort 'high', got %q", capturedOpts.Effort)
	}
}

func TestCodingAgentTool_AppendSystemPromptDefault(t *testing.T) {
	var capturedOpts CodingAgentExecOpts
	backend := &capturingBackend{
		CodingAgentBackend: &mockCodingBackend{name: "test", available: true, result: &CodingAgentResult{Summary: "done"}},
		captured:           &capturedOpts,
	}

	tool := NewCodingAgentTool(backend, "/tmp", CodingAgentToolConfig{TimeoutSeconds: 60})
	tool.Execute(context.Background(), map[string]any{"task": "do something"})

	absPath, _ := filepath.Abs("/tmp")
	if !strings.Contains(capturedOpts.AppendSystemPrompt, "PicoClaw") {
		t.Errorf("expected default prompt to mention PicoClaw, got %q", capturedOpts.AppendSystemPrompt)
	}
	if !strings.Contains(capturedOpts.AppendSystemPrompt, absPath) {
		t.Errorf("expected default prompt to contain workspace path %q, got %q", absPath, capturedOpts.AppendSystemPrompt)
	}
}

func TestCodingAgentTool_AppendSystemPromptCustom(t *testing.T) {
	var capturedOpts CodingAgentExecOpts
	backend := &capturingBackend{
		CodingAgentBackend: &mockCodingBackend{name: "test", available: true, result: &CodingAgentResult{Summary: "done"}},
		captured:           &capturedOpts,
	}

	custom := "Custom system prompt for this project"
	tool := NewCodingAgentTool(backend, "/tmp", CodingAgentToolConfig{TimeoutSeconds: 60, AppendSystemPrompt: custom})
	tool.Execute(context.Background(), map[string]any{"task": "do something"})

	if capturedOpts.AppendSystemPrompt != custom {
		t.Errorf("expected custom prompt %q, got %q", custom, capturedOpts.AppendSystemPrompt)
	}
}

func TestCodingAgentTool_WorktreeFlag(t *testing.T) {
	var capturedOpts CodingAgentExecOpts
	backend := &capturingBackend{
		CodingAgentBackend: &mockCodingBackend{name: "test", available: true, result: &CodingAgentResult{Summary: "done"}},
		captured:           &capturedOpts,
	}

	tool := NewCodingAgentTool(backend, "/tmp", CodingAgentToolConfig{TimeoutSeconds: 60, Worktree: true})
	tool.Execute(context.Background(), map[string]any{"task": "do something"})

	if !capturedOpts.Worktree {
		t.Error("expected worktree to be true")
	}
}

func TestCodingAgentTool_VerboseFlag(t *testing.T) {
	var capturedOpts CodingAgentExecOpts
	backend := &capturingBackend{
		CodingAgentBackend: &mockCodingBackend{name: "test", available: true, result: &CodingAgentResult{Summary: "done"}},
		captured:           &capturedOpts,
	}

	tool := NewCodingAgentTool(backend, "/tmp", CodingAgentToolConfig{TimeoutSeconds: 60, Verbose: true})
	tool.Execute(context.Background(), map[string]any{"task": "do something"})

	if !capturedOpts.Verbose {
		t.Error("expected verbose to be true")
	}
}

func TestValidateEffortLevel(t *testing.T) {
	valid := []string{"", "low", "medium", "high", "max"}
	for _, v := range valid {
		if err := validateEffortLevel(v); err != nil {
			t.Errorf("expected %q to be valid, got error: %v", v, err)
		}
	}

	invalid := []string{"extreme", "LOW", "Medium", "none", "超高"}
	for _, v := range invalid {
		if err := validateEffortLevel(v); err == nil {
			t.Errorf("expected %q to be invalid, got nil error", v)
		}
	}
}
