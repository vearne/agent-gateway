# Standalone Lark Channel

单独启动一个飞书 Channel 的最小示例，不依赖 `agent-gateway` 主程序。

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

示例展示了如何直接使用 `agent-gateway` 的内部包组装一个完整的飞书 Bot：

- `lark.NewLarkBot` — 创建飞书 Bot，通过 WebSocket 接收消息
- `agent.NewFactory` — 创建 LLM Agent 工厂
- `session.NewFileStore` — 文件会话存储
- `bot.OnMessage` — 注册消息回调，处理命令和 AI 回复
