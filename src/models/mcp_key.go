package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// MCPKey is the single active API key a user holds for the MCP server.
//
// Exactly one document exists per user (unique index on user_id); regenerating
// overwrites it in place rather than inserting a second row, which is what
// makes "regenerating revokes the previous key" true. The plaintext key is
// never stored -- only KeyHash, the lowercase hex SHA-256 of it.
type MCPKey struct {
	ID     primitive.ObjectID `json:"id" bson:"_id,omitempty"`
	UserID string             `json:"user_id" bson:"user_id"`

	// KeyHash is the lowercase hex SHA-256 of the plaintext key and the only
	// field /mcp/verify looks up. json:"-" so that returning this struct from a
	// handler by accident cannot leak it.
	KeyHash string `json:"-" bson:"key_hash"`

	// KeyHint is a non-secret display string, e.g. "ondc_mcp_AbC1x2", so the UI
	// can show which key the user holds without storing the key.
	KeyHint string `json:"key_hint" bson:"key_hint"`

	// Consent audit. Section 4 of the MCP consent document promises we record
	// the version agreed to, when, the user's IP address and their browser
	// details. All four are required for that statement to be true.
	ConsentVersion   string    `json:"consent_version" bson:"consent_version"`
	ConsentedAt      time.Time `json:"consented_at" bson:"consented_at"`
	ConsentIP        string    `json:"-" bson:"consent_ip"`
	ConsentUserAgent string    `json:"-" bson:"consent_user_agent"`

	CreatedAt time.Time `json:"created_at" bson:"created_at"`
	ExpiresAt time.Time `json:"expires_at" bson:"expires_at"`

	// RevokedAt is reserved. Revocation deletes the document outright (see
	// HandleRevokeMCPKey), so nothing sets this today. Verification still checks
	// it, cheaply, so that a document which ever acquires the field -- by hand,
	// by a migration, or by a future soft-revoke -- is refused rather than
	// honoured. Generate $unsets it for the same reason.
	RevokedAt *time.Time `json:"revoked_at,omitempty" bson:"revoked_at,omitempty"`

	// Pointer and absent until it happens: "never used" has to be
	// distinguishable from the zero time, which BSON would round-trip as year 1.
	LastUsedAt *time.Time `json:"last_used_at,omitempty" bson:"last_used_at,omitempty"`
}

// GenerateMCPKeyRequest is the POST body for issuing a key.
//
// ConsentAccepted carries no `validate:"required"` tag on purpose: to
// validator/v10 a plain false is the zero value and therefore indistinguishable
// from an absent field, so requiring it would reject "I declined" with a
// confusing schema error. The handler rejects a false explicitly instead, with a
// message that says what is actually wrong.
type GenerateMCPKeyRequest struct {
	ConsentVersion  string `json:"consent_version" validate:"required"`
	ConsentAccepted bool   `json:"consent_accepted"`
}

// GenerateMCPKeyResponse carries the plaintext key, which is returned exactly
// once and is not recoverable afterwards.
type GenerateMCPKeyResponse struct {
	Key            string    `json:"key"`
	KeyHint        string    `json:"key_hint"`
	ConsentVersion string    `json:"consent_version"`
	CreatedAt      time.Time `json:"created_at"`
	ExpiresAt      time.Time `json:"expires_at"`
}

// MCPKeyStatusResponse is metadata only and never contains the key.
//
// CurrentConsentVersion is always populated, including when the user has no key:
// it lets the UI notice a version mismatch before asking someone to read the
// whole document, rather than after, when generate would 409.
type MCPKeyStatusResponse struct {
	HasKey                bool       `json:"has_key"`
	CurrentConsentVersion string     `json:"current_consent_version"`
	KeyHint               string     `json:"key_hint,omitempty"`
	ConsentVersion        string     `json:"consent_version,omitempty"`
	CreatedAt             *time.Time `json:"created_at,omitempty"`
	ExpiresAt             *time.Time `json:"expires_at,omitempty"`
	LastUsedAt            *time.Time `json:"last_used_at,omitempty"`
	Expired               bool       `json:"expired"`
}

// VerifyMCPKeyRequest is the body the MCP server posts to /mcp/verify.
type VerifyMCPKeyRequest struct {
	Key string `json:"key"`
}

// VerifyMCPKeyResponse is the success shape. Failures use the house error
// convention ({valid:false, reason}) rather than this struct.
type VerifyMCPKeyResponse struct {
	Valid     bool      `json:"valid"`
	UserID    string    `json:"user_id"`
	Username  string    `json:"username"`
	Email     string    `json:"email"`
	ExpiresAt time.Time `json:"expires_at"`
}
