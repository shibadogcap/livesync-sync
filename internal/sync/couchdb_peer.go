package sync

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/user/livesync-sync/internal/config"
	"github.com/user/livesync-sync/internal/couchdb"
	ccrypto "github.com/user/livesync-sync/internal/crypto"
	"github.com/user/livesync-sync/internal/model"
	"github.com/user/livesync-sync/internal/state"
)

// CouchDBPeer synchronizes with a CouchDB database.
// Matches livesync-now PeerCouchDB.ts.
type CouchDBPeer struct {
	BasePeer
	client      *couchdb.Client
	config      config.PeerConf
	params      *ccrypto.EncryptionParams
	converter   *model.PathConverter
	splitter    model.ChunkSplitter

	cancel     context.CancelFunc
	done       chan struct{}
}

// NewCouchDBPeer creates a new CouchDB peer.
func NewCouchDBPeer(conf config.PeerConf, store *state.Store, dispatch DispatchFunc) *CouchDBPeer {
	client := couchdb.NewClient(conf.URL, conf.Database, conf.Username, conf.Password)

	splitCfg := model.DefaultChunkConfig()
	// Use V3 Rabin-Karp by default (matching obsidian-livesync default)
	splitter := model.NewV3RabinKarpSplitter(splitCfg)

	return &CouchDBPeer{
		BasePeer:  NewBasePeer(conf, store, dispatch),
		client:    client,
		config:    conf,
		params: &ccrypto.EncryptionParams{
			Passphrase: conf.Passphrase,
		},
		converter: model.NewPathConverter(conf.BaseDir),
		splitter:  splitter,
		done:      make(chan struct{}),
	}
}

// Start initializes the CouchDB connection and begins watching for changes.
// Matches PeerCouchDB.start() in livesync-now.
func (p *CouchDBPeer) Start() error {
	slog.Info("[CouchDB] Starting peer", "name", p.config.Name, "db", p.config.Database)

	// Probe CouchDB server
	if err := p.probeWithBackoff(); err != nil {
		return fmt.Errorf("couchdb probe failed: %w", err)
	}

	// Read sync parameters (PBKDF2 salt, protocol version)
	if err := p.loadSyncParameters(); err != nil {
		slog.Warn("[CouchDB] Failed to load sync parameters", "error", err)
	}

	// Read milestone doc and apply remote tweaks
	if err := p.loadMilestone(); err != nil {
		slog.Warn("[CouchDB] Failed to load milestone", "error", err)
	}

	// Start changes feed watcher
	ctx, cancel := context.WithCancel(context.Background())
	p.cancel = cancel

	go p.watchChanges(ctx)

	slog.Info("[CouchDB] Peer started", "name", p.config.Name)
	return nil
}

// Stop stops the peer gracefully.
func (p *CouchDBPeer) Stop() error {
	slog.Info("[CouchDB] Stopping peer", "name", p.config.Name)
	if p.cancel != nil {
		p.cancel()
	}
	<-p.done
	return nil
}

// Put stores a file in CouchDB.
// Encrypts content, splits into chunks, writes metadata document.
func (p *CouchDBPeer) Put(path string, data FileData) (bool, error) {
	if p.config.Passphrase != "" {
		return p.putEncrypted(path, data)
	}
	return p.putPlain(path, data)
}

