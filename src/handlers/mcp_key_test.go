package handlers

import (
	"testing"
	"time"

	"automation-developer-guide/src/models"
)

func TestValidateConsent_Accepted(t *testing.T) {
	reason := validateConsent(models.GenerateMCPKeyRequest{
		ConsentVersion:  "2026-10-v1",
		ConsentAccepted: true,
	}, "2026-10-v1")
	if reason != "" {
		t.Fatalf("expected consent to pass, got %q", reason)
	}
}

// Acceptance is checked before the version: somebody who ticked nothing should
// be told that, not sent round a version-refresh loop.
func TestValidateConsent_DeclinedBeatsVersionMismatch(t *testing.T) {
	reason := validateConsent(models.GenerateMCPKeyRequest{
		ConsentVersion:  "ancient",
		ConsentAccepted: false,
	}, "2026-10-v1")
	if reason != consentDeclined {
		t.Fatalf("expected %q, got %q", consentDeclined, reason)
	}
}

func TestValidateConsent_VersionMismatch(t *testing.T) {
	reason := validateConsent(models.GenerateMCPKeyRequest{
		ConsentVersion:  "2026-09-v1",
		ConsentAccepted: true,
	}, "2026-10-v1")
	if reason != consentVersionMismatch {
		t.Fatalf("expected %q, got %q", consentVersionMismatch, reason)
	}
}

func TestClassifyMCPKey_Healthy(t *testing.T) {
	now := time.Now().UTC()
	reason := classifyMCPKey(models.MCPKey{ExpiresAt: now.Add(time.Hour)}, now)
	if reason != "" {
		t.Fatalf("expected a usable key, got %q", reason)
	}
}

func TestClassifyMCPKey_Expired(t *testing.T) {
	now := time.Now().UTC()
	reason := classifyMCPKey(models.MCPKey{ExpiresAt: now.Add(-time.Hour)}, now)
	if reason != reasonExpired {
		t.Fatalf("expected %q, got %q", reasonExpired, reason)
	}
}

// expires_at is exclusive: at exactly that instant the key is gone.
func TestClassifyMCPKey_ExpiryBoundaryIsExclusive(t *testing.T) {
	now := time.Now().UTC()
	reason := classifyMCPKey(models.MCPKey{ExpiresAt: now}, now)
	if reason != reasonExpired {
		t.Fatalf("expected %q at the exact expiry instant, got %q", reasonExpired, reason)
	}
}

// A document with no expiry is corrupt, and that is not a reason to honour it
// forever.
func TestClassifyMCPKey_MissingExpiryFailsClosed(t *testing.T) {
	reason := classifyMCPKey(models.MCPKey{}, time.Now().UTC())
	if reason != reasonExpired {
		t.Fatalf("expected %q for a zero expiry, got %q", reasonExpired, reason)
	}
}

// Revocation is the more specific and more actionable fact, so it wins.
func TestClassifyMCPKey_RevokedBeatsExpired(t *testing.T) {
	now := time.Now().UTC()
	revoked := now.Add(-2 * time.Hour)
	reason := classifyMCPKey(models.MCPKey{
		RevokedAt: &revoked,
		ExpiresAt: now.Add(-time.Hour),
	}, now)
	if reason != reasonRevoked {
		t.Fatalf("expected %q, got %q", reasonRevoked, reason)
	}
}

// The UI needs the current consent version even when there is no key, so it can
// build a generate request the server will accept.
func TestBuildMCPKeyStatus_NoKeyStillCarriesConsentVersion(t *testing.T) {
	status := buildMCPKeyStatus(models.MCPKey{}, false, time.Now().UTC(), "2026-10-v1")
	if status.HasKey {
		t.Fatalf("expected has_key false, got %+v", status)
	}
	if status.CurrentConsentVersion != "2026-10-v1" {
		t.Fatalf("expected the current consent version, got %+v", status)
	}
}

// This is the no-TTL-index decision expressed as a regression test: add a TTL
// index on expires_at and this is what breaks.
func TestBuildMCPKeyStatus_ExpiredKeyIsStillReported(t *testing.T) {
	now := time.Now().UTC()
	status := buildMCPKeyStatus(models.MCPKey{
		KeyHint:        "ondc_mcp_AbC1x2",
		ConsentVersion: "2026-10-v1",
		CreatedAt:      now.Add(-100 * 24 * time.Hour),
		ExpiresAt:      now.Add(-10 * 24 * time.Hour),
	}, true, now, "2026-10-v1")

	// has_key stays true so the UI can say "expired, generate a new one"
	// instead of "you have never had a key".
	if !status.HasKey || !status.Expired {
		t.Fatalf("expected an expired-but-present key, got %+v", status)
	}
}

// A revoked key reads the same as no key: the user asked for it to stop
// existing, so the UI should offer "generate", not "your key is dead".
func TestBuildMCPKeyStatus_RevokedReadsAsNoKey(t *testing.T) {
	now := time.Now().UTC()
	revoked := now.Add(-time.Hour)
	status := buildMCPKeyStatus(models.MCPKey{
		KeyHint:   "ondc_mcp_AbC1x2",
		RevokedAt: &revoked,
		ExpiresAt: now.Add(time.Hour),
	}, true, now, "2026-10-v1")

	if status.HasKey || status.KeyHint != "" {
		t.Fatalf("expected a revoked key to read as absent, got %+v", status)
	}
}

func TestBuildMCPKeyStatus_HealthyKey(t *testing.T) {
	now := time.Now().UTC()
	lastUsed := now.Add(-time.Minute)
	status := buildMCPKeyStatus(models.MCPKey{
		KeyHint:        "ondc_mcp_AbC1x2",
		ConsentVersion: "2026-10-v1",
		CreatedAt:      now.Add(-24 * time.Hour),
		ExpiresAt:      now.Add(89 * 24 * time.Hour),
		LastUsedAt:     &lastUsed,
	}, true, now, "2026-10-v1")

	if !status.HasKey || status.Expired {
		t.Fatalf("expected a healthy key, got %+v", status)
	}
	if status.KeyHint != "ondc_mcp_AbC1x2" {
		t.Fatalf("expected the hint to be carried over, got %+v", status)
	}
	if status.CreatedAt == nil || status.ExpiresAt == nil || status.LastUsedAt == nil {
		t.Fatalf("expected all timestamps populated, got %+v", status)
	}
}

// The frontend has to handle a null last_used_at, so it must survive as nil
// rather than becoming a year-1 zero time.
func TestBuildMCPKeyStatus_NeverUsedStaysNil(t *testing.T) {
	now := time.Now().UTC()
	status := buildMCPKeyStatus(models.MCPKey{
		ExpiresAt: now.Add(time.Hour),
	}, true, now, "2026-10-v1")
	if status.LastUsedAt != nil {
		t.Fatalf("expected last_used_at to stay nil, got %v", *status.LastUsedAt)
	}
}

func TestTruncateString(t *testing.T) {
	if got := truncateString("abc", 10); got != "abc" {
		t.Fatalf("expected passthrough, got %q", got)
	}
	if got := truncateString("abcdef", 3); got != "abc" {
		t.Fatalf("expected truncation to 3, got %q", got)
	}
}
