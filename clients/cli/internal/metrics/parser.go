package metrics

import (
	"io"
	"time"

	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
	prommodel "github.com/prometheus/common/model"
)

// ParseMetrics parses Prometheus text format into a MetricsSnapshot.
func ParseMetrics(r io.Reader) (*MetricsSnapshot, error) {
	parser := expfmt.NewTextParser(prommodel.LegacyValidation)
	families, err := parser.TextToMetricFamilies(r)
	if err != nil {
		return nil, err
	}

	snap := &MetricsSnapshot{
		Timestamp:      time.Now(),
		LLMCalls:       make(map[string]CounterByStatus),
		LLMDuration:    make(map[string]HistogramData),
		LLMPromptTok:   make(map[string]float64),
		LLMCompleteTok: make(map[string]float64),
		ToolCalls:      make(map[string]CounterByStatus),
		ToolDuration:   make(map[string]HistogramData),
		Inbound:        make(map[string]float64),
		Outbound:       make(map[string]float64),
	}

	for name, family := range families {
		switch name {
		case "picoclaw_llm_calls_total":
			extractCounterByStatus(family, "model", snap.LLMCalls)
		case "picoclaw_llm_duration_seconds":
			extractHistogram(family, "model", snap.LLMDuration)
		case "picoclaw_llm_prompt_tokens_total":
			extractCounterByLabel(family, "model", snap.LLMPromptTok)
		case "picoclaw_llm_completion_tokens_total":
			extractCounterByLabel(family, "model", snap.LLMCompleteTok)
		case "picoclaw_tool_calls_total":
			extractCounterByStatus(family, "tool", snap.ToolCalls)
		case "picoclaw_tool_duration_seconds":
			extractHistogram(family, "tool", snap.ToolDuration)
		case "picoclaw_messages_inbound_total":
			extractCounterByLabel(family, "channel", snap.Inbound)
		case "picoclaw_messages_outbound_total":
			extractCounterByLabel(family, "channel", snap.Outbound)
		case "picoclaw_turns_total":
			extractTurns(family, &snap.Turns)
		case "picoclaw_active_sessions":
			snap.ActiveSessions = getGaugeValue(family)
		case "go_goroutines":
			snap.Goroutines = getGaugeValue(family)
		case "go_memstats_heap_alloc_bytes":
			snap.HeapAlloc = getGaugeValue(family)
		case "go_memstats_sys_bytes":
			snap.SysMemory = getGaugeValue(family)
		case "process_resident_memory_bytes":
			snap.RSS = getGaugeValue(family)
		}
	}

	return snap, nil
}

func getLabelValue(m *dto.Metric, name string) string {
	for _, lp := range m.GetLabel() {
		if lp.GetName() == name {
			return lp.GetValue()
		}
	}
	return ""
}

func extractCounterByStatus(family *dto.MetricFamily, keyLabel string, dest map[string]CounterByStatus) {
	for _, m := range family.GetMetric() {
		key := getLabelValue(m, keyLabel)
		status := getLabelValue(m, "status")
		val := m.GetCounter().GetValue()
		entry := dest[key]
		switch status {
		case "success":
			entry.Success = val
		case "error":
			entry.Error = val
		case "completed":
			entry.Success = val
		}
		dest[key] = entry
	}
}

func extractCounterByLabel(family *dto.MetricFamily, keyLabel string, dest map[string]float64) {
	for _, m := range family.GetMetric() {
		key := getLabelValue(m, keyLabel)
		dest[key] = m.GetCounter().GetValue()
	}
}

func extractHistogram(family *dto.MetricFamily, keyLabel string, dest map[string]HistogramData) {
	for _, m := range family.GetMetric() {
		key := getLabelValue(m, keyLabel)
		h := m.GetHistogram()
		dest[key] = HistogramData{
			Count: h.GetSampleCount(),
			Sum:   h.GetSampleSum(),
		}
	}
}

func extractTurns(family *dto.MetricFamily, turns *CounterByStatus) {
	for _, m := range family.GetMetric() {
		status := getLabelValue(m, "status")
		val := m.GetCounter().GetValue()
		switch status {
		case "completed":
			turns.Success = val
		case "error":
			turns.Error = val
		}
	}
}

func getGaugeValue(family *dto.MetricFamily) float64 {
	metrics := family.GetMetric()
	if len(metrics) == 0 {
		return 0
	}
	return metrics[0].GetGauge().GetValue()
}
