# agent-gateway

[![golang-ci](https://github.com/vearne/agent-gateway/actions/workflows/golang-ci.yml/badge.svg)](https://github.com/vearne/agent-gateway/actions/workflows/golang-ci.yml)

**agent-gateway** 是一个 Go 网关，将基于 [agentscope-go](https://github.com/vearne/agentscope-go) 的 LLM Agent 接入多种即时通讯平台。单个进程可配置多个机器人通道，让用户在飞书、Telegram、Discord、Slack 或 WhatsApp 中与 DeepAgent 对话。

[English](README.md)

## 功能特性

- **多平台** — 飞书（Lark）、Telegram、Discord、Slack、WhatsApp 统一二进制
- **多通道** — 同一进程运行多个机器人，各自可配置人设、模型、会话存储
- **流式回复** — 工具调用与文本通过流式通道写入卡片，各平台以不同 UI 实时更新
- **会话持久化** — 按聊天维度保存历史，支持 JSON 文件或 Redis；`/new` 创建版本化会话
- **配置分层** — 全局 `agent` / `session` 默认值，通道级可覆盖
- **内置指令** — `/new`、`/clear`、`/reset`、`/help`

## 支持的平台

| 平台 | 配置键 | 说明 |
|------|--------|------|
| 飞书 Lark | `lark` | WebSocket；交互式卡片；群聊需 @ 机器人 |
| Telegram | `telegram` | 长轮询；Markdown 风格卡片 |
| Discord | `discord` | Embed 展示卡片内容 |
| Slack | `slack` | Socket Mode；Block Kit |
| WhatsApp | `whatsapp` | [whatsmeow](https://github.com/tulir/whatsmeow)；首次需扫码登录 |

## 环境要求

- Go **1.25+**
- 兼容 OpenAI 的 LLM API（或通过 `base_url` 指定自定义端点）
- 各平台机器人凭证（Token、飞书 App ID/Secret 等）

## 快速开始

```bash
git clone https://github.com/vearne/agent-gateway.git
cd agent-gateway

# 复制并编辑配置
cp config.yaml config.local.yaml
# 填写 agent.api_key 与各通道凭证

make build
./agent-gateway -config config.local.yaml
```

最小 `config.yaml` 示例：

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

更多示例（多机器人、Redis、全平台）：[docs/config-examples.md](docs/config-examples.md)。

## 聊天指令

| 指令 | 说明 |
|------|------|
| `/new` | 开启新的版本化会话（`chatID` → `chatID_1` → …），旧会话历史仍保留在对应 key 下 |
| `/clear` 或 `/reset` | 清空当前会话并回到基础 session key |
| `/help` | 显示可用指令 |

## 配置说明

| 配置段 | 作用 |
|--------|------|
| `agent` | 全局 LLM：模型、API Key、系统提示词、`max_iters`、可选 `base_url` |
| `session` | 全局会话存储：`file`（目录）或 `redis` |
| `channels` | 机器人列表；每项含 `name`、`platform` 及平台专用配置块 |

每个通道可通过 `agent:` / `session:` 覆盖全局字段。以下环境变量仅覆盖**全局** `agent` 配置：

| 环境变量 | 对应配置 |
|----------|----------|
| `AGENT_API_KEY` | `agent.api_key` |
| `AGENT_MODEL_NAME` | `agent.model_name` |
| `AGENT_BASE_URL` | `agent.base_url` |
| `AGENT_MAX_ITERS` | `agent.max_iters` |

## 架构概览

```
cmd/agent-gateway/main.go     入口：加载配置、创建 bot 与 channel、优雅退出
pkg/
  adapter/                    BotAdapter、Agent、SessionStore 接口
  channel/                    消息路由、流式卡片更新、斜杠指令
  config/                     YAML 加载、校验、环境变量覆盖
  agent/                      DeepAgent 工厂（agentscope-go）
  session/                    文件或 Redis 会话存储
  lark/ telegram/ discord/ slack/ whatsapp/   各平台适配器
```

**消息链路：** IM 事件 → `BotAdapter` → `Channel` 加载会话 → `ReplyStream` → 每 500ms 更新卡片 → 流结束后持久化会话。

**消息 ID** 格式为 `channelID:messageID`（冒号分隔），各适配器负责编解码。

## 开发命令

```bash
make build    # go build ./...
make test     # go test -v -race ./...
make lint     # golangci-lint run ./...
make tidy     # go mod tidy
make fmt      # gofmt + goimports
```

## 示例程序

各平台独立演示（单平台、配置最少）：

| 示例 | 运行 |
|------|------|
| 飞书 | `go run ./examples/standalone-lark-channel` |
| Telegram | `go run ./examples/standalone-telegram-channel` |
| Discord | `go run ./examples/standalone-discord-channel` |
| Slack | `go run ./examples/standalone-slack-channel` |
| WhatsApp | `go run ./examples/standalone-whatsapp-channel` |
| 飞书 + 工具 | `go run ./examples/lark-tool-agent` |

各目录下的 `README.md` 含平台专属配置说明。

## 扩展开发

- **新增 IM 平台** — 实现 `adapter.BotAdapter`，在 `cmd/agent-gateway/main.go` 与 `pkg/config/config.go` 中注册。
- **自定义工具** — 实现传入 `tool.Toolkit` 的 `AgentFactory`；参考 `examples/lark-tool-agent`。

## 注意事项

- **WhatsApp** 会话数据保存在 `whatsapp.data_dir`（默认 `./whatsapp-data`），首次连接需在日志中扫描二维码。
- **飞书群聊** 仅在 @ 机器人时响应；WebSocket 可能重复投递，内置去重（600 秒 TTL）。
- 凭证或初始化失败时，部分平台在启动阶段会 **快速失败**（构造函数 panic）。

## 许可证

见 [LICENSE](LICENSE)。
