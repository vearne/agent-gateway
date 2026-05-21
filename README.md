# agent-gateway

[![golang-ci](https://github.com/vearne/agent-gateway/actions/workflows/golang-ci.yml/badge.svg)](https://github.com/vearne/agent-gateway/actions/workflows/golang-ci.yml)

**agent-gateway** is a Go gateway that connects LLM agents ([agentscope-go](https://github.com/vearne/agentscope-go)) to instant-messaging platforms. Run one process, configure multiple bot channels, and let users chat with a DeepAgent from Lark, Telegram, Discord, Slack, or WhatsApp.

[中文文档](README_zh.md)

## Features

- **Multi-platform** — Lark (Feishu), Telegram, Discord, Slack, WhatsApp in a single binary
- **Multi-channel** — Run several bots with different personas, models, or session backends
- **Streaming replies** — Tool-use steps and text stream into a live-updating card (platform-specific UI)
- **Session persistence** — Per-chat history via JSON files or Redis; versioned sessions with `/new`
- **Config layering** — Global defaults with per-channel `agent` / `session` overrides
- **Built-in commands** — `/new`, `/clear`, `/reset`, `/help`

## Supported platforms

| Platform | Config key | Notes |
|----------|------------|-------|
| Lark (Feishu) | `lark` | WebSocket; interactive cards; group chats require @mention |
| Telegram | `telegram` | Long polling; markdown-style cards |
| Discord | `discord` | Embeds for card content |
| Slack | `slack` | Socket Mode; Block Kit |
| WhatsApp | `whatsapp` | [whatsmeow](https://github.com/tulir/whatsmeow); QR scan on first run |

## Requirements

- Go **1.25+**
- An OpenAI-compatible LLM API (or custom `base_url`)
- Platform credentials (bot tokens, Lark app ID/secret, etc.)

## Quick start

```bash
git clone https://github.com/vearne/agent-gateway.git
cd agent-gateway

# Copy and edit config
cp config.yaml config.local.yaml
# Set agent.api_key and channel credentials

make build
./agent-gateway -config config.local.yaml
```

Minimal `config.yaml`:

```yaml
agent:
  model_name: "gpt-4o"
  api_key: "your_api_key"
  system_prompt: "You are a helpful assistant."
  max_iters: 20

session:
  backend: "file"
  dir: "./sessions"

channels:
  - name: "lark-bot"
    platform: "lark"
    lark:
      app_id: "cli_xxx"
      app_secret: "your_app_secret"
```

More examples (multi-bot, Redis, all platforms): [docs/config-examples.md](docs/config-examples.md).

## Chat commands

| Command | Description |
|---------|-------------|
| `/new` | Start a new versioned session (`chatID` → `chatID_1` → …); previous history kept under old keys |
| `/clear` or `/reset` | Clear current session and return to the base session key |
| `/help` | Show available commands |

## Configuration

| Section | Purpose |
|---------|---------|
| `agent` | Global LLM defaults: model, API key, system prompt, `max_iters`, optional `base_url` |
| `session` | Global session backend: `file` (directory) or `redis` |
| `channels` | List of bot instances; each has `name`, `platform`, and platform-specific block |

Per-channel blocks can override any `agent` or `session` field. Environment variables override **global** agent settings only:

| Variable | Overrides |
|----------|-----------|
| `AGENT_API_KEY` | `agent.api_key` |
| `AGENT_MODEL_NAME` | `agent.model_name` |
| `AGENT_BASE_URL` | `agent.base_url` |
| `AGENT_MAX_ITERS` | `agent.max_iters` |

## Architecture

```
cmd/agent-gateway/main.go     Entry: load config, create bots + channels, graceful shutdown
pkg/
  adapter/                    BotAdapter, Agent, SessionStore interfaces
  channel/                    Message routing, streaming card updates, slash commands
  config/                     YAML load, validation, env overrides
  agent/                      DeepAgent factory (agentscope-go)
  session/                    File or Redis session store
  lark/ telegram/ discord/ slack/ whatsapp/   Platform adapters
```

**Message flow:** IM event → `BotAdapter` → `Channel` loads session → `ReplyStream` → card updates every 500ms → session saved after stream completes.

**Message IDs** use `channelID:messageID` (colon-separated); each adapter encodes/decodes this format.

## Development

```bash
make build    # go build ./...
make test     # go test -v -race ./...
make lint     # golangci-lint run ./...
make tidy     # go mod tidy
make fmt      # gofmt + goimports
```

## Examples

Standalone demos (single platform, minimal setup):

| Example | Run |
|---------|-----|
| Lark | `go run ./examples/standalone-lark-channel` |
| Telegram | `go run ./examples/standalone-telegram-channel` |
| Discord | `go run ./examples/standalone-discord-channel` |
| Slack | `go run ./examples/standalone-slack-channel` |
| WhatsApp | `go run ./examples/standalone-whatsapp-channel` |
| Lark + tools | `go run ./examples/lark-tool-agent` |

See each `examples/*/README.md` for platform-specific setup.

## Extending

- **New IM platform** — Implement `adapter.BotAdapter`, add cases in `cmd/agent-gateway/main.go` and `pkg/config/config.go`.
- **Custom tools** — Provide an `AgentFactory` that passes a `tool.Toolkit` to `agentscope.NewDeepAgent`; see `examples/lark-tool-agent`.

## Notes

- **WhatsApp** stores session data under `whatsapp.data_dir` (default `./whatsapp-data`); scan the QR code printed in logs on first connect.
- **Lark groups** only respond when the bot is @mentioned; duplicate WebSocket deliveries are deduplicated (600s TTL).
- Bots **fail fast** on startup if credentials or initialization are invalid (panic in some constructors).

## License

See [LICENSE](LICENSE).
