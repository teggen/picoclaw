// loop_compression_test.go contains unit tests for session compression and summarization.

package agent

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sipeed/picoclaw/pkg/providers"
	"github.com/sipeed/picoclaw/pkg/session"
)

// --- In-memory session store for testing ---

type memSessionStore struct {
	history map[string][]providers.Message
	summary map[string]string
}

func newMemSessionStore() *memSessionStore {
	return &memSessionStore{
		history: make(map[string][]providers.Message),
		summary: make(map[string]string),
	}
}

func (m *memSessionStore) AddMessage(key, role, content string) {
	m.history[key] = append(m.history[key], providers.Message{Role: role, Content: content})
}

func (m *memSessionStore) AddFullMessage(key string, msg providers.Message) {
	m.history[key] = append(m.history[key], msg)
}

func (m *memSessionStore) GetHistory(key string) []providers.Message {
	h := m.history[key]
	out := make([]providers.Message, len(h))
	copy(out, h)
	return out
}

func (m *memSessionStore) GetSummary(key string) string   { return m.summary[key] }
func (m *memSessionStore) SetSummary(key, summary string) { m.summary[key] = summary }
func (m *memSessionStore) SetHistory(key string, history []providers.Message) {
	m.history[key] = history
}

func (m *memSessionStore) TruncateHistory(key string, keepLast int) {
	h := m.history[key]
	if keepLast >= len(h) {
		return
	}
	m.history[key] = h[len(h)-keepLast:]
}

func (m *memSessionStore) Save(string) error { return nil }
func (m *memSessionStore) Close() error      { return nil }

// Compile-time check that memSessionStore implements SessionStore.
var _ session.SessionStore = (*memSessionStore)(nil)

// --- controllableProvider lets tests control LLM responses ---

type controllableProvider struct {
	responses []*providers.LLMResponse
	errors    []error
	calls     int
}

func (c *controllableProvider) Chat(
	_ context.Context,
	_ []providers.Message,
	_ []providers.ToolDefinition,
	_ string,
	_ map[string]any,
) (*providers.LLMResponse, error) {
	idx := c.calls
	c.calls++
	if idx < len(c.errors) && c.errors[idx] != nil {
		return nil, c.errors[idx]
	}
	if idx < len(c.responses) {
		return c.responses[idx], nil
	}
	return &providers.LLMResponse{Content: "default response"}, nil
}

func (c *controllableProvider) GetDefaultModel() string { return "controllable-model" }

// --- helpers ---

func makeMessages(roles ...string) []providers.Message {
	msgs := make([]providers.Message, len(roles))
	for i, r := range roles {
		msgs[i] = providers.Message{Role: r, Content: "message " + r}
	}
	return msgs
}

func testAgent(store session.SessionStore, prov providers.LLMProvider) *AgentInstance {
	return &AgentInstance{
		ID:                        "test-agent",
		Model:                     "test-model",
		MaxTokens:                 4096,
		ContextWindow:             100000,
		SummarizeMessageThreshold: 10,
		SummarizeTokenPercent:     70,
		Provider:                  prov,
		Sessions:                  store,
	}
}

// ============================================================
// estimateTokens
// ============================================================

