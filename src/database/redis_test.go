package database

import (
	"testing"
	"time"

	"automation-developer-guide/src/utils"

	"github.com/alicebob/miniredis/v2"
)

// startRedis runs an in-process Redis and connects the package client to it.
func startRedis(t *testing.T) *miniredis.Miniredis {
	t.Helper()
	server := miniredis.RunT(t)
	if err := ConnectRedis(server.Host(), server.Port(), "", "", "ondc-mcp"); err != nil {
		t.Fatalf("connect to miniredis: %v", err)
	}
	t.Cleanup(func() {
		_ = redisClient.Close()
		redisClient = nil
	})
	return server
}

// The exact entry the MCP server wrote during a live test, for this key with
// REDIS_KEY_PREFIX=ondc-mcp. If this ever fails, our deletes miss the MCP's cache.
func TestMCPKeyCheckCacheKey_MatchesMCPServer(t *testing.T) {
	const want = "ondc-mcp::mcp_key_check:bec04d9115f61c238cf78cd2e006e851da9f043a71b1c375df7127513d85fdd6"
	got := MCPKeyCheckCacheKey("ondc-mcp", utils.HashMCPKey("ondc_mcp_LocalTestKey123"))
	if got != want {
		t.Fatalf("expected %s, got %s", want, got)
	}
}

func TestMCPKeyCheckCacheKey_EmptyPrefixHasNoSeparator(t *testing.T) {
	if got := MCPKeyCheckCacheKey("", "abc"); got != "mcp_key_check:abc" {
		t.Fatalf("expected mcp_key_check:abc, got %s", got)
	}
}

func TestInvalidateMCPKeyCheck_DeletesOnlyThatEntry(t *testing.T) {
	server := startRedis(t)
	target := MCPKeyCheckCacheKey("ondc-mcp", "oldhash")
	other := MCPKeyCheckCacheKey("ondc-mcp", "otherhash")
	server.Set(target, `{"valid":true}`)
	server.Set(other, `{"valid":true}`)

	InvalidateMCPKeyCheck("oldhash", "ondc_mcp_AbC1x2")

	if server.Exists(target) {
		t.Fatal("expected the old key's cached check to be deleted")
	}
	if !server.Exists(other) {
		t.Fatal("expected other keys' cached checks to be left alone")
	}
}

// A lost delete must never fail or stall the user's regenerate or revoke.
func TestInvalidateMCPKeyCheck_RedisDownReturnsQuietly(t *testing.T) {
	server := startRedis(t)
	server.Close()

	started := time.Now()
	InvalidateMCPKeyCheck("oldhash", "ondc_mcp_AbC1x2")

	if elapsed := time.Since(started); elapsed > redisTimeout+time.Second {
		t.Fatalf("expected the delete to give up within %s, took %s", redisTimeout, elapsed)
	}
}

func TestInvalidateMCPKeyCheck_NotConfiguredIsNoOp(t *testing.T) {
	redisClient = nil
	InvalidateMCPKeyCheck("oldhash", "ondc_mcp_AbC1x2")
}

func TestInvalidateMCPKeyCheck_EmptyHashIsNoOp(t *testing.T) {
	server := startRedis(t)
	server.Set(MCPKeyCheckCacheKey("ondc-mcp", ""), "x")

	InvalidateMCPKeyCheck("", "")

	if !server.Exists(MCPKeyCheckCacheKey("ondc-mcp", "")) {
		t.Fatal("expected an empty hash to delete nothing")
	}
}