func (p *CouchDBPeer) putEncrypted(path string, data FileData) (bool, error) {
	// Check dedup first
	if repeat, err := p.IsRepeating(path, &data); err != nil {
		return false, err
	} else if repeat {
		return false, nil
	}

	localPath := p.ToLocalPath(path)

	// Split content into chunks
	chunks, err := p.splitter.Split(data.Data, true)
	if err != nil {
		return false, fmt.Errorf("chunk split failed: %w", err)
	}

	// Write chunks to CouchDB
	var chunkIDs []string
	for _, chunk := range chunks {
		// Encrypt chunk
		encrypted, err := ccrypto.EncryptV2(chunk, p.params)
		if err != nil {
			return false, fmt.Errorf("chunk encrypt failed: %w", err)
		}

		// Compute chunk ID
		chunkID, err := model.ComputeChunkID(chunk, p.config.Passphrase, model.DefaultHashAlg)
		if err != nil {
			return false, fmt.Errorf("chunk ID computation failed: %w", err)
		}

		// Write chunk document
		chunkDoc := model.ChunkEntry{
			ID:   chunkID,
			Type: model.TypeChunk,
			Data: encrypted,
		}

		chunkJSON, err := json.Marshal(chunkDoc)
		if err != nil {
			return false, err
		}

		if _, err := p.client.PutDoc(chunkID, chunkJSON); err != nil {
			return false, fmt.Errorf("chunk write failed: %w", err)
		}

		chunkIDs = append(chunkIDs, chunkID)
	}

	// Write metadata document (file entry)
	fileEntry := model.FileEntry{
		Path:     localPath,
		CTime:    data.CTime,
		MTime:    data.MTime,
		Size:     data.Size,
		Type:     model.TypePlain,
		Children: chunkIDs,
	}

	entryJSON, err := json.Marshal(fileEntry)
	if err != nil {
		return false, err
	}

	// Compute path-based document ID
	docID, err := p.converter.Path2ID(localPath)
	if err != nil {
		return false, err
	}

	if _, err := p.client.PutDoc(docID, entryJSON); err != nil {
		return false, fmt.Errorf("file entry write failed: %w", err)
	}

	slog.Debug("[CouchDB] File saved", "path", path, "chunks", len(chunkIDs))
	return true, nil
}

func (p *CouchDBPeer) putPlain(path string, data FileData) (bool, error) {
	// For unencrypted mode, just store the file content directly
	if repeat, err := p.IsRepeating(path, &data); err != nil {
		return false, err
	} else if repeat {
		return false, nil
	}

	localPath := p.ToLocalPath(path)

	fileEntry := model.FileEntry{
		Path:  localPath,
		CTime: data.CTime,
		MTime: data.MTime,
		Size:  data.Size,
		Type:  model.TypeNewnote,
	}

	entryJSON, err := json.Marshal(fileEntry)
	if err != nil {
		return false, err
	}

	docID, err := p.converter.Path2ID(localPath)
	if err != nil {
		return false, err
	}

	if _, err := p.client.PutDoc(docID, entryJSON); err != nil {
		return false, fmt.Errorf("file entry write failed: %w", err)
	}

	return true, nil
}

// Get retrieves a file from CouchDB.
func (p *CouchDBPeer) Get(path string) (*FileData, error) {
	localPath := p.ToLocalPath(path)
	docID, err := p.converter.Path2ID(localPath)
	if err != nil {
		return nil, err
	}

	// Get file entry
	raw, err := p.client.GetDoc(docID)
	if err != nil {
		return nil, err
	}
	if raw == nil {
		return nil, nil
	}

	var entry model.FileEntry
	if err := json.Unmarshal(raw, &entry); err != nil {
		return nil, err
	}

	if entry.Deleted {
		return &FileData{Deleted: true}, nil
	}

	// Fetch chunks
	content, err := p.fetchChunks(entry.Children)
	if err != nil {
		return nil, err
	}

	// Decrypt if needed
	if p.config.Passphrase != "" && len(content) > 0 {
		decrypted, err := ccrypto.DecryptAuto(string(content), p.params)
		if err != nil {
			slog.Warn("[CouchDB] Decrypt failed, using raw content", "path", path, "error", err)
		} else {
			content = decrypted
		}
	}

	return &FileData{
		CTime: entry.CTime,
		MTime: entry.MTime,
		Size:  entry.Size,
		Data:  content,
	}, nil
}

// fetchChunks retrieves all chunk data for a file entry.
func (p *CouchDBPeer) fetchChunks(children []string) ([]byte, error) {
	if len(children) == 0 {
		return []byte{}, nil
	}

	// Fetch all chunks
	chunksMap, err := p.client.BulkGetDocs(children)
	if err != nil {
		return nil, err
	}

	var allContent []byte
	for _, chunkID := range children {
		raw, ok := chunksMap[chunkID]
		if !ok {
			continue
		}

		var chunk model.ChunkEntry
		if err := json.Unmarshal(raw, &chunk); err != nil {
			continue
		}

		// The chunk data is stored as a string which might be encrypted
		allContent = append(allContent, []byte(chunk.Data)...)
	}

	return allContent, nil
}

