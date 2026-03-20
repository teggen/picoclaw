package metrics

import (
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
)

// DefaultCollector is the package-level collector set during gateway startup.
var DefaultCollector *Collector

// Collector holds all Prometheus metrics for PicoClaw.
type Collector struct {
	registry *prometheus.Registry
	mu       sync.RWMutex

	toolCallsTotal      *prometheus.CounterVec
	toolDuration        *prometheus.HistogramVec
	llmCallsTotal       *prometheus.CounterVec
	llmDuration         *prometheus.HistogramVec
	llmPromptTokens     *prometheus.CounterVec
	llmCompletionTokens *prometheus.CounterVec
	messagesInbound     *prometheus.CounterVec
	messagesOutbound    *prometheus.CounterVec
	turnsTotal          *prometheus.CounterVec
	activeSessions      prometheus.Gauge
}

// NewCollector creates a new Collector with all metrics registered.
func NewCollector() *Collector {
	reg := prometheus.NewRegistry()

	c := &Collector{
		registry: reg,

		toolCallsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "picoclaw_tool_calls_total",
			Help: "Total number of tool calls",
		}, []string{"tool", "status"}),

		toolDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "picoclaw_tool_duration_seconds",
			Help:    "Duration of tool executions in seconds",
			Buckets: prometheus.DefBuckets,
		}, []string{"tool"}),

		llmCallsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "picoclaw_llm_calls_total",
			Help: "Total number of LLM calls",
		}, []string{"model", "status"}),

		llmDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "picoclaw_llm_duration_seconds",
			Help:    "Duration of LLM calls in seconds",
			Buckets: prometheus.DefBuckets,
		}, []string{"model"}),

		llmPromptTokens: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "picoclaw_llm_prompt_tokens_total",
			Help: "Total prompt tokens consumed",
		}, []string{"model"}),

		llmCompletionTokens: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "picoclaw_llm_completion_tokens_total",
			Help: "Total completion tokens consumed",
		}, []string{"model"}),

		messagesInbound: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "picoclaw_messages_inbound_total",
			Help: "Total inbound messages",
		}, []string{"channel"}),

		messagesOutbound: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "picoclaw_messages_outbound_total",
			Help: "Total outbound messages",
		}, []string{"channel"}),

		turnsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "picoclaw_turns_total",
			Help: "Total agent turns",
		}, []string{"status"}),

		activeSessions: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "picoclaw_active_sessions",
			Help: "Number of active sessions",
		}),
	}

	reg.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
		c.toolCallsTotal,
		c.toolDuration,
		c.llmCallsTotal,
		c.llmDuration,
		c.llmPromptTokens,
		c.llmCompletionTokens,
		c.messagesInbound,
		c.messagesOutbound,
		c.turnsTotal,
		c.activeSessions,
	)

	return c
}

// Registry returns the underlying Prometheus registry.
func (c *Collector) Registry() *prometheus.Registry {
	return c.registry
}

// RecordToolCall records a tool execution.
func (c *Collector) RecordToolCall(tool string, duration time.Duration, isError bool) {
	status := "success"
	if isError {
		status = "error"
	}
	c.toolCallsTotal.WithLabelValues(tool, status).Inc()
	c.toolDuration.WithLabelValues(tool).Observe(duration.Seconds())
}

// RecordLLMCall records an LLM call with duration, status, and token usage.
func (c *Collector) RecordLLMCall(
	model string,
	duration time.Duration,
	isError bool,
	promptTokens, completionTokens int,
) {
	status := "success"
	if isError {
		status = "error"
	}
	c.llmCallsTotal.WithLabelValues(model, status).Inc()
	c.llmDuration.WithLabelValues(model).Observe(duration.Seconds())
	if promptTokens > 0 {
		c.llmPromptTokens.WithLabelValues(model).Add(float64(promptTokens))
	}
	if completionTokens > 0 {
		c.llmCompletionTokens.WithLabelValues(model).Add(float64(completionTokens))
	}
}

// RecordInboundMessage records an inbound message from a channel.
func (c *Collector) RecordInboundMessage(channel string) {
	c.messagesInbound.WithLabelValues(channel).Inc()
}

// RecordOutboundMessage records an outbound message to a channel.
func (c *Collector) RecordOutboundMessage(channel string) {
	c.messagesOutbound.WithLabelValues(channel).Inc()
}

// RecordTurnCompleted records a successful turn completion.
func (c *Collector) RecordTurnCompleted() {
	c.turnsTotal.WithLabelValues("completed").Inc()
}

// RecordTurnError records a turn that ended in error.
func (c *Collector) RecordTurnError() {
	c.turnsTotal.WithLabelValues("error").Inc()
}

// SessionStarted increments the active sessions gauge.
func (c *Collector) SessionStarted() {
	c.activeSessions.Inc()
}

// SessionCleared decrements the active sessions gauge.
func (c *Collector) SessionCleared() {
	c.activeSessions.Dec()
}
