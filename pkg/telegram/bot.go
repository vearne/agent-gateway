package telegram

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/vearne/agent-gateway/pkg/adapter"
)

var _ adapter.BotAdapter = (*TelegramBot)(nil)

type TelegramBot struct {
	api     *tgbotapi.BotAPI
	handler func(ctx context.Context, msg adapter.InboundMessage)
	stop    chan struct{}
}

func NewTelegramBot(token string) *TelegramBot {
	api, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		panic(fmt.Sprintf("telegram: create bot api: %v", err))
	}
	return &TelegramBot{
		api:  api,
		stop: make(chan struct{}),
	}
}

func (b *TelegramBot) Start(ctx context.Context) error {
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60
	updates := b.api.GetUpdatesChan(u)

	go func() {
		for {
			select {
			case <-b.stop:
				return
			case <-ctx.Done():
				return
			case update := <-updates:
				b.processUpdate(ctx, update)
			}
		}
	}()

	return nil
}

func (b *TelegramBot) Stop() {
	close(b.stop)
	if b.api != nil {
		b.api.StopReceivingUpdates()
	}
}

func (b *TelegramBot) OnMessage(cb func(ctx context.Context, msg adapter.InboundMessage)) {
	b.handler = cb
}

func (b *TelegramBot) SendText(ctx context.Context, parentMsgID string, text string) error {
	chatID, msgID, err := parseParentMsgID(parentMsgID)
	if err != nil {
		return fmt.Errorf("telegram: send text: %w", err)
	}

	msg := tgbotapi.NewMessage(chatID, text)
	msg.ReplyToMessageID = msgID
	_, err = b.api.Send(msg)
	if err != nil {
		return fmt.Errorf("telegram: send text: %w", err)
	}
	return nil
}

func (b *TelegramBot) SendCard(ctx context.Context, parentMsgID string, card adapter.CardContent) (string, error) {
	chatID, _, err := parseParentMsgID(parentMsgID)
	if err != nil {
		return "", fmt.Errorf("telegram: send card: %w", err)
	}

	text := formatCard(card)
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = tgbotapi.ModeMarkdownV2

	sent, err := b.api.Send(msg)
	if err != nil {
		return "", fmt.Errorf("telegram: send card: %w", err)
	}

	return fmt.Sprintf("%d:%d", sent.Chat.ID, sent.MessageID), nil
}

func (b *TelegramBot) UpdateCard(ctx context.Context, cardMsgID string, card adapter.CardContent) error {
	chatID, msgID, err := parseParentMsgID(cardMsgID)
	if err != nil {
		return fmt.Errorf("telegram: update card: %w", err)
	}

	text := formatCard(card)
	editMsg := tgbotapi.NewEditMessageText(chatID, msgID, text)
	editMsg.ParseMode = tgbotapi.ModeMarkdownV2

	_, err = b.api.Send(editMsg)
	if err != nil {
		return fmt.Errorf("telegram: update card: %w", err)
	}
	return nil
}

func (b *TelegramBot) processUpdate(ctx context.Context, update tgbotapi.Update) {
	if update.Message == nil {
		return
	}

	msg := update.Message

	if msg.From != nil && b.api != nil && msg.From.ID == b.api.Self.ID {
		return
	}

	if msg.Text == "" {
		return
	}

	chatID := strconv.FormatInt(msg.Chat.ID, 10)
	msgID := fmt.Sprintf("%d:%d", msg.Chat.ID, msg.MessageID)

	if b.handler != nil {
		b.handler(ctx, adapter.InboundMessage{
			MsgID:  msgID,
			ChatID: chatID,
			Text:   msg.Text,
		})
	}
}

func parseParentMsgID(parentMsgID string) (chatID int64, messageID int, err error) {
	parts := strings.SplitN(parentMsgID, ":", 2)
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("invalid parentMsgID format: %q", parentMsgID)
	}
	chatID, err = strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid chatID in parentMsgID: %w", err)
	}
	messageID, err = strconv.Atoi(parts[1])
	if err != nil {
		return 0, 0, fmt.Errorf("invalid messageID in parentMsgID: %w", err)
	}
	return chatID, messageID, nil
}

func formatCard(card adapter.CardContent) string {
	var sb strings.Builder

	if card.Thinking != "" {
		sb.WriteString("🧠 思考过程\n")
		sb.WriteString(card.Thinking)
		sb.WriteString("\n\n")
	}

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
