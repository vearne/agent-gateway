package channel

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"time"

	"github.com/vearne/agentscope-go/pkg/message"
	"go.uber.org/zap"

	"github.com/vearne/agent-gateway/internal/adapter"
)

type Channel struct {
	name    string
	bot     adapter.BotAdapter
	factory adapter.AgentFactory
	store   adapter.SessionStore
	locks   sync.Map
	cancel  context.CancelFunc
}

func New(name string, bot adapter.BotAdapter, factory adapter.AgentFactory, store adapter.SessionStore) *Channel {
	return &Channel{
		name:    name,
		bot:     bot,
		factory: factory,
		store:   store,
	}
}

func (ch *Channel) Start(ctx context.Context) {
	ctx, ch.cancel = context.WithCancel(ctx)

	ch.bot.OnMessage(ch.handleMessage)

	go func() {
		if err := ch.bot.Start(ctx); err != nil && ctx.Err() == nil {
			zap.L().Error("bot stopped unexpectedly",
				zap.String("channel", ch.name), zap.Error(err))
		}
	}()
}

func (ch *Channel) Stop() {
	if ch.cancel != nil {
		ch.cancel()
	}
}

func (ch *Channel) handleMessage(ctx context.Context, msg adapter.InboundMessage) {
	if msg.Text != "" {
		switch strings.TrimSpace(msg.Text) {
		case "/new":
			ch.store.NewSession(ctx, msg.ChatID)
			ch.bot.SendText(ctx, msg.MsgID, "✅ 已开启新会话，历史记录已清除。")
			return
		case "/clear", "/reset":
			ch.store.ResetSession(ctx, msg.ChatID)
			ch.bot.SendText(ctx, msg.MsgID, "🔄 已清空当前会话。")
			return
		case "/help":
			ch.bot.SendText(ctx, msg.MsgID, "📖 **可用指令**\n/new          开启新会话\n/clear /reset  清空当前会话\n/help         查看此帮助")
			return
		}
	}

	go ch.processReply(ctx, msg)
}

func (ch *Channel) processReply(ctx context.Context, msg adapter.InboundMessage) {
	mu := ch.getLock(msg.ChatID)
	mu.Lock()
	defer mu.Unlock()

	deepAgent := ch.factory.Create()

	if err := ch.store.LoadSession(ctx, msg.ChatID, deepAgent.Memory()); err != nil {
		zap.L().Warn("load session failed, starting fresh",
			zap.String("chat_id", msg.ChatID), zap.Error(err))
	}

	var contentBlocks []message.ContentBlock
	if msg.Text != "" {
		contentBlocks = append(contentBlocks, message.NewTextBlock(msg.Text))
	}
	if len(contentBlocks) == 0 {
		contentBlocks = append(contentBlocks, message.NewTextBlock(""))
	}
	agentMsg := message.NewMsg("user", contentBlocks, "user")

	// Send initial card — capture cardMsgID for subsequent updates
	cardMsgID, err := ch.bot.SendCard(ctx, msg.MsgID, adapter.CardContent{
		Tools:     nil,
		Text:      "⏳ 思考中...",
		Streaming: true,
	})
	if err != nil {
		ch.bot.SendText(ctx, msg.MsgID, "❌ 发送卡片失败："+err.Error())
		return
	}

	streamCh, err := deepAgent.ReplyStream(ctx, agentMsg)

	if saveErr := ch.store.SaveSession(ctx, msg.ChatID, deepAgent.Memory()); saveErr != nil {
		zap.L().Error("save session failed",
			zap.String("chat_id", msg.ChatID), zap.Error(saveErr))
	}

	if err != nil {
		zap.L().Error("agent reply stream failed",
			zap.String("chat_id", msg.ChatID),
			zap.String("msg_id", msg.MsgID),
			zap.Error(err))
		ch.bot.UpdateCard(ctx, cardMsgID, adapter.CardContent{
			Text: "❌ 回复失败：" + err.Error(),
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
		card := adapter.CardContent{Tools: tools, Text: streamMsg.GetTextContent(), Streaming: true}
		if updateErr := ch.bot.UpdateCard(ctx, cardMsgID, card); updateErr != nil {
			zap.L().Warn("update card during stream failed",
				zap.String("msg_id", cardMsgID), zap.Error(updateErr))
		}
	}

	// Final card update: remove streaming cursor
	if finalResp != nil {
		tools := extractToolEntries(finalResp)
		ch.bot.UpdateCard(ctx, cardMsgID, adapter.CardContent{
			Tools:     tools,
			Text:      finalResp.GetTextContent(),
			Streaming: false,
		})
	}
}

func (ch *Channel) getLock(chatID string) *sync.Mutex {
	v, _ := ch.locks.LoadOrStore(chatID, &sync.Mutex{})
	return v.(*sync.Mutex)
}

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
