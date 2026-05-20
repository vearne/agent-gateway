package adapter

import "context"

type BotAdapter interface {
	Start(ctx context.Context) error
	Stop()
	OnMessage(func(ctx context.Context, msg InboundMessage))

	SendText(ctx context.Context, parentMsgID string, text string) error

	SendCard(ctx context.Context, parentMsgID string, card CardContent) (cardMsgID string, err error)

	UpdateCard(ctx context.Context, cardMsgID string, card CardContent) error
}

type InboundMessage struct {
	MsgID     string
	ChatID    string
	Text      string
	ImageKeys []string
}

type CardContent struct {
	Tools     []ToolEntry
	Text      string
	Streaming bool
}

type ToolEntry struct {
	Name   string
	Args   string
	Result string
	Done   bool
}
