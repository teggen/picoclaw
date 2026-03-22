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
