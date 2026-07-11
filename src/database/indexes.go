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