func TestEstimateTokens(t *testing.T) {
	al, _, _, _, cleanup := newTestAgentLoop(t)
	defer cleanup()

	tests := []struct {
		name     string
		messages []providers.Message
		wantMin  int
	}{
		{
			name:     "empty messages",
			messages: nil,
			wantMin:  0,
		},
		{
			name: "single short message",
			messages: []providers.Message{
				{Role: "user", Content: "hello"},
			},
			wantMin: 1,
		},
		{
			name: "multiple messages accumulate",
			messages: []providers.Message{
				{Role: "user", Content: "hello world"},
				{Role: "assistant", Content: "hi there, how can I help?"},
			},
			wantMin: 2,
		},
		{
			name: "tool call content counted",
			messages: []providers.Message{
				{
					Role:    "assistant",
					Content: "let me check",
					ToolCalls: []providers.ToolCall{
						{
							ID:   "call_1",
							Type: "function",
							Function: &providers.FunctionCall{
								Name:      "shell",
								Arguments: `{"command":"ls -la"}`,
							},
						},
					},
				},
			},
			wantMin: 3,
		},
		{
			name: "reasoning content counted",
			messages: []providers.Message{
				{
					Role:             "assistant",
					Content:          "answer",
					ReasoningContent: strings.Repeat("think ", 100),
				},
			},
			wantMin: 10,
		},
		{
			name: "tool call without function uses name",
			messages: []providers.Message{
				{
					Role:    "assistant",
					Content: "checking",
					ToolCalls: []providers.ToolCall{
						{ID: "call_2", Name: "my_tool"},
					},
				},
			},
			wantMin: 1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := al.compression.estimateTokens(tc.messages)
			assert.GreaterOrEqual(t, got, tc.wantMin, "token estimate should be >= %d", tc.wantMin)
		})
	}

	t.Run("monotonically increases with content", func(t *testing.T) {
		short := []providers.Message{{Role: "user", Content: "hi"}}
		long := []providers.Message{{Role: "user", Content: strings.Repeat("word ", 500)}}
		assert.Greater(t, al.compression.estimateTokens(long), al.compression.estimateTokens(short))
	})
}

// ============================================================
// findNearestUserMessage
// ============================================================

func TestFindNearestUserMessage(t *testing.T) {
	al, _, _, _, cleanup := newTestAgentLoop(t)
	defer cleanup()

	tests := []struct {
		name     string
		messages []providers.Message
		mid      int
		want     int
	}{
		{
			name:     "mid is already user",
			messages: makeMessages("user", "assistant", "user", "assistant"),
			mid:      2,
			want:     2,
		},
		{
			name:     "searches backward to user",
			messages: makeMessages("user", "assistant", "assistant", "user"),
			mid:      2,
			want:     0,
		},
		{
			name:     "searches forward when backward fails",
			messages: makeMessages("assistant", "tool", "assistant", "user"),
			mid:      1,
			want:     3,
		},
		{
			name:     "returns original mid when no user found",
			messages: makeMessages("assistant", "tool", "assistant"),
			mid:      1,
			want:     1,
		},
		{
			name:     "mid at zero and is user",
			messages: makeMessages("user", "assistant"),
			mid:      0,
			want:     0,
		},
		{
			name:     "mid at last element is user",
			messages: makeMessages("assistant", "user"),
			mid:      1,
			want:     1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := al.compression.findNearestUserMessage(tc.messages, tc.mid)
			assert.Equal(t, tc.want, got)
		})
	}
}

// ============================================================
// forceCompression
// ============================================================

