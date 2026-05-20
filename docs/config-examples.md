# Configuration Examples

## Minimal (single Lark bot)

```yaml
agent:
  model_name: "gpt-4o"
  api_key: "your_api_key"
  system_prompt: "You are a helpful assistant."
  max_iters: 20

session:
  backend: "file"
  dir: "./sessions"

channels:
  - name: "lark-bot"
    platform: "lark"
    lark:
      app_id: "cli_xxx"
      app_secret: "your_app_secret"
```

## Multiple Lark bots with different personas

```yaml
agent:
  model_name: "gpt-4o"
  api_key: "your_api_key"
  system_prompt: "You are a helpful assistant."
  max_iters: 20

session:
  backend: "redis"
  addr: "localhost:6379"

channels:
  - name: "lark-support"
    platform: "lark"
    lark:
      app_id: "cli_support_xxx"
      app_secret: "support_secret"
    agent:
      system_prompt: "你是技术支持助手，擅长排查问题并提供解决方案。"

  - name: "lark-sales"
    platform: "lark"
    lark:
      app_id: "cli_sales_xxx"
      app_secret: "sales_secret"
    agent:
      system_prompt: "你是一位专业的销售顾问，善于了解客户需求。"
    session:
      backend: "redis"
      addr: "localhost:6379"
      db: 1

  - name: "lark-hr"
    platform: "lark"
    lark:
      app_id: "cli_hr_xxx"
      app_secret: "hr_secret"
    agent:
      system_prompt: "你是HR助手，帮助员工解答人事相关问题。"
      model_name: "gpt-4o-mini"
```

## Per-channel model and session override

```yaml
agent:
  model_name: "gpt-4o"
  api_key: "your_api_key"
  system_prompt: "You are a helpful assistant."
  max_iters: 20

session:
  backend: "file"
  dir: "./sessions"

channels:
  - name: "lark-main"
    platform: "lark"
    lark:
      app_id: "cli_main_xxx"
      app_secret: "main_secret"
    # Uses global agent & session defaults

  - name: "lark-cheap"
    platform: "lark"
    lark:
      app_id: "cli_cheap_xxx"
      app_secret: "cheap_secret"
    agent:
      model_name: "gpt-4o-mini"
      api_key: "another_api_key"
      max_iters: 10
    session:
      backend: "file"
      dir: "./sessions-cheap"
```

## Full reference with all options

```yaml
# Global agent config — serves as default for all channels
agent:
  model_name: "gpt-4o"               # Required. LLM model name.
  api_key: "your_api_key"            # Required. Or set AGENT_API_KEY env var.
  base_url: ""                        # Optional. Custom API endpoint.
  system_prompt: "You are a helpful assistant."  # Required.
  max_iters: 20                       # Optional. Max tool-use iterations. Default: 20.
  # max_context_tokens: 128000        # Optional. Context window. Default: 128000.

# Global session config — serves as default for all channels
session:
  # backend: "file" or "redis"
  backend: "file"
  # File backend options:
  dir: "./sessions"
  # Redis backend options:
  # addr: "localhost:6379"
  # password: ""
  # db: 0

# Channels — each entry is one bot instance
# Supported platforms: lark, telegram, discord, slack, whatsapp
channels:
  - name: "lark-bot"                  # Required. Human-readable channel name.
    platform: "lark"                  # Required. Platform identifier.
    lark:                             # Required for platform: "lark"
      app_id: "cli_xxx"              # Lark App ID.
      app_secret: "your_secret"      # Lark App Secret.
      # public_base_url: ""           # Optional. For generating download links.

    # Optional per-channel agent overrides (non-zero fields replace global)
    # agent:
    #   model_name: "gpt-4o-mini"
    #   api_key: "channel_specific_key"
    #   base_url: "https://api.custom.com/v1"
    #   system_prompt: "你是专属客服助手。"
    #   max_iters: 10
    #   max_context_tokens: 64000

    # Optional per-channel session overrides (non-empty fields replace global)
    # session:
    #   backend: "redis"
    #   dir: "./sessions-custom"
    #   addr: "redis.example.com:6379"
    #   password: "redis_password"
    #   db: 1

  # --- Telegram ---
  # - name: "telegram-bot"
  #   platform: "telegram"
  #   telegram:
  #     token: "123456:ABC-DEF..."      # BotFather token

  # --- Discord ---
  # - name: "discord-bot"
  #   platform: "discord"
  #   discord:
  #     token: "your_discord_bot_token"
  #     application_id: "your_app_id"

  # --- Slack ---
  # - name: "slack-bot"
  #   platform: "slack"
  #   slack:
  #     bot_token: "xoxb-..."           # Bot User OAuth Token
  #     app_token: "xapp-..."           # App-Level Token (for Socket Mode)

  # --- WhatsApp (via whatsmeow, requires QR scan on first run) ---
  # - name: "whatsapp-bot"
  #   platform: "whatsapp"
  #   whatsapp:
  #     data_dir: "./whatsapp-data"     # Session storage directory

# Environment variable overrides (apply to global config only):
# AGENT_API_KEY       → agent.api_key
# AGENT_MODEL_NAME    → agent.model_name
# AGENT_BASE_URL      → agent.base_url
# AGENT_MAX_ITERS     → agent.max_iters
```
