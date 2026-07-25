package main

import (
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/user/livesync-sync/internal/config"
	"github.com/user/livesync-sync/internal/logger"
	"github.com/user/livesync-sync/internal/state"
	"github.com/user/livesync-sync/internal/sync"
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

	// Create sync Hub
	hub := sync.NewHub(cfg, store)

	// Start sync engine
	if err := hub.Start(); err != nil {
		slog.Error("Failed to start sync engine", "error", err)
		os.Exit(1)
	}

	// Wait for shutdown signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	if *daemon {
		slog.Info("Running in daemon mode")
	} else {
		slog.Info("Running in foreground mode (tray available on desktop)")
	}

	<-sigCh

	slog.Info("Shutting down...")
	hub.Stop()
	slog.Info("Goodbye")
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
