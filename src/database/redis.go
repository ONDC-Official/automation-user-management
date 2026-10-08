package database

import (
	"context"
	"log"
	"net"
	"time"

	"github.com/redis/go-redis/v9"
)

// redisTimeout bounds every Redis call, so a slow Redis can never hold up a
// user's generate or revoke.
const redisTimeout = 2 * time.Second

var (
	redisClient *redis.Client
	// mcpRedisKeyPrefix is the MCP server's REDIS_KEY_PREFIX; see MCPKeyCheckCacheKey.
	mcpRedisKeyPrefix string
)

// ConnectRedis opens the client used to clear the MCP server's key-check cache.
//
// The client is kept even when the first ping fails: go-redis reconnects on
// its own, so a Redis that comes up after this service still gets used.
func ConnectRedis(host, port, username, password, keyPrefix string) error {
	mcpRedisKeyPrefix = keyPrefix
	redisClient = redis.NewClient(&redis.Options{
		Addr:         net.JoinHostPort(host, port),
		Username:     username,
		Password:     password,
		DialTimeout:  redisTimeout,
		ReadTimeout:  redisTimeout,
		WriteTimeout: redisTimeout,
	})

	ctx, cancel := context.WithTimeout(context.Background(), redisTimeout)
	defer cancel()
	return redisClient.Ping(ctx).Err()
}

// MCPKeyCheckCacheKey is the Redis key the MCP server caches its verify answer
// under. It must match the MCP exactly: "<prefix>::mcp_key_check:<key hash>",
// where the key hash is utils.HashMCPKey of the plaintext key.
func MCPKeyCheckCacheKey(prefix, keyHash string) string {
	key := "mcp_key_check:" + keyHash
	// The MCP's store joins prefix and key with "::", and omits both when the prefix is empty.
	if prefix == "" {
		return key
	}
	return prefix + "::" + key
}

// InvalidateMCPKeyCheck deletes the MCP server's cached verify answer for a key
// that has just stopped being valid, so it is refused on its next use rather
// than when the cache entry expires.
//
// It never fails the caller: the key is already gone from Mongo, and if this
// delete is lost the MCP's entry still expires within its TTL. keyHint is the
// non-secret display string, logged instead of the hash.
func InvalidateMCPKeyCheck(keyHash, keyHint string) {
	if redisClient == nil || keyHash == "" {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), redisTimeout)
	defer cancel()
	cacheKey := MCPKeyCheckCacheKey(mcpRedisKeyPrefix, keyHash)
	if err := redisClient.Del(ctx, cacheKey).Err(); err != nil {
		log.Printf("mcp cache: could not clear the cached check for %s; it expires within the MCP's TTL: %v", keyHint, err)
	}
}
