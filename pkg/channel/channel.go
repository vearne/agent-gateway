package channel

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/vearne/agentscope-go/pkg/memory"
	"github.com/vearne/agentscope-go/pkg/message"
	"go.uber.org/zap"

	"github.com/vearne/agent-gateway/pkg/adapter"
)

type Channel struct {
	name    string
	bot     adapter.BotAdapter
	factory adapter.AgentFactory
	store   adapter.SessionStore
	locks   sync.Map
	cancel  context.CancelFunc
}

func New(name string, bot adapter.BotAdapter, factory adapter.AgentFactory, store adapter.SessionStore) *Channel {
	return &Channel{
		name:    name,
		bot:     bot,
		factory: factory,
		store:   store,
	}
}

func (ch *Channel) Start(ctx context.Context) {
	ctx, ch.cancel = context.WithCancel(ctx)

	ch.bot.OnMessage(ch.handleMessage)

	go func() {
		if err := ch.bot.Start(ctx); err != nil && ctx.Err() == nil {
			zap.L().Error("bot stopped unexpectedly",
				zap.String("channel", ch.name), zap.Error(err))
		}
	}()
}

func (ch *Channel) Stop() {
	if ch.cancel != nil {
		ch.cancel()
	}
}

func (ch *Channel) handleMessage(ctx context.Context, msg adapter.InboundMessage) {
	zap.L().Info("received message",
		zap.String("channel", ch.name),
		zap.String("chat_id", msg.ChatID),
		zap.String("msg_id", msg.MsgID),
		zap.String("text", msg.Text))

	if msg.Text != "" {
		switch strings.TrimSpace(msg.Text) {
		case "/new":
			ch.store.NewSession(ctx, msg.ChatID)
			if err := ch.bot.SendText(ctx, msg.MsgID, "✅ 已开启新会话"); err != nil {
				zap.L().Warn("send command reply failed", zap.String("command", "/new"), zap.Error(err))
			}
			return
		case "/clear", "/reset":
			ch.store.ResetSession(ctx, msg.ChatID)
			if err := ch.bot.SendText(ctx, msg.MsgID, "🔄 已清空当前会话。"); err != nil {
				zap.L().Warn("send command reply failed", zap.String("command", "/clear"), zap.Error(err))
			}
			return
		case "/help":
			if err := ch.bot.SendText(ctx, msg.MsgID, "📖 **可用指令**\n/new          开启新会话\n/clear /reset  清空当前会话\n/help         查看此帮助"); err != nil {
				zap.L().Warn("send command reply failed", zap.String("command", "/help"), zap.Error(err))
			}
			return
		}
	}

	go ch.processReply(ctx, msg)
}

