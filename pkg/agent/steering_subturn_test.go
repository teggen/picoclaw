package agent

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sipeed/picoclaw/pkg/bus"
	"github.com/sipeed/picoclaw/pkg/config"
	"github.com/sipeed/picoclaw/pkg/providers"
	"github.com/sipeed/picoclaw/pkg/tools"
)

// delayedMockProvider introduces a configurable delay per Chat call
// and tracks call count. Used for timing-sensitive integration tests.
type delayedMockProvider struct {
	delay     time.Duration
	callCount atomic.Int32
	mu        sync.Mutex
	responses []*providers.LLMResponse // sequential responses; last one is repeated
}

func (p *delayedMockProvider) Chat(
	ctx context.Context,
	messages []providers.Message,
	_ []providers.ToolDefinition,
	_ string,
	_ map[string]any,
) (*providers.LLMResponse, error) {
	idx := int(p.callCount.Add(1)) - 1

	select {
	case <-time.After(p.delay):
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.responses) == 0 {
		return &providers.LLMResponse{Content: "delayed response"}, nil
	}
	i := idx
	if i >= len(p.responses) {
		i = len(p.responses) - 1
	}
	return p.responses[i], nil
}

func (p *delayedMockProvider) GetDefaultModel() string { return "delayed-mock" }

// --- Test helpers ---

func newIntegrationAgentLoop(t *testing.T, p providers.LLMProvider) (*AgentLoop, func()) {
	t.Helper()
	tmpDir := t.TempDir()
	cfg := &config.Config{
		Agents: config.AgentsConfig{
			Defaults: config.AgentDefaults{
				Workspace:         tmpDir,
				ModelName:         "test-model",
				MaxTokens:         4096,
				MaxToolIterations: 10,
			},
		},
	}
	msgBus := bus.NewMessageBus()
	al := NewAgentLoop(cfg, msgBus, p)
	return al, func() {}
}

