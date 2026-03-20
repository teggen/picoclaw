package metrics

import (
	"strings"
	"testing"
	"time"

	dto "github.com/prometheus/client_model/go"
)

func TestNewCollector(t *testing.T) {
	c := NewCollector()
	if c == nil {
		t.Fatal("NewCollector returned nil")
	}
	if c.registry == nil {
		t.Fatal("registry is nil")
	}
}

func TestRecordToolCall(t *testing.T) {
	c := NewCollector()

	c.RecordToolCall("exec", 150*time.Millisecond, false)
	c.RecordToolCall("exec", 200*time.Millisecond, true)
	c.RecordToolCall("web_fetch", 500*time.Millisecond, false)

	families := gatherMetrics(t, c)

	assertCounter(t, families, "picoclaw_tool_calls_total", map[string]string{"tool": "exec", "status": "success"}, 1)
	assertCounter(t, families, "picoclaw_tool_calls_total", map[string]string{"tool": "exec", "status": "error"}, 1)
	assertCounter(
		t,
		families,
		"picoclaw_tool_calls_total",
		map[string]string{"tool": "web_fetch", "status": "success"},
		1,
	)

	assertHistogramCount(t, families, "picoclaw_tool_duration_seconds", map[string]string{"tool": "exec"}, 2)
}

func TestRecordLLMCall(t *testing.T) {
	c := NewCollector()

	c.RecordLLMCall("gpt-4", 2*time.Second, false, 100, 50)
	c.RecordLLMCall("gpt-4", 3*time.Second, true, 0, 0)
	c.RecordLLMCall("claude-3", 1*time.Second, false, 200, 80)

	families := gatherMetrics(t, c)

	assertCounter(t, families, "picoclaw_llm_calls_total", map[string]string{"model": "gpt-4", "status": "success"}, 1)
	assertCounter(t, families, "picoclaw_llm_calls_total", map[string]string{"model": "gpt-4", "status": "error"}, 1)
	assertCounter(t, families, "picoclaw_llm_prompt_tokens_total", map[string]string{"model": "gpt-4"}, 100)
	assertCounter(t, families, "picoclaw_llm_completion_tokens_total", map[string]string{"model": "gpt-4"}, 50)
	assertCounter(t, families, "picoclaw_llm_prompt_tokens_total", map[string]string{"model": "claude-3"}, 200)
	assertCounter(t, families, "picoclaw_llm_completion_tokens_total", map[string]string{"model": "claude-3"}, 80)
	assertHistogramCount(t, families, "picoclaw_llm_duration_seconds", map[string]string{"model": "gpt-4"}, 2)
}

func TestRecordMessages(t *testing.T) {
	c := NewCollector()

	c.RecordInboundMessage("telegram")
	c.RecordInboundMessage("telegram")
	c.RecordInboundMessage("discord")
	c.RecordOutboundMessage("telegram")

	families := gatherMetrics(t, c)

	assertCounter(t, families, "picoclaw_messages_inbound_total", map[string]string{"channel": "telegram"}, 2)
	assertCounter(t, families, "picoclaw_messages_inbound_total", map[string]string{"channel": "discord"}, 1)
	assertCounter(t, families, "picoclaw_messages_outbound_total", map[string]string{"channel": "telegram"}, 1)
}

func TestRecordTurns(t *testing.T) {
	c := NewCollector()

	c.RecordTurnCompleted()
	c.RecordTurnCompleted()
	c.RecordTurnError()

	families := gatherMetrics(t, c)

	assertCounter(t, families, "picoclaw_turns_total", map[string]string{"status": "completed"}, 2)
	assertCounter(t, families, "picoclaw_turns_total", map[string]string{"status": "error"}, 1)
}

func TestActiveSessions(t *testing.T) {
	c := NewCollector()

	c.SessionStarted()
	c.SessionStarted()
	c.SessionStarted()
	c.SessionCleared()

	families := gatherMetrics(t, c)

	assertGauge(t, families, "picoclaw_active_sessions", 2)
}

