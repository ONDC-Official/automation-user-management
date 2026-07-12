package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type Comment struct {
	ID              primitive.ObjectID  `json:"id" bson:"_id,omitempty"`
	UseCaseID       string              `json:"use_case_id" bson:"use_case_id"`
	FlowID          string              `json:"flow_id" bson:"flow_id,omitempty"`
	ActionID        string              `json:"action_id" bson:"action_id,omitempty"`
	JSONPath        string              `json:"json_path" bson:"json_path,omitempty"`
	DocumentSlug    string              `json:"document_slug" bson:"document_slug,omitempty"`
	SectionID       string              `json:"section_id" bson:"section_id,omitempty"`
	Comment         string              `json:"comment" bson:"comment"`
	ParentCommentID *primitive.ObjectID `json:"parent_comment_id,omitempty" bson:"parent_comment_id,omitempty"`
	Resolved        bool                `json:"resolved" bson:"resolved"`
	CreatedBy       string              `json:"created_by" bson:"created_by"`
	CreatedAt       time.Time           `json:"created_at" bson:"created_at"`
	UpdatedAt       time.Time           `json:"updated_at" bson:"updated_at"`
}
