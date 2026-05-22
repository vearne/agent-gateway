package lark

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestBuildCardThinkingUsesCollapsiblePanel(t *testing.T) {
	raw := BuildCard(nil, "thinking body", "", true)
	if !strings.Contains(raw, "collapsible_panel") {
		t.Fatalf("expected collapsible_panel: %s", raw)
	}
	if !strings.Contains(raw, "🧠 思考过程") {
		t.Fatalf("expected thinking heading: %s", raw)
	}
	if !strings.Contains(raw, `"expanded":true`) {
		t.Fatalf("thinking panel should expand while streaming: %s", raw)
	}
	var doc map[string]any
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		t.Fatalf("invalid json: %v\n%s", err, raw)
	}
}

func TestBuildCardToolUsesCollapsiblePanel(t *testing.T) {
	raw := BuildCard([]ToolEntry{{
		Name:   "get_time",
		Args:   `{"command":"date"}`,
		Result: "2026-05-21",
		Done:   true,
	}}, "", "done", false)
	if !strings.Contains(raw, "collapsible_panel") {
		t.Fatalf("expected collapsible_panel: %s", raw)
	}
	if !strings.Contains(raw, "⚙️ 工具调用（1）") {
		t.Fatalf("expected tool panel title: %s", raw)
	}
	if !strings.Contains(raw, "get_time") {
		t.Fatalf("expected tool name in trace: %s", raw)
	}
	if !strings.Contains(raw, "2026-05-21") {
		t.Fatalf("expected tool result in card: %s", raw)
	}
	if strings.Contains(raw, `"expanded":true`) {
		t.Fatalf("tool panel should collapse when not streaming: %s", raw)
	}
}
