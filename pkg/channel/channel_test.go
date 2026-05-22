package channel

import (
	"testing"

	"github.com/vearne/agent-gateway/pkg/adapter"
)

func TestSplitThinkingForDisplayKeepsTextWhenPresent(t *testing.T) {
	thinking, text := splitThinkingForDisplay("reasoning", "answer", nil)
	if thinking != "reasoning" || text != "answer" {
		t.Fatalf("got thinking=%q text=%q", thinking, text)
	}
}

func TestSplitThinkingForDisplayPromotesPostToolSuffix(t *testing.T) {
	thinking := "\n用户询问当前时间。我需要执行 date。\n\n\n当前时间是 2026年5月22日 CST。"
	tools := []adapter.ToolEntry{{Name: "execute_shell", Done: true}}

	gotThinking, gotText := splitThinkingForDisplay(thinking, "", tools)
	if gotText != "当前时间是 2026年5月22日 CST。" {
		t.Fatalf("text=%q", gotText)
	}
	if gotThinking != "用户询问当前时间。我需要执行 date。" {
		t.Fatalf("thinking=%q", gotThinking)
	}
}

func TestSplitThinkingForDisplayWaitsForDoneTools(t *testing.T) {
	thinking := "plan\n\n\nanswer"
	tools := []adapter.ToolEntry{{Name: "execute_shell", Done: false}}

	gotThinking, gotText := splitThinkingForDisplay(thinking, "", tools)
	if gotThinking != thinking || gotText != "" {
		t.Fatalf("thinking=%q text=%q", gotThinking, gotText)
	}
}
