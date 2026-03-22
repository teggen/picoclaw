# Claude Code Automations Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Set up Claude Code automations (hooks, subagent, skills) for the PicoClaw project to streamline development workflows.

**Architecture:** All automations live under `.claude/` (agents, skills, settings) in the project root. Each is an independent config file — no code changes needed. Hooks go in `.claude/settings.json` (project-level, checked into repo). Skills and agents are markdown files in their respective directories.

**Tech Stack:** Claude Code configuration (JSON, Markdown)

**Prerequisites:** `jq` (confirmed available at `/usr/bin/jq`)

**Skipped:**
- context7 MCP server — already connected via both `claude.ai Context7` and `plugin:context7:context7`
- GitHub MCP server — `gh` CLI is not installed. Install later (`sudo apt install gh && gh auth login`) and add to `.mcp.json`

---

### Task 1: Add Project-Level Hooks in settings.json

**Files:**
- Create: `.claude/settings.json`

Note: `.claude/settings.local.json` already exists with user permissions — do NOT modify it. The new `.claude/settings.json` is the project-level config (checked into repo) for hooks.

**Important:** Claude Code hooks receive input as JSON on stdin (not environment variables). The JSON includes `tool_name` and `tool_input` (with `file_path`, `command`, etc.). Use `jq` to parse.

- [ ] **Step 1: Create `.claude/settings.json` with both hooks**

The PostToolUse hook runs `make fmt` after any `.go` file is edited or written. The PreToolUse hook blocks edits to sensitive files (.env, .key, .pem, .secret).

```json
{
  "hooks": {
    "PostToolUse": [
      {
        "matcher": "Edit|Write",
        "hooks": [
          {
            "type": "command",
            "command": "INPUT=$(cat); FILE=$(echo \"$INPUT\" | jq -r '.tool_input.file_path // \"\"' 2>/dev/null); if echo \"$FILE\" | grep -qE '\\.go$'; then make fmt 2>/dev/null; fi"
          }
        ]
      }
    ],
    "PreToolUse": [
      {
        "matcher": "Edit|Write",
        "hooks": [
          {
            "type": "command",
            "command": "INPUT=$(cat); FILE=$(echo \"$INPUT\" | jq -r '.tool_input.file_path // \"\"' 2>/dev/null); if echo \"$FILE\" | grep -qE '\\.(env|key|pem|secret)$'; then echo 'BLOCK: Refusing to edit sensitive file' >&2; exit 2; fi"
          }
        ]
      }
    ]
  }
}
```

- [ ] **Step 2: Test the auto-format hook**

Edit any `.go` file (e.g., add a blank line to `pkg/config/config.go`), then check that `make fmt` ran automatically by observing the hook output in the Claude Code session.

- [ ] **Step 3: Test the sensitive file block hook**

Attempt to edit a file named `.env` and verify Claude blocks it with exit code 2.

- [ ] **Step 4: Commit**

```bash
git add .claude/settings.json
git commit -m "chore: add Claude Code hooks for auto-format and sensitive file protection"
```

---

### Task 2: Add code-reviewer Subagent

**Files:**
- Create: `.claude/agents/code-reviewer.md`

- [ ] **Step 1: Create the agents directory and agent file**

```bash
mkdir -p .claude/agents
```

Then create `.claude/agents/code-reviewer.md`:

```markdown
---
name: code-reviewer
description: Reviews Go code for bugs, concurrency issues, and adherence to PicoClaw project conventions
tools:
  - Read
  - Grep
  - Glob
---

You are a code reviewer for PicoClaw, an ultra-lightweight Go AI assistant.

## Review Checklist

When reviewing code changes, check for:

1. **Correctness**: Logic errors, off-by-one, nil pointer dereferences
2. **Concurrency safety**: Goroutine leaks, race conditions, missing mutex locks
3. **Error handling**: Unchecked errors, swallowed errors, missing context in error wrapping
4. **Interface compliance**: Channel implementations must embed BaseChannel, tools must implement Name()/Description()/Parameters()/Execute()
5. **CGO_ENABLED=0 compatibility**: No cgo dependencies, no C bindings
6. **Logging**: Use zerolog consistently (rs/zerolog), not fmt.Println or log package
7. **Code style**: Max 120 char lines, proper import ordering (stdlib, third-party, local)
8. **Test coverage**: New code should have corresponding tests using testify

## How to Review

1. Read all changed files
2. For each file, check against the checklist above
3. Report issues with exact file:line references
4. Categorize as: BUG, SECURITY, STYLE, or SUGGESTION
5. Only report issues with high confidence — no speculation
```

