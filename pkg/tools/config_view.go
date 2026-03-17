package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/sipeed/picoclaw/pkg/config"
)

// ConfigViewTool lets the agent inspect the running configuration with secrets redacted.
type ConfigViewTool struct {
	cfg *config.Config
}

func NewConfigViewTool(cfg *config.Config) *ConfigViewTool {
	return &ConfigViewTool{cfg: cfg}
}

func (t *ConfigViewTool) Name() string {
	return "config_view"
}

func (t *ConfigViewTool) Description() string {
	return "View the running picoclaw configuration. Query a specific section to inspect models, channels, tools, agents, providers, or build info. API keys and secrets are redacted."
}

func (t *ConfigViewTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"section": map[string]any{
				"type":        "string",
				"enum":        []string{"overview", "models", "channels", "tools", "agents", "providers", "build_info"},
				"description": "Configuration section to view.",
			},
		},
		"required": []string{"section"},
	}
}

func (t *ConfigViewTool) Execute(_ context.Context, args map[string]any) *ToolResult {
	section, ok := args["section"].(string)
	if !ok || section == "" {
		return ErrorResult("section is required")
	}

	switch section {
	case "overview":
		return SilentResult(t.overview())
	case "models":
		return SilentResult(t.models())
	case "channels":
		return SilentResult(t.channels())
	case "tools":
		return SilentResult(t.tools())
	case "agents":
		return SilentResult(t.agents())
	case "providers":
		return SilentResult(t.providers())
	case "build_info":
		return SilentResult(t.buildInfo())
	default:
		return ErrorResult(fmt.Sprintf(
			"unknown section: %q (valid: overview, models, channels, tools, agents, providers, build_info)",
			section,
		))
	}
}

func (t *ConfigViewTool) overview() string {
	var b strings.Builder
	b.WriteString("=== PicoClaw Configuration Overview ===\n")
	fmt.Fprintf(&b, "Version: %s\n", t.cfg.BuildInfo.Version)
	fmt.Fprintf(&b, "Default model: %s\n", t.cfg.Agents.Defaults.ModelName)
	fmt.Fprintf(&b, "Workspace: %s\n", t.cfg.Agents.Defaults.Workspace)

	enabledChannels := t.countEnabledChannels()
	fmt.Fprintf(&b, "Enabled channels: %d\n", enabledChannels)

	enabledTools := t.countEnabledTools()
	fmt.Fprintf(&b, "Enabled tools: %d\n", enabledTools)

	configuredModels := 0
	for _, m := range t.cfg.ModelList {
		if m.APIKey != "" {
			configuredModels++
		}
	}
	fmt.Fprintf(&b, "Configured models (with API key): %d\n", configuredModels)

	return b.String()
}

func (t *ConfigViewTool) models() string {
	var b strings.Builder
	b.WriteString("=== Model List ===\n")

	if len(t.cfg.ModelList) == 0 {
		b.WriteString("No models configured.\n")
		return b.String()
	}

	for i, m := range t.cfg.ModelList {
		if i > 0 {
			b.WriteString("\n")
		}
		fmt.Fprintf(&b, "- %s\n", m.ModelName)
		fmt.Fprintf(&b, "  Model: %s\n", m.Model)
		if m.APIBase != "" {
			fmt.Fprintf(&b, "  API Base: %s\n", m.APIBase)
		}
		fmt.Fprintf(&b, "  API Key: %s\n", redactKey(m.APIKey))
		if m.RPM > 0 {
			fmt.Fprintf(&b, "  RPM: %d\n", m.RPM)
		}
		if m.ThinkingLevel != "" {
			fmt.Fprintf(&b, "  Thinking Level: %s\n", m.ThinkingLevel)
		}
	}

	return b.String()
}

func (t *ConfigViewTool) channels() string {
	var b strings.Builder
	b.WriteString("=== Channels ===\n")

	ch := t.cfg.Channels
	entries := []struct {
		name    string
		enabled bool
	}{
		{"whatsapp", ch.WhatsApp.Enabled},
		{"telegram", ch.Telegram.Enabled},
		{"feishu", ch.Feishu.Enabled},
		{"discord", ch.Discord.Enabled},
		{"maixcam", ch.MaixCam.Enabled},
		{"qq", ch.QQ.Enabled},
		{"dingtalk", ch.DingTalk.Enabled},
		{"slack", ch.Slack.Enabled},
		{"matrix", ch.Matrix.Enabled},
		{"line", ch.LINE.Enabled},
		{"onebot", ch.OneBot.Enabled},
		{"wecom", ch.WeCom.Enabled},
		{"wecom_app", ch.WeComApp.Enabled},
		{"wecom_aibot", ch.WeComAIBot.Enabled},
		{"pico", ch.Pico.Enabled},
		{"irc", ch.IRC.Enabled},
	}

	for _, e := range entries {
		status := "disabled"
		if e.enabled {
			status = "enabled"
		}
		fmt.Fprintf(&b, "- %s: %s\n", e.name, status)
	}

	return b.String()
}

func (t *ConfigViewTool) tools() string {
	var b strings.Builder
	b.WriteString("=== Tools ===\n")

	toolNames := []string{
		"web", "cron", "exec", "skills", "media_cleanup", "mcp",
		"append_file", "edit_file", "find_skills", "i2c", "install_skill",
		"list_dir", "message", "read_file", "send_file", "spawn",
		"spi", "subagent", "web_fetch", "write_file", "coding_agent",
		"config_view",
	}

	for _, name := range toolNames {
		status := "disabled"
		if t.cfg.Tools.IsToolEnabled(name) {
			status = "enabled"
		}
		fmt.Fprintf(&b, "- %s: %s\n", name, status)
	}

	b.WriteString("\nNotable settings:\n")
	fmt.Fprintf(&b, "  Exec timeout: %ds\n", t.cfg.Tools.Exec.TimeoutSeconds)
	fmt.Fprintf(&b, "  Coding agent backend: %s\n", t.cfg.Tools.CodingAgent.Backend)

	return b.String()
}