func TestForceCompression(t *testing.T) {
	t.Run("no compression when history too short", func(t *testing.T) {
		al, _, _, _, cleanup := newTestAgentLoop(t)
		defer cleanup()
		store := newMemSessionStore()
		agent := testAgent(store, &mockProvider{})

		// Only 2 messages — should not compress
		store.history["s1"] = makeMessages("user", "assistant")
		result, ok := al.compression.forceCompression(agent, "s1")
		assert.False(t, ok)
		assert.Equal(t, compressionResult{}, result)
	})

	t.Run("compresses longer history", func(t *testing.T) {
		al, _, _, _, cleanup := newTestAgentLoop(t)
		defer cleanup()
		store := newMemSessionStore()
		agent := testAgent(store, &mockProvider{})

		// 6 messages with user-turn boundaries
		store.history["s1"] = makeMessages("user", "assistant", "user", "assistant", "user", "assistant")
		result, ok := al.compression.forceCompression(agent, "s1")
		require.True(t, ok)
		assert.Greater(t, result.DroppedMessages, 0)
		assert.Greater(t, result.RemainingMessages, 0)
		assert.Equal(t, 6, result.DroppedMessages+result.RemainingMessages)

		// Summary should contain compression note
		summary := store.GetSummary("s1")
		assert.Contains(t, summary, "Emergency compression")
	})

	t.Run("appends to existing summary", func(t *testing.T) {
		al, _, _, _, cleanup := newTestAgentLoop(t)
		defer cleanup()
		store := newMemSessionStore()
		agent := testAgent(store, &mockProvider{})

		store.summary["s1"] = "Previous context summary"
		store.history["s1"] = makeMessages("user", "assistant", "user", "assistant", "user", "assistant")
		_, ok := al.compression.forceCompression(agent, "s1")
		require.True(t, ok)

		summary := store.GetSummary("s1")
		assert.Contains(t, summary, "Previous context summary")
		assert.Contains(t, summary, "Emergency compression")
	})

	t.Run("single turn falls back to last user message", func(t *testing.T) {
		al, _, _, _, cleanup := newTestAgentLoop(t)
		defer cleanup()
		store := newMemSessionStore()
		agent := testAgent(store, &mockProvider{})

		// Single turn: user, then many tool responses (no safe boundary)
		store.history["s1"] = []providers.Message{
			{Role: "user", Content: "do something"},
			{Role: "assistant", Content: "calling tool"},
			{Role: "tool", Content: "result 1"},
			{Role: "tool", Content: "result 2"},
			{Role: "tool", Content: "result 3"},
		}
		result, ok := al.compression.forceCompression(agent, "s1")
		require.True(t, ok)

		remaining := store.GetHistory("s1")
		// Should keep at least the user message
		require.NotEmpty(t, remaining)
		assert.Equal(t, result.RemainingMessages, len(remaining))
	})
}

// ============================================================
// maybeSummarize
// ============================================================

func TestMaybeSummarize(t *testing.T) {
	t.Run("does not trigger below threshold", func(t *testing.T) {
		al, _, _, _, cleanup := newTestAgentLoop(t)
		defer cleanup()
		store := newMemSessionStore()
		agent := testAgent(store, &mockProvider{})
		agent.SummarizeMessageThreshold = 100
		agent.SummarizeTokenPercent = 90
		agent.ContextWindow = 100000

		// Only 3 messages, well below threshold
		store.history["s1"] = makeMessages("user", "assistant", "user")
		scope := al.events.newTurnEventScope("test-agent", "s1")
		al.compression.maybeSummarize(agent, "s1", scope)

		// Should not have started summarization (sync.Map should be empty)
		_, loaded := al.compression.summarizing.Load("test-agent:s1")
		// If loaded is true, goroutine was started but should finish quickly
		// Either way, with only 3 messages, no summarization should trigger
		assert.False(t, loaded, "should not trigger summarization below threshold")
	})

	t.Run("triggers when message count exceeds threshold", func(t *testing.T) {
		al, _, _, _, cleanup := newTestAgentLoop(t)
		defer cleanup()
		store := newMemSessionStore()

		prov := &controllableProvider{
			responses: []*providers.LLMResponse{
				{Content: "summary of conversation"},
			},
		}
		agent := testAgent(store, prov)
		agent.SummarizeMessageThreshold = 3 // Low threshold
		agent.SummarizeTokenPercent = 90
		agent.ContextWindow = 100000

		// 5 messages, above threshold of 3
		store.history["s1"] = makeMessages("user", "assistant", "user", "assistant", "user")
		scope := al.events.newTurnEventScope("test-agent", "s1")
		al.compression.maybeSummarize(agent, "s1", scope)

		// The summarization runs in a goroutine; verify the key was stored
		// (or already completed). We just check it doesn't panic.
	})

	t.Run("triggers when token estimate exceeds threshold", func(t *testing.T) {
		al, _, _, _, cleanup := newTestAgentLoop(t)
		defer cleanup()
		store := newMemSessionStore()

		prov := &controllableProvider{
			responses: []*providers.LLMResponse{
				{Content: "summary"},
			},
		}
		agent := testAgent(store, prov)
		agent.SummarizeMessageThreshold = 1000 // Very high message threshold
		agent.SummarizeTokenPercent = 1        // 1% of context window
		agent.ContextWindow = 100              // Small context window => threshold = 1 token

		// Enough content to exceed the token threshold
		store.history["s1"] = []providers.Message{
			{Role: "user", Content: strings.Repeat("word ", 100)},
			{Role: "assistant", Content: strings.Repeat("reply ", 100)},
			{Role: "user", Content: strings.Repeat("more ", 100)},
			{Role: "assistant", Content: strings.Repeat("response ", 100)},
			{Role: "user", Content: strings.Repeat("again ", 100)},
		}
		scope := al.events.newTurnEventScope("test-agent", "s1")
		al.compression.maybeSummarize(agent, "s1", scope)
		// Should not panic; summarization triggered in background
	})
}

