package weixin

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/vearne/agent-gateway/pkg/adapter"
)

var _ adapter.BotAdapter = (*WeixinBot)(nil)

type cardInfo struct {
	chatID       string
	contextToken string
}

type WeixinBot struct {
	api     *API
	store   *Store
	handler func(ctx context.Context, msg adapter.InboundMessage)
	stopCh  chan struct{}

	mu       sync.Mutex
	toolSent map[string]map[string]bool
	cards    map[string]*cardInfo
}

func NewWeixinBot(dataDir string) (*WeixinBot, error) {
	store := NewStore(dataDir)
	api := NewAPI(DefaultBaseURL, DefaultCDNBaseURL)

	if store.IsConfigured() {
		acct := store.GetAccount()
		api.SetToken(acct.Token)
		if acct.BaseURL != "" {
			api.SetBaseURL(acct.BaseURL)
		}
	}

	return &WeixinBot{
		api:      api,
		store:    store,
		stopCh:   make(chan struct{}),
		toolSent: make(map[string]map[string]bool),
		cards:    make(map[string]*cardInfo),
	}, nil
}

func (b *WeixinBot) Start(ctx context.Context) error {
	if !b.store.IsConfigured() {
		if err := b.qrLogin(ctx); err != nil {
			return fmt.Errorf("weixin: qr login: %w", err)
		}
	}

	_ = b.api.NotifyStart(ctx)
	go b.pollLoop(ctx)
	return nil
}

func (b *WeixinBot) Stop() {
	close(b.stopCh)
	_ = b.api.NotifyStop(context.Background())
}

func (b *WeixinBot) OnMessage(cb func(ctx context.Context, msg adapter.InboundMessage)) {
	b.handler = cb
}

func (b *WeixinBot) pollLoop(ctx context.Context) {
	syncBuf := b.store.GetSyncBuf()
	for {
		select {
		case <-b.stopCh:
			return
		case <-ctx.Done():
			return
		default:
		}

		resp, err := b.api.GetUpdates(ctx, syncBuf)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			zap.L().Warn("weixin getupdates failed", zap.Error(err))
			time.Sleep(2 * time.Second)
			continue
		}

		if resp.GetUpdatesBuf != "" {
			_ = b.store.SaveSyncBuf(resp.GetUpdatesBuf)
			syncBuf = resp.GetUpdatesBuf
		}

		for _, msg := range resp.Msgs {
			if msg.MessageType != MessageTypeUser {
				continue
			}
			b.processMessage(ctx, msg)
		}
	}
}

func (b *WeixinBot) processMessage(ctx context.Context, msg *WeixinMessage) {
	text := extractText(msg)

	if msg.ContextToken != "" {
		b.store.SetContextToken(msg.FromUserID, msg.ContextToken)
	}

	msgID := fmt.Sprintf("weixin:%s:%d", msg.FromUserID, msg.MessageID)
	chatID := msg.FromUserID

	if b.handler != nil && text != "" {
		b.handler(ctx, adapter.InboundMessage{
			MsgID:  msgID,
			ChatID: chatID,
			Text:   text,
		})
	}
}

func (b *WeixinBot) SendText(ctx context.Context, parentMsgID string, text string) error {
	chatID, contextToken := b.parseParentInfo(parentMsgID)
	return b.sendTextMessage(ctx, chatID, text, contextToken)
}

func (b *WeixinBot) SendCard(ctx context.Context, parentMsgID string, card adapter.CardContent) (string, error) {
	chatID, contextToken := b.parseParentInfo(parentMsgID)
	cardMsgID := generateClientID()

	msg := &WeixinMessage{
		ToUserID:     chatID,
		ClientID:     cardMsgID,
		MessageType:  MessageTypeBot,
		MessageState: MessageStateGenerating,
		ContextToken: contextToken,
		ItemList: []*MessageItem{
			{Type: ItemText, TextItem: &TextItem{Text: "⏳ 思考中..."}},
		},
	}
	if err := b.api.SendMessage(ctx, msg); err != nil {
		return "", fmt.Errorf("weixin: send card: %w", err)
	}

	b.mu.Lock()
	b.toolSent[cardMsgID] = make(map[string]bool)
	b.cards[cardMsgID] = &cardInfo{chatID: chatID, contextToken: contextToken}
	b.mu.Unlock()

	return cardMsgID, nil
}

