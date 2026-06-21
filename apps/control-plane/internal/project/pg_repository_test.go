package project

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
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
	jobID := uuid.New()
	routeID := uuid.New()
	stage := int32(2)
	task, err := taskFromRecord(queries.ProjectTask{
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
		CoordinationJobID:         uuid.NullUUID{UUID: jobID, Valid: true},
		RouteDecisionID:           uuid.NullUUID{UUID: routeID, Valid: true},
		PlannedTaskKey:            pgtype.Text{String: "review_1", Valid: true},
		TaskKind:                  pgtype.Text{String: "review", Valid: true},
		StageIndex:                pgtype.Int4{Int32: stage, Valid: true},
		ExpectedOutputs:           []byte(`["execution_summary","evidence_refs"]`),
		InputRequirements:         []byte(`{"needs":["implementation_summary"]}`),
		HandoffContract:           []byte(`{"required_refs":["evidence_refs"]}`),
		PlannerMetadata:           []byte(`{"planner":"test"}`),
		CreatedAt:                 pgtype.Timestamptz{Time: now, Valid: true},
		UpdatedAt:                 pgtype.Timestamptz{Time: now, Valid: true},
	})
	if err != nil {
		t.Fatalf("map task: %v", err)
	}
	if task.DemandID == nil || *task.DemandID != demandID {
		t.Fatalf("expected demand id, got %#v", task.DemandID)
	}
	if task.AssignedDigitalEmployeeID == nil || *task.AssignedDigitalEmployeeID != employeeID || !task.RequiresHumanApproval {
		t.Fatalf("unexpected task: %#v", task)
	}
	if task.CoordinationJobID == nil || *task.CoordinationJobID != jobID {
		t.Fatalf("expected coordination job id, got %#v", task.CoordinationJobID)
	}
	if task.RouteDecisionID == nil || *task.RouteDecisionID != routeID {
		t.Fatalf("expected route decision id, got %#v", task.RouteDecisionID)
	}
	if task.PlannedTaskKey == nil || *task.PlannedTaskKey != "review_1" {
		t.Fatalf("expected planned task key, got %#v", task.PlannedTaskKey)
	}
	if task.TaskKind == nil || *task.TaskKind != "review" {
		t.Fatalf("expected task kind, got %#v", task.TaskKind)
	}
	if task.StageIndex == nil || *task.StageIndex != stage {
		t.Fatalf("expected stage index %d, got %#v", stage, task.StageIndex)
	}
	require.Equal(t, []any{"execution_summary", "evidence_refs"}, task.ExpectedOutputs)
	require.Equal(t, []any{"implementation_summary"}, task.InputRequirements["needs"])
	require.Equal(t, []any{"evidence_refs"}, task.HandoffContract["required_refs"])
	require.Equal(t, "test", task.PlannerMetadata["planner"])

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

func TestProjectTaskContractMappersRejectWrongJSONShapes(t *testing.T) {
	now := time.Date(2026, 6, 14, 10, 0, 0, 0, time.UTC)
	tenantID := uuid.New()
	projectID := uuid.New()

	_, err := taskFromRecord(queries.ProjectTask{
		ID:                        uuid.New(),
		TenantID:                  tenantID,
		ProjectID:                 projectID,
		Title:                     "复核证据",
		Status:                    "blocked",
		RequiresHumanApproval:     true,
		ExpectedOutputs:           []byte(`{"bad":true}`),
		InputRequirements:         []byte(`{}`),
		HandoffContract:           []byte(`{}`),
		PlannerMetadata:           []byte(`{}`),
		CreatedAt:                 pgtype.Timestamptz{Time: now, Valid: true},
		UpdatedAt:                 pgtype.Timestamptz{Time: now, Valid: true},
		AssignedDigitalEmployeeID: uuid.NullUUID{},
	})
	require.ErrorContains(t, err, "expected_outputs")

	_, err = completionContractFromRecord(queries.GetProjectTaskCompletionContractRow{
		ID:              uuid.New(),
		TenantID:        tenantID,
		ProjectID:       projectID,
		ExpectedOutputs: []byte(`[]`),
		HandoffContract: []byte(`["bad"]`),
	})
	require.ErrorContains(t, err, "handoff_contract")

	for _, tc := range []struct {
		name  string
		field string
		row   queries.ProjectTask
	}{
		{
			name:  "task expected outputs null",
			field: "expected_outputs",
			row:   projectTaskContractShapeRow(tenantID, projectID, []byte("null"), []byte(`{}`), []byte(`{}`), []byte(`{}`)),
		},
		{
			name:  "task input requirements null",
			field: "input_requirements",
			row:   projectTaskContractShapeRow(tenantID, projectID, []byte(`[]`), []byte("null"), []byte(`{}`), []byte(`{}`)),
		},
		{
			name:  "task handoff contract null",
			field: "handoff_contract",
			row:   projectTaskContractShapeRow(tenantID, projectID, []byte(`[]`), []byte(`{}`), []byte("null"), []byte(`{}`)),
		},
		{
			name:  "task planner metadata null",
			field: "planner_metadata",
			row:   projectTaskContractShapeRow(tenantID, projectID, []byte(`[]`), []byte(`{}`), []byte(`{}`), []byte("null")),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := taskFromRecord(tc.row)
			require.ErrorContains(t, err, tc.field)
		})
	}

	for _, tc := range []struct {
		name            string
		field           string
		expectedOutputs []byte
		handoffContract []byte
	}{
		{
			name:            "completion expected outputs null",
			field:           "expected_outputs",
			expectedOutputs: []byte("null"),
			handoffContract: []byte(`{}`),
		},
		{
			name:            "completion handoff contract null",
			field:           "handoff_contract",
			expectedOutputs: []byte(`[]`),
			handoffContract: []byte("null"),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := completionContractFromRecord(queries.GetProjectTaskCompletionContractRow{
				ID:              uuid.New(),
				TenantID:        tenantID,
				ProjectID:       projectID,
				ExpectedOutputs: tc.expectedOutputs,
				HandoffContract: tc.handoffContract,
			})
			require.ErrorContains(t, err, tc.field)
		})
	}
}

func projectTaskContractShapeRow(tenantID, projectID uuid.UUID, expectedOutputs, inputRequirements, handoffContract, plannerMetadata []byte) queries.ProjectTask {
	now := time.Date(2026, 6, 14, 10, 0, 0, 0, time.UTC)
	return queries.ProjectTask{
		ID:                    uuid.New(),
		TenantID:              tenantID,
		ProjectID:             projectID,
		Title:                 "复核证据",
		Status:                "blocked",
		RequiresHumanApproval: true,
		ExpectedOutputs:       expectedOutputs,
		InputRequirements:     inputRequirements,
		HandoffContract:       handoffContract,
		PlannerMetadata:       plannerMetadata,
		CreatedAt:             pgtype.Timestamptz{Time: now, Valid: true},
		UpdatedAt:             pgtype.Timestamptz{Time: now, Valid: true},
	}
}

func TestCreateProjectTaskPersistsGraphContractFields(t *testing.T) {
	repo, tenantID := newProjectRepositoryTestStore(t)
	projectID := createProjectFixture(t, repo, tenantID)
	jobID := uuid.New()
	routeID := uuid.New()
	demandID := uuid.New()
	employeeID := uuid.New()
	stage := int32(2)

	task, err := repo.CreateProjectTask(context.Background(), CreateProjectTaskRequest{
		TenantID:                  tenantID,
		ProjectID:                 projectID,
		DemandID:                  &demandID,
		Title:                     "复核证据",
		Summary:                   "检查上游产物",
		Status:                    "blocked",
		AssignedDigitalEmployeeID: &employeeID,
		CoordinationJobID:         &jobID,
		RouteDecisionID:           &routeID,
		PlannedTaskKey:            strPtr("review_1"),
		TaskKind:                  strPtr("review"),
		StageIndex:                &stage,
		ExpectedOutputs:           []any{"execution_summary", "evidence_refs"},
		InputRequirements:         map[string]any{"needs": []any{"implementation_summary"}},
		HandoffContract:           map[string]any{"required_refs": []any{"evidence_refs"}},
		PlannerMetadata:           map[string]any{"planner": "test"},
	})
	require.NoError(t, err)
	require.NotNil(t, task.CoordinationJobID)
	require.Equal(t, jobID, *task.CoordinationJobID)
	require.NotNil(t, task.RouteDecisionID)
	require.Equal(t, routeID, *task.RouteDecisionID)
	require.Equal(t, "review_1", *task.PlannedTaskKey)
	require.Equal(t, "review", *task.TaskKind)
	require.Equal(t, stage, *task.StageIndex)
	require.Equal(t, []any{"execution_summary", "evidence_refs"}, task.ExpectedOutputs)
	require.Equal(t, []any{"implementation_summary"}, task.InputRequirements["needs"])
	require.Equal(t, []any{"evidence_refs"}, task.HandoffContract["required_refs"])
	require.Equal(t, "test", task.PlannerMetadata["planner"])
}

func TestQueueProjectTaskWithAttemptMovesPlannedTaskToQueued(t *testing.T) {
	repo, tenantID := newProjectRepositoryTestStore(t)
	projectID := createProjectFixture(t, repo, tenantID)
	employeeID := uuid.New()
	task, err := repo.CreateProjectTask(context.Background(), CreateProjectTaskRequest{
		TenantID:                  tenantID,
		ProjectID:                 projectID,
		Title:                     "验证状态机",
		Status:                    ProjectTaskStatusPlanned,
		AssignedDigitalEmployeeID: &employeeID,
	})
	require.NoError(t, err)
	runID := uuid.New()
	runtimeTaskID := uuid.New()
	runtimeNodeID := uuid.New()

	result, err := repo.QueueProjectTaskWithAttempt(context.Background(), QueueProjectTaskRequest{
		TenantID:                      tenantID,
		ProjectID:                     projectID,
		ProjectTaskID:                 task.ID,
		DigitalEmployeeID:             employeeID,
		DigitalEmployeeRunID:          &runID,
		RuntimeTaskID:                 &runtimeTaskID,
		RuntimeNodeID:                 &runtimeNodeID,
		IdempotencyKey:                "project-task:" + task.ID.String() + ":attempt:1:queue",
		LeaseToken:                    "lease-token-1",
		ExecutionContextPacket:        map[string]any{"task_title": task.Title},
		ExecutionContextPacketVersion: "v1",
	})
	require.NoError(t, err)
	require.Equal(t, ProjectTaskStatusQueued, result.Task.Status)
	require.Equal(t, result.Attempt.ID, *result.Task.CurrentAttemptID)
	require.Equal(t, runID, *result.Task.DigitalEmployeeRunID)
	require.Equal(t, runtimeTaskID, *result.Task.RuntimeTaskID)
	require.Equal(t, int32(1), result.Attempt.AttemptNo)
	require.Equal(t, ProjectTaskAttemptStatusQueued, result.Attempt.Status)
	require.Equal(t, runID, *result.Attempt.DigitalEmployeeRunID)
	require.Equal(t, runtimeTaskID, *result.Attempt.RuntimeTaskID)
	require.Equal(t, runtimeNodeID, *result.Attempt.RuntimeNodeID)
	require.Equal(t, "lease-token-1", result.Attempt.LeaseToken)
	require.Equal(t, "v1", result.Attempt.ExecutionContextPacketVersion)
	require.Equal(t, "project_coordinator", result.Event.ActorType)
	require.Equal(t, task.ID.String(), result.Event.ActorID)
	require.Equal(t, result.Attempt.ID.String(), result.Event.Payload["project_task_attempt_id"])
	require.Equal(t, ProjectTaskStatusQueued, result.Event.Payload["project_task_status"])
	require.Equal(t, runID.String(), result.Event.Payload["digital_employee_run_id"])
	require.Equal(t, runtimeTaskID.String(), result.Event.Payload["runtime_task_id"])
	require.Equal(t, runtimeNodeID.String(), result.Event.Payload["runtime_node_id"])
}

func TestStartProjectTaskAttemptAdvancesTaskAndAttempt(t *testing.T) {
	repo, tenantID := newProjectRepositoryTestStore(t)
	writebacks := repo.(ProjectTaskAttemptWritebackRepository)
	projectID := createProjectFixture(t, repo, tenantID)
	employeeID := uuid.New()
	task, err := repo.CreateProjectTask(context.Background(), CreateProjectTaskRequest{
		TenantID:                  tenantID,
		ProjectID:                 projectID,
		Title:                     "验证 attempt started 写回",
		Status:                    ProjectTaskStatusPlanned,
		AssignedDigitalEmployeeID: &employeeID,
	})
	require.NoError(t, err)
	runID := uuid.New()
	runtimeTaskID := uuid.New()
	nodeID := uuid.New()
	queued, err := repo.QueueProjectTaskWithAttempt(context.Background(), QueueProjectTaskRequest{
		TenantID:             tenantID,
		ProjectID:            projectID,
		ProjectTaskID:        task.ID,
		DigitalEmployeeID:    employeeID,
		DigitalEmployeeRunID: &runID,
		RuntimeTaskID:        &runtimeTaskID,
		RuntimeNodeID:        &nodeID,
		IdempotencyKey:       "project-task:" + task.ID.String() + ":attempt:1:queue",
		LeaseToken:           "lease-token-1",
	})
	require.NoError(t, err)

	started, err := writebacks.StartProjectTaskAttemptWriteback(context.Background(), StartProjectTaskAttemptRequest{
		ProjectTaskAttemptRuntimeRequest: ProjectTaskAttemptRuntimeRequest{
			TenantID:       tenantID,
			AttemptID:      queued.Attempt.ID,
			ProjectTaskID:  queued.Task.ID,
			RuntimeNodeID:  nodeID,
			LeaseToken:     queued.Attempt.LeaseToken,
			IdempotencyKey: "start-" + queued.Attempt.ID.String(),
		},
	})

	require.NoError(t, err)
	require.Equal(t, ProjectTaskAttemptStatusRunning, started.Attempt.Status)
	require.Equal(t, ProjectTaskStatusRunning, started.Task.Status)
	require.NotNil(t, started.Attempt.StartedAt)
	require.NotNil(t, started.Attempt.RenewedAt)
}

