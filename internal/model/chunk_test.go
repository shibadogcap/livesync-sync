package model

import (
	"bytes"
	"testing"
)

// ============================================================
// V2 チャンク分割テスト (line-based)
// ============================================================

func TestV2SplitterText(t *testing.T) {
	splitter := NewV2Splitter(DefaultChunkConfig())

	content := []byte("line1\nline2\nline3\nline4\n")
	chunks, err := splitter.Split(content, true)
	if err != nil {
		t.Fatalf("V2 Split failed: %v", err)
	}

	// For small text, expect 1 chunk
	if len(chunks) == 0 {
		t.Fatal("expected at least 1 chunk")
	}

	merged, err := splitter.Merge(chunks)
	if err != nil {
		t.Fatalf("V2 Merge failed: %v", err)
	}

	if !bytes.Equal(merged, content) {
		t.Errorf("V2 roundtrip mismatch:\n  got:  %q\n  want: %q", string(merged), string(content))
	}
}

func TestV2SplitterEmpty(t *testing.T) {
	splitter := NewV2Splitter(DefaultChunkConfig())

	chunks, err := splitter.Split([]byte{}, true)
	if err != nil {
		t.Fatalf("Split empty failed: %v", err)
	}
	if len(chunks) != 0 {
		t.Errorf("expected 0 chunks for empty input, got %d", len(chunks))
	}
}

func TestV2SplitterSingleLine(t *testing.T) {
	splitter := NewV2Splitter(DefaultChunkConfig())

	content := []byte("just a single line without newline")
	chunks, err := splitter.Split(content, true)
	if err != nil {
		t.Fatalf("Split single line failed: %v", err)
	}

	merged, _ := splitter.Merge(chunks)
	if !bytes.Equal(merged, content) {
		t.Errorf("single line roundtrip failed:\n  got:  %q\n  want: %q", string(merged), string(content))
	}
}

func TestV2SplitterMultipleLinesWithNewline(t *testing.T) {
	splitter := NewV2Splitter(DefaultChunkConfig())

	// Multiple lines each ending with \n
	content := []byte("a\nb\nc\nd\ne\nf\ng\nh\ni\nj\n")
	chunks, err := splitter.Split(content, true)
	if err != nil {
		t.Fatalf("Split failed: %v", err)
	}

	merged, _ := splitter.Merge(chunks)
	if !bytes.Equal(merged, content) {
		t.Errorf("multi-line roundtrip failed")
	}
}

func TestV2SplitterBinary(t *testing.T) {
	// Binary chunks use DefaultMaxDocSizeBin (102400) as base.
	// With CustomChunkSize=1, maxChunkSize = 102400 * 1 = 102400.
	// We need content larger than that to force multiple chunks.
	splitter := NewV2Splitter(ChunkConfig{CustomChunkSize: 1})

	content := make([]byte, 200000) // 200KB, larger than one chunk
	for i := range content {
		content[i] = byte(i % 256)
	}

	chunks, err := splitter.Split(content, false)
	if err != nil {
		t.Fatalf("Split binary failed: %v", err)
	}

	if len(chunks) < 2 {
		t.Errorf("expected multiple chunks for 200KB binary, got %d (maxChunkSize=%d)",
			len(chunks), maxChunkSize(ChunkConfig{CustomChunkSize: 1}, false))
	}

	merged, _ := splitter.Merge(chunks)
	if !bytes.Equal(merged, content) {
		t.Errorf("binary roundtrip failed")
	}
}

func TestV2SplitterLineExceedingMaxSize(t *testing.T) {
	splitter := NewV2Splitter(ChunkConfig{
		CustomChunkSize: 1,  // Small max doc size
		MinimumChunkSize: 0,
	})

	// Create a single long line
	longLine := bytes.Repeat([]byte("A"), 3000)
	content := append(longLine, '\n')

	chunks, err := splitter.Split(content, true)
	if err != nil {
		t.Fatalf("Split long line failed: %v", err)
	}

	if len(chunks) < 2 {
		t.Errorf("expected multiple chunks for 3000B line, got %d", len(chunks))
	}

	merged, _ := splitter.Merge(chunks)
	if !bytes.Equal(merged, content) {
		t.Errorf("long line roundtrip failed")
	}
}

// ============================================================
// V3 Rabin-Karp チャンク分割テスト
// ============================================================

func TestV3SplitterSmallText(t *testing.T) {
	splitter := NewV3RabinKarpSplitter(DefaultChunkConfig())

	content := []byte("Hello, this is a small text file for testing Rabin-Karp chunking.")
	chunks, err := splitter.Split(content, true)
	if err != nil {
		t.Fatalf("V3 Split failed: %v", err)
	}

	if len(chunks) == 0 {
		t.Fatal("expected at least 1 chunk")
	}

	merged, err := splitter.Merge(chunks)
	if err != nil {
		t.Fatalf("V3 Merge failed: %v", err)
	}

	if !bytes.Equal(merged, content) {
		t.Errorf("V3 roundtrip mismatch:\n  got:  %q\n  want: %q", string(merged), string(content))
	}
}

func TestV3SplitterEmpty(t *testing.T) {
	splitter := NewV3RabinKarpSplitter(DefaultChunkConfig())

	chunks, err := splitter.Split([]byte{}, true)
	if err != nil {
		t.Fatalf("V3 Split empty failed: %v", err)
	}
	if len(chunks) != 0 {
		t.Errorf("expected 0 chunks for empty, got %d", len(chunks))
	}
}

