package config

import (
	"fmt"
	"os"
	"strconv"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Lark   LarkConfig   `yaml:"lark"`
	Agent  AgentConfig  `yaml:"agent"`
	Session SessionConfig `yaml:"session"`
}

type LarkConfig struct {
	AppID         string `yaml:"app_id"`
	AppSecret     string `yaml:"app_secret"`
	PublicBaseURL string `yaml:"public_base_url"`
}

type AgentConfig struct {
	ModelName       string `yaml:"model_name"`
	APIKey          string `yaml:"api_key"`
	BaseURL         string `yaml:"base_url"`
	SystemPrompt    string `yaml:"system_prompt"`
	MaxIters        int    `yaml:"max_iters"`
	MaxContextTokens int   `yaml:"max_context_tokens"`
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
	return &cfg, nil
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
	if v := os.Getenv("LARK_APP_ID"); v != "" {
		c.Lark.AppID = v
	}
	if v := os.Getenv("LARK_APP_SECRET"); v != "" {
		c.Lark.AppSecret = v
	}
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
