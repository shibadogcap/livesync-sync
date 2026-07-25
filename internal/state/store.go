// Package state provides a JSON-file-backed key-value store.
// This is a direct port of livesync-now's Storage.ts (StorageShim class).
// Same persistence pattern: debounced writes, dedup by value, atomic saves.
package state

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Store is a JSON-file-backed key-value store.
// Pattern matches livesync-now's Storage.ts:
//   - Keys: "{name}-{type}-{baseDir}-{key}" (managed by caller)
//   - Values: arbitrary strings
//   - Persisted to a JSON file with debounced writes
type Store struct {
	mu       sync.RWMutex
	data     map[string]string
	filePath string

	saveTimer *time.Timer
	isSaving  bool
	needsSave bool
	done      chan struct{}
}

// New creates a new Store backed by the given file path.
// If the file exists, it's loaded. If not, an empty store is created.
func New(filePath string) (*Store, error) {
	s := &Store{
		data:     make(map[string]string),
		filePath: filePath,
		done:     make(chan struct{}),
	}

	// Ensure directory exists
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}

	// Try to load existing state
	data, err := os.ReadFile(filePath)
	if err == nil {
		if err := json.Unmarshal(data, &s.data); err != nil {
			// Corrupted file, start fresh
			s.data = make(map[string]string)
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}

	return s, nil
}

// GetItem returns the value for a key, or empty string if not found.
// Matches livesync-now StorageShim.getItem().
func (s *Store) GetItem(key string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.data[key]
}

// SetItem sets a value for a key.
// Skips write if the value hasn't changed (same dedup as livesync-now).
// Triggers a debounced save.
func (s *Store) SetItem(key, value string) {
	s.mu.Lock()
	if s.data[key] == value {
		s.mu.Unlock()
		return
	}
	s.data[key] = value
	s.mu.Unlock()
	s.scheduleSave()
}

// RemoveItem deletes a key from the store.
func (s *Store) RemoveItem(key string) {
	s.mu.Lock()
	if _, exists := s.data[key]; !exists {
		s.mu.Unlock()
		return
	}
	delete(s.data, key)
	s.mu.Unlock()
	s.scheduleSave()
}

// Clear removes all data and immediately saves.
func (s *Store) Clear() error {
	s.mu.Lock()
	s.data = make(map[string]string)
	s.mu.Unlock()
	return s.save()
}

// Close stops the save timer and ensures final save.
func (s *Store) Close() error {
	if s.saveTimer != nil {
		s.saveTimer.Stop()
	}
	close(s.done)
	return s.save()
}

// scheduleSave triggers a debounced save (1 second delay, like livesync-now).
func (s *Store) scheduleSave() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.saveTimer != nil {
		s.saveTimer.Stop()
	}
	s.saveTimer = time.AfterFunc(1*time.Second, func() {
		s.mu.Lock()
		s.mu.Unlock()
		// save must be called outside the lock
		go func() {
			if err := s.save(); err != nil {
				// Log but don't fail
			}
		}()
	})
}

// save persists the data to disk.
// Implements the same isSaving/needsSave pattern as livesync-now.
func (s *Store) save() error {
	s.mu.Lock()
	if s.isSaving {
		s.needsSave = true
		s.mu.Unlock()
		return nil
	}
	s.isSaving = true
	s.mu.Unlock()

	// Write to temp file first for atomicity
	tmpPath := s.filePath + ".tmp"
	data, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		s.mu.Lock()
		s.isSaving = false
		s.mu.Unlock()
		return err
	}

	if err := os.WriteFile(tmpPath, data, 0600); err != nil {
		s.mu.Lock()
		s.isSaving = false
		s.mu.Unlock()
		return err
	}
	if err := os.Rename(tmpPath, s.filePath); err != nil {
		s.mu.Lock()
		s.isSaving = false
		s.mu.Unlock()
		return err
	}

	s.mu.Lock()
	s.isSaving = false
	if s.needsSave {
		s.needsSave = false
		s.mu.Unlock()
		return s.save()
	}
	s.mu.Unlock()
	return nil
}
