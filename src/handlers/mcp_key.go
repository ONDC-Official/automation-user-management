package handlers

import (
	"bytes"
	"encoding/json"
	"errors"
	"log"
	"strings"
	"time"

	"automation-developer-guide/src/config"
	"automation-developer-guide/src/database"
	"automation-developer-guide/src/models"
	"automation-developer-guide/src/utils"

	"github.com/go-playground/validator/v10"
	"github.com/gofiber/fiber/v2"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

var validateMCP = validator.New()

// HandleGetMCPKeyStatus returns metadata about the authenticated user's MCP key.
// It never returns the key or its hash, and it never 404s -- "no key" is a
// normal state the UI renders, not an error.
func HandleGetMCPKeyStatus(c *fiber.Ctx) error {
	userID, ok := c.Locals("user_id").(string)
	if !ok || userID == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}

	now := time.Now().UTC()

	var doc models.MCPKey
	err := database.FindOne(mcpKeysCollection, bson.M{"user_id": userID}, &doc)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return c.JSON(buildMCPKeyStatus(models.MCPKey{}, false, now, config.MCPConsentVersion))
		}
		log.Printf("mcp key status: lookup failed for user %s: %v", userID, err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to fetch key status"})
	}

	return c.JSON(buildMCPKeyStatus(doc, true, now, config.MCPConsentVersion))
}

// HandleGenerateMCPKey issues a new MCP key for the authenticated user,
// replacing any key they already hold. The plaintext is in the response body
// and nowhere else, now or ever.
func HandleGenerateMCPKey(c *fiber.Ctx) error {
	userID, ok := c.Locals("user_id").(string)
	if !ok || userID == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}

	// DisallowUnknownFields, matching notes.go and comments.go: a client that
	// sends consentVersion instead of consent_version should be told so, not
	// have its consent silently read as the empty string.
	var req models.GenerateMCPKeyRequest
	decoder := json.NewDecoder(bytes.NewReader(c.Body()))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid json or unknown fields"})
	}
	if err := validateMCP.Struct(req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	switch reason := validateConsent(req, config.MCPConsentVersion); reason {
	case "":
		// consent is good
	case consentVersionMismatch:
		// 409 with the live version, so the client can refetch the consent text
		// and retry rather than guess.
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{
			"error":                    reason,
			"expected_consent_version": config.MCPConsentVersion,
		})
	default:
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": reason})
	}

	now := time.Now().UTC()
	expiresAt := utils.MCPKeyExpiry(now, config.MCPKeyTTLDays)

	// Section 4 of the MCP consent document promises we record the IP address
	// and browser details alongside the consent, so capture them here.
	// clientIP prefers the forwarded address (main.go sets ProxyHeader) and
	// falls back to the socket when there is no proxy in front.
	consentIP := clientIP(c)
	consentUA := truncateString(c.Get(fiber.HeaderUserAgent), mcpUserAgentMaxLen)

	var resp models.GenerateMCPKeyResponse

	for attempt := 1; ; attempt++ {
		key, err := utils.GenerateMCPKey()
		if err != nil {
			log.Printf("mcp key generate: rand failure for user %s: %v", userID, err)
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to generate key"})
		}

		hint := utils.MCPKeyHint(key)

		update := bson.M{
			"$set": bson.M{
				"user_id":            userID,
				"key_hash":           utils.HashMCPKey(key),
				"key_hint":           hint,
				"consent_version":    req.ConsentVersion,
				"consented_at":       now,
				"consent_ip":         consentIP,
				"consent_user_agent": consentUA,
				"created_at":         now,
				"expires_at":         expiresAt,
			},
			// Cleared, never $set: a regenerated key must not inherit the
			// previous key's revocation or usage. These two names must stay out
			// of $set above -- Mongo rejects an update with the same path in
			// both and errors with "would create a conflict at 'revoked_at'".
			"$unset": bson.M{"revoked_at": "", "last_used_at": ""},
		}

		// Returning the old document in the same atomic write gives us exactly the
		// key this one replaces, even if two generates race.
		var replaced models.MCPKey
		err = database.UpsertReturningOld(mcpKeysCollection, bson.M{"user_id": userID}, update, &replaced)
		if err == nil || errors.Is(err, mongo.ErrNoDocuments) {
			// nil means a previous key was replaced; clear the MCP's cached check so it stops working now.
			if err == nil {
				database.InvalidateMCPKeyCheck(replaced.KeyHash, replaced.KeyHint)
			}
			resp = models.GenerateMCPKeyResponse{
				Key:            key,
				KeyHint:        hint,
				ConsentVersion: req.ConsentVersion,
				CreatedAt:      now,
				ExpiresAt:      expiresAt,
			}
			break
		}

		// Two concurrent generates by a user with no existing document can both
		// find nothing and both attempt an insert. Mongo fails one with E11000
		// rather than serialising them, and the driver does not retry, so
		// without this a first-ever double-click 500s one of the two requests.
		// A second attempt finds the row the winner inserted and updates it.
		// A fresh key on the retry also covers a key_hash collision, which is
		// not worth distinguishing from this case.
		if mongo.IsDuplicateKeyError(err) && attempt < maxMCPKeyWriteTries {
			continue
		}

		log.Printf("mcp key generate: upsert failed for user %s: %v", userID, err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to save key"})
	}

	c.Set("Cache-Control", "no-store")
	return c.Status(fiber.StatusCreated).JSON(resp)
}

