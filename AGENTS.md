# Repository Guidelines

## Project Structure & Module Organization
`cmd/` contains entrypoints: `cmd/picoclaw` for the main CLI and `cmd/picoclaw-launcher-tui` for the TUI launcher. Core application code lives in `pkg/` and is organized by domain (`pkg/agent`, `pkg/tools`, `pkg/channels`, `pkg/api`, `pkg/config`). The desktop/web launcher is split between `web/backend` (Go) and `web/frontend` (Vite + React + TypeScript). Docs live in `docs/`, sample configs in `config/`, helper scripts in `scripts/`, and media/demo assets in `assets/`.

## Build, Test, and Development Commands
Use `make deps` to download and verify Go modules. Use `make build` to build the main binary into `build/`, `make build-cli` for the CLI client, and `make build-launcher` for the web launcher. Run `make test` for the full Go test suite plus web checks, `make vet` for static analysis, `make lint` for `golangci-lint`, and `make check` before opening a PR. For frontend-only work, use `cd web/frontend && pnpm dev`, `pnpm build`, `pnpm lint`, and `pnpm check`.

## Coding Style & Naming Conventions
Go code should stay `gofmt`/`gofumpt` clean and pass the formatter stack configured in `.golangci.yaml`; prefer short, lower-case package names and colocate tests with implementation. TypeScript and React code in `web/frontend` use 2-space indentation via `.editorconfig`, ESLint, and Prettier. Follow existing naming patterns: exported Go identifiers in `PascalCase`, internal helpers in `camelCase`, and test files ending in `_test.go`. Do not hand-edit generated files such as `web/frontend/src/routeTree.gen.ts`.

## Testing Guidelines
Most coverage is Go-first and lives next to source files across `cmd/` and `pkg/`. Name tests with Go’s standard `*_test.go` pattern and prefer table-driven tests for command, provider, and channel behavior. Run `make test` for repository-wide coverage; use `go test ./pkg/...` or `go test ./cmd/...` for narrower iterations. Frontend changes should at minimum pass `pnpm lint` because there is no separate frontend test suite yet.

## Commit & Pull Request Guidelines
Recent commits use short, imperative subjects such as `Fix security vulnerabilities and session reliability from code review` and `Add channel test coverage`. Keep commit titles concise, capitalized, and action-oriented. PRs should describe the user-visible change, note affected areas (`pkg/agent`, `web/frontend`, channel integrations, etc.), link related issues, and include screenshots for Web UI changes. Always mention the verification you ran.
