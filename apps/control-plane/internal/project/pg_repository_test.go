package project

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/superteam/control-plane/internal/storage/queries"
)

func TestProjectFromRecordMapsPoliciesAndOptionalUsers(t *testing.T) {
	leaderID := uuid.New()
	teamID := uuid.New()
	archivedAt := time.Date(2026, 6, 11, 10, 0, 0, 0, time.UTC)
	row := queries.Project{
		ID:                     uuid.New(),
		TenantID:               uuid.New(),
		TeamID:                 uuid.NullUUID{UUID: teamID, Valid: true},
		Name:                   "支付网关稳定性整改",
		Description:            pgtype.Text{String: "线上超时整改", Valid: true},
		Goal:                   pgtype.Text{String: "修复超时链路", Valid: true},
		Status:                 "running",
		HumanOwnerUserID:       uuid.New(),
		LeaderUserID:           uuid.NullUUID{UUID: leaderID, Valid: true},
		CoordinationWorkflowID: pgtype.Text{String: "project-coordinator:abc", Valid: true},
		CoordinationStatus:     pgtype.Text{String: "registered", Valid: true},
		CoordinationPolicy:     []byte(`{"auto_dispatch_low_risk":true}`),
		ApprovalPolicy:         []byte(`{"high_risk":"required"}`),
		EvidencePolicy:         []byte(`{"required":["TaskSummary"]}`),
		ArchivedAt:             pgtype.Timestamptz{Time: archivedAt, Valid: true},
		CreatedAt:              pgtype.Timestamptz{Time: archivedAt.Add(-time.Hour), Valid: true},
		UpdatedAt:              pgtype.Timestamptz{Time: archivedAt, Valid: true},
	}

	project, err := projectFromRecord(row)
	if err != nil {
		t.Fatalf("map project: %v", err)
	}
	if project.Name != "支付网关稳定性整改" || project.Status != ProjectStatusRunning {
		t.Fatalf("unexpected project: %#v", project)
	}
	if project.TeamID == nil || *project.TeamID != teamID {
		t.Fatalf("expected team id %s, got %#v", teamID, project.TeamID)
	}
	if project.LeaderUserID == nil || *project.LeaderUserID != leaderID {
		t.Fatalf("expected leader id %s, got %#v", leaderID, project.LeaderUserID)
	}
	if project.Description == nil || *project.Description != "线上超时整改" {
		t.Fatalf("expected description, got %#v", project.Description)
	}
	if project.ArchivedAt == nil || !project.ArchivedAt.Equal(archivedAt) {
		t.Fatalf("expected archived at %s, got %#v", archivedAt, project.ArchivedAt)
	}
	if project.CoordinationPolicy["auto_dispatch_low_risk"] != true {
		t.Fatalf("expected coordination policy, got %#v", project.CoordinationPolicy)
	}
	if project.ApprovalPolicy["high_risk"] != "required" {
		t.Fatalf("expected approval policy, got %#v", project.ApprovalPolicy)
	}
}

