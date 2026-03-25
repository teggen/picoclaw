// loop_turn_exec_test.go contains tests for turn execution, LLM interaction, and tool dispatch.

package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sipeed/picoclaw/pkg/bus"
	"github.com/sipeed/picoclaw/pkg/channels"
	"github.com/sipeed/picoclaw/pkg/config"
	"github.com/sipeed/picoclaw/pkg/media"
	"github.com/sipeed/picoclaw/pkg/providers"
	"github.com/sipeed/picoclaw/pkg/routing"
	"github.com/sipeed/picoclaw/pkg/tools"
)

type simpleMockProvider struct {
	response string
}

func (m *simpleMockProvider) Chat(
	ctx context.Context,
	messages []providers.Message,
	tools []providers.ToolDefinition,
	model string,
	opts map[string]any,
) (*providers.LLMResponse, error) {
	return &providers.LLMResponse{
		Content:   m.response,
		ToolCalls: []providers.ToolCall{},
	}, nil
}

func (m *simpleMockProvider) GetDefaultModel() string {
	return "mock-model"
}

type reasoningContentProvider struct {
	response         string
	reasoningContent string
}

func (m *reasoningContentProvider) Chat(
	ctx context.Context,
	messages []providers.Message,
	tools []providers.ToolDefinition,
	model string,
	opts map[string]any,
) (*providers.LLMResponse, error) {
	return &providers.LLMResponse{
		Content:          m.response,
		ReasoningContent: m.reasoningContent,
		ToolCalls:        []providers.ToolCall{},
	}, nil
}

func (m *reasoningContentProvider) GetDefaultModel() string {
	return "reasoning-content-model"
}
type handledMediaProvider struct {
	calls      int
	toolCounts []int
}

func (m *handledMediaProvider) Chat(
	ctx context.Context,
	messages []providers.Message,
	tools []providers.ToolDefinition,
	model string,
	opts map[string]any,
) (*providers.LLMResponse, error) {
	m.calls++
	m.toolCounts = append(m.toolCounts, len(tools))
	if m.calls == 1 {
		return &providers.LLMResponse{
			Content: "Taking the screenshot now.",
			ToolCalls: []providers.ToolCall{{
				ID:        "call_handled_media",
				Type:      "function",
				Name:      "handled_media_tool",
				Arguments: map[string]any{},
			}},
		}, nil
	}
	return &providers.LLMResponse{}, nil
}

func (m *handledMediaProvider) GetDefaultModel() string {
	return "handled-media-model"
}

type artifactThenSendProvider struct {
	calls int
}

func (m *artifactThenSendProvider) Chat(
	ctx context.Context,
	messages []providers.Message,
	tools []providers.ToolDefinition,
	model string,
	opts map[string]any,
) (*providers.LLMResponse, error) {
	m.calls++
	if m.calls == 1 {
		return &providers.LLMResponse{
			Content: "Taking the screenshot now.",
			ToolCalls: []providers.ToolCall{{
				ID:        "call_artifact_media",
				Type:      "function",
				Name:      "media_artifact_tool",
				Arguments: map[string]any{},
			}},
		}, nil
	}

	var artifactPath string
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role != "tool" {
			continue
		}
		start := strings.Index(messages[i].Content, "[file:")
		if start < 0 {
			continue
		}
		rest := messages[i].Content[start+len("[file:"):]
		end := strings.Index(rest, "]")
		if end < 0 {
			continue
		}
		artifactPath = rest[:end]
		break
	}
	if artifactPath == "" {
		return nil, fmt.Errorf("provider did not receive artifact path in tool result")
	}

	return &providers.LLMResponse{
		Content: "",
		ToolCalls: []providers.ToolCall{{
			ID:        "call_send_file",
			Type:      "function",
			Name:      "send_file",
			Arguments: map[string]any{"path": artifactPath},
		}},
	}, nil
}

func (m *artifactThenSendProvider) GetDefaultModel() string {
	return "artifact-then-send-model"
}

type toolLimitOnlyProvider struct{}

func (m *toolLimitOnlyProvider) Chat(
	ctx context.Context,
	messages []providers.Message,
	tools []providers.ToolDefinition,
	model string,
	opts map[string]any,
) (*providers.LLMResponse, error) {
	return &providers.LLMResponse{
		ToolCalls: []providers.ToolCall{{
			ID:        "call_tool_limit_test",
			Type:      "function",
			Name:      "tool_limit_test_tool",
			Arguments: map[string]any{"value": "x"},
		}},
	}, nil
}

func (m *toolLimitOnlyProvider) GetDefaultModel() string {
	return "tool-limit-only-model"
}