func TestCompleteProjectTaskAttemptWritebackPersistsTerminalFacts(t *testing.T) {
	repo, tenantID := newProjectRepositoryTestStore(t)
	writebacks := repo.(ProjectTaskAttemptWritebackRepository)
	projectID := createProjectFixture(t, repo, tenantID)
	employeeID := uuid.New()
	task, err := repo.CreateProjectTask(context.Background(), CreateProjectTaskRequest{
		TenantID:                  tenantID,
		ProjectID:                 projectID,
		Title:                     "验证 attempt complete 写回",
		Status:                    ProjectTaskStatusPlanned,
		AssignedDigitalEmployeeID: &employeeID,
	})
	require.NoError(t, err)
	runID := uuid.New()
	runtimeTaskID := uuid.New()
	nodeID := uuid.New()
	queued, err := repo.QueueProjectTaskWithAttempt(context.Background(), QueueProjectTaskRequest{
		TenantID:             tenantID,
		ProjectID:            projectID,
		ProjectTaskID:        task.ID,
		DigitalEmployeeID:    employeeID,
		DigitalEmployeeRunID: &runID,
		RuntimeTaskID:        &runtimeTaskID,
		RuntimeNodeID:        &nodeID,
		IdempotencyKey:       "project-task:" + task.ID.String() + ":attempt:1:queue",
		LeaseToken:           "lease-token-complete",
	})
	require.NoError(t, err)
	providerSessionID := "provider-session-complete"
	_, err = writebacks.StartProjectTaskAttemptWriteback(context.Background(), StartProjectTaskAttemptRequest{
		ProjectTaskAttemptRuntimeRequest: ProjectTaskAttemptRuntimeRequest{
			TenantID:          tenantID,
			AttemptID:         queued.Attempt.ID,
			ProjectTaskID:     queued.Task.ID,
			RuntimeNodeID:     nodeID,
			LeaseToken:        queued.Attempt.LeaseToken,
			IdempotencyKey:    "start-" + queued.Attempt.ID.String(),
			ProviderSessionID: &providerSessionID,
		},
	})
	require.NoError(t, err)

	completed, err := writebacks.CompleteProjectTaskAttemptWriteback(context.Background(), CompleteProjectTaskAttemptRequest{
		ProjectTaskAttemptRuntimeRequest: ProjectTaskAttemptRuntimeRequest{
			TenantID:          tenantID,
			AttemptID:         queued.Attempt.ID,
			ProjectTaskID:     queued.Task.ID,
			RuntimeNodeID:     nodeID,
			LeaseToken:        queued.Attempt.LeaseToken,
			IdempotencyKey:    "complete-" + queued.Attempt.ID.String(),
			ProviderSessionID: &providerSessionID,
		},
		DigitalEmployeeID:     employeeID,
		Conclusion:            "真实开发库 complete 写回验证通过",
		EvidenceRefs:          []any{map[string]any{"type": "dev_db", "ref": "project_task_attempts"}},
		ArtifactRefs:          []any{map[string]any{"type": "test_log", "ref": "pg_repository_test"}},
		ConfidenceFactors:     map[string]any{"writeback_path": "project_task_attempt_writeback"},
		Uncertainty:           "无",
		MissingInformation:    []any{},
		RecommendedNextAction: "进入验收",
		RequiresHumanReview:   false,
	})

	require.NoError(t, err)
	require.Equal(t, ProjectTaskStatusCompleted, completed.Task.Status)
	require.Equal(t, ProjectEventTaskCompleted, completed.Event.EventType)
	require.Equal(t, queued.Task.ID.String(), completed.Event.Payload["project_task_id"])
	require.Equal(t, queued.Attempt.ID.String(), completed.Event.Payload["project_task_attempt_id"])
	require.Equal(t, "真实开发库 complete 写回验证通过", completed.Summary.Conclusion)
	require.Equal(t, employeeID, completed.Summary.DigitalEmployeeID)
	require.NotNil(t, completed.Summary.CreatedEventID)
	require.Equal(t, completed.Event.ID, *completed.Summary.CreatedEventID)

	attempt, err := repo.GetProjectTaskAttempt(context.Background(), tenantID, queued.Attempt.ID)
	require.NoError(t, err)
	require.Equal(t, ProjectTaskAttemptStatusSucceeded, attempt.Status)
	require.NotNil(t, attempt.FinishedAt)
	require.NotNil(t, attempt.TerminalEventID)
	require.Equal(t, completed.Event.ID, *attempt.TerminalEventID)
	require.NotNil(t, attempt.ProviderSessionID)
	require.Equal(t, providerSessionID, *attempt.ProviderSessionID)
	persistedTask, err := repo.GetProjectTask(context.Background(), tenantID, queued.Task.ID)
	require.NoError(t, err)
	require.Equal(t, ProjectTaskStatusCompleted, persistedTask.Status)
	require.NotNil(t, persistedTask.TerminalEventID)
	require.Equal(t, completed.Event.ID, *persistedTask.TerminalEventID)
}

func TestFailProjectTaskAttemptWritebackPersistsTerminalFacts(t *testing.T) {
	repo, tenantID := newProjectRepositoryTestStore(t)
	writebacks := repo.(ProjectTaskAttemptWritebackRepository)
	projectID := createProjectFixture(t, repo, tenantID)
	employeeID := uuid.New()
	task, err := repo.CreateProjectTask(context.Background(), CreateProjectTaskRequest{
		TenantID:                  tenantID,
		ProjectID:                 projectID,
		Title:                     "验证 attempt fail 写回",
		Status:                    ProjectTaskStatusPlanned,
		AssignedDigitalEmployeeID: &employeeID,
	})
	require.NoError(t, err)
	runID := uuid.New()
	runtimeTaskID := uuid.New()
	nodeID := uuid.New()
	queued, err := repo.QueueProjectTaskWithAttempt(context.Background(), QueueProjectTaskRequest{
		TenantID:             tenantID,
		ProjectID:            projectID,
		ProjectTaskID:        task.ID,
		DigitalEmployeeID:    employeeID,
		DigitalEmployeeRunID: &runID,
		RuntimeTaskID:        &runtimeTaskID,
		RuntimeNodeID:        &nodeID,
		IdempotencyKey:       "project-task:" + task.ID.String() + ":attempt:1:queue",
		LeaseToken:           "lease-token-fail",
	})
	require.NoError(t, err)
	providerSessionID := "provider-session-fail"
	_, err = writebacks.StartProjectTaskAttemptWriteback(context.Background(), StartProjectTaskAttemptRequest{
		ProjectTaskAttemptRuntimeRequest: ProjectTaskAttemptRuntimeRequest{
			TenantID:          tenantID,
			AttemptID:         queued.Attempt.ID,
			ProjectTaskID:     queued.Task.ID,
			RuntimeNodeID:     nodeID,
			LeaseToken:        queued.Attempt.LeaseToken,
			IdempotencyKey:    "start-" + queued.Attempt.ID.String(),
			ProviderSessionID: &providerSessionID,
		},
	})
	require.NoError(t, err)
	retryable := true

	failed, err := writebacks.FailProjectTaskAttemptWriteback(context.Background(), FailProjectTaskAttemptRequest{
		ProjectTaskAttemptRuntimeRequest: ProjectTaskAttemptRuntimeRequest{
			TenantID:          tenantID,
			AttemptID:         queued.Attempt.ID,
			ProjectTaskID:     queued.Task.ID,
			RuntimeNodeID:     nodeID,
			LeaseToken:        queued.Attempt.LeaseToken,
			IdempotencyKey:    "fail-" + queued.Attempt.ID.String(),
			ProviderSessionID: &providerSessionID,
		},
		DigitalEmployeeID: employeeID,
		FailureSummary:    "Provider 执行失败",
		FailureFamily:     "provider_error",
		Retryable:         &retryable,
	})

	require.NoError(t, err)
	require.Equal(t, ProjectTaskStatusFailed, failed.Task.Status)
	require.Equal(t, ProjectEventTaskFailed, failed.Event.EventType)
	require.Equal(t, queued.Task.ID.String(), failed.Event.Payload["project_task_id"])
	require.Equal(t, queued.Attempt.ID.String(), failed.Event.Payload["project_task_attempt_id"])
	require.Equal(t, "Provider 执行失败", failed.Event.Payload["failure_summary"])
	require.Equal(t, "provider_error", failed.Event.Payload["failure_family"])

	attempt, err := repo.GetProjectTaskAttempt(context.Background(), tenantID, queued.Attempt.ID)
	require.NoError(t, err)
	require.Equal(t, ProjectTaskAttemptStatusFailed, attempt.Status)
	require.NotNil(t, attempt.FinishedAt)
	require.NotNil(t, attempt.TerminalEventID)
	require.Equal(t, failed.Event.ID, *attempt.TerminalEventID)
	require.NotNil(t, attempt.Retryable)
	require.True(t, *attempt.Retryable)
	require.NotNil(t, attempt.FailureFamily)
	require.Equal(t, "provider_error", *attempt.FailureFamily)
	require.NotNil(t, attempt.FailureMessage)
	require.Equal(t, "Provider 执行失败", *attempt.FailureMessage)
	persistedTask, err := repo.GetProjectTask(context.Background(), tenantID, queued.Task.ID)
	require.NoError(t, err)
	require.Equal(t, ProjectTaskStatusFailed, persistedTask.Status)
	require.NotNil(t, persistedTask.TerminalEventID)
	require.Equal(t, failed.Event.ID, *persistedTask.TerminalEventID)
}

func TestCreateProjectTaskGraphCreatesTasksEdgesAndEvents(t *testing.T) {
	repo, tenantID := newProjectRepositoryTestStore(t)
	projectID := createProjectFixture(t, repo, tenantID)
	demandID := createDemandFixture(t, repo, tenantID, projectID)
	jobID := createCoordinationJobFixture(t, repo, tenantID, projectID)
	routeID := createRouteDecisionFixture(t, repo, tenantID, projectID, jobID, demandID)
	employeeID := uuid.New()
	stageZero := int32(0)
	stageOne := int32(1)

	result, err := repo.CreateProjectTaskGraph(context.Background(), CreateProjectTaskGraphRequest{
		TenantID:          tenantID,
		ProjectID:         projectID,
		DemandID:          demandID,
		CoordinationJobID: jobID,
		RouteDecisionID:   routeID,
		Tasks: []ProjectTaskGraphCreateTask{
			{Key: "t1", Title: "分析", Summary: "分析", Status: "planned", AssignedDigitalEmployeeID: employeeID, ExpectedOutputs: []any{"execution_summary"}, InputRequirements: map[string]any{}, HandoffContract: map[string]any{}, PlannerMetadata: map[string]any{"planner": "test"}, StageIndex: &stageZero, TaskKind: "analysis", RiskLevel: "medium"},
			{Key: "t2", Title: "复核", Summary: "复核", Status: "blocked", AssignedDigitalEmployeeID: employeeID, ExpectedOutputs: []any{"execution_summary"}, InputRequirements: map[string]any{}, HandoffContract: map[string]any{}, PlannerMetadata: map[string]any{"planner": "test"}, StageIndex: &stageOne, TaskKind: "review", RiskLevel: "normal", BlockedByKeys: []string{"t1"}},
		},
	})

	require.NoError(t, err)
	require.Len(t, result.Tasks, 2)
	require.Len(t, result.Dependencies, 1)
	require.NotEqual(t, uuid.Nil, result.GraphEventID)
	require.Equal(t, "t1", result.Tasks[0].PlannedTaskKey)
	require.True(t, result.Tasks[0].IsRoot)
	require.Equal(t, "t2", result.Tasks[1].PlannedTaskKey)
	require.False(t, result.Tasks[1].IsRoot)
	require.Equal(t, result.Tasks[1].ID, result.Dependencies[0].DependentTaskID)
	require.Equal(t, result.Tasks[0].ID, result.Dependencies[0].BlockerTaskID)

	tasks, err := repo.ListProjectTasksByCoordinationJob(context.Background(), tenantID, projectID, jobID)
	require.NoError(t, err)
	require.Len(t, tasks, 2)
	require.Equal(t, "planned", tasks[0].Status)
	require.Equal(t, "blocked", tasks[1].Status)
	require.Equal(t, "t1", *tasks[0].PlannedTaskKey)
	require.Equal(t, "t2", *tasks[1].PlannedTaskKey)
	require.Equal(t, routeID, *tasks[0].RouteDecisionID)
	require.Equal(t, demandID, *tasks[0].DemandID)
	require.Equal(t, []any{"execution_summary"}, tasks[0].ExpectedOutputs)

	dependencies, err := repo.ListProjectTaskDependencies(context.Background(), tenantID, projectID, []uuid.UUID{tasks[1].ID})
	require.NoError(t, err)
	require.Len(t, dependencies, 1)
	require.Equal(t, tasks[1].ID, dependencies[0].DependentTaskID)
	require.Equal(t, tasks[0].ID, dependencies[0].BlockerTaskID)

	events, err := repo.ListProjectEvents(context.Background(), tenantID, projectID, 20, 0)
	require.NoError(t, err)
	requireEventCount(t, events, ProjectEventTaskCreated, 2)
	requireEventCount(t, events, ProjectEventTaskGraphPlanned, 1)
}

