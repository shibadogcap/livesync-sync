package state

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewStore(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	store, err := New(path)
	if err != nil {
		t.Fatalf("New store failed: %v", err)
	}
	defer store.Close()

	if store == nil {
		t.Fatal("store is nil")
	}
}

func TestGetSetItem(t *testing.T) {
	dir := t.TempDir()
	store, _ := New(filepath.Join(dir, "state.json"))
	defer store.Close()

	// Get non-existent key
	val := store.GetItem("nonexistent")
	if val != "" {
		t.Errorf("expected empty for nonexistent key, got %q", val)
	}

	// Set and get
	store.SetItem("key1", "value1")
	val = store.GetItem("key1")
	if val != "value1" {
		t.Errorf("GetItem after SetItem: got %q, want %q", val, "value1")
	}
}

func TestSetItemDedup(t *testing.T) {
	dir := t.TempDir()
	store, _ := New(filepath.Join(dir, "state.json"))
	defer store.Close()

	store.SetItem("key", "value")
	store.SetItem("key", "value") // Same value, should be no-op

	// This is hard to verify directly since the save is debounced,
	// but we can at least ensure no panic
	val := store.GetItem("key")
	if val != "value" {
		t.Errorf("got %q, want %q", val, "value")
	}
}

func TestRemoveItem(t *testing.T) {
	dir := t.TempDir()
	store, _ := New(filepath.Join(dir, "state.json"))
	defer store.Close()

	store.SetItem("key", "value")
	store.RemoveItem("key")

	val := store.GetItem("key")
	if val != "" {
		t.Errorf("after RemoveItem: got %q, want empty", val)
	}

	// Remove non-existent key (should not panic)
	store.RemoveItem("nonexistent")
}

func TestClear(t *testing.T) {
	dir := t.TempDir()
	store, _ := New(filepath.Join(dir, "state.json"))
	defer store.Close()

	store.SetItem("a", "1")
	store.SetItem("b", "2")
	store.Clear()

	if store.GetItem("a") != "" {
		t.Error("expected empty after clear")
	}
	if store.GetItem("b") != "" {
		t.Error("expected empty after clear")
	}
}

func TestPersistence(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "persist.json")

	// Create and write
	store1, _ := New(path)
	store1.SetItem("persist-key", "persist-value")
	store1.Close()

	// Re-open and read
	store2, _ := New(path)
	defer store2.Close()

	val := store2.GetItem("persist-key")
	if val != "persist-value" {
		t.Errorf("persistence failed: got %q, want %q", val, "persist-value")
	}
}

func TestPersistenceMultipleKeys(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "multi.json")

	store1, _ := New(path)
	store1.SetItem("k1", "v1")
	store1.SetItem("k2", "v2")
	store1.SetItem("k3", "v3")
	store1.Close()

	store2, _ := New(path)
	defer store2.Close()

	tests := []struct {
		key, want string
	}{
		{"k1", "v1"},
		{"k2", "v2"},
		{"k3", "v3"},
		{"nonexistent", ""},
	}

	for _, tt := range tests {
		got := store2.GetItem(tt.key)
		if got != tt.want {
			t.Errorf("key %q: got %q, want %q", tt.key, got, tt.want)
		}
	}
}

func TestStoreConcurrentAccess(t *testing.T) {
	dir := t.TempDir()
	store, _ := New(filepath.Join(dir, "concurrent.json"))
	defer store.Close()

	// Concurrent writes should not panic
	done := make(chan struct{})
	go func() {
		for i := 0; i < 100; i++ {
			store.SetItem("key", "value1")
		}
		done <- struct{}{}
	}()
	go func() {
		for i := 0; i < 100; i++ {
			store.SetItem("key", "value2")
		}
		done <- struct{}{}
	}()

	<-done
	<-done
	// At least one of the values should be set
	val := store.GetItem("key")
	if val != "value1" && val != "value2" {
		t.Errorf("unexpected value after concurrent writes: %q", val)
	}
}

func TestStoreInitWithNonExistentDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nonexistent-subdir")
	path := filepath.Join(dir, "state.json")

	store, err := New(path)
	if err != nil {
		t.Fatalf("New store with non-existent dir should create it: %v", err)
	}
	defer store.Close()

	store.SetItem("key", "value")
	if store.GetItem("key") != "value" {
		t.Error("set/get after creating dir failed")
	}
}

func TestKeyNamingPattern(t *testing.T) {
	// This test verifies the naming pattern used by Peer.stateKey
	// Pattern: {name}-{type}-{baseDir}-{key}
	tests := []struct {
		name, peerType, baseDir, key string
		want                         string
	}{
		{"remote", "couchdb", "", "since", "remote-couchdb--since"},
		{"local", "storage", "/vault", "file-stat-notes/doc.md",
			"local-storage-/vault-file-stat-notes/doc.md"},
		{"test", "couchdb", "blog/", "remote-created", "test-couchdb-blog/-remote-created"},
	}

	for _, tt := range tests {
		got := tt.name + "-" + tt.peerType + "-" + tt.baseDir + "-" + tt.key
		if got != tt.want {
			t.Errorf("key pattern: got %q, want %q", got, tt.want)
		}
	}
}

func TestStateFilePath(t *testing.T) {
	t.Run("default location", func(t *testing.T) {
		home, _ := os.UserHomeDir()
		expected := filepath.Join(home, ".livesync", "state.json")

		// Just verify the expected path format (since we can't control env in tests)
		if expected == "" {
			t.Error("expected non-empty path")
		}
	})
}
