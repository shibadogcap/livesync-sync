package crypto

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"strings"

	"golang.org/x/crypto/hkdf"
	"golang.org/x/crypto/pbkdf2"
)

// PathObfuscator provides path obfuscation compatible with obsidian-livesync.
// Supports V2 (HMAC-based, non-reversible, default) and V1 (AES-GCM-based, reversible).
//
// V2 format:  "%/\\" + base64url(HMAC)
// V1 format:  "%" + hex(IV) + hex(Salt) + base64(ciphertext)
type PathObfuscator struct {
	Passphrase string
	UseV2      bool // true = V2 (HMAC, default), false = V1 (AES, legacy)
}

const (
	obfV2Prefix = "%/\\"
)

// NewPathObfuscator creates a new path obfuscator.
func NewPathObfuscator(passphrase string, useV2 bool) *PathObfuscator {
	return &PathObfuscator{
		Passphrase: passphrase,
		UseV2:      useV2,
	}
}

// Obfuscate obfuscates a path string.
// Returns the obfuscated path ready to be used as a CouchDB document ID suffix.
func (po *PathObfuscator) Obfuscate(path string) (string, error) {
	lowerPath := strings.ToLower(path)

	if po.UseV2 {
		return po.obfuscateV2(lowerPath)
	}
	return po.obfuscateV1(lowerPath)
}

// Deobfuscate reverses path obfuscation (V1 only; V2 is non-reversible).
func (po *PathObfuscator) Deobfuscate(obfuscated string) (string, error) {
	if strings.HasPrefix(obfuscated, obfV2Prefix) {
		// V2 is non-reversible; return as-is (caller must match by hash)
		// In practice, the plugin uses the path stored in the document metadata
		return obfuscated, nil
	}
	if strings.HasPrefix(obfuscated, "%") {
		return po.deobfuscateV1(obfuscated)
	}
	return obfuscated, nil
}

// obfuscateV2 implements V2 path obfuscation (non-reversible, HMAC-based).
// Algorithm:
//  1. HKDF(passphrase, hkdfSalt) → HMAC key
//  2. HMAC-SHA256(path) → digest
//  3. Output: "%/\\" + base64url(digest)
func (po *PathObfuscator) obfuscateV2(path string) (string, error) {
	// Generate HKDF salt from passphrase hash
	passHash := sha256.Sum256([]byte(po.Passphrase))
	hkdfSalt := passHash[:16]

	// PBKDF2 key material
	keyMaterial := pbkdf2.Key([]byte(po.Passphrase), hkdfSalt, 310000, 32, sha256.New)

	// HKDF to derive HMAC key
	hkdfReader := hkdf.New(sha256.New, keyMaterial, hkdfSalt, nil)
	hmacKey := make([]byte, 32)
	if _, err := hkdfReader.Read(hmacKey); err != nil {
		return "", err
	}

	// HMAC-SHA256 of the path
	mac := hmac.New(sha256.New, hmacKey)
	mac.Write([]byte(path))
	digest := mac.Sum(nil)

	// Output: "%/\\" + base64url(digest)
	return obfV2Prefix + base64.RawURLEncoding.EncodeToString(digest), nil
}

// obfuscateV1 implements V1 path obfuscation (reversible, AES-GCM-based).
// Algorithm:
//  1. SHA-256(passphrase) → key material
//  2. SHA-256(path + passphrase) → deterministic salt + IV
//  3. PBKDF2(key, salt) → AES key
//  4. AES-256-GCM encrypt(path, IV) → ciphertext
//  5. Output: "%" + hex(IV) + hex(Salt) + base64(ciphertext)
func (po *PathObfuscator) obfuscateV1(path string) (string, error) {
	// Deterministic salt + IV from SHA-256(path + passphrase)
	combined := append([]byte(path), []byte(po.Passphrase)...)
	hash := sha256.Sum256(combined)
	salt := hash[:16]
	iv := hash[16:28] // 12 bytes

	// Key material from passphrase
	passHash := sha256.Sum256([]byte(po.Passphrase))
	key := pbkdf2.Key(passHash[:], salt, 100000, 32, sha256.New)

	// AES-256-GCM encrypt
	ciphertext, err := aesGCMEncrypt(key, iv, []byte(path))
	if err != nil {
		return "", err
	}

	// Output
	ivHex := hex.EncodeToString(iv)
	saltHex := hex.EncodeToString(salt)
	b64 := base64.StdEncoding.EncodeToString(ciphertext)

	return "%" + ivHex + saltHex + b64, nil
}

// deobfuscateV1 reverses V1 path obfuscation.
func (po *PathObfuscator) deobfuscateV1(obfuscated string) (string, error) {
	withoutPrefix := strings.TrimPrefix(obfuscated, "%")

	// Expected format: hex(IV=24chars) + hex(Salt=32chars) + base64(ciphertext)
	if len(withoutPrefix) < 56 {
		return obfuscated, nil
	}

	ivHex := withoutPrefix[:24]
	saltHex := withoutPrefix[24:56]
	b64Data := withoutPrefix[56:]

	iv, err := hex.DecodeString(ivHex)
	if err != nil {
		return obfuscated, nil
	}
	salt, err := hex.DecodeString(saltHex)
	if err != nil {
		return obfuscated, nil
	}
	ciphertext, err := base64.StdEncoding.DecodeString(b64Data)
	if err != nil {
		return obfuscated, nil
	}

	// Re-derive key
	passHash := sha256.Sum256([]byte(po.Passphrase))
	key := pbkdf2.Key(passHash[:], salt, 100000, 32, sha256.New)

	// Decrypt
	plaintext, err := aesGCMDecrypt(key, iv, ciphertext)
	if err != nil {
		return obfuscated, nil
	}

	return string(plaintext), nil
}
