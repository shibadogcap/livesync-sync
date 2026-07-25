// Package crypto provides E2EE encryption and decryption compatible with obsidian-livesync.
// Supports V2 (PBKDF2+HKDF+AES-256-GCM, default) and V1 (PBKDF2+AES-256-GCM, fallback).
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/hkdf"
	"golang.org/x/crypto/pbkdf2"
)

// EncryptionVersion represents the detected encryption format.
type EncryptionVersion int

const (
	VersionUnknown    EncryptionVersion = iota
	VersionUnencrypted
	Version1                             // % prefix
	Version2                             // %= prefix
	Version2Ephemeral                    // %$ prefix (includes PBKDF2 salt)
	Version3                             // %~ prefix (legacy)
)

// EncryptionParams holds the parameters for encryption/decryption.
type EncryptionParams struct {
	Passphrase              string
	PBKDF2Salt              []byte // 32 bytes, from SyncParameters
	E2EEAlgorithm           string // "v2" or "" or "forceV1"
	UseDynamicIterationCount bool
}

// EncryptedData represents parsed encrypted data with its components.
type EncryptedData struct {
	Version    EncryptionVersion
	IV         []byte // 12 bytes for V2/V3, 12 bytes for V1
	Salt       []byte // HKDF salt (32B) for V2, PBKDF2 salt (16B) for V1
	Ciphertext []byte
	// Ephemeral (V2 with embedded PBKDF2 salt)
	PBKDF2Salt []byte
}

// V2 constants
const (
	v2Prefix         = "%="
	v2SaltPrefix     = "%$"
	v2IVLength       = 12
	v2HKDFSaltLength = 32
	v2PBKDF2SaltLen  = 32
	v2Iterations     = 310000
)

// V1 constants
const (
	v1Prefix       = "%"
	v1IVHexLength  = 32 // 16 bytes as hex
	v1SaltHexLen   = 32 // 16 bytes as hex
	v1IVLength     = 12
	v1SaltLength   = 16
	v1DefaultIter  = 100000
)

// V3 prefix (legacy, decrypt only)
const v3Prefix = "%~"

// Common errors
var (
	ErrInvalidFormat  = errors.New("invalid encrypted data format")
	ErrUnknownVersion = errors.New("unknown encryption version")
	ErrShortData      = errors.New("data too short")
	ErrDecryptFailed  = errors.New("decryption failed")
)

// DetectVersion detects the encryption version from a data string.
func DetectVersion(data string) EncryptionVersion {
	switch {
	case strings.HasPrefix(data, v2SaltPrefix):
		return Version2Ephemeral
	case strings.HasPrefix(data, v2Prefix):
		return Version2
	case strings.HasPrefix(data, v3Prefix):
		return Version3
	case strings.HasPrefix(data, "["):
		// V1 JSON array format: ["base64(ciphertext)","hex(iv)","hex(salt)"]
		// This is the format produced by TS's encryptV1().
		return Version1
	case strings.HasPrefix(data, v1Prefix):
		return Version1
	default:
		return VersionUnencrypted
	}
}

