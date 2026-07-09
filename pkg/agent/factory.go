package agent

import (
	"github.com/vearne/agent-gateway/pkg/adapter"
	"github.com/vearne/agent-gateway/pkg/config"
	agentscope "github.com/vearne/agentscope-go/pkg/agent"
	"github.com/vearne/agentscope-go/pkg/formatter"
	"github.com/vearne/agentscope-go/pkg/memory"
	"github.com/vearne/agentscope-go/pkg/model"
	"github.com/vearne/agentscope-go/pkg/tool"
)

// FactoryOption configures a Factory or ToolKitFactory.
type FactoryOption func(*Factory)

// WithToolApproval injects a tool approval callback into the factory.
func WithToolApproval(fn agentscope.ToolApprovalFunc) FactoryOption {
	return func(f *Factory) { f.toolApproval = fn }
}

type Factory struct {
	cfg          config.AgentConfig
	toolApproval agentscope.ToolApprovalFunc
}

func NewFactory(cfg config.AgentConfig, opts ...FactoryOption) *Factory {
	f := &Factory{cfg: cfg}
	for _, opt := range opts {
		opt(f)
	}
	return f
}

func (f *Factory) Create() adapter.Agent {
	m := model.NewOpenAIChatModel(f.cfg.ModelName, f.cfg.APIKey, f.cfg.BaseURL, true)
	fmt := formatter.NewOpenAIChatFormatter()
	mem := memory.NewInMemoryMemory()

	deepOpts := []agentscope.DeepOption{
		agentscope.WithDeepName("agent-gateway"),
		agentscope.WithDeepModel(m),
		agentscope.WithDeepFormatter(fmt),
		agentscope.WithDeepMemory(mem),
		agentscope.WithDeepSystemPrompt(f.cfg.SystemPrompt),
		agentscope.WithDeepMaxIters(f.cfg.MaxIters),
		agentscope.WithDeepMaxContextTokens(f.cfg.MaxContextTokens),
	}
	if f.toolApproval != nil {
		deepOpts = append(deepOpts, agentscope.WithDeepToolApproval(f.toolApproval))
	}
	return agentscope.NewDeepAgent(deepOpts...)
}

type ToolKitFactory struct {
	cfg          config.AgentConfig
	toolkit      *tool.Toolkit
	toolApproval agentscope.ToolApprovalFunc
}

func NewToolKitFactory(cfg config.AgentConfig, toolkit *tool.Toolkit, opts ...FactoryOption) *ToolKitFactory {
	f := &Factory{cfg: cfg}
	for _, opt := range opts {
		opt(f)
	}
	tf := &ToolKitFactory{
		cfg:          f.cfg,
		toolkit:      toolkit,
		toolApproval: f.toolApproval,
	}
	return tf
}

func (f *ToolKitFactory) Create() adapter.Agent {
	m := model.NewOpenAIChatModel(f.cfg.ModelName, f.cfg.APIKey, f.cfg.BaseURL, true)
	fmt := formatter.NewOpenAIChatFormatter()
	mem := memory.NewInMemoryMemory()

	deepOpts := []agentscope.DeepOption{
		agentscope.WithDeepName("agent-gateway"),
		agentscope.WithDeepModel(m),
		agentscope.WithDeepFormatter(fmt),
		agentscope.WithDeepMemory(mem),
		agentscope.WithDeepSystemPrompt(f.cfg.SystemPrompt),
		agentscope.WithDeepMaxIters(f.cfg.MaxIters),
		agentscope.WithDeepMaxContextTokens(f.cfg.MaxContextTokens),
		agentscope.WithDeepToolkit(f.toolkit),
	}
	if f.toolApproval != nil {
		deepOpts = append(deepOpts, agentscope.WithDeepToolApproval(f.toolApproval))
	}
	return agentscope.NewDeepAgent(deepOpts...)
}