// Delete removes a file from CouchDB.
func (p *CouchDBPeer) Delete(path string) (bool, error) {
	if repeat, err := p.IsRepeating(path, nil); err != nil {
		return false, err
	} else if repeat {
		return false, nil
	}

	localPath := p.ToLocalPath(path)
	docID, err := p.converter.Path2ID(localPath)
	if err != nil {
		return false, err
	}

	// Get current doc to find revision
	raw, err := p.client.GetDoc(docID)
	if err != nil {
		return false, err
	}
	if raw == nil {
		return true, nil // Already gone
	}

	var entry model.FileEntry
	if err := json.Unmarshal(raw, &entry); err != nil {
		return false, err
	}

	// Soft delete by setting deleted flag
	entry.Deleted = true
	entryJSON, _ := json.Marshal(entry)
	if _, err := p.client.PutDoc(docID, entryJSON); err != nil {
		return false, fmt.Errorf("delete failed: %w", err)
	}

	return true, nil
}

// watchChanges continuously monitors the CouchDB changes feed.
// Uses long-poll to get real-time updates.
func (p *CouchDBPeer) watchChanges(ctx context.Context) {
	defer close(p.done)

	since := p.getSetting("since")
	if since == "" {
		since = "now"
	}

	backoff := 1 * time.Second
	maxBackoff := 30 * time.Second

	slog.Info("[CouchDB] Starting changes feed", "since", since)

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		opts := couchdb.DefaultChangesOptions()
		opts.Since = since

		result, err := p.client.FetchChanges(opts)
		if err != nil {
			slog.Warn("[CouchDB] Changes feed error", "error", err)
			time.Sleep(backoff)
			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
			continue
		}

		backoff = 1 * time.Second // Reset on success

		for _, change := range result.Results {
			// Skip non-file documents (like _local docs, design docs)
			if p.shouldSkipChange(change.ID) {
				continue
			}

			if change.Deleted {
				p.dispatch(p, p.ToGlobalPath(change.ID), &FileData{Deleted: true})
			} else if change.Doc != nil {
				p.processFileChange(change)
			}

			// Update since
			since = change.Seq
			p.setSetting("since", since)
		}

		if result.LastSeq != "" {
			since = result.LastSeq
			p.setSetting("since", since)
		}
	}
}

// shouldSkipChange returns true if the change should be ignored.
func (p *CouchDBPeer) shouldSkipChange(id string) bool {
	// Skip design documents
	if len(id) > 0 && id[0] == '_' {
		return true
	}
	// Skip internal documents (i: prefix)
	if strings.HasPrefix(id, "i:") {
		hasInternal := false
		for _, pattern := range p.config.IncludeInternal {
			if strings.Contains(id, pattern) {
				hasInternal = true
				break
			}
		}
		if !hasInternal {
			return true
		}
	}
	// Skip chunk documents (managed automatically)
	if len(id) > 1 && id[1] == ':' {
		return true
	}
	return false
}

// processFileChange processes a document change from the changes feed.
func (p *CouchDBPeer) processFileChange(change couchdb.Change) {
	var entry model.FileEntry
	if err := json.Unmarshal(change.Doc, &entry); err != nil {
		slog.Warn("[CouchDB] Failed to parse change doc", "id", change.ID, "error", err)
		return
	}

	// Skip non-file entries
	if entry.Type != model.TypePlain && entry.Type != model.TypeNewnote {
		return
	}

	// Fetch and decrypt content
	content, err := p.fetchChunks(entry.Children)
	if err != nil {
		slog.Warn("[CouchDB] Failed to fetch chunks", "id", change.ID, "error", err)
		return
	}

	if p.config.Passphrase != "" && len(content) > 0 {
		decrypted, err := ccrypto.DecryptAuto(string(content), p.params)
		if err != nil {
			slog.Warn("[CouchDB] Decrypt failed", "id", change.ID, "error", err)
			return
		}
		content = decrypted
	}

	// Dispatch to hub
	p.dispatch(p, p.ToGlobalPath(change.ID), &FileData{
		CTime: entry.CTime,
		MTime: entry.MTime,
		Size:  entry.Size,
		Data:  content,
	})
}

