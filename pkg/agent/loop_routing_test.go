// loop_routing_test.go contains tests for message routing and command dispatch.

package agent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sipeed/picoclaw/pkg/bus"
	"github.com/sipeed/picoclaw/pkg/config"
	"github.com/sipeed/picoclaw/pkg/providers"
	"github.com/sipeed/picoclaw/pkg/routing"
)

func TestProcessMessage_IncludesCurrentSenderInDynamicContext(t *testing.T) {
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
	provider := &recordingProvider{}
	al := NewAgentLoop(cfg, msgBus, provider)

	response, err := al.processMessage(context.Background(), bus.InboundMessage{
		Channel:  "discord",
		SenderID: "discord:123",
		Sender: bus.SenderInfo{
			DisplayName: "Alice",
		},
		ChatID:  "group-1",
		Content: "hello",
	})
	if err != nil {
		t.Fatalf("processMessage() error = %v", err)
	}
	if response != "Mock response" {
		t.Fatalf("processMessage() response = %q, want %q", response, "Mock response")
	}
	if len(provider.lastMessages) == 0 {
		t.Fatal("provider did not receive any messages")
	}

	systemPrompt := provider.lastMessages[0].Content
	wantSender := "## Current Sender\nCurrent sender: Alice (ID: discord:123)"
	if !strings.Contains(systemPrompt, wantSender) {
		t.Fatalf("system prompt missing sender context %q:\n%s", wantSender, systemPrompt)
	}

	lastMessage := provider.lastMessages[len(provider.lastMessages)-1]
	if lastMessage.Role != "user" || lastMessage.Content != "hello" {
		t.Fatalf("last provider message = %+v, want unchanged user message", lastMessage)
	}
}

func TestProcessMessage_UseCommandLoadsRequestedSkill(t *testing.T) {
	tmpDir := t.TempDir()
	skillDir := filepath.Join(tmpDir, "skills", "shell")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("mkdir skill dir: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(skillDir, "SKILL.md"),
		[]byte("# shell\n\nPrefer concise shell commands and explain them briefly."),
		0o644,
	); err != nil {
		t.Fatalf("write skill file: %v", err)
	}

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
	provider := &recordingProvider{}
	al := NewAgentLoop(cfg, msgBus, provider)

	response, err := al.processMessage(context.Background(), bus.InboundMessage{
		Channel:  "telegram",
		SenderID: "telegram:123",
		ChatID:   "chat-1",
		Content:  "/use shell explain how to list files",
	})
	if err != nil {
		t.Fatalf("processMessage() error = %v", err)
	}
	if response != "Mock response" {
		t.Fatalf("processMessage() response = %q, want %q", response, "Mock response")
	}
	if len(provider.lastMessages) == 0 {
		t.Fatal("provider did not receive any messages")
	}

	systemPrompt := provider.lastMessages[0].Content
	if !strings.Contains(systemPrompt, "# Active Skills") {
		t.Fatalf("system prompt missing active skills section:\n%s", systemPrompt)
	}
	if !strings.Contains(systemPrompt, "### Skill: shell") {
		t.Fatalf("system prompt missing requested skill content:\n%s", systemPrompt)
	}

	lastMessage := provider.lastMessages[len(provider.lastMessages)-1]
	if lastMessage.Role != "user" || lastMessage.Content != "explain how to list files" {
		t.Fatalf("last provider message = %+v, want rewritten user message", lastMessage)
	}
}

func TestHandleCommand_UseCommandRejectsUnknownSkill(t *testing.T) {
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
	provider := &recordingProvider{}
	al := NewAgentLoop(cfg, msgBus, provider)
	agent := al.GetRegistry().GetDefaultAgent()

	opts := processOptions{}
	reply, handled := al.handleCommand(context.Background(), bus.InboundMessage{
		Channel:  "telegram",
		SenderID: "telegram:123",
		ChatID:   "chat-1",
		Content:  "/use missing explain how to list files",
	}, agent, &opts)
	if !handled {
		t.Fatal("expected /use with unknown skill to be handled")
	}
	if !strings.Contains(reply, "Unknown skill: missing") {
		t.Fatalf("reply = %q, want unknown skill error", reply)
	}
}

