package database

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// EnsureCommentIndexes creates partial compound indexes for flow and document comment list queries.
func EnsureCommentIndexes() error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	collection := GetCollection("comments")

	flowPartial := bson.M{
		"flow_id": bson.M{"$exists": true, "$gt": ""},
	}
	flowIndex := mongo.IndexModel{
		Keys: bson.D{
			{Key: "use_case_id", Value: 1},
			{Key: "flow_id", Value: 1},
			{Key: "action_id", Value: 1},
		},
		Options: options.Index().
			SetName("comments_flow_list").
			SetPartialFilterExpression(flowPartial),
	}

	documentPartial := bson.M{
		"document_slug": bson.M{"$exists": true, "$gt": ""},
	}
	documentIndex := mongo.IndexModel{
		Keys: bson.D{
			{Key: "use_case_id", Value: 1},
			{Key: "document_slug", Value: 1},
		},
		Options: options.Index().
			SetName("comments_document_list").
			SetPartialFilterExpression(documentPartial),
	}

	replyIndex := mongo.IndexModel{
		Keys: bson.D{{Key: "parent_comment_id", Value: 1}},
		Options: options.Index().
			SetName("comments_parent_comment_id"),
	}

	_, err := collection.Indexes().CreateMany(ctx, []mongo.IndexModel{flowIndex, documentIndex, replyIndex})
	return err
}

// EnsureMCPKeyIndexes creates the two uniqueness constraints the MCP key model
// depends on.
//
// There is deliberately NO TTL index on expires_at, and that is a product
// decision rather than an oversight:
//
//   - A TTL index deletes the document the moment the key lapses, so
//     GET /user/mcp-key would answer has_key:false and the user would be told
//     they never had a key instead of "this expired, generate a new one". That
//     message is the entire reason expires_at is stored.
//   - It would erase consent_version, consented_at, consent_ip and
//     consent_user_agent, which section 4 of the MCP consent document promises
//     we keep for as long as the key exists.
//   - /mcp/verify would answer not_found instead of expired, so the MCP server
//     could not tell a stale key from a forged one.
//   - It would not even enforce anything. Mongo's TTL sweep runs roughly once a
//     minute, so the handlers check expires_at in code regardless; the index
//     buys no correctness and costs the audit trail.
//   - There is nothing to reclaim: one small document per user.
//
// If retention ever becomes a requirement, make it an explicit job that copies
// rows into an audit collection before removing them, not a silent sweep.
//
// No index on expires_at at all, either: no query filters on it. /mcp/verify
// looks up key_hash and the status endpoint looks up user_id.
func EnsureMCPKeyIndexes() error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	collection := GetCollection("mcp_keys")

	// One active key per user. The generate handler upserts on user_id, so this
	// index is what makes "regenerating replaces the old key" true rather than
	// merely intended.
	userIndex := mongo.IndexModel{
		Keys:    bson.D{{Key: "user_id", Value: 1}},
		Options: options.Index().SetName("mcp_keys_user_id").SetUnique(true),
	}

	// The /mcp/verify lookup path, and a guard against ever storing a hash twice.
	hashIndex := mongo.IndexModel{
		Keys:    bson.D{{Key: "key_hash", Value: 1}},
		Options: options.Index().SetName("mcp_keys_key_hash").SetUnique(true),
	}

	_, err := collection.Indexes().CreateMany(ctx, []mongo.IndexModel{userIndex, hashIndex})
	return err
}
