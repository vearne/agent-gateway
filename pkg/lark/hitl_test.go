package lark

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/vearne/agent-gateway/pkg/adapter"
)

func TestBuildHITLCardValidJSON(t *testing.T) {
	raw := BuildHITLCard(adapter.HITLPayload{
		ToolName: "delete_file",
		ToolID:   "tool-1",
		Args:     map[string]any{"path": "/tmp/test"},
	})

	var doc map[string]any
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, raw)
	}
}

func TestBuildHITLCardContainsButtons(t *testing.T) {
	raw := BuildHITLCard(adapter.HITLPayload{
		ToolName: "send_email",
		ToolID:   "tool-2",
		Args:     map[string]any{"to": "user@example.com"},
	})

	if !strings.Contains(raw, "approve") {
		t.Fatalf("expected approve button: %s", raw)
	}
	if !strings.Contains(raw, "reject") {
		t.Fatalf("expected reject button: %s", raw)
	}
	if !strings.Contains(raw, "send_email") {
		t.Fatalf("expected tool name in card: %s", raw)
	}
}

func TestBuildHITLCardEmptyToolName(t *testing.T) {
	raw := BuildHITLCard(adapter.HITLPayload{
		ToolName: "",
		ToolID:   "tool-3",
		Args:     map[string]any{},
	})

	if !strings.Contains(raw, "unknown") {
		t.Fatalf("expected 'unknown' for empty tool name: %s", raw)
	}
}

func TestBuildHITLCardLargeArgs(t *testing.T) {
	largeArgs := map[string]any{
		"data": strings.Repeat("x", 3000),
	}

	raw := BuildHITLCard(adapter.HITLPayload{
		ToolName: "big_tool",
		ToolID:   "tool-4",
		Args:     largeArgs,
	})

	var doc map[string]any
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		t.Fatalf("invalid JSON with large args: %v", err)
	}

	if !strings.Contains(raw, "...") {
		t.Fatal("expected truncation marker for large args")
	}
}

func TestParseCardActionValueApprove(t *testing.T) {
	action, values := ParseCardActionValue(map[string]interface{}{
		"action": "approve",
	})

	if action != "approve" {
		t.Fatalf("expected 'approve', got %q", action)
	}
	if len(values) != 0 {
		t.Fatalf("expected no extra values, got %v", values)
	}
}

func TestParseCardActionValueReject(t *testing.T) {
	action, values := ParseCardActionValue(map[string]interface{}{
		"action": "reject",
		"reason": "too dangerous",
	})

	if action != "reject" {
		t.Fatalf("expected 'reject', got %q", action)
	}
	if values["reason"] != "too dangerous" {
		t.Fatalf("expected reason value, got %v", values)
	}
}

func TestParseCardActionValueMissingAction(t *testing.T) {
	action, _ := ParseCardActionValue(map[string]interface{}{
		"foo": "bar",
	})

	if action != "" {
		t.Fatalf("expected empty action, got %q", action)
	}
}

func TestParseCardActionValueNonStringAction(t *testing.T) {
	action, _ := ParseCardActionValue(map[string]interface{}{
		"action": 123,
	})

	if action != "" {
		t.Fatalf("expected empty action for non-string, got %q", action)
	}
}