func TestProcessMessage_UseCommandArmsSkillForNextMessage(t *testing.T) {
	tmpDir := t.TempDir()
	skillDir := filepath.Join(tmpDir, "skills", "shell")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("mkdir skill dir: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(skillDir, "SKILL.md"),
		[]byte("# shell\n\nPrefer concise shell commands and explain them briefly."),
		0o644,
	); err != nil {
		t.Fatalf("write skill file: %v", err)
	}

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
	provider := &recordingProvider{}
	al := NewAgentLoop(cfg, msgBus, provider)

	response, err := al.processMessage(context.Background(), bus.InboundMessage{
		Channel:  "telegram",
		SenderID: "telegram:123",
		ChatID:   "chat-1",
		Content:  "/use shell",
	})
	if err != nil {
		t.Fatalf("processMessage() arm error = %v", err)
	}
	if !strings.Contains(response, `Skill "shell" is armed for your next message.`) {
		t.Fatalf("arm response = %q, want armed confirmation", response)
	}

	response, err = al.processMessage(context.Background(), bus.InboundMessage{
		Channel:  "telegram",
		SenderID: "telegram:123",
		ChatID:   "chat-1",
		Content:  "explain how to list files",
	})
	if err != nil {
		t.Fatalf("processMessage() follow-up error = %v", err)
	}
	if response != "Mock response" {
		t.Fatalf("follow-up response = %q, want %q", response, "Mock response")
	}
	if len(provider.lastMessages) == 0 {
		t.Fatal("provider did not receive any messages")
	}

	systemPrompt := provider.lastMessages[0].Content
	if !strings.Contains(systemPrompt, "### Skill: shell") {
		t.Fatalf("system prompt missing pending skill content:\n%s", systemPrompt)
	}
	lastMessage := provider.lastMessages[len(provider.lastMessages)-1]
	if lastMessage.Role != "user" || lastMessage.Content != "explain how to list files" {
		t.Fatalf("last provider message = %+v, want unchanged follow-up user message", lastMessage)
	}
}

