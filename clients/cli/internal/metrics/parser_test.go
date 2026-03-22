package metrics

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const sampleMetrics = `# HELP picoclaw_llm_calls_total Total number of LLM calls
# TYPE picoclaw_llm_calls_total counter
picoclaw_llm_calls_total{model="gpt-4",status="success"} 15
picoclaw_llm_calls_total{model="gpt-4",status="error"} 2
picoclaw_llm_calls_total{model="claude-3",status="success"} 8
picoclaw_llm_calls_total{model="claude-3",status="error"} 0
# HELP picoclaw_llm_duration_seconds Duration of LLM calls in seconds
# TYPE picoclaw_llm_duration_seconds histogram
picoclaw_llm_duration_seconds_bucket{model="gpt-4",le="0.005"} 0
picoclaw_llm_duration_seconds_bucket{model="gpt-4",le="0.01"} 0
picoclaw_llm_duration_seconds_bucket{model="gpt-4",le="0.025"} 0
picoclaw_llm_duration_seconds_bucket{model="gpt-4",le="0.05"} 0
picoclaw_llm_duration_seconds_bucket{model="gpt-4",le="0.1"} 0
picoclaw_llm_duration_seconds_bucket{model="gpt-4",le="0.25"} 0
picoclaw_llm_duration_seconds_bucket{model="gpt-4",le="0.5"} 0
picoclaw_llm_duration_seconds_bucket{model="gpt-4",le="1"} 5
picoclaw_llm_duration_seconds_bucket{model="gpt-4",le="2.5"} 15
picoclaw_llm_duration_seconds_bucket{model="gpt-4",le="5"} 15
picoclaw_llm_duration_seconds_bucket{model="gpt-4",le="10"} 15
picoclaw_llm_duration_seconds_bucket{model="gpt-4",le="+Inf"} 15
picoclaw_llm_duration_seconds_sum{model="gpt-4"} 18.5
picoclaw_llm_duration_seconds_count{model="gpt-4"} 15
# HELP picoclaw_llm_prompt_tokens_total Total prompt tokens consumed
# TYPE picoclaw_llm_prompt_tokens_total counter
picoclaw_llm_prompt_tokens_total{model="gpt-4"} 3000
picoclaw_llm_prompt_tokens_total{model="claude-3"} 2000
# HELP picoclaw_llm_completion_tokens_total Total completion tokens consumed
# TYPE picoclaw_llm_completion_tokens_total counter
picoclaw_llm_completion_tokens_total{model="gpt-4"} 1500
picoclaw_llm_completion_tokens_total{model="claude-3"} 800
# HELP picoclaw_tool_calls_total Total number of tool calls
# TYPE picoclaw_tool_calls_total counter
picoclaw_tool_calls_total{tool="exec",status="success"} 42
picoclaw_tool_calls_total{tool="exec",status="error"} 3
picoclaw_tool_calls_total{tool="web_fetch",status="success"} 12
picoclaw_tool_calls_total{tool="web_fetch",status="error"} 1
# HELP picoclaw_tool_duration_seconds Duration of tool executions in seconds
# TYPE picoclaw_tool_duration_seconds histogram
picoclaw_tool_duration_seconds_bucket{tool="exec",le="0.005"} 0
picoclaw_tool_duration_seconds_bucket{tool="exec",le="0.01"} 0
picoclaw_tool_duration_seconds_bucket{tool="exec",le="0.025"} 0
picoclaw_tool_duration_seconds_bucket{tool="exec",le="0.05"} 5
picoclaw_tool_duration_seconds_bucket{tool="exec",le="0.1"} 20
picoclaw_tool_duration_seconds_bucket{tool="exec",le="0.25"} 35
picoclaw_tool_duration_seconds_bucket{tool="exec",le="0.5"} 42
picoclaw_tool_duration_seconds_bucket{tool="exec",le="1"} 42
picoclaw_tool_duration_seconds_bucket{tool="exec",le="2.5"} 42
picoclaw_tool_duration_seconds_bucket{tool="exec",le="5"} 42
picoclaw_tool_duration_seconds_bucket{tool="exec",le="10"} 42
picoclaw_tool_duration_seconds_bucket{tool="exec",le="+Inf"} 42
picoclaw_tool_duration_seconds_sum{tool="exec"} 8.4
picoclaw_tool_duration_seconds_count{tool="exec"} 42
# HELP picoclaw_messages_inbound_total Total inbound messages
# TYPE picoclaw_messages_inbound_total counter
picoclaw_messages_inbound_total{channel="telegram"} 23
picoclaw_messages_inbound_total{channel="discord"} 10
# HELP picoclaw_messages_outbound_total Total outbound messages
# TYPE picoclaw_messages_outbound_total counter
picoclaw_messages_outbound_total{channel="telegram"} 20
picoclaw_messages_outbound_total{channel="discord"} 9
# HELP picoclaw_turns_total Total agent turns
# TYPE picoclaw_turns_total counter
picoclaw_turns_total{status="completed"} 18
picoclaw_turns_total{status="error"} 2
# HELP picoclaw_active_sessions Number of active sessions
# TYPE picoclaw_active_sessions gauge
picoclaw_active_sessions 3
# HELP go_goroutines Number of goroutines that currently exist.
# TYPE go_goroutines gauge
go_goroutines 45
# HELP go_memstats_heap_alloc_bytes Number of heap bytes allocated and still in use.
# TYPE go_memstats_heap_alloc_bytes gauge
go_memstats_heap_alloc_bytes 1.3002752e+07
# HELP go_memstats_sys_bytes Number of bytes obtained from system.
# TYPE go_memstats_sys_bytes gauge
go_memstats_sys_bytes 2.9456384e+07
# HELP process_resident_memory_bytes Resident memory size in bytes.
# TYPE process_resident_memory_bytes gauge
process_resident_memory_bytes 3.6896768e+07
`

