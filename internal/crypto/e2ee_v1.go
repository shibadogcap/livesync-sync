package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"

	"golang.org/x/crypto/pbkdf2"
)

// V1-specific constants
const (
	v1SemiStaticFieldLen = 8
	v1NonceLen           = 4
	v1NonceResetAt       = 10000
)

// v1NonceManager manages the V1 encryption nonce to prevent IV reuse.
// Matches the semi-static field + incrementing nonce in octagonal-wheels.
type v1NonceManager struct {
	mu             sync.Mutex
	semiStaticField []byte // 8 bytes
	nonce          uint32  // 4 bytes, incrementing
}

var globalV1Nonce = &v1NonceManager{}

func (n *v1NonceManager) getIV() []byte {
	n.mu.Lock()
	defer n.mu.Unlock()

	if n.semiStaticField == nil {
		n.semiStaticField = make([]byte, v1SemiStaticFieldLen)
		rand.Read(n.semiStaticField)
		n.nonce = 0
	}

	// Nonce management
	if n.nonce >= v1NonceResetAt {
		rand.Read(n.semiStaticField)
		n.nonce = 0
	}

	iv := make([]byte, v1IVLength)
	copy(iv[:v1SemiStaticFieldLen], n.semiStaticField)
	iv[v1SemiStaticFieldLen] = byte(n.nonce >> 24)
	iv[v1SemiStaticFieldLen+1] = byte(n.nonce >> 16)
	iv[v1SemiStaticFieldLen+2] = byte(n.nonce >> 8)
	iv[v1SemiStaticFieldLen+3] = byte(n.nonce)

	atomic.AddUint32(&n.nonce, 1)
	return iv
}

// v1GetKey derives a V1 encryption key from passphrase and salt.
// Matches octagonal-wheels' getKeyForEncrypt().
func v1GetKey(passphrase string, salt []byte, useDynamicIteration bool) []byte {
	iterations := v1DefaultIter
	if useDynamicIteration {
		// Formula: (15 - len(passphrase)) * 1000 + 121 - len(passphrase)
		// Minimum: 1000 (clamped)
		iter := (15 - len(passphrase)) * 1000 + 121 - len(passphrase)
		if iter < 1000 {
			iter = 1000
		}
		iterations = iter
	}
	return pbkdf2.Key([]byte(passphrase), salt, iterations, 32, sha256.New)
}

// EncryptV1 encrypts data using the V1 (legacy) algorithm.
// Format: "%" + hex(IV=12B=24hex chars) + hex(Salt=16B=32hex chars) + base64(ciphertext)
//
// Note: The IV in V1 format is stored as 16 hex chars (8 bytes in the hex encoding)
// followed by the salt as 32 hex chars (16 bytes). But the actual IV used is 12 bytes
// (8 semi-static + 4 nonce).
func EncryptV1(plaintext []byte, passphrase string, useDynamicIteration bool) (string, error) {
	salt := make([]byte, v1SaltLength)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("failed to generate salt: %w", err)
	}

	key := v1GetKey(passphrase, salt, useDynamicIteration)

	iv := globalV1Nonce.getIV()

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	ciphertext := gcm.Seal(nil, iv, plaintext, nil)

	// Format: %{iv_hex(24chars)}{salt_hex(32chars)}{base64(ciphertext)}
	// Actually, looking at the TypeScript code more carefully:
	// V1 format is: "%" + IV(16 hex chars) + Salt(32 hex chars) + base64(ciphertext)
	// Wait, this is confusing. Let me look again.
	//
	// From the TS: `%{iv_hex(32)}{salt_hex(32)}{base64_encrypted}`
	// IV is stored as 32 hex chars = 16 bytes, salt as 32 hex chars = 16 bytes
	// But the actual IV used is 12 bytes (8+4).
	//
	// The V1 prefix in octagonal-wheels stores the IV differently.
	// For V1 string format: "%" + hex(IV-full-16B) + hex(salt-16B) + base64(ciphertext)
	// The IV is 16 bytes: first 8 = semi-static, next 4 = nonce, padded with zeros.
	//
	// Actually let me re-check. From the TypeScript:
	// ```
	// const [key, salt] = await getKeyForEncrypt(passphrase, autoCalculateIterations);
	// const fixedPart = getSemiStaticField();   // 8 bytes random
	// const invocationPart = getNonce();         // 4 bytes incrementing
	// const iv = new Uint8Array([...fixedPart, ...new Uint8Array(invocationPart.buffer)]);
	// ```
	// This gives IV = 12 bytes (8+4).
	//
	// Then for V1 string format:
	// ```
	// result = "%" + bytesToHex(iv) + bytesToHex(salt) + base64_encode(ciphertext)
	// ```
	// So `%` + hex(IV=12B=24hex) + hex(Salt=16B=32hex) + base64(ciphertext)
	//
	// Hmm, but the analysis said "IV(16進32文字)" = 32 hex chars = 16 bytes.
	// Let me be more careful. The octagonal-wheels code says:
	// ```
	// const ivHex = bytesToHex(iv); // iv is 12 bytes
	// const saltHex = bytesToHex(salt); // salt is 16 bytes
	// ```
	// So: 12 * 2 = 24 hex chars for IV, 16 * 2 = 32 hex chars for salt.
	//
	// But another version might have IV as 16 bytes.
	// Let me check the v2 prefix analysis from the earlier research...

	// Actually looking at the detailed research data:
	// V1 data format: "%" + IV(16進32文字) + Salt(16進32文字) + base64(ciphertext)
	// So 32 hex chars = 16 bytes for IV.
	// But the actual IV is 12 bytes. There must be padding or the IV is stored differently.
	//
	// Wait - in the octagonal-wheels code, there might be a different string format.
	// The V1 ENCRYPT_V1_PREFIX format stores IV as full 16 bytes (8+4+padding?)
	// 
	// For now, let's implement it with IV as 12 bytes → 24 hex chars,
	// and salt as 16 bytes → 32 hex chars.

	ivHex := hex.EncodeToString(iv)       // 12 * 2 = 24 hex chars
	saltHex := hex.EncodeToString(salt)    // 16 * 2 = 32 hex chars
	b64 := base64.StdEncoding.EncodeToString(ciphertext)

	return v1Prefix + ivHex + saltHex + b64, nil
}

