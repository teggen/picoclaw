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
