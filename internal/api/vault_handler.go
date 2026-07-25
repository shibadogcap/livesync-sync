package api

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
)

// VaultHandler handles vault-related HTTP endpoints.
type VaultHandler struct {
	Ops *VaultOps
	hub HubProvider
}

// HubProvider abstracts access to the sync hub for API handlers.
type HubProvider interface {
	Pause()
	Resume()
	Status() map[string]interface{}
}

// NewVaultHandler creates a new VaultHandler.
func NewVaultHandler(ops *VaultOps, hub HubProvider) *VaultHandler {
	return &VaultHandler{Ops: ops, hub: hub}
}

// Mount adds vault routes to the given chi router.
func (h *VaultHandler) Mount(r chi.Router) {
	r.Route("/api/vault", func(r chi.Router) {
		r.Get("/search", h.handleSearch)
		r.Get("/health", h.handleHealth)
		r.Get("/*", h.handleGet)       // GET /api/vault/* — list or read
		r.Put("/*", h.handlePut)       // PUT /api/vault/* — write
		r.Post("/*", h.handlePost)     // POST /api/vault/* — append
		r.Delete("/*", h.handleDelete) // DELETE /api/vault/* — delete
	})
}

// handleGet handles GET /api/vault/{path}
// If the path is a directory, returns a listing.
// If the path is a file, returns the file content.
func (h *VaultHandler) handleGet(w http.ResponseWriter, r *http.Request) {
	vaultPath := chi.URLParam(r, "*")

	// Check if it's a directory (trailing slash or known directory)
	if strings.HasSuffix(vaultPath, "/") || vaultPath == "" {
		h.listDir(w, r, vaultPath)
		return
	}

	// Try to read as file first
	data, info, err := h.Ops.ReadFile(vaultPath)
	if err == nil {
		// File found — return content
		accept := r.Header.Get("Accept")
		if strings.Contains(accept, "application/json") {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"content": string(data),
				"file":    info,
			})
		} else {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.Write(data)
		}
		return
	}

	// Not a file or error — try as directory
	h.listDir(w, r, vaultPath)
}

func (h *VaultHandler) listDir(w http.ResponseWriter, r *http.Request, vaultPath string) {
	entry, err := h.Ops.ListDir(vaultPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	writeJSON(w, entry)
}

// handlePut handles PUT /api/vault/{path} — write file.
func (h *VaultHandler) handlePut(w http.ResponseWriter, r *http.Request) {
	vaultPath := chi.URLParam(r, "*")
	if vaultPath == "" || strings.HasSuffix(vaultPath, "/") {
		http.Error(w, "path is a directory", http.StatusBadRequest)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "failed to read body", http.StatusBadRequest)
		return
	}

	// If content type is JSON, extract "content" field
	var data []byte
	ct := r.Header.Get("Content-Type")
	if strings.Contains(ct, "application/json") {
		var payload struct {
			Content string `json:"content"`
		}
		if err := json.Unmarshal(body, &payload); err == nil && payload.Content != "" {
			data = []byte(payload.Content)
		} else {
			data = body
		}
	} else {
		data = body
	}

	if err := h.Ops.WriteFile(vaultPath, data); err != nil {
		slog.Warn("[API] Write failed", "path", vaultPath, "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, map[string]bool{"ok": true})
}

// handlePost handles POST /api/vault/{path} — append to file.
func (h *VaultHandler) handlePost(w http.ResponseWriter, r *http.Request) {
	vaultPath := chi.URLParam(r, "*")
	if vaultPath == "" || strings.HasSuffix(vaultPath, "/") {
		http.Error(w, "path is a directory", http.StatusBadRequest)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "failed to read body", http.StatusBadRequest)
		return
	}

	if err := h.Ops.AppendFile(vaultPath, body); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, map[string]bool{"ok": true})
}

// handleDelete handles DELETE /api/vault/{path} — delete file.
func (h *VaultHandler) handleDelete(w http.ResponseWriter, r *http.Request) {
	vaultPath := chi.URLParam(r, "*")
	if vaultPath == "" {
		http.Error(w, "path required", http.StatusBadRequest)
		return
	}

	permanent := r.URL.Query().Get("permanent") == "true"

	if err := h.Ops.DeleteFile(vaultPath, permanent); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, map[string]bool{"ok": true})
}

// handleSearch handles GET /api/vault/search?q=query.
func (h *VaultHandler) handleSearch(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")

	results, err := h.Ops.SearchFiles(query)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if results == nil {
		results = []FileInfo{}
	}

	writeJSON(w, map[string]interface{}{
		"results": results,
		"total":   len(results),
	})
}

// handleHealth handles GET /api/vault/health.
func (h *VaultHandler) handleHealth(w http.ResponseWriter, r *http.Request) {
	status := h.hub.Status()
	status["vault"] = h.Ops.BaseDir()
	writeJSON(w, status)
}