- [ ] **Step 2: Verify the agent is recognized**

Run: `claude agents`
Expected: `code-reviewer` appears in the agent list

- [ ] **Step 3: Commit**

```bash
git add .claude/agents/code-reviewer.md
git commit -m "chore: add code-reviewer subagent for Go code review"
```

---

### Task 3: Add gen-test Skill

**Files:**
- Create: `.claude/skills/gen-test/SKILL.md`

- [ ] **Step 1: Create the skill directory and file**

```bash
mkdir -p .claude/skills/gen-test
```

Then create `.claude/skills/gen-test/SKILL.md`:

```markdown
---
name: gen-test
description: Generate Go tests using testify for a given package, file, or function
---

# Generate Tests

Generate Go tests for the specified target using project conventions.

## Conventions

- Use `github.com/stretchr/testify/assert` and `github.com/stretchr/testify/require`
- Test file naming: `<file>_test.go` in the same package
- Test function naming: `Test<FunctionName>` or `Test<Type>_<Method>`
- Use table-driven tests when testing multiple input/output cases
- Build tag: `stdjson` (add `//go:build stdjson` if the file under test uses it)
- Use `t.Parallel()` for independent test cases
- Test both success and error paths

## Steps

1. Read the target file to understand the functions/methods
2. Check if a test file already exists — if so, add to it rather than overwriting
3. Write tests covering:
   - Happy path for each exported function
   - Error/edge cases (nil input, empty strings, boundary values)
   - Interface compliance where applicable
4. Run the tests: `go test -run TestName -v ./pkg/<package>/`
5. Fix any compilation or assertion errors

## Usage

`/gen-test pkg/channels/telegram/telegram.go`
`/gen-test pkg/tools/shell.go:Execute`
```

- [ ] **Step 2: Verify the skill appears**

Run: `/gen-test` in Claude Code to confirm it's recognized.

- [ ] **Step 3: Commit**

```bash
git add .claude/skills/gen-test/SKILL.md
git commit -m "chore: add gen-test skill for Go test generation"
```

---

### Task 4: Add check Skill

**Files:**
- Create: `.claude/skills/check/SKILL.md`

- [ ] **Step 1: Create the skill directory and file**

```bash
mkdir -p .claude/skills/check
```

Then create `.claude/skills/check/SKILL.md`:

```markdown
---
name: check
description: Run full pre-commit checks (make check) and report results with fix suggestions
---

# Pre-Commit Check

Run the full `make check` pipeline and report results.

## What It Runs

`make check` executes in order:
1. `make deps` — verify and tidy go.mod
2. `make fmt` — format with gci, gofmt, gofumpt, goimports, golines
3. `make vet` — static analysis
4. `make test` — run all tests

## Steps

1. Run `make check` and capture full output
2. If all pass: report success with a summary
3. If any step fails:
   - Identify which step failed
   - Show the relevant error output
   - Suggest specific fixes (e.g., "run `make fmt`" or "fix compilation error in pkg/foo/bar.go:42")
   - Offer to fix automatically if possible
4. After fixes, re-run `make check` to confirm

## Usage

`/check` — run before committing or creating a PR
```

- [ ] **Step 2: Verify the skill appears**

Run: `/check` in Claude Code to confirm it's recognized.

- [ ] **Step 3: Commit**

```bash
git add .claude/skills/check/SKILL.md
git commit -m "chore: add check skill for pre-commit validation"
```

---

## Summary

| Task | What | Files |
|------|------|-------|
| 1 | Auto-format + sensitive file hooks | `.claude/settings.json` |
| 2 | code-reviewer subagent | `.claude/agents/code-reviewer.md` |
| 3 | gen-test skill | `.claude/skills/gen-test/SKILL.md` |
| 4 | check skill | `.claude/skills/check/SKILL.md` |

All tasks are independent and can be executed in any order or in parallel.