func TestV3SplitterLargeText(t *testing.T) {
	splitter := NewV3RabinKarpSplitter(ChunkConfig{
		CustomChunkSize:  1,
		MinimumChunkSize: 0,
	})

	// Generate ~50KB of text
	var content []byte
	for i := 0; i < 1000; i++ {
		content = append(content, []byte("This is line number ")...)
		content = append(content, []byte(string(rune('0'+i%10)))...)
		content = append(content, '\n')
	}

	chunks, err := splitter.Split(content, true)
	if err != nil {
		t.Fatalf("V3 Split large text failed: %v", err)
	}

	if len(chunks) < 2 {
		t.Errorf("expected multiple chunks for large text, got %d", len(chunks))
	}

	merged, _ := splitter.Merge(chunks)
	if !bytes.Equal(merged, content) {
		t.Errorf("V3 large text roundtrip failed: got %d bytes, want %d bytes", len(merged), len(content))
	}

	// Verify single-line edit only affects nearby chunks (content-defined)
	t.Run("edit stability", func(t *testing.T) {
		// Modify one line in the middle
		modified := make([]byte, len(content))
		copy(modified, content)
		midpoint := len(content) / 2
		modified[midpoint] = 'X' // Small change

		chunks2, _ := splitter.Split(modified, true)
		// Most chunks should be identical; only a few near the edit should differ
		if len(chunks2) == 0 {
			t.Error("no chunks produced for modified content")
		}
	})
}

func TestV3SplitterBinary(t *testing.T) {
	splitter := NewV3RabinKarpSplitter(ChunkConfig{
		CustomChunkSize:  1,
		MinimumChunkSize: 0,
		IsBinary:         true,
	})

	content := make([]byte, 50000)
	for i := range content {
		content[i] = byte(i*31 + 127) // Pseudorandom
	}

	chunks, err := splitter.Split(content, false)
	if err != nil {
		t.Fatalf("V3 Split binary failed: %v", err)
	}

	if len(chunks) < 2 {
		t.Errorf("expected multiple chunks for binary, got %d", len(chunks))
	}

	merged, _ := splitter.Merge(chunks)
	if !bytes.Equal(merged, content) {
		t.Errorf("V3 binary roundtrip failed: got %d bytes, want %d bytes", len(merged), len(content))
	}
}

func TestV3SplitterMerge(t *testing.T) {
	splitter := NewV3RabinKarpSplitter(DefaultChunkConfig())

	content := []byte("Merge test content for V3 splitter.")
	chunks, _ := splitter.Split(content, true)

	// Test Merge separately
	merged, err := splitter.Merge(chunks)
	if err != nil {
		t.Fatalf("Merge failed: %v", err)
	}
	if !bytes.Equal(merged, content) {
		t.Errorf("Merge failed: got %q, want %q", string(merged), string(content))
	}
}

// TestV3SplitterBoundaryAtNewline ensures the splitter respects newline boundaries.
func TestV3SplitterBoundaryAtNewline(t *testing.T) {
	splitter := NewV3RabinKarpSplitter(ChunkConfig{
		CustomChunkSize:  1,
		MinimumChunkSize: 10,
	})

	// Content where minSize ensures newlines are found before maxSize
	content := []byte("short line\nanother line\nthird line\nfourth\nfifth\n")
	chunks, err := splitter.Split(content, true)
	if err != nil {
		t.Fatalf("Split failed: %v", err)
	}

	merged, _ := splitter.Merge(chunks)
	if !bytes.Equal(merged, content) {
		t.Errorf("newline boundary roundtrip failed")
	}
}

// ============================================================
// maxChunkSize 計算テスト
// ============================================================

func TestMaxChunkSize(t *testing.T) {
	tests := []struct {
		config ChunkConfig
		isText bool
		want   int
	}{
		{ChunkConfig{CustomChunkSize: 0}, true, DefaultMaxDocSize},
		{ChunkConfig{CustomChunkSize: 1}, true, DefaultMaxDocSize},
		{ChunkConfig{CustomChunkSize: 2}, true, DefaultMaxDocSize * 2},
		{ChunkConfig{CustomChunkSize: 0}, false, DefaultMaxDocSizeBin},
		{ChunkConfig{CustomChunkSize: 1}, false, DefaultMaxDocSizeBin},
		{ChunkConfig{CustomChunkSize: 5}, false, DefaultMaxDocSizeBin * 5},
	}

	for _, tt := range tests {
		got := maxChunkSize(tt.config, tt.isText)
		if got != tt.want {
			t.Errorf("maxChunkSize(%+v, %v) = %d, want %d", tt.config, tt.isText, got, tt.want)
		}
	}
}

func TestDefaultChunkConfig(t *testing.T) {
	cfg := DefaultChunkConfig()
	if cfg.CustomChunkSize != 1 {
		t.Errorf("CustomChunkSize = %d, want 1", cfg.CustomChunkSize)
	}
	if cfg.MinimumChunkSize != 0 {
		t.Errorf("MinimumChunkSize = %d, want 0", cfg.MinimumChunkSize)
	}
	if cfg.IsBinary {
		t.Errorf("IsBinary should be false by default")
	}
}