func TestProjectRelatedMappersHandleJSONAndOptionalFields(t *testing.T) {
	now := time.Date(2026, 6, 11, 11, 0, 0, 0, time.UTC)
	projectID := uuid.New()
	tenantID := uuid.New()
	actorID := uuid.New()

	member, err := memberFromRecord(queries.ProjectMember{
		ID:                  uuid.New(),
		TenantID:            tenantID,
		ProjectID:           projectID,
		PrincipalType:       string(PrincipalTypeDigitalEmployee),
		PrincipalID:         uuid.New(),
		ProjectRole:         string(ProjectRoleExecutor),
		DisplayNameSnapshot: pgtype.Text{String: "后端执行 A", Valid: true},
		Status:              "active",
		Settings:            []byte(`{"concurrency_slots":2}`),
		CreatedAt:           pgtype.Timestamptz{Time: now, Valid: true},
		UpdatedAt:           pgtype.Timestamptz{Time: now, Valid: true},
	})
	if err != nil {
		t.Fatalf("map member: %v", err)
	}
	if member.ProjectRole != ProjectRoleExecutor || member.Settings["concurrency_slots"] != float64(2) {
		t.Fatalf("unexpected member: %#v", member)
	}
	if member.DisplayNameSnapshot == nil || *member.DisplayNameSnapshot != "后端执行 A" {
		t.Fatalf("expected display name snapshot, got %#v", member.DisplayNameSnapshot)
	}

	demandID := uuid.New()
	employeeID := uuid.New()
	task := taskFromRecord(queries.ProjectTask{
		ID:                        uuid.New(),
		TenantID:                  tenantID,
		ProjectID:                 projectID,
		DemandID:                  uuid.NullUUID{UUID: demandID, Valid: true},
		Title:                     "验证 Runtime 连接",
		Summary:                   pgtype.Text{String: "检查心跳", Valid: true},
		Status:                    "waiting_human",
		AssignedDigitalEmployeeID: uuid.NullUUID{UUID: employeeID, Valid: true},
		RiskLevel:                 pgtype.Text{String: "medium", Valid: true},
		RequiresHumanApproval:     true,
		CreatedAt:                 pgtype.Timestamptz{Time: now, Valid: true},
		UpdatedAt:                 pgtype.Timestamptz{Time: now, Valid: true},
	})
	if task.DemandID == nil || *task.DemandID != demandID {
		t.Fatalf("expected demand id, got %#v", task.DemandID)
	}
	if task.AssignedDigitalEmployeeID == nil || *task.AssignedDigitalEmployeeID != employeeID || !task.RequiresHumanApproval {
		t.Fatalf("unexpected task: %#v", task)
	}

	event, err := eventFromRecord(queries.ProjectEvent{
		ID:             uuid.New(),
		TenantID:       tenantID,
		ProjectID:      projectID,
		SequenceNumber: 7,
		EventType:      string(ProjectEventDemandSubmitted),
		ActorType:      "human_user",
		ActorID:        actorID.String(),
		ResourceType:   pgtype.Text{String: "project_demand", Valid: true},
		ResourceID:     pgtype.Text{String: demandID.String(), Valid: true},
		Summary:        pgtype.Text{String: "需求已提交", Valid: true},
		Payload:        []byte(`{"title":"验证 Runtime 连接"}`),
		CreatedAt:      pgtype.Timestamptz{Time: now, Valid: true},
	})
	if err != nil {
		t.Fatalf("map event: %v", err)
	}
	if event.EventType != ProjectEventDemandSubmitted || event.Payload["title"] != "验证 Runtime 连接" {
		t.Fatalf("unexpected event: %#v", event)
	}

	createdEventID := uuid.New()
	demand, err := demandFromRecord(queries.ProjectDemand{
		ID:                demandID,
		TenantID:          tenantID,
		ProjectID:         projectID,
		SubmittedByUserID: actorID,
		Title:             "验证 Runtime 连接",
		Content:           pgtype.Text{String: "检查心跳和命令回写", Valid: true},
		SourceType:        string(DemandSourceManual),
		SourceRefs:        []byte(`{"ticket":"ST-42"}`),
		Attachments:       []byte(`[{"name":"report.md"}]`),
		Status:            string(ProjectDemandStatusRecorded),
		CreatedEventID:    uuid.NullUUID{UUID: createdEventID, Valid: true},
		CreatedAt:         pgtype.Timestamptz{Time: now, Valid: true},
		UpdatedAt:         pgtype.Timestamptz{Time: now, Valid: true},
	})
	if err != nil {
		t.Fatalf("map demand: %v", err)
	}
	if demand.SourceType != DemandSourceManual || demand.Status != ProjectDemandStatusRecorded {
		t.Fatalf("unexpected demand: %#v", demand)
	}
	if demand.SourceRefs["ticket"] != "ST-42" {
		t.Fatalf("expected source refs to be preserved, got %#v", demand.SourceRefs)
	}
	if len(demand.Attachments) != 1 {
		t.Fatalf("expected attachments to be preserved, got %#v", demand.Attachments)
	}
	if demand.CreatedEventID == nil || *demand.CreatedEventID != createdEventID {
		t.Fatalf("expected created event id, got %#v", demand.CreatedEventID)
	}

	revision, err := configRevisionFromRecord(queries.ProjectConfigRevision{
		ID:              uuid.New(),
		TenantID:        tenantID,
		ProjectID:       projectID,
		RevisionNumber:  3,
		ConfigSnapshot:  []byte(`{"name":"项目","status":"running"}`),
		ChangeSummary:   pgtype.Text{String: "项目配置已更新", Valid: true},
		CreatedByUserID: actorID,
		CreatedEventID:  uuid.NullUUID{UUID: createdEventID, Valid: true},
		CreatedAt:       pgtype.Timestamptz{Time: now, Valid: true},
	})
	if err != nil {
		t.Fatalf("map revision: %v", err)
	}
	if revision.RevisionNumber != 3 || revision.ConfigSnapshot["status"] != "running" {
		t.Fatalf("unexpected revision: %#v", revision)
	}
}

