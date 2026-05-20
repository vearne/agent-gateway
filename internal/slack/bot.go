package slack

import (
	"context"
	"fmt"
	"strings"

	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
	"github.com/slack-go/slack/socketmode"

	"github.com/vearne/agent-gateway/internal/adapter"
)

var _ adapter.BotAdapter = (*SlackBot)(nil)

type SlackBot struct {
	api     *slack.Client
	client  *socketmode.Client
	handler func(ctx context.Context, msg adapter.InboundMessage)
	cancel  context.CancelFunc
	botID   string
}

func NewSlackBot(botToken, appToken string) *SlackBot {
	api := slack.New(botToken, slack.OptionAppLevelToken(appToken))
	client := socketmode.New(api)
	return &SlackBot{
		api:    api,
		client: client,
	}
}

func (s *SlackBot) Start(ctx context.Context) error {
	innerCtx, cancel := context.WithCancel(ctx)
	s.cancel = cancel

	resp, err := s.api.AuthTestContext(innerCtx)
	if err != nil {
		cancel()
		return fmt.Errorf("slack auth test: %w", err)
	}
	s.botID = resp.UserID

	go s.client.RunContext(innerCtx)

	go func() {
		for {
			select {
			case <-innerCtx.Done():
				return
			case evt, ok := <-s.client.Events:
				if !ok {
					return
				}
				s.handleEvent(innerCtx, evt)
			}
		}
	}()

	<-innerCtx.Done()
	return innerCtx.Err()
}

func (s *SlackBot) Stop() {
	if s.cancel != nil {
		s.cancel()
	}
}

func (s *SlackBot) OnMessage(cb func(ctx context.Context, msg adapter.InboundMessage)) {
	s.handler = cb
}

func (s *SlackBot) SendText(ctx context.Context, parentMsgID string, text string) error {
	channelID, timestamp := parseMsgID(parentMsgID)
	_, _, err := s.api.PostMessage(channelID,
		slack.MsgOptionText(text, false),
		slack.MsgOptionTS(timestamp),
	)
	return err
}

func (s *SlackBot) SendCard(ctx context.Context, parentMsgID string, card adapter.CardContent) (string, error) {
	channelID, timestamp := parseMsgID(parentMsgID)
	blocks := buildBlocks(card)
	_, respTS, err := s.api.PostMessage(channelID,
		slack.MsgOptionBlocks(blocks...),
		slack.MsgOptionTS(timestamp),
	)
	if err != nil {
		return "", err
	}
	return channelID + ":" + respTS, nil
}

func (s *SlackBot) UpdateCard(ctx context.Context, cardMsgID string, card adapter.CardContent) error {
	channelID, timestamp := parseMsgID(cardMsgID)
	blocks := buildBlocks(card)
	_, _, _, err := s.api.UpdateMessage(channelID, timestamp,
		slack.MsgOptionBlocks(blocks...),
	)
	return err
}

func (s *SlackBot) handleEvent(ctx context.Context, evt socketmode.Event) {
	if evt.Type != socketmode.EventTypeEventsAPI {
		return
	}

	eventsAPIEvent, ok := evt.Data.(slackevents.EventsAPIEvent)
	if !ok {
		return
	}

	s.client.Ack(*evt.Request)

	switch ev := eventsAPIEvent.InnerEvent.Data.(type) {
	case *slackevents.MessageEvent:
		if ev.User == s.botID {
			return
		}
		if s.handler != nil {
			s.handler(ctx, adapter.InboundMessage{
				MsgID:  ev.Channel + ":" + ev.TimeStamp,
				ChatID: ev.Channel,
				Text:   ev.Text,
			})
		}
	}
}

func parseMsgID(msgID string) (channelID, timestamp string) {
	parts := strings.SplitN(msgID, ":", 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return msgID, ""
}

func buildBlocks(card adapter.CardContent) []slack.Block {
	var blocks []slack.Block

	for _, tool := range card.Tools {
		toolText := fmt.Sprintf("*%s*", tool.Name)
		if tool.Args != "" {
			toolText += fmt.Sprintf("\nArgs: %s", tool.Args)
		}
		if tool.Result != "" {
			toolText += fmt.Sprintf("\nResult: %s", tool.Result)
		}
		if !tool.Done {
			toolText += " ⏳"
		}
		blocks = append(blocks, slack.NewSectionBlock(
			slack.NewTextBlockObject("mrkdwn", toolText, false, false),
			nil, nil,
		))
	}

	text := card.Text
	if card.Streaming {
		text += " ▌"
	}
	if text != "" {
		blocks = append(blocks, slack.NewSectionBlock(
			slack.NewTextBlockObject("mrkdwn", text, false, false),
			nil, nil,
		))
	}

	if len(blocks) == 0 {
		blocks = append(blocks, slack.NewSectionBlock(
			slack.NewTextBlockObject("mrkdwn", " ", false, false),
			nil, nil,
		))
	}

	return blocks
}