// ============================================================
// retryLLMCall
// ============================================================

func TestRetryLLMCall(t *testing.T) {
	t.Run("succeeds on first try", func(t *testing.T) {
		al, _, _, _, cleanup := newTestAgentLoop(t)
		defer cleanup()

		prov := &controllableProvider{
			responses: []*providers.LLMResponse{
				{Content: "success"},
			},
		}
		agent := testAgent(newMemSessionStore(), prov)

		resp, err := al.compression.retryLLMCall(context.Background(), agent, "test prompt", 3)
		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Equal(t, "success", resp.Content)
		assert.Equal(t, 1, prov.calls)
	})

	t.Run("retries on error and succeeds", func(t *testing.T) {
		al, _, _, _, cleanup := newTestAgentLoop(t)
		defer cleanup()

		prov := &controllableProvider{
			responses: []*providers.LLMResponse{
				nil,
				{Content: "recovered"},
			},
			errors: []error{
				errors.New("temporary failure"),
				nil,
			},
		}
		agent := testAgent(newMemSessionStore(), prov)

		resp, err := al.compression.retryLLMCall(context.Background(), agent, "test prompt", 3)
		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Equal(t, "recovered", resp.Content)
		assert.Equal(t, 2, prov.calls)
	})

	t.Run("exhausts retries and returns error", func(t *testing.T) {
		al, _, _, _, cleanup := newTestAgentLoop(t)
		defer cleanup()

		prov := &controllableProvider{
			errors: []error{
				errors.New("fail 1"),
				errors.New("fail 2"),
				errors.New("fail 3"),
			},
		}
		agent := testAgent(newMemSessionStore(), prov)

		_, err := al.compression.retryLLMCall(context.Background(), agent, "test prompt", 3)
		assert.Error(t, err)
		assert.Equal(t, 3, prov.calls)
	})

	t.Run("retries on empty content", func(t *testing.T) {
		al, _, _, _, cleanup := newTestAgentLoop(t)
		defer cleanup()

		prov := &controllableProvider{
			responses: []*providers.LLMResponse{
				{Content: ""},
				{Content: "got it"},
			},
		}
		agent := testAgent(newMemSessionStore(), prov)

		resp, err := al.compression.retryLLMCall(context.Background(), agent, "test prompt", 3)
		require.NoError(t, err)
		assert.Equal(t, "got it", resp.Content)
		assert.Equal(t, 2, prov.calls)
	})

	t.Run("single retry max", func(t *testing.T) {
		al, _, _, _, cleanup := newTestAgentLoop(t)
		defer cleanup()

		prov := &controllableProvider{
			errors: []error{errors.New("fail")},
		}
		agent := testAgent(newMemSessionStore(), prov)

		_, err := al.compression.retryLLMCall(context.Background(), agent, "prompt", 1)
		assert.Error(t, err)
		assert.Equal(t, 1, prov.calls)
	})
}

// ============================================================
// summarizeBatch
// ============================================================

