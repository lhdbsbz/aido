package config

import (
	"context"
	"log/slog"
	"path/filepath"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/spf13/viper"
)

// Watch watches the config file with Viper (WatchConfig + OnConfigChange) and hot-reloads.
// Run in a goroutine. On reload, updates in-memory config and runs RegisterOnReload callbacks.
func Watch(ctx context.Context) {
	path := Path()
	v := viper.New()
	v.SetConfigFile(path)
	if err := v.ReadInConfig(); err != nil {
		slog.Warn("config watch initial read failed", "path", path, "error", err)
		return
	}

	reload := func() {
		cfg, err := Load(path)
		if err != nil {
			slog.Warn("config hot-reload load failed", "path", path, "error", err)
			return
		}
		Set(cfg)
		notifyReload(cfg)
		slog.Info("config hot-reloaded", "path", path)
	}

	var debounce *time.Timer
	v.WatchConfig()
	v.OnConfigChange(func(e fsnotify.Event) {
		if e.Op&(fsnotify.Write|fsnotify.Create) == 0 {
			return
		}
		if filepath.Clean(e.Name) != filepath.Clean(path) {
			return
		}
		if debounce != nil {
			debounce.Stop()
		}
		debounce = time.AfterFunc(200*time.Millisecond, reload)
	})

	<-ctx.Done()
}
