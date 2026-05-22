// Standalone Lark Channel Example
//
// Minimal example: start a single Feishu (Lark) bot using channel.New().
// All message handling, streaming card replies, and session management
// are handled by the channel package — no boilerplate needed.
//
// Usage:
//
//	export LARK_APP_ID="cli_xxx"
//	export LARK_APP_SECRET="your_secret"
//	export AGENT_API_KEY="your_openai_api_key"
//	go run ./examples/standalone-lark-channel
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/vearne/agent-gateway/pkg/agent"
	"github.com/vearne/agent-gateway/pkg/channel"
	"github.com/vearne/agent-gateway/pkg/config"
	"github.com/vearne/agent-gateway/pkg/lark"
	"github.com/vearne/agent-gateway/pkg/session"
	"github.com/vearne/agentscope-go/pkg/tool"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func main() {
	cfg := zap.NewProductionConfig()
	cfg.Level = zap.NewAtomicLevelAt(zapcore.DebugLevel) // 改为 Debug
	logger, _ := cfg.Build()
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

	tk := tool.NewToolkit()
	// --- Built-in tools ---
	if err := tool.RegisterPrintTool(tk); err != nil {
		zap.L().Fatal("RegisterPrintTool")
	}
	if err := tool.RegisterShellTool(tk); err != nil {
		zap.L().Fatal("RegisterShellTool")
	}
	factory := agent.NewToolKitFactory(
		config.AgentConfig{
			BaseURL:          envOr("OPENAI_BASE_URL", "https://api.openai.com/v1"),
			ModelName:        envOr("OPENAI_MODEL", "gpt-4o"),
			APIKey:           apiKey,
			SystemPrompt:     "你是一个有帮助的助手，请用中文回答问题。",
			MaxIters:         20,
			MaxContextTokens: 128000,
		},
		tk,
	)

	store, err := session.NewFileStore(envOr("SESSION_DIR", "./sessions-standalone"))
	if err != nil {
		logger.Fatal("create session store", zap.Error(err))
	}
	bot := lark.NewLarkBot(appID, appSecret)

	ch := channel.New("standalone-lark", bot, factory, store)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch.Start(ctx)
	logger.Info("standalone lark channel started")

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