// EncryptV2 encrypts data using the V2 (PBKDF2+HKDF+AES-256-GCM) algorithm.
// This is the default encryption mode for new documents.
//
// Key derivation:
//  1. PBKDF2-SHA256(passphrase, pbkdf2Salt, 310000 iterations) → masterKey (32B)
//  2. HKDF-SHA256(masterKey, hkdfSalt, info="") → chunkKey (32B)
//  3. AES-256-GCM(chunkKey, IV=12B) → ciphertext
//
// Output format: "%=` + base64(IV(12B) + HKDF-salt(32B) + ciphertext)
func EncryptV2(plaintext []byte, params *EncryptionParams) (string, error) {
	// Generate random IV (12 bytes)
	iv := make([]byte, v2IVLength)
	if _, err := rand.Read(iv); err != nil {
		return "", fmt.Errorf("failed to generate IV: %w", err)
	}

	// Generate random HKDF salt (32 bytes)
	hkdfSalt := make([]byte, v2HKDFSaltLength)
	if _, err := rand.Read(hkdfSalt); err != nil {
		return "", fmt.Errorf("failed to generate HKDF salt: %w", err)
	}

	// Step 1: PBKDF2 to derive master key
	masterKey := pbkdf2.Key([]byte(params.Passphrase), params.PBKDF2Salt, v2Iterations, 32, sha256.New)

	// Step 2: HKDF to derive chunk key
	chunkKey, err := deriveHKDFKey(masterKey, hkdfSalt)
	if err != nil {
		return "", fmt.Errorf("HKDF derivation failed: %w", err)
	}

	// Step 3: AES-256-GCM encrypt
	ciphertext, err := aesGCMEncrypt(chunkKey, iv, plaintext)
	if err != nil {
		return "", fmt.Errorf("AES-GCM encrypt failed: %w", err)
	}

	// Combine: IV + HKDF-salt + ciphertext → base64
	combined := make([]byte, 0, v2IVLength+v2HKDFSaltLength+len(ciphertext))
	combined = append(combined, iv...)
	combined = append(combined, hkdfSalt...)
	combined = append(combined, ciphertext...)

	return v2Prefix + base64.StdEncoding.EncodeToString(combined), nil
}

// EncryptV2Ephemeral encrypts data with embedded PBKDF2 salt (ephemeral mode).
// Output format: "%$" + base64(PBKDF2-salt(32B) + IV(12B) + HKDF-salt(32B) + ciphertext)
//
// This is used when the PBKDF2 salt needs to be embedded in the encrypted data
// itself (e.g., when the SyncParameters doc may not yet have a salt).
func EncryptV2Ephemeral(plaintext []byte, passphrase string) (string, error) {
	// Generate fresh PBKDF2 salt
	pbkdf2Salt := make([]byte, v2PBKDF2SaltLen)
	if _, err := rand.Read(pbkdf2Salt); err != nil {
		return "", fmt.Errorf("failed to generate PBKDF2 salt: %w", err)
	}

	iv := make([]byte, v2IVLength)
	if _, err := rand.Read(iv); err != nil {
		return "", fmt.Errorf("failed to generate IV: %w", err)
	}

	hkdfSalt := make([]byte, v2HKDFSaltLength)
	if _, err := rand.Read(hkdfSalt); err != nil {
		return "", fmt.Errorf("failed to generate HKDF salt: %w", err)
	}

	masterKey := pbkdf2.Key([]byte(passphrase), pbkdf2Salt, v2Iterations, 32, sha256.New)
	chunkKey, err := deriveHKDFKey(masterKey, hkdfSalt)
	if err != nil {
		return "", fmt.Errorf("HKDF derivation failed: %w", err)
	}

	ciphertext, err := aesGCMEncrypt(chunkKey, iv, plaintext)
	if err != nil {
		return "", fmt.Errorf("AES-GCM encrypt failed: %w", err)
	}

	// Combine: PBKDF2-salt + IV + HKDF-salt + ciphertext → base64
	combined := make([]byte, 0, v2PBKDF2SaltLen+v2IVLength+v2HKDFSaltLength+len(ciphertext))
	combined = append(combined, pbkdf2Salt...)
	combined = append(combined, iv...)
	combined = append(combined, hkdfSalt...)
	combined = append(combined, ciphertext...)

	return v2SaltPrefix + base64.StdEncoding.EncodeToString(combined), nil
}

// DecryptV2 decrypts data using the V2 algorithm (handles both %= and %$ prefixes).
func DecryptV2(data string, params *EncryptionParams) ([]byte, error) {
	enc, err := parseV2Data(data)
	if err != nil {
		return nil, err
	}

	// Determine PBKDF2 salt
	pbkdf2Salt := params.PBKDF2Salt
	if enc.Version == Version2Ephemeral && len(enc.PBKDF2Salt) > 0 {
		pbkdf2Salt = enc.PBKDF2Salt
	}

	if len(pbkdf2Salt) == 0 {
		return nil, errors.New("PBKDF2 salt is required for V2 decryption")
	}

	// Step 1: PBKDF2 to derive master key
	masterKey := pbkdf2.Key([]byte(params.Passphrase), pbkdf2Salt, v2Iterations, 32, sha256.New)

	// Step 2: HKDF to derive chunk key
	chunkKey, err := deriveHKDFKey(masterKey, enc.Salt)
	if err != nil {
		return nil, fmt.Errorf("HKDF derivation failed: %w", err)
	}

	// Step 3: AES-256-GCM decrypt
	plaintext, err := aesGCMDecrypt(chunkKey, enc.IV, enc.Ciphertext)
	if err != nil {
		return nil, fmt.Errorf("AES-GCM decrypt failed: %w", err)
	}

	return plaintext, nil
}

