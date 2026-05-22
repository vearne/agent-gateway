// Lark Channel with Tools Example
//
// Demonstrates how to create a Feishu (Lark) bot whose Agent is equipped
// with custom tools (Toolkit). This is the key extensibility point:
//   - Implement adapter.AgentFactory to produce agents with any tools you want
//   - Pass it to channel.New() — no framework code changes needed
//
// Tools registered in this example:
//  1. get_weather       — custom function tool (mock weather data)
//  2. search_knowledge  — struct-based tool via tk.RegisterFunc (auto JSON schema)
//  3. execute_shell_command — builtin shell tool
//  4. view_text_file    — builtin file viewer tool
//  5. write_text_file   — builtin file writer tool
//
// Usage:
//
//	export LARK_APP_ID="cli_xxx"
//	export LARK_APP_SECRET="your_secret"
//	export AGENT_API_KEY="your_openai_api_key"
//	go run ./examples/lark-tool-agent
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"go.uber.org/zap"

	"github.com/vearne/agent-gateway/pkg/adapter"
	"github.com/vearne/agent-gateway/pkg/channel"
	"github.com/vearne/agent-gateway/pkg/lark"
	"github.com/vearne/agent-gateway/pkg/session"

	agentscope "github.com/vearne/agentscope-go/pkg/agent"
	"github.com/vearne/agentscope-go/pkg/formatter"
	"github.com/vearne/agentscope-go/pkg/memory"
	"github.com/vearne/agentscope-go/pkg/model"
	"github.com/vearne/agentscope-go/pkg/tool"
)

// ---------------------------------------------------------------------------
// Tool implementations
// ---------------------------------------------------------------------------

func initGetWeather(tk *tool.Toolkit) {
	params := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"city": map[string]interface{}{
				"type":        "string",
				"description": "City name, e.g. Beijing, Shanghai",
			},
		},
		"required": []string{"city"},
	}
	tk.Register("get_weather", "Query current weather for a city", params,
		func(_ context.Context, args map[string]interface{}) (*tool.ToolResponse, error) {
			city, _ := args["city"].(string)
			// Mock data — replace with real API in production
			weather := map[string]string{
				"Beijing":  "☀️ 晴天, 28°C, 湿度 45%, 北风 3级",
				"Shanghai": "🌤️ 多云, 32°C, 湿度 78%, 东南风 2级",
				"Shenzhen": "🌧️ 小雨, 30°C, 湿度 85%, 南风 2级",
			}
			if w, ok := weather[city]; ok {
				return tool.NewToolResponse(fmt.Sprintf("%s: %s", city, w)), nil
			}
			return tool.NewToolResponse(fmt.Sprintf("暂无 %s 的天气数据", city)), nil
		},
	)
}

type SearchKnowledgeArgs struct {
	Query string `json:"query" description:"Search query text"`
	TopK  int    `json:"top_k,omitempty" description:"Max results to return (default 3)"`
}

func searchKnowledge(_ context.Context, args SearchKnowledgeArgs) (*tool.ToolResponse, error) {
	if args.TopK <= 0 {
		args.TopK = 3
	}
	return tool.NewToolResponse(fmt.Sprintf(
		"搜索 \"%s\" 命中 %d 条结果（示例，实际对接知识库 API）:\n1. 文档A - 相关度 0.95\n2. 文档B - 相关度 0.87\n3. 文档C - 相关度 0.82",
		args.Query, args.TopK,
	)), nil
}

// ---------------------------------------------------------------------------
// Custom AgentFactory — produces agents with a pre-configured Toolkit
// ---------------------------------------------------------------------------

type toolAgentFactory struct {
	modelName    string
	apiKey       string
	baseURL      string
	systemPrompt string
	maxIters     int
	toolkit      *tool.Toolkit
}

func (f *toolAgentFactory) Create() adapter.Agent {
	m := model.NewOpenAIChatModel(f.modelName, f.apiKey, f.baseURL, true)
	mem := memory.NewInMemoryMemory()

	return agentscope.NewDeepAgent(
		agentscope.WithDeepName("tool-agent"),
		agentscope.WithDeepModel(m),
		agentscope.WithDeepFormatter(formatter.NewOpenAIChatFormatter()),
		agentscope.WithDeepMemory(mem),
		agentscope.WithDeepSystemPrompt(f.systemPrompt),
		agentscope.WithDeepMaxIters(f.maxIters),
		agentscope.WithDeepToolkit(f.toolkit),
	)
}

func buildToolkit() *tool.Toolkit {
	tk := tool.NewToolkit()

	// 1) Custom function tool
	initGetWeather(tk)

	// 2) Struct-based tool — RegisterFunc auto-generates JSON schema from struct tags
	tk.RegisterFunc(searchKnowledge, tool.RegisterOption{
		Name:        "search_knowledge",
		Description: "Search internal knowledge base for relevant documents",
	})

	// 3) Builtin tools
	_ = tool.RegisterExecuteShellCommandTool(tk)
	_ = tool.RegisterViewTextFileTool(tk)
	_ = tool.RegisterWriteTextFileTool(tk)

	// 4) Tool groups — disable shell by default for safety, can be toggled at runtime
	tk.CreateToolGroup("dangerous", "Shell commands — use with caution", false,
		"Shell tools are disabled by default for security. Enable only if the user explicitly requests code execution.")

	return tk
}

// ---------------------------------------------------------------------------
// Main
// ---------------------------------------------------------------------------

func main() {
	logger, _ := zap.NewProduction()
	defer logger.Sync()
	zap.ReplaceGlobals(logger)

	appID := os.Getenv("LARK_APP_ID")
	appSecret := os.Getenv("LARK_APP_SECRET")
	if appID == "" || appSecret == "" {
		logger.Fatal("set LARK_APP_ID and LARK_APP_SECRET env vars")
	}

	apiKey := os.Getenv("AGENT_API_KEY")
	if apiKey == "" {
		apiKey = os.Getenv("OPENAI_API_KEY")
	}
	if apiKey == "" {
		logger.Fatal("set AGENT_API_KEY or OPENAI_API_KEY env var")
	}

	tk := buildToolkit()
	skillPrompt := tk.GetAgentSkillPrompt()

	sysPrompt := "你是一个有帮助的助手，可以使用工具来回答问题。请用中文回答。"
	if skillPrompt != "" {
		sysPrompt += "\n\n" + skillPrompt
	}

	factory := &toolAgentFactory{
		modelName:    envOr("AGENT_MODEL_NAME", "gpt-4o"),
		apiKey:       apiKey,
		baseURL:      os.Getenv("AGENT_BASE_URL"),
		systemPrompt: sysPrompt,
		maxIters:     20,
		toolkit:      tk,
	}

	store, err := session.NewFileStore(envOr("SESSION_DIR", "./sessions-tool"))
	if err != nil {
		logger.Fatal("create session store", zap.Error(err))
	}

	bot := lark.NewLarkBot(appID, appSecret)
	ch := channel.New("lark-tool-bot", bot, factory, store)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch.Start(ctx)
	logger.Info("lark-tool-agent started",
		zap.Strings("tools", tk.GetToolNames()))

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	logger.Info("shutting down...")
	ch.Stop()
	cancel()
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
