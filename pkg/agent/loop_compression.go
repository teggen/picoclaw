// loop_compression.go contains session compression, summarization, and token estimation.
// The compressionManager type owns all compression state and methods, following the
// steeringQueue extraction pattern. AgentLoop delegates to it via al.compression.

package agent

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/sipeed/picoclaw/pkg/logger"
	"github.com/sipeed/picoclaw/pkg/providers"
)

// compressionManager encapsulates session compression, summarization, and token estimation.
type compressionManager struct {
	summarizing    sync.Map
	activeRequests *sync.WaitGroup
	emitEvent      func(EventKind, EventMeta, any)
}

func newCompressionManager(
	activeRequests *sync.WaitGroup,
	emitEvent func(EventKind, EventMeta, any),
) *compressionManager {
	return &compressionManager{
		activeRequests: activeRequests,
		emitEvent:      emitEvent,
	}
}

func (cm *compressionManager) maybeSummarize(agent *AgentInstance, sessionKey string, turnScope turnEventScope) {
	newHistory := agent.Sessions.GetHistory(sessionKey)
	tokenEstimate := cm.estimateTokens(newHistory)
	threshold := agent.ContextWindow * agent.SummarizeTokenPercent / 100

	if len(newHistory) > agent.SummarizeMessageThreshold || tokenEstimate > threshold {
		summarizeKey := agent.ID + ":" + sessionKey
		if _, loading := cm.summarizing.LoadOrStore(summarizeKey, true); !loading {
			cm.activeRequests.Add(1)
			go func() {
				defer cm.activeRequests.Done()
				defer cm.summarizing.Delete(summarizeKey)
				logger.Debug("Memory threshold reached. Optimizing conversation history...")
				cm.summarizeSession(agent, sessionKey, turnScope)
			}()
		}
	}
}

type compressionResult struct {
	DroppedMessages   int
	RemainingMessages int
}

// forceCompression aggressively reduces context when the limit is hit.
// It drops the oldest ~50% of Turns (a Turn is a complete user→LLM→response
// cycle, as defined in #1316), so tool-call sequences are never split.
//
// If the history is a single Turn with no safe split point, the function
// falls back to keeping only the most recent user message. This breaks
// Turn atomicity as a last resort to avoid a context-exceeded loop.
//
// Session history contains only user/assistant/tool messages — the system
// prompt is built dynamically by BuildMessages and is NOT stored here.
// The compression note is recorded in the session summary so that
// BuildMessages can include it in the next system prompt.
func (cm *compressionManager) forceCompression(agent *AgentInstance, sessionKey string) (compressionResult, bool) {
	history := agent.Sessions.GetHistory(sessionKey)
	if len(history) <= 2 {
		return compressionResult{}, false
	}

	// Split at a Turn boundary so no tool-call sequence is torn apart.
	// parseTurnBoundaries gives us the start of each Turn; we drop the
	// oldest half of Turns and keep the most recent ones.
	turns := parseTurnBoundaries(history)
	var mid int
	if len(turns) >= 2 {
		mid = turns[len(turns)/2]
	} else {
		// Fewer than 2 Turns — fall back to message-level midpoint
		// aligned to the nearest Turn boundary.
		mid = findSafeBoundary(history, len(history)/2)
	}
	var keptHistory []providers.Message
	if mid <= 0 {
		// No safe Turn boundary — the entire history is a single Turn
		// (e.g. one user message followed by a massive tool response).
		// Keeping everything would leave the agent stuck in a context-
		// exceeded loop, so fall back to keeping only the most recent
		// user message. This breaks Turn atomicity as a last resort.
		for i := len(history) - 1; i >= 0; i-- {
			if history[i].Role == "user" {
				keptHistory = []providers.Message{history[i]}
				break
			}
		}
	} else {
		keptHistory = history[mid:]
	}

	droppedCount := len(history) - len(keptHistory)

	// Record compression in the session summary so BuildMessages includes it
	// in the system prompt. We do not modify history messages themselves.
	existingSummary := agent.Sessions.GetSummary(sessionKey)
	compressionNote := fmt.Sprintf(
		"[Emergency compression dropped %d oldest messages due to context limit]",
		droppedCount,
	)
	if existingSummary != "" {
		compressionNote = existingSummary + "\n\n" + compressionNote
	}
	agent.Sessions.SetSummary(sessionKey, compressionNote)

	agent.Sessions.SetHistory(sessionKey, keptHistory)
	agent.Sessions.Save(sessionKey)

	logger.WarnCF("agent", "Forced compression executed", map[string]any{
		"session_key":  sessionKey,
		"dropped_msgs": droppedCount,
		"new_count":    len(keptHistory),
	})

	return compressionResult{
		DroppedMessages:   droppedCount,
		RemainingMessages: len(keptHistory),
	}, true
}

