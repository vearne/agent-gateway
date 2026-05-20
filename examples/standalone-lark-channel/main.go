// Standalone Lark Channel Example
//
// This example shows how to use a single Feishu (Lark) channel independently,
// without the full agent-gateway binary. It demonstrates:
//   - Creating a LarkBot directly
//   - Setting up an agent with a specific model and system prompt
//   - Managing conversation sessions with a file-based store
//   - Handling inbound messages with streaming card replies
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
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"go.uber.org/zap"

	"github.com/vearne/agent-gateway/internal/adapter"
	"github.com/vearne/agent-gateway/internal/agent"
	"github.com/vearne/agent-gateway/internal/config"
	"github.com/vearne/agent-gateway/internal/lark"
	"github.com/vearne/agent-gateway/internal/session"
	"github.com/vearne/agentscope-go/pkg/message"
)

func main() {
	logger, _ := zap.NewProduction()
	defer logger.Sync()
	zap.ReplaceGlobals(logger)

	// --- 1. Lark credentials from environment ---
	appID := os.Getenv("LARK_APP_ID")
	appSecret := os.Getenv("LARK_APP_SECRET")
	if appID == "" || appSecret == "" {
		logger.Fatal("set LARK_APP_ID and LARK_APP_SECRET env vars")
	}

	// --- 2. Agent config (can also load from YAML) ---
	apiKey := os.Getenv("AGENT_API_KEY")
	if apiKey == "" {
		apiKey = os.Getenv("OPENAI_API_KEY")
	}
	if apiKey == "" {
		logger.Fatal("set AGENT_API_KEY or OPENAI_API_KEY env var")
	}
	modelName := envOr("AGENT_MODEL_NAME", "gpt-4o")

	agentCfg := config.AgentConfig{
		ModelName:        modelName,
		APIKey:           apiKey,
		SystemPrompt:     "你是一个有帮助的助手，请用中文回答问题。",
		MaxIters:         20,
		MaxContextTokens: 128000,
	}

	// --- 3. Session store (file-based) ---
	sessionDir := envOr("SESSION_DIR", "./sessions-standalone")
	store := session.NewFileStore(sessionDir)

	// --- 4. Create LarkBot and wire up message handler ---
	bot := lark.NewLarkBot(appID, appSecret)
	factory := agent.NewFactory(agentCfg)

	// Per-chat locking to serialize replies for the same conversation
	var chatLocks sync.Map

	bot.OnMessage(func(ctx context.Context, msg adapter.InboundMessage) {
		handleMessage(ctx, bot, factory, store, &chatLocks, msg)
	})

	// --- 5. Start and wait for shutdown signal ---
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		if err := bot.Start(ctx); err != nil && ctx.Err() == nil {
			logger.Error("lark bot stopped", zap.Error(err))
		}
	}()
	logger.Info("standalone lark channel started")

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	logger.Info("shutting down...")
	bot.Stop()
	cancel()
	logger.Info("stopped")
}

// handleMessage processes a single inbound message: command or AI reply.
func handleMessage(
	ctx context.Context,
	bot *lark.LarkBot,
	factory *agent.Factory,
	store *session.Store,
	chatLocks *sync.Map,
	msg adapter.InboundMessage,
) {
	if msg.Text != "" {
		switch strings.TrimSpace(msg.Text) {
		case "/new":
			store.NewSession(ctx, msg.ChatID)
			bot.SendText(ctx, msg.MsgID, "New session started.")
			return
		case "/clear", "/reset":
			store.ResetSession(ctx, msg.ChatID)
			bot.SendText(ctx, msg.MsgID, "Session cleared.")
			return
		case "/help":
			bot.SendText(ctx, msg.MsgID, "Commands:\n/new   - start new session\n/clear - clear session\n/help  - show this help")
			return
		}
	}

	go processReply(ctx, bot, factory, store, chatLocks, msg)
}

// processReply runs the agent and streams the card response.
func processReply(
	ctx context.Context,
	bot *lark.LarkBot,
	factory *agent.Factory,
	store *session.Store,
	chatLocks *sync.Map,
	msg adapter.InboundMessage,
) {
	mu, _ := chatLocks.LoadOrStore(msg.ChatID, &sync.Mutex{})
	mu.(*sync.Mutex).Lock()
	defer mu.(*sync.Mutex).Unlock()

	ag := factory.Create()
	if err := store.LoadSession(ctx, msg.ChatID, ag.Memory()); err != nil {
		zap.L().Warn("load session failed", zap.String("chat_id", msg.ChatID), zap.Error(err))
	}

	var blocks []message.ContentBlock
	if msg.Text != "" {
		blocks = append(blocks, message.NewTextBlock(msg.Text))
	}
	if len(blocks) == 0 {
		blocks = append(blocks, message.NewTextBlock(""))
	}
	agentMsg := message.NewMsg("user", blocks, "user")

	cardMsgID, err := bot.SendCard(ctx, msg.MsgID, adapter.CardContent{
		Text:      "Thinking...",
		Streaming: true,
	})
	if err != nil {
		bot.SendText(ctx, msg.MsgID, "Failed to send card: "+err.Error())
		return
	}

	streamCh, err := ag.ReplyStream(ctx, agentMsg)

	// Persist session before checking stream error
	if saveErr := store.SaveSession(ctx, msg.ChatID, ag.Memory()); saveErr != nil {
		zap.L().Error("save session failed", zap.String("chat_id", msg.ChatID), zap.Error(saveErr))
	}

	if err != nil {
		bot.UpdateCard(ctx, cardMsgID, adapter.CardContent{
			Text: fmt.Sprintf("Reply failed: %v", err),
		})
		return
	}

	var lastUpdate time.Time
	var finalResp *message.Msg

	for streamMsg := range streamCh {
		finalResp = streamMsg
		if time.Since(lastUpdate) < 500*time.Millisecond {
			continue
		}
		lastUpdate = time.Now()
		tools := extractToolEntries(streamMsg)
		bot.UpdateCard(ctx, cardMsgID, adapter.CardContent{
			Tools:     tools,
			Text:      streamMsg.GetTextContent(),
			Streaming: true,
		})
	}

	// Final update: remove streaming cursor
	if finalResp != nil {
		tools := extractToolEntries(finalResp)
		bot.UpdateCard(ctx, cardMsgID, adapter.CardContent{
			Tools:     tools,
			Text:      finalResp.GetTextContent(),
			Streaming: false,
		})
	}
}

// extractToolEntries extracts tool-use blocks from an agent message.
func extractToolEntries(msg *message.Msg) []adapter.ToolEntry {
	if msg == nil {
		return nil
	}
	var entries []adapter.ToolEntry
	for _, block := range msg.Content {
		if message.IsToolUseBlock(block) {
			name := message.GetBlockToolUseName(block)
			input := message.GetBlockToolUseInput(block)
			args := "{}"
			if input != nil {
				if bs, err := json.Marshal(input); err == nil {
					args = string(bs)
				}
			}
			entries = append(entries, adapter.ToolEntry{
				Name: name,
				Args: args,
			})
		}
	}
	return entries
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
