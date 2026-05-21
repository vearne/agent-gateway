package lark

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strings"

	lark "github.com/larksuite/oapi-sdk-go/v3"
	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
)

type ParsedMessage struct {
	Text      string
	HasText   bool
	HasImages bool
	ImageKeys []string
}

var mentionPlaceholder = regexp.MustCompile(`@_user_\d+`)

func stripMentions(s string) string {
	return mentionPlaceholder.ReplaceAllString(s, "")
}

func ParseMessage(msg *larkim.EventMessage) (*ParsedMessage, bool) {
	if msg == nil || msg.MessageType == nil || msg.Content == nil {
		return nil, false
	}

	mt := *msg.MessageType
	content := *msg.Content

	// In group chats, only process messages with @mentions
	if msg.ChatType != nil && *msg.ChatType == "group" && len(msg.Mentions) == 0 {
		return nil, false
	}

	pm := &ParsedMessage{}

	switch mt {
	case "text":
		var body struct {
			Text string `json:"text"`
		}
		if err := json.Unmarshal([]byte(content), &body); err != nil {
			return nil, false
		}
		pm.Text = strings.TrimSpace(stripMentions(body.Text))
		pm.HasText = pm.Text != ""

	case "post":
		var body struct {
			Title   string `json:"title"`
			Content [][]struct {
				Tag      string `json:"tag"`
				Text     string `json:"text"`
				ImageKey string `json:"image_key"`
			} `json:"content"`
		}
		if err := json.Unmarshal([]byte(content), &body); err != nil {
			return nil, false
		}
		var parts []string
		if body.Title != "" {
			parts = append(parts, stripMentions(body.Title))
		}
		for _, line := range body.Content {
			for _, elem := range line {
				switch elem.Tag {
				case "text":
					parts = append(parts, stripMentions(elem.Text))
				case "img", "image":
					if elem.ImageKey != "" {
						pm.ImageKeys = append(pm.ImageKeys, elem.ImageKey)
					}
				}
			}
		}
		pm.Text = strings.TrimSpace(strings.Join(parts, " "))
		pm.HasText = pm.Text != ""
		pm.HasImages = len(pm.ImageKeys) > 0

	case "image":
		var body struct {
			ImageKey string `json:"image_key"`
		}
		if err := json.Unmarshal([]byte(content), &body); err != nil {
			return nil, false
		}
		if body.ImageKey != "" {
			pm.ImageKeys = append(pm.ImageKeys, body.ImageKey)
			pm.HasImages = true
		}

	default:
		return nil, false
	}

	return pm, pm.HasText || pm.HasImages
}

func GetImageURL(ctx context.Context, larkAPI *lark.Client, messageID, imageKey string) (string, error) {
	req := larkim.NewGetImageReqBuilder().
		ImageKey(imageKey).
		Build()

	resp, err := larkAPI.Im.Image.Get(ctx, req)
	if err != nil {
		return "", fmt.Errorf("get image %s: %w", imageKey, err)
	}
	if !resp.Success() {
		return "", fmt.Errorf("get image %s failed: code=%d, msg=%s", imageKey, resp.Code, resp.Msg)
	}

	data, err := io.ReadAll(resp.File)
	if err != nil {
		return "", fmt.Errorf("read image data: %w", err)
	}

	b64 := base64.StdEncoding.EncodeToString(data)
	return "data:image/png;base64," + b64, nil
}
