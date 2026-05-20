# Standalone Slack Channel

最小化示例：使用 `channel.New()` 一行启动 Slack Bot（Socket Mode）。

消息处理、流式 Block Kit 回复、会话管理全部由 `channel` 包内部处理，无需手写样板代码。

## 前置条件

- Go 1.25+
- Slack App（[api.slack.com/apps](https://api.slack.com/apps)）并启用 Socket Mode
- Bot Token（`xoxb-...`）与 App-Level Token（`xapp-...`，需 `connections:write` scope）
- 将 Bot 安装到工作区，并订阅 `message.im`、`message.channels` 等事件
- OpenAI 兼容的 API Key

## 环境变量

| 变量 | 必填 | 说明 |
|------|------|------|
| `SLACK_BOT_TOKEN` | 是 | Slack Bot User OAuth Token |
| `SLACK_APP_TOKEN` | 是 | Slack App-Level Token（Socket Mode） |
| `AGENT_API_KEY` | 是 | LLM API Key（也可用 `OPENAI_API_KEY`） |
| `AGENT_MODEL_NAME` | 否 | 模型名称，默认 `gpt-4o` |
| `SESSION_DIR` | 否 | 会话存储目录，默认 `./sessions-standalone-slack` |

## 运行

```bash
export SLACK_BOT_TOKEN="xoxb-..."
export SLACK_APP_TOKEN="xapp-..."
export AGENT_API_KEY="your_api_key"

go run .
```

## 用户指令

| 指令 | 作用 |
|------|------|
| `/new` | 开启新会话 |
| `/clear` `/reset` | 清空当前会话 |
| `/help` | 查看帮助 |

## 代码结构

核心只需三步：

1. `agent.NewFactory(cfg)` — 创建 LLM Agent 工厂
2. `session.NewFileStore(dir)` — 文件会话存储
3. `channel.New(name, bot, factory, store)` — 组装并启动