// summarizeSession summarizes the conversation history for a session.
func (cm *compressionManager) summarizeSession(agent *AgentInstance, sessionKey string, turnScope turnEventScope) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	history := agent.Sessions.GetHistory(sessionKey)
	summary := agent.Sessions.GetSummary(sessionKey)

	// Keep the most recent Turns for continuity, aligned to a Turn boundary
	// so that no tool-call sequence is split.
	if len(history) <= 4 {
		return
	}

	safeCut := findSafeBoundary(history, len(history)-4)
	if safeCut <= 0 {
		return
	}
	keepCount := len(history) - safeCut
	toSummarize := history[:safeCut]

	// Oversized Message Guard
	maxMessageTokens := agent.ContextWindow / 2
	validMessages := make([]providers.Message, 0)
	omitted := false

	for _, m := range toSummarize {
		if m.Role != "user" && m.Role != "assistant" {
			continue
		}
		msgTokens := len(m.Content) / 2
		if msgTokens > maxMessageTokens {
			omitted = true
			continue
		}
		validMessages = append(validMessages, m)
	}

	if len(validMessages) == 0 {
		return
	}

	const (
		maxSummarizationMessages = 10
		llmMaxRetries            = 3
		llmTemperature           = 0.3
		fallbackMaxContentLength = 200
	)

	// Multi-Part Summarization
	var finalSummary string
	if len(validMessages) > maxSummarizationMessages {
		mid := len(validMessages) / 2

		mid = cm.findNearestUserMessage(validMessages, mid)

		part1 := validMessages[:mid]
		part2 := validMessages[mid:]

		s1, _ := cm.summarizeBatch(ctx, agent, part1, "")
		s2, _ := cm.summarizeBatch(ctx, agent, part2, "")

		mergePrompt := fmt.Sprintf(
			"Merge these two conversation summaries into one cohesive summary:\n\n1: %s\n\n2: %s",
			s1,
			s2,
		)

		resp, err := cm.retryLLMCall(ctx, agent, mergePrompt, llmMaxRetries)
		if err == nil && resp.Content != "" {
			finalSummary = resp.Content
		} else {
			finalSummary = s1 + " " + s2
		}
	} else {
		finalSummary, _ = cm.summarizeBatch(ctx, agent, validMessages, summary)
	}

	if omitted && finalSummary != "" {
		finalSummary += "\n[Note: Some oversized messages were omitted from this summary for efficiency.]"
	}

	if finalSummary != "" {
		agent.Sessions.SetSummary(sessionKey, finalSummary)
		agent.Sessions.TruncateHistory(sessionKey, keepCount)
		agent.Sessions.Save(sessionKey)
		cm.emitEvent(
			EventKindSessionSummarize,
			turnScope.meta(0, "summarizeSession", "turn.session.summarize"),
			SessionSummarizePayload{
				SummarizedMessages: len(validMessages),
				KeptMessages:       keepCount,
				SummaryLen:         len(finalSummary),
				OmittedOversized:   omitted,
			},
		)
	}
}

