// Package config provides configuration loading for livesync-sync.
// The config format follows the same peers[] structure as livesync-now (vrtmrz/livesync-bridge).
package config

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config is the top-level configuration structure.
// Same concept as livesync-now's Config with peers[] array.
type Config struct {
	Peers []PeerConf `yaml:"peers" json:"peers"`
}

// PeerConf is a union of CouchDB and Storage peer configurations.
// The Type field discriminates between them (matches livesync-now types).
type PeerConf struct {
	// Common fields (all peers)
	Type    string `yaml:"type" json:"type"`       // "couchdb" | "storage"
	Name    string `yaml:"name" json:"name"`       // Unique identifier
	Group   string `yaml:"group" json:"group"`     // Group isolation (default "")
	BaseDir string `yaml:"baseDir" json:"baseDir"` // Base directory path

	// CouchDB-specific fields
	URL                 string   `yaml:"url,omitempty" json:"url,omitempty"`
	Database            string   `yaml:"database,omitempty" json:"database,omitempty"`
	Username            string   `yaml:"username,omitempty" json:"username,omitempty"`
	Password            string   `yaml:"password,omitempty" json:"password,omitempty"`
	Passphrase          string   `yaml:"passphrase,omitempty" json:"passphrase,omitempty"`
	ObfuscatePassphrase string   `yaml:"obfuscatePassphrase,omitempty" json:"obfuscatePassphrase,omitempty"`
	UseRemoteTweaks     *bool    `yaml:"useRemoteTweaks,omitempty" json:"useRemoteTweaks,omitempty"`
	IncludeInternal     []string `yaml:"includeInternal,omitempty" json:"includeInternal,omitempty"`

	// Storage-specific fields
	ScanOfflineChanges *bool    `yaml:"scanOfflineChanges,omitempty" json:"scanOfflineChanges,omitempty"`
	UseChokidar        *bool    `yaml:"useChokidar,omitempty" json:"useChokidar,omitempty"`
	IgnorePatterns     []string `yaml:"ignorePatterns,omitempty" json:"ignorePatterns,omitempty"`
	ProcessorCmd       string   `yaml:"processorCmd,omitempty" json:"processorCmd,omitempty"`
	ProcessorArgs      []string `yaml:"processorArgs,omitempty" json:"processorArgs,omitempty"`
}

// LoggingConfig controls logging behavior.
type LoggingConfig struct {
	Level   string `yaml:"level" json:"level"`     // debug | info | warn | error
	File    string `yaml:"file" json:"file"`       // Log file path (empty = stderr)
	MaxSize int    `yaml:"maxSize" json:"maxSize"` // Max size in MB before rotation
}

// APIConfig controls the embedded settings UI server (Phase 1).
type APIConfig struct {
	Listen string `yaml:"listen" json:"listen"` // e.g. "localhost:2324"
}

// TrayConfig controls system tray behavior.
type TrayConfig struct {
	Enable    bool `yaml:"enable" json:"enable"`
	Autostart bool `yaml:"autostart" json:"autostart"`
}

// FullConfig is the complete configuration with all sections.
type FullConfig struct {
	Sync     SyncConfig     `yaml:"sync" json:"sync"`
	State    StateConfig    `yaml:"state" json:"state"`
	Logging  LoggingConfig  `yaml:"logging" json:"logging"`
	API      APIConfig      `yaml:"api" json:"api"`
	Tray     TrayConfig     `yaml:"tray" json:"tray"`
}

// SyncConfig wraps the peers array (matches livesync-now structure).
type SyncConfig struct {
	Peers []PeerConf `yaml:"peers" json:"peers"`
}

// StateConfig configures the state file location.
type StateConfig struct {
	File string `yaml:"file" json:"file"`
}

// DefaultConfig returns a FullConfig with sensible defaults.
func DefaultConfig() FullConfig {
	return FullConfig{
		Sync: SyncConfig{
			Peers: []PeerConf{},
		},
		State: StateConfig{
			File: "~/.livesync/state.json",
		},
		Logging: LoggingConfig{
			Level:   "info",
			File:    "",
			MaxSize: 10,
		},
		API: APIConfig{
			Listen: "localhost:2324",
		},
		Tray: TrayConfig{
			Enable:    true,
			Autostart: false,
		},
	}
}

