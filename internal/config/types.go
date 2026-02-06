package config

import "time"

type Config struct {
	Gateway   GatewayConfig            `yaml:"gateway"`
	Agents    map[string]AgentConfig   `yaml:"agents"`
	Providers map[string]ProviderConfig `yaml:"providers"`
	Message   MessageConfig            `yaml:"message"`
	Session   SessionConfig            `yaml:"session"`
	Cron      CronConfig               `yaml:"cron"`
	Memory    MemoryConfig             `yaml:"memory"`
	Tools     ToolsConfig              `yaml:"tools"`
}

type GatewayConfig struct {
	Port int        `yaml:"port"`
	Auth AuthConfig `yaml:"auth"`
}

type AuthConfig struct {
	Token    string `yaml:"token"`
	Password string `yaml:"password"`
}

type AgentConfig struct {
	Model      string           `yaml:"model"`
	Fallbacks  []string         `yaml:"fallbacks"`
	Thinking   string           `yaml:"thinking"`
	Tools      AgentToolsConfig `yaml:"tools"`
	Compaction CompactionConfig `yaml:"compaction"`
	Workspace  string           `yaml:"workspace"`
	Skills     SkillsConfig     `yaml:"skills"`
}

type AgentToolsConfig struct {
	Profile string   `yaml:"profile"`
	Allow   []string `yaml:"allow"`
	Deny    []string `yaml:"deny"`
}

type CompactionConfig struct {
	KeepRecentTokens int     `yaml:"keepRecentTokens"`
	ReserveTokens    int     `yaml:"reserveTokens"`
	ChunkRatio       float64 `yaml:"chunkRatio"`
}

type SkillsConfig struct {
	Dirs []string `yaml:"dirs"`
}

type ProviderConfig struct {
	APIKey  string `yaml:"apiKey"`
	BaseURL string `yaml:"baseURL"`
	Type    string `yaml:"type"` // "openai" | "anthropic" (default: inferred from provider name)
}

// ClientType returns which LLM client to use for this provider.
func (p ProviderConfig) ClientType(providerName string) string {
	if p.Type != "" {
		return p.Type
	}
	if providerName == "anthropic" {
		return "anthropic"
	}
	return "openai"
}

type MessageConfig struct {
	Dedup    DedupConfig    `yaml:"dedup"`
	Debounce DebounceConfig `yaml:"debounce"`
	Queue    QueueConfig    `yaml:"queue"`
}

type DedupConfig struct {
	TTL time.Duration `yaml:"ttl"`
}

type DebounceConfig struct {
	Default time.Duration `yaml:"default"`
}

type QueueConfig struct {
	Mode string `yaml:"mode"` // "collect" | "followup" | "steer"
}

type SessionConfig struct {
	DailyReset string        `yaml:"dailyReset"`
	IdleExpiry time.Duration `yaml:"idleExpiry"`
}

type CronConfig struct {
	Jobs []CronJobConfig `yaml:"jobs"`
}

type CronJobConfig struct {
	Name     string `yaml:"name"`
	Schedule string `yaml:"schedule"`
	Agent    string `yaml:"agent"`
	Message  string `yaml:"message"`
}

type MemoryConfig struct {
	Enabled  bool   `yaml:"enabled"`
	Provider string `yaml:"provider"`
}

type ToolsConfig struct {
	MCP []MCPServerConfig `yaml:"mcp"`
}

type MCPServerConfig struct {
	Name      string            `yaml:"name"`
	Command   string            `yaml:"command"`
	Args      []string          `yaml:"args"`
	URL       string            `yaml:"url"`
	Transport string            `yaml:"transport"` // "stdio" | "http"
	Env       map[string]string `yaml:"env"`
}

func DefaultConfig() *Config {
	return &Config{
		Gateway: GatewayConfig{
			Port: 18789,
		},
		Agents: map[string]AgentConfig{
			"default": {
				Model:    "anthropic/claude-sonnet-4-20250514",
				Thinking: "medium",
				Tools: AgentToolsConfig{
					Profile: "coding",
				},
				Compaction: CompactionConfig{
					KeepRecentTokens: 20000,
					ReserveTokens:    16384,
					ChunkRatio:       0.4,
				},
			},
		},
		Message: MessageConfig{
			Dedup:    DedupConfig{TTL: 30 * time.Second},
			Debounce: DebounceConfig{Default: 1500 * time.Millisecond},
			Queue:    QueueConfig{Mode: "followup"},
		},
		Session: SessionConfig{
			DailyReset: "04:00",
			IdleExpiry: 4 * time.Hour,
		},
	}
}
