> Back to [README](../../../README.md)

# WhatsApp Native

The native WhatsApp channel connects in-process using [whatsmeow](https://github.com/tulir/whatsmeow) with no external bridge required. Authentication is done by scanning a QR code with the WhatsApp mobile app.

> **Build tag required:** The native channel is optional to keep the default binary small. Build with `-tags whatsapp_native` (e.g. `make build-whatsapp-native` or `go build -tags whatsapp_native ./cmd/...`).

## Configuration

```json
{
  "channels": {
    "whatsapp": {
      "enabled": true,
      "use_native": true,
      "session_store_path": "",
      "allow_from": []
    }
  }
}
```

| Field              | Type   | Required | Description                                                        |
| ------------------ | ------ | -------- | ------------------------------------------------------------------ |
| enabled            | bool   | Yes      | Whether to enable the WhatsApp channel                             |
| use_native         | bool   | Yes      | Must be `true` for native mode                                     |
| session_store_path | string | No       | Path for the SQLite session database; defaults to `<workspace>/whatsapp/` |
| allow_from         | array  | No       | User ID whitelist; empty means all users are allowed               |
| reasoning_channel_id | string | No     | Target channel for reasoning output                                |

## Setup

1. Build PicoClaw with the `whatsapp_native` tag
2. Set `use_native` to `true` in the configuration
3. Run `picoclaw gateway`
4. On first run, scan the QR code printed in the terminal with WhatsApp (Linked Devices)
5. The session is stored under `<workspace>/whatsapp/` (or the path specified in `session_store_path`)

## Features

- QR code authentication (no API keys needed)
- Direct and group chat support
- Message editing
- Typing notifications
- Placeholder messages
- Auto-reconnection with exponential backoff
