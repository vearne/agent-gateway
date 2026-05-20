package main

import (
	"context"
	"flag"
	"os"
	"os/signal"
	"syscall"

	"go.uber.org/zap"

	"github.com/vearne/agent-gateway/internal/agent"
	"github.com/vearne/agent-gateway/internal/channel"
	"github.com/vearne/agent-gateway/internal/config"
	"github.com/vearne/agent-gateway/internal/lark"
	"github.com/vearne/agent-gateway/internal/session"
)

func main() {
	configPath := flag.String("config", "config.yaml", "path to config file")
	flag.Parse()

	logger, _ := zap.NewProduction()
	defer logger.Sync()
	zap.ReplaceGlobals(logger)

	cfg, err := config.Load(*configPath)
	if err != nil {
		logger.Fatal("load config", zap.Error(err))
	}

	logger.Info("starting agent-gateway",
		zap.String("app_id", cfg.Lark.AppID),
		zap.String("model", cfg.Agent.ModelName),
		zap.String("session_backend", cfg.Session.Backend))

	var store *session.Store
	switch cfg.Session.Backend {
	case "redis":
		store = session.NewRedisStore(cfg.Session.Addr, cfg.Session.Password, cfg.Session.DB)
	default:
		store = session.NewFileStore(cfg.Session.Dir)
	}

	larkClient := lark.NewClient(cfg.Lark.AppID, cfg.Lark.AppSecret)
	factory := agent.NewFactory(cfg.Agent)
	ch := channel.New(larkClient, factory, store)

	ctx, cancel := context.WithCancel(context.Background())
	ch.Start(ctx)
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		logger.Info("shutting down...")
		cancel()
	}()

	<-ctx.Done()
	logger.Info("agent-gateway stopped")
}
