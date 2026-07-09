package channel

import (
	"context"
	"fmt"
	"sync"
	"time"

	agentscope "github.com/vearne/agentscope-go/pkg/agent"
	"go.uber.org/zap"

	"github.com/vearne/agent-gateway/pkg/adapter"
)

// ApprovalBroker bridges agentscope-go's synchronous ToolApprovalFunc
// to asynchronous IM card-action callbacks.
type ApprovalBroker struct {
	mu      sync.Mutex
	pending map[string]chan agentscope.ToolApprovalDecision // key: approvalKey (cardMsgID)
	timeout time.Duration
}

// NewApprovalBroker creates a broker with a 24h default timeout.
func NewApprovalBroker() *ApprovalBroker {
	return &ApprovalBroker{
		pending: make(map[string]chan agentscope.ToolApprovalDecision),
		timeout: 24 * time.Hour,
	}
}

// Register creates a pending approval slot and returns the decision channel.
func (b *ApprovalBroker) Register(approvalKey string) <-chan agentscope.ToolApprovalDecision {
	ch := make(chan agentscope.ToolApprovalDecision, 1)
	b.mu.Lock()
	b.pending[approvalKey] = ch
	b.mu.Unlock()
	return ch
}

// Resolve delivers the user's decision and removes the pending slot.
func (b *ApprovalBroker) Resolve(approvalKey string, decision agentscope.ToolApprovalDecision) bool {
	b.mu.Lock()
	ch, ok := b.pending[approvalKey]
	if ok {
		delete(b.pending, approvalKey)
	}
	b.mu.Unlock()
	if !ok {
		return false
	}
	select {
	case ch <- decision:
		return true
	default:
		return false
	}
}

// Cancel removes a pending approval without resolving.
func (b *ApprovalBroker) Cancel(approvalKey string) {
	b.mu.Lock()
	delete(b.pending, approvalKey)
	b.mu.Unlock()
}

// Wait blocks until decision, timeout, or context cancellation.
func (b *ApprovalBroker) Wait(ctx context.Context, ch <-chan agentscope.ToolApprovalDecision, approvalKey string) (agentscope.ToolApprovalDecision, error) {
	timer := time.NewTimer(b.timeout)
	defer timer.Stop()
	defer b.Cancel(approvalKey)

	select {
	case dec := <-ch:
		return dec, nil
	case <-timer.C:
		zap.L().Info("tool approval timed out", zap.String("approval_key", approvalKey))
		return agentscope.ToolApprovalDecision{
			Type:   agentscope.ToolDecisionReject,
			Reason: "approval timeout (24h)",
		}, nil
	case <-ctx.Done():
		return agentscope.ToolApprovalDecision{}, ctx.Err()
	}
}

// MakeToolApprovalFunc creates a ToolApprovalFunc that sends HITL cards
// via the bot and waits for user response through the broker.
// The bot must implement adapter.HITLAdapter.
func (b *ApprovalBroker) MakeToolApprovalFunc(bot adapter.HITLAdapter) agentscope.ToolApprovalFunc {
	return func(ctx context.Context, req agentscope.ToolApprovalRequest) (agentscope.ToolApprovalDecision, error) {
		// Recover from panics to prevent agent goroutine crash
		defer func() {
			if r := recover(); r != nil {
				zap.L().Error("panic in tool approval callback",
					zap.Any("panic", r),
					zap.String("tool", req.ToolName))
			}
		}()

		zap.L().Info("tool approval requested",
			zap.String("tool_name", req.ToolName),
			zap.String("tool_id", req.ToolID))

		// Send HITL card — use the tool ID as parent so the card appears in reply
		cardMsgID, err := bot.SendHITLCard(ctx, "", adapter.HITLPayload{
			ToolName: req.ToolName,
			ToolID:   req.ToolID,
			Args:     req.Args,
		})
		if err != nil {
			return agentscope.ToolApprovalDecision{
				Type:   agentscope.ToolDecisionReject,
				Reason: fmt.Sprintf("failed to send HITL card: %v", err),
			}, nil
		}

		// Register and wait
		approvalKey := cardMsgID
		ch := b.Register(approvalKey)
		return b.Wait(ctx, ch, approvalKey)
	}
}