func TestProjectTaskGraphDemandReadReturnsAllTasks(t *testing.T) {
	repo, tenantID := newProjectRepositoryTestStore(t)
	projectID := createProjectFixture(t, repo, tenantID)
	demandID := createDemandFixture(t, repo, tenantID, projectID)
	jobID := createCoordinationJobFixture(t, repo, tenantID, projectID)
	routeID := createRouteDecisionFixture(t, repo, tenantID, projectID, jobID, demandID)
	employeeID := uuid.New()
	tasks := make([]ProjectTaskGraphCreateTask, 0, 55)
	for i := 0; i < 55; i++ {
		stage := int32(i)
		key := fmt.Sprintf("t%d", i+1)
		planned := ProjectTaskGraphCreateTask{
			Key:                       key,
			Title:                     fmt.Sprintf("任务 %02d", i+1),
			Summary:                   "完整需求图",
			Status:                    "planned",
			AssignedDigitalEmployeeID: employeeID,
			TaskKind:                  "analysis",
			StageIndex:                &stage,
			RiskLevel:                 "medium",
			ExpectedOutputs:           []any{"execution_summary"},
			InputRequirements:         map[string]any{},
			HandoffContract:           map[string]any{},
			PlannerMetadata:           map[string]any{"planner": "test"},
		}
		if i > 0 {
			planned.Status = "blocked"
			planned.BlockedByKeys = []string{fmt.Sprintf("t%d", i)}
		}
		tasks = append(tasks, planned)
	}

	_, err := repo.CreateProjectTaskGraph(context.Background(), CreateProjectTaskGraphRequest{
		TenantID:          tenantID,
		ProjectID:         projectID,
		DemandID:          demandID,
		CoordinationJobID: jobID,
		RouteDecisionID:   routeID,
		Tasks:             tasks,
	})
	require.NoError(t, err)

	graph, err := repo.GetProjectTaskGraph(context.Background(), GetProjectTaskGraphRequest{
		TenantID: tenantID, ProjectID: projectID, DemandID: &demandID,
	})
	require.NoError(t, err)
	require.Len(t, graph.Nodes, 55)
	require.Len(t, graph.Edges, 54)
	require.NotNil(t, graph.Employees)
	require.NotNil(t, graph.Runs)
	require.NotNil(t, graph.ExecutionSummaries)
	require.NotNil(t, graph.RecentEvents)
	require.NotNil(t, graph.DecisionRequests)
}

func TestListWorkflowInstancesFiltersVisibleDemandsAndSortsRunningFirst(t *testing.T) {
	repo, tenantID := newProjectRepositoryTestStore(t)
	ctx := context.Background()
	visibleProjectID := createProjectFixture(t, repo, tenantID)
	hiddenProjectID := createProjectFixture(t, repo, tenantID)
	actorID := uuid.New()
	visibleDemandID := createDemandFixture(t, repo, tenantID, visibleProjectID)
	hiddenDemandID := createDemandFixture(t, repo, tenantID, hiddenProjectID)
	jobID := createCoordinationJobFixture(t, repo, tenantID, visibleProjectID)
	routeID := createRouteDecisionFixture(t, repo, tenantID, visibleProjectID, jobID, visibleDemandID)
	employeeID := uuid.New()
	stage := int32(1)

	_, err := repo.ReplaceProjectMembers(ctx, tenantID, visibleProjectID, []ProjectMemberInput{{
		PrincipalType:       PrincipalTypeHumanUser,
		PrincipalID:         actorID,
		ProjectRole:         ProjectRoleObserver,
		DisplayNameSnapshot: "观察者",
	}})
	require.NoError(t, err)
	_, err = repo.CreateProjectTaskGraph(ctx, CreateProjectTaskGraphRequest{
		TenantID:          tenantID,
		ProjectID:         visibleProjectID,
		DemandID:          visibleDemandID,
		CoordinationJobID: jobID,
		RouteDecisionID:   routeID,
		Tasks: []ProjectTaskGraphCreateTask{{
			Key:                       "root",
			Title:                     "定位问题",
			Status:                    "assigned",
			AssignedDigitalEmployeeID: employeeID,
			TaskKind:                  "analysis",
			StageIndex:                &stage,
			RiskLevel:                 "medium",
			ExpectedOutputs:           []any{"summary"},
			InputRequirements:         map[string]any{},
			HandoffContract:           map[string]any{},
			PlannerMetadata:           map[string]any{},
		}},
	})
	require.NoError(t, err)
	_, err = repo.AppendProjectEvent(ctx, AppendProjectEventRequest{
		TenantID:     tenantID,
		ProjectID:    visibleProjectID,
		EventType:    ProjectEventDecisionRequested,
		ActorType:    "project_coordinator",
		ActorID:      "project-coordinator:" + visibleProjectID.String(),
		ResourceType: strPtr("project_task"),
		ResourceID:   strPtr(visibleDemandID.String()),
		Summary:      "已创建恢复决策请求",
		Payload: map[string]any{
			"demand_id": visibleDemandID.String(),
		},
	})
	require.NoError(t, err)

	items, err := repo.ListWorkflowInstances(ctx, ListWorkflowInstancesRequest{
		TenantID:    tenantID,
		ActorUserID: actorID,
		Limit:       20,
	})
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, visibleDemandID, items[0].DemandID)
	require.NotEqual(t, hiddenDemandID, items[0].DemandID)
	require.Equal(t, int32(1), items[0].Progress.TotalNodes)
	require.Equal(t, int32(1), items[0].Progress.RunningNodes)
	require.Equal(t, WorkflowInstanceStatusRunning, items[0].Status)
	require.Equal(t, &jobID, items[0].SelectedCoordinationJobID)
	require.NotNil(t, items[0].Risk)
	require.Equal(t, "medium", items[0].Risk.Level)
	require.Equal(t, "project_tasks.risk_level", items[0].Risk.Source)
	require.Equal(t, int32(0), items[0].Progress.FailedNodes)
	require.Equal(t, int32(0), items[0].Progress.CancelledNodes)
	require.NotNil(t, items[0].RecentEvent)
	require.Equal(t, string(ProjectEventDecisionRequested), items[0].RecentEvent.EventType)
	require.Equal(t, "已创建恢复决策请求", items[0].RecentEvent.Summary)
}

func TestListWorkflowInstancesOrdersAttentionBeforePagination(t *testing.T) {
	repo, tenantID := newProjectRepositoryTestStore(t)
	ctx := context.Background()
	projectID := createProjectFixture(t, repo, tenantID)
	actorID := uuid.New()
	waitingDemandID := createDemandFixture(t, repo, tenantID, projectID)
	failedDemandID := createDemandFixture(t, repo, tenantID, projectID)
	cancelledFailedDemandID := createDemandFixtureWithStatusAndSourceRefs(t, repo, tenantID, projectID, ProjectDemandStatusCancelled, nil)
	cancelledDemandID := createDemandFixtureWithStatusAndSourceRefs(t, repo, tenantID, projectID, ProjectDemandStatusCancelled, nil)
	runningDemandID := createDemandFixture(t, repo, tenantID, projectID)
	planningDemandID := createDemandFixture(t, repo, tenantID, projectID)
	completedDemandID := createDemandFixture(t, repo, tenantID, projectID)
	waitingJobID := createCoordinationJobFixture(t, repo, tenantID, projectID)
	waitingRouteID := createRouteDecisionFixture(t, repo, tenantID, projectID, waitingJobID, waitingDemandID)
	failedJobID := createCoordinationJobFixture(t, repo, tenantID, projectID)
	failedRouteID := createRouteDecisionFixture(t, repo, tenantID, projectID, failedJobID, failedDemandID)
	cancelledFailedJobID := createCoordinationJobFixture(t, repo, tenantID, projectID)
	cancelledFailedRouteID := createRouteDecisionFixture(t, repo, tenantID, projectID, cancelledFailedJobID, cancelledFailedDemandID)
	runningJobID := createCoordinationJobFixture(t, repo, tenantID, projectID)
	runningRouteID := createRouteDecisionFixture(t, repo, tenantID, projectID, runningJobID, runningDemandID)
	completedJobID := createCoordinationJobFixture(t, repo, tenantID, projectID)
	completedRouteID := createRouteDecisionFixture(t, repo, tenantID, projectID, completedJobID, completedDemandID)
	employeeID := uuid.New()
	stage := int32(1)

	_, err := repo.ReplaceProjectMembers(ctx, tenantID, projectID, []ProjectMemberInput{{
		PrincipalType:       PrincipalTypeHumanUser,
		PrincipalID:         actorID,
		ProjectRole:         ProjectRoleObserver,
		DisplayNameSnapshot: "观察者",
	}})
	require.NoError(t, err)
	waitingGraph, err := repo.CreateProjectTaskGraph(ctx, CreateProjectTaskGraphRequest{
		TenantID:          tenantID,
		ProjectID:         projectID,
		DemandID:          waitingDemandID,
		CoordinationJobID: waitingJobID,
		RouteDecisionID:   waitingRouteID,
		Tasks: []ProjectTaskGraphCreateTask{{
			Key:                       "waiting",
			Title:                     "等待人工确认",
			Status:                    "blocked",
			AssignedDigitalEmployeeID: employeeID,
			TaskKind:                  "analysis",
			StageIndex:                &stage,
			RequiresHumanApproval:     true,
			ExpectedOutputs:           []any{"summary"},
			InputRequirements:         map[string]any{},
			HandoffContract:           map[string]any{},
			PlannerMetadata:           map[string]any{},
		}},
	})
	require.NoError(t, err)
	waitingTaskID := waitingGraph.Tasks[0].ID
	decisionRequest, err := repo.CreateDecisionRequest(ctx, CreateDecisionRequestRequest{
		TenantID:          tenantID,
		ProjectID:         projectID,
		ApprovalRequestID: uuid.New(),
		CoordinationJobID: &waitingJobID,
		ProjectTaskID:     &waitingTaskID,
		TargetUserID:      actorID,
		DecisionType:      "task_failure_recovery",
		TitleSnapshot:     "等待人工恢复决策",
		StatusSnapshot:    "pending",
	})
	require.NoError(t, err)
	_, err = repo.CreateProjectTaskGraph(ctx, CreateProjectTaskGraphRequest{
		TenantID:          tenantID,
		ProjectID:         projectID,
		DemandID:          failedDemandID,
		CoordinationJobID: failedJobID,
		RouteDecisionID:   failedRouteID,
		Tasks: []ProjectTaskGraphCreateTask{{
			Key:                       "failed",
			Title:                     "执行失败",
			Status:                    "failed",
			AssignedDigitalEmployeeID: employeeID,
			TaskKind:                  "analysis",
			StageIndex:                &stage,
			ExpectedOutputs:           []any{"summary"},
			InputRequirements:         map[string]any{},
			HandoffContract:           map[string]any{},
			PlannerMetadata:           map[string]any{},
		}},
	})
	require.NoError(t, err)
	_, err = repo.CreateProjectTaskGraph(ctx, CreateProjectTaskGraphRequest{
		TenantID:          tenantID,
		ProjectID:         projectID,
		DemandID:          cancelledFailedDemandID,
		CoordinationJobID: cancelledFailedJobID,
		RouteDecisionID:   cancelledFailedRouteID,
		Tasks: []ProjectTaskGraphCreateTask{{
			Key:                       "cancelled-failed",
			Title:                     "已取消但存在失败",
			Status:                    "failed",
			AssignedDigitalEmployeeID: employeeID,
			TaskKind:                  "analysis",
			StageIndex:                &stage,
			ExpectedOutputs:           []any{"summary"},
			InputRequirements:         map[string]any{},
			HandoffContract:           map[string]any{},
			PlannerMetadata:           map[string]any{},
		}},
	})
	require.NoError(t, err)
	_, err = repo.CreateProjectTaskGraph(ctx, CreateProjectTaskGraphRequest{
		TenantID:          tenantID,
		ProjectID:         projectID,
		DemandID:          runningDemandID,
		CoordinationJobID: runningJobID,
		RouteDecisionID:   runningRouteID,
		Tasks: []ProjectTaskGraphCreateTask{{
			Key:                       "running",
			Title:                     "执行巡检",
			Status:                    "running",
			AssignedDigitalEmployeeID: employeeID,
			TaskKind:                  "analysis",
			StageIndex:                &stage,
			ExpectedOutputs:           []any{"summary"},
			InputRequirements:         map[string]any{},
			HandoffContract:           map[string]any{},
			PlannerMetadata:           map[string]any{},
		}},
	})
	require.NoError(t, err)
	_, err = repo.CreateProjectTaskGraph(ctx, CreateProjectTaskGraphRequest{
		TenantID:          tenantID,
		ProjectID:         projectID,
		DemandID:          completedDemandID,
		CoordinationJobID: completedJobID,
		RouteDecisionID:   completedRouteID,
		Tasks: []ProjectTaskGraphCreateTask{{
			Key:                       "completed",
			Title:                     "执行完成",
			Status:                    "completed",
			AssignedDigitalEmployeeID: employeeID,
			TaskKind:                  "analysis",
			StageIndex:                &stage,
			ExpectedOutputs:           []any{"summary"},
			InputRequirements:         map[string]any{},
			HandoffContract:           map[string]any{},
			PlannerMetadata:           map[string]any{},
		}},
	})
	require.NoError(t, err)

	items, err := repo.ListWorkflowInstances(ctx, ListWorkflowInstancesRequest{
		TenantID:    tenantID,
		ActorUserID: actorID,
		Limit:       1,
	})
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, waitingDemandID, items[0].DemandID)
	require.NotEqual(t, planningDemandID, items[0].DemandID)
	require.NotEqual(t, runningDemandID, items[0].DemandID)
	require.Equal(t, WorkflowInstanceStatusWaitingHuman, items[0].Status)
	require.NotNil(t, items[0].CurrentBlocker)
	require.Equal(t, "decision_request", items[0].CurrentBlocker.Type)
	require.Equal(t, "等待人工恢复决策", items[0].CurrentBlocker.Title)
	require.Equal(t, decisionRequest.ID, *items[0].CurrentBlocker.ResourceID)

	items, err = repo.ListWorkflowInstances(ctx, ListWorkflowInstancesRequest{
		TenantID:    tenantID,
		ActorUserID: actorID,
		Limit:       1,
		Offset:      1,
	})
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, failedDemandID, items[0].DemandID)
	require.NotEqual(t, planningDemandID, items[0].DemandID)
	require.NotEqual(t, runningDemandID, items[0].DemandID)
	require.Equal(t, WorkflowInstanceStatusFailed, items[0].Status)
	require.NotNil(t, items[0].CurrentBlocker)
	require.Equal(t, "project_task", items[0].CurrentBlocker.Type)

	items, err = repo.ListWorkflowInstances(ctx, ListWorkflowInstancesRequest{
		TenantID:    tenantID,
		ActorUserID: actorID,
		Limit:       10,
	})
	require.NoError(t, err)
	itemsByDemandID := make(map[uuid.UUID]WorkflowInstanceSummary, len(items))
	for _, item := range items {
		itemsByDemandID[item.DemandID] = item
	}
	require.Equal(t, WorkflowInstanceStatusFailed, itemsByDemandID[cancelledFailedDemandID].Status)
	require.Equal(t, WorkflowInstanceStatusCancelled, itemsByDemandID[cancelledDemandID].Status)

	items, err = repo.ListWorkflowInstances(ctx, ListWorkflowInstancesRequest{
		TenantID:    tenantID,
		ActorUserID: actorID,
		Limit:       1,
		Offset:      4,
	})
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, planningDemandID, items[0].DemandID)
	require.Equal(t, WorkflowInstanceStatusPlanning, items[0].Status)

	items, err = repo.ListWorkflowInstances(ctx, ListWorkflowInstancesRequest{
		TenantID:    tenantID,
		ActorUserID: actorID,
		Limit:       1,
		Offset:      5,
	})
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, completedDemandID, items[0].DemandID)
	require.Equal(t, WorkflowInstanceStatusCompleted, items[0].Status)

	items, err = repo.ListWorkflowInstances(ctx, ListWorkflowInstancesRequest{
		TenantID:    tenantID,
		ActorUserID: actorID,
		Limit:       1,
		Offset:      6,
	})
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, cancelledDemandID, items[0].DemandID)
	require.Equal(t, WorkflowInstanceStatusCancelled, items[0].Status)
}

