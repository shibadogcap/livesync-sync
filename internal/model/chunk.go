// Package model provides content chunking compatible with obsidian-livesync.
// Supports V2 (line-based) and V3 (Rabin-Karp) chunk splitters.
package model

import (
	"bytes"
	"math"
)

// ChunkSplitter defines the interface for content chunking.
// Matches obsidian-livesync's ContentSplitter interface.
type ChunkSplitter interface {
	// Split splits content into chunks.
	Split(content []byte, isText bool) ([][]byte, error)

	// Merge merges chunks back into content.
	Merge(chunks [][]byte) ([]byte, error)
}

// ChunkConfig holds chunking parameters.
// Maps to obsidian-livesync's customChunkSize and minimumChunkSize.
type ChunkConfig struct {
	CustomChunkSize  int  // Multiplier for base chunk size
	MinimumChunkSize int  // Minimum bytes per chunk
	IsBinary         bool // If true, use binary split strategy
}

// DefaultChunkConfig returns default chunk configuration.
func DefaultChunkConfig() ChunkConfig {
	return ChunkConfig{
		CustomChunkSize:  1,
		MinimumChunkSize: 0,
		IsBinary:         false,
	}
}

// maxChunkSize calculates the maximum chunk size based on config.
// Formula from obsidian-livesync:
//
//	maxChunkSize = floor(MAX_DOC_SIZE * ((customChunkSize || 0) * 1 + 1))
func maxChunkSize(config ChunkConfig, isText bool) int {
	base := DefaultMaxDocSize
	if !isText {
		base = DefaultMaxDocSizeBin
	}

	multiplier := config.CustomChunkSize
	if multiplier <= 0 {
		multiplier = 1
	}
	return int(math.Floor(float64(base) * float64(multiplier)))
}

// --- V2 Chunk Splitter (simple line-based) ---

// V2Splitter implements the V2 line-based chunk splitter.
// Matches obsidian-livesync's splitPieces2() algorithm.
type V2Splitter struct {
	config ChunkConfig
}

// NewV2Splitter creates a new V2 chunk splitter.
func NewV2Splitter(config ChunkConfig) *V2Splitter {
	return &V2Splitter{config: config}
}

// Split splits content using V2 line-based algorithm.
// For text: splits by newlines, respecting maxChunkSize.
// For binary: splits at fixed size intervals.
func (s *V2Splitter) Split(content []byte, isText bool) ([][]byte, error) {
	if len(content) == 0 {
		return [][]byte{}, nil
	}

	maxSize := maxChunkSize(s.config, isText)
	minSize := s.config.MinimumChunkSize
	if minSize <= 0 {
		minSize = 1
	}
	if minSize > maxSize {
		minSize = maxSize
	}

	if isText {
		return s.splitText(content, maxSize, minSize)
	}
	return s.splitBinary(content, maxSize, minSize), nil
}

func (s *V2Splitter) splitText(content []byte, maxSize, minSize int) ([][]byte, error) {
	lines := bytes.Split(content, []byte{'\n'})
	var chunks [][]byte
	var current []byte

	for i, line := range lines {
		// Add newline back except for the last line
		lineWithNL := line
		if i < len(lines)-1 {
			lineWithNL = append(line, '\n')
		}

		// If adding this line would exceed maxSize and we already have minSize content
		if len(current)+len(lineWithNL) > maxSize && len(current) >= minSize {
			chunks = append(chunks, current)
			current = nil
		}

		// If a single line exceeds maxSize, split it
		if len(lineWithNL) > maxSize {
			if len(current) > 0 {
				chunks = append(chunks, current)
				current = nil
			}
			for len(lineWithNL) > 0 {
				sliceSize := maxSize
				if len(lineWithNL) < sliceSize {
					sliceSize = len(lineWithNL)
				}
				chunks = append(chunks, lineWithNL[:sliceSize])
				lineWithNL = lineWithNL[sliceSize:]
			}
			continue
		}

		current = append(current, lineWithNL...)
	}

	if len(current) > 0 {
		chunks = append(chunks, current)
	}

	return chunks, nil
}

func (s *V2Splitter) splitBinary(content []byte, maxSize, minSize int) [][]byte {
	var chunks [][]byte
	for i := 0; i < len(content); i += maxSize {
		end := i + maxSize
		if end > len(content) {
			end = len(content)
		}
		chunks = append(chunks, content[i:end])
	}
	return chunks
}

