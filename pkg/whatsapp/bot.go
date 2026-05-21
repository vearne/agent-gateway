package whatsapp

import (
	"context"
	"fmt"
	"os"
	"strings"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/store/sqlstore"
	waLog "go.mau.fi/whatsmeow/util/log"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waE2E "go.mau.fi/whatsmeow/proto/waE2E"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"

	"github.com/vearne/agent-gateway/pkg/adapter"
)

var _ adapter.BotAdapter = (*WhatsAppBot)(nil)

type WhatsAppBot struct {
	client  *whatsmeow.Client
	handler func(ctx context.Context, msg adapter.InboundMessage)
}

func NewWhatsAppBot(dataDir string) *WhatsAppBot {
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		panic(fmt.Sprintf("whatsapp: create data dir: %v", err))
	}

	logger := waLog.Stdout("Database", "INFO", true)
	container, err := sqlstore.New(context.Background(), "sqlite3", "file:"+dataDir+"/whatsmeow.db?_foreign_keys=on", logger)
	if err != nil {
		panic(fmt.Sprintf("whatsapp: create sqlstore: %v", err))
	}

	deviceStore, err := container.GetFirstDevice(context.Background())
	if err != nil {
		panic(fmt.Sprintf("whatsapp: get device: %v", err))
	}

	client := whatsmeow.NewClient(deviceStore, waLog.Stdout("Client", "INFO", true))
	client.EnableAutoReconnect = true

	return &WhatsAppBot{client: client}
}

func (b *WhatsAppBot) Start(ctx context.Context) error {
	b.client.AddEventHandler(b.handleEvent)

	if b.client.Store.ID == nil {
		qrChan, err := b.client.GetQRChannel(ctx)
		if err != nil {
			return fmt.Errorf("whatsapp: get qr channel: %w", err)
		}

		if err := b.client.Connect(); err != nil {
			return fmt.Errorf("whatsapp: connect: %w", err)
		}

		for evt := range qrChan {
			if evt.Event == "code" {
				zap.L().Info("WhatsApp QR code — scan with your phone", zap.String("qr", evt.Code))
			}
			if evt.Event == "success" {
				break
			}
		}
	} else if err := b.client.Connect(); err != nil {
		return fmt.Errorf("whatsapp: connect: %w", err)
	}

	<-ctx.Done()
	b.client.Disconnect()
	return nil
}

func (b *WhatsAppBot) Stop() {
	if b.client != nil {
		b.client.Disconnect()
	}
}

func (b *WhatsAppBot) OnMessage(cb func(ctx context.Context, msg adapter.InboundMessage)) {
	b.handler = cb
}

func (b *WhatsAppBot) SendText(ctx context.Context, parentMsgID string, text string) error {
	chatJID, _, err := parseMsgID(parentMsgID)
	if err != nil {
		return fmt.Errorf("whatsapp: send text: %w", err)
	}

	_, err = b.client.SendMessage(ctx, chatJID, &waE2E.Message{
		Conversation: proto.String(text),
	})
	if err != nil {
		return fmt.Errorf("whatsapp: send text: %w", err)
	}
	return nil
}

func (b *WhatsAppBot) SendCard(ctx context.Context, parentMsgID string, card adapter.CardContent) (string, error) {
	chatJID, _, err := parseMsgID(parentMsgID)
	if err != nil {
		return "", fmt.Errorf("whatsapp: send card: %w", err)
	}

	text := formatCard(card)
	resp, err := b.client.SendMessage(ctx, chatJID, &waE2E.Message{
		Conversation: proto.String(text),
	})
	if err != nil {
		return "", fmt.Errorf("whatsapp: send card: %w", err)
	}

	return chatJID.String() + ":" + resp.ID, nil
}

func (b *WhatsAppBot) UpdateCard(ctx context.Context, cardMsgID string, card adapter.CardContent) error {
	chatJID, msgID, err := parseMsgID(cardMsgID)
	if err != nil {
		return fmt.Errorf("whatsapp: update card: %w", err)
	}

	text := formatCard(card)
	editedMsg := b.client.BuildEdit(chatJID, msgID, &waE2E.Message{
		Conversation: proto.String(text),
	})

	_, err = b.client.SendMessage(ctx, chatJID, editedMsg)
	if err != nil {
		return fmt.Errorf("whatsapp: update card: %w", err)
	}
	return nil
}

func (b *WhatsAppBot) handleEvent(evt interface{}) {
	v, ok := evt.(*events.Message)
	if !ok {
		return
	}

	if v.Info.IsFromMe {
		return
	}

	text := v.Message.GetConversation()
	if text == "" {
		return
	}

	chatJID := v.Info.Chat.String()
	msgID := chatJID + ":" + v.Info.ID

	if b.handler != nil {
		b.handler(context.Background(), adapter.InboundMessage{
			MsgID:  msgID,
			ChatID: chatJID,
			Text:   text,
		})
	}
}

func parseMsgID(id string) (types.JID, string, error) {
	idx := strings.LastIndex(id, ":")
	if idx < 0 {
		return types.JID{}, "", fmt.Errorf("invalid msgID format: %q", id)
	}
	chatJID, err := types.ParseJID(id[:idx])
	if err != nil {
		return types.JID{}, "", fmt.Errorf("invalid chat JID in msgID: %w", err)
	}
	return chatJID, id[idx+1:], nil
}

func formatCard(card adapter.CardContent) string {
	var sb strings.Builder

	for _, tool := range card.Tools {
		if tool.Done {
			sb.WriteString("✅ ")
		} else {
			sb.WriteString("⏳ ")
		}
		sb.WriteString(tool.Name)
		sb.WriteString("\n")

		if tool.Args != "" {
			sb.WriteString("Args: ")
			sb.WriteString(tool.Args)
			sb.WriteString("\n")
		}
		if tool.Result != "" {
			sb.WriteString("Result: ")
			sb.WriteString(tool.Result)
			sb.WriteString("\n")
		}
	}

	if card.Text != "" {
		if sb.Len() > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString(card.Text)
	}

	if card.Streaming {
		sb.WriteString("▌")
	}

	return sb.String()
}
