// Package api provides the embedded settings HTTP server and REST API.
package api

import (
	"embed"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os/exec"
	"runtime"

	"github.com/go-chi/chi/v5"
	"github.com/user/livesync-sync/internal/config"
)

//go:embed ui/settings.html
var settingsUI embed.FS

// Server is the HTTP server providing settings UI, config API, and vault REST API.
type Server struct {
	cfg     *config.FullConfig
	onSave  func(*config.FullConfig) error
	onReset func() error
	onPause func(bool) error
	running bool

	// REST API
	vaultHandler *VaultHandler

	// hub status access
	hubStatus func() map[string]interface{}
}

// New creates a new API server.
func New(cfg *config.FullConfig, opts ...Option) *Server {
	s := &Server{cfg: cfg}
	for _, o := range opts {
		o(s)
	}

	// Initialize vault ops (find first storage peer's base dir)
	if baseDir := findFirstStorageDir(cfg); baseDir != "" {
		ops := NewVaultOps(baseDir)
		s.vaultHandler = NewVaultHandler(ops, s)
	}

	return s
}

// Option configures the API server.
type Option func(*Server)

// WithOnSave sets the config save callback.
func WithOnSave(fn func(*config.FullConfig) error) Option {
	return func(s *Server) { s.onSave = fn }
}

// WithOnReset sets the sync reset callback.
func WithOnReset(fn func() error) Option {
	return func(s *Server) { s.onReset = fn }
}

// WithOnPause sets the pause/resume callback.
func WithOnPause(fn func(bool) error) Option {
	return func(s *Server) { s.onPause = fn }
}

// WithRunning sets the initial running state.
func WithRunning(running bool) Option {
	return func(s *Server) { s.running = running }
}

// WithHubStatus provides a function to query hub status.
func WithHubStatus(fn func() map[string]interface{}) Option {
	return func(s *Server) { s.hubStatus = fn }
}

// ListenAndServe starts the HTTP server.
func (s *Server) ListenAndServe() error {
	r := chi.NewRouter()

	// Settings UI
	r.Get("/settings", func(w http.ResponseWriter, r *http.Request) {
		data, err := settingsUI.ReadFile("ui/settings.html")
		if err != nil {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(data)
	})

	// Config endpoints
	r.Get("/api/config", s.handleConfig)
	r.Put("/api/config", s.handleConfig)
	r.Get("/api/status", s.handleStatus)
	r.Post("/api/sync/pause", s.handlePause)
	r.Post("/api/sync/resume", s.handleResume)
	r.Post("/api/sync/reset", s.handleReset)

	// Vault REST API (if vault handler is configured)
	if s.vaultHandler != nil {
		s.vaultHandler.Mount(r)

		// MCP server (uses the same vault ops)
		mcpHandler := NewMCPHandler(s.vaultHandler.Ops)
		r.Post("/mcp", mcpHandler.HandleRequest)
		r.Get("/mcp", mcpHandler.HandleRequest) // Some clients use GET
	}

	// Redirect root to settings
	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/settings", http.StatusFound)
	})

	addr := s.cfg.API.Listen
	if addr == "" {
		addr = "localhost:2324"
	}

	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("api listen failed: %w", err)
	}

	slog.Info("[API] Settings UI available at", "url", fmt.Sprintf("http://%s/settings", addr))
	return http.Serve(listener, r)
}

// OpenBrowser opens the settings UI in the default browser.
func (s *Server) OpenBrowser() {
	addr := s.cfg.API.Listen
	if addr == "" {
		addr = "localhost:2324"
	}
	url := fmt.Sprintf("http://%s/settings", addr)

	switch runtime.GOOS {
	case "linux":
		exec.Command("xdg-open", url).Start()
	case "windows":
		exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	case "darwin":
		exec.Command("open", url).Start()
	default:
		slog.Info("[API] Open in browser:", "url", url)
	}
}

// HubProvider implementation for the vault handler.
func (s *Server) Pause() {
	if s.onPause != nil {
		s.onPause(true)
	}
	s.running = false
}

func (s *Server) Resume() {
	if s.onPause != nil {
		s.onPause(false)
	}
	s.running = true
}

func (s *Server) Status() map[string]interface{} {
	if s.hubStatus != nil {
		return s.hubStatus()
	}
	peerCount := len(s.cfg.Sync.Peers)
	return map[string]interface{}{
		"running": s.running,
		"peers":   peerCount,
	}
}

// findFirstStorageDir finds the first storage peer's base directory from config.
func findFirstStorageDir(cfg *config.FullConfig) string {
	for _, p := range cfg.Sync.Peers {
		if p.Type == "storage" && p.BaseDir != "" {
			return p.BaseDir
		}
	}
	return ""
}

func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, s.cfg)
	case http.MethodPut:
		var newCfg config.FullConfig
		if err := json.NewDecoder(r.Body).Decode(&newCfg); err != nil {
			http.Error(w, fmt.Sprintf("invalid JSON: %v", err), http.StatusBadRequest)
			return
		}
		if s.onSave != nil {
			if err := s.onSave(&newCfg); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}
		s.cfg = &newCfg
		writeJSON(w, map[string]bool{"ok": true})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	st := s.Status()
	st["version"] = "dev"
	writeJSON(w, st)
}

func (s *Server) handlePause(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.Pause()
	writeJSON(w, map[string]bool{"ok": true})
}

func (s *Server) handleResume(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.Resume()
	writeJSON(w, map[string]bool{"ok": true})
}

func (s *Server) handleReset(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.onReset != nil {
		s.onReset()
	}
	writeJSON(w, map[string]bool{"ok": true})
}

// writeJSON is a helper to write JSON responses.
func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}


