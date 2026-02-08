package config

import (
	"crypto/rand"
	_ "embed"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"

	"gopkg.in/yaml.v3"
)

var current atomic.Pointer[Config]

var (
	onReloadMu      sync.Mutex
	onReloadCallbacks []func(*Config)
)

// Get returns the current in-memory config (hot-reloaded when the file changes).
func Get() *Config { return current.Load() }

// Set sets the current in-memory config. Used at startup and by the file watcher.
func Set(c *Config) {
	if c != nil {
		current.Store(c)
	}
}

// RegisterOnReload registers a callback that runs after config is hot-reloaded (e.g. for MCP, skills).
func RegisterOnReload(fn func(*Config)) {
	onReloadMu.Lock()
	defer onReloadMu.Unlock()
	onReloadCallbacks = append(onReloadCallbacks, fn)
}

func notifyReload(cfg *Config) {
	onReloadMu.Lock()
	cb := make([]func(*Config), len(onReloadCallbacks))
	copy(cb, onReloadCallbacks)
	onReloadMu.Unlock()
	for _, fn := range cb {
		fn(cfg)
	}
}

//go:embed config.example.yaml
var exampleConfigBytes []byte

var envVarPattern = regexp.MustCompile(`\$\{([^}]+)\}`)

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	expanded := expandEnvVars(string(data))

	var cfg Config
	if err := yaml.Unmarshal([]byte(expanded), &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	ensureNonNilMaps(&cfg)
	applyLoadDefaults(&cfg)
	resolveRelativePaths(&cfg, filepath.Dir(path))

	return &cfg, nil
}

func ensureNonNilMaps(cfg *Config) {
	if cfg.Agents == nil {
		cfg.Agents = make(map[string]AgentConfig)
	}
	if cfg.Providers == nil {
		cfg.Providers = make(map[string]ProviderConfig)
	}
	if cfg.Tools.MCP == nil {
		cfg.Tools.MCP = []MCPServerConfig{}
	}
	if cfg.Bridges.Instances == nil {
		cfg.Bridges.Instances = []BridgeInstanceConfig{}
	}
}

func applyLoadDefaults(cfg *Config) {
	if cfg.Gateway.Port <= 0 {
		cfg.Gateway.Port = 19800
	}
	if cfg.Gateway.ToolsProfile == "" {
		cfg.Gateway.ToolsProfile = "coding"
	}
}

// LoadFromExample unmarshals the embedded config.example.yaml as the default config.
// baseDir is used to resolve relative paths (e.g. workspace). Use config dir or ResolveHome().
func LoadFromExample(baseDir string) (*Config, error) {
	expanded := expandEnvVars(string(exampleConfigBytes))
	var cfg Config
	if err := yaml.Unmarshal([]byte(expanded), &cfg); err != nil {
		return nil, fmt.Errorf("parse example config: %w", err)
	}
	ensureNonNilMaps(&cfg)
	resolveRelativePaths(&cfg, baseDir)
	return &cfg, nil
}

func expandEnvVars(content string) string {
	return envVarPattern.ReplaceAllStringFunc(content, func(match string) string {
		varName := match[2 : len(match)-1]
		if val, ok := os.LookupEnv(varName); ok {
			return val
		}
		return match
	})
}

func resolveRelativePaths(cfg *Config, baseDir string) {
	for name, agent := range cfg.Agents {
		if agent.Workspace != "" && !filepath.IsAbs(agent.Workspace) {
			agent.Workspace = filepath.Join(baseDir, agent.Workspace)
		}
		for i, dir := range agent.Skills.Dirs {
			if !filepath.IsAbs(dir) {
				agent.Skills.Dirs[i] = filepath.Join(baseDir, dir)
			}
		}
		cfg.Agents[name] = agent
	}
}

// ResolveHome returns the AIDO_HOME directory.
// Priority: AIDO_HOME env > ~/.aido/
func ResolveHome() string {
	if home := os.Getenv("AIDO_HOME"); home != "" {
		return home
	}
	userHome, err := os.UserHomeDir()
	if err != nil {
		return ".aido"
	}
	return filepath.Join(userHome, ".aido")
}

// ResolveConfigPath finds the config file.
// Priority: --config flag > AIDO_HOME/config.yaml
func ResolveConfigPath(flagPath string) string {
	if flagPath != "" {
		return flagPath
	}
	return filepath.Join(ResolveHome(), "config.yaml")
}

// Path returns the process-wide config file path (ResolveConfigPath("")).
// All components should use this instead of receiving the path by parameter.
func Path() string {
	return ResolveConfigPath("")
}

// GenerateToken returns a random hex token (32 bytes = 64 chars) for gateway auth.
func GenerateToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "fallback-token-please-set-gateway-auth-token-in-config"
	}
	return hex.EncodeToString(b)
}

// CreateFromExample writes the embedded config.example.yaml to targetPath with token placeholder replaced by a generated token.
func CreateFromExample(targetPath string) error {
	dir := filepath.Dir(targetPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	token := GenerateToken()
	content := strings.ReplaceAll(string(exampleConfigBytes), "${AIDO_TOKEN}", token)
	if err := os.WriteFile(targetPath, []byte(content), 0600); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	return nil
}

// Write marshals cfg to YAML and writes it to path. Creates parent directory if needed.
func Write(path string, cfg *Config) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	return nil
}

// ResolveProvider parses "provider/model" format and returns provider config + model name.
func ResolveProvider(cfg *Config, modelRef string) (provider string, model string, provCfg ProviderConfig, err error) {
	parts := strings.SplitN(modelRef, "/", 2)
	if len(parts) != 2 {
		return "", "", ProviderConfig{}, fmt.Errorf("invalid model reference %q, expected 'provider/model'", modelRef)
	}
	provider = parts[0]
	model = parts[1]
	provCfg, ok := cfg.Providers[provider]
	if !ok {
		return "", "", ProviderConfig{}, fmt.Errorf("provider %q not configured", provider)
	}
	return provider, model, provCfg, nil
}

// ResolveProviderForAgent returns provider config and model for an agent (uses agent.Provider + agent.Model).
// If agent.Provider is set, model is agent.Model; otherwise Model is parsed as "provider/model" for backward compat.
func ResolveProviderForAgent(cfg *Config, agent *AgentConfig) (provider string, model string, provCfg ProviderConfig, err error) {
	if agent.Provider != "" {
		provCfg, ok := cfg.Providers[agent.Provider]
		if !ok {
			return "", "", ProviderConfig{}, fmt.Errorf("provider %q not configured", agent.Provider)
		}
		return agent.Provider, agent.Model, provCfg, nil
	}
	return ResolveProvider(cfg, agent.Model)
}

// ResolveProviderWithDefault resolves modelRef: if it contains "/" then "provider/model", else defaultProvider + "/" + modelRef.
func ResolveProviderWithDefault(cfg *Config, modelRef, defaultProvider string) (provider string, model string, provCfg ProviderConfig, err error) {
	if strings.Contains(modelRef, "/") {
		return ResolveProvider(cfg, modelRef)
	}
	if defaultProvider == "" {
		return "", "", ProviderConfig{}, fmt.Errorf("model ref %q has no provider and no default", modelRef)
	}
	provCfg, ok := cfg.Providers[defaultProvider]
	if !ok {
		return "", "", ProviderConfig{}, fmt.Errorf("provider %q not configured", defaultProvider)
	}
	return defaultProvider, modelRef, provCfg, nil
}
