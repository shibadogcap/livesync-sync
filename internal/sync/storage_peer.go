package sync

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/user/livesync-sync/internal/config"
	"github.com/user/livesync-sync/internal/state"
)

// StoragePeer synchronizes a local filesystem directory.
// Matches livesync-now PeerStorage.ts.
type StoragePeer struct {
	BasePeer
	config       config.PeerConf
	watcher      *fsnotify.Watcher
	done         chan struct{}
}

// NewStoragePeer creates a new Storage peer.
func NewStoragePeer(conf config.PeerConf, store *state.Store, dispatch DispatchFunc) *StoragePeer {
	return &StoragePeer{
		BasePeer: NewBasePeer(conf, store, dispatch),
		config:   conf,
		done:     make(chan struct{}),
	}
}

// Start begins watching the filesystem for changes.
// Matches PeerStorage.start() in livesync-now.
func (p *StoragePeer) Start() error {
	slog.Info("[Storage] Starting peer", "name", p.config.Name, "dir", p.config.BaseDir)

	// Ensure base directory exists
	if err := os.MkdirAll(p.config.BaseDir, 0755); err != nil {
		return fmt.Errorf("mkdir failed: %w", err)
	}

	// Scan offline changes if enabled
	if p.config.ScanOfflineChanges != nil && *p.config.ScanOfflineChanges {
		if err := p.scanOfflineChanges(); err != nil {
			slog.Warn("[Storage] Offline scan failed", "error", err)
		}
	}

	// Start fsnotify watcher
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("fsnotify init failed: %w", err)
	}
	p.watcher = watcher

	// Watch the base directory recursively
	if err := p.addRecursiveWatch(p.config.BaseDir); err != nil {
		return fmt.Errorf("add watch failed: %w", err)
	}

	go p.watchLoop()

	slog.Info("[Storage] Peer started", "name", p.config.Name)
	return nil
}

// Stop stops the filesystem watcher.
func (p *StoragePeer) Stop() error {
	slog.Info("[Storage] Stopping peer", "name", p.config.Name)
	if p.watcher != nil {
		p.watcher.Close()
	}
	close(p.done)
	return nil
}

// Put writes a file to the local filesystem.
// Preserves mtime from the source.
// Matches PeerStorage.put() in livesync-now.
func (p *StoragePeer) Put(path string, data FileData) (bool, error) {
	if p.isIgnored(path) {
		return false, nil
	}

	if repeat, err := p.IsRepeating(path, &data); err != nil {
		return false, err
	} else if repeat {
		return false, nil
	}

	localPath := p.ToLocalPath(path)
	fullPath := filepath.Join(p.config.BaseDir, localPath)

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
		return false, fmt.Errorf("mkdir failed: %w", err)
	}

	// Write file
	if err := os.WriteFile(fullPath, data.Data, 0644); err != nil {
		return false, fmt.Errorf("write failed: %w", err)
	}

	// Preserve mtime
	if data.MTime > 0 {
		os.Chtimes(fullPath, time.UnixMilli(data.CTime), time.UnixMilli(data.MTime))
	}

	// Update file stat
	p.writeFileStat(path, data.MTime, data.Size)

	slog.Debug("[Storage] File written", "path", localPath)
	return true, nil
}

// Get reads a file from the local filesystem.
func (p *StoragePeer) Get(path string) (*FileData, error) {
	localPath := p.ToLocalPath(path)
	fullPath := filepath.Join(p.config.BaseDir, localPath)

	stat, err := os.Stat(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	content, err := os.ReadFile(fullPath)
	if err != nil {
		return nil, err
	}

	return &FileData{
		CTime: stat.ModTime().UnixMilli(),
		MTime: stat.ModTime().UnixMilli(),
		Size:  stat.Size(),
		Data:  content,
	}, nil
}

// Delete removes a file from the local filesystem.
func (p *StoragePeer) Delete(path string) (bool, error) {
	if p.isIgnored(path) {
		return false, nil
	}

	if repeat, err := p.IsRepeating(path, nil); err != nil {
		return false, err
	} else if repeat {
		return false, nil
	}

	localPath := p.ToLocalPath(path)
	fullPath := filepath.Join(p.config.BaseDir, localPath)

	if err := os.Remove(fullPath); err != nil {
		if os.IsNotExist(err) {
			return true, nil // Already gone
		}
		return false, fmt.Errorf("delete failed: %w", err)
	}

	slog.Debug("[Storage] File deleted", "path", localPath)
	return true, nil
}

// scanOfflineChanges walks the directory and dispatches changed files.
// Matches PeerStorage.scanOfflineChanges in livesync-now.
func (p *StoragePeer) scanOfflineChanges() error {
	slog.Info("[Storage] Scanning for offline changes")

	err := filepath.Walk(p.config.BaseDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip inaccessible files
		}
		if info.IsDir() {
			return nil
		}

		relPath, _ := filepath.Rel(p.config.BaseDir, path)
		relPath = filepath.ToSlash(relPath)

		if p.isIgnored(relPath) {
			return nil
		}

		// Check if file has changed since last seen
		if p.isFileChanged(relPath, info) {
			slog.Debug("[Storage] Offline change detected", "path", relPath)
			content, err := os.ReadFile(path)
			if err != nil {
				return nil
			}

			p.dispatch(p, relPath, &FileData{
				MTime: info.ModTime().UnixMilli(),
				Size:  info.Size(),
				Data:  content,
			})
		}

		return nil
	})

	return err
}

