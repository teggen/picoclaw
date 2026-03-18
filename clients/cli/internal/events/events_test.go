package events

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sipeed/picoclaw/clients/cli/internal/client"
)

var testUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

func TestNewEventsModel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := testUpgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		time.Sleep(200 * time.Millisecond)
	}))
	defer srv.Close()

	conn, err := client.DialEvents(srv.URL)
	require.NoError(t, err)
	defer conn.Close()

	model := newEventsModel(conn)
	assert.Empty(t, model.events)
	assert.False(t, model.quitting)
	assert.Equal(t, -1, model.cursor)
	assert.False(t, model.showDetails)
	assert.NotNil(t, model.expanded)
}

func TestEventReceivedMsg(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := testUpgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()

		event := client.EventMessage{
			Type:      "config.updated",
			Timestamp: time.Now().UnixMilli(),
		}
		data, _ := json.Marshal(event)
		conn.WriteMessage(websocket.TextMessage, data)
		time.Sleep(100 * time.Millisecond)
	}))
	defer srv.Close()

	conn, err := client.DialEvents(srv.URL)
	require.NoError(t, err)
	defer conn.Close()

	cmd := waitForEvent(conn)
	result := cmd()
	evtMsg, ok := result.(eventReceivedMsg)
	assert.True(t, ok)
	assert.Equal(t, "config.updated", evtMsg.Type)
}

func TestFormatEventDetail(t *testing.T) {
	tests := []struct {
		name string
		data any
		want string
	}{
		{
			name: "nil data",
			data: nil,
			want: "",
		},
		{
			name: "tool call completed",
			data: map[string]any{
				"tool":     "shell",
				"session":  "tg:123",
				"duration": 1.234,
				"isError":  false,
			},
			want: "tool=shell session=tg:123 duration=1.2s",
		},
		{
			name: "turn started",
			data: map[string]any{
				"session": "tg:123",
				"channel": "telegram",
				"agent":   "default",
			},
			want: "session=tg:123 channel=telegram agent=default",
		},
		{
			name: "tool call with arguments excluded from compact",
			data: map[string]any{
				"tool":      "shell",
				"session":   "tg:123",
				"arguments": `{"command":"ls -la"}`,
			},
			want: "tool=shell session=tg:123",
		},
		{
			name: "turn started with message excluded from compact",
			data: map[string]any{
				"session": "tg:123",
				"channel": "telegram",
				"agent":   "default",
				"message": "Hello world",
			},
			want: "session=tg:123 channel=telegram agent=default",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event := client.EventMessage{Data: tt.data}
			got := formatEventDetail(event)
			// Strip any ANSI escape sequences for comparison.
			assert.Contains(t, stripAnsi(got), stripAnsi(tt.want))
		})
	}
}

func TestColorizeType(t *testing.T) {
	// Just verify it doesn't panic and returns non-empty strings.
	types := []string{
		"turn.started",
		"turn.completed",
		"turn.error",
		"session.started",
		"session.cleared",
		"session.summarized",
		"tool.call.started",
		"tool.call.completed",
		"config.updated",
	}
	for _, et := range types {
		result := colorizeType(et)
		assert.NotEmpty(t, result, "colorizeType(%q) should not be empty", et)
	}
}

