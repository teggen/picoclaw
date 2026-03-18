package api

import "time"

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

// newMessage creates an APIMessage with the given type and payload.
func newMessage(msgType string, payload map[string]any) APIMessage {
	return APIMessage{
		Type:      msgType,
		Timestamp: time.Now().UnixMilli(),
		Payload:   payload,
	}
}

// newError creates an error APIMessage.
func newError(code, message string) APIMessage {
	return newMessage(TypeError, map[string]any{
		"code":    code,
		"message": message,
	})
}
