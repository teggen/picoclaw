// loop_routing.go contains message routing, command dispatch, and skill management for AgentLoop.

package agent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/sipeed/picoclaw/pkg/bus"
	"github.com/sipeed/picoclaw/pkg/commands"
	"github.com/sipeed/picoclaw/pkg/constants"
	"github.com/sipeed/picoclaw/pkg/logger"
	"github.com/sipeed/picoclaw/pkg/providers"
	"github.com/sipeed/picoclaw/pkg/routing"
	"github.com/sipeed/picoclaw/pkg/utils"
)

func (al *AgentLoop) processMessage(ctx context.Context, msg bus.InboundMessage) (string, error) {
	// Add message preview to log (show full content for error messages)
	var logContent string
	if strings.Contains(msg.Content, "Error:") || strings.Contains(msg.Content, "error") {
		logContent = msg.Content // Full content for errors
	} else {
		logContent = utils.Truncate(msg.Content, 80)
	}
	logger.InfoCF(
		"agent",
		fmt.Sprintf("Processing message from %s:%s: %s", msg.Channel, msg.SenderID, logContent),
		map[string]any{
			"channel":     msg.Channel,
			"chat_id":     msg.ChatID,
			"sender_id":   msg.SenderID,
			"session_key": msg.SessionKey,
		},
	)

	var hadAudio bool
	msg, hadAudio = al.transcribeAudioInMessage(ctx, msg)

	// For audio messages the placeholder was deferred by the channel.
	// Now that transcription (and optional feedback) is done, send it.
	if hadAudio && al.channelManager != nil {
		al.channelManager.SendPlaceholder(ctx, msg.Channel, msg.ChatID)
	}

	// Route system messages to processSystemMessage
	if msg.Channel == "system" {
		return al.processSystemMessage(ctx, msg)
	}

	route, agent, routeErr := al.resolveMessageRoute(msg)
	if routeErr != nil {
		return "", routeErr
	}

	// Reset message-tool state for this round so we don't skip publishing due to a previous round.
	if tool, ok := agent.Tools.Get("message"); ok {
		if resetter, ok := tool.(interface{ ResetSentInRound() }); ok {
			resetter.ResetSentInRound()
		}
	}

	// Resolve session key from route, while preserving explicit agent-scoped keys.
	scopeKey := resolveScopeKey(route, msg.SessionKey)
	sessionKey := scopeKey

	logger.InfoCF("agent", "Routed message",
		map[string]any{
			"agent_id":      agent.ID,
			"scope_key":     scopeKey,
			"session_key":   sessionKey,
			"matched_by":    route.MatchedBy,
			"route_agent":   route.AgentID,
			"route_channel": route.Channel,
		})

	opts := processOptions{
		SessionKey:        sessionKey,
		Channel:           msg.Channel,
		ChatID:            msg.ChatID,
		SenderID:          msg.SenderID,
		SenderDisplayName: msg.Sender.DisplayName,
		UserMessage:       msg.Content,
		Media:             msg.Media,
		DefaultResponse:   defaultResponse,
		EnableSummary:     true,
		SendResponse:      false,
	}

	// context-dependent commands check their own Runtime fields and report
	// "unavailable" when the required capability is nil.
	if response, handled := al.handleCommand(ctx, msg, agent, &opts); handled {
		return response, nil
	}

	if pending := al.takePendingSkills(opts.SessionKey); len(pending) > 0 {
		opts.ForcedSkills = append(opts.ForcedSkills, pending...)
		logger.InfoCF("agent", "Applying pending skill override",
			map[string]any{
				"session_key": opts.SessionKey,
				"skills":      strings.Join(pending, ","),
			})
	}

	return al.runAgentLoop(ctx, agent, opts)
}

func (al *AgentLoop) resolveMessageRoute(msg bus.InboundMessage) (routing.ResolvedRoute, *AgentInstance, error) {
	registry := al.GetRegistry()
	route := registry.ResolveRoute(routing.RouteInput{
		Channel:    msg.Channel,
		AccountID:  inboundMetadata(msg, metadataKeyAccountID),
		Peer:       extractPeer(msg),
		ParentPeer: extractParentPeer(msg),
		GuildID:    inboundMetadata(msg, metadataKeyGuildID),
		TeamID:     inboundMetadata(msg, metadataKeyTeamID),
	})

	agent, ok := registry.GetAgent(route.AgentID)
	if !ok {
		agent = registry.GetDefaultAgent()
	}
	if agent == nil {
		return routing.ResolvedRoute{}, nil, fmt.Errorf("no agent available for route (agent_id=%s)", route.AgentID)
	}

	return route, agent, nil
}

