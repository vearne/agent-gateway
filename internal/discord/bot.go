package discord

import (
	"context"
	"fmt"
	"strings"

	"github.com/bwmarrin/discordgo"
	"github.com/vearne/agent-gateway/internal/adapter"
)

type DiscordBot struct {
	dg       *discordgo.Session
	handler  func(ctx context.Context, msg adapter.InboundMessage)
}

func NewDiscordBot(token string) *DiscordBot {
	if !strings.HasPrefix(token, "Bot ") {
		token = "Bot " + token
	}
	dg, err := discordgo.New(token)
	if err != nil {
		panic(fmt.Sprintf("failed to create Discord session: %v", err))
	}
	dg.Identify.Intents = discordgo.IntentsGuildMessages | discordgo.IntentsGuilds
	return &DiscordBot{dg: dg}
}

func (b *DiscordBot) Start(ctx context.Context) error {
	b.dg.AddHandler(func(s *discordgo.Session, m *discordgo.MessageCreate) {
		if s.State.User != nil && m.Author.ID == s.State.User.ID {
			return
		}
		if m.Content == "" {
			return
		}
		if b.handler == nil {
			return
		}
		msg := adapter.InboundMessage{
			MsgID:  m.ChannelID + ":" + m.ID,
			ChatID: m.ChannelID,
			Text:   m.Content,
		}
		b.handler(ctx, msg)
	})

	if err := b.dg.Open(); err != nil {
		return fmt.Errorf("failed to open Discord session: %w", err)
	}

	go func() {
		<-ctx.Done()
		b.dg.Close()
	}()

	return nil
}

func (b *DiscordBot) Stop() {
	b.dg.Close()
}

func (b *DiscordBot) OnMessage(cb func(ctx context.Context, msg adapter.InboundMessage)) {
	b.handler = cb
}

func (b *DiscordBot) SendText(ctx context.Context, parentMsgID string, text string) error {
	channelID, messageID := parseCompositeID(parentMsgID)
	_, err := b.dg.ChannelMessageSendReply(channelID, text, &discordgo.MessageReference{
		MessageID: messageID,
		ChannelID: channelID,
	})
	if err != nil {
		return fmt.Errorf("discord send text: %w", err)
	}
	return nil
}

func (b *DiscordBot) SendCard(ctx context.Context, parentMsgID string, card adapter.CardContent) (string, error) {
	channelID, _ := parseCompositeID(parentMsgID)
	embed := buildEmbed(card)
	sentMsg, err := b.dg.ChannelMessageSendEmbed(channelID, embed)
	if err != nil {
		return "", fmt.Errorf("discord send card: %w", err)
	}
	return channelID + ":" + sentMsg.ID, nil
}

func (b *DiscordBot) UpdateCard(ctx context.Context, cardMsgID string, card adapter.CardContent) error {
	channelID, messageID := parseCompositeID(cardMsgID)
	embed := buildEmbed(card)
	_, err := b.dg.ChannelMessageEditComplex(discordgo.NewMessageEdit(channelID, messageID).SetEmbeds([]*discordgo.MessageEmbed{embed}))
	if err != nil {
		return fmt.Errorf("discord update card: %w", err)
	}
	return nil
}

func buildEmbed(card adapter.CardContent) *discordgo.MessageEmbed {
	description := card.Text
	if card.Streaming {
		description += "▌"
	}
	embed := &discordgo.MessageEmbed{
		Title:       "Agent Response",
		Description: description,
		Color:       0x0099ff,
		Fields:      make([]*discordgo.MessageEmbedField, 0, len(card.Tools)),
	}
	for _, tool := range card.Tools {
		value := tool.Args
		if tool.Result != "" {
			value = fmt.Sprintf("Args: %s\nResult: %s", tool.Args, tool.Result)
		}
		embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
			Name:   tool.Name,
			Value:  value,
			Inline: false,
		})
	}
	return embed
}

func parseCompositeID(id string) (channelID, messageID string) {
	parts := strings.SplitN(id, ":", 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return id, ""
}
