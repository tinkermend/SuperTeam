package approval

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/superteam/control-plane/internal/storage/queries"
)

func TestApprovalRequestMapperPreservesJSONAndOptionalFields(t *testing.T) {
	now := time.Date(2026, 6, 11, 13, 30, 0, 0, time.UTC)
	requesterID := uuid.New()
	resolvedAt := now.Add(time.Minute)
	row := queries.ApprovalRequest{
		ID:             uuid.New(),
		TenantID:       uuid.New(),
		ResourceType:   "project_decision",
		ResourceID:     uuid.New(),
		RequesterType:  "project_coordinator",
		RequesterID:    uuid.NullUUID{UUID: requesterID, Valid: true},
		TargetUserID:   uuid.New(),
		DecisionType:   "route_review",
		Title:          "确认高风险路由",
		Summary:        pgtype.Text{String: "需要负责人确认", Valid: true},
		RiskLevel:      pgtype.Text{String: "high", Valid: true},
		Status:         "approved",
		Options:        []byte(`["approved","rejected"]`),
		ContextPayload: []byte(`{"project_id":"project-1","risk":"high"}`),
		CreatedAt:      pgtype.Timestamptz{Time: now, Valid: true},
		UpdatedAt:      pgtype.Timestamptz{Time: now, Valid: true},
		ResolvedAt:     pgtype.Timestamptz{Time: resolvedAt, Valid: true},
	}

	request, err := requestFromRecord(row)
	if err != nil {
		t.Fatalf("map request: %v", err)
	}
	if request.RequesterID == nil || *request.RequesterID != requesterID {
		t.Fatalf("expected requester id, got %#v", request.RequesterID)
	}
	if request.Summary == nil || *request.Summary != "需要负责人确认" {
		t.Fatalf("expected summary, got %#v", request.Summary)
	}
	if request.RiskLevel == nil || *request.RiskLevel != "high" {
		t.Fatalf("expected risk level, got %#v", request.RiskLevel)
	}
	if request.Status != ApprovalStatusApproved {
		t.Fatalf("expected approved status, got %s", request.Status)
	}
	if len(request.Options) != 2 || request.Options[0] != "approved" {
		t.Fatalf("expected options, got %#v", request.Options)
	}
	if request.ContextPayload["risk"] != "high" {
		t.Fatalf("expected context payload, got %#v", request.ContextPayload)
	}
	if request.ResolvedAt == nil || !request.ResolvedAt.Equal(resolvedAt) {
		t.Fatalf("expected resolved timestamp, got %#v", request.ResolvedAt)
	}
}

func TestApprovalDecisionMapperPreservesPayload(t *testing.T) {
	now := time.Date(2026, 6, 11, 13, 40, 0, 0, time.UTC)
	row := queries.ApprovalDecision{
		ID:                uuid.New(),
		TenantID:          uuid.New(),
		ApprovalRequestID: uuid.New(),
		DecidedByUserID:   uuid.New(),
		Decision:          "needs_more_evidence",
		Comment:           pgtype.Text{String: "请补充测试截图", Valid: true},
		Payload:           []byte(`{"required":["screenshot"]}`),
		CreatedAt:         pgtype.Timestamptz{Time: now, Valid: true},
	}

	decision, err := decisionFromRecord(row)
	if err != nil {
		t.Fatalf("map decision: %v", err)
	}
	if decision.Decision != ApprovalDecisionNeedsMoreEvidence {
		t.Fatalf("expected needs_more_evidence, got %s", decision.Decision)
	}
	if decision.Comment == nil || *decision.Comment != "请补充测试截图" {
		t.Fatalf("expected comment, got %#v", decision.Comment)
	}
	if len(decision.Payload["required"].([]any)) != 1 {
		t.Fatalf("expected payload array, got %#v", decision.Payload)
	}
}

func TestApprovalJSONHelpersRejectInvalidShapes(t *testing.T) {
	if _, err := jsonbObject(map[string]any{"bad": func() {}}, "context_payload"); err == nil {
		t.Fatal("expected object marshal error")
	}
	if _, err := jsonbArray([]any{func() {}}, "options"); err == nil {
		t.Fatal("expected array marshal error")
	}
	if _, err := requestFromRecord(queries.ApprovalRequest{
		Options:        []byte(`{"not":"array"}`),
		ContextPayload: []byte(`{}`),
	}); err == nil {
		t.Fatal("expected invalid options shape error")
	}
}