func resolveScopeKey(route routing.ResolvedRoute, msgSessionKey string) string {
	if msgSessionKey != "" && strings.HasPrefix(msgSessionKey, sessionKeyAgentPrefix) {
		return msgSessionKey
	}
	return route.SessionKey
}

func (al *AgentLoop) resolveSteeringTarget(msg bus.InboundMessage) (string, string, bool) {
	if msg.Channel == "system" {
		return "", "", false
	}

	route, agent, err := al.resolveMessageRoute(msg)
	if err != nil || agent == nil {
		return "", "", false
	}

	return resolveScopeKey(route, msg.SessionKey), agent.ID, true
}

func (al *AgentLoop) requeueInboundMessage(msg bus.InboundMessage) error {
	if al.bus == nil {
		return nil
	}
	pubCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	return al.bus.PublishOutbound(pubCtx, bus.OutboundMessage{
		Channel: msg.Channel,
		ChatID:  msg.ChatID,
		Content: msg.Content,
	})
}

func (al *AgentLoop) processSystemMessage(
	ctx context.Context,
	msg bus.InboundMessage,
) (string, error) {
	if msg.Channel != "system" {
		return "", fmt.Errorf(
			"processSystemMessage called with non-system message channel: %s",
			msg.Channel,
		)
	}

	logger.InfoCF("agent", "Processing system message",
		map[string]any{
			"sender_id": msg.SenderID,
			"chat_id":   msg.ChatID,
		})

	// Parse origin channel from chat_id (format: "channel:chat_id")
	var originChannel, originChatID string
	if idx := strings.Index(msg.ChatID, ":"); idx > 0 {
		originChannel = msg.ChatID[:idx]
		originChatID = msg.ChatID[idx+1:]
	} else {
		originChannel = "cli"
		originChatID = msg.ChatID
	}

	// Extract subagent result from message content
	// Format: "Task 'label' completed.\n\nResult:\n<actual content>"
	content := msg.Content
	if idx := strings.Index(content, "Result:\n"); idx >= 0 {
		content = content[idx+8:] // Extract just the result part
	}

	// Skip internal channels - only log, don't send to user
	if constants.IsInternalChannel(originChannel) {
		logger.InfoCF("agent", "Subagent completed (internal channel)",
			map[string]any{
				"sender_id":   msg.SenderID,
				"content_len": len(content),
				"channel":     originChannel,
			})
		return "", nil
	}

	// Use default agent for system messages
	agent := al.GetRegistry().GetDefaultAgent()
	if agent == nil {
		return "", fmt.Errorf("no default agent for system message")
	}

	// Use the origin session for context
	sessionKey := routing.BuildAgentMainSessionKey(agent.ID)

	return al.runAgentLoop(ctx, agent, processOptions{
		SessionKey:      sessionKey,
		Channel:         originChannel,
		ChatID:          originChatID,
		UserMessage:     fmt.Sprintf("[System: %s] %s", msg.SenderID, msg.Content),
		DefaultResponse: "Background task completed.",
		EnableSummary:   false,
		SendResponse:    true,
	})
}

// runAgentLoop remains the top-level shell that starts a turn and publishes
// any post-turn work. runTurn owns the full turn lifecycle.

func (al *AgentLoop) handleCommand(
	ctx context.Context,
	msg bus.InboundMessage,
	agent *AgentInstance,
	opts *processOptions,
) (string, bool) {
	if !commands.HasCommandPrefix(msg.Content) {
		return "", false
	}

	if matched, handled, reply := al.applyExplicitSkillCommand(msg.Content, agent, opts); matched {
		return reply, handled
	}

	if al.cmdRegistry == nil {
		return "", false
	}

	rt := al.buildCommandsRuntime(agent, opts)
	executor := commands.NewExecutor(al.cmdRegistry, rt)

	var commandReply string
	result := executor.Execute(ctx, commands.Request{
		Channel:  msg.Channel,
		ChatID:   msg.ChatID,
		SenderID: msg.SenderID,
		Text:     msg.Content,
		Reply: func(text string) error {
			commandReply = text
			return nil
		},
	})

	switch result.Outcome {
	case commands.OutcomeHandled:
		if result.Err != nil {
			return mapCommandError(result), true
		}
		if commandReply != "" {
			return commandReply, true
		}
		return "", true
	default: // OutcomePassthrough — let the message fall through to LLM
		return "", false
	}
}

