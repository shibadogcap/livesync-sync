package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ============================================================
// デフォルト設定テスト
// ============================================================

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Logging.Level != "info" {
		t.Errorf("default log level = %q, want %q", cfg.Logging.Level, "info")
	}
	if cfg.API.Listen != "127.0.0.1:2324" {
		t.Errorf("default API listen = %q, want %q", cfg.API.Listen, "127.0.0.1:2324")
	}
	if cfg.Tray.Enable != true {
		t.Errorf("default tray enable = %v, want true", cfg.Tray.Enable)
	}
	if len(cfg.Sync.Peers) != 0 {
		t.Errorf("default peers should be empty, got %d", len(cfg.Sync.Peers))
	}
}

// ============================================================
// JSON 設定読み込みテスト
// ============================================================

func TestLoadJSONConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	configJSON := `{
		"sync": {
			"peers": [
				{
					"type": "couchdb",
					"name": "remote",
					"url": "http://localhost:5984",
					"database": "testdb",
					"username": "admin",
					"password": "secret",
					"passphrase": "e2ee-key",
					"baseDir": ""
				},
				{
					"type": "storage",
					"name": "local",
					"baseDir": "/tmp/vault",
					"scanOfflineChanges": true
				}
			]
		},
		"logging": {
			"level": "debug"
		},
		"tray": {
			"enable": false
		}
	}`

	if err := os.WriteFile(path, []byte(configJSON), 0644); err != nil {
		t.Fatalf("write config failed: %v", err)
	}

	// Override config path
	os.Setenv("LSYNC_CONFIG", path)
	defer os.Unsetenv("LSYNC_CONFIG")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if len(cfg.Sync.Peers) != 2 {
		t.Fatalf("expected 2 peers, got %d", len(cfg.Sync.Peers))
	}

	// Verify CouchDB peer
	if cfg.Sync.Peers[0].Type != "couchdb" {
		t.Errorf("peer[0].type = %q, want %q", cfg.Sync.Peers[0].Type, "couchdb")
	}
	if cfg.Sync.Peers[0].URL != "http://localhost:5984" {
		t.Errorf("peer[0].url = %q", cfg.Sync.Peers[0].URL)
	}
	if cfg.Sync.Peers[0].Database != "testdb" {
		t.Errorf("peer[0].database = %q", cfg.Sync.Peers[0].Database)
	}

	// Verify Storage peer
	if cfg.Sync.Peers[1].Type != "storage" {
		t.Errorf("peer[1].type = %q, want %q", cfg.Sync.Peers[1].Type, "storage")
	}
	if cfg.Sync.Peers[1].BaseDir != "/tmp/vault" {
		t.Errorf("peer[1].baseDir = %q", cfg.Sync.Peers[1].BaseDir)
	}

	if cfg.Logging.Level != "debug" {
		t.Errorf("log level = %q, want %q", cfg.Logging.Level, "debug")
	}
	if cfg.Tray.Enable != false {
		t.Errorf("tray enable = %v, want false", cfg.Tray.Enable)
	}
}

// ============================================================
// YAML 設定読み込みテスト
// ============================================================

func TestLoadYAMLConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	configYAML := `sync:
  peers:
    - type: couchdb
      name: remote
      url: http://localhost:5984
      database: yamldb
      username: admin
      password: secret
      passphrase: mykey
      baseDir: ""
    - type: storage
      name: local
      baseDir: /yaml/vault
      scanOfflineChanges: true
logging:
  level: warn
tray:
  enable: false
`

	if err := os.WriteFile(path, []byte(configYAML), 0644); err != nil {
		t.Fatalf("write YAML config failed: %v", err)
	}

	os.Setenv("LSYNC_CONFIG", path)
	defer os.Unsetenv("LSYNC_CONFIG")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig (YAML) failed: %v", err)
	}

	if len(cfg.Sync.Peers) != 2 {
		t.Fatalf("expected 2 peers, got %d", len(cfg.Sync.Peers))
	}

	if cfg.Sync.Peers[0].Database != "yamldb" {
		t.Errorf("database = %q, want %q", cfg.Sync.Peers[0].Database, "yamldb")
	}
	if cfg.Logging.Level != "warn" {
		t.Errorf("log level = %q, want %q", cfg.Logging.Level, "warn")
	}
}

// ============================================================
// レガシーV2形式変換テスト
// ============================================================

func TestConvertLegacyV2Format(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "legacy.json")

	// This is the V2 format used by older versions of livesync-now
	legacyJSON := `{
		"couchdb": {
			"database": "legacydb",
			"username": "admin",
			"password": "oldpass",
			"url": "http://couchdb:5984",
			"passphrase": "oldkey",
			"obfuscatePassphrase": "oldkey"
		},
		"localStorage": {
			"basePath": "/data/vault",
			"ignorePatterns": [".trash/", ".obsidian/"]
		}
	}`

	if err := os.WriteFile(path, []byte(legacyJSON), 0644); err != nil {
		t.Fatalf("write legacy config failed: %v", err)
	}

	os.Setenv("LSYNC_CONFIG", path)
	defer os.Unsetenv("LSYNC_CONFIG")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig (legacy) failed: %v", err)
	}

	if len(cfg.Sync.Peers) != 2 {
		t.Fatalf("expected 2 peers from legacy format, got %d", len(cfg.Sync.Peers))
	}

	// CouchDB peer
	if cfg.Sync.Peers[0].Type != "couchdb" {
		t.Errorf("legacy couchdb peer type = %q", cfg.Sync.Peers[0].Type)
	}
	if cfg.Sync.Peers[0].Database != "legacydb" {
		t.Errorf("legacy database = %q", cfg.Sync.Peers[0].Database)
	}

	// Storage peer
	if cfg.Sync.Peers[1].Type != "storage" {
		t.Errorf("legacy storage peer type = %q", cfg.Sync.Peers[1].Type)
	}
	if cfg.Sync.Peers[1].BaseDir != "/data/vault" {
		t.Errorf("legacy baseDir = %q", cfg.Sync.Peers[1].BaseDir)
	}
	if len(cfg.Sync.Peers[1].IgnorePatterns) != 2 {
		t.Errorf("expected 2 ignore patterns, got %d", len(cfg.Sync.Peers[1].IgnorePatterns))
	}
}

// ============================================================
// 空の設定ファイルが存在しない場合のテスト
// ============================================================

func TestLoadConfigNoFile(t *testing.T) {
	// Ensure no config file is found by overriding HOME to an empty temp dir
	os.Unsetenv("LSYNC_CONFIG")

	tmpHome := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", origHome)

	origDir, _ := os.Getwd()
	os.Chdir(tmpHome)
	defer os.Chdir(origDir)

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig with no file should return defaults, got error: %v", err)
	}

	if cfg == nil {
		t.Fatal("expected default config, got nil")
	}

	// Should return defaults, not error
	if cfg.API.Listen != "127.0.0.1:2324" {
		t.Errorf("expected default API listen, got %q", cfg.API.Listen)
	}
}

// ============================================================
// SaveConfig テスト
// ============================================================

func TestSaveConfig(t *testing.T) {
	// Save to a temp location
	home := t.TempDir()
	os.Setenv("HOME", home)
	defer os.Unsetenv("HOME")

	cfg := DefaultConfig()
	cfg.Sync.Peers = []PeerConf{
		{
			Type:     "couchdb",
			Name:     "test-remote",
			URL:      "http://example.com:5984",
			Database: "savedb",
		},
	}

	if err := SaveConfig(&cfg); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}

	// Verify the file was created
	savedPath := filepath.Join(home, ".livesync", "config.json")
	data, err := os.ReadFile(savedPath)
	if err != nil {
		t.Fatalf("read saved config failed: %v", err)
	}

	if len(data) == 0 {
		t.Fatal("saved config is empty")
	}
}

// ============================================================
// PeerConf 判定関数テスト
// ============================================================

func TestPeerTypeDetection(t *testing.T) {
	tests := []struct {
		peer    PeerConf
		isDB    bool
		isStore bool
	}{
		{PeerConf{Type: "couchdb"}, true, false},
		{PeerConf{Type: "storage"}, false, true},
		{PeerConf{Type: "unknown"}, false, false},
		{PeerConf{Type: ""}, false, false},
	}

	for _, tt := range tests {
		if got := IsCouchDBPeer(&tt.peer); got != tt.isDB {
			t.Errorf("IsCouchDBPeer(%q) = %v, want %v", tt.peer.Type, got, tt.isDB)
		}
		if got := IsStoragePeer(&tt.peer); got != tt.isStore {
			t.Errorf("IsStoragePeer(%q) = %v, want %v", tt.peer.Type, got, tt.isStore)
		}
	}
}

func TestExpandHome(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skipf("cannot get home dir: %v", err)
	}

	tests := []struct {
		input string
		want  string
	}{
		{"~/test", filepath.Join(home, "test")},
		{"~/.livesync/config.json", filepath.Join(home, ".livesync", "config.json")},
		{"/absolute/path", "/absolute/path"},
		{"relative/path", "relative/path"},
		// "~" alone should not expand (no trailing slash)
		{"~", "~"},
		// "~other" should not expand (not ~/)
		{"~other/path", "~other/path"},
	}

	for _, tt := range tests {
		got := expandHome(tt.input)
		if got != tt.want {
			t.Errorf("expandHome(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}

	// Verify ~ expansion actually produces a path containing the home dir
	expanded := expandHome("~/test")
	if !strings.HasPrefix(expanded, home) {
		t.Errorf("expandHome should expand ~ to home dir: got %q, expected prefix %q", expanded, home)
	}
}
