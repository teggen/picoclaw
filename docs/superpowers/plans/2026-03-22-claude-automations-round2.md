# Claude Code Automations (Round 2) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a security-reviewer subagent, new-channel scaffolding skill, api-doc skill, and frontend auto-format hook to complement the existing Claude Code automations.

**Architecture:** All changes are config files under `.claude/` — no source code modifications. The hooks file (`.claude/settings.json`) already exists and will be modified to add a new PostToolUse entry. Skills and agents are new markdown files in their respective directories. All tasks are independent.

**Tech Stack:** Claude Code configuration (JSON, Markdown), jq (available at `/usr/bin/jq`)

**Prerequisites:**
- Existing `.claude/settings.json` with PostToolUse and PreToolUse hooks (already in place)
- Existing `.claude/agents/code-reviewer.md` (already in place)
- `npx` available for Prettier in frontend hook
- Frontend at `web/frontend/` with Prettier configured

---

## File Structure

| File | Responsibility |
|------|---------------|
| `.claude/agents/security-reviewer.md` | Security-focused code review agent for auth, credentials, API code |
| `.claude/skills/new-channel/SKILL.md` | Scaffold new messaging channel adapters following BaseChannel pattern |
| `.claude/skills/api-doc/SKILL.md` | Update/validate OpenAPI spec against gateway handler code |
| `.claude/settings.json` | Add frontend Prettier hook to existing PostToolUse array |

---

### Task 1: Add security-reviewer Subagent

**Files:**
- Create: `.claude/agents/security-reviewer.md`

- [ ] **Step 1: Create the agent file**

Create `.claude/agents/security-reviewer.md`:

```markdown
---
name: security-reviewer
description: Reviews auth, credential, and API code for security vulnerabilities in PicoClaw
tools:
  - Read
  - Grep
  - Glob
---

You are a security reviewer for PicoClaw, an AI assistant with OAuth/PKCE auth, credential storage, and an HTTP/WebSocket API gateway.

## Security-Sensitive Areas

- `pkg/auth/` — OAuth flow, PKCE, token management
- `pkg/credential/` — Key generation, credential storage
- `pkg/api/` — HTTP handlers, WebSocket endpoints, Swagger UI
- `pkg/gateway/` — Gateway routing
- `pkg/channels/` — External service integrations (Telegram, Discord, Slack, etc.)
- `pkg/tools/` — Agent tools (shell execution, filesystem access, web fetch)

## Review Checklist

1. **Credential exposure**: API keys, tokens, or secrets logged, hardcoded, or returned in responses
2. **Input validation**: Unsanitized user input in shell commands, SQL, file paths, or URLs
3. **Auth bypass**: Missing auth checks on API endpoints, token validation gaps
4. **SSRF**: Unvalidated URLs in web fetch or proxy configurations
5. **Path traversal**: File operations without path sanitization
6. **Command injection**: User input passed to shell execution without escaping
7. **Token handling**: Tokens stored in plaintext, not rotated, or leaked in logs
8. **CORS/headers**: Missing or overly permissive security headers on API responses
9. **WebSocket security**: Missing origin checks, auth on WS upgrade
10. **Timing attacks**: Non-constant-time comparisons on secrets

## How to Review

1. Read all changed files
2. For each file, check against the checklist above
3. For auth/credential code, also check the test files for coverage gaps
4. Report issues with exact file:line references
5. Categorize as: CRITICAL, HIGH, MEDIUM, or LOW
6. Only report issues with high confidence — no speculation
```

- [ ] **Step 2: Verify the agent file is valid**

Run: `cat .claude/agents/security-reviewer.md | head -5`
Expected: YAML frontmatter with `name: security-reviewer`

- [ ] **Step 3: Commit**

```bash
git add .claude/agents/security-reviewer.md
git commit -m "chore: add security-reviewer subagent for auth and API code"
```

---

### Task 2: Add new-channel Skill

**Files:**
- Create: `.claude/skills/new-channel/SKILL.md`

Reference files for understanding the pattern:
- `pkg/channels/base.go` — `BaseChannel` struct and `Channel` interface
- `pkg/channels/telegram/telegram.go` — Example adapter (embeds `*channels.BaseChannel`)
- Any channel's `init.go` — Registration pattern

- [ ] **Step 1: Create the skill directory and file**

```bash
mkdir -p .claude/skills/new-channel
```

Create `.claude/skills/new-channel/SKILL.md`:

```markdown
---
name: new-channel
description: Scaffold a new messaging channel adapter following the BaseChannel embedding pattern
---

# Scaffold a New Channel

Create a new messaging channel adapter for PicoClaw.

## Architecture

Every channel adapter follows this pattern:
- Lives in `pkg/channels/<name>/`
- Embeds `*channels.BaseChannel` (from `pkg/channels/base.go`)
- Implements the `channels.Channel` interface (defined in `pkg/channels/base.go`)
- Has an `init.go` that registers the channel with the channel manager
- Uses `pkg/bus` to publish/subscribe messages
- Uses `pkg/logger` (zerolog) for logging
- Uses `pkg/config` for configuration

## Reference

Study these files before scaffolding:
1. `pkg/channels/base.go` — BaseChannel struct, Channel interface, functional options
2. `pkg/channels/telegram/telegram.go` — Full adapter example
3. `pkg/channels/telegram/init.go` — Registration pattern
4. `pkg/config/config.go` — Where channel config structs live

## Steps

1. Read `pkg/channels/base.go` to understand the Channel interface methods
2. Read an existing adapter (e.g., `pkg/channels/telegram/`) for the pattern
3. Create directory `pkg/channels/<name>/`
4. Create `init.go` with channel registration
5. Create `<name>.go` with:
   - Struct embedding `*channels.BaseChannel`
   - Constructor following the pattern in the reference adapter (e.g., `NewTelegramChannel`)
   - All Channel interface methods (see `pkg/channels/base.go` for exact signatures)
6. Add config struct to `pkg/config/config.go` if needed
7. Create `<name>_test.go` with basic tests
8. Run `make check` to verify everything compiles and passes

## Usage

`/new-channel signal`
`/new-channel teams`
```

