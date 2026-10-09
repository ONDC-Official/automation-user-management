package utils

import (
	"strings"
	"testing"
	"time"
)

func TestGenerateMCPKey_Format(t *testing.T) {
	key, err := GenerateMCPKey()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.HasPrefix(key, MCPKeyPrefix) {
		t.Fatalf("expected prefix %q, got %q", MCPKeyPrefix, key)
	}
	// 32 bytes unpadded base64url is always 43 chars.
	if want := len(MCPKeyPrefix) + 43; len(key) != want {
		t.Fatalf("expected length %d, got %d (%q)", want, len(key), key)
	}
	// Padding in a credential is a trap for anyone pasting it into a shell.
	if strings.Contains(key, "=") {
		t.Fatalf("key must not contain base64 padding: %q", key)
	}
}

func TestGenerateMCPKey_Unique(t *testing.T) {
	seen := make(map[string]struct{}, 1000)
	for i := 0; i < 1000; i++ {
		key, err := GenerateMCPKey()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if _, dup := seen[key]; dup {
			t.Fatalf("duplicate key generated: %q", key)
		}
		seen[key] = struct{}{}
	}
}

// The highest-value test here: it catches a padding, length or alphabet
// disagreement between the generator and the validator, which is exactly the
// bug that would make every issued key fail verification.
func TestGeneratedKeyPassesFormatCheck(t *testing.T) {
	for i := 0; i < 100; i++ {
		key, err := GenerateMCPKey()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !LooksLikeMCPKey(key) {
			t.Fatalf("generated key rejected by LooksLikeMCPKey: %q", key)
		}
	}
}

// Pins hex, lowercase and the absence of any pepper, so nobody can "optimise"
// the encoding without this failing.
func TestHashMCPKey_KnownVector(t *testing.T) {
	const emptySHA256 = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	if got := HashMCPKey(""); got != emptySHA256 {
		t.Fatalf("expected %s, got %s", emptySHA256, got)
	}
}

func TestHashMCPKey_Deterministic(t *testing.T) {
	const key = "ondc_mcp_abcdefghijklmnopqrstuvwxyz0123456789ABCDEF"
	first, second := HashMCPKey(key), HashMCPKey(key)
	if first != second {
		t.Fatalf("hash not stable: %s vs %s", first, second)
	}
	if len(first) != 64 {
		t.Fatalf("expected 64 hex chars, got %d", len(first))
	}
}

func TestHashMCPKey_DiffersByInput(t *testing.T) {
	a := HashMCPKey("ondc_mcp_aaaa")
	b := HashMCPKey("ondc_mcp_aaab")
	if a == b {
		t.Fatal("expected different hashes for a one-character difference")
	}
}

func TestMCPKeyHint_HidesSecret(t *testing.T) {
	key, err := GenerateMCPKey()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	hint := MCPKeyHint(key)

	if want := len(MCPKeyPrefix) + 6; len(hint) != want {
		t.Fatalf("expected hint length %d, got %d (%q)", want, len(hint), hint)
	}
	if !strings.HasPrefix(key, hint) {
		t.Fatalf("hint %q is not a prefix of key", hint)
	}
	// The whole point: the hint must not be enough to reconstruct the key.
	if hint == key {
		t.Fatal("hint must not equal the key")
	}
}

func TestLooksLikeMCPKey(t *testing.T) {
	valid := "ondc_mcp_" + strings.Repeat("a", 43)

	cases := []struct {
		name string
		key  string
		want bool
	}{
		{"valid", valid, true},
		{"valid with url-safe alphabet", "ondc_mcp_" + strings.Repeat("-_9Z", 10) + "abc", true},
		{"empty", "", false},
		{"prefix only", MCPKeyPrefix, false},
		{"wrong prefix", "ondc_key_" + strings.Repeat("a", 43), false},
		{"no prefix", strings.Repeat("a", 43), false},
		{"too short", "ondc_mcp_" + strings.Repeat("a", 42), false},
		{"too long", "ondc_mcp_" + strings.Repeat("a", 44), false},
		{"base64 padding", "ondc_mcp_" + strings.Repeat("a", 42) + "=", false},
		{"base64 std plus", "ondc_mcp_" + strings.Repeat("a", 42) + "+", false},
		{"base64 std slash", "ondc_mcp_" + strings.Repeat("a", 42) + "/", false},
	}

	for _, tc := range cases {
		if got := LooksLikeMCPKey(tc.key); got != tc.want {
			t.Fatalf("%s: expected %v for %q, got %v", tc.name, tc.want, tc.key, got)
		}
	}
}

func TestMCPKeyExpiry(t *testing.T) {
	now := time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC)
	got := MCPKeyExpiry(now, 90)
	want := time.Date(2027, 1, 6, 12, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
}