// findNearestUserMessage finds the nearest user message to the given index.
// It searches backward first, then forward if no user message is found.
func (cm *compressionManager) findNearestUserMessage(messages []providers.Message, mid int) int {
	originalMid := mid

	for mid > 0 && messages[mid].Role != "user" {
		mid--
	}

	if messages[mid].Role == "user" {
		return mid
	}

	mid = originalMid
	for mid < len(messages) && messages[mid].Role != "user" {
		mid++
	}

	if mid < len(messages) {
		return mid
	}

	return originalMid
}

// retryLLMCall calls the LLM with retry logic.
func (cm *compressionManager) retryLLMCall(
	ctx context.Context,
	agent *AgentInstance,
	prompt string,
	maxRetries int,
) (*providers.LLMResponse, error) {
	const (
		llmTemperature = 0.3
	)

	var resp *providers.LLMResponse
	var err error

	for attempt := 0; attempt < maxRetries; attempt++ {
		cm.activeRequests.Add(1)
		resp, err = func() (*providers.LLMResponse, error) {
			defer cm.activeRequests.Done()
			return agent.Provider.Chat(
				ctx,
				[]providers.Message{{Role: "user", Content: prompt}},
				nil,
				agent.Model,
				map[string]any{
					"max_tokens":       agent.MaxTokens,
					"temperature":      llmTemperature,
					"prompt_cache_key": agent.ID,
				},
			)
		}()

		if err == nil && resp != nil && resp.Content != "" {
			return resp, nil
		}
		if attempt < maxRetries-1 {
			time.Sleep(time.Duration(attempt+1) * 100 * time.Millisecond)
		}
	}

	return resp, err
}

// summarizeBatch summarizes a batch of messages.
func (cm *compressionManager) summarizeBatch(
	ctx context.Context,
	agent *AgentInstance,
	batch []providers.Message,
	existingSummary string,
) (string, error) {
	const (
		llmMaxRetries             = 3
		llmTemperature            = 0.3
		fallbackMinContentLength  = 200
		fallbackMaxContentPercent = 10
	)

	var sb strings.Builder
	sb.WriteString(
		"Provide a concise summary of this conversation segment, preserving core context and key points.\n",
	)
	if existingSummary != "" {
		sb.WriteString("Existing context: ")
		sb.WriteString(existingSummary)
		sb.WriteString("\n")
	}
	sb.WriteString("\nCONVERSATION:\n")
	for _, m := range batch {
		fmt.Fprintf(&sb, "%s: %s\n", m.Role, m.Content)
	}
	prompt := sb.String()

	response, err := cm.retryLLMCall(ctx, agent, prompt, llmMaxRetries)
	if err == nil && response.Content != "" {
		return strings.TrimSpace(response.Content), nil
	}

	var fallback strings.Builder
	fallback.WriteString("Conversation summary: ")
	for i, m := range batch {
		if i > 0 {
			fallback.WriteString(" | ")
		}
		content := strings.TrimSpace(m.Content)
		runes := []rune(content)
		if len(runes) == 0 {
			fallback.WriteString(fmt.Sprintf("%s: ", m.Role))
			continue
		}

		keepLength := len(runes) * fallbackMaxContentPercent / 100
		if keepLength < fallbackMinContentLength {
			keepLength = fallbackMinContentLength
		}

		if keepLength > len(runes) {
			keepLength = len(runes)
		}

		content = string(runes[:keepLength])
		if keepLength < len(runes) {
			content += "..."
		}
		fallback.WriteString(fmt.Sprintf("%s: %s", m.Role, content))
	}
	return fallback.String(), nil
}

// estimateTokens estimates the number of tokens in a message list.
// Counts Content, ToolCalls arguments, and ToolCallID metadata so that
// tool-heavy conversations are not systematically undercounted.
func (cm *compressionManager) estimateTokens(messages []providers.Message) int {
	total := 0
	for _, m := range messages {
		total += estimateMessageTokens(m)
	}
	return total
}
