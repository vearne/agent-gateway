package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"go.uber.org/zap"

	"github.com/vearne/agent-gateway/pkg/adapter"
	"github.com/vearne/agent-gateway/pkg/agent"
	"github.com/vearne/agent-gateway/pkg/channel"
	"github.com/vearne/agent-gateway/pkg/config"
	"github.com/vearne/agent-gateway/pkg/discord"
	"github.com/vearne/agent-gateway/pkg/lark"
	"github.com/vearne/agent-gateway/pkg/session"
	"github.com/vearne/agent-gateway/pkg/slack"
	"github.com/vearne/agent-gateway/pkg/telegram"
	"github.com/vearne/agent-gateway/pkg/weixin"
	"github.com/vearne/agent-gateway/pkg/whatsapp"
)

func main() {
	configPath := flag.String("config", "config.yaml", "path to config file")
	flag.Parse()

	logger, err := zap.NewProduction()
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: failed to init zap logger: %v\n", err)
		logger = zap.NewNop()
	}
	defer func() { _ = logger.Sync() }()
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

		var broker *channel.ApprovalBroker
		var factory adapter.AgentFactory
		if agentCfg.ToolApproval {
			broker = channel.NewApprovalBroker()
		}

		if broker != nil {
			hitlBot, ok := bot.(adapter.HITLAdapter)
			if ok {
				approvalFn := broker.MakeToolApprovalFunc(hitlBot)
				factory = agent.NewFactory(agentCfg, agent.WithToolApproval(approvalFn))
			} else {
				logger.Warn("tool_approval enabled but bot does not support HITL, disabling",
					zap.String("channel", chCfg.Name),
					zap.String("platform", chCfg.Platform))
				factory = agent.NewFactory(agentCfg)
				broker = nil
			}
		} else {
			factory = agent.NewFactory(agentCfg)
		}

		sessionCfg := chCfg.EffectiveSession(cfg.Session)
		store, err := createStore(sessionCfg)
		if err != nil {
			logger.Fatal("create session store failed",
				zap.String("channel", chCfg.Name),
				zap.Error(err))
		}

		ch := channel.New(chCfg.Name, bot, factory, store)
		if broker != nil {
			ch.EnableHITL(broker)
		}
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
	case "telegram":
		bot, err := telegram.NewTelegramBot(chCfg.Telegram.Token)
		if err != nil {
			return nil, err
		}
		return bot, nil
	case "discord":
		bot, err := discord.NewDiscordBot(chCfg.Discord.Token)
		if err != nil {
			return nil, err
		}
		return bot, nil
	case "slack":
		return slack.NewSlackBot(chCfg.Slack.BotToken, chCfg.Slack.AppToken), nil
	case "whatsapp":
		dataDir := "./whatsapp-data"
		if chCfg.WhatsApp != nil && chCfg.WhatsApp.DataDir != "" {
			dataDir = chCfg.WhatsApp.DataDir
		}
		return whatsapp.NewWhatsAppBot(dataDir)
	case "weixin":
		dataDir := "./weixin-data"
		if chCfg.Weixin != nil && chCfg.Weixin.DataDir != "" {
			dataDir = chCfg.Weixin.DataDir
		}
		bot, err := weixin.NewWeixinBot(dataDir)
		if err != nil {
			return nil, err
		}
		return bot, nil
	default:
		return nil, fmt.Errorf("unsupported platform: %s", chCfg.Platform)
	}
}

func createStore(cfg config.SessionConfig) (adapter.SessionStore, error) {
	switch cfg.Backend {
	case "redis":
		return session.NewRedisStore(cfg.Addr, cfg.Password, cfg.DB), nil
	default:
		fs, err := session.NewFileStore(cfg.Dir)
		if err != nil {
			return nil, fmt.Errorf("create file store: %w", err)
		}
		return fs, nil
	}
}
