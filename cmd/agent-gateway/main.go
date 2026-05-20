package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"go.uber.org/zap"

	"github.com/vearne/agent-gateway/internal/adapter"
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
		zap.Int("channels", len(cfg.Channels)))

	mgr := channel.NewChannelManager()

	for _, chCfg := range cfg.Channels {
		bot, err := createBot(chCfg)
		if err != nil {
			logger.Fatal("create bot failed",
				zap.String("channel", chCfg.Name),
				zap.Error(err))
		}

		agentCfg := chCfg.EffectiveAgent(cfg.Agent)
		factory := agent.NewFactory(agentCfg)

		sessionCfg := chCfg.EffectiveSession(cfg.Session)
		store := createStore(sessionCfg)

		ch := channel.New(chCfg.Name, bot, factory, store)
		mgr.Add(ch)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	mgr.Start(ctx)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		logger.Info("shutting down...")
		mgr.Stop()
		cancel()
	}()

	<-ctx.Done()
	logger.Info("agent-gateway stopped")
}

func createBot(chCfg config.ChannelConfig) (adapter.BotAdapter, error) {
	switch chCfg.Platform {
	case "lark":
		return lark.NewLarkBot(chCfg.Lark.AppID, chCfg.Lark.AppSecret), nil
	default:
		return nil, fmt.Errorf("unsupported platform: %s", chCfg.Platform)
	}
}

func createStore(cfg config.SessionConfig) *session.Store {
	switch cfg.Backend {
	case "redis":
		return session.NewRedisStore(cfg.Addr, cfg.Password, cfg.DB)
	default:
		return session.NewFileStore(cfg.Dir)
	}
}
