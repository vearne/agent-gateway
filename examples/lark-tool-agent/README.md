# Lark Tool Agent

演示如何为飞书机器人绑定自定义 Tool 的 Agent。通过实现 `adapter.AgentFactory` 接口，可以在不修改框架代码的前提下，让 Agent 拥有任意工具能力。

## 与 standalone-lark-channel 的区别

| | standalone-lark-channel | lark-tool-agent |
|---|---|---|
| Agent 工厂 | `agent.NewFactory(cfg)` (内置) | 自定义 `toolAgentFactory` (带 Toolkit) |
| 工具能力 | 无 | 天气查询、知识库搜索、Shell、文件读写 |
| 扩展方式 | 仅配置 system_prompt/model | 可注册任意自定义 Tool + MCP |

## 核心模式

```go
// 1. 定义自己的 AgentFactory，实现 adapter.AgentFactory 接口
type toolAgentFactory struct {
    toolkit *tool.Toolkit
    // ...
}

func (f *toolAgentFactory) Create() adapter.Agent {
    return agentscope.NewDeepAgent(
        agentscope.WithDeepToolkit(f.toolkit),
        // ...
    )
}

// 2. 构建 Toolkit，注册各种工具
tk := tool.NewToolkit()
tk.Register("my_tool", "...", params, handler)

// 3. 传入 channel.New — 无需修改框架
ch := channel.New("my-bot", bot, factory, store)
```

## 注册工具的三种方式

### 1. 手动注册（完整控制 schema）

```go
tk.Register("get_weather", "查询天气", map[string]interface{}{
    "type": "object",
    "properties": map[string]interface{}{
        "city": map[string]interface{}{
            "type": "string",
            "description": "城市名称",
        },
    },
    "required": []string{"city"},
}, func(ctx context.Context, args map[string]interface{}) (*tool.ToolResponse, error) {
    city := args["city"].(string)
    return tool.NewToolResponse(city + ": 晴, 28°C"), nil
})
```

### 2. Struct 自动推导 schema（推荐简单工具）

```go
type SearchArgs struct {
    Query string `json:"query" description:"搜索关键词"`
    TopK  int    `json:"top_k,omitempty" description:"返回条数"`
}

func searchHandler(_ context.Context, args SearchArgs) (*tool.ToolResponse, error) {
    return tool.NewToolResponse("搜索结果..."), nil
}

tk.RegisterFunc(searchHandler, tool.RegisterOption{
    Name:        "search_knowledge",
    Description: "搜索知识库",
})
```

### 3. 使用内置工具

```go
tool.RegisterExecuteShellCommandTool(tk)
tool.RegisterViewTextFileTool(tk)
tool.RegisterWriteTextFileTool(tk)
```

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
| `AGENT_BASE_URL` | 否 | 自定义 API 地址 |
| `SESSION_DIR` | 否 | 会话存储目录，默认 `./sessions-tool` |

## 运行

```bash
export LARK_APP_ID="cli_xxx"
export LARK_APP_SECRET="your_secret"
export AGENT_API_KEY="your_api_key"

go run ./examples/lark-tool-agent
```

## 用户指令

| 指令 | 作用 |
|------|------|
| `/new` | 开启新会话 |
| `/clear` `/reset` | 清空当前会话 |
| `/help` | 查看帮助 |

## 代码结构

- `toolAgentFactory` — 实现 `adapter.AgentFactory` 接口，创建带 Toolkit 的 Agent
- `initGetWeather` — 注册天气查询工具（手动 schema）
- `SearchKnowledgeArgs` + `searchKnowledge` — 注册知识库搜索工具（struct 自动 schema）
- `buildToolkit` — 组装所有工具，创建 Tool Group
