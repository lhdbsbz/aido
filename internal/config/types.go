package config

type Config struct {
	Gateway   GatewayConfig             `yaml:"gateway" json:"gateway"`
	Agents    map[string]AgentConfig    `yaml:"agents" json:"agents"`
	Providers map[string]ProviderConfig `yaml:"providers" json:"providers"`
	Tools     ToolsConfig               `yaml:"tools" json:"tools"`
}

type GatewayConfig struct {
	Port         int        `yaml:"port" json:"port"`
	Auth         AuthConfig `yaml:"auth" json:"auth"`
	CurrentAgent string     `yaml:"currentAgent" json:"currentAgent"`   // 固定使用的 agent，空则可由请求指定
	ToolsProfile string     `yaml:"toolsProfile" json:"toolsProfile"`   // 全局工具集档位：minimal | coding | messaging | full
}

type AuthConfig struct {
	Token string `yaml:"token" json:"token"`
}

type AgentConfig struct {
	Provider   string           `yaml:"provider" json:"provider"`     // 绑定的 provider（providers 的 key）
	Model      string           `yaml:"model" json:"model"`          // 模型 id（如 claude-sonnet-4-20250514）
	Fallbacks  []string         `yaml:"fallbacks" json:"fallbacks"`  // 备选，可为 "modelId" 或 "provider/modelId"
	Tools      AgentToolsConfig `yaml:"tools" json:"tools"`
	Compaction CompactionConfig `yaml:"compaction" json:"compaction"`
	Workspace  string           `yaml:"workspace" json:"workspace"`
	Skills     SkillsConfig     `yaml:"skills" json:"skills"`
}

type AgentToolsConfig struct {
	Allow []string `yaml:"allow" json:"allow"`
	Deny  []string `yaml:"deny" json:"deny"`
}

type CompactionConfig struct {
	KeepRecentTokens int     `yaml:"keepRecentTokens" json:"keepRecentTokens"`
	ReserveTokens    int     `yaml:"reserveTokens" json:"reserveTokens"`
	ChunkRatio       float64 `yaml:"chunkRatio" json:"chunkRatio"`
}

type SkillsConfig struct {
	Dirs []string `yaml:"dirs" json:"dirs"`
}

type ProviderConfig struct {
	APIKey  string `yaml:"apiKey" json:"apiKey"`
	BaseURL string `yaml:"baseURL" json:"baseURL"`
	Type    string `yaml:"type" json:"type"` // "openai" | "anthropic" (default: inferred from provider name)
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

type ToolsConfig struct {
	MCP []MCPServerConfig `yaml:"mcp" json:"mcp"`
}

type MCPServerConfig struct {
	Name      string            `yaml:"name" json:"name"`
	Command   string            `yaml:"command" json:"command"`
	Args      []string          `yaml:"args" json:"args"`
	URL       string            `yaml:"url" json:"url"`
	Transport string            `yaml:"transport" json:"transport"`
	Env       map[string]string `yaml:"env" json:"env"`
}

func DefaultConfig() *Config {
	return &Config{
		Gateway: GatewayConfig{
			Port:         19800,
			CurrentAgent: "default",
			ToolsProfile: "coding",
		},
		Providers: map[string]ProviderConfig{
			"anthropic": { Type: "anthropic" },
			"openai":    { Type: "openai" },
			"deepseek":  { BaseURL: "https://api.deepseek.com", Type: "openai" },
			"minimax":   { BaseURL: "https://api.minimaxi.com/anthropic", Type: "anthropic" }, // 国内默认；海外用 https://api.minimax.io/anthropic
		},
		Agents: map[string]AgentConfig{
			"default": {
				Provider: "anthropic",
				Model:    "claude-sonnet-4-20250514",
				Tools:    AgentToolsConfig{},
				Compaction: CompactionConfig{
					KeepRecentTokens: 20000,
					ReserveTokens:    16384,
					ChunkRatio:       0.4,
				},
			},
			"openai": {
				Provider: "openai",
				Model:    "gpt-4o",
				Tools:    AgentToolsConfig{},
				Compaction: CompactionConfig{
					KeepRecentTokens: 20000,
					ReserveTokens:    16384,
					ChunkRatio:       0.4,
				},
			},
		},
	}
}
