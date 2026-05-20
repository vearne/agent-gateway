# Standalone Lark Channel

最小化示例：使用 `channel.New()` 一行启动飞书 Bot。

消息处理、流式卡片回复、会话管理全部由 `channel` 包内部处理，无需手写样板代码。

## 前置条件

- Go 1.25+
- 飞书自建应用（获取 App ID 和 App Secret）
- OpenAI 兼容的 API Key

## 环境变量

| 变量 | 必填 | 说明 |
|------|------|------|
| `LARK_APP_ID` | 是 | 飞书应用 App ID |
| `LARK_APP_SECRET` | 是 | 飞书应用 App Secret |
| `AGENT_API_KEY` | 是 | LLM API Key（也可用 `OPENAI_API_KEY`） |
| `AGENT_MODEL_NAME` | 否 | 模型名称，默认 `gpt-4o` |
| `SESSION_DIR` | 否 | 会话存储目录，默认 `./sessions-standalone` |

## 运行

```bash
export LARK_APP_ID="cli_xxx"
export LARK_APP_SECRET="your_secret"
export AGENT_API_KEY="your_api_key"

go run .
```

## 用户指令

在飞书对话中发送以下指令：

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

如果需要自定义 Agent（如绑定 Tool），参考 `examples/lark-tool-agent`。
