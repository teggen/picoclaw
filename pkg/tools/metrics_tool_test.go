package tools

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/sipeed/picoclaw/pkg/metrics"
)

func TestMetricsTool_Name(t *testing.T) {
	c := metrics.NewCollector()
	tool := NewMetricsTool(c)
	if tool.Name() != "metrics" {
		t.Errorf("Name() = %q, want %q", tool.Name(), "metrics")
	}
}

func TestMetricsTool_Parameters(t *testing.T) {
	c := metrics.NewCollector()
	tool := NewMetricsTool(c)
	params := tool.Parameters()
	if params == nil {
		t.Fatal("Parameters() returned nil")
	}
	props, ok := params["properties"].(map[string]any)
	if !ok {
		t.Fatal("parameters missing properties")
	}
	if _, ok := props["category"]; !ok {
		t.Error("parameters missing category property")
	}
}

func TestMetricsTool_Execute(t *testing.T) {
	c := metrics.NewCollector()
	tool := NewMetricsTool(c)

	// Record some data
	c.RecordToolCall("exec", 100*time.Millisecond, false)
	c.RecordLLMCall("gpt-4", 1*time.Second, false, 50, 25)

	t.Run("all", func(t *testing.T) {
		result := tool.Execute(context.Background(), map[string]any{"category": "all"})
		if result.IsError {
			t.Fatalf("unexpected error: %s", result.ForLLM)
		}
		if !strings.Contains(result.ForLLM, "LLM Metrics") {
			t.Error("result missing LLM Metrics section")
		}
		if !strings.Contains(result.ForLLM, "Tool Metrics") {
			t.Error("result missing Tool Metrics section")
		}
		if !result.Silent {
			t.Error("result should be silent")
		}
	})

	t.Run("llm", func(t *testing.T) {
		result := tool.Execute(context.Background(), map[string]any{"category": "llm"})
		if result.IsError {
			t.Fatalf("unexpected error: %s", result.ForLLM)
		}
		if !strings.Contains(result.ForLLM, "LLM Metrics") {
			t.Error("result missing LLM Metrics section")
		}
		if strings.Contains(result.ForLLM, "Tool Metrics") {
			t.Error("llm category should not include Tool Metrics")
		}
	})

	t.Run("empty_category_defaults_to_all", func(t *testing.T) {
		result := tool.Execute(context.Background(), map[string]any{})
		if result.IsError {
			t.Fatalf("unexpected error: %s", result.ForLLM)
		}
		if !strings.Contains(result.ForLLM, "LLM Metrics") || !strings.Contains(result.ForLLM, "Tool Metrics") {
			t.Error("empty category should default to all")
		}
	})

	t.Run("invalid_category_defaults_to_all", func(t *testing.T) {
		result := tool.Execute(context.Background(), map[string]any{"category": "invalid"})
		if result.IsError {
			t.Fatalf("unexpected error: %s", result.ForLLM)
		}
		if !strings.Contains(result.ForLLM, "LLM Metrics") || !strings.Contains(result.ForLLM, "Tool Metrics") {
			t.Error("invalid category should default to all")
		}
	})

	t.Run("system", func(t *testing.T) {
		result := tool.Execute(context.Background(), map[string]any{"category": "system"})
		if result.IsError {
			t.Fatalf("unexpected error: %s", result.ForLLM)
		}
		if !strings.Contains(result.ForLLM, "System Metrics") {
			t.Error("result missing System Metrics section")
		}
	})
}
