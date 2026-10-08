package middleware

import (
	"crypto/sha256"
	"crypto/subtle"
	"time"

	"automation-developer-guide/src/config"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/limiter"
)

// RequireServiceToken optionally gates the server-to-server MCP verify endpoint.
//
// The API key in the request body is already the real credential, so this is a
// second factor rather than the only one. It exists because an open /mcp/verify
// is an identity oracle: anybody could post candidate strings and, on a hit,
// read back a user's id, username and email.
//
// When MCP_SERVICE_TOKEN is unset the gate is skipped, so local development and
// a first MCP integration work without cross-repo secret coordination. Set it in
// production to close the endpoint.
func RequireServiceToken(c *fiber.Ctx) error {
	if config.MCPServiceToken == "" {
		return c.Next()
	}

	if !serviceTokenMatches(c) {
		// 403, not 401: "you are not an allowed service" is a different problem
		// from "that API key is bad" and should not be debugged as if it were
		// the same one.
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "forbidden"})
	}

	return c.Next()
}

// serviceTokenMatches reports whether the request carries the configured service
// token. Returns false when no token is configured, so callers must handle the
// unconfigured case themselves.
//
// Unlike the API key, this is a bare secret compared against a stored copy with
// no hash in between, so the comparison has to be constant-time. Digesting both
// sides first makes them unconditionally 32 bytes, so ConstantTimeCompare sees
// equal lengths and the check leaks neither the token nor its length.
func serviceTokenMatches(c *fiber.Ctx) bool {
	expected := config.MCPServiceToken
	if expected == "" {
		return false
	}
	got := sha256.Sum256([]byte(c.Get("X-Service-Token")))
	want := sha256.Sum256([]byte(expected))
	return subtle.ConstantTimeCompare(got[:], want[:]) == 1
}

// mcpVerifyMaxPerMinute bounds anonymous verify traffic from one address.
//
// Sized for a real integration rather than a single user: the MCP server makes
// one verify call per agent tool call, and every user's traffic arrives from
// that one host, so a tight per-IP cap would throttle the whole integration
// rather than an abuser. Callers presenting the service token skip the limiter
// entirely (see Next below), so this ceiling only ever applies to untrusted
// callers -- which is exactly who it is meant to stop.
const mcpVerifyMaxPerMinute = 300

// MCPVerifyLimiter caps how fast an untrusted caller can ask whether a key is
// valid.
//
// Section 8 of the MCP consent document tells the user "request limits are in
// place to prevent overload and misuse", so this is a commitment rather than
// optional hardening. Brute force is not the threat -- the search space is
// 2^256 -- but checking a list of leaked keys against us en masse is, as is one
// noisy client loading Mongo. That threat is many different keys from one
// address, which is why this is keyed on the address and not on the key.
//
// c.IP() is the real client address because main.go sets ProxyHeader. Storage is
// fiber's in-memory default, so the limit is per pod and the effective ceiling
// is pods x Max. That is fine here: this is a throttle, not an access control.
// The service token is the access control.
func MCPVerifyLimiter() fiber.Handler {
	return limiter.New(limiter.Config{
		Max:        mcpVerifyMaxPerMinute,
		Expiration: time.Minute,
		// A caller holding the service token is the MCP server itself, which is
		// already authenticated and whose volume is legitimate. Throttling it
		// would break the product to slow down an attacker who, by definition,
		// does not have this token.
		Next: serviceTokenMatches,
		LimitReached: func(c *fiber.Ctx) error {
			return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{
				"valid":  false,
				"reason": "rate_limited",
			})
		},
	})
}

// MCPGenerateLimiter caps key issuance per user.
//
// It must run after IsAuthenticated so that user_id is in Locals. Keyed on the
// user rather than the IP because this is where abuse costs something: each call
// draws from the CSPRNG, writes to Mongo, and invalidates the user's working
// key.
func MCPGenerateLimiter() fiber.Handler {
	return limiter.New(limiter.Config{
		Max:        10,
		Expiration: time.Hour,
		KeyGenerator: func(c *fiber.Ctx) string {
			if userID, ok := c.Locals("user_id").(string); ok && userID != "" {
				return "mcpgen:" + userID
			}
			return "mcpgen:" + c.IP()
		},
		LimitReached: func(c *fiber.Ctx) error {
			return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{
				"error": "too many key generation requests, try again later",
			})
		},
	})
}
