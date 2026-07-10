package handlers

import (
	"bytes"
	"encoding/json"
	"time"

	"automation-developer-guide/src/database"
	"automation-developer-guide/src/models"

	"github.com/go-playground/validator/v10"
	"github.com/gofiber/fiber/v2"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

var validateComment = validator.New()

// HandleCreateComment adds a new comment (or reply if parent_comment_id is set)
func HandleCreateComment(c *fiber.Ctx) error {
	userID := c.Locals("user_id").(string)

	payload := new(CreateCommentPayload)
	decoder := json.NewDecoder(bytes.NewReader(c.Body()))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(payload); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid json or unknown fields"})
	}

	if payload.Comment == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "comment is required"})
	}

	now := time.Now()
	comment := models.Comment{
		Comment:   payload.Comment,
		CreatedBy: userID,
		Resolved:  false,
		CreatedAt: now,
		UpdatedAt: now,
	}

	if payload.ParentCommentID != nil {
		objID, err := primitive.ObjectIDFromHex(*payload.ParentCommentID)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid parent_comment_id"})
		}
		comment.ParentCommentID = &objID

		var parentComment models.Comment
		if err := database.FindOne("comments", bson.M{"_id": objID}, &parentComment); err != nil {
			if err == mongo.ErrNoDocuments {
				return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "parent comment not found"})
			}
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to verify parent comment"})
		}

		parentScope := applyParentScope(parentComment)
		comment.UseCaseID = parentScope.UseCaseID
		comment.FlowID = parentScope.FlowID
		comment.ActionID = parentScope.ActionID
		comment.JSONPath = parentScope.JSONPath
		comment.DocumentSlug = parentScope.DocumentSlug
		comment.SectionID = parentScope.SectionID
	} else {
		scope, err := buildCommentFromPayload(*payload)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
		}
		comment.UseCaseID = scope.UseCaseID
		comment.FlowID = scope.FlowID
		comment.ActionID = scope.ActionID
		comment.JSONPath = scope.JSONPath
		comment.DocumentSlug = scope.DocumentSlug
		comment.SectionID = scope.SectionID
	}

	result, err := database.CreateOne("comments", comment)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to save comment"})
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"message": "comment created", "id": result.InsertedID})
}

// HandleGetComments retrieves comments. Visible to ANY authenticated user.
func HandleGetComments(c *fiber.Ctx) error {
	filter, err := buildCommentFilter(commentListQueryFromCtx(c))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: filter}},
		{{Key: "$sort", Value: bson.M{"created_at": -1}}},
		{{Key: "$addFields", Value: bson.M{"created_by_oid": bson.M{"$toObjectId": "$created_by"}}}},
		{{Key: "$lookup", Value: bson.M{
			"from":         "users",
			"localField":   "created_by_oid",
			"foreignField": "_id",
			"as":           "user_details",
		}}},
		{{Key: "$unwind", Value: bson.M{"path": "$user_details", "preserveNullAndEmptyArrays": true}}},
		{{Key: "$addFields", Value: bson.M{
			"user": bson.M{"email": "$user_details.email", "username": "$user_details.login"},
		}}},
		{{Key: "$project", Value: bson.M{"user_details": 0, "created_by_oid": 0}}},
	}

	var comments []bson.M = []bson.M{}
	if err := database.Aggregate("comments", pipeline, &comments); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to fetch comments"})
	}

	return c.JSON(comments)
}