func (ch *Channel) processReply(ctx context.Context, msg adapter.InboundMessage) {
	// Detach from the bot callback ctx so a short-lived event context does not
	// cancel the agent stream or session save mid-reply.
	ctx = context.WithoutCancel(ctx)

	mu := ch.getLock(msg.ChatID)
	mu.Lock()
	defer mu.Unlock()

	agent := ch.factory.Create()

	if err := ch.store.LoadSession(ctx, msg.ChatID, agent.Memory()); err != nil {
		zap.L().Warn("load session failed, starting fresh",
			zap.String("chat_id", msg.ChatID), zap.Error(err))
	}

	var contentBlocks []message.ContentBlock
	if msg.Text != "" {
		contentBlocks = append(contentBlocks, message.NewTextBlock(msg.Text))
	}
	if len(contentBlocks) == 0 {
		contentBlocks = append(contentBlocks, message.NewTextBlock(""))
	}
	agentMsg := message.NewMsg("user", contentBlocks, "user")

	zap.L().Info("sending to agent",
		zap.String("channel", ch.name),
		zap.String("chat_id", msg.ChatID),
		zap.String("msg_id", msg.MsgID),
		zap.String("text", msg.Text))

	// Send initial card — capture cardMsgID for subsequent updates
	cardMsgID, err := ch.bot.SendCard(ctx, msg.MsgID, adapter.CardContent{
		Tools:     nil,
		Text:      "⏳ 思考中...",
		Streaming: true,
	})
	if err != nil {
		if sendErr := ch.bot.SendText(ctx, msg.MsgID, "❌ 发送卡片失败："+err.Error()); sendErr != nil {
			zap.L().Warn("send card failure notice failed", zap.Error(sendErr))
		}
		return
	}

	streamCh, err := agent.ReplyStream(ctx, agentMsg)
	if err != nil {
		zap.L().Error("agent reply stream failed",
			zap.String("chat_id", msg.ChatID),
			zap.String("msg_id", msg.MsgID),
			zap.Error(err))
		if updateErr := ch.bot.UpdateCard(ctx, cardMsgID, adapter.CardContent{
			Text: "❌ 回复失败：" + err.Error(),
		}); updateErr != nil {
			zap.L().Warn("update card after stream error failed",
				zap.String("msg_id", cardMsgID), zap.Error(updateErr))
		}
		return
	}

	zap.L().Info("agent reply stream started",
		zap.String("chat_id", msg.ChatID),
		zap.String("msg_id", msg.MsgID))

	var lastUpdate time.Time
	var lastStreamState string
	var finalResp *message.Msg
	cardState := newStreamCardState()

	for streamMsg := range streamCh {
		// 去重优化：如果队列中还有更新的请求就跳过
		if len(streamCh) > 0 {
			continue
		}

		finalResp = streamMsg
		card := cardState.build(streamMsg, agent.Memory(), true)
		stateKey := cardStateKey(card)
		if stateKey == lastStreamState {
			continue
		}
		lastStreamState = stateKey

		zap.L().Debug("agent stream chunk",
			zap.String("chat_id", msg.ChatID),
			zap.String("msg_id", msg.MsgID),
			zap.String("text", card.Text),
			zap.Any("tools", card.Tools),
			zap.String("thinking", card.Thinking))

		if time.Since(lastUpdate) < 300*time.Millisecond {
			continue
		}
		lastUpdate = time.Now()
		if updateErr := ch.bot.UpdateCard(ctx, cardMsgID, card); updateErr != nil {
			zap.L().Warn("update card during stream failed",
				zap.String("msg_id", cardMsgID), zap.Error(updateErr))
		}
	}

	// Final card update: remove streaming cursor
	if finalResp != nil {
		zap.L().Info("agent reply completed",
			zap.String("chat_id", msg.ChatID),
			zap.String("msg_id", msg.MsgID),
			zap.String("response", finalResp.GetTextContent()))

		if updateErr := ch.bot.UpdateCard(ctx, cardMsgID, cardState.build(finalResp, agent.Memory(), false)); updateErr != nil {
			zap.L().Warn("final card update failed",
				zap.String("msg_id", cardMsgID), zap.Error(updateErr))
		}
	}

	// Save after the stream completes so assistant/tool messages are in memory.
	if saveErr := ch.store.SaveSession(ctx, msg.ChatID, agent.Memory()); saveErr != nil {
		zap.L().Error("save session failed",
			zap.String("chat_id", msg.ChatID), zap.Error(saveErr))
	}
}

func (ch *Channel) getLock(chatID string) *sync.Mutex {
	v, _ := ch.locks.LoadOrStore(chatID, &sync.Mutex{})
	return v.(*sync.Mutex)
}

// streamCardState accumulates thinking and tool calls across stream chunks and
// agent iterations so the Lark card keeps showing earlier steps.
type streamCardState struct {
	thinkingSegments []string
	tools            map[string]adapter.ToolEntry
}

func newStreamCardState() *streamCardState {
	return &streamCardState{tools: make(map[string]adapter.ToolEntry)}
}

func (s *streamCardState) build(msg *message.Msg, mem memory.MemoryBase, streaming bool) adapter.CardContent {
	if msg != nil {
		s.mergeThinking(extractThinking(msg))
		mergeToolMap(s.tools, extractToolEntries(msg))
	}
	enrichToolsFromMemory(mem, s.tools)

	text := ""
	if msg != nil {
		text = msg.GetTextContent()
	}
	thinking, text := splitThinkingForDisplay(s.mergedThinking(), text, toolsFromMap(s.tools))

	return adapter.CardContent{
		Tools:     toolsFromMap(s.tools),
		Thinking:  thinking,
		Text:      text,
		Streaming: streaming,
	}
}

// splitThinkingForDisplay moves post-tool synthesis out of thinking blocks.
// Some models stream the final answer into reasoning_content (thinking) while
// leaving text blocks empty after tool execution.
func splitThinkingForDisplay(thinking, text string, tools []adapter.ToolEntry) (string, string) {
	if text != "" || thinking == "" {
		return thinking, text
	}
	if !hasDoneTools(tools) {
		return thinking, text
	}
	for _, sep := range []string{"\n\n\n", "\n\n"} {
		idx := strings.LastIndex(thinking, sep)
		if idx < 0 {
			continue
		}
		suffix := strings.TrimSpace(thinking[idx+len(sep):])
		if suffix == "" {
			continue
		}
		prefix := strings.TrimSpace(thinking[:idx])
		return prefix, suffix
	}
	return thinking, text
}

func hasDoneTools(tools []adapter.ToolEntry) bool {
	for _, t := range tools {
		if t.Done {
			return true
		}
	}
	return false
}