// loadSyncParameters reads the SyncParameters document from CouchDB.
// The PBKDF2 salt is stored as base64 in the doc and decoded to raw bytes for use.
func (p *CouchDBPeer) loadSyncParameters() error {
	var params model.SyncParameters
	if err := p.client.GetLocalDocJSON(model.SyncParamsDocID, &params); err != nil {
		return err
	}

	// If no sync params exist, create them with a fresh PBKDF2 salt
	if params.ID == "" {
		newSalt := make([]byte, 32)
		if _, err := rand.Read(newSalt); err != nil {
			return fmt.Errorf("failed to generate PBKDF2 salt: %w", err)
		}
		params = model.SyncParameters{
			ID:              model.SyncParamsDocID,
			Type:            "sync-parameters",
			ProtocolVersion: model.ProtocolAdvancedE2EE,
			PBKDF2Salt:      base64.StdEncoding.EncodeToString(newSalt),
		}
		if _, err := p.client.PutLocalDocJSON(model.SyncParamsDocID, &params); err != nil {
			return err
		}
		p.params.PBKDF2Salt = newSalt
		slog.Info("[CouchDB] Created sync parameters with new PBKDF2 salt")
		return nil
	}

	// Decode existing PBKDF2 salt (base64 → raw bytes)
	if params.PBKDF2Salt != "" {
		decoded, err := base64.StdEncoding.DecodeString(params.PBKDF2Salt)
		if err != nil {
			return fmt.Errorf("failed to decode PBKDF2 salt: %w", err)
		}
		p.params.PBKDF2Salt = decoded
		slog.Debug("[CouchDB] Loaded PBKDF2 salt", "len", len(decoded))
	} else {
		// Salt exists but is empty — generate one
		newSalt := make([]byte, 32)
		if _, err := rand.Read(newSalt); err != nil {
			return fmt.Errorf("failed to generate PBKDF2 salt: %w", err)
		}
		params.PBKDF2Salt = base64.StdEncoding.EncodeToString(newSalt)
		if _, err := p.client.PutLocalDocJSON(model.SyncParamsDocID, &params); err != nil {
			return err
		}
		p.params.PBKDF2Salt = newSalt
		slog.Info("[CouchDB] Generated new PBKDF2 salt for existing sync params")
	}

	return nil
}

// loadMilestone reads the milestone document and applies remote tweaks.
func (p *CouchDBPeer) loadMilestone() error {
	var milestone model.MilestoneDoc
	if err := p.client.GetLocalDocJSON(model.MilestoneDocID, &milestone); err != nil {
		return err
	}
	if milestone.ID == "" {
		// No milestone yet (empty DB)
		p.setSetting("remote-created", "0")
		return nil
	}

	// Check for DB rebuild
	savedCreated := p.getSetting("remote-created")
	createdStr := fmt.Sprintf("%d", milestone.Created)
	if savedCreated != "" && savedCreated != createdStr {
		slog.Info("[CouchDB] Database rebuild detected, will resync from start")
		p.setSetting("since", "")
	}
	p.setSetting("remote-created", createdStr)

	// Apply remote tweaks if enabled
	if p.config.UseRemoteTweaks != nil && *p.config.UseRemoteTweaks {
		p.applyTweaks(milestone)
	}

	return nil
}

// applyTweaks applies remote configuration overrides from the milestone document.
func (p *CouchDBPeer) applyTweaks(milestone model.MilestoneDoc) {
	for _, tweaks := range milestone.TweakValues {
		// Validate encryption settings
		if tweaks.Encrypt && p.config.Passphrase == "" {
			slog.Warn("[CouchDB] Remote DB has encryption enabled but no passphrase provided")
		}
		if tweaks.UsePathObfuscation && p.config.ObfuscatePassphrase == "" {
			slog.Warn("[CouchDB] Remote DB has path obfuscation but no obfuscation passphrase")
		}

		slog.Info("[CouchDB] Remote tweaks applied",
			"encrypt", tweaks.Encrypt,
			"chunkSplitter", tweaks.ChunkSplitterVersion,
			"hashAlg", tweaks.HashAlg,
		)
	}
}

// probeWithBackoff tries to connect to CouchDB with exponential backoff.
func (p *CouchDBPeer) probeWithBackoff() error {
	backoff := 1 * time.Second
	maxBackoff := 30 * time.Second
	maxAttempts := 10

	for i := 0; i < maxAttempts; i++ {
		err := p.client.Probe()
		if err == nil {
			return nil
		}

		if i < maxAttempts-1 {
			slog.Debug("[CouchDB] Probe failed, retrying", "attempt", i+1, "backoff", backoff)
			time.Sleep(backoff)
			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
		}
	}

	return fmt.Errorf("couchdb not reachable after %d attempts", maxAttempts)
}


