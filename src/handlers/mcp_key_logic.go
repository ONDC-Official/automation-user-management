package handlers

import (
	"time"

	"automation-developer-guide/src/models"

	"github.com/gofiber/fiber/v2"
)

const (
	mcpKeysCollection = "mcp_keys"

	// maxMCPKeyWriteTries is 2: one attempt to lose a race, one to win it.
	// See HandleGenerateMCPKey for the race this covers.
	maxMCPKeyWriteTries = 2

	// mcpUserAgentMaxLen bounds what we store from the User-Agent header. It is
	// attacker-controlled free text and there is no reason to accept a blob.
	mcpUserAgentMaxLen = 512
)

// Machine-readable rejection reasons returned to the MCP server, so it can tell
// the user "regenerate your key" rather than "something went wrong".
const (
	reasonNotFound      = "not_found"
	reasonRevoked       = "revoked"
	reasonExpired       = "expired"
	reasonInvalidFormat = "invalid_format"
	reasonUserNotFound  = "user_not_found"
)

// Consent rejection reasons for the generate endpoint.
const (
	consentDeclined        = "consent_declined"
	consentVersionMismatch = "consent_version_mismatch"
)

// validateConsent checks submitted consent against the version this server is
// currently serving. Returns "" when the consent is good.
//
// Acceptance is checked before the version deliberately: somebody who ticked
// nothing should be told that, not sent round a version-refresh loop.
func validateConsent(req models.GenerateMCPKeyRequest, currentVersion string) string {
	if !req.ConsentAccepted {
		return consentDeclined
	}
	if req.ConsentVersion != currentVersion {
		return consentVersionMismatch
	}
	return ""
}

// classifyMCPKey returns "" when the key is usable, or the reason it is not.
//
// A missing expiry fails closed: a document without expires_at is either
// corrupt or predates the field, and neither is a reason to honour it forever.
func classifyMCPKey(k models.MCPKey, now time.Time) string {
	if k.RevokedAt != nil {
		return reasonRevoked
	}
	// expires_at is exclusive: at exactly that instant the key is expired.
	if k.ExpiresAt.IsZero() || !now.Before(k.ExpiresAt) {
		return reasonExpired
	}
	return ""
}

// buildMCPKeyStatus renders the status payload from whatever the database held.
// found is false when the user has no document at all.
func buildMCPKeyStatus(k models.MCPKey, found bool, now time.Time, currentConsentVersion string) models.MCPKeyStatusResponse {
	resp := models.MCPKeyStatusResponse{CurrentConsentVersion: currentConsentVersion}

	// A revoked key reads the same as no key: the user asked for it to stop
	// existing, so the UI should offer "generate", not "your key is dead".
	if !found || k.RevokedAt != nil {
		return resp
	}

	createdAt, expiresAt := k.CreatedAt, k.ExpiresAt
	resp.HasKey = true
	resp.KeyHint = k.KeyHint
	resp.ConsentVersion = k.ConsentVersion
	resp.CreatedAt = &createdAt
	resp.ExpiresAt = &expiresAt
	resp.LastUsedAt = k.LastUsedAt
	// An expired key still reports has_key:true, so the UI can say "this expired
	// on the 4th, generate a new one" rather than "you have never had a key".
	// That is the whole reason there is no TTL index on expires_at -- see
	// database.EnsureMCPKeyIndexes.
	resp.Expired = classifyMCPKey(k, now) == reasonExpired
	return resp
}

// clientIP returns the address to record on a consent record.
//
// main.go sets ProxyHeader, so c.IP() reads X-Forwarded-For -- and returns ""
// when that header is absent, rather than falling back to the socket. Absent is
// the normal case for a direct request (local development, a port-forward, a
// health probe), and an empty consent_ip would make section 4 of the MCP
// consent document untrue. Fall back to the peer address.
func clientIP(c *fiber.Ctx) string {
	if ip := c.IP(); ip != "" {
		return ip
	}
	return c.Context().RemoteIP().String()
}

// truncateString bounds attacker-controlled text before it is stored.
func truncateString(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max]
}
