> Back to [README](../../../README.md)

# IRC

The IRC channel connects directly to an IRC server over TCP with optional TLS. It supports multiple channels, IRCv3 capabilities, and three authentication methods.

## Configuration

```json
{
  "channels": {
    "irc": {
      "enabled": true,
      "server": "irc.libera.chat:6697",
      "tls": true,
      "nick": "picoclaw",
      "channels": ["#mychannel"],
      "allow_from": []
    }
  }
}
```

| Field              | Type   | Required | Description                                                        |
| ------------------ | ------ | -------- | ------------------------------------------------------------------ |
| enabled            | bool   | Yes      | Whether to enable the IRC channel                                  |
| server             | string | Yes      | IRC server address in `host:port` format                           |
| tls                | bool   | No       | Enable TLS encryption                                              |
| nick               | string | Yes      | Bot nickname                                                       |
| channels           | array  | Yes      | List of channels to join (e.g. `["#mychannel"]`)                   |
| user               | string | No       | Username shown in WHOIS                                            |
| real_name          | string | No       | Real name shown in WHOIS                                           |
| password           | string | No       | Server password (stored in security config)                        |
| nickserv_password  | string | No       | NickServ identification password (stored in security config)       |
| sasl_user          | string | No       | SASL authentication username                                       |
| sasl_password      | string | No       | SASL authentication password (stored in security config)           |
| request_caps       | array  | No       | Additional IRCv3 capabilities to request                           |
| allow_from         | array  | No       | User allowlist; empty means all users are allowed                  |
| group_trigger      | object | No       | Group trigger configuration (mention/prefix-based)                 |
| typing             | object | No       | Typing notification settings (requires IRCv3 `+typing` capability) |
| reasoning_channel_id | string | No     | Target channel for reasoning output                                |

## Authentication

Three methods are supported (checked in this order):

1. **SASL** - When `sasl_user` and `sasl_password` are both set
2. **NickServ** - When `nickserv_password` is set, identifies with NickServ after connection
3. **Server password** - When `password` is set, sent during connection

## Setup

1. Choose an IRC server and note its address and port (TLS ports are typically 6697)
2. Set `server`, `nick`, and `channels` in the configuration
3. (Optional) Configure authentication using one of the methods above
4. Start PicoClaw with `picoclaw gateway`

> **Note:** IRC has a 400-character message limit. Longer responses are automatically split across multiple messages.