// ====================== 3a: Steering During Active SubTurn ======================
// Verifies that a steering message enqueued while a SubTurn is running is
// delivered to the parent on its next iteration, and the SubTurn result is
// still received.
func TestSteering_DuringActiveSubTurn(t *testing.T) {
	al, _, _, _, cleanup := newTestAgentLoop(t)
	defer cleanup()

	// Create parent turn
	parentCtx, parentCancel := context.WithCancel(context.Background())
	defer parentCancel()

	sessionKey := "parent-steering-subturn"
	parentSession := &ephemeralSessionStore{}
	parent := &turnState{
		ctx:            parentCtx,
		cancelFunc:     parentCancel,
		turnID:         sessionKey,
		sessionKey:     sessionKey,
		depth:          0,
		session:        parentSession,
		pendingResults: make(chan *tools.ToolResult, 4),
		concurrencySem: make(chan struct{}, 5),
		al:             al,
	}
	al.activeTurnStates.Store(sessionKey, parent)
	defer al.activeTurnStates.Delete(sessionKey)

	// Spawn an async SubTurn (will complete quickly via mock provider)
	cfg := SubTurnConfig{
		Model: "gpt-4o-mini",
		Tools: []tools.Tool{},
		Async: true,
	}
	_, err := spawnSubTurn(context.Background(), al, parent, cfg)
	if err != nil {
		t.Fatalf("spawnSubTurn: %v", err)
	}

	// Enqueue a steering message while SubTurn was processing
	steerMsg := providers.Message{Role: "user", Content: "change direction"}
	al.Steer(steerMsg)

	// Verify steering message is queued for the parent's scope
	msgs := al.dequeueSteeringMessagesForScopeWithFallback(sessionKey)
	if len(msgs) != 1 {
		t.Fatalf("expected 1 steering message, got %d", len(msgs))
	}
	if msgs[0].Content != "change direction" {
		t.Errorf("steering content = %q", msgs[0].Content)
	}

	// Verify SubTurn result was still delivered
	select {
	case res := <-parent.pendingResults:
		if res == nil {
			t.Error("nil SubTurn result")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for SubTurn result")
	}
}

// ====================== 3b: Hard Abort with Grandchild SubTurn ======================
// Verifies that HardAbort cascades through root → child → grandchild.
func TestHardAbort_NestedGrandchild(t *testing.T) {
	al, _, _, _, cleanup := newTestAgentLoop(t)
	defer cleanup()

	// Create root → child → grandchild hierarchy
	rootCtx, rootCancel := context.WithCancel(context.Background())
	rootTS := &turnState{
		ctx:            rootCtx,
		cancelFunc:     rootCancel,
		turnID:         "root",
		depth:          0,
		session:        &ephemeralSessionStore{},
		pendingResults: make(chan *tools.ToolResult, 4),
		concurrencySem: make(chan struct{}, 5),
		al:             al,
	}

	childCtx, childCancel := context.WithCancel(context.Background())
	childTS := &turnState{
		ctx:            childCtx,
		cancelFunc:     childCancel,
		turnID:         "child",
		depth:          1,
		session:        &ephemeralSessionStore{},
		pendingResults: make(chan *tools.ToolResult, 4),
		al:             al,
	}

	grandchildCtx, grandchildCancel := context.WithCancel(context.Background())
	grandchildTS := &turnState{
		ctx:            grandchildCtx,
		cancelFunc:     grandchildCancel,
		turnID:         "grandchild",
		depth:          2,
		session:        &ephemeralSessionStore{},
		pendingResults: make(chan *tools.ToolResult, 4),
		al:             al,
	}

	// Wire hierarchy
	rootTS.childTurnIDs = []string{"child"}
	childTS.childTurnIDs = []string{"grandchild"}

	al.activeTurnStates.Store("root", rootTS)
	al.activeTurnStates.Store("child", childTS)
	al.activeTurnStates.Store("grandchild", grandchildTS)
	defer func() {
		al.activeTurnStates.Delete("root")
		al.activeTurnStates.Delete("child")
		al.activeTurnStates.Delete("grandchild")
	}()

	// Verify all contexts are alive
	for _, ts := range []*turnState{rootTS, childTS, grandchildTS} {
		select {
		case <-ts.ctx.Done():
			t.Fatalf("%s context should not be canceled yet", ts.turnID)
		default:
		}
	}

	// Abort root
	if err := al.HardAbort("root"); err != nil {
		t.Fatalf("HardAbort: %v", err)
	}

	// All three contexts must be canceled
	for _, ts := range []*turnState{rootTS, childTS, grandchildTS} {
		select {
		case <-ts.ctx.Done():
			// expected
		default:
			t.Errorf("%s context should be canceled after HardAbort", ts.turnID)
		}
	}
}

// ====================== 3d: Concurrent Steering Across Sessions ======================
// Verifies that steering messages are correctly scoped and isolated across
// many concurrent sessions. No races should be detected with -race.
func TestSteering_ConcurrentMultiSession(t *testing.T) {
	al, _, _, _, cleanup := newTestAgentLoop(t)
	defer cleanup()

	const numSessions = 10
	const msgsPerSession = 20

	// Create turn states for each session
	for i := 0; i < numSessions; i++ {
		sessionKey := fmt.Sprintf("session-%d", i)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		ts := &turnState{
			ctx:            ctx,
			cancelFunc:     cancel,
			turnID:         sessionKey,
			depth:          0,
			session:        &ephemeralSessionStore{},
			pendingResults: make(chan *tools.ToolResult, 4),
			al:             al,
		}
		al.activeTurnStates.Store(sessionKey, ts)
		defer al.activeTurnStates.Delete(sessionKey)
	}

	// Concurrently steer each session
	var wg sync.WaitGroup
	for i := 0; i < numSessions; i++ {
		wg.Add(1)
		go func(sessionIdx int) {
			defer wg.Done()
			sessionKey := fmt.Sprintf("session-%d", sessionIdx)
			for j := 0; j < msgsPerSession; j++ {
				msg := providers.Message{
					Role:    "user",
					Content: fmt.Sprintf("steer-%d-%d", sessionIdx, j),
				}
				_ = al.enqueueSteeringMessage(sessionKey, "", msg)
			}
		}(i)
	}
	wg.Wait()

	// Switch to drain-all mode so we can read all queued messages at once.
	al.SetSteeringMode(SteeringAll)

	// Verify each session received exactly its own messages
	for i := 0; i < numSessions; i++ {
		sessionKey := fmt.Sprintf("session-%d", i)
		msgs := al.dequeueSteeringMessagesForScopeWithFallback(sessionKey)
		if len(msgs) > msgsPerSession {
			t.Errorf("session %d: expected <= %d messages, got %d",
				i, msgsPerSession, len(msgs))
		}
		for _, m := range msgs {
			prefix := fmt.Sprintf("steer-%d-", i)
			if len(m.Content) < len(prefix) || m.Content[:len(prefix)] != prefix {
				t.Errorf("session %d received wrong message: %q", i, m.Content)
			}
		}
	}
}

// ====================== 3e: Steering Message During LLM Retry ======================
// Verifies that a steering message enqueued during an LLM retry window
// is not lost and is picked up after the retry succeeds.
func TestSteering_DuringLLMRetry(t *testing.T) {
	// Provider fails once, then succeeds — simulates timeout+retry
	retryProvider := &retryOnceProvider{
		failErr:   fmt.Errorf("context deadline exceeded (timeout)"),
		finalResp: "response after retry",
	}

	al, cleanup := newIntegrationAgentLoop(t, retryProvider)
	defer cleanup()

	tool := &slowTool{name: "test_tool", duration: 10 * time.Millisecond}
	al.RegisterTool(tool)

	type result struct {
		resp string
		err  error
	}
	resultCh := make(chan result, 1)

	go func() {
		resp, err := al.ProcessDirectWithChannel(
			context.Background(),
			"do work",
			"retry-session",
			"test",
			"chat1",
		)
		resultCh <- result{resp, err}
	}()

	// Give the first attempt time to fail and retry to start
	time.Sleep(50 * time.Millisecond)

	// Enqueue a steering message during the retry window
	al.Steer(providers.Message{Role: "user", Content: "urgent update"})

	select {
	case r := <-resultCh:
		if r.err != nil {
			t.Fatalf("unexpected error: %v", r.err)
		}
		// The provider was called at least twice (first fail + retry)
		if retryProvider.callCount.Load() < 2 {
			t.Errorf("expected >= 2 provider calls, got %d", retryProvider.callCount.Load())
		}
	case <-time.After(10 * time.Second):
		t.Fatal("timeout waiting for agent loop to complete")
	}
}

// retryOnceProvider fails on the first call, then succeeds on subsequent calls.
type retryOnceProvider struct {
	callCount atomic.Int32
	failErr   error
	finalResp string
}

func (p *retryOnceProvider) Chat(
	ctx context.Context,
	_ []providers.Message,
	_ []providers.ToolDefinition,
	_ string,
	_ map[string]any,
) (*providers.LLMResponse, error) {
	n := p.callCount.Add(1)
	if n == 1 {
		return nil, p.failErr
	}
	return &providers.LLMResponse{
		Content:   p.finalResp,
		ToolCalls: []providers.ToolCall{},
	}, nil
}

func (p *retryOnceProvider) GetDefaultModel() string { return "retry-mock" }