func TestParseMetrics(t *testing.T) {
	snap, err := ParseMetrics(strings.NewReader(sampleMetrics))
	require.NoError(t, err)
	require.NotNil(t, snap)

	// LLM calls
	assert.Equal(t, float64(15), snap.LLMCalls["gpt-4"].Success)
	assert.Equal(t, float64(2), snap.LLMCalls["gpt-4"].Error)
	assert.Equal(t, float64(8), snap.LLMCalls["claude-3"].Success)
	assert.Equal(t, float64(0), snap.LLMCalls["claude-3"].Error)

	// LLM duration
	assert.Equal(t, uint64(15), snap.LLMDuration["gpt-4"].Count)
	assert.InDelta(t, 18.5, snap.LLMDuration["gpt-4"].Sum, 0.01)

	// LLM tokens
	assert.Equal(t, float64(3000), snap.LLMPromptTok["gpt-4"])
	assert.Equal(t, float64(2000), snap.LLMPromptTok["claude-3"])
	assert.Equal(t, float64(1500), snap.LLMCompleteTok["gpt-4"])
	assert.Equal(t, float64(800), snap.LLMCompleteTok["claude-3"])

	// Tool calls
	assert.Equal(t, float64(42), snap.ToolCalls["exec"].Success)
	assert.Equal(t, float64(3), snap.ToolCalls["exec"].Error)
	assert.Equal(t, float64(12), snap.ToolCalls["web_fetch"].Success)
	assert.Equal(t, float64(1), snap.ToolCalls["web_fetch"].Error)

	// Tool duration
	assert.Equal(t, uint64(42), snap.ToolDuration["exec"].Count)
	assert.InDelta(t, 8.4, snap.ToolDuration["exec"].Sum, 0.01)

	// Messages
	assert.Equal(t, float64(23), snap.Inbound["telegram"])
	assert.Equal(t, float64(10), snap.Inbound["discord"])
	assert.Equal(t, float64(20), snap.Outbound["telegram"])
	assert.Equal(t, float64(9), snap.Outbound["discord"])

	// Turns
	assert.Equal(t, float64(18), snap.Turns.Success)
	assert.Equal(t, float64(2), snap.Turns.Error)

	// System
	assert.Equal(t, float64(3), snap.ActiveSessions)
	assert.Equal(t, float64(45), snap.Goroutines)
	assert.InDelta(t, 1.3002752e+07, snap.HeapAlloc, 1)
	assert.InDelta(t, 2.9456384e+07, snap.SysMemory, 1)
	assert.InDelta(t, 3.6896768e+07, snap.RSS, 1)
}

func TestParseMetricsEmpty(t *testing.T) {
	snap, err := ParseMetrics(strings.NewReader(""))
	require.NoError(t, err)
	require.NotNil(t, snap)

	assert.Empty(t, snap.LLMCalls)
	assert.Empty(t, snap.ToolCalls)
	assert.Equal(t, float64(0), snap.Goroutines)
}

func TestParseMetricsPartial(t *testing.T) {
	partial := `# HELP go_goroutines Number of goroutines that currently exist.
# TYPE go_goroutines gauge
go_goroutines 12
# HELP picoclaw_active_sessions Number of active sessions
# TYPE picoclaw_active_sessions gauge
picoclaw_active_sessions 5
`
	snap, err := ParseMetrics(strings.NewReader(partial))
	require.NoError(t, err)
	assert.Equal(t, float64(12), snap.Goroutines)
	assert.Equal(t, float64(5), snap.ActiveSessions)
	assert.Empty(t, snap.LLMCalls)
	assert.Empty(t, snap.ToolCalls)
}
