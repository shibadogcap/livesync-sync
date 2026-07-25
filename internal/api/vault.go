// Package api provides REST API and settings UI for livesync-sync.
package api

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// VaultOps provides filesystem operations for the REST API.
// It works directly on the vault directory without sync engine coupling.
type VaultOps struct {
	baseDir string // Absolute path to the vault root
}

// NewVaultOps creates a new VaultOps with the given base directory.
func NewVaultOps(baseDir string) *VaultOps {
	abs, _ := filepath.Abs(baseDir)
	return &VaultOps{baseDir: abs}
}

// BaseDir returns the absolute base directory path.
func (vo *VaultOps) BaseDir() string {
	return vo.baseDir
}

// resolve checks path traversal and returns absolute path.
func (vo *VaultOps) resolve(vaultPath string) (string, error) {
	full := filepath.Join(vo.baseDir, vaultPath)
	abs, err := filepath.Abs(full)
	if err != nil {
		return "", err
	}
	if !strings.HasPrefix(abs, vo.baseDir) {
		return "", fmt.Errorf("path traversal denied")
	}
	return abs, nil
}

// ListDir lists files and directories under the given path.
func (vo *VaultOps) ListDir(vaultPath string) (*DirEntry, error) {
	absPath, err := vo.resolve(vaultPath)
	if err != nil {
		return nil, err
	}

	info, err := os.Stat(absPath)
	if err != nil {
		return nil, err
	}

	entry := &DirEntry{
		Name:  filepath.Base(absPath),
		Path:  toRel(vo.baseDir, absPath),
		IsDir: info.IsDir(),
		Size:  info.Size(),
		MTime: info.ModTime().UnixMilli(),
	}

	if info.IsDir() {
		entries, err := os.ReadDir(absPath)
		if err != nil {
			return nil, err
		}
		sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
		for _, e := range entries {
			fi, _ := e.Info()
			entry.Children = append(entry.Children, FileInfo{
				Name:  e.Name(),
				IsDir: e.IsDir(),
				Size:  fi.Size(),
				MTime: fi.ModTime().UnixMilli(),
			})
		}
	}
	return entry, nil
}

// ReadFile reads a file's content and metadata.
func (vo *VaultOps) ReadFile(vaultPath string) ([]byte, *FileInfo, error) {
	absPath, err := vo.resolve(vaultPath)
	if err != nil {
		return nil, nil, err
	}

	info, err := os.Stat(absPath)
	if err != nil {
		return nil, nil, err
	}
	if info.IsDir() {
		return nil, nil, fmt.Errorf("is a directory")
	}

	content, err := os.ReadFile(absPath)
	if err != nil {
		return nil, nil, err
	}

	return content, &FileInfo{
		Name:  info.Name(),
		IsDir: false,
		Size:  info.Size(),
		MTime: info.ModTime().UnixMilli(),
		Rel:   toRel(vo.baseDir, absPath),
	}, nil
}

// WriteFile creates or overwrites a file.
func (vo *VaultOps) WriteFile(vaultPath string, content []byte) error {
	absPath, err := vo.resolve(vaultPath)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(absPath), 0755); err != nil {
		return err
	}
	return os.WriteFile(absPath, content, 0644)
}

// AppendFile appends content to a file.
func (vo *VaultOps) AppendFile(vaultPath string, content []byte) error {
	absPath, err := vo.resolve(vaultPath)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(absPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(content)
	return err
}

// DeleteFile removes a file. If permanent is false, moves to .trash.
func (vo *VaultOps) DeleteFile(vaultPath string, permanent bool) error {
	absPath, err := vo.resolve(vaultPath)
	if err != nil {
		return err
	}

	if permanent {
		return os.Remove(absPath)
	}

	// Move to trash with timestamp suffix
	trashDir := filepath.Join(vo.baseDir, ".trash")
	if err := os.MkdirAll(trashDir, 0755); err != nil {
		return err
	}
	ts := strconv.FormatInt(time.Now().UnixMilli(), 36)
	trashPath := filepath.Join(trashDir, filepath.Base(absPath)+"."+ts)
	return os.Rename(absPath, trashPath)
}

// SearchFiles searches for files matching a query.
func (vo *VaultOps) SearchFiles(query string) ([]FileInfo, error) {
	var results []FileInfo
	q := strings.ToLower(query)

	err := filepath.Walk(vo.baseDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		rel := toRel(vo.baseDir, path)
		if q == "" || strings.Contains(strings.ToLower(rel), q) {
			results = append(results, FileInfo{
				Name:  info.Name(),
				IsDir: false,
				Size:  info.Size(),
				MTime: info.ModTime().UnixMilli(),
				Rel:   rel,
			})
		}
		return nil
	})
	return results, err
}

func toRel(base, target string) string {
	r, _ := filepath.Rel(base, target)
	return strings.ReplaceAll(r, "\\", "/")
}

// DirEntry is a directory listing result.
type DirEntry struct {
	Name     string     `json:"name"`
	Path     string     `json:"path"`
	IsDir    bool       `json:"isDir"`
	Size     int64      `json:"size"`
	MTime    int64      `json:"mtime"`
	Children []FileInfo `json:"children,omitempty"`
}

// FileInfo is a file's metadata.
type FileInfo struct {
	Name  string `json:"name"`
	IsDir bool   `json:"isDir"`
	Size  int64  `json:"size"`
	MTime int64  `json:"mtime"`
	Rel   string `json:"rel,omitempty"`
}
