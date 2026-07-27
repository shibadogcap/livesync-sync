package api

import (
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
)

// handleBrowse handles GET /api/browse?path=xxx
// Returns a directory listing for the file browser UI.
func (s *Server) handleBrowse(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	if path == "" {
		// Return drive letters on Windows, root on Unix
		if isWindows() {
			writeJSON(w, listWindowsDrives())
		} else {
			entry, _ := browseDir("/")
			writeJSON(w, entry)
		}
		return
	}

	entry, err := browseDir(path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, entry)
}

func isWindows() bool {
	return runtime.GOOS == "windows"
}

func listWindowsDrives() *DirEntry {
	root := &DirEntry{Name: "Computer", Path: "", IsDir: true}
	for _, d := range "ABCDEFGHIJKLMNOPQRSTUVWXYZ" {
		drive := string(d) + ":\\"
		// Use a timeout to avoid hanging on empty CD-ROM/floppy drives
		// os.Stat can block indefinitely on Windows for removable drives
		ch := make(chan bool, 1)
		go func(p string) {
			_, err := os.Stat(p)
			ch <- (err == nil)
		}(drive)

		select {
		case exists := <-ch:
			if exists {
				root.Children = append(root.Children, FileInfo{
					Name:  string(d) + ":",
					IsDir: true,
				})
			}
		case <-time.After(500 * time.Millisecond):
			// Drive timeout — skip it (likely empty removable drive)
			continue
		}
	}
	return root
}

// browseDir returns a directory listing for the given path.
func browseDir(path string) (*DirEntry, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}

	entry := &DirEntry{
		Name:  filepath.Base(path),
		Path:  path,
		IsDir: info.IsDir(),
		Size:  info.Size(),
		MTime: info.ModTime().UnixMilli(),
	}

	if info.IsDir() {
		entries, err := os.ReadDir(path)
		if err != nil {
			return nil, err
		}
		sort.Slice(entries, func(i, j int) bool {
			if entries[i].IsDir() != entries[j].IsDir() {
				return entries[i].IsDir() // directories first
			}
			return strings.ToLower(entries[i].Name()) < strings.ToLower(entries[j].Name())
		})
		for _, e := range entries {
			fi, _ := e.Info()
			stat := FileInfo{
				Name:  e.Name(),
				IsDir: e.IsDir(),
				Size:  fi.Size(),
				MTime: fi.ModTime().UnixMilli(),
			}
			entry.Children = append(entry.Children, stat)
		}
	}

	return entry, nil
}

// handleBrowseParent handles GET /api/browse/parent?path=xxx
// Returns the parent directory listing with the given path highlighted.
func (s *Server) handleBrowseParent(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	if path == "" {
		http.Error(w, "path required", http.StatusBadRequest)
		return
	}

	parent := filepath.Dir(path)
	entry, err := browseDir(parent)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	writeJSON(w, map[string]interface{}{
		"parent":  parent,
		"current": path,
		"listing": entry,
	})
}
