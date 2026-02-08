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
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(0)
	}

	switch os.Args[1] {
	case "version":
		fmt.Printf("aido v%s\n", version)
	case "serve":
		if err := serve(); err != nil {
			slog.Error("fatal", "error", err)
			os.Exit(1)
		}
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", os.Args[1])
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println("Aido - AI Agent Gateway")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  aido serve     Start the gateway server")
	fmt.Println("  aido version   Show version info")
}

func serve() error {
	// Setup structured logging
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})))

	// Resolve AIDO_HOME
	home := config.ResolveHome()
	slog.Info("Aido starting", "version", version, "home", home)

	// Ensure directories
	for _, dir := range []string{
		filepath.Join(home, "data", "sessions"),
		filepath.Join(home, "logs"),
		filepath.Join(home, "workspace", "skills"),
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

	// Initialize session store
	sessionDir := filepath.Join(home, "data", "sessions")
	store := session.NewStore(sessionDir)
	if err := store.Load(); err != nil {
		slog.Warn("failed to load session store", "error", err)
	}

	// Initialize tool registry
	defaultWorkspace := filepath.Join(home, "workspace")
	if agentCfg, ok := cfg.Agents["default"]; ok && agentCfg.Workspace != "" {
		defaultWorkspace = agentCfg.Workspace
	}

	// 注册内置工具
	registry := tool.NewRegistry()
	tool.RegisterFSTools(registry, defaultWorkspace)
	tool.RegisterExecTools(registry, defaultWorkspace)
	tool.RegisterWebTools(registry)

	mcpClient := mcp.NewClient()
	reloadMCP(context.Background(), cfg, mcpClient, registry, home)

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

	// Initialize router
	router := agent.NewRouter(loop, store)
	reloadSkills(cfg, router, defaultWorkspace)

	config.RegisterOnReload(func(cfg *config.Config) {
		reloadMCP(context.Background(), cfg, mcpClient, registry, home)
		dw := filepath.Join(home, "workspace")
		if agentCfg, ok := cfg.Agents["default"]; ok && agentCfg.Workspace != "" {
			dw = agentCfg.Workspace
		}
		reloadSkills(cfg, router, dw)
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

	configDir := filepath.Dir(config.Path())
	bridgeCount := 0
	for _, inst := range cfg.Bridges.Instances {
		if !inst.Enabled || inst.Path == "" {
			slog.Debug("bridge skipped", "id", inst.ID, "reason", "disabled or empty path")
			continue
		}
		bridgeDir := inst.Path
		if !filepath.IsAbs(bridgeDir) {
			bridgeDir = filepath.Join(configDir, bridgeDir)
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

	configDir := filepath.Dir(config.Path())
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
	for _, inst := range cfg.Bridges.Instances {
		if !inst.Enabled || inst.Path == "" {
			continue
		}
		bridgeDir := inst.Path
		if !filepath.IsAbs(bridgeDir) {
			bridgeDir = filepath.Join(configDir, bridgeDir)
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

func reloadSkills(cfg *config.Config, router *agent.Router, defaultWorkspace string) {
	for agentID, agentCfg := range cfg.Agents {
		skillDirs := agentCfg.Skills.Dirs
		if len(skillDirs) == 0 {
			ws := agentCfg.Workspace
			if ws == "" {
				ws = defaultWorkspace
			}
			skillDirs = []string{filepath.Join(ws, "skills")}
		}
		loaded := skills.LoadFromDirs(skillDirs)
		router.SetSkills(agentID, loaded)
		if len(loaded) > 0 {
			slog.Info("skills loaded", "agent", agentID, "count", len(loaded))
		}
	}
}
