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
