// Standalone Telegram Channel Example
//
// Minimal example: start a single Telegram bot using channel.New().
// All message handling, streaming card replies, and session management
// are handled by the channel package — no boilerplate needed.
//
// Usage:
//
//	export TELEGRAM_BOT_TOKEN="your_bot_token"
//	export AGENT_API_KEY="your_openai_api_key"
//	go run ./examples/standalone-telegram-channel
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"go.uber.org/zap"

	"github.com/vearne/agent-gateway/pkg/agent"
	"github.com/vearne/agent-gateway/pkg/channel"
	"github.com/vearne/agent-gateway/pkg/config"
	"github.com/vearne/agent-gateway/pkg/session"
	"github.com/vearne/agent-gateway/pkg/telegram"
)

func main() {
	logger, _ := zap.NewProduction()
	defer logger.Sync()
	zap.ReplaceGlobals(logger)

	token := os.Getenv("TELEGRAM_BOT_TOKEN")
	if token == "" {
		logger.Fatal("set TELEGRAM_BOT_TOKEN env var")
	}

	apiKey := os.Getenv("AGENT_API_KEY")
	if apiKey == "" {
		apiKey = os.Getenv("OPENAI_API_KEY")
	}
	if apiKey == "" {
		logger.Fatal("set AGENT_API_KEY or OPENAI_API_KEY env var")
	}

	factory := agent.NewFactory(config.AgentConfig{
		ModelName:        envOr("AGENT_MODEL_NAME", "gpt-4o"),
		APIKey:           apiKey,
		SystemPrompt:     "你是一个有帮助的助手，请用中文回答问题。",
		MaxIters:         20,
		MaxContextTokens: 128000,
	})

	store, err := session.NewFileStore(envOr("SESSION_DIR", "./sessions-standalone-telegram"))
	if err != nil {
		logger.Fatal("create session store", zap.Error(err))
	}
	bot, err := telegram.NewTelegramBot(token)
	if err != nil {
		logger.Fatal("create telegram bot", zap.Error(err))
	}

	ch := channel.New("standalone-telegram", bot, factory, store)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch.Start(ctx)
	logger.Info("standalone telegram channel started")

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