func TestListWorkflowInstancesUsesTaskHumanBlockerWithoutDecisionRequest(t *testing.T) {
	repo, tenantID := newProjectRepositoryTestStore(t)
	ctx := context.Background()
	projectID := createProjectFixture(t, repo, tenantID)
	actorID := uuid.New()
	demandID := createDemandFixture(t, repo, tenantID, projectID)
	jobID := createCoordinationJobFixture(t, repo, tenantID, projectID)
	routeID := createRouteDecisionFixture(t, repo, tenantID, projectID, jobID, demandID)
	employeeID := uuid.New()
	stage := int32(1)

	_, err := repo.ReplaceProjectMembers(ctx, tenantID, projectID, []ProjectMemberInput{{
		PrincipalType:       PrincipalTypeHumanUser,
		PrincipalID:         actorID,
		ProjectRole:         ProjectRoleObserver,
		DisplayNameSnapshot: "观察者",
	}})
	require.NoError(t, err)
	graph, err := repo.CreateProjectTaskGraph(ctx, CreateProjectTaskGraphRequest{
		TenantID:          tenantID,
		ProjectID:         projectID,
		DemandID:          demandID,
		CoordinationJobID: jobID,
		RouteDecisionID:   routeID,
		Tasks: []ProjectTaskGraphCreateTask{{
			Key:                       "human-review",
			Title:                     "等待人工复核",
			Status:                    "pending_review",
			AssignedDigitalEmployeeID: employeeID,
			TaskKind:                  "review",
			StageIndex:                &stage,
			ExpectedOutputs:           []any{"summary"},
			InputRequirements:         map[string]any{},
			HandoffContract:           map[string]any{},
			PlannerMetadata:           map[string]any{},
		}},
	})
	require.NoError(t, err)

	items, err := repo.ListWorkflowInstances(ctx, ListWorkflowInstancesRequest{
		TenantID:    tenantID,
		ActorUserID: actorID,
		Limit:       20,
	})
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, WorkflowInstanceStatusWaitingHuman, items[0].Status)
	require.NotEmpty(t, items[0].StatusReason)
	require.NotNil(t, items[0].CurrentBlocker)
	require.Equal(t, "project_task", items[0].CurrentBlocker.Type)
	require.Equal(t, "等待人工复核", items[0].CurrentBlocker.Title)
	require.Equal(t, graph.Tasks[0].ID, *items[0].CurrentBlocker.ResourceID)
}

func TestListWorkflowInstancesIgnoresMalformedDemandMetadata(t *testing.T) {
	repo, tenantID := newProjectRepositoryTestStore(t)
	ctx := context.Background()
	projectID := createProjectFixture(t, repo, tenantID)
	actorID := uuid.New()
	demandIDs := []uuid.UUID{
		createDemandFixtureWithSourceRefs(t, repo, tenantID, projectID, map[string]any{
			"sla_due_at": "not-a-timestamp",
		}),
		createDemandFixtureWithSourceRefs(t, repo, tenantID, projectID, map[string]any{
			"sla_due_at": "2026-02-31",
		}),
		createDemandFixtureWithSourceRefs(t, repo, tenantID, projectID, map[string]any{
			"sla_due_at": "2026-99-99",
		}),
		createDemandFixtureWithSourceRefs(t, repo, tenantID, projectID, map[string]any{
			"sla_due_at": "2026-06-16T10:00:00+08:00",
		}),
	}
	farFutureDemandID := createDemandFixtureWithSourceRefs(t, repo, tenantID, projectID, map[string]any{
		"sla_due_at": "9999-12-31",
	})

	_, err := repo.ReplaceProjectMembers(ctx, tenantID, projectID, []ProjectMemberInput{{
		PrincipalType:       PrincipalTypeHumanUser,
		PrincipalID:         actorID,
		ProjectRole:         ProjectRoleObserver,
		DisplayNameSnapshot: "观察者",
	}})
	require.NoError(t, err)
	_, err = repo.AppendProjectEvent(ctx, AppendProjectEventRequest{
		TenantID:  tenantID,
		ProjectID: projectID,
		EventType: ProjectEventDecisionRequested,
		ActorType: "project_coordinator",
		ActorID:   "project-coordinator:" + projectID.String(),
		Summary:   "坏 demand id 元数据",
		Payload: map[string]any{
			"demand_id": "not-a-uuid",
		},
	})
	require.NoError(t, err)

	items, err := repo.ListWorkflowInstances(ctx, ListWorkflowInstancesRequest{
		TenantID:    tenantID,
		ActorUserID: actorID,
		Limit:       20,
	})
	require.NoError(t, err)
	require.Len(t, items, len(demandIDs)+1)
	itemsByDemandID := make(map[uuid.UUID]WorkflowInstanceSummary, len(items))
	for _, item := range items {
		itemsByDemandID[item.DemandID] = item
	}
	for _, demandID := range demandIDs {
		item, ok := itemsByDemandID[demandID]
		require.True(t, ok, "missing demand %s in workflow instances", demandID)
		require.Nil(t, item.SLA)
	}
	farFutureItem, ok := itemsByDemandID[farFutureDemandID]
	require.True(t, ok, "missing far-future demand %s in workflow instances", farFutureDemandID)
	require.NotNil(t, farFutureItem.SLA)
	require.NotNil(t, farFutureItem.SLA.RemainingSeconds)
	require.Equal(t, int32(2147483647), *farFutureItem.SLA.RemainingSeconds)
}

func TestCompleteProjectTaskWritebackAdvancesDemandLifecycle(t *testing.T) {
	repo, tenantID := newProjectRepositoryTestStore(t)
	writebacks := repo.(ProjectTaskWritebackRepository)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	projectID := createProjectFixture(t, repo, tenantID)
	demandID := createDemandFixture(t, repo, tenantID, projectID)

	tasks := make([]ProjectTask, 0, 2)
	for index := 0; index < 2; index++ {
		employeeID := uuid.New()
		task, err := repo.CreateProjectTask(ctx, CreateProjectTaskRequest{
			TenantID:                  tenantID,
			ProjectID:                 projectID,
			DemandID:                  &demandID,
			Title:                     fmt.Sprintf("需求任务 %02d", index+1),
			Status:                    "assigned",
			AssignedDigitalEmployeeID: &employeeID,
			ExpectedOutputs:           []any{"execution_summary"},
			InputRequirements:         map[string]any{},
			HandoffContract:           map[string]any{},
			PlannerMetadata:           map[string]any{},
		})
		require.NoError(t, err)
		tasks = append(tasks, task)
	}

	completeTask := func(task ProjectTask) {
		employeeID := *task.AssignedDigitalEmployeeID
		_, err := writebacks.CompleteProjectTaskWriteback(ctx, CompleteProjectTaskWritebackRequest{
			Task: task,
			Summary: CreateExecutionSummaryRequest{
				TenantID:              tenantID,
				ProjectID:             projectID,
				ProjectTaskID:         task.ID,
				DigitalEmployeeID:     employeeID,
				Conclusion:            "完成写回成功",
				EvidenceRefs:          []any{task.ID.String()},
				ArtifactRefs:          []any{},
				ConfidenceFactors:     map[string]any{},
				MissingInformation:    []any{},
				RecommendedNextAction: "继续协调",
			},
			Event: AppendProjectEventRequest{
				TenantID:     tenantID,
				ProjectID:    projectID,
				EventType:    ProjectEventTaskCompleted,
				ActorType:    "digital_employee",
				ActorID:      employeeID.String(),
				ResourceType: strPtr("project_task"),
				ResourceID:   strPtr(task.ID.String()),
				Summary:      "项目任务已完成",
				Payload:      map[string]any{"project_task_id": task.ID.String()},
			},
			AllowedCurrentStatuses: []string{"assigned", "running"},
		})
		require.NoError(t, err)
	}

	completeTask(tasks[0])
	demand, err := repo.GetProjectDemand(ctx, tenantID, demandID)
	require.NoError(t, err)
	require.Equal(t, ProjectDemandStatusExecuting, demand.Status, "demand should be executing while a sibling task remains active")

	completeTask(tasks[1])
	demand, err = repo.GetProjectDemand(ctx, tenantID, demandID)
	require.NoError(t, err)
	require.Equal(t, ProjectDemandStatusCompleted, demand.Status, "demand should be completed once all tasks finish")
}

func TestCompleteProjectTaskWritebackSerializesConcurrentProjectEvents(t *testing.T) {
	repo, tenantID := newProjectRepositoryTestStore(t)
	writebacks := repo.(ProjectTaskWritebackRepository)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	projectID := createProjectFixture(t, repo, tenantID)
	const taskCount = 8
	tasks := make([]ProjectTask, 0, taskCount)
	for index := 0; index < taskCount; index++ {
		employeeID := uuid.New()
		task, err := repo.CreateProjectTask(ctx, CreateProjectTaskRequest{
			TenantID:                  tenantID,
			ProjectID:                 projectID,
			Title:                     fmt.Sprintf("并发完成任务 %02d", index+1),
			Status:                    "assigned",
			AssignedDigitalEmployeeID: &employeeID,
			ExpectedOutputs:           []any{"execution_summary"},
			InputRequirements:         map[string]any{},
			HandoffContract:           map[string]any{},
			PlannerMetadata:           map[string]any{},
		})
		require.NoError(t, err)
		tasks = append(tasks, task)
	}

	start := make(chan struct{})
	errs := make(chan error, taskCount)
	var wg sync.WaitGroup
	for _, task := range tasks {
		task := task
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			employeeID := *task.AssignedDigitalEmployeeID
			_, err := writebacks.CompleteProjectTaskWriteback(ctx, CompleteProjectTaskWritebackRequest{
				Task: task,
				Summary: CreateExecutionSummaryRequest{
					TenantID:              tenantID,
					ProjectID:             projectID,
					ProjectTaskID:         task.ID,
					DigitalEmployeeID:     employeeID,
					Conclusion:            "并发完成写回成功",
					EvidenceRefs:          []any{task.ID.String()},
					ArtifactRefs:          []any{},
					ConfidenceFactors:     map[string]any{"source": "concurrent-writeback-test"},
					MissingInformation:    []any{},
					RecommendedNextAction: "继续协调",
				},
				Event: AppendProjectEventRequest{
					TenantID:     tenantID,
					ProjectID:    projectID,
					EventType:    ProjectEventTaskCompleted,
					ActorType:    "digital_employee",
					ActorID:      employeeID.String(),
					ResourceType: strPtr("project_task"),
					ResourceID:   strPtr(task.ID.String()),
					Summary:      "项目任务已完成",
					Payload: map[string]any{
						"project_task_id": task.ID.String(),
					},
				},
				AllowedCurrentStatuses: []string{"assigned", "running"},
			})
			errs <- err
		}()
	}
	close(start)
	wg.Wait()
	close(errs)

	for err := range errs {
		require.NoError(t, err)
	}
	completed := "completed"
	completedTasks, err := repo.ListProjectTasks(ctx, tenantID, projectID, &completed, 50, 0)
	require.NoError(t, err)
	require.Len(t, completedTasks, taskCount)
	summaries, err := repo.ListExecutionSummaries(ctx, tenantID, projectID, 50, 0)
	require.NoError(t, err)
	require.Len(t, summaries, taskCount)
	events, err := repo.ListProjectEvents(ctx, tenantID, projectID, 100, 0)
	require.NoError(t, err)
	requireEventCount(t, events, ProjectEventTaskCompleted, taskCount)
}