func TestFormatDetailBlock(t *testing.T) {
	tests := []struct {
		name      string
		event     client.EventMessage
		width     int
		wantEmpty bool
		wantKeys  []string // keys that should appear in output
	}{
		{
			name:      "nil data",
			event:     client.EventMessage{Data: nil},
			width:     80,
			wantEmpty: true,
		},
		{
			name: "tool call started with arguments",
			event: client.EventMessage{
				Type: "tool.call.started",
				Data: map[string]any{
					"session":   "tg:123",
					"tool":      "shell",
					"arguments": `{"command":"ls -la","workdir":"/home/user"}`,
				},
			},
			width:    80,
			wantKeys: []string{"arguments", "session", "tool"},
		},
		{
			name: "tool call completed with result",
			event: client.EventMessage{
				Type: "tool.call.completed",
				Data: map[string]any{
					"session":  "tg:123",
					"tool":     "shell",
					"duration": 1.5,
					"isError":  false,
					"result":   "file1.txt\nfile2.txt\nfile3.txt",
				},
			},
			width:    80,
			wantKeys: []string{"result", "session", "tool", "duration", "isError"},
		},
		{
			name: "turn started with message",
			event: client.EventMessage{
				Type: "turn.started",
				Data: map[string]any{
					"session": "tg:123",
					"channel": "telegram",
					"agent":   "default",
					"message": "Hello, how are you?",
				},
			},
			width:    80,
			wantKeys: []string{"message", "session", "channel", "agent"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatDetailBlock(tt.event, tt.width)
			stripped := stripAnsi(result)
			if tt.wantEmpty {
				assert.Empty(t, stripped)
				return
			}
			assert.NotEmpty(t, stripped)
			for _, key := range tt.wantKeys {
				assert.Contains(t, stripped, key+":")
			}
			// Verify tree characters are present.
			assert.Contains(t, stripped, "─ ")
		})
	}
}

func TestFormatDetailBlockTruncation(t *testing.T) {
	// Build a long result string.
	var longResult string
	for i := range 20 {
		longResult += "line " + string(rune('0'+i%10)) + "\n"
	}

	event := client.EventMessage{
		Type: "tool.call.completed",
		Data: map[string]any{
			"result": longResult,
		},
	}
	result := formatDetailBlock(event, 80)
	stripped := stripAnsi(result)
	assert.Contains(t, stripped, "(truncated)")
}

func TestFormatDetailBlockPrettyJSON(t *testing.T) {
	event := client.EventMessage{
		Type: "tool.call.started",
		Data: map[string]any{
			"arguments": `{"command":"ls","workdir":"/tmp"}`,
		},
	}
	result := formatDetailBlock(event, 80)
	stripped := stripAnsi(result)
	assert.Contains(t, stripped, "arguments:")
	// Should contain pretty-printed JSON with the key.
	assert.Contains(t, stripped, "command")
}

func TestEventRecord(t *testing.T) {
	event := client.EventMessage{
		Type:      "tool.call.started",
		Timestamp: time.Now().UnixMilli(),
		Data: map[string]any{
			"tool":      "shell",
			"session":   "tg:123",
			"arguments": `{"command":"echo hello"}`,
		},
	}
	compact := formatEvent(event)
	rec := eventRecord{msg: event, compact: compact}

	stripped := stripAnsi(rec.compact)
	assert.Contains(t, stripped, "tool.call.started")
	assert.Contains(t, stripped, "tool=shell")
	assert.Contains(t, stripped, "session=tg:123")
	// arguments should NOT be in the compact line.
	assert.NotContains(t, stripped, "arguments=")
}

func TestPrettyJSON(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string // substring expected in output
	}{
		{
			name:  "valid JSON",
			input: `{"key":"value"}`,
			want:  "key",
		},
		{
			name:  "invalid JSON",
			input: "not json",
			want:  "not json",
		},
		{
			name:  "empty string",
			input: "",
			want:  "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := prettyJSON(tt.input, 80)
			assert.Contains(t, got, tt.want)
		})
	}
}

func TestTruncateLines(t *testing.T) {
	input := "line1\nline2\nline3\nline4\nline5"
	result := truncateLines(input, 3, 80)
	assert.Contains(t, result, "line1")
	assert.Contains(t, result, "line3")
	assert.Contains(t, result, "(truncated)")
	assert.NotContains(t, result, "line4")
}

// stripAnsi removes ANSI escape sequences for test comparison.
func stripAnsi(s string) string {
	var out []byte
	inEsc := false
	for i := 0; i < len(s); i++ {
		if s[i] == '\x1b' {
			inEsc = true
			continue
		}
		if inEsc {
			if s[i] == 'm' {
				inEsc = false
			}
			continue
		}
		out = append(out, s[i])
	}
	return string(out)
}
