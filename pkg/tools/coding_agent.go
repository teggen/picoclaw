package tools

import (
	"context"
	"crypto/sha256"
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

// CodingAgentBackend is the interface for coding agent backends (Claude Code, Aider, etc.).
type CodingAgentBackend interface {
	Name() string
	Available() bool
	Execute(ctx context.Context, opts CodingAgentExecOpts) (*CodingAgentResult, error)
}

// CodingAgentExecOpts contains the options for executing a coding agent task.
type CodingAgentExecOpts struct {
	Task               string
	WorkingDir         string
	SessionID          string // must be a valid UUID if non-empty
	ResumeID           string // session ID from a previous response (for --resume)
	Model              string
	MaxTurns           int
	Effort             string
	AppendSystemPrompt string
	Worktree           bool
	Verbose            bool
}

// CodingAgentResult contains the structured result from a coding agent execution.
type CodingAgentResult struct {
	Summary       string
	FilesModified []string
	IsError       bool
	ErrorMessage  string
	CostUSD       float64
	DurationMS    int
	TokensUsed    int
	RawOutput     string
	SessionID     string
}

// CodingAgentTool implements the Tool interface for delegating coding tasks to a coding agent.
type CodingAgentTool struct {
	backend            CodingAgentBackend
	workspace          string
	timeout            time.Duration
	maxTurns           int
	sessionContinuity  bool
	effort             string
	appendSystemPrompt string
	worktree           bool
	verbose            bool
}

// NewCodingAgentTool creates a new CodingAgentTool.
func NewCodingAgentTool(backend CodingAgentBackend, workspace string, cfg CodingAgentToolConfig) *CodingAgentTool {
	timeout := 600 * time.Second
	if cfg.TimeoutSeconds > 0 {
		timeout = time.Duration(cfg.TimeoutSeconds) * time.Second
	}

	effort := cfg.Effort
	if effort != "" {
		if err := validateEffortLevel(effort); err != nil {
			fmt.Printf("Warning: %v, ignoring\n", err)
			effort = ""
		}
	}

	appendPrompt := cfg.AppendSystemPrompt
	if appendPrompt == "" {
		appendPrompt = buildDefaultAppendPrompt(workspace)
	}

	return &CodingAgentTool{
		backend:            backend,
		workspace:          workspace,
		timeout:            timeout,
		maxTurns:           cfg.MaxTurns,
		sessionContinuity:  cfg.SessionContinuity,
		effort:             effort,
		appendSystemPrompt: appendPrompt,
		worktree:           cfg.Worktree,
		verbose:            cfg.Verbose,
	}
}

// validateEffortLevel checks that the effort level is valid.
func validateEffortLevel(level string) error {
	switch level {
	case "", "low", "medium", "high", "max":
		return nil
	default:
		return fmt.Errorf("invalid effort level %q: must be low, medium, high, or max", level)
	}
}

// buildDefaultAppendPrompt generates a default --append-system-prompt blurb.
func buildDefaultAppendPrompt(workspace string) string {
	absWorkspace, _ := filepath.Abs(workspace)
	return fmt.Sprintf(
		"You are being invoked as a coding agent by PicoClaw, "+
			"an AI assistant framework. The PicoClaw workspace is at: %s. "+
			"Any CLAUDE.md files in the working directory are already available to you.",
		absWorkspace,
	)
}

func (t *CodingAgentTool) Name() string {
	return "coding_agent"
}

func (t *CodingAgentTool) Description() string {
	return `Delegate a coding task to an autonomous coding agent. Use this tool for ANY task that involves writing, modifying, creating, or deleting code files. The coding agent will execute the task and return a structured result with a summary, list of modified files, and execution metadata. Provide a clear, detailed task description including relevant context, file paths, and expected behavior.`
}

func (t *CodingAgentTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"task": map[string]any{
				"type":        "string",
				"description": "Detailed coding task description including context, file paths, and expected behavior",
			},
			"working_dir": map[string]any{
				"type":        "string",
				"description": "Optional subdirectory within workspace to use as working directory",
			},
			"model": map[string]any{
				"type":        "string",
				"description": "Optional model override for the coding agent",
			},
		},
		"required": []string{"task"},
	}
}

