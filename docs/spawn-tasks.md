# Heartbeat — Periodic Tasks

> Back to [README](../README.md)

The heartbeat system periodically checks `workspace/HEARTBEAT.md` for scheduled tasks. On first run, a default template is auto-generated. You can customize it to define tasks the agent should execute on each heartbeat cycle.

**Example `HEARTBEAT.md`:**

```markdown
# Heartbeat Tasks

Add tasks below the separator line. If empty, the agent responds with HEARTBEAT_OK.

---

- Check my email for important messages
- Search the web for AI news and summarize
```

**Behavior:**

- The agent reads `HEARTBEAT.md` and executes listed tasks inline using available tools.
- If no tasks are actionable, the agent responds with `HEARTBEAT_OK`.
- The agent avoids unsolicited messages unless a task explicitly requires notifying the user.

**Configuration:**

```json
{
  "heartbeat": {
    "enabled": true,
    "interval": 30
  }
}
```

| Option     | Default | Description                        |
| ---------- | ------- | ---------------------------------- |
| `enabled`  | `true`  | Enable/disable heartbeat           |
| `interval` | `30`    | Check interval in minutes (min: 5) |

**Environment variables:**

* `PICOCLAW_HEARTBEAT_ENABLED=false` to disable
* `PICOCLAW_HEARTBEAT_INTERVAL=60` to change interval
