// Package restapi provides a REST API for vault file CRUD and sync management.
// Pattern follows obsidian-local-rest-api with adaptations for standalone operation.
package restapi

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// Server is the REST API server.
type Server struct {
	cfg       Config
	vault     *VaultOps
	hub       SyncControl
	authToken string
	srv       *http.Server
}

// Config holds REST API configuration.
type Config struct {
	Listen    string // e.g. ":2323"
	AuthToken string // API key for Bearer auth
	VaultDir  string // Root directory for vault operations
}

// SyncControl is the interface for sync engine control (injected from main).
type SyncControl interface {
	Pause() error
	Resume() error
	IsRunning() bool
	Peers() int
}

// New creates a new REST API server.
func New(cfg Config, hub SyncControl) *Server {
	return &Server{
		cfg:       cfg,
		vault:     NewVaultOps(cfg.VaultDir),
		hub:       hub,
		authToken: cfg.AuthToken,
	}
}

// ListenAndServe starts the HTTP server.
func (s *Server) ListenAndServe() error {
	r := chi.NewRouter()

	// Middleware
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(60 * time.Second))
	r.Use(s.corsMiddleware)
	r.Use(s.authMiddleware)

	// Public routes
	r.Get("/", s.handleRoot)
	r.Get("/status", s.handleStatus)

	// Vault CRUD
	r.Route("/vault", func(r chi.Router) {
		r.Get("/*", s.handleVaultGet)
		r.Put("/*", s.handleVaultPut)
		r.Post("/*", s.handleVaultPost)
		r.Delete("/*", s.handleVaultDelete)
		r.Patch("/*", s.handleVaultPatch)
		r.MethodFunc("MOVE", "/*", s.handleVaultMove)
		r.MethodFunc("COPY", "/*", s.handleVaultCopy)
	})

	// Extra
	r.Get("/tags", s.handleTags)
	r.Post("/search/simple", s.handleSearchSimple)

	// Sync control
	r.Get("/sync/status", s.handleSyncStatus)
	r.Post("/sync/pause", s.handleSyncPause)
	r.Post("/sync/resume", s.handleSyncResume)

	addr := s.cfg.Listen
	if addr == "" {
		addr = ":2323"
	}

	s.srv = &http.Server{
		Addr:         addr,
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	slog.Info("[REST] API server starting", "addr", addr)
	return s.srv.ListenAndServe()
}

// Shutdown gracefully stops the server.
func (s *Server) Shutdown(ctx context.Context) error {
	if s.srv != nil {
		return s.srv.Shutdown(ctx)
	}
	return nil
}

// corsMiddleware allows cross-origin requests.
func (s *Server) corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods",
			"GET,PUT,POST,DELETE,PATCH,MOVE,COPY,OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers",
			"Authorization,Content-Type,Destination,Allow-Overwrite")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// authMiddleware checks the Bearer token.
func (s *Server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Public endpoints
		if r.URL.Path == "/" || r.URL.Path == "/status" {
			next.ServeHTTP(w, r)
			return
		}
		if s.authToken == "" {
			next.ServeHTTP(w, r)
			return
		}
		auth := r.Header.Get("Authorization")
		if auth == "" || len(auth) < 7 || auth[:7] != "Bearer " {
			writeError(w, http.StatusUnauthorized, "Authorization: Bearer <token> required")
			return
		}
		if auth[7:] != s.authToken {
			writeError(w, http.StatusUnauthorized, "Invalid API key")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// --- Response helpers ---

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Warn("[REST] encode error", "error", err)
	}
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{
		"error":   http.StatusText(status),
		"message": msg,
	})
}

func writePlain(w http.ResponseWriter, status int, contentType, body string) {
	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(status)
	fmt.Fprint(w, body)
}

var _ = fmt.Sprintf

