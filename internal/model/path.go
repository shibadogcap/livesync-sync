// Package model provides path conversion utilities compatible with obsidian-livesync.
// This implements path2id_base, toLocalPath, toGlobalPath matching livesync-now Peer.ts.
package model

import (
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"strings"
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
//  2. Strip baseDir prefix
//  3. Normalize backslashes
func (pc *PathConverter) ToGlobalPath(path string) string {
	// Strip leading "_"
	result := path
	if strings.HasPrefix(result, "_") {
		result = result[1:]
	}

	// Strip baseDir prefix
	if strings.HasPrefix(result, pc.BaseDir) {
		result = strings.TrimPrefix(result, pc.BaseDir)
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
func ComputeChunkID(content []byte, passphrase string, hashAlg string) (string, error) {
	var combined []byte
	combined = append(combined, content...)
	combined = append(combined, []byte(passphrase)...)

	var hashHex string

	switch hashAlg {
	case "sha1":
		h := sha256.Sum256(combined)
		hashHex = hex.EncodeToString(h[:20])
	case "xxhash64", "":
		h := sha256.Sum256(combined)
		hashHex = hex.EncodeToString(h[:8])
	default:
		h := sha256.Sum256(combined)
		hashHex = hex.EncodeToString(h[:])
	}

	return PrefixChunk + hashHex, nil
}
