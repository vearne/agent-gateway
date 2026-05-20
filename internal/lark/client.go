package lark

import (
	"context"

	lark "github.com/larksuite/oapi-sdk-go/v3"
	"github.com/larksuite/oapi-sdk-go/v3/event/dispatcher"
	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
	larkws "github.com/larksuite/oapi-sdk-go/v3/ws"
)

type MessageHandler func(ctx context.Context, msgID, chatID string, parsed *ParsedMessage)

type Client struct {
	appID     string
	appSecret string
	api       *lark.Client
	handler   MessageHandler
}

func NewClient(appID, appSecret string) *Client {
	api := lark.NewClient(appID, appSecret)
	return &Client{
		appID:     appID,
		appSecret: appSecret,
		api:       api,
	}
}

func (c *Client) API() *lark.Client { return c.api }

func (c *Client) OnMessage(h MessageHandler) { c.handler = h }

func (c *Client) Start(ctx context.Context) error {
	dispatcher := dispatcher.NewEventDispatcher("", "").
		OnP2MessageReceiveV1(c.onEvent)

	wsClient := larkws.NewClient(
		c.appID,
		c.appSecret,
		larkws.WithEventHandler(dispatcher),
		larkws.WithAutoReconnect(true),
	)

	return wsClient.Start(ctx)
}

func (c *Client) onEvent(ctx context.Context, event *larkim.P2MessageReceiveV1) error {
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

	if !globalDedup.tryAdd(msgID) {
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

	if c.handler != nil {
		c.handler(ctx, msgID, chatID, parsed)
	}
	return nil
}