func TestProjectTaskGraphReadReturnsGraphScopedSidecarsAfterUnrelatedRows(t *testing.T) {
	repo, tenantID := newProjectRepositoryTestStore(t)
	pgRepo := repo.(*PgRepository)
	ctx := context.Background()
	projectID := createProjectFixture(t, repo, tenantID)
	demandID := createDemandFixture(t, repo, tenantID, projectID)
	jobID := createCoordinationJobFixture(t, repo, tenantID, projectID)
	routeID := createRouteDecisionFixture(t, repo, tenantID, projectID, jobID, demandID)
	req := createProjectTaskGraphFixtureRequest(tenantID, projectID, demandID, jobID, routeID)
	result, err := repo.CreateProjectTaskGraph(ctx, req)
	require.NoError(t, err)
	graphTaskID := result.Tasks[0].ID
	graphEmployeeID := req.Tasks[0].AssignedDigitalEmployeeID

	graphSummary, err := repo.CreateExecutionSummary(ctx, CreateExecutionSummaryRequest{
		TenantID:              tenantID,
		ProjectID:             projectID,
		ProjectTaskID:         graphTaskID,
		DigitalEmployeeID:     graphEmployeeID,
		Conclusion:            "graph summary",
		EvidenceRefs:          []any{"graph-evidence"},
		ArtifactRefs:          []any{},
		ConfidenceFactors:     map[string]any{},
		MissingInformation:    []any{},
		RecommendedNextAction: "continue",
	})
	require.NoError(t, err)
	graphDecision, err := repo.CreateDecisionRequest(ctx, CreateDecisionRequestRequest{
		TenantID:          tenantID,
		ProjectID:         projectID,
		ApprovalRequestID: uuid.New(),
		CoordinationJobID: &jobID,
		ProjectTaskID:     &graphTaskID,
		TargetUserID:      uuid.New(),
		DecisionType:      "task_failure_recovery",
		TitleSnapshot:     "graph decision",
		StatusSnapshot:    "pending",
	})
	require.NoError(t, err)
	graphEvent, err := repo.AppendProjectEvent(ctx, AppendProjectEventRequest{
		TenantID:  tenantID,
		ProjectID: projectID,
		EventType: ProjectEventTaskDispatched,
		ActorType: "workflow",
		ActorID:   jobID.String(),
		Summary:   "graph event",
		Payload: map[string]any{
			"coordination_job_id": jobID.String(),
			"project_task_id":     graphTaskID.String(),
		},
	})
	require.NoError(t, err)
	runtimeTask, err := pgRepo.q.CreateTask(ctx, queries.CreateTaskParams{
		TenantID:     nullUUID(&tenantID),
		Title:        "runtime graph task",
		Status:       "pending",
		Priority:     1,
		ProviderType: "codex",
		Params:       []byte(`{}`),
	})
	require.NoError(t, err)
	run, err := pgRepo.q.CreateTaskRun(ctx, queries.CreateTaskRunParams{
		TenantID: nullUUID(&tenantID),
		TaskID:   runtimeTask.ID,
		NodeID:   "graph-node",
		Status:   "running",
	})
	require.NoError(t, err)
	_, err = repo.BindProjectTaskRun(ctx, BindProjectTaskRunRequest{
		TenantID:             tenantID,
		ProjectTaskID:        graphTaskID,
		RuntimeTaskID:        runtimeTask.ID,
		DigitalEmployeeRunID: run.ID,
		CurrentStatuses:      []string{"planned"},
	})
	require.NoError(t, err)

	unrelatedTask, err := repo.CreateProjectTask(ctx, CreateProjectTaskRequest{
		TenantID:                  tenantID,
		ProjectID:                 projectID,
		Title:                     "unrelated",
		Status:                    "completed",
		AssignedDigitalEmployeeID: &graphEmployeeID,
	})
	require.NoError(t, err)
	for i := 0; i < 501; i++ {
		_, err = repo.CreateExecutionSummary(ctx, CreateExecutionSummaryRequest{
			TenantID:              tenantID,
			ProjectID:             projectID,
			ProjectTaskID:         unrelatedTask.ID,
			DigitalEmployeeID:     graphEmployeeID,
			Conclusion:            fmt.Sprintf("unrelated summary %03d", i),
			EvidenceRefs:          []any{},
			ArtifactRefs:          []any{},
			ConfidenceFactors:     map[string]any{},
			MissingInformation:    []any{},
			RecommendedNextAction: "none",
		})
		require.NoError(t, err)
	}
	for i := 0; i < 201; i++ {
		_, err = repo.CreateDecisionRequest(ctx, CreateDecisionRequestRequest{
			TenantID:          tenantID,
			ProjectID:         projectID,
			ApprovalRequestID: uuid.New(),
			CoordinationJobID: &jobID,
			ProjectTaskID:     &graphTaskID,
			TargetUserID:      uuid.New(),
			DecisionType:      "task_failure_recovery",
			TitleSnapshot:     fmt.Sprintf("related decision %03d", i),
			StatusSnapshot:    "pending",
		})
		require.NoError(t, err)
	}
	for i := 0; i < 101; i++ {
		_, err = repo.AppendProjectEvent(ctx, AppendProjectEventRequest{
			TenantID:  tenantID,
			ProjectID: projectID,
			EventType: ProjectEventTaskDispatched,
			ActorType: "workflow",
			ActorID:   uuid.New().String(),
			Summary:   fmt.Sprintf("unrelated event %03d", i),
			Payload:   map[string]any{"project_task_id": uuid.New().String()},
		})
		require.NoError(t, err)
	}

	graph, err := repo.GetProjectTaskGraph(ctx, GetProjectTaskGraphRequest{
		TenantID: tenantID, ProjectID: projectID, CoordinationJobID: &jobID,
	})
	require.NoError(t, err)
	require.Contains(t, executionSummaryIDs(graph.ExecutionSummaries), graphSummary.ID)
	require.Contains(t, decisionRequestIDs(graph.DecisionRequests), graphDecision.ID)
	require.Contains(t, projectEventIDs(graph.RecentEvents), graphEvent.ID)
	require.Len(t, graph.DecisionRequests, 202)
	require.Len(t, graph.Runs, 1)
	require.Equal(t, run.ID, *graph.Runs[0].DigitalEmployeeRunID)
	require.Equal(t, runtimeTask.ID, *graph.Runs[0].RuntimeTaskID)
	require.NotEmpty(t, graph.StageSummaries)
	require.Equal(t, int32(2), graph.StageSummaries[0].TotalNodes+graph.StageSummaries[1].TotalNodes)
	nodesByTaskID := make(map[uuid.UUID]ProjectTaskGraphNode, len(graph.Nodes))
	for _, node := range graph.Nodes {
		nodesByTaskID[node.Task.ID] = node
	}
	graphNode := nodesByTaskID[graphTaskID]
	require.Equal(t, "等待人工决策", graphNode.StatusReason)
	require.NotNil(t, graphNode.UpdatedAt)
	require.NotNil(t, graphNode.CurrentBlocker)
	require.Equal(t, "decision_request", graphNode.CurrentBlocker.Type)
	require.Equal(t, graphDecision.ID, *graphNode.CurrentBlocker.ResourceID)
	blockedNode := nodesByTaskID[result.Tasks[1].ID]
	require.Equal(t, "任务受阻", blockedNode.StatusReason)
	require.NotNil(t, blockedNode.CurrentBlocker)
	require.Equal(t, "project_task", blockedNode.CurrentBlocker.Type)
}

func TestCreateCoordinationJobIdempotentReplayReturnsExisting(t *testing.T) {
	repo, tenantID := newProjectRepositoryTestStore(t)
	projectID := createProjectFixture(t, repo, tenantID)
	triggerEventID := uuid.New()
	req := CreateCoordinationJobRequest{
		TenantID:         tenantID,
		ProjectID:        projectID,
		WorkflowID:       "project-coordinator:" + projectID.String(),
		TriggerEventID:   &triggerEventID,
		JobType:          "demand_route",
		Status:           "running",
		InputSnapshotRef: map[string]any{"trigger_event_id": triggerEventID.String()},
	}

	first, err := repo.CreateCoordinationJob(context.Background(), req)
	require.NoError(t, err)
	second, err := repo.CreateCoordinationJob(context.Background(), req)
	require.NoError(t, err)

	require.Equal(t, first.ID, second.ID)
	require.Equal(t, first.WorkflowID, second.WorkflowID)
}

func TestCreateRouteDecisionIdempotentReplayReturnsExisting(t *testing.T) {
	repo, tenantID := newProjectRepositoryTestStore(t)
	projectID := createProjectFixture(t, repo, tenantID)
	demandID := createDemandFixture(t, repo, tenantID, projectID)
	jobID := createCoordinationJobFixture(t, repo, tenantID, projectID)
	eventID := uuid.New()
	req := CreateRouteDecisionRequest{
		TenantID:                    tenantID,
		ProjectID:                   projectID,
		CoordinationJobID:           jobID,
		DemandID:                    &demandID,
		CandidateDigitalEmployeeIDs: []uuid.UUID{uuid.New()},
		SelectedDigitalEmployeeIDs:  []uuid.UUID{uuid.New()},
		Reason:                      "idempotent route",
		InputRequirements:           map[string]any{"mode": "test"},
		ExpectedOutputs:             []any{"execution_summary"},
		BudgetEstimate:              map[string]any{"mode": "test"},
		RequiresHumanReview:         true,
		CreatedEventID:              &eventID,
	}

	first, err := repo.CreateRouteDecision(context.Background(), req)
	require.NoError(t, err)
	second, err := repo.CreateRouteDecision(context.Background(), req)
	require.NoError(t, err)

	require.Equal(t, first.ID, second.ID)
	require.Equal(t, first.CoordinationJobID, second.CoordinationJobID)
}

func TestCreateProjectTaskGraphIdempotentReplayReturnsExistingGraph(t *testing.T) {
	repo, tenantID := newProjectRepositoryTestStore(t)
	projectID := createProjectFixture(t, repo, tenantID)
	demandID := createDemandFixture(t, repo, tenantID, projectID)
	jobID := createCoordinationJobFixture(t, repo, tenantID, projectID)
	routeID := createRouteDecisionFixture(t, repo, tenantID, projectID, jobID, demandID)
	req := createProjectTaskGraphFixtureRequest(tenantID, projectID, demandID, jobID, routeID)

	first, err := repo.CreateProjectTaskGraph(context.Background(), req)
	require.NoError(t, err)

	second, err := repo.CreateProjectTaskGraph(context.Background(), req)
	require.NoError(t, err)

	require.Equal(t, first.GraphEventID, second.GraphEventID)
	require.Equal(t, first.Tasks, second.Tasks)
	require.Equal(t, first.Dependencies, second.Dependencies)

	tasks, err := repo.ListProjectTasksByCoordinationJob(context.Background(), tenantID, projectID, jobID)
	require.NoError(t, err)
	require.Len(t, tasks, 2)
	events, err := repo.ListProjectEvents(context.Background(), tenantID, projectID, 20, 0)
	require.NoError(t, err)
	requireEventCount(t, events, ProjectEventTaskCreated, 2)
	requireEventCount(t, events, ProjectEventTaskGraphPlanned, 1)
}

func TestDecomposeAcceptedPlanRevisionIsIdempotent(t *testing.T) {
	repo, tenantID := newProjectRepositoryTestStore(t)
	projectID := createProjectFixture(t, repo, tenantID)
	demandID := createDemandFixture(t, repo, tenantID, projectID)
	jobID := createCoordinationJobFixture(t, repo, tenantID, projectID)
	routeID := createRouteDecisionFixture(t, repo, tenantID, projectID, jobID, demandID)
	req := createDecomposeAcceptedPlanRevisionFixtureRequest(t, repo, tenantID, projectID, demandID, jobID, routeID)

	first, err := repo.DecomposeAcceptedPlanRevision(context.Background(), req)
	require.NoError(t, err)
	require.False(t, first.Replayed)
	require.Len(t, first.Tasks, 2)
	require.Len(t, first.Dependencies, 1)
	for _, task := range first.Tasks {
		require.NotNil(t, task.AcceptedPlanRevisionID)
		require.Equal(t, req.AcceptedPlanRevisionID, *task.AcceptedPlanRevisionID)
		require.NotNil(t, task.DecompositionClaimKey)
		require.Equal(t, req.DecompositionClaimKey, *task.DecompositionClaimKey)
		require.Equal(t, req.AcceptedPlanRevisionID.String(), task.PlannerMetadata["accepted_plan_revision_id"])
	}

	second, err := repo.DecomposeAcceptedPlanRevision(context.Background(), req)
	require.NoError(t, err)
	require.True(t, second.Replayed)
	require.Equal(t, first.Tasks, second.Tasks)
	require.Equal(t, first.Dependencies, second.Dependencies)

	events, err := repo.ListProjectEvents(context.Background(), tenantID, projectID, 20, 0)
	require.NoError(t, err)
	requireEventCount(t, events, ProjectEventTaskCreated, 2)
	requireEventCount(t, events, ProjectEventTaskGraphPlanned, 1)
}