func TestSnapshot(t *testing.T) {
	c := NewCollector()

	c.RecordToolCall("exec", 100*time.Millisecond, false)
	c.RecordLLMCall("gpt-4", 1*time.Second, false, 50, 25)
	c.RecordInboundMessage("telegram")
	c.SessionStarted()

	t.Run("all", func(t *testing.T) {
		s := c.Snapshot("all")
		for _, section := range []string{"LLM Metrics", "Tool Metrics", "Message Metrics", "System Metrics"} {
			if !strings.Contains(s, section) {
				t.Errorf("snapshot missing section %q", section)
			}
		}
	})

	t.Run("llm", func(t *testing.T) {
		s := c.Snapshot("llm")
		if !strings.Contains(s, "LLM Metrics") {
			t.Error("llm snapshot missing LLM Metrics section")
		}
		if strings.Contains(s, "Tool Metrics") {
			t.Error("llm snapshot should not contain Tool Metrics")
		}
	})

	t.Run("tools", func(t *testing.T) {
		s := c.Snapshot("tools")
		if !strings.Contains(s, "Tool Metrics") {
			t.Error("tools snapshot missing Tool Metrics section")
		}
	})

	t.Run("messages", func(t *testing.T) {
		s := c.Snapshot("messages")
		if !strings.Contains(s, "Message Metrics") {
			t.Error("messages snapshot missing Message Metrics section")
		}
	})

	t.Run("system", func(t *testing.T) {
		s := c.Snapshot("system")
		if !strings.Contains(s, "System Metrics") {
			t.Error("system snapshot missing System Metrics section")
		}
	})

	t.Run("empty_defaults_to_all", func(t *testing.T) {
		s := c.Snapshot("")
		if !strings.Contains(s, "LLM Metrics") || !strings.Contains(s, "Tool Metrics") {
			t.Error("empty category should default to all")
		}
	})
}

// Helper functions

func gatherMetrics(t *testing.T, c *Collector) map[string]*dto.MetricFamily {
	t.Helper()
	families, err := c.registry.Gather()
	if err != nil {
		t.Fatalf("failed to gather metrics: %v", err)
	}
	fm := make(map[string]*dto.MetricFamily, len(families))
	for _, f := range families {
		fm[f.GetName()] = f
	}
	return fm
}

func findMetric(family *dto.MetricFamily, labels map[string]string) *dto.Metric {
	for _, m := range family.GetMetric() {
		labelMap := make(map[string]string)
		for _, l := range m.GetLabel() {
			labelMap[l.GetName()] = l.GetValue()
		}
		match := true
		for k, v := range labels {
			if labelMap[k] != v {
				match = false
				break
			}
		}
		if match {
			return m
		}
	}
	return nil
}

func assertCounter(
	t *testing.T,
	families map[string]*dto.MetricFamily,
	name string,
	labels map[string]string,
	expected float64,
) {
	t.Helper()
	f, ok := families[name]
	if !ok {
		t.Errorf("metric %q not found", name)
		return
	}
	m := findMetric(f, labels)
	if m == nil {
		t.Errorf("metric %q with labels %v not found", name, labels)
		return
	}
	got := m.GetCounter().GetValue()
	if got != expected {
		t.Errorf("metric %q labels=%v: got %.0f, want %.0f", name, labels, got, expected)
	}
}

func assertHistogramCount(
	t *testing.T,
	families map[string]*dto.MetricFamily,
	name string,
	labels map[string]string,
	expected uint64,
) {
	t.Helper()
	f, ok := families[name]
	if !ok {
		t.Errorf("metric %q not found", name)
		return
	}
	m := findMetric(f, labels)
	if m == nil {
		t.Errorf("metric %q with labels %v not found", name, labels)
		return
	}
	got := m.GetHistogram().GetSampleCount()
	if got != expected {
		t.Errorf("metric %q labels=%v histogram count: got %d, want %d", name, labels, got, expected)
	}
}

func assertGauge(t *testing.T, families map[string]*dto.MetricFamily, name string, expected float64) {
	t.Helper()
	f, ok := families[name]
	if !ok {
		t.Errorf("metric %q not found", name)
		return
	}
	if len(f.GetMetric()) == 0 {
		t.Errorf("metric %q has no values", name)
		return
	}
	got := f.GetMetric()[0].GetGauge().GetValue()
	if got != expected {
		t.Errorf("metric %q: got %.0f, want %.0f", name, got, expected)
	}
}
