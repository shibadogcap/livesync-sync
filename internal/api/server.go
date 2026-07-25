// Package api provides the embedded settings HTTP server.
// Serves the settings UI and provides REST endpoints for config/status management.
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

	"github.com/user/livesync-sync/internal/config"
)

//go:embed ui/settings.html
var settingsUI embed.FS

// Server is the embedded settings HTTP server.
type Server struct {
	cfg     *config.FullConfig
	onSave  func(*config.FullConfig) error
	onReset func() error
	onPause func(bool) error
	running bool
}

// New creates a new settings API server.
func New(cfg *config.FullConfig, opts ...Option) *Server {
	s := &Server{cfg: cfg}
	for _, o := range opts {
		o(s)
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

// ListenAndServe starts the HTTP server on the configured address.
func (s *Server) ListenAndServe() error {
	mux := http.NewServeMux()

	// Settings UI
	mux.HandleFunc("/settings", func(w http.ResponseWriter, r *http.Request) {
		data, err := settingsUI.ReadFile("ui/settings.html")
		if err != nil {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(data)
	})

	// Config endpoints
	mux.HandleFunc("/api/config", s.handleConfig)
	mux.HandleFunc("/api/status", s.handleStatus)
	mux.HandleFunc("/api/sync/pause", s.handlePause)
	mux.HandleFunc("/api/sync/resume", s.handleResume)
	mux.HandleFunc("/api/sync/reset", s.handleReset)

	// Redirect root to settings
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			http.Redirect(w, r, "/settings", http.StatusFound)
			return
		}
		http.NotFound(w, r)
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
	return http.Serve(listener, mux)
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

func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")

	switch r.Method {
	case "GET":
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(s.cfg)

	case "PUT":
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
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]bool{"ok": true})

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	peerCount := len(s.cfg.Sync.Peers)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"running": s.running,
		"peers":   peerCount,
		"version": "dev",
	})
}

func (s *Server) handlePause(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.onPause != nil {
		s.onPause(true)
	}
	s.running = false
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}

func (s *Server) handleResume(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.onPause != nil {
		s.onPause(false)
	}
	s.running = true
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}

func (s *Server) handleReset(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.onReset != nil {
		s.onReset()
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}
