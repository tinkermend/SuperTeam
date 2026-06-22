package project

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestCreateProjectRequiresHumanOwnerAndCreatesEvents(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	ownerID := uuid.New()
	employeeID := uuid.New()

	created, err := service.CreateProject(context.Background(), CreateProjectRequest{
		TenantID:         tenantID,
		ActorUserID:      ownerID,
		Name:             "支付网关稳定性整改",
		Goal:             "修复超时链路并形成验收报告",
		HumanOwnerUserID: ownerID,
		Members: []ProjectMemberInput{
			{PrincipalType: PrincipalTypeHumanUser, PrincipalID: ownerID, ProjectRole: ProjectRoleOwner, DisplayNameSnapshot: "王佩"},
			{PrincipalType: PrincipalTypeDigitalEmployee, PrincipalID: employeeID, ProjectRole: ProjectRoleExecutor, DisplayNameSnapshot: "后端执行 A", Settings: map[string]any{"concurrency_slots": float64(2)}},
		},
	})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	if created.Project.Status != ProjectStatusRunning {
		t.Fatalf("expected running project, got %s", created.Project.Status)
	}
	if created.Project.CoordinationStatus != "registered" {
		t.Fatalf("expected registered coordination status, got %s", created.Project.CoordinationStatus)
	}
	if !strings.HasPrefix(created.Project.CoordinationWorkflowID, "project-coordinator:") {
		t.Fatalf("expected coordination workflow id, got %q", created.Project.CoordinationWorkflowID)
	}
	if repo.eventTypes[0] != ProjectEventCreated || repo.eventTypes[1] != ProjectEventConfigChanged {
		t.Fatalf("expected create/config events, got %#v", repo.eventTypes)
	}
	for _, member := range created.Members {
		if member.ProjectRole == ProjectRole("coordinator") {
			t.Fatal("coordinator must not be represented as a project member")
		}
	}
}

func TestRuntimeWritebackProjectTaskStatusesIncludeQueued(t *testing.T) {
	require.ElementsMatch(t, []string{"assigned", "queued", "running"}, runtimeWritebackProjectTaskStatuses())
	require.True(t, projectTaskAcceptsRuntimeWriteback("queued"))
}

func TestGetExecutionTraceGroupsEventsByAttempt(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	taskID := uuid.New()
	attemptID := uuid.New()
	summaryID := uuid.New()
	now := time.Now().UTC()
	repo := newMemoryRepository()
	repo.tasks = append(repo.tasks, ProjectTask{
		ID:        taskID,
		TenantID:  tenantID,
		ProjectID: projectID,
		Title:     "核对证据链",
		Status:    ProjectTaskStatusCompleted,
	})
	repo.projectTaskAttempts = append(repo.projectTaskAttempts, ProjectTaskAttempt{
		ID:            attemptID,
		TenantID:      tenantID,
		ProjectTaskID: taskID,
		AttemptNo:     1,
		Status:        ProjectTaskAttemptStatusSucceeded,
		StartedAt:     &now,
		FinishedAt:    &now,
		CreatedAt:     now,
		UpdatedAt:     now,
	})
	repo.executionSummaries = append(repo.executionSummaries, ExecutionSummary{
		ID:                  summaryID,
		TenantID:            tenantID,
		ProjectID:           projectID,
		ProjectTaskID:       taskID,
		DigitalEmployeeID:   uuid.New(),
		Conclusion:          "证据链完整",
		EvidenceRefs:        []any{map[string]any{"ref": "evidence://1"}},
		ArtifactRefs:        []any{map[string]any{"ref": "artifact://1"}},
		ConfidenceFactors:   map[string]any{"source": "test"},
		MissingInformation:  []any{},
		RequiresHumanReview: false,
		CreatedAt:           now,
	})
	repo.executionLedgerEvents = append(repo.executionLedgerEvents, ExecutionLedgerEvent{
		ID:                   uuid.New(),
		TenantID:             tenantID,
		ProjectID:            projectID,
		ProjectTaskID:        &taskID,
		ProjectTaskAttemptID: &attemptID,
		EventType:            ExecutionLedgerEventAttemptStarted,
		SourceType:           "project_task_attempt",
		SourceID:             attemptID.String(),
		ActorType:            "runtime_node",
		InputSummary:         strPtr("Runtime started attempt"),
		OccurredAt:           now,
		CreatedAt:            now,
	})
	repo.executionLedgerEvents = append(repo.executionLedgerEvents, ExecutionLedgerEvent{
		ID:                   uuid.New(),
		TenantID:             tenantID,
		ProjectID:            projectID,
		ProjectTaskID:        &taskID,
		ProjectTaskAttemptID: &attemptID,
		EventType:            ExecutionLedgerEventSummaryCreated,
		SourceType:           "project_execution_summary",
		SourceID:             summaryID.String(),
		ActorType:            "system",
		OutputSummary:        strPtr("证据链完整"),
		OccurredAt:           now.Add(time.Second),
		CreatedAt:            now.Add(time.Second),
	})

	service, err := NewService(repo)
	require.NoError(t, err)
	trace, err := service.GetExecutionTrace(context.Background(), GetExecutionTraceRequest{
		TenantID:  tenantID,
		ProjectID: projectID,
		Limit:     100,
	})
	require.NoError(t, err)
	require.Equal(t, projectID, trace.ProjectID)
	require.Equal(t, int32(1), trace.Summary.AttemptCount)
	require.Equal(t, int32(1), trace.Summary.ArtifactRefCount)
	require.Equal(t, int32(1), trace.Summary.EvidenceRefCount)
	require.Len(t, trace.Attempts, 1)
	require.Equal(t, attemptID, trace.Attempts[0].AttemptID)
	require.Len(t, trace.Attempts[0].Events, 2)
	require.NotNil(t, trace.Attempts[0].Summary)
	require.Equal(t, summaryID, trace.Attempts[0].Summary.ExecutionSummaryID)
}

func TestGetExecutionTraceDoesNotFallbackSummaryWhenTaskHasMatchedSummaryEvent(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	taskID := uuid.New()
	firstAttemptID := uuid.New()
	secondAttemptID := uuid.New()
	summaryID := uuid.New()
	now := time.Now().UTC()
	repo := newMemoryRepository()
	repo.tasks = append(repo.tasks, ProjectTask{
		ID:        taskID,
		TenantID:  tenantID,
		ProjectID: projectID,
		Title:     "核对证据链",
		Status:    ProjectTaskStatusCompleted,
	})
	repo.projectTaskAttempts = append(repo.projectTaskAttempts,
		ProjectTaskAttempt{
			ID:            firstAttemptID,
			TenantID:      tenantID,
			ProjectTaskID: taskID,
			AttemptNo:     1,
			Status:        ProjectTaskAttemptStatusFailed,
			CreatedAt:     now,
			UpdatedAt:     now,
		},
		ProjectTaskAttempt{
			ID:            secondAttemptID,
			TenantID:      tenantID,
			ProjectTaskID: taskID,
			AttemptNo:     2,
			Status:        ProjectTaskAttemptStatusSucceeded,
			CreatedAt:     now.Add(time.Second),
			UpdatedAt:     now.Add(time.Second),
		},
	)
	repo.executionSummaries = append(repo.executionSummaries, ExecutionSummary{
		ID:                  summaryID,
		TenantID:            tenantID,
		ProjectID:           projectID,
		ProjectTaskID:       taskID,
		DigitalEmployeeID:   uuid.New(),
		Conclusion:          "第二次尝试证据链完整",
		EvidenceRefs:        []any{map[string]any{"ref": "evidence://2"}},
		ArtifactRefs:        []any{map[string]any{"ref": "artifact://2"}},
		ConfidenceFactors:   map[string]any{"source": "test"},
		MissingInformation:  []any{},
		RequiresHumanReview: true,
		CreatedAt:           now.Add(2 * time.Second),
	})
	repo.executionLedgerEvents = append(repo.executionLedgerEvents, ExecutionLedgerEvent{
		ID:                   uuid.New(),
		TenantID:             tenantID,
		ProjectID:            projectID,
		ProjectTaskID:        &taskID,
		ProjectTaskAttemptID: &secondAttemptID,
		EventType:            ExecutionLedgerEventSummaryCreated,
		SourceType:           "project_execution_summary",
		SourceID:             summaryID.String(),
		ActorType:            "system",
		OutputSummary:        strPtr("第二次尝试证据链完整"),
		OccurredAt:           now.Add(3 * time.Second),
		CreatedAt:            now.Add(3 * time.Second),
	})

	service, err := NewService(repo)
	require.NoError(t, err)
	trace, err := service.GetExecutionTrace(context.Background(), GetExecutionTraceRequest{
		TenantID:  tenantID,
		ProjectID: projectID,
		Limit:     100,
	})
	require.NoError(t, err)
	require.Len(t, trace.Attempts, 2)
	require.Equal(t, firstAttemptID, trace.Attempts[0].AttemptID)
	require.Nil(t, trace.Attempts[0].Summary)
	require.Equal(t, secondAttemptID, trace.Attempts[1].AttemptID)
	require.NotNil(t, trace.Attempts[1].Summary)
	require.Equal(t, summaryID, trace.Attempts[1].Summary.ExecutionSummaryID)
	require.Equal(t, int32(1), trace.Summary.ArtifactRefCount)
	require.Equal(t, int32(1), trace.Summary.EvidenceRefCount)
	require.Equal(t, int32(1), trace.Summary.HumanReviewRequiredCount)
}

func TestGetExecutionTraceRequestsThousandExecutionSummaries(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	repo := newMemoryRepository()
	service, err := NewService(repo)
	require.NoError(t, err)

	_, err = service.GetExecutionTrace(context.Background(), GetExecutionTraceRequest{
		TenantID:  tenantID,
		ProjectID: projectID,
		Limit:     100,
	})
	require.NoError(t, err)
	require.Equal(t, int32(1000), repo.lastExecutionSummariesLimit)
	require.Equal(t, int32(0), repo.lastExecutionSummariesOffset)
}

func TestGetExecutionTraceUsesSummaryMappingOutsideVisibleEventFilter(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	taskID := uuid.New()
	firstAttemptID := uuid.New()
	secondAttemptID := uuid.New()
	summaryID := uuid.New()
	now := time.Now().UTC()
	eventType := ExecutionLedgerEventProviderEvent
	repo := newMemoryRepository()
	repo.tasks = append(repo.tasks, ProjectTask{
		ID:        taskID,
		TenantID:  tenantID,
		ProjectID: projectID,
		Title:     "核对执行事件",
		Status:    ProjectTaskStatusCompleted,
	})
	repo.projectTaskAttempts = append(repo.projectTaskAttempts,
		ProjectTaskAttempt{
			ID:            firstAttemptID,
			TenantID:      tenantID,
			ProjectTaskID: taskID,
			AttemptNo:     1,
			Status:        ProjectTaskAttemptStatusFailed,
			CreatedAt:     now,
			UpdatedAt:     now,
		},
		ProjectTaskAttempt{
			ID:            secondAttemptID,
			TenantID:      tenantID,
			ProjectTaskID: taskID,
			AttemptNo:     2,
			Status:        ProjectTaskAttemptStatusSucceeded,
			CreatedAt:     now.Add(time.Second),
			UpdatedAt:     now.Add(time.Second),
		},
	)
	repo.executionSummaries = append(repo.executionSummaries, ExecutionSummary{
		ID:                  summaryID,
		TenantID:            tenantID,
		ProjectID:           projectID,
		ProjectTaskID:       taskID,
		DigitalEmployeeID:   uuid.New(),
		Conclusion:          "第二次尝试完成",
		EvidenceRefs:        []any{map[string]any{"ref": "evidence://summary-filtered"}},
		ArtifactRefs:        []any{map[string]any{"ref": "artifact://summary-filtered"}},
		ConfidenceFactors:   map[string]any{"source": "test"},
		MissingInformation:  []any{},
		RequiresHumanReview: false,
		CreatedAt:           now.Add(2 * time.Second),
	})
	repo.executionLedgerEvents = append(repo.executionLedgerEvents,
		ExecutionLedgerEvent{
			ID:                   uuid.New(),
			TenantID:             tenantID,
			ProjectID:            projectID,
			ProjectTaskID:        &taskID,
			ProjectTaskAttemptID: &secondAttemptID,
			EventType:            ExecutionLedgerEventProviderEvent,
			SourceType:           "provider_session_event",
			SourceID:             uuid.NewString(),
			ActorType:            "provider",
			OutputSummary:        strPtr("visible provider event"),
			OccurredAt:           now.Add(3 * time.Second),
			CreatedAt:            now.Add(3 * time.Second),
		},
		ExecutionLedgerEvent{
			ID:                   uuid.New(),
			TenantID:             tenantID,
			ProjectID:            projectID,
			ProjectTaskID:        &taskID,
			ProjectTaskAttemptID: &secondAttemptID,
			EventType:            ExecutionLedgerEventSummaryCreated,
			SourceType:           "project_execution_summary",
			SourceID:             summaryID.String(),
			ActorType:            "system",
			OutputSummary:        strPtr("hidden by visible filter"),
			OccurredAt:           now.Add(4 * time.Second),
			CreatedAt:            now.Add(4 * time.Second),
		},
	)

	service, err := NewService(repo)
	require.NoError(t, err)
	trace, err := service.GetExecutionTrace(context.Background(), GetExecutionTraceRequest{
		TenantID:  tenantID,
		ProjectID: projectID,
		EventType: &eventType,
		Limit:     100,
	})
	require.NoError(t, err)
	require.Len(t, trace.Attempts, 2)
	require.Nil(t, trace.Attempts[0].Summary)
	require.NotNil(t, trace.Attempts[1].Summary)
	require.Equal(t, summaryID, trace.Attempts[1].Summary.ExecutionSummaryID)
	require.Empty(t, trace.Attempts[0].Events)
	require.Len(t, trace.Attempts[1].Events, 1)
	require.Equal(t, ExecutionLedgerEventProviderEvent, trace.Attempts[1].Events[0].EventType)
	require.Len(t, repo.executionLedgerEventListRequests, 2)
	require.Equal(t, &eventType, repo.executionLedgerEventListRequests[0].EventType)
	require.NotNil(t, repo.executionLedgerEventListRequests[1].EventType)
	require.Equal(t, ExecutionLedgerEventSummaryCreated, *repo.executionLedgerEventListRequests[1].EventType)
	require.Nil(t, repo.executionLedgerEventListRequests[1].ErrorFamily)
	require.Equal(t, int32(1000), repo.executionLedgerEventListRequests[1].Limit)
	require.Equal(t, int32(0), repo.executionLedgerEventListRequests[1].Offset)
}

func TestGetExecutionTraceFallbackSummaryAttachesToLatestAttempt(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	taskID := uuid.New()
	firstAttemptID := uuid.New()
	secondAttemptID := uuid.New()
	summaryID := uuid.New()
	startedAt := time.Now().UTC()
	finishedAt := startedAt.Add(5 * time.Second)
	secondStartedAt := startedAt.Add(10 * time.Second)
	repo := newMemoryRepository()
	repo.tasks = append(repo.tasks, ProjectTask{
		ID:        taskID,
		TenantID:  tenantID,
		ProjectID: projectID,
		Title:     "生成验收报告",
		Status:    ProjectTaskStatusCompleted,
	})
	repo.projectTaskAttempts = append(repo.projectTaskAttempts,
		ProjectTaskAttempt{
			ID:            firstAttemptID,
			TenantID:      tenantID,
			ProjectTaskID: taskID,
			AttemptNo:     1,
			Status:        ProjectTaskAttemptStatusFailed,
			StartedAt:     &startedAt,
			FinishedAt:    &finishedAt,
			CreatedAt:     startedAt,
			UpdatedAt:     finishedAt,
		},
		ProjectTaskAttempt{
			ID:            secondAttemptID,
			TenantID:      tenantID,
			ProjectTaskID: taskID,
			AttemptNo:     2,
			Status:        ProjectTaskAttemptStatusSucceeded,
			StartedAt:     &secondStartedAt,
			CreatedAt:     secondStartedAt,
			UpdatedAt:     secondStartedAt,
		},
	)
	repo.executionSummaries = append(repo.executionSummaries, ExecutionSummary{
		ID:                  summaryID,
		TenantID:            tenantID,
		ProjectID:           projectID,
		ProjectTaskID:       taskID,
		DigitalEmployeeID:   uuid.New(),
		Conclusion:          "最新尝试完成报告",
		EvidenceRefs:        []any{map[string]any{"ref": "evidence://latest"}},
		ArtifactRefs:        []any{map[string]any{"ref": "artifact://latest"}},
		ConfidenceFactors:   map[string]any{"source": "test"},
		MissingInformation:  []any{},
		RequiresHumanReview: false,
		CreatedAt:           startedAt.Add(11 * time.Second),
	})

	service, err := NewService(repo)
	require.NoError(t, err)
	trace, err := service.GetExecutionTrace(context.Background(), GetExecutionTraceRequest{
		TenantID:  tenantID,
		ProjectID: projectID,
		Limit:     100,
	})
	require.NoError(t, err)
	require.Len(t, trace.Attempts, 2)
	require.Equal(t, firstAttemptID, trace.Attempts[0].AttemptID)
	require.Nil(t, trace.Attempts[0].Summary)
	require.Equal(t, secondAttemptID, trace.Attempts[1].AttemptID)
	require.NotNil(t, trace.Attempts[1].Summary)
	require.Equal(t, summaryID, trace.Attempts[1].Summary.ExecutionSummaryID)
}

func TestGetExecutionTraceDeduplicatesAggregateRefsAcrossSummaryEventAndSummary(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	taskID := uuid.New()
	attemptID := uuid.New()
	summaryID := uuid.New()
	now := time.Now().UTC()
	artifactRef := map[string]any{"ref": "artifact://same"}
	evidenceRef := map[string]any{"ref": "evidence://same"}
	repo := newMemoryRepository()
	repo.tasks = append(repo.tasks, ProjectTask{
		ID:        taskID,
		TenantID:  tenantID,
		ProjectID: projectID,
		Title:     "去重引用计数",
		Status:    ProjectTaskStatusCompleted,
	})
	repo.projectTaskAttempts = append(repo.projectTaskAttempts, ProjectTaskAttempt{
		ID:            attemptID,
		TenantID:      tenantID,
		ProjectTaskID: taskID,
		AttemptNo:     1,
		Status:        ProjectTaskAttemptStatusSucceeded,
		CreatedAt:     now,
		UpdatedAt:     now,
	})
	repo.executionSummaries = append(repo.executionSummaries, ExecutionSummary{
		ID:                  summaryID,
		TenantID:            tenantID,
		ProjectID:           projectID,
		ProjectTaskID:       taskID,
		DigitalEmployeeID:   uuid.New(),
		Conclusion:          "引用相同",
		EvidenceRefs:        []any{evidenceRef},
		ArtifactRefs:        []any{artifactRef},
		ConfidenceFactors:   map[string]any{"source": "test"},
		MissingInformation:  []any{},
		RequiresHumanReview: false,
		CreatedAt:           now,
	})
	repo.executionLedgerEvents = append(repo.executionLedgerEvents, ExecutionLedgerEvent{
		ID:                   uuid.New(),
		TenantID:             tenantID,
		ProjectID:            projectID,
		ProjectTaskID:        &taskID,
		ProjectTaskAttemptID: &attemptID,
		EventType:            ExecutionLedgerEventSummaryCreated,
		SourceType:           "project_execution_summary",
		SourceID:             summaryID.String(),
		ActorType:            "system",
		ArtifactRefs:         []any{artifactRef},
		EvidenceRefs:         []any{evidenceRef},
		OccurredAt:           now,
		CreatedAt:            now,
	})

	service, err := NewService(repo)
	require.NoError(t, err)
	trace, err := service.GetExecutionTrace(context.Background(), GetExecutionTraceRequest{
		TenantID:  tenantID,
		ProjectID: projectID,
		Limit:     100,
	})
	require.NoError(t, err)
	require.Equal(t, int32(1), trace.Summary.ArtifactRefCount)
	require.Equal(t, int32(1), trace.Summary.EvidenceRefCount)
	require.Len(t, trace.Attempts, 1)
	require.Len(t, trace.Attempts[0].Events, 1)
	require.Len(t, trace.Attempts[0].Events[0].ArtifactRefs, 1)
	require.Len(t, trace.Attempts[0].Summary.ArtifactRefs, 1)
}

func TestBuildProjectTaskExecutionPacketIncludesDependenciesAndHumanDecisions(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	require.NoError(t, err)
	tenantID := uuid.New()
	projectID := uuid.New()
	taskID := uuid.New()
	blockerID := uuid.New()
	decisionID := uuid.New()
	task := ProjectTask{
		ID:              taskID,
		TenantID:        tenantID,
		ProjectID:       projectID,
		Title:           "执行上线检查",
		Status:          ProjectTaskStatusPlanned,
		ExpectedOutputs: []any{"deployment_report"},
		InputRequirements: map[string]any{
			"environment": "staging",
		},
		HandoffContract: map[string]any{
			"completion_path": "project_task_attempt_writeback",
		},
		BlockedByTaskIDs: []uuid.UUID{blockerID},
	}
	repo.executionSummaries = append(repo.executionSummaries, ExecutionSummary{
		ID:            uuid.New(),
		TenantID:      tenantID,
		ProjectID:     projectID,
		ProjectTaskID: blockerID,
		Conclusion:    "依赖任务已完成，产出 staging 检查清单。",
		EvidenceRefs:  []any{"evidence://staging-checklist"},
	})
	repo.decisionRequests = append(repo.decisionRequests, DecisionRequest{
		ID:             decisionID,
		TenantID:       tenantID,
		ProjectID:      projectID,
		ProjectTaskID:  &taskID,
		DecisionType:   "approval_required",
		StatusSnapshot: ProjectTaskStatusWaitingHuman,
	})

	packet, err := service.BuildProjectTaskExecutionPacket(context.Background(), task)
	require.NoError(t, err)
	require.Equal(t, "v1", packet.Version)
	require.Equal(t, taskID.String(), packet.ProjectTaskID)
	require.Contains(t, packet.ExpectedOutputs, "deployment_report")
	require.Len(t, packet.DependencyOutputs, 1)
	require.Equal(t, "evidence://staging-checklist", packet.DependencyOutputs[0].EvidenceRefs[0])
	require.Len(t, packet.HumanDecisionRefs, 1)
}

func TestRecordAttemptContextUpdateRoutesContractChangeToReplan(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	require.NoError(t, err)
	tenantID := uuid.New()
	projectID := uuid.New()
	taskID := uuid.New()
	attemptID := uuid.New()
	repo.tasks = append(repo.tasks, ProjectTask{
		ID:               taskID,
		TenantID:         tenantID,
		ProjectID:        projectID,
		Status:           ProjectTaskStatusRunning,
		CurrentAttemptID: &attemptID,
	})

	update, err := service.RecordAttemptContextUpdate(context.Background(), RecordAttemptContextUpdateRequest{
		TenantID:      tenantID,
		ProjectID:     projectID,
		ProjectTaskID: taskID,
		AttemptID:     &attemptID,
		UpdateKind:    "requirement_changed",
		Payload:       map[string]any{"new_scope": "include production"},
	})
	require.NoError(t, err)
	require.Equal(t, ContextUpdateDeliveryCancelAndReplan, update.DeliveryMode)
}

func TestProjectTaskLivenessProjectionExplainsNextAction(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	require.NoError(t, err)
	tenantID := uuid.New()
	projectID := uuid.New()
	taskID := uuid.New()
	attemptID := uuid.New()
	waitingReason := HumanWaitReasonMissingContext
	repo.tasks = append(repo.tasks, ProjectTask{
		ID:               taskID,
		TenantID:         tenantID,
		ProjectID:        projectID,
		Status:           ProjectTaskStatusWaitingHuman,
		CurrentAttemptID: &attemptID,
		WaitingReason:    &waitingReason,
	})

	items, err := service.ListProjectTaskLiveness(context.Background(), tenantID, projectID)
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, ProjectTaskLivenessWaitingHuman, items[0].Liveness)
	require.Equal(t, "human response", items[0].NextAction)
	require.Equal(t, HumanWaitReasonMissingContext, items[0].Reason)
}

func TestQueueProjectTaskCreatesAttemptAndMovesTaskToQueued(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	require.NoError(t, err)

	tenantID := uuid.New()
	projectID := uuid.New()
	taskID := uuid.New()
	employeeID := uuid.New()
	runID := uuid.New()
	runtimeTaskID := uuid.New()
	runtimeNodeID := uuid.New()
	repo.tasks = append(repo.tasks, ProjectTask{
		ID:                        taskID,
		TenantID:                  tenantID,
		ProjectID:                 projectID,
		Title:                     "实现幂等写回",
		Status:                    ProjectTaskStatusPlanned,
		AssignedDigitalEmployeeID: &employeeID,
		CreatedAt:                 time.Now().UTC(),
		UpdatedAt:                 time.Now().UTC(),
	})

	result, err := service.QueueProjectTask(context.Background(), QueueProjectTaskRequest{
		TenantID:             tenantID,
		ProjectID:            projectID,
		ProjectTaskID:        taskID,
		DigitalEmployeeID:    employeeID,
		DigitalEmployeeRunID: &runID,
		RuntimeTaskID:        &runtimeTaskID,
		RuntimeNodeID:        &runtimeNodeID,
		IdempotencyKey:       "project-task:" + taskID.String() + ":attempt:1:queue",
		LeaseToken:           "lease-token-1",
	})
	require.NoError(t, err)
	require.Equal(t, ProjectTaskStatusQueued, result.Task.Status)
	require.NotNil(t, result.Task.CurrentAttemptID)
	require.Equal(t, runID, *result.Task.DigitalEmployeeRunID)
	require.Equal(t, runtimeTaskID, *result.Task.RuntimeTaskID)
	require.Equal(t, int32(1), result.Attempt.AttemptNo)
	require.Equal(t, ProjectTaskAttemptStatusQueued, result.Attempt.Status)
	require.Equal(t, runID, *result.Attempt.DigitalEmployeeRunID)
	require.Equal(t, runtimeTaskID, *result.Attempt.RuntimeTaskID)
	require.Equal(t, runtimeNodeID, *result.Attempt.RuntimeNodeID)
	require.Equal(t, "lease-token-1", result.Attempt.LeaseToken)
	require.Equal(t, "v1", result.Attempt.ExecutionContextPacketVersion)
	require.Equal(t, taskID.String(), result.Attempt.ExecutionContextPacket["project_task_id"])
	require.Equal(t, "实现幂等写回", result.Attempt.ExecutionContextPacket["title"])
	require.Equal(t, "project_coordinator", result.Event.ActorType)
	require.Equal(t, taskID.String(), result.Event.ActorID)
	require.Equal(t, result.Attempt.ID.String(), result.Event.Payload["project_task_attempt_id"])
	require.Equal(t, ProjectTaskStatusQueued, result.Event.Payload["project_task_status"])
	require.Equal(t, runID.String(), result.Event.Payload["digital_employee_run_id"])
	require.Equal(t, runtimeTaskID.String(), result.Event.Payload["runtime_task_id"])
	require.Equal(t, runtimeNodeID.String(), result.Event.Payload["runtime_node_id"])
}

func TestStartProjectTaskAttemptAdvancesRunning(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	require.NoError(t, err)
	fixture := newProjectTaskAttemptServiceFixture(repo, ProjectTaskStatusQueued, ProjectTaskAttemptStatusQueued)

	started, err := service.StartProjectTaskAttempt(context.Background(), StartProjectTaskAttemptRequest{
		ProjectTaskAttemptRuntimeRequest: fixture.runtimeRequest("attempt-start-1"),
	})

	require.NoError(t, err)
	require.Equal(t, ProjectTaskAttemptStatusRunning, started.Status)
	require.Equal(t, ProjectTaskStatusRunning, repo.tasks[0].Status)
	require.NotNil(t, started.StartedAt)
	require.NotNil(t, started.RenewedAt)
}

func TestStartProjectTaskAttemptRetriesUntilQueuedAttemptVisible(t *testing.T) {
	baseRepo := newMemoryRepository()
	fixture := newProjectTaskAttemptServiceFixture(baseRepo, ProjectTaskStatusQueued, ProjectTaskAttemptStatusQueued)
	repo := &delayedAttemptReadinessRepository{
		memoryRepository:    baseRepo,
		staleProjectTaskID:  fixture.taskID,
		staleReadsRemaining: 1,
	}
	service, err := NewService(repo)
	require.NoError(t, err)

	started, err := service.StartProjectTaskAttempt(context.Background(), StartProjectTaskAttemptRequest{
		ProjectTaskAttemptRuntimeRequest: fixture.runtimeRequest("attempt-start-race"),
	})

	require.NoError(t, err)
	require.Equal(t, ProjectTaskAttemptStatusRunning, started.Status)
	require.Zero(t, repo.staleReadsRemaining)
}

func TestStartProjectTaskAttemptWritesLedgerEvent(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	require.NoError(t, err)
	fixture := newProjectTaskAttemptServiceFixture(repo, ProjectTaskStatusQueued, ProjectTaskAttemptStatusQueued)
	_, err = service.StartProjectTaskAttempt(context.Background(), StartProjectTaskAttemptRequest{
		ProjectTaskAttemptRuntimeRequest: fixture.runtimeRequest("start-ledger"),
	})
	require.NoError(t, err)
	require.Len(t, repo.executionLedgerEvents, 1)
	require.Equal(t, ExecutionLedgerEventAttemptStarted, repo.executionLedgerEvents[0].EventType)
	require.Equal(t, fixture.attemptID, *repo.executionLedgerEvents[0].ProjectTaskAttemptID)
}

func TestStartProjectTaskAttemptIgnoresLedgerWriteFailure(t *testing.T) {
	repo := newMemoryRepository()
	repo.createExecutionLedgerEventErr = fmt.Errorf("ledger unavailable")
	service, err := NewService(repo)
	require.NoError(t, err)
	fixture := newProjectTaskAttemptServiceFixture(repo, ProjectTaskStatusQueued, ProjectTaskAttemptStatusQueued)

	started, err := service.StartProjectTaskAttempt(context.Background(), StartProjectTaskAttemptRequest{
		ProjectTaskAttemptRuntimeRequest: fixture.runtimeRequest("start-ledger-error"),
	})

	require.NoError(t, err)
	require.Equal(t, ProjectTaskAttemptStatusRunning, started.Status)
	require.Equal(t, ProjectTaskStatusRunning, repo.tasks[0].Status)
	require.Equal(t, ProjectTaskAttemptStatusRunning, repo.projectTaskAttempts[0].Status)
	require.Empty(t, repo.executionLedgerEvents)
}

func TestStartProjectTaskAttemptRejectsWrongLeaseToken(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	require.NoError(t, err)
	fixture := newProjectTaskAttemptServiceFixture(repo, ProjectTaskStatusQueued, ProjectTaskAttemptStatusQueued)
	req := fixture.runtimeRequest("attempt-start-1")
	req.LeaseToken = "wrong-token"

	_, err = service.StartProjectTaskAttempt(context.Background(), StartProjectTaskAttemptRequest{
		ProjectTaskAttemptRuntimeRequest: req,
	})

	require.ErrorIs(t, err, ErrProjectConflict)
}

func TestRenewProjectTaskAttemptLeaseUpdatesExpiry(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	require.NoError(t, err)
	fixture := newProjectTaskAttemptServiceFixture(repo, ProjectTaskStatusRunning, ProjectTaskAttemptStatusRunning)
	expiresAt := time.Now().UTC().Add(5 * time.Minute)

	err = service.RenewProjectTaskAttemptLease(context.Background(), RenewProjectTaskAttemptLeaseRequest{
		ProjectTaskAttemptRuntimeRequest: fixture.runtimeRequest("attempt-lease-1"),
		LeaseExpiresAt:                   &expiresAt,
	})

	require.NoError(t, err)
	require.NotNil(t, repo.projectTaskAttempts[0].LeaseExpiresAt)
	require.True(t, repo.projectTaskAttempts[0].LeaseExpiresAt.Equal(expiresAt))
	require.NotNil(t, repo.projectTaskAttempts[0].RenewedAt)
}

func TestCompleteProjectTaskAttemptCreatesSummaryAndCompletesTask(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	require.NoError(t, err)
	fixture := newProjectTaskAttemptServiceFixture(repo, ProjectTaskStatusRunning, ProjectTaskAttemptStatusRunning)

	summary, err := service.CompleteProjectTaskAttempt(context.Background(), CompleteProjectTaskAttemptRequest{
		ProjectTaskAttemptRuntimeRequest: fixture.runtimeRequest("attempt-complete-1"),
		Conclusion:                       "done",
		EvidenceRefs:                     []any{"s3://bucket/report.md"},
	})

	require.NoError(t, err)
	require.Equal(t, fixture.taskID, summary.ProjectTaskID)
	require.Equal(t, ProjectTaskStatusCompleted, repo.tasks[0].Status)
	require.Equal(t, ProjectTaskAttemptStatusSucceeded, repo.projectTaskAttempts[0].Status)
	require.Contains(t, repo.eventTypes, ProjectEventTaskCompleted)
}

func TestCompleteProjectTaskAttemptStoresStructuredResultContract(t *testing.T) {
	repo := newProjectTaskResultMemoryRepository()
	service, err := NewService(repo)
	require.NoError(t, err)
	fixture := newProjectTaskAttemptServiceFixture(repo.memoryRepository, ProjectTaskStatusRunning, ProjectTaskAttemptStatusRunning)
	contract := TaskResultContract{
		Status:  TaskResultStatusCompleted,
		Summary: "完成分析",
		AcceptanceResults: []TaskResultAcceptanceResult{
			{
				Criterion:    "输出结论",
				Status:       TaskResultCriterionStatusPassed,
				EvidenceRefs: []string{"artifact:report"},
			},
		},
		EvidenceRefs: []TaskResultRef{{Type: "report", Ref: "artifact:report"}},
		ArtifactRefs: []TaskResultRef{{Type: "markdown", Ref: "artifact:analysis-report"}},
		Verification: []TaskResultVerification{{Type: "command", Status: TaskResultVerificationStatusPassed, Summary: "命令通过"}},
	}

	summary, err := service.CompleteProjectTaskAttempt(context.Background(), CompleteProjectTaskAttemptRequest{
		ProjectTaskAttemptRuntimeRequest: fixture.runtimeRequest("attempt-complete-with-result"),
		Conclusion:                       "legacy conclusion",
		ResultContract:                   &contract,
	})

	require.NoError(t, err)
	results, err := repo.ListProjectTaskResults(context.Background(), ListProjectTaskResultsRequest{
		TenantID:      fixture.tenantID,
		ProjectID:     fixture.projectID,
		ProjectTaskID: fixture.taskID,
		Limit:         10,
	})
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Equal(t, contract, results[0].Contract)
	require.Equal(t, TaskResultStatusCompleted, results[0].ResultStatus)
	require.Equal(t, TaskResultDecisionCompleteAccepted, results[0].Decision)
	require.Equal(t, "accepted", results[0].ValidationStatus)
	require.NotNil(t, results[0].AttemptID)
	require.Equal(t, fixture.attemptID, *results[0].AttemptID)
	require.NotNil(t, results[0].ExecutionSummaryID)
	require.Equal(t, summary.ID, *results[0].ExecutionSummaryID)
	require.NotNil(t, repo.tasks[0].LatestTaskResultID)
	require.Equal(t, results[0].ID, *repo.tasks[0].LatestTaskResultID)
}

func TestCompleteProjectTaskAttemptResultContractHumanReviewRoutesToWaitingHuman(t *testing.T) {
	repo := newProjectTaskResultMemoryRepository()
	service, err := NewService(repo)
	require.NoError(t, err)
	fixture := newProjectTaskAttemptServiceFixture(repo.memoryRepository, ProjectTaskStatusRunning, ProjectTaskAttemptStatusRunning)
	contract := validCompletedTaskResultContract()
	contract.HumanReviewRequest = &TaskResultHumanReviewRequest{
		Reason:     "需要负责人确认验收口径",
		Prompt:     "请确认是否接受该结果",
		Options:    []string{"accept", "request_revision"},
		RequiredBy: "human_owner",
		ReviewType: "acceptance",
	}

	summary, err := service.CompleteProjectTaskAttempt(context.Background(), CompleteProjectTaskAttemptRequest{
		ProjectTaskAttemptRuntimeRequest: fixture.runtimeRequest("attempt-complete-contract-human-review"),
		Conclusion:                       "legacy conclusion",
		RequiresHumanReview:              false,
		ResultContract:                   &contract,
	})

	require.NoError(t, err)
	require.Equal(t, fixture.taskID, summary.ProjectTaskID)
	require.Equal(t, ProjectTaskStatusWaitingHuman, repo.tasks[0].Status)
	require.NotNil(t, repo.tasks[0].WaitingRequestID)
	require.Equal(t, ProjectTaskAttemptStatusSucceeded, repo.projectTaskAttempts[0].Status)
	require.Len(t, repo.decisionRequests, 1)
	require.Equal(t, fixture.taskID, *repo.decisionRequests[0].ProjectTaskID)
	results, err := repo.ListProjectTaskResults(context.Background(), ListProjectTaskResultsRequest{
		TenantID:      fixture.tenantID,
		ProjectID:     fixture.projectID,
		ProjectTaskID: fixture.taskID,
		Limit:         10,
	})
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Equal(t, contract, results[0].Contract)
	require.Equal(t, TaskResultDecisionWaitingHumanReview, results[0].Decision)
	require.NotNil(t, repo.tasks[0].LatestTaskResultID)
	require.Equal(t, results[0].ID, *repo.tasks[0].LatestTaskResultID)
}

func TestCompleteProjectTaskAttemptInvalidResultContractDoesNotCommitTerminalWriteback(t *testing.T) {
	repo := newProjectTaskResultMemoryRepository()
	service, err := NewService(repo)
	require.NoError(t, err)
	fixture := newProjectTaskAttemptServiceFixture(repo.memoryRepository, ProjectTaskStatusRunning, ProjectTaskAttemptStatusRunning)
	contract := validCompletedTaskResultContract()
	contract.Summary = ""
	initialTask := repo.tasks[0]
	initialAttempt := repo.projectTaskAttempts[0]
	initialSummaryCount := len(repo.executionSummaries)
	initialEventCount := len(repo.events)
	initialLedgerCount := len(repo.executionLedgerEvents)

	_, err = service.CompleteProjectTaskAttempt(context.Background(), CompleteProjectTaskAttemptRequest{
		ProjectTaskAttemptRuntimeRequest: fixture.runtimeRequest("attempt-complete-invalid-result"),
		Conclusion:                       "legacy conclusion",
		ResultContract:                   &contract,
	})

	require.ErrorIs(t, err, ErrInvalidProjectEvidence)
	require.Equal(t, initialTask.Status, repo.tasks[0].Status)
	require.Equal(t, initialTask.CurrentAttemptID, repo.tasks[0].CurrentAttemptID)
	require.Nil(t, repo.tasks[0].LatestTaskResultID)
	require.Equal(t, initialAttempt.Status, repo.projectTaskAttempts[0].Status)
	require.Len(t, repo.executionSummaries, initialSummaryCount)
	require.Len(t, repo.events, initialEventCount)
	require.Len(t, repo.executionLedgerEvents, initialLedgerCount)
	require.Empty(t, repo.projectTaskResults)
}

func TestCompleteProjectTaskAttemptResultLinkFailureRollsBackTerminalWriteback(t *testing.T) {
	repo := newProjectTaskResultMemoryRepository()
	repo.linkProjectTaskLatestResultErr = ErrProjectConflict
	service, err := NewService(repo)
	require.NoError(t, err)
	fixture := newProjectTaskAttemptServiceFixture(repo.memoryRepository, ProjectTaskStatusRunning, ProjectTaskAttemptStatusRunning)
	contract := validCompletedTaskResultContract()
	initialTask := repo.tasks[0]
	initialAttempt := repo.projectTaskAttempts[0]
	initialSummaryCount := len(repo.executionSummaries)
	initialEventCount := len(repo.events)
	initialLedgerCount := len(repo.executionLedgerEvents)

	_, err = service.CompleteProjectTaskAttempt(context.Background(), CompleteProjectTaskAttemptRequest{
		ProjectTaskAttemptRuntimeRequest: fixture.runtimeRequest("attempt-complete-link-fails"),
		Conclusion:                       "legacy conclusion",
		ResultContract:                   &contract,
	})

	require.ErrorIs(t, err, ErrProjectConflict)
	require.Equal(t, initialTask.Status, repo.tasks[0].Status)
	require.Equal(t, initialTask.CurrentAttemptID, repo.tasks[0].CurrentAttemptID)
	require.Nil(t, repo.tasks[0].LatestTaskResultID)
	require.Equal(t, initialAttempt.Status, repo.projectTaskAttempts[0].Status)
	require.Len(t, repo.executionSummaries, initialSummaryCount)
	require.Len(t, repo.events, initialEventCount)
	require.Len(t, repo.executionLedgerEvents, initialLedgerCount)
	require.Empty(t, repo.projectTaskResults)
}

func TestCompleteProjectTaskAttemptAcceptanceResultLinkFailureRollsBackTerminalWriteback(t *testing.T) {
	repo := newProjectTaskResultMemoryRepository()
	repo.linkProjectTaskLatestResultErr = ErrProjectConflict
	service, err := NewService(repo)
	require.NoError(t, err)
	fixture := newProjectTaskAttemptServiceFixture(repo.memoryRepository, ProjectTaskStatusRunning, ProjectTaskAttemptStatusRunning)
	repo.tasks[0].RequiresHumanApproval = true
	contract := validCompletedTaskResultContract()
	initialTask := repo.tasks[0]
	initialAttempt := repo.projectTaskAttempts[0]
	initialSummaryCount := len(repo.executionSummaries)
	initialEventCount := len(repo.events)
	initialLedgerCount := len(repo.executionLedgerEvents)
	initialDecisionCount := len(repo.decisionRequests)

	_, err = service.CompleteProjectTaskAttempt(context.Background(), CompleteProjectTaskAttemptRequest{
		ProjectTaskAttemptRuntimeRequest: fixture.runtimeRequest("attempt-acceptance-link-fails"),
		Conclusion:                       "legacy conclusion",
		ResultContract:                   &contract,
	})

	require.ErrorIs(t, err, ErrProjectConflict)
	require.Equal(t, initialTask.Status, repo.tasks[0].Status)
	require.Equal(t, initialTask.CurrentAttemptID, repo.tasks[0].CurrentAttemptID)
	require.Nil(t, repo.tasks[0].LatestTaskResultID)
	require.Equal(t, initialAttempt.Status, repo.projectTaskAttempts[0].Status)
	require.Len(t, repo.executionSummaries, initialSummaryCount)
	require.Len(t, repo.events, initialEventCount)
	require.Len(t, repo.executionLedgerEvents, initialLedgerCount)
	require.Len(t, repo.decisionRequests, initialDecisionCount)
	require.Empty(t, repo.projectTaskResults)
}

func TestSubmitProjectTaskAttemptResultUsesRealServiceAndStoresContract(t *testing.T) {
	repo := newProjectTaskResultMemoryRepository()
	service, err := NewService(repo)
	require.NoError(t, err)
	fixture := newProjectTaskAttemptServiceFixture(repo.memoryRepository, ProjectTaskStatusRunning, ProjectTaskAttemptStatusRunning)
	contract := validCompletedTaskResultContract()
	contract.Summary = "结构化结果"

	summary, err := service.SubmitProjectTaskAttemptResult(context.Background(), SubmitProjectTaskAttemptResultRequest{
		ProjectTaskAttemptRuntimeRequest: fixture.runtimeRequest("attempt-result-real-service"),
		ResultContract:                   contract,
	})

	require.NoError(t, err)
	require.Equal(t, "结构化结果", summary.Conclusion)
	results, err := repo.ListProjectTaskResults(context.Background(), ListProjectTaskResultsRequest{
		TenantID:      fixture.tenantID,
		ProjectID:     fixture.projectID,
		ProjectTaskID: fixture.taskID,
		Limit:         10,
	})
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Equal(t, contract, results[0].Contract)
}

func validCompletedTaskResultContract() TaskResultContract {
	return TaskResultContract{
		Status:  TaskResultStatusCompleted,
		Summary: "完成分析",
		AcceptanceResults: []TaskResultAcceptanceResult{
			{
				Criterion:    "输出结论",
				Status:       TaskResultCriterionStatusPassed,
				EvidenceRefs: []string{"artifact:report"},
			},
		},
		EvidenceRefs: []TaskResultRef{{Type: "report", Ref: "artifact:report"}},
		ArtifactRefs: []TaskResultRef{{Type: "markdown", Ref: "artifact:analysis-report"}},
		Verification: []TaskResultVerification{{Type: "command", Status: TaskResultVerificationStatusPassed, Summary: "命令通过"}},
	}
}

func TestCompleteProjectTaskAttemptWritesLedgerEvents(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	require.NoError(t, err)
	fixture := newProjectTaskAttemptServiceFixture(repo, ProjectTaskStatusRunning, ProjectTaskAttemptStatusRunning)
	_, err = service.CompleteProjectTaskAttempt(context.Background(), CompleteProjectTaskAttemptRequest{
		ProjectTaskAttemptRuntimeRequest: fixture.runtimeRequest("complete-ledger"),
		Conclusion:                       "验收证据已生成",
		EvidenceRefs:                     []any{map[string]any{"ref": "evidence://complete"}},
		ArtifactRefs:                     []any{map[string]any{"ref": "artifact://complete"}},
		ConfidenceFactors:                map[string]any{"verified": true},
		MissingInformation:               []any{},
		RecommendedNextAction:            "进入验收",
	})
	require.NoError(t, err)
	requireLedgerEventTypes(t, repo.executionLedgerEvents, ExecutionLedgerEventAttemptCompleted, ExecutionLedgerEventSummaryCreated)
}

func TestCompleteProjectTaskAttemptAcceptanceBeforeCompletedWritesLedgerEvents(t *testing.T) {
	repo := newMemoryRepository()
	inbox := &fakeDecisionInboxProjector{}
	service, err := NewServiceWithCoordinatorApprovalsInboxAndArchiveArtifactLocker(repo, nil, nil, inbox, nil)
	require.NoError(t, err)
	fixture := newProjectTaskAttemptServiceFixture(repo, ProjectTaskStatusRunning, ProjectTaskAttemptStatusRunning)
	high := "high"
	repo.tasks[0].RiskLevel = &high

	_, err = service.CompleteProjectTaskAttempt(context.Background(), CompleteProjectTaskAttemptRequest{
		ProjectTaskAttemptRuntimeRequest: fixture.runtimeRequest("complete-high-risk-1"),
		Conclusion:                       "候选结果已完成",
	})

	require.NoError(t, err)
	task := repo.tasks[0]
	require.Equal(t, ProjectTaskStatusWaitingHuman, task.Status)
	require.NotNil(t, task.WaitingReason)
	require.Equal(t, HumanWaitReasonAcceptanceRequired, *task.WaitingReason)
	require.Equal(t, ProjectTaskAttemptStatusSucceeded, repo.projectTaskAttempts[0].Status)
	require.Len(t, repo.decisionRequests, 1)
	require.Equal(t, "project_task_acceptance", repo.decisionRequests[0].DecisionType)
	require.Len(t, inbox.upserts, 1)
	require.Equal(t, repo.decisionRequests[0].ID, inbox.upserts[0].ID)
	require.Equal(t, repo.decisionRequests[0].ProjectTaskID, inbox.upserts[0].ProjectTaskID)
	require.Len(t, repo.executionLedgerEvents, 2)
	completedEvent := repo.executionLedgerEvents[0]
	summaryEvent := repo.executionLedgerEvents[1]
	require.Equal(t, ExecutionLedgerEventAttemptCompleted, completedEvent.EventType)
	require.Equal(t, fixture.attemptID.String(), completedEvent.SourceID)
	require.Equal(t, "project_task_attempt:"+fixture.attemptID.String()+":attempt.completed", completedEvent.IdempotencyKey)
	require.Equal(t, true, completedEvent.Metadata["requires_human_review"])
	require.Equal(t, ExecutionLedgerEventSummaryCreated, summaryEvent.EventType)
	require.Equal(t, repo.executionSummaries[0].ID.String(), summaryEvent.SourceID)
	require.Equal(t, "project_execution_summary:"+repo.executionSummaries[0].ID.String()+":summary.created", summaryEvent.IdempotencyKey)
	require.Equal(t, true, summaryEvent.Metadata["requires_human_review"])
}

func TestResolveProjectTaskHumanWaitAcceptanceApprovedCompletesTask(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	require.NoError(t, err)
	fixture := newProjectTaskAttemptServiceFixture(repo, ProjectTaskStatusWaitingHuman, ProjectTaskAttemptStatusSucceeded)
	reason := HumanWaitReasonAcceptanceRequired
	repo.tasks[0].WaitingReason = &reason
	waitingRequestID := uuid.New()
	repo.tasks[0].WaitingRequestID = &waitingRequestID

	task, err := service.ResolveProjectTaskHumanWait(context.Background(), ResolveProjectTaskHumanWaitRequest{
		TenantID:        fixture.tenantID,
		ProjectID:       fixture.projectID,
		ProjectTaskID:   fixture.taskID,
		ActorUserID:     repo.projects[fixture.projectID].HumanOwnerUserID,
		Resolution:      HumanWaitResolutionApprove,
		ResponseSummary: "验收通过",
	})

	require.NoError(t, err)
	require.Equal(t, ProjectTaskStatusCompleted, task.Status)
}

func TestResolveProjectTaskHumanWaitResumeSameTaskCreatesQueuedAttempt(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	require.NoError(t, err)
	fixture := newProjectTaskAttemptServiceFixture(repo, ProjectTaskStatusWaitingHuman, ProjectTaskAttemptStatusWaitingHuman)
	reason := HumanWaitReasonMissingContext
	repo.tasks[0].WaitingReason = &reason
	repo.tasks[0].AttemptCount = 1

	task, err := service.ResolveProjectTaskHumanWait(context.Background(), ResolveProjectTaskHumanWaitRequest{
		TenantID:        fixture.tenantID,
		ProjectID:       fixture.projectID,
		ProjectTaskID:   fixture.taskID,
		ActorUserID:     repo.projects[fixture.projectID].HumanOwnerUserID,
		Resolution:      HumanWaitResolutionResumeSameTask,
		ResponseSummary: "已补充上下文",
		ContextRefs:     []any{"customer_scope"},
	})

	require.NoError(t, err)
	require.Equal(t, ProjectTaskStatusQueued, task.Status)
	require.NotEqual(t, fixture.attemptID, *task.CurrentAttemptID)
	require.Equal(t, int32(2), task.AttemptCount)
}

func TestResolveProjectTaskHumanWaitMarkFailedFailsTask(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	require.NoError(t, err)
	fixture := newProjectTaskAttemptServiceFixture(repo, ProjectTaskStatusWaitingHuman, ProjectTaskAttemptStatusWaitingHuman)
	reason := HumanWaitReasonClarification
	repo.tasks[0].WaitingReason = &reason

	task, err := service.ResolveProjectTaskHumanWait(context.Background(), ResolveProjectTaskHumanWaitRequest{
		TenantID:        fixture.tenantID,
		ProjectID:       fixture.projectID,
		ProjectTaskID:   fixture.taskID,
		ActorUserID:     repo.projects[fixture.projectID].HumanOwnerUserID,
		Resolution:      HumanWaitResolutionMarkFailed,
		ResponseSummary: "无法继续",
	})

	require.NoError(t, err)
	require.Equal(t, ProjectTaskStatusFailed, task.Status)
}

func TestFailProjectTaskAttemptFailsTaskAndAttempt(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	require.NoError(t, err)
	fixture := newProjectTaskAttemptServiceFixture(repo, ProjectTaskStatusRunning, ProjectTaskAttemptStatusRunning)
	retryable := true

	task, err := service.FailProjectTaskAttempt(context.Background(), FailProjectTaskAttemptRequest{
		ProjectTaskAttemptRuntimeRequest: fixture.runtimeRequest("attempt-fail-1"),
		FailureSummary:                   "provider crashed",
		FailureFamily:                    "runtime_agent_failure",
		Retryable:                        &retryable,
	})

	require.NoError(t, err)
	require.Equal(t, ProjectTaskStatusFailed, task.Status)
	require.Equal(t, ProjectTaskAttemptStatusFailed, repo.projectTaskAttempts[0].Status)
	require.Equal(t, "runtime_agent_failure", *repo.projectTaskAttempts[0].FailureFamily)
	require.Equal(t, "provider crashed", *repo.projectTaskAttempts[0].FailureMessage)
	require.Equal(t, ProjectEventTaskFailed, repo.eventTypes[len(repo.eventTypes)-1])
}

func TestFailProjectTaskAttemptWritesLedgerEvent(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	require.NoError(t, err)
	fixture := newProjectTaskAttemptServiceFixture(repo, ProjectTaskStatusRunning, ProjectTaskAttemptStatusRunning)
	retryable := true

	_, err = service.FailProjectTaskAttempt(context.Background(), FailProjectTaskAttemptRequest{
		ProjectTaskAttemptRuntimeRequest: fixture.runtimeRequest("fail-ledger"),
		FailureSummary:                   "provider crashed",
		FailureFamily:                    "runtime_agent_failure",
		Retryable:                        &retryable,
	})

	require.NoError(t, err)
	require.Len(t, repo.executionLedgerEvents, 1)
	event := repo.executionLedgerEvents[0]
	require.Equal(t, ExecutionLedgerEventAttemptFailed, event.EventType)
	require.Equal(t, fixture.attemptID, *event.ProjectTaskAttemptID)
	require.Equal(t, "runtime_agent_failure", *event.ErrorFamily)
	require.Equal(t, "provider crashed", *event.ErrorMessage)
	require.NotNil(t, event.Retryable)
	require.True(t, *event.Retryable)
}

func TestFailProjectTaskAttemptTransientRuntimeSchedulesRetry(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	require.NoError(t, err)
	fixture := newProjectTaskAttemptServiceFixture(repo, ProjectTaskStatusRunning, ProjectTaskAttemptStatusRunning)
	maxAttempts := int32(3)
	repo.tasks[0].AttemptCount = 1
	repo.tasks[0].MaxAttempts = &maxAttempts
	retryable := true

	task, err := service.FailProjectTaskAttempt(context.Background(), FailProjectTaskAttemptRequest{
		ProjectTaskAttemptRuntimeRequest: fixture.runtimeRequest("fail-transient-1"),
		FailureSummary:                   "runtime node restarted",
		FailureFamily:                    FailureFamilyTransientRuntime,
		Retryable:                        &retryable,
	})

	require.NoError(t, err)
	require.Equal(t, ProjectTaskStatusQueued, task.Status)
	require.NotNil(t, task.CurrentAttemptID)
	require.NotEqual(t, fixture.attemptID, *task.CurrentAttemptID)
	require.Equal(t, int32(2), task.AttemptCount)
	require.Len(t, repo.executionLedgerEvents, 1)
	require.Equal(t, (*task.CurrentAttemptID).String(), repo.executionLedgerEvents[0].Metadata["retry_project_task_attempt_id"])
}

func TestFailProjectTaskAttemptRetryExhaustionMovesToWaitingHuman(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	require.NoError(t, err)
	fixture := newProjectTaskAttemptServiceFixture(repo, ProjectTaskStatusRunning, ProjectTaskAttemptStatusRunning)
	maxAttempts := int32(3)
	repo.tasks[0].AttemptCount = 3
	repo.tasks[0].MaxAttempts = &maxAttempts
	retryable := true

	task, err := service.FailProjectTaskAttempt(context.Background(), FailProjectTaskAttemptRequest{
		ProjectTaskAttemptRuntimeRequest: fixture.runtimeRequest("fail-exhausted-1"),
		FailureSummary:                   "provider timed out repeatedly",
		FailureFamily:                    FailureFamilyTimeout,
		Retryable:                        &retryable,
	})

	require.NoError(t, err)
	require.Equal(t, ProjectTaskStatusWaitingHuman, task.Status)
	require.NotNil(t, task.WaitingReason)
	require.Equal(t, HumanWaitReasonClarification, *task.WaitingReason)
	require.Equal(t, fixture.attemptID, *task.CurrentAttemptID)
	require.Equal(t, int32(3), task.AttemptCount)
	require.Equal(t, ProjectTaskAttemptStatusTimedOut, repo.projectTaskAttempts[0].Status)
	require.Len(t, repo.executionLedgerEvents, 1)
	require.NotContains(t, repo.executionLedgerEvents[0].Metadata, "retry_project_task_attempt_id")
}

func TestFailProjectTaskAttemptNonRetryableExecutionFailsTask(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	require.NoError(t, err)
	fixture := newProjectTaskAttemptServiceFixture(repo, ProjectTaskStatusRunning, ProjectTaskAttemptStatusRunning)
	retryable := false

	task, err := service.FailProjectTaskAttempt(context.Background(), FailProjectTaskAttemptRequest{
		ProjectTaskAttemptRuntimeRequest: fixture.runtimeRequest("fail-non-retryable-1"),
		FailureSummary:                   "output contract cannot be parsed",
		FailureFamily:                    FailureFamilyNonRetryableExecution,
		Retryable:                        &retryable,
	})

	require.NoError(t, err)
	require.Equal(t, ProjectTaskStatusFailed, task.Status)
	require.Equal(t, ProjectTaskAttemptStatusFailed, repo.projectTaskAttempts[0].Status)
	require.Equal(t, "output contract cannot be parsed", *repo.projectTaskAttempts[0].FailureMessage)
}

func TestWaitHumanProjectTaskAttemptMovesTaskAndCreatesDecisionRequest(t *testing.T) {
	repo := newMemoryRepository()
	inbox := &fakeDecisionInboxProjector{}
	service, err := NewServiceWithCoordinatorApprovalsInboxAndArchiveArtifactLocker(repo, nil, nil, inbox, nil)
	require.NoError(t, err)
	fixture := newProjectTaskAttemptServiceFixture(repo, ProjectTaskStatusRunning, ProjectTaskAttemptStatusRunning)
	employeeID := *repo.tasks[0].AssignedDigitalEmployeeID

	task, err := service.WaitHumanProjectTaskAttempt(context.Background(), WaitHumanProjectTaskAttemptRequest{
		ProjectTaskAttemptRuntimeRequest: fixture.runtimeRequest("attempt-wait-human-1"),
		DigitalEmployeeID:                employeeID,
		Reason:                           HumanWaitReasonMissingContext,
		Summary:                          "Need customer scope",
		MissingContextRefs:               []any{"customer_scope"},
		SuggestedResolutionOptions:       []string{HumanWaitResolutionResumeSameTask},
	})

	require.NoError(t, err)
	require.Equal(t, ProjectTaskStatusWaitingHuman, task.Status)
	require.NotNil(t, task.WaitingReason)
	require.Equal(t, HumanWaitReasonMissingContext, *task.WaitingReason)
	require.Equal(t, ProjectTaskAttemptStatusWaitingHuman, repo.projectTaskAttempts[0].Status)
	require.Len(t, repo.decisionRequests, 1)
	require.Equal(t, "project_task_missing_context", repo.decisionRequests[0].DecisionType)
	require.Len(t, inbox.upserts, 1)
	require.Equal(t, repo.decisionRequests[0].ID, inbox.upserts[0].ID)
	require.Equal(t, repo.decisionRequests[0].ProjectTaskID, inbox.upserts[0].ProjectTaskID)
	require.Equal(t, ProjectEventTaskWaitingHuman, repo.eventTypes[len(repo.eventTypes)-1])
}

func TestMemoryRepositoryRecordPreDispatchGateResultReturnsLinkedGateWithoutOverwrite(t *testing.T) {
	repo := newMemoryRepository()
	tenantID := uuid.New()
	projectID := uuid.New()
	employeeID := uuid.New()
	task, err := repo.CreateProjectTask(context.Background(), CreateProjectTaskRequest{
		TenantID:                  tenantID,
		ProjectID:                 projectID,
		Title:                     "linked gate replay",
		Status:                    ProjectTaskStatusPlanned,
		AssignedDigitalEmployeeID: &employeeID,
	})
	require.NoError(t, err)
	retryAfter := time.Date(2026, 6, 21, 13, 0, 0, 0, time.UTC)
	checkedAt := time.Date(2026, 6, 21, 12, 45, 0, 0, time.UTC)
	idempotencyKey := "gate:" + task.ID.String() + ":attempt:1:linked-replay"
	gate, err := repo.RecordPreDispatchGateResult(context.Background(), RecordPreDispatchGateResultRequest{
		TenantID:           tenantID,
		ProjectID:          projectID,
		ProjectTaskID:      task.ID,
		SelectedEmployeeID: employeeID,
		AttemptNo:          1,
		DispatchReason:     DispatchReasonRootReady,
		IdempotencyKey:     idempotencyKey,
		DispatchToken:      "dispatch-token-linked-replay",
		Status:             PreDispatchGateStatusRetryLater,
		CheckedAt:          checkedAt,
		Checks: []PreDispatchGateCheck{{
			Key:    "runtime.ready",
			Status: "failed",
		}},
		Blockers: []PreDispatchGateBlocker{{
			Key:       "runtime.slot_unavailable",
			Severity:  "transient",
			Retryable: true,
		}},
		HumanActionRequest: HumanActionRequest{
			"action": "wait_for_runtime_slot",
		},
		RetryAfter: &retryAfter,
	})
	require.NoError(t, err)
	queued, err := repo.QueueProjectTaskWithAttempt(context.Background(), QueueProjectTaskRequest{
		TenantID:             tenantID,
		ProjectID:            projectID,
		ProjectTaskID:        task.ID,
		DigitalEmployeeID:    employeeID,
		DigitalEmployeeRunID: ptrUUIDValue(uuid.New()),
		RuntimeTaskID:        ptrUUIDValue(uuid.New()),
		RuntimeNodeID:        ptrUUIDValue(uuid.New()),
		IdempotencyKey:       "project-task:" + task.ID.String() + ":attempt:1:linked-replay",
		LeaseToken:           "lease-token-linked-replay",
		DispatchGateResultID: &gate.ID,
	})
	require.NoError(t, err)

	replayed, err := repo.RecordPreDispatchGateResult(context.Background(), RecordPreDispatchGateResultRequest{
		TenantID:           tenantID,
		ProjectID:          projectID,
		ProjectTaskID:      task.ID,
		SelectedEmployeeID: employeeID,
		AttemptNo:          1,
		DispatchReason:     DispatchReasonRootReady,
		IdempotencyKey:     idempotencyKey,
		DispatchToken:      "dispatch-token-linked-replay",
		Status:             PreDispatchGateStatusPassed,
		CheckedAt:          checkedAt.Add(time.Minute),
		Checks: []PreDispatchGateCheck{{
			Key:    "runtime.ready",
			Status: "passed",
		}},
		Blockers: nil,
		HumanActionRequest: HumanActionRequest{
			"action": "dispatch_now",
		},
		RetryAfter: nil,
	})
	require.NoError(t, err)

	require.Equal(t, gate.ID, replayed.ID)
	require.Equal(t, PreDispatchGateStatusRetryLater, replayed.Status)
	require.NotNil(t, replayed.RetryAfter)
	require.Equal(t, retryAfter, *replayed.RetryAfter)
	require.Len(t, replayed.Blockers, 1)
	require.Equal(t, "failed", replayed.Checks[0].Status)
	require.Equal(t, HumanActionRequest{"action": "wait_for_runtime_slot"}, replayed.HumanActionRequest)
	require.NotNil(t, replayed.AttemptID)
	require.Equal(t, queued.Attempt.ID, *replayed.AttemptID)
}

func TestMemoryRepositoryRecordLinkedPreDispatchGateReplayDoesNotMoveLatest(t *testing.T) {
	repo := newMemoryRepository()
	tenantID := uuid.New()
	projectID := uuid.New()
	employeeID := uuid.New()
	task, err := repo.CreateProjectTask(context.Background(), CreateProjectTaskRequest{
		TenantID:                  tenantID,
		ProjectID:                 projectID,
		Title:                     "linked gate replay keeps latest",
		Status:                    ProjectTaskStatusPlanned,
		AssignedDigitalEmployeeID: &employeeID,
	})
	require.NoError(t, err)
	checkedAt := time.Date(2026, 6, 21, 15, 0, 0, 0, time.UTC)
	gateAKey := "gate:" + task.ID.String() + ":attempt:1:linked-latest-a"
	gateA, err := repo.RecordPreDispatchGateResult(context.Background(), RecordPreDispatchGateResultRequest{
		TenantID:           tenantID,
		ProjectID:          projectID,
		ProjectTaskID:      task.ID,
		SelectedEmployeeID: employeeID,
		AttemptNo:          1,
		DispatchReason:     DispatchReasonRootReady,
		IdempotencyKey:     gateAKey,
		DispatchToken:      "dispatch-token-linked-latest-a",
		Status:             PreDispatchGateStatusPassed,
		CheckedAt:          checkedAt,
		Checks: []PreDispatchGateCheck{{
			Key:    "runtime.ready",
			Status: "passed",
		}},
	})
	require.NoError(t, err)
	_, err = repo.QueueProjectTaskWithAttempt(context.Background(), QueueProjectTaskRequest{
		TenantID:             tenantID,
		ProjectID:            projectID,
		ProjectTaskID:        task.ID,
		DigitalEmployeeID:    employeeID,
		DigitalEmployeeRunID: ptrUUIDValue(uuid.New()),
		RuntimeTaskID:        ptrUUIDValue(uuid.New()),
		RuntimeNodeID:        ptrUUIDValue(uuid.New()),
		IdempotencyKey:       "project-task:" + task.ID.String() + ":attempt:1:linked-latest-a",
		LeaseToken:           "lease-token-linked-latest-a",
		DispatchGateResultID: &gateA.ID,
	})
	require.NoError(t, err)
	gateB, err := repo.RecordPreDispatchGateResult(context.Background(), RecordPreDispatchGateResultRequest{
		TenantID:           tenantID,
		ProjectID:          projectID,
		ProjectTaskID:      task.ID,
		SelectedEmployeeID: employeeID,
		AttemptNo:          2,
		DispatchReason:     DispatchReasonRootReady,
		IdempotencyKey:     "gate:" + task.ID.String() + ":attempt:2:latest-b",
		DispatchToken:      "dispatch-token-latest-b",
		Status:             PreDispatchGateStatusRetryLater,
		CheckedAt:          checkedAt.Add(time.Minute),
		Checks: []PreDispatchGateCheck{{
			Key:    "runtime.ready",
			Status: "failed",
		}},
	})
	require.NoError(t, err)
	latestBeforeReplay, err := repo.GetProjectTask(context.Background(), tenantID, task.ID)
	require.NoError(t, err)
	require.NotNil(t, latestBeforeReplay.LatestDispatchGateResultID)
	require.Equal(t, gateB.ID, *latestBeforeReplay.LatestDispatchGateResultID)

	replayed, err := repo.RecordPreDispatchGateResult(context.Background(), RecordPreDispatchGateResultRequest{
		TenantID:           tenantID,
		ProjectID:          projectID,
		ProjectTaskID:      task.ID,
		SelectedEmployeeID: employeeID,
		AttemptNo:          1,
		DispatchReason:     DispatchReasonRootReady,
		IdempotencyKey:     gateAKey,
		DispatchToken:      "dispatch-token-linked-latest-a",
		Status:             PreDispatchGateStatusPassed,
		CheckedAt:          checkedAt.Add(2 * time.Minute),
		Checks: []PreDispatchGateCheck{{
			Key:    "runtime.ready",
			Status: "passed",
		}},
	})
	require.NoError(t, err)
	require.Equal(t, gateA.ID, replayed.ID)

	latestAfterReplay, err := repo.GetProjectTask(context.Background(), tenantID, task.ID)
	require.NoError(t, err)
	require.NotNil(t, latestAfterReplay.LatestDispatchGateResultID)
	require.Equal(t, gateB.ID, *latestAfterReplay.LatestDispatchGateResultID)
}

func TestMemoryRepositoryRecordPreDispatchGateResultWithInvalidProjectIsAtomic(t *testing.T) {
	repo := newMemoryRepository()
	tenantID := uuid.New()
	projectID := uuid.New()
	wrongProjectID := uuid.New()
	employeeID := uuid.New()
	task, err := repo.CreateProjectTask(context.Background(), CreateProjectTaskRequest{
		TenantID:                  tenantID,
		ProjectID:                 projectID,
		Title:                     "invalid project gate replay",
		Status:                    ProjectTaskStatusPlanned,
		AssignedDigitalEmployeeID: &employeeID,
	})
	require.NoError(t, err)
	gate, err := repo.RecordPreDispatchGateResult(context.Background(), RecordPreDispatchGateResultRequest{
		TenantID:           tenantID,
		ProjectID:          projectID,
		ProjectTaskID:      task.ID,
		SelectedEmployeeID: employeeID,
		AttemptNo:          1,
		DispatchReason:     DispatchReasonRootReady,
		IdempotencyKey:     "gate:" + task.ID.String() + ":attempt:1:invalid-project",
		DispatchToken:      "dispatch-token-invalid-project",
		Status:             PreDispatchGateStatusRetryLater,
		CheckedAt:          time.Date(2026, 6, 21, 14, 0, 0, 0, time.UTC),
		Checks: []PreDispatchGateCheck{{
			Key:    "runtime.ready",
			Status: "failed",
		}},
		Blockers: []PreDispatchGateBlocker{{
			Key:       "runtime.slot_unavailable",
			Severity:  "transient",
			Retryable: true,
		}},
		HumanActionRequest: HumanActionRequest{
			"action": "wait_for_runtime_slot",
		},
	})
	require.NoError(t, err)
	originalTask := repo.tasks[0]
	originalGate := repo.dispatchGateResults[0]

	_, err = repo.RecordPreDispatchGateResult(context.Background(), RecordPreDispatchGateResultRequest{
		TenantID:           tenantID,
		ProjectID:          wrongProjectID,
		ProjectTaskID:      task.ID,
		SelectedEmployeeID: employeeID,
		AttemptNo:          1,
		DispatchReason:     DispatchReasonRootReady,
		IdempotencyKey:     gate.IdempotencyKey,
		DispatchToken:      "dispatch-token-invalid-project-replay",
		Status:             PreDispatchGateStatusPassed,
		CheckedAt:          time.Date(2026, 6, 21, 14, 5, 0, 0, time.UTC),
		Checks: []PreDispatchGateCheck{{
			Key:    "runtime.ready",
			Status: "passed",
		}},
		HumanActionRequest: HumanActionRequest{
			"action": "dispatch_now",
		},
	})
	require.ErrorIs(t, err, ErrProjectNotFound)
	require.Len(t, repo.dispatchGateResults, 1)
	require.Equal(t, originalGate, repo.dispatchGateResults[0])
	require.Equal(t, originalTask, repo.tasks[0])

	_, err = repo.RecordPreDispatchGateResult(context.Background(), RecordPreDispatchGateResultRequest{
		TenantID:           tenantID,
		ProjectID:          wrongProjectID,
		ProjectTaskID:      task.ID,
		SelectedEmployeeID: employeeID,
		AttemptNo:          2,
		DispatchReason:     DispatchReasonRetry,
		IdempotencyKey:     "gate:" + task.ID.String() + ":attempt:2:invalid-project",
		DispatchToken:      "dispatch-token-invalid-project-new",
		Status:             PreDispatchGateStatusRetryLater,
		CheckedAt:          time.Date(2026, 6, 21, 14, 10, 0, 0, time.UTC),
	})
	require.ErrorIs(t, err, ErrProjectNotFound)
	require.Len(t, repo.dispatchGateResults, 1)
	require.Equal(t, originalGate, repo.dispatchGateResults[0])
	require.Equal(t, originalTask, repo.tasks[0])
}

func TestMemoryRepositoryGetPreDispatchGateResultByKeyScopesProject(t *testing.T) {
	repo := newMemoryRepository()
	tenantID := uuid.New()
	projectID := uuid.New()
	wrongProjectID := uuid.New()
	employeeID := uuid.New()
	task, err := repo.CreateProjectTask(context.Background(), CreateProjectTaskRequest{
		TenantID:                  tenantID,
		ProjectID:                 projectID,
		Title:                     "gate lookup project scope",
		Status:                    ProjectTaskStatusPlanned,
		AssignedDigitalEmployeeID: &employeeID,
	})
	require.NoError(t, err)
	idempotencyKey := "gate:" + task.ID.String() + ":attempt:1:project-scope"
	gate, err := repo.RecordPreDispatchGateResult(context.Background(), RecordPreDispatchGateResultRequest{
		TenantID:           tenantID,
		ProjectID:          projectID,
		ProjectTaskID:      task.ID,
		SelectedEmployeeID: employeeID,
		AttemptNo:          1,
		DispatchReason:     DispatchReasonRootReady,
		IdempotencyKey:     idempotencyKey,
		DispatchToken:      "dispatch-token-project-scope",
		Status:             PreDispatchGateStatusRetryLater,
		CheckedAt:          time.Date(2026, 6, 21, 14, 20, 0, 0, time.UTC),
		Checks: []PreDispatchGateCheck{{
			Key:    "runtime.ready",
			Status: "failed",
		}},
	})
	require.NoError(t, err)

	found, err := repo.GetPreDispatchGateResultByKey(context.Background(), tenantID, projectID, task.ID, idempotencyKey)
	require.NoError(t, err)
	require.Equal(t, gate.ID, found.ID)

	_, err = repo.GetPreDispatchGateResultByKey(context.Background(), tenantID, wrongProjectID, task.ID, idempotencyKey)
	require.ErrorIs(t, err, ErrProjectNotFound)
}

func TestMemoryRepositoryLinkPreDispatchGateAttemptRejectsWrongTaskAndUpdatesAttempt(t *testing.T) {
	repo := newMemoryRepository()
	tenantID := uuid.New()
	projectID := uuid.New()
	employeeID := uuid.New()
	taskA, err := repo.CreateProjectTask(context.Background(), CreateProjectTaskRequest{
		TenantID:                  tenantID,
		ProjectID:                 projectID,
		Title:                     "gate task A",
		Status:                    ProjectTaskStatusPlanned,
		AssignedDigitalEmployeeID: &employeeID,
	})
	require.NoError(t, err)
	taskB, err := repo.CreateProjectTask(context.Background(), CreateProjectTaskRequest{
		TenantID:                  tenantID,
		ProjectID:                 projectID,
		Title:                     "gate task B",
		Status:                    ProjectTaskStatusPlanned,
		AssignedDigitalEmployeeID: &employeeID,
	})
	require.NoError(t, err)
	gate, err := repo.RecordPreDispatchGateResult(context.Background(), RecordPreDispatchGateResultRequest{
		TenantID:           tenantID,
		ProjectID:          projectID,
		ProjectTaskID:      taskA.ID,
		SelectedEmployeeID: employeeID,
		AttemptNo:          1,
		DispatchReason:     DispatchReasonRootReady,
		IdempotencyKey:     "gate:" + taskA.ID.String() + ":attempt:1:wrong-attempt",
		DispatchToken:      "dispatch-token-wrong-attempt",
		Status:             PreDispatchGateStatusPassed,
		CheckedAt:          time.Date(2026, 6, 21, 13, 15, 0, 0, time.UTC),
		Checks: []PreDispatchGateCheck{{
			Key:    "runtime.ready",
			Status: "passed",
		}},
	})
	require.NoError(t, err)
	wrongAttempt, err := repo.QueueProjectTaskWithAttempt(context.Background(), QueueProjectTaskRequest{
		TenantID:             tenantID,
		ProjectID:            projectID,
		ProjectTaskID:        taskB.ID,
		DigitalEmployeeID:    employeeID,
		DigitalEmployeeRunID: ptrUUIDValue(uuid.New()),
		RuntimeTaskID:        ptrUUIDValue(uuid.New()),
		RuntimeNodeID:        ptrUUIDValue(uuid.New()),
		IdempotencyKey:       "project-task:" + taskB.ID.String() + ":attempt:1:wrong-attempt",
		LeaseToken:           "lease-token-wrong-attempt",
	})
	require.NoError(t, err)

	_, err = repo.LinkPreDispatchGateAttempt(context.Background(), LinkPreDispatchGateAttemptRequest{
		TenantID:      tenantID,
		ProjectID:     projectID,
		ProjectTaskID: taskA.ID,
		GateResultID:  gate.ID,
		AttemptID:     wrongAttempt.Attempt.ID,
	})
	require.ErrorIs(t, err, ErrProjectNotFound)
	require.Nil(t, repo.dispatchGateResults[0].AttemptID)
	require.Nil(t, repo.projectTaskAttempts[0].DispatchGateResultID)

	correctAttempt, err := repo.QueueProjectTaskWithAttempt(context.Background(), QueueProjectTaskRequest{
		TenantID:             tenantID,
		ProjectID:            projectID,
		ProjectTaskID:        taskA.ID,
		DigitalEmployeeID:    employeeID,
		DigitalEmployeeRunID: ptrUUIDValue(uuid.New()),
		RuntimeTaskID:        ptrUUIDValue(uuid.New()),
		RuntimeNodeID:        ptrUUIDValue(uuid.New()),
		IdempotencyKey:       "project-task:" + taskA.ID.String() + ":attempt:1:correct-attempt",
		LeaseToken:           "lease-token-correct-attempt",
	})
	require.NoError(t, err)

	linked, err := repo.LinkPreDispatchGateAttempt(context.Background(), LinkPreDispatchGateAttemptRequest{
		TenantID:      tenantID,
		ProjectID:     projectID,
		ProjectTaskID: taskA.ID,
		GateResultID:  gate.ID,
		AttemptID:     correctAttempt.Attempt.ID,
	})
	require.NoError(t, err)
	require.NotNil(t, linked.AttemptID)
	require.Equal(t, correctAttempt.Attempt.ID, *linked.AttemptID)
	linkedAttempt, err := repo.GetProjectTaskAttempt(context.Background(), tenantID, correctAttempt.Attempt.ID)
	require.NoError(t, err)
	require.NotNil(t, linkedAttempt.DispatchGateResultID)
	require.Equal(t, gate.ID, *linkedAttempt.DispatchGateResultID)
}

func TestQueueProjectTaskWithInvalidDispatchGateResultIsAtomic(t *testing.T) {
	repo := newMemoryRepository()
	tenantID := uuid.New()
	projectID := uuid.New()
	employeeID := uuid.New()
	task, err := repo.CreateProjectTask(context.Background(), CreateProjectTaskRequest{
		TenantID:                  tenantID,
		ProjectID:                 projectID,
		Title:                     "invalid gate queue",
		Status:                    ProjectTaskStatusPlanned,
		AssignedDigitalEmployeeID: &employeeID,
	})
	require.NoError(t, err)
	gate, err := repo.RecordPreDispatchGateResult(context.Background(), RecordPreDispatchGateResultRequest{
		TenantID:           tenantID,
		ProjectID:          projectID,
		ProjectTaskID:      task.ID,
		SelectedEmployeeID: employeeID,
		AttemptNo:          1,
		DispatchReason:     DispatchReasonRootReady,
		IdempotencyKey:     "gate:" + task.ID.String() + ":attempt:1:valid",
		DispatchToken:      "dispatch-token-valid",
		Status:             PreDispatchGateStatusPassed,
		CheckedAt:          time.Date(2026, 6, 21, 14, 30, 0, 0, time.UTC),
		Checks: []PreDispatchGateCheck{{
			Key:    "runtime.ready",
			Status: "passed",
		}},
	})
	require.NoError(t, err)
	originalTask := repo.tasks[0]
	originalGate := repo.dispatchGateResults[0]
	missingGateID := uuid.New()

	_, err = repo.QueueProjectTaskWithAttempt(context.Background(), QueueProjectTaskRequest{
		TenantID:             tenantID,
		ProjectID:            projectID,
		ProjectTaskID:        task.ID,
		DigitalEmployeeID:    employeeID,
		DigitalEmployeeRunID: ptrUUIDValue(uuid.New()),
		RuntimeTaskID:        ptrUUIDValue(uuid.New()),
		RuntimeNodeID:        ptrUUIDValue(uuid.New()),
		IdempotencyKey:       "project-task:" + task.ID.String() + ":attempt:1:missing-gate",
		LeaseToken:           "lease-token-missing-gate",
		DispatchGateResultID: &missingGateID,
	})
	require.ErrorIs(t, err, ErrProjectNotFound)
	require.Empty(t, repo.events)
	require.Empty(t, repo.projectTaskAttempts)
	require.Len(t, repo.dispatchGateResults, 1)
	require.Equal(t, originalGate, repo.dispatchGateResults[0])
	require.Equal(t, gate.ID, *repo.tasks[0].LatestDispatchGateResultID)
	require.Equal(t, originalTask, repo.tasks[0])
}

func TestMemoryRepositoryMoveProjectTaskToWaitingHumanForPreDispatchGateRequiresExistingGate(t *testing.T) {
	repo := newMemoryRepository()
	tenantID := uuid.New()
	projectID := uuid.New()
	employeeID := uuid.New()
	task, err := repo.CreateProjectTask(context.Background(), CreateProjectTaskRequest{
		TenantID:                  tenantID,
		ProjectID:                 projectID,
		Title:                     "missing gate wait human",
		Status:                    ProjectTaskStatusPlanned,
		AssignedDigitalEmployeeID: &employeeID,
	})
	require.NoError(t, err)
	originalTask := repo.tasks[0]

	_, err = repo.MoveProjectTaskToWaitingHumanForPreDispatchGate(context.Background(), MoveProjectTaskToWaitingHumanForPreDispatchGateRequest{
		TenantID:      tenantID,
		ProjectID:     projectID,
		ProjectTaskID: task.ID,
		GateResultID:  uuid.New(),
		WaitingReason: HumanWaitReasonClarification,
	})
	require.ErrorIs(t, err, ErrProjectNotFound)
	require.Equal(t, originalTask, repo.tasks[0])
}

func TestMemoryRepositoryLinkPreDispatchGateDecisionRequestRejectsWrongTask(t *testing.T) {
	repo := newMemoryRepository()
	tenantID := uuid.New()
	projectID := uuid.New()
	employeeID := uuid.New()
	taskA, err := repo.CreateProjectTask(context.Background(), CreateProjectTaskRequest{
		TenantID:                  tenantID,
		ProjectID:                 projectID,
		Title:                     "gate decision task A",
		Status:                    ProjectTaskStatusPlanned,
		AssignedDigitalEmployeeID: &employeeID,
	})
	require.NoError(t, err)
	taskB, err := repo.CreateProjectTask(context.Background(), CreateProjectTaskRequest{
		TenantID:                  tenantID,
		ProjectID:                 projectID,
		Title:                     "gate decision task B",
		Status:                    ProjectTaskStatusPlanned,
		AssignedDigitalEmployeeID: &employeeID,
	})
	require.NoError(t, err)
	gate, err := repo.RecordPreDispatchGateResult(context.Background(), RecordPreDispatchGateResultRequest{
		TenantID:           tenantID,
		ProjectID:          projectID,
		ProjectTaskID:      taskA.ID,
		SelectedEmployeeID: employeeID,
		AttemptNo:          1,
		DispatchReason:     DispatchReasonRootReady,
		IdempotencyKey:     "gate:" + taskA.ID.String() + ":attempt:1:wrong-decision",
		DispatchToken:      "dispatch-token-wrong-decision",
		Status:             PreDispatchGateStatusWaitingHuman,
		CheckedAt:          time.Date(2026, 6, 21, 13, 30, 0, 0, time.UTC),
		Checks: []PreDispatchGateCheck{{
			Key:    "risk.approval",
			Status: "failed",
		}},
	})
	require.NoError(t, err)
	decision, err := repo.CreateDecisionRequest(context.Background(), CreateDecisionRequestRequest{
		TenantID:          tenantID,
		ProjectID:         projectID,
		ApprovalRequestID: uuid.New(),
		ProjectTaskID:     &taskB.ID,
		TargetUserID:      uuid.New(),
		DecisionType:      "pre_dispatch_gate_review",
		TitleSnapshot:     "Review task B",
		StatusSnapshot:    "pending",
	})
	require.NoError(t, err)

	_, err = repo.LinkPreDispatchGateDecisionRequest(context.Background(), LinkPreDispatchGateDecisionRequest{
		TenantID:          tenantID,
		ProjectID:         projectID,
		ProjectTaskID:     taskA.ID,
		GateResultID:      gate.ID,
		DecisionRequestID: decision.ID,
	})
	require.ErrorIs(t, err, ErrProjectNotFound)
	require.Nil(t, repo.dispatchGateResults[0].DecisionRequestID)
	require.Nil(t, repo.decisionRequests[0].DispatchGateResultID)
}

func TestWaitHumanProjectTaskAttemptWritesLedgerEvent(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	require.NoError(t, err)
	fixture := newProjectTaskAttemptServiceFixture(repo, ProjectTaskStatusRunning, ProjectTaskAttemptStatusRunning)
	employeeID := *repo.tasks[0].AssignedDigitalEmployeeID

	_, err = service.WaitHumanProjectTaskAttempt(context.Background(), WaitHumanProjectTaskAttemptRequest{
		ProjectTaskAttemptRuntimeRequest: fixture.runtimeRequest("wait-human-ledger"),
		DigitalEmployeeID:                employeeID,
		Reason:                           HumanWaitReasonMissingContext,
		Summary:                          "Need customer scope",
		MissingContextRefs:               []any{"customer_scope"},
		SuggestedResolutionOptions:       []string{HumanWaitResolutionResumeSameTask},
	})

	require.NoError(t, err)
	require.Len(t, repo.executionLedgerEvents, 1)
	event := repo.executionLedgerEvents[0]
	require.Equal(t, ExecutionLedgerEventAttemptWaitingHuman, event.EventType)
	require.Equal(t, fixture.attemptID, *event.ProjectTaskAttemptID)
	require.Equal(t, "Need customer scope", *event.OutputSummary)
}

func TestProjectTaskAttemptRejectsWrongRuntimeNode(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	require.NoError(t, err)
	fixture := newProjectTaskAttemptServiceFixture(repo, ProjectTaskStatusRunning, ProjectTaskAttemptStatusRunning)
	req := fixture.runtimeRequest("attempt-lease-1")
	req.RuntimeNodeID = uuid.New()

	err = service.RenewProjectTaskAttemptLease(context.Background(), RenewProjectTaskAttemptLeaseRequest{
		ProjectTaskAttemptRuntimeRequest: req,
	})

	require.ErrorIs(t, err, ErrProjectConflict)
}

type projectTaskAttemptServiceFixture struct {
	tenantID  uuid.UUID
	projectID uuid.UUID
	taskID    uuid.UUID
	attemptID uuid.UUID
	nodeID    uuid.UUID
	lease     string
}

func (f projectTaskAttemptServiceFixture) runtimeRequest(idempotencyKey string) ProjectTaskAttemptRuntimeRequest {
	return ProjectTaskAttemptRuntimeRequest{
		TenantID:       f.tenantID,
		AttemptID:      f.attemptID,
		ProjectTaskID:  f.taskID,
		RuntimeNodeID:  f.nodeID,
		LeaseToken:     f.lease,
		IdempotencyKey: idempotencyKey,
	}
}

func newProjectTaskAttemptServiceFixture(repo *memoryRepository, taskStatus, attemptStatus string) projectTaskAttemptServiceFixture {
	tenantID := uuid.New()
	projectID := uuid.New()
	taskID := uuid.New()
	attemptID := uuid.New()
	employeeID := uuid.New()
	nodeID := uuid.New()
	lease := "lease-token-1"
	now := time.Now().UTC()
	repo.projects[projectID] = Project{
		ID:                     projectID,
		TenantID:               tenantID,
		Name:                   "Runtime closure",
		Goal:                   "Close task through attempts",
		Status:                 ProjectStatusRunning,
		HumanOwnerUserID:       uuid.New(),
		CoordinationWorkflowID: "project-coordinator:" + projectID.String(),
		CreatedAt:              now,
		UpdatedAt:              now,
	}
	repo.tasks = append(repo.tasks, ProjectTask{
		ID:                        taskID,
		TenantID:                  tenantID,
		ProjectID:                 projectID,
		Title:                     "attempt writeback",
		Status:                    taskStatus,
		AssignedDigitalEmployeeID: &employeeID,
		CurrentAttemptID:          &attemptID,
		CreatedAt:                 now,
		UpdatedAt:                 now,
	})
	repo.projectTaskAttempts = append(repo.projectTaskAttempts, ProjectTaskAttempt{
		ID:             attemptID,
		TenantID:       tenantID,
		ProjectTaskID:  taskID,
		AttemptNo:      1,
		Status:         attemptStatus,
		RuntimeNodeID:  &nodeID,
		LeaseToken:     lease,
		IdempotencyKey: "project-task:" + taskID.String() + ":attempt:1:queue",
		CreatedAt:      now,
		UpdatedAt:      now,
	})
	return projectTaskAttemptServiceFixture{
		tenantID:  tenantID,
		projectID: projectID,
		taskID:    taskID,
		attemptID: attemptID,
		nodeID:    nodeID,
		lease:     lease,
	}
}

type delayedAttemptReadinessRepository struct {
	*memoryRepository
	staleProjectTaskID  uuid.UUID
	staleReadsRemaining int
}

func (r *delayedAttemptReadinessRepository) GetProjectTask(ctx context.Context, tenantID, taskID uuid.UUID) (ProjectTask, error) {
	task, err := r.memoryRepository.GetProjectTask(ctx, tenantID, taskID)
	if err != nil {
		return ProjectTask{}, err
	}
	if task.ID == r.staleProjectTaskID && r.staleReadsRemaining > 0 {
		r.staleReadsRemaining--
		task.Status = ProjectTaskStatusWaitingHuman
		task.CurrentAttemptID = nil
		return task, nil
	}
	return task, nil
}

func requireLedgerEventTypes(t *testing.T, events []ExecutionLedgerEvent, expected ...string) {
	t.Helper()
	actual := make([]string, 0, len(events))
	for _, event := range events {
		actual = append(actual, event.EventType)
	}
	for _, eventType := range expected {
		require.Contains(t, actual, eventType)
	}
}

func TestQueueProjectTaskReplaysIdempotencyKey(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	require.NoError(t, err)

	tenantID := uuid.New()
	projectID := uuid.New()
	taskID := uuid.New()
	employeeID := uuid.New()
	repo.tasks = append(repo.tasks, ProjectTask{
		ID:                        taskID,
		TenantID:                  tenantID,
		ProjectID:                 projectID,
		Title:                     "验证幂等重放",
		Status:                    ProjectTaskStatusPlanned,
		AssignedDigitalEmployeeID: &employeeID,
		CreatedAt:                 time.Now().UTC(),
		UpdatedAt:                 time.Now().UTC(),
	})
	req := QueueProjectTaskRequest{
		TenantID:          tenantID,
		ProjectID:         projectID,
		ProjectTaskID:     taskID,
		DigitalEmployeeID: employeeID,
		IdempotencyKey:    "project-task:" + taskID.String() + ":attempt:1:queue",
		LeaseToken:        "lease-token-1",
	}

	first, err := service.QueueProjectTask(context.Background(), req)
	require.NoError(t, err)
	second, err := service.QueueProjectTask(context.Background(), req)
	require.NoError(t, err)

	require.Equal(t, first.Attempt.ID, second.Attempt.ID)
	require.Equal(t, first.Task.CurrentAttemptID, second.Task.CurrentAttemptID)
	require.Equal(t, ProjectTaskStatusQueued, second.Task.Status)
	require.Len(t, repo.events, 1)
	require.Len(t, repo.projectTaskAttempts, 1)
}

func TestQueueProjectTaskRejectsIdempotencyKeyForDifferentTask(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	require.NoError(t, err)

	tenantID := uuid.New()
	projectID := uuid.New()
	employeeID := uuid.New()
	firstTaskID := uuid.New()
	secondTaskID := uuid.New()
	repo.tasks = append(repo.tasks,
		ProjectTask{
			ID:                        firstTaskID,
			TenantID:                  tenantID,
			ProjectID:                 projectID,
			Title:                     "首次排队任务",
			Status:                    ProjectTaskStatusPlanned,
			AssignedDigitalEmployeeID: &employeeID,
			CreatedAt:                 time.Now().UTC(),
			UpdatedAt:                 time.Now().UTC(),
		},
		ProjectTask{
			ID:                        secondTaskID,
			TenantID:                  tenantID,
			ProjectID:                 projectID,
			Title:                     "冲突排队任务",
			Status:                    ProjectTaskStatusPlanned,
			AssignedDigitalEmployeeID: &employeeID,
			CreatedAt:                 time.Now().UTC(),
			UpdatedAt:                 time.Now().UTC(),
		},
	)
	idempotencyKey := "project-task:" + firstTaskID.String() + ":attempt:1:queue"
	_, err = service.QueueProjectTask(context.Background(), QueueProjectTaskRequest{
		TenantID:          tenantID,
		ProjectID:         projectID,
		ProjectTaskID:     firstTaskID,
		DigitalEmployeeID: employeeID,
		IdempotencyKey:    idempotencyKey,
		LeaseToken:        "lease-token-1",
	})
	require.NoError(t, err)

	_, err = service.QueueProjectTask(context.Background(), QueueProjectTaskRequest{
		TenantID:          tenantID,
		ProjectID:         projectID,
		ProjectTaskID:     secondTaskID,
		DigitalEmployeeID: employeeID,
		IdempotencyKey:    idempotencyKey,
		LeaseToken:        "lease-token-2",
	})
	require.ErrorIs(t, err, ErrProjectConflict)
	require.Len(t, repo.events, 1)
	require.Len(t, repo.projectTaskAttempts, 1)
}

func TestQueueProjectTaskRejectsInvalidCurrentStatus(t *testing.T) {
	for _, status := range []string{ProjectTaskStatusRunning, ProjectTaskStatusCompleted} {
		t.Run(status, func(t *testing.T) {
			repo := newMemoryRepository()
			service, err := NewService(repo)
			require.NoError(t, err)

			tenantID := uuid.New()
			projectID := uuid.New()
			taskID := uuid.New()
			employeeID := uuid.New()
			repo.tasks = append(repo.tasks, ProjectTask{
				ID:                        taskID,
				TenantID:                  tenantID,
				ProjectID:                 projectID,
				Title:                     "状态冲突任务",
				Status:                    status,
				AssignedDigitalEmployeeID: &employeeID,
				CreatedAt:                 time.Now().UTC(),
				UpdatedAt:                 time.Now().UTC(),
			})

			_, err = service.QueueProjectTask(context.Background(), QueueProjectTaskRequest{
				TenantID:          tenantID,
				ProjectID:         projectID,
				ProjectTaskID:     taskID,
				DigitalEmployeeID: employeeID,
				IdempotencyKey:    "project-task:" + taskID.String() + ":attempt:1:queue",
				LeaseToken:        "lease-token-1",
			})
			require.ErrorIs(t, err, ErrProjectConflict)
			require.Empty(t, repo.events)
			require.Empty(t, repo.projectTaskAttempts)
		})
	}
}

func TestCreateProjectRejectsUnauthorizedTeamScope(t *testing.T) {
	repo := newMemoryRepository()
	coordinator := &fakeCoordinatorSignalClient{}
	service, err := NewServiceWithCoordinator(repo, coordinator)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	actorID := uuid.New()
	ownerID := uuid.New()
	teamID := uuid.New()

	_, err = service.CreateProject(context.Background(), CreateProjectRequest{
		TenantID:         tenantID,
		TeamID:           &teamID,
		ActorUserID:      actorID,
		Name:             "未授权团队项目",
		Goal:             "验证团队授权边界",
		HumanOwnerUserID: ownerID,
	})
	if !errors.Is(err, ErrUnauthorizedProjectTeamScope) {
		t.Fatalf("expected unauthorized team scope error, got %v", err)
	}
	assertNoCreateProjectSideEffects(t, repo, coordinator)
}

func TestCreateProjectAllowsAuthorizedTeamScope(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	actorID := uuid.New()
	ownerID := uuid.New()
	teamID := uuid.New()
	repo.authorizeProjectTeamScope(tenantID, actorID, teamID)

	created, err := service.CreateProject(context.Background(), CreateProjectRequest{
		TenantID:         tenantID,
		TeamID:           &teamID,
		ActorUserID:      actorID,
		Name:             "授权团队项目",
		Goal:             "验证授权通过",
		HumanOwnerUserID: ownerID,
		Members: []ProjectMemberInput{{
			PrincipalType:       PrincipalTypeTeam,
			PrincipalID:         teamID,
			ProjectRole:         ProjectRoleObserver,
			DisplayNameSnapshot: "研发团队",
		}},
	})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	if created.Project.TeamID == nil || *created.Project.TeamID != teamID {
		t.Fatalf("expected team id %s, got %#v", teamID, created.Project.TeamID)
	}
}

func TestCreateProjectRejectsUnauthorizedMemberOnlyTeamScope(t *testing.T) {
	repo := newMemoryRepository()
	coordinator := &fakeCoordinatorSignalClient{}
	service, err := NewServiceWithCoordinator(repo, coordinator)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	actorID := uuid.New()
	ownerID := uuid.New()
	teamID := uuid.New()

	_, err = service.CreateProject(context.Background(), CreateProjectRequest{
		TenantID:         tenantID,
		ActorUserID:      actorID,
		Name:             "未授权团队成员项目",
		Goal:             "验证成员团队授权边界",
		HumanOwnerUserID: ownerID,
		Members: []ProjectMemberInput{{
			PrincipalType:       PrincipalTypeTeam,
			PrincipalID:         teamID,
			ProjectRole:         ProjectRoleObserver,
			DisplayNameSnapshot: "研发团队",
		}},
	})
	if !errors.Is(err, ErrUnauthorizedProjectTeamScope) {
		t.Fatalf("expected unauthorized team scope error, got %v", err)
	}
	assertNoCreateProjectSideEffects(t, repo, coordinator)
}

func TestCreateProjectAllowsAuthorizedMemberOnlyTeamScope(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	actorID := uuid.New()
	ownerID := uuid.New()
	teamID := uuid.New()
	repo.authorizeProjectTeamScope(tenantID, actorID, teamID)

	created, err := service.CreateProject(context.Background(), CreateProjectRequest{
		TenantID:         tenantID,
		ActorUserID:      actorID,
		Name:             "授权团队成员项目",
		Goal:             "验证成员团队授权通过",
		HumanOwnerUserID: ownerID,
		Members: []ProjectMemberInput{{
			PrincipalType:       PrincipalTypeTeam,
			PrincipalID:         teamID,
			ProjectRole:         ProjectRoleObserver,
			DisplayNameSnapshot: "研发团队",
		}},
	})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	if created.Project.TeamID != nil {
		t.Fatalf("expected no top-level team id, got %#v", created.Project.TeamID)
	}
}

func TestCreateProjectWithoutTeamScopeSucceedsWithoutAuthorizer(t *testing.T) {
	backing := newMemoryRepository()
	service, err := NewService(&repositoryWithoutProjectTeamScopeAuthorizer{Repository: backing})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	actorID := uuid.New()
	ownerID := uuid.New()
	employeeID := uuid.New()

	created, err := service.CreateProject(context.Background(), CreateProjectRequest{
		TenantID:         tenantID,
		ActorUserID:      actorID,
		Name:             "无团队项目",
		Goal:             "验证无团队路径不要求授权器",
		HumanOwnerUserID: ownerID,
		Members: []ProjectMemberInput{{
			PrincipalType:       PrincipalTypeDigitalEmployee,
			PrincipalID:         employeeID,
			ProjectRole:         ProjectRoleExecutor,
			DisplayNameSnapshot: "后端执行 A",
		}},
	})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	if created.Project.TeamID != nil {
		t.Fatalf("expected no team id, got %#v", created.Project.TeamID)
	}
}

func TestCreateProjectRequiresMandatoryFields(t *testing.T) {
	service, err := NewService(newMemoryRepository())
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	_, err = service.CreateProject(context.Background(), CreateProjectRequest{
		TenantID:         uuid.New(),
		ActorUserID:      uuid.New(),
		Name:             "缺少目标",
		HumanOwnerUserID: uuid.New(),
	})
	if !errors.Is(err, ErrInvalidProject) {
		t.Fatalf("expected invalid project error, got %v", err)
	}
}

func TestCreateProjectRejectsCoordinatorMemberRole(t *testing.T) {
	service, err := NewService(newMemoryRepository())
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	_, err = service.CreateProject(context.Background(), CreateProjectRequest{
		TenantID:         uuid.New(),
		ActorUserID:      uuid.New(),
		Name:             "项目",
		Goal:             "目标",
		HumanOwnerUserID: uuid.New(),
		Members: []ProjectMemberInput{{
			PrincipalType: PrincipalTypeDigitalEmployee,
			PrincipalID:   uuid.New(),
			ProjectRole:   ProjectRole("coordinator"),
		}},
	})
	if !errors.Is(err, ErrInvalidProjectMember) {
		t.Fatalf("expected invalid member error, got %v", err)
	}
}

func TestCreateProjectValidatesRolePrincipalTypes(t *testing.T) {
	service, err := NewService(newMemoryRepository())
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	for _, tc := range []struct {
		name          string
		principalType PrincipalType
		role          ProjectRole
	}{
		{name: "owner must be human", principalType: PrincipalTypeDigitalEmployee, role: ProjectRoleOwner},
		{name: "leader must be human", principalType: PrincipalTypeDigitalEmployee, role: ProjectRoleLeader},
		{name: "acceptance must be human", principalType: PrincipalTypeDigitalEmployee, role: ProjectRoleAcceptance},
		{name: "executor must be digital employee", principalType: PrincipalTypeHumanUser, role: ProjectRoleExecutor},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := service.CreateProject(context.Background(), CreateProjectRequest{
				TenantID:         uuid.New(),
				ActorUserID:      uuid.New(),
				Name:             "项目",
				Goal:             "目标",
				HumanOwnerUserID: uuid.New(),
				Members: []ProjectMemberInput{{
					PrincipalType: tc.principalType,
					PrincipalID:   uuid.New(),
					ProjectRole:   tc.role,
				}},
			})
			if !errors.Is(err, ErrInvalidProjectMember) {
				t.Fatalf("expected invalid member error, got %v", err)
			}
		})
	}
}

func TestSubmitDemandRecordsDemandAndEventWithoutAutoCreatingTask(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	projectID := uuid.New()
	ownerID := uuid.New()
	repo.projects[projectID] = Project{
		ID:               projectID,
		TenantID:         uuid.MustParse("00000000-0000-0000-0000-000000000001"),
		Name:             "客户侧 Runtime 接入验收",
		Status:           ProjectStatusRunning,
		HumanOwnerUserID: ownerID,
	}
	seedHumanOwnerMember(repo, repo.projects[projectID].TenantID, projectID, ownerID)

	demand, err := service.SubmitDemand(context.Background(), SubmitProjectDemandRequest{
		TenantID:          repo.projects[projectID].TenantID,
		ProjectID:         projectID,
		SubmittedByUserID: ownerID,
		Title:             "验证 Runtime 连接",
		Content:           "检查心跳和命令回写",
		SourceType:        DemandSourceManual,
	})
	if err != nil {
		t.Fatalf("submit demand: %v", err)
	}
	if demand.Status != ProjectDemandStatusPlanningPending {
		t.Fatalf("expected planning pending demand, got %s", demand.Status)
	}
	if len(repo.tasks) != 0 {
		t.Fatalf("service must not create project tasks from demand directly")
	}
	if len(repo.eventTypes) != 1 || repo.eventTypes[0] != ProjectEventDemandSubmitted {
		t.Fatalf("expected demand event only, got %#v", repo.eventTypes)
	}
}

func TestGetDemandLaunchDetailAggregatesDemandFacts(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	ownerID := uuid.New()
	repo := newMemoryRepository()
	repo.projects[projectID] = Project{
		ID:               projectID,
		TenantID:         tenantID,
		Name:             "客户侧 Runtime 接入验收",
		Status:           ProjectStatusRunning,
		HumanOwnerUserID: ownerID,
	}
	seedHumanOwnerMember(repo, tenantID, projectID, ownerID)
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	demand, err := service.SubmitDemand(context.Background(), SubmitProjectDemandRequest{
		TenantID: tenantID, ProjectID: projectID, SubmittedByUserID: ownerID, Title: "审查 PR",
	})
	if err != nil {
		t.Fatalf("submit demand: %v", err)
	}
	job := CoordinationJob{ID: uuid.New(), TenantID: tenantID, ProjectID: projectID, TriggerEventID: demand.CreatedEventID, JobType: "demand_route", Status: "running"}
	inputJob := CoordinationJob{ID: uuid.New(), TenantID: tenantID, ProjectID: projectID, JobType: "demand_route", Status: "running", InputSnapshotRef: map[string]any{"demand_id": demand.ID.String()}}
	repo.coordinationJobs = append(repo.coordinationJobs, job, inputJob)
	task := ProjectTask{ID: uuid.New(), TenantID: tenantID, ProjectID: projectID, DemandID: &demand.ID, Title: "审查 PR", Status: "pending"}
	repo.tasks = append(repo.tasks, task)
	repo.routeDecisions = append(repo.routeDecisions, RouteDecision{ID: uuid.New(), TenantID: tenantID, ProjectID: projectID, CoordinationJobID: job.ID, DemandID: &demand.ID, Reason: "按能力分派"})
	decisionRequest := DecisionRequest{ID: uuid.New(), TenantID: tenantID, ProjectID: projectID, CoordinationJobID: &job.ID, TargetUserID: ownerID, DecisionType: "route_review", TitleSnapshot: "确认路由", StatusSnapshot: "pending"}
	taskDecisionRequest := DecisionRequest{ID: uuid.New(), TenantID: tenantID, ProjectID: projectID, ProjectTaskID: &task.ID, TargetUserID: ownerID, DecisionType: "task_review", TitleSnapshot: "确认任务", StatusSnapshot: "pending"}
	repo.decisionRequests = append(repo.decisionRequests, decisionRequest, taskDecisionRequest)
	demandResourceType := "project_demand"
	demandResourceID := demand.ID.String()
	demandResourceEvent := ProjectEvent{ID: uuid.New(), TenantID: tenantID, ProjectID: projectID, ResourceType: &demandResourceType, ResourceID: &demandResourceID, EventType: ProjectEventDemandSubmitted, ActorType: "human_user", ActorID: ownerID.String(), Payload: map[string]any{}}
	taskPayloadEvent := ProjectEvent{ID: uuid.New(), TenantID: tenantID, ProjectID: projectID, EventType: ProjectEventTaskDispatched, ActorType: "workflow", ActorID: job.ID.String(), Payload: map[string]any{"project_task_id": task.ID.String()}}
	decisionPayloadEvent := ProjectEvent{ID: uuid.New(), TenantID: tenantID, ProjectID: projectID, EventType: ProjectEventDecisionRequested, ActorType: "workflow", ActorID: job.ID.String(), Payload: map[string]any{"decision_request_id": decisionRequest.ID.String()}}
	repo.events = append(repo.events, demandResourceEvent, taskPayloadEvent, decisionPayloadEvent)
	for i := 0; i < 120; i++ {
		unrelatedDemandID := uuid.New()
		unrelatedJobID := uuid.New()
		unrelatedTaskID := uuid.New()
		unrelatedDecisionID := uuid.New()
		later := time.Now().UTC().Add(time.Duration(i+1) * time.Minute)
		repo.coordinationJobs = append(repo.coordinationJobs, CoordinationJob{ID: unrelatedJobID, TenantID: tenantID, ProjectID: projectID, JobType: "demand_route", Status: "running", InputSnapshotRef: map[string]any{"demand_id": unrelatedDemandID.String()}, CreatedAt: later})
		repo.tasks = append(repo.tasks, ProjectTask{ID: unrelatedTaskID, TenantID: tenantID, ProjectID: projectID, DemandID: &unrelatedDemandID, Title: "其他任务", Status: "pending", UpdatedAt: later})
		repo.routeDecisions = append(repo.routeDecisions, RouteDecision{ID: uuid.New(), TenantID: tenantID, ProjectID: projectID, CoordinationJobID: unrelatedJobID, DemandID: &unrelatedDemandID, Reason: "其他路由", CreatedAt: later})
		repo.decisionRequests = append(repo.decisionRequests, DecisionRequest{ID: unrelatedDecisionID, TenantID: tenantID, ProjectID: projectID, CoordinationJobID: &unrelatedJobID, ProjectTaskID: &unrelatedTaskID, TargetUserID: ownerID, DecisionType: "route_review", TitleSnapshot: "其他决策", StatusSnapshot: "pending", CreatedAt: later})
		unrelatedTaskResourceID := unrelatedTaskID.String()
		repo.events = append(repo.events, ProjectEvent{ID: uuid.New(), TenantID: tenantID, ProjectID: projectID, SequenceNumber: int64(1000 + i), EventType: ProjectEventTaskDispatched, ActorType: "workflow", ActorID: unrelatedJobID.String(), ResourceID: &unrelatedTaskResourceID, Payload: map[string]any{"project_task_id": unrelatedTaskID.String()}, CreatedAt: later})
	}

	detail, err := service.GetDemandLaunchDetail(context.Background(), tenantID, demand.ID)
	if err != nil {
		t.Fatalf("launch detail: %v", err)
	}
	if detail.Demand.ID != demand.ID || detail.Project.ID != projectID {
		t.Fatalf("unexpected demand/project: %#v", detail)
	}
	if detail.Reviewer == nil || detail.Reviewer.ReviewerUserID != ownerID {
		t.Fatalf("expected reviewer preference in launch detail: %#v", detail.Reviewer)
	}
	if len(detail.CoordinationJobs) != 2 || len(detail.RouteDecisions) != 1 || len(detail.ProjectTasks) != 1 || len(detail.DecisionRequests) != 2 {
		t.Fatalf("expected related facts, got %#v", detail)
	}
	if len(detail.RecentEvents) != 4 {
		t.Fatalf("expected demand event in launch detail: %#v", detail.RecentEvents)
	}
	eventIDs := map[uuid.UUID]struct{}{}
	for _, event := range detail.RecentEvents {
		eventIDs[event.ID] = struct{}{}
	}
	if _, ok := eventIDs[*demand.CreatedEventID]; !ok {
		t.Fatalf("expected created demand event in launch detail: %#v", detail.RecentEvents)
	}
	if _, ok := eventIDs[demandResourceEvent.ID]; !ok {
		t.Fatalf("expected demand resource event in launch detail: %#v", detail.RecentEvents)
	}
	if _, ok := eventIDs[taskPayloadEvent.ID]; !ok {
		t.Fatalf("expected task payload event in launch detail: %#v", detail.RecentEvents)
	}
	if _, ok := eventIDs[decisionPayloadEvent.ID]; !ok {
		t.Fatalf("expected decision payload event in launch detail: %#v", detail.RecentEvents)
	}
}

func TestGetProjectTaskGraphRequiresFilterAndDoesNotApplyHiddenLimit(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	demandID := uuid.New()
	repo := &taskGraphLimitRepository{memoryRepository: newMemoryRepository()}
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	_, err = service.GetProjectTaskGraph(context.Background(), GetProjectTaskGraphRequest{
		TenantID:  tenantID,
		ProjectID: projectID,
	})
	if !errors.Is(err, ErrInvalidProject) {
		t.Fatalf("expected missing graph filter to be invalid, got %v", err)
	}
	if repo.calls != 0 {
		t.Fatalf("expected invalid graph request not to call repository, got %d calls", repo.calls)
	}

	graph, err := service.GetProjectTaskGraph(context.Background(), GetProjectTaskGraphRequest{
		TenantID: tenantID, ProjectID: projectID, DemandID: &demandID,
	})
	if err != nil {
		t.Fatalf("get graph: %v", err)
	}
	if len(graph.Nodes) != 55 {
		t.Fatalf("expected complete demand graph, got %d nodes", len(graph.Nodes))
	}
	if graph.Edges == nil || graph.Employees == nil || graph.Runs == nil || graph.ExecutionSummaries == nil || graph.RecentEvents == nil || graph.DecisionRequests == nil {
		t.Fatalf("expected non-nil graph sidecar slices: %#v", graph)
	}
	if repo.lastReq.Limit != 0 || repo.lastReq.Offset != 0 {
		t.Fatalf("expected graph service not to apply hidden pagination, got limit=%d offset=%d", repo.lastReq.Limit, repo.lastReq.Offset)
	}
}

func TestGetProjectTaskGraphBuildsStageSummariesWhenRepositoryOmitsThem(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	demandID := uuid.New()
	stageOne := int32(1)
	stageTwo := int32(2)
	repo := &taskGraphLimitRepository{
		memoryRepository: newMemoryRepository(),
		graph: ProjectTaskGraph{
			Nodes: []ProjectTaskGraphNode{
				{Task: ProjectTask{ID: uuid.New(), TenantID: tenantID, ProjectID: projectID, Title: "入口", Status: "completed", StageIndex: &stageOne}},
				{Task: ProjectTask{ID: uuid.New(), TenantID: tenantID, ProjectID: projectID, Title: "巡检", Status: "running", StageIndex: &stageTwo}},
				{Task: ProjectTask{ID: uuid.New(), TenantID: tenantID, ProjectID: projectID, Title: "审批", Status: "waiting_human", StageIndex: &stageTwo, RequiresHumanApproval: true}},
			},
		},
	}
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	graph, err := service.GetProjectTaskGraph(context.Background(), GetProjectTaskGraphRequest{
		TenantID: tenantID, ProjectID: projectID, DemandID: &demandID,
	})
	if err != nil {
		t.Fatalf("get graph: %v", err)
	}
	if len(graph.StageSummaries) != 2 {
		t.Fatalf("expected two stage summaries, got %#v", graph.StageSummaries)
	}
	if graph.StageSummaries[0].StageIndex != 1 || graph.StageSummaries[0].CompletedNodes != 1 {
		t.Fatalf("unexpected first stage summary: %#v", graph.StageSummaries[0])
	}
	if graph.StageSummaries[1].StageIndex != 2 || graph.StageSummaries[1].RunningNodes != 1 || graph.StageSummaries[1].WaitingHumanNodes != 1 {
		t.Fatalf("unexpected second stage summary: %#v", graph.StageSummaries[1])
	}
}

func TestListWorkflowInstancesNormalizesPaginationAndStatusPriority(t *testing.T) {
	tenantID := uuid.New()
	actorID := uuid.New()
	projectID := uuid.New()
	demandID := uuid.New()
	repo := &workflowInstanceServiceRepository{
		memoryRepository: newMemoryRepository(),
		items: []WorkflowInstanceSummary{{
			DemandID:          demandID,
			ProjectID:         projectID,
			ProjectName:       "支付巡检",
			Title:             "定位支付成功率下降",
			SubmittedByUserID: actorID,
			Status:            WorkflowInstanceStatusUnknown,
			Progress: WorkflowInstanceProgress{
				TotalNodes:        3,
				CompletedNodes:    1,
				RunningNodes:      1,
				BlockedNodes:      1,
				WaitingHumanNodes: 1,
			},
		}},
	}
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	items, err := service.ListWorkflowInstances(context.Background(), ListWorkflowInstancesRequest{
		TenantID:    tenantID,
		ActorUserID: actorID,
		Limit:       0,
		Offset:      -4,
	})
	if err != nil {
		t.Fatalf("list workflow instances: %v", err)
	}
	if repo.lastReq.Limit != 20 || repo.lastReq.Offset != 0 {
		t.Fatalf("expected normalized pagination, got limit=%d offset=%d", repo.lastReq.Limit, repo.lastReq.Offset)
	}
	if len(items) != 1 {
		t.Fatalf("expected one workflow instance, got %#v", items)
	}
	if items[0].Status != WorkflowInstanceStatusWaitingHuman {
		t.Fatalf("expected waiting_human to outrank running and planning, got %#v", items[0])
	}
}

func TestListWorkflowInstancesRejectsMissingActor(t *testing.T) {
	tenantID := uuid.New()
	repo := &workflowInstanceServiceRepository{memoryRepository: newMemoryRepository()}
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	_, err = service.ListWorkflowInstances(context.Background(), ListWorkflowInstancesRequest{
		TenantID: tenantID,
	})
	if !errors.Is(err, ErrInvalidProject) {
		t.Fatalf("expected invalid project error, got %v", err)
	}
	if repo.calls != 0 {
		t.Fatalf("expected invalid request not to call repository, got %d calls", repo.calls)
	}
}

func TestListWorkflowInstancesKeepsOptionalReadModelFieldsAndSortsAttentionFirst(t *testing.T) {
	tenantID := uuid.New()
	actorID := uuid.New()
	waitingDemandID := uuid.New()
	runningDemandID := uuid.New()
	completedDemandID := uuid.New()
	dueAt := time.Now().UTC().Add(15 * time.Minute)
	remaining := int32(900)
	repo := &workflowInstanceServiceRepository{
		memoryRepository: newMemoryRepository(),
		items: []WorkflowInstanceSummary{
			{
				DemandID:          completedDemandID,
				ProjectID:         uuid.New(),
				ProjectName:       "归档项目",
				Title:             "复盘归档",
				SubmittedByUserID: actorID,
				Status:            WorkflowInstanceStatusCompleted,
				UpdatedAt:         time.Now().UTC().Add(-2 * time.Minute),
				Progress: WorkflowInstanceProgress{
					TotalNodes:     2,
					CompletedNodes: 2,
				},
			},
			{
				DemandID:          runningDemandID,
				ProjectID:         uuid.New(),
				ProjectName:       "运行项目",
				Title:             "服务巡检",
				SubmittedByUserID: actorID,
				Status:            WorkflowInstanceStatusRunning,
				UpdatedAt:         time.Now().UTC().Add(-1 * time.Minute),
				Progress: WorkflowInstanceProgress{
					TotalNodes:   3,
					RunningNodes: 1,
				},
			},
			{
				DemandID:          waitingDemandID,
				ProjectID:         uuid.New(),
				ProjectName:       "支付项目",
				Title:             "支付成功率下降",
				SubmittedByUserID: actorID,
				Status:            WorkflowInstanceStatusUnknown,
				UpdatedAt:         time.Now().UTC().Add(-3 * time.Minute),
				Progress: WorkflowInstanceProgress{
					TotalNodes:        5,
					CompletedNodes:    2,
					RunningNodes:      1,
					BlockedNodes:      1,
					WaitingHumanNodes: 1,
					PlannedNodes:      1,
					FailedNodes:       0,
					CancelledNodes:    0,
				},
				CurrentBlocker: &WorkflowInstanceCurrentBlocker{
					Type:  "decision_request",
					Title: "等待人工审批回滚方案",
				},
				Priority: &WorkflowInstancePriority{
					Value:  "p1",
					Label:  "P1",
					Source: "source_refs.priority",
				},
				Risk: &WorkflowInstanceRisk{
					Level:  "high",
					Label:  "高风险",
					Source: "project_tasks.risk_level",
				},
				SLA: &WorkflowInstanceSLA{
					DueAt:            &dueAt,
					RemainingSeconds: &remaining,
					Breached:         false,
					Label:            "剩余 15 分钟",
					Source:           "source_refs.sla_due_at",
				},
				RecentEvent: &WorkflowInstanceRecentEvent{
					EventType:  string(ProjectEventDecisionRequested),
					Summary:    "已创建恢复决策请求",
					OccurredAt: time.Now().UTC().Add(-30 * time.Second),
				},
			},
		},
	}
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	items, err := service.ListWorkflowInstances(context.Background(), ListWorkflowInstancesRequest{
		TenantID:    tenantID,
		ActorUserID: actorID,
	})
	if err != nil {
		t.Fatalf("list workflow instances: %v", err)
	}
	if len(items) != 3 {
		t.Fatalf("expected three workflow instances, got %#v", items)
	}
	if items[0].DemandID != waitingDemandID {
		t.Fatalf("expected waiting-human workflow first, got %#v", items)
	}
	if items[0].Status != WorkflowInstanceStatusWaitingHuman {
		t.Fatalf("expected waiting_human status, got %s", items[0].Status)
	}
	if items[0].Priority == nil || items[0].Priority.Label != "P1" {
		t.Fatalf("expected priority field to survive service normalization: %#v", items[0].Priority)
	}
	if items[0].Risk == nil || items[0].Risk.Level != "high" {
		t.Fatalf("expected risk field to survive service normalization: %#v", items[0].Risk)
	}
	if items[0].SLA == nil || items[0].SLA.RemainingSeconds == nil || *items[0].SLA.RemainingSeconds != 900 {
		t.Fatalf("expected SLA field to survive service normalization: %#v", items[0].SLA)
	}
	if items[0].RecentEvent == nil || items[0].RecentEvent.EventType != string(ProjectEventDecisionRequested) {
		t.Fatalf("expected recent event field to survive service normalization: %#v", items[0].RecentEvent)
	}
}

func TestSubmitDemandPersistsDefaultReviewerPreference(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	ownerID := uuid.New()
	reviewerID := uuid.New()
	repo := newMemoryRepository()
	repo.projects[projectID] = Project{
		ID:               projectID,
		TenantID:         tenantID,
		Status:           ProjectStatusRunning,
		HumanOwnerUserID: ownerID,
	}
	repo.members[projectID] = []ProjectMember{
		{
			ID: uuid.New(), TenantID: tenantID, ProjectID: projectID,
			PrincipalType: PrincipalTypeHumanUser, PrincipalID: ownerID,
			ProjectRole: ProjectRoleOwner, Status: "active",
		},
		{
			ID: uuid.New(), TenantID: tenantID, ProjectID: projectID,
			PrincipalType: PrincipalTypeHumanUser, PrincipalID: reviewerID,
			ProjectRole: ProjectRoleReviewer, Status: "active",
		},
	}
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	demand, err := service.SubmitDemand(context.Background(), SubmitProjectDemandRequest{
		TenantID: tenantID, ProjectID: projectID, SubmittedByUserID: ownerID,
		Title: "审查 PR", Content: "统计 PR 并分派审查",
	})
	if err != nil {
		t.Fatalf("submit demand: %v", err)
	}

	if demand.ReviewerPreference == nil {
		t.Fatalf("expected reviewer preference on demand: %#v", demand)
	}
	if demand.ReviewerPreference.ReviewerUserID != reviewerID {
		t.Fatalf("expected reviewer %s, got %#v", reviewerID, demand.ReviewerPreference)
	}
	if demand.ReviewerPreference.SelectionReason != ReviewerSelectionProjectReviewerDefault {
		t.Fatalf("unexpected reviewer reason: %#v", demand.ReviewerPreference)
	}
	if demand.SourceRefs["reviewer_user_id"] != reviewerID.String() {
		t.Fatalf("expected reviewer persisted in source refs: %#v", demand.SourceRefs)
	}
}

func TestSubmitDemandPersistsExplicitReviewerSelectionReason(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	ownerID := uuid.New()
	reviewerID := uuid.New()
	reviewerName := "审查负责人"
	repo := newMemoryRepository()
	repo.projects[projectID] = Project{
		ID:               projectID,
		TenantID:         tenantID,
		Status:           ProjectStatusRunning,
		HumanOwnerUserID: ownerID,
	}
	repo.members[projectID] = []ProjectMember{
		{
			ID: uuid.New(), TenantID: tenantID, ProjectID: projectID,
			PrincipalType: PrincipalTypeHumanUser, PrincipalID: ownerID,
			ProjectRole: ProjectRoleOwner, Status: "active",
		},
		{
			ID: uuid.New(), TenantID: tenantID, ProjectID: projectID,
			PrincipalType: PrincipalTypeHumanUser, PrincipalID: reviewerID,
			ProjectRole: ProjectRoleReviewer, DisplayNameSnapshot: &reviewerName, Status: "active",
		},
	}
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	demand, err := service.SubmitDemand(context.Background(), SubmitProjectDemandRequest{
		TenantID: tenantID, ProjectID: projectID, SubmittedByUserID: ownerID,
		Title: "审查 PR", ReviewerUserID: &reviewerID,
		ReviewerSelectionReason: ReviewerSelectionProjectReviewerDefault,
	})
	if err != nil {
		t.Fatalf("submit demand: %v", err)
	}

	if demand.ReviewerPreference == nil {
		t.Fatalf("expected reviewer preference on demand: %#v", demand)
	}
	if demand.ReviewerPreference.SelectionReason != ReviewerSelectionProjectReviewerDefault {
		t.Fatalf("expected explicit reason to be preserved, got %#v", demand.ReviewerPreference)
	}
	if demand.SourceRefs["reviewer_selection_reason"] != string(ReviewerSelectionProjectReviewerDefault) {
		t.Fatalf("expected reviewer reason persisted in source refs: %#v", demand.SourceRefs)
	}
	if demand.SourceRefs["reviewer_display_name"] != reviewerName {
		t.Fatalf("expected reviewer display name persisted in source refs: %#v", demand.SourceRefs)
	}
}

func TestSubmitDemandRejectsInvalidReviewerSelectionReason(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	ownerID := uuid.New()
	reviewerID := uuid.New()
	repo := newMemoryRepository()
	repo.projects[projectID] = Project{
		ID:               projectID,
		TenantID:         tenantID,
		Status:           ProjectStatusRunning,
		HumanOwnerUserID: ownerID,
	}
	repo.members[projectID] = []ProjectMember{
		{
			ID: uuid.New(), TenantID: tenantID, ProjectID: projectID,
			PrincipalType: PrincipalTypeHumanUser, PrincipalID: ownerID,
			ProjectRole: ProjectRoleOwner, Status: "active",
		},
		{
			ID: uuid.New(), TenantID: tenantID, ProjectID: projectID,
			PrincipalType: PrincipalTypeHumanUser, PrincipalID: reviewerID,
			ProjectRole: ProjectRoleReviewer, Status: "active",
		},
	}
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	_, err = service.SubmitDemand(context.Background(), SubmitProjectDemandRequest{
		TenantID: tenantID, ProjectID: projectID, SubmittedByUserID: ownerID,
		Title: "审查 PR", ReviewerUserID: &reviewerID,
		ReviewerSelectionReason: ReviewerSelectionReason("invalid_reason"),
	})
	if !errors.Is(err, ErrInvalidProjectMember) {
		t.Fatalf("expected invalid project member, got %v", err)
	}
}

func TestSubmitDemandDiscardsSpoofedReviewerSourceRefs(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	ownerID := uuid.New()
	reviewerID := uuid.New()
	repo := newMemoryRepository()
	repo.projects[projectID] = Project{
		ID:               projectID,
		TenantID:         tenantID,
		Status:           ProjectStatusRunning,
		HumanOwnerUserID: ownerID,
	}
	repo.members[projectID] = []ProjectMember{
		{
			ID: uuid.New(), TenantID: tenantID, ProjectID: projectID,
			PrincipalType: PrincipalTypeHumanUser, PrincipalID: ownerID,
			ProjectRole: ProjectRoleOwner, Status: "active",
		},
		{
			ID: uuid.New(), TenantID: tenantID, ProjectID: projectID,
			PrincipalType: PrincipalTypeHumanUser, PrincipalID: reviewerID,
			ProjectRole: ProjectRoleReviewer, Status: "active",
		},
	}
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	demand, err := service.SubmitDemand(context.Background(), SubmitProjectDemandRequest{
		TenantID: tenantID, ProjectID: projectID, SubmittedByUserID: ownerID,
		Title: "审查 PR", ReviewerUserID: &reviewerID,
		SourceRefs: map[string]any{
			"reviewer_display_name": "Spoofed",
			"reviewer_user_id":      "bad",
			"external_ticket":       "T-1",
		},
	})
	if err != nil {
		t.Fatalf("submit demand: %v", err)
	}

	if demand.SourceRefs["reviewer_user_id"] != reviewerID.String() {
		t.Fatalf("expected canonical reviewer id, got source refs: %#v", demand.SourceRefs)
	}
	if _, ok := demand.SourceRefs["reviewer_display_name"]; ok {
		t.Fatalf("expected spoofed display name to be discarded: %#v", demand.SourceRefs)
	}
	if demand.SourceRefs["external_ticket"] != "T-1" {
		t.Fatalf("expected non-reviewer source ref to remain: %#v", demand.SourceRefs)
	}
}

func TestSubmitDemandFallsBackToHumanOwnerWhenNoReviewer(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	ownerID := uuid.New()
	repo := newMemoryRepository()
	repo.projects[projectID] = Project{
		ID:               projectID,
		TenantID:         tenantID,
		Status:           ProjectStatusRunning,
		HumanOwnerUserID: ownerID,
	}
	repo.members[projectID] = []ProjectMember{{
		ID: uuid.New(), TenantID: tenantID, ProjectID: projectID,
		PrincipalType: PrincipalTypeHumanUser, PrincipalID: ownerID,
		ProjectRole: ProjectRoleOwner, Status: "active",
	}}
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	demand, err := service.SubmitDemand(context.Background(), SubmitProjectDemandRequest{
		TenantID: tenantID, ProjectID: projectID, SubmittedByUserID: ownerID,
		Title: "补充证据",
	})
	if err != nil {
		t.Fatalf("submit demand: %v", err)
	}
	if demand.ReviewerPreference == nil || demand.ReviewerPreference.ReviewerUserID != ownerID {
		t.Fatalf("expected owner fallback preference: %#v", demand.ReviewerPreference)
	}
	if demand.ReviewerPreference.SelectionReason != ReviewerSelectionProjectHumanOwnerFallback {
		t.Fatalf("expected owner fallback reason, got %#v", demand.ReviewerPreference)
	}
}

func TestSubmitDemandRequiresActiveHumanOwnerMemberForFallback(t *testing.T) {
	for _, tc := range []struct {
		name    string
		members []ProjectMember
	}{
		{name: "missing owner member"},
		{
			name: "inactive owner member",
			members: []ProjectMember{{
				PrincipalType: PrincipalTypeHumanUser,
				ProjectRole:   ProjectRoleOwner,
				Status:        "inactive",
			}},
		},
		{
			name: "digital owner member",
			members: []ProjectMember{{
				PrincipalType: PrincipalTypeDigitalEmployee,
				ProjectRole:   ProjectRoleOwner,
				Status:        "active",
			}},
		},
		{
			name: "observer owner member",
			members: []ProjectMember{{
				PrincipalType: PrincipalTypeHumanUser,
				ProjectRole:   ProjectRoleObserver,
				Status:        "active",
			}},
		},
		{
			name: "executor owner member",
			members: []ProjectMember{{
				PrincipalType: PrincipalTypeHumanUser,
				ProjectRole:   ProjectRoleExecutor,
				Status:        "active",
			}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tenantID := uuid.New()
			projectID := uuid.New()
			ownerID := uuid.New()
			repo := newMemoryRepository()
			repo.projects[projectID] = Project{
				ID:               projectID,
				TenantID:         tenantID,
				Status:           ProjectStatusRunning,
				HumanOwnerUserID: ownerID,
			}
			for _, member := range tc.members {
				member.ID = uuid.New()
				member.TenantID = tenantID
				member.ProjectID = projectID
				member.PrincipalID = ownerID
				repo.members[projectID] = append(repo.members[projectID], member)
			}
			service, err := NewService(repo)
			if err != nil {
				t.Fatalf("new service: %v", err)
			}

			_, err = service.SubmitDemand(context.Background(), SubmitProjectDemandRequest{
				TenantID: tenantID, ProjectID: projectID, SubmittedByUserID: ownerID,
				Title: "补充证据",
			})
			if !errors.Is(err, ErrInvalidProjectMember) {
				t.Fatalf("expected invalid project member, got %v", err)
			}
		})
	}
}

func TestReviewerPreferenceFromSourceRefsRestoresDisplayName(t *testing.T) {
	reviewerID := uuid.New()
	preference := reviewerPreferenceFromSourceRefs(map[string]any{
		"reviewer_user_id":            reviewerID.String(),
		"reviewer_selection_reason":   string(ReviewerSelectionProjectReviewerDefault),
		"reviewer_project_role":       string(ProjectRoleReviewer),
		"reviewer_resolved_from_rule": true,
		"reviewer_display_name":       "审查负责人",
	})

	if preference == nil {
		t.Fatal("expected reviewer preference")
	}
	if preference.DisplayName == nil || *preference.DisplayName != "审查负责人" {
		t.Fatalf("expected display name restored, got %#v", preference)
	}
	if preference.ReviewerUserID != reviewerID || preference.SelectionReason != ReviewerSelectionProjectReviewerDefault || preference.ProjectRole != ProjectRoleReviewer || !preference.ResolvedFromRule {
		t.Fatalf("unexpected reviewer preference: %#v", preference)
	}
}

func TestSubmitDemandRejectsDigitalEmployeeReviewer(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	ownerID := uuid.New()
	digitalEmployeeID := uuid.New()
	repo := newMemoryRepository()
	repo.projects[projectID] = Project{
		ID:               projectID,
		TenantID:         tenantID,
		Status:           ProjectStatusRunning,
		HumanOwnerUserID: ownerID,
	}
	repo.members[projectID] = []ProjectMember{
		{
			ID: uuid.New(), TenantID: tenantID, ProjectID: projectID,
			PrincipalType: PrincipalTypeHumanUser, PrincipalID: ownerID,
			ProjectRole: ProjectRoleOwner, Status: "active",
		},
		{
			ID: uuid.New(), TenantID: tenantID, ProjectID: projectID,
			PrincipalType: PrincipalTypeDigitalEmployee, PrincipalID: digitalEmployeeID,
			ProjectRole: ProjectRoleExecutor, Status: "active",
		},
	}
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	_, err = service.SubmitDemand(context.Background(), SubmitProjectDemandRequest{
		TenantID: tenantID, ProjectID: projectID, SubmittedByUserID: ownerID,
		Title: "需要审核", ReviewerUserID: &digitalEmployeeID,
	})
	if !errors.Is(err, ErrInvalidProjectMember) {
		t.Fatalf("expected invalid project member, got %v", err)
	}
}

func TestSubmitDemandRequiresExplicitReviewerWhenMultipleReviewers(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	ownerID := uuid.New()
	repo := newMemoryRepository()
	repo.projects[projectID] = Project{
		ID:               projectID,
		TenantID:         tenantID,
		Status:           ProjectStatusRunning,
		HumanOwnerUserID: ownerID,
	}
	repo.members[projectID] = []ProjectMember{{
		ID: uuid.New(), TenantID: tenantID, ProjectID: projectID,
		PrincipalType: PrincipalTypeHumanUser, PrincipalID: ownerID,
		ProjectRole: ProjectRoleOwner, Status: "active",
	}}
	for range 2 {
		repo.members[projectID] = append(repo.members[projectID], ProjectMember{
			ID: uuid.New(), TenantID: tenantID, ProjectID: projectID,
			PrincipalType: PrincipalTypeHumanUser, PrincipalID: uuid.New(),
			ProjectRole: ProjectRoleReviewer, Status: "active",
		})
	}
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	_, err = service.SubmitDemand(context.Background(), SubmitProjectDemandRequest{
		TenantID: tenantID, ProjectID: projectID, SubmittedByUserID: ownerID,
		Title: "多审核人项目",
	})
	if !errors.Is(err, ErrInvalidProjectMember) {
		t.Fatalf("expected reviewer selection error, got %v", err)
	}
}

func TestProjectGovernanceCreatesEvidenceAndProjectEvent(t *testing.T) {
	repo := newGovernanceMemoryRepository()
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	projectID := uuid.New()
	actorID := uuid.New()
	repo.projects[projectID] = Project{ID: projectID, TenantID: tenantID, Status: ProjectStatusRunning, HumanOwnerUserID: actorID}

	evidence, err := service.CreateEvidenceRef(context.Background(), CreateEvidenceRefServiceRequest{
		TenantID: tenantID, ProjectID: projectID, ActorType: "human_user", ActorID: actorID,
		EvidenceType: "test_result", Title: "回归测试结果", SourceType: "artifact",
		SourceRef: "s3://bucket/reports/regression.json", SubmittedByType: "human_user", SubmittedByID: &actorID,
	})
	if err != nil {
		t.Fatalf("create evidence: %v", err)
	}
	if evidence.VerificationStatus != EvidenceVerificationStatusSubmitted {
		t.Fatalf("expected submitted evidence, got %s", evidence.VerificationStatus)
	}
	if repo.eventTypes[len(repo.eventTypes)-1] != ProjectEventEvidenceLinked {
		t.Fatalf("expected evidence event, got %#v", repo.eventTypes)
	}
}

func TestProjectAcceptanceRequiresHumanOwnerAndFinalReport(t *testing.T) {
	repo := newGovernanceMemoryRepository()
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	projectID := uuid.New()
	ownerID := uuid.New()
	otherUserID := uuid.New()
	repo.projects[projectID] = Project{ID: projectID, TenantID: tenantID, Status: ProjectStatusAcceptance, HumanOwnerUserID: ownerID}

	_, err = service.CreateAcceptanceRecord(context.Background(), CreateAcceptanceServiceRequest{
		TenantID: tenantID, ProjectID: projectID, AcceptedByUserID: otherUserID,
		Status: "accepted", Conclusion: "通过", EvidenceRefIDs: []uuid.UUID{uuid.New()}, ReportRefIDs: []uuid.UUID{uuid.New()},
	})
	if !errors.Is(err, ErrInvalidProjectAcceptance) {
		t.Fatalf("expected invalid acceptance actor, got %v", err)
	}
}

func TestProjectGovernanceEvidenceFailureDoesNotLeaveSuccessEvent(t *testing.T) {
	repo := newGovernanceMemoryRepository()
	repo.createEvidenceRefErr = fmt.Errorf("evidence store unavailable")
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	projectID := uuid.New()
	actorID := uuid.New()
	repo.projects[projectID] = Project{ID: projectID, TenantID: tenantID, Status: ProjectStatusRunning, HumanOwnerUserID: actorID}

	_, err = service.CreateEvidenceRef(context.Background(), CreateEvidenceRefServiceRequest{
		TenantID: tenantID, ProjectID: projectID, ActorType: "human_user", ActorID: actorID,
		EvidenceType: "test_result", Title: "回归测试结果", SourceType: "artifact",
		SourceRef: "s3://bucket/reports/regression.json", SubmittedByType: "human_user", SubmittedByID: &actorID,
	})
	if err == nil {
		t.Fatal("expected evidence write error")
	}
	if countProjectEvents(repo.eventTypes, ProjectEventEvidenceLinked) != 0 {
		t.Fatalf("expected no success event after evidence failure, got %#v", repo.eventTypes)
	}
}

func TestProjectPatchEvidencePreservesOrClearsMetadata(t *testing.T) {
	repo := newGovernanceMemoryRepository()
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	projectID := uuid.New()
	ownerID := uuid.New()
	repo.projects[projectID] = Project{ID: projectID, TenantID: tenantID, Status: ProjectStatusRunning, HumanOwnerUserID: ownerID}
	evidence, err := repo.CreateEvidenceRef(context.Background(), CreateEvidenceRefRequest{
		TenantID: tenantID, ProjectID: projectID, EvidenceType: "test_result", Title: "回归测试结果",
		SourceType: "artifact", SourceRef: "s3://bucket/reports/regression.json",
		SubmittedByType: "human_user", SubmittedByID: &ownerID, VerificationStatus: EvidenceVerificationStatusSubmitted,
		Metadata: map[string]any{"suite": "regression", "passed": true},
	})
	if err != nil {
		t.Fatalf("seed evidence: %v", err)
	}

	updated, err := service.PatchEvidence(context.Background(), PatchEvidenceRequest{
		TenantID: tenantID, ProjectID: projectID, EvidenceID: evidence.ID, ActorUserID: ownerID,
		VerificationStatus: EvidenceVerificationStatusVerified,
	})
	if err != nil {
		t.Fatalf("patch evidence with omitted metadata: %v", err)
	}
	if updated.VerificationStatus != EvidenceVerificationStatusVerified || updated.Metadata["suite"] != "regression" || updated.Metadata["passed"] != true {
		t.Fatalf("expected omitted metadata to keep existing values, got %#v", updated)
	}

	cleared, err := service.PatchEvidence(context.Background(), PatchEvidenceRequest{
		TenantID: tenantID, ProjectID: projectID, EvidenceID: evidence.ID, ActorUserID: ownerID,
		VerificationStatus: EvidenceVerificationStatusRejected,
		Metadata:           map[string]any{},
	})
	if err != nil {
		t.Fatalf("patch evidence with empty metadata: %v", err)
	}
	if cleared.VerificationStatus != EvidenceVerificationStatusRejected || len(cleared.Metadata) != 0 {
		t.Fatalf("expected explicit empty metadata to clear values, got %#v", cleared)
	}
}

func TestProjectPatchEvidenceEventFailureRollsBackStatusAndMetadata(t *testing.T) {
	repo := newGovernanceMemoryRepository()
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	projectID := uuid.New()
	ownerID := uuid.New()
	repo.projects[projectID] = Project{ID: projectID, TenantID: tenantID, Status: ProjectStatusRunning, HumanOwnerUserID: ownerID}
	evidence, err := repo.CreateEvidenceRef(context.Background(), CreateEvidenceRefRequest{
		TenantID: tenantID, ProjectID: projectID, EvidenceType: "test_result", Title: "回归测试结果",
		SourceType: "artifact", SourceRef: "s3://bucket/reports/regression.json",
		SubmittedByType: "human_user", SubmittedByID: &ownerID, VerificationStatus: EvidenceVerificationStatusSubmitted,
		Metadata: map[string]any{"suite": "regression"},
	})
	if err != nil {
		t.Fatalf("seed evidence: %v", err)
	}
	repo.appendProjectEventErr = errors.New("event store unavailable")

	_, err = service.PatchEvidence(context.Background(), PatchEvidenceRequest{
		TenantID: tenantID, ProjectID: projectID, EvidenceID: evidence.ID, ActorUserID: ownerID,
		VerificationStatus: EvidenceVerificationStatusVerified,
		Metadata:           map[string]any{"suite": "smoke"},
	})
	if err == nil {
		t.Fatal("expected event write failure")
	}
	if repo.evidenceRefs[0].VerificationStatus != EvidenceVerificationStatusSubmitted || repo.evidenceRefs[0].Metadata["suite"] != "regression" {
		t.Fatalf("expected evidence update rolled back, got %#v", repo.evidenceRefs[0])
	}
	if countProjectEvents(repo.eventTypes, ProjectEventEvidenceVerified) != 0 {
		t.Fatalf("expected no verification event after rollback, got %#v", repo.eventTypes)
	}
}

func TestProjectGovernanceMissingRecordsReturnNotFound(t *testing.T) {
	repo := newGovernanceMemoryRepository()
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	projectID := uuid.New()
	ownerID := uuid.New()
	repo.projects[projectID] = Project{ID: projectID, TenantID: tenantID, Status: ProjectStatusRunning, HumanOwnerUserID: ownerID}

	_, err = service.PatchEvidence(context.Background(), PatchEvidenceRequest{
		TenantID: tenantID, ProjectID: projectID, EvidenceID: uuid.New(), ActorUserID: ownerID,
		VerificationStatus: EvidenceVerificationStatusVerified,
	})
	if !errors.Is(err, ErrProjectNotFound) {
		t.Fatalf("expected missing evidence not found, got %v", err)
	}
	_, err = service.GetAcceptance(context.Background(), tenantID, projectID)
	if !errors.Is(err, ErrProjectNotFound) {
		t.Fatalf("expected missing acceptance not found, got %v", err)
	}
	_, err = service.GetConfigRevision(context.Background(), tenantID, projectID, uuid.New())
	if !errors.Is(err, ErrProjectNotFound) {
		t.Fatalf("expected missing config revision not found, got %v", err)
	}
}

func TestProjectBudgetSummaryRequiresExistingProject(t *testing.T) {
	repo := newGovernanceMemoryRepository()
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	_, err = service.GetBudgetSummary(context.Background(), uuid.New(), uuid.New())
	if !errors.Is(err, ErrProjectNotFound) {
		t.Fatalf("expected missing project budget summary to return not found, got %v", err)
	}
}

func TestProjectAcceptanceRejectsMissingEvidenceOrReportRefs(t *testing.T) {
	repo := newGovernanceMemoryRepository()
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	projectID := uuid.New()
	ownerID := uuid.New()
	repo.projects[projectID] = Project{ID: projectID, TenantID: tenantID, Status: ProjectStatusAcceptance, HumanOwnerUserID: ownerID}

	_, err = service.CreateAcceptanceRecord(context.Background(), CreateAcceptanceServiceRequest{
		TenantID: tenantID, ProjectID: projectID, AcceptedByUserID: ownerID,
		Status: "accepted", Conclusion: "通过", EvidenceRefIDs: []uuid.UUID{uuid.New()}, ReportRefIDs: []uuid.UUID{uuid.New()},
	})
	if !errors.Is(err, ErrInvalidProjectAcceptance) {
		t.Fatalf("expected invalid acceptance refs, got %v", err)
	}
	if countProjectEvents(repo.eventTypes, ProjectEventAcceptanceSubmitted) != 0 {
		t.Fatalf("expected no acceptance event for invalid refs, got %#v", repo.eventTypes)
	}
}

func TestProjectAcceptanceSucceedsWithExistingEvidenceAndReportRefs(t *testing.T) {
	repo := newGovernanceMemoryRepository()
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	projectID := uuid.New()
	ownerID := uuid.New()
	repo.projects[projectID] = Project{ID: projectID, TenantID: tenantID, Status: ProjectStatusAcceptance, HumanOwnerUserID: ownerID}
	evidence, err := repo.CreateEvidenceRef(context.Background(), CreateEvidenceRefRequest{
		TenantID:           tenantID,
		ProjectID:          projectID,
		EvidenceType:       "test_result",
		Title:              "回归测试结果",
		SourceType:         "artifact",
		SourceRef:          "s3://bucket/reports/regression.json",
		SubmittedByType:    "human_user",
		SubmittedByID:      &ownerID,
		VerificationStatus: EvidenceVerificationStatusSubmitted,
	})
	if err != nil {
		t.Fatalf("seed evidence: %v", err)
	}
	report, err := repo.CreateReportRef(context.Background(), CreateReportRefRequest{
		TenantID:        tenantID,
		ProjectID:       projectID,
		ReportType:      "final_report",
		Title:           "验收报告",
		ObjectRef:       "s3://bucket/reports/final.md",
		Format:          "markdown",
		GeneratedByType: "human_user",
		GeneratedByID:   &ownerID,
	})
	if err != nil {
		t.Fatalf("seed report: %v", err)
	}

	acceptance, err := service.CreateAcceptanceRecord(context.Background(), CreateAcceptanceServiceRequest{
		TenantID: tenantID, ProjectID: projectID, AcceptedByUserID: ownerID,
		Status: "accepted", Conclusion: "通过", EvidenceRefIDs: []uuid.UUID{evidence.ID}, ReportRefIDs: []uuid.UUID{report.ID},
	})
	if err != nil {
		t.Fatalf("create acceptance: %v", err)
	}
	if acceptance.CreatedEventID == nil || countProjectEvents(repo.eventTypes, ProjectEventAcceptanceSubmitted) != 1 {
		t.Fatalf("expected acceptance event and record link, events=%#v acceptance=%#v", repo.eventTypes, acceptance)
	}
}

func TestProjectArchivePreviewCountsAllPages(t *testing.T) {
	repo := newGovernanceMemoryRepository()
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	projectID := uuid.New()
	ownerID := uuid.New()
	repo.projects[projectID] = Project{ID: projectID, TenantID: tenantID, Status: ProjectStatusAcceptance, HumanOwnerUserID: ownerID}

	for i := 0; i < 105; i++ {
		_, err := repo.CreateEvidenceRef(context.Background(), CreateEvidenceRefRequest{
			TenantID:           tenantID,
			ProjectID:          projectID,
			EvidenceType:       "test_result",
			Title:              fmt.Sprintf("证据 %d", i),
			SourceType:         "artifact",
			SourceRef:          fmt.Sprintf("s3://bucket/evidence/%d.json", i),
			SubmittedByType:    "human_user",
			SubmittedByID:      &ownerID,
			VerificationStatus: EvidenceVerificationStatusSubmitted,
		})
		if err != nil {
			t.Fatalf("seed evidence: %v", err)
		}
	}
	for i := 0; i < 103; i++ {
		_, err := repo.CreateArtifactRef(context.Background(), CreateArtifactRefRequest{
			TenantID:        tenantID,
			ProjectID:       projectID,
			ArtifactType:    "execution_log",
			Title:           fmt.Sprintf("工件 %d", i),
			ObjectRef:       fmt.Sprintf("s3://bucket/artifacts/%d.log", i),
			RetentionStatus: "locked",
		})
		if err != nil {
			t.Fatalf("seed artifact: %v", err)
		}
	}
	for i := 0; i < 102; i++ {
		_, err := repo.CreateReportRef(context.Background(), CreateReportRefRequest{
			TenantID:        tenantID,
			ProjectID:       projectID,
			ReportType:      "final_report",
			Title:           fmt.Sprintf("报告 %d", i),
			ObjectRef:       fmt.Sprintf("s3://bucket/reports/%d.md", i),
			Format:          "markdown",
			GeneratedByType: "human_user",
			GeneratedByID:   &ownerID,
		})
		if err != nil {
			t.Fatalf("seed report: %v", err)
		}
	}

	preview, err := service.BuildArchivePreview(context.Background(), tenantID, projectID)
	if err != nil {
		t.Fatalf("build archive preview: %v", err)
	}
	if preview.EvidenceCount != 105 || preview.ArtifactCount != 103 || preview.ReportCount != 102 {
		t.Fatalf("expected full counts, got evidence=%d artifact=%d report=%d", preview.EvidenceCount, preview.ArtifactCount, preview.ReportCount)
	}
}

func TestArchiveSnapshotLocksReferencedArtifactsBeforeArchiving(t *testing.T) {
	repo := newGovernanceMemoryRepository()
	lockEventID := uuid.New()
	locker := &fakeArchiveArtifactLocker{eventID: &lockEventID}
	service, err := NewServiceWithArchiveArtifactLocker(repo, locker)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	projectID := uuid.New()
	ownerID := uuid.New()
	artifactID := uuid.New()
	repo.projects[projectID] = Project{ID: projectID, TenantID: tenantID, Status: ProjectStatusAcceptance, HumanOwnerUserID: ownerID}
	repo.artifactRefs = append(repo.artifactRefs, ProjectArtifactRef{
		ID: uuid.New(), TenantID: tenantID, ProjectID: projectID, ArtifactID: &artifactID,
		ObjectRef: "s3://bucket/report.md", Title: "最终报告",
	})
	snapshot, err := service.CreateArchiveSnapshot(context.Background(), CreateArchiveSnapshotServiceRequest{
		TenantID: tenantID, ProjectID: projectID, CreatedByUserID: ownerID,
		SnapshotType: "final_archive", Summary: "验收通过后归档", ObjectRef: "s3://bucket/archive/project.json",
	})
	if err != nil {
		t.Fatalf("archive snapshot: %v", err)
	}
	if snapshot.Status != "archived" {
		t.Fatalf("expected archived snapshot, got %s", snapshot.Status)
	}
	if len(locker.artifactIDs) != 1 || locker.artifactIDs[0] != artifactID {
		t.Fatalf("expected artifact lock, got %#v", locker.artifactIDs)
	}
	if snapshot.RetentionLockEventID == nil || *snapshot.RetentionLockEventID != lockEventID {
		t.Fatalf("expected retention lock event id %s, got %#v", lockEventID, snapshot.RetentionLockEventID)
	}
	if repo.projects[projectID].Status != ProjectStatusArchived {
		t.Fatalf("expected project archived after retention lock, got %s", repo.projects[projectID].Status)
	}
}

func TestArchiveSnapshotStaysPendingWhenArtifactLockFails(t *testing.T) {
	repo := newGovernanceMemoryRepository()
	locker := &fakeArchiveArtifactLocker{err: errors.New("retention store unavailable")}
	service, err := NewServiceWithArchiveArtifactLocker(repo, locker)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	projectID := uuid.New()
	ownerID := uuid.New()
	artifactID := uuid.New()
	repo.projects[projectID] = Project{ID: projectID, TenantID: tenantID, Status: ProjectStatusAcceptance, HumanOwnerUserID: ownerID}
	repo.artifactRefs = append(repo.artifactRefs, ProjectArtifactRef{
		ID: uuid.New(), TenantID: tenantID, ProjectID: projectID, ArtifactID: &artifactID,
		ObjectRef: "s3://bucket/report.md", Title: "最终报告",
	})

	snapshot, err := service.CreateArchiveSnapshot(context.Background(), CreateArchiveSnapshotServiceRequest{
		TenantID: tenantID, ProjectID: projectID, CreatedByUserID: ownerID,
		SnapshotType: "final_archive", Summary: "验收通过后归档", ObjectRef: "s3://bucket/archive/project.json",
	})
	if err != nil {
		t.Fatalf("archive snapshot should return pending state without error: %v", err)
	}
	if snapshot.Status != "archive_pending_retention" {
		t.Fatalf("expected retention pending snapshot, got %s", snapshot.Status)
	}
	if repo.projects[projectID].Status == ProjectStatusArchived {
		t.Fatalf("project must not be archived when retention lock fails")
	}
	if len(repo.archiveSnapshots) != 1 || repo.archiveSnapshots[0].Status != "archive_pending_retention" {
		t.Fatalf("expected persisted pending snapshot, got %#v", repo.archiveSnapshots)
	}
}

func TestArchiveSnapshotReturnsArchiveProjectErrorAfterSuccessfulLock(t *testing.T) {
	repo := newGovernanceMemoryRepository()
	repo.archiveProjectErr = errors.New("archive update failed")
	locker := &fakeArchiveArtifactLocker{}
	service, err := NewServiceWithArchiveArtifactLocker(repo, locker)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	projectID := uuid.New()
	ownerID := uuid.New()
	artifactID := uuid.New()
	repo.projects[projectID] = Project{ID: projectID, TenantID: tenantID, Status: ProjectStatusAcceptance, HumanOwnerUserID: ownerID}
	repo.artifactRefs = append(repo.artifactRefs, ProjectArtifactRef{
		ID: uuid.New(), TenantID: tenantID, ProjectID: projectID, ArtifactID: &artifactID,
		ObjectRef: "s3://bucket/report.md", Title: "最终报告",
	})

	_, err = service.CreateArchiveSnapshot(context.Background(), CreateArchiveSnapshotServiceRequest{
		TenantID: tenantID, ProjectID: projectID, CreatedByUserID: ownerID,
		SnapshotType: "final_archive", Summary: "验收通过后归档", ObjectRef: "s3://bucket/archive/project.json",
	})
	if !errors.Is(err, repo.archiveProjectErr) {
		t.Fatalf("expected archive project error, got %v", err)
	}
	if len(repo.archiveSnapshots) != 0 {
		t.Fatalf("expected archived snapshot to roll back after archive project failure, got %#v", repo.archiveSnapshots)
	}
	if countProjectEvents(repo.eventTypes, ProjectEventArchiveSnapshotCreated) != 0 {
		t.Fatalf("expected archive snapshot event to roll back after archive project failure, got %#v", repo.eventTypes)
	}
	if repo.projects[projectID].Status == ProjectStatusArchived {
		t.Fatalf("project must not be marked archived when repository archive update fails")
	}
}

func TestSubmitDemandSignalsProjectCoordinatorInV1(t *testing.T) {
	repo := newMemoryRepository()
	coordinator := &fakeCoordinatorSignalClient{}
	service, err := NewServiceWithCoordinator(repo, coordinator)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	projectID := uuid.New()
	tenantID := uuid.New()
	ownerID := uuid.New()
	repo.projects[projectID] = Project{
		ID:                     projectID,
		TenantID:               tenantID,
		Name:                   "客户侧 Runtime 接入验收",
		Status:                 ProjectStatusRunning,
		HumanOwnerUserID:       ownerID,
		CoordinationWorkflowID: "project-coordinator:" + projectID.String(),
		CoordinationStatus:     "registered",
	}
	seedHumanOwnerMember(repo, tenantID, projectID, ownerID)

	demand, err := service.SubmitDemand(context.Background(), SubmitProjectDemandRequest{
		TenantID:          tenantID,
		ProjectID:         projectID,
		SubmittedByUserID: ownerID,
		Title:             "验证 Runtime 连接",
		Content:           "检查心跳和命令回写",
	})
	if err != nil {
		t.Fatalf("submit demand: %v", err)
	}
	if demand.Status != ProjectDemandStatusPlanningPending {
		t.Fatalf("expected planning pending demand, got %s", demand.Status)
	}
	if coordinator.demandSignals != 1 {
		t.Fatalf("expected one DemandSubmitted signal, got %d", coordinator.demandSignals)
	}
	if coordinator.ensureSignals != 1 {
		t.Fatalf("expected coordinator to be ensured before demand signal, got %d", coordinator.ensureSignals)
	}
	if coordinator.lastDemand.DemandID != demand.ID || coordinator.lastDemand.CreatedEventID == uuid.Nil {
		t.Fatalf("unexpected demand signal: %#v", coordinator.lastDemand)
	}
}

func TestSubmitDemandRecordsRetryableWorkflowSignalFailure(t *testing.T) {
	repo := newMemoryRepository()
	coordinator := &fakeCoordinatorSignalClient{demandSignalErr: errors.New("temporal unavailable")}
	service, err := NewServiceWithCoordinator(repo, coordinator)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	projectID := uuid.New()
	tenantID := uuid.New()
	ownerID := uuid.New()
	repo.projects[projectID] = Project{
		ID:                     projectID,
		TenantID:               tenantID,
		Name:                   "客户侧 Runtime 接入验收",
		Status:                 ProjectStatusRunning,
		HumanOwnerUserID:       ownerID,
		CoordinationWorkflowID: "project-coordinator:" + projectID.String(),
		CoordinationStatus:     "registered",
	}
	seedHumanOwnerMember(repo, tenantID, projectID, ownerID)

	_, err = service.SubmitDemand(context.Background(), SubmitProjectDemandRequest{
		TenantID:          tenantID,
		ProjectID:         projectID,
		SubmittedByUserID: ownerID,
		Title:             "验证 Runtime 连接",
		Content:           "检查心跳和命令回写",
	})
	if err == nil {
		t.Fatal("expected signal error")
	}
	if len(repo.eventTypes) != 2 || repo.eventTypes[1] != ProjectEventWorkflowSignaled {
		t.Fatalf("expected workflow signal failure event, got %#v", repo.eventTypes)
	}
	payload := repo.events[len(repo.events)-1].Payload
	if payload["signal_name"] != "DemandSubmitted" || payload["status"] != "failed" || payload["retryable"] != true {
		t.Fatalf("unexpected workflow signal payload: %#v", payload)
	}
	if payload["demand_id"] == "" || payload["error"] == "" {
		t.Fatalf("expected retry payload to include demand id and error: %#v", payload)
	}
}

func TestRetryWorkflowSignalReplaysFailedDemandSignal(t *testing.T) {
	repo := newMemoryRepository()
	coordinator := &fakeCoordinatorSignalClient{demandSignalErr: fmt.Errorf("temporal unavailable")}
	service, err := NewServiceWithCoordinator(repo, coordinator)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	projectID := uuid.New()
	ownerID := uuid.New()
	repo.projects[projectID] = Project{
		ID:                     projectID,
		TenantID:               tenantID,
		Name:                   "客户侧 Runtime 接入验收",
		Status:                 ProjectStatusRunning,
		HumanOwnerUserID:       ownerID,
		CoordinationWorkflowID: "project-coordinator:" + projectID.String(),
		CoordinationStatus:     "registered",
	}
	seedHumanOwnerMember(repo, tenantID, projectID, ownerID)

	_, err = service.SubmitDemand(context.Background(), SubmitProjectDemandRequest{
		TenantID:          tenantID,
		ProjectID:         projectID,
		SubmittedByUserID: ownerID,
		Title:             "验证 Runtime 连接",
		Content:           "检查心跳和命令回写",
	})
	if err == nil {
		t.Fatal("expected first signal error")
	}
	failedEvent := repo.events[len(repo.events)-1]
	coordinator.demandSignalErr = nil

	event, err := service.RetryWorkflowSignal(context.Background(), RetryWorkflowSignalRequest{
		TenantID:  tenantID,
		ProjectID: projectID,
		EventID:   failedEvent.ID,
		ActorID:   ownerID,
	})
	if err != nil {
		t.Fatalf("retry workflow signal: %v", err)
	}
	if repo.demands[0].CreatedEventID == nil {
		t.Fatalf("expected demand created event id: %#v", repo.demands[0])
	}
	if coordinator.demandSignals != 2 || coordinator.lastDemand.DemandID != repo.demands[0].ID || coordinator.lastDemand.CreatedEventID != *repo.demands[0].CreatedEventID {
		t.Fatalf("expected demand signal replay, count=%d signal=%#v demand=%#v", coordinator.demandSignals, coordinator.lastDemand, repo.demands[0])
	}
	if coordinator.ensureSignals != 2 {
		t.Fatalf("expected coordinator ensure before initial and retried demand signals, got %d", coordinator.ensureSignals)
	}
	if event.EventType != ProjectEventWorkflowSignaled || event.Payload["signal_name"] != "DemandSubmitted" || event.Payload["status"] != "sent" || event.Payload["retry_of_event_id"] != failedEvent.ID.String() {
		t.Fatalf("unexpected retry event: %#v", event)
	}
}

func TestParseEvidenceRefElement(t *testing.T) {
	if _, ok := parseEvidenceRefElement(map[string]any{"summary": "no ref"}); ok {
		t.Fatalf("expected element without source ref to be skipped")
	}
	if _, ok := parseEvidenceRefElement(42); ok {
		t.Fatalf("expected non string/map element to be skipped")
	}
	strParsed, ok := parseEvidenceRefElement("s3://bucket/report.md")
	if !ok || strParsed.SourceRef != "s3://bucket/report.md" || strParsed.Title != "s3://bucket/report.md" ||
		strParsed.EvidenceType != "execution_evidence" || strParsed.SourceType != "runtime_output" {
		t.Fatalf("unexpected string parse: %#v ok=%v", strParsed, ok)
	}
	mapParsed, ok := parseEvidenceRefElement(map[string]any{
		"ref": "doc-1", "title": "需求摘要文档", "summary": "v1.0", "type": "document", "source_type": "workspace_file",
	})
	if !ok || mapParsed.SourceRef != "doc-1" || mapParsed.Title != "需求摘要文档" || mapParsed.Summary != "v1.0" ||
		mapParsed.EvidenceType != "document" || mapParsed.SourceType != "workspace_file" {
		t.Fatalf("unexpected map parse: %#v ok=%v", mapParsed, ok)
	}
}

func TestParseArtifactRefElement(t *testing.T) {
	if _, ok := parseArtifactRefElement(map[string]any{"title": "no ref"}); ok {
		t.Fatalf("expected element without object ref to be skipped")
	}
	strParsed, ok := parseArtifactRefElement("artifact-1")
	if !ok || strParsed.ObjectRef != "artifact-1" || strParsed.Title != "artifact-1" || strParsed.ArtifactType != "execution_artifact" {
		t.Fatalf("unexpected string parse: %#v ok=%v", strParsed, ok)
	}
	mapParsed, ok := parseArtifactRefElement(map[string]any{"id": "plan-0.1", "title": "任务计划草案", "type": "plan", "content_type": "text/markdown"})
	if !ok || mapParsed.ObjectRef != "plan-0.1" || mapParsed.Title != "任务计划草案" || mapParsed.ArtifactType != "plan" || mapParsed.ContentType != "text/markdown" {
		t.Fatalf("unexpected map parse: %#v ok=%v", mapParsed, ok)
	}
}

func TestCompleteProjectTaskWritesSummaryAndSignalsCoordinator(t *testing.T) {
	repo := newMemoryRepository()
	coordinator := &fakeCoordinatorSignalClient{}
	service, err := NewServiceWithCoordinatorAndApprovals(repo, coordinator, nil)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	projectID := uuid.New()
	employeeID := uuid.New()
	taskID := uuid.New()
	runtimeNodeID := uuid.New()
	repo.projects[projectID] = Project{
		ID:                     projectID,
		TenantID:               tenantID,
		Name:                   "项目",
		Goal:                   "目标",
		Status:                 ProjectStatusRunning,
		HumanOwnerUserID:       uuid.New(),
		CoordinationWorkflowID: "project-coordinator:" + projectID.String(),
	}
	repo.tasks = append(repo.tasks, ProjectTask{
		ID:                        taskID,
		TenantID:                  tenantID,
		ProjectID:                 projectID,
		Title:                     "整理证据",
		Status:                    "assigned",
		AssignedDigitalEmployeeID: &employeeID,
	})
	bindTaskToRuntimeRun(repo, 0, runtimeNodeID)

	summary, err := service.CompleteProjectTask(context.Background(), CompleteProjectTaskRequest{
		TenantID:              tenantID,
		RuntimeNodeID:         runtimeNodeID,
		ProjectTaskID:         taskID,
		DigitalEmployeeID:     employeeID,
		Conclusion:            "证据充分",
		EvidenceRefs:          []any{"s3://bucket/report.md"},
		ArtifactRefs:          []any{"artifact-1"},
		ConfidenceFactors:     map[string]any{"tests": "passed"},
		RecommendedNextAction: "提交负责人验收",
	})
	if err != nil {
		t.Fatalf("complete project task: %v", err)
	}
	if summary.ProjectTaskID != taskID || summary.DigitalEmployeeID != employeeID {
		t.Fatalf("unexpected summary: %#v", summary)
	}
	if summary.CreatedEventID == nil {
		t.Fatalf("expected summary to reference created event: %#v", summary)
	}
	if repo.tasks[0].Status != "completed" {
		t.Fatalf("expected task completed, got %s", repo.tasks[0].Status)
	}
	if coordinator.completedSignals != 1 || coordinator.lastCompleted.ExecutionSummaryID != summary.ID {
		t.Fatalf("expected completed signal for summary, got count=%d signal=%#v", coordinator.completedSignals, coordinator.lastCompleted)
	}
	if countProjectEvents(repo.eventTypes, ProjectEventTaskCompleted) != 1 {
		t.Fatalf("expected one completed event, got %#v", repo.eventTypes)
	}
	if countProjectEvents(repo.eventTypes, ProjectEventEvidenceLinked) != 1 || countProjectEvents(repo.eventTypes, ProjectEventArtifactLinked) != 1 {
		t.Fatalf("expected evidence+artifact materialization events, got %#v", repo.eventTypes)
	}
}

func TestCompleteProjectTaskRejectsMissingRequiredEvidence(t *testing.T) {
	service, repo, coordinator, task, runtimeNodeID := newProjectServiceWritebackFixture(t, ProjectTask{
		ExpectedOutputs: []any{"execution_summary", "evidence_refs"},
		HandoffContract: map[string]any{},
	})

	_, err := service.CompleteProjectTask(context.Background(), CompleteProjectTaskRequest{
		TenantID:          task.TenantID,
		RuntimeNodeID:     runtimeNodeID,
		ProjectTaskID:     task.ID,
		DigitalEmployeeID: *task.AssignedDigitalEmployeeID,
		Conclusion:        "完成",
		EvidenceRefs:      nil,
	})

	require.ErrorIs(t, err, ErrInvalidProjectEvidence)
	require.Equal(t, "assigned", repo.tasks[0].Status)
	require.Len(t, repo.executionSummaries, 0)
	require.Equal(t, 0, coordinator.completedSignals)
	require.Equal(t, []ProjectEventType{ProjectEventTaskContractMissing}, repo.eventTypes)
	require.Equal(t, task.ID.String(), repo.events[0].Payload["project_task_id"])
	require.Equal(t, []any{"evidence_refs"}, repo.events[0].Payload["missing_outputs"])
}

func TestCompleteProjectTaskWithRequiredOutputsContractWritesSummaryAndSignalsCoordinator(t *testing.T) {
	service, repo, coordinator, task, runtimeNodeID := newProjectServiceWritebackFixture(t, ProjectTask{
		ExpectedOutputs: []any{"execution_summary", "evidence_refs", "artifact_refs", "recommended_next_action"},
		HandoffContract: map[string]any{},
	})

	summary, err := service.CompleteProjectTask(context.Background(), CompleteProjectTaskRequest{
		TenantID:              task.TenantID,
		RuntimeNodeID:         runtimeNodeID,
		ProjectTaskID:         task.ID,
		DigitalEmployeeID:     *task.AssignedDigitalEmployeeID,
		Conclusion:            "完成",
		EvidenceRefs:          []any{"evidence://report"},
		ArtifactRefs:          []any{"artifact://patch"},
		RecommendedNextAction: "提交验收",
	})

	require.NoError(t, err)
	require.Equal(t, task.ID, summary.ProjectTaskID)
	require.Equal(t, "completed", repo.tasks[0].Status)
	require.Len(t, repo.executionSummaries, 1)
	require.Equal(t, 1, coordinator.completedSignals)
	require.Equal(t, ProjectEventTaskCompleted, repo.eventTypes[0])
}

func TestCompleteProjectTaskMissingInformationContractRequiresExplicitArray(t *testing.T) {
	service, repo, coordinator, task, runtimeNodeID := newProjectServiceWritebackFixture(t, ProjectTask{
		ExpectedOutputs: []any{"execution_summary", "missing_information"},
		HandoffContract: map[string]any{},
	})

	_, err := service.CompleteProjectTask(context.Background(), CompleteProjectTaskRequest{
		TenantID:          task.TenantID,
		RuntimeNodeID:     runtimeNodeID,
		ProjectTaskID:     task.ID,
		DigitalEmployeeID: *task.AssignedDigitalEmployeeID,
		Conclusion:        "完成",
	})

	require.ErrorIs(t, err, ErrInvalidProjectEvidence)
	require.Equal(t, []ProjectEventType{ProjectEventTaskContractMissing}, repo.eventTypes)
	require.Equal(t, []any{"missing_information"}, repo.events[0].Payload["missing_outputs"])
	require.Len(t, repo.executionSummaries, 0)
	require.Equal(t, 0, coordinator.completedSignals)

	repo.eventTypes = nil
	repo.events = nil
	summary, err := service.CompleteProjectTask(context.Background(), CompleteProjectTaskRequest{
		TenantID:            task.TenantID,
		RuntimeNodeID:       runtimeNodeID,
		ProjectTaskID:       task.ID,
		DigitalEmployeeID:   *task.AssignedDigitalEmployeeID,
		Conclusion:          "完成",
		MissingInformation:  []any{},
		ConfidenceFactors:   map[string]any{"contract": "explicit_empty_missing_information"},
		RequiresHumanReview: false,
	})

	require.NoError(t, err)
	require.Equal(t, task.ID, summary.ProjectTaskID)
	require.Equal(t, ProjectEventTaskCompleted, repo.eventTypes[0])
}

func TestCompleteProjectTaskWorkProductsContractRequiresBoundRunProducts(t *testing.T) {
	service, repo, coordinator, task, runtimeNodeID := newProjectServiceWritebackFixture(t, ProjectTask{
		ExpectedOutputs: []any{"execution_summary", "work_products"},
		HandoffContract: map[string]any{},
	})

	_, err := service.CompleteProjectTask(context.Background(), CompleteProjectTaskRequest{
		TenantID:          task.TenantID,
		RuntimeNodeID:     runtimeNodeID,
		ProjectTaskID:     task.ID,
		DigitalEmployeeID: *task.AssignedDigitalEmployeeID,
		Conclusion:        "完成",
	})

	require.ErrorIs(t, err, ErrInvalidProjectEvidence)
	require.Equal(t, []ProjectEventType{ProjectEventTaskContractMissing}, repo.eventTypes)
	require.Equal(t, []any{"work_products"}, repo.events[0].Payload["missing_outputs"])
	require.Len(t, repo.executionSummaries, 0)
	require.Equal(t, 0, coordinator.completedSignals)

	repo.eventTypes = nil
	repo.events = nil
	repo.projectTaskRunWorkProducts[*task.DigitalEmployeeRunID] = []any{map[string]any{"ref": "wp://analysis", "title": "分析报告"}}
	summary, err := service.CompleteProjectTask(context.Background(), CompleteProjectTaskRequest{
		TenantID:          task.TenantID,
		RuntimeNodeID:     runtimeNodeID,
		ProjectTaskID:     task.ID,
		DigitalEmployeeID: *task.AssignedDigitalEmployeeID,
		Conclusion:        "完成",
	})

	require.NoError(t, err)
	require.Equal(t, task.ID, summary.ProjectTaskID)
	require.Equal(t, ProjectEventTaskCompleted, repo.eventTypes[0])
}

func TestCompleteProjectTaskHandoffContractRequiredRefsMissingCustomRefFails(t *testing.T) {
	service, repo, coordinator, task, runtimeNodeID := newProjectServiceWritebackFixture(t, ProjectTask{
		ExpectedOutputs: []any{"execution_summary"},
		HandoffContract: map[string]any{"required_refs": []any{"wp://analysis", "evidence://report"}},
	})
	repo.projectTaskRunWorkProducts[*task.DigitalEmployeeRunID] = []any{map[string]any{"ref": "wp://draft", "title": "草稿"}}

	_, err := service.CompleteProjectTask(context.Background(), CompleteProjectTaskRequest{
		TenantID:          task.TenantID,
		RuntimeNodeID:     runtimeNodeID,
		ProjectTaskID:     task.ID,
		DigitalEmployeeID: *task.AssignedDigitalEmployeeID,
		Conclusion:        "完成",
		EvidenceRefs:      []any{"evidence://report"},
	})

	require.ErrorIs(t, err, ErrInvalidProjectEvidence)
	require.Len(t, repo.executionSummaries, 0)
	require.Equal(t, 0, coordinator.completedSignals)
	require.Equal(t, []ProjectEventType{ProjectEventTaskContractMissing}, repo.eventTypes)
	require.Equal(t, []any{"wp://analysis"}, repo.events[0].Payload["missing_handoff_refs"])
}

func TestCompleteProjectTaskHandoffContractRequiredRefsMatchWorkProductFields(t *testing.T) {
	service, repo, coordinator, task, runtimeNodeID := newProjectServiceWritebackFixture(t, ProjectTask{
		ExpectedOutputs: []any{"execution_summary"},
		HandoffContract: map[string]any{"required_refs": []any{"wp://analysis", "分析报告", "report"}},
	})
	repo.projectTaskRunWorkProducts[*task.DigitalEmployeeRunID] = []any{map[string]any{
		"ref":   "wp://analysis",
		"title": "分析报告",
		"type":  "report",
	}}

	summary, err := service.CompleteProjectTask(context.Background(), CompleteProjectTaskRequest{
		TenantID:          task.TenantID,
		RuntimeNodeID:     runtimeNodeID,
		ProjectTaskID:     task.ID,
		DigitalEmployeeID: *task.AssignedDigitalEmployeeID,
		Conclusion:        "完成",
	})

	require.NoError(t, err)
	require.Equal(t, task.ID, summary.ProjectTaskID)
	require.Equal(t, "completed", repo.tasks[0].Status)
	require.Len(t, repo.executionSummaries, 1)
	require.Equal(t, 1, coordinator.completedSignals)
	require.Equal(t, ProjectEventTaskCompleted, repo.eventTypes[0])
}

func TestCompleteProjectTaskContractMissingEventAppendFailureReturnsAppendError(t *testing.T) {
	service, repo, coordinator, task, runtimeNodeID := newProjectServiceWritebackFixture(t, ProjectTask{
		ExpectedOutputs: []any{"execution_summary", "evidence_refs"},
		HandoffContract: map[string]any{},
	})
	appendErr := fmt.Errorf("event store unavailable")
	repo.appendProjectEventErr = appendErr

	_, err := service.CompleteProjectTask(context.Background(), CompleteProjectTaskRequest{
		TenantID:          task.TenantID,
		RuntimeNodeID:     runtimeNodeID,
		ProjectTaskID:     task.ID,
		DigitalEmployeeID: *task.AssignedDigitalEmployeeID,
		Conclusion:        "完成",
	})

	require.ErrorIs(t, err, appendErr)
	require.Len(t, repo.executionSummaries, 0)
	require.Equal(t, "assigned", repo.tasks[0].Status)
	require.Equal(t, 0, coordinator.completedSignals)
	require.Len(t, repo.eventTypes, 0)
}

func TestCompleteProjectTaskEmptyLegacyContractStillCompletes(t *testing.T) {
	service, repo, coordinator, task, runtimeNodeID := newProjectServiceWritebackFixture(t, ProjectTask{})

	summary, err := service.CompleteProjectTask(context.Background(), CompleteProjectTaskRequest{
		TenantID:          task.TenantID,
		RuntimeNodeID:     runtimeNodeID,
		ProjectTaskID:     task.ID,
		DigitalEmployeeID: *task.AssignedDigitalEmployeeID,
		Conclusion:        "完成",
	})

	require.NoError(t, err)
	require.Equal(t, task.ID, summary.ProjectTaskID)
	require.Equal(t, "completed", repo.tasks[0].Status)
	require.Len(t, repo.executionSummaries, 1)
	require.Equal(t, 1, coordinator.completedSignals)
	require.Equal(t, ProjectEventTaskCompleted, repo.eventTypes[0])
}

func newProjectServiceWritebackFixture(t *testing.T, taskOverrides ProjectTask) (*Service, *memoryRepository, *fakeCoordinatorSignalClient, ProjectTask, uuid.UUID) {
	t.Helper()

	repo := newMemoryRepository()
	coordinator := &fakeCoordinatorSignalClient{}
	service, err := NewServiceWithCoordinator(repo, coordinator)
	require.NoError(t, err)

	tenantID := uuid.New()
	projectID := uuid.New()
	employeeID := uuid.New()
	taskID := uuid.New()
	runtimeNodeID := uuid.New()
	repo.projects[projectID] = Project{
		ID:                     projectID,
		TenantID:               tenantID,
		Name:                   "项目",
		Goal:                   "目标",
		Status:                 ProjectStatusRunning,
		HumanOwnerUserID:       uuid.New(),
		CoordinationWorkflowID: "project-coordinator:" + projectID.String(),
	}
	task := ProjectTask{
		ID:                        taskID,
		TenantID:                  tenantID,
		ProjectID:                 projectID,
		Title:                     "整理证据",
		Status:                    "assigned",
		AssignedDigitalEmployeeID: &employeeID,
		ExpectedOutputs:           taskOverrides.ExpectedOutputs,
		HandoffContract:           taskOverrides.HandoffContract,
	}
	if task.HandoffContract == nil {
		task.HandoffContract = map[string]any{}
	}
	repo.tasks = append(repo.tasks, task)
	bindTaskToRuntimeRun(repo, 0, runtimeNodeID)
	task = repo.tasks[0]
	return service, repo, coordinator, task, runtimeNodeID
}

func TestBindProjectTaskRunEnablesCompleteProjectTaskWriteback(t *testing.T) {
	repo := newMemoryRepository()
	coordinator := &fakeCoordinatorSignalClient{}
	service, err := NewServiceWithCoordinator(repo, coordinator)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	projectID := uuid.New()
	employeeID := uuid.New()
	taskID := uuid.New()
	runID := uuid.New()
	runtimeTaskID := uuid.New()
	runtimeNodeID := uuid.New()
	repo.projects[projectID] = Project{
		ID:                     projectID,
		TenantID:               tenantID,
		Name:                   "项目",
		Goal:                   "目标",
		Status:                 ProjectStatusRunning,
		HumanOwnerUserID:       uuid.New(),
		CoordinationWorkflowID: "project-coordinator:" + projectID.String(),
	}
	repo.tasks = append(repo.tasks, ProjectTask{
		ID:                        taskID,
		TenantID:                  tenantID,
		ProjectID:                 projectID,
		Title:                     "整理证据",
		Status:                    "planned",
		AssignedDigitalEmployeeID: &employeeID,
	})

	bound, err := repo.BindProjectTaskRun(context.Background(), BindProjectTaskRunRequest{
		TenantID:             tenantID,
		ProjectTaskID:        taskID,
		DigitalEmployeeRunID: runID,
		RuntimeTaskID:        runtimeTaskID,
		CurrentStatuses:      []string{"planned", "pending"},
	})
	if err != nil {
		t.Fatalf("bind project task run: %v", err)
	}
	repo.projectTaskRunRuntimeNodes[taskID] = runtimeNodeID
	if bound.Status != "assigned" || bound.DigitalEmployeeRunID == nil || *bound.DigitalEmployeeRunID != runID ||
		bound.RuntimeTaskID == nil || *bound.RuntimeTaskID != runtimeTaskID {
		t.Fatalf("expected assigned run binding, got %#v", bound)
	}

	summary, err := service.CompleteProjectTask(context.Background(), CompleteProjectTaskRequest{
		TenantID:          tenantID,
		RuntimeNodeID:     runtimeNodeID,
		ProjectTaskID:     taskID,
		DigitalEmployeeID: employeeID,
		Conclusion:        "证据充分",
	})
	if err != nil {
		t.Fatalf("complete project task: %v", err)
	}
	if summary.ProjectTaskID != taskID || summary.DigitalEmployeeID != employeeID {
		t.Fatalf("unexpected summary: %#v", summary)
	}
	if repo.tasks[0].Status != "completed" {
		t.Fatalf("expected task completed after runtime writeback, got %s", repo.tasks[0].Status)
	}
	if coordinator.completedSignals != 1 {
		t.Fatalf("expected coordinator completion signal, got %d", coordinator.completedSignals)
	}
}

func TestRetryWorkflowSignalReplaysCompletedTaskWithoutDuplicateWriteback(t *testing.T) {
	repo := newMemoryRepository()
	coordinator := &fakeCoordinatorSignalClient{completedSignalErr: fmt.Errorf("temporal unavailable")}
	service, err := NewServiceWithCoordinator(repo, coordinator)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	projectID := uuid.New()
	employeeID := uuid.New()
	taskID := uuid.New()
	runtimeNodeID := uuid.New()
	repo.projects[projectID] = Project{
		ID:                     projectID,
		TenantID:               tenantID,
		Name:                   "项目",
		Goal:                   "目标",
		Status:                 ProjectStatusRunning,
		HumanOwnerUserID:       uuid.New(),
		CoordinationWorkflowID: "project-coordinator:" + projectID.String(),
	}
	repo.tasks = append(repo.tasks, ProjectTask{
		ID:                        taskID,
		TenantID:                  tenantID,
		ProjectID:                 projectID,
		Title:                     "整理证据",
		Status:                    "assigned",
		AssignedDigitalEmployeeID: &employeeID,
	})
	bindTaskToRuntimeRun(repo, 0, runtimeNodeID)

	_, err = service.CompleteProjectTask(context.Background(), CompleteProjectTaskRequest{
		TenantID:          tenantID,
		RuntimeNodeID:     runtimeNodeID,
		ProjectTaskID:     taskID,
		DigitalEmployeeID: employeeID,
		Conclusion:        "证据充分",
	})
	if err == nil {
		t.Fatal("expected first completed signal error")
	}
	if len(repo.executionSummaries) != 1 {
		t.Fatalf("expected one summary after failed signal, got %d", len(repo.executionSummaries))
	}
	completedEvents := countProjectEvents(repo.eventTypes, ProjectEventTaskCompleted)
	if completedEvents != 1 {
		t.Fatalf("expected one completed event after failed signal, got %d events=%#v", completedEvents, repo.eventTypes)
	}
	failedSignalEvent := repo.events[len(repo.events)-1]
	coordinator.completedSignalErr = nil

	retryEvent, err := service.RetryWorkflowSignal(context.Background(), RetryWorkflowSignalRequest{
		TenantID:  tenantID,
		ProjectID: projectID,
		EventID:   failedSignalEvent.ID,
		ActorID:   repo.projects[projectID].HumanOwnerUserID,
	})
	if err != nil {
		t.Fatalf("retry completed workflow signal: %v", err)
	}
	if coordinator.completedSignals != 2 || coordinator.lastCompleted.ProjectTaskID != taskID || coordinator.lastCompleted.ExecutionSummaryID != repo.executionSummaries[0].ID {
		t.Fatalf("expected completed signal replay, count=%d signal=%#v summary=%#v", coordinator.completedSignals, coordinator.lastCompleted, repo.executionSummaries[0])
	}
	if len(repo.executionSummaries) != 1 {
		t.Fatalf("expected retry not to create duplicate summary, got %d", len(repo.executionSummaries))
	}
	if countProjectEvents(repo.eventTypes, ProjectEventTaskCompleted) != 1 {
		t.Fatalf("expected retry not to create duplicate completed event, events=%#v", repo.eventTypes)
	}
	if retryEvent.Payload["status"] != "sent" || retryEvent.Payload["retry_of_event_id"] != failedSignalEvent.ID.String() {
		t.Fatalf("unexpected retry event payload: %#v", retryEvent.Payload)
	}
}

func TestProjectCoordinationBackendE2ESimulation(t *testing.T) {
	repo := newMemoryRepository()
	coordinator := &fakeCoordinatorSignalClient{demandSignalErr: fmt.Errorf("temporal unavailable")}
	service, err := NewServiceWithCoordinator(repo, coordinator)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	projectID := uuid.New()
	ownerID := uuid.New()
	employeeID := uuid.New()
	taskID := uuid.New()
	runtimeNodeID := uuid.New()
	repo.projects[projectID] = Project{
		ID:                     projectID,
		TenantID:               tenantID,
		Name:                   "E2E 仿真项目",
		Goal:                   "验证需求、Runtime 写回和 Workflow signal 重试闭环",
		Status:                 ProjectStatusRunning,
		HumanOwnerUserID:       ownerID,
		CoordinationWorkflowID: "project-coordinator:" + projectID.String(),
		CoordinationStatus:     "registered",
	}
	seedHumanOwnerMember(repo, tenantID, projectID, ownerID)

	_, err = service.SubmitDemand(context.Background(), SubmitProjectDemandRequest{
		TenantID:          tenantID,
		ProjectID:         projectID,
		SubmittedByUserID: ownerID,
		Title:             "验证 Runtime 执行回写",
		Content:           "模拟 Temporal 短暂不可用后的重试恢复",
	})
	if err == nil {
		t.Fatal("expected demand signal failure")
	}
	if len(repo.demands) != 1 || countProjectEvents(repo.eventTypes, ProjectEventDemandSubmitted) != 1 {
		t.Fatalf("expected one persisted demand before retry, demands=%d events=%#v", len(repo.demands), repo.eventTypes)
	}
	failedDemandSignalEvent := repo.events[len(repo.events)-1]
	if failedDemandSignalEvent.EventType != ProjectEventWorkflowSignaled || failedDemandSignalEvent.Payload["signal_name"] != "DemandSubmitted" || failedDemandSignalEvent.Payload["status"] != "failed" {
		t.Fatalf("expected retryable demand signal failure event, got %#v", failedDemandSignalEvent)
	}

	coordinator.demandSignalErr = nil
	retryDemandEvent, err := service.RetryWorkflowSignal(context.Background(), RetryWorkflowSignalRequest{
		TenantID:  tenantID,
		ProjectID: projectID,
		EventID:   failedDemandSignalEvent.ID,
		ActorID:   ownerID,
	})
	if err != nil {
		t.Fatalf("retry demand workflow signal: %v", err)
	}
	if retryDemandEvent.Payload["status"] != "sent" || retryDemandEvent.Payload["retry_of_event_id"] != failedDemandSignalEvent.ID.String() {
		t.Fatalf("unexpected demand retry event payload: %#v", retryDemandEvent.Payload)
	}
	if coordinator.demandSignals != 2 || len(repo.demands) != 1 || countProjectEvents(repo.eventTypes, ProjectEventDemandSubmitted) != 1 {
		t.Fatalf("expected demand retry to only resend signal, signals=%d demands=%d events=%#v", coordinator.demandSignals, len(repo.demands), repo.eventTypes)
	}

	repo.tasks = append(repo.tasks, ProjectTask{
		ID:                        taskID,
		TenantID:                  tenantID,
		ProjectID:                 projectID,
		Title:                     "整理执行证据",
		Status:                    "assigned",
		AssignedDigitalEmployeeID: &employeeID,
	})
	bindTaskToRuntimeRun(repo, 0, runtimeNodeID)

	_, err = service.CompleteProjectTask(context.Background(), CompleteProjectTaskRequest{
		TenantID:          tenantID,
		RuntimeNodeID:     uuid.New(),
		ProjectTaskID:     taskID,
		DigitalEmployeeID: employeeID,
		Conclusion:        "错误 Runtime 尝试写回",
	})
	if !errors.Is(err, ErrProjectTaskForbidden) {
		t.Fatalf("expected wrong runtime rejection, got %v", err)
	}
	if repo.tasks[0].Status != "assigned" || len(repo.executionSummaries) != 0 || countProjectEvents(repo.eventTypes, ProjectEventTaskCompleted) != 0 {
		t.Fatalf("expected rejected runtime writeback to have no side effects, task=%#v summaries=%d events=%#v", repo.tasks[0], len(repo.executionSummaries), repo.eventTypes)
	}

	coordinator.completedSignalErr = fmt.Errorf("temporal unavailable")
	_, err = service.CompleteProjectTask(context.Background(), CompleteProjectTaskRequest{
		TenantID:              tenantID,
		RuntimeNodeID:         runtimeNodeID,
		ProjectTaskID:         taskID,
		DigitalEmployeeID:     employeeID,
		Conclusion:            "证据充分",
		EvidenceRefs:          []any{"s3://bucket/e2e-report.md"},
		ArtifactRefs:          []any{"artifact-runtime-log"},
		ConfidenceFactors:     map[string]any{"tests": "passed"},
		RecommendedNextAction: "提交负责人验收",
	})
	if err == nil {
		t.Fatal("expected completed task signal failure")
	}
	if repo.tasks[0].Status != "completed" || len(repo.executionSummaries) != 1 || countProjectEvents(repo.eventTypes, ProjectEventTaskCompleted) != 1 {
		t.Fatalf("expected successful writeback before signal retry, task=%#v summaries=%d events=%#v", repo.tasks[0], len(repo.executionSummaries), repo.eventTypes)
	}
	failedCompletedSignalEvent := repo.events[len(repo.events)-1]
	if failedCompletedSignalEvent.EventType != ProjectEventWorkflowSignaled || failedCompletedSignalEvent.Payload["signal_name"] != "EmployeeTaskCompleted" || failedCompletedSignalEvent.Payload["status"] != "failed" {
		t.Fatalf("expected retryable completed signal failure event, got %#v", failedCompletedSignalEvent)
	}

	coordinator.completedSignalErr = nil
	retryCompletedEvent, err := service.RetryWorkflowSignal(context.Background(), RetryWorkflowSignalRequest{
		TenantID:  tenantID,
		ProjectID: projectID,
		EventID:   failedCompletedSignalEvent.ID,
		ActorID:   ownerID,
	})
	if err != nil {
		t.Fatalf("retry completed workflow signal: %v", err)
	}
	if retryCompletedEvent.Payload["status"] != "sent" || retryCompletedEvent.Payload["retry_of_event_id"] != failedCompletedSignalEvent.ID.String() {
		t.Fatalf("unexpected completed retry event payload: %#v", retryCompletedEvent.Payload)
	}
	if coordinator.completedSignals != 2 || coordinator.lastCompleted.ExecutionSummaryID != repo.executionSummaries[0].ID {
		t.Fatalf("expected completed signal replay, signals=%d last=%#v summary=%#v", coordinator.completedSignals, coordinator.lastCompleted, repo.executionSummaries[0])
	}
	if len(repo.executionSummaries) != 1 || countProjectEvents(repo.eventTypes, ProjectEventTaskCompleted) != 1 {
		t.Fatalf("expected completed retry not to duplicate facts, summaries=%d events=%#v", len(repo.executionSummaries), repo.eventTypes)
	}

	demands, err := service.ListProjectDemands(context.Background(), tenantID, projectID, 50, 0)
	if err != nil {
		t.Fatalf("list demands: %v", err)
	}
	summaries, err := service.ListExecutionSummaries(context.Background(), tenantID, projectID, 50, 0)
	if err != nil {
		t.Fatalf("list execution summaries: %v", err)
	}
	events, err := service.ListProjectEvents(context.Background(), tenantID, projectID, 50, 0)
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	if len(demands) != 1 || len(summaries) != 1 || countProjectEvents(projectEventTypes(events), ProjectEventWorkflowSignaled) != 4 {
		t.Fatalf("unexpected API-facing read model: demands=%d summaries=%d events=%#v", len(demands), len(summaries), projectEventTypes(events))
	}
}

func TestProjectTaskWritebackRequiresRuntimeNodeIdentity(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewServiceWithCoordinator(repo, &fakeCoordinatorSignalClient{})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	projectID := uuid.New()
	employeeID := uuid.New()
	taskID := uuid.New()
	repo.projects[projectID] = Project{
		ID:                     projectID,
		TenantID:               tenantID,
		Name:                   "项目",
		Goal:                   "目标",
		Status:                 ProjectStatusRunning,
		HumanOwnerUserID:       uuid.New(),
		CoordinationWorkflowID: "project-coordinator:" + projectID.String(),
	}
	repo.tasks = append(repo.tasks, ProjectTask{
		ID:                        taskID,
		TenantID:                  tenantID,
		ProjectID:                 projectID,
		Title:                     "整理证据",
		Status:                    "assigned",
		AssignedDigitalEmployeeID: &employeeID,
	})

	_, err = service.CompleteProjectTask(context.Background(), CompleteProjectTaskRequest{
		TenantID:          tenantID,
		ProjectTaskID:     taskID,
		DigitalEmployeeID: employeeID,
		Conclusion:        "证据充分",
	})
	if !errors.Is(err, ErrProjectTaskForbidden) {
		t.Fatalf("expected runtime identity rejection, got %v", err)
	}
	if len(repo.executionSummaries) != 0 || len(repo.eventTypes) != 0 {
		t.Fatalf("expected rejection before side effects, summaries=%d events=%#v", len(repo.executionSummaries), repo.eventTypes)
	}
}

func TestProjectTaskWritebackRequiresDigitalEmployeeRunBinding(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewServiceWithCoordinator(repo, &fakeCoordinatorSignalClient{})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	projectID := uuid.New()
	employeeID := uuid.New()
	taskID := uuid.New()
	repo.projects[projectID] = Project{
		ID:                     projectID,
		TenantID:               tenantID,
		Name:                   "项目",
		Goal:                   "目标",
		Status:                 ProjectStatusRunning,
		HumanOwnerUserID:       uuid.New(),
		CoordinationWorkflowID: "project-coordinator:" + projectID.String(),
	}
	repo.tasks = append(repo.tasks, ProjectTask{
		ID:                        taskID,
		TenantID:                  tenantID,
		ProjectID:                 projectID,
		Title:                     "整理证据",
		Status:                    "assigned",
		AssignedDigitalEmployeeID: &employeeID,
	})

	_, err = service.CompleteProjectTask(context.Background(), CompleteProjectTaskRequest{
		TenantID:          tenantID,
		RuntimeNodeID:     uuid.New(),
		ProjectTaskID:     taskID,
		DigitalEmployeeID: employeeID,
		Conclusion:        "未绑定运行记录",
	})
	if !errors.Is(err, ErrProjectTaskForbidden) {
		t.Fatalf("expected missing run binding rejection, got %v", err)
	}
	if len(repo.executionSummaries) != 0 || len(repo.eventTypes) != 0 {
		t.Fatalf("expected rejection before side effects, summaries=%d events=%#v", len(repo.executionSummaries), repo.eventTypes)
	}
}

func TestBindProjectTaskRunRejectsSameRunDifferentRuntimeTask(t *testing.T) {
	repo := newMemoryRepository()
	tenantID := uuid.New()
	projectID := uuid.New()
	taskID := uuid.New()
	runID := uuid.New()
	originalRuntimeTaskID := uuid.New()
	repo.tasks = append(repo.tasks, ProjectTask{
		ID:                   taskID,
		TenantID:             tenantID,
		ProjectID:            projectID,
		Title:                "整理证据",
		Status:               "assigned",
		DigitalEmployeeRunID: &runID,
		RuntimeTaskID:        &originalRuntimeTaskID,
	})

	_, err := repo.BindProjectTaskRun(context.Background(), BindProjectTaskRunRequest{
		TenantID:             tenantID,
		ProjectTaskID:        taskID,
		DigitalEmployeeRunID: runID,
		RuntimeTaskID:        uuid.New(),
		CurrentStatuses:      []string{"pending", "running"},
	})
	if !errors.Is(err, ErrProjectConflict) {
		t.Fatalf("expected project conflict, got %v", err)
	}
	if repo.tasks[0].RuntimeTaskID == nil || *repo.tasks[0].RuntimeTaskID != originalRuntimeTaskID {
		t.Fatalf("expected runtime task id to remain unchanged, got %#v", repo.tasks[0].RuntimeTaskID)
	}
}

func TestCompleteProjectTaskRejectsTerminalReplay(t *testing.T) {
	repo := newMemoryRepository()
	coordinator := &fakeCoordinatorSignalClient{}
	service, err := NewServiceWithCoordinator(repo, coordinator)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	projectID := uuid.New()
	employeeID := uuid.New()
	taskID := uuid.New()
	repo.projects[projectID] = Project{
		ID:                     projectID,
		TenantID:               tenantID,
		Name:                   "项目",
		Goal:                   "目标",
		Status:                 ProjectStatusRunning,
		HumanOwnerUserID:       uuid.New(),
		CoordinationWorkflowID: "project-coordinator:" + projectID.String(),
	}
	repo.tasks = append(repo.tasks, ProjectTask{
		ID:                        taskID,
		TenantID:                  tenantID,
		ProjectID:                 projectID,
		Title:                     "整理证据",
		Status:                    "completed",
		AssignedDigitalEmployeeID: &employeeID,
	})

	_, err = service.CompleteProjectTask(context.Background(), CompleteProjectTaskRequest{
		TenantID:          tenantID,
		RuntimeNodeID:     uuid.New(),
		ProjectTaskID:     taskID,
		DigitalEmployeeID: employeeID,
		Conclusion:        "重复完成",
	})
	if !errors.Is(err, ErrProjectTaskForbidden) {
		t.Fatalf("expected terminal replay rejection, got %v", err)
	}
	if len(repo.executionSummaries) != 0 || len(repo.eventTypes) != 0 || coordinator.completedSignals != 0 {
		t.Fatalf("expected rejection before side effects, summaries=%d events=%#v signals=%d", len(repo.executionSummaries), repo.eventTypes, coordinator.completedSignals)
	}
}

func TestCompleteProjectTaskRejectsConcurrentTerminalTransitionBeforeSideEffects(t *testing.T) {
	repo := newMemoryRepository()
	coordinator := &fakeCoordinatorSignalClient{}
	service, err := NewServiceWithCoordinator(repo, coordinator)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	projectID := uuid.New()
	employeeID := uuid.New()
	taskID := uuid.New()
	runtimeNodeID := uuid.New()
	completed := "completed"
	repo.taskStatusBeforeUpdate = &completed
	repo.projects[projectID] = Project{
		ID:                     projectID,
		TenantID:               tenantID,
		Name:                   "项目",
		Goal:                   "目标",
		Status:                 ProjectStatusRunning,
		HumanOwnerUserID:       uuid.New(),
		CoordinationWorkflowID: "project-coordinator:" + projectID.String(),
	}
	repo.tasks = append(repo.tasks, ProjectTask{
		ID:                        taskID,
		TenantID:                  tenantID,
		ProjectID:                 projectID,
		Title:                     "整理证据",
		Status:                    "assigned",
		AssignedDigitalEmployeeID: &employeeID,
	})
	bindTaskToRuntimeRun(repo, 0, runtimeNodeID)

	_, err = service.CompleteProjectTask(context.Background(), CompleteProjectTaskRequest{
		TenantID:          tenantID,
		RuntimeNodeID:     runtimeNodeID,
		ProjectTaskID:     taskID,
		DigitalEmployeeID: employeeID,
		Conclusion:        "并发完成",
	})
	if !errors.Is(err, ErrProjectNotFound) {
		t.Fatalf("expected conditional status update rejection, got %v", err)
	}
	if len(repo.executionSummaries) != 0 || len(repo.eventTypes) != 0 || coordinator.completedSignals != 0 {
		t.Fatalf("expected rejection before side effects, summaries=%d events=%#v signals=%d", len(repo.executionSummaries), repo.eventTypes, coordinator.completedSignals)
	}
}

func TestCompleteProjectTaskRollsBackStatusWhenSummaryCreationFails(t *testing.T) {
	repo := newMemoryRepository()
	coordinator := &fakeCoordinatorSignalClient{}
	service, err := NewServiceWithCoordinator(repo, coordinator)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	projectID := uuid.New()
	employeeID := uuid.New()
	taskID := uuid.New()
	runtimeNodeID := uuid.New()
	repo.createExecutionSummaryErr = fmt.Errorf("summary unavailable")
	repo.projects[projectID] = Project{
		ID:                     projectID,
		TenantID:               tenantID,
		Name:                   "项目",
		Goal:                   "目标",
		Status:                 ProjectStatusRunning,
		HumanOwnerUserID:       uuid.New(),
		CoordinationWorkflowID: "project-coordinator:" + projectID.String(),
	}
	repo.tasks = append(repo.tasks, ProjectTask{
		ID:                        taskID,
		TenantID:                  tenantID,
		ProjectID:                 projectID,
		Title:                     "整理证据",
		Status:                    "assigned",
		AssignedDigitalEmployeeID: &employeeID,
	})
	bindTaskToRuntimeRun(repo, 0, runtimeNodeID)

	_, err = service.CompleteProjectTask(context.Background(), CompleteProjectTaskRequest{
		TenantID:          tenantID,
		RuntimeNodeID:     runtimeNodeID,
		ProjectTaskID:     taskID,
		DigitalEmployeeID: employeeID,
		Conclusion:        "写摘要失败",
	})
	if err == nil {
		t.Fatal("expected summary creation error")
	}
	if repo.tasks[0].Status != "assigned" || len(repo.executionSummaries) != 0 || len(repo.eventTypes) != 0 || coordinator.completedSignals != 0 {
		t.Fatalf("expected rollback before side effects, task=%#v summaries=%d events=%#v signals=%d", repo.tasks[0], len(repo.executionSummaries), repo.eventTypes, coordinator.completedSignals)
	}
}

func TestFailProjectTaskRollsBackStatusWhenEventAppendFails(t *testing.T) {
	repo := newMemoryRepository()
	coordinator := &fakeCoordinatorSignalClient{}
	service, err := NewServiceWithCoordinator(repo, coordinator)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	projectID := uuid.New()
	employeeID := uuid.New()
	taskID := uuid.New()
	runtimeNodeID := uuid.New()
	repo.appendProjectEventErr = fmt.Errorf("event store unavailable")
	repo.projects[projectID] = Project{
		ID:                     projectID,
		TenantID:               tenantID,
		Name:                   "项目",
		Goal:                   "目标",
		Status:                 ProjectStatusRunning,
		HumanOwnerUserID:       uuid.New(),
		CoordinationWorkflowID: "project-coordinator:" + projectID.String(),
	}
	repo.tasks = append(repo.tasks, ProjectTask{
		ID:                        taskID,
		TenantID:                  tenantID,
		ProjectID:                 projectID,
		Title:                     "整理证据",
		Status:                    "running",
		AssignedDigitalEmployeeID: &employeeID,
	})
	bindTaskToRuntimeRun(repo, 0, runtimeNodeID)

	_, err = service.FailProjectTask(context.Background(), FailProjectTaskRequest{
		TenantID:          tenantID,
		RuntimeNodeID:     runtimeNodeID,
		ProjectTaskID:     taskID,
		DigitalEmployeeID: employeeID,
		FailureSummary:    "工具链失败",
	})
	if err == nil {
		t.Fatal("expected event append error")
	}
	if repo.tasks[0].Status != "running" || len(repo.eventTypes) != 0 || coordinator.failedSignals != 0 {
		t.Fatalf("expected rollback before side effects, task=%#v events=%#v signals=%d", repo.tasks[0], repo.eventTypes, coordinator.failedSignals)
	}
}

func TestRequestProjectTaskTransferRollsBackStatusWhenTransferCreationFails(t *testing.T) {
	repo := newMemoryRepository()
	coordinator := &fakeCoordinatorSignalClient{}
	service, err := NewServiceWithCoordinator(repo, coordinator)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	projectID := uuid.New()
	employeeID := uuid.New()
	taskID := uuid.New()
	runtimeNodeID := uuid.New()
	repo.createTransferRequestErr = fmt.Errorf("transfer store unavailable")
	repo.projects[projectID] = Project{
		ID:                     projectID,
		TenantID:               tenantID,
		Name:                   "项目",
		Goal:                   "目标",
		Status:                 ProjectStatusRunning,
		HumanOwnerUserID:       uuid.New(),
		CoordinationWorkflowID: "project-coordinator:" + projectID.String(),
	}
	repo.tasks = append(repo.tasks, ProjectTask{
		ID:                        taskID,
		TenantID:                  tenantID,
		ProjectID:                 projectID,
		Title:                     "整理证据",
		Status:                    "assigned",
		AssignedDigitalEmployeeID: &employeeID,
	})
	bindTaskToRuntimeRun(repo, 0, runtimeNodeID)

	_, err = service.RequestProjectTaskTransfer(context.Background(), RequestProjectTaskTransferRequest{
		TenantID:          tenantID,
		RuntimeNodeID:     runtimeNodeID,
		ProjectTaskID:     taskID,
		DigitalEmployeeID: employeeID,
		Reason:            "上下文不足",
	})
	if err == nil {
		t.Fatal("expected transfer creation error")
	}
	if repo.tasks[0].Status != "assigned" || len(repo.transferRequests) != 0 || len(repo.eventTypes) != 0 || coordinator.transferSignals != 0 {
		t.Fatalf("expected rollback before side effects, task=%#v transfers=%d events=%#v signals=%d", repo.tasks[0], len(repo.transferRequests), repo.eventTypes, coordinator.transferSignals)
	}
}

func TestCompleteProjectTaskRejectsWrongRuntimeWhenRunIsBound(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewServiceWithCoordinator(repo, &fakeCoordinatorSignalClient{})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	projectID := uuid.New()
	employeeID := uuid.New()
	taskID := uuid.New()
	runID := uuid.New()
	expectedRuntimeNodeID := uuid.New()
	repo.projectTaskRunRuntimeNodes[taskID] = expectedRuntimeNodeID
	repo.projects[projectID] = Project{
		ID:                     projectID,
		TenantID:               tenantID,
		Name:                   "项目",
		Goal:                   "目标",
		Status:                 ProjectStatusRunning,
		HumanOwnerUserID:       uuid.New(),
		CoordinationWorkflowID: "project-coordinator:" + projectID.String(),
	}
	repo.tasks = append(repo.tasks, ProjectTask{
		ID:                        taskID,
		TenantID:                  tenantID,
		ProjectID:                 projectID,
		Title:                     "整理证据",
		Status:                    "assigned",
		AssignedDigitalEmployeeID: &employeeID,
		DigitalEmployeeRunID:      &runID,
	})

	_, err = service.CompleteProjectTask(context.Background(), CompleteProjectTaskRequest{
		TenantID:          tenantID,
		RuntimeNodeID:     uuid.New(),
		ProjectTaskID:     taskID,
		DigitalEmployeeID: employeeID,
		Conclusion:        "错误 Runtime 写回",
	})
	if !errors.Is(err, ErrProjectTaskForbidden) {
		t.Fatalf("expected wrong runtime rejection, got %v", err)
	}
	if len(repo.executionSummaries) != 0 || len(repo.eventTypes) != 0 {
		t.Fatalf("expected rejection before side effects, summaries=%d events=%#v", len(repo.executionSummaries), repo.eventTypes)
	}
}

func TestRequestProjectTaskTransferRejectsWaitingHumanTask(t *testing.T) {
	repo := newMemoryRepository()
	coordinator := &fakeCoordinatorSignalClient{}
	service, err := NewServiceWithCoordinator(repo, coordinator)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	projectID := uuid.New()
	employeeID := uuid.New()
	taskID := uuid.New()
	repo.projects[projectID] = Project{
		ID:                     projectID,
		TenantID:               tenantID,
		Name:                   "项目",
		Goal:                   "目标",
		Status:                 ProjectStatusRunning,
		HumanOwnerUserID:       uuid.New(),
		CoordinationWorkflowID: "project-coordinator:" + projectID.String(),
	}
	repo.tasks = append(repo.tasks, ProjectTask{
		ID:                        taskID,
		TenantID:                  tenantID,
		ProjectID:                 projectID,
		Title:                     "等待负责人确认",
		Status:                    "waiting_human",
		AssignedDigitalEmployeeID: &employeeID,
	})

	_, err = service.RequestProjectTaskTransfer(context.Background(), RequestProjectTaskTransferRequest{
		TenantID:          tenantID,
		RuntimeNodeID:     uuid.New(),
		ProjectTaskID:     taskID,
		DigitalEmployeeID: employeeID,
		Reason:            "上下文不足",
	})
	if !errors.Is(err, ErrProjectTaskForbidden) {
		t.Fatalf("expected waiting human transfer rejection, got %v", err)
	}
	if len(repo.transferRequests) != 0 || len(repo.eventTypes) != 0 || coordinator.transferSignals != 0 {
		t.Fatalf("expected rejection before side effects, transfers=%d events=%#v signals=%d", len(repo.transferRequests), repo.eventTypes, coordinator.transferSignals)
	}
}

func TestRequestProjectTaskTransferMovesTaskToWaitingHuman(t *testing.T) {
	repo := newMemoryRepository()
	coordinator := &fakeCoordinatorSignalClient{}
	service, err := NewServiceWithCoordinator(repo, coordinator)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	projectID := uuid.New()
	employeeID := uuid.New()
	taskID := uuid.New()
	runtimeNodeID := uuid.New()
	repo.projects[projectID] = Project{
		ID:                     projectID,
		TenantID:               tenantID,
		Name:                   "项目",
		Goal:                   "目标",
		Status:                 ProjectStatusRunning,
		HumanOwnerUserID:       uuid.New(),
		CoordinationWorkflowID: "project-coordinator:" + projectID.String(),
	}
	repo.tasks = append(repo.tasks, ProjectTask{
		ID:                        taskID,
		TenantID:                  tenantID,
		ProjectID:                 projectID,
		Title:                     "整理证据",
		Status:                    "assigned",
		AssignedDigitalEmployeeID: &employeeID,
	})
	bindTaskToRuntimeRun(repo, 0, runtimeNodeID)

	transfer, err := service.RequestProjectTaskTransfer(context.Background(), RequestProjectTaskTransferRequest{
		TenantID:          tenantID,
		RuntimeNodeID:     runtimeNodeID,
		ProjectTaskID:     taskID,
		DigitalEmployeeID: employeeID,
		Reason:            "上下文不足",
	})
	if err != nil {
		t.Fatalf("request transfer: %v", err)
	}
	if transfer.Status != "requested" || repo.tasks[0].Status != "waiting_human" {
		t.Fatalf("expected transfer to pause task, transfer=%#v task=%#v", transfer, repo.tasks[0])
	}
	if coordinator.transferSignals != 1 {
		t.Fatalf("expected transfer signal, got %d", coordinator.transferSignals)
	}
}

func TestResolveDecisionUsesApprovalAndSignalsCoordinator(t *testing.T) {
	repo := newMemoryRepository()
	coordinator := &fakeCoordinatorSignalClient{}
	approvals := &fakeApprovalResolver{}
	service, err := NewServiceWithCoordinatorAndApprovals(repo, coordinator, approvals)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	projectID := uuid.New()
	decisionID := uuid.New()
	approvalID := uuid.New()
	actorID := uuid.New()
	repo.projects[projectID] = Project{
		ID:                     projectID,
		TenantID:               tenantID,
		Name:                   "项目",
		Goal:                   "目标",
		Status:                 ProjectStatusRunning,
		HumanOwnerUserID:       actorID,
		CoordinationWorkflowID: "project-coordinator:" + projectID.String(),
	}
	repo.decisionRequests = append(repo.decisionRequests, DecisionRequest{
		ID:                decisionID,
		TenantID:          tenantID,
		ProjectID:         projectID,
		ApprovalRequestID: approvalID,
		TargetUserID:      actorID,
		DecisionType:      "route_review",
		TitleSnapshot:     "需要负责人确认",
		StatusSnapshot:    "pending",
	})

	resolved, err := service.ResolveDecision(context.Background(), ResolveDecisionRequest{
		TenantID:          tenantID,
		ProjectID:         projectID,
		DecisionRequestID: decisionID,
		DecidedByUserID:   actorID,
		Decision:          "approved",
		Comment:           "同意",
		Payload:           map[string]any{"source": "console"},
	})
	if err != nil {
		t.Fatalf("resolve decision: %v", err)
	}
	if resolved.StatusSnapshot != "approved" {
		t.Fatalf("expected approved projection, got %s", resolved.StatusSnapshot)
	}
	if approvals.calls != 1 || approvals.last.ApprovalRequestID != approvalID || approvals.last.Decision != "approved" {
		t.Fatalf("expected approval resolver call, got count=%d last=%#v", approvals.calls, approvals.last)
	}
	if approvals.last.Payload["source"] != "console" {
		t.Fatalf("expected approval payload to be preserved, got %#v", approvals.last.Payload)
	}
	if coordinator.decisionSignals != 1 || coordinator.lastDecision.DecisionRequestID != decisionID || coordinator.lastDecision.ResolvedEventID == uuid.Nil {
		t.Fatalf("expected decision signal, got count=%d signal=%#v", coordinator.decisionSignals, coordinator.lastDecision)
	}
	if coordinator.lastDecision.Payload["source"] != "console" {
		t.Fatalf("expected decision signal payload to be preserved, got %#v", coordinator.lastDecision.Payload)
	}
}

func TestResolveDecisionSkipsApprovalResolverForProjectOnlyDecision(t *testing.T) {
	repo := newMemoryRepository()
	coordinator := &fakeCoordinatorSignalClient{}
	approvals := &fakeApprovalResolver{}
	service, err := NewServiceWithCoordinatorAndApprovals(repo, coordinator, approvals)
	require.NoError(t, err)
	tenantID := uuid.New()
	projectID := uuid.New()
	decisionID := uuid.New()
	actorID := uuid.New()
	repo.projects[projectID] = Project{
		ID:                     projectID,
		TenantID:               tenantID,
		Name:                   "项目",
		Goal:                   "目标",
		Status:                 ProjectStatusRunning,
		HumanOwnerUserID:       actorID,
		CoordinationWorkflowID: "project-coordinator:" + projectID.String(),
	}
	repo.decisionRequests = append(repo.decisionRequests, DecisionRequest{
		ID:             decisionID,
		TenantID:       tenantID,
		ProjectID:      projectID,
		TargetUserID:   actorID,
		DecisionType:   "project_task_acceptance",
		TitleSnapshot:  "任务验收",
		StatusSnapshot: "pending",
	})

	resolved, err := service.ResolveDecision(context.Background(), ResolveDecisionRequest{
		TenantID:          tenantID,
		ProjectID:         projectID,
		DecisionRequestID: decisionID,
		DecidedByUserID:   actorID,
		Decision:          "approved",
	})

	require.NoError(t, err)
	require.Equal(t, "approved", resolved.StatusSnapshot)
	require.Equal(t, 0, approvals.calls)
	require.Equal(t, 1, coordinator.decisionSignals)
	require.Equal(t, uuid.Nil, coordinator.lastDecision.ApprovalRequestID)
}

func TestResolveDecisionApprovedProjectTaskAcceptanceCompletesWaitingTask(t *testing.T) {
	repo := newMemoryRepository()
	coordinator := &fakeCoordinatorSignalClient{}
	approvals := &fakeApprovalResolver{}
	service, err := NewServiceWithCoordinatorAndApprovals(repo, coordinator, approvals)
	require.NoError(t, err)
	fixture := newProjectTaskAttemptServiceFixture(repo, ProjectTaskStatusWaitingHuman, ProjectTaskAttemptStatusSucceeded)
	actorID := repo.projects[fixture.projectID].HumanOwnerUserID
	decisionID := uuid.New()
	reason := HumanWaitReasonAcceptanceRequired
	repo.tasks[0].WaitingReason = &reason
	repo.tasks[0].WaitingRequestID = &decisionID
	repo.decisionRequests = append(repo.decisionRequests, DecisionRequest{
		ID:             decisionID,
		TenantID:       fixture.tenantID,
		ProjectID:      fixture.projectID,
		ProjectTaskID:  &fixture.taskID,
		TargetUserID:   actorID,
		DecisionType:   "project_task_acceptance",
		TitleSnapshot:  "任务验收",
		StatusSnapshot: "pending",
	})

	resolved, err := service.ResolveDecision(context.Background(), ResolveDecisionRequest{
		TenantID:          fixture.tenantID,
		ProjectID:         fixture.projectID,
		DecisionRequestID: decisionID,
		DecidedByUserID:   actorID,
		Decision:          "approved",
		Comment:           "验收通过",
	})

	require.NoError(t, err)
	require.Equal(t, "approved", resolved.StatusSnapshot)
	task, err := repo.GetProjectTask(context.Background(), fixture.tenantID, fixture.taskID)
	require.NoError(t, err)
	require.Equal(t, ProjectTaskStatusCompleted, task.Status)
	require.Equal(t, 0, approvals.calls)
	require.Equal(t, 1, coordinator.decisionSignals)
}

func TestResolveDecisionIsIdempotentForSameResolvedDecision(t *testing.T) {
	repo := newMemoryRepository()
	coordinator := &fakeCoordinatorSignalClient{}
	approvals := &fakeApprovalResolver{}
	service, err := NewServiceWithCoordinatorAndApprovals(repo, coordinator, approvals)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	projectID := uuid.New()
	decisionID := uuid.New()
	approvalID := uuid.New()
	actorID := uuid.New()
	resolvedEventID := uuid.New()
	resolvedAt := time.Now().UTC()
	repo.projects[projectID] = Project{
		ID:                     projectID,
		TenantID:               tenantID,
		Name:                   "项目",
		Goal:                   "目标",
		Status:                 ProjectStatusArchived,
		HumanOwnerUserID:       actorID,
		CoordinationWorkflowID: "project-coordinator:" + projectID.String(),
	}
	repo.decisionRequests = append(repo.decisionRequests, DecisionRequest{
		ID:                decisionID,
		TenantID:          tenantID,
		ProjectID:         projectID,
		ApprovalRequestID: approvalID,
		TargetUserID:      actorID,
		DecisionType:      "project_acceptance",
		TitleSnapshot:     "验收项目交付",
		StatusSnapshot:    "approved",
		ResolvedEventID:   &resolvedEventID,
		ResolvedAt:        &resolvedAt,
	})

	resolved, err := service.ResolveDecision(context.Background(), ResolveDecisionRequest{
		TenantID:          tenantID,
		ProjectID:         projectID,
		DecisionRequestID: decisionID,
		DecidedByUserID:   actorID,
		Decision:          "approved",
	})

	if err != nil {
		t.Fatalf("resolve decision replay: %v", err)
	}
	if resolved.ID != decisionID || resolved.StatusSnapshot != "approved" || resolved.ResolvedEventID == nil || *resolved.ResolvedEventID != resolvedEventID {
		t.Fatalf("expected existing resolved decision, got %#v", resolved)
	}
	if approvals.calls != 0 || coordinator.decisionSignals != 0 || len(repo.events) != 0 {
		t.Fatalf("expected idempotent replay without side effects, approvals=%d signals=%d events=%d", approvals.calls, coordinator.decisionSignals, len(repo.events))
	}
}

func TestRetryWorkflowSignalReplaysHumanDecisionPayload(t *testing.T) {
	repo := newMemoryRepository()
	coordinator := &fakeCoordinatorSignalClient{}
	service, err := NewServiceWithCoordinator(repo, coordinator)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	projectID := uuid.New()
	ownerID := uuid.New()
	approvalID := uuid.New()
	decisionID := uuid.New()
	resolvedEventID := uuid.New()
	repo.projects[projectID] = Project{
		ID:                     projectID,
		TenantID:               tenantID,
		Name:                   "项目",
		Goal:                   "目标",
		Status:                 ProjectStatusRunning,
		HumanOwnerUserID:       ownerID,
		CoordinationWorkflowID: "project-coordinator:" + projectID.String(),
	}
	failedEvent, err := repo.AppendProjectEvent(context.Background(), AppendProjectEventRequest{
		TenantID:  tenantID,
		ProjectID: projectID,
		EventType: ProjectEventWorkflowSignaled,
		ActorType: "control_plane",
		ActorID:   "project_service",
		Summary:   "Workflow signal 状态已记录",
		Payload: map[string]any{
			"signal_name":         "HumanDecisionSubmitted",
			"status":              "failed",
			"retryable":           true,
			"approval_request_id": approvalID.String(),
			"decision_request_id": decisionID.String(),
			"resolved_event_id":   resolvedEventID.String(),
			"decision":            "approved",
			"payload":             map[string]any{"recovery_action": "retry"},
		},
	})
	if err != nil {
		t.Fatalf("seed failed workflow signal: %v", err)
	}

	event, err := service.RetryWorkflowSignal(context.Background(), RetryWorkflowSignalRequest{
		TenantID:  tenantID,
		ProjectID: projectID,
		EventID:   failedEvent.ID,
		ActorID:   ownerID,
	})

	if err != nil {
		t.Fatalf("retry human decision workflow signal: %v", err)
	}
	if coordinator.decisionSignals != 1 || coordinator.lastDecision.DecisionRequestID != decisionID || coordinator.lastDecision.ResolvedEventID != resolvedEventID {
		t.Fatalf("expected human decision signal replay, count=%d signal=%#v", coordinator.decisionSignals, coordinator.lastDecision)
	}
	if coordinator.lastDecision.Payload["recovery_action"] != "retry" {
		t.Fatalf("expected human decision payload replay, got %#v", coordinator.lastDecision.Payload)
	}
	if event.EventType != ProjectEventWorkflowSignaled || event.Payload["status"] != "sent" || event.Payload["retry_of_event_id"] != failedEvent.ID.String() {
		t.Fatalf("unexpected retry event: %#v", event)
	}
}

func TestResolveDecisionProjectsInboxResolution(t *testing.T) {
	repo := newMemoryRepository()
	coordinator := &fakeCoordinatorSignalClient{}
	approvals := &fakeApprovalResolver{}
	inbox := &fakeDecisionInboxProjector{}
	service, err := NewServiceWithCoordinatorApprovalsInboxAndArchiveArtifactLocker(repo, coordinator, approvals, inbox, nil)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	projectID := uuid.New()
	decisionID := uuid.New()
	approvalID := uuid.New()
	actorID := uuid.New()
	repo.projects[projectID] = Project{
		ID:                     projectID,
		TenantID:               tenantID,
		Name:                   "项目",
		Goal:                   "目标",
		Status:                 ProjectStatusRunning,
		HumanOwnerUserID:       actorID,
		CoordinationWorkflowID: "project-coordinator:" + projectID.String(),
	}
	repo.decisionRequests = append(repo.decisionRequests, DecisionRequest{
		ID:                decisionID,
		TenantID:          tenantID,
		ProjectID:         projectID,
		ApprovalRequestID: approvalID,
		TargetUserID:      actorID,
		DecisionType:      "route_review",
		TitleSnapshot:     "需要负责人确认",
		StatusSnapshot:    "pending",
	})

	resolved, err := service.ResolveDecision(context.Background(), ResolveDecisionRequest{
		TenantID:          tenantID,
		ProjectID:         projectID,
		DecisionRequestID: decisionID,
		DecidedByUserID:   actorID,
		Decision:          "approved",
	})
	if err != nil {
		t.Fatalf("resolve decision: %v", err)
	}
	if len(inbox.resolutions) != 1 || inbox.resolutions[0].ID != decisionID || inbox.resolutions[0].StatusSnapshot != "approved" || inbox.resolutions[0].ResolvedEventID == nil {
		t.Fatalf("expected inbox resolution projection, got %#v", inbox.resolutions)
	}
	if resolved.ID != decisionID || coordinator.decisionSignals != 1 {
		t.Fatalf("expected resolved decision and coordinator signal, resolved=%#v signals=%d", resolved, coordinator.decisionSignals)
	}

	projectionErr := errors.New("inbox unavailable")
	failingRepo := newMemoryRepository()
	failingCoordinator := &fakeCoordinatorSignalClient{}
	failingApprovals := &fakeApprovalResolver{}
	failingInbox := &fakeDecisionInboxProjector{resolveErr: projectionErr}
	failingService, err := NewServiceWithCoordinatorApprovalsInboxAndArchiveArtifactLocker(failingRepo, failingCoordinator, failingApprovals, failingInbox, nil)
	if err != nil {
		t.Fatalf("new failing service: %v", err)
	}
	failingProjectID := uuid.New()
	failingDecisionID := uuid.New()
	failingRepo.projects[failingProjectID] = Project{
		ID:                     failingProjectID,
		TenantID:               tenantID,
		Name:                   "项目",
		Goal:                   "目标",
		Status:                 ProjectStatusRunning,
		HumanOwnerUserID:       actorID,
		CoordinationWorkflowID: "project-coordinator:" + failingProjectID.String(),
	}
	failingRepo.decisionRequests = append(failingRepo.decisionRequests, DecisionRequest{
		ID:                failingDecisionID,
		TenantID:          tenantID,
		ProjectID:         failingProjectID,
		ApprovalRequestID: uuid.New(),
		TargetUserID:      actorID,
		DecisionType:      "route_review",
		TitleSnapshot:     "需要负责人确认",
		StatusSnapshot:    "pending",
	})
	if _, err := failingService.ResolveDecision(context.Background(), ResolveDecisionRequest{
		TenantID:          tenantID,
		ProjectID:         failingProjectID,
		DecisionRequestID: failingDecisionID,
		DecidedByUserID:   actorID,
		Decision:          "approved",
	}); !errors.Is(err, projectionErr) {
		t.Fatalf("expected projector error, got %v", err)
	}
	if failingCoordinator.decisionSignals != 0 {
		t.Fatalf("expected no coordinator signal after projection failure, got %d", failingCoordinator.decisionSignals)
	}
}

func TestResolveDecisionFindsDecisionBeyondFirstPage(t *testing.T) {
	repo := newMemoryRepository()
	coordinator := &fakeCoordinatorSignalClient{}
	approvals := &fakeApprovalResolver{}
	service, err := NewServiceWithCoordinatorAndApprovals(repo, coordinator, approvals)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	projectID := uuid.New()
	actorID := uuid.New()
	targetDecisionID := uuid.New()
	targetApprovalID := uuid.New()
	repo.projects[projectID] = Project{
		ID:                     projectID,
		TenantID:               tenantID,
		Name:                   "项目",
		Goal:                   "目标",
		Status:                 ProjectStatusRunning,
		HumanOwnerUserID:       actorID,
		CoordinationWorkflowID: "project-coordinator:" + projectID.String(),
	}
	for i := 0; i < 100; i++ {
		repo.decisionRequests = append(repo.decisionRequests, DecisionRequest{
			ID:                uuid.New(),
			TenantID:          tenantID,
			ProjectID:         projectID,
			ApprovalRequestID: uuid.New(),
			TargetUserID:      actorID,
			DecisionType:      "route_review",
			TitleSnapshot:     "较新的决策",
			StatusSnapshot:    "pending",
			CreatedAt:         time.Now().UTC().Add(time.Duration(i+1) * time.Minute),
		})
	}
	repo.decisionRequests = append(repo.decisionRequests, DecisionRequest{
		ID:                targetDecisionID,
		TenantID:          tenantID,
		ProjectID:         projectID,
		ApprovalRequestID: targetApprovalID,
		TargetUserID:      actorID,
		DecisionType:      "route_review",
		TitleSnapshot:     "较早的决策",
		StatusSnapshot:    "pending",
		CreatedAt:         time.Now().UTC().Add(-time.Hour),
	})

	resolved, err := service.ResolveDecision(context.Background(), ResolveDecisionRequest{
		TenantID:          tenantID,
		ProjectID:         projectID,
		DecisionRequestID: targetDecisionID,
		DecidedByUserID:   actorID,
		Decision:          "approved",
	})
	if err != nil {
		t.Fatalf("resolve older decision: %v", err)
	}
	if resolved.ID != targetDecisionID || approvals.last.ApprovalRequestID != targetApprovalID {
		t.Fatalf("expected target decision to resolve, decision=%#v approval=%#v", resolved, approvals.last)
	}
}

func TestUpdateConfigRejectsArchivedProject(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	projectID := uuid.New()
	repo.projects[projectID] = Project{
		ID:               projectID,
		TenantID:         uuid.New(),
		Name:             "已归档项目",
		Status:           ProjectStatusArchived,
		HumanOwnerUserID: uuid.New(),
	}
	_, err = service.UpdateProjectConfig(context.Background(), UpdateProjectConfigRequest{
		TenantID:    repo.projects[projectID].TenantID,
		ProjectID:   projectID,
		ActorUserID: uuid.New(),
		Name:        "新名称",
	})
	if !errors.Is(err, ErrProjectArchived) {
		t.Fatalf("expected archived error, got %v", err)
	}
}

func TestUpdateProjectConfigCreatesRevision(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	projectID := uuid.New()
	tenantID := uuid.New()
	repo.projects[projectID] = Project{
		ID:               projectID,
		TenantID:         tenantID,
		Name:             "旧项目",
		Goal:             "旧目标",
		Status:           ProjectStatusRunning,
		HumanOwnerUserID: uuid.New(),
	}

	updated, err := service.UpdateProjectConfig(context.Background(), UpdateProjectConfigRequest{
		TenantID:    tenantID,
		ProjectID:   projectID,
		ActorUserID: uuid.New(),
		Name:        "新项目",
		Goal:        "新目标",
	})
	if err != nil {
		t.Fatalf("update config: %v", err)
	}
	if updated.Name != "新项目" {
		t.Fatalf("expected updated project name, got %q", updated.Name)
	}
	if len(repo.revisions) != 1 {
		t.Fatalf("expected config revision, got %d", len(repo.revisions))
	}
	if len(repo.eventTypes) != 1 || repo.eventTypes[0] != ProjectEventConfigChanged {
		t.Fatalf("expected config changed event, got %#v", repo.eventTypes)
	}
}

func TestUpdateProjectConfigRecordsRetryableWorkflowSignalFailure(t *testing.T) {
	repo := newMemoryRepository()
	coordinator := &fakeCoordinatorSignalClient{policySignalErr: errors.New("temporal unavailable")}
	service, err := NewServiceWithCoordinator(repo, coordinator)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	projectID := uuid.New()
	tenantID := uuid.New()
	repo.projects[projectID] = Project{
		ID:                     projectID,
		TenantID:               tenantID,
		Name:                   "旧项目",
		Goal:                   "旧目标",
		Status:                 ProjectStatusRunning,
		HumanOwnerUserID:       uuid.New(),
		CoordinationWorkflowID: "project-coordinator:" + projectID.String(),
	}

	_, err = service.UpdateProjectConfig(context.Background(), UpdateProjectConfigRequest{
		TenantID:    tenantID,
		ProjectID:   projectID,
		ActorUserID: uuid.New(),
		Name:        "新项目",
		Goal:        "新目标",
	})
	if err == nil {
		t.Fatal("expected signal error")
	}
	if len(repo.eventTypes) != 2 || repo.eventTypes[1] != ProjectEventWorkflowSignaled {
		t.Fatalf("expected workflow signal failure event, got %#v", repo.eventTypes)
	}
	payload := repo.events[len(repo.events)-1].Payload
	if payload["signal_name"] != "ProjectPolicyChanged" || payload["status"] != "failed" || payload["retryable"] != true {
		t.Fatalf("unexpected workflow signal payload: %#v", payload)
	}
	if payload["changed_event_id"] == "" || payload["error"] == "" {
		t.Fatalf("expected retry payload to include event id and error: %#v", payload)
	}
}

func TestUpdateProjectConfigRejectsMissingIDs(t *testing.T) {
	service, err := NewService(newMemoryRepository())
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	for _, tc := range []struct {
		name string
		req  UpdateProjectConfigRequest
	}{
		{name: "tenant", req: UpdateProjectConfigRequest{ProjectID: uuid.New(), ActorUserID: uuid.New()}},
		{name: "project", req: UpdateProjectConfigRequest{TenantID: uuid.New(), ActorUserID: uuid.New()}},
		{name: "actor", req: UpdateProjectConfigRequest{TenantID: uuid.New(), ProjectID: uuid.New()}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := service.UpdateProjectConfig(context.Background(), tc.req)
			if !errors.Is(err, ErrInvalidProject) {
				t.Fatalf("expected invalid project error, got %v", err)
			}
		})
	}
}

func TestUpdateProjectConfigWithoutMembersPreservesExistingMembers(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	projectID := uuid.New()
	tenantID := uuid.New()
	memberID := uuid.New()
	repo.projects[projectID] = Project{
		ID:               projectID,
		TenantID:         tenantID,
		Name:             "旧项目",
		Goal:             "旧目标",
		Status:           ProjectStatusRunning,
		HumanOwnerUserID: memberID,
	}
	repo.members[projectID] = []ProjectMember{{
		ID:            uuid.New(),
		TenantID:      tenantID,
		ProjectID:     projectID,
		PrincipalType: PrincipalTypeHumanUser,
		PrincipalID:   memberID,
		ProjectRole:   ProjectRoleOwner,
		Status:        "active",
	}}

	_, err = service.UpdateProjectConfig(context.Background(), UpdateProjectConfigRequest{
		TenantID:    tenantID,
		ProjectID:   projectID,
		ActorUserID: uuid.New(),
		Name:        " 新项目 ",
	})
	if err != nil {
		t.Fatalf("update config: %v", err)
	}
	if got := repo.projects[projectID].Name; got != "新项目" {
		t.Fatalf("expected trimmed name, got %q", got)
	}
	if len(repo.members[projectID]) != 1 {
		t.Fatalf("expected members to be preserved, got %d", len(repo.members[projectID]))
	}
}

func TestUpdateProjectConfigRejectsUnauthorizedTeamMembers(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	projectID := uuid.New()
	tenantID := uuid.New()
	actorUserID := uuid.New()
	allowedTeamID := uuid.New()
	unauthorizedTeamID := uuid.New()
	repo.projects[projectID] = Project{
		ID:               projectID,
		TenantID:         tenantID,
		Name:             "旧项目",
		Goal:             "旧目标",
		Status:           ProjectStatusRunning,
		HumanOwnerUserID: actorUserID,
	}
	repo.members[projectID] = []ProjectMember{{
		ID:            uuid.New(),
		TenantID:      tenantID,
		ProjectID:     projectID,
		PrincipalType: PrincipalTypeTeam,
		PrincipalID:   allowedTeamID,
		ProjectRole:   ProjectRoleObserver,
		Status:        "active",
	}}
	repo.authorizeProjectTeamScope(tenantID, actorUserID, allowedTeamID)

	updatedMembers := []ProjectMemberInput{{
		PrincipalType: PrincipalTypeTeam,
		PrincipalID:   unauthorizedTeamID,
		ProjectRole:   ProjectRoleObserver,
	}}
	_, err = service.UpdateProjectConfig(context.Background(), UpdateProjectConfigRequest{
		TenantID:    tenantID,
		ProjectID:   projectID,
		ActorUserID: actorUserID,
		Members:     &updatedMembers,
	})
	if !errors.Is(err, ErrUnauthorizedProjectTeamScope) {
		t.Fatalf("expected unauthorized team scope, got %v", err)
	}
	if got := repo.members[projectID][0].PrincipalID; got != allowedTeamID {
		t.Fatalf("expected existing members unchanged, got %s", got)
	}
}

func TestReplaceProjectMembersRequiresActorAndRecordsEvent(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	projectID := uuid.New()
	tenantID := uuid.New()
	repo.projects[projectID] = Project{
		ID:               projectID,
		TenantID:         tenantID,
		Name:             "项目",
		Goal:             "目标",
		Status:           ProjectStatusRunning,
		HumanOwnerUserID: uuid.New(),
	}

	_, err = service.ReplaceProjectMembers(context.Background(), tenantID, projectID, uuid.Nil, nil)
	if !errors.Is(err, ErrInvalidProject) {
		t.Fatalf("expected invalid project error, got %v", err)
	}

	members, err := service.ReplaceProjectMembers(context.Background(), tenantID, projectID, uuid.New(), []ProjectMemberInput{{
		PrincipalType: PrincipalTypeDigitalEmployee,
		PrincipalID:   uuid.New(),
		ProjectRole:   ProjectRoleExecutor,
	}})
	if err != nil {
		t.Fatalf("replace members: %v", err)
	}
	if len(members) != 1 {
		t.Fatalf("expected one member, got %d", len(members))
	}
	if len(repo.eventTypes) != 1 || repo.eventTypes[0] != ProjectEventConfigChanged {
		t.Fatalf("expected config changed event, got %#v", repo.eventTypes)
	}
	if got := repo.events[0].Payload["member_count"]; got != 1 {
		t.Fatalf("expected member_count payload, got %#v", got)
	}
}

func TestListPaginationIsNormalized(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	projectID := uuid.New()
	tenantID := uuid.New()
	repo.projects[projectID] = Project{
		ID:               projectID,
		TenantID:         tenantID,
		Name:             "项目",
		Goal:             "目标",
		Status:           ProjectStatusRunning,
		HumanOwnerUserID: uuid.New(),
	}

	if _, err := service.ListProjects(context.Background(), ListProjectsRequest{TenantID: tenantID, Limit: 200, Offset: -5}); err != nil {
		t.Fatalf("list projects: %v", err)
	}
	if repo.lastListProjects.Limit != 100 || repo.lastListProjects.Offset != 0 {
		t.Fatalf("expected projects pagination 100/0, got %d/%d", repo.lastListProjects.Limit, repo.lastListProjects.Offset)
	}
	if _, err := service.ListProjectEvents(context.Background(), tenantID, projectID, 0, -1); err != nil {
		t.Fatalf("list events: %v", err)
	}
	if repo.lastEventsLimit != 50 || repo.lastEventsOffset != 0 {
		t.Fatalf("expected events pagination 50/0, got %d/%d", repo.lastEventsLimit, repo.lastEventsOffset)
	}
	if _, err := service.ListProjectDemands(context.Background(), tenantID, projectID, 101, -2); err != nil {
		t.Fatalf("list demands: %v", err)
	}
	if repo.lastDemandsLimit != 100 || repo.lastDemandsOffset != 0 {
		t.Fatalf("expected demands pagination 100/0, got %d/%d", repo.lastDemandsLimit, repo.lastDemandsOffset)
	}
	if _, err := service.GetOverview(context.Background(), tenantID, projectID); err != nil {
		t.Fatalf("get overview: %v", err)
	}
	if repo.lastTasksLimit != 20 || repo.lastTasksOffset != 0 || repo.lastEventsLimit != 20 || repo.lastEventsOffset != 0 {
		t.Fatalf("expected overview pagination 20/0, got tasks %d/%d events %d/%d", repo.lastTasksLimit, repo.lastTasksOffset, repo.lastEventsLimit, repo.lastEventsOffset)
	}
}

type memoryRepository struct {
	projects                         map[uuid.UUID]Project
	members                          map[uuid.UUID][]ProjectMember
	tasks                            []ProjectTask
	projectTaskAttempts              []ProjectTaskAttempt
	dispatchGateResults              []PreDispatchGateResult
	events                           []ProjectEvent
	eventTypes                       []ProjectEventType
	demands                          []ProjectDemand
	revisions                        []ProjectConfigRevision
	coordinationJobs                 []CoordinationJob
	routeDecisions                   []RouteDecision
	executionSummaries               []ExecutionSummary
	executionLedgerEvents            []ExecutionLedgerEvent
	projectTaskResults               []ProjectTaskResult
	transferRequests                 []TransferRequest
	decisionRequests                 []DecisionRequest
	contextUpdates                   []ProjectTaskAttemptContextUpdate
	evidenceRefs                     []ProjectEvidenceRef
	artifactRefs                     []ProjectArtifactRef
	reportRefs                       []ProjectReportRef
	budgetLedger                     []ProjectBudgetLedgerEntry
	acceptanceRecords                []ProjectAcceptanceRecord
	archiveSnapshots                 []ProjectArchiveSnapshot
	projectTeamScopes                map[uuid.UUID]map[uuid.UUID]map[uuid.UUID]bool
	lastListProjects                 ListProjectsRequest
	lastTasksLimit                   int32
	lastTasksOffset                  int32
	lastEventsLimit                  int32
	lastEventsOffset                 int32
	lastDemandsLimit                 int32
	lastDemandsOffset                int32
	lastExecutionSummariesLimit      int32
	lastExecutionSummariesOffset     int32
	executionLedgerEventListRequests []GetExecutionTraceRequest

	taskStatusBeforeUpdate        *string
	appendProjectEventErr         error
	createExecutionSummaryErr     error
	createExecutionLedgerEventErr error
	createTransferRequestErr      error
	archiveProjectErr             error
	projectTaskRunRuntimeNodes    map[uuid.UUID]uuid.UUID
	projectTaskRunWorkProducts    map[uuid.UUID][]any
}

type projectTaskResultMemoryRepository struct {
	*memoryRepository
	recordProjectTaskResultErr     error
	linkProjectTaskLatestResultErr error
}

type repositoryWithoutProjectTeamScopeAuthorizer struct {
	Repository
}

type taskGraphLimitRepository struct {
	*memoryRepository
	calls   int
	lastReq GetProjectTaskGraphRequest
	graph   ProjectTaskGraph
}

type workflowInstanceServiceRepository struct {
	*memoryRepository
	calls   int
	lastReq ListWorkflowInstancesRequest
	items   []WorkflowInstanceSummary
}

func newMemoryRepository() *memoryRepository {
	return &memoryRepository{
		projects:                   map[uuid.UUID]Project{},
		members:                    map[uuid.UUID][]ProjectMember{},
		projectTeamScopes:          map[uuid.UUID]map[uuid.UUID]map[uuid.UUID]bool{},
		projectTaskRunRuntimeNodes: map[uuid.UUID]uuid.UUID{},
		projectTaskRunWorkProducts: map[uuid.UUID][]any{},
	}
}

func newProjectTaskResultMemoryRepository() *projectTaskResultMemoryRepository {
	return &projectTaskResultMemoryRepository{memoryRepository: newMemoryRepository()}
}

func ptrUUIDValue(id uuid.UUID) *uuid.UUID {
	return &id
}

func (r *memoryRepository) authorizeProjectTeamScope(tenantID, userID, teamID uuid.UUID) {
	if r.projectTeamScopes == nil {
		r.projectTeamScopes = map[uuid.UUID]map[uuid.UUID]map[uuid.UUID]bool{}
	}
	if r.projectTeamScopes[tenantID] == nil {
		r.projectTeamScopes[tenantID] = map[uuid.UUID]map[uuid.UUID]bool{}
	}
	if r.projectTeamScopes[tenantID][userID] == nil {
		r.projectTeamScopes[tenantID][userID] = map[uuid.UUID]bool{}
	}
	r.projectTeamScopes[tenantID][userID][teamID] = true
}

func (r *memoryRepository) CanUseTeamForProject(ctx context.Context, tenantID, userID, teamID uuid.UUID) (bool, error) {
	return r.projectTeamScopes[tenantID][userID][teamID], nil
}

func assertNoCreateProjectSideEffects(t *testing.T, repo *memoryRepository, coordinator *fakeCoordinatorSignalClient) {
	t.Helper()
	if len(repo.projects) != 0 || len(repo.members) != 0 || len(repo.events) != 0 || len(repo.eventTypes) != 0 || coordinator.ensureSignals != 0 {
		t.Fatalf("expected rejection before side effects, projects=%d members=%d events=%d eventTypes=%#v ensureSignals=%d", len(repo.projects), len(repo.members), len(repo.events), repo.eventTypes, coordinator.ensureSignals)
	}
}

func (r *taskGraphLimitRepository) GetProjectTaskGraph(ctx context.Context, req GetProjectTaskGraphRequest) (ProjectTaskGraph, error) {
	r.calls++
	r.lastReq = req
	if r.graph.Nodes != nil {
		return r.graph, nil
	}
	count := 55
	if req.Limit > 0 && int(req.Limit) < count {
		count = int(req.Limit)
	}
	nodes := make([]ProjectTaskGraphNode, 0, count)
	for i := 0; i < count; i++ {
		nodes = append(nodes, ProjectTaskGraphNode{
			Task: ProjectTask{
				ID:        uuid.New(),
				TenantID:  req.TenantID,
				ProjectID: req.ProjectID,
				Title:     fmt.Sprintf("graph task %02d", i+1),
				Status:    "planned",
			},
		})
	}
	return ProjectTaskGraph{
		Nodes:              nodes,
		Edges:              []ProjectTaskGraphEdge{},
		Employees:          []ProjectTaskGraphEmployee{},
		Runs:               []ProjectTaskGraphRun{},
		ExecutionSummaries: []ExecutionSummary{},
		RecentEvents:       []ProjectEvent{},
		DecisionRequests:   []DecisionRequest{},
		StageSummaries:     []ProjectTaskGraphStageSummary{},
	}, nil
}

func (r *workflowInstanceServiceRepository) ListWorkflowInstances(ctx context.Context, req ListWorkflowInstancesRequest) ([]WorkflowInstanceSummary, error) {
	r.calls++
	r.lastReq = req
	return append([]WorkflowInstanceSummary(nil), r.items...), nil
}

func (r *memoryRepository) ListWorkflowInstances(ctx context.Context, req ListWorkflowInstancesRequest) ([]WorkflowInstanceSummary, error) {
	return []WorkflowInstanceSummary{}, nil
}

func cloneProjects(projects map[uuid.UUID]Project) map[uuid.UUID]Project {
	cloned := make(map[uuid.UUID]Project, len(projects))
	for id, project := range projects {
		cloned[id] = project
	}
	return cloned
}

func strPtrOrNil(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func nonEmptyString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func paginateTestSlice[T any](items []T, limit, offset int32) []T {
	start := int(offset)
	if start > len(items) {
		return []T{}
	}
	end := start + int(limit)
	if end > len(items) {
		end = len(items)
	}
	return items[start:end]
}

func seedHumanOwnerMember(repo *memoryRepository, tenantID, projectID, ownerID uuid.UUID) {
	repo.members[projectID] = append(repo.members[projectID], ProjectMember{
		ID:            uuid.New(),
		TenantID:      tenantID,
		ProjectID:     projectID,
		PrincipalType: PrincipalTypeHumanUser,
		PrincipalID:   ownerID,
		ProjectRole:   ProjectRoleOwner,
		Status:        "active",
	})
}

func bindTaskToRuntimeRun(repo *memoryRepository, taskIndex int, runtimeNodeID uuid.UUID) uuid.UUID {
	runID := uuid.New()
	repo.tasks[taskIndex].DigitalEmployeeRunID = &runID
	repo.projectTaskRunRuntimeNodes[repo.tasks[taskIndex].ID] = runtimeNodeID
	return runID
}

func (r *memoryRepository) CreateProject(ctx context.Context, req CreateProjectRequest, projectID uuid.UUID, workflowID string) (Project, error) {
	project := Project{
		ID:                     projectID,
		TenantID:               req.TenantID,
		TeamID:                 req.TeamID,
		Name:                   req.Name,
		Description:            strPtrOrNil(req.Description),
		Goal:                   req.Goal,
		Status:                 ProjectStatusRunning,
		HumanOwnerUserID:       req.HumanOwnerUserID,
		LeaderUserID:           req.LeaderUserID,
		AcceptanceUserID:       req.AcceptanceUserID,
		CoordinationWorkflowID: workflowID,
		CoordinationStatus:     "registered",
		CoordinationPolicy:     req.CoordinationPolicy,
		ApprovalPolicy:         req.ApprovalPolicy,
		EvidencePolicy:         req.EvidencePolicy,
	}
	r.projects[project.ID] = project
	return project, nil
}

func (r *memoryRepository) GetProject(ctx context.Context, tenantID, projectID uuid.UUID) (Project, error) {
	project, ok := r.projects[projectID]
	if !ok || project.TenantID != tenantID {
		return Project{}, ErrProjectNotFound
	}
	return project, nil
}

func (r *memoryRepository) ListProjects(ctx context.Context, req ListProjectsRequest) ([]Project, error) {
	r.lastListProjects = req
	projects := make([]Project, 0, len(r.projects))
	for _, project := range r.projects {
		if project.TenantID != req.TenantID {
			continue
		}
		if req.Status != nil && project.Status != *req.Status {
			continue
		}
		if req.Query != "" && !strings.Contains(project.Name, req.Query) && !strings.Contains(project.Goal, req.Query) {
			continue
		}
		projects = append(projects, project)
	}
	return projects, nil
}

func (r *memoryRepository) UpdateProjectConfig(ctx context.Context, req UpdateProjectConfigRequest) (Project, error) {
	project, ok := r.projects[req.ProjectID]
	if !ok || project.TenantID != req.TenantID {
		return Project{}, ErrProjectNotFound
	}
	if req.Name != "" {
		project.Name = req.Name
	}
	if req.Description != "" {
		project.Description = strPtrOrNil(req.Description)
	}
	if req.Goal != "" {
		project.Goal = req.Goal
	}
	if req.HumanOwnerUserID != uuid.Nil {
		project.HumanOwnerUserID = req.HumanOwnerUserID
	}
	if req.LeaderUserID != nil {
		project.LeaderUserID = req.LeaderUserID
	}
	if req.AcceptanceUserID != nil {
		project.AcceptanceUserID = req.AcceptanceUserID
	}
	if req.CoordinationPolicy != nil {
		project.CoordinationPolicy = req.CoordinationPolicy
	}
	if req.ApprovalPolicy != nil {
		project.ApprovalPolicy = req.ApprovalPolicy
	}
	if req.EvidencePolicy != nil {
		project.EvidencePolicy = req.EvidencePolicy
	}
	r.projects[project.ID] = project
	return project, nil
}

func (r *memoryRepository) ArchiveProject(ctx context.Context, tenantID, projectID uuid.UUID) (Project, error) {
	if r.archiveProjectErr != nil {
		return Project{}, r.archiveProjectErr
	}
	project, ok := r.projects[projectID]
	if !ok || project.TenantID != tenantID {
		return Project{}, ErrProjectNotFound
	}
	now := time.Now()
	project.Status = ProjectStatusArchived
	project.ArchivedAt = &now
	r.projects[projectID] = project
	return project, nil
}

func (r *memoryRepository) TransitionProjectStatus(ctx context.Context, tenantID, projectID uuid.UUID, fromStatuses []string, toStatus string) (Project, error) {
	project, ok := r.projects[projectID]
	if !ok || project.TenantID != tenantID {
		return Project{}, ErrProjectNotFound
	}
	for _, from := range fromStatuses {
		if project.Status == ProjectStatus(from) {
			project.Status = ProjectStatus(toStatus)
			r.projects[projectID] = project
			return project, nil
		}
	}
	return Project{}, ErrProjectNotFound
}

func (r *memoryRepository) AreAllProjectDemandsTerminal(ctx context.Context, tenantID, projectID uuid.UUID) (bool, error) {
	terminal := map[ProjectDemandStatus]bool{
		ProjectDemandStatusCompleted: true,
		ProjectDemandStatusFailed:    true,
		ProjectDemandStatusCancelled: true,
	}
	count := 0
	for _, demand := range r.demands {
		if demand.TenantID == tenantID && demand.ProjectID == projectID {
			count++
			if !terminal[demand.Status] {
				return false, nil
			}
		}
	}
	return count > 0, nil
}

func (r *memoryRepository) ReplaceProjectMembers(ctx context.Context, tenantID, projectID uuid.UUID, members []ProjectMemberInput) ([]ProjectMember, error) {
	project, ok := r.projects[projectID]
	if !ok || project.TenantID != tenantID {
		return nil, ErrProjectNotFound
	}
	mapped := make([]ProjectMember, 0, len(members))
	for _, member := range members {
		mapped = append(mapped, ProjectMember{
			ID:                  uuid.New(),
			TenantID:            tenantID,
			ProjectID:           projectID,
			PrincipalType:       member.PrincipalType,
			PrincipalID:         member.PrincipalID,
			ProjectRole:         member.ProjectRole,
			DisplayNameSnapshot: strPtrOrNil(member.DisplayNameSnapshot),
			Status:              "active",
			Settings:            member.Settings,
		})
	}
	r.members[projectID] = mapped
	return mapped, nil
}

func (r *memoryRepository) ListProjectMembers(ctx context.Context, tenantID, projectID uuid.UUID) ([]ProjectMember, error) {
	members := r.members[projectID]
	filtered := make([]ProjectMember, 0, len(members))
	for _, member := range members {
		if member.TenantID == tenantID {
			filtered = append(filtered, member)
		}
	}
	return filtered, nil
}

func (r *memoryRepository) ListProjectTasks(ctx context.Context, tenantID, projectID uuid.UUID, status *string, limit, offset int32) ([]ProjectTask, error) {
	r.lastTasksLimit = limit
	r.lastTasksOffset = offset
	filtered := make([]ProjectTask, 0, len(r.tasks))
	for _, task := range r.tasks {
		if task.TenantID == tenantID && task.ProjectID == projectID && (status == nil || task.Status == *status) {
			filtered = append(filtered, task)
		}
	}
	sort.SliceStable(filtered, func(i, j int) bool {
		return filtered[i].UpdatedAt.After(filtered[j].UpdatedAt)
	})
	return paginateTestSlice(filtered, limit, offset), nil
}

func (r *memoryRepository) AppendProjectEvent(ctx context.Context, event AppendProjectEventRequest) (ProjectEvent, error) {
	if r.appendProjectEventErr != nil {
		return ProjectEvent{}, r.appendProjectEventErr
	}
	projectEvent := ProjectEvent{
		ID:             uuid.New(),
		TenantID:       event.TenantID,
		ProjectID:      event.ProjectID,
		SequenceNumber: int64(len(r.events) + 1),
		EventType:      event.EventType,
		ActorType:      event.ActorType,
		ActorID:        event.ActorID,
		ResourceType:   event.ResourceType,
		ResourceID:     event.ResourceID,
		Summary:        strPtrOrNil(event.Summary),
		Payload:        event.Payload,
	}
	r.events = append(r.events, projectEvent)
	r.eventTypes = append(r.eventTypes, event.EventType)
	return projectEvent, nil
}

func (r *memoryRepository) GetProjectEvent(ctx context.Context, tenantID, projectID, eventID uuid.UUID) (ProjectEvent, error) {
	for _, event := range r.events {
		if event.ID == eventID && event.TenantID == tenantID && event.ProjectID == projectID {
			return event, nil
		}
	}
	return ProjectEvent{}, ErrProjectNotFound
}

func (r *memoryRepository) GetProjectEventByTypeAndActor(ctx context.Context, tenantID, projectID uuid.UUID, eventType ProjectEventType, actorID string) (ProjectEvent, error) {
	for i := len(r.events) - 1; i >= 0; i-- {
		event := r.events[i]
		if event.TenantID == tenantID && event.ProjectID == projectID && event.EventType == eventType && event.ActorID == actorID {
			return event, nil
		}
	}
	return ProjectEvent{}, ErrProjectNotFound
}

func (r *memoryRepository) ListProjectEvents(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]ProjectEvent, error) {
	r.lastEventsLimit = limit
	r.lastEventsOffset = offset
	filtered := make([]ProjectEvent, 0, len(r.events))
	for _, event := range r.events {
		if event.TenantID == tenantID && event.ProjectID == projectID {
			filtered = append(filtered, event)
		}
	}
	sort.SliceStable(filtered, func(i, j int) bool {
		return filtered[i].SequenceNumber > filtered[j].SequenceNumber
	})
	return paginateTestSlice(filtered, limit, offset), nil
}

func (r *memoryRepository) CreateProjectDemand(ctx context.Context, req SubmitProjectDemandRequest, status ProjectDemandStatus, createdEventID *uuid.UUID) (ProjectDemand, error) {
	demand := ProjectDemand{
		ID:                 uuid.New(),
		TenantID:           req.TenantID,
		ProjectID:          req.ProjectID,
		SubmittedByUserID:  req.SubmittedByUserID,
		Title:              req.Title,
		Content:            strPtrOrNil(req.Content),
		SourceType:         req.SourceType,
		SourceRefs:         req.SourceRefs,
		Attachments:        req.Attachments,
		ReviewerPreference: reviewerPreferenceFromSourceRefs(req.SourceRefs),
		Status:             status,
		CreatedEventID:     createdEventID,
	}
	r.demands = append(r.demands, demand)
	return demand, nil
}

func (r *memoryRepository) ListProjectDemands(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]ProjectDemand, error) {
	r.lastDemandsLimit = limit
	r.lastDemandsOffset = offset
	filtered := make([]ProjectDemand, 0, len(r.demands))
	for _, demand := range r.demands {
		if demand.TenantID == tenantID && demand.ProjectID == projectID {
			filtered = append(filtered, demand)
		}
	}
	return filtered, nil
}

func (r *memoryRepository) CreateConfigRevision(ctx context.Context, req UpdateProjectConfigRequest, project Project, eventID uuid.UUID) (ProjectConfigRevision, error) {
	revision := ProjectConfigRevision{
		ID:              uuid.New(),
		TenantID:        req.TenantID,
		ProjectID:       req.ProjectID,
		RevisionNumber:  int32(len(r.revisions) + 1),
		ConfigSnapshot:  map[string]any{"name": project.Name, "status": string(project.Status)},
		ChangeSummary:   strPtrOrNil("项目配置已更新"),
		CreatedByUserID: req.ActorUserID,
		CreatedEventID:  &eventID,
	}
	r.revisions = append(r.revisions, revision)
	return revision, nil
}

func (r *memoryRepository) GetProjectDemand(ctx context.Context, tenantID, demandID uuid.UUID) (ProjectDemand, error) {
	for _, demand := range r.demands {
		if demand.ID == demandID && demand.TenantID == tenantID {
			return demand, nil
		}
	}
	return ProjectDemand{}, ErrProjectNotFound
}

func (r *memoryRepository) AdvanceProjectDemandStatus(ctx context.Context, tenantID, projectID, demandID uuid.UUID, target ProjectDemandStatus) error {
	for i := range r.demands {
		if r.demands[i].ID == demandID && r.demands[i].TenantID == tenantID {
			if ProjectDemandStatusCanAdvance(r.demands[i].Status, target) {
				r.demands[i].Status = target
			}
			return nil
		}
	}
	return nil
}

func (r *memoryRepository) GetProjectTask(ctx context.Context, tenantID, projectTaskID uuid.UUID) (ProjectTask, error) {
	for _, task := range r.tasks {
		if task.ID == projectTaskID && task.TenantID == tenantID {
			return task, nil
		}
	}
	return ProjectTask{}, ErrProjectNotFound
}

func (r *memoryRepository) GetProjectTaskRunRuntimeNodeID(ctx context.Context, tenantID, projectTaskID, runID uuid.UUID) (uuid.UUID, error) {
	runtimeNodeID, ok := r.projectTaskRunRuntimeNodes[projectTaskID]
	if !ok {
		return uuid.Nil, ErrProjectNotFound
	}
	return runtimeNodeID, nil
}

func (r *memoryRepository) GetProjectTaskRunWorkProducts(ctx context.Context, tenantID, runID uuid.UUID) ([]any, error) {
	workProducts, ok := r.projectTaskRunWorkProducts[runID]
	if !ok {
		return []any{}, nil
	}
	return workProducts, nil
}

func (r *memoryRepository) CreateCoordinationJob(ctx context.Context, req CreateCoordinationJobRequest) (CoordinationJob, error) {
	job := CoordinationJob{
		ID:               uuid.New(),
		TenantID:         req.TenantID,
		ProjectID:        req.ProjectID,
		WorkflowID:       req.WorkflowID,
		TriggerEventID:   req.TriggerEventID,
		JobType:          req.JobType,
		Status:           req.Status,
		InputSnapshotRef: req.InputSnapshotRef,
		OutputEventIDs:   []any{},
		CreatedAt:        time.Now().UTC(),
	}
	r.coordinationJobs = append(r.coordinationJobs, job)
	return job, nil
}

func (r *memoryRepository) FinishCoordinationJob(ctx context.Context, req FinishCoordinationJobRequest) (CoordinationJob, error) {
	for index, job := range r.coordinationJobs {
		if job.ID == req.ID && job.TenantID == req.TenantID {
			now := time.Now().UTC()
			job.Status = req.Status
			job.OutputEventIDs = req.OutputEventIDs
			job.FinishedAt = &now
			r.coordinationJobs[index] = job
			return job, nil
		}
	}
	return CoordinationJob{}, ErrProjectNotFound
}

func (r *memoryRepository) ListCoordinationJobs(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]CoordinationJob, error) {
	filtered := make([]CoordinationJob, 0, len(r.coordinationJobs))
	for _, job := range r.coordinationJobs {
		if job.TenantID == tenantID && job.ProjectID == projectID {
			filtered = append(filtered, job)
		}
	}
	sort.SliceStable(filtered, func(i, j int) bool {
		return filtered[i].CreatedAt.After(filtered[j].CreatedAt)
	})
	return paginateTestSlice(filtered, limit, offset), nil
}

func (r *memoryRepository) ListDemandLaunchCoordinationJobs(ctx context.Context, tenantID, projectID, demandID uuid.UUID, createdEventID *uuid.UUID, limit int32) ([]CoordinationJob, error) {
	candidates := make([]CoordinationJob, 0, len(r.coordinationJobs))
	for _, job := range r.coordinationJobs {
		if job.TenantID == tenantID && job.ProjectID == projectID {
			candidates = append(candidates, job)
		}
	}
	filtered := filterJobsForDemand(candidates, ProjectDemand{ID: demandID, CreatedEventID: createdEventID})
	sort.SliceStable(filtered, func(i, j int) bool {
		return filtered[i].CreatedAt.After(filtered[j].CreatedAt)
	})
	return paginateTestSlice(filtered, limit, 0), nil
}

func (r *memoryRepository) CreateRouteDecision(ctx context.Context, req CreateRouteDecisionRequest) (RouteDecision, error) {
	decision := RouteDecision{
		ID:                          uuid.New(),
		TenantID:                    req.TenantID,
		ProjectID:                   req.ProjectID,
		CoordinationJobID:           req.CoordinationJobID,
		DemandID:                    req.DemandID,
		CandidateDigitalEmployeeIDs: req.CandidateDigitalEmployeeIDs,
		SelectedDigitalEmployeeIDs:  req.SelectedDigitalEmployeeIDs,
		Reason:                      req.Reason,
		InputRequirements:           req.InputRequirements,
		ExpectedOutputs:             req.ExpectedOutputs,
		BudgetEstimate:              req.BudgetEstimate,
		RequiresHumanReview:         req.RequiresHumanReview,
		CreatedEventID:              req.CreatedEventID,
		CreatedAt:                   time.Now().UTC(),
	}
	r.routeDecisions = append(r.routeDecisions, decision)
	return decision, nil
}

func (r *memoryRepository) ListRouteDecisions(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]RouteDecision, error) {
	filtered := make([]RouteDecision, 0, len(r.routeDecisions))
	for _, decision := range r.routeDecisions {
		if decision.TenantID == tenantID && decision.ProjectID == projectID {
			filtered = append(filtered, decision)
		}
	}
	sort.SliceStable(filtered, func(i, j int) bool {
		return filtered[i].CreatedAt.After(filtered[j].CreatedAt)
	})
	return paginateTestSlice(filtered, limit, offset), nil
}

func (r *memoryRepository) ListDemandLaunchRouteDecisions(ctx context.Context, tenantID, projectID, demandID uuid.UUID, limit int32) ([]RouteDecision, error) {
	candidates := make([]RouteDecision, 0, len(r.routeDecisions))
	for _, decision := range r.routeDecisions {
		if decision.TenantID == tenantID && decision.ProjectID == projectID {
			candidates = append(candidates, decision)
		}
	}
	filtered := filterRoutesForDemand(candidates, demandID)
	sort.SliceStable(filtered, func(i, j int) bool {
		return filtered[i].CreatedAt.After(filtered[j].CreatedAt)
	})
	return paginateTestSlice(filtered, limit, 0), nil
}

func (r *memoryRepository) CreatePlanRevision(ctx context.Context, req CreatePlanRevisionRequest) (PlanRevision, error) {
	return PlanRevision{}, ErrProjectNotFound
}

func (r *memoryRepository) GetPlanRevision(ctx context.Context, tenantID, projectID, revisionID uuid.UUID) (PlanRevision, error) {
	return PlanRevision{}, ErrProjectNotFound
}

func (r *memoryRepository) ListPlanRevisions(ctx context.Context, req ListPlanRevisionsRequest) ([]PlanRevision, error) {
	return []PlanRevision{}, nil
}

func (r *memoryRepository) AcceptPlanRevision(ctx context.Context, req AcceptPlanRevisionRequest) (PlanRevision, error) {
	return PlanRevision{}, ErrProjectNotFound
}

func (r *memoryRepository) RejectPlanRevision(ctx context.Context, req RejectPlanRevisionRequest) (PlanRevision, error) {
	return PlanRevision{}, ErrProjectNotFound
}

func (r *memoryRepository) CreateProjectTask(ctx context.Context, req CreateProjectTaskRequest) (ProjectTask, error) {
	task := ProjectTask{
		ID:                        uuid.New(),
		TenantID:                  req.TenantID,
		ProjectID:                 req.ProjectID,
		DemandID:                  req.DemandID,
		Title:                     req.Title,
		Summary:                   strPtrOrNil(req.Summary),
		Status:                    req.Status,
		AssignedDigitalEmployeeID: req.AssignedDigitalEmployeeID,
		RiskLevel:                 strPtrOrNil(req.RiskLevel),
		RequiresHumanApproval:     req.RequiresHumanApproval,
		CreatedAt:                 time.Now().UTC(),
		UpdatedAt:                 time.Now().UTC(),
	}
	r.tasks = append(r.tasks, task)
	return task, nil
}

func (r *memoryRepository) ListDemandLaunchProjectTasks(ctx context.Context, tenantID, projectID, demandID uuid.UUID, limit int32) ([]ProjectTask, error) {
	candidates := make([]ProjectTask, 0, len(r.tasks))
	for _, task := range r.tasks {
		if task.TenantID == tenantID && task.ProjectID == projectID {
			candidates = append(candidates, task)
		}
	}
	filtered := filterTasksForDemand(candidates, demandID)
	sort.SliceStable(filtered, func(i, j int) bool {
		return filtered[i].UpdatedAt.After(filtered[j].UpdatedAt)
	})
	return paginateTestSlice(filtered, limit, 0), nil
}

func (r *memoryRepository) createProjectTaskAttempt(req QueueProjectTaskRequest, attemptNo int32, eventID *uuid.UUID) ProjectTaskAttempt {
	version := nonEmptyString(req.ExecutionContextPacketVersion, "v1")
	packet := req.ExecutionContextPacket
	if packet == nil {
		packet = map[string]any{}
	}
	attemptID := uuid.New()
	if req.ProjectTaskAttemptID != nil {
		attemptID = *req.ProjectTaskAttemptID
	}
	attempt := ProjectTaskAttempt{
		ID:                            attemptID,
		TenantID:                      req.TenantID,
		ProjectTaskID:                 req.ProjectTaskID,
		AttemptNo:                     attemptNo,
		Status:                        ProjectTaskAttemptStatusQueued,
		DigitalEmployeeRunID:          req.DigitalEmployeeRunID,
		RuntimeTaskID:                 req.RuntimeTaskID,
		RuntimeNodeID:                 req.RuntimeNodeID,
		ExecutionContextPacket:        packet,
		ExecutionContextPacketVersion: version,
		LeaseToken:                    req.LeaseToken,
		LeaseExpiresAt:                req.LeaseExpiresAt,
		IdempotencyKey:                req.IdempotencyKey,
		DispatchGateResultID:          req.DispatchGateResultID,
		CreatedEventID:                eventID,
		CreatedAt:                     time.Now().UTC(),
		UpdatedAt:                     time.Now().UTC(),
	}
	r.projectTaskAttempts = append(r.projectTaskAttempts, attempt)
	return attempt
}

func (r *memoryRepository) replayQueueProjectTaskAttempt(ctx context.Context, req QueueProjectTaskRequest) (QueueProjectTaskResult, bool, error) {
	for _, attempt := range r.projectTaskAttempts {
		if attempt.TenantID != req.TenantID || attempt.IdempotencyKey != req.IdempotencyKey {
			continue
		}
		if attempt.ProjectTaskID != req.ProjectTaskID {
			return QueueProjectTaskResult{}, true, ErrProjectConflict
		}
		task, err := r.GetProjectTask(ctx, req.TenantID, req.ProjectTaskID)
		if err != nil {
			return QueueProjectTaskResult{}, true, err
		}
		if task.ProjectID != req.ProjectID {
			return QueueProjectTaskResult{}, true, ErrProjectNotFound
		}
		var event ProjectEvent
		if attempt.CreatedEventID != nil {
			for _, candidate := range r.events {
				if candidate.TenantID == req.TenantID && candidate.ProjectID == req.ProjectID && candidate.ID == *attempt.CreatedEventID {
					event = candidate
					break
				}
			}
		}
		return QueueProjectTaskResult{Task: task, Attempt: attempt, Event: event}, true, nil
	}
	return QueueProjectTaskResult{}, false, nil
}

func (r *memoryRepository) QueueProjectTaskWithAttempt(ctx context.Context, req QueueProjectTaskRequest) (QueueProjectTaskResult, error) {
	if result, replayed, err := r.replayQueueProjectTaskAttempt(ctx, req); replayed || err != nil {
		return result, err
	}
	for index, task := range r.tasks {
		if task.TenantID != req.TenantID || task.ProjectID != req.ProjectID || task.ID != req.ProjectTaskID {
			continue
		}
		if task.Status != ProjectTaskStatusPlanned && task.Status != ProjectTaskStatusWaitingHuman {
			return QueueProjectTaskResult{}, ErrProjectConflict
		}
		if task.AssignedDigitalEmployeeID != nil && *task.AssignedDigitalEmployeeID != req.DigitalEmployeeID {
			return QueueProjectTaskResult{}, ErrProjectTaskForbidden
		}
		gateIndex := -1
		if req.DispatchGateResultID != nil {
			for candidateIndex, gate := range r.dispatchGateResults {
				if gate.TenantID == req.TenantID && gate.ProjectID == req.ProjectID && gate.ProjectTaskID == req.ProjectTaskID && gate.ID == *req.DispatchGateResultID {
					if gate.AttemptID != nil && (req.ProjectTaskAttemptID == nil || *gate.AttemptID != *req.ProjectTaskAttemptID) {
						return QueueProjectTaskResult{}, ErrProjectNotFound
					}
					gateIndex = candidateIndex
					break
				}
			}
			if gateIndex == -1 {
				return QueueProjectTaskResult{}, ErrProjectNotFound
			}
		}
		event, err := r.AppendProjectEvent(ctx, AppendProjectEventRequest{
			TenantID:     req.TenantID,
			ProjectID:    req.ProjectID,
			EventType:    ProjectEventTaskDispatched,
			ActorType:    "project_coordinator",
			ActorID:      req.ProjectTaskID.String(),
			ResourceType: strPtr("project_task"),
			ResourceID:   strPtr(req.ProjectTaskID.String()),
			Summary:      "项目任务已排队",
			Payload:      queueProjectTaskEventPayload(req, uuid.Nil, task.AttemptCount+1),
		})
		if err != nil {
			return QueueProjectTaskResult{}, err
		}
		attempt := r.createProjectTaskAttempt(req, task.AttemptCount+1, &event.ID)
		event.Payload["project_task_attempt_id"] = attempt.ID.String()
		r.events[len(r.events)-1] = event
		now := time.Now().UTC()
		task.Status = ProjectTaskStatusQueued
		task.CurrentAttemptID = &attempt.ID
		task.DigitalEmployeeRunID = req.DigitalEmployeeRunID
		task.RuntimeTaskID = req.RuntimeTaskID
		if req.DispatchGateResultID != nil {
			task.LatestDispatchGateResultID = req.DispatchGateResultID
			gate := r.dispatchGateResults[gateIndex]
			gate.AttemptID = &attempt.ID
			gate.UpdatedAt = now
			r.dispatchGateResults[gateIndex] = gate
		}
		task.AttemptCount++
		task.RetryNotBefore = nil
		task.WaitingReason = nil
		task.WaitingRequestID = nil
		task.StatusChangedAt = now
		task.UpdatedAt = now
		r.tasks[index] = task
		return QueueProjectTaskResult{Task: task, Attempt: attempt, Event: event}, nil
	}
	return QueueProjectTaskResult{}, ErrProjectNotFound
}

func (r *memoryRepository) RecordPreDispatchGateResult(ctx context.Context, req RecordPreDispatchGateResultRequest) (PreDispatchGateResult, error) {
	now := time.Now().UTC()
	checkedAt := req.CheckedAt
	if checkedAt.IsZero() {
		checkedAt = now
	}
	taskExists := false
	for _, task := range r.tasks {
		if task.TenantID == req.TenantID && task.ProjectID == req.ProjectID && task.ID == req.ProjectTaskID {
			taskExists = true
			break
		}
	}
	if !taskExists {
		return PreDispatchGateResult{}, ErrProjectNotFound
	}
	for index, gate := range r.dispatchGateResults {
		if gate.TenantID == req.TenantID && gate.ProjectTaskID == req.ProjectTaskID && gate.IdempotencyKey == req.IdempotencyKey {
			if gate.ProjectID != req.ProjectID {
				return PreDispatchGateResult{}, ErrProjectNotFound
			}
			if gate.AttemptID != nil || gate.DecisionRequestID != nil {
				return gate, nil
			}
			gate.Status = req.Status
			gate.CheckedAt = checkedAt
			gate.Checks = append([]PreDispatchGateCheck(nil), req.Checks...)
			gate.Blockers = append([]PreDispatchGateBlocker(nil), req.Blockers...)
			gate.HumanActionRequest = HumanActionRequest(cloneMap(map[string]any(req.HumanActionRequest)))
			gate.RetryAfter = req.RetryAfter
			if gate.CreatedEventID == nil {
				gate.CreatedEventID = req.CreatedEventID
			}
			gate.UpdatedAt = now
			r.dispatchGateResults[index] = gate
			if err := r.markLatestDispatchGate(req.TenantID, req.ProjectID, req.ProjectTaskID, gate.ID); err != nil {
				return PreDispatchGateResult{}, err
			}
			return gate, nil
		}
	}
	gate := PreDispatchGateResult{
		ID:                     uuid.New(),
		TenantID:               req.TenantID,
		ProjectID:              req.ProjectID,
		ProjectTaskID:          req.ProjectTaskID,
		AcceptedPlanRevisionID: req.AcceptedPlanRevisionID,
		PlannedTaskKey:         req.PlannedTaskKey,
		SelectedEmployeeID:     req.SelectedEmployeeID,
		AttemptNo:              req.AttemptNo,
		DispatchReason:         req.DispatchReason,
		IdempotencyKey:         req.IdempotencyKey,
		DispatchToken:          req.DispatchToken,
		Status:                 req.Status,
		CheckedAt:              checkedAt,
		Checks:                 append([]PreDispatchGateCheck(nil), req.Checks...),
		Blockers:               append([]PreDispatchGateBlocker(nil), req.Blockers...),
		HumanActionRequest:     HumanActionRequest(cloneMap(map[string]any(req.HumanActionRequest))),
		RetryAfter:             req.RetryAfter,
		CreatedEventID:         req.CreatedEventID,
		CreatedAt:              now,
		UpdatedAt:              now,
	}
	r.dispatchGateResults = append(r.dispatchGateResults, gate)
	if err := r.markLatestDispatchGate(req.TenantID, req.ProjectID, req.ProjectTaskID, gate.ID); err != nil {
		return PreDispatchGateResult{}, err
	}
	return gate, nil
}

func (r *memoryRepository) GetPreDispatchGateResult(ctx context.Context, tenantID, projectID, gateResultID uuid.UUID) (PreDispatchGateResult, error) {
	for _, gate := range r.dispatchGateResults {
		if gate.TenantID == tenantID && gate.ProjectID == projectID && gate.ID == gateResultID {
			return gate, nil
		}
	}
	return PreDispatchGateResult{}, ErrProjectNotFound
}

func (r *memoryRepository) GetPreDispatchGateResultByKey(ctx context.Context, tenantID, projectID, projectTaskID uuid.UUID, idempotencyKey string) (PreDispatchGateResult, error) {
	for _, gate := range r.dispatchGateResults {
		if gate.TenantID == tenantID && gate.ProjectID == projectID && gate.ProjectTaskID == projectTaskID && gate.IdempotencyKey == idempotencyKey {
			return gate, nil
		}
	}
	return PreDispatchGateResult{}, ErrProjectNotFound
}

func (r *memoryRepository) ListPreDispatchGateResults(ctx context.Context, req ListPreDispatchGateResultsRequest) ([]PreDispatchGateResult, error) {
	limit := req.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}
	offset := req.Offset
	if offset < 0 {
		offset = 0
	}
	filtered := make([]PreDispatchGateResult, 0, len(r.dispatchGateResults))
	for _, gate := range r.dispatchGateResults {
		if gate.TenantID == req.TenantID && gate.ProjectID == req.ProjectID && gate.ProjectTaskID == req.ProjectTaskID {
			filtered = append(filtered, gate)
		}
	}
	sort.SliceStable(filtered, func(i, j int) bool {
		return filtered[i].CreatedAt.After(filtered[j].CreatedAt)
	})
	return paginateTestSlice(filtered, limit, offset), nil
}

func (r *memoryRepository) LinkPreDispatchGateAttempt(ctx context.Context, req LinkPreDispatchGateAttemptRequest) (PreDispatchGateResult, error) {
	gateIndex := -1
	for index, gate := range r.dispatchGateResults {
		if gate.TenantID == req.TenantID && gate.ProjectID == req.ProjectID && gate.ProjectTaskID == req.ProjectTaskID && gate.ID == req.GateResultID {
			if gate.AttemptID != nil && *gate.AttemptID != req.AttemptID {
				return PreDispatchGateResult{}, ErrProjectNotFound
			}
			gateIndex = index
			break
		}
	}
	if gateIndex == -1 {
		return PreDispatchGateResult{}, ErrProjectNotFound
	}
	attemptIndex := -1
	for index, attempt := range r.projectTaskAttempts {
		if attempt.TenantID == req.TenantID && attempt.ProjectTaskID == req.ProjectTaskID && attempt.ID == req.AttemptID {
			if attempt.DispatchGateResultID != nil && *attempt.DispatchGateResultID != req.GateResultID {
				return PreDispatchGateResult{}, ErrProjectNotFound
			}
			attemptIndex = index
			break
		}
	}
	if attemptIndex == -1 {
		return PreDispatchGateResult{}, ErrProjectNotFound
	}
	now := time.Now().UTC()
	gate := r.dispatchGateResults[gateIndex]
	gate.AttemptID = &req.AttemptID
	gate.UpdatedAt = now
	r.dispatchGateResults[gateIndex] = gate
	attempt := r.projectTaskAttempts[attemptIndex]
	attempt.DispatchGateResultID = &req.GateResultID
	attempt.UpdatedAt = now
	r.projectTaskAttempts[attemptIndex] = attempt
	return gate, nil
}

func (r *memoryRepository) LinkPreDispatchGateDecisionRequest(ctx context.Context, req LinkPreDispatchGateDecisionRequest) (PreDispatchGateResult, error) {
	decisionIndex := -1
	for index, decision := range r.decisionRequests {
		if decision.TenantID == req.TenantID && decision.ProjectID == req.ProjectID && decision.ID == req.DecisionRequestID {
			if decision.ProjectTaskID == nil || *decision.ProjectTaskID != req.ProjectTaskID {
				return PreDispatchGateResult{}, ErrProjectNotFound
			}
			if decision.DispatchGateResultID != nil && *decision.DispatchGateResultID != req.GateResultID {
				return PreDispatchGateResult{}, ErrProjectNotFound
			}
			decisionIndex = index
			break
		}
	}
	if decisionIndex == -1 {
		return PreDispatchGateResult{}, ErrProjectNotFound
	}
	gateIndex := -1
	for index, gate := range r.dispatchGateResults {
		if gate.TenantID == req.TenantID && gate.ProjectID == req.ProjectID && gate.ProjectTaskID == req.ProjectTaskID && gate.ID == req.GateResultID {
			if gate.DecisionRequestID != nil && *gate.DecisionRequestID != req.DecisionRequestID {
				return PreDispatchGateResult{}, ErrProjectNotFound
			}
			gateIndex = index
			break
		}
	}
	if gateIndex == -1 {
		return PreDispatchGateResult{}, ErrProjectNotFound
	}
	now := time.Now().UTC()
	gate := r.dispatchGateResults[gateIndex]
	gate.DecisionRequestID = &req.DecisionRequestID
	gate.UpdatedAt = now
	r.dispatchGateResults[gateIndex] = gate
	decision := r.decisionRequests[decisionIndex]
	decision.DispatchGateResultID = &req.GateResultID
	decision.UpdatedAt = now
	r.decisionRequests[decisionIndex] = decision
	return gate, nil
}

func (r *memoryRepository) MoveProjectTaskToWaitingHumanForPreDispatchGate(ctx context.Context, req MoveProjectTaskToWaitingHumanForPreDispatchGateRequest) (ProjectTask, error) {
	gateExists := false
	for _, gate := range r.dispatchGateResults {
		if gate.TenantID == req.TenantID && gate.ProjectID == req.ProjectID && gate.ProjectTaskID == req.ProjectTaskID && gate.ID == req.GateResultID {
			gateExists = true
			break
		}
	}
	if !gateExists {
		return ProjectTask{}, ErrProjectNotFound
	}
	for index, task := range r.tasks {
		if task.TenantID != req.TenantID || task.ProjectID != req.ProjectID || task.ID != req.ProjectTaskID {
			continue
		}
		if task.Status != ProjectTaskStatusPlanned && task.Status != ProjectTaskStatusWaitingHuman {
			return ProjectTask{}, ErrProjectNotFound
		}
		now := time.Now().UTC()
		task.Status = ProjectTaskStatusWaitingHuman
		task.WaitingReason = strPtrOrNil(req.WaitingReason)
		if req.DecisionRequestID != uuid.Nil {
			task.WaitingRequestID = &req.DecisionRequestID
		} else {
			task.WaitingRequestID = nil
		}
		task.LatestDispatchGateResultID = &req.GateResultID
		task.StatusChangedAt = now
		task.UpdatedAt = now
		r.tasks[index] = task
		return task, nil
	}
	return ProjectTask{}, ErrProjectNotFound
}

func (r *memoryRepository) markLatestDispatchGate(tenantID, projectID, projectTaskID, gateResultID uuid.UUID) error {
	for index, task := range r.tasks {
		if task.TenantID == tenantID && task.ProjectID == projectID && task.ID == projectTaskID {
			task.LatestDispatchGateResultID = &gateResultID
			task.UpdatedAt = time.Now().UTC()
			r.tasks[index] = task
			return nil
		}
	}
	return ErrProjectNotFound
}

func (r *memoryRepository) GetProjectTaskAttempt(ctx context.Context, tenantID, attemptID uuid.UUID) (ProjectTaskAttempt, error) {
	for _, attempt := range r.projectTaskAttempts {
		if attempt.TenantID == tenantID && attempt.ID == attemptID {
			return attempt, nil
		}
	}
	return ProjectTaskAttempt{}, ErrProjectNotFound
}

func (r *memoryRepository) GetCurrentProjectTaskAttempt(ctx context.Context, tenantID, projectTaskID uuid.UUID) (ProjectTaskAttempt, error) {
	task, err := r.GetProjectTask(ctx, tenantID, projectTaskID)
	if err != nil {
		return ProjectTaskAttempt{}, err
	}
	if task.CurrentAttemptID == nil {
		return ProjectTaskAttempt{}, ErrProjectNotFound
	}
	return r.GetProjectTaskAttempt(ctx, tenantID, *task.CurrentAttemptID)
}

func (r *memoryRepository) RecordProjectTaskAttemptContextUpdate(ctx context.Context, req RecordProjectTaskAttemptContextUpdateRepositoryRequest) (ProjectTaskAttemptContextUpdate, error) {
	update := ProjectTaskAttemptContextUpdate{
		ID:             uuid.New(),
		TenantID:       req.TenantID,
		ProjectTaskID:  req.ProjectTaskID,
		AttemptID:      req.AttemptID,
		UpdateKind:     req.UpdateKind,
		Payload:        cloneMap(req.Payload),
		DeliveryMode:   req.DeliveryMode,
		CreatedEventID: req.CreatedEventID,
		CreatedAt:      time.Now().UTC(),
		UpdatedAt:      time.Now().UTC(),
	}
	r.contextUpdates = append(r.contextUpdates, update)
	return update, nil
}

func (r *memoryRepository) DecomposeAcceptedPlanRevision(ctx context.Context, req DecomposeAcceptedPlanRevisionRequest) (DecomposeAcceptedPlanRevisionResult, error) {
	return DecomposeAcceptedPlanRevisionResult{}, ErrProjectTaskGraphPending
}

func (r *memoryRepository) UpdateProjectTaskStatus(ctx context.Context, tenantID, projectTaskID uuid.UUID, status string, eventID *uuid.UUID, currentStatuses []string) (ProjectTask, error) {
	for index, task := range r.tasks {
		if task.ID == projectTaskID && task.TenantID == tenantID {
			if r.taskStatusBeforeUpdate != nil {
				task.Status = *r.taskStatusBeforeUpdate
				r.tasks[index] = task
				r.taskStatusBeforeUpdate = nil
			}
			if !containsString(currentStatuses, task.Status) {
				return ProjectTask{}, ErrProjectNotFound
			}
			task.Status = status
			task.UpdatedAt = time.Now().UTC()
			r.tasks[index] = task
			return task, nil
		}
	}
	return ProjectTask{}, ErrProjectNotFound
}

func (r *memoryRepository) BindProjectTaskRun(ctx context.Context, req BindProjectTaskRunRequest) (ProjectTask, error) {
	for i, task := range r.tasks {
		if task.TenantID != req.TenantID || task.ID != req.ProjectTaskID {
			continue
		}
		if task.DigitalEmployeeRunID != nil || task.RuntimeTaskID != nil {
			if task.DigitalEmployeeRunID != nil && *task.DigitalEmployeeRunID == req.DigitalEmployeeRunID &&
				task.RuntimeTaskID != nil && *task.RuntimeTaskID == req.RuntimeTaskID {
				return task, nil
			}
			return ProjectTask{}, ErrProjectConflict
		}
		allowed := false
		for _, status := range req.CurrentStatuses {
			if task.Status == status {
				allowed = true
				break
			}
		}
		if !allowed {
			return ProjectTask{}, ErrProjectConflict
		}
		task.Status = "assigned"
		task.DigitalEmployeeRunID = &req.DigitalEmployeeRunID
		task.RuntimeTaskID = &req.RuntimeTaskID
		task.UpdatedAt = time.Now().UTC()
		r.tasks[i] = task
		return task, nil
	}
	return ProjectTask{}, ErrProjectNotFound
}

func (r *memoryRepository) ProjectTaskEventExists(ctx context.Context, tenantID, projectID uuid.UUID, eventType ProjectEventType, actorID string) (bool, error) {
	for _, event := range r.events {
		if event.TenantID == tenantID && event.ProjectID == projectID && event.EventType == eventType && event.ActorID == actorID {
			return true, nil
		}
	}
	return false, nil
}

func (r *memoryRepository) AssignProjectTask(ctx context.Context, tenantID, projectTaskID uuid.UUID, status string, assignedDigitalEmployeeID, eventID *uuid.UUID) (ProjectTask, error) {
	for index, task := range r.tasks {
		if task.ID == projectTaskID && task.TenantID == tenantID {
			task.Status = status
			task.AssignedDigitalEmployeeID = assignedDigitalEmployeeID
			task.UpdatedAt = time.Now().UTC()
			r.tasks[index] = task
			return task, nil
		}
	}
	return ProjectTask{}, ErrProjectNotFound
}

func (r *memoryRepository) CreateExecutionSummary(ctx context.Context, req CreateExecutionSummaryRequest) (ExecutionSummary, error) {
	if r.createExecutionSummaryErr != nil {
		return ExecutionSummary{}, r.createExecutionSummaryErr
	}
	summary := ExecutionSummary{
		ID:                    uuid.New(),
		TenantID:              req.TenantID,
		ProjectID:             req.ProjectID,
		ProjectTaskID:         req.ProjectTaskID,
		DigitalEmployeeID:     req.DigitalEmployeeID,
		Conclusion:            req.Conclusion,
		EvidenceRefs:          req.EvidenceRefs,
		ArtifactRefs:          req.ArtifactRefs,
		ConfidenceFactors:     req.ConfidenceFactors,
		Uncertainty:           strPtrOrNil(req.Uncertainty),
		MissingInformation:    req.MissingInformation,
		RecommendedNextAction: strPtrOrNil(req.RecommendedNextAction),
		RequiresHumanReview:   req.RequiresHumanReview,
		TransferRequestID:     req.TransferRequestID,
		CreatedEventID:        req.CreatedEventID,
		CreatedAt:             time.Now().UTC(),
	}
	r.executionSummaries = append(r.executionSummaries, summary)
	return summary, nil
}

func (r *memoryRepository) ListExecutionSummaries(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]ExecutionSummary, error) {
	r.lastExecutionSummariesLimit = limit
	r.lastExecutionSummariesOffset = offset
	filtered := make([]ExecutionSummary, 0, len(r.executionSummaries))
	for _, summary := range r.executionSummaries {
		if summary.TenantID == tenantID && summary.ProjectID == projectID {
			filtered = append(filtered, summary)
		}
	}
	return paginateTestSlice(filtered, limit, offset), nil
}

func (r *memoryRepository) CreateExecutionLedgerEvent(ctx context.Context, req CreateExecutionLedgerEventRequest) (ExecutionLedgerEvent, error) {
	if r.createExecutionLedgerEventErr != nil {
		return ExecutionLedgerEvent{}, r.createExecutionLedgerEventErr
	}
	if req.IdempotencyKey != "" {
		for _, event := range r.executionLedgerEvents {
			if event.TenantID == req.TenantID && event.IdempotencyKey == req.IdempotencyKey {
				return cloneExecutionLedgerEvent(event), nil
			}
		}
	}
	now := time.Now().UTC()
	occurredAt := now
	if req.OccurredAt != nil {
		occurredAt = *req.OccurredAt
	}
	event := ExecutionLedgerEvent{
		ID:                   uuid.New(),
		TenantID:             req.TenantID,
		TeamID:               req.TeamID,
		ProjectID:            req.ProjectID,
		ProjectTaskID:        req.ProjectTaskID,
		ProjectTaskAttemptID: req.ProjectTaskAttemptID,
		EventType:            req.EventType,
		SourceType:           req.SourceType,
		SourceID:             req.SourceID,
		ActorType:            req.ActorType,
		ActorID:              req.ActorID,
		RuntimeNodeID:        req.RuntimeNodeID,
		ProviderType:         req.ProviderType,
		ProviderSessionID:    req.ProviderSessionID,
		InputSummary:         strPtrOrNil(req.InputSummary),
		OutputSummary:        strPtrOrNil(req.OutputSummary),
		ErrorFamily:          strPtrOrNil(req.ErrorFamily),
		ErrorCode:            strPtrOrNil(req.ErrorCode),
		ErrorMessage:         strPtrOrNil(req.ErrorMessage),
		Retryable:            req.Retryable,
		ArtifactRefs:         append([]any(nil), sliceOrEmptyAny(req.ArtifactRefs)...),
		EvidenceRefs:         append([]any(nil), sliceOrEmptyAny(req.EvidenceRefs)...),
		Metadata:             cloneMap(mapOrEmptyAny(req.Metadata)),
		OccurredAt:           occurredAt,
		IdempotencyKey:       req.IdempotencyKey,
		CreatedAt:            now,
		UpdatedAt:            now,
	}
	r.executionLedgerEvents = append(r.executionLedgerEvents, cloneExecutionLedgerEvent(event))
	return event, nil
}

func (r *memoryRepository) ListProjectExecutionLedgerEvents(ctx context.Context, req GetExecutionTraceRequest) ([]ExecutionLedgerEvent, error) {
	r.executionLedgerEventListRequests = append(r.executionLedgerEventListRequests, req)
	filtered := make([]ExecutionLedgerEvent, 0, len(r.executionLedgerEvents))
	for _, event := range r.executionLedgerEvents {
		if event.TenantID != req.TenantID || event.ProjectID != req.ProjectID {
			continue
		}
		if req.ProjectTaskID != nil && (event.ProjectTaskID == nil || *event.ProjectTaskID != *req.ProjectTaskID) {
			continue
		}
		if req.ProjectTaskAttemptID != nil && (event.ProjectTaskAttemptID == nil || *event.ProjectTaskAttemptID != *req.ProjectTaskAttemptID) {
			continue
		}
		if req.EventType != nil && event.EventType != *req.EventType {
			continue
		}
		if req.ErrorFamily != nil && (event.ErrorFamily == nil || *event.ErrorFamily != *req.ErrorFamily) {
			continue
		}
		filtered = append(filtered, cloneExecutionLedgerEvent(event))
	}
	sort.SliceStable(filtered, func(i, j int) bool {
		if !filtered[i].OccurredAt.Equal(filtered[j].OccurredAt) {
			return filtered[i].OccurredAt.Before(filtered[j].OccurredAt)
		}
		return filtered[i].CreatedAt.Before(filtered[j].CreatedAt)
	})
	return paginateTestSlice(filtered, req.Limit, req.Offset), nil
}

func (r *memoryRepository) ListProjectTaskAttemptsForExecutionTrace(ctx context.Context, tenantID, projectID uuid.UUID) ([]ProjectTaskAttempt, error) {
	taskProjects := make(map[uuid.UUID]uuid.UUID, len(r.tasks))
	for _, task := range r.tasks {
		if task.TenantID == tenantID {
			taskProjects[task.ID] = task.ProjectID
		}
	}
	filtered := make([]ProjectTaskAttempt, 0, len(r.projectTaskAttempts))
	for _, attempt := range r.projectTaskAttempts {
		if attempt.TenantID == tenantID && taskProjects[attempt.ProjectTaskID] == projectID {
			filtered = append(filtered, attempt)
		}
	}
	sort.SliceStable(filtered, func(i, j int) bool {
		if filtered[i].ProjectTaskID != filtered[j].ProjectTaskID {
			return filtered[i].ProjectTaskID.String() < filtered[j].ProjectTaskID.String()
		}
		if filtered[i].AttemptNo != filtered[j].AttemptNo {
			return filtered[i].AttemptNo < filtered[j].AttemptNo
		}
		return filtered[i].CreatedAt.Before(filtered[j].CreatedAt)
	})
	return filtered, nil
}

func (r *memoryRepository) CreateTransferRequest(ctx context.Context, req CreateTransferRequestRequest) (TransferRequest, error) {
	if r.createTransferRequestErr != nil {
		return TransferRequest{}, r.createTransferRequestErr
	}
	transfer := TransferRequest{
		ID:                           uuid.New(),
		TenantID:                     req.TenantID,
		ProjectID:                    req.ProjectID,
		ProjectTaskID:                req.ProjectTaskID,
		RequestedByDigitalEmployeeID: req.RequestedByDigitalEmployeeID,
		Reason:                       req.Reason,
		SuggestedEmployeeType:        strPtrOrNil(req.SuggestedEmployeeType),
		SuggestedDigitalEmployeeIDs:  req.SuggestedDigitalEmployeeIDs,
		MissingContextRefs:           req.MissingContextRefs,
		Status:                       req.Status,
		CreatedEventID:               req.CreatedEventID,
		CreatedAt:                    time.Now().UTC(),
		UpdatedAt:                    time.Now().UTC(),
	}
	r.transferRequests = append(r.transferRequests, transfer)
	return transfer, nil
}

func (r *memoryRepository) CompleteProjectTaskWriteback(ctx context.Context, req CompleteProjectTaskWritebackRequest) (ProjectTaskWritebackResult, error) {
	snapshot := r.writebackSnapshot()
	if _, err := r.UpdateProjectTaskStatus(ctx, req.Task.TenantID, req.Task.ID, "completed", nil, req.AllowedCurrentStatuses); err != nil {
		return ProjectTaskWritebackResult{}, err
	}
	event, err := r.AppendProjectEvent(ctx, req.Event)
	if err != nil {
		r.restoreWritebackSnapshot(snapshot)
		return ProjectTaskWritebackResult{}, err
	}
	summaryReq := req.Summary
	summaryReq.CreatedEventID = &event.ID
	summary, err := r.CreateExecutionSummary(ctx, summaryReq)
	if err != nil {
		r.restoreWritebackSnapshot(snapshot)
		return ProjectTaskWritebackResult{}, err
	}
	task, err := r.UpdateProjectTaskStatus(ctx, req.Task.TenantID, req.Task.ID, "completed", &event.ID, []string{"completed"})
	if err != nil {
		r.restoreWritebackSnapshot(snapshot)
		return ProjectTaskWritebackResult{}, err
	}
	return ProjectTaskWritebackResult{Task: task, Event: event, Summary: summary}, nil
}

func (r *memoryRepository) FailProjectTaskWriteback(ctx context.Context, req FailProjectTaskWritebackRequest) (ProjectTaskWritebackResult, error) {
	snapshot := r.writebackSnapshot()
	if _, err := r.UpdateProjectTaskStatus(ctx, req.Task.TenantID, req.Task.ID, "failed", nil, req.AllowedCurrentStatuses); err != nil {
		return ProjectTaskWritebackResult{}, err
	}
	event, err := r.AppendProjectEvent(ctx, req.Event)
	if err != nil {
		r.restoreWritebackSnapshot(snapshot)
		return ProjectTaskWritebackResult{}, err
	}
	task, err := r.UpdateProjectTaskStatus(ctx, req.Task.TenantID, req.Task.ID, "failed", &event.ID, []string{"failed"})
	if err != nil {
		r.restoreWritebackSnapshot(snapshot)
		return ProjectTaskWritebackResult{}, err
	}
	return ProjectTaskWritebackResult{Task: task, Event: event}, nil
}

func (r *memoryRepository) RequestProjectTaskTransferWriteback(ctx context.Context, req RequestProjectTaskTransferWritebackRequest) (ProjectTaskTransferWritebackResult, error) {
	snapshot := r.writebackSnapshot()
	if _, err := r.UpdateProjectTaskStatus(ctx, req.Task.TenantID, req.Task.ID, "waiting_human", nil, req.AllowedCurrentStatuses); err != nil {
		return ProjectTaskTransferWritebackResult{}, err
	}
	event, err := r.AppendProjectEvent(ctx, req.Event)
	if err != nil {
		r.restoreWritebackSnapshot(snapshot)
		return ProjectTaskTransferWritebackResult{}, err
	}
	transferReq := req.Transfer
	transferReq.CreatedEventID = &event.ID
	transfer, err := r.CreateTransferRequest(ctx, transferReq)
	if err != nil {
		r.restoreWritebackSnapshot(snapshot)
		return ProjectTaskTransferWritebackResult{}, err
	}
	task, err := r.UpdateProjectTaskStatus(ctx, req.Task.TenantID, req.Task.ID, "waiting_human", &event.ID, []string{"waiting_human"})
	if err != nil {
		r.restoreWritebackSnapshot(snapshot)
		return ProjectTaskTransferWritebackResult{}, err
	}
	return ProjectTaskTransferWritebackResult{Task: task, Event: event, Transfer: transfer}, nil
}

func (r *memoryRepository) StartProjectTaskAttemptWriteback(ctx context.Context, req StartProjectTaskAttemptRequest) (ProjectTaskAttemptWritebackResult, error) {
	for i, attempt := range r.projectTaskAttempts {
		if attempt.TenantID != req.TenantID || attempt.ID != req.AttemptID {
			continue
		}
		if attempt.LeaseToken != req.LeaseToken {
			return ProjectTaskAttemptWritebackResult{}, ErrProjectConflict
		}
		now := time.Now().UTC()
		attempt.Status = ProjectTaskAttemptStatusRunning
		attempt.RuntimeNodeID = &req.RuntimeNodeID
		attempt.ProviderSessionID = req.ProviderSessionID
		if attempt.StartedAt == nil {
			attempt.StartedAt = &now
		}
		attempt.RenewedAt = &now
		attempt.UpdatedAt = now
		r.projectTaskAttempts[i] = attempt
		task, err := r.UpdateProjectTaskStatus(ctx, req.TenantID, req.ProjectTaskID, ProjectTaskStatusRunning, nil, []string{ProjectTaskStatusQueued, ProjectTaskStatusRunning})
		if err != nil {
			return ProjectTaskAttemptWritebackResult{}, err
		}
		return ProjectTaskAttemptWritebackResult{Task: task, Attempt: attempt}, nil
	}
	return ProjectTaskAttemptWritebackResult{}, ErrProjectNotFound
}

func (r *memoryRepository) RenewProjectTaskAttemptLeaseWriteback(ctx context.Context, req RenewProjectTaskAttemptLeaseRequest) (ProjectTaskAttempt, error) {
	for i, attempt := range r.projectTaskAttempts {
		if attempt.TenantID != req.TenantID || attempt.ID != req.AttemptID {
			continue
		}
		if attempt.LeaseToken != req.LeaseToken {
			return ProjectTaskAttempt{}, ErrProjectConflict
		}
		now := time.Now().UTC()
		attempt.LeaseExpiresAt = req.LeaseExpiresAt
		attempt.RenewedAt = &now
		attempt.UpdatedAt = now
		r.projectTaskAttempts[i] = attempt
		return attempt, nil
	}
	return ProjectTaskAttempt{}, ErrProjectNotFound
}

func (r *memoryRepository) CompleteProjectTaskAttemptWriteback(ctx context.Context, req CompleteProjectTaskAttemptRequest) (ProjectTaskWritebackResult, error) {
	snapshot := r.writebackSnapshot()
	task, err := r.UpdateProjectTaskStatus(ctx, req.TenantID, req.ProjectTaskID, ProjectTaskStatusCompleted, nil, []string{ProjectTaskStatusQueued, ProjectTaskStatusRunning})
	if err != nil {
		return ProjectTaskWritebackResult{}, err
	}
	event, err := r.AppendProjectEvent(ctx, AppendProjectEventRequest{
		TenantID:     req.TenantID,
		ProjectID:    task.ProjectID,
		EventType:    ProjectEventTaskCompleted,
		ActorType:    "digital_employee",
		ActorID:      req.DigitalEmployeeID.String(),
		ResourceType: strPtr("project_task"),
		ResourceID:   strPtr(task.ID.String()),
		Summary:      "项目任务已完成",
		Payload:      map[string]any{"project_task_id": task.ID.String(), "project_task_attempt_id": req.AttemptID.String()},
	})
	if err != nil {
		r.restoreWritebackSnapshot(snapshot)
		return ProjectTaskWritebackResult{}, err
	}
	summary, err := r.CreateExecutionSummary(ctx, CreateExecutionSummaryRequest{
		TenantID:              req.TenantID,
		ProjectID:             task.ProjectID,
		ProjectTaskID:         task.ID,
		DigitalEmployeeID:     req.DigitalEmployeeID,
		Conclusion:            req.Conclusion,
		EvidenceRefs:          sliceOrEmptyAny(req.EvidenceRefs),
		ArtifactRefs:          sliceOrEmptyAny(req.ArtifactRefs),
		ConfidenceFactors:     mapOrEmptyAny(req.ConfidenceFactors),
		Uncertainty:           strings.TrimSpace(req.Uncertainty),
		MissingInformation:    sliceOrEmptyAny(req.MissingInformation),
		RecommendedNextAction: strings.TrimSpace(req.RecommendedNextAction),
		RequiresHumanReview:   req.RequiresHumanReview,
		CreatedEventID:        &event.ID,
	})
	if err != nil {
		r.restoreWritebackSnapshot(snapshot)
		return ProjectTaskWritebackResult{}, err
	}
	if err := r.finishProjectTaskAttempt(req.TenantID, req.AttemptID, ProjectTaskAttemptStatusSucceeded, &event.ID, nil, nil, nil); err != nil {
		r.restoreWritebackSnapshot(snapshot)
		return ProjectTaskWritebackResult{}, err
	}
	for _, ledgerReq := range projectTaskAttemptCompletionLedgerEventRequests(req, task, event, summary, req.RequiresHumanReview) {
		if _, err := r.CreateExecutionLedgerEvent(ctx, ledgerReq); err != nil {
			r.restoreWritebackSnapshot(snapshot)
			return ProjectTaskWritebackResult{}, err
		}
	}
	task, err = r.UpdateProjectTaskStatus(ctx, req.TenantID, req.ProjectTaskID, ProjectTaskStatusCompleted, &event.ID, []string{ProjectTaskStatusCompleted})
	if err != nil {
		r.restoreWritebackSnapshot(snapshot)
		return ProjectTaskWritebackResult{}, err
	}
	return ProjectTaskWritebackResult{Task: task, Event: event, Summary: summary}, nil
}

func (r *memoryRepository) CompleteProjectTaskAttemptResultWriteback(ctx context.Context, req CompleteProjectTaskAttemptResultWritebackRequest) (ProjectTaskWritebackResult, error) {
	snapshot := r.writebackSnapshot()
	result, err := r.CompleteProjectTaskAttemptWriteback(ctx, req.Complete)
	if err != nil {
		return ProjectTaskWritebackResult{}, err
	}
	if _, err := recordProjectTaskResultForMemoryWriteback(ctx, r, req.Result, result); err != nil {
		r.restoreWritebackSnapshot(snapshot)
		return ProjectTaskWritebackResult{}, err
	}
	return result, nil
}

func (r *projectTaskResultMemoryRepository) CompleteProjectTaskAttemptResultWriteback(ctx context.Context, req CompleteProjectTaskAttemptResultWritebackRequest) (ProjectTaskWritebackResult, error) {
	snapshot := r.writebackSnapshot()
	resultSnapshot := append([]ProjectTaskResult(nil), r.projectTaskResults...)
	result, err := r.memoryRepository.CompleteProjectTaskAttemptWriteback(ctx, req.Complete)
	if err != nil {
		return ProjectTaskWritebackResult{}, err
	}
	if _, err := recordProjectTaskResultForMemoryWriteback(ctx, r, req.Result, result); err != nil {
		r.restoreWritebackSnapshot(snapshot)
		r.projectTaskResults = resultSnapshot
		return ProjectTaskWritebackResult{}, err
	}
	return result, nil
}

func recordProjectTaskResultForMemoryWriteback(ctx context.Context, repository interface {
	RecordProjectTaskResult(context.Context, RecordProjectTaskResultRequest) (ProjectTaskResult, error)
	LinkProjectTaskLatestResult(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, uuid.UUID) (ProjectTask, error)
}, req RecordProjectTaskResultRequest, result ProjectTaskWritebackResult) (ProjectTaskResult, error) {
	req.ExecutionSummaryID = &result.Summary.ID
	req.CreatedEventID = &result.Event.ID
	taskResult, err := repository.RecordProjectTaskResult(ctx, req)
	if err != nil {
		return ProjectTaskResult{}, err
	}
	if _, err := repository.LinkProjectTaskLatestResult(ctx, req.TenantID, req.ProjectID, req.ProjectTaskID, taskResult.ID); err != nil {
		return ProjectTaskResult{}, err
	}
	return taskResult, nil
}

func (r *projectTaskResultMemoryRepository) RecordProjectTaskResult(ctx context.Context, req RecordProjectTaskResultRequest) (ProjectTaskResult, error) {
	if r.recordProjectTaskResultErr != nil {
		return ProjectTaskResult{}, r.recordProjectTaskResultErr
	}
	for _, result := range r.projectTaskResults {
		if result.TenantID == req.TenantID && result.IdempotencyKey == req.IdempotencyKey {
			return result, nil
		}
	}
	now := time.Now().UTC()
	result := ProjectTaskResult{
		ID:                 uuid.New(),
		TenantID:           req.TenantID,
		ProjectID:          req.ProjectID,
		ProjectTaskID:      req.ProjectTaskID,
		AttemptID:          req.AttemptID,
		ExecutionSummaryID: req.ExecutionSummaryID,
		ResultStatus:       req.ResultStatus,
		ValidationStatus:   req.ValidationStatus,
		Decision:           req.Decision,
		Contract:           req.Contract,
		ValidationErrors:   append([]string(nil), req.ValidationErrors...),
		ValidationWarnings: append([]string(nil), req.ValidationWarnings...),
		IdempotencyKey:     req.IdempotencyKey,
		DecisionRequestID:  req.DecisionRequestID,
		RevisionTaskID:     req.RevisionTaskID,
		CreatedEventID:     req.CreatedEventID,
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	r.projectTaskResults = append(r.projectTaskResults, result)
	return result, nil
}

func (r *projectTaskResultMemoryRepository) ListProjectTaskResults(ctx context.Context, req ListProjectTaskResultsRequest) ([]ProjectTaskResult, error) {
	results := make([]ProjectTaskResult, 0, len(r.projectTaskResults))
	for _, result := range r.projectTaskResults {
		if result.TenantID == req.TenantID && result.ProjectID == req.ProjectID && result.ProjectTaskID == req.ProjectTaskID {
			results = append(results, result)
		}
	}
	sort.SliceStable(results, func(i, j int) bool {
		return results[i].CreatedAt.After(results[j].CreatedAt)
	})
	return paginateTestSlice(results, req.Limit, req.Offset), nil
}

func (r *projectTaskResultMemoryRepository) LinkProjectTaskLatestResult(ctx context.Context, tenantID, projectID, projectTaskID, resultID uuid.UUID) (ProjectTask, error) {
	if r.linkProjectTaskLatestResultErr != nil {
		return ProjectTask{}, r.linkProjectTaskLatestResultErr
	}
	for _, result := range r.projectTaskResults {
		if result.TenantID != tenantID || result.ProjectID != projectID || result.ProjectTaskID != projectTaskID || result.ID != resultID {
			continue
		}
		for i, task := range r.tasks {
			if task.TenantID == tenantID && task.ProjectID == projectID && task.ID == projectTaskID {
				task.LatestTaskResultID = &resultID
				task.UpdatedAt = time.Now().UTC()
				r.tasks[i] = task
				return task, nil
			}
		}
		return ProjectTask{}, ErrProjectNotFound
	}
	return ProjectTask{}, ErrProjectConflict
}

func (r *memoryRepository) CompleteProjectTaskAttemptAcceptanceWriteback(ctx context.Context, req CompleteProjectTaskAttemptAcceptanceWritebackRequest) (ProjectTaskWritebackResult, error) {
	snapshot := r.writebackSnapshot()
	event, err := r.AppendProjectEvent(ctx, AppendProjectEventRequest{
		TenantID:     req.Complete.TenantID,
		ProjectID:    req.Task.ProjectID,
		EventType:    ProjectEventTaskWaitingHuman,
		ActorType:    "digital_employee",
		ActorID:      req.Complete.DigitalEmployeeID.String(),
		ResourceType: strPtr("project_task"),
		ResourceID:   strPtr(req.Task.ID.String()),
		Summary:      "项目任务等待验收",
		Payload: map[string]any{
			"project_task_id":         req.Complete.ProjectTaskID.String(),
			"project_task_attempt_id": req.Complete.AttemptID.String(),
			"waiting_reason":          HumanWaitReasonAcceptanceRequired,
		},
	})
	if err != nil {
		r.restoreWritebackSnapshot(snapshot)
		return ProjectTaskWritebackResult{}, err
	}
	summary, err := r.CreateExecutionSummary(ctx, CreateExecutionSummaryRequest{
		TenantID:              req.Complete.TenantID,
		ProjectID:             req.Task.ProjectID,
		ProjectTaskID:         req.Task.ID,
		DigitalEmployeeID:     req.Complete.DigitalEmployeeID,
		Conclusion:            req.Complete.Conclusion,
		EvidenceRefs:          sliceOrEmptyAny(req.Complete.EvidenceRefs),
		ArtifactRefs:          sliceOrEmptyAny(req.Complete.ArtifactRefs),
		ConfidenceFactors:     mapOrEmptyAny(req.Complete.ConfidenceFactors),
		Uncertainty:           strings.TrimSpace(req.Complete.Uncertainty),
		MissingInformation:    sliceOrEmptyAny(req.Complete.MissingInformation),
		RecommendedNextAction: strings.TrimSpace(req.Complete.RecommendedNextAction),
		RequiresHumanReview:   true,
		CreatedEventID:        &event.ID,
	})
	if err != nil {
		r.restoreWritebackSnapshot(snapshot)
		return ProjectTaskWritebackResult{}, err
	}
	if err := r.finishProjectTaskAttempt(req.Complete.TenantID, req.Complete.AttemptID, ProjectTaskAttemptStatusSucceeded, &event.ID, nil, nil, nil); err != nil {
		r.restoreWritebackSnapshot(snapshot)
		return ProjectTaskWritebackResult{}, err
	}
	for _, ledgerReq := range projectTaskAttemptCompletionLedgerEventRequests(req.Complete, req.Task, event, summary, true) {
		if _, err := r.CreateExecutionLedgerEvent(ctx, ledgerReq); err != nil {
			r.restoreWritebackSnapshot(snapshot)
			return ProjectTaskWritebackResult{}, err
		}
	}
	decisionReq := req.Decision
	decisionReq.CreatedEventID = &event.ID
	decision, err := r.CreateDecisionRequest(ctx, decisionReq)
	if err != nil {
		r.restoreWritebackSnapshot(snapshot)
		return ProjectTaskWritebackResult{}, err
	}
	task, err := r.moveProjectTaskToWaitingHumanWithRequest(req.Complete.TenantID, req.Complete.ProjectTaskID, HumanWaitReasonAcceptanceRequired, &decision.ID)
	if err != nil {
		r.restoreWritebackSnapshot(snapshot)
		return ProjectTaskWritebackResult{}, err
	}
	return ProjectTaskWritebackResult{Task: task, Event: event, Summary: summary, Decision: decision}, nil
}

func (r *memoryRepository) CompleteProjectTaskAttemptAcceptanceResultWriteback(ctx context.Context, req CompleteProjectTaskAttemptAcceptanceResultWritebackRequest) (ProjectTaskWritebackResult, error) {
	snapshot := r.writebackSnapshot()
	result, err := r.CompleteProjectTaskAttemptAcceptanceWriteback(ctx, req.Acceptance)
	if err != nil {
		return ProjectTaskWritebackResult{}, err
	}
	if _, err := recordProjectTaskResultForMemoryWriteback(ctx, r, req.Result, result); err != nil {
		r.restoreWritebackSnapshot(snapshot)
		return ProjectTaskWritebackResult{}, err
	}
	return result, nil
}

func (r *projectTaskResultMemoryRepository) CompleteProjectTaskAttemptAcceptanceResultWriteback(ctx context.Context, req CompleteProjectTaskAttemptAcceptanceResultWritebackRequest) (ProjectTaskWritebackResult, error) {
	snapshot := r.writebackSnapshot()
	resultSnapshot := append([]ProjectTaskResult(nil), r.projectTaskResults...)
	result, err := r.memoryRepository.CompleteProjectTaskAttemptAcceptanceWriteback(ctx, req.Acceptance)
	if err != nil {
		return ProjectTaskWritebackResult{}, err
	}
	if _, err := recordProjectTaskResultForMemoryWriteback(ctx, r, req.Result, result); err != nil {
		r.restoreWritebackSnapshot(snapshot)
		r.projectTaskResults = resultSnapshot
		return ProjectTaskWritebackResult{}, err
	}
	return result, nil
}

func (r *memoryRepository) FailProjectTaskAttemptWriteback(ctx context.Context, req FailProjectTaskAttemptRequest) (ProjectTaskWritebackResult, error) {
	snapshot := r.writebackSnapshot()
	task, err := r.UpdateProjectTaskStatus(ctx, req.TenantID, req.ProjectTaskID, ProjectTaskStatusFailed, nil, []string{ProjectTaskStatusQueued, ProjectTaskStatusRunning})
	if err != nil {
		return ProjectTaskWritebackResult{}, err
	}
	event, err := r.AppendProjectEvent(ctx, AppendProjectEventRequest{
		TenantID:     req.TenantID,
		ProjectID:    task.ProjectID,
		EventType:    ProjectEventTaskFailed,
		ActorType:    "digital_employee",
		ActorID:      req.DigitalEmployeeID.String(),
		ResourceType: strPtr("project_task"),
		ResourceID:   strPtr(task.ID.String()),
		Summary:      "项目任务执行失败",
		Payload: map[string]any{
			"project_task_id":         task.ID.String(),
			"project_task_attempt_id": req.AttemptID.String(),
			"failure_summary":         req.FailureSummary,
			"failure_family":          req.FailureFamily,
		},
	})
	if err != nil {
		r.restoreWritebackSnapshot(snapshot)
		return ProjectTaskWritebackResult{}, err
	}
	if err := r.finishProjectTaskAttempt(req.TenantID, req.AttemptID, ProjectTaskAttemptStatusFailed, &event.ID, req.Retryable, &req.FailureFamily, &req.FailureSummary); err != nil {
		r.restoreWritebackSnapshot(snapshot)
		return ProjectTaskWritebackResult{}, err
	}
	if _, err := r.CreateExecutionLedgerEvent(ctx, projectTaskAttemptFailureLedgerEventRequest(req, task, event)); err != nil {
		r.restoreWritebackSnapshot(snapshot)
		return ProjectTaskWritebackResult{}, err
	}
	task, err = r.UpdateProjectTaskStatus(ctx, req.TenantID, req.ProjectTaskID, ProjectTaskStatusFailed, &event.ID, []string{ProjectTaskStatusFailed})
	if err != nil {
		r.restoreWritebackSnapshot(snapshot)
		return ProjectTaskWritebackResult{}, err
	}
	return ProjectTaskWritebackResult{Task: task, Event: event}, nil
}

func (r *memoryRepository) RecoverProjectTaskAttemptFailureWriteback(ctx context.Context, req RecoverProjectTaskAttemptFailureWritebackRequest) (ProjectTaskWritebackResult, error) {
	snapshot := r.writebackSnapshot()
	eventType := ProjectEventTaskFailed
	summary := "项目任务执行失败"
	if req.TaskTargetStatus == ProjectTaskStatusCancelled {
		eventType = ProjectEventTaskCancelled
		summary = "项目任务已取消"
	}
	payload := map[string]any{
		"project_task_id":         req.Failure.ProjectTaskID.String(),
		"project_task_attempt_id": req.Failure.AttemptID.String(),
		"failure_summary":         req.Failure.FailureSummary,
		"failure_family":          req.Failure.FailureFamily,
		"recovery_status":         req.TaskTargetStatus,
	}
	if req.TaskTargetStatus == ProjectTaskStatusWaitingHuman {
		payload["waiting_reason"] = req.WaitingReason
	}
	if req.TaskTargetStatus == ProjectTaskStatusQueued {
		payload["retry_project_task_attempt_id"] = req.RetryAttemptID.String()
	}
	event, err := r.AppendProjectEvent(ctx, AppendProjectEventRequest{
		TenantID:     req.Failure.TenantID,
		ProjectID:    req.Task.ProjectID,
		EventType:    eventType,
		ActorType:    "digital_employee",
		ActorID:      req.Failure.DigitalEmployeeID.String(),
		ResourceType: strPtr("project_task"),
		ResourceID:   strPtr(req.Task.ID.String()),
		Summary:      summary,
		Payload:      payload,
	})
	if err != nil {
		r.restoreWritebackSnapshot(snapshot)
		return ProjectTaskWritebackResult{}, err
	}
	if err := r.finishProjectTaskAttempt(req.Failure.TenantID, req.Failure.AttemptID, req.AttemptTerminalStatus, &event.ID, req.Failure.Retryable, &req.Failure.FailureFamily, &req.Failure.FailureSummary); err != nil {
		r.restoreWritebackSnapshot(snapshot)
		return ProjectTaskWritebackResult{}, err
	}
	if ledgerReq, ok := recoveredProjectTaskAttemptLedgerEventRequest(req, event); ok {
		if _, err := r.CreateExecutionLedgerEvent(ctx, ledgerReq); err != nil {
			r.restoreWritebackSnapshot(snapshot)
			return ProjectTaskWritebackResult{}, err
		}
	}

	switch req.TaskTargetStatus {
	case ProjectTaskStatusQueued:
		task, err := r.scheduleProjectTaskRetry(req, &event.ID)
		if err != nil {
			r.restoreWritebackSnapshot(snapshot)
			return ProjectTaskWritebackResult{}, err
		}
		return ProjectTaskWritebackResult{Task: task, Event: event}, nil
	case ProjectTaskStatusWaitingHuman:
		task, err := r.moveProjectTaskToWaitingHuman(req, &event.ID)
		if err != nil {
			r.restoreWritebackSnapshot(snapshot)
			return ProjectTaskWritebackResult{}, err
		}
		return ProjectTaskWritebackResult{Task: task, Event: event}, nil
	case ProjectTaskStatusFailed, ProjectTaskStatusCancelled:
		task, err := r.UpdateProjectTaskStatus(ctx, req.Failure.TenantID, req.Failure.ProjectTaskID, req.TaskTargetStatus, &event.ID, []string{ProjectTaskStatusQueued, ProjectTaskStatusRunning})
		if err != nil {
			r.restoreWritebackSnapshot(snapshot)
			return ProjectTaskWritebackResult{}, err
		}
		return ProjectTaskWritebackResult{Task: task, Event: event}, nil
	default:
		r.restoreWritebackSnapshot(snapshot)
		return ProjectTaskWritebackResult{}, ErrInvalidProject
	}
}

func (r *memoryRepository) WaitHumanProjectTaskAttemptWriteback(ctx context.Context, req WaitHumanProjectTaskAttemptWritebackRequest) (ProjectTaskWritebackResult, error) {
	snapshot := r.writebackSnapshot()
	event, err := r.AppendProjectEvent(ctx, AppendProjectEventRequest{
		TenantID:     req.Wait.TenantID,
		ProjectID:    req.Task.ProjectID,
		EventType:    ProjectEventTaskWaitingHuman,
		ActorType:    "digital_employee",
		ActorID:      req.Wait.DigitalEmployeeID.String(),
		ResourceType: strPtr("project_task"),
		ResourceID:   strPtr(req.Task.ID.String()),
		Summary:      req.Wait.Summary,
		Payload: map[string]any{
			"project_task_id":              req.Wait.ProjectTaskID.String(),
			"project_task_attempt_id":      req.Wait.AttemptID.String(),
			"reason":                       req.Wait.Reason,
			"summary":                      req.Wait.Summary,
			"missing_context_refs":         sliceOrEmptyAny(req.Wait.MissingContextRefs),
			"suggested_resolution_options": req.Wait.SuggestedResolutionOptions,
		},
	})
	if err != nil {
		r.restoreWritebackSnapshot(snapshot)
		return ProjectTaskWritebackResult{}, err
	}
	if err := r.finishProjectTaskAttempt(req.Wait.TenantID, req.Wait.AttemptID, ProjectTaskAttemptStatusWaitingHuman, &event.ID, nil, nil, &req.Wait.Summary); err != nil {
		r.restoreWritebackSnapshot(snapshot)
		return ProjectTaskWritebackResult{}, err
	}
	decisionReq := req.Decision
	decisionReq.CreatedEventID = &event.ID
	decision, err := r.CreateDecisionRequest(ctx, decisionReq)
	if err != nil {
		r.restoreWritebackSnapshot(snapshot)
		return ProjectTaskWritebackResult{}, err
	}
	if _, err := r.CreateExecutionLedgerEvent(ctx, projectTaskAttemptHumanWaitLedgerEventRequest(req, event, decision)); err != nil {
		r.restoreWritebackSnapshot(snapshot)
		return ProjectTaskWritebackResult{}, err
	}
	task, err := r.moveProjectTaskToWaitingHumanWithRequest(req.Wait.TenantID, req.Wait.ProjectTaskID, req.Wait.Reason, &decision.ID)
	if err != nil {
		r.restoreWritebackSnapshot(snapshot)
		return ProjectTaskWritebackResult{}, err
	}
	return ProjectTaskWritebackResult{Task: task, Event: event, Decision: decision}, nil
}

func (r *memoryRepository) moveProjectTaskToWaitingHumanWithRequest(tenantID, projectTaskID uuid.UUID, waitingReason string, waitingRequestID *uuid.UUID) (ProjectTask, error) {
	for index, task := range r.tasks {
		if task.TenantID != tenantID || task.ID != projectTaskID {
			continue
		}
		if task.Status != ProjectTaskStatusQueued && task.Status != ProjectTaskStatusRunning {
			return ProjectTask{}, ErrProjectConflict
		}
		now := time.Now().UTC()
		task.Status = ProjectTaskStatusWaitingHuman
		task.WaitingReason = &waitingReason
		task.WaitingRequestID = waitingRequestID
		task.StatusChangedAt = now
		task.UpdatedAt = now
		r.tasks[index] = task
		return task, nil
	}
	return ProjectTask{}, ErrProjectNotFound
}

func (r *memoryRepository) ResolveProjectTaskHumanWaitWriteback(ctx context.Context, req ResolveProjectTaskHumanWaitWritebackRequest) (ProjectTaskWritebackResult, error) {
	snapshot := r.writebackSnapshot()
	eventType := ProjectEventTaskCompleted
	summary := "项目任务等待已处理"
	switch req.TargetStatus {
	case ProjectTaskStatusQueued:
		eventType = ProjectEventTaskDispatched
		summary = "项目任务已恢复排队"
	case ProjectTaskStatusCancelled:
		eventType = ProjectEventTaskCancelled
		summary = "项目任务已取消"
	case ProjectTaskStatusFailed:
		eventType = ProjectEventTaskFailed
		summary = "项目任务已标记失败"
	}
	event, err := r.AppendProjectEvent(ctx, AppendProjectEventRequest{
		TenantID:     req.Resolve.TenantID,
		ProjectID:    req.Resolve.ProjectID,
		EventType:    eventType,
		ActorType:    "human_user",
		ActorID:      req.Resolve.ActorUserID.String(),
		ResourceType: strPtr("project_task"),
		ResourceID:   strPtr(req.Resolve.ProjectTaskID.String()),
		Summary:      summary,
		Payload: map[string]any{
			"project_task_id":  req.Resolve.ProjectTaskID.String(),
			"resolution":       req.Resolve.Resolution,
			"response_summary": req.Resolve.ResponseSummary,
			"context_refs":     sliceOrEmptyAny(req.Resolve.ContextRefs),
		},
	})
	if err != nil {
		r.restoreWritebackSnapshot(snapshot)
		return ProjectTaskWritebackResult{}, err
	}
	var task ProjectTask
	switch req.TargetStatus {
	case ProjectTaskStatusQueued:
		task, err = r.resumeProjectTaskAfterHumanWait(req, &event.ID)
	case ProjectTaskStatusCompleted, ProjectTaskStatusCancelled, ProjectTaskStatusFailed:
		task, err = r.UpdateProjectTaskStatus(ctx, req.Resolve.TenantID, req.Resolve.ProjectTaskID, req.TargetStatus, &event.ID, []string{ProjectTaskStatusWaitingHuman})
	default:
		err = ErrInvalidProject
	}
	if err != nil {
		r.restoreWritebackSnapshot(snapshot)
		return ProjectTaskWritebackResult{}, err
	}
	return ProjectTaskWritebackResult{Task: task, Event: event}, nil
}

func (r *memoryRepository) resumeProjectTaskAfterHumanWait(req ResolveProjectTaskHumanWaitWritebackRequest, eventID *uuid.UUID) (ProjectTask, error) {
	if req.CurrentAttempt.ID == uuid.Nil {
		return ProjectTask{}, ErrProjectNotFound
	}
	for index, task := range r.tasks {
		if task.TenantID != req.Resolve.TenantID || task.ID != req.Resolve.ProjectTaskID {
			continue
		}
		if task.Status != ProjectTaskStatusWaitingHuman {
			return ProjectTask{}, ErrProjectConflict
		}
		attemptReq := QueueProjectTaskRequest{
			TenantID:                      task.TenantID,
			ProjectID:                     task.ProjectID,
			ProjectTaskID:                 task.ID,
			ProjectTaskAttemptID:          &req.RetryAttemptID,
			DigitalEmployeeID:             *task.AssignedDigitalEmployeeID,
			DigitalEmployeeRunID:          req.CurrentAttempt.DigitalEmployeeRunID,
			RuntimeTaskID:                 req.CurrentAttempt.RuntimeTaskID,
			RuntimeNodeID:                 req.CurrentAttempt.RuntimeNodeID,
			IdempotencyKey:                req.RetryIdempotencyKey,
			LeaseToken:                    req.RetryLeaseToken,
			ExecutionContextPacket:        req.CurrentAttempt.ExecutionContextPacket,
			ExecutionContextPacketVersion: req.CurrentAttempt.ExecutionContextPacketVersion,
		}
		attempt := r.createProjectTaskAttempt(attemptReq, task.AttemptCount+1, eventID)
		attempt.ID = req.RetryAttemptID
		r.projectTaskAttempts[len(r.projectTaskAttempts)-1] = attempt
		now := time.Now().UTC()
		task.Status = ProjectTaskStatusQueued
		task.CurrentAttemptID = &attempt.ID
		task.AttemptCount++
		task.WaitingReason = nil
		task.WaitingRequestID = nil
		task.StatusChangedAt = now
		task.UpdatedAt = now
		r.tasks[index] = task
		return task, nil
	}
	return ProjectTask{}, ErrProjectNotFound
}

func (r *memoryRepository) scheduleProjectTaskRetry(req RecoverProjectTaskAttemptFailureWritebackRequest, eventID *uuid.UUID) (ProjectTask, error) {
	retryAttemptID := req.RetryAttemptID
	if retryAttemptID == uuid.Nil {
		retryAttemptID = uuid.New()
	}
	retryLeaseToken := strings.TrimSpace(req.RetryLeaseToken)
	if retryLeaseToken == "" {
		retryLeaseToken = "retry-" + uuid.NewString()
	}
	retryIdempotencyKey := strings.TrimSpace(req.RetryIdempotencyKey)
	if retryIdempotencyKey == "" {
		retryIdempotencyKey = "project-task:" + req.Task.ID.String() + ":attempt:" + fmt.Sprint(req.Task.AttemptCount+1) + ":retry"
	}
	for index, task := range r.tasks {
		if task.TenantID != req.Failure.TenantID || task.ID != req.Failure.ProjectTaskID {
			continue
		}
		if task.Status != ProjectTaskStatusRunning && task.Status != ProjectTaskStatusWaitingHuman {
			return ProjectTask{}, ErrProjectConflict
		}
		attemptReq := QueueProjectTaskRequest{
			TenantID:                      task.TenantID,
			ProjectID:                     task.ProjectID,
			ProjectTaskID:                 task.ID,
			ProjectTaskAttemptID:          &retryAttemptID,
			DigitalEmployeeID:             req.Failure.DigitalEmployeeID,
			DigitalEmployeeRunID:          req.Attempt.DigitalEmployeeRunID,
			RuntimeTaskID:                 req.Attempt.RuntimeTaskID,
			RuntimeNodeID:                 req.Attempt.RuntimeNodeID,
			IdempotencyKey:                retryIdempotencyKey,
			LeaseToken:                    retryLeaseToken,
			ExecutionContextPacket:        req.Attempt.ExecutionContextPacket,
			ExecutionContextPacketVersion: req.Attempt.ExecutionContextPacketVersion,
		}
		attempt := r.createProjectTaskAttempt(attemptReq, task.AttemptCount+1, eventID)
		attempt.ID = retryAttemptID
		r.projectTaskAttempts[len(r.projectTaskAttempts)-1] = attempt
		now := time.Now().UTC()
		task.Status = ProjectTaskStatusQueued
		task.CurrentAttemptID = &attempt.ID
		task.AttemptCount++
		task.RetryNotBefore = req.RetryNotBefore
		task.WaitingReason = nil
		task.WaitingRequestID = nil
		task.StatusChangedAt = now
		task.UpdatedAt = now
		r.tasks[index] = task
		return task, nil
	}
	return ProjectTask{}, ErrProjectNotFound
}

func (r *memoryRepository) moveProjectTaskToWaitingHuman(req RecoverProjectTaskAttemptFailureWritebackRequest, eventID *uuid.UUID) (ProjectTask, error) {
	for index, task := range r.tasks {
		if task.TenantID != req.Failure.TenantID || task.ID != req.Failure.ProjectTaskID {
			continue
		}
		if task.Status != ProjectTaskStatusQueued && task.Status != ProjectTaskStatusRunning {
			return ProjectTask{}, ErrProjectConflict
		}
		waitingReason := req.WaitingReason
		now := time.Now().UTC()
		task.Status = ProjectTaskStatusWaitingHuman
		task.WaitingReason = &waitingReason
		task.WaitingRequestID = nil
		task.StatusChangedAt = now
		task.UpdatedAt = now
		r.tasks[index] = task
		return task, nil
	}
	return ProjectTask{}, ErrProjectNotFound
}

func (r *memoryRepository) finishProjectTaskAttempt(tenantID, attemptID uuid.UUID, status string, terminalEventID *uuid.UUID, retryable *bool, failureFamily, failureMessage *string) error {
	for i, attempt := range r.projectTaskAttempts {
		if attempt.TenantID != tenantID || attempt.ID != attemptID {
			continue
		}
		now := time.Now().UTC()
		attempt.Status = status
		attempt.FinishedAt = &now
		attempt.TerminalEventID = terminalEventID
		attempt.Retryable = retryable
		attempt.FailureFamily = failureFamily
		attempt.FailureMessage = failureMessage
		attempt.UpdatedAt = now
		r.projectTaskAttempts[i] = attempt
		return nil
	}
	return ErrProjectNotFound
}

type memoryWritebackSnapshot struct {
	tasks                 []ProjectTask
	projectTaskAttempts   []ProjectTaskAttempt
	events                []ProjectEvent
	eventTypes            []ProjectEventType
	executionSummaries    []ExecutionSummary
	executionLedgerEvents []ExecutionLedgerEvent
	transferRequests      []TransferRequest
	decisionRequests      []DecisionRequest
}

func (r *memoryRepository) writebackSnapshot() memoryWritebackSnapshot {
	return memoryWritebackSnapshot{
		tasks:                 append([]ProjectTask(nil), r.tasks...),
		projectTaskAttempts:   append([]ProjectTaskAttempt(nil), r.projectTaskAttempts...),
		events:                append([]ProjectEvent(nil), r.events...),
		eventTypes:            append([]ProjectEventType(nil), r.eventTypes...),
		executionSummaries:    append([]ExecutionSummary(nil), r.executionSummaries...),
		executionLedgerEvents: append([]ExecutionLedgerEvent(nil), r.executionLedgerEvents...),
		transferRequests:      append([]TransferRequest(nil), r.transferRequests...),
		decisionRequests:      append([]DecisionRequest(nil), r.decisionRequests...),
	}
}

func (r *memoryRepository) restoreWritebackSnapshot(snapshot memoryWritebackSnapshot) {
	r.tasks = snapshot.tasks
	r.projectTaskAttempts = snapshot.projectTaskAttempts
	r.events = snapshot.events
	r.eventTypes = snapshot.eventTypes
	r.executionSummaries = snapshot.executionSummaries
	r.executionLedgerEvents = snapshot.executionLedgerEvents
	r.transferRequests = snapshot.transferRequests
	r.decisionRequests = snapshot.decisionRequests
}

func (r *memoryRepository) ListTransferRequests(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]TransferRequest, error) {
	filtered := make([]TransferRequest, 0, len(r.transferRequests))
	for _, transfer := range r.transferRequests {
		if transfer.TenantID == tenantID && transfer.ProjectID == projectID {
			filtered = append(filtered, transfer)
		}
	}
	return filtered, nil
}

func (r *memoryRepository) CreateDecisionRequest(ctx context.Context, req CreateDecisionRequestRequest) (DecisionRequest, error) {
	decision := DecisionRequest{
		ID:                uuid.New(),
		TenantID:          req.TenantID,
		ProjectID:         req.ProjectID,
		ApprovalRequestID: req.ApprovalRequestID,
		CoordinationJobID: req.CoordinationJobID,
		ProjectTaskID:     req.ProjectTaskID,
		TargetUserID:      req.TargetUserID,
		DecisionType:      req.DecisionType,
		TitleSnapshot:     req.TitleSnapshot,
		SummarySnapshot:   strPtrOrNil(req.SummarySnapshot),
		RiskLevelSnapshot: strPtrOrNil(req.RiskLevelSnapshot),
		StatusSnapshot:    req.StatusSnapshot,
		CreatedEventID:    req.CreatedEventID,
		CreatedAt:         time.Now().UTC(),
		UpdatedAt:         time.Now().UTC(),
	}
	r.decisionRequests = append(r.decisionRequests, decision)
	return decision, nil
}

func (r *memoryRepository) GetDecisionRequest(ctx context.Context, tenantID, projectID, decisionRequestID uuid.UUID) (DecisionRequest, error) {
	for _, decision := range r.decisionRequests {
		if decision.ID == decisionRequestID && decision.TenantID == tenantID && decision.ProjectID == projectID {
			return decision, nil
		}
	}
	return DecisionRequest{}, ErrProjectNotFound
}

func (r *memoryRepository) ResolveDecisionRequest(ctx context.Context, req ResolveDecisionRequestRepositoryRequest) (DecisionRequest, error) {
	for index, decision := range r.decisionRequests {
		if decision.ID == req.ID && decision.TenantID == req.TenantID && decision.ProjectID == req.ProjectID {
			now := time.Now().UTC()
			decision.StatusSnapshot = req.StatusSnapshot
			decision.ResolvedEventID = req.ResolvedEventID
			decision.ResolvedAt = &now
			decision.UpdatedAt = now
			r.decisionRequests[index] = decision
			return decision, nil
		}
	}
	return DecisionRequest{}, ErrProjectNotFound
}

func (r *memoryRepository) ListDecisionRequests(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]DecisionRequest, error) {
	filtered := make([]DecisionRequest, 0, len(r.decisionRequests))
	for _, decision := range r.decisionRequests {
		if decision.TenantID == tenantID && decision.ProjectID == projectID {
			filtered = append(filtered, decision)
		}
	}
	sort.SliceStable(filtered, func(i, j int) bool {
		return filtered[i].CreatedAt.After(filtered[j].CreatedAt)
	})
	return paginateTestSlice(filtered, limit, offset), nil
}

func (r *memoryRepository) ListDemandLaunchDecisionRequests(ctx context.Context, tenantID, projectID uuid.UUID, coordinationJobIDs, projectTaskIDs []uuid.UUID, limit int32) ([]DecisionRequest, error) {
	jobIDs := map[uuid.UUID]struct{}{}
	for _, id := range coordinationJobIDs {
		jobIDs[id] = struct{}{}
	}
	taskIDs := map[uuid.UUID]struct{}{}
	for _, id := range projectTaskIDs {
		taskIDs[id] = struct{}{}
	}
	filtered := make([]DecisionRequest, 0, len(r.decisionRequests))
	for _, decision := range r.decisionRequests {
		if decision.TenantID != tenantID || decision.ProjectID != projectID {
			continue
		}
		if decision.CoordinationJobID != nil {
			if _, ok := jobIDs[*decision.CoordinationJobID]; ok {
				filtered = append(filtered, decision)
				continue
			}
		}
		if decision.ProjectTaskID != nil {
			if _, ok := taskIDs[*decision.ProjectTaskID]; ok {
				filtered = append(filtered, decision)
			}
		}
	}
	sort.SliceStable(filtered, func(i, j int) bool {
		return filtered[i].CreatedAt.After(filtered[j].CreatedAt)
	})
	return paginateTestSlice(filtered, limit, 0), nil
}

func (r *memoryRepository) ListDemandLaunchEvents(ctx context.Context, tenantID, projectID, demandID uuid.UUID, createdEventID *uuid.UUID, projectTaskIDs, decisionRequestIDs []uuid.UUID, limit int32) ([]ProjectEvent, error) {
	candidates := make([]ProjectEvent, 0, len(r.events))
	for _, event := range r.events {
		if event.TenantID == tenantID && event.ProjectID == projectID {
			candidates = append(candidates, event)
		}
	}
	tasks := make([]ProjectTask, 0, len(projectTaskIDs))
	for _, id := range projectTaskIDs {
		tasks = append(tasks, ProjectTask{ID: id})
	}
	decisions := make([]DecisionRequest, 0, len(decisionRequestIDs))
	for _, id := range decisionRequestIDs {
		decisions = append(decisions, DecisionRequest{ID: id})
	}
	filtered := filterEventsForDemand(candidates, ProjectDemand{ID: demandID, CreatedEventID: createdEventID}, tasks, decisions)
	sort.SliceStable(filtered, func(i, j int) bool {
		return filtered[i].SequenceNumber > filtered[j].SequenceNumber
	})
	return paginateTestSlice(filtered, limit, 0), nil
}

func (r *memoryRepository) CreateEvidenceRefWithEvent(ctx context.Context, req CreateEvidenceRefWithEventRequest) (ProjectEvidenceRefWriteResult, error) {
	event, err := r.AppendProjectEvent(ctx, req.Event)
	if err != nil {
		return ProjectEvidenceRefWriteResult{}, err
	}
	evidenceReq := req.Evidence
	evidenceReq.CreatedEventID = &event.ID
	evidence, err := r.CreateEvidenceRef(ctx, evidenceReq)
	if err != nil {
		return ProjectEvidenceRefWriteResult{}, err
	}
	return ProjectEvidenceRefWriteResult{Event: event, Evidence: evidence}, nil
}

func (r *memoryRepository) UpdateEvidenceVerificationStatusWithEvent(ctx context.Context, req UpdateEvidenceVerificationStatusWithEventRequest) (ProjectEvidenceRefWriteResult, error) {
	evidence, err := r.UpdateEvidenceVerificationStatus(ctx, req.Evidence)
	if err != nil {
		return ProjectEvidenceRefWriteResult{}, err
	}
	event, err := r.AppendProjectEvent(ctx, req.Event)
	if err != nil {
		return ProjectEvidenceRefWriteResult{}, err
	}
	return ProjectEvidenceRefWriteResult{Event: event, Evidence: evidence}, nil
}

func (r *memoryRepository) CreateAcceptanceRecordWithEvent(ctx context.Context, req CreateAcceptanceRecordWithEventRequest) (ProjectAcceptanceRecordWriteResult, error) {
	event, err := r.AppendProjectEvent(ctx, req.Event)
	if err != nil {
		return ProjectAcceptanceRecordWriteResult{}, err
	}
	acceptanceReq := req.Acceptance
	acceptanceReq.CreatedEventID = &event.ID
	acceptance, err := r.CreateAcceptanceRecord(ctx, acceptanceReq)
	if err != nil {
		return ProjectAcceptanceRecordWriteResult{}, err
	}
	return ProjectAcceptanceRecordWriteResult{Event: event, Acceptance: acceptance}, nil
}

func (r *memoryRepository) CreateArchiveSnapshotWithEvent(ctx context.Context, req CreateArchiveSnapshotWithEventRequest) (ProjectArchiveSnapshotWriteResult, error) {
	event, err := r.AppendProjectEvent(ctx, req.Event)
	if err != nil {
		return ProjectArchiveSnapshotWriteResult{}, err
	}
	snapshotReq := req.Snapshot
	snapshotReq.CreatedEventID = &event.ID
	snapshot, err := r.CreateArchiveSnapshot(ctx, snapshotReq)
	if err != nil {
		return ProjectArchiveSnapshotWriteResult{}, err
	}
	return ProjectArchiveSnapshotWriteResult{Event: event, Snapshot: snapshot}, nil
}

func (r *memoryRepository) CreateArchiveSnapshotWithEventAndArchiveProject(ctx context.Context, req CreateArchiveSnapshotWithEventRequest) (ProjectArchiveSnapshotWriteResult, error) {
	eventSnapshot := append([]ProjectEvent(nil), r.events...)
	eventTypesSnapshot := append([]ProjectEventType(nil), r.eventTypes...)
	archiveSnapshotsSnapshot := append([]ProjectArchiveSnapshot(nil), r.archiveSnapshots...)
	projectsSnapshot := cloneProjects(r.projects)
	result, err := r.CreateArchiveSnapshotWithEvent(ctx, req)
	if err != nil {
		return ProjectArchiveSnapshotWriteResult{}, err
	}
	if _, err := r.ArchiveProject(ctx, req.Snapshot.TenantID, req.Snapshot.ProjectID); err != nil {
		r.events = eventSnapshot
		r.eventTypes = eventTypesSnapshot
		r.archiveSnapshots = archiveSnapshotsSnapshot
		r.projects = projectsSnapshot
		return ProjectArchiveSnapshotWriteResult{}, err
	}
	return result, nil
}

type governanceMemoryRepository struct {
	*memoryRepository
	evidenceRefs         []ProjectEvidenceRef
	artifactRefs         []ProjectArtifactRef
	reportRefs           []ProjectReportRef
	budgetLedger         []ProjectBudgetLedgerEntry
	acceptanceRecords    []ProjectAcceptanceRecord
	archiveSnapshots     []ProjectArchiveSnapshot
	createEvidenceRefErr error
	createAcceptanceErr  error
	createArchiveSnapErr error
}

func newGovernanceMemoryRepository() *governanceMemoryRepository {
	return &governanceMemoryRepository{memoryRepository: newMemoryRepository()}
}

func (r *governanceMemoryRepository) CreateEvidenceRef(ctx context.Context, req CreateEvidenceRefRequest) (ProjectEvidenceRef, error) {
	if r.createEvidenceRefErr != nil {
		return ProjectEvidenceRef{}, r.createEvidenceRefErr
	}
	evidence := ProjectEvidenceRef{
		ID:                 uuid.New(),
		TenantID:           req.TenantID,
		ProjectID:          req.ProjectID,
		ProjectTaskID:      req.ProjectTaskID,
		RouteDecisionID:    req.RouteDecisionID,
		ExecutionSummaryID: req.ExecutionSummaryID,
		EvidenceType:       req.EvidenceType,
		Title:              req.Title,
		Summary:            strPtrOrNil(req.Summary),
		SourceType:         req.SourceType,
		SourceRef:          req.SourceRef,
		ArtifactRefID:      req.ArtifactRefID,
		SubmittedByType:    req.SubmittedByType,
		SubmittedByID:      req.SubmittedByID,
		VerificationStatus: req.VerificationStatus,
		Metadata:           req.Metadata,
		CreatedEventID:     req.CreatedEventID,
		CreatedAt:          time.Now().UTC(),
		UpdatedAt:          time.Now().UTC(),
	}
	r.evidenceRefs = append(r.evidenceRefs, evidence)
	return evidence, nil
}

func (r *governanceMemoryRepository) CreateEvidenceRefWithEvent(ctx context.Context, req CreateEvidenceRefWithEventRequest) (ProjectEvidenceRefWriteResult, error) {
	snapshot := r.governanceSnapshot()
	event, err := r.AppendProjectEvent(ctx, req.Event)
	if err != nil {
		return ProjectEvidenceRefWriteResult{}, err
	}
	evidenceReq := req.Evidence
	evidenceReq.CreatedEventID = &event.ID
	evidence, err := r.CreateEvidenceRef(ctx, evidenceReq)
	if err != nil {
		r.restoreGovernanceSnapshot(snapshot)
		return ProjectEvidenceRefWriteResult{}, err
	}
	return ProjectEvidenceRefWriteResult{Event: event, Evidence: evidence}, nil
}

func (r *governanceMemoryRepository) ListEvidenceRefs(ctx context.Context, tenantID, projectID uuid.UUID, status *EvidenceVerificationStatus, limit, offset int32) ([]ProjectEvidenceRef, error) {
	filtered := make([]ProjectEvidenceRef, 0, len(r.evidenceRefs))
	for _, evidence := range r.evidenceRefs {
		if evidence.TenantID == tenantID && evidence.ProjectID == projectID && (status == nil || evidence.VerificationStatus == *status) {
			filtered = append(filtered, evidence)
		}
	}
	return paginateSlice(filtered, limit, offset), nil
}

func (r *governanceMemoryRepository) UpdateEvidenceVerificationStatus(ctx context.Context, req UpdateEvidenceVerificationStatusRequest) (ProjectEvidenceRef, error) {
	for index, evidence := range r.evidenceRefs {
		if evidence.ID == req.ID && evidence.TenantID == req.TenantID && evidence.ProjectID == req.ProjectID {
			evidence.VerificationStatus = req.VerificationStatus
			if req.Metadata != nil {
				evidence.Metadata = req.Metadata
			}
			evidence.UpdatedAt = time.Now().UTC()
			r.evidenceRefs[index] = evidence
			return evidence, nil
		}
	}
	return ProjectEvidenceRef{}, ErrProjectNotFound
}

func (r *governanceMemoryRepository) UpdateEvidenceVerificationStatusWithEvent(ctx context.Context, req UpdateEvidenceVerificationStatusWithEventRequest) (ProjectEvidenceRefWriteResult, error) {
	snapshot := r.governanceSnapshot()
	evidence, err := r.UpdateEvidenceVerificationStatus(ctx, req.Evidence)
	if err != nil {
		return ProjectEvidenceRefWriteResult{}, err
	}
	event, err := r.AppendProjectEvent(ctx, req.Event)
	if err != nil {
		r.restoreGovernanceSnapshot(snapshot)
		return ProjectEvidenceRefWriteResult{}, err
	}
	return ProjectEvidenceRefWriteResult{Event: event, Evidence: evidence}, nil
}

func (r *governanceMemoryRepository) CreateArtifactRef(ctx context.Context, req CreateArtifactRefRequest) (ProjectArtifactRef, error) {
	artifact := ProjectArtifactRef{
		ID:              uuid.New(),
		TenantID:        req.TenantID,
		ProjectID:       req.ProjectID,
		ProjectTaskID:   req.ProjectTaskID,
		ArtifactID:      req.ArtifactID,
		ArtifactType:    req.ArtifactType,
		Title:           req.Title,
		ObjectRef:       req.ObjectRef,
		ContentType:     strPtrOrNil(req.ContentType),
		SizeBytes:       req.SizeBytes,
		Checksum:        strPtrOrNil(req.Checksum),
		RetentionStatus: req.RetentionStatus,
		RetentionHoldID: req.RetentionHoldID,
		Metadata:        req.Metadata,
		CreatedEventID:  req.CreatedEventID,
		CreatedAt:       time.Now().UTC(),
		UpdatedAt:       time.Now().UTC(),
	}
	r.artifactRefs = append(r.artifactRefs, artifact)
	return artifact, nil
}

func (r *governanceMemoryRepository) ListArtifactRefs(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]ProjectArtifactRef, error) {
	filtered := make([]ProjectArtifactRef, 0, len(r.artifactRefs))
	for _, artifact := range r.artifactRefs {
		if artifact.TenantID == tenantID && artifact.ProjectID == projectID {
			filtered = append(filtered, artifact)
		}
	}
	return paginateSlice(filtered, limit, offset), nil
}

func (r *governanceMemoryRepository) UpdateArtifactRetention(ctx context.Context, req UpdateArtifactRetentionRequest) (ProjectArtifactRef, error) {
	for index, artifact := range r.artifactRefs {
		if artifact.ID == req.ID && artifact.TenantID == req.TenantID && artifact.ProjectID == req.ProjectID {
			artifact.RetentionStatus = req.RetentionStatus
			artifact.RetentionHoldID = req.RetentionHoldID
			artifact.UpdatedAt = time.Now().UTC()
			r.artifactRefs[index] = artifact
			return artifact, nil
		}
	}
	return ProjectArtifactRef{}, ErrProjectNotFound
}

func (r *governanceMemoryRepository) CreateReportRef(ctx context.Context, req CreateReportRefRequest) (ProjectReportRef, error) {
	report := ProjectReportRef{
		ID:              uuid.New(),
		TenantID:        req.TenantID,
		ProjectID:       req.ProjectID,
		ReportType:      req.ReportType,
		Title:           req.Title,
		Summary:         strPtrOrNil(req.Summary),
		ObjectRef:       req.ObjectRef,
		Format:          req.Format,
		GeneratedByType: req.GeneratedByType,
		GeneratedByID:   req.GeneratedByID,
		CreatedEventID:  req.CreatedEventID,
		CreatedAt:       time.Now().UTC(),
	}
	r.reportRefs = append(r.reportRefs, report)
	return report, nil
}

func (r *governanceMemoryRepository) ListReportRefs(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]ProjectReportRef, error) {
	filtered := make([]ProjectReportRef, 0, len(r.reportRefs))
	for _, report := range r.reportRefs {
		if report.TenantID == tenantID && report.ProjectID == projectID {
			filtered = append(filtered, report)
		}
	}
	return paginateSlice(filtered, limit, offset), nil
}

func (r *governanceMemoryRepository) CreateBudgetLedgerEntry(ctx context.Context, req CreateBudgetLedgerEntryRequest) (ProjectBudgetLedgerEntry, error) {
	entry := ProjectBudgetLedgerEntry{
		ID:                uuid.New(),
		TenantID:          req.TenantID,
		ProjectID:         req.ProjectID,
		CoordinationJobID: req.CoordinationJobID,
		ProjectTaskID:     req.ProjectTaskID,
		DigitalEmployeeID: req.DigitalEmployeeID,
		CostType:          req.CostType,
		EstimatedTokens:   req.EstimatedTokens,
		ActualTokens:      req.ActualTokens,
		EstimatedCost:     req.EstimatedCost,
		ActualCost:        req.ActualCost,
		Source:            req.Source,
		Reason:            strPtrOrNil(req.Reason),
		CreatedEventID:    req.CreatedEventID,
		CreatedAt:         time.Now().UTC(),
	}
	r.budgetLedger = append(r.budgetLedger, entry)
	return entry, nil
}

func (r *governanceMemoryRepository) ListBudgetLedger(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]ProjectBudgetLedgerEntry, error) {
	filtered := make([]ProjectBudgetLedgerEntry, 0, len(r.budgetLedger))
	for _, entry := range r.budgetLedger {
		if entry.TenantID == tenantID && entry.ProjectID == projectID {
			filtered = append(filtered, entry)
		}
	}
	return paginateSlice(filtered, limit, offset), nil
}

func (r *governanceMemoryRepository) GetBudgetSummary(ctx context.Context, tenantID, projectID uuid.UUID) (ProjectBudgetSummary, error) {
	var summary ProjectBudgetSummary
	for _, entry := range r.budgetLedger {
		if entry.TenantID != tenantID || entry.ProjectID != projectID {
			continue
		}
		summary.LedgerCount++
		if entry.EstimatedTokens != nil {
			summary.EstimatedTokens += *entry.EstimatedTokens
		}
		if entry.ActualTokens != nil {
			summary.ActualTokens += *entry.ActualTokens
		}
		if entry.EstimatedCost != "" {
			summary.EstimatedCost = entry.EstimatedCost
		}
		if entry.ActualCost != "" {
			summary.ActualCost = entry.ActualCost
		}
	}
	return summary, nil
}

func (r *governanceMemoryRepository) CreateAcceptanceRecord(ctx context.Context, req CreateAcceptanceRecordRequest) (ProjectAcceptanceRecord, error) {
	if r.createAcceptanceErr != nil {
		return ProjectAcceptanceRecord{}, r.createAcceptanceErr
	}
	record := ProjectAcceptanceRecord{
		ID:               uuid.New(),
		TenantID:         req.TenantID,
		ProjectID:        req.ProjectID,
		AcceptedByUserID: req.AcceptedByUserID,
		Status:           req.Status,
		Conclusion:       req.Conclusion,
		Summary:          strPtrOrNil(req.Summary),
		EvidenceRefIDs:   req.EvidenceRefIDs,
		ReportRefIDs:     req.ReportRefIDs,
		UnresolvedRisks:  req.UnresolvedRisks,
		CreatedEventID:   req.CreatedEventID,
		CreatedAt:        time.Now().UTC(),
	}
	r.acceptanceRecords = append(r.acceptanceRecords, record)
	return record, nil
}

func (r *governanceMemoryRepository) CreateAcceptanceRecordWithEvent(ctx context.Context, req CreateAcceptanceRecordWithEventRequest) (ProjectAcceptanceRecordWriteResult, error) {
	snapshot := r.governanceSnapshot()
	event, err := r.AppendProjectEvent(ctx, req.Event)
	if err != nil {
		return ProjectAcceptanceRecordWriteResult{}, err
	}
	acceptanceReq := req.Acceptance
	acceptanceReq.CreatedEventID = &event.ID
	acceptance, err := r.CreateAcceptanceRecord(ctx, acceptanceReq)
	if err != nil {
		r.restoreGovernanceSnapshot(snapshot)
		return ProjectAcceptanceRecordWriteResult{}, err
	}
	return ProjectAcceptanceRecordWriteResult{Event: event, Acceptance: acceptance}, nil
}

func (r *governanceMemoryRepository) GetLatestAcceptanceRecord(ctx context.Context, tenantID, projectID uuid.UUID) (ProjectAcceptanceRecord, error) {
	for index := len(r.acceptanceRecords) - 1; index >= 0; index-- {
		record := r.acceptanceRecords[index]
		if record.TenantID == tenantID && record.ProjectID == projectID {
			return record, nil
		}
	}
	return ProjectAcceptanceRecord{}, ErrProjectNotFound
}

func (r *governanceMemoryRepository) CreateArchiveSnapshot(ctx context.Context, req CreateArchiveSnapshotRequest) (ProjectArchiveSnapshot, error) {
	if r.createArchiveSnapErr != nil {
		return ProjectArchiveSnapshot{}, r.createArchiveSnapErr
	}
	snapshot := ProjectArchiveSnapshot{
		ID:                   uuid.New(),
		TenantID:             req.TenantID,
		ProjectID:            req.ProjectID,
		SnapshotType:         req.SnapshotType,
		Status:               req.Status,
		ObjectRef:            strPtrOrNil(req.ObjectRef),
		Summary:              strPtrOrNil(req.Summary),
		IncludedCounts:       req.IncludedCounts,
		RetainedArtifactIDs:  req.RetainedArtifactIDs,
		RetentionLockEventID: req.RetentionLockEventID,
		CreatedByUserID:      req.CreatedByUserID,
		CreatedEventID:       req.CreatedEventID,
		CreatedAt:            time.Now().UTC(),
	}
	r.archiveSnapshots = append(r.archiveSnapshots, snapshot)
	return snapshot, nil
}

func (r *governanceMemoryRepository) CreateArchiveSnapshotWithEvent(ctx context.Context, req CreateArchiveSnapshotWithEventRequest) (ProjectArchiveSnapshotWriteResult, error) {
	snapshot := r.governanceSnapshot()
	event, err := r.AppendProjectEvent(ctx, req.Event)
	if err != nil {
		return ProjectArchiveSnapshotWriteResult{}, err
	}
	snapshotReq := req.Snapshot
	snapshotReq.CreatedEventID = &event.ID
	archiveSnapshot, err := r.CreateArchiveSnapshot(ctx, snapshotReq)
	if err != nil {
		r.restoreGovernanceSnapshot(snapshot)
		return ProjectArchiveSnapshotWriteResult{}, err
	}
	return ProjectArchiveSnapshotWriteResult{Event: event, Snapshot: archiveSnapshot}, nil
}

func (r *governanceMemoryRepository) CreateArchiveSnapshotWithEventAndArchiveProject(ctx context.Context, req CreateArchiveSnapshotWithEventRequest) (ProjectArchiveSnapshotWriteResult, error) {
	snapshot := r.governanceSnapshot()
	result, err := r.CreateArchiveSnapshotWithEvent(ctx, req)
	if err != nil {
		return ProjectArchiveSnapshotWriteResult{}, err
	}
	if _, err := r.ArchiveProject(ctx, req.Snapshot.TenantID, req.Snapshot.ProjectID); err != nil {
		r.restoreGovernanceSnapshot(snapshot)
		return ProjectArchiveSnapshotWriteResult{}, err
	}
	return result, nil
}

func (r *governanceMemoryRepository) ListArchiveSnapshots(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]ProjectArchiveSnapshot, error) {
	filtered := make([]ProjectArchiveSnapshot, 0, len(r.archiveSnapshots))
	for _, snapshot := range r.archiveSnapshots {
		if snapshot.TenantID == tenantID && snapshot.ProjectID == projectID {
			filtered = append(filtered, snapshot)
		}
	}
	return paginateSlice(filtered, limit, offset), nil
}

func (r *governanceMemoryRepository) ListConfigRevisions(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]ProjectConfigRevision, error) {
	filtered := make([]ProjectConfigRevision, 0, len(r.revisions))
	for _, revision := range r.revisions {
		if revision.TenantID == tenantID && revision.ProjectID == projectID {
			filtered = append(filtered, revision)
		}
	}
	return paginateSlice(filtered, limit, offset), nil
}

func (r *governanceMemoryRepository) GetConfigRevision(ctx context.Context, tenantID, projectID, revisionID uuid.UUID) (ProjectConfigRevision, error) {
	for _, revision := range r.revisions {
		if revision.ID == revisionID && revision.TenantID == tenantID && revision.ProjectID == projectID {
			return revision, nil
		}
	}
	return ProjectConfigRevision{}, ErrProjectNotFound
}

type governanceMemorySnapshot struct {
	projects          map[uuid.UUID]Project
	events            []ProjectEvent
	eventTypes        []ProjectEventType
	evidenceRefs      []ProjectEvidenceRef
	acceptanceRecords []ProjectAcceptanceRecord
	archiveSnapshots  []ProjectArchiveSnapshot
}

func (r *governanceMemoryRepository) governanceSnapshot() governanceMemorySnapshot {
	return governanceMemorySnapshot{
		projects:          cloneProjects(r.projects),
		events:            append([]ProjectEvent(nil), r.events...),
		eventTypes:        append([]ProjectEventType(nil), r.eventTypes...),
		evidenceRefs:      append([]ProjectEvidenceRef(nil), r.evidenceRefs...),
		acceptanceRecords: append([]ProjectAcceptanceRecord(nil), r.acceptanceRecords...),
		archiveSnapshots:  append([]ProjectArchiveSnapshot(nil), r.archiveSnapshots...),
	}
}

func (r *governanceMemoryRepository) restoreGovernanceSnapshot(snapshot governanceMemorySnapshot) {
	r.projects = snapshot.projects
	r.events = snapshot.events
	r.eventTypes = snapshot.eventTypes
	r.evidenceRefs = snapshot.evidenceRefs
	r.acceptanceRecords = snapshot.acceptanceRecords
	r.archiveSnapshots = snapshot.archiveSnapshots
}

type fakeArchiveArtifactLocker struct {
	artifactIDs []uuid.UUID
	holdIDs     []uuid.UUID
	eventID     *uuid.UUID
	err         error
}

func (l *fakeArchiveArtifactLocker) LockProjectArtifacts(ctx context.Context, tenantID, projectID uuid.UUID, artifactIDs []uuid.UUID) (ArchiveArtifactLockResult, error) {
	l.artifactIDs = append([]uuid.UUID(nil), artifactIDs...)
	if len(l.holdIDs) == 0 {
		l.holdIDs = make([]uuid.UUID, 0, len(artifactIDs))
		for range artifactIDs {
			l.holdIDs = append(l.holdIDs, uuid.New())
		}
	}
	return ArchiveArtifactLockResult{
		HoldIDs:     append([]uuid.UUID(nil), l.holdIDs...),
		ArtifactIDs: append([]uuid.UUID(nil), artifactIDs...),
		EventID:     l.eventID,
	}, l.err
}

type fakeCoordinatorSignalClient struct {
	ensureSignals      int
	demandSignals      int
	policySignals      int
	memberSignals      int
	completedSignals   int
	failedSignals      int
	transferSignals    int
	decisionSignals    int
	lastDemand         DemandSubmittedSignal
	lastCompleted      EmployeeTaskCompletedSignal
	lastDecision       HumanDecisionSubmittedSignal
	demandSignalErr    error
	policySignalErr    error
	completedSignalErr error
}

func (f *fakeCoordinatorSignalClient) EnsureProjectCoordinator(ctx context.Context, signal ProjectCoordinatorSignal) error {
	f.ensureSignals++
	return nil
}

func (f *fakeCoordinatorSignalClient) SignalDemandSubmitted(ctx context.Context, signal DemandSubmittedSignal) error {
	f.demandSignals++
	f.lastDemand = signal
	return f.demandSignalErr
}

func (f *fakeCoordinatorSignalClient) SignalProjectPolicyChanged(ctx context.Context, signal ProjectPolicyChangedSignal) error {
	f.policySignals++
	return f.policySignalErr
}

func (f *fakeCoordinatorSignalClient) SignalProjectMemberChanged(ctx context.Context, signal ProjectMemberChangedSignal) error {
	f.memberSignals++
	return nil
}

func (f *fakeCoordinatorSignalClient) SignalEmployeeTaskCompleted(ctx context.Context, signal EmployeeTaskCompletedSignal) error {
	f.completedSignals++
	f.lastCompleted = signal
	return f.completedSignalErr
}

func (f *fakeCoordinatorSignalClient) SignalEmployeeTaskFailed(ctx context.Context, signal EmployeeTaskFailedSignal) error {
	f.failedSignals++
	return nil
}

func (f *fakeCoordinatorSignalClient) SignalEmployeeTransferRequested(ctx context.Context, signal EmployeeTransferRequestedSignal) error {
	f.transferSignals++
	return nil
}

func (f *fakeCoordinatorSignalClient) SignalHumanDecisionSubmitted(ctx context.Context, signal HumanDecisionSubmittedSignal) error {
	f.decisionSignals++
	f.lastDecision = signal
	return nil
}

type fakeApprovalResolver struct {
	calls int
	last  ResolveApprovalRequest
}

func (f *fakeApprovalResolver) ResolveApproval(ctx context.Context, req ResolveApprovalRequest) error {
	f.calls++
	f.last = req
	return nil
}

type fakeDecisionInboxProjector struct {
	upserts     []DecisionRequest
	resolutions []DecisionRequest
	upsertErr   error
	resolveErr  error
}

func (f *fakeDecisionInboxProjector) UpsertProjectDecisionRequest(ctx context.Context, decision DecisionRequest) error {
	f.upserts = append(f.upserts, decision)
	return f.upsertErr
}

func (f *fakeDecisionInboxProjector) ResolveProjectDecisionRequest(ctx context.Context, decision DecisionRequest) error {
	f.resolutions = append(f.resolutions, decision)
	return f.resolveErr
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func paginateSlice[T any](values []T, limit, offset int32) []T {
	start := int(offset)
	if start > len(values) {
		return []T{}
	}
	end := start + int(limit)
	if end > len(values) {
		end = len(values)
	}
	return values[start:end]
}

func countProjectEvents(values []ProjectEventType, target ProjectEventType) int {
	count := 0
	for _, value := range values {
		if value == target {
			count++
		}
	}
	return count
}

func projectEventTypes(events []ProjectEvent) []ProjectEventType {
	values := make([]ProjectEventType, 0, len(events))
	for _, event := range events {
		values = append(values, event.EventType)
	}
	return values
}
