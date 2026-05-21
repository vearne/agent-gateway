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
	WideScreenMode bool         `json:"wide_screen_mode,omitempty"`
	StreamingMode  bool         `json:"streaming_mode,omitempty"`
	Summary        *cardSummary `json:"summary,omitempty"`
}

type cardSummary struct {
	Content string `json:"content"`
}

type cardBody struct {
	Elements []cardElement `json:"elements"`
}

type cardText struct {
	Tag     string `json:"tag"`
	Content string `json:"content"`
}

type panelHeader struct {
	Title cardText `json:"title"`
}

type cardElement struct {
	Tag string `json:"tag"`

	// markdown
	Content string `json:"content,omitempty"`

	// collapsible_panel
	Expanded *bool         `json:"expanded,omitempty"`
	Header   *panelHeader  `json:"header,omitempty"`
	Elements []cardElement `json:"elements,omitempty"`
}

func boolPtr(b bool) *bool { return &b }

func mdElement(content string) cardElement {
	return cardElement{Tag: "markdown", Content: content}
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

func thinkingPanel(thinking string, expanded bool) cardElement {
	display := truncate(SanitizeCardContent(thinking), 3000)
	return cardElement{
		Tag:      "collapsible_panel",
		Expanded: boolPtr(expanded),
		Header: &panelHeader{
			Title: cardText{Tag: "plain_text", Content: "🧠 思考过程"},
		},
		Elements: []cardElement{mdElement(display)},
	}
}

func toolsPanel(tools []ToolEntry, streaming bool) cardElement {
	return cardElement{
		Tag:      "collapsible_panel",
		Expanded: boolPtr(streaming),
		Header: &panelHeader{
			Title: cardText{Tag: "plain_text", Content: fmt.Sprintf("⚙️ 工具调用（%d）", len(tools))},
		},
		Elements: []cardElement{mdElement(buildToolTrace(tools))},
	}
}

func buildToolTrace(tools []ToolEntry) string {
	var sb strings.Builder
	for i, t := range tools {
		if i > 0 {
			sb.WriteString("\n")
		}
		name := t.Name
		if name == "" {
			name = "unknown"
		}
		sb.WriteString("🔧 **")
		sb.WriteString(name)
		sb.WriteString("**\n")

		args := strings.TrimSpace(t.Args)
		if args != "" && args != "{}" && args != "null" {
			if formatted, isJSON := formatJSON(args); isJSON {
				sb.WriteString("```json\n")
				sb.WriteString(truncate(SanitizeCardContent(formatted), 1500))
				sb.WriteString("\n```\n")
			} else {
				sb.WriteString(truncate(SanitizeCardContent(args), 1500))
				sb.WriteString("\n")
			}
		}

		if t.Done {
			result := strings.TrimSpace(t.Result)
			if result == "" {
				sb.WriteString("> （空）\n")
			} else if strings.HasPrefix(result, "❌ ") {
				sb.WriteString("> ❌\n")
				sb.WriteString("```\n")
				sb.WriteString(truncate(SanitizeCardContent(strings.TrimPrefix(result, "❌ ")), 2000))
				sb.WriteString("\n```\n")
			} else if formatted, isJSON := formatJSON(result); isJSON {
				sb.WriteString("> ✅\n")
				sb.WriteString("```json\n")
				sb.WriteString(truncate(SanitizeCardContent(formatted), 2000))
				sb.WriteString("\n```\n")
			} else {
				sb.WriteString("> ✅\n")
				sb.WriteString("```\n")
				sb.WriteString(truncate(SanitizeCardContent(result), 2000))
				sb.WriteString("\n```\n")
			}
		} else {
			sb.WriteString("> ⏳ 执行中…\n")
		}
	}
	return strings.TrimRight(sb.String(), "\n")
}

func formatJSON(s string) (formatted string, ok bool) {
	var v any
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		return "", false
	}
	bs, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return "", false
	}
	return string(bs), true
}

func BuildCard(tools []ToolEntry, thinking, responseText string, streaming bool) string {
	var elems []cardElement

	if thinking != "" {
		elems = append(elems, thinkingPanel(thinking, streaming && responseText == "" && len(tools) == 0))
	}
	if len(tools) > 0 {
		elems = append(elems, toolsPanel(tools, streaming))
	}

	text := SanitizeCardContent(responseText)
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
		summary := "生成中..."
		if thinking != "" && responseText == "" && len(tools) == 0 {
			summary = "推理中..."
		} else if len(tools) > 0 && responseText == "" {
			summary = "调用工具..."
		}
		cfg.Summary = &cardSummary{Content: summary}
	}

	card := richCard{
		Schema: "2.0",
		Config: cfg,
		Body:   cardBody{Elements: elems},
	}

	data, _ := json.Marshal(card)
	return string(data)
}

func PatchCard(ctx context.Context, larkAPI *lark.Client, messageID, cardJSON string) error {
	req := larkim.NewPatchMessageReqBuilder().
		MessageId(messageID).
		Body(larkim.NewPatchMessageReqBodyBuilder().
			Content(cardJSON).
			Build()).
		Build()

	var resp *larkim.PatchMessageResp
	err := retryOnNetErr(ctx, func() error {
		var callErr error
		resp, callErr = larkAPI.Im.Message.Patch(ctx, req)
		return callErr
	})
	if err != nil {
		return fmt.Errorf("patch card: %w", err)
	}
	if !resp.Success() {
		return fmt.Errorf("patch card failed: code=%d, msg=%s", resp.Code, resp.Msg)
	}
	return nil
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

	var resp *larkim.ReplyMessageResp
	err := retryOnNetErr(ctx, func() error {
		var callErr error
		resp, callErr = larkAPI.Im.Message.Reply(ctx, req)
		return callErr
	})
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

	var resp *larkim.ReplyMessageResp
	err := retryOnNetErr(ctx, func() error {
		var callErr error
		resp, callErr = larkAPI.Im.Message.Reply(ctx, req)
		return callErr
	})
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

	var resp *larkim.UpdateMessageResp
	err := retryOnNetErr(ctx, func() error {
		var callErr error
		resp, callErr = larkAPI.Im.Message.Update(ctx, req)
		return callErr
	})
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
