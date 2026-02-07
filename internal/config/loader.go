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

	"gopkg.in/yaml.v3"
)

//go:embed config.example.yaml
var exampleConfigBytes []byte

var envVarPattern = regexp.MustCompile(`\$\{([^}]+)\}`)

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	expanded := expandEnvVars(string(data))

	cfg := DefaultConfig()
	if err := yaml.Unmarshal([]byte(expanded), cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	resolveRelativePaths(cfg, filepath.Dir(path))

	return cfg, nil
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