func (t *ConfigViewTool) agents() string {
	var b strings.Builder
	b.WriteString("=== Agents ===\n")

	d := t.cfg.Agents.Defaults
	b.WriteString("Defaults:\n")
	fmt.Fprintf(&b, "  Model: %s\n", d.ModelName)
	if len(d.ModelFallbacks) > 0 {
		fmt.Fprintf(&b, "  Fallbacks: %s\n", strings.Join(d.ModelFallbacks, ", "))
	}
	fmt.Fprintf(&b, "  Max tokens: %d\n", d.MaxTokens)
	if d.ContextWindow > 0 {
		fmt.Fprintf(&b, "  Context window: %d\n", d.ContextWindow)
	}
	if d.Temperature != nil {
		fmt.Fprintf(&b, "  Temperature: %.2f\n", *d.Temperature)
	}
	fmt.Fprintf(&b, "  Max tool iterations: %d\n", d.MaxToolIterations)
	if d.Routing != nil {
		b.WriteString("  Routing: configured\n")
	}

	if len(t.cfg.Agents.List) > 0 {
		b.WriteString("\nAgent list:\n")
		for _, a := range t.cfg.Agents.List {
			fmt.Fprintf(&b, "- %s (id: %s)\n", a.Name, a.ID)
			if a.Model != nil {
				fmt.Fprintf(&b, "  Model: %s\n", a.Model.Primary)
			}
			if len(a.Skills) > 0 {
				fmt.Fprintf(&b, "  Skills: %s\n", strings.Join(a.Skills, ", "))
			}
		}
	}

	return b.String()
}

func (t *ConfigViewTool) providers() string {
	var b strings.Builder
	b.WriteString("=== Providers (legacy) ===\n")

	p := t.cfg.Providers
	entries := []struct {
		name   string
		apiKey string
	}{
		{"anthropic", p.Anthropic.APIKey},
		{"openai", p.OpenAI.APIKey},
		{"litellm", p.LiteLLM.APIKey},
		{"openrouter", p.OpenRouter.APIKey},
		{"groq", p.Groq.APIKey},
		{"zhipu", p.Zhipu.APIKey},
		{"vllm", p.VLLM.APIKey},
		{"gemini", p.Gemini.APIKey},
		{"nvidia", p.Nvidia.APIKey},
		{"ollama", p.Ollama.APIKey},
		{"moonshot", p.Moonshot.APIKey},
		{"shengsuanyun", p.ShengSuanYun.APIKey},
		{"deepseek", p.DeepSeek.APIKey},
		{"cerebras", p.Cerebras.APIKey},
		{"vivgrid", p.Vivgrid.APIKey},
		{"volcengine", p.VolcEngine.APIKey},
		{"github_copilot", p.GitHubCopilot.APIKey},
		{"antigravity", p.Antigravity.APIKey},
		{"qwen", p.Qwen.APIKey},
		{"mistral", p.Mistral.APIKey},
		{"avian", p.Avian.APIKey},
		{"minimax", p.Minimax.APIKey},
		{"longcat", p.LongCat.APIKey},
		{"modelscope", p.ModelScope.APIKey},
	}

	for _, e := range entries {
		fmt.Fprintf(&b, "- %s: API key %s\n", e.name, redactKey(e.apiKey))
	}

	return b.String()
}

func (t *ConfigViewTool) buildInfo() string {
	var b strings.Builder
	b.WriteString("=== Build Info ===\n")
	bi := t.cfg.BuildInfo
	fmt.Fprintf(&b, "Version: %s\n", bi.Version)
	fmt.Fprintf(&b, "Git commit: %s\n", bi.GitCommit)
	fmt.Fprintf(&b, "Build time: %s\n", bi.BuildTime)
	fmt.Fprintf(&b, "Go version: %s\n", bi.GoVersion)
	return b.String()
}

func (t *ConfigViewTool) countEnabledChannels() int {
	count := 0
	ch := t.cfg.Channels
	for _, enabled := range []bool{
		ch.WhatsApp.Enabled, ch.Telegram.Enabled, ch.Feishu.Enabled,
		ch.Discord.Enabled, ch.MaixCam.Enabled, ch.QQ.Enabled,
		ch.DingTalk.Enabled, ch.Slack.Enabled, ch.Matrix.Enabled,
		ch.LINE.Enabled, ch.OneBot.Enabled, ch.WeCom.Enabled,
		ch.WeComApp.Enabled, ch.WeComAIBot.Enabled, ch.Pico.Enabled,
		ch.IRC.Enabled,
	} {
		if enabled {
			count++
		}
	}
	return count
}

func (t *ConfigViewTool) countEnabledTools() int {
	count := 0
	toolNames := []string{
		"web", "cron", "exec", "skills", "media_cleanup", "mcp",
		"append_file", "edit_file", "find_skills", "i2c", "install_skill",
		"list_dir", "message", "read_file", "send_file", "spawn",
		"spi", "subagent", "web_fetch", "write_file", "coding_agent",
		"config_view",
	}
	for _, name := range toolNames {
		if t.cfg.Tools.IsToolEnabled(name) {
			count++
		}
	}
	return count
}

func redactKey(s string) string {
	if strings.TrimSpace(s) != "" {
		return "[SET]"
	}
	return "[NOT SET]"
}
