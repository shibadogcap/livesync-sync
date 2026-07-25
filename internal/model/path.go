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
// Matches Peer.toLocalPath() in livesync-now.
// NOTE: Do NOT special-case leading "_". The commonlib path2id_base/id2path_base
// already handle "_". Adding special handling here causes double-mangling.
func (pc *PathConverter) ToLocalPath(path string) string {
	// Normalize Windows backslashes
	normalized := strings.ReplaceAll(path, "\\", "/")

	// Join with baseDir
	joined := filepath.Join(pc.BaseDir, normalized)

	// Handle "." case
	if joined == "." {
		return ""
	}

	return joined
}

// ToGlobalPath converts a local filesystem path to a global (CouchDB-relative) path.
// Matches Peer.toGlobalPath() in livesync-now.
// Does NOT strip "_" prefix; that is handled by path2id_base/id2path_base.
func (pc *PathConverter) ToGlobalPath(path string) string {
	result := path

	// Strip baseDir prefix (try both configured and normalized versions)
	baseDir := pc.BaseDir
	if strings.HasPrefix(result, baseDir) {
		result = strings.TrimPrefix(result, baseDir)
	} else {
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
