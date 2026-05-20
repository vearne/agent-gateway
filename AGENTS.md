# AGENTS.md — agent-gateway

## What It Is

A Go gateway that connects LLM agents (backed by `agentscope-go`) to multiple IM platforms: Lark, Telegram, Discord, Slack, WhatsApp. Each platform gets its own `BotAdapter`; the `channel` package orchestrates message handling, streaming card updates, and session persistence.

## Build & Run

```bash
# Build
go build ./cmd/agent-gateway

# Run (uses config.yaml in cwd)
./agent-gateway -config config.yaml

# Run examples
go run ./examples/standalone-lark-channel
go run ./examples/lark-tool-agent
```

No Makefile, no Dockerfile, no CI, no code generation. Standard `go build` / `go run`.

## Architecture

```
cmd/agent-gateway/main.go   — entrypoint: loads config, creates bots + channels, manages lifecycle
internal/
  adapter/types.go           — core interfaces: BotAdapter, Agent, AgentFactory, SessionStore
  channel/
    channel.go               — Channel: wires bot → agent → session, handles /new /clear /help commands
    manager.go               — ChannelManager: starts/stops all channels with 10s graceful shutdown
  config/config.go           — YAML config with per-channel agent/session overrides, env var support
  agent/factory.go           — Default AgentFactory using agentscope-go DeepAgent
  session/store.go           — Session persistence: file-based (JSON) or Redis
  lark/                      — Lark (Feishu) bot: WebSocket, card streaming, message dedup, image parsing
  telegram/                  — Telegram bot: long-polling, markdown card formatting
  discord/                   — Discord bot: embed-based card representation
  slack/                     — Slack bot: Socket Mode, Block Kit formatting
  whatsapp/                  — WhatsApp bot: whatsmeow, SQLite session store, QR auth on first run
```

## Key Patterns

**Adding a new platform:** Implement `adapter.BotAdapter` (5 methods: Start, Stop, OnMessage, SendText, SendCard, UpdateCard), add a case in `main.go:createBot()` and `config/config.go:Validate()`.

**Adding tools to an agent:** Implement `adapter.AgentFactory` that passes a `tool.Toolkit` to `agentscope.NewDeepAgent(...)`. See `examples/lark-tool-agent/main.go` for the full pattern. No framework changes needed.

**Message ID format:** Composite `channelID:messageID` joined by `:`. Each platform adapter is responsible for encoding/decoding this in `parseParentMsgID()` or equivalent.

**Streaming:** The channel sends an initial card (`Streaming: true`), then updates it every 500ms from the agent's `ReplyStream` channel. Final update sets `Streaming: false`. Platform adapters render the card differently (Lark interactive card, Discord embed, Slack blocks, Telegram/Discord/WhatsApp markdown text).

**Session management:** Per-chat sessions with versioned keys (`chatID`, `chatID_1`, `chatID_2`). `/new` creates a new versioned session, `/reset` goes back to the base key. Both file and Redis backends exist.

**Lark specifics:** In group chats, only processes messages with `@mentions`. Has a message dedup cache (600s TTL, 5000 entry cap) because Lark WebSocket may deliver duplicates.

**Config layering:** Global `agent` + `session` blocks serve as defaults. Each channel can override any field via per-channel `agent:` / `session:` blocks. Environment variables (`AGENT_API_KEY`, `AGENT_MODEL_NAME`, `AGENT_BASE_URL`, `AGENT_MAX_ITERS`) override global config.

## Environment Variables

| Variable | Overrides |
|---|---|
| `AGENT_API_KEY` | `agent.api_key` |
| `AGENT_MODEL_NAME` | `agent.model_name` |
| `AGENT_BASE_URL` | `agent.base_url` |
| `AGENT_MAX_ITERS` | `agent.max_iters` |

## Dependencies

- `github.com/vearne/agentscope-go` — LLM agent framework (DeepAgent, tools, memory, session)
- `github.com/larksuite/oapi-sdk-go/v3` — Lark/Feishu SDK
- `github.com/go-telegram-bot-api/telegram-bot-api/v5` — Telegram
- `github.com/bwmarrin/discordgo` — Discord
- `github.com/slack-go/slack` — Slack (Socket Mode)
- `go.mau.fi/whatsmeow` — WhatsApp
- `go.uber.org/zap` — structured logging (set as global logger)
- `github.com/redis/go-redis/v9` — Redis session backend

## Gotchas

- **Go 1.25+** required (per go.mod).
- **No test files exist yet** — all 18 `.go` files are source code only.
- **WhatsApp** requires QR code scan on first run; stores session in SQLite under `data_dir`.
- **Telegram** `NewTelegramBot()` panics on token creation failure (not returnable error).
- **Discord/Telegram/WhatsApp** bots panic in constructors on initialization failure — this is intentional for fail-fast startup.