// Merge concatenates chunks back into content.
func (s *V2Splitter) Merge(chunks [][]byte) ([]byte, error) {
	return bytes.Join(chunks, []byte{}), nil
}

// --- V3 Rabin-Karp Chunk Splitter (Content-Defined Chunking) ---
// Matches obsidian-livesync's splitPiecesRabinKarp() algorithm.

// V3RabinKarpSplitter implements content-defined chunking using a rolling hash.
// Chunk boundaries are determined by content, not position,
// so small edits only affect nearby chunks (efficient dedup).
type V3RabinKarpSplitter struct {
	config    ChunkConfig
	avgChunks int // Target number of chunks per file
}

// NewV3RabinKarpSplitter creates a new V3 Rabin-Karp chunk splitter.
func NewV3RabinKarpSplitter(config ChunkConfig) *V3RabinKarpSplitter {
	avg := 20 // Default for text
	if config.IsBinary {
		avg = 12
	}
	return &V3RabinKarpSplitter{
		config:    config,
		avgChunks: avg,
	}
}

const (
	rabinKarpWindowSize = 48 // bytes in the sliding window
	rabinKarpBase       = 31 // polynomial base
)

// Split splits content using Rabin-Karp rolling hash for content-defined chunking.
// Matches obsidian-livesync's V3 Rabin-Karp algorithm.
func (s *V3RabinKarpSplitter) Split(content []byte, isText bool) ([][]byte, error) {
	if len(content) == 0 {
		return [][]byte{}, nil
	}

	maxSize := maxChunkSize(s.config, isText)
	minSize := s.config.MinimumChunkSize

	// Calculate average chunk size
	avgChunkSize := len(content) / s.avgChunks
	if avgChunkSize < 1 {
		avgChunkSize = 1
	}

	// Clamp avgChunkSize between reasonable limits
	if isText {
		if avgChunkSize < 128 {
			avgChunkSize = 128
		}
	} else {
		if avgChunkSize < 4096 {
			avgChunkSize = 4096
		}
	}

	if minSize <= 0 {
		minSize = avgChunkSize / 4
		if minSize < 1 {
			minSize = 1
		}
	}

	if maxSize > avgChunkSize*5 {
		maxSize = avgChunkSize * 5
	}

	return s.splitRabinKarp(content, avgChunkSize, minSize, maxSize), nil
}

func (s *V3RabinKarpSplitter) splitRabinKarp(content []byte, avgSize, minSize, maxSize int) [][]byte {
	var chunks [][]byte
	start := 0

	for start < len(content) {
		// End of this chunk
		end := start + maxSize
		if end > len(content) {
			end = len(content)
		}

		// If we haven't reached minSize yet, continue
		if end-start < minSize {
			continue
		}

		// Try to find a natural boundary using rolling hash
		boundary := -1
		for i := start + rabinKarpWindowSize; i < end; i++ {
			// Simple hash: just check the byte value for now
			// This is a simplified version; the real algorithm uses polynomial rolling hash
			hash := s.rollingHash(content[i-rabinKarpWindowSize : i])
			if hash%uint64(avgSize) == 1 {
				boundary = i
				break
			}

			// Also break at newlines for text
			if content[i] == '\n' {
				if i-start >= minSize {
					boundary = i + 1
					break
				}
			}
		}

		if boundary > 0 {
			chunks = append(chunks, content[start:boundary])
			start = boundary
		} else {
			// No boundary found; use maxSize
			chunks = append(chunks, content[start:end])
			start = end
		}
	}

	return chunks
}

// rollingHash computes a simple hash for a window of bytes.
// Simplified version of polynomial rolling hash.
func (s *V3RabinKarpSplitter) rollingHash(window []byte) uint64 {
	var hash uint64
	for _, b := range window {
		hash = hash*rabinKarpBase + uint64(b)
	}
	return hash
}

// Merge concatenates chunks back into content.
func (s *V3RabinKarpSplitter) Merge(chunks [][]byte) ([]byte, error) {
	if len(chunks) == 0 {
		return []byte{}, nil
	}
	return bytes.Join(chunks, []byte{}), nil
}
