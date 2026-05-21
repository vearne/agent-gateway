package lark

import (
	"context"

	lark "github.com/larksuite/oapi-sdk-go/v3"
	"github.com/larksuite/oapi-sdk-go/v3/event/dispatcher"
	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
	larkws "github.com/larksuite/oapi-sdk-go/v3/ws"

	"github.com/vearne/agent-gateway/pkg/adapter"
)

var _ adapter.BotAdapter = (*LarkBot)(nil)

type LarkBot struct {
	appID     string
	appSecret string
	api       *lark.Client
	handler   func(ctx context.Context, msg adapter.InboundMessage)
	dedup     *MsgDedup
	wsCancel  context.CancelFunc
}

func NewLarkBot(appID, appSecret string) *LarkBot {
	return &LarkBot{
		appID:     appID,
		appSecret: appSecret,
		api:       lark.NewClient(appID, appSecret),
		dedup:     NewMsgDedup(),
	}
}

func (b *LarkBot) Start(ctx context.Context) error {
	wsCtx, cancel := context.WithCancel(ctx)
	b.wsCancel = cancel

	dispatcher := dispatcher.NewEventDispatcher("", "").
		OnP2MessageReceiveV1(b.onEvent)

	wsClient := larkws.NewClient(
		b.appID,
		b.appSecret,
		larkws.WithEventHandler(dispatcher),
		larkws.WithAutoReconnect(true),
	)

	return wsClient.Start(wsCtx)
}

func (b *LarkBot) Stop() {
	if b.wsCancel != nil {
		b.wsCancel()
	}
}

func (b *LarkBot) OnMessage(cb func(ctx context.Context, msg adapter.InboundMessage)) {
	b.handler = cb
}

func (b *LarkBot) SendText(ctx context.Context, parentMsgID string, text string) error {
	return SendTextReply(ctx, b.api, parentMsgID, text)
}

func (b *LarkBot) SendCard(ctx context.Context, parentMsgID string, card adapter.CardContent) (string, error) {
	cardJSON := BuildCard(convertFromAdapterTools(card.Tools), card.Text, card.Streaming)
	return sendCardReply(ctx, b.api, parentMsgID, cardJSON)
}

func (b *LarkBot) UpdateCard(ctx context.Context, cardMsgID string, card adapter.CardContent) error {
	cardJSON := BuildUpdateCard(convertFromAdapterTools(card.Tools), card.Text, card.Streaming)
	return UpdateCard(ctx, b.api, cardMsgID, cardJSON)
}

func (b *LarkBot) onEvent(ctx context.Context, event *larkim.P2MessageReceiveV1) error {
	if event == nil || event.Event == nil || event.Event.Message == nil || event.Event.Sender == nil {
		return nil
	}

	msg := event.Event.Message
	sender := event.Event.Sender

	if sender.SenderType != nil && *sender.SenderType != "user" {
		return nil
	}

	msgID := ""
	if msg.MessageId != nil {
		msgID = *msg.MessageId
	}
	if msgID == "" {
		return nil
	}

	if !b.dedup.TryAdd(msgID) {
		return nil
	}

	chatID := ""
	if msg.ChatId != nil {
		chatID = *msg.ChatId
	}

	parsed, ok := ParseMessage(msg)
	if !ok {
		return nil
	}

	if b.handler != nil {
		b.handler(ctx, adapter.InboundMessage{
			MsgID:     msgID,
			ChatID:    chatID,
			Text:      parsed.Text,
			ImageKeys: parsed.ImageKeys,
		})
	}
	return nil
}

func convertFromAdapterTools(tools []adapter.ToolEntry) []ToolEntry {
	if len(tools) == 0 {
		return nil
	}
	out := make([]ToolEntry, len(tools))
	for i, t := range tools {
		out[i] = ToolEntry{
			Name:   t.Name,
			Args:   t.Args,
			Result: t.Result,
			Done:   t.Done,
		}
	}
	return out
}
