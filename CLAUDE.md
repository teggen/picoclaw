# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

PicoClaw is an ultra-lightweight personal AI assistant written in Go, designed for extremely low-resource hardware (<10MB RAM). It supports 17+ messaging channels (Telegram, Discord, Slack, WhatsApp, etc.) and multiple LLM providers (OpenAI, Anthropic, Gemini, DeepSeek, Ollama, etc.). Built with CGO_ENABLED=0 for pure Go, no C dependencies.

## Build & Development Commands

```bash
make build                              # Build for current platform (runs go generate first)
make test                               # Run all tests
make check                              # Full pre-commit: deps + fmt + vet + test (run before PRs)
make lint                               # Run golangci-lint
make fmt                                # Format code
make vet                                # Static analysis
go test -run TestName -v ./pkg/session/ # Run a single test
make run ARGS="agent"                   # Build and run
make build-all                          # Cross-compile for all platforms
```

Build output goes to `build/picoclaw-{os}-{arch}`. The binary is symlinked as `build/picoclaw`.

## Architecture

### Message Flow

Channels → Channel Manager → Message Bus → Agent → Tools → Agent → Bus → Channel Manager → Channels

### Key Packages (`pkg/`)

- **`agent/`** — Agent orchestration loop: receives messages, calls LLM, executes tools, returns responses
- **`channels/`** — 17+ messaging platform adapters implementing a common `Channel` interface via `BaseChannel` embedding. `manager.go` handles lifecycle and routing, `split.go` handles message length limits
- **`providers/`** — LLM provider abstraction with factory pattern, fallback chains, and round-robin load balancing. `openai_compat/` handles OpenAI-compatible APIs
- **`tools/`** — Agent tool implementations (shell, filesystem, web, cron, MCP, I2C/SPI hardware). Registry pattern with `Name()`, `Description()`, `Parameters()`, `Execute()` interface
- **`config/`** — Configuration system with versioned migration. `config.go` is the main structure (~80KB)
- **`bus/`** — Event pub/sub message bus decoupling channels from agent
- **`memory/`** — JSONL-based long-term conversation memory
- **`session/`** — JSONL-based session state management
- **`skills/`** — Skill discovery, installation, and ClawHub registry integration
- **`api/`** — HTTP API client for ClawHub services
- **`auth/`** — Authentication and credential management for ClawHub
- **`gateway/`** — HTTP gateway server exposing channels as webhook endpoints
- **`identity/`** — User identity resolution across channels
- **`logger/`** — Structured logging utilities
- **`media/`** — Media file handling (download, conversion, transcription)
- **`voice/`** — Voice message processing (STT/TTS)
- **`mcp/`** — Model Context Protocol support
- **`metrics/`** — Lightweight runtime metrics collection
- **`health/`** — Health check and heartbeat monitoring
- **`heartbeat/`** — Periodic heartbeat signals for liveness

### Entry Points (`cmd/picoclaw/`)

CLI uses Cobra with subcommands: `onboard`, `agent`, `gateway`, `auth`, `cron`, `migrate`, `skills`, `model`, `slack`, `status`, `version`.

### Web Console (`web/`)

- `web/frontend/` — Node.js/pnpm frontend
- `web/backend/` — Go HTTP server that embeds built frontend assets
- Build with `make build-launcher`; frontend must be built first (`cd web/frontend && pnpm install && pnpm build:backend`)

### Workspace

Runtime state lives in `~/.picoclaw/workspace/` (identity, agents, user profile, memory, installed skills).

## Code Style

- Go 1.25+, module path `github.com/sipeed/picoclaw`
- Max line length: 120 characters
- Formatters: gci, gofmt, gofumpt, goimports, golines (all via `make fmt`)
- Import order: standard library, third-party, local module (enforced by gci)
- `interface{}` → `any`, `a[b:len(a)]` → `a[b:]` (enforced by gofmt rewrite rules)
- Default build tags: `goolm,stdjson`; optional: `whatsapp_native` (for native WhatsApp support, larger binary)

## Conventions

- Commit messages: imperative mood, conventional commits style, reference issues (`Fix session leak (#123)`)
- Branch naming: `fix/telegram-timeout`, `feat/ollama-provider`, `docs/contributing-guide`
- Branch off `main`, target `main` for PRs
- `make check` must pass before merging
- AI-assisted contributions are embraced; PRs must disclose AI involvement level
