package sync

import (
	"log/slog"
	"sync"

	"github.com/user/livesync-sync/internal/config"
	"github.com/user/livesync-sync/internal/state"
)

// Hub is the central dispatcher for the synchronization engine.
// Matches livesync-now Hub.ts.
//
// Architecture:
//   - Maintains a list of peers
//   - Dispatches changes from one peer to all other peers in the same group
//   - Controls concurrency (max 10 active tasks, semaphore pattern)
type Hub struct {
	conf       *config.FullConfig
	store      *state.Store
	peers      []Peer

	// Concurrency control (matches livesync-now: maxConcurrency=10)
	activeTasks    int
	maxConcurrency int
	taskQueue      []func()
	mu             sync.Mutex
	cond           *sync.Cond
}

// NewHub creates a new Hub.
func NewHub(conf *config.FullConfig, store *state.Store) *Hub {
	h := &Hub{
		conf:           conf,
		store:          store,
		peers:          make([]Peer, 0),
		maxConcurrency: 10,
	}
	h.cond = sync.NewCond(&h.mu)
	return h
}

// Start initializes all peers and begins synchronization.
// Startup order (matching livesync-now):
//  1. CouchDB peers start first (await all)
//  2. Storage peers start after
func (h *Hub) Start() error {
	slog.Info("[Hub] Starting...")

	// Separate peers by type
	var couchdbPeers []*CouchDBPeer
	var storagePeers []*StoragePeer

	for i := range h.conf.Sync.Peers {
		peer := &h.conf.Sync.Peers[i]
		switch {
		case config.IsCouchDBPeer(peer):
			cp := NewCouchDBPeer(*peer, h.store, h.dispatch)
			h.peers = append(h.peers, cp)
			couchdbPeers = append(couchdbPeers, cp)

		case config.IsStoragePeer(peer):
			sp := NewStoragePeer(*peer, h.store, h.dispatch)
			h.peers = append(h.peers, sp)
			storagePeers = append(storagePeers, sp)

		default:
			slog.Warn("[Hub] Unknown peer type", "type", peer.Type, "name", peer.Name)
		}
	}

	// Start CouchDB peers first (sequential)
	slog.Info("[Hub] Starting CouchDB peers...")
	for _, cp := range couchdbPeers {
		if err := cp.Start(); err != nil {
			slog.Error("[Hub] CouchDB peer start failed", "name", cp.Name(), "error", err)
			return err
		}
	}

	// Then start storage peers
	slog.Info("[Hub] Starting storage peers...")
	for _, sp := range storagePeers {
		if err := sp.Start(); err != nil {
			slog.Error("[Hub] Storage peer start failed", "name", sp.Name(), "error", err)
			return err
		}
	}

	slog.Info("[Hub] All peers started")
	return nil
}

// Stop gracefully stops all peers.
func (h *Hub) Stop() {
	slog.Info("[Hub] Stopping...")

	for _, p := range h.peers {
		if err := p.Stop(); err != nil {
			slog.Warn("[Hub] Peer stop error", "name", p.Name(), "error", err)
		}
	}

	slog.Info("[Hub] All peers stopped")
}

// dispatch routes a change from the source peer to all other peers in the same group.
// Matches Hub.dispatch() in livesync-now.
func (h *Hub) dispatch(source Peer, path string, data *FileData) {
	// Acquire semaphore
	h.mu.Lock()
	for h.activeTasks >= h.maxConcurrency {
		h.cond.Wait()
	}
	h.activeTasks++
	h.mu.Unlock()

	go func() {
		defer func() {
			h.mu.Lock()
			h.activeTasks--
			// Signal waiting tasks
			h.cond.Signal()
			h.mu.Unlock()
		}()

		sourceGroup := source.Group()

		for _, peer := range h.peers {
			if peer == source {
				continue // Don't send back to source
			}

			// Group isolation (matches livesync-now)
			if sourceGroup != peer.Group() {
				continue
			}

			var err error
			if data == nil || data.Deleted {
				_, err = peer.Delete(path)
			} else {
				_, err = peer.Put(path, *data)
			}

			if err != nil {
				slog.Warn("[Hub] Dispatch error",
					"from", source.Name(),
					"to", peer.Name(),
					"path", path,
					"error", err,
				)
			}
		}
	}()
}

// Peers returns the list of all peers.
func (h *Hub) Peers() []Peer {
	return h.peers
}
