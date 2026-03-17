package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

// ClaudeCodeBackend implements CodingAgentBackend using the Claude Code CLI.
type ClaudeCodeBackend struct {
	command      string
	defaultModel string
}

// NewClaudeCodeBackend creates a new Claude Code backend.
func NewClaudeCodeBackend(command, defaultModel string) *ClaudeCodeBackend {
	if command == "" {
		command = "claude"
	}
	return &ClaudeCodeBackend{
		command:      command,
		defaultModel: defaultModel,
	}
}

func (b *ClaudeCodeBackend) Name() string {
	return "claude_code"
}

func (b *ClaudeCodeBackend) Available() bool {
	_, err := exec.LookPath(b.command)
	return err == nil
}

func (b *ClaudeCodeBackend) Execute(ctx context.Context, opts CodingAgentExecOpts) (*CodingAgentResult, error) {
	args := []string{"-p", "--output-format", "json", "--dangerously-skip-permissions", "--no-chrome"}

	// Session continuity: --resume takes precedence (continues existing session),
	// --session-id creates/reuses a session with a specific UUID.
	if opts.ResumeID != "" {
		args = append(args, "--resume", opts.ResumeID)
	} else if opts.SessionID != "" {
		args = append(args, "--session-id", opts.SessionID)
	}

	model := opts.Model
	if model == "" {
		model = b.defaultModel
	}
	if model != "" {
		args = append(args, "--model", model)
	}

	if opts.MaxTurns > 0 {
		args = append(args, "--max-turns", fmt.Sprintf("%d", opts.MaxTurns))
	}

	if opts.Effort != "" {
		args = append(args, "--effort", opts.Effort)
	}
	if opts.AppendSystemPrompt != "" {
		args = append(args, "--append-system-prompt", opts.AppendSystemPrompt)
	}
	if opts.Worktree {
		args = append(args, "--worktree")
	}
	if opts.Verbose {
		args = append(args, "--verbose")
	}

	args = append(args, "-") // read from stdin

	cmd := exec.CommandContext(ctx, b.command, args...)
	if opts.WorkingDir != "" {
		cmd.Dir = opts.WorkingDir
	}
	cmd.Stdin = bytes.NewReader([]byte(opts.Task))

	// Set up process group for clean termination on timeout
	prepareCommandForTermination(cmd)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("claude cli start error: %w", err)
	}

	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	var err error
	select {
	case err = <-done:
	case <-ctx.Done():
		// Context cancelled/timed out — terminate process tree
		_ = terminateProcessTree(cmd)
		select {
		case err = <-done:
		case <-time.After(3 * time.Second):
			if cmd.Process != nil {
				_ = cmd.Process.Kill()
			}
			err = <-done
		}
		return nil, ctx.Err()
	}

	if err != nil {
		stderrStr := strings.TrimSpace(stderr.String())
		if stderrStr != "" {
			return nil, fmt.Errorf("claude cli error: %s", stderrStr)
		}
		return nil, fmt.Errorf("claude cli error: %w", err)
	}

	return b.parseResponse(stdout.String())
}

// claudeCodeJSONResponse represents the JSON output from the claude CLI.
// Duplicated from providers/claude_cli_provider.go to avoid coupling tools → providers.
type claudeCodeJSONResponse struct {
	Type         string              `json:"type"`
	Subtype      string              `json:"subtype"`
	IsError      bool                `json:"is_error"`
	Result       string              `json:"result"`
	SessionID    string              `json:"session_id"`
	TotalCostUSD float64             `json:"total_cost_usd"`
	DurationMS   int                 `json:"duration_ms"`
	DurationAPI  int                 `json:"duration_api_ms"`
	NumTurns     int                 `json:"num_turns"`
	Usage        claudeCodeUsageInfo `json:"usage"`
}

// claudeCodeUsageInfo represents token usage from the claude CLI response.
type claudeCodeUsageInfo struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens"`
}

// filePathRegex matches common file paths in output text.
var filePathRegex = regexp.MustCompile(`(?m)(?:^|\s)([a-zA-Z0-9_./\-]+\.[a-zA-Z0-9]+)`)

func (b *ClaudeCodeBackend) parseResponse(output string) (*CodingAgentResult, error) {
	var resp claudeCodeJSONResponse
	if err := json.Unmarshal([]byte(output), &resp); err != nil {
		return nil, fmt.Errorf("failed to parse claude cli response: %w", err)
	}

	totalTokens := resp.Usage.InputTokens + resp.Usage.OutputTokens +
		resp.Usage.CacheCreationInputTokens + resp.Usage.CacheReadInputTokens

	result := &CodingAgentResult{
		DurationMS: resp.DurationMS,
		CostUSD:    resp.TotalCostUSD,
		TokensUsed: totalTokens,
		SessionID:  resp.SessionID,
		RawOutput:  resp.Result,
	}

	if resp.IsError {
		result.IsError = true
		result.ErrorMessage = resp.Result
		return result, nil
	}

	// Extract summary: use first line or first sentence
	result.Summary = extractSummary(resp.Result)

	// Extract file paths from the result text
	result.FilesModified = extractFilePaths(resp.Result)

	return result, nil
}

// extractSummary extracts a concise summary from the result text.
func extractSummary(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}

	// Use first line, capped at 200 chars
	firstLine := text
	if idx := strings.IndexByte(text, '\n'); idx > 0 {
		firstLine = text[:idx]
	}
	firstLine = strings.TrimSpace(firstLine)
	if len(firstLine) > 200 {
		firstLine = firstLine[:200] + "..."
	}
	return firstLine
}

// extractFilePaths attempts to extract file paths from the result text.
func extractFilePaths(text string) []string {
	matches := filePathRegex.FindAllStringSubmatch(text, -1)
	seen := make(map[string]bool)
	var paths []string
	for _, m := range matches {
		p := strings.TrimSpace(m[1])
		// Filter: must contain a path separator and a reasonable extension
		if !strings.Contains(p, "/") && !strings.Contains(p, "\\") {
			continue
		}
		if seen[p] {
			continue
		}
		seen[p] = true
		paths = append(paths, p)
	}
	return paths
}
