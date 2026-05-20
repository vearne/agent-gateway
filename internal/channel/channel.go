package channel

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"time"

	"github.com/vearne/agentscope-go/pkg/message"
	"go.uber.org/zap"

	agFactory "github.com/vearne/agent-gateway/internal/agent"
	"github.com/vearne/agent-gateway/internal/lark"
	"github.com/vearne/agent-gateway/internal/session"
)

type Channel struct {
	lark    *lark.Client
	factory *agFactory.Factory
	store   *session.Store
	locks   sync.Map
}

func New(larkClient *lark.Client, factory *agFactory.Factory, store *session.Store) *Channel {
	return &Channel{
		lark:    larkClient,
		factory: factory,
		store:   store,
	}
}

func (ch *Channel) Start(ctx context.Context) {
	ch.lark.OnMessage(ch.handleMessage)

	go func() {
		if err := ch.lark.Start(ctx); err != nil && ctx.Err() == nil {
			zap.L().Error("lark client stopped unexpectedly", zap.Error(err))
		}
	}()
}

func (ch *Channel) handleMessage(ctx context.Context, msgID, chatID string, parsed *lark.ParsedMessage) {
	if parsed.HasText {
		switch strings.TrimSpace(parsed.Text) {
		case "/new":
			ch.store.NewSession(ctx, chatID)
			lark.SendTextReply(ctx, ch.lark.API(), msgID, "✅ 已开启新会话，历史记录已清除。")
			return
		case "/clear", "/reset":
			ch.store.ResetSession(ctx, chatID)
			lark.SendTextReply(ctx, ch.lark.API(), msgID, "🔄 已清空当前会话。")
			return
		case "/help":
			lark.SendTextReply(ctx, ch.lark.API(), msgID, "📖 **可用指令**\n/new          开启新会话\n/clear /reset  清空当前会话\n/help         查看此帮助")
			return
		}
	}

	go ch.processReply(ctx, msgID, chatID, parsed)
}

func (ch *Channel) processReply(ctx context.Context, msgID, chatID string, parsed *lark.ParsedMessage) {
	mu := ch.getLock(chatID)
	mu.Lock()
	defer mu.Unlock()

	deepAgent := ch.factory.Create()

	if err := ch.store.LoadSession(ctx, chatID, deepAgent.Memory()); err != nil {
		zap.L().Warn("load session failed, starting fresh",
			zap.String("chat_id", chatID), zap.Error(err))
	}

	var contentBlocks []message.ContentBlock
	if parsed.HasText {
		contentBlocks = append(contentBlocks, message.NewTextBlock(parsed.Text))
	}
	if len(contentBlocks) == 0 {
		contentBlocks = append(contentBlocks, message.NewTextBlock(""))
	}
	agentMsg := message.NewMsg("user", contentBlocks, "user")

	larkAPI := ch.lark.API()
	cardJSON := lark.BuildCard(nil, "⏳ 思考中...", true)
	if err := lark.SendCardReply(ctx, larkAPI, msgID, cardJSON); err != nil {
		lark.SendTextReply(ctx, larkAPI, msgID, "❌ 发送卡片失败："+err.Error())
		return
	}

	streamCh, err := deepAgent.ReplyStream(ctx, agentMsg)

	if saveErr := ch.store.SaveSession(ctx, chatID, deepAgent.Memory()); saveErr != nil {
		zap.L().Error("save session failed",
			zap.String("chat_id", chatID), zap.Error(saveErr))
	}

	if err != nil {
		zap.L().Error("agent reply stream failed",
			zap.String("chat_id", chatID),
			zap.String("msg_id", msgID),
			zap.Error(err))
		errorCard := lark.BuildCard(nil, "❌ 回复失败："+err.Error(), false)
		lark.UpdateCard(ctx, larkAPI, msgID, errorCard)
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
		text := streamMsg.GetTextContent()
		tools := extractToolEntries(streamMsg)
		cardJSON := lark.BuildCard(tools, text, true)
		if updateErr := lark.UpdateCard(ctx, larkAPI, msgID, cardJSON); updateErr != nil {
			zap.L().Warn("update card during stream failed",
				zap.String("msg_id", msgID), zap.Error(updateErr))
		}
	}

	// Final card update: remove streaming cursor
	if finalResp != nil {
		text := finalResp.GetTextContent()
		tools := extractToolEntries(finalResp)
		finalCard := lark.BuildCard(tools, text, false)
		lark.UpdateCard(ctx, larkAPI, msgID, finalCard)
	}
}

func (ch *Channel) getLock(chatID string) *sync.Mutex {
	v, _ := ch.locks.LoadOrStore(chatID, &sync.Mutex{})
	return v.(*sync.Mutex)
}

func extractToolEntries(msg *message.Msg) []lark.ToolEntry {
	if msg == nil {
		return nil
	}
	var entries []lark.ToolEntry
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
			entries = append(entries, lark.ToolEntry{
				Name: name,
				Args: args,
			})
		}
	}
	return entries
}