func TestApplyExplicitSkillCommand_ArmsSkillForNextMessage(t *testing.T) {
	al, cfg, _, _, cleanup := newTestAgentLoop(t)
	defer cleanup()

	if err := os.MkdirAll(filepath.Join(cfg.Agents.Defaults.Workspace, "skills", "finance-news"), 0o755); err != nil {
		t.Fatalf("MkdirAll(skill) error = %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(cfg.Agents.Defaults.Workspace, "skills", "finance-news", "SKILL.md"),
		[]byte("# Finance News\n\nUse web tools for current finance updates.\n"),
		0o644,
	); err != nil {
		t.Fatalf("WriteFile(SKILL.md) error = %v", err)
	}

	agent := al.GetRegistry().GetDefaultAgent()
	if agent == nil {
		t.Fatal("expected default agent")
	}

	opts := &processOptions{SessionKey: "agent:main:test"}
	matched, handled, reply := al.applyExplicitSkillCommand("/use finance-news", agent, opts)
	if !matched {
		t.Fatal("expected /use command to match")
	}
	if !handled {
		t.Fatal("expected /use without inline message to be handled immediately")
	}
	if !strings.Contains(reply, `Skill "finance-news" is armed for your next message`) {
		t.Fatalf("unexpected reply: %q", reply)
	}

	pending := al.takePendingSkills(opts.SessionKey)
	if len(pending) != 1 || pending[0] != "finance-news" {
		t.Fatalf("pending skills = %#v, want [finance-news]", pending)
	}
}

func TestApplyExplicitSkillCommand_InlineMessageMutatesOptions(t *testing.T) {
	al, cfg, _, _, cleanup := newTestAgentLoop(t)
	defer cleanup()

	if err := os.MkdirAll(filepath.Join(cfg.Agents.Defaults.Workspace, "skills", "finance-news"), 0o755); err != nil {
		t.Fatalf("MkdirAll(skill) error = %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(cfg.Agents.Defaults.Workspace, "skills", "finance-news", "SKILL.md"),
		[]byte("# Finance News\n\nUse web tools for current finance updates.\n"),
		0o644,
	); err != nil {
		t.Fatalf("WriteFile(SKILL.md) error = %v", err)
	}

	agent := al.GetRegistry().GetDefaultAgent()
	if agent == nil {
		t.Fatal("expected default agent")
	}

	opts := &processOptions{
		SessionKey:  "agent:main:test",
		UserMessage: "/use finance-news dammi le ultime news",
	}
	matched, handled, reply := al.applyExplicitSkillCommand(opts.UserMessage, agent, opts)
	if !matched {
		t.Fatal("expected /use command to match")
	}
	if handled {
		t.Fatal("expected /use with inline message to fall through into normal agent execution")
	}
	if reply != "" {
		t.Fatalf("unexpected reply: %q", reply)
	}
	if opts.UserMessage != "dammi le ultime news" {
		t.Fatalf("opts.UserMessage = %q, want %q", opts.UserMessage, "dammi le ultime news")
	}
	if len(opts.ForcedSkills) != 1 || opts.ForcedSkills[0] != "finance-news" {
		t.Fatalf("opts.ForcedSkills = %#v, want [finance-news]", opts.ForcedSkills)
	}
}




// TestAgentLoop_GetStartupInfo verifies startup info contains tools
func TestProcessMessage_UsesRouteSessionKey(t *testing.T) {
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
	provider := &simpleMockProvider{response: "ok"}
	al := NewAgentLoop(cfg, msgBus, provider)

	msg := bus.InboundMessage{
		Channel:  "telegram",
		SenderID: "user1",
		ChatID:   "chat1",
		Content:  "hello",
		Peer: bus.Peer{
			Kind: "direct",
			ID:   "user1",
		},
	}

	route := al.registry.ResolveRoute(routing.RouteInput{
		Channel: msg.Channel,
		Peer:    extractPeer(msg),
	})
	sessionKey := route.SessionKey

	defaultAgent := al.registry.GetDefaultAgent()
	if defaultAgent == nil {
		t.Fatal("No default agent found")
	}

	helper := testHelper{al: al}
	_ = helper.executeAndGetResponse(t, context.Background(), msg)

	history := defaultAgent.Sessions.GetHistory(sessionKey)
	if len(history) != 2 {
		t.Fatalf("expected session history len=2, got %d", len(history))
	}
	if history[0].Role != "user" || history[0].Content != "hello" {
		t.Fatalf("unexpected first message in session: %+v", history[0])
	}
}

func TestProcessMessage_CommandOutcomes(t *testing.T) {
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
		Session: config.SessionConfig{
			DMScope: "per-channel-peer",
		},
	}

	msgBus := bus.NewMessageBus()
	provider := &countingMockProvider{response: "LLM reply"}
	al := NewAgentLoop(cfg, msgBus, provider)
	helper := testHelper{al: al}

	baseMsg := bus.InboundMessage{
		Channel:  "whatsapp",
		SenderID: "user1",
		ChatID:   "chat1",
		Peer: bus.Peer{
			Kind: "direct",
			ID:   "user1",
		},
	}

	showResp := helper.executeAndGetResponse(t, context.Background(), bus.InboundMessage{
		Channel:  baseMsg.Channel,
		SenderID: baseMsg.SenderID,
		ChatID:   baseMsg.ChatID,
		Content:  "/show channel",
		Peer:     baseMsg.Peer,
	})
	if showResp != "Current Channel: whatsapp" {
		t.Fatalf("unexpected /show reply: %q", showResp)
	}
	if provider.calls != 0 {
		t.Fatalf("LLM should not be called for handled command, calls=%d", provider.calls)
	}

	fooResp := helper.executeAndGetResponse(t, context.Background(), bus.InboundMessage{
		Channel:  baseMsg.Channel,
		SenderID: baseMsg.SenderID,
		ChatID:   baseMsg.ChatID,
		Content:  "/foo",
		Peer:     baseMsg.Peer,
	})
	if fooResp != "LLM reply" {
		t.Fatalf("unexpected /foo reply: %q", fooResp)
	}
	if provider.calls != 1 {
		t.Fatalf("LLM should be called exactly once after /foo passthrough, calls=%d", provider.calls)
	}

	newResp := helper.executeAndGetResponse(t, context.Background(), bus.InboundMessage{
		Channel:  baseMsg.Channel,
		SenderID: baseMsg.SenderID,
		ChatID:   baseMsg.ChatID,
		Content:  "/new",
		Peer:     baseMsg.Peer,
	})
	if newResp != "LLM reply" {
		t.Fatalf("unexpected /new reply: %q", newResp)
	}
	if provider.calls != 2 {
		t.Fatalf("LLM should be called for passthrough /new command, calls=%d", provider.calls)
	}
}

func TestProcessMessage_SwitchModelShowModelConsistency(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "agent-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfg := &config.Config{
		Agents: config.AgentsConfig{
			Defaults: config.AgentDefaults{
				Workspace:         tmpDir,
				Provider:          "openai",
				ModelName:         "local",
				MaxTokens:         4096,
				MaxToolIterations: 10,
			},
		},
		ModelList: []*config.ModelConfig{
			{
				ModelName: "local",
				Model:     "openai/local-model",
				APIBase:   "https://local.example.invalid/v1",
			},
			{
				ModelName: "deepseek",
				Model:     "openrouter/deepseek/deepseek-v3.2",
				APIBase:   "https://openrouter.ai/api/v1",
			},
		},
	}
	cfg.WithSecurity(&config.SecurityConfig{
		ModelList: map[string]config.ModelSecurityEntry{
			"local": {
				APIKeys: []string{"test-key"},
			},
			"deepseek": {
				APIKeys: []string{"test-key"},
			},
		},
	})

	msgBus := bus.NewMessageBus()
	provider := &countingMockProvider{response: "LLM reply"}
	al := NewAgentLoop(cfg, msgBus, provider)
	helper := testHelper{al: al}

	switchResp := helper.executeAndGetResponse(t, context.Background(), bus.InboundMessage{
		Channel:  "telegram",
		SenderID: "user1",
		ChatID:   "chat1",
		Content:  "/switch model to deepseek",
		Peer: bus.Peer{
			Kind: "direct",
			ID:   "user1",
		},
	})
	if !strings.Contains(switchResp, "Switched model from local to deepseek") {
		t.Fatalf("unexpected /switch reply: %q", switchResp)
	}

	showResp := helper.executeAndGetResponse(t, context.Background(), bus.InboundMessage{
		Channel:  "telegram",
		SenderID: "user1",
		ChatID:   "chat1",
		Content:  "/show model",
		Peer: bus.Peer{
			Kind: "direct",
			ID:   "user1",
		},
	})
	if !strings.Contains(showResp, "Current Model: deepseek (Provider: openrouter)") {
		t.Fatalf("unexpected /show model reply after switch: %q", showResp)
	}

	if provider.calls != 0 {
		t.Fatalf("LLM should not be called for /switch and /show, calls=%d", provider.calls)
	}
}