// watchLoop processes filesystem events.
func (p *StoragePeer) watchLoop() {
	for {
		select {
		case <-p.done:
			return
		case event, ok := <-p.watcher.Events:
			if !ok {
				return
			}

			relPath, _ := filepath.Rel(p.config.BaseDir, event.Name)
			relPath = filepath.ToSlash(relPath)

			if p.isIgnored(relPath) {
				continue
			}

			switch {
			case event.Op&fsnotify.Write == fsnotify.Write,
				event.Op&fsnotify.Create == fsnotify.Create:
				p.handleFileChange(relPath)

			case event.Op&fsnotify.Remove == fsnotify.Remove,
				event.Op&fsnotify.Rename == fsnotify.Rename:
				p.dispatch(p, relPath, &FileData{Deleted: true})
			}

		case err, ok := <-p.watcher.Errors:
			if !ok {
				return
			}
			slog.Warn("[Storage] Watcher error", "error", err)
		}
	}
}

// handleFileChange processes a file change event with debouncing.
func (p *StoragePeer) handleFileChange(relPath string) {
	// Simple debounce: wait 250ms then check again
	time.Sleep(250 * time.Millisecond)

	fullPath := filepath.Join(p.config.BaseDir, relPath)
	info, err := os.Stat(fullPath)
	if err != nil {
		return
	}

	content, err := os.ReadFile(fullPath)
	if err != nil {
		return
	}

	// Update stat and dispatch
	p.writeFileStat(relPath, info.ModTime().UnixMilli(), info.Size())

	p.dispatch(p, relPath, &FileData{
		MTime: info.ModTime().UnixMilli(),
		Size:  info.Size(),
		Data:  content,
	})
}

// writeFileStat records a file's mtime and size for change detection.
// Key format: "file-stat-{path}" → "{mtime}-{size}"
// Matches PeerStorage.writeFileStat() in livesync-now.
func (p *StoragePeer) writeFileStat(path string, mtime, size int64) {
	key := "file-stat-" + p.ToLocalPath(path)
	value := fmt.Sprintf("%d-%d", mtime, size)
	p.setSetting(key, value)
}

// isFileChanged checks if a file has changed since last recorded stat.
// Matches PeerStorage.isChanged() in livesync-now.
func (p *StoragePeer) isFileChanged(path string, info os.FileInfo) bool {
	key := "file-stat-" + p.ToLocalPath(path)
	last := p.getSetting(key)
	if last == "" {
		return true // New file
	}

	current := fmt.Sprintf("%d-%d", info.ModTime().UnixMilli(), info.Size())
	return last != current
}

// isIgnored checks if a path should be ignored.
// Matches the hardcoded ignores in PeerStorage.ts plus user-configured patterns.
func (p *StoragePeer) isIgnored(path string) bool {
	// Hardcoded ignores (same as livesync-now PeerStorage.ts)
	hardcoded := []string{
		".livesync.lock",
		".livesync.log",
		".livesync_storage.json",
		"livesync-win.exe",
		"livesync-linux",
		".direnv/",
		".git/",
		"node_modules/",
		".obsidian/",
		".trash/",
		"_sysdb__.sqlite",
		"config.json",
	}

	for _, pattern := range hardcoded {
		if strings.HasSuffix(pattern, "/") {
			dir := strings.TrimSuffix(pattern, "/")
			if path == dir || strings.HasPrefix(path, dir+"/") {
				return true
			}
		} else if path == pattern || strings.HasSuffix(path, "/"+pattern) {
			return true
		}
	}

	// User-configured ignore patterns
	for _, pattern := range p.config.IgnorePatterns {
		if strings.HasSuffix(pattern, "/") {
			dir := strings.TrimSuffix(pattern, "/")
			if path == dir || strings.HasPrefix(path, dir+"/") {
				return true
			}
		} else if path == pattern || strings.HasSuffix(path, "/"+pattern) {
			return true
		}
	}

	return false
}

// addRecursiveWatch adds a watcher for a directory and all its subdirectories.
func (p *StoragePeer) addRecursiveWatch(dir string) error {
	return filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			rel, _ := filepath.Rel(p.config.BaseDir, path)
			rel = filepath.ToSlash(rel)

			// Skip ignored directories
			if rel != "." && p.isIgnored(rel+"/") {
				return filepath.SkipDir
			}

			if err := p.watcher.Add(path); err != nil {
				slog.Warn("[Storage] Failed to watch directory", "dir", path, "error", err)
			}
		}
		return nil
	})
}
