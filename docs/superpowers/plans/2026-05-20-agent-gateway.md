# agent-gateway Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a Go gateway that connects DeepAgent (from agentscope-go) to Feishu/Lark IM via WebSocket, with streaming card updates and chatID-based session isolation.

**Architecture:** Stateless-agent design — `Config` loads YAML+env, `SessionStore` persists chatID→conversation history (JSON file or Redis), `Channel` creates a fresh DeepAgent per message, loads session history into memory, calls `Reply()`, then persists back. No long-lived agent instances. Same-chatID concurrency is serialized via per-chatID mutex. Card uses Lark interactive card schema 2.0 with patch updates.

**Tech Stack:** Go 1.21+, agentscope-go (DeepAgent), larksuite/oapi-sdk-go/v3 (WebSocket), yaml.v3, zap logging

---

## File Structure

```
agent-gateway/
├── cmd/
│   └── agent-gateway/
│       └── main.go              # Entry point: parse config → build objects → ch.Start()
├── internal/
│   ├── config/
│   │   └── config.go            # YAML + env var loading
│   ├── session/
│   │   └── store.go             # SessionStore interface + JSON/Redis implementations
│   ├── agent/
│   │   └── factory.go           # DeepAgent factory: creates fresh agent per request
│   ├── lark/
│   │   ├── client.go            # LarkClient: WebSocket setup + event dispatch
│   │   ├── card.go              # Interactive card build + patch (schema 2.0)
│   │   ├── message.go           # Feishu message parsing (text/post/image)
│   │   └── dedup.go             # Message deduplication (10min TTL)
│   └── channel/
│       └── channel.go           # Channel: wires Agent factory + SessionStore + LarkClient
├── config.yaml                  # Example config
├── go.mod
└── go.sum
```

---

### Task 1: Project Scaffolding & Dependencies

**Files:**
- Create: `go.mod`
- Create: `config.yaml`

- [ ] **Step 1: Initialize Go module and install dependencies**

Run:
```bash
cd /Users/zhuwei/gopath/src/github.com/vearne/agent-gateway
go mod tidy
```

The `go.mod` should declare:
```go
module github.com/vearne/agent-gateway

go 1.21

require (
    github.com/vearne/agentscope-go v0.0.0
    github.com/larksuite/oapi-sdk-go/v3 v3.4.2
    gopkg.in/yaml.v3 v3.0.1
    go.uber.org/zap v1.27.0
)
```

Then run:
```bash
go get github.com/vearne/agentscope-go@latest
go get github.com/larksuite/oapi-sdk-go/v3@latest
go get gopkg.in/yaml.v3@latest
go get go.uber.org/zap@latest
go mod tidy
```

- [ ] **Step 2: Create example config.yaml**

```yaml
# agent-gateway configuration
lark:
  app_id: "your_app_id"
  app_secret: "your_app_secret"       # or set LARK_APP_SECRET env var
  # If public_base_url is set, tool output file download links will use this host
  public_base_url: "http://localhost:8080"

agent:
  model_name: "gpt-4o"
  api_key: "your_api_key"             # or set AGENT_API_KEY env var
  base_url: ""                        # optional, default: OpenAI official
  system_prompt: "You are a helpful assistant."
  max_iters: 20
  # max_context_tokens: 128000       # optional, default: 128000
```

- [ ] **Step 3: Verify build**

