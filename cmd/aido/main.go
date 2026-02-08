package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/lhdbsbz/aido/internal/agent"
	"github.com/lhdbsbz/aido/internal/bridge"
	"github.com/lhdbsbz/aido/internal/config"
	"github.com/lhdbsbz/aido/internal/gateway"
	"github.com/lhdbsbz/aido/internal/llm"
	"github.com/lhdbsbz/aido/internal/mcp"
	"github.com/lhdbsbz/aido/internal/session"
	"github.com/lhdbsbz/aido/internal/skills"
	"github.com/lhdbsbz/aido/internal/tool"
)

const version = "0.1.0"

func main() {
	if err := serve(); err != nil {
		slog.Error("fatal", "error", err)
		os.Exit(1)
	}
}

func serve() error {
	// Setup structured logging
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})))

	home := config.ResolveHome()
	slog.Info("Aido starting", "version", version, "home", home)

	for _, dir := range []string{
		config.Workspace(),
		config.SessionDir(),
		config.CronDir(),
		config.LogsDir(),
		config.TempDir(),
		config.StoreDir(),
		config.SkillsDir(),
	} {
		os.MkdirAll(dir, 0755)
	}

	// Load config; if missing, create from embedded example (token replaced) then load
	cfgPath := config.Path()
	cfg, err := config.Load(cfgPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			if writeErr := config.CreateFromExample(cfgPath); writeErr != nil {
				return fmt.Errorf("create config: %w", writeErr)
			}
			slog.Info("config created", "path", cfgPath)
			cfg, err = config.Load(cfgPath)
			if err != nil {
				return fmt.Errorf("load created config: %w", err)
			}
		} else {
			slog.Warn("config load failed, using example as template", "path", cfgPath, "error", err)
			cfg, err = config.LoadFromExample(filepath.Dir(cfgPath))
			if err != nil {
				return fmt.Errorf("load config and fallback example: %w", err)
			}
		}
	}
	config.Set(cfg)

	store := session.NewStore(config.SessionDir())
	if err := store.Load(); err != nil {
		slog.Warn("failed to load session store", "error", err)
	}

	registry := tool.NewRegistry()
	tool.RegisterFSTools(registry, config.Workspace())
	tool.RegisterExecTools(registry, config.Workspace())
	tool.RegisterWebTools(registry)
	tool.RegisterSessionTools(registry)
	tool.RegisterMemoryTools(registry, config.Workspace())
	tool.RegisterCronTools(registry, config.CronJobsPath())

	mcpClient := mcp.NewClient()
	reloadMCP(context.Background(), cfg, mcpClient, registry, config.ResolveHome())

	// Build global policy from gateway.toolsProfile
	profile := cfg.Gateway.ToolsProfile
	if profile == "" {
		profile = "coding"
	}
	globalPolicy := tool.PolicyLayer{Profile: profile}
	if agentCfg, ok := cfg.Agents["default"]; ok {
		globalPolicy.Allow = agentCfg.Tools.Allow
		globalPolicy.Deny = agentCfg.Tools.Deny
	}
	policy := tool.NewPolicy(globalPolicy)

	// Initialize agent loop
	loop := &agent.Loop{
		OpenAI:    llm.NewOpenAIClient(),
		Anthropic: llm.NewAnthropicClient(),
		Tools:     registry,
		Policy:    policy,
		Config:    cfg,
	}

	router := agent.NewRouter(loop, store)
	reloadSkills(cfg, router)

	config.RegisterOnReload(func(cfg *config.Config) {
		reloadMCP(context.Background(), cfg, mcpClient, registry, config.ResolveHome())
		reloadSkills(cfg, router)
		profile := cfg.Gateway.ToolsProfile
		if profile == "" {
			profile = "coding"
		}
		layer := tool.PolicyLayer{Profile: profile}
		if agentCfg, ok := cfg.Agents["default"]; ok {
			layer.Allow = agentCfg.Tools.Allow
			layer.Deny = agentCfg.Tools.Deny
		}
		loop.SetPolicy(tool.NewPolicy(layer))
	})

	// Start gateway with graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigCh
		slog.Info("shutdown signal received", "signal", sig)
		cancel()
	}()

	port := cfg.Gateway.Port
	if port <= 0 {
		port = 19800
	}
	aidoWSURL := fmt.Sprintf("ws://127.0.0.1:%d/ws", port)
	bridgeMgr := bridge.NewManager(aidoWSURL, cfg.Gateway.Auth.Token)
	defer bridgeMgr.StopAll()

	config.RegisterOnReload(func(cfg *config.Config) {
		reloadBridges(ctx, cfg, bridgeMgr)
	})

	bridgeCount := 0
	for _, inst := range cfg.Bridges.Instances {
		if !inst.Enabled || inst.Path == "" {
			slog.Debug("bridge skipped", "id", inst.ID, "reason", "disabled or empty path")
			continue
		}
		bridgeDir := inst.Path
		if !filepath.IsAbs(bridgeDir) {
			bridgeDir = filepath.Join(home, bridgeDir)
		}
		slog.Info("bridge starting", "id", inst.ID, "dir", bridgeDir)
		if bridgeMgr.Start(ctx, bridgeDir, inst.ID, inst.Enabled, inst.Env) {
			bridgeCount++
		}
	}
	if len(cfg.Bridges.Instances) > 0 {
		slog.Info("bridges init", "total_in_config", len(cfg.Bridges.Instances), "started", bridgeCount)
	}

	srv := gateway.NewServer(router, bridgeMgr)
	return srv.Start(ctx)
}