// LoadConfig reads and parses a configuration file.
// Supports JSON and YAML formats.
// Config locations (first found wins):
//  1. LSYNC_CONFIG environment variable
//  2. ~/.livesync/config.yaml
//  3. ~/.livesync/config.json
//  4. ./livesync.yaml
//  5. ./livesync.json
//  6. ./config.yaml
//  7. ./config.json
func LoadConfig() (*FullConfig, error) {
	locations := []string{
		os.Getenv("LSYNC_CONFIG"),
		expandHome("~/.livesync/config.yaml"),
		expandHome("~/.livesync/config.json"),
		"./livesync.yaml",
		"./livesync.json",
		"./config.yaml",
		"./config.json",
	}

	var cfg FullConfig
	loaded := false

	for _, loc := range locations {
		if loc == "" {
			continue
		}
		data, err := os.ReadFile(loc)
		if err != nil {
			continue
		}

		// Detect format by extension
		switch {
		case strings.HasSuffix(loc, ".yaml") || strings.HasSuffix(loc, ".yml"):
			if err := yaml.Unmarshal(data, &cfg); err != nil {
				return nil, fmt.Errorf("failed to parse YAML config %s: %w", loc, err)
			}
		default:
			if err := json.Unmarshal(data, &cfg); err != nil {
				return nil, fmt.Errorf("failed to parse JSON config %s: %w", loc, err)
			}
		}

		// Handle V2 format: if config has Sync.Peers itself, fine.
		// Otherwise, try to detect if it's a V2 (flat couchdb+localStorage) format.
		if len(cfg.Sync.Peers) == 0 {
			cfg = convertLegacyFormat(data, cfg)
		}

		loaded = true
		fmt.Printf("[CONFIG] Loaded from %s\n", loc)
		break
	}

	if !loaded {
		// Return defaults if no config found
		fmt.Println("[CONFIG] No config file found, using defaults")
		cfg = DefaultConfig()
	}

	// Apply environment variable overrides
	applyEnvOverrides(&cfg)

	// Expand home directory in paths
	expandPaths(&cfg)

	return &cfg, nil
}

// convertLegacyFormat attempts to detect and convert livesync-now V2 config format.
// V2 format: {"couchdb":{...}, "localStorage":{...}}
// V1 format: {"peers":[...]}
func convertLegacyFormat(data []byte, cfg FullConfig) FullConfig {
	var legacy struct {
		Couchdb *struct {
			Database            string `json:"database"`
			Username            string `json:"username"`
			Password            string `json:"password"`
			URL                 string `json:"url"`
			Passphrase          string `json:"passphrase"`
			ObfuscatePassphrase string `json:"obfuscatePassphrase"`
		} `json:"couchdb"`
		LocalStorage *struct {
			BasePath       string   `json:"basePath"`
			IgnorePatterns []string `json:"ignorePatterns"`
		} `json:"localStorage"`
	}
	if err := json.Unmarshal(data, &legacy); err != nil {
		return cfg
	}
	if legacy.Couchdb == nil || legacy.LocalStorage == nil {
		return cfg
	}

	useRT := true
	scanOffline := true

	cfg.Sync.Peers = append(cfg.Sync.Peers, PeerConf{
		Type:                "couchdb",
		Name:                "remote",
		Group:               "main",
		Database:            legacy.Couchdb.Database,
		Username:            legacy.Couchdb.Username,
		Password:            legacy.Couchdb.Password,
		URL:                 legacy.Couchdb.URL,
		Passphrase:          legacy.Couchdb.Passphrase,
		ObfuscatePassphrase: legacy.Couchdb.ObfuscatePassphrase,
		BaseDir:             "",
		UseRemoteTweaks:     &useRT,
	})

	basePath := legacy.LocalStorage.BasePath
	if basePath == "" {
		basePath = "."
	}

	cfg.Sync.Peers = append(cfg.Sync.Peers, PeerConf{
		Type:                "storage",
		Name:                "local",
		Group:               "main",
		BaseDir:             basePath,
		ScanOfflineChanges:  &scanOffline,
		IgnorePatterns:      legacy.LocalStorage.IgnorePatterns,
	})

	fmt.Println("[CONFIG] Converted legacy V2 format to peers[]")
	return cfg
}

// applyEnvOverrides overrides config values from environment variables.
func applyEnvOverrides(cfg *FullConfig) {
	if v := os.Getenv("LSYNC_LOG_LEVEL"); v != "" {
		cfg.Logging.Level = v
	}
	if v := os.Getenv("LSYNC_STATE_DIR"); v != "" {
		cfg.State.File = filepath.Join(v, "state.json")
	}
	// Per-peer overrides via env are not supported;
	// use the config file for complex setups.
}

// expandPaths expands ~/ and ${HOME} in paths.
func expandPaths(cfg *FullConfig) {
	cfg.State.File = expandHome(cfg.State.File)
}

func expandHome(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err == nil {
			return filepath.Join(home, path[2:])
		}
	}
	return path
}

// IsCouchDBPeer returns true if the peer config is for CouchDB.
func IsCouchDBPeer(p *PeerConf) bool {
	return p.Type == "couchdb"
}

// IsStoragePeer returns true if the peer config is for storage (filesystem).
func IsStoragePeer(p *PeerConf) bool {
	return p.Type == "storage"
}

// SaveConfig persists the configuration to the default config file path.
// Writes as JSON to ~/.livesync/config.json.
func SaveConfig(cfg *FullConfig) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("cannot determine home dir: %w", err)
	}

	dir := filepath.Join(home, ".livesync")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("cannot create config dir: %w", err)
	}

	path := filepath.Join(dir, "config.json")
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config failed: %w", err)
	}

	// Atomic write: write to temp file then rename
	// Use 0600 because the config contains passwords and passphrases
	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0600); err != nil {
		return fmt.Errorf("write config failed: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("rename config failed: %w", err)
	}
	// Ensure the final file has the correct permissions regardless of umask
	if err := os.Chmod(path, 0600); err != nil {
		return fmt.Errorf("chmod config failed: %w", err)
	}

	slog.Info("[Config] Saved to", "path", path)
	return nil
}
