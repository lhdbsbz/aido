package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/lhdbsbz/aido/internal/agent"
	"github.com/lhdbsbz/aido/internal/config"
	"github.com/lhdbsbz/aido/internal/gateway"
	"github.com/lhdbsbz/aido/internal/llm"
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

	// Load config
	cfgPath := config.ResolveConfigPath("")
	cfg, err := config.Load(cfgPath)
	if err != nil {
		slog.Warn("config not found, using defaults", "path", cfgPath, "error", err)
		cfg = config.DefaultConfig()
	}

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

	registry := tool.NewRegistry()
	tool.RegisterFSTools(registry, defaultWorkspace)
	tool.RegisterExecTools(registry, defaultWorkspace)
	tool.RegisterWebTools(registry, "")

	// Build default policy
	var globalPolicy tool.PolicyLayer
	if agentCfg, ok := cfg.Agents["default"]; ok {
		globalPolicy = tool.PolicyLayer{
			Profile: agentCfg.Tools.Profile,
			Allow:   agentCfg.Tools.Allow,
			Deny:    agentCfg.Tools.Deny,
		}
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
	router := agent.NewRouter(loop, cfg, store)

	// Load skills
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

	srv := gateway.NewServer(cfg, router)
	return srv.Start(ctx)
}
