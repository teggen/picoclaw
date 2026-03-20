package tools

import (
	"context"

	"github.com/sipeed/picoclaw/pkg/metrics"
)

// MetricsTool provides agents with a tool to read runtime metrics.
type MetricsTool struct {
	collector *metrics.Collector
}

// NewMetricsTool creates a new MetricsTool.
func NewMetricsTool(collector *metrics.Collector) *MetricsTool {
	return &MetricsTool{collector: collector}
}

func (t *MetricsTool) Name() string {
	return "metrics"
}

func (t *MetricsTool) Description() string {
	return "View runtime metrics for PicoClaw including LLM usage, tool calls, messages, and system stats. " +
		"Use this to monitor token consumption, tool performance, and system health."
}

func (t *MetricsTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"category": map[string]any{
				"type":        "string",
				"description": "Metrics category to display: all, llm, tools, messages, or system",
				"enum":        []string{"all", "llm", "tools", "messages", "system"},
			},
		},
	}
}

func (t *MetricsTool) Execute(_ context.Context, args map[string]any) *ToolResult {
	category, _ := args["category"].(string)
	if category == "" {
		category = "all"
	}

	snapshot := t.collector.Snapshot(category)
	return &ToolResult{
		ForLLM: snapshot,
		Silent: true,
	}
}
