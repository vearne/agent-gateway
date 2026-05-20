# Standalone WhatsApp Channel

最小化示例：使用 `channel.New()` 一行启动 WhatsApp Bot（whatsmeow）。

消息处理、流式回复、会话管理全部由 `channel` 包内部处理，无需手写样板代码。

## 前置条件

- Go 1.25+
- 可扫码登录的 WhatsApp 账号
- OpenAI 兼容的 API Key

## 环境变量

| 变量 | 必填 | 说明 |
|------|------|------|
| `AGENT_API_KEY` | 是 | LLM API Key（也可用 `OPENAI_API_KEY`） |
| `AGENT_MODEL_NAME` | 否 | 模型名称，默认 `gpt-4o` |
| `SESSION_DIR` | 否 | Agent 会话存储目录，默认 `./sessions-standalone-whatsapp` |
| `WHATSAPP_DATA_DIR` | 否 | WhatsApp 登录态 SQLite 目录，默认 `./data/whatsapp-standalone` |

## 运行

```bash
export AGENT_API_KEY="your_api_key"

go run .
```

**首次运行**会在终端打印 QR 码，用手机 WhatsApp 扫码完成配对。登录态保存在 `WHATSAPP_DATA_DIR` 下，后续启动无需再次扫码。

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