func TestDecomposeAcceptedPlanRevisionReplaysAcrossCoordinationJobs(t *testing.T) {
	repo, tenantID := newProjectRepositoryTestStore(t)
	projectID := createProjectFixture(t, repo, tenantID)
	demandID := createDemandFixture(t, repo, tenantID, projectID)
	firstJobID := createCoordinationJobFixture(t, repo, tenantID, projectID)
	firstRouteID := createRouteDecisionFixture(t, repo, tenantID, projectID, firstJobID, demandID)
	secondJobID := createCoordinationJobFixture(t, repo, tenantID, projectID)
	secondRouteID := createRouteDecisionFixture(t, repo, tenantID, projectID, secondJobID, demandID)
	req := createDecomposeAcceptedPlanRevisionFixtureRequest(t, repo, tenantID, projectID, demandID, firstJobID, firstRouteID)

	first, err := repo.DecomposeAcceptedPlanRevision(context.Background(), req)
	require.NoError(t, err)
	require.False(t, first.Replayed)

	replayReq := req
	replayReq.CoordinationJobID = secondJobID
	replayReq.RouteDecisionID = secondRouteID
	second, err := repo.DecomposeAcceptedPlanRevision(context.Background(), replayReq)
	require.NoError(t, err)
	require.True(t, second.Replayed)
	require.Equal(t, first.Tasks, second.Tasks)
	require.Equal(t, first.Dependencies, second.Dependencies)

	for _, task := range second.Tasks {
		require.NotNil(t, task.CoordinationJobID)
		require.Equal(t, firstJobID, *task.CoordinationJobID)
		require.NotNil(t, task.RouteDecisionID)
		require.Equal(t, firstRouteID, *task.RouteDecisionID)
	}
	events, err := repo.ListProjectEvents(context.Background(), tenantID, projectID, 20, 0)
	require.NoError(t, err)
	requireEventCount(t, events, ProjectEventTaskCreated, 2)
	requireEventCount(t, events, ProjectEventTaskGraphPlanned, 1)
}

func TestDecomposeAcceptedPlanRevisionRejectsChangedPayload(t *testing.T) {
	repo, tenantID := newProjectRepositoryTestStore(t)
	projectID := createProjectFixture(t, repo, tenantID)
	demandID := createDemandFixture(t, repo, tenantID, projectID)
	jobID := createCoordinationJobFixture(t, repo, tenantID, projectID)
	routeID := createRouteDecisionFixture(t, repo, tenantID, projectID, jobID, demandID)
	req := createDecomposeAcceptedPlanRevisionFixtureRequest(t, repo, tenantID, projectID, demandID, jobID, routeID)
	_, err := repo.DecomposeAcceptedPlanRevision(context.Background(), req)
	require.NoError(t, err)

	changed := req
	changed.PlanFingerprint = "different-fingerprint"
	changed.Tasks = append([]ProjectTaskGraphCreateTask(nil), req.Tasks...)
	changed.Tasks[0].Title = "变更后的分析"
	changed.Tasks[0].InputRequirements = map[string]any{"scope": "changed"}

	_, err = repo.DecomposeAcceptedPlanRevision(context.Background(), changed)
	require.ErrorIs(t, err, ErrProjectConflict)

	events, listEventsErr := repo.ListProjectEvents(context.Background(), tenantID, projectID, 20, 0)
	require.NoError(t, listEventsErr)
	requireEventCount(t, events, ProjectEventTaskCreated, 2)
	requireEventCount(t, events, ProjectEventTaskGraphPlanned, 1)
}

func TestProjectPlanRevisionCreateAcceptAndList(t *testing.T) {
	repo, tenantID := newProjectRepositoryTestStore(t)
	projectID := createProjectFixture(t, repo, tenantID)
	demandID := createDemandFixture(t, repo, tenantID, projectID)
	jobID := createCoordinationJobFixture(t, repo, tenantID, projectID)
	routeID := createRouteDecisionFixture(t, repo, tenantID, projectID, jobID, demandID)
	reviewReason := "high risk"

	created, err := repo.CreatePlanRevision(context.Background(), CreatePlanRevisionRequest{
		TenantID:          tenantID,
		ProjectID:         projectID,
		DemandID:          demandID,
		CoordinationJobID: &jobID,
		RouteDecisionID:   &routeID,
		Status:            PlanRevisionStatusPendingReview,
		Payload:           map[string]any{"summary": "计划"},
		PlanFingerprint:   "fingerprint-1",
		ReviewRequired:    true,
		ReviewReason:      &reviewReason,
	})
	require.NoError(t, err)
	require.Equal(t, int32(1), created.RevisionNumber)
	require.Equal(t, PlanRevisionStatusPendingReview, created.Status)

	acceptedBy := uuid.New()
	accepted, err := repo.AcceptPlanRevision(context.Background(), AcceptPlanRevisionRequest{
		TenantID:   tenantID,
		ProjectID:  projectID,
		RevisionID: created.ID,
		AcceptedBy: &acceptedBy,
	})
	require.NoError(t, err)
	require.Equal(t, PlanRevisionStatusAccepted, accepted.Status)
	require.NotNil(t, accepted.AcceptedAt)

	revisions, err := repo.ListPlanRevisions(context.Background(), ListPlanRevisionsRequest{
		TenantID:  tenantID,
		ProjectID: projectID,
		DemandID:  &demandID,
		Limit:     20,
		Offset:    0,
	})
	require.NoError(t, err)
	require.Len(t, revisions, 1)
	require.Equal(t, created.ID, revisions[0].ID)
}

func TestProjectPlanRevisionSupersedesOpenRevisions(t *testing.T) {
	repo, tenantID := newProjectRepositoryTestStore(t)
	projectID := createProjectFixture(t, repo, tenantID)
	demandID := createDemandFixture(t, repo, tenantID, projectID)

	first, err := repo.CreatePlanRevision(context.Background(), CreatePlanRevisionRequest{
		TenantID:        tenantID,
		ProjectID:       projectID,
		DemandID:        demandID,
		Status:          PlanRevisionStatusPendingReview,
		Payload:         map[string]any{"summary": "旧计划"},
		PlanFingerprint: "fingerprint-old",
		ReviewRequired:  true,
	})
	require.NoError(t, err)
	reason := "human requested changes"

	second, err := repo.CreatePlanRevision(context.Background(), CreatePlanRevisionRequest{
		TenantID:               tenantID,
		ProjectID:              projectID,
		DemandID:               demandID,
		Status:                 PlanRevisionStatusPendingReview,
		Payload:                map[string]any{"summary": "新计划"},
		PlanFingerprint:        "fingerprint-new",
		ReviewRequired:         true,
		SupersedeOpenRevisions: true,
		SupersedeReason:        &reason,
	})
	require.NoError(t, err)
	require.Equal(t, int32(2), second.RevisionNumber)

	stale, err := repo.GetPlanRevision(context.Background(), tenantID, projectID, first.ID)
	require.NoError(t, err)
	require.Equal(t, PlanRevisionStatusSuperseded, stale.Status)
	require.NotNil(t, stale.SupersededByRevisionID)
	require.Equal(t, second.ID, *stale.SupersededByRevisionID)
}

func TestDecomposeAcceptedPlanRevisionRequiresAcceptedRevisionAndCompletesClaim(t *testing.T) {
	repo, tenantID := newProjectRepositoryTestStore(t)
	projectID := createProjectFixture(t, repo, tenantID)
	demandID := createDemandFixture(t, repo, tenantID, projectID)
	jobID := createCoordinationJobFixture(t, repo, tenantID, projectID)
	routeID := createRouteDecisionFixture(t, repo, tenantID, projectID, jobID, demandID)
	employeeID := uuid.New()

	revision, err := repo.CreatePlanRevision(context.Background(), CreatePlanRevisionRequest{
		TenantID:        tenantID,
		ProjectID:       projectID,
		DemandID:        demandID,
		Status:          PlanRevisionStatusAccepted,
		Payload:         map[string]any{"summary": "accepted"},
		PlanFingerprint: "fingerprint-accepted",
	})
	require.NoError(t, err)

	req := DecomposeAcceptedPlanRevisionRequest{
		TenantID:               tenantID,
		ProjectID:              projectID,
		DemandID:               demandID,
		CoordinationJobID:      jobID,
		RouteDecisionID:        routeID,
		AcceptedPlanRevisionID: revision.ID,
		PlanFingerprint:        revision.PlanFingerprint,
		DecompositionClaimKey:  revision.ID.String(),
		Tasks: []ProjectTaskGraphCreateTask{
			{
				Key:                       "inspect",
				Title:                     "检查",
				Summary:                   "检查输入",
				Status:                    ProjectTaskStatusPlanned,
				AssignedDigitalEmployeeID: employeeID,
				ExpectedOutputs:           []any{"结论"},
				InputRequirements:         map[string]any{"context": "demand"},
				HandoffContract:           map[string]any{"acceptance_criteria": []any{"结论可复核"}},
				PlannerMetadata:           map[string]any{"accepted_plan_revision_id": revision.ID.String()},
			},
		},
	}

	first, err := repo.DecomposeAcceptedPlanRevision(context.Background(), req)
	require.NoError(t, err)
	require.False(t, first.Replayed)
	require.Len(t, first.Tasks, 1)

	second, err := repo.DecomposeAcceptedPlanRevision(context.Background(), req)
	require.NoError(t, err)
	require.True(t, second.Replayed)
	require.Equal(t, first.Tasks[0].ID, second.Tasks[0].ID)

	stored, err := repo.GetPlanRevision(context.Background(), tenantID, projectID, revision.ID)
	require.NoError(t, err)
	require.Equal(t, PlanRevisionStatusDecomposed, stored.Status)
	require.Equal(t, []uuid.UUID{first.Tasks[0].ID}, stored.CreatedTaskIDs)
	require.NotNil(t, stored.DecompositionClaimID)
}

func TestCreateProjectTaskGraphReplayFindsEventsAfterProjectEventChurn(t *testing.T) {
	repo, tenantID := newProjectRepositoryTestStore(t)
	projectID := createProjectFixture(t, repo, tenantID)
	demandID := createDemandFixture(t, repo, tenantID, projectID)
	jobID := createCoordinationJobFixture(t, repo, tenantID, projectID)
	routeID := createRouteDecisionFixture(t, repo, tenantID, projectID, jobID, demandID)
	req := createProjectTaskGraphFixtureRequest(tenantID, projectID, demandID, jobID, routeID)

	first, err := repo.CreateProjectTaskGraph(context.Background(), req)
	require.NoError(t, err)
	for i := 0; i < 1001; i++ {
		_, err = repo.AppendProjectEvent(context.Background(), AppendProjectEventRequest{
			TenantID:  tenantID,
			ProjectID: projectID,
			EventType: ProjectEventTaskDispatched,
			ActorType: "workflow",
			ActorID:   uuid.New().String(),
			Summary:   fmt.Sprintf("unrelated event %04d", i),
			Payload:   map[string]any{"project_task_id": uuid.New().String()},
		})
		require.NoError(t, err)
	}

	second, err := repo.CreateProjectTaskGraph(context.Background(), req)
	require.NoError(t, err)

	require.Equal(t, first.GraphEventID, second.GraphEventID)
	require.Equal(t, first.Tasks, second.Tasks)
	require.Equal(t, first.Dependencies, second.Dependencies)
}

func TestCreateProjectTaskGraphReplayFindsTaskCreatedEventsByPayloadTaskID(t *testing.T) {
	repo, tenantID := newProjectRepositoryTestStore(t)
	pgRepo := repo.(*PgRepository)
	pool, ok := pgRepo.db.(*pgxpool.Pool)
	require.True(t, ok, "project repository test store should use pgxpool")
	projectID := createProjectFixture(t, repo, tenantID)
	demandID := createDemandFixture(t, repo, tenantID, projectID)
	jobID := createCoordinationJobFixture(t, repo, tenantID, projectID)
	routeID := createRouteDecisionFixture(t, repo, tenantID, projectID, jobID, demandID)
	req := createProjectTaskGraphFixtureRequest(tenantID, projectID, demandID, jobID, routeID)

	first, err := repo.CreateProjectTaskGraph(context.Background(), req)
	require.NoError(t, err)
	_, err = pool.Exec(
		context.Background(),
		`DELETE FROM project_events WHERE tenant_id = $1 AND project_id = $2 AND event_type = $3`,
		tenantID,
		projectID,
		string(ProjectEventTaskCreated),
	)
	require.NoError(t, err)
	replacementEventIDs := map[uuid.UUID]uuid.UUID{}
	for _, task := range first.Tasks {
		event, err := repo.AppendProjectEvent(context.Background(), AppendProjectEventRequest{
			TenantID:     tenantID,
			ProjectID:    projectID,
			EventType:    ProjectEventTaskCreated,
			ActorType:    "project_coordinator",
			ActorID:      uuid.New().String(),
			ResourceType: strPtr("project_task"),
			ResourceID:   strPtr(uuid.New().String()),
			Summary:      "payload-only task created event",
			Payload: map[string]any{
				"project_task_id":     task.ID.String(),
				"coordination_job_id": jobID.String(),
			},
		})
		require.NoError(t, err)
		replacementEventIDs[task.ID] = event.ID
	}

	second, err := repo.CreateProjectTaskGraph(context.Background(), req)
	require.NoError(t, err)
	require.Equal(t, first.GraphEventID, second.GraphEventID)
	require.Equal(t, first.Dependencies, second.Dependencies)
	for _, task := range second.Tasks {
		require.Equal(t, replacementEventIDs[task.ID], task.CreatedEventID)
	}
}

func TestGraphTaskCreatedEventTaskIDFallsBackToPayloadTaskID(t *testing.T) {
	taskID := uuid.New()
	unrelatedActorID := uuid.New().String()
	event := ProjectEvent{
		EventType: ProjectEventTaskCreated,
		ActorID:   unrelatedActorID,
		Payload:   map[string]any{"project_task_id": taskID.String()},
	}
	neededTasks := map[string]uuid.UUID{taskID.String(): taskID}

	matchedTaskID, ok := graphTaskCreatedEventTaskID(event, neededTasks)

	require.True(t, ok)
	require.Equal(t, taskID, matchedTaskID)
}