// HandleRevokeMCPKey deletes the authenticated user's MCP key.
//
// The row is removed rather than soft-revoked: there is one key per user, the
// consent record's value is in the key that replaces it, and a tombstone in the
// live collection buys nothing. If an audit trail of revocations is ever wanted,
// append to a separate collection rather than parking dead rows here.
//
// Returns 200 even when nothing was deleted, deviating from
// HandleDeleteScenarioPreference's 404-on-no-match: DELETE should be idempotent,
// and a user double-clicking revoke should not get a scary 404 for succeeding.
func HandleRevokeMCPKey(c *fiber.Ctx) error {
	userID, ok := c.Locals("user_id").(string)
	if !ok || userID == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}

	// Returning the deleted document gives us its hash, to clear the MCP's cached check.
	var revoked models.MCPKey
	err := database.DeleteReturningOld(mcpKeysCollection, bson.M{"user_id": userID}, &revoked)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return c.JSON(fiber.Map{"revoked": false})
	}
	if err != nil {
		log.Printf("mcp key revoke: delete failed for user %s: %v", userID, err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to revoke key"})
	}

	database.InvalidateMCPKeyCheck(revoked.KeyHash, revoked.KeyHint)
	return c.JSON(fiber.Map{"revoked": true})
}

// HandleVerifyMCPKey is called by the MCP server, not by a browser. The key in
// the body is the credential; there is no JWT, which is why this route does not
// sit behind IsAuthenticated.
func HandleVerifyMCPKey(c *fiber.Ctx) error {
	// The body carries a credential and the response carries identity; neither
	// belongs in an HTTP cache. Section 3 of the consent document also promises
	// a regenerated key's predecessor stops working immediately: the MCP's own
	// Redis cache honours that only because generate and revoke clear it (see
	// database.InvalidateMCPKeyCheck), which no HTTP cache would allow.
	c.Set("Cache-Control", "no-store")

	// json.Unmarshal rather than c.BodyParser: BodyParser insists on a JSON
	// Content-Type and answers 422 without one, which is a miserable first
	// contact for a service-to-service caller whose HTTP client forgot the
	// header. Lenient here, strict on the browser-facing generate endpoint.
	var req models.VerifyMCPKeyRequest
	if err := json.Unmarshal(c.Body(), &req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"valid": false, "reason": "invalid_body"})
	}

	key := strings.TrimSpace(req.Key)
	if !utils.LooksLikeMCPKey(key) {
		// Rejected on shape alone, before any database work.
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"valid": false, "reason": reasonInvalidFormat})
	}

	hash := utils.HashMCPKey(key)

	var doc models.MCPKey
	if err := database.FindOne(mcpKeysCollection, bson.M{"key_hash": hash}, &doc); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"valid": false, "reason": reasonNotFound})
		}
		// Log the display hint, never the key and never the hash.
		log.Printf("mcp verify: lookup failed for %s: %v", utils.MCPKeyHint(key), err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"valid": false, "reason": "internal_error"})
	}

	now := time.Now().UTC()
	if reason := classifyMCPKey(doc, now); reason != "" {
		body := fiber.Map{"valid": false, "reason": reason}
		if reason == reasonExpired {
			// So the MCP server can say "expired on the 4th", not just "expired".
			body["expires_at"] = doc.ExpiresAt
		}
		return c.Status(fiber.StatusUnauthorized).JSON(body)
	}

	// Identity comes from the users collection rather than fields copied onto
	// the key at issue time: auth.go refreshes users on every login, so this is
	// the current login and email rather than a 90-day-old snapshot. It also
	// means a deleted user's key stops working, which a denormalised copy would
	// not.
	userObjID, err := primitive.ObjectIDFromHex(doc.UserID)
	if err != nil {
		// A corrupt user_id is a 401, not a 500: the key cannot be honoured, and
		// the caller can do nothing about our data being wrong.
		log.Printf("mcp verify: unparseable user_id %q on key %s", doc.UserID, doc.KeyHint)
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"valid": false, "reason": reasonUserNotFound})
	}

	var user models.User
	if err := database.FindOne("users", bson.M{"_id": userObjID}, &user); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"valid": false, "reason": reasonUserNotFound})
		}
		log.Printf("mcp verify: user lookup failed for %s: %v", doc.UserID, err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"valid": false, "reason": "internal_error"})
	}

	touchMCPKeyLastUsed(hash, now)

	return c.JSON(models.VerifyMCPKeyResponse{
		Valid:     true,
		UserID:    doc.UserID,
		Username:  user.Login,
		Email:     user.Email,
		ExpiresAt: doc.ExpiresAt,
	})
}

// touchMCPKeyLastUsed records usage off the request path. Verify sits on the hot
// path of every MCP tool call, and a "last used" line in a settings UI does not
// justify making a read into a blocking write.
//
// It takes the hash and the time as values and never touches *fiber.Ctx, which
// Fiber recycles the moment the handler returns. That is safe because
// database.UpdateOne builds its own context and does not depend on the request.
//
// Filtering on key_hash rather than user_id is load-bearing: if the user
// regenerates between the verify and this write, the hash no longer matches and
// the update quietly affects nothing -- which is right, because the new key has
// never been used.
//
// This writes once per verify. If MCP traffic ever makes that a problem, add an
// "only if older than N minutes" clause to the filter; it is not worth the
// complexity until the load is real.
func touchMCPKeyLastUsed(hash string, now time.Time) {
	go func(hash string, now time.Time) {
		update := bson.M{"$set": bson.M{"last_used_at": now}}
		if _, err := database.UpdateOne(mcpKeysCollection, bson.M{"key_hash": hash}, update); err != nil {
			log.Printf("mcp verify: last_used_at update failed: %v", err)
		}
	}(hash, now)
}