func (s *streamCardState) mergeThinking(t string) {
	if t == "" {
		return
	}
	if len(s.thinkingSegments) == 0 {
		s.thinkingSegments = append(s.thinkingSegments, t)
		return
	}
	last := s.thinkingSegments[len(s.thinkingSegments)-1]
	if t == last || strings.HasPrefix(t, last) {
		s.thinkingSegments[len(s.thinkingSegments)-1] = t
		return
	}
	if strings.HasPrefix(last, t) {
		return
	}
	s.thinkingSegments = append(s.thinkingSegments, t)
}

func (s *streamCardState) mergedThinking() string {
	return strings.Join(s.thinkingSegments, "\n\n")
}

// cardStateKey fingerprints visible stream progress (text, thinking, tools).
// agentscope-go emits a message on every SSE tick; many have empty text while
// thinking or tool-call blocks are still accumulating.
func cardStateKey(card adapter.CardContent) string {
	type snapshot struct {
		Text     string              `json:"text"`
		Thinking string              `json:"thinking,omitempty"`
		Tools    []adapter.ToolEntry `json:"tools,omitempty"`
	}
	snap := snapshot{
		Text:     card.Text,
		Thinking: card.Thinking,
		Tools:    card.Tools,
	}
	b, err := json.Marshal(snap)
	if err != nil {
		return card.Text
	}
	return string(b)
}

func extractThinking(msg *message.Msg) string {
	if msg == nil {
		return ""
	}
	var parts []string
	for _, block := range msg.Content {
		if message.IsThinkingBlock(block) {
			if t := message.GetBlockThinking(block); t != "" {
				parts = append(parts, t)
			}
		}
	}
	return strings.Join(parts, "")
}

func toolMapKey(id, name, args string) string {
	if id != "" {
		return id
	}
	return name + "\x00" + args
}

func mergeToolMap(dst map[string]adapter.ToolEntry, entries []adapter.ToolEntry) {
	for _, te := range entries {
		key := toolMapKey(te.ID, te.Name, te.Args)
		existing := dst[key]
		if te.ID != "" {
			existing.ID = te.ID
		}
		if te.Name != "" {
			existing.Name = te.Name
		}
		if te.Args != "" {
			existing.Args = te.Args
		}
		if te.Result != "" {
			existing.Result = te.Result
		}
		if te.Done {
			existing.Done = te.Done
		}
		dst[key] = existing
	}
}

func toolsFromMap(m map[string]adapter.ToolEntry) []adapter.ToolEntry {
	if len(m) == 0 {
		return nil
	}
	out := make([]adapter.ToolEntry, 0, len(m))
	for _, te := range m {
		out = append(out, te)
	}
	return out
}

func extractToolEntries(msg *message.Msg) []adapter.ToolEntry {
	if msg == nil {
		return nil
	}
	tools := make(map[string]adapter.ToolEntry)
	for _, block := range msg.Content {
		if message.IsToolUseBlock(block) {
			id := message.GetBlockToolUseID(block)
			name := message.GetBlockToolUseName(block)
			if name == "" {
				name = "…"
			}
			args := formatToolInput(message.GetBlockToolUseInput(block))
			key := toolMapKey(id, name, args)
			existing := tools[key]
			existing.ID = id
			existing.Name = name
			existing.Args = args
			tools[key] = existing
		}
		if message.IsToolResultBlock(block) {
			id := message.GetBlockToolResultID(block)
			result := formatToolOutput(message.GetBlockToolResultOutput(block))
			if message.GetBlockToolResultIsError(block) && result != "" {
				result = "❌ " + result
			}
			key := toolMapKey(id, "", "")
			existing := tools[key]
			existing.ID = id
			existing.Result = result
			existing.Done = true
			tools[key] = existing
		}
	}
	return toolsFromMap(tools)
}

func enrichToolsFromMemory(mem memory.MemoryBase, tools map[string]adapter.ToolEntry) {
	if mem == nil || len(tools) == 0 {
		return
	}
	results := make(map[string]adapter.ToolEntry)
	for _, msg := range mem.GetMessages() {
		if msg == nil || msg.Role != "tool" {
			continue
		}
		for _, te := range extractToolEntries(msg) {
			if te.ID != "" {
				results[te.ID] = te
			}
		}
	}
	for key, te := range tools {
		if te.ID == "" {
			continue
		}
		if r, ok := results[te.ID]; ok {
			te.Result = r.Result
			te.Done = r.Done
			tools[key] = te
		}
	}
}

func formatToolInput(input interface{}) string {
	if input == nil {
		return "{}"
	}
	bs, err := json.Marshal(input)
	if err != nil {
		return fmt.Sprintf("%v", input)
	}
	return string(bs)
}

func formatToolOutput(output interface{}) string {
	if output == nil {
		return ""
	}
	switch v := output.(type) {
	case string:
		return v
	default:
		bs, err := json.Marshal(v)
		if err != nil {
			return fmt.Sprintf("%v", v)
		}
		return string(bs)
	}
}