type handledMediaTool struct {
	store media.MediaStore
	path  string
}

func (m *handledMediaTool) Name() string { return "handled_media_tool" }
func (m *handledMediaTool) Description() string {
	return "Returns a media attachment and fully handles the user response"
}

func (m *handledMediaTool) Parameters() map[string]any {
	return map[string]any{
		"type":       "object",
		"properties": map[string]any{},
	}
}

func (m *handledMediaTool) Execute(ctx context.Context, args map[string]any) *tools.ToolResult {
	ref, err := m.store.Store(m.path, media.MediaMeta{
		Filename:    filepath.Base(m.path),
		ContentType: "image/png",
		Source:      "test:handled_media_tool",
	}, "test:handled_media")
	if err != nil {
		return tools.ErrorResult(err.Error()).WithError(err)
	}
	return tools.MediaResult("Attachment delivered by tool.", []string{ref}).WithResponseHandled()
}


type handledMediaWithSteeringProvider struct {
	calls int
}

func (m *handledMediaWithSteeringProvider) Chat(
	ctx context.Context,
	messages []providers.Message,
	tools []providers.ToolDefinition,
	model string,
	opts map[string]any,
) (*providers.LLMResponse, error) {
	m.calls++
	if m.calls == 1 {
		return &providers.LLMResponse{
			Content: "Taking the screenshot now.",
			ToolCalls: []providers.ToolCall{{
				ID:        "call_handled_media_steering",
				Type:      "function",
				Name:      "handled_media_with_steering_tool",
				Arguments: map[string]any{},
			}},
		}, nil
	}

	for _, msg := range messages {
		if msg.Role == "user" && msg.Content == "what about this instead?" {
			return &providers.LLMResponse{Content: "Handled the queued steering message."}, nil
		}
	}

	return nil, fmt.Errorf("provider did not receive queued steering message")
}

func (m *handledMediaWithSteeringProvider) GetDefaultModel() string {
	return "handled-media-with-steering-model"
}

type handledMediaWithSteeringTool struct {
	store media.MediaStore
	path  string
	loop  *AgentLoop
}

func (m *handledMediaWithSteeringTool) Name() string { return "handled_media_with_steering_tool" }
func (m *handledMediaWithSteeringTool) Description() string {
	return "Returns handled media and enqueues a steering message during execution"
}

func (m *handledMediaWithSteeringTool) Parameters() map[string]any {
	return map[string]any{
		"type":       "object",
		"properties": map[string]any{},
	}
}

func (m *handledMediaWithSteeringTool) Execute(ctx context.Context, args map[string]any) *tools.ToolResult {
	if err := m.loop.Steer(providers.Message{Role: "user", Content: "what about this instead?"}); err != nil {
		return tools.ErrorResult(err.Error()).WithError(err)
	}

	ref, err := m.store.Store(m.path, media.MediaMeta{
		Filename:    filepath.Base(m.path),
		ContentType: "image/png",
		Source:      "test:handled_media_with_steering_tool",
	}, "test:handled_media_with_steering")
	if err != nil {
		return tools.ErrorResult(err.Error()).WithError(err)
	}
	return tools.MediaResult("Attachment delivered by tool.", []string{ref}).WithResponseHandled()
}


type mediaArtifactTool struct {
	store media.MediaStore
	path  string
}

func (m *mediaArtifactTool) Name() string { return "media_artifact_tool" }
func (m *mediaArtifactTool) Description() string {
	return "Returns a media artifact that the agent can forward or save later"
}

func (m *mediaArtifactTool) Parameters() map[string]any {
	return map[string]any{
		"type":       "object",
		"properties": map[string]any{},
	}
}

func (m *mediaArtifactTool) Execute(ctx context.Context, args map[string]any) *tools.ToolResult {
	ref, err := m.store.Store(m.path, media.MediaMeta{
		Filename:    filepath.Base(m.path),
		ContentType: "image/png",
		Source:      "test:media_artifact_tool",
	}, "test:media_artifact")
	if err != nil {
		return tools.ErrorResult(err.Error()).WithError(err)
	}
	return tools.MediaResult("Artifact created.", []string{ref})
}


type toolLimitTestTool struct{}

func (m *toolLimitTestTool) Name() string {
	return "tool_limit_test_tool"
}

func (m *toolLimitTestTool) Description() string {
	return "Tool used to exhaust the iteration budget in tests"
}

func (m *toolLimitTestTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"value": map[string]any{"type": "string"},
		},
	}
}

func (m *toolLimitTestTool) Execute(ctx context.Context, args map[string]any) *tools.ToolResult {
	return tools.SilentResult("tool limit test result")
}