func TestProcessMessage_SwitchModelRejectsUnknownAlias(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "agent-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfg := &config.Config{
		Agents: config.AgentsConfig{
			Defaults: config.AgentDefaults{
				Workspace:         tmpDir,
				Provider:          "openai",
				ModelName:         "local",
				MaxTokens:         4096,
				MaxToolIterations: 10,
			},
		},
		ModelList: []*config.ModelConfig{
			{
				ModelName: "local",
				Model:     "openai/local-model",
				APIBase:   "https://local.example.invalid/v1",
			},
		},
	}
	cfg.WithSecurity(&config.SecurityConfig{
		ModelList: map[string]config.ModelSecurityEntry{
			"local": {
				APIKeys: []string{"test-key"},
			},
		},
	})

	msgBus := bus.NewMessageBus()
	provider := &countingMockProvider{response: "LLM reply"}
	al := NewAgentLoop(cfg, msgBus, provider)
	helper := testHelper{al: al}

	switchResp := helper.executeAndGetResponse(t, context.Background(), bus.InboundMessage{
		Channel:  "telegram",
		SenderID: "user1",
		ChatID:   "chat1",
		Content:  "/switch model to missing",
		Peer: bus.Peer{
			Kind: "direct",
			ID:   "user1",
		},
	})
	if switchResp != `model "missing" not found in model_list or providers` {
		t.Fatalf("unexpected /switch error reply: %q", switchResp)
	}

	showResp := helper.executeAndGetResponse(t, context.Background(), bus.InboundMessage{
		Channel:  "telegram",
		SenderID: "user1",
		ChatID:   "chat1",
		Content:  "/show model",
		Peer: bus.Peer{
			Kind: "direct",
			ID:   "user1",
		},
	})
	if !strings.Contains(showResp, "Current Model: local (Provider: openai)") {
		t.Fatalf("unexpected /show model reply after rejected switch: %q", showResp)
	}

	if provider.calls != 0 {
		t.Fatalf("LLM should not be called for rejected /switch and /show, calls=%d", provider.calls)
	}
}

