# Documentation Audit Report

**Date:** 2026-03-26
**Scope:** All files in `docs/`
**Method:** Cross-referenced each document against the current codebase on branch `customization-phase-1`

---

## Summary

| Category | Count |
|----------|-------|
| Up-to-date | 22 |
| Partially outdated | 3 |
| References removed/unimplemented feature | 1 |
| Planning docs (all implemented) | 7 |
| **Total documents** | **33** |

The documentation is in good shape overall. Most docs accurately reflect the codebase. The issues found are minor field-name mismatches and one doc referencing a removed channel.

---

## Issues Found

### 1. `docs/configuration.md` — Field name error in agent defaults example

**Severity:** Medium
**Line:** 512

The example shows `"model": "gpt-5.4"` under `agents.defaults`, but the actual config struct field is `model_name` (JSON tag `json:"model_name"`, defined at `pkg/config/config.go:308`). The `"model"` key is only valid inside `model_list[]` entries (where it holds the protocol-prefixed identifier like `openai/gpt-5.4`).

```json
// DOCUMENTED (incorrect):
"agents": { "defaults": { "model": "gpt-5.4" } }

// ACTUAL (correct):
"agents": { "defaults": { "model_name": "gpt-5.4" } }
```

### 2. `docs/chat-apps.md` — Documents WeCom channel, but implementation was removed

**Severity:** Medium
**Lines:** 377-420

The WeCom section describes configuration, `picoclaw auth wecom` setup, and WebSocket-based AI Bot functionality. However, the WeCom channel implementation has been removed from the codebase — `pkg/channels/manager_channel.go:52` contains `// WeCom channel removed`, and no `pkg/channels/wecom/` directory exists.

Additionally, the doc references a guide at `channels/wecom/README.md` which does not exist either.

The config struct (`WeComConfig` at `pkg/config/config.go:684`) still exists for backward compatibility, but there is no functional channel.

### 3. `docs/channels/feishu/README.md` — Secret field storage location unclear

**Severity:** Low

The doc shows `encrypt_key` and `verification_token` in the config JSON block, but these are private fields in code (`encryptKey`, `verificationToken`) that should be stored in the security config (`~/.picoclaw/auth.json`), not directly in `config.json`.

### 4. `docs/providers.md` — Three providers listed but undocumented in channel detail

**Severity:** Low

Minimax, Avian, and Mistral are listed in the provider table (lines 25-27) and are implemented in `pkg/providers/openai_compat/provider.go`, but they have no vendor-specific example sections like the other providers do. This is cosmetic — they work via the standard `model_list` configuration.

---

## Planning/Design Documents — All Implemented

These documents describe planned work that has been **fully completed**. They serve as historical reference but no longer represent pending work.

| Document | Planned Work | Status |
|----------|-------------|--------|
| `docs/design/provider-refactoring.md` | Model-centric config via `model_list` | Implemented in `pkg/config/config.go`, `pkg/providers/factory.go` |
| `docs/design/provider-refactoring-tests.md` | Test plan for provider refactoring | All tests exist in `pkg/config/*_test.go`, `pkg/providers/factory*_test.go` |
| `docs/design/steering-spec.md` | Steering queue for real-time agent interruption | Implemented in `pkg/agent/steering.go` |
| `docs/agent-refactor/README.md` | Agent architecture consolidation | Completed across `pkg/agent/` (definition.go, loop.go, context.go, subturn.go) |
| `docs/agent-refactor/context.md` | Context window compression strategies | Implemented in `pkg/agent/context.go`, `loop_compression.go`, `context_budget.go` |
| `docs/superpowers/plans/2026-03-22-claude-code-automations.md` | Round 1 Claude Code automations (hooks, agents, skills) | All created in `.claude/` |
| `docs/superpowers/plans/2026-03-22-claude-automations-round2.md` | Round 2 automations (security-reviewer, new-channel skill, api-doc) | All created in `.claude/` |

---