// decryptV2WithChunkKey decrypts V2 data using a pre-derived chunk key.
// This is a lower-level function for compatibility.
// DecryptV2WithChunkKey decrypts V2 data using a pre-derived chunk key.
func DecryptV2WithChunkKey(enc *EncryptedData, chunkKey []byte) ([]byte, error) {
	return aesGCMDecrypt(chunkKey, enc.IV, enc.Ciphertext)
}

// parseV2Data parses V2 formatted encrypted data (%= or %$ prefixes).
func parseV2Data(data string) (*EncryptedData, error) {
	var raw string
	version := Version2

	switch {
	case strings.HasPrefix(data, v2SaltPrefix):
		raw = strings.TrimPrefix(data, v2SaltPrefix)
		version = Version2Ephemeral
	case strings.HasPrefix(data, v2Prefix):
		raw = strings.TrimPrefix(data, v2Prefix)
	default:
		return nil, ErrInvalidFormat
	}

	decoded, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return nil, fmt.Errorf("base64 decode failed: %w", err)
	}

	enc := &EncryptedData{Version: version}

	if version == Version2Ephemeral {
		// Format: PBKDF2-salt(32B) + IV(12B) + HKDF-salt(32B) + ciphertext
		if len(decoded) < v2PBKDF2SaltLen+v2IVLength+v2HKDFSaltLength {
			return nil, ErrShortData
		}
		enc.PBKDF2Salt = decoded[:v2PBKDF2SaltLen]
		enc.IV = decoded[v2PBKDF2SaltLen : v2PBKDF2SaltLen+v2IVLength]
		enc.Salt = decoded[v2PBKDF2SaltLen+v2IVLength : v2PBKDF2SaltLen+v2IVLength+v2HKDFSaltLength]
		enc.Ciphertext = decoded[v2PBKDF2SaltLen+v2IVLength+v2HKDFSaltLength:]
	} else {
		// Format: IV(12B) + HKDF-salt(32B) + ciphertext
		if len(decoded) < v2IVLength+v2HKDFSaltLength {
			return nil, ErrShortData
		}
		enc.IV = decoded[:v2IVLength]
		enc.Salt = decoded[v2IVLength : v2IVLength+v2HKDFSaltLength]
		enc.Ciphertext = decoded[v2IVLength+v2HKDFSaltLength:]
	}

	return enc, nil
}

// deriveHKDFKey derives a 32-byte AES key from a master key using HKDF-SHA256.
func deriveHKDFKey(masterKey, salt []byte) ([]byte, error) {
	hkdfReader := hkdf.New(sha256.New, masterKey, salt, nil) // info = empty
	chunkKey := make([]byte, 32)
	if _, err := hkdfReader.Read(chunkKey); err != nil {
		return nil, err
	}
	return chunkKey, nil
}

// aesGCMEncrypt encrypts plaintext with AES-256-GCM.
func aesGCMEncrypt(key, iv, plaintext []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	// Seal appends the ciphertext+tag and returns the result
	ciphertext := gcm.Seal(nil, iv, plaintext, nil)
	return ciphertext, nil
}

// aesGCMDecrypt decrypts ciphertext with AES-256-GCM.
func aesGCMDecrypt(key, iv, ciphertext []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	plaintext, err := gcm.Open(nil, iv, ciphertext, nil)
	if err != nil {
		return nil, ErrDecryptFailed
	}

	return plaintext, nil
}

// computeHMACSHA256 computes HMAC-SHA256 of data with the given key.
func computeHMACSHA256(key, data []byte) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write(data)
	return mac.Sum(nil)
}
