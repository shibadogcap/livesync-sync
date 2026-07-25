// Package model provides path conversion utilities compatible with obsidian-livesync.
// This implements path2id_base, toLocalPath, toGlobalPath matching livesync-now Peer.ts.
package model

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/cespare/xxhash/v2"
)

// PathConverter handles path conversions between local filesystem and CouchDB document IDs.
// Matches the toLocalPath/toGlobalPath logic in livesync-now's Peer.ts.
//
// Note: Path obfuscation is handled at the crypto layer, not here.
type PathConverter struct {
	BaseDir         string
	CaseInsensitive bool
}

// NewPathConverter creates a new PathConverter.
func NewPathConverter(baseDir string) *PathConverter {
	return &PathConverter{
		BaseDir: baseDir,
	}
}

// ToLocalPath converts a global (CouchDB-relative) path to a local filesystem path.
// Matches Peer.toLocalPath() in livesync-now:
//  1. Normalize backslashes to forward slashes
//  2. Join with baseDir
//  3. If result is ".", return ""
//  4. If path starts with "_", prefix with "/"
func (pc *PathConverter) ToLocalPath(path string) string {
	// Normalize Windows backslashes
	normalized := strings.ReplaceAll(path, "\\", "/")

	// Join with baseDir
	joined := filepath.Join(pc.BaseDir, normalized)

	// Handle "." case
	if joined == "." {
		return ""
	}

	// Handle "_" prefix (internal files)
	if strings.HasPrefix(normalized, "_") && !strings.HasPrefix(joined, "/") {
		joined = "/" + joined
	}

	return joined
}

// ToGlobalPath converts a local filesystem path to a global (CouchDB-relative) path.
// Matches Peer.toGlobalPath() in livesync-now:
//  1. Strip leading "_" from internal files
//  2. Strip baseDir prefix (normalized)
//  3. Normalize backslashes
func (pc *PathConverter) ToGlobalPath(path string) string {
	// Strip leading "_"
	result := path
	if strings.HasPrefix(result, "_") {
		result = result[1:]
	}

	// Strip baseDir prefix (try both configured and normalized versions)
	baseDir := pc.BaseDir
	if strings.HasPrefix(result, baseDir) {
		result = strings.TrimPrefix(result, baseDir)
	} else {
		// Try with cleaned baseDir (e.g., "./vault" → "vault")
		cleaned := filepath.Clean(baseDir)
		if cleaned != baseDir && strings.HasPrefix(result, cleaned) {
			result = strings.TrimPrefix(result, cleaned)
		}
	}

	// Normalize backslashes
	result = strings.ReplaceAll(result, "\\", "/")

	return result
}

// Path2ID converts a file path to a CouchDB document ID (basic, no obfuscation).
// For obfuscated IDs, use the crypto.PathObfuscator after calling this.
func (pc *PathConverter) Path2ID(path string) (string, error) {
	clean := path

	if pc.CaseInsensitive {
		clean = strings.ToLower(clean)
	}

	if strings.HasPrefix(clean, "_") && !strings.HasPrefix(clean, "/_") {
		clean = "/" + clean
	}

	return clean, nil
}

// ComputeChunkID computes a chunk document ID from content.
// Matches obsidian-livesync's xxhash64-based chunk ID generation.
// The chunk ID is "h:" + base36(xxhash64(content + ":" + passphrase + ":" + len(content))).
func ComputeChunkID(content []byte, passphrase string, hashAlg string) (string, error) {
	switch hashAlg {
	case "sha1":
		h := sha256.Sum256(content)
		return PrefixChunk + hex.EncodeToString(h[:20]), nil
	case "xxhash64", "", "xxhash32":
		// Match TS: xxhash.h64(`${piece}-${passphrase}-${piece.length}`).toString(36)
		input := fmt.Sprintf("%s-%s-%d", string(content), passphrase, len(content))
		h := xxhash.Sum64String(input)
		return PrefixChunk + strconv.FormatUint(h, 36), nil
	default:
		h := sha256.Sum256(content)
		return PrefixChunk + hex.EncodeToString(h[:]), nil
	}
}
