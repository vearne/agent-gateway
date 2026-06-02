package main

import (
	"context"
	"flag"
	"os"
	"os/signal"
	"syscall"

	"go.uber.org/zap"

	"github.com/vearne/agent-gateway/pkg/agent"
	"github.com/vearne/agent-gateway/pkg/channel"
	"github.com/vearne/agent-gateway/pkg/config"
	"github.com/vearne/agent-gateway/pkg/session"
	"github.com/vearne/agent-gateway/pkg/weixin"
)

func main() {
	dataDir := flag.String("data-dir", "./weixin-data", "weixin data directory (token, sync buf)")
	apiKey := flag.String("api-key", "", "LLM API key")
	model := flag.String("model", "gpt-4o", "LLM model name")
	baseURL := flag.String("base-url", "", "LLM base URL")
	flag.Parse()

	logger, _ := zap.NewProduction()
	defer func() { _ = logger.Sync() }()
	zap.ReplaceGlobals(logger)

	bot, err := weixin.NewWeixinBot(*dataDir)
	if err != nil {
		logger.Fatal("create weixin bot", zap.Error(err))
	}

	agentCfg := config.AgentConfig{
		ModelName:        *model,
		APIKey:           *apiKey,
		BaseURL:          *baseURL,
		SystemPrompt:     "You are a helpful assistant.",
		MaxIters:         20,
		MaxContextTokens: 128000,
	}
	factory := agent.NewFactory(agentCfg)

	store, err := session.NewFileStore("./sessions")
	if err != nil {
		logger.Fatal("create session store", zap.Error(err))
	}

	ch := channel.New("weixin-standalone", bot, factory, store)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch.Start(ctx)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		logger.Info("shutting down...")
		ch.Stop()
		cancel()
	}()

	<-ctx.Done()
	logger.Info("weixin standalone stopped")
}