func (b *WeixinBot) UpdateCard(ctx context.Context, cardMsgID string, card adapter.CardContent) error {
	b.mu.Lock()
	info, ok := b.cards[cardMsgID]
	if !ok {
		b.mu.Unlock()
		return fmt.Errorf("weixin: unknown card %s", cardMsgID)
	}
	chatID := info.chatID
	contextToken := info.contextToken
	sent := b.toolSent[cardMsgID]
	b.mu.Unlock()

	for _, tool := range card.Tools {
		if tool.Done {
			key := tool.ID
			if sent[key] {
				continue
			}
			item := &MessageItem{
				Type: ItemToolCallResult,
				ToolCallResultItem: &ToolCallResultItem{
					ToolName:   tool.Name,
					ToolCallID: tool.ID,
					Status:     "completed",
				},
			}
			if err := b.sendItemMessage(ctx, chatID, item, contextToken); err != nil {
				zap.L().Warn("weixin: send tool result failed", zap.Error(err))
			}
			b.mu.Lock()
			if b.toolSent[cardMsgID] != nil {
				b.toolSent[cardMsgID][key] = true
			}
			b.mu.Unlock()
		} else if tool.Name != "" {
			key := "start_" + tool.ID
			if sent[key] {
				continue
			}
			item := &MessageItem{
				Type: ItemToolCallStart,
				ToolCallStartItem: &ToolCallStartItem{
					ToolName:   tool.Name,
					ToolCallID: tool.ID,
				},
			}
			if err := b.sendItemMessage(ctx, chatID, item, contextToken); err != nil {
				zap.L().Warn("weixin: send tool start failed", zap.Error(err))
			}
			b.mu.Lock()
			if b.toolSent[cardMsgID] != nil {
				b.toolSent[cardMsgID][key] = true
			}
			b.mu.Unlock()
		}
	}

	if !card.Streaming && card.Text != "" {
		msg := &WeixinMessage{
			ToUserID:     chatID,
			MessageType:  MessageTypeBot,
			MessageState: MessageStateFinish,
			ContextToken: contextToken,
			ItemList: []*MessageItem{
				{Type: ItemText, TextItem: &TextItem{Text: filterMarkdown(card.Text)}},
			},
		}
		if err := b.api.SendMessage(ctx, msg); err != nil {
			return fmt.Errorf("weixin: update card final: %w", err)
		}
		b.mu.Lock()
		delete(b.toolSent, cardMsgID)
		delete(b.cards, cardMsgID)
		b.mu.Unlock()
	}

	return nil
}

func (b *WeixinBot) sendTextMessage(ctx context.Context, chatID, text, contextToken string) error {
	msg := &WeixinMessage{
		ToUserID:     chatID,
		MessageType:  MessageTypeBot,
		MessageState: MessageStateFinish,
		ContextToken: contextToken,
		ItemList: []*MessageItem{
			{Type: ItemText, TextItem: &TextItem{Text: filterMarkdown(text)}},
		},
	}
	return b.api.SendMessage(ctx, msg)
}

func (b *WeixinBot) sendItemMessage(ctx context.Context, chatID string, item *MessageItem, contextToken string) error {
	msg := &WeixinMessage{
		ToUserID:     chatID,
		MessageType:  MessageTypeBot,
		MessageState: MessageStateGenerating,
		ContextToken: contextToken,
		ItemList:     []*MessageItem{item},
	}
	return b.api.SendMessage(ctx, msg)
}

func (b *WeixinBot) qrLogin(ctx context.Context) error {
	qrResp, err := b.api.GetBotQRCode(ctx)
	if err != nil {
		return fmt.Errorf("get qrcode: %w", err)
	}

	if qrResp.QRCodeImgContent != "" {
		zap.L().Info("weixin: scan QR code to login",
			zap.String("url", qrResp.QRCodeImgContent))
	} else {
		zap.L().Info("weixin: QR code obtained",
			zap.String("qrcode", qrResp.QRCode))
	}

	deadline := time.After(8 * time.Minute)
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline:
			return fmt.Errorf("weixin: QR login timed out")
		case <-ticker.C:
			status, err := b.api.GetQRCodeStatus(ctx, qrResp.QRCode)
			if err != nil {
				zap.L().Warn("weixin: qr status check failed", zap.Error(err))
				continue
			}
			switch status.Status {
			case "confirmed":
				acct := &AccountData{
					Token:   status.BotToken,
					BaseURL: status.BaseURL,
					UserID:  status.ILinkUserID,
				}
				if err := b.store.SaveAccount(acct); err != nil {
					return fmt.Errorf("weixin: save account: %w", err)
				}
				b.api.SetToken(status.BotToken)
				if status.BaseURL != "" {
					b.api.SetBaseURL(status.BaseURL)
				}
				zap.L().Info("weixin: QR login successful",
					zap.String("user_id", status.ILinkUserID))
				return nil
			case "expired":
				return fmt.Errorf("weixin: QR code expired")
			}
		}
	}
}

func (b *WeixinBot) parseParentInfo(parentMsgID string) (chatID, contextToken string) {
	parts := strings.SplitN(parentMsgID, ":", 3)
	if len(parts) >= 2 && parts[0] == "weixin" {
		chatID = parts[1]
	}
	contextToken = b.store.GetContextToken(chatID)
	return chatID, contextToken
}

func extractText(msg *WeixinMessage) string {
	var parts []string
	for _, item := range msg.ItemList {
		if item.Type == ItemText && item.TextItem != nil {
			parts = append(parts, item.TextItem.Text)
		}
	}
	return strings.Join(parts, "\n")
}

func generateClientID() string {
	var buf [16]byte
	_, _ = rand.Read(buf[:])
	return base64.RawURLEncoding.EncodeToString(buf[:])
}

func filterMarkdown(text string) string {
	f := &StreamingMarkdownFilter{}
	result := f.Feed(text)
	if tail := f.Flush(); tail != "" {
		result += tail
	}
	return result
}

// parseMessageIDInt extracts the numeric message ID from a WeChat parentMsgID.
func parseMessageIDInt(parentMsgID string) int64 {
	parts := strings.SplitN(parentMsgID, ":", 3)
	if len(parts) >= 3 {
		n, err := strconv.ParseInt(parts[2], 10, 64)
		if err == nil {
			return n
		}
	}
	return 0
}
