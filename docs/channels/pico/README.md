> Back to [README](../../../README.md)

# Pico (Native Protocol)

The Pico channel is PicoClaw's native WebSocket protocol. It provides real-time bidirectional communication with support for typing notifications, message editing, and placeholder messages. It runs on the shared Gateway HTTP server.

## Configuration

```json
{
  "channels": {
    "pico": {
      "enabled": true,
      "token": "your-secret-token",
      "allow_from": []
    }
  }
}
```

| Field              | Type   | Required | Description                                                        |
| ------------------ | ------ | -------- | ------------------------------------------------------------------ |
| enabled            | bool   | Yes      | Whether to enable the Pico channel                                 |
| token              | string | Yes      | Authentication token (stored in security config)                   |
| allow_token_query  | bool   | No       | Allow token via query parameter (`?token=...`); disabled by default |
| allow_origins      | array  | No       | CORS allowed origins                                               |
| ping_interval      | int    | No       | WebSocket ping interval in seconds (default: 30)                   |
| read_timeout       | int    | No       | WebSocket read timeout in seconds (default: 60)                    |
| write_timeout      | int    | No       | WebSocket write timeout in seconds                                 |
| max_connections    | int    | No       | Maximum simultaneous WebSocket connections (default: 100)          |
| allow_from         | array  | No       | User allowlist; empty means all users are allowed                  |
| placeholder        | object | No       | Placeholder message configuration                                 |

## Authentication

Clients authenticate using the configured token via one of three methods (checked in this order):

1. **Authorization header:** `Authorization: Bearer <token>`
2. **WebSocket subprotocol:** `token.<value>`
3. **Query parameter:** `?token=<value>` (only when `allow_token_query` is `true`)

## Setup

1. Set a secret `token` in the configuration
2. Start PicoClaw with `picoclaw gateway`
3. Connect via WebSocket at `ws://<gateway.host>:<gateway.port>/pico/ws`

## Features

- Multiple simultaneous connections per session
- Message creation and editing
- Typing start/stop notifications
- Placeholder messages
- Ping/pong keep-alive
- CORS support