func activeSkillNames(agent *AgentInstance, opts processOptions) []string {
	if agent == nil {
		return nil
	}

	combined := make([]string, 0, len(agent.SkillsFilter)+len(opts.ForcedSkills))
	combined = append(combined, agent.SkillsFilter...)
	combined = append(combined, opts.ForcedSkills...)
	if len(combined) == 0 {
		return nil
	}

	var resolved []string
	seen := make(map[string]struct{}, len(combined))
	for _, name := range combined {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if agent.ContextBuilder != nil {
			if canonical, ok := agent.ContextBuilder.ResolveSkillName(name); ok {
				name = canonical
			}
		}
		key := strings.ToLower(name)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		resolved = append(resolved, name)
	}

	return resolved
}

func (al *AgentLoop) applyExplicitSkillCommand(
	raw string,
	agent *AgentInstance,
	opts *processOptions,
) (matched bool, handled bool, reply string) {
	cmdName, ok := commands.CommandName(raw)
	if !ok || cmdName != "use" {
		return false, false, ""
	}

	if agent == nil || agent.ContextBuilder == nil {
		return true, true, commandsUnavailableSkillMessage()
	}

	parts := strings.Fields(strings.TrimSpace(raw))
	if len(parts) < 2 {
		return true, true, buildUseCommandHelp(agent)
	}

	arg := strings.TrimSpace(parts[1])
	if strings.EqualFold(arg, "clear") || strings.EqualFold(arg, "off") {
		if opts != nil {
			al.clearPendingSkills(opts.SessionKey)
		}
		return true, true, "Cleared pending skill override."
	}

	skillName, ok := agent.ContextBuilder.ResolveSkillName(arg)
	if !ok {
		return true, true, fmt.Sprintf("Unknown skill: %s\nUse /list skills to see installed skills.", arg)
	}

	if len(parts) < 3 {
		if opts == nil || strings.TrimSpace(opts.SessionKey) == "" {
			return true, true, commandsUnavailableSkillMessage()
		}
		al.setPendingSkills(opts.SessionKey, []string{skillName})
		return true, true, fmt.Sprintf(
			"Skill %q is armed for your next message. Send your next prompt normally, or use /use clear to cancel.",
			skillName,
		)
	}

	message := strings.TrimSpace(strings.Join(parts[2:], " "))
	if message == "" {
		return true, true, buildUseCommandHelp(agent)
	}

	if opts != nil {
		opts.ForcedSkills = append(opts.ForcedSkills, skillName)
		opts.UserMessage = message
	}

	return true, false, ""
}

func (al *AgentLoop) buildCommandsRuntime(agent *AgentInstance, opts *processOptions) *commands.Runtime {
	registry := al.GetRegistry()
	cfg := al.GetConfig()
	rt := &commands.Runtime{
		Config:          cfg,
		ListAgentIDs:    registry.ListAgentIDs,
		ListDefinitions: al.cmdRegistry.Definitions,
		GetEnabledChannels: func() []string {
			if al.channelManager == nil {
				return nil
			}
			return al.channelManager.GetEnabledChannels()
		},
		GetActiveTurn: func() any {
			info := al.GetActiveTurn()
			if info == nil {
				return nil
			}
			return info
		},
		SwitchChannel: func(value string) error {
			if al.channelManager == nil {
				return fmt.Errorf("channel manager not initialized")
			}
			if _, exists := al.channelManager.GetChannel(value); !exists && value != "cli" {
				return fmt.Errorf("channel '%s' not found or not enabled", value)
			}
			return nil
		},
	}
	if agent != nil && agent.ContextBuilder != nil {
		rt.ListSkillNames = agent.ContextBuilder.ListSkillNames
	}
	rt.ReloadConfig = func() error {
		if al.reloadFunc == nil {
			return fmt.Errorf("reload not configured")
		}
		return al.reloadFunc()
	}
	if agent != nil {
		if agent.ContextBuilder != nil {
			rt.ListSkillNames = agent.ContextBuilder.ListSkillNames
		}
		rt.GetModelInfo = func() (string, string) {
			return agent.Model, resolvedCandidateProvider(agent.Candidates, cfg.Agents.Defaults.Provider)
		}
		rt.SwitchModel = func(value string) (string, error) {
			value = strings.TrimSpace(value)
			modelCfg, err := resolvedModelConfig(cfg, value, agent.Workspace)
			if err != nil {
				return "", err
			}

			nextProvider, _, err := providers.CreateProviderFromConfig(modelCfg)
			if err != nil {
				return "", fmt.Errorf("failed to initialize model %q: %w", value, err)
			}

			nextCandidates := resolveModelCandidates(cfg, cfg.Agents.Defaults.Provider, modelCfg.Model, agent.Fallbacks)
			if len(nextCandidates) == 0 {
				return "", fmt.Errorf("model %q did not resolve to any provider candidates", value)
			}

			oldModel := agent.Model
			oldProvider := agent.Provider
			agent.Model = value
			agent.Provider = nextProvider
			agent.Candidates = nextCandidates
			agent.ThinkingLevel = parseThinkingLevel(modelCfg.ThinkingLevel)

			if oldProvider != nil && oldProvider != nextProvider {
				if stateful, ok := oldProvider.(providers.StatefulProvider); ok {
					stateful.Close()
				}
			}
			return oldModel, nil
		}

		rt.ClearHistory = func() error {
			if opts == nil {
				return fmt.Errorf("process options not available")
			}
			if agent.Sessions == nil {
				return fmt.Errorf("sessions not initialized for agent")
			}

			agent.Sessions.SetHistory(opts.SessionKey, make([]providers.Message, 0))
			agent.Sessions.SetSummary(opts.SessionKey, "")
			agent.Sessions.Save(opts.SessionKey)
			return nil
		}
	}
	return rt
}

