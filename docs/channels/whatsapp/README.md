> Back to [README](../../../README.md)

# WhatsApp (Bridge)

The WhatsApp bridge channel connects to an external WhatsApp bridge service over WebSocket. This mode requires a separately running bridge process (e.g. a whatsmeow-based bridge). For a simpler setup without an external bridge, see [WhatsApp Native](../whatsapp_native/README.md).

## Configuration

```json
{
  "channels": {
    "whatsapp": {
      "enabled": true,
      "use_native": false,
      "bridge_url": "ws://localhost:8080/ws",
      "allow_from": []
    }
  }
}
```

| Field              | Type   | Required | Description                                                        |
| ------------------ | ------ | -------- | ------------------------------------------------------------------ |
| enabled            | bool   | Yes      | Whether to enable the WhatsApp channel                             |
| use_native         | bool   | No       | Set to `false` (default) for bridge mode                           |
| bridge_url         | string | Yes      | WebSocket URL of the external WhatsApp bridge                      |
| allow_from         | array  | No       | User ID whitelist; empty means all users are allowed               |
| reasoning_channel_id | string | No     | Target channel for reasoning output                                |

## Setup

1. Set up and run a WhatsApp WebSocket bridge (e.g. a whatsmeow-based bridge)
2. Note the bridge's WebSocket URL
3. Set `bridge_url` to the WebSocket URL in the configuration
4. Start PicoClaw with `picoclaw gateway`