// testHelper executes a message and returns the response
type testHelper struct {
	al *AgentLoop
}

func newChatCompletionTestServer(
	t *testing.T,
	label string,
	response string,
	calls *int,
	model *string,
) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Fatalf("%s server path = %q, want /chat/completions", label, r.URL.Path)
		}
		*calls = *calls + 1
		defer r.Body.Close()

		var req struct {
			Model string `json:"model"`
		}
		decodeErr := json.NewDecoder(r.Body).Decode(&req)
		if decodeErr != nil {
			t.Fatalf("decode %s request: %v", label, decodeErr)
		}
		*model = req.Model

		w.Header().Set("Content-Type", "application/json")
		encodeErr := json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{
					"message":       map[string]any{"content": response},
					"finish_reason": "stop",
				},
			},
		})
		if encodeErr != nil {
			t.Fatalf("encode %s response: %v", label, encodeErr)
		}
	}))
}

func (h testHelper) executeAndGetResponse(tb testing.TB, ctx context.Context, msg bus.InboundMessage) string {
	// Use a short timeout to avoid hanging
	timeoutCtx, cancel := context.WithTimeout(ctx, responseTimeout)
	defer cancel()

	response, err := h.al.processMessage(timeoutCtx, msg)
	if err != nil {
		tb.Fatalf("processMessage failed: %v", err)
	}
	return response
}

const responseTimeout = 3 * time.Second

func TestToolResult_SilentToolDoesNotSendUserMessage(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "agent-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

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
	provider := &simpleMockProvider{response: "File operation complete"}
	al := NewAgentLoop(cfg, msgBus, provider)
	helper := testHelper{al: al}

	// ReadFileTool returns SilentResult, which should not send user message
	ctx := context.Background()
	msg := bus.InboundMessage{
		Channel:    "test",
		SenderID:   "user1",
		ChatID:     "chat1",
		Content:    "read test.txt",
		SessionKey: "test-session",
	}

	response := helper.executeAndGetResponse(t, ctx, msg)

	// Silent tool should return the LLM's response directly
	if response != "File operation complete" {
		t.Errorf("Expected 'File operation complete', got: %s", response)
	}
}

func TestToolResult_UserFacingToolDoesSendMessage(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "agent-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

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
	provider := &simpleMockProvider{response: "Command output: hello world"}
	al := NewAgentLoop(cfg, msgBus, provider)
	helper := testHelper{al: al}

	// ExecTool returns UserResult, which should send user message
	ctx := context.Background()
	msg := bus.InboundMessage{
		Channel:    "test",
		SenderID:   "user1",
		ChatID:     "chat1",
		Content:    "run hello",
		SessionKey: "test-session",
	}

	response := helper.executeAndGetResponse(t, ctx, msg)

	// User-facing tool should include the output in final response
	if response != "Command output: hello world" {
		t.Errorf("Expected 'Command output: hello world', got: %s", response)
	}
}

// failFirstMockProvider fails on the first N calls with a specific error
type failFirstMockProvider struct {
	failures    int
	currentCall int
	failError   error
	successResp string
}

func (m *failFirstMockProvider) Chat(
	ctx context.Context,
	messages []providers.Message,
	tools []providers.ToolDefinition,
	model string,
	opts map[string]any,
) (*providers.LLMResponse, error) {
	m.currentCall++
	if m.currentCall <= m.failures {
		return nil, m.failError
	}
	return &providers.LLMResponse{
		Content:   m.successResp,
		ToolCalls: []providers.ToolCall{},
	}, nil
}

func (m *failFirstMockProvider) GetDefaultModel() string {
	return "mock-fail-model"
}

