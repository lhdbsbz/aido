package config

type Config struct {
	Gateway   GatewayConfig             `yaml:"gateway" json:"gateway"`
	Agents    map[string]AgentConfig    `yaml:"agents" json:"agents"`
	Providers map[string]ProviderConfig `yaml:"providers" json:"providers"`
	Tools     ToolsConfig               `yaml:"tools" json:"tools"`
	Bridges   BridgesConfig             `yaml:"bridges" json:"bridges"`
}

type BridgesConfig struct {
	Instances []BridgeInstanceConfig `yaml:"instances" json:"instances"`
}

type BridgeInstanceConfig struct {
	ID      string            `yaml:"id" json:"id"`
	Enabled bool              `yaml:"enabled" json:"enabled"`
	Path    string            `yaml:"path" json:"path"`
	Env     map[string]string `yaml:"env" json:"env"`
}

type GatewayConfig struct {
	Port         int        `yaml:"port" json:"port"`
	Auth         AuthConfig `yaml:"auth" json:"auth"`
	CurrentAgent string     `yaml:"currentAgent" json:"currentAgent"`   // 固定使用的 agent，空则可由请求指定
	ToolsProfile string     `yaml:"toolsProfile" json:"toolsProfile"`   // 全局工具集档位：minimal | coding | messaging | full
	Locale       string     `yaml:"locale" json:"locale"`               // 系统提示词语言：en（英语）| zh（中文），默认 zh
}

type AuthConfig struct {
	Token string `yaml:"token" json:"token"`
}

type AgentConfig struct {
	Provider   string           `yaml:"provider" json:"provider"`   // 绑定的 provider（providers 的 key）
	Model      string           `yaml:"model" json:"model"`        // 模型 id（如 claude-sonnet-4-20250514）
	Tools      AgentToolsConfig `yaml:"tools" json:"tools"`
	Compaction CompactionConfig `yaml:"compaction" json:"compaction"`
}

type AgentToolsConfig struct {
	Allow []string `yaml:"allow" json:"allow"`
	Deny  []string `yaml:"deny" json:"deny"`
}

type CompactionConfig struct {
	ContextWindow    int     `yaml:"contextWindow" json:"contextWindow"`       // 模型上下文上限 token 数，0 表示用默认 200000
	KeepRecentTokens int     `yaml:"keepRecentTokens" json:"keepRecentTokens"`
	ReserveTokens    int     `yaml:"reserveTokens" json:"reserveTokens"`
	ChunkRatio       float64 `yaml:"chunkRatio" json:"chunkRatio"`
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
