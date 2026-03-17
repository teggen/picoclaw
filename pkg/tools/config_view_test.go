package tools

import (
	"context"
	"strings"
	"testing"

	"github.com/sipeed/picoclaw/pkg/config"
)

func newTestConfig() *config.Config {
	cfg := config.DefaultConfig()
	cfg.BuildInfo = config.BuildInfo{
		Version:   "1.2.3",
		GitCommit: "abc1234",
		BuildTime: "2025-01-01T00:00:00Z",
		GoVersion: "go1.25.0",
	}
	cfg.Agents.Defaults.ModelName = "test-model"
	cfg.Agents.Defaults.Workspace = "/tmp/test-workspace"
	cfg.ModelList = []config.ModelConfig{
		{
			ModelName:     "gpt-4o",
			Model:         "openai/gpt-4o",
			APIKey:        "sk-super-secret-key-12345",
			APIBase:       "https://api.openai.com/v1",
			RPM:           60,
			ThinkingLevel: "high",
		},
		{
			ModelName: "local-llama",
			Model:     "ollama/llama3",
			APIKey:    "",
		},
	}
	cfg.Channels.Telegram.Enabled = true
	cfg.Providers.Anthropic.APIKey = "ant-secret-key-67890"
	cfg.Providers.OpenAI.APIKey = "sk-openai-secret"
	return cfg
}

func TestConfigViewOverview(t *testing.T) {
	tool := NewConfigViewTool(newTestConfig())
	result := tool.Execute(context.Background(), map[string]any{"section": "overview"})

	if result.IsError {
		t.Fatalf("unexpected error: %s", result.ForLLM)
	}
	if !result.Silent {
		t.Error("expected silent result")
	}

	out := result.ForLLM
	if !strings.Contains(out, "1.2.3") {
		t.Error("expected version in overview")
	}
	if !strings.Contains(out, "test-model") {
		t.Error("expected default model in overview")
	}
	if !strings.Contains(out, "Configured models (with API key): 1") {
		t.Error("expected 1 configured model (only one has API key set)")
	}
}

func TestConfigViewModels(t *testing.T) {
	tool := NewConfigViewTool(newTestConfig())
	result := tool.Execute(context.Background(), map[string]any{"section": "models"})

	if result.IsError {
		t.Fatalf("unexpected error: %s", result.ForLLM)
	}

	out := result.ForLLM
	if !strings.Contains(out, "gpt-4o") {
		t.Error("expected model name gpt-4o")
	}
	if !strings.Contains(out, "openai/gpt-4o") {
		t.Error("expected model identifier")
	}
	if !strings.Contains(out, "[SET]") {
		t.Error("expected [SET] for API key")
	}
	if !strings.Contains(out, "[NOT SET]") {
		t.Error("expected [NOT SET] for missing API key")
	}
	if !strings.Contains(out, "RPM: 60") {
		t.Error("expected RPM")
	}
	if !strings.Contains(out, "Thinking Level: high") {
		t.Error("expected thinking level")
	}
}

func TestConfigViewModelsNoSecrets(t *testing.T) {
	tool := NewConfigViewTool(newTestConfig())
	result := tool.Execute(context.Background(), map[string]any{"section": "models"})

	out := result.ForLLM
	if strings.Contains(out, "sk-super-secret-key-12345") {
		t.Error("API key must not appear in output")
	}
}

func TestConfigViewChannels(t *testing.T) {
	tool := NewConfigViewTool(newTestConfig())
	result := tool.Execute(context.Background(), map[string]any{"section": "channels"})

	if result.IsError {
		t.Fatalf("unexpected error: %s", result.ForLLM)
	}

	out := result.ForLLM
	if !strings.Contains(out, "telegram: enabled") {
		t.Error("expected telegram enabled")
	}
	if !strings.Contains(out, "discord: disabled") {
		t.Error("expected discord disabled")
	}
}

func TestConfigViewTools(t *testing.T) {
	tool := NewConfigViewTool(newTestConfig())
	result := tool.Execute(context.Background(), map[string]any{"section": "tools"})

	if result.IsError {
		t.Fatalf("unexpected error: %s", result.ForLLM)
	}

	out := result.ForLLM
	if !strings.Contains(out, "exec: enabled") {
		t.Error("expected exec enabled")
	}
	if !strings.Contains(out, "Exec timeout:") {
		t.Error("expected exec timeout")
	}
	if !strings.Contains(out, "Coding agent backend:") {
		t.Error("expected coding agent backend")
	}
}

func TestConfigViewAgents(t *testing.T) {
	tool := NewConfigViewTool(newTestConfig())
	result := tool.Execute(context.Background(), map[string]any{"section": "agents"})

	if result.IsError {
		t.Fatalf("unexpected error: %s", result.ForLLM)
	}

	out := result.ForLLM
	if !strings.Contains(out, "test-model") {
		t.Error("expected default model name")
	}
	if !strings.Contains(out, "Max tokens:") {
		t.Error("expected max tokens")
	}
}

func TestConfigViewProviders(t *testing.T) {
	tool := NewConfigViewTool(newTestConfig())
	result := tool.Execute(context.Background(), map[string]any{"section": "providers"})

	if result.IsError {
		t.Fatalf("unexpected error: %s", result.ForLLM)
	}

	out := result.ForLLM
	if !strings.Contains(out, "anthropic: API key [SET]") {
		t.Error("expected anthropic key set")
	}
	if strings.Contains(out, "ant-secret-key-67890") {
		t.Error("anthropic API key must not appear in output")
	}
	if strings.Contains(out, "sk-openai-secret") {
		t.Error("openai API key must not appear in output")
	}
}

func TestConfigViewBuildInfo(t *testing.T) {
	tool := NewConfigViewTool(newTestConfig())
	result := tool.Execute(context.Background(), map[string]any{"section": "build_info"})

	if result.IsError {
		t.Fatalf("unexpected error: %s", result.ForLLM)
	}

	out := result.ForLLM
	if !strings.Contains(out, "1.2.3") {
		t.Error("expected version")
	}
	if !strings.Contains(out, "abc1234") {
		t.Error("expected git commit")
	}
	if !strings.Contains(out, "go1.25.0") {
		t.Error("expected go version")
	}
}

func TestConfigViewUnknownSection(t *testing.T) {
	tool := NewConfigViewTool(newTestConfig())
	result := tool.Execute(context.Background(), map[string]any{"section": "foobar"})

	if !result.IsError {
		t.Error("expected error for unknown section")
	}
}

func TestConfigViewMissingSection(t *testing.T) {
	tool := NewConfigViewTool(newTestConfig())
	result := tool.Execute(context.Background(), map[string]any{})

	if !result.IsError {
		t.Error("expected error for missing section")
	}
}

func TestConfigViewEmptyModelList(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.ModelList = nil
	tool := NewConfigViewTool(cfg)
	result := tool.Execute(context.Background(), map[string]any{"section": "models"})

	if result.IsError {
		t.Fatalf("unexpected error: %s", result.ForLLM)
	}
	if !strings.Contains(result.ForLLM, "No models configured") {
		t.Error("expected empty model list message")
	}
}