// TestAgentLoop_ContextExhaustionRetry verify that the agent retries on context errors
func TestAgentLoop_ContextExhaustionRetry(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "agent-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

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

	// Create a provider that fails once with a context error
	contextErr := fmt.Errorf("InvalidParameter: Total tokens of image and text exceed max message tokens")
	provider := &failFirstMockProvider{
		failures:    1,
		failError:   contextErr,
		successResp: "Recovered from context error",
	}

	al := NewAgentLoop(cfg, msgBus, provider)

	// Inject some history to simulate a full context.
	// Session history only stores user/assistant/tool messages — the system
	// prompt is built dynamically by BuildMessages and is NOT stored here.
	sessionKey := "test-session-context"
	history := []providers.Message{
		{Role: "user", Content: "Old message 1"},
		{Role: "assistant", Content: "Old response 1"},
		{Role: "user", Content: "Old message 2"},
		{Role: "assistant", Content: "Old response 2"},
		{Role: "user", Content: "Trigger message"},
	}
	defaultAgent := al.registry.GetDefaultAgent()
	if defaultAgent == nil {
		t.Fatal("No default agent found")
	}
	defaultAgent.Sessions.SetHistory(sessionKey, history)

	// Call ProcessDirectWithChannel
	// Note: ProcessDirectWithChannel calls processMessage which will execute runLLMIteration
	response, err := al.ProcessDirectWithChannel(
		context.Background(),
		"Trigger message",
		sessionKey,
		"test",
		"test-chat",
	)
	if err != nil {
		t.Fatalf("Expected success after retry, got error: %v", err)
	}

	if response != "Recovered from context error" {
		t.Errorf("Expected 'Recovered from context error', got '%s'", response)
	}

	// We expect 2 calls: 1st failed, 2nd succeeded
	if provider.currentCall != 2 {
		t.Errorf("Expected 2 calls (1 fail + 1 success), got %d", provider.currentCall)
	}

	// Check final history length
	finalHistory := defaultAgent.Sessions.GetHistory(sessionKey)
	// We verify that the history has been modified (compressed)
	// Original length: 5
	// Expected behavior: compression drops ~50% of Turns
	// Without compression: 5 + 1 (new user msg) + 1 (assistant msg) = 7
	if len(finalHistory) >= 7 {
		t.Errorf("Expected history to be compressed (len < 7), got %d", len(finalHistory))
	}
}

func TestAgentLoop_EmptyModelResponseUsesAccurateFallback(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "agent-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfg := &config.Config{
		Agents: config.AgentsConfig{
			Defaults: config.AgentDefaults{
				Workspace:         tmpDir,
				ModelName:         "test-model",
				MaxTokens:         4096,
				MaxToolIterations: 3,
			},
		},
	}

	msgBus := bus.NewMessageBus()
	provider := &simpleMockProvider{response: ""}
	al := NewAgentLoop(cfg, msgBus, provider)

	response, err := al.ProcessDirectWithChannel(context.Background(), "hello", "empty-response", "test", "chat1")
	if err != nil {
		t.Fatalf("ProcessDirectWithChannel failed: %v", err)
	}
	if response != defaultResponse {
		t.Fatalf("response = %q, want %q", response, defaultResponse)
	}
}

func TestAgentLoop_ToolLimitUsesDedicatedFallback(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "agent-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfg := &config.Config{
		Agents: config.AgentsConfig{
			Defaults: config.AgentDefaults{
				Workspace:         tmpDir,
				ModelName:         "test-model",
				MaxTokens:         4096,
				MaxToolIterations: 1,
			},
		},
	}

	msgBus := bus.NewMessageBus()
	provider := &toolLimitOnlyProvider{}
	al := NewAgentLoop(cfg, msgBus, provider)
	al.RegisterTool(&toolLimitTestTool{})

	response, err := al.ProcessDirectWithChannel(context.Background(), "hello", "tool-limit", "test", "chat1")
	if err != nil {
		t.Fatalf("ProcessDirectWithChannel failed: %v", err)
	}
	if response != toolLimitResponse {
		t.Fatalf("response = %q, want %q", response, toolLimitResponse)
	}

	defaultAgent := al.registry.GetDefaultAgent()
	if defaultAgent == nil {
		t.Fatal("No default agent found")
	}
	route := al.registry.ResolveRoute(routing.RouteInput{
		Channel: "test",
		Peer: &routing.RoutePeer{
			Kind: "direct",
			ID:   "cron",
		},
	})
	history := defaultAgent.Sessions.GetHistory(route.SessionKey)
	if len(history) != 4 {
		t.Fatalf("history len = %d, want 4", len(history))
	}
	assertRoles(t, history, "user", "assistant", "tool", "assistant")
	if history[3].Content != toolLimitResponse {
		t.Fatalf("final assistant content = %q, want %q", history[3].Content, toolLimitResponse)
	}
}