- [ ] **Step 2: Verify the skill file is valid**

Run: `cat .claude/skills/new-channel/SKILL.md | head -5`
Expected: YAML frontmatter with `name: new-channel`

- [ ] **Step 3: Commit**

```bash
git add .claude/skills/new-channel/SKILL.md
git commit -m "chore: add new-channel skill for scaffolding channel adapters"
```

---

### Task 3: Add api-doc Skill

**Files:**
- Create: `.claude/skills/api-doc/SKILL.md`

Reference files:
- `pkg/api/openapi.yaml` — Existing OpenAPI 3.0.3 spec
- `pkg/api/handler.go` — HTTP handler implementations
- `pkg/api/state_ws.go` — WebSocket handler

- [ ] **Step 1: Create the skill directory and file**

```bash
mkdir -p .claude/skills/api-doc
```

Create `.claude/skills/api-doc/SKILL.md`:

```markdown
---
name: api-doc
description: Update OpenAPI spec from gateway handler code and validate consistency
---

# Update API Documentation

Sync the OpenAPI spec with the current gateway handler code.

## Files

- **Spec**: `pkg/api/openapi.yaml` (OpenAPI 3.0.3)
- **Handlers**: `pkg/api/handler.go`, `pkg/api/state_ws.go`, `pkg/api/config_api.go`
- **Swagger UI**: `pkg/api/swagger.go`

## Steps

1. Read all handler files in `pkg/api/` to identify current endpoints
2. Read `pkg/api/openapi.yaml` to understand current spec
3. Compare endpoints: find any handlers not documented in the spec, or spec entries with no handler
4. For missing endpoints, add them to `openapi.yaml` following the existing style:
   - Use `$ref` for shared schemas in `components/schemas`
   - Include request/response examples where helpful
   - Add proper error responses (400, 401, 404, 500)
5. For removed endpoints, remove from spec
6. Validate the YAML is well-formed
7. Run `make check` to verify nothing is broken

## Usage

`/api-doc` — Full sync of spec with handlers
`/api-doc pkg/api/config_api.go` — Update spec for a specific handler file
```

- [ ] **Step 2: Verify the skill file is valid**

Run: `cat .claude/skills/api-doc/SKILL.md | head -5`
Expected: YAML frontmatter with `name: api-doc`

- [ ] **Step 3: Commit**

```bash
git add .claude/skills/api-doc/SKILL.md
git commit -m "chore: add api-doc skill for OpenAPI spec maintenance"
```

---

### Task 4: Add Frontend Auto-Format Hook

**Files:**
- Modify: `.claude/settings.json`

The existing file has one PostToolUse hook (Go auto-format) and one PreToolUse hook (sensitive file block). We need to add a second PostToolUse hook for frontend files.

- [ ] **Step 1: Add the frontend Prettier hook to the PostToolUse array**

The current PostToolUse array has one entry. Add a second entry that runs Prettier on TypeScript/CSS files under `web/frontend/`.

Update `.claude/settings.json` so the `PostToolUse` array becomes:

```json
"PostToolUse": [
  {
    "matcher": "Edit|Write",
    "hooks": [
      {
        "type": "command",
        "command": "INPUT=$(cat); FILE=$(echo \"$INPUT\" | jq -r '.tool_input.file_path // \"\"' 2>/dev/null); if echo \"$FILE\" | grep -qE '\\.go$'; then make fmt 2>/dev/null; fi"
      }
    ]
  },
  {
    "matcher": "Edit|Write",
    "hooks": [
      {
        "type": "command",
        "command": "INPUT=$(cat); FILE=$(echo \"$INPUT\" | jq -r '.tool_input.file_path // \"\"' 2>/dev/null); if echo \"$FILE\" | grep -qE 'web/frontend/.*\\.(ts|tsx|js|jsx|css)$'; then cd web/frontend && npx prettier --write \"$FILE\" 2>/dev/null; fi"
      }
    ]
  }
]
```

The PreToolUse section remains unchanged.

- [ ] **Step 2: Verify the JSON is valid**

Run: `jq . .claude/settings.json`
Expected: Pretty-printed JSON with no errors, showing two PostToolUse entries and one PreToolUse entry.

- [ ] **Step 3: Commit**

```bash
git add .claude/settings.json
git commit -m "chore: add frontend Prettier auto-format hook for web/frontend"
```

---

## Summary

| Task | What | Files |
|------|------|-------|
| 1 | security-reviewer subagent | `.claude/agents/security-reviewer.md` |
| 2 | new-channel scaffolding skill | `.claude/skills/new-channel/SKILL.md` |
| 3 | api-doc skill | `.claude/skills/api-doc/SKILL.md` |
| 4 | Frontend Prettier hook | `.claude/settings.json` (modify) |

All tasks are independent and can be executed in any order or in parallel.
