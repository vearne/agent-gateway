package lark

import (
	"encoding/json"
	"fmt"

	"github.com/vearne/agent-gateway/pkg/adapter"
)

func BuildHITLCard(payload adapter.HITLPayload) string {
	argsJSON, _ := json.MarshalIndent(payload.Args, "", "  ")
	if len(argsJSON) > 1500 {
		argsJSON = append(argsJSON[:1500], []byte("...")...)
	}

	toolDisplay := payload.ToolName
	if toolDisplay == "" {
		toolDisplay = "unknown"
	}

	type btnValue struct {
		Action string `json:"action"`
	}
	type button struct {
		Tag   string   `json:"tag"`
		Text  cardText `json:"text"`
		Type  string   `json:"type,omitempty"`
		Value btnValue `json:"value"`
	}
	type column struct {
		Tag      string   `json:"tag"`
		Elements []button `json:"elements"`
	}
	type columnSet struct {
		Tag     string   `json:"tag"`
		Columns []column `json:"columns"`
	}
	type element struct {
		Tag      string      `json:"tag"`
		Content  string      `json:"content,omitempty"`
		Elements []columnSet `json:"elements,omitempty"`
	}
	type body struct {
		Elements []element `json:"elements"`
	}
	type card struct {
		Schema string `json:"schema"`
		Body   body   `json:"body"`
	}

	c := card{
		Schema: "2.0",
		Body: body{
			Elements: []element{
				{
					Tag: "markdown",
					Content: fmt.Sprintf("🔧 **%s** 请求审批\n\n```json\n%s\n```",
						SanitizeCardContent(toolDisplay), string(argsJSON)),
				},
				{
					Tag: "column_set",
					Elements: []columnSet{
						{
							Tag: "column_set",
							Columns: []column{
								{
									Tag: "column",
									Elements: []button{
										{
											Tag:   "button",
											Text:  cardText{Tag: "plain_text", Content: "✅ 批准"},
											Type:  "primary",
											Value: btnValue{Action: "approve"},
										},
										{
											Tag:   "button",
											Text:  cardText{Tag: "plain_text", Content: "❌ 拒绝"},
											Type:  "danger",
											Value: btnValue{Action: "reject"},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	data, _ := json.Marshal(c)
	return string(data)
}

func ParseCardActionValue(value map[string]interface{}) (action string, values map[string]string) {
	values = make(map[string]string)
	if v, ok := value["action"]; ok {
		if s, ok := v.(string); ok {
			action = s
		}
	}
	for k, v := range value {
		if k == "action" {
			continue
		}
		if s, ok := v.(string); ok {
			values[k] = s
		}
	}
	return action, values
}
