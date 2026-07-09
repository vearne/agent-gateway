package lark

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSanitizeCardContentEscapesHTML(t *testing.T) {
	got := SanitizeCardContent(`<script>alert("xss")</script> & <b>bold</b>`)
	if strings.Contains(got, "<") || strings.Contains(got, ">") {
		t.Fatalf("expected HTML entities, got %q", got)
	}
	if !strings.Contains(got, "&lt;script&gt;") {
		t.Fatalf("expected escaped script tag: %q", got)
	}
	if !strings.Contains(got, "&amp;") {
		t.Fatalf("expected escaped ampersand: %q", got)
	}
}

func TestSanitizeCardContentPassthrough(t *testing.T) {
	got := SanitizeCardContent("hello world")
	if got != "hello world" {
		t.Fatalf("expected passthrough, got %q", got)
	}
}

func TestTruncateWithLogUnderLimit(t *testing.T) {
	input := "short text"
	got := truncateWithLog(input, 100, "test")
	if got != input {
		t.Fatalf("expected no truncation, got %q", got)
	}
}

func TestTruncateWithLogOverLimit(t *testing.T) {
	input := strings.Repeat("a", 200)
	got := truncateWithLog(input, 50, "test")
	if len(got) <= 50 {
		t.Fatalf("expected truncation, got len=%d", len(got))
	}
	if !strings.Contains(got, "已截断") {
		t.Fatalf("expected truncation marker: %q", got)
	}
	if !strings.HasPrefix(got, strings.Repeat("a", 50)) {
		t.Fatalf("expected first 50 chars preserved")
	}
}

func TestTruncateShortString(t *testing.T) {
	got := truncate("hello", 100)
	if got != "hello" {
		t.Fatalf("expected passthrough, got %q", got)
	}
}

func TestTruncateLongString(t *testing.T) {
	input := strings.Repeat("x", 200)
	got := truncate(input, 50)
	if !strings.HasPrefix(got, strings.Repeat("x", 50)) {
		t.Fatalf("expected first 50 chars: %q", got)
	}
	if !strings.HasSuffix(got, "...") {
		t.Fatalf("expected ellipsis suffix: %q", got)
	}
}

func TestFormatJSONValid(t *testing.T) {
	formatted, ok := formatJSON(`{"b":2,"a":1}`)
	if !ok {
		t.Fatal("expected formatJSON to succeed on valid JSON")
	}
	if !strings.Contains(formatted, "\"a\": 1") {
		t.Fatalf("expected sorted keys: %q", formatted)
	}
}

func TestFormatJSONInvalid(t *testing.T) {
	_, ok := formatJSON("not json")
	if ok {
		t.Fatal("expected formatJSON to fail on invalid JSON")
	}
}

func TestBuildCardLargeContentTriggersCompact(t *testing.T) {
	big := strings.Repeat("A paragraph of text. ", 1500)
	raw := BuildCard(nil, "", big, false)

	var doc map[string]any
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		t.Fatalf("compact card should still be valid JSON: %v", err)
	}
	if len(raw) > maxCardBytes+2000 {
		t.Fatalf("compact card still too large: %d bytes (limit %d)", len(raw), maxCardBytes)
	}
}

func TestBuildCardStreamingAddsCursor(t *testing.T) {
	raw := BuildCard(nil, "", "hello", true)
	if !strings.Contains(raw, "▌") {
		t.Fatal("expected streaming cursor in card")
	}
}

func TestBuildCardNonStreamingNoCursor(t *testing.T) {
	raw := BuildCard(nil, "", "hello", false)
	if strings.Contains(raw, "▌") {
		t.Fatal("expected no streaming cursor in non-streaming card")
	}
}

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
