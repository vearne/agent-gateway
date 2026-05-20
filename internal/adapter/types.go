package adapter

import (
	"context"

	"github.com/vearne/agentscope-go/pkg/memory"
	"github.com/vearne/agentscope-go/pkg/message"
)

// BotAdapter is the interface that all IM platform bots must implement.
type BotAdapter interface {
	Start(ctx context.Context) error
	Stop()
	OnMessage(func(ctx context.Context, msg InboundMessage))

	SendText(ctx context.Context, parentMsgID string, text string) error

	SendCard(ctx context.Context, parentMsgID string, card CardContent) (cardMsgID string, err error)

	UpdateCard(ctx context.Context, cardMsgID string, card CardContent) error
}

// Agent is the minimal interface that channel needs from an LLM agent.
type Agent interface {
	Memory() memory.MemoryBase
	ReplyStream(ctx context.Context, msg *message.Msg) (<-chan *message.Msg, error)
}

// AgentFactory creates new Agent instances.
type AgentFactory interface {
	Create() Agent
}

// SessionStore manages conversation sessions (persist/load/reset).
type SessionStore interface {
	NewSession(ctx context.Context, chatID string) string
	LoadSession(ctx context.Context, chatID string, mem memory.MemoryBase) error
	SaveSession(ctx context.Context, chatID string, mem memory.MemoryBase) error
	ResetSession(ctx context.Context, chatID string)
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