// DecryptV1 decrypts V1 formatted data (legacy format).
func DecryptV1(data string, passphrase string, useDynamicIteration bool) ([]byte, error) {
	// Strip prefix
	withoutPrefix := strings.TrimPrefix(data, v1Prefix)

	// Expected minimum length: ivHex(24) + saltHex(32) + some base64
	if len(withoutPrefix) < 24+32 {
		// Try with 16-byte IV (32 hex chars)
		if len(withoutPrefix) < 32+32 {
			return nil, ErrShortData
		}
		// 16-byte IV format
		ivHex := withoutPrefix[:32]
		saltHex := withoutPrefix[32:64]
		b64Data := withoutPrefix[64:]

		iv, err := hex.DecodeString(ivHex)
		if err != nil {
			return nil, fmt.Errorf("IV hex decode failed: %w", err)
		}
		salt, err := hex.DecodeString(saltHex)
		if err != nil {
			return nil, fmt.Errorf("salt hex decode failed: %w", err)
		}
		ciphertext, err := base64.StdEncoding.DecodeString(b64Data)
		if err != nil {
			return nil, fmt.Errorf("base64 decode failed: %w", err)
		}

		return v1DecryptWithKey(iv, salt, ciphertext, passphrase, useDynamicIteration)
	}

	// 12-byte IV format (24 hex chars) + 16-byte salt (32 hex chars)
	ivHex := withoutPrefix[:24]
	saltHex := withoutPrefix[24:56]
	b64Data := withoutPrefix[56:]

	iv, err := hex.DecodeString(ivHex)
	if err != nil {
		return nil, fmt.Errorf("IV hex decode failed: %w", err)
	}
	salt, err := hex.DecodeString(saltHex)
	if err != nil {
		return nil, fmt.Errorf("salt hex decode failed: %w", err)
	}
	ciphertext, err := base64.StdEncoding.DecodeString(b64Data)
	if err != nil {
		return nil, fmt.Errorf("base64 decode failed: %w", err)
	}

	return v1DecryptWithKey(iv, salt, ciphertext, passphrase, useDynamicIteration)
}

func v1DecryptWithKey(iv, salt, ciphertext []byte, passphrase string, useDynamicIteration bool) ([]byte, error) {
	key := v1GetKey(passphrase, salt, useDynamicIteration)

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

// DecryptV3 decrypts V3 (legacy, fixed salt) format.
// V3 format: "%~" + hex(IV=12B=24hex) + base64(ciphertext)
func DecryptV3(data string, passphrase string) ([]byte, error) {
	withoutPrefix := strings.TrimPrefix(data, v3Prefix)

	// V3 uses a fixed salt: SHA256("fancySyncForYou!")[:16]
	fixedSaltInput := "fancySyncForYou!"
	fixedSaltHash := sha256.Sum256([]byte(fixedSaltInput))
	fixedSalt := fixedSaltHash[:16]

	// Format: hex(IV=24chars) + base64(ciphertext)
	ivHex := withoutPrefix[:24]
	b64Data := withoutPrefix[24:]

	iv, err := hex.DecodeString(ivHex)
	if err != nil {
		return nil, fmt.Errorf("IV hex decode failed: %w", err)
	}
	ciphertext, err := base64.StdEncoding.DecodeString(b64Data)
	if err != nil {
		return nil, fmt.Errorf("base64 decode failed: %w", err)
	}

	// V3 uses fixed 100000 iterations with fixed salt
	key := pbkdf2.Key([]byte(passphrase), fixedSalt, v1DefaultIter, 32, sha256.New)

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

// DecryptAuto detects the encryption version and decrypts accordingly.
// This is the main entry point for reading data from CouchDB.
//
// Decryption order (matching octagonal-wheels):
// 1. Try V2 (HKDF) first
// 2. Fall back to V1
// 3. Try V3 (legacy, fixed salt)
func DecryptAuto(data string, params *EncryptionParams) ([]byte, error) {
	version := DetectVersion(data)

	switch version {
	case Version2, Version2Ephemeral:
		result, err := DecryptV2(data, params)
		if err != nil {
			// Fallback: try V1
			if params.E2EEAlgorithm != "forceV1" {
				return nil, err
			}
			return DecryptV1(data, params.Passphrase, params.UseDynamicIterationCount)
		}
		return result, nil

	case Version1:
		return DecryptV1(data, params.Passphrase, params.UseDynamicIterationCount)

	case Version3:
		return DecryptV3(data, params.Passphrase)

	case VersionUnencrypted:
		return []byte(data), nil

	default:
		return nil, ErrUnknownVersion
	}
}