func commandsUnavailableSkillMessage() string {
	return "Skill selection is unavailable in the current context."
}

func buildUseCommandHelp(agent *AgentInstance) string {
	if agent == nil || agent.ContextBuilder == nil {
		return "Usage: /use <skill> [message]"
	}

	names := agent.ContextBuilder.ListSkillNames()
	if len(names) == 0 {
		return "Usage: /use <skill> [message]\nNo installed skills found."
	}

	return fmt.Sprintf(
		"Usage: /use <skill> [message]\n\nInstalled Skills:\n- %s\n\nUse /use <skill> to apply a skill to your next message, or /use <skill> <message> to force it immediately.",
		strings.Join(names, "\n- "),
	)
}

func (al *AgentLoop) setPendingSkills(sessionKey string, skillNames []string) {
	sessionKey = strings.TrimSpace(sessionKey)
	if sessionKey == "" || len(skillNames) == 0 {
		return
	}

	filtered := make([]string, 0, len(skillNames))
	for _, name := range skillNames {
		name = strings.TrimSpace(name)
		if name != "" {
			filtered = append(filtered, name)
		}
	}
	if len(filtered) == 0 {
		return
	}

	al.pendingSkills.Store(sessionKey, filtered)
}

func (al *AgentLoop) takePendingSkills(sessionKey string) []string {
	sessionKey = strings.TrimSpace(sessionKey)
	if sessionKey == "" {
		return nil
	}

	value, ok := al.pendingSkills.LoadAndDelete(sessionKey)
	if !ok {
		return nil
	}

	skills, ok := value.([]string)
	if !ok {
		return nil
	}

	return append([]string(nil), skills...)
}

func (al *AgentLoop) clearPendingSkills(sessionKey string) {
	sessionKey = strings.TrimSpace(sessionKey)
	if sessionKey == "" {
		return
	}
	al.pendingSkills.Delete(sessionKey)
}

func mapCommandError(result commands.ExecuteResult) string {
	if result.Command == "" {
		return fmt.Sprintf("Failed to execute command: %v", result.Err)
	}
	return fmt.Sprintf("Failed to execute /%s: %v", result.Command, result.Err)
}

// extractPeer extracts the routing peer from the inbound message's structured Peer field.
func extractPeer(msg bus.InboundMessage) *routing.RoutePeer {
	if msg.Peer.Kind == "" {
		return nil
	}
	peerID := msg.Peer.ID
	if peerID == "" {
		if msg.Peer.Kind == "direct" {
			peerID = msg.SenderID
		} else {
			peerID = msg.ChatID
		}
	}
	return &routing.RoutePeer{Kind: msg.Peer.Kind, ID: peerID}
}

func inboundMetadata(msg bus.InboundMessage, key string) string {
	if msg.Metadata == nil {
		return ""
	}
	return msg.Metadata[key]
}

// extractParentPeer extracts the parent peer (reply-to) from inbound message metadata.
func extractParentPeer(msg bus.InboundMessage) *routing.RoutePeer {
	parentKind := inboundMetadata(msg, metadataKeyParentPeerKind)
	parentID := inboundMetadata(msg, metadataKeyParentPeerID)
	if parentKind == "" || parentID == "" {
		return nil
	}
	return &routing.RoutePeer{Kind: parentKind, ID: parentID}
}
