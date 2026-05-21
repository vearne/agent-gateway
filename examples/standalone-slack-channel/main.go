// Standalone Slack Channel Example
//
// Minimal example: start a single Slack bot (Socket Mode) using channel.New().
// All message handling, streaming block replies, and session management
// are handled by the channel package — no boilerplate needed.
//
// Usage:
//
//	export SLACK_BOT_TOKEN="xoxb-..."
//	export SLACK_APP_TOKEN="xapp-..."
//	export AGENT_API_KEY="your_openai_api_key"
//	go run ./examples/standalone-slack-channel
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
	"github.com/vearne/agent-gateway/pkg/slack"
)

func main() {
	logger, _ := zap.NewProduction()
	defer logger.Sync()
	zap.ReplaceGlobals(logger)

	botToken := os.Getenv("SLACK_BOT_TOKEN")
	appToken := os.Getenv("SLACK_APP_TOKEN")
	if botToken == "" || appToken == "" {
		logger.Fatal("set SLACK_BOT_TOKEN and SLACK_APP_TOKEN env vars")
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

	store := session.NewFileStore(envOr("SESSION_DIR", "./sessions-standalone-slack"))
	bot := slack.NewSlackBot(botToken, appToken)

	ch := channel.New("standalone-slack", bot, factory, store)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch.Start(ctx)
	logger.Info("standalone slack channel started")

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