// HandleGetCommentByID retrieves a single comment by ID and its immediate replies
func HandleGetCommentByID(c *fiber.Ctx) error {
	commentID := c.Params("id")
	objID, err := primitive.ObjectIDFromHex(commentID)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid comment id"})
	}

	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: bson.M{"_id": objID}}},
		{{Key: "$addFields", Value: bson.M{"created_by_oid": bson.M{"$toObjectId": "$created_by"}}}},
		{{Key: "$lookup", Value: bson.M{
			"from":         "users",
			"localField":   "created_by_oid",
			"foreignField": "_id",
			"as":           "user_details",
		}}},
		{{Key: "$unwind", Value: bson.M{"path": "$user_details", "preserveNullAndEmptyArrays": true}}},
		{{Key: "$addFields", Value: bson.M{
			"user": bson.M{"email": "$user_details.email", "username": "$user_details.login"},
		}}},
		{{Key: "$project", Value: bson.M{"user_details": 0, "created_by_oid": 0}}},
	}

	var comments []bson.M
	if err := database.Aggregate("comments", pipeline, &comments); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to fetch comment"})
	}
	if len(comments) == 0 {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "comment not found"})
	}
	comment := comments[0]

	pipelineReplies := mongo.Pipeline{
		{{Key: "$match", Value: bson.M{"parent_comment_id": objID}}},
		{{Key: "$sort", Value: bson.M{"created_at": -1}}},
		{{Key: "$addFields", Value: bson.M{"created_by_oid": bson.M{"$toObjectId": "$created_by"}}}},
		{{Key: "$lookup", Value: bson.M{
			"from":         "users",
			"localField":   "created_by_oid",
			"foreignField": "_id",
			"as":           "user_details",
		}}},
		{{Key: "$unwind", Value: bson.M{"path": "$user_details", "preserveNullAndEmptyArrays": true}}},
		{{Key: "$addFields", Value: bson.M{
			"user": bson.M{"email": "$user_details.email", "username": "$user_details.login"},
		}}},
		{{Key: "$project", Value: bson.M{"user_details": 0, "created_by_oid": 0}}},
	}

	var replies []bson.M = []bson.M{}
	if err := database.Aggregate("comments", pipelineReplies, &replies); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to fetch replies"})
	}

	return c.JSON(fiber.Map{
		"comment": comment,
		"replies": replies,
	})
}

// HandleUpdateComment handles text updates AND resolving. Only Creator can perform this.
func HandleUpdateComment(c *fiber.Ctx) error {
	userID := c.Locals("user_id").(string)
	commentID := c.Params("id")

	objID, err := primitive.ObjectIDFromHex(commentID)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid comment id"})
	}

	type UpdatePayload struct {
		Comment  *string `json:"comment"`
		Resolved *bool   `json:"resolved"`
	}

	payload := new(UpdatePayload)
	decoder := json.NewDecoder(bytes.NewReader(c.Body()))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(payload); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid json or unknown fields"})
	}

	updateData := bson.M{"updated_at": time.Now()}
	if payload.Comment != nil {
		updateData["comment"] = *payload.Comment
	}
	if payload.Resolved != nil {
		updateData["resolved"] = *payload.Resolved
	}

	// Filter by ID AND CreatedBy to ensure ownership
	filter := bson.M{"_id": objID, "created_by": userID}
	update := bson.M{"$set": updateData}

	result, err := database.UpdateOne("comments", filter, update)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to update comment"})
	}

	if result.MatchedCount == 0 {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "comment not found or unauthorized"})
	}

	return c.JSON(fiber.Map{"message": "comment updated"})
}

// HandleResolveComment allows any authenticated user to update the resolution status
func HandleResolveComment(c *fiber.Ctx) error {
	commentID := c.Params("id")

	objID, err := primitive.ObjectIDFromHex(commentID)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid comment id"})
	}

	type ResolvePayload struct {
		Resolved *bool `json:"resolved" validate:"required"`
	}

	payload := new(ResolvePayload)
	decoder := json.NewDecoder(bytes.NewReader(c.Body()))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(payload); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid json or unknown fields"})
	}
	if err := validateComment.Struct(payload); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	// Filter by ID only (no created_by check), allowing any authenticated user
	filter := bson.M{"_id": objID}
	update := bson.M{
		"$set": bson.M{
			"resolved":   *payload.Resolved,
			"updated_at": time.Now(),
		},
	}

	result, err := database.UpdateOne("comments", filter, update)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to update comment"})
	}

	if result.MatchedCount == 0 {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "comment not found"})
	}

	return c.JSON(fiber.Map{"message": "comment resolution updated"})
}

// HandleDeleteComment deletes a comment. Only Creator can perform this.
func HandleDeleteComment(c *fiber.Ctx) error {
	userID := c.Locals("user_id").(string)
	commentID := c.Params("id")

	objID, err := primitive.ObjectIDFromHex(commentID)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid comment id"})
	}

	// Filter by ID AND CreatedBy to ensure ownership
	filter := bson.M{"_id": objID, "created_by": userID}

	result, err := database.DeleteOne("comments", filter)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to delete comment"})
	}

	if result.DeletedCount == 0 {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "comment not found or unauthorized"})
	}

	return c.JSON(fiber.Map{"message": "comment deleted"})
}
