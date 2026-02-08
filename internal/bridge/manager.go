package bridge

import (
	"bufio"
	"bytes"
	"context"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type InstanceStatus struct {
	ID       string    `json:"id"`
	Name     string    `json:"name"`
	Enabled  bool      `json:"enabled"`
	Path     string    `json:"path"`
	Running  bool      `json:"running"`
	PID      int       `json:"pid,omitempty"`
	StartedAt time.Time `json:"startedAt,omitempty"`
}

type Manager struct {
	aidoWSURL string
	aidoToken string
	mu        sync.Mutex
	procs     map[string]*procEntry
}

type procEntry struct {
	cmd       *exec.Cmd
	cancel    context.CancelFunc
	startedAt time.Time
}

func NewManager(aidoWSURL, aidoToken string) *Manager {
	return &Manager{
		aidoWSURL: aidoWSURL,
		aidoToken: aidoToken,
		procs:     make(map[string]*procEntry),
	}
}

func (m *Manager) SetGateway(aidoWSURL, aidoToken string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.aidoWSURL = aidoWSURL
	m.aidoToken = aidoToken
}

func (m *Manager) RunningIDs() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	ids := make([]string, 0, len(m.procs))
	for id := range m.procs {
		ids = append(ids, id)
	}
	return ids
}

func (m *Manager) Start(ctx context.Context, bridgeDir, id string, enabled bool, env map[string]string) bool {
	if !enabled {
		return false
	}
	manifest, err := LoadManifest(bridgeDir)
	if err != nil {
		slog.Warn("bridge manifest load failed", "id", id, "dir", bridgeDir, "error", err)
		return false
	}
	if manifest.ID != id {
		slog.Warn("bridge not started: config id must equal manifest id", "config_id", id, "manifest_id", manifest.ID, "hint", "set bridges.instances[].id to \""+manifest.ID+"\" in config")
		return false
	}

	workDir := filepath.Join(bridgeDir, manifest.Cwd)
	envSlice := m.buildEnv(manifest, workDir, env)

	if len(manifest.Commands) == 0 {
		slog.Warn("bridge has no commands", "id", id)
		return false
	}
	setupArgv := manifest.Commands[:len(manifest.Commands)-1]
	runArgv := manifest.Commands[len(manifest.Commands)-1]
	if len(runArgv) == 0 {
		slog.Warn("bridge last command is empty", "id", id)
		return false
	}

	for i, argv := range setupArgv {
		if len(argv) == 0 {
			continue
		}
		setupCtx, cancelSetup := context.WithTimeout(ctx, 5*time.Minute)
		setupCmd, err := m.buildCmdFromArgv(setupCtx, manifest, workDir, envSlice, argv)
		if err != nil {
			slog.Warn("bridge setup command build failed", "id", id, "step", i+1, "error", err)
			cancelSetup()
			return false
		}
		setupCmd.Stdout = os.Stdout
		setupCmd.Stderr = os.Stderr
		if err := setupCmd.Run(); err != nil {
			slog.Warn("bridge setup command failed", "id", id, "step", i+1, "argv", argv, "error", err)
			cancelSetup()
			return false
		}
		cancelSetup()
		slog.Info("bridge setup done", "id", id, "step", i+1, "argv", argv)
	}

	procCtx, cancel := context.WithCancel(ctx)
	cmd, err := m.buildCmdFromArgv(procCtx, manifest, workDir, envSlice, runArgv)
	if err != nil {
		slog.Warn("bridge command build failed", "id", id, "error", err)
		cancel()
		return false
	}

	m.mu.Lock()
	if e, ok := m.procs[id]; ok {
		if e.cancel != nil {
			e.cancel()
		}
		delete(m.procs, id)
	}
	m.mu.Unlock()

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		slog.Warn("bridge start failed", "id", id, "error", err)
		cancel()
		return false
	}

	m.mu.Lock()
	m.procs[id] = &procEntry{cmd: cmd, cancel: cancel, startedAt: time.Now()}
	m.mu.Unlock()
	slog.Info("bridge started", "id", id, "pid", cmd.Process.Pid)

	go func() {
		_ = cmd.Wait()
		m.mu.Lock()
		if e, ok := m.procs[id]; ok && e.cmd == cmd {
			delete(m.procs, id)
		}
		m.mu.Unlock()
		slog.Info("bridge exited", "id", id)
	}()
	return true
}

