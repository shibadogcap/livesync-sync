package restapi

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
)

// handleRoot returns API info.
func (s *Server) handleRoot(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"name":        "livesync-sync API",
		"version":     "0.1.0",
		"description": "REST API for vault file operations and sync management",
		"endpoints": map[string]string{
			"status":            "GET /status",
			"vault":             "GET/PUT/POST/DELETE/PATCH /vault/*",
			"vault_move":        "MOVE /vault/* (with Destination header)",
			"vault_copy":        "COPY /vault/* (with Destination header)",
			"tags":              "GET /tags",
			"search":            "POST /search/simple",
			"sync_status":       "GET /sync/status",
			"sync_pause":        "POST /sync/pause",
			"sync_resume":       "POST /sync/resume",
		},
	})
}

// handleStatus returns server health.
func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status": "healthy",
		"vault":  s.cfg.VaultDir,
	})
}

// handleVaultGet handles GET /vault/* — list directory or read file.
func (s *Server) handleVaultGet(w http.ResponseWriter, r *http.Request) {
	vpath := chiURLParam(r, "*")
	if vpath == "" {
		vpath = "."
	}

	result, err := s.vault.Get(vpath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if result.IsDir {
		// Return directory listing
		writeJSON(w, http.StatusOK, result)
		return
	}

	// Return file content with appropriate content type
	contentType := detectContentType(vpath)
	writePlain(w, http.StatusOK, contentType, result.Content)
}

// handleVaultPut handles PUT /vault/* — create or overwrite file.
func (s *Server) handleVaultPut(w http.ResponseWriter, r *http.Request) {
	vpath := chiURLParam(r, "*")
	if vpath == "" {
		writeError(w, http.StatusBadRequest, "path required")
		return
	}

	body, err := readBody(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "failed to read body")
		return
	}

	if err := s.vault.Put(vpath, body); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"path": vpath, "status": "created"})
}

// handleVaultPost handles POST /vault/* — append to file.
func (s *Server) handleVaultPost(w http.ResponseWriter, r *http.Request) {
	vpath := chiURLParam(r, "*")
	if vpath == "" {
		writeError(w, http.StatusBadRequest, "path required")
		return
	}

	body, err := readBody(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "failed to read body")
		return
	}

	if err := s.vault.Append(vpath, body); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"path": vpath, "status": "appended"})
}

// handleVaultDelete handles DELETE /vault/* — delete file.
func (s *Server) handleVaultDelete(w http.ResponseWriter, r *http.Request) {
	vpath := chiURLParam(r, "*")
	if vpath == "" {
		writeError(w, http.StatusBadRequest, "path required")
		return
	}

	if err := s.vault.Delete(vpath); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"path": vpath, "status": "deleted"})
}

// handleVaultPatch handles PATCH /vault/* — section operations.
// Simplified: supports replacing content at a section.
func (s *Server) handleVaultPatch(w http.ResponseWriter, r *http.Request) {
	vpath := chiURLParam(r, "*")
	if vpath == "" {
		writeError(w, http.StatusBadRequest, "path required")
		return
	}

	var patchReq struct {
		Operation string `json:"operation"` // replace, prepend, append, delete
		Target    string `json:"target"`    // heading path or block ref
		Content   string `json:"content"`
	}

	if err := json.NewDecoder(r.Body).Decode(&patchReq); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	if err := s.vault.Patch(vpath, patchReq.Operation, patchReq.Target, patchReq.Content); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"path": vpath, "status": "patched"})
}

// handleVaultMove handles MOVE /vault/* — move/rename file.
func (s *Server) handleVaultMove(w http.ResponseWriter, r *http.Request) {
	dest := r.Header.Get("Destination")
	if dest == "" {
		writeError(w, http.StatusBadRequest, "Destination header required")
		return
	}

	vpath := chiURLParam(r, "*")
	allowOverwrite := r.Header.Get("Allow-Overwrite") == "true"

	if err := s.vault.Move(vpath, dest, allowOverwrite); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"source":      vpath,
		"destination": dest,
		"status":      "moved",
	})
}

// handleVaultCopy handles COPY /vault/* — copy file.
func (s *Server) handleVaultCopy(w http.ResponseWriter, r *http.Request) {
	dest := r.Header.Get("Destination")
	if dest == "" {
		writeError(w, http.StatusBadRequest, "Destination header required")
		return
	}

	vpath := chiURLParam(r, "*")
	allowOverwrite := r.Header.Get("Allow-Overwrite") == "true"

	if err := s.vault.Copy(vpath, dest, allowOverwrite); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"source":      vpath,
		"destination": dest,
		"status":      "copied",
	})
}

// handleTags returns all tags found in vault files.
func (s *Server) handleTags(w http.ResponseWriter, r *http.Request) {
	tags, err := s.vault.ListTags()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"tags": tags})
}

// handleSearchSimple handles POST /search/simple — full-text search.
func (s *Server) handleSearchSimple(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("query")
	if query == "" {
		writeError(w, http.StatusBadRequest, "query parameter required")
		return
	}

	results, err := s.vault.Search(query)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"query":   query,
		"results": results,
	})
}

// --- Sync handlers ---

func (s *Server) handleSyncStatus(w http.ResponseWriter, r *http.Request) {
	if s.hub == nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"running": false,
			"peers":   0,
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"running": s.hub.IsRunning(),
		"peers":   s.hub.Peers(),
	})
}

func (s *Server) handleSyncPause(w http.ResponseWriter, r *http.Request) {
	if s.hub == nil {
		writeError(w, http.StatusServiceUnavailable, "sync engine not available")
		return
	}
	if err := s.hub.Pause(); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "paused"})
}

func (s *Server) handleSyncResume(w http.ResponseWriter, r *http.Request) {
	if s.hub == nil {
		writeError(w, http.StatusServiceUnavailable, "sync engine not available")
		return
	}
	if err := s.hub.Resume(); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "resumed"})
}

// --- Helpers ---

func chiURLParam(r *http.Request, key string) string {
	// For chi v5, URLParam is on the context
	return chi.URLParam(r, key)
}

func readBody(r *http.Request) ([]byte, error) {
	defer r.Body.Close()
	body := make([]byte, r.ContentLength)
	if r.ContentLength > 0 {
		_, err := r.Body.Read(body)
		if err != nil {
			return nil, err
		}
	}
	return body, nil
}

func detectContentType(path string) string {
	// Simple content type detection based on extension
	ext := ""
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '.' {
			ext = path[i:]
			break
		}
		if path[i] == '/' {
			break
		}
	}
	switch ext {
	case ".md", ".markdown":
		return "text/markdown; charset=utf-8"
	case ".html", ".htm":
		return "text/html; charset=utf-8"
	case ".json":
		return "application/json"
	case ".yaml", ".yml":
		return "text/yaml; charset=utf-8"
	case ".txt":
		return "text/plain; charset=utf-8"
	case ".css":
		return "text/css; charset=utf-8"
	case ".js", ".mjs":
		return "application/javascript"
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".svg":
		return "image/svg+xml"
	case ".pdf":
		return "application/pdf"
	default:
		return "text/plain; charset=utf-8"
	}
}

var _ = slog.Debug
