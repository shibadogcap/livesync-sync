package main

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"

	"github.com/user/livesync-sync/internal/api"
	"github.com/user/livesync-sync/internal/config"
	"github.com/user/livesync-sync/internal/logger"
	"github.com/user/livesync-sync/internal/state"
	"github.com/user/livesync-sync/internal/sync"
	"github.com/user/livesync-sync/internal/tray"
)

var version = "dev"

func main() {
	// Parse command-line flags
	daemon := parseFlags()
	if daemon == nil {
		return // --version was shown
	}

	// Load configuration
	cfg, err := config.LoadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}

	// Initialize logger
	if err := logger.Init(cfg.Logging.Level, cfg.Logging.File); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to init logger: %v\n", err)
		os.Exit(1)
	}

	slog.Info("livesync-sync starting", "version", version)

	// Initialize state store
	store, err := state.New(cfg.State.File)
	if err != nil {
		slog.Error("Failed to init state store", "error", err)
		os.Exit(1)
	}
	defer store.Close()

	// Handle --reset (state reset requested via env)
	if os.Getenv("LSYNC_RESET_STATE") == "1" {
		slog.Info("Resetting sync state (--reset)")
		store.Clear()
	}

	// Create hub first (pre-declared for closures), start later
	var hub *sync.Hub

	// Status function that lazily reads from hub (updated after hub creation)
	hubStatusFn := func() map[string]interface{} {
		if hub == nil {
			return map[string]interface{}{"running": false, "peers": 0}
		}
		return map[string]interface{}{
			"running": true,
			"peers":   len(hub.Peers()),
		}
	}

	// Start settings API server before sync engine (UI must always be accessible)
	apiSrv := api.New(cfg,
		api.WithOnSave(func(newCfg *config.FullConfig) error {
			return config.SaveConfig(newCfg)
		}),
		api.WithOnReset(func() error {
			store.Clear()
			return nil
		}),
		api.WithOnPause(func(pause bool) error {
			if hub == nil {
				return nil
			}
			if pause {
				hub.Stop()
			} else {
				hub.Start()
			}
			return nil
		}),
		api.WithRunning(true),
		api.WithHubStatus(hubStatusFn),
	)

	go func() {
		if err := apiSrv.ListenAndServe(); err != nil {
			slog.Warn("[API] Server stopped", "error", err)
		}
	}()

	// Create and start sync Hub
	hub = sync.NewHub(cfg, store)

	go func() {
		if err := hub.Start(); err != nil {
			slog.Error("[Hub] Sync engine failed to start", "error", err)
		}
	}()

	// Wait for shutdown
	if *daemon {
		slog.Info("Running in daemon mode (--daemon)")
		waitForSignal()
	} else {
		slog.Info("Running in desktop mode (tray available)")
		if cfg.Tray.Enable {
			runWithTray(cfg, apiSrv, hub, store)
		} else {
			waitForSignal()
		}
	}

	slog.Info("Shutting down...")
	hub.Stop()
	slog.Info("Goodbye")
}

// runWithTray starts the system tray and blocks until quit.
func runWithTray(cfg *config.FullConfig, apiSrv *api.Server, hub *sync.Hub, store *state.Store) {
	trayMgr := tray.New(tray.Config{
		Enable:    cfg.Tray.Enable,
		Autostart: cfg.Tray.Autostart,
		OnOpenVault: func() {
			for _, p := range cfg.Sync.Peers {
				if p.Type == "storage" && p.BaseDir != "" {
					openFolder(p.BaseDir)
					return
				}
			}
		},
		OnSettings: func() {
			apiSrv.OpenBrowser()
		},
		OnPause: func() {
			if hub != nil {
				hub.Stop()
			}
		},
		OnResume: func() {
			if hub != nil {
				hub.Start()
			}
		},
	})

	trayMgr.SetSyncStatus(true)
	if err := trayMgr.Run(); err != nil {
		slog.Warn("[Tray] Tray exited", "error", err)
	}
}

// waitForSignal blocks until SIGINT or SIGTERM.
func waitForSignal() {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh
}

// openFolder opens a folder in the system file manager.
func openFolder(path string) {
	abs, err := filepath.Abs(path)
	if err != nil {
		slog.Warn("[App] Cannot open vault folder", "path", path, "error", err)
		return
	}
	slog.Info("[App] Opening vault folder", "path", abs)
	switch runtime.GOOS {
	case "linux":
		exec.Command("xdg-open", abs).Start()
	case "windows":
		exec.Command("explorer", abs).Start()
	case "darwin":
		exec.Command("open", abs).Start()
	default:
		slog.Info("[App] Open folder manually:", "path", abs)
	}
}

func parseFlags() *bool {
	daemon := false
	showVersion := false

	args := os.Args[1:]
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--daemon":
			daemon = true
		case "--version":
			showVersion = true
		case "--reset":
			os.Setenv("LSYNC_RESET_STATE", "1")
		case "--config":
			if i+1 < len(args) {
				os.Setenv("LSYNC_CONFIG", args[i+1])
				i++
			}
		}
	}

	if showVersion {
		fmt.Printf("livesync-sync version %s\n", version)
		return nil
	}

	return &daemon
}
