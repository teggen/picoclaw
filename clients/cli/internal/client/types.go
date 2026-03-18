package client

// Protocol message types — compatible with Pico Protocol.
const (
	// Client to server.
	TypeMessageSend = "message.send"
	TypePing        = "ping"

	// Server to client.
	TypeMessageCreate = "message.create"
	TypeMessageUpdate = "message.update"
	TypeTypingStart   = "typing.start"
	TypeTypingStop    = "typing.stop"
	TypeError         = "error"
	TypePong          = "pong"
)

// APIMessage is the wire format for all API Protocol messages.
type APIMessage struct {
	Type      string         `json:"type"`
	ID        string         `json:"id,omitempty"`
	SessionID string         `json:"session_id,omitempty"`
	Timestamp int64          `json:"timestamp,omitempty"`
	Payload   map[string]any `json:"payload,omitempty"`
}

// StatusResponse is the shape returned by GET /api/v1/status.
type StatusResponse struct {
	Status   string   `json:"status"`
	Uptime   string   `json:"uptime"`
	Model    string   `json:"model"`
	Channels []string `json:"channels"`
	Tools    any      `json:"tools"`
	Agents   any      `json:"agents"`
	Skills   any      `json:"skills"`
}

// ChannelInfo is the shape returned by GET /api/v1/channels.
type ChannelInfo struct {
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`
}

// AgentInfo is the shape returned by GET /api/v1/agents.
type AgentInfo struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Model         string `json:"model"`
	MaxIterations int    `json:"max_iterations"`
	MaxTokens     int    `json:"max_tokens"`
}

// AgentDetail is the shape returned by GET /api/v1/agents/{id}.
type AgentDetail struct {
	AgentInfo
	Temperature float64  `json:"temperature"`
	Tools       []string `json:"tools"`
}

// ToolInfo is the shape returned by GET /api/v1/tools.
type ToolInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Parameters  any    `json:"parameters"`
}

// SessionListItem is the shape returned by GET /api/v1/sessions.
type SessionListItem struct {
	ID           string `json:"id"`
	Title        string `json:"title"`
	Preview      string `json:"preview"`
	MessageCount int    `json:"message_count"`
	Created      string `json:"created"`
	Updated      string `json:"updated"`
}

// SessionDetail is the shape returned by GET /api/v1/sessions/{id}.
type SessionDetail struct {
	ID       string           `json:"id"`
	Messages []SessionMessage `json:"messages"`
	Summary  string           `json:"summary"`
	Created  string           `json:"created"`
	Updated  string           `json:"updated"`
}

// SessionMessage is a single message within a session.
type SessionMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// EventMessage is the shape received from the events WebSocket.
type EventMessage struct {
	Type      string `json:"type"`
	Data      any    `json:"data"`
	Timestamp int64  `json:"timestamp"`
}