func (m *Manager) Stop(id string) {
	m.mu.Lock()
	e, ok := m.procs[id]
	m.mu.Unlock()
	if !ok {
		return
	}
	if e.cancel != nil {
		e.cancel()
	}
	m.mu.Lock()
	delete(m.procs, id)
	m.mu.Unlock()
}

func (m *Manager) StopAll() {
	m.mu.Lock()
	ids := make([]string, 0, len(m.procs))
	for id := range m.procs {
		ids = append(ids, id)
	}
	m.mu.Unlock()
	for _, id := range ids {
		m.Stop(id)
	}
}

type InstanceConfig struct {
	ID      string
	Enabled bool
	Path    string
}

func (m *Manager) List(cfgInstances []InstanceConfig) []InstanceStatus {
	m.mu.Lock()
	procs := make(map[string]*procEntry)
	for k, v := range m.procs {
		procs[k] = v
	}
	m.mu.Unlock()

	out := make([]InstanceStatus, 0, len(cfgInstances))
	for _, inst := range cfgInstances {
		s := InstanceStatus{
			ID:      inst.ID,
			Enabled: inst.Enabled,
			Path:    inst.Path,
		}
		if e, ok := procs[inst.ID]; ok && e.cmd != nil && e.cmd.Process != nil {
			s.Running = true
			s.PID = e.cmd.Process.Pid
			s.StartedAt = e.startedAt
		}
		if manifest, err := LoadManifest(inst.Path); err == nil {
			s.Name = manifest.Name
		} else {
			s.Name = inst.ID
		}
		out = append(out, s)
	}
	return out
}

func (m *Manager) buildEnv(manifest *Manifest, workDir string, extraEnv map[string]string) []string {
	env := os.Environ()
	env = appendEnv(env, "AIDO_WS_URL", m.aidoWSURL)
	env = appendEnv(env, "AIDO_TOKEN", m.aidoToken)
	for k, v := range extraEnv {
		env = appendEnv(env, k, v)
	}
	if manifest.EnvFile != "" {
		envPath := filepath.Join(workDir, manifest.EnvFile)
		if data, err := os.ReadFile(envPath); err == nil {
			env = parseEnvFile(data, env)
		}
	}
	return env
}

func (m *Manager) buildCmdFromArgv(ctx context.Context, manifest *Manifest, workDir string, env []string, argv []string) (*exec.Cmd, error) {
	if len(argv) == 0 {
		return nil, nil
	}
	var name string
	var args []string
	switch manifest.Runtime {
	case "node", "npx":
		name = argv[0]
		args = argv[1:]
	case "python":
		name = "python"
		args = argv
	case "exec":
		name = argv[0]
		args = argv[1:]
	default:
		name = argv[0]
		args = argv[1:]
	}
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = workDir
	cmd.Env = env
	return cmd, nil
}

func appendEnv(env []string, key, value string) []string {
	prefix := key + "="
	for i, e := range env {
		if len(e) > len(prefix) && e[:len(prefix)] == prefix {
			env[i] = prefix + value
			return env
		}
	}
	return append(env, prefix+value)
}

func parseEnvFile(data []byte, base []string) []string {
	env := make([]string, len(base))
	copy(env, base)
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if i := strings.Index(line, "="); i > 0 {
			key := strings.TrimSpace(line[:i])
			val := strings.TrimSpace(line[i+1:])
			if key != "" {
				env = appendEnv(env, key, val)
			}
		}
	}
	return env
}