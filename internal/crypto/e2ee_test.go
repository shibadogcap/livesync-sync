package crypto

import (
	"bytes"
	"encoding/base64"
	"strings"
	"testing"
)

// ============================================================
// E2EE V2 暗号化・復号テスト
// ============================================================

func TestEncryptV2Roundtrip(t *testing.T) {
	params := &EncryptionParams{
		Passphrase: "test-passphrase-123",
		PBKDF2Salt: make([]byte, 32),
	}
	// Fixed salt for deterministic test
	for i := range params.PBKDF2Salt {
		params.PBKDF2Salt[i] = byte(i)
	}

	plaintext := []byte("Hello, livesync-sync! This is a test message.")

	encrypted, err := EncryptV2(plaintext, params)
	if err != nil {
		t.Fatalf("EncryptV2 failed: %v", err)
	}

	// Verify V2 prefix
	if !strings.HasPrefix(encrypted, v2Prefix) {
		t.Errorf("expected prefix %q, got %q", v2Prefix, encrypted[:2])
	}

	// Decrypt
	decrypted, err := DecryptV2(encrypted, params)
	if err != nil {
		t.Fatalf("DecryptV2 failed: %v", err)
	}

	if !bytes.Equal(decrypted, plaintext) {
		t.Errorf("decrypted text mismatch:\n  got:  %q\n  want: %q", string(decrypted), string(plaintext))
	}
}

func TestEncryptV2EphemeralRoundtrip(t *testing.T) {
	passphrase := "test-passphrase-ephemeral"
	plaintext := []byte("Ephemeral mode test with embedded salt.")

	encrypted, err := EncryptV2Ephemeral(plaintext, passphrase)
	if err != nil {
		t.Fatalf("EncryptV2Ephemeral failed: %v", err)
	}

	// Verify V2 ephemeral prefix
	if !strings.HasPrefix(encrypted, v2SaltPrefix) {
		t.Errorf("expected prefix %q, got %q", v2SaltPrefix, encrypted[:2])
	}

	// Decrypt (no external salt needed - it's embedded)
	params := &EncryptionParams{
		Passphrase: passphrase,
		// PBKDF2Salt is embedded in the data itself
	}
	decrypted, err := DecryptV2(encrypted, params)
	if err != nil {
		t.Fatalf("DecryptV2 (ephemeral) failed: %v", err)
	}

	if !bytes.Equal(decrypted, plaintext) {
		t.Errorf("decrypted text mismatch:\n  got:  %q\n  want: %q", string(decrypted), string(plaintext))
	}
}

func TestEncryptV2MultipleTimes(t *testing.T) {
	// V2 with random IV+salt should produce different ciphertext each time
	params := &EncryptionParams{
		Passphrase: "test-multi",
		PBKDF2Salt: []byte("01234567890123456789012345678901"), // 32 bytes
	}
	plaintext := []byte("Same plaintext, different IV each time.")

	results := make(map[string]bool)
	for i := 0; i < 5; i++ {
		enc, err := EncryptV2(plaintext, params)
		if err != nil {
			t.Fatalf("EncryptV2 iteration %d failed: %v", i, err)
		}
		if results[enc] {
			t.Errorf("iteration %d produced duplicate ciphertext (IV reuse?)", i)
		}
		results[enc] = true

		// But all should decrypt to the same plaintext
		dec, _ := DecryptV2(enc, params)
		if !bytes.Equal(dec, plaintext) {
			t.Errorf("iteration %d decrypt mismatch", i)
		}
	}
}

// ============================================================
// E2EE V1 復号テスト (互換性維持)
// ============================================================

func TestDetectVersion(t *testing.T) {
	tests := []struct {
		input   string
		want    EncryptionVersion
	}{
		{"%=base64data", Version2},
		{"%$base64data", Version2Ephemeral},
		{"%~base64data", Version3},
		{"%somelegacydata", Version1},
		{"plain unencrypted text", VersionUnencrypted},
		{"", VersionUnencrypted},
	}

	for _, tt := range tests {
		got := DetectVersion(tt.input)
		if got != tt.want {
			t.Errorf("DetectVersion(%q) = %v, want %v", tt.input[:min(len(tt.input), 10)], got, tt.want)
		}
	}
}

func TestEncryptV1Roundtrip(t *testing.T) {
	passphrase := "test-v1-passphrase"
	plaintext := []byte("V1 legacy format test.")

	encrypted, err := EncryptV1(plaintext, passphrase, false)
	if err != nil {
		t.Fatalf("EncryptV1 failed: %v", err)
	}

	if !strings.HasPrefix(encrypted, v1Prefix) {
		t.Errorf("expected prefix %q, got %q", v1Prefix, encrypted[:1])
	}

	// Decrypt V1
	decrypted, err := DecryptV1(encrypted, passphrase, false)
	if err != nil {
		t.Fatalf("DecryptV1 failed: %v", err)
	}

	if !bytes.Equal(decrypted, plaintext) {
		t.Errorf("decrypted text mismatch:\n  got:  %q\n  want: %q", string(decrypted), string(plaintext))
	}
}

func TestDecryptV1WithDynamicIteration(t *testing.T) {
	passphrase := "short" // short passphrase → higher iteration count
	plaintext := []byte("V1 with dynamic iteration.")

	encrypted, err := EncryptV1(plaintext, passphrase, true)
	if err != nil {
		t.Fatalf("EncryptV1 (dynamic) failed: %v", err)
	}

	decrypted, err := DecryptV1(encrypted, passphrase, true)
	if err != nil {
		t.Fatalf("DecryptV1 (dynamic) failed: %v", err)
	}

	if !bytes.Equal(decrypted, plaintext) {
		t.Errorf("decrypted text mismatch")
	}
}

func TestDecryptV3Roundtrip(t *testing.T) {
	// V3 is decrypt-only legacy format; no EncryptV3 implemented
	// Just verify the format detection works
	v3data := "%~0102030405060708090a0b0cbase64ciphertext"
	if DetectVersion(v3data) != Version3 {
		t.Error("V3 prefix detection failed")
	}
	t.Skip("V3 roundtrip requires external test vector")
}

// ============================================================
// DecryptAuto テスト (自動検出)
// ============================================================

func TestDecryptAutoV2(t *testing.T) {
	params := &EncryptionParams{
		Passphrase: "auto-test",
		PBKDF2Salt: bytes.Repeat([]byte{0xAB}, 32),
	}
	plaintext := []byte("Auto-detect V2 decryption.")

	encrypted, err := EncryptV2(plaintext, params)
	if err != nil {
		t.Fatalf("EncryptV2 failed: %v", err)
	}

	decrypted, err := DecryptAuto(encrypted, params)
	if err != nil {
		t.Fatalf("DecryptAuto(V2) failed: %v", err)
	}

	if !bytes.Equal(decrypted, plaintext) {
		t.Errorf("decrypted text mismatch")
	}
}

func TestDecryptAutoUnencrypted(t *testing.T) {
	plaintext := []byte("This is not encrypted at all.")
	params := &EncryptionParams{Passphrase: "any"}

	decrypted, err := DecryptAuto(string(plaintext), params)
	if err != nil {
		t.Fatalf("DecryptAuto(unencrypted) failed: %v", err)
	}

	if !bytes.Equal(decrypted, plaintext) {
		t.Errorf("unencrypted passthrough failed")
	}
}

// ============================================================
// 互換性テストベクター
// ============================================================

// TestKnownV2Format tests that we can decrypt a known V2-format string.
// This format must be kept compatible with obsidian-livesync's octagonal-wheels.
//
// V2 format: %=base64(IV(12B) + HKDFsalt(32B) + ciphertext)
func TestKnownV2FormatParsing(t *testing.T) {
	// We can't hardcode a known ciphertext without knowing the exact salt,
	// but we can verify the structural parsing is correct.
	// The real compatibility test must be done with the JS implementation.
	
	// Verify structure of V2 output
	params := &EncryptionParams{
		Passphrase: "compat-test",
		PBKDF2Salt: make([]byte, 32),
	}
	plaintext := []byte("Compatibility test vector.")

	enc, err := EncryptV2(plaintext, params)
	if err != nil {
		t.Fatalf("EncryptV2 failed: %v", err)
	}

	// Parse the V2 data
	raw := strings.TrimPrefix(enc, v2Prefix)
	decoded, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		t.Fatalf("base64 decode failed: %v", err)
	}

	// Structure: IV(12) + HKDFsalt(32) + ciphertext
	if len(decoded) < v2IVLength+v2HKDFSaltLength {
		t.Fatalf("decoded data too short: %d bytes", len(decoded))
	}

	iv := decoded[:v2IVLength]
	salt := decoded[v2IVLength : v2IVLength+v2HKDFSaltLength]
	ciphertext := decoded[v2IVLength+v2HKDFSaltLength:]

	if len(iv) != v2IVLength {
		t.Errorf("IV length: got %d, want %d", len(iv), v2IVLength)
	}
	if len(salt) != v2HKDFSaltLength {
		t.Errorf("HKDF salt length: got %d, want %d", len(salt), v2HKDFSaltLength)
	}
	if len(ciphertext) < 16 { // GCM tag is at least 16 bytes
		t.Errorf("ciphertext too short: %d bytes", len(ciphertext))
	}
}

// TestEncryptDecryptLargeData tests with larger payloads (multiple chunks).
func TestEncryptDecryptLargeData(t *testing.T) {
	params := &EncryptionParams{
		Passphrase: "large-data-test",
		PBKDF2Salt: bytes.Repeat([]byte{0x42}, 32),
	}

	// Create a ~10KB payload
	plaintext := bytes.Repeat([]byte("The quick brown fox jumps over the lazy dog. "), 200)

	encrypted, err := EncryptV2(plaintext, params)
	if err != nil {
		t.Fatalf("EncryptV2 (large) failed: %v", err)
	}

	decrypted, err := DecryptV2(encrypted, params)
	if err != nil {
		t.Fatalf("DecryptV2 (large) failed: %v", err)
	}

	if !bytes.Equal(decrypted, plaintext) {
		t.Errorf("large data mismatch: got %d bytes, want %d bytes", len(decrypted), len(plaintext))
	}
}

// TestDecryptWithWrongPassphrase ensures decryption fails with wrong key.
func TestDecryptWithWrongPassphrase(t *testing.T) {
	params := &EncryptionParams{
		Passphrase: "correct-passphrase",
		PBKDF2Salt: bytes.Repeat([]byte{0x11}, 32),
	}
	plaintext := []byte("Secret data.")

	encrypted, err := EncryptV2(plaintext, params)
	if err != nil {
		t.Fatalf("EncryptV2 failed: %v", err)
	}

	// Wrong passphrase
	wrongParams := &EncryptionParams{
		Passphrase: "wrong-passphrase",
		PBKDF2Salt: bytes.Repeat([]byte{0x11}, 32),
	}

	_, err = DecryptV2(encrypted, wrongParams)
	if err == nil {
		t.Errorf("expected error when decrypting with wrong passphrase, got nil")
	}
}

func TestDecryptWithWrongSalt(t *testing.T) {
	params := &EncryptionParams{
		Passphrase: "test-salt",
		PBKDF2Salt: bytes.Repeat([]byte{0xAA}, 32),
	}
	plaintext := []byte("Salt-protected data.")

	encrypted, err := EncryptV2(plaintext, params)
	if err != nil {
		t.Fatalf("EncryptV2 failed: %v", err)
	}

	// Wrong salt
	wrongParams := &EncryptionParams{
		Passphrase: "test-salt",
		PBKDF2Salt: bytes.Repeat([]byte{0xBB}, 32),
	}

	_, err = DecryptV2(encrypted, wrongParams)
	if err == nil {
		t.Errorf("expected error when decrypting with wrong salt, got nil")
	}
}

// ============================================================
// HKDF deriveKey test
// ============================================================

func TestDeriveHKDFKey(t *testing.T) {
	masterKey := make([]byte, 32)
	for i := range masterKey {
		masterKey[i] = byte(i)
	}
	salt := bytes.Repeat([]byte{0xDE, 0xAD}, 16) // 32 bytes

	key1, err := deriveHKDFKey(masterKey, salt)
	if err != nil {
		t.Fatalf("deriveHKDFKey failed: %v", err)
	}

	if len(key1) != 32 {
		t.Errorf("key length: got %d, want 32", len(key1))
	}

	// Derive same key again (deterministic)
	key2, _ := deriveHKDFKey(masterKey, salt)
	if !bytes.Equal(key1, key2) {
		t.Errorf("HKDF derived keys differ for same inputs")
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