func TestSummarizeBatch(t *testing.T) {
	t.Run("returns LLM summary on success", func(t *testing.T) {
		al, _, _, _, cleanup := newTestAgentLoop(t)
		defer cleanup()

		prov := &controllableProvider{
			responses: []*providers.LLMResponse{
				{Content: "  concise summary  "},
			},
		}
		agent := testAgent(newMemSessionStore(), prov)

		batch := []providers.Message{
			{Role: "user", Content: "What is Go?"},
			{Role: "assistant", Content: "Go is a programming language."},
		}

		result, err := al.compression.summarizeBatch(context.Background(), agent, batch, "")
		require.NoError(t, err)
		assert.Equal(t, "concise summary", result, "should trim whitespace")
	})

	t.Run("includes existing summary in prompt", func(t *testing.T) {
		al, _, _, _, cleanup := newTestAgentLoop(t)
		defer cleanup()

		prov := &controllableProvider{
			responses: []*providers.LLMResponse{
				{Content: "updated summary"},
			},
		}
		agent := testAgent(newMemSessionStore(), prov)

		batch := []providers.Message{
			{Role: "user", Content: "new question"},
		}

		result, err := al.compression.summarizeBatch(context.Background(), agent, batch, "prior context")
		require.NoError(t, err)
		assert.Equal(t, "updated summary", result)
		assert.Equal(t, 1, prov.calls)
	})

	t.Run("falls back to truncated content on LLM failure", func(t *testing.T) {
		al, _, _, _, cleanup := newTestAgentLoop(t)
		defer cleanup()

		prov := &controllableProvider{
			errors: []error{
				errors.New("fail"),
				errors.New("fail"),
				errors.New("fail"),
			},
		}
		agent := testAgent(newMemSessionStore(), prov)

		batch := []providers.Message{
			{Role: "user", Content: "Tell me about Go programming language"},
			{Role: "assistant", Content: "Go is great"},
		}

		result, err := al.compression.summarizeBatch(context.Background(), agent, batch, "")
		require.NoError(t, err)
		assert.Contains(t, result, "Conversation summary:")
		assert.Contains(t, result, "user:")
		assert.Contains(t, result, "assistant:")
	})

	t.Run("fallback handles empty content", func(t *testing.T) {
		al, _, _, _, cleanup := newTestAgentLoop(t)
		defer cleanup()

		prov := &controllableProvider{
			errors: []error{
				errors.New("fail"),
				errors.New("fail"),
				errors.New("fail"),
			},
		}
		agent := testAgent(newMemSessionStore(), prov)

		batch := []providers.Message{
			{Role: "user", Content: ""},
		}

		result, err := al.compression.summarizeBatch(context.Background(), agent, batch, "")
		require.NoError(t, err)
		assert.Contains(t, result, "user:")
	})

	t.Run("fallback truncates long content", func(t *testing.T) {
		al, _, _, _, cleanup := newTestAgentLoop(t)
		defer cleanup()

		prov := &controllableProvider{
			errors: []error{
				errors.New("fail"),
				errors.New("fail"),
				errors.New("fail"),
			},
		}
		agent := testAgent(newMemSessionStore(), prov)

		longContent := strings.Repeat("x", 5000)
		batch := []providers.Message{
			{Role: "user", Content: longContent},
		}

		result, err := al.compression.summarizeBatch(context.Background(), agent, batch, "")
		require.NoError(t, err)
		// Fallback keeps max(10% of length, 200) chars — so 500 chars for 5000 char content
		// The result should be shorter than the original + overhead
		assert.Less(t, len(result), len(longContent))
		assert.Contains(t, result, "...")
	})

	t.Run("multiple batch messages separated by pipe", func(t *testing.T) {
		al, _, _, _, cleanup := newTestAgentLoop(t)
		defer cleanup()

		prov := &controllableProvider{
			errors: []error{
				errors.New("fail"),
				errors.New("fail"),
				errors.New("fail"),
			},
		}
		agent := testAgent(newMemSessionStore(), prov)

		batch := []providers.Message{
			{Role: "user", Content: "first"},
			{Role: "assistant", Content: "second"},
		}

		result, err := al.compression.summarizeBatch(context.Background(), agent, batch, "")
		require.NoError(t, err)
		assert.Contains(t, result, " | ")
	})
}
