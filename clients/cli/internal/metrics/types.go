package metrics

import "time"

// MetricsSnapshot holds a point-in-time view of all PicoClaw metrics.
type MetricsSnapshot struct {
	Timestamp      time.Time
	LLMCalls       map[string]CounterByStatus // key=model
	LLMDuration    map[string]HistogramData   // key=model
	LLMPromptTok   map[string]float64         // key=model
	LLMCompleteTok map[string]float64         // key=model
	ToolCalls      map[string]CounterByStatus // key=tool
	ToolDuration   map[string]HistogramData   // key=tool
	Inbound        map[string]float64         // key=channel
	Outbound       map[string]float64         // key=channel
	Turns          CounterByStatus
	ActiveSessions float64
	Goroutines     float64
	HeapAlloc      float64
	SysMemory      float64
	RSS            float64
}

// CounterByStatus holds success/error counts.
type CounterByStatus struct {
	Success float64
	Error   float64
}

// HistogramData holds histogram count and sum.
type HistogramData struct {
	Count uint64
	Sum   float64
}
