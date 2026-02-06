package config

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

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
