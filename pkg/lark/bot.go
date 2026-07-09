package lark

import (
	"context"

	lark "github.com/larksuite/oapi-sdk-go/v3"
	"github.com/larksuite/oapi-sdk-go/v3/event/dispatcher"
	"github.com/larksuite/oapi-sdk-go/v3/event/dispatcher/callback"
	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
	larkws "github.com/larksuite/oapi-sdk-go/v3/ws"

	"github.com/vearne/agent-gateway/pkg/adapter"
)

var _ adapter.BotAdapter = (*LarkBot)(nil)
var _ adapter.HITLAdapter = (*LarkBot)(nil)

type LarkBot struct {
	appID             string
	appSecret         string
	api               *lark.Client
	handler           func(ctx context.Context, msg adapter.InboundMessage)
	cardActionHandler func(ctx context.Context, action adapter.CardAction)
	dedup             *MsgDedup
	wsCancel          context.CancelFunc
}

func NewLarkBot(appID, appSecret string) *LarkBot {
	return &LarkBot{
		appID:     appID,
		appSecret: appSecret,
		api:       newAPIClient(appID, appSecret),
		dedup:     NewMsgDedup(),
	}
}

func (b *LarkBot) Start(ctx context.Context) error {
	wsCtx, cancel := context.WithCancel(ctx)
	b.wsCancel = cancel

	dispatcher := dispatcher.NewEventDispatcher("", "").
		OnP2MessageReceiveV1(b.onEvent).
		OnP2CardActionTrigger(func(ctx context.Context, event *callback.CardActionTriggerEvent) (*callback.CardActionTriggerResponse, error) {
			return b.onCardActionEvent(ctx, event)
		})

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
	cardJSON := buildCardJSON(card)
	return sendCardReply(ctx, b.api, parentMsgID, cardJSON)
}

func (b *LarkBot) UpdateCard(ctx context.Context, cardMsgID string, card adapter.CardContent) error {
	cardJSON := buildCardJSON(card)
	return PatchCard(ctx, b.api, cardMsgID, cardJSON)
}

func buildCardJSON(card adapter.CardContent) string {
	return BuildCard(convertFromAdapterTools(card.Tools), card.Thinking, card.Text, card.Streaming)
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

func (b *LarkBot) SendHITLCard(ctx context.Context, parentMsgID string, payload adapter.HITLPayload) (string, error) {
	cardJSON := BuildHITLCard(payload)
	return sendCardReply(ctx, b.api, parentMsgID, cardJSON)
}

func (b *LarkBot) OnCardAction(handler func(ctx context.Context, action adapter.CardAction)) {
	b.cardActionHandler = handler
}

func (b *LarkBot) onCardActionEvent(ctx context.Context, event *callback.CardActionTriggerEvent) (*callback.CardActionTriggerResponse, error) {
	if event == nil || event.Event == nil || event.Event.Action == nil {
		return nil, nil
	}

	action := event.Event.Action
	cardMsgID := ""
	chatID := ""
	if event.Event.Context != nil {
		cardMsgID = event.Event.Context.OpenMessageID
		chatID = event.Event.Context.OpenChatID
	}

	act := ""
	values := make(map[string]string)
	if action.Value != nil {
		if v, ok := action.Value["action"]; ok {
			if s, ok := v.(string); ok {
				act = s
			}
		}
		for k, v := range action.FormValue {
			if s, ok := v.(string); ok {
				values[k] = s
			}
		}
	}

	if b.cardActionHandler != nil {
		b.cardActionHandler(ctx, adapter.CardAction{
			CardMsgID: cardMsgID,
			ChatID:    chatID,
			Action:    act,
			Values:    values,
		})
	}

	return &callback.CardActionTriggerResponse{
		Toast: &callback.Toast{
			Type:    "success",
			Content: "处理中...",
		},
	}, nil
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