func TestProjectConfigSnapshotIncludesHumanOwner(t *testing.T) {
	ownerID := uuid.New()
	leaderID := uuid.New()
	project := Project{
		Name:             "项目",
		Goal:             "目标",
		Status:           ProjectStatusRunning,
		HumanOwnerUserID: ownerID,
		LeaderUserID:     &leaderID,
	}

	snapshot := projectConfigSnapshot(project)
	if snapshot["human_owner_user_id"] != ownerID.String() {
		t.Fatalf("expected human owner in snapshot, got %#v", snapshot)
	}
	if snapshot["leader_user_id"] != leaderID.String() {
		t.Fatalf("expected leader in snapshot, got %#v", snapshot)
	}
	if snapshot["acceptance_user_id"] != "" {
		t.Fatalf("expected empty acceptance id, got %#v", snapshot)
	}
}

func TestJSONMarshalErrorsAreReturned(t *testing.T) {
	if _, err := jsonbObject(map[string]any{"bad": func() {}}, "settings"); err == nil {
		t.Fatal("expected object marshal error")
	}
	if _, err := jsonbArray([]any{func() {}}, "attachments"); err == nil {
		t.Fatal("expected array marshal error")
	}
}

func TestProjectEventSequenceConflictDetection(t *testing.T) {
	conflict := &pgconn.PgError{
		Code:           "23505",
		ConstraintName: "uq_project_events_project_sequence",
	}
	if !isProjectEventSequenceConflict(conflict) {
		t.Fatal("expected project event sequence conflict")
	}

	otherUnique := &pgconn.PgError{Code: "23505", ConstraintName: "other_constraint"}
	if isProjectEventSequenceConflict(otherUnique) {
		t.Fatal("did not expect unrelated unique violation to retry")
	}
	if isProjectEventSequenceConflict(errors.New("plain error")) {
		t.Fatal("did not expect non pg error to retry")
	}
	if maxProjectEventAppendAttempts != 3 {
		t.Fatalf("expected 3 append attempts, got %d", maxProjectEventAppendAttempts)
	}
}

func TestProjectConfigRevisionConflictDetection(t *testing.T) {
	conflict := &pgconn.PgError{
		Code:           "23505",
		ConstraintName: "uq_project_config_revisions_project_rev",
	}
	if !isProjectConfigRevisionConflict(conflict) {
		t.Fatal("expected project config revision conflict")
	}

	otherUnique := &pgconn.PgError{Code: "23505", ConstraintName: "other_constraint"}
	if isProjectConfigRevisionConflict(otherUnique) {
		t.Fatal("did not expect unrelated unique violation to retry")
	}
	if isProjectConfigRevisionConflict(errors.New("plain error")) {
		t.Fatal("did not expect non pg error to retry")
	}
	if maxProjectConfigRevisionAttempts != 3 {
		t.Fatalf("expected 3 config revision attempts, got %d", maxProjectConfigRevisionAttempts)
	}
}

func TestProjectGovernanceRepositoryMapsEvidenceBudgetAndArchive(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	eventID := uuid.New()
	now := time.Now()

	evidence, err := evidenceRefFromRecord(queries.ProjectEvidenceRef{
		ID:                 uuid.New(),
		TenantID:           tenantID,
		ProjectID:          projectID,
		EvidenceType:       "execution_log",
		Title:              "测试日志",
		SourceType:         "artifact",
		SourceRef:          "s3://bucket/log.txt",
		SubmittedByType:    "digital_employee",
		VerificationStatus: "submitted",
		Metadata:           []byte(`{"suite":"regression"}`),
		CreatedEventID:     uuid.NullUUID{UUID: eventID, Valid: true},
		CreatedAt:          pgtype.Timestamptz{Time: now, Valid: true},
	})
	if err != nil {
		t.Fatalf("map evidence: %v", err)
	}
	if evidence.Metadata["suite"] != "regression" || *evidence.CreatedEventID != eventID {
		t.Fatalf("unexpected evidence mapping: %#v", evidence)
	}

	summary := budgetSummaryFromRecord(queries.GetProjectBudgetSummaryRow{
		EstimatedTokens: 1000,
		ActualTokens:    800,
		EstimatedCost:   numericFromString(t, "0.120000"),
		ActualCost:      numericFromString(t, "0.096000"),
		LedgerCount:     2,
	})
	if summary.ActualTokens != 800 || summary.LedgerCount != 2 {
		t.Fatalf("unexpected budget summary: %#v", summary)
	}
}

