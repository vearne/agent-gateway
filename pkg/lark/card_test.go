package lark

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestBuildCardThinkingUsesMarkdown(t *testing.T) {
	raw := BuildCard(nil, "thinking body", "", true)
	if strings.Contains(raw, "collapsible_panel") {
		t.Fatalf("should not use collapsible_panel: %s", raw)
	}
	if !strings.Contains(raw, "🧠 思考过程") {
		t.Fatalf("expected thinking heading: %s", raw)
	}
	var doc map[string]any
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		t.Fatalf("invalid json: %v\n%s", err, raw)
	}
}

func TestBuildCardToolUsesMarkdown(t *testing.T) {
	raw := BuildCard([]ToolEntry{{
		Name:   "get_time",
		Args:   `{"command":"date"}`,
		Result: "2026-05-21",
		Done:   true,
	}}, "", "done", false)
	if strings.Contains(raw, "collapsible_panel") {
		t.Fatalf("should not use collapsible_panel: %s", raw)
	}
	if !strings.Contains(raw, "🔧 get_time") {
		t.Fatalf("expected tool heading: %s", raw)
	}
	if !strings.Contains(raw, "**Input:**") || !strings.Contains(raw, "**Output:**") {
		t.Fatalf("expected tool input/output sections: %s", raw)
	}
	if !strings.Contains(raw, "2026-05-21") {
		t.Fatalf("expected tool result in card: %s", raw)
	}
}