## Documents Verified as Up-to-Date

| Document | Notes |
|----------|-------|
| `docs/config-versioning.md` | CurrentVersion=1, migration logic matches `pkg/config/migration.go` |
| `docs/providers.md` | All 25+ providers verified in `pkg/providers/` (minor gap: 3 new providers lack examples) |
| `docs/steering.md` | Matches `pkg/agent/steering.go` exactly |
| `docs/credential_encryption.md` | AES-256-GCM, HKDF-SHA256, SSH key paths all match `pkg/credential/` |
| `docs/sensitive_data_filtering.md` | Filter logic, defaults (enabled=true, min_length=8) match `pkg/config/` |
| `docs/tools_configuration.md` | All tools (web search, exec, cron, MCP, skills) match `pkg/tools/` |
| `docs/debug.md` | `--debug`, `--no-truncate`, tool_feedback all match implementation |
| `docs/docker.md` | docker-compose profiles, ports, env vars match `docker/docker-compose.yml` |
| `docs/hardware-compatibility.md` | Reference/informational doc, not code-dependent |
| `docs/troubleshooting.md` | Error scenarios and fixes reference correct config structure |
| `docs/spawn-tasks.md` | Heartbeat and spawn tool match `pkg/heartbeat/`, `pkg/tools/spawn.go` |
| `docs/subturn.md` | SubTurn spec fully matches `pkg/agent/subturn.go` |
| `docs/hooks/README.md` | Hook system (in-process + JSON-RPC) matches `pkg/agent/hooks.go`, `hook_mount.go`, `hook_process.go` |
| `docs/ANTIGRAVITY_AUTH.md` | Google Cloud Code Assist OAuth flow matches implementation |
| `docs/ANTIGRAVITY_USAGE.md` | Antigravity provider usage matches `pkg/providers/` |
| `docs/migration/model-list-migration.md` | Migration complete, backward compat working, matches `pkg/config/migration.go` |
| `docs/channels/dingtalk/README.md` | Matches `pkg/channels/dingtalk/` |
| `docs/channels/discord/README.md` | Matches code; minor: root-level `mention_only` field not documented |
| `docs/channels/feishu/README.md` | Matches code (see issue #3 above) |
| `docs/channels/line/README.md` | Matches `pkg/channels/line/` |
| `docs/channels/matrix/README.md` | Matches `pkg/channels/matrix/` |
| `docs/channels/onebot/README.md` | Matches `pkg/channels/onebot/` |
| `docs/channels/qq/README.md` | Matches `pkg/channels/qq/` |
| `docs/channels/slack/README.md` | Matches `pkg/channels/slack/` |
| `docs/channels/telegram/README.md` | Matches `pkg/channels/telegram/` |
| `docs/channels/weixin/README.md` | Matches `pkg/channels/weixin/` |

---

## Missing Documentation

These implemented features have no dedicated docs in `docs/channels/`:

- **WhatsApp** (`pkg/channels/whatsapp/`) — documented in `chat-apps.md` but no `docs/channels/whatsapp/README.md`
- **WhatsApp Native** (`pkg/channels/whatsapp_native/`) — documented in `chat-apps.md` but no dedicated README
- **IRC** (`pkg/channels/irc/`) — documented in `chat-apps.md` but no `docs/channels/irc/README.md`
- **Pico** (`pkg/channels/pico/`) — documented in `chat-apps.md` but no `docs/channels/pico/README.md`

These channels are documented in `chat-apps.md` but lack the per-channel README that other channels have.

---

## Recommendations

1. **Fix `docs/configuration.md` line 512:** Change `"model"` to `"model_name"` in the agent defaults example
2. **Remove or mark WeCom section in `docs/chat-apps.md`:** The channel implementation was removed; the documentation is misleading
3. **Clarify Feishu secret field storage** in `docs/channels/feishu/README.md`
4. **Consider archiving completed design docs** (move to `docs/design/archive/` or add "Status: Implemented" headers)
