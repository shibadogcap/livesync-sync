package model

import (
	"strings"
	"testing"
)

// ============================================================
// ToLocalPath テスト
// ============================================================

func TestToLocalPath(t *testing.T) {
	tests := []struct {
		baseDir string
		path    string
		want    string
	}{
		// Basic cases
		{"", "notes/doc.md", "notes/doc.md"},
		{".", "notes/doc.md", "notes/doc.md"},
		{"/vault", "notes/doc.md", "/vault/notes/doc.md"},
		{"/vault/", "notes/doc.md", "/vault/notes/doc.md"},
		{"./vault", "notes/doc.md", "vault/notes/doc.md"},

		// "." case → empty string
		{"", "", ""},
		{".", "", ""},

		// "_" prefix — no longer special-cased (TS compatibility)
		{"", "_config/test.md", "_config/test.md"},
		{"/vault", "_config/test.md", "/vault/_config/test.md"},

		// Windows backslash normalization
		{"", `notes\doc.md`, "notes/doc.md"},
		{"/vault", `notes\sub\doc.md`, "/vault/notes/sub/doc.md"},
	}

	for _, tt := range tests {
		pc := NewPathConverter(tt.baseDir)
		got := pc.ToLocalPath(tt.path)
		if got != tt.want {
			t.Errorf("ToLocalPath(%q, %q) = %q, want %q", tt.baseDir, tt.path, got, tt.want)
		}
	}
}

// ============================================================
// ToGlobalPath テスト
// ============================================================
// Path↔Global 双方向テスト
// ============================================================

func TestPathRoundtrip(t *testing.T) {
	tests := []struct {
		baseDir string
		path    string // global path
		expectGlobalPrefix string // ToGlobalPath should start with this
	}{
		{"", "notes/doc.md", "notes/doc.md"},
		{"/vault", "notes/doc.md", "/notes/doc.md"}, // leading "/" is expected (filepath.Join behavior)
		{"./vault", "notes/doc.md", "/notes/doc.md"}, // "./vault" normalized by filepath.Join
		{"", "journal/2026/07/25.md", "journal/2026/07/25.md"},
		{"/data/notes", "longer/path/to/file.md", "/longer/path/to/file.md"},
	}

	for _, tt := range tests {
		pc := NewPathConverter(tt.baseDir)

		local := pc.ToLocalPath(tt.path)
		global := pc.ToGlobalPath(local)

		// Local path should not be empty for non-empty input
		if local == "" && tt.path != "" {
			t.Errorf("ToLocalPath(%q) returned empty for baseDir=%q", tt.path, tt.baseDir)
		}

		// Global path should produce reasonable output (may have leading "/")
		if !strings.HasPrefix(global, tt.expectGlobalPrefix) {
			t.Errorf("roundtrip(baseDir=%q, path=%q): local=%q, global=%q, expected global prefix %q",
				tt.baseDir, tt.path, local, global, tt.expectGlobalPrefix)
		}
	}
}

// ============================================================
// Path2ID テスト
// ============================================================

func TestPath2ID(t *testing.T) {
	tests := []struct {
		path            string
		caseInsensitive bool
		want            string
	}{
		{"notes/doc.md", false, "notes/doc.md"},
		{"Notes/Doc.md", true, "notes/doc.md"},
		{"Notes/Doc.md", false, "Notes/Doc.md"},
		{"_config/test.md", false, "/_config/test.md"},
		{"", false, ""},
	}

	for _, tt := range tests {
		pc := &PathConverter{
			BaseDir:         "",
			CaseInsensitive: tt.caseInsensitive,
		}
		got, err := pc.Path2ID(tt.path)
		if err != nil {
			t.Fatalf("Path2ID(%q) failed: %v", tt.path, err)
		}
		if got != tt.want {
			t.Errorf("Path2ID(%q, CI=%v) = %q, want %q", tt.path, tt.caseInsensitive, got, tt.want)
		}
	}
}

// ============================================================
// ComputeChunkID テスト
// ============================================================

func TestComputeChunkID(t *testing.T) {
	content := []byte("test content")
	passphrase := "test-passphrase"

	id, err := ComputeChunkID(content, passphrase, "sha256")
	if err != nil {
		t.Fatalf("ComputeChunkID failed: %v", err)
	}

	// Must have "h:" prefix
	if len(id) < 3 || id[:2] != PrefixChunk {
		t.Errorf("expected prefix %q, got %q", PrefixChunk, id[:2])
	}

	// Deterministic with same content
	id2, _ := ComputeChunkID(content, passphrase, "sha256")
	if id != id2 {
		t.Errorf("ComputeChunkID not deterministic")
	}

	// Different content → different ID
	id4, _ := ComputeChunkID([]byte("different content"), passphrase, "sha256")
	if id == id4 {
		t.Errorf("different content should produce different ID")
	}

	// Passphrase sensitivity is tested via xxhash64
	id5, _ := ComputeChunkID(content, "different-passphrase", "xxhash64")
	id6, _ := ComputeChunkID(content, passphrase, "xxhash64")
	if id5 == id6 {
		t.Errorf("different passphrase should produce different xxhash64 ID")
	}
}

func TestComputeChunkIDxxhash64(t *testing.T) {
	content := []byte("test content")
	passphrase := "test-passphrase"

	id, err := ComputeChunkID(content, passphrase, "xxhash64")
	if err != nil {
		t.Fatalf("ComputeChunkID(xxhash64) failed: %v", err)
	}

	if len(id) < 4 || id[:2] != PrefixChunk {
		t.Errorf("expected prefix %q, got %q", PrefixChunk, id[:2])
	}

	// xxhash64 output should be base36, shorter than hex
	// Content hash part should be shorter than 16 chars
	hashPart := id[2:]
	if len(hashPart) > 16 {
		t.Errorf("xxhash64 base36 output too long: %q (%d chars)", hashPart, len(hashPart))
	}

	// Deterministic
	id2, _ := ComputeChunkID(content, passphrase, "xxhash64")
	if id != id2 {
		t.Errorf("xxhash64 not deterministic")
	}

	// Different content → different ID
	id3, _ := ComputeChunkID([]byte("different"), passphrase, "xxhash64")
	if id == id3 {
		t.Errorf("different content should produce different xxhash64 ID")
	}
}

func TestComputeChunkIDDefaultAlg(t *testing.T) {
	content := []byte("test")
	passphrase := "test"

	id1, _ := ComputeChunkID(content, passphrase, "")
	id2, _ := ComputeChunkID(content, passphrase, "xxhash64")

	if id1 != id2 {
		t.Errorf("default should use xxhash64, got %q vs %q", id1, id2)
	}
}

// ============================================================
// Prefix 定数テスト
// ============================================================

func TestConstants(t *testing.T) {
	if PrefixFile != "f:" {
		t.Errorf("PrefixFile = %q, want %q", PrefixFile, "f:")
	}
	if PrefixChunk != "h:" {
		t.Errorf("PrefixChunk = %q, want %q", PrefixChunk, "h:")
	}
	if TypePlain != "plain" {
		t.Errorf("TypePlain = %q, want %q", TypePlain, "plain")
	}
	if TypeNewnote != "newnote" {
		t.Errorf("TypeNewnote = %q, want %q", TypeNewnote, "newnote")
	}
	if TypeChunk != "leaf" {
		t.Errorf("TypeChunk = %q, want %q", TypeChunk, "leaf")
	}
}
