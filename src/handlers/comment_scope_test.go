package handlers

import (
	"testing"

	"automation-developer-guide/src/models"
)

func TestBuildCommentFromPayload_Flow(t *testing.T) {
	scope, err := buildCommentFromPayload(CreateCommentPayload{
		UseCaseID: "retail",
		FlowID:    "order-flow",
		ActionID:  "on_search",
		JSONPath:  "$.context.domain",
		Comment:   "test",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if scope.FlowID != "order-flow" || scope.DocumentSlug != "" {
		t.Fatalf("expected flow scope, got %+v", scope)
	}
}

func TestBuildCommentFromPayload_Document(t *testing.T) {
	scope, err := buildCommentFromPayload(CreateCommentPayload{
		UseCaseID:    "retail",
		DocumentSlug: "overview",
		SectionID:    "summary",
		Comment:      "test",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if scope.DocumentSlug != "overview" || scope.FlowID != "" {
		t.Fatalf("expected document scope, got %+v", scope)
	}
}

func TestBuildCommentFromPayload_DocumentIgnoresFlowFields(t *testing.T) {
	scope, err := buildCommentFromPayload(CreateCommentPayload{
		UseCaseID:    "retail",
		DocumentSlug: "overview",
		SectionID:    "summary",
		FlowID:       "ignored",
		ActionID:     "ignored",
		JSONPath:     "$.ignored",
		Comment:      "test",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if scope.FlowID != "" {
		t.Fatalf("flow fields should not be set for document comment, got %+v", scope)
	}
}

func TestBuildCommentFromPayload_MissingTarget(t *testing.T) {
	_, err := buildCommentFromPayload(CreateCommentPayload{
		UseCaseID: "retail",
		Comment:   "test",
	})
	if err == nil {
		t.Fatal("expected error when neither flow_id nor document_slug is set")
	}
}

func TestApplyParentScope_Flow(t *testing.T) {
	parent := models.Comment{
		UseCaseID: "retail",
		FlowID:    "order-flow",
		ActionID:  "on_search",
		JSONPath:  "$.foo",
	}
	scope := applyParentScope(parent)
	if scope.FlowID != "order-flow" || scope.DocumentSlug != "" {
		t.Fatalf("expected flow scope from parent, got %+v", scope)
	}
}

func TestApplyParentScope_Document(t *testing.T) {
	parent := models.Comment{
		UseCaseID:    "retail",
		DocumentSlug: "overview",
		SectionID:    "summary",
	}
	scope := applyParentScope(parent)
	if scope.DocumentSlug != "overview" || scope.FlowID != "" {
		t.Fatalf("expected document scope from parent, got %+v", scope)
	}
}

func TestBuildCommentFilter_IncludesSectionID(t *testing.T) {
	filter, err := buildCommentFilter(commentListQuery{
		UseCaseID:    "retail",
		DocumentSlug: "overview",
		SectionID:    "summary",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if filter["section_id"] != "summary" {
		t.Fatalf("expected section_id filter, got %+v", filter)
	}
}

func TestBuildCommentFilter_InvalidParentID(t *testing.T) {
	_, err := buildCommentFilter(commentListQuery{ParentCommentID: "not-valid"})
	if err == nil {
		t.Fatal("expected error for invalid parent_comment_id")
	}
}
