// Package model defines the core data types for livesync-sync.
// These types map directly to obsidian-livesync's CouchDB document structures.
package model

// FileData represents a file's content and metadata.
// Matches livesync-now's FileData type.
type FileData struct {
	CTime   int64  `json:"ctime"`
	MTime   int64  `json:"mtime"`
	Size    int64  `json:"size"`
	Data    []byte `json:"data,omitempty"`
	Deleted bool   `json:"deleted,omitempty"`
}

// FileEntry is the metadata document stored in CouchDB (f: prefix).
// Matches obsidian-livesync's NewEntry / PlainEntry.
// For "newnote" (binary) entries, Data contains the inline content.
// For "plain" (text) entries, content is in Children chunks.
type FileEntry struct {
	ID       string                `json:"_id,omitempty"`
	Rev      string                `json:"_rev,omitempty"`
	Path     string                `json:"path"`
	CTime    int64                 `json:"ctime"`
	MTime    int64                 `json:"mtime"`
	Size     int64                 `json:"size"`
	Type     string                `json:"type"`             // "plain" | "newnote"
	Children []string              `json:"children,omitempty"` // Chunk document IDs
	Data     *string               `json:"data,omitempty"`     // Inline content (newnote/binary)
	Deleted  bool                  `json:"deleted,omitempty"`
	DeletedFromPlugin bool         `json:"_deleted,omitempty"` // TS sets _deleted for deletes
	Eden     map[string]EdenChunk  `json:"eden,omitempty"`
}

// ChunkEntry is a chunk document stored in CouchDB (h: prefix).
// Matches obsidian-livesync's EntryLeaf.
type ChunkEntry struct {
	ID   string `json:"_id,omitempty"`
	Rev  string `json:"_rev,omitempty"`
	Type string `json:"type"`             // "leaf"
	Data string `json:"data"`             // Encrypted content, base64 or prefixed format
}

// EdenChunk is an inline chunk within a FileEntry (obsolete/not recommended).
type EdenChunk struct {
	Data  string `json:"data"`
	Epoch int64  `json:"epoch"`
}

// SyncParameters is stored as _local/obsidian_livesync_sync_parameters.
type SyncParameters struct {
	ID              string `json:"_id"`              // "_local/obsidian_livesync_sync_parameters"
	Rev             string `json:"_rev,omitempty"`
	Type            string `json:"type"`             // "sync-parameters"
	ProtocolVersion string `json:"protocolVersion"`  // "advanced-e2ee"
	PBKDF2Salt      string `json:"pbkdf2salt"`       // 32 bytes, base64 encoded
}

// MilestoneDoc is stored as _local/obsydian_livesync_milestone.
type MilestoneDoc struct {
	ID          string                  `json:"_id"`   // "_local/obsydian_livesync_milestone"
	Rev         string                  `json:"_rev,omitempty"`
	Created     int64                   `json:"created"`
	NodeVersion map[string]interface{}  `json:"node_version"`
	TweakValues map[string]TweakValues  `json:"tweak_values"`
}

// TweakValues holds the remote database configuration overrides.
// Applied via useRemoteTweaks to ensure cross-client consistency.
type TweakValues struct {
	Encrypt                        bool   `json:"encrypt"`
	UsePathObfuscation             bool   `json:"usePathObfuscation"`
	CustomChunkSize                *int   `json:"customChunkSize,omitempty"`
	MinimumChunkSize               *int   `json:"minimumChunkSize,omitempty"`
	EnableCompression              bool   `json:"enableCompression"`
	HashAlg                        string `json:"hashAlg"`
	E2EEAlgorithm                  string `json:"E2EEAlgorithm"`
	UseEden                        bool   `json:"useEden"`
	MaxAgeInEden                   *int   `json:"maxAgeInEden,omitempty"`
	MaxTotalLengthInEden           *int   `json:"maxTotalLengthInEden,omitempty"`
	MaxChunksInEden                *int   `json:"maxChunksInEden,omitempty"`
	EnableChunkSplitterV2          bool   `json:"enableChunkSplitterV2"`
	ChunkSplitterVersion           string `json:"chunkSplitterVersion"`
	UseDynamicIterationCount       bool   `json:"useDynamicIterationCount"`
	DoNotUseFixedRevisionForChunks bool   `json:"doNotUseFixedRevisionForChunks"`
	HandleFilenameCaseSensitive    bool   `json:"handleFilenameCaseSensitive"`
}

// Constants matching obsidian-livesync
const (
	// Document ID prefixes
	PrefixFile   = "f:"
	PrefixChunk  = "h:"
	PrefixChunkE = "h:+" // Encrypted chunk (legacy)

	// File entry types
	TypePlain   = "plain"
	TypeNewnote = "newnote"
	TypeChunk   = "leaf"

	// Special document IDs
	MilestoneDocID   = "_local/obsydian_livesync_milestone"
	SyncParamsDocID  = "_local/obsidian_livesync_sync_parameters"
	VersionDocID     = "obsydian_livesync_version"

	// Protocol versions
	ProtocolAdvancedE2EE = "advanced-e2ee"

	// Encryption prefixes (matching octagonal-wheels)
	EncV1Prefix   = "%"
	EncV2Prefix   = "%="
	EncV2SaltPref = "%$"
	EncV3Prefix   = "%~"
	ObfV2Prefix   = "%/\\"

	// Default chunk sizes
	DefaultMaxDocSize    = 1000   // Text chunks
	DefaultMaxDocSizeBin = 102400 // Binary chunks

	// Eden
	DefaultUseEden = false

	// Versioning
	CurrentSettingVersion = 10
	DBProtocolVersion     = 12

	// Default hash algorithm
	DefaultHashAlg = "xxhash64"
	// Default chunk splitter
	DefaultChunkSplitterVersion = "v3-rabin-karp"
	// Default E2EE algorithm
	DefaultE2EEAlgorithm = "v2"
)

// CurrentHashAlg tracks the active hash algorithm (may be overridden by remote tweaks)
var CurrentHashAlg = DefaultHashAlg

// SetHashAlg updates the active hash algorithm for chunk ID computation.
func SetHashAlg(alg string) {
	if alg != "" {
		CurrentHashAlg = alg
	}
}
