package bridge

import (
	"encoding/json"
	"os"
	"path/filepath"
)

const manifestName = "aido-bridge.json"

type Manifest struct {
	ID          string       `json:"id"`
	Name        string       `json:"name"`
	Description string       `json:"description"`
	Runtime     string     `json:"runtime"`
	Commands    [][]string `json:"commands"` // 按顺序执行：前 N-1 条执行完即结束，最后一条为常驻进程；单条时即只有一条
	Cwd         string     `json:"cwd"`
	EnvFile     string       `json:"envFile"`
	EnvSchema   []EnvSchema  `json:"envSchema"`
}

type EnvSchema struct {
	Key         string `json:"key"`
	Description string `json:"description"`
	Required    bool   `json:"required"`
}

func LoadManifest(bridgeDir string) (*Manifest, error) {
	p := filepath.Join(bridgeDir, manifestName)
	data, err := os.ReadFile(p)
	if err != nil {
		return nil, err
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	if m.Cwd == "" {
		m.Cwd = "."
	}
	return &m, nil
}
