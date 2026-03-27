# GEMINI.md - PicoClaw Developer Context

This file provides foundational context and instructions for AI agents working on the **PicoClaw** project.

## 🚀 Project Overview

**PicoClaw** is an ultra-lightweight personal AI assistant written entirely in **Go**. It is designed to run on extremely low-resource hardware (as low as $10 boards with <10MB RAM) while supporting a wide range of LLM providers and messaging channels.

- **Core Goal:** Deliver a high-performance, low-footprint AI agent that is truly portable (single binary for RISC-V, ARM, MIPS, x86).
- **Architecture:** 
    - `cmd/picoclaw`: Main CLI entry point.
    - `pkg/agent`: Orchestration loop and core agent logic.
    - `pkg/channels`: Multi-platform adapters (Telegram, Discord, Slack, WhatsApp, etc.).
    - `pkg/providers`: LLM provider abstractions (OpenAI, Anthropic, Gemini, DeepSeek, etc.).
    - `pkg/tools`: Built-in and extensible tools (Filesystem, Shell, Web Search, MCP).
    - `pkg/mcp`: Native Model Context Protocol support.
    - `web/`: Web-based launcher and console.
- **Bootstrapping:** Much of the project's architecture migration and optimization was driven by the AI agent itself.

## 🛠 Building and Running

### Prerequisites
- Go 1.25+
- `make`
- `golangci-lint` (for linting)
- `pnpm` (for web frontend development)

### Key Commands
```bash
# Core Development
make build                # Build the core 'picoclaw' binary for the current platform
make test                 # Run all backend tests
make lint                 # Run golangci-lint
make fmt                  # Format code (gofmt, gofumpt, goimports, etc.)
make check                # Full pre-commit check (deps + fmt + vet + test) - RUN BEFORE PRs

# Web UI & Launcher
make build-launcher       # Build the web-based launcher (requires frontend build)
cd web/frontend && pnpm install && pnpm build:backend # Build frontend for embedding

# Cross-Compilation
make build-all            # Build binaries for all supported platforms (linux, darwin, windows, etc.)
make build-pi-zero        # Specialized build for Raspberry Pi Zero 2 W (32/64-bit)

# Running
./build/picoclaw onboard  # Initialize configuration and workspace (~/.picoclaw)
./build/picoclaw agent    # Start interactive chat mode
./build/picoclaw gateway  # Start the message gateway for connected channels
```

## 🏗 Development Conventions

### Code Style & Standards
- **Go Version:** 1.25+
- **CGO:** Use `CGO_ENABLED=0` (pure Go) unless absolutely necessary (e.g., native WhatsApp support).
- **Types:** Prefer `any` over `interface{}`.
- **Formatting:** Enforced via `make fmt`. Max line length is 120 characters.
- **Imports:** Standard library, third-party, then local `github.com/sipeed/picoclaw` (enforced by `gci`).
- **Build Tags:** Default is `stdjson`. Use `-tags whatsapp_native` for extended WhatsApp support.

### Architecture Patterns
- **Provider Factory:** New LLM providers should be added to `pkg/providers/` following the factory pattern.
- **Channel Interface:** Messaging adapters must implement the `Channel` interface in `pkg/channels/`.
- **Tool Registry:** Add new agent capabilities to `pkg/tools/` using the registry pattern.
- **Message Bus:** All communication between channels and the agent is decoupled via the `pkg/bus` event system.

### Testing & Validation
- **Unit Tests:** Mandatory for new features in `pkg/`.
- **Pre-PR Check:** Always run `make check` to ensure code quality and prevent regressions.

## 📂 Key File Map
- `cmd/picoclaw/main.go`: CLI entry point and subcommand registration.
- `pkg/config/config.go`: Main configuration structure and default values.
- `pkg/agent/instance.go`: The core agent "think-act" loop.
- `CLAUDE.md`: Additional technical guidance for AI-assisted development.
- `ROADMAP.md`: Current project priorities and future plans.
