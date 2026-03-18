package events

// Broadcaster is the interface for broadcasting events to subscribers.
// Implemented by api.EventHub; consumed by agent.AgentLoop.
type Broadcaster interface {
	Broadcast(eventType string, data any)
}

// Event type constants.
const (
	// Session lifecycle events.
	SessionStarted    = "session.started"
	SessionCleared    = "session.cleared"
	SessionSummarized = "session.summarized"

	// Per-turn events (one message in → one response out).
	TurnStarted   = "turn.started"
	TurnCompleted = "turn.completed"
	TurnError     = "turn.error"

	// Tool events.
	ToolCallStarted   = "tool.call.started"
	ToolCallCompleted = "tool.call.completed"

	// Config events.
	ConfigUpdated = "config.updated"
)