func TestProcessMessage_SwitchModelRoutesSubsequentRequestsToSelectedProvider(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "agent-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	localCalls := 0
	localModel := ""
	localServer := newChatCompletionTestServer(t, "local", "local reply", &localCalls, &localModel)
	defer localServer.Close()

	remoteCalls := 0
	remoteModel := ""
	remoteServer := newChatCompletionTestServer(t, "remote", "remote reply", &remoteCalls, &remoteModel)
	defer remoteServer.Close()

	cfg := &config.Config{
		Agents: config.AgentsConfig{
			Defaults: config.AgentDefaults{
				Workspace:         tmpDir,
				Provider:          "openai",
				ModelName:         "local",
				MaxTokens:         4096,
				MaxToolIterations: 10,
			},
		},
		ModelList: []*config.ModelConfig{
			{
				ModelName: "local",
				Model:     "openai/Qwen3.5-35B-A3B",
				APIBase:   localServer.URL,
			},
			{
				ModelName: "deepseek",
				Model:     "openrouter/deepseek/deepseek-v3.2",
				APIBase:   remoteServer.URL,
			},
		},
	}
	cfg.WithSecurity(&config.SecurityConfig{
		ModelList: map[string]config.ModelSecurityEntry{
			"local": {
				APIKeys: []string{"local-key"},
			},
			"deepseek": {
				APIKeys: []string{"remote-key"},
			},
		},
	})

	msgBus := bus.NewMessageBus()
	provider, _, err := providers.CreateProvider(cfg)
	if err != nil {
		t.Fatalf("CreateProvider() error = %v", err)
	}
	al := NewAgentLoop(cfg, msgBus, provider)
	helper := testHelper{al: al}

	firstResp := helper.executeAndGetResponse(t, context.Background(), bus.InboundMessage{
		Channel:  "telegram",
		SenderID: "user1",
		ChatID:   "chat1",
		Content:  "hello before switch",
		Peer: bus.Peer{
			Kind: "direct",
			ID:   "user1",
		},
	})
	if firstResp != "local reply" {
		t.Fatalf("unexpected response before switch: %q", firstResp)
	}
	if localCalls != 1 {
		t.Fatalf("local calls before switch = %d, want 1", localCalls)
	}
	if remoteCalls != 0 {
		t.Fatalf("remote calls before switch = %d, want 0", remoteCalls)
	}
	if localModel != "Qwen3.5-35B-A3B" {
		t.Fatalf("local model before switch = %q, want %q", localModel, "Qwen3.5-35B-A3B")
	}

	switchResp := helper.executeAndGetResponse(t, context.Background(), bus.InboundMessage{
		Channel:  "telegram",
		SenderID: "user1",
		ChatID:   "chat1",
		Content:  "/switch model to deepseek",
		Peer: bus.Peer{
			Kind: "direct",
			ID:   "user1",
		},
	})
	if !strings.Contains(switchResp, "Switched model from local to deepseek") {
		t.Fatalf("unexpected /switch reply: %q", switchResp)
	}

	secondResp := helper.executeAndGetResponse(t, context.Background(), bus.InboundMessage{
		Channel:  "telegram",
		SenderID: "user1",
		ChatID:   "chat1",
		Content:  "hello after switch",
		Peer: bus.Peer{
			Kind: "direct",
			ID:   "user1",
		},
	})
	if secondResp != "remote reply" {
		t.Fatalf("unexpected response after switch: %q", secondResp)
	}
	if localCalls != 1 {
		t.Fatalf("local calls after switch = %d, want 1", localCalls)
	}
	if remoteCalls != 1 {
		t.Fatalf("remote calls after switch = %d, want 1", remoteCalls)
	}
	if remoteModel != "deepseek-v3.2" {
		t.Fatalf(
			"remote model after switch = %q, want %q",
			remoteModel,
			"deepseek-v3.2",
		)
	}
}

// TestToolResult_SilentToolDoesNotSendUserMessage verifies silent tools don't trigger outbound
