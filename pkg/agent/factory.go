package agent

import (
	"github.com/vearne/agent-gateway/pkg/adapter"

	"github.com/vearne/agent-gateway/pkg/config"

	agentscope "github.com/vearne/agentscope-go/pkg/agent"
	"github.com/vearne/agentscope-go/pkg/formatter"
	"github.com/vearne/agentscope-go/pkg/memory"
	"github.com/vearne/agentscope-go/pkg/model"
)

type Factory struct {
	cfg config.AgentConfig
}

func NewFactory(cfg config.AgentConfig) *Factory {
	return &Factory{cfg: cfg}
}

func (f *Factory) Create() adapter.Agent {
	m := model.NewOpenAIChatModel(f.cfg.ModelName, f.cfg.APIKey, f.cfg.BaseURL, true)
	fmt := formatter.NewOpenAIChatFormatter()
	mem := memory.NewInMemoryMemory()

	return agentscope.NewDeepAgent(
		agentscope.WithDeepName("agent-gateway"),
		agentscope.WithDeepModel(m),
		agentscope.WithDeepFormatter(fmt),
		agentscope.WithDeepMemory(mem),
		agentscope.WithDeepSystemPrompt(f.cfg.SystemPrompt),
		agentscope.WithDeepMaxIters(f.cfg.MaxIters),
		agentscope.WithDeepMaxContextTokens(f.cfg.MaxContextTokens),
	)
}
