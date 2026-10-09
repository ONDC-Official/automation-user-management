package utils

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"strings"
	"time"
)

const (
	// MCPKeyPrefix is the fixed, non-secret label every key carries. It makes a
	// leaked key recognisable on sight and gives secret scanners a pattern to
	// match: ondc_mcp_[A-Za-z0-9_-]{43}
	MCPKeyPrefix = "ondc_mcp_"

	// mcpKeyRandomBytes is the entropy behind each key. 32 bytes = 256 bits.
	mcpKeyRandomBytes = 32

	// mcpKeySecretLen is the length of the random part. 32 bytes in unpadded
	// base64url is always exactly 43 chars, so this doubles as a free prescreen
	// before any database work. Keep in step with mcpKeyRandomBytes.
	mcpKeySecretLen = 43

	// mcpKeyHintChars is how much of the random part the UI may see. Section 4
	// of the MCP consent document tells the user we store "the first few
	// characters of your key"; this is that. 6 base64 chars is 36 of 256 bits,
	// which is no help to an attacker and enough for a human to recognise their
	// own key.
	mcpKeyHintChars = 6
)

// GenerateMCPKey returns a new plaintext key. The caller must hash it with
// HashMCPKey before storing, must return it to the user exactly once, and must
// never log it.
func GenerateMCPKey() (string, error) {
	b := make([]byte, mcpKeyRandomBytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	// RawURLEncoding, not URLEncoding: the padding '=' that GenerateRandomState
	// produces has no business in a credential that gets pasted into shells,
	// .env files and JSON bodies.
	return MCPKeyPrefix + base64.RawURLEncoding.EncodeToString(b), nil
}

// HashMCPKey returns the lowercase hex SHA-256 of a plaintext key. This is the
// only form of a key that is ever persisted, and both issuance and verification
// must go through it or they will disagree.
//
// SHA-256 rather than bcrypt/argon2 deliberately: the input is 32 bytes of
// CSPRNG output, so there is no dictionary for a slow KDF to slow an attacker
// down over, and a KDF's cost would land on every single MCP tool call.
//
// No constant-time comparison is involved anywhere in the key path: we look a
// key up by this digest on a unique index rather than comparing a stored secret
// byte by byte, so there is no timing oracle to defend against. (The
// X-Service-Token check in middleware is a different matter -- see there.)
//
// Hex rather than base64 so the value is case-stable, free of '+/=' in BSON and
// JSON, and reproducible by hand:
//
//	printf '%s' "$KEY" | shasum -a 256
func HashMCPKey(key string) string {
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:])
}

// MCPKeyHint returns the non-secret display string stored alongside the hash,
// e.g. "ondc_mcp_AbC1x2". Safe to show in a UI and safe to log.
func MCPKeyHint(key string) string {
	secret := strings.TrimPrefix(key, MCPKeyPrefix)
	if len(secret) > mcpKeyHintChars {
		secret = secret[:mcpKeyHintChars]
	}
	return MCPKeyPrefix + secret
}

// LooksLikeMCPKey screens structurally impossible input so that garbage never
// reaches the database. /mcp/verify is a public endpoint and will be probed, so
// rejecting on shape first keeps junk off the Mongo index.
//
// It inspects only the public shape of a key -- prefix, length, alphabet -- so
// it is not, and does not need to be, constant-time.
func LooksLikeMCPKey(key string) bool {
	if !strings.HasPrefix(key, MCPKeyPrefix) {
		return false
	}
	secret := key[len(MCPKeyPrefix):]
	if len(secret) != mcpKeySecretLen {
		return false
	}
	for i := 0; i < len(secret); i++ {
		switch ch := secret[i]; {
		case ch >= 'A' && ch <= 'Z', ch >= 'a' && ch <= 'z',
			ch >= '0' && ch <= '9', ch == '-', ch == '_':
		default:
			return false
		}
	}
	return true
}

// MCPKeyExpiry returns the moment a key issued at now stops being valid.
func MCPKeyExpiry(now time.Time, ttlDays int) time.Time {
	return now.AddDate(0, 0, ttlDays)
}
