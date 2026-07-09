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
	Thinking  string
	Text      string
	Streaming bool
}

type ToolEntry struct {
	ID     string
	Name   string
	Args   string
	Result string
	Done   bool
}

// HITLPayload carries tool approval request info for display.
type HITLPayload struct {
	ToolName string
	ToolID   string
	Args     map[string]any
}

// CardAction represents a user's interaction with an interactive card.
type CardAction struct {
	CardMsgID string            // The message ID of the card
	ChatID    string            // The chat where the card was shown
	Action    string            // "approve", "reject", "edit"
	Values    map[string]string // form values (e.g., edited args, reject reason)
}

// HITLAdapter is optionally implemented by bots that support HITL card actions.
// Bots that don't support interactive cards simply don't implement this interface.
type HITLAdapter interface {
	// SendHITLCard sends an interactive approval card and returns the card message ID.
	SendHITLCard(ctx context.Context, parentMsgID string, payload HITLPayload) (cardMsgID string, err error)
	// OnCardAction registers a handler for card action callbacks (button clicks, form submits).
	OnCardAction(handler func(ctx context.Context, action CardAction))
}