func (t *CodingAgentTool) Execute(ctx context.Context, args map[string]any) *ToolResult {
	task, _ := args["task"].(string)
	if strings.TrimSpace(task) == "" {
		return ErrorResult("task is required and must be non-empty")
	}

	workDir := t.workspace
	if wd, ok := args["working_dir"].(string); ok && strings.TrimSpace(wd) != "" {
		resolved, err := validatePath(wd, t.workspace, true)
		if err != nil {
			return ErrorResult(fmt.Sprintf("invalid working_dir: %v", err))
		}
		workDir = resolved
	}

	model, _ := args["model"].(string)

	// Derive deterministic session ID (UUID v5 format) for continuity.
	// --session-id requires a valid UUID per Claude Code CLI docs.
	var sessionID string
	if t.sessionContinuity {
		channel := ToolChannel(ctx)
		chatID := ToolChatID(ctx)
		if channel != "" || chatID != "" {
			sessionID = deterministicUUID(channel, chatID)
		}
	}

	// Create timeout context
	execCtx, cancel := context.WithTimeout(ctx, t.timeout)
	defer cancel()

	result, err := t.backend.Execute(execCtx, CodingAgentExecOpts{
		Task:               task,
		WorkingDir:         workDir,
		SessionID:          sessionID,
		Model:              model,
		MaxTurns:           t.maxTurns,
		Effort:             t.effort,
		AppendSystemPrompt: t.appendSystemPrompt,
		Worktree:           t.worktree,
		Verbose:            t.verbose,
	})
	if err != nil {
		if execCtx.Err() == context.DeadlineExceeded {
			return &ToolResult{
				ForLLM: fmt.Sprintf(
					`<coding_agent_result backend="%s" success="false">
  <error>Command timed out after %d seconds</error>
</coding_agent_result>`, t.backend.Name(), int(t.timeout.Seconds())),
				ForUser: fmt.Sprintf("Coding agent timed out after %d seconds", int(t.timeout.Seconds())),
				IsError: true,
			}
		}
		return &ToolResult{
			ForLLM: fmt.Sprintf(
				`<coding_agent_result backend="%s" success="false">
  <error>%s</error>
</coding_agent_result>`, t.backend.Name(), xmlEscape(err.Error())),
			ForUser: fmt.Sprintf("Coding agent error: %s", err.Error()),
			IsError: true,
		}
	}

	if result.IsError {
		partialOutput := ""
		if result.RawOutput != "" {
			partialOutput = fmt.Sprintf(
				"\n  <partial_output>%s</partial_output>",
				xmlEscape(truncateString(result.RawOutput, 10000)),
			)
		}
		return &ToolResult{
			ForLLM: fmt.Sprintf(
				`<coding_agent_result backend="%s" success="false">
  <error>%s</error>%s
</coding_agent_result>`, t.backend.Name(), xmlEscape(result.ErrorMessage), partialOutput),
			ForUser: fmt.Sprintf("Coding agent error: %s", result.ErrorMessage),
			IsError: true,
		}
	}

	return t.formatSuccessResult(result)
}

func (t *CodingAgentTool) formatSuccessResult(result *CodingAgentResult) *ToolResult {
	var sb strings.Builder
	fmt.Fprintf(&sb, `<coding_agent_result backend="%s" success="true">`, t.backend.Name())

	if result.Summary != "" {
		fmt.Fprintf(&sb, "\n  <summary>%s</summary>", xmlEscape(result.Summary))
	}

	if len(result.FilesModified) > 0 {
		sb.WriteString("\n  <files_modified>")
		for _, f := range result.FilesModified {
			// Make paths relative to workspace if possible
			relPath := f
			if rel, err := filepath.Rel(t.workspace, f); err == nil && !strings.HasPrefix(rel, "..") {
				relPath = rel
			}
			fmt.Fprintf(&sb, "\n    <file>%s</file>", xmlEscape(relPath))
		}
		sb.WriteString("\n  </files_modified>")
	}

	if result.DurationMS > 0 {
		fmt.Fprintf(&sb, "\n  <duration_ms>%d</duration_ms>", result.DurationMS)
	}
	if result.CostUSD > 0 {
		fmt.Fprintf(&sb, "\n  <cost_usd>%.4f</cost_usd>", result.CostUSD)
	}
	if result.TokensUsed > 0 {
		fmt.Fprintf(&sb, "\n  <tokens_used>%d</tokens_used>", result.TokensUsed)
	}
	if result.SessionID != "" {
		fmt.Fprintf(&sb, "\n  <session_id>%s</session_id>", xmlEscape(result.SessionID))
	}

	if result.RawOutput != "" {
		fmt.Fprintf(&sb, "\n  <details>%s</details>", xmlEscape(truncateString(result.RawOutput, 10000)))
	}

	sb.WriteString("\n</coding_agent_result>")

	// User-facing summary
	userMsg := "Coding task completed"
	if result.Summary != "" {
		userMsg = result.Summary
	}
	if len(result.FilesModified) > 0 {
		userMsg += fmt.Sprintf(" (%d files modified)", len(result.FilesModified))
	}

	return &ToolResult{
		ForLLM:  sb.String(),
		ForUser: userMsg,
	}
}

// deterministicUUID generates a deterministic UUID from channel and chatID.
// It produces a UUID v5-like format (xxxxxxxx-xxxx-5xxx-yxxx-xxxxxxxxxxxx)
// where the bytes are derived from SHA256, with version and variant bits set
// per RFC 4122. This satisfies the --session-id UUID requirement.
func deterministicUUID(channel, chatID string) string {
	h := sha256.Sum256([]byte(channel + ":" + chatID))
	// Use first 16 bytes for UUID
	b := h[:16]
	// Set version 5 (SHA-based)
	b[6] = (b[6] & 0x0f) | 0x50
	// Set variant (RFC 4122)
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// xmlEscape escapes special XML characters in a string.
func xmlEscape(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, `"`, "&quot;")
	return s
}

// truncateString truncates a string to maxLen characters with an indicator.
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + fmt.Sprintf("\n... (truncated, %d more chars)", len(s)-maxLen)
}

// NewCodingAgentBackendFromConfig creates a coding agent backend from config.
func NewCodingAgentBackendFromConfig(cfg CodingAgentToolConfig) CodingAgentBackend {
	switch cfg.Backend {
	case "claude_code", "":
		cmd := cfg.Command
		if cmd == "" {
			cmd = "claude"
		}
		return NewClaudeCodeBackend(cmd, cfg.Model)
	default:
		fmt.Printf("Warning: unknown coding_agent backend %q, skipping\n", cfg.Backend)
		return nil
	}
}

// CodingAgentToolConfig mirrors the config.CodingAgentConfig fields needed by the tools package.
// This avoids importing the config package from tools.
type CodingAgentToolConfig struct {
	Backend            string
	Command            string
	Model              string
	Force              bool
	TimeoutSeconds     int
	MaxTurns           int
	SessionContinuity  bool
	Effort             string
	AppendSystemPrompt string
	Worktree           bool
	Verbose            bool
}
