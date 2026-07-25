// Package sync implements the synchronization engine.
// Architecture follows livesync-now (vrtmrz/livesync-bridge):
//   Hub (dispatcher) → Peers (CouchDB, Storage)
package sync

import (
	"bytes"
	"container/list"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync"

	"github.com/user/livesync-sync/internal/config"
	"github.com/user/livesync-sync/internal/model"
	"github.com/user/livesync-sync/internal/state"
)

// FileData represents a file's content and metadata.
// Alias for model.FileData for convenience.
type FileData = model.FileData

// Peer defines the abstract interface for a synchronization peer.
// Matches the abstract class Peer in livesync-now's Peer.ts.
type Peer interface {
	// Name returns the peer's name (identifier).
	Name() string

	// Start initializes the peer and begins watching for changes.
	Start() error

	// Stop gracefully stops the peer.
	Stop() error

	// Put stores data at the given path.
	Put(path string, data FileData) (bool, error)

	// Get retrieves data from the given path.
	Get(path string) (*FileData, error)

	// Delete removes the file at the given path.
	Delete(path string) (bool, error)

	// Group returns the peer's group for dispatch filtering.
	Group() string

	// IsRepeating checks if this change has already been processed (dedup).
	IsRepeating(path string, data *FileData) (bool, error)
}

// DispatchFunc is the callback type for dispatching changes to the Hub.
type DispatchFunc func(source Peer, path string, data *FileData)

// BasePeer provides shared functionality for all peer implementations.
// This maps to the abstract Peer class in livesync-now.
type BasePeer struct {
	config     config.PeerConf
	store      *state.Store
	dispatch   DispatchFunc
	dedupCache *DedupCache
}

// NewBasePeer creates a new BasePeer.
func NewBasePeer(conf config.PeerConf, store *state.Store, dispatch DispatchFunc) BasePeer {
	return BasePeer{
		config:     conf,
		store:      store,
		dispatch:   dispatch,
		dedupCache: NewDedupCache(5000, 1000000000),
	}
}

// Name returns the peer's name.
func (bp *BasePeer) Name() string {
	return bp.config.Name
}

// Group returns the peer's group.
func (bp *BasePeer) Group() string {
	return bp.config.Group
}

// stateKey builds a namespaced key for state storage.
// Pattern: "{name}-{type}-{baseDir}-{key}" (same as livesync-now Peer._getKey).
func (bp *BasePeer) stateKey(key string) string {
	return fmt.Sprintf("%s-%s-%s-%s", bp.config.Name, bp.config.Type, bp.config.BaseDir, key)
}

// getSetting retrieves a setting from the state store.
func (bp *BasePeer) getSetting(key string) string {
	return bp.store.GetItem(bp.stateKey(key))
}

// setSetting saves a setting to the state store.
func (bp *BasePeer) setSetting(key, value string) {
	bp.store.SetItem(bp.stateKey(key), value)
}

// ToLocalPath converts a global (CouchDB-relative) path to a local filesystem path.
// Matches Peer.toLocalPath() in livesync-now.
func (bp *BasePeer) ToLocalPath(path string) string {
	pc := model.NewPathConverter(bp.config.BaseDir)
	return pc.ToLocalPath(path)
}

// ToGlobalPath converts a local path to a global (CouchDB-relative) path.
// Matches Peer.toGlobalPath() in livesync-now.
func (bp *BasePeer) ToGlobalPath(path string) string {
	pc := model.NewPathConverter(bp.config.BaseDir)
	return pc.ToGlobalPath(path)
}

// IsRepeating checks if this path+data combination has already been processed.
// Uses a content hash for dedup (SHA-256).
// Matches livesync-now Peer.isRepeating().
func (bp *BasePeer) IsRepeating(path string, data *FileData) (bool, error) {
	var hashStr string
	if data == nil {
		// Deletion marker — match TS: computeHash(["\u0001Deleted"])
		h := sha256.Sum256([]byte("\u0001Deleted"))
		hashStr = hex.EncodeToString(h[:])
	} else {
		hashStr = computeContentHash(data.Data)
	}

	return bp.dedupCache.Check(path, hashStr)
}

// computeContentHash computes a SHA-256 hash of content for dedup.
// Normalizes CRLF to LF before hashing (matching TS computeHash behavior)
// so cross-platform sync works correctly.
func computeContentHash(data []byte) string {
	// Normalize \r\n to \n (TS: data.join("").replace(/\r\n/g, "\n"))
	normalized := bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n"))
	h := sha256.Sum256(normalized)
	return hex.EncodeToString(h[:])
}

// DedupCache is a proper LRU cache for deduplication.
// Matches livesync-now's LRUCache<string, string>(5000, 1000000000, true).
type DedupCache struct {
	mu       sync.Mutex
	capacity int
	maxSize  int64
	current  int64
	items    map[string]*list.Element
	order    *list.List // Doubly-linked list for true LRU ordering
}

type cacheEntry struct {
	key   string
	value string
	size  int64
}

// NewDedupCache creates a new LRU dedup cache.
func NewDedupCache(capacity int, maxSize int64) *DedupCache {
	return &DedupCache{
		capacity: capacity,
		maxSize:  maxSize,
		items:    make(map[string]*list.Element),
		order:    list.New(),
	}
}

// Check returns true if the path+hash combination already exists (is a repeat).
// If new, stores it. If the path exists with a different hash, updates it.
func (dc *DedupCache) Check(path, hash string) (bool, error) {
	dc.mu.Lock()
	defer dc.mu.Unlock()

	if el, exists := dc.items[path]; exists {
		entry := el.Value.(*cacheEntry)
		if entry.value == hash {
			// Same hash — move to front (LRU promotion) and return true
			dc.order.MoveToFront(el)
			return true, nil
		}
		// Different hash — update value and move to front
		dc.current -= entry.size
		entry.value = hash
		entry.size = int64(len(path) + len(hash))
		dc.current += entry.size
		dc.order.MoveToFront(el)
		return false, nil
	}

	// Calculate size for new entry
	size := int64(len(path) + len(hash))

	// Evict while over capacity or over maxSize (use > not >= to avoid constant eviction at boundary)
	for dc.order.Len() > dc.capacity || (dc.maxSize > 0 && dc.current+size > dc.maxSize) {
		dc.evict()
	}

	// Store new entry at front
	entry := &cacheEntry{key: path, value: hash, size: size}
	el := dc.order.PushFront(entry)
	dc.items[path] = el
	dc.current += size

	return false, nil
}

func (dc *DedupCache) evict() {
	el := dc.order.Back()
	if el == nil {
		return
	}
	entry := dc.order.Remove(el).(*cacheEntry)
	dc.current -= entry.size
	delete(dc.items, entry.key)
}
