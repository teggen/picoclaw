# Agent Configuration

PicoClaw's agent configuration is located in the `agents.defaults` field of `config.json`.

These settings apply to all agents unless overridden in individual agent definitions under `agents.list`.

## Agent Defaults

```json
{
  "agents": {
    "defaults": {
      "workspace": "~/.picoclaw/workspace",
      "restrict_to_workspace": true,
      "allow_read_outside_workspace": false,
      "model_name": "gpt-5.4",
      "max_tokens": 32768,
      "temperature": 0.7,
      "max_tool_iterations": 50,
      "summarize_message_threshold": 20,
      "summarize_token_percent": 75
    }
  }
}
```

### General Settings

| Config                       | Type    | Default                    | Description                                               |
|------------------------------|---------|----------------------------|-----------------------------------------------------------|
| `workspace`                  | string  | `~/.picoclaw/workspace`    | Working directory for file operations                     |
| `restrict_to_workspace`      | bool    | `true`                     | Restrict file access to the workspace directory           |
| `allow_read_outside_workspace` | bool  | `false`                    | Allow reading files outside the workspace                 |
| `model_name`                 | string  | `""`                       | Name of the model to use (references `model_list`)        |
| `max_tokens`                 | int     | `32768`                    | Context window size in tokens                             |
| `temperature`                | float   | (provider default)         | Sampling temperature; omit to use the provider's default  |
| `max_tool_iterations`        | int     | `50`                       | Maximum number of tool calls per agent turn               |

### Memory Summarization Settings

When a conversation grows long, PicoClaw automatically compresses it into a summary to stay within the model's context window and reduce API costs. Two settings control when this happens:

| Config                          | Type | Default | Description                                                   |
|---------------------------------|------|---------|---------------------------------------------------------------|
| `summarize_message_threshold`   | int  | `20`    | Trigger summarization after this many messages in history     |
| `summarize_token_percent`       | int  | `75`    | Trigger summarization when estimated tokens reach this % of `max_tokens` |

#### `summarize_message_threshold`

This setting defines the **message count** at which automatic conversation summarization is triggered.

When the number of messages in a session's history exceeds this value, the agent starts a background summarization task. The summarization:

1. Keeps the **last 4 messages** intact for immediate context continuity.
2. Summarizes all older messages into a compact text summary.
3. Stores the summary and replaces the old messages, freeing up context space.

**Example**: With the default value of `20`, summarization begins once there are more than 20 messages in the conversation history.

#### `summarize_token_percent`

This is the **token usage percentage** at which summarization is triggered, regardless of message count.

When the estimated token count of the current conversation history exceeds `summarize_token_percent`% of `max_tokens`, summarization is triggered even if the message count is still below `summarize_message_threshold`.

**Example**: With defaults of `max_tokens: 32768` and `summarize_token_percent: 75`, summarization triggers when the conversation uses more than ~24,576 tokens (75% of 32,768).

#### How the two thresholds work together

Summarization is triggered when **either** condition is met:

```
len(history) > summarize_message_threshold
  OR
estimated_tokens > (max_tokens × summarize_token_percent / 100)
```

This means PicoClaw will summarize the conversation if it gets too long in terms of **message count** or **token usage**, whichever comes first. This dual-trigger ensures memory is managed proactively even with short messages that accumulate quickly in count, and also when a few very long messages exhaust the token budget.

#### Tuning recommendations

- **Increase** `summarize_message_threshold` if you want the agent to retain more raw conversation history before compressing. Useful when exact message recall is important.
- **Decrease** it if you are running on resource-constrained hardware and want to keep context short.
- **Adjust** `summarize_token_percent` based on how aggressively you want to manage the context window. A lower value (e.g. `50`) leaves more headroom for tool outputs and responses; a higher value (e.g. `90`) preserves more history at the risk of hitting the hard token limit.
- Setting `summarize_message_threshold` to a very high value (e.g. `9999`) effectively disables message-count-based summarization, relying solely on the token percentage trigger.

## Environment Variables

All agent defaults can be set via environment variables:

| Environment Variable                                            | Config Field                      |
|-----------------------------------------------------------------|-----------------------------------|
| `PICOCLAW_AGENTS_DEFAULTS_MODEL_NAME`                           | `model_name`                      |
| `PICOCLAW_AGENTS_DEFAULTS_MAX_TOKENS`                           | `max_tokens`                      |
| `PICOCLAW_AGENTS_DEFAULTS_TEMPERATURE`                          | `temperature`                     |
| `PICOCLAW_AGENTS_DEFAULTS_MAX_TOOL_ITERATIONS`                  | `max_tool_iterations`             |
| `PICOCLAW_AGENTS_DEFAULTS_SUMMARIZE_MESSAGE_THRESHOLD`          | `summarize_message_threshold`     |
| `PICOCLAW_AGENTS_DEFAULTS_SUMMARIZE_TOKEN_PERCENT`              | `summarize_token_percent`         |
| `PICOCLAW_AGENTS_DEFAULTS_WORKSPACE`                            | `workspace`                       |
| `PICOCLAW_AGENTS_DEFAULTS_RESTRICT_TO_WORKSPACE`                | `restrict_to_workspace`           |
| `PICOCLAW_AGENTS_DEFAULTS_ALLOW_READ_OUTSIDE_WORKSPACE`         | `allow_read_outside_workspace`    |