func TestTargetReasoningChannelID_AllChannels(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "agent-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

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

	al := NewAgentLoop(cfg, bus.NewMessageBus(), &mockProvider{})
	chManager, err := channels.NewManager(&config.Config{}, bus.NewMessageBus(), nil)
	if err != nil {
		t.Fatalf("Failed to create channel manager: %v", err)
	}
	for name, id := range map[string]string{
		"whatsapp": "rid-whatsapp",
		"telegram": "rid-telegram",
		"feishu":   "rid-feishu",
		"discord":  "rid-discord",
		"maixcam":  "rid-maixcam",
		"qq":       "rid-qq",
		"dingtalk": "rid-dingtalk",
		"slack":    "rid-slack",
		"line":     "rid-line",
		"onebot":   "rid-onebot",
		"wecom":    "rid-wecom",
	} {
		chManager.RegisterChannel(name, &fakeChannel{id: id})
	}
	al.SetChannelManager(chManager)
	tests := []struct {
		channel string
		wantID  string
	}{
		{channel: "whatsapp", wantID: "rid-whatsapp"},
		{channel: "telegram", wantID: "rid-telegram"},
		{channel: "feishu", wantID: "rid-feishu"},
		{channel: "discord", wantID: "rid-discord"},
		{channel: "maixcam", wantID: "rid-maixcam"},
		{channel: "qq", wantID: "rid-qq"},
		{channel: "dingtalk", wantID: "rid-dingtalk"},
		{channel: "slack", wantID: "rid-slack"},
		{channel: "line", wantID: "rid-line"},
		{channel: "onebot", wantID: "rid-onebot"},
		{channel: "wecom", wantID: "rid-wecom"},
		{channel: "unknown", wantID: ""},
	}

	for _, tt := range tests {
		t.Run(tt.channel, func(t *testing.T) {
			got := al.targetReasoningChannelID(tt.channel)
			if got != tt.wantID {
				t.Fatalf("targetReasoningChannelID(%q) = %q, want %q", tt.channel, got, tt.wantID)
			}
		})
	}
}

func TestHandleReasoning(t *testing.T) {
	newLoop := func(t *testing.T) (*AgentLoop, *bus.MessageBus) {
		t.Helper()
		tmpDir, err := os.MkdirTemp("", "agent-test-*")
		if err != nil {
			t.Fatalf("Failed to create temp dir: %v", err)
		}
		t.Cleanup(func() { _ = os.RemoveAll(tmpDir) })
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
		return NewAgentLoop(cfg, msgBus, &mockProvider{}), msgBus
	}

	t.Run("skips when any required field is empty", func(t *testing.T) {
		al, msgBus := newLoop(t)
		al.handleReasoning(context.Background(), "reasoning", "telegram", "")

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		for {
			select {
			case msg, ok := <-msgBus.OutboundChan():
				if !ok {
					t.Fatalf("expected no outbound message, got %+v", msg)
				}
				if msg.Content == "reasoning" {
					t.Fatalf("expected no message for empty chatID, got %+v", msg)
				}
				return
			case <-ctx.Done():
				t.Log("expected an outbound message, got none within timeout")
				return
			default:
				// Continue to check for message
				time.Sleep(5 * time.Millisecond) // Avoid busy loop
			}
		}
	})

	t.Run("publishes one message for non telegram", func(t *testing.T) {
		al, msgBus := newLoop(t)
		al.handleReasoning(context.Background(), "hello reasoning", "slack", "channel-1")

		msg, ok := <-msgBus.OutboundChan()
		if !ok {
			t.Fatal("expected an outbound message")
		}
		if msg.Channel != "slack" || msg.ChatID != "channel-1" || msg.Content != "hello reasoning" {
			t.Fatalf("unexpected outbound message: %+v", msg)
		}
	})

	t.Run("publishes one message for telegram", func(t *testing.T) {
		al, msgBus := newLoop(t)
		reasoning := "hello telegram reasoning"
		al.handleReasoning(context.Background(), reasoning, "telegram", "tg-chat")

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		for {
			select {
			case <-ctx.Done():
				t.Fatal("expected an outbound message, got none within timeout")
				return
			case msg, ok := <-msgBus.OutboundChan():
				if !ok {
					t.Fatal("expected outbound message")
				}

				if msg.Channel != "telegram" {
					t.Fatalf("expected telegram channel message, got %+v", msg)
				}
				if msg.ChatID != "tg-chat" {
					t.Fatalf("expected chatID tg-chat, got %+v", msg)
				}
				if msg.Content != reasoning {
					t.Fatalf("content mismatch: got %q want %q", msg.Content, reasoning)
				}
				return
			}
		}
	})
	t.Run("expired ctx", func(t *testing.T) {
		al, msgBus := newLoop(t)
		reasoning := "hello telegram reasoning"

		al.handleReasoning(context.Background(), reasoning, "telegram", "tg-chat")

		consumeCtx, consumeCancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer consumeCancel()

		for {
			select {
			case msg, ok := <-msgBus.OutboundChan():
				if !ok {
					t.Fatalf("expected no outbound message, but received: %+v", msg)
				}
				t.Logf("Received unexpected outbound message: %+v", msg)
				return
			case <-consumeCtx.Done():
				t.Fatalf("failed: no message received within timeout")
				return
			}
		}
	})

	t.Run("returns promptly when bus is full", func(t *testing.T) {
		al, msgBus := newLoop(t)

		// Fill the outbound bus buffer until a publish would block.
		// Use a short timeout to detect when the buffer is full,
		// rather than hardcoding the buffer size.
		for i := 0; ; i++ {
			fillCtx, fillCancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
			err := msgBus.PublishOutbound(fillCtx, bus.OutboundMessage{
				Channel: "filler",
				ChatID:  "filler",
				Content: fmt.Sprintf("filler-%d", i),
			})
			fillCancel()
			if err != nil {
				// Buffer is full (timed out trying to send).
				break
			}
		}

		// Use a short-deadline parent context to bound the test.
		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		defer cancel()

		start := time.Now()
		al.handleReasoning(ctx, "should timeout", "slack", "channel-full")
		elapsed := time.Since(start)

		// handleReasoning uses a 5s internal timeout, but the parent ctx
		// expires in 500ms. It should return within ~500ms, not 5s.
		if elapsed > 2*time.Second {
			t.Fatalf("handleReasoning blocked too long (%v); expected prompt return", elapsed)
		}

		// Drain the bus and verify the reasoning message was NOT published
		// (it should have been dropped due to timeout).
		timeer := time.After(1 * time.Second)
		for {
			select {
			case <-timeer:
				t.Logf(
					"no reasoning message received after draining bus for 1s, as expected,length=%d",
					len(msgBus.OutboundChan()),
				)
				return
			case msg, ok := <-msgBus.OutboundChan():
				if !ok {
					break
				}
				if msg.Content == "should timeout" {
					t.Fatal("expected reasoning message to be dropped when bus is full, but it was published")
				}
			}
		}
	})
}

