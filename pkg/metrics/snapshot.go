package metrics

import (
	"fmt"
	"strings"

	dto "github.com/prometheus/client_model/go"
)

// Snapshot returns a human-readable summary of collected metrics.
// category can be "all", "llm", "tools", "messages", or "system".
func (c *Collector) Snapshot(category string) string {
	if category == "" {
		category = "all"
	}

	families, err := c.registry.Gather()
	if err != nil {
		return fmt.Sprintf("Error gathering metrics: %v", err)
	}

	familyMap := make(map[string]*dto.MetricFamily, len(families))
	for _, f := range families {
		familyMap[f.GetName()] = f
	}

	var sb strings.Builder

	switch category {
	case "llm":
		writeLLMMetrics(&sb, familyMap)
	case "tools":
		writeToolMetrics(&sb, familyMap)
	case "messages":
		writeMessageMetrics(&sb, familyMap)
	case "system":
		writeSystemMetrics(&sb, familyMap)
	default:
		writeLLMMetrics(&sb, familyMap)
		sb.WriteString("\n")
		writeToolMetrics(&sb, familyMap)
		sb.WriteString("\n")
		writeMessageMetrics(&sb, familyMap)
		sb.WriteString("\n")
		writeSystemMetrics(&sb, familyMap)
	}

	return sb.String()
}

func writeLLMMetrics(sb *strings.Builder, fm map[string]*dto.MetricFamily) {
	sb.WriteString("=== LLM Metrics ===\n")
	writeCounterVec(sb, fm, "picoclaw_llm_calls_total", "Calls")
	writeHistogramVec(sb, fm, "picoclaw_llm_duration_seconds", "Duration")
	writeCounterVec(sb, fm, "picoclaw_llm_prompt_tokens_total", "Prompt Tokens")
	writeCounterVec(sb, fm, "picoclaw_llm_completion_tokens_total", "Completion Tokens")
}

func writeToolMetrics(sb *strings.Builder, fm map[string]*dto.MetricFamily) {
	sb.WriteString("=== Tool Metrics ===\n")
	writeCounterVec(sb, fm, "picoclaw_tool_calls_total", "Calls")
	writeHistogramVec(sb, fm, "picoclaw_tool_duration_seconds", "Duration")
}

func writeMessageMetrics(sb *strings.Builder, fm map[string]*dto.MetricFamily) {
	sb.WriteString("=== Message Metrics ===\n")
	writeCounterVec(sb, fm, "picoclaw_messages_inbound_total", "Inbound")
	writeCounterVec(sb, fm, "picoclaw_messages_outbound_total", "Outbound")
	writeCounterVec(sb, fm, "picoclaw_turns_total", "Turns")
	writeGauge(sb, fm, "picoclaw_active_sessions", "Active Sessions")
}

func writeSystemMetrics(sb *strings.Builder, fm map[string]*dto.MetricFamily) {
	sb.WriteString("=== System Metrics ===\n")
	writeGauge(sb, fm, "go_goroutines", "Goroutines")
	writeGauge(sb, fm, "go_memstats_heap_alloc_bytes", "Heap Alloc (bytes)")
	writeGauge(sb, fm, "go_memstats_sys_bytes", "Sys Memory (bytes)")
	writeGauge(sb, fm, "process_resident_memory_bytes", "RSS (bytes)")
}

func writeCounterVec(sb *strings.Builder, fm map[string]*dto.MetricFamily, name, label string) {
	f, ok := fm[name]
	if !ok || len(f.GetMetric()) == 0 {
		fmt.Fprintf(sb, "  %s: (none)\n", label)
		return
	}
	for _, m := range f.GetMetric() {
		labels := formatLabels(m.GetLabel())
		fmt.Fprintf(sb, "  %s [%s]: %.0f\n", label, labels, m.GetCounter().GetValue())
	}
}

func writeHistogramVec(sb *strings.Builder, fm map[string]*dto.MetricFamily, name, label string) {
	f, ok := fm[name]
	if !ok || len(f.GetMetric()) == 0 {
		fmt.Fprintf(sb, "  %s: (none)\n", label)
		return
	}
	for _, m := range f.GetMetric() {
		labels := formatLabels(m.GetLabel())
		h := m.GetHistogram()
		count := h.GetSampleCount()
		sum := h.GetSampleSum()
		avg := 0.0
		if count > 0 {
			avg = sum / float64(count)
		}
		fmt.Fprintf(sb, "  %s [%s]: count=%d avg=%.3fs total=%.3fs\n", label, labels, count, avg, sum)
	}
}

func writeGauge(sb *strings.Builder, fm map[string]*dto.MetricFamily, name, label string) {
	f, ok := fm[name]
	if !ok || len(f.GetMetric()) == 0 {
		fmt.Fprintf(sb, "  %s: (none)\n", label)
		return
	}
	for _, m := range f.GetMetric() {
		fmt.Fprintf(sb, "  %s: %.0f\n", label, m.GetGauge().GetValue())
	}
}

func formatLabels(labels []*dto.LabelPair) string {
	if len(labels) == 0 {
		return ""
	}
	parts := make([]string, 0, len(labels))
	for _, l := range labels {
		parts = append(parts, fmt.Sprintf("%s=%s", l.GetName(), l.GetValue()))
	}
	return strings.Join(parts, ", ")
}
