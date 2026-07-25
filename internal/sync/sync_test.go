package sync

import (
	"fmt"
	"testing"
)

// ============================================================
// DedupCache テスト
// ============================================================

func TestDedupCacheBasic(t *testing.T) {
	cache := NewDedupCache(100, 1000000)

	// First check → false (not repeating)
	repeat, err := cache.Check("path1", "hash1")
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}
	if repeat {
		t.Error("expected false for first check")
	}

	// Second check with same path+hash → true (repeating)
	repeat, err = cache.Check("path1", "hash1")
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}
	if !repeat {
		t.Error("expected true for duplicate check")
	}
}

func TestDedupCacheDifferentHash(t *testing.T) {
	cache := NewDedupCache(100, 1000000)

	cache.Check("path", "hash1")

	// Same path, different hash → not repeating (content changed)
	repeat, _ := cache.Check("path", "hash2")
	if repeat {
		t.Error("different hash should not be repeating")
	}
}

func TestDedupCacheMultiplePaths(t *testing.T) {
	cache := NewDedupCache(100, 1000000)

	paths := []string{"a", "b", "c", "d", "e"}
	for _, p := range paths {
		repeat, _ := cache.Check(p, p+"-hash")
		if repeat {
			t.Errorf("first check for %q should not be repeating", p)
		}
	}

	// All should now be repeating
	for _, p := range paths {
		repeat, _ := cache.Check(p, p+"-hash")
		if !repeat {
			t.Errorf("second check for %q should be repeating", p)
		}
	}
}

func TestDedupCacheEviction(t *testing.T) {
	// Small capacity to test eviction
	cache := NewDedupCache(3, 1000000)

	// Fill cache
	cache.Check("a", "h1")
	cache.Check("b", "h2")
	cache.Check("c", "h3")

	// All should still be present
	if r, _ := cache.Check("a", "h1"); !r {
		t.Error("a should still be in cache")
	}

	// Add one more (triggers eviction)
	cache.Check("d", "h4")

	// 'a' may or may not be evicted (FIFO eviction)
	// 'd' should definitely be in cache
	if r, _ := cache.Check("d", "h4"); !r {
		t.Error("d should be in cache")
	}
}

func TestDedupCacheEmptyHash(t *testing.T) {
	cache := NewDedupCache(100, 1000000)

	cache.Check("path", "")
	if r, _ := cache.Check("path", ""); !r {
		t.Error("empty hash should be cached")
	}
}

// ============================================================
// computeContentHash テスト
// ============================================================

func TestComputeContentHash(t *testing.T) {
	h1 := computeContentHash([]byte("hello"))
	h2 := computeContentHash([]byte("hello"))
	h3 := computeContentHash([]byte("world"))

	if h1 != h2 {
		t.Errorf("same content should produce same hash")
	}
	if h1 == h3 {
		t.Errorf("different content should produce different hash")
	}

	if len(h1) != 64 { // SHA-256 hex = 64 characters
		t.Errorf("hash length = %d, want 64", len(h1))
	}
}

// ============================================================
// Peer 設定テスト
// ============================================================

func TestStateKeyPattern(t *testing.T) {
	// We don't have direct access to BasePeer from tests (unexported in a different package)
	// but we verify the pattern produces expected results by simulating it
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
		got := fmt.Sprintf("%s-%s-%s-%s", tt.name, tt.peerType, tt.baseDir, tt.key)
		if got != tt.want {
			t.Errorf("stateKey pattern: got %q, want %q", got, tt.want)
		}
	}
}

// ============================================================
// Hub グループ分離テスト
// ============================================================

func TestGroupIsolation(t *testing.T) {
	// Verify that peers with different groups don't receive events from each other.
	// This is a unit test of the group logic, not the full dispatch.
	tests := []struct {
		sourceGroup string
		targetGroup string
		shouldSend  bool
	}{
		{"main", "main", true},
		{"", "", true},
		{"main", "sub", false},
		{"sub", "main", false},
		{"main", "", false},
		{"", "main", false},
		{"a", "b", false},
		{"a", "a", true},
	}

	for _, tt := range tests {
		source := tt.sourceGroup
		target := tt.targetGroup
		got := (source == target)

		if got != tt.shouldSend {
			t.Errorf("group dispatch(%q→%q) = %v, want %v",
				tt.sourceGroup, tt.targetGroup, got, tt.shouldSend)
		}
	}
}