type nativeSearchProvider struct {
	supported bool
}

func (p *nativeSearchProvider) Chat(
	ctx context.Context, msgs []providers.Message, tools []providers.ToolDefinition,
	model string, opts map[string]any,
) (*providers.LLMResponse, error) {
	return &providers.LLMResponse{Content: "ok"}, nil
}

func (p *nativeSearchProvider) GetDefaultModel() string { return "test-model" }

func (p *nativeSearchProvider) SupportsNativeSearch() bool { return p.supported }

type plainProvider struct{}

func (p *plainProvider) Chat(
	ctx context.Context, msgs []providers.Message, tools []providers.ToolDefinition,
	model string, opts map[string]any,
) (*providers.LLMResponse, error) {
	return &providers.LLMResponse{Content: "ok"}, nil
}

func (p *plainProvider) GetDefaultModel() string { return "test-model" }

func TestIsNativeSearchProvider_Supported(t *testing.T) {
	if !isNativeSearchProvider(&nativeSearchProvider{supported: true}) {
		t.Fatal("expected true for provider that supports native search")
	}
}

func TestIsNativeSearchProvider_NotSupported(t *testing.T) {
	if isNativeSearchProvider(&nativeSearchProvider{supported: false}) {
		t.Fatal("expected false for provider that does not support native search")
	}
}

func TestIsNativeSearchProvider_NoInterface(t *testing.T) {
	if isNativeSearchProvider(&plainProvider{}) {
		t.Fatal("expected false for provider that does not implement NativeSearchCapable")
	}
}

func TestFilterClientWebSearch_RemovesWebSearch(t *testing.T) {
	defs := []providers.ToolDefinition{
		{Type: "function", Function: providers.ToolFunctionDefinition{Name: "web_search"}},
		{Type: "function", Function: providers.ToolFunctionDefinition{Name: "read_file"}},
		{Type: "function", Function: providers.ToolFunctionDefinition{Name: "exec"}},
	}
	result := filterClientWebSearch(defs)
	if len(result) != 2 {
		t.Fatalf("len(result) = %d, want 2", len(result))
	}
	for _, td := range result {
		if td.Function.Name == "web_search" {
			t.Fatal("web_search should be filtered out")
		}
	}
}

func TestFilterClientWebSearch_NoWebSearch(t *testing.T) {
	defs := []providers.ToolDefinition{
		{Type: "function", Function: providers.ToolFunctionDefinition{Name: "read_file"}},
		{Type: "function", Function: providers.ToolFunctionDefinition{Name: "exec"}},
	}
	result := filterClientWebSearch(defs)
	if len(result) != 2 {
		t.Fatalf("len(result) = %d, want 2", len(result))
	}
}

func TestFilterClientWebSearch_EmptyInput(t *testing.T) {
	result := filterClientWebSearch(nil)
	if len(result) != 0 {
		t.Fatalf("len(result) = %d, want 0", len(result))
	}
}