func TestCreateProjectTaskGraphReplayWithChangedPayloadConflicts(t *testing.T) {
	repo, tenantID := newProjectRepositoryTestStore(t)
	projectID := createProjectFixture(t, repo, tenantID)
	demandID := createDemandFixture(t, repo, tenantID, projectID)
	jobID := createCoordinationJobFixture(t, repo, tenantID, projectID)
	routeID := createRouteDecisionFixture(t, repo, tenantID, projectID, jobID, demandID)
	req := createProjectTaskGraphFixtureRequest(tenantID, projectID, demandID, jobID, routeID)

	_, err := repo.CreateProjectTaskGraph(context.Background(), req)
	require.NoError(t, err)

	changed := req
	changed.Tasks = append([]ProjectTaskGraphCreateTask(nil), req.Tasks...)
	changed.Tasks[0].Title = "变更后的分析"
	changed.Tasks[0].InputRequirements = map[string]any{"scope": "changed"}

	_, err = repo.CreateProjectTaskGraph(context.Background(), changed)

	require.ErrorIs(t, err, ErrProjectConflict)
	tasks, listErr := repo.ListProjectTasksByCoordinationJob(context.Background(), tenantID, projectID, jobID)
	require.NoError(t, listErr)
	require.Len(t, tasks, 2)
	events, listEventsErr := repo.ListProjectEvents(context.Background(), tenantID, projectID, 20, 0)
	require.NoError(t, listEventsErr)
	requireEventCount(t, events, ProjectEventTaskCreated, 2)
	requireEventCount(t, events, ProjectEventTaskGraphPlanned, 1)
}

func TestCreateProjectTaskGraphPartialGraphConflict(t *testing.T) {
	repo, tenantID := newProjectRepositoryTestStore(t)
	projectID := createProjectFixture(t, repo, tenantID)
	demandID := createDemandFixture(t, repo, tenantID, projectID)
	jobID := createCoordinationJobFixture(t, repo, tenantID, projectID)
	routeID := createRouteDecisionFixture(t, repo, tenantID, projectID, jobID, demandID)
	req := createProjectTaskGraphFixtureRequest(tenantID, projectID, demandID, jobID, routeID)
	employeeID := req.Tasks[0].AssignedDigitalEmployeeID
	_, err := repo.CreateProjectTask(context.Background(), CreateProjectTaskRequest{
		TenantID:                  tenantID,
		ProjectID:                 projectID,
		DemandID:                  &demandID,
		CoordinationJobID:         &jobID,
		RouteDecisionID:           &routeID,
		PlannedTaskKey:            strPtr("t1"),
		Title:                     "分析",
		Summary:                   "分析",
		Status:                    "planned",
		AssignedDigitalEmployeeID: &employeeID,
		ExpectedOutputs:           []any{"execution_summary"},
		InputRequirements:         map[string]any{},
		HandoffContract:           map[string]any{},
		PlannerMetadata:           map[string]any{"planner": "test"},
	})
	require.NoError(t, err)

	_, err = repo.CreateProjectTaskGraph(context.Background(), req)

	require.ErrorIs(t, err, ErrProjectConflict)
	tasks, listErr := repo.ListProjectTasksByCoordinationJob(context.Background(), tenantID, projectID, jobID)
	require.NoError(t, listErr)
	require.Len(t, tasks, 1)
}

func TestGraphTaskPayloadMatchesRequest(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	demandID := uuid.New()
	jobID := uuid.New()
	routeID := uuid.New()
	req := createProjectTaskGraphFixtureRequest(tenantID, projectID, demandID, jobID, routeID)
	planned := req.Tasks[0]
	existing := projectTaskFromGraphRequestForTest(req, planned)

	require.True(t, graphTaskPayloadMatchesRequest(req, planned, existing))

	for _, tc := range []struct {
		name   string
		mutate func(*CreateProjectTaskGraphRequest, *ProjectTaskGraphCreateTask)
	}{
		{name: "demand id", mutate: func(req *CreateProjectTaskGraphRequest, planned *ProjectTaskGraphCreateTask) {
			req.DemandID = uuid.New()
		}},
		{name: "coordination job id", mutate: func(req *CreateProjectTaskGraphRequest, planned *ProjectTaskGraphCreateTask) {
			req.CoordinationJobID = uuid.New()
		}},
		{name: "route decision id", mutate: func(req *CreateProjectTaskGraphRequest, planned *ProjectTaskGraphCreateTask) {
			req.RouteDecisionID = uuid.New()
		}},
		{name: "title", mutate: func(req *CreateProjectTaskGraphRequest, planned *ProjectTaskGraphCreateTask) {
			planned.Title = "changed"
		}},
		{name: "summary", mutate: func(req *CreateProjectTaskGraphRequest, planned *ProjectTaskGraphCreateTask) {
			planned.Summary = "changed"
		}},
		{name: "status", mutate: func(req *CreateProjectTaskGraphRequest, planned *ProjectTaskGraphCreateTask) {
			planned.Status = "blocked"
		}},
		{name: "assignee", mutate: func(req *CreateProjectTaskGraphRequest, planned *ProjectTaskGraphCreateTask) {
			planned.AssignedDigitalEmployeeID = uuid.New()
		}},
		{name: "task kind", mutate: func(req *CreateProjectTaskGraphRequest, planned *ProjectTaskGraphCreateTask) {
			planned.TaskKind = "changed"
		}},
		{name: "stage index", mutate: func(req *CreateProjectTaskGraphRequest, planned *ProjectTaskGraphCreateTask) {
			changed := int32(99)
			planned.StageIndex = &changed
		}},
		{name: "risk level", mutate: func(req *CreateProjectTaskGraphRequest, planned *ProjectTaskGraphCreateTask) {
			planned.RiskLevel = "high"
		}},
		{name: "approval", mutate: func(req *CreateProjectTaskGraphRequest, planned *ProjectTaskGraphCreateTask) {
			planned.RequiresHumanApproval = !planned.RequiresHumanApproval
		}},
		{name: "expected outputs", mutate: func(req *CreateProjectTaskGraphRequest, planned *ProjectTaskGraphCreateTask) {
			planned.ExpectedOutputs = []any{"changed"}
		}},
		{name: "input requirements", mutate: func(req *CreateProjectTaskGraphRequest, planned *ProjectTaskGraphCreateTask) {
			planned.InputRequirements = map[string]any{"scope": "changed"}
		}},
		{name: "handoff contract", mutate: func(req *CreateProjectTaskGraphRequest, planned *ProjectTaskGraphCreateTask) {
			planned.HandoffContract = map[string]any{"format": "changed"}
		}},
		{name: "planner metadata", mutate: func(req *CreateProjectTaskGraphRequest, planned *ProjectTaskGraphCreateTask) {
			planned.PlannerMetadata = map[string]any{"planner": "changed"}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			changedReq := req
			changedPlanned := planned
			tc.mutate(&changedReq, &changedPlanned)

			require.False(t, graphTaskPayloadMatchesRequest(changedReq, changedPlanned, existing))
		})
	}
}

func TestFilterEventsForTaskGraphIncludesCoordinationJobActor(t *testing.T) {
	jobID := uuid.New()
	taskID := uuid.New()
	decisionID := uuid.New()
	jobActorEventID := uuid.New()
	payloadEventID := uuid.New()
	events := []ProjectEvent{
		{ID: uuid.New(), ActorID: uuid.New().String(), Payload: map[string]any{}},
		{ID: jobActorEventID, ActorID: jobID.String(), Payload: map[string]any{}},
		{ID: payloadEventID, ActorID: uuid.New().String(), Payload: map[string]any{"coordination_job_id": jobID.String()}},
	}

	filtered := filterEventsForTaskGraph(events, &jobID, []uuid.UUID{taskID}, []uuid.UUID{decisionID})

	require.Equal(t, []uuid.UUID{jobActorEventID, payloadEventID}, projectEventIDs(filtered))
}

func TestProjectTaskGraphRunsFromRowsMapsBoundRuns(t *testing.T) {
	tenantID := uuid.New()
	projectTaskID := uuid.New()
	projectTaskWithRuntimeID := uuid.New()
	runID := uuid.New()
	runWithRuntimeID := uuid.New()
	runtimeTaskID := uuid.New()
	runtimeTaskFromRunID := uuid.New()
	runtimeNodeID := uuid.New()
	tasks := []ProjectTask{
		{ID: projectTaskID, TenantID: tenantID, DigitalEmployeeRunID: &runID},
		{ID: projectTaskWithRuntimeID, TenantID: tenantID, DigitalEmployeeRunID: &runWithRuntimeID, RuntimeTaskID: &runtimeTaskID},
	}
	rows := []queries.TaskRun{
		{
			ID:            runWithRuntimeID,
			TenantID:      tenantID,
			TaskID:        uuid.New(),
			NodeID:        "explicit-runtime-task",
			RuntimeNodeID: uuid.NullUUID{UUID: runtimeNodeID, Valid: true},
			Status:        "running",
			ProviderType:  pgtype.Text{String: "codex", Valid: true},
		},
		{
			ID:           runID,
			TenantID:     tenantID,
			TaskID:       runtimeTaskFromRunID,
			NodeID:       "runtime-task-from-run",
			Status:       "completed",
			ProviderType: pgtype.Text{String: "claude-code", Valid: true},
		},
	}

	runs, err := projectTaskGraphRunsFromRows(tasks, rows)

	require.NoError(t, err)
	require.Len(t, runs, 2)
	require.Equal(t, projectTaskID, runs[0].ProjectTaskID)
	require.Equal(t, runID, *runs[0].DigitalEmployeeRunID)
	require.Equal(t, runtimeTaskFromRunID, *runs[0].RuntimeTaskID)
	require.Equal(t, "runtime-task-from-run", runs[0].RuntimeNodeSummary)
	require.Equal(t, "completed", runs[0].Status)
	require.Equal(t, "claude-code", runs[0].ProviderType)
	require.Equal(t, projectTaskWithRuntimeID, runs[1].ProjectTaskID)
	require.Equal(t, runWithRuntimeID, *runs[1].DigitalEmployeeRunID)
	require.Equal(t, runtimeTaskID, *runs[1].RuntimeTaskID)
	require.Equal(t, runtimeNodeID, *runs[1].RuntimeNodeID)
	require.Equal(t, "codex", runs[1].ProviderType)
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

	retainedArtifactID := uuid.New()
	createdByUserID := uuid.New()
	archive, err := archiveSnapshotFromRecord(queries.ProjectArchiveSnapshot{
		ID:                   uuid.New(),
		TenantID:             tenantID,
		ProjectID:            projectID,
		SnapshotType:         "final_archive",
		Status:               "created",
		ObjectRef:            pgtype.Text{String: "s3://bucket/archive.zip", Valid: true},
		Summary:              pgtype.Text{String: "项目归档快照", Valid: true},
		IncludedCounts:       []byte(`{"evidence":3,"artifact":1,"report":2}`),
		RetainedArtifactIds:  []byte(`["` + retainedArtifactID.String() + `"]`),
		RetentionLockEventID: uuid.NullUUID{UUID: eventID, Valid: true},
		CreatedByUserID:      createdByUserID,
		CreatedAt:            pgtype.Timestamptz{Time: now, Valid: true},
	})
	if err != nil {
		t.Fatalf("map archive snapshot: %v", err)
	}
	if archive.IncludedCounts["evidence"] != float64(3) || len(archive.RetainedArtifactIDs) != 1 || archive.RetainedArtifactIDs[0] != retainedArtifactID {
		t.Fatalf("unexpected archive mapping: %#v", archive)
	}
	if archive.RetentionLockEventID == nil || *archive.RetentionLockEventID != eventID || archive.CreatedByUserID != createdByUserID {
		t.Fatalf("unexpected archive actors/events: %#v", archive)
	}
}

func TestPgRepositoryMapsGovernanceNoRowsToDomainNotFound(t *testing.T) {
	repo := NewPgRepository(queries.New(noRowsDB{}))
	ctx := context.Background()
	tenantID := uuid.New()
	projectID := uuid.New()

	_, err := repo.GetProject(ctx, tenantID, projectID)
	if !errors.Is(err, ErrProjectNotFound) {
		t.Fatalf("expected missing project to map to not found, got %v", err)
	}
	_, err = repo.UpdateEvidenceVerificationStatus(ctx, UpdateEvidenceVerificationStatusRequest{
		TenantID: tenantID, ProjectID: projectID, ID: uuid.New(), VerificationStatus: EvidenceVerificationStatusVerified,
	})
	if !errors.Is(err, ErrProjectNotFound) {
		t.Fatalf("expected missing evidence to map to not found, got %v", err)
	}
	_, err = repo.GetLatestAcceptanceRecord(ctx, tenantID, projectID)
	if !errors.Is(err, ErrProjectNotFound) {
		t.Fatalf("expected missing acceptance to map to not found, got %v", err)
	}
	_, err = repo.GetConfigRevision(ctx, tenantID, projectID, uuid.New())
	if !errors.Is(err, ErrProjectNotFound) {
		t.Fatalf("expected missing config revision to map to not found, got %v", err)
	}
}

type noRowsDB struct{}

func (noRowsDB) Exec(context.Context, string, ...interface{}) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, nil
}

func (noRowsDB) Query(context.Context, string, ...interface{}) (pgx.Rows, error) {
	return nil, pgx.ErrNoRows
}

func (noRowsDB) QueryRow(context.Context, string, ...interface{}) pgx.Row {
	return noRowsRow{}
}