func numericFromString(t *testing.T, value string) pgtype.Numeric {
	t.Helper()

	var numeric pgtype.Numeric
	if err := numeric.Scan(value); err != nil {
		t.Fatalf("scan numeric %q: %v", value, err)
	}
	return numeric
}

func (r *memoryRepository) CreateEvidenceRef(ctx context.Context, req CreateEvidenceRefRequest) (ProjectEvidenceRef, error) {
	return ProjectEvidenceRef{}, nil
}

func (r *memoryRepository) ListEvidenceRefs(ctx context.Context, tenantID, projectID uuid.UUID, status *EvidenceVerificationStatus, limit, offset int32) ([]ProjectEvidenceRef, error) {
	return nil, nil
}

func (r *memoryRepository) UpdateEvidenceVerificationStatus(ctx context.Context, req UpdateEvidenceVerificationStatusRequest) (ProjectEvidenceRef, error) {
	return ProjectEvidenceRef{}, nil
}

func (r *memoryRepository) CreateArtifactRef(ctx context.Context, req CreateArtifactRefRequest) (ProjectArtifactRef, error) {
	return ProjectArtifactRef{}, nil
}

func (r *memoryRepository) ListArtifactRefs(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]ProjectArtifactRef, error) {
	return nil, nil
}

func (r *memoryRepository) UpdateArtifactRetention(ctx context.Context, req UpdateArtifactRetentionRequest) (ProjectArtifactRef, error) {
	return ProjectArtifactRef{}, nil
}

func (r *memoryRepository) CreateReportRef(ctx context.Context, req CreateReportRefRequest) (ProjectReportRef, error) {
	return ProjectReportRef{}, nil
}

func (r *memoryRepository) ListReportRefs(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]ProjectReportRef, error) {
	return nil, nil
}

func (r *memoryRepository) CreateBudgetLedgerEntry(ctx context.Context, req CreateBudgetLedgerEntryRequest) (ProjectBudgetLedgerEntry, error) {
	return ProjectBudgetLedgerEntry{}, nil
}

func (r *memoryRepository) ListBudgetLedger(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]ProjectBudgetLedgerEntry, error) {
	return nil, nil
}

func (r *memoryRepository) GetBudgetSummary(ctx context.Context, tenantID, projectID uuid.UUID) (ProjectBudgetSummary, error) {
	return ProjectBudgetSummary{}, nil
}

func (r *memoryRepository) CreateAcceptanceRecord(ctx context.Context, req CreateAcceptanceRecordRequest) (ProjectAcceptanceRecord, error) {
	return ProjectAcceptanceRecord{}, nil
}

func (r *memoryRepository) GetLatestAcceptanceRecord(ctx context.Context, tenantID, projectID uuid.UUID) (ProjectAcceptanceRecord, error) {
	return ProjectAcceptanceRecord{}, nil
}

func (r *memoryRepository) CreateArchiveSnapshot(ctx context.Context, req CreateArchiveSnapshotRequest) (ProjectArchiveSnapshot, error) {
	return ProjectArchiveSnapshot{}, nil
}

func (r *memoryRepository) ListArchiveSnapshots(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]ProjectArchiveSnapshot, error) {
	return nil, nil
}

func (r *memoryRepository) ListConfigRevisions(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]ProjectConfigRevision, error) {
	return nil, nil
}

func (r *memoryRepository) GetConfigRevision(ctx context.Context, tenantID, projectID, revisionID uuid.UUID) (ProjectConfigRevision, error) {
	return ProjectConfigRevision{}, nil
}