func TestProcessMessage_MediaToolHandledSkipsFollowUpLLMAndFinalText(t *testing.T) {
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
	provider := &handledMediaProvider{}
	al := NewAgentLoop(cfg, msgBus, provider)

	store := media.NewFileMediaStore()
	al.SetMediaStore(store)
	telegramChannel := &fakeMediaChannel{fakeChannel: fakeChannel{id: "rid-telegram"}}
	al.SetChannelManager(newStartedTestChannelManager(t, msgBus, store, "telegram", telegramChannel))

	imagePath := filepath.Join(tmpDir, "screen.png")
	if err := os.WriteFile(imagePath, []byte("fake screenshot"), 0o644); err != nil {
		t.Fatalf("WriteFile(imagePath) error = %v", err)
	}

	al.RegisterTool(&handledMediaTool{
		store: store,
		path:  imagePath,
	})

	response, err := al.processMessage(context.Background(), bus.InboundMessage{
		Channel:  "telegram",
		ChatID:   "chat1",
		SenderID: "user1",
		Content:  "take a screenshot of the screen and send it to me",
	})
	if err != nil {
		t.Fatalf("processMessage() error = %v", err)
	}
	if response != "" {
		t.Fatalf("expected no final response when media tool already handled delivery, got %q", response)
	}
	if provider.calls != 1 {
		t.Fatalf("expected exactly 1 LLM call, got %d", provider.calls)
	}
	if len(provider.toolCounts) != 1 {
		t.Fatalf("expected tool counts for 1 provider call, got %d", len(provider.toolCounts))
	}
	if provider.toolCounts[0] == 0 {
		t.Fatal("expected tools to be available on the first LLM call")
	}

	if len(telegramChannel.sentMedia) != 1 {
		t.Fatalf("expected exactly 1 synchronously sent media message, got %d", len(telegramChannel.sentMedia))
	}
	if telegramChannel.sentMedia[0].Channel != "telegram" || telegramChannel.sentMedia[0].ChatID != "chat1" {
		t.Fatalf("unexpected sent media target: %+v", telegramChannel.sentMedia[0])
	}
	if len(telegramChannel.sentMedia[0].Parts) != 1 {
		t.Fatalf("expected exactly 1 sent media part, got %d", len(telegramChannel.sentMedia[0].Parts))
	}

	select {
	case extra := <-msgBus.OutboundMediaChan():
		t.Fatalf("expected handled media to bypass async queue, got %+v", extra)
	default:
	}

	defaultAgent := al.GetRegistry().GetDefaultAgent()
	if defaultAgent == nil {
		t.Fatal("expected default agent")
	}
	route, _, err := al.resolveMessageRoute(bus.InboundMessage{
		Channel:  "telegram",
		ChatID:   "chat1",
		SenderID: "user1",
		Content:  "take a screenshot of the screen and send it to me",
	})
	if err != nil {
		t.Fatalf("resolveMessageRoute() error = %v", err)
	}
	sessionKey := resolveScopeKey(route, "")
	history := defaultAgent.Sessions.GetHistory(sessionKey)
	if len(history) == 0 {
		t.Fatal("expected session history to be saved")
	}
	last := history[len(history)-1]
	if last.Role != "assistant" || last.Content != "Requested output delivered via tool attachment." {
		t.Fatalf("expected handled assistant summary in history, got %+v", last)
	}
}

func TestProcessMessage_HandledToolProcessesQueuedSteeringBeforeReturning(t *testing.T) {
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
	provider := &handledMediaWithSteeringProvider{}
	al := NewAgentLoop(cfg, msgBus, provider)

	store := media.NewFileMediaStore()
	al.SetMediaStore(store)
	telegramChannel := &fakeMediaChannel{fakeChannel: fakeChannel{id: "rid-telegram"}}
	al.SetChannelManager(newStartedTestChannelManager(t, msgBus, store, "telegram", telegramChannel))

	imagePath := filepath.Join(tmpDir, "screen-steering.png")
	if err := os.WriteFile(imagePath, []byte("fake screenshot"), 0o644); err != nil {
		t.Fatalf("WriteFile(imagePath) error = %v", err)
	}

	al.RegisterTool(&handledMediaWithSteeringTool{
		store: store,
		path:  imagePath,
		loop:  al,
	})

	response, err := al.processMessage(context.Background(), bus.InboundMessage{
		Channel:  "telegram",
		ChatID:   "chat1",
		SenderID: "user1",
		Content:  "take a screenshot of the screen and send it to me",
	})
	if err != nil {
		t.Fatalf("processMessage() error = %v", err)
	}
	if response != "Handled the queued steering message." {
		t.Fatalf("response = %q, want queued steering response", response)
	}
	if provider.calls != 2 {
		t.Fatalf("expected 2 LLM calls after queued steering, got %d", provider.calls)
	}
	if len(telegramChannel.sentMedia) != 1 {
		t.Fatalf("expected exactly 1 synchronously sent media message, got %d", len(telegramChannel.sentMedia))
	}
}