type noRowsRow struct{}

func (noRowsRow) Scan(...interface{}) error {
	return pgx.ErrNoRows
}

func numericFromString(t *testing.T, value string) pgtype.Numeric {
	t.Helper()

	var numeric pgtype.Numeric
	if err := numeric.Scan(value); err != nil {
		t.Fatalf("scan numeric %q: %v", value, err)
	}
	return numeric
}

func (r *memoryRepository) CreateProjectTaskGraph(ctx context.Context, req CreateProjectTaskGraphRequest) (CreateProjectTaskGraphResult, error) {
	return CreateProjectTaskGraphResult{}, ErrProjectTaskGraphPending
}

func (r *memoryRepository) ListProjectTaskDependencies(ctx context.Context, tenantID, projectID uuid.UUID, dependentTaskIDs []uuid.UUID) ([]ProjectTaskDependency, error) {
	return nil, nil
}

func (r *memoryRepository) ListDependentsOfTask(ctx context.Context, tenantID, projectID, blockerTaskID uuid.UUID) ([]uuid.UUID, error) {
	return nil, nil
}

func (r *memoryRepository) ListUnresolvedBlockersForTasks(ctx context.Context, tenantID, projectID uuid.UUID, dependentTaskIDs []uuid.UUID) ([]ProjectTaskDependencyReadiness, error) {
	return nil, nil
}

func (r *memoryRepository) ListProjectTasksByCoordinationJob(ctx context.Context, tenantID, projectID, coordinationJobID uuid.UUID) ([]ProjectTask, error) {
	return nil, nil
}

func (r *memoryRepository) GetProjectTaskCompletionContract(ctx context.Context, tenantID, taskID uuid.UUID) (ProjectTaskCompletionContract, error) {
	return ProjectTaskCompletionContract{}, ErrProjectNotFound
}

func (r *memoryRepository) GetCoordinationJobByTrigger(ctx context.Context, tenantID uuid.UUID, workflowID string, triggerEventID uuid.UUID, jobType string) (CoordinationJob, error) {
	return CoordinationJob{}, ErrProjectNotFound
}

func (r *memoryRepository) GetRouteDecisionByCoordinationJob(ctx context.Context, tenantID, coordinationJobID uuid.UUID) (RouteDecision, error) {
	return RouteDecision{}, ErrProjectNotFound
}

func (r *memoryRepository) GetProjectTaskGraph(ctx context.Context, req GetProjectTaskGraphRequest) (ProjectTaskGraph, error) {
	return ProjectTaskGraph{}, ErrProjectTaskGraphPending
}

func newProjectRepositoryTestStore(t *testing.T) (Repository, uuid.UUID) {
	t.Helper()

	databaseURL := strings.TrimSpace(os.Getenv("TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("set TEST_DATABASE_URL to run project repository integration test")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, databaseURL)
	require.NoError(t, err)
	require.NoError(t, pool.Ping(ctx))

	tenantID := uuid.New()
	_, err = pool.Exec(ctx,
		`INSERT INTO tenants (id, slug, name, status)
		 VALUES ($1, $2, $3, 'active')
		 ON CONFLICT (id) DO NOTHING`,
		tenantID,
		"project-repository-test-"+tenantID.String(),
		"Project repository test "+tenantID.String(),
	)
	require.NoError(t, err)
	t.Cleanup(func() {
		for _, statement := range []string{
			"DELETE FROM project_task_attempts WHERE tenant_id = $1",
			"DELETE FROM project_execution_summaries WHERE tenant_id = $1",
			"DELETE FROM project_decision_requests WHERE tenant_id = $1",
			"DELETE FROM project_task_dependencies WHERE tenant_id = $1",
			"DELETE FROM project_tasks WHERE tenant_id = $1",
			"DELETE FROM project_plan_decomposition_claims WHERE tenant_id = $1",
			"DELETE FROM project_plan_revisions WHERE tenant_id = $1",
			"DELETE FROM task_runs WHERE tenant_id = $1",
			"DELETE FROM tasks WHERE tenant_id = $1",
			"DELETE FROM project_route_decisions WHERE tenant_id = $1",
			"DELETE FROM project_coordination_jobs WHERE tenant_id = $1",
			"DELETE FROM project_demands WHERE tenant_id = $1",
			"DELETE FROM project_events WHERE tenant_id = $1",
			"DELETE FROM project_config_revisions WHERE tenant_id = $1",
			"DELETE FROM project_members WHERE tenant_id = $1",
			"DELETE FROM projects WHERE tenant_id = $1",
			"DELETE FROM tenants WHERE id = $1",
		} {
			_, _ = pool.Exec(context.Background(), statement, tenantID)
		}
		pool.Close()
	})

	return NewPgRepository(queries.New(pool), pool), tenantID
}

func createProjectFixture(t *testing.T, repo Repository, tenantID uuid.UUID) uuid.UUID {
	t.Helper()

	projectID := uuid.New()
	_, err := repo.CreateProject(context.Background(), CreateProjectRequest{
		TenantID:         tenantID,
		ActorUserID:      uuid.New(),
		Name:             "项目任务图测试",
		Goal:             "验证任务图契约字段",
		HumanOwnerUserID: uuid.New(),
	}, projectID, "project-coordinator:"+projectID.String())
	require.NoError(t, err)
	return projectID
}

func createDemandFixture(t *testing.T, repo Repository, tenantID, projectID uuid.UUID) uuid.UUID {
	t.Helper()
	return createDemandFixtureWithSourceRefs(t, repo, tenantID, projectID, nil)
}

func createDemandFixtureWithSourceRefs(t *testing.T, repo Repository, tenantID, projectID uuid.UUID, sourceRefs map[string]any) uuid.UUID {
	t.Helper()
	return createDemandFixtureWithStatusAndSourceRefs(t, repo, tenantID, projectID, ProjectDemandStatusRecorded, sourceRefs)
}

func createDemandFixtureWithStatusAndSourceRefs(t *testing.T, repo Repository, tenantID, projectID uuid.UUID, status ProjectDemandStatus, sourceRefs map[string]any) uuid.UUID {
	t.Helper()

	event, err := repo.AppendProjectEvent(context.Background(), AppendProjectEventRequest{
		TenantID:  tenantID,
		ProjectID: projectID,
		EventType: ProjectEventDemandSubmitted,
		ActorType: "human_user",
		ActorID:   uuid.New().String(),
		Summary:   "需求已提交",
		Payload:   map[string]any{"title": "验证任务图"},
	})
	require.NoError(t, err)
	demand, err := repo.CreateProjectDemand(context.Background(), SubmitProjectDemandRequest{
		TenantID:          tenantID,
		ProjectID:         projectID,
		SubmittedByUserID: uuid.New(),
		Title:             "验证任务图",
		Content:           "验证任务节点和依赖边",
		SourceType:        DemandSourceManual,
		SourceRefs:        sourceRefs,
	}, status, &event.ID)
	require.NoError(t, err)
	return demand.ID
}

func createCoordinationJobFixture(t *testing.T, repo Repository, tenantID, projectID uuid.UUID) uuid.UUID {
	t.Helper()

	job, err := repo.CreateCoordinationJob(context.Background(), CreateCoordinationJobRequest{
		TenantID:         tenantID,
		ProjectID:        projectID,
		WorkflowID:       "project-coordinator:" + projectID.String(),
		TriggerEventID:   nil,
		JobType:          "demand_launch",
		Status:           "running",
		InputSnapshotRef: map[string]any{"mode": "test"},
	})
	require.NoError(t, err)
	return job.ID
}

func createRouteDecisionFixture(t *testing.T, repo Repository, tenantID, projectID, jobID, demandID uuid.UUID) uuid.UUID {
	t.Helper()

	route, err := repo.CreateRouteDecision(context.Background(), CreateRouteDecisionRequest{
		TenantID:                    tenantID,
		ProjectID:                   projectID,
		CoordinationJobID:           jobID,
		DemandID:                    &demandID,
		CandidateDigitalEmployeeIDs: []uuid.UUID{uuid.New()},
		SelectedDigitalEmployeeIDs:  []uuid.UUID{uuid.New()},
		Reason:                      "测试任务图",
		InputRequirements:           map[string]any{"mode": "test"},
		ExpectedOutputs:             []any{"execution_summary"},
		BudgetEstimate:              map[string]any{"mode": "test"},
		RequiresHumanReview:         false,
	})
	require.NoError(t, err)
	return route.ID
}

func createProjectTaskGraphFixtureRequest(tenantID, projectID, demandID, jobID, routeID uuid.UUID) CreateProjectTaskGraphRequest {
	employeeID := uuid.New()
	stageZero := int32(0)
	stageOne := int32(1)
	return CreateProjectTaskGraphRequest{
		TenantID:          tenantID,
		ProjectID:         projectID,
		DemandID:          demandID,
		CoordinationJobID: jobID,
		RouteDecisionID:   routeID,
		Tasks: []ProjectTaskGraphCreateTask{
			{
				Key:                       "t1",
				Title:                     "分析",
				Summary:                   "分析",
				Status:                    "planned",
				AssignedDigitalEmployeeID: employeeID,
				TaskKind:                  "analysis",
				StageIndex:                &stageZero,
				RiskLevel:                 "medium",
				ExpectedOutputs:           []any{"execution_summary"},
				InputRequirements:         map[string]any{},
				HandoffContract:           map[string]any{},
				PlannerMetadata:           map[string]any{"planner": "test"},
			},
			{
				Key:                       "t2",
				Title:                     "复核",
				Summary:                   "复核",
				Status:                    "blocked",
				AssignedDigitalEmployeeID: employeeID,
				TaskKind:                  "review",
				StageIndex:                &stageOne,
				RiskLevel:                 "normal",
				ExpectedOutputs:           []any{"execution_summary"},
				InputRequirements:         map[string]any{},
				HandoffContract:           map[string]any{},
				PlannerMetadata:           map[string]any{"planner": "test"},
				BlockedByKeys:             []string{"t1"},
			},
		},
	}
}

func createDecomposeAcceptedPlanRevisionFixtureRequest(t *testing.T, repo Repository, tenantID, projectID, demandID, jobID, routeID uuid.UUID) DecomposeAcceptedPlanRevisionRequest {
	t.Helper()

	revision, err := repo.CreatePlanRevision(context.Background(), CreatePlanRevisionRequest{
		TenantID:        tenantID,
		ProjectID:       projectID,
		DemandID:        demandID,
		Status:          PlanRevisionStatusAccepted,
		Payload:         map[string]any{"summary": "accepted fixture"},
		PlanFingerprint: "fingerprint-" + uuid.NewString(),
	})
	require.NoError(t, err)
	claimKey := "project-plan-decomposition:" + tenantID.String() + ":" + projectID.String() + ":" + demandID.String() + ":" + revision.ID.String()
	graphReq := createProjectTaskGraphFixtureRequest(tenantID, projectID, demandID, jobID, routeID)
	for i := range graphReq.Tasks {
		metadata := map[string]any{}
		for key, value := range graphReq.Tasks[i].PlannerMetadata {
			metadata[key] = value
		}
		metadata["accepted_plan_revision_id"] = revision.ID.String()
		graphReq.Tasks[i].PlannerMetadata = metadata
	}
	return DecomposeAcceptedPlanRevisionRequest{
		TenantID:               tenantID,
		ProjectID:              projectID,
		DemandID:               demandID,
		CoordinationJobID:      jobID,
		RouteDecisionID:        routeID,
		AcceptedPlanRevisionID: revision.ID,
		PlanFingerprint:        revision.PlanFingerprint,
		DecompositionClaimKey:  claimKey,
		Tasks:                  graphReq.Tasks,
	}
}

func projectTaskFromGraphRequestForTest(req CreateProjectTaskGraphRequest, planned ProjectTaskGraphCreateTask) ProjectTask {
	demandID := req.DemandID
	jobID := req.CoordinationJobID
	routeID := req.RouteDecisionID
	employeeID := planned.AssignedDigitalEmployeeID
	return ProjectTask{
		ID:                        uuid.New(),
		TenantID:                  req.TenantID,
		ProjectID:                 req.ProjectID,
		DemandID:                  &demandID,
		Title:                     planned.Title,
		Summary:                   strPtr(planned.Summary),
		Status:                    planned.Status,
		AssignedDigitalEmployeeID: &employeeID,
		RiskLevel:                 strPtr(planned.RiskLevel),
		RequiresHumanApproval:     planned.RequiresHumanApproval,
		CoordinationJobID:         &jobID,
		RouteDecisionID:           &routeID,
		PlannedTaskKey:            strPtr(planned.Key),
		TaskKind:                  strPtr(planned.TaskKind),
		StageIndex:                planned.StageIndex,
		ExpectedOutputs:           planned.ExpectedOutputs,
		InputRequirements:         planned.InputRequirements,
		HandoffContract:           planned.HandoffContract,
		PlannerMetadata:           planned.PlannerMetadata,
	}
}

func requireEventCount(t *testing.T, events []ProjectEvent, eventType ProjectEventType, expected int) {
	t.Helper()

	count := 0
	for _, event := range events {
		if event.EventType == eventType {
			count++
		}
	}
	require.Equal(t, expected, count, "event type %s", eventType)
}

func executionSummaryIDs(summaries []ExecutionSummary) []uuid.UUID {
	ids := make([]uuid.UUID, 0, len(summaries))
	for _, summary := range summaries {
		ids = append(ids, summary.ID)
	}
	return ids
}

func projectEventIDs(events []ProjectEvent) []uuid.UUID {
	ids := make([]uuid.UUID, 0, len(events))
	for _, event := range events {
		ids = append(ids, event.ID)
	}
	return ids
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
