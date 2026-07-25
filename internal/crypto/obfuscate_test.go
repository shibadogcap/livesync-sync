package crypto

import (
	"strings"
	"testing"
)

func TestPathObfuscateV2(t *testing.T) {
	po := NewPathObfuscator("test-obfuscate-passphrase", true) // V2 mode

	tests := []struct {
		path string
	}{
		{"notes/document.md"},
		{"journal/2026/07/25.md"},
		{"_config/test.md"},
		{"folder/with spaces.md"},
		{"a/b/c/d/e/file.md"},
		{"単一ファイル.md"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			obfuscated, err := po.Obfuscate(tt.path)
			if err != nil {
				t.Fatalf("Obfuscate(%q) failed: %v", tt.path, err)
			}

			// V2 format: "%/\\" + base64url(HMAC)
			if !strings.HasPrefix(obfuscated, obfV2Prefix) {
				t.Errorf("expected V2 prefix %q, got %q", obfV2Prefix, obfuscated[:min(4, len(obfuscated))])
			}

			if len(obfuscated) <= len(obfV2Prefix) {
				t.Errorf("obfuscated string too short: %q", obfuscated)
			}

			// Deterministic: same path+passphrase → same output
			obfuscated2, _ := po.Obfuscate(tt.path)
			if obfuscated != obfuscated2 {
				t.Errorf("V2 obfuscation not deterministic")
			}

			// Different path → different output
			obfuscated3, _ := po.Obfuscate(tt.path + "x")
			if obfuscated == obfuscated3 {
				t.Errorf("different paths produced same obfuscation")
			}
		})
	}
}

func TestPathObfuscateV2Deterministic(t *testing.T) {
	po := NewPathObfuscator("same-passphrase", true)
	path := "always/the/same/path.md"

	out1, _ := po.Obfuscate(path)
	out2, _ := po.Obfuscate(path)

	if out1 != out2 {
		t.Errorf("V2 must be deterministic: %q != %q", out1, out2)
	}
}

func TestPathObfuscateV2DifferentPassphrase(t *testing.T) {
	po1 := NewPathObfuscator("passphrase-a", true)
	po2 := NewPathObfuscator("passphrase-b", true)
	path := "shared/path.md"

	out1, _ := po1.Obfuscate(path)
	out2, _ := po2.Obfuscate(path)

	if out1 == out2 {
		t.Errorf("different passphrases should produce different obfuscation")
	}
}

func TestPathObfuscateV1(t *testing.T) {
	po := NewPathObfuscator("test-v1-passphrase", false) // V1 mode

	tests := []struct {
		path string
	}{
		{"notes/document.md"},
		{"journal/entry.md"},
		{"_config/settings.yaml"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			obfuscated, err := po.Obfuscate(tt.path)
			if err != nil {
				t.Fatalf("Obfuscate V1(%q) failed: %v", tt.path, err)
			}

			// V1 format: "%" + data
			if !strings.HasPrefix(obfuscated, "%") {
				t.Errorf("expected V1 prefix '%%', got %q", obfuscated[:1])
			}

			// Deterministic
			obfuscated2, _ := po.Obfuscate(tt.path)
			if obfuscated != obfuscated2 {
				t.Errorf("V1 obfuscation not deterministic")
			}

			// Reversible
			deobfuscated, err := po.Deobfuscate(obfuscated)
			if err != nil {
				t.Fatalf("Deobfuscate V1(%q) failed: %v", obfuscated, err)
			}

			if deobfuscated != strings.ToLower(tt.path) {
				t.Errorf("V1 roundtrip: got %q, want %q", deobfuscated, strings.ToLower(tt.path))
			}
		})
	}
}

func TestPathObfuscateV2NonReversible(t *testing.T) {
	po := NewPathObfuscator("test-v2", true)
	path := "secret/document.md"

	obfuscated, _ := po.Obfuscate(path)
	deobfuscated, err := po.Deobfuscate(obfuscated)
	if err != nil {
		t.Fatalf("Deobfuscate should not error for V2: %v", err)
	}

	// V2 is non-reversible; deobfuscate returns the obfuscated string as-is
	if deobfuscated != obfuscated {
		t.Errorf("V2 deobfuscate should return obfuscated string as-is, got %q", deobfuscated)
	}
}

func TestPathDeobfuscateNonObfuscated(t *testing.T) {
	po := NewPathObfuscator("test", true)

	// Non-obfuscated paths pass through
	paths := []string{"plain/path.md", "", "f:somehash"}
	for _, p := range paths {
		result, err := po.Deobfuscate(p)
		if err != nil {
			t.Errorf("Deobfuscate(%q) should not error: %v", p, err)
		}
		if result != p {
			t.Errorf("Deobfuscate(%q) = %q, want passthrough", p, result)
		}
	}
}

func TestNewPathObfuscatorDefaults(t *testing.T) {
	po := NewPathObfuscator("test", true)
	if po.Passphrase != "test" {
		t.Errorf("Passphrase = %q, want %q", po.Passphrase, "test")
	}
	if !po.UseV2 {
		t.Errorf("UseV2 should default to true")
	}

	po2 := NewPathObfuscator("test", false)
	if po2.UseV2 {
		t.Errorf("UseV2 should be false")
	}
}