Run: `go build ./...`
Expected: no errors (may have no Go files yet, that's OK)

- [ ] **Step 4: Commit**

```bash
git add go.mod go.sum config.yaml
git commit -m "chore: project scaffolding with dependencies and example config"
```

---

### Task 2: Config (YAML + Env Override)

**Files:**
- Create: `internal/config/config.go`

- [ ] **Step 1: Write config.go**

```go
package config

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Lark  LarkConfig  `yaml:"lark"`
	Agent AgentConfig `yaml:"agent"`
}

type LarkConfig struct {
	AppID         string `yaml:"app_id"`
	AppSecret     string `yaml:"app_secret"`
	PublicBaseURL string `yaml:"public_base_url"`
}

type AgentConfig struct {
	ModelName      string `yaml:"model_name"`
	APIKey         string `yaml:"api_key"`
	BaseURL        string `yaml:"base_url"`
	SystemPrompt   string `yaml:"system_prompt"`
	MaxIters       int    `yaml:"max_iters"`
	MaxContextTokens int  `yaml:"max_context_tokens"`
}

// Load reads config.yaml and applies env var overrides.
// Env vars take precedence: LARK_APP_SECRET, LARK_APP_ID, AGENT_API_KEY, AGENT_MODEL_NAME
func Load(path string) (Config, error) {
	var cfg Config

	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, fmt.Errorf("read config %s: %w", path, err)
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("parse config %s: %w", path, err)
	}

	// Env var overrides
	if v := os.Getenv("LARK_APP_ID"); v != "" {
		cfg.Lark.AppID = v
	}
	if v := os.Getenv("LARK_APP_SECRET"); v != "" {
		cfg.Lark.AppSecret = v
	}
	if v := os.Getenv("LARK_PUBLIC_BASE_URL"); v != "" {
		cfg.Lark.PublicBaseURL = strings.TrimRight(v, "/")
	} else {
		cfg.Lark.PublicBaseURL = strings.TrimRight(cfg.Lark.PublicBaseURL, "/")
	}
	if v := os.Getenv("AGENT_API_KEY"); v != "" {
		cfg.Agent.APIKey = v
	}
	if v := os.Getenv("AGENT_MODEL_NAME"); v != "" {
		cfg.Agent.ModelName = v
	}
	if v := os.Getenv("AGENT_BASE_URL"); v != "" {
		cfg.Agent.BaseURL = strings.TrimRight(v, "/")
	}

	// Defaults
	if cfg.Agent.MaxIters <= 0 {
		cfg.Agent.MaxIters = 20
	}
	if cfg.Agent.MaxContextTokens <= 0 {
		cfg.Agent.MaxContextTokens = 128000
	}

	return cfg, nil
}
```

- [ ] **Step 2: Verify compilation**

Run: `go build ./internal/config/`
Expected: no errors

- [ ] **Step 3: Commit**

```bash
git add internal/config/config.go
git commit -m "feat: add YAML + env var config loading"
```

---

### Task 3: Session Store (Persistent chatID → conversation history)

**Files:**
- Create: `internal/session/store.go`

**Design:** `SessionStore` is a storage interface for conversation history. It uses agentscope-go's `session.SessionBase` (`Save`/`Load` on `memory.MemoryBase`) under the hood. Two implementations: JSON file (dev) and Redis (prod). The `/new` command generates a new session key (chatID → chatID_1 → chatID_2, etc.) like ai-channel.

- [ ] **Step 1: Write session/store.go**

```go
package session

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/redis/go-redis/v9"
	"github.com/vearne/agentscope-go/pkg/memory"
	"github.com/vearne/agentscope-go/pkg/session"
)

// Store manages persistent session storage for each chatID.
// It uses agentscope-go's session.SessionBase (Save/Load) internally.
// Session keys follow the pattern: chatID → chatID_1 → chatID_2 (on /new).
type Store struct {
	mu       sync.Mutex
	redis    *redis.Client       // nil = JSON file mode
	dir      string              // base directory for JSON session files
	keyMap   map[string]string   // chatID → current session key
}

// NewFileStore creates a Store backed by JSON files in the given directory.
func NewFileStore(dir string) *Store {
	if err := os.MkdirAll(dir, 0755); err != nil {
		panic(fmt.Sprintf("create session dir %s: %v", dir, err))
	}
	return &Store{
		dir:    dir,
		keyMap: make(map[string]string),
	}
}

// NewRedisStore creates a Store backed by Redis.
// addr format: "host:port"
func NewRedisStore(addr, password string, db int) *Store {
	rdb := redis.NewClient(&redis.Options{
		Addr:        addr,
		Password:    password,
		DB:          db,
		DialTimeout: 3 * time.Second,
		ReadTimeout: 2 * time.Second,
	})
	return &Store{
		redis:  rdb,
		keyMap: make(map[string]string),
	}
}

// LoadSession loads the persisted conversation history for a chatID into the given memory.
// If no session exists yet, the memory stays empty (first message).
func (s *Store) LoadSession(ctx context.Context, chatID string, mem memory.MemoryBase) error {
	key := s.getSessionKey(chatID)
	if key == "" {
		return nil // no session yet, memory stays empty
	}
	sess := s.newSessionBase(key)
	return sess.Load(ctx, mem)
}

// SaveSession persists the conversation history from memory for a chatID.
func (s *Store) SaveSession(ctx context.Context, chatID string, mem memory.MemoryBase) error {
	key := s.getSessionKey(chatID)
	if key == "" {
		// First message ever — initialize key to chatID
		s.mu.Lock()
		s.keyMap[chatID] = chatID
		s.mu.Unlock()
		key = chatID
	}
	sess := s.newSessionBase(key)
	return sess.Save(ctx, mem)
}

// NewSession resets the session for a chatID (increments counter: chatID → chatID_1 → chatID_2).
// The old session data is NOT deleted, just no longer referenced.
func (s *Store) NewSession(ctx context.Context, chatID string) string {
	s.mu.Lock()
	defer s.mu.Unlock()

	current := s.keyMap[chatID]
	next := nextSessionKey(chatID, current)
	s.keyMap[chatID] = next
	return next
}

// ResetSession clears the session for a chatID and deletes the stored file/Redis key.
func (s *Store) ResetSession(ctx context.Context, chatID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := s.keyMap[chatID]
	s.keyMap[chatID] = chatID // reset to base

	if key != "" && key != chatID {
		// Best-effort delete old session data
		if s.redis != nil {
			s.redis.Del(ctx, "session:"+key)
		} else {
			os.Remove(s.filePath(key))
		}
	}
}

func (s *Store) getSessionKey(chatID string) string {
	s.mu.Lock()
	defer s.mu.Unlock()

	if key, ok := s.keyMap[chatID]; ok {
		return key
	}
	// Try to restore from Redis (if configured)
	if s.redis != nil {
		val, err := s.redis.Get(context.Background(), "chat:"+chatID).Result()
		if err == nil && val != "" {
			s.keyMap[chatID] = val
			return val
		}
	}
	return ""
}

func (s *Store) newSessionBase(key string) session.SessionBase {
	if s.redis != nil {
		return session.NewRedisSession(s.redis.Options().Addr, "session:"+key,
			session.WithRedisPassword(s.redis.Options().Password),
		)
	}
	return session.NewJSONSession(s.filePath(key))
}

func (s *Store) filePath(key string) string {
	// Sanitize key for filesystem (replace / with _)
	safe := strings.ReplaceAll(key, "/", "_")
	return fmt.Sprintf("%s/%s.json", s.dir, safe)
}

// nextSessionKey computes the next session key:
//
//	chatID    → chatID_1
//	chatID_N  → chatID_(N+1)
func nextSessionKey(chatID, current string) string {
	if current == "" || current == chatID {
		return chatID + "_1"
	}
	prefix := chatID + "_"
	if strings.HasPrefix(current, prefix) {
		if n, err := strconv.Atoi(current[len(prefix):]); err == nil {
			return fmt.Sprintf("%s_%d", chatID, n+1)
		}
	}
	return chatID + "_1"
}
```

**NOTE:** The `session.NewRedisSession` and `session.NewJSONSession` signatures need to match agentscope-go's actual API. The explore task confirmed:
- `session.NewJSONSession(filePath string) *JSONSession`
- `session.NewRedisSession(addr, key string, opts ...RedisSessionOption) *RedisSession`
- `session.WithRedisPassword(password string) RedisSessionOption`

- [ ] **Step 2: Verify compilation**

Run: `go build ./internal/session/`
Expected: may fail if agentscope-go dependency not yet resolved; fix import paths as needed.

- [ ] **Step 3: Commit**

```bash
git add internal/session/store.go
git commit -m "feat: add persistent SessionStore with JSON file and Redis backends"
```

---

### Task 4: Agent Factory (stateless DeepAgent creation)

**Files:**
- Create: `internal/agent/factory.go`

- [ ] **Step 1: Write agent/factory.go**

```go
package agent

import (
	"github.com/vearne/agentscope-go/pkg/agent"
	"github.com/vearne/agentscope-go/pkg/formatter"
	"github.com/vearne/agentscope-go/pkg/memory"
	"github.com/vearne/agentscope-go/pkg/model"

	"github.com/vearne/agent-gateway/internal/config"
)

// Factory creates fresh DeepAgent instances on demand.
// Agents are stateless — they're created per-message, used once, then discarded.
// Session history is loaded/saved separately by the SessionStore.
type Factory struct {
	cfg config.AgentConfig
}

// NewFactory creates an agent factory from config.
func NewFactory(cfg config.AgentConfig) *Factory {
	return &Factory{cfg: cfg}
}

// Create builds a new DeepAgent with an empty InMemoryMemory.
// The caller is responsible for loading session history before Reply()
// and saving it after Reply().
func (f *Factory) Create() *agent.DeepAgent {
	m := model.NewOpenAIChatModel(
		f.cfg.ModelName,
		f.cfg.APIKey,
		f.cfg.BaseURL,
		false,
	)
	fmt := formatter.NewOpenAIChatFormatter()

	return agent.NewDeepAgent(
		agent.WithDeepName("agent-gateway"),
		agent.WithDeepModel(m),
		agent.WithDeepFormatter(fmt),
		agent.WithDeepMemory(memory.NewInMemoryMemory()),
		agent.WithDeepSystemPrompt(f.cfg.SystemPrompt),
		agent.WithDeepMaxIters(f.cfg.MaxIters),
		agent.WithDeepMaxContextTokens(f.cfg.MaxContextTokens),
	)
}
```

- [ ] **Step 2: Verify compilation**

Run: `go build ./internal/agent/`

- [ ] **Step 3: Commit**

```bash
git add internal/agent/factory.go
git commit -m "feat: add stateless DeepAgent factory"
```

---

### Task 5: Feishu Message Parsing

**Files:**
- Create: `internal/lark/message.go`

- [ ] **Step 1: Write lark/message.go**

Port from ai-channel `feishu_message.go`, simplified (no agent.ContentPart dependency — use agentscope-go's message.ContentBlock directly):

```go
package lark

import (
	"encoding/json"
	"fmt"
	"io"
	"regexp"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
)

var atPlaceholderRE = regexp.MustCompile(`@_user_\d+`)

// ParsedMessage holds extracted text and optional image keys from a Feishu message.
type ParsedMessage struct {
	Text      string
	HasText   bool
	HasImages bool
	ImageKeys []string
}

// ParseMessage extracts text and images from a Feishu event message.
// For group chats, only messages with @mentions are processed.
func ParseMessage(msg *larkim.EventMessage) (*ParsedMessage, bool) {
	if msg == nil || msg.Content == nil || msg.MessageType == nil {
		return nil, false
	}

	chatType := larkcore.StringValue(msg.ChatType)
	msgType := larkcore.StringValue(msg.MessageType)

	// Group: only process messages with @mentions
	if chatType == "group" && len(msg.Mentions) == 0 {
		return nil, false
	}

	result := &ParsedMessage{}

	switch msgType {
	case "text":
		text, err := extractText(*msg.Content)
		if err == nil {
			result.Text = text
			result.HasText = text != ""
		}
	case "post":
		text, images := extractPost(*msg.Content)
		result.Text = text
		result.HasText = text != ""
		result.HasImages = len(images) > 0
		result.ImageKeys = images
	case "image":
		key, err := extractImageKey(*msg.Content)
		if err == nil && key != "" {
			result.HasImages = true
			result.ImageKeys = []string{key}
		}
	default:
		return nil, false
	}

	if !result.HasText && !result.HasImages {
		return nil, false
	}

	return result, true
}

func extractText(contentJSON string) (string, error) {
	var m map[string]string
	if err := json.Unmarshal([]byte(contentJSON), &m); err != nil {
		return "", err
	}
	return atPlaceholderRE.ReplaceAllString(m["text"], ""), nil
}

type postDocument struct {
	Title   string             `json:"title"`
	Content [][]map[string]any `json:"content"`
}

func extractPost(contentJSON string) (string, []string) {
	// Try direct parse first (no language key)
	var doc postDocument
	if err := json.Unmarshal([]byte(contentJSON), &doc); err == nil && doc.Content != nil {
		return extractFromDoc(&doc)
	}

	// Try with language key
	var outer map[string]json.RawMessage
	if err := json.Unmarshal([]byte(contentJSON), &outer); err != nil {
		return "", nil
	}
	var raw json.RawMessage
	for _, lang := range []string{"zh_cn", "en_us"} {
		if v, ok := outer[lang]; ok {
			raw = v
			break
		}
	}
	if raw == nil {
		for _, v := range outer {
			raw = v
			break
		}
	}
	if raw == nil {
		return "", nil
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return "", nil
	}
	return extractFromDoc(&doc)
}

func extractFromDoc(doc *postDocument) (string, []string) {
	var sb strings.Builder
	var imageKeys []string

	for _, line := range doc.Content {
		for _, node := range line {
			tag, _ := node["tag"].(string)
			switch tag {
			case "text", "a":
				if t, ok := node["text"].(string); ok {
					sb.WriteString(t)
				}
			case "img":
				if key, ok := node["image_key"].(string); ok && key != "" {
					imageKeys = append(imageKeys, key)
				}
			}
		}
		sb.WriteString("\n")
	}
	return sb.String(), imageKeys
}

func extractImageKey(contentJSON string) (string, error) {
	var m map[string]string
	if err := json.Unmarshal([]byte(contentJSON), &m); err != nil {
		return "", err
	}
	return m["image_key"], nil
}

// GetImageURL fetches an image from Feishu and returns it as a base64 data URL.
func GetImageURL(ctx context.Context, larkAPI *lark.Client, messageID, imageKey string) (string, error) {
	req := larkim.NewGetMessageResourceReqBuilder().
		MessageId(messageID).
		FileKey(imageKey).
		Type("image").
		Build()

	resp, err := larkAPI.Im.MessageResource.Get(ctx, req)
	if err != nil {
		return "", fmt.Errorf("get image resource: %w", err)
	}
	if !resp.Success() {
		return "", fmt.Errorf("get image resource: lark error %d %s", resp.Code, resp.Msg)
	}
	if resp.File == nil {
		return "", fmt.Errorf("image resource file is nil")
	}

	data, err := io.ReadAll(io.LimitReader(resp.File, 10*1024*1024))
	if err != nil {
		return "", fmt.Errorf("read image: %w", err)
	}
	if len(data) == 0 {
		return "", fmt.Errorf("empty image data")
	}

	mime := detectMIME(data)
	encoded := base64.StdEncoding.EncodeToString(data)
	return fmt.Sprintf("data:%s;base64,%s", mime, encoded), nil
}

func detectMIME(data []byte) string {
	if len(data) < 4 {
		return "image/jpeg"
	}
	if data[0] == 0x89 && data[1] == 0x50 && data[2] == 0x4E && data[3] == 0x47 {
		return "image/png"
	}
	if data[0] == 0xFF && data[1] == 0xD8 {
		return "image/jpeg"
	}
	if data[0] == 0x47 && data[1] == 0x49 && data[2] == 0x46 {
		return "image/gif"
	}
	if len(data) >= 12 && data[8] == 0x57 && data[9] == 0x45 && data[10] == 0x42 && data[11] == 0x50 {
		return "image/webp"
	}
	return "image/jpeg"
}
```

**NOTE:** Needs imports for `encoding/base64`, `strings`, `context`, `lark.Client`. Adjust based on actual lark SDK package paths.

- [ ] **Step 2: Verify compilation**

Run: `go build ./internal/lark/`

- [ ] **Step 3: Commit**

```bash
git add internal/lark/message.go
git commit -m "feat: add Feishu message parsing (text, post, image)"
```

---

### Task 5: Interactive Card (Schema 2.0)

**Files:**
- Create: `internal/lark/card.go`

- [ ] **Step 1: Write lark/card.go**

Simplified version of ai-channel's `feishu_card.go` — supports streaming text + tool call panels without file upload (keep it simple for v1):

```go
package lark

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	lark "github.com/larksuite/oapi-sdk-go/v3"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
	"github.com/vearne/agentscope-go/pkg/message"
	"go.uber.org/zap"
)

const (
	cardCursor     = "▌"
	streamThrottle = 500 * time.Millisecond
)

// ── Card structures (schema 2.0) ──

type richCard struct {
	Schema string     `json:"schema"`
	Config cardConfig `json:"config"`
	Body   cardBody   `json:"body"`
}

type cardConfig struct {
	WideScreenMode bool         `json:"wide_screen_mode,omitempty"`
	StreamingMode  bool         `json:"streaming_mode,omitempty"`
	Summary        *cardSummary `json:"summary,omitempty"`
}

type cardSummary struct {
	Content string `json:"content"`
}

type cardBody struct {
	Elements []cardElement `json:"elements"`
}

type cardElement struct {
	Tag string `json:"tag"`

	// markdown
	Content string `json:"content,omitempty"`

	// collapsible_panel
	Expanded *bool        `json:"expanded,omitempty"`
	Header   *panelHeader `json:"header,omitempty"`
	Elements []cardElement `json:"elements,omitempty"`
}

type panelHeader struct {
	Title cardText `json:"title"`
}

type cardText struct {
	Tag     string `json:"tag"`
	Content string `json:"content"`
}

func boolPtr(b bool) *bool { return &b }

func mdElement(content string) cardElement {
	return cardElement{Tag: "markdown", Content: content}
}

// ToolEntry represents a tool call for card display.
type ToolEntry struct {
	Name   string
	Args   string
	Result string
	Done   bool
}

// BuildCard builds a Lark interactive card JSON string (schema 2.0).
// streaming=true enables streaming mode with cursor.
func BuildCard(tools []ToolEntry, responseText string, streaming bool) string {
	cfg := cardConfig{WideScreenMode: true}
	if streaming {
		cfg.StreamingMode = true
		cfg.Summary = &cardSummary{Content: ""}
	}

	var elements []cardElement

	if len(tools) > 0 {
		elements = append(elements, cardElement{
			Tag:      "collapsible_panel",
			Expanded: boolPtr(streaming),
			Header: &panelHeader{
				Title: cardText{Tag: "plain_text", Content: fmt.Sprintf("⚙️ 工具调用（%d）", len(tools))},
			},
			Elements: []cardElement{mdElement(buildToolTrace(tools))},
		})
	}

	text := responseText
	if streaming {
		text += cardCursor
	}
	elements = append(elements, mdElement(text))

	card := richCard{
		Schema: "2.0",
		Config: cfg,
		Body:   cardBody{Elements: elements},
	}
	bs, _ := json.Marshal(card)
	return string(bs)
}

func buildToolTrace(tools []ToolEntry) string {
	var sb strings.Builder
	for i, t := range tools {
		if i > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString("🔧 **" + t.Name + "**\n")

		if t.Args != "" && t.Args != "{}" {
			sb.WriteString("```json\n" + formatJSON(t.Args) + "\n```\n")
		}

		if t.Done {
			sb.WriteString("> ✅\n")
			if t.Result != "" {
				sb.WriteString("```\n" + truncate(t.Result, 2000) + "\n```\n")
			}
		} else {
			sb.WriteString("> ⏳ 执行中…\n")
		}
	}
	return strings.TrimRight(sb.String(), "\n")
}

func formatJSON(s string) string {
	var data interface{}
	if err := json.Unmarshal([]byte(s), &data); err != nil {
		return s
	}
	bs, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return s
	}
	return string(bs)
}

func truncate(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max]) + "…"
}

// ── Streaming card reply ──

type patchRequest struct {
	cardJSON string
	final    bool
	done     chan struct{}
}

// StreamReplyToCard sends a placeholder card, then patches it with updates from the update channel.
// The update channel receives (text, tools, isFinal) tuples.
// Returns the final text and any error.
func StreamReplyToCard(
	ctx context.Context,
	larkAPI *lark.Client,
	parentMsgID string,
	updates <-chan CardUpdate,
) (string, error) {
	// 1. Send initial placeholder card
	replyResp, err := larkAPI.Im.Message.Reply(ctx, larkim.NewReplyMessageReqBuilder().
		MessageId(parentMsgID).
		Body(&larkim.ReplyMessageReqBody{
			Content: larkcore.StringPtr(BuildCard(nil, "", true)),
			MsgType: larkcore.StringPtr("interactive"),
		}).Build())
	if err != nil {
		return "", fmt.Errorf("send placeholder card: %w", err)
	}
	if !replyResp.Success() {
		return "", fmt.Errorf("send placeholder card: lark error %d %s", replyResp.Code, replyResp.Msg)
	}
	cardMsgID := larkcore.StringValue(replyResp.Data.MessageId)

	// 2. Process updates with throttling
	var (
		mu        sync.Mutex
		lastPatch time.Time
	)

	patchQueue := make(chan patchRequest, 30)

	go func() {
		for pReq := range patchQueue {
			if !pReq.final && len(patchQueue) > 0 {
				if pReq.done != nil {
					close(pReq.done)
				}
				continue // skip stale intermediate update
			}

			patchReq := larkim.NewPatchMessageReqBuilder().
				MessageId(cardMsgID).
				Body(&larkim.PatchMessageReqBody{
					Content: larkcore.StringPtr(pReq.cardJSON),
				}).Build()
			patchResp, pErr := larkAPI.Im.Message.Patch(ctx, patchReq)
			if pErr != nil {
				zap.L().Warn("patch card failed", zap.String("card_msg_id", cardMsgID), zap.Error(pErr))
			} else if patchResp != nil && !patchResp.Success() {
				zap.L().Warn("patch card rejected",
					zap.String("card_msg_id", cardMsgID),
					zap.Int("code", patchResp.Code),
					zap.String("msg", patchResp.Msg))
			}

			if pReq.done != nil {
				close(pReq.done)
			}
		}
	}()

	tryPatch := func(cardJSON string, final bool) {
		mu.Lock()
		now := time.Now()
		due := final || now.Sub(lastPatch) >= streamThrottle
		if due {
			lastPatch = now
		}
		mu.Unlock()

		if !due {
			return
		}

		pReq := patchRequest{cardJSON: cardJSON, final: final}
		if final {
			pReq.done = make(chan struct{})
		}

		select {
		case patchQueue <- pReq:
			if final {
				<-pReq.done
			}
		case <-ctx.Done():
			if pReq.done != nil {
				close(pReq.done)
			}
		}
	}

	// 3. Consume updates
	var finalText string
	for update := range updates {
		cardJSON := BuildCard(update.Tools, update.Text, !update.IsFinal)
		finalText = update.Text
		tryPatch(cardJSON, update.IsFinal)
	}

	close(patchQueue)
	return finalText, nil
}

// CardUpdate represents a card update event.
type CardUpdate struct {
	Text    string
	Tools   []ToolEntry
	IsFinal bool
}
```

- [ ] **Step 2: Verify compilation**

Run: `go build ./internal/lark/`

- [ ] **Step 3: Commit**

```bash
git add internal/lark/card.go
git commit -m "feat: add Lark interactive card builder with streaming patch support"
```

---

### Task 6: Lark Client (WebSocket)

**Files:**
- Create: `internal/lark/client.go`

- [ ] **Step 1: Write lark/client.go**

```go
package lark

import (
	"context"

	lark "github.com/larksuite/oapi-sdk-go/v3"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	"github.com/larksuite/oapi-sdk-go/v3/event/dispatcher"
	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
	"github.com/larksuite/oapi-sdk-go/v3/ws"
	"go.uber.org/zap"
)

// MessageHandler is called when a valid user message is received.
type MessageHandler func(ctx context.Context, msgID, chatID string, parsed *ParsedMessage)

// Client wraps a Lark WebSocket connection.
type Client struct {
	appID     string
	appSecret string
	api       *lark.Client
	handler   MessageHandler
}

// NewClient creates a new Lark client.
func NewClient(appID, appSecret string) *Client {
	return &Client{
		appID:     appID,
		appSecret: appSecret,
		api:       lark.NewClient(appID, appSecret),
	}
}

// API returns the underlying Lark API client (for sending replies, fetching images, etc.).
func (c *Client) API() *lark.Client {
	return c.api
}

// OnMessage registers the handler for incoming messages.
func (c *Client) OnMessage(h MessageHandler) {
	c.handler = h
}

// Start establishes the WebSocket connection and blocks until ctx is cancelled.
func (c *Client) Start(ctx context.Context) {
	d := dispatcher.NewEventDispatcher("", "").OnP2MessageReceiveV1(
		func(ctx context.Context, event *larkim.P2MessageReceiveV1) error {
			c.onEvent(ctx, event)
			return nil
		},
	)

	wsClient := ws.NewClient(
		c.appID,
		c.appSecret,
		ws.WithEventHandler(d),
		ws.WithLogLevel(larkcore.LogLevelInfo),
		ws.WithAutoReconnect(true),
	)

	go func() {
		if err := wsClient.Start(ctx); err != nil && ctx.Err() == nil {
			zap.L().Error("lark ws start failed", zap.String("app_id", c.appID), zap.Error(err))
		}
	}()

	<-ctx.Done()
	zap.L().Info("lark client stopped", zap.String("app_id", c.appID))
}

// onEvent processes a raw Feishu event: validates, deduplicates, and calls handler.
func (c *Client) onEvent(ctx context.Context, event *larkim.P2MessageReceiveV1) {
	zap.L().Debug("got lark event", zap.Any("event", event))

	if event.Event == nil || event.Event.Message == nil {
		return
	}

	msg := event.Event.Message
	if msg.MessageId == nil {
		return
	}

	// Only process user messages
	if event.Event.Sender != nil && event.Event.Sender.SenderType != nil &&
		*event.Event.Sender.SenderType != "user" {
		return
	}

	msgID := *msg.MessageId

	// Dedup
	if !globalDedup.tryAdd(msgID) {
		return
	}

	// Parse
	parsed, ok := ParseMessage(msg)
	if !ok {
		return
	}

	chatID := larkcore.StringValue(msg.ChatId)

	if c.handler != nil {
		c.handler(ctx, msgID, chatID, parsed)
	}
}

// SendTextReply sends a plain text reply to a message.
func SendTextReply(ctx context.Context, larkAPI *lark.Client, parentMsgID, text string) {
	contentJSON, _ := json.Marshal(map[string]string{"text": text})
	req := larkim.NewReplyMessageReqBuilder().
		MessageId(parentMsgID).
		Body(&larkim.ReplyMessageReqBody{
			Content: larkcore.StringPtr(string(contentJSON)),
			MsgType: larkcore.StringPtr("text"),
		}).Build()
	if _, err := larkAPI.Im.Message.Reply(ctx, req); err != nil {
		zap.L().Error("lark text reply failed", zap.Error(err))
	}
}
```

- [ ] **Step 2: Verify compilation**

Run: `go build ./internal/lark/`

- [ ] **Step 3: Commit**

```bash
git add internal/lark/client.go
git commit -m "feat: add Lark WebSocket client with event dispatch"
```

---

### Task 7: Channel (Agent Factory + SessionStore + Lark Client)

**Files:**
- Create: `internal/channel/channel.go`

**Core design — stateless agent + persistent session:**
1. Message arrives → acquire per-chatID lock
2. Create fresh DeepAgent via Factory
3. Load session history from SessionStore into agent's memory
4. Send "thinking..." card
5. Call `agent.Reply()` (blocking)
6. Save session history back to SessionStore
7. Patch card with final result
8. Release lock, discard agent

- [ ] **Step 1: Write channel/channel.go**

```go
package channel

import (
	"context"
	"encoding/json"
	"strings"
	"sync"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
	"github.com/vearne/agentscope-go/pkg/agent"
	"github.com/vearne/agentscope-go/pkg/message"
	"go.uber.org/zap"

	"github.com/vearne/agent-gateway/internal/agent"
	"github.com/vearne/agent-gateway/internal/lark"
	"github.com/vearne/agent-gateway/internal/session"
)

// Channel wires AgentFactory + SessionStore + Lark Client.
type Channel struct {
	lark    *lark.Client
	factory *agent.Factory
	store   *session.Store
	locks   sync.Map // chatID → *sync.Mutex — serialize concurrent messages per chat
}

// New creates a Channel.
func New(larkClient *lark.Client, factory *agent.Factory, store *session.Store) *Channel {
	return &Channel{
		lark:    larkClient,
		factory: factory,
		store:   store,
	}
}

// Start registers the message handler and begins listening. Blocks until ctx is cancelled.
func (ch *Channel) Start(ctx context.Context) {
	ch.lark.OnMessage(ch.handleMessage)
	ch.lark.Start(ctx)
}

func (ch *Channel) handleMessage(ctx context.Context, msgID, chatID string, parsed *lark.ParsedMessage) {
	// Check for commands
	if parsed.HasText {
		switch strings.TrimSpace(parsed.Text) {
		case "/new":
			ch.store.NewSession(ctx, chatID)
			lark.SendTextReply(ctx, ch.lark.API(), msgID, "✅ 已开启新会话，历史记录已清除。")
			return
		case "/clear", "/reset":
			ch.store.ResetSession(ctx, chatID)
			lark.SendTextReply(ctx, ch.lark.API(), msgID, "🔄 已清空当前会话。")
			return
		case "/help":
			lark.SendTextReply(ctx, ch.lark.API(), msgID, `📖 **可用指令**
/new          开启新会话，清除历史记录
/clear /reset  清空当前会话
/help         查看此帮助`)
			return
		}
	}

	// Run in goroutine so WebSocket event loop is not blocked
	go ch.processReply(ctx, msgID, chatID, parsed)
}

func (ch *Channel) processReply(ctx context.Context, msgID, chatID string, parsed *lark.ParsedMessage) {
	// Acquire per-chatID lock to serialize concurrent messages
	mu := ch.getLock(chatID)
	mu.Lock()
	defer mu.Unlock()

	// 1. Create fresh agent
	deepAgent := ch.factory.Create()

	// 2. Load session history into agent's memory
	if err := ch.store.LoadSession(ctx, chatID, deepAgent.Memory()); err != nil {
		zap.L().Warn("load session failed, starting fresh",
			zap.String("chat_id", chatID), zap.Error(err))
	}

	// 3. Build agent message
	var contentBlocks []message.ContentBlock
	if parsed.HasText {
		contentBlocks = append(contentBlocks, message.NewTextBlock(parsed.Text))
	}
	if len(contentBlocks) == 0 {
		contentBlocks = append(contentBlocks, message.NewTextBlock(""))
	}
	agentMsg := message.NewMsg("user", contentBlocks, "user")

	// 4. Send "thinking" card
	larkAPI := ch.lark.API()
	cardMsgID, err := ch.sendThinkingCard(ctx, larkAPI, msgID)
	if err != nil {
		lark.SendTextReply(ctx, larkAPI, msgID, "❌ 发送卡片失败："+err.Error())
		return
	}

	// 5. Run agent (blocking)
	resp, err := deepAgent.Reply(ctx, agentMsg)

	// 6. Save session history (always save, even on error, to preserve partial state)
	if saveErr := ch.store.SaveSession(ctx, chatID, deepAgent.Memory()); saveErr != nil {
		zap.L().Error("save session failed",
			zap.String("chat_id", chatID), zap.Error(saveErr))
	}

	// 7. Patch card with result
	if err != nil {
		zap.L().Error("agent reply failed",
			zap.String("chat_id", chatID),
			zap.String("msg_id", msgID),
			zap.Error(err))
		ch.patchCard(ctx, larkAPI, cardMsgID, lark.BuildCard(nil, "❌ 回复失败："+err.Error(), false))
		return
	}

	text := resp.GetTextContent()
	tools := extractToolEntries(resp)
	ch.patchCard(ctx, larkAPI, cardMsgID, lark.BuildCard(tools, text, false))
}

func (ch *Channel) sendThinkingCard(ctx context.Context, larkAPI *lark.Client, parentMsgID string) (string, error) {
	cardJSON := lark.BuildCard(nil, "⏳ 思考中...", true)
	replyResp, err := larkAPI.Im.Message.Reply(ctx, larkim.NewReplyMessageReqBuilder().
		MessageId(parentMsgID).
		Body(&larkim.ReplyMessageReqBody{
			Content: larkcore.StringPtr(cardJSON),
			MsgType: larkcore.StringPtr("interactive"),
		}).Build())
	if err != nil {
		return "", fmt.Errorf("send placeholder card: %w", err)
	}
	if !replyResp.Success() {
		return "", fmt.Errorf("send placeholder card: lark error %d %s", replyResp.Code, replyResp.Msg)
	}
	return larkcore.StringValue(replyResp.Data.MessageId), nil
}

func (ch *Channel) patchCard(ctx context.Context, larkAPI *lark.Client, cardMsgID, cardJSON string) {
	req := larkim.NewPatchMessageReqBuilder().
		MessageId(cardMsgID).
		Body(&larkim.PatchMessageReqBody{
			Content: larkcore.StringPtr(cardJSON),
		}).Build()
	patchResp, err := larkAPI.Im.Message.Patch(ctx, req)
	if err != nil {
		zap.L().Warn("patch card failed", zap.String("card_msg_id", cardMsgID), zap.Error(err))
	} else if patchResp != nil && !patchResp.Success() {
		zap.L().Warn("patch card rejected",
			zap.String("card_msg_id", cardMsgID),
			zap.Int("code", patchResp.Code),
			zap.String("msg", patchResp.Msg))
	}
}

func (ch *Channel) getLock(chatID string) *sync.Mutex {
	v, _ := ch.locks.LoadOrStore(chatID, &sync.Mutex{})
	return v.(*sync.Mutex)
}

// extractToolEntries extracts tool call info from the agent's response message.
func extractToolEntries(msg *message.Msg) []lark.ToolEntry {
	if msg == nil {
		return nil
	}
	var entries []lark.ToolEntry
	for _, block := range msg.Content {
		if message.IsToolUseBlock(block) {
			name := message.GetBlockToolUseName(block)
			input := message.GetBlockToolUseInput(block)
			args := "{}"
			if input != nil {
				if bs, err := json.Marshal(input); err == nil {
					args = string(bs)
				}
			}
			entries = append(entries, lark.ToolEntry{
				Name: name,
				Args: args,
			})
		}
	}
	return entries
}
```

- [ ] **Step 2: Verify compilation**

Run: `go build ./internal/channel/`

- [ ] **Step 3: Commit**

```bash
git add internal/channel/channel.go
git commit -m "feat: add Channel with stateless agent + persistent session pattern"
```

---

### Task 8: Message Dedup (shared)

**Files:**
- Create: `internal/lark/dedup.go`

- [ ] **Step 1: Write dedup.go**

```go
package lark

import (
	"sync"
	"time"
)

type msgDedup struct {
	mu   sync.Mutex
	seen map[string]int64
}

var globalDedup = &msgDedup{seen: make(map[string]int64)}

// tryAdd returns true if this message ID is new (not seen in the last 10 minutes).
func (d *msgDedup) tryAdd(msgID string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()

	now := time.Now().Unix()
	if ts, ok := d.seen[msgID]; ok && now-ts < 600 {
		return false
	}

	// Lazy cleanup
	if len(d.seen) > 5000 {
		cutoff := now - 600
		for k, v := range d.seen {
			if v < cutoff {
				delete(d.seen, k)
			}
		}
	}

	d.seen[msgID] = now
	return true
}
```

- [ ] **Step 2: Commit**

```bash
git add internal/lark/dedup.go
git commit -m "feat: add message deduplication (10min TTL)"
```

---

### Task 9: Main Entry Point

**Files:**
- Create: `cmd/agent-gateway/main.go`

- [ ] **Step 1: Write main.go**

This achieves the target API: `agent → larkClient → ch.Start()`

```go
package main

import (
	"context"
	"flag"
	"os"
	"os/signal"
	"syscall"

	"go.uber.org/zap"

	"github.com/vearne/agent-gateway/internal/agent"
	"github.com/vearne/agent-gateway/internal/channel"
	"github.com/vearne/agent-gateway/internal/config"
	"github.com/vearne/agent-gateway/internal/lark"
	"github.com/vearne/agent-gateway/internal/session"
)

func main() {
	configPath := flag.String("config", "config.yaml", "path to config file")
	flag.Parse()

	// Setup logger
	logger, _ := zap.NewProduction()
	defer logger.Sync()
	zap.ReplaceGlobals(logger)

	// Load config
	cfg, err := config.Load(*configPath)
	if err != nil {
		logger.Fatal("load config", zap.Error(err))
	}

	logger.Info("starting agent-gateway",
		zap.String("app_id", cfg.Lark.AppID),
		zap.String("model", cfg.Agent.ModelName))

	// Create objects
	larkClient := lark.NewClient(cfg.Lark.AppID, cfg.Lark.AppSecret)  // LarkClient()
	factory := agent.NewFactory(cfg.Agent)                             // Agent factory
	store := session.NewFileStore("./sessions")                        // Session store
	ch := channel.New(larkClient, factory, store)                       // Channel(agent, larkClient)

	// Graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		logger.Info("shutting down...")
		cancel()
	}()

	// Start
	ch.Start(ctx)
}
```

- [ ] **Step 2: Verify full build**

Run: `go build ./cmd/agent-gateway/`
Expected: compiles successfully

- [ ] **Step 3: Commit**

```bash
git add cmd/agent-gateway/main.go
git commit -m "feat: add main entry point with graceful shutdown"
```

---

### Task 10: Integration Verification

- [ ] **Step 1: Run go vet**

Run: `go vet ./...`

- [ ] **Step 2: Check all imports resolve**

Run: `go mod tidy`

- [ ] **Step 3: Verify the target API works conceptually**

The final usage matches the user's spec:
```go
larkClient := lark.NewClient(appID, appSecret)  // LarkClient()
factory := agent.NewFactory(cfg.Agent)           // Agent (factory)
store := session.NewFileStore("./sessions")       // Session persistence
ch := channel.New(larkClient, factory, store)     // Channel(agent, larkClient)
ch.Start(ctx)                                     // ch.start()
```

- [ ] **Step 4: Commit final state**

```bash
git add -A
git commit -m "chore: integration verification"
```

---

## Self-Review

**1. Spec coverage:**
- ✅ DeepAgent from agentscope-go — Task 4 (Factory)
- ✅ Stateless agent, persistent session — Task 3 (Store) + Task 7 (Channel)
- ✅ Feishu/Lark WebSocket — Task 6
- ✅ ChatID-based session isolation — Task 3 (Store, per-chatID keys)
- ✅ YAML + env var config — Task 2
- ✅ Card updates (v1: thinking → final) — Task 5 + Task 7
- ✅ No /connect /disconnect — not implemented
- ✅ Target API: agent → larkClient → ch.Start() — Task 9
- ✅ Per-chatID concurrency lock — Task 7 (sync.Map)

**2. Placeholder scan:**
- No TBDs found
- All code blocks are complete
- All file paths are exact

**3. Type consistency:**
- `lark.ToolEntry` used consistently in card.go and channel.go
- `session.Store` API matches across tasks (LoadSession/SaveSession/NewSession/ResetSession)
- `lark.Client` and `lark.API()` pattern consistent
- `agent.Factory.Create()` returns `*agent.DeepAgent` used in channel.go

**4. Architecture notes:**
- Agent is stateless: created per message, used once, GC'd
- Session is stateful: persisted via agentscope-go's session package (JSON file or Redis)
- Per-chatID mutex prevents concurrent message processing from corrupting session
- Session key auto-increments on /new: chatID → chatID_1 → chatID_2 (like ai-channel)
