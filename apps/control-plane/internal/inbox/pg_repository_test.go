package inbox

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/superteam/control-plane/internal/storage/queries"
)

func TestInboxItemMapperPreservesJSONAndOptionalFields(t *testing.T) {
	now := time.Date(2026, 6, 12, 9, 30, 0, 0, time.UTC)
	resolvedAt := now.Add(30 * time.Minute)
	teamID := uuid.New()
	projectID := uuid.New()
	taskID := uuid.New()
	approvalID := uuid.New()

	row := queries.InboxItem{
		ID:                      uuid.New(),
		TenantID:                uuid.New(),
		TeamID:                  uuid.NullUUID{UUID: teamID, Valid: true},
		TargetUserID:            uuid.New(),
		Scope:                   "team",
		ItemType:                "project_decision",
		SourceType:              "project_decision_request",
		SourceID:                uuid.New(),
		SourceProjectID:         uuid.NullUUID{UUID: projectID, Valid: true},
		SourceTaskID:            uuid.NullUUID{UUID: taskID, Valid: true},
		SourceApprovalRequestID: uuid.NullUUID{UUID: approvalID, Valid: true},
		Title:                   "确认高风险项目决策",
		Summary:                 pgtype.Text{String: "需要负责人确认", Valid: true},
		RiskLevel:               pgtype.Text{String: "high", Valid: true},
		Priority:                pgtype.Text{String: "urgent", Valid: true},
		Status:                  "resolved",
		ActionSchema:            []byte(`[{"key":"approved","label":"Approve","tone":"positive","requires_comment":false,"metadata":{"decision":"approved"}}]`),
		ContextPayload:          []byte(`{"project_id":"project-1","risk":"high"}`),
		DeepLink:                []byte(`{"route":"/projects/project-1","tab":"approval"}`),
		ResolvedAt:              pgtype.Timestamptz{Time: resolvedAt, Valid: true},
		LastActivityAt:          pgtype.Timestamptz{Time: now, Valid: true},
		CreatedAt:               pgtype.Timestamptz{Time: now.Add(-time.Hour), Valid: true},
		UpdatedAt:               pgtype.Timestamptz{Time: now, Valid: true},
	}

	item, err := itemFromRecord(row)
	if err != nil {
		t.Fatalf("map inbox item: %v", err)
	}
	if item.SourceProjectID == nil || *item.SourceProjectID != projectID {
		t.Fatalf("expected source project id, got %#v", item.SourceProjectID)
	}
	if item.SourceApprovalRequestID == nil || *item.SourceApprovalRequestID != approvalID {
		t.Fatalf("expected source approval request id, got %#v", item.SourceApprovalRequestID)
	}
	if item.TeamID == nil || *item.TeamID != teamID {
		t.Fatalf("expected team id, got %#v", item.TeamID)
	}
	if item.SourceTaskID == nil || *item.SourceTaskID != taskID {
		t.Fatalf("expected source task id, got %#v", item.SourceTaskID)
	}
	if item.Summary == nil || *item.Summary != "需要负责人确认" {
		t.Fatalf("expected summary, got %#v", item.Summary)
	}
	if item.RiskLevel == nil || *item.RiskLevel != "high" {
		t.Fatalf("expected risk level, got %#v", item.RiskLevel)
	}
	if item.Priority == nil || *item.Priority != "urgent" {
		t.Fatalf("expected priority, got %#v", item.Priority)
	}
	if len(item.Actions) != 1 || item.Actions[0].Key != "approved" {
		t.Fatalf("expected action schema, got %#v", item.Actions)
	}
	if item.Actions[0].Metadata["decision"] != "approved" {
		t.Fatalf("expected action metadata, got %#v", item.Actions[0].Metadata)
	}
	if item.ContextPayload["risk"] != "high" {
		t.Fatalf("expected context payload, got %#v", item.ContextPayload)
	}
	if item.DeepLink["tab"] != "approval" {
		t.Fatalf("expected deep link, got %#v", item.DeepLink)
	}
	if item.ResolvedAt == nil || !item.ResolvedAt.Equal(resolvedAt) {
		t.Fatalf("expected resolved timestamp, got %#v", item.ResolvedAt)
	}
	if !item.LastActivityAt.Equal(row.LastActivityAt.Time) {
		t.Fatalf("expected last activity timestamp %s, got %s", row.LastActivityAt.Time, item.LastActivityAt)
	}
	if !item.CreatedAt.Equal(row.CreatedAt.Time) {
		t.Fatalf("expected created timestamp %s, got %s", row.CreatedAt.Time, item.CreatedAt)
	}
	if !item.UpdatedAt.Equal(row.UpdatedAt.Time) {
		t.Fatalf("expected updated timestamp %s, got %s", row.UpdatedAt.Time, item.UpdatedAt)
	}
}

func TestInboxItemMapperDefaultsEmptyJSONFields(t *testing.T) {
	item, err := itemFromRecord(queries.InboxItem{})
	if err != nil {
		t.Fatalf("map inbox item: %v", err)
	}
	if len(item.Actions) != 0 {
		t.Fatalf("expected empty actions, got %#v", item.Actions)
	}
	if len(item.ContextPayload) != 0 {
		t.Fatalf("expected empty context payload, got %#v", item.ContextPayload)
	}
	if len(item.DeepLink) != 0 {
		t.Fatalf("expected empty deep link, got %#v", item.DeepLink)
	}
}

func TestInboxApprovalSourceUpsertParamsRejectsMissingApprovalRequestID(t *testing.T) {
	req := UpsertItemRequest{
		TenantID:     uuid.New(),
		TargetUserID: uuid.New(),
		Scope:        "personal",
		ItemType:     ItemTypeApproval,
		SourceType:   SourceTypeApprovalRequest,
		SourceID:     uuid.New(),
		Title:        "需要审批",
		Status:       StatusOpen,
	}
	if _, err := upsertApprovalParams(req); !errors.Is(err, ErrInvalidItem) {
		t.Fatalf("expected ErrInvalidItem for nil source approval request id, got %v", err)
	}

	nilApprovalID := uuid.Nil
	req.SourceApprovalRequestID = &nilApprovalID
	if _, err := upsertApprovalParams(req); !errors.Is(err, ErrInvalidItem) {
		t.Fatalf("expected ErrInvalidItem for nil uuid source approval request id, got %v", err)
	}
}
