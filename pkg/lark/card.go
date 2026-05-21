package lark

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	lark "github.com/larksuite/oapi-sdk-go/v3"
	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
)

type ToolEntry struct {
	Name   string
	Args   string
	Result string
	Done   bool
}

type richCard struct {
	Schema string     `json:"schema"`
	Config cardConfig `json:"config"`
	Body   cardBody   `json:"body"`
}

type cardConfig struct {
	WideScreenMode bool         `json:"wide_screen_mode"`
	StreamingMode  bool         `json:"streaming_mode,omitempty"`
	Summary        *cardSummary `json:"summary,omitempty"`
}

type cardSummary struct {
	Tag     string `json:"tag"`
	Content string `json:"content"`
}

type cardBody struct {
	Elements []cardElement `json:"elements"`
}

type cardElement struct {
	Tag      string        `json:"tag"`
	Content  string        `json:"content,omitempty"`
	Expanded *bool         `json:"expanded,omitempty"`
	Header   *panelHeader  `json:"header,omitempty"`
	Elements []cardElement `json:"elements,omitempty"`
}

type panelHeader struct {
	Tag      string      `json:"tag"`
	Template string      `json:"template,omitempty"`
	Title    *panelTitle `json:"title"`
}

type panelTitle struct {
	Tag     string `json:"tag"`
	Content string `json:"content"`
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

func toolElements(tools []ToolEntry) []cardElement {
	var elems []cardElement
	for _, t := range tools {
		statusIcon := "⏳"
		if t.Done {
			statusIcon = "✅"
		}
		argsDisplay := truncate(t.Args, 200)
		title := fmt.Sprintf("%s %s", statusIcon, t.Name)

		innerElems := []cardElement{
			{Tag: "markdown", Content: fmt.Sprintf("**Args:** `%s`", argsDisplay)},
		}
		if t.Done && t.Result != "" {
			resultDisplay := truncate(t.Result, 500)
			innerElems = append(innerElems, cardElement{
				Tag:     "markdown",
				Content: fmt.Sprintf("**Result:**\n```\n%s\n```", resultDisplay),
			})
		}

		elems = append(elems, cardElement{
			Tag:      "collapsible_panel",
			Expanded: boolPtr(false),
			Header: &panelHeader{
				Tag: "plain_text",
				Title: &panelTitle{
					Tag:     "plain_text",
					Content: title,
				},
			},
			Elements: innerElems,
		})
	}
	return elems
}

func boolPtr(b bool) *bool { return &b }

func BuildCard(tools []ToolEntry, responseText string, streaming bool) string {
	var elems []cardElement

	if len(tools) > 0 {
		elems = append(elems, toolElements(tools)...)
	}

	text := responseText
	if streaming {
		text += "▌"
	}
	if text != "" {
		elems = append(elems, cardElement{
			Tag:     "markdown",
			Content: text,
		})
	}

	cfg := cardConfig{
		WideScreenMode: true,
	}
	if streaming {
		cfg.StreamingMode = true
		cfg.Summary = &cardSummary{
			Tag:     "plain_text",
			Content: "thinking...",
		}
	}

	card := richCard{
		Schema: "2.0",
		Config: cfg,
		Body:   cardBody{Elements: elems},
	}

	data, _ := json.Marshal(card)
	return string(data)
}

func SendTextReply(ctx context.Context, larkAPI *lark.Client, parentMsgID, text string) error {
	content, _ := json.Marshal(map[string]string{"text": text})
	req := larkim.NewReplyMessageReqBuilder().
		MessageId(parentMsgID).
		Body(larkim.NewReplyMessageReqBodyBuilder().
			MsgType(larkim.MsgTypeText).
			Content(string(content)).
			Build()).
		Build()

	resp, err := larkAPI.Im.Message.Reply(ctx, req)
	if err != nil {
		return fmt.Errorf("reply message: %w", err)
	}
	if !resp.Success() {
		return fmt.Errorf("reply message failed: code=%d, msg=%s", resp.Code, resp.Msg)
	}
	return nil
}

func SendCardReply(ctx context.Context, larkAPI *lark.Client, parentMsgID, cardJSON string) error {
	_, err := sendCardReply(ctx, larkAPI, parentMsgID, cardJSON)
	return err
}

func sendCardReply(ctx context.Context, larkAPI *lark.Client, parentMsgID, cardJSON string) (string, error) {
	req := larkim.NewReplyMessageReqBuilder().
		MessageId(parentMsgID).
		Body(larkim.NewReplyMessageReqBodyBuilder().
			MsgType(larkim.MsgTypeInteractive).
			Content(cardJSON).
			Build()).
		Build()

	resp, err := larkAPI.Im.Message.Reply(ctx, req)
	if err != nil {
		return "", fmt.Errorf("reply card: %w", err)
	}
	if !resp.Success() {
		return "", fmt.Errorf("reply card failed: code=%d, msg=%s", resp.Code, resp.Msg)
	}
	if resp.Data != nil && resp.Data.MessageId != nil {
		return *resp.Data.MessageId, nil
	}
	return "", nil
}

func UpdateCard(ctx context.Context, larkAPI *lark.Client, messageID, cardJSON string) error {
	req := larkim.NewUpdateMessageReqBuilder().
		MessageId(messageID).
		Body(larkim.NewUpdateMessageReqBodyBuilder().
			Content(cardJSON).
			Build()).
		Build()

	resp, err := larkAPI.Im.Message.Update(ctx, req)
	if err != nil {
		return fmt.Errorf("update card: %w", err)
	}
	if !resp.Success() {
		return fmt.Errorf("update card failed: code=%d, msg=%s", resp.Code, resp.Msg)
	}
	return nil
}

func CreateMessage(ctx context.Context, larkAPI *lark.Client, receiveID, receiveIDType, msgType, content string) (string, error) {
	req := larkim.NewCreateMessageReqBuilder().
		ReceiveIdType(receiveIDType).
		Body(larkim.NewCreateMessageReqBodyBuilder().
			ReceiveId(receiveID).
			MsgType(msgType).
			Content(content).
			Build()).
		Build()

	resp, err := larkAPI.Im.Message.Create(ctx, req)
	if err != nil {
		return "", fmt.Errorf("create message: %w", err)
	}
	if !resp.Success() {
		return "", fmt.Errorf("create message failed: code=%d, msg=%s", resp.Code, resp.Msg)
	}
	if resp.Data != nil && resp.Data.MessageId != nil {
		return *resp.Data.MessageId, nil
	}
	return "", fmt.Errorf("create message: no message_id in response")
}

func SanitizeCardContent(s string) string {
	r := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
	)
	return r.Replace(s)
}
