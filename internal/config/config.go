package config

import (
	"fmt"
	"os"
	"strconv"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Agent    AgentConfig     `yaml:"agent"`
	Channels []ChannelConfig `yaml:"channels"`
	Session  SessionConfig   `yaml:"session"`
}

type ChannelConfig struct {
	Name     string         `yaml:"name"`
	Platform string         `yaml:"platform"`
	Lark     *LarkConfig    `yaml:"lark,omitempty"`

	Agent   *AgentConfig   `yaml:"agent,omitempty"`
	Session *SessionConfig `yaml:"session,omitempty"`
}

type LarkConfig struct {
	AppID         string `yaml:"app_id"`
	AppSecret     string `yaml:"app_secret"`
	PublicBaseURL string `yaml:"public_base_url"`
}

type AgentConfig struct {
	ModelName        string `yaml:"model_name"`
	APIKey           string `yaml:"api_key"`
	BaseURL          string `yaml:"base_url"`
	SystemPrompt     string `yaml:"system_prompt"`
	MaxIters         int    `yaml:"max_iters"`
	MaxContextTokens int    `yaml:"max_context_tokens"`
}

type SessionConfig struct {
	Backend  string `yaml:"backend"`
	Dir      string `yaml:"dir"`
	Addr     string `yaml:"addr"`
	Password string `yaml:"password"`
	DB       int    `yaml:"db"`
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	cfg.applyDefaults()
	cfg.applyEnvOverrides()
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func (c *Config) Validate() error {
	if len(c.Channels) == 0 {
		return fmt.Errorf("config: at least one channel required")
	}
	for i, ch := range c.Channels {
		if ch.Platform == "" {
			return fmt.Errorf("config: channel[%d] missing platform", i)
		}
		switch ch.Platform {
		case "lark":
			if ch.Lark == nil {
				return fmt.Errorf("config: channel[%d] (%s) missing lark config", i, ch.Name)
			}
		default:
			return fmt.Errorf("config: channel[%d] (%s) unsupported platform: %s", i, ch.Name, ch.Platform)
		}
	}
	return nil
}

// EffectiveAgent returns the agent config for this channel,
// merging global defaults with per-channel overrides.
func (ch *ChannelConfig) EffectiveAgent(global AgentConfig) AgentConfig {
	if ch.Agent == nil {
		return global
	}
	merged := global
	if ch.Agent.ModelName != "" {
		merged.ModelName = ch.Agent.ModelName
	}
	if ch.Agent.APIKey != "" {
		merged.APIKey = ch.Agent.APIKey
	}
	if ch.Agent.BaseURL != "" {
		merged.BaseURL = ch.Agent.BaseURL
	}
	if ch.Agent.SystemPrompt != "" {
		merged.SystemPrompt = ch.Agent.SystemPrompt
	}
	if ch.Agent.MaxIters != 0 {
		merged.MaxIters = ch.Agent.MaxIters
	}
	if ch.Agent.MaxContextTokens != 0 {
		merged.MaxContextTokens = ch.Agent.MaxContextTokens
	}
	return merged
}

// EffectiveSession returns the session config for this channel,
// merging global defaults with per-channel overrides.
func (ch *ChannelConfig) EffectiveSession(global SessionConfig) SessionConfig {
	if ch.Session == nil {
		return global
	}
	merged := global
	if ch.Session.Backend != "" {
		merged.Backend = ch.Session.Backend
	}
	if ch.Session.Dir != "" {
		merged.Dir = ch.Session.Dir
	}
	if ch.Session.Addr != "" {
		merged.Addr = ch.Session.Addr
	}
	if ch.Session.Password != "" {
		merged.Password = ch.Session.Password
	}
	// DB = 0 is a valid default; always override if session block exists.
	merged.DB = ch.Session.DB
	return merged
}

func (c *Config) applyDefaults() {
	if c.Agent.MaxIters == 0 {
		c.Agent.MaxIters = 20
	}
	if c.Agent.MaxContextTokens == 0 {
		c.Agent.MaxContextTokens = 128000
	}
}

func (c *Config) applyEnvOverrides() {
	if v := os.Getenv("AGENT_API_KEY"); v != "" {
		c.Agent.APIKey = v
	}
	if v := os.Getenv("AGENT_MODEL_NAME"); v != "" {
		c.Agent.ModelName = v
	}
	if v := os.Getenv("AGENT_BASE_URL"); v != "" {
		c.Agent.BaseURL = v
	}
	if v := os.Getenv("AGENT_MAX_ITERS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			c.Agent.MaxIters = n
		}
	}
}
