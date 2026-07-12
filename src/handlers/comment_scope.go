package handlers

import (
	"errors"
	"fmt"

	"automation-developer-guide/src/models"

	"github.com/gofiber/fiber/v2"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// CreateCommentPayload is the POST body for creating a comment or reply.
// Scope fields are required for top-level comments; replies need only parent_comment_id + comment.
type CreateCommentPayload struct {
	UseCaseID       string  `json:"use_case_id"`
	FlowID          string  `json:"flow_id"`
	ActionID        string  `json:"action_id"`
	JSONPath        string  `json:"json_path"`
	DocumentSlug    string  `json:"document_slug"`
	SectionID       string  `json:"section_id"`
	Comment         string  `json:"comment"`
	ParentCommentID *string `json:"parent_comment_id"`
}

type commentListQuery struct {
	UseCaseID       string
	FlowID          string
	ActionID        string
	JSONPath        string
	DocumentSlug    string
	SectionID       string
	ParentCommentID string
}

func commentListQueryFromCtx(c *fiber.Ctx) commentListQuery {
	return commentListQuery{
		UseCaseID:       c.Query("use_case_id"),
		FlowID:          c.Query("flow_id"),
		ActionID:        c.Query("action_id"),
		JSONPath:        c.Query("json_path"),
		DocumentSlug:    c.Query("document_slug"),
		SectionID:       c.Query("section_id"),
		ParentCommentID: c.Query("parent_comment_id"),
	}
}

func buildCommentFilter(q commentListQuery) (bson.M, error) {
	filter := bson.M{}

	if q.UseCaseID != "" {
		filter["use_case_id"] = q.UseCaseID
	}
	if q.FlowID != "" {
		filter["flow_id"] = q.FlowID
	}
	if q.ActionID != "" {
		filter["action_id"] = q.ActionID
	}
	if q.JSONPath != "" {
		filter["json_path"] = q.JSONPath
	}
	if q.DocumentSlug != "" {
		filter["document_slug"] = q.DocumentSlug
	}
	if q.SectionID != "" {
		filter["section_id"] = q.SectionID
	}
	if q.ParentCommentID != "" {
		objID, err := primitive.ObjectIDFromHex(q.ParentCommentID)
		if err != nil {
			return nil, fmt.Errorf("invalid parent_comment_id")
		}
		filter["parent_comment_id"] = objID
	}

	return filter, nil
}

func isDocumentComment(parent models.Comment) bool {
	return parent.DocumentSlug != ""
}

func applyParentScope(parent models.Comment) models.Comment {
	scope := models.Comment{
		UseCaseID: parent.UseCaseID,
	}
	if isDocumentComment(parent) {
		scope.DocumentSlug = parent.DocumentSlug
		scope.SectionID = parent.SectionID
	} else {
		scope.FlowID = parent.FlowID
		scope.ActionID = parent.ActionID
		scope.JSONPath = parent.JSONPath
	}
	return scope
}

func buildCommentFromPayload(payload CreateCommentPayload) (models.Comment, error) {
	if payload.Comment == "" {
		return models.Comment{}, errors.New("comment is required")
	}

	if payload.DocumentSlug != "" {
		if payload.UseCaseID == "" {
			return models.Comment{}, errors.New("use_case_id is required")
		}
		if payload.SectionID == "" {
			return models.Comment{}, errors.New("section_id is required")
		}
		return models.Comment{
			UseCaseID:    payload.UseCaseID,
			DocumentSlug: payload.DocumentSlug,
			SectionID:    payload.SectionID,
			Comment:      payload.Comment,
		}, nil
	}

	if payload.FlowID != "" {
		if payload.UseCaseID == "" {
			return models.Comment{}, errors.New("use_case_id is required")
		}
		if payload.ActionID == "" {
			return models.Comment{}, errors.New("action_id is required")
		}
		if payload.JSONPath == "" {
			return models.Comment{}, errors.New("json_path is required")
		}
		return models.Comment{
			UseCaseID: payload.UseCaseID,
			FlowID:    payload.FlowID,
			ActionID:  payload.ActionID,
			JSONPath:  payload.JSONPath,
			Comment:   payload.Comment,
		}, nil
	}

	return models.Comment{}, errors.New("either flow_id or document_slug is required")
}
