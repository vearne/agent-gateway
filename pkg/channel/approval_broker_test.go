package channel

import (
	"context"
	"testing"
	"time"

	agentscope "github.com/vearne/agentscope-go/pkg/agent"
	"github.com/vearne/agent-gateway/pkg/adapter"
)

// mockHITLBot implements adapter.HITLAdapter for testing.
type mockHITLBot struct {
	cardMsgID string
	err       error
}

func (m *mockHITLBot) SendHITLCard(_ context.Context, _ string, _ adapter.HITLPayload) (string, error) {
	return m.cardMsgID, m.err
}

func (m *mockHITLBot) OnCardAction(_ func(context.Context, adapter.CardAction)) {}

func TestApprovalBrokerRegisterAndResolve(t *testing.T) {
	b := NewApprovalBroker()
	ch := b.Register("key-1")

	decision := agentscope.ToolApprovalDecision{
		Type:   agentscope.ToolDecisionApprove,
		Reason: "ok",
	}

	ok := b.Resolve("key-1", decision)
	if !ok {
		t.Fatal("expected Resolve to return true")
	}

	select {
	case got := <-ch:
		if got.Type != agentscope.ToolDecisionApprove {
			t.Fatalf("expected approve, got %v", got.Type)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for decision")
	}
}

func TestApprovalBrokerResolveUnknownKey(t *testing.T) {
	b := NewApprovalBroker()
	ok := b.Resolve("nonexistent", agentscope.ToolApprovalDecision{
		Type: agentscope.ToolDecisionApprove,
	})
	if ok {
		t.Fatal("expected Resolve to return false for unknown key")
	}
}

func TestApprovalBrokerDoubleResolve(t *testing.T) {
	b := NewApprovalBroker()
	b.Register("key-2")

	ok1 := b.Resolve("key-2", agentscope.ToolApprovalDecision{
		Type: agentscope.ToolDecisionApprove,
	})
	if !ok1 {
		t.Fatal("first Resolve should succeed")
	}

	ok2 := b.Resolve("key-2", agentscope.ToolApprovalDecision{
		Type: agentscope.ToolDecisionReject,
	})
	if ok2 {
		t.Fatal("second Resolve should fail")
	}
}

func TestApprovalBrokerCancel(t *testing.T) {
	b := NewApprovalBroker()
	b.Register("key-3")
	b.Cancel("key-3")

	ok := b.Resolve("key-3", agentscope.ToolApprovalDecision{
		Type: agentscope.ToolDecisionApprove,
	})
	if ok {
		t.Fatal("Resolve should fail after Cancel")
	}
}

func TestApprovalBrokerWaitContextCancelled(t *testing.T) {
	b := NewApprovalBroker()
	ch := b.Register("key-4")

	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	_, err := b.Wait(ctx, ch, "key-4")
	if err != context.Canceled {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func TestApprovalBrokerMakeToolApprovalFunc(t *testing.T) {
	b := NewApprovalBroker()
	bot := &mockHITLBot{cardMsgID: "card-123"}
	fn := b.MakeToolApprovalFunc(bot)

	type result struct {
		decision agentscope.ToolApprovalDecision
		err      error
	}
	resultCh := make(chan result, 1)

	go func() {
		dec, err := fn(context.Background(), agentscope.ToolApprovalRequest{
			ToolName: "delete_file",
			ToolID:   "tool-1",
			Args:     map[string]any{"path": "/tmp/test"},
		})
		resultCh <- result{dec, err}
	}()

	// Wait for the card to be registered, then resolve
	time.Sleep(100 * time.Millisecond)
	ok := b.Resolve("card-123", agentscope.ToolApprovalDecision{
		Type:   agentscope.ToolDecisionApprove,
		Reason: "approved by user",
	})
	if !ok {
		t.Fatal("expected Resolve to succeed")
	}

	select {
	case r := <-resultCh:
		if r.err != nil {
			t.Fatalf("unexpected error: %v", r.err)
		}
		if r.decision.Type != agentscope.ToolDecisionApprove {
			t.Fatalf("expected approve, got %v", r.decision.Type)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for approval result")
	}
}