func reloadMCP(ctx context.Context, cfg *config.Config, mcpClient *mcp.Client, registry *tool.Registry, home string) {
	for _, name := range mcpClient.ServerNames() {
		mcpClient.RemoveServer(name)
		registry.UnregisterByPrefix(name)
	}
	for _, srv := range cfg.Tools.MCP {
		name := srv.Name
		if name == "" {
			name = "mcp"
		}
		if srv.Transport == "http" || srv.URL != "" {
			sseURL := srv.URL
			if sseURL == "" {
				slog.Warn("MCP HTTP transport requires url", "name", name)
				continue
			}
			headers := make(map[string]string, len(srv.Env))
			for k, v := range srv.Env {
				headers[k] = v
			}
			transport := mcp.NewHTTPTransport(sseURL, headers)
			if err := mcpClient.AddServer(ctx, name, transport); err != nil {
				slog.Warn("failed to add MCP server (HTTP)", "name", name, "error", err)
				continue
			}
			slog.Info("MCP server added (HTTP)", "name", name)
			continue
		}
		envSlice := make([]string, 0, len(srv.Env))
		for k, v := range srv.Env {
			envSlice = append(envSlice, k+"="+v)
		}
		transport := mcp.NewStdioTransport(srv.Command, srv.Args, envSlice, home)
		if err := mcpClient.AddServer(ctx, name, transport); err != nil {
			slog.Warn("failed to add MCP server", "name", name, "error", err)
			continue
		}
		slog.Info("MCP server added", "name", name)
	}
	mcpClient.RegisterTools(registry)
}

func reloadBridges(ctx context.Context, cfg *config.Config, bridgeMgr *bridge.Manager) {
	port := cfg.Gateway.Port
	if port <= 0 {
		port = 19800
	}
	bridgeMgr.SetGateway(fmt.Sprintf("ws://127.0.0.1:%d/ws", port), cfg.Gateway.Auth.Token)

	enabledIDs := make(map[string]bool)
	for _, inst := range cfg.Bridges.Instances {
		if inst.Enabled && inst.Path != "" {
			enabledIDs[inst.ID] = true
		}
	}
	stopped := 0
	for _, id := range bridgeMgr.RunningIDs() {
		if !enabledIDs[id] {
			bridgeMgr.Stop(id)
			slog.Info("bridge stopped (config reload)", "id", id)
			stopped++
		}
	}
	started := 0
	home := config.ResolveHome()
	for _, inst := range cfg.Bridges.Instances {
		if !inst.Enabled || inst.Path == "" {
			continue
		}
		bridgeDir := inst.Path
		if !filepath.IsAbs(bridgeDir) {
			bridgeDir = filepath.Join(home, bridgeDir)
		}
		bridgeMgr.Stop(inst.ID)
		slog.Info("bridge starting (config reload)", "id", inst.ID, "dir", bridgeDir)
		if bridgeMgr.Start(ctx, bridgeDir, inst.ID, true, inst.Env) {
			started++
		}
	}
	if stopped > 0 || started > 0 {
		slog.Info("bridges reload done", "stopped", stopped, "started", started)
	}
}

func reloadSkills(cfg *config.Config, router *agent.Router) {
	skillDir := config.SkillsDir()
	for agentID := range cfg.Agents {
		loaded := skills.LoadFromDirs([]string{skillDir})
		router.SetSkills(agentID, loaded)
		if len(loaded) > 0 {
			slog.Info("skills loaded", "agent", agentID, "count", len(loaded))
		}
	}
}