func TestProcessMessage_MediaArtifactCanBeForwardedBySendFile(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := config.DefaultConfig()
	cfg.Agents.Defaults.Workspace = tmpDir
	cfg.Agents.Defaults.ModelName = "test-model"
	cfg.Agents.Defaults.MaxTokens = 4096
	cfg.Agents.Defaults.MaxToolIterations = 10

	msgBus := bus.NewMessageBus()
	provider := &artifactThenSendProvider{}
	al := NewAgentLoop(cfg, msgBus, provider)

	store := media.NewFileMediaStore()
	al.SetMediaStore(store)
	telegramChannel := &fakeMediaChannel{fakeChannel: fakeChannel{id: "rid-telegram"}}
	al.SetChannelManager(newStartedTestChannelManager(t, msgBus, store, "telegram", telegramChannel))

	mediaDir := media.TempDir()
	if err := os.MkdirAll(mediaDir, 0o700); err != nil {
		t.Fatalf("MkdirAll(mediaDir) error = %v", err)
	}
	imagePath := filepath.Join(mediaDir, "artifact-screen.png")
	if err := os.WriteFile(imagePath, []byte("fake screenshot"), 0o644); err != nil {
		t.Fatalf("WriteFile(imagePath) error = %v", err)
	}

	al.RegisterTool(&mediaArtifactTool{
		store: store,
		path:  imagePath,
	})

	response, err := al.processMessage(context.Background(), bus.InboundMessage{
		Channel:  "telegram",
		ChatID:   "chat1",
		SenderID: "user1",
		Content:  "take a screenshot of the screen and send it to me",
	})
	if err != nil {
		t.Fatalf("processMessage() error = %v", err)
	}
	if response != "" {
		t.Fatalf("expected no final response after send_file handled delivery, got %q", response)
	}
	if provider.calls != 2 {
		t.Fatalf("expected 2 LLM calls (artifact + send_file), got %d", provider.calls)
	}

	if len(telegramChannel.sentMedia) != 1 {
		t.Fatalf("expected exactly 1 synchronously sent media message, got %d", len(telegramChannel.sentMedia))
	}
	if telegramChannel.sentMedia[0].Channel != "telegram" || telegramChannel.sentMedia[0].ChatID != "chat1" {
		t.Fatalf("unexpected sent media target: %+v", telegramChannel.sentMedia[0])
	}
	if len(telegramChannel.sentMedia[0].Parts) != 1 {
		t.Fatalf("expected exactly 1 sent media part, got %d", len(telegramChannel.sentMedia[0].Parts))
	}

	select {
	case extra := <-msgBus.OutboundMediaChan():
		t.Fatalf("expected synchronous send_file delivery to bypass async queue, got %+v", extra)
	default:
	}
}

func TestProcessMessage_PublishesReasoningContentToReasoningChannel(t *testing.T) {
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
	provider := &reasoningContentProvider{
		response:         "final answer",
		reasoningContent: "thinking trace",
	}
	al := NewAgentLoop(cfg, msgBus, provider)

	chManager, err := channels.NewManager(&config.Config{}, msgBus, nil)
	if err != nil {
		t.Fatalf("Failed to create channel manager: %v", err)
	}
	chManager.RegisterChannel("telegram", &fakeChannel{id: "reason-chat"})
	al.SetChannelManager(chManager)

	response, err := al.processMessage(context.Background(), bus.InboundMessage{
		Channel:  "telegram",
		SenderID: "user1",
		ChatID:   "chat1",
		Content:  "hello",
	})
	if err != nil {
		t.Fatalf("processMessage() error = %v", err)
	}
	if response != "final answer" {
		t.Fatalf("processMessage() response = %q, want %q", response, "final answer")
	}

	select {
	case outbound := <-msgBus.OutboundChan():
		if outbound.Channel != "telegram" {
			t.Fatalf("reasoning channel = %q, want %q", outbound.Channel, "telegram")
		}
		if outbound.ChatID != "reason-chat" {
			t.Fatalf("reasoning chatID = %q, want %q", outbound.ChatID, "reason-chat")
		}
		if outbound.Content != "thinking trace" {
			t.Fatalf("reasoning content = %q, want %q", outbound.Content, "thinking trace")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("expected reasoning content to be published to reasoning channel")
	}
}
