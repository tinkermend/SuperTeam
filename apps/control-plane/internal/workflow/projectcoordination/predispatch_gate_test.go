package projectcoordination

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/superteam/control-plane/internal/approval"
	"github.com/superteam/control-plane/internal/project"
)

func TestProjectStoreRunPreDispatchGatePersistsPassedResult(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	taskID := uuid.New()
	employeeID := uuid.New()
	demandID := uuid.New()
	revisionID := uuid.New()
	taskKey := "collect-context"
	fixedNow := time.Date(2026, 6, 21, 11, 0, 0, 0, time.UTC)
	repo := &preDispatchGateRepositoryFake{
		projectRecord: project.Project{ID: projectID, TenantID: tenantID, HumanOwnerUserID: uuid.New()},
		task: project.ProjectTask{
			ID:                        taskID,
			TenantID:                  tenantID,
			ProjectID:                 projectID,
			DemandID:                  &demandID,
			Title:                     "Collect context",
			Status:                    project.ProjectTaskStatusPlanned,
			AssignedDigitalEmployeeID: &employeeID,
			AcceptedPlanRevisionID:    &revisionID,
			PlannedTaskKey:            &taskKey,
			AttemptCount:              0,
		},
		members: []project.ProjectMember{{
			ID:            uuid.New(),
			TenantID:      tenantID,
			ProjectID:     projectID,
			PrincipalType: project.PrincipalTypeDigitalEmployee,
			PrincipalID:   employeeID,
			ProjectRole:   project.ProjectRoleExecutor,
			Status:        "active",
		}},
	}
	store := NewProjectStoreWithApprovalsInboxAndRunStarter(repo, nil, nil, &projectTaskRunStarterFake{}).
		WithClock(func() time.Time { return fixedNow })

	decision, err := store.RunPreDispatchGate(context.Background(), DispatchProjectTaskInput{
		TenantID:  tenantID,
		ProjectID: projectID,
		TaskID:    taskID,
	})

	require.NoError(t, err)
	require.Equal(t, project.PreDispatchGateStatusPassed, decision.Gate.Status)
	require.True(t, decision.AllowRunStart)
	require.False(t, decision.Retryable)
	require.False(t, decision.Terminal)
	require.Empty(t, repo.decisionRequests)
	require.Len(t, repo.gates, 1)
	require.Equal(t, fixedNow, repo.gates[0].CheckedAt)
	require.Equal(t, project.ProjectEventTaskDispatchGateChecked, repo.events[0].EventType)
}

func TestProjectStoreRunPreDispatchGateCreatesHumanRequestOnce(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	taskID := uuid.New()
	employeeID := uuid.New()
	ownerID := uuid.New()
	demandID := uuid.New()
	fixedNow := time.Date(2026, 6, 21, 11, 0, 0, 0, time.UTC)
	repo := &preDispatchGateRepositoryFake{
		projectRecord: project.Project{ID: projectID, TenantID: tenantID, HumanOwnerUserID: ownerID},
		task: project.ProjectTask{
			ID:                        taskID,
			TenantID:                  tenantID,
			ProjectID:                 projectID,
			DemandID:                  &demandID,
			Title:                     "Approve risky work",
			Status:                    project.ProjectTaskStatusPlanned,
			AssignedDigitalEmployeeID: &employeeID,
			RequiresHumanApproval:     true,
			AttemptCount:              0,
		},
		members: []project.ProjectMember{{
			ID:            uuid.New(),
			TenantID:      tenantID,
			ProjectID:     projectID,
			PrincipalType: project.PrincipalTypeDigitalEmployee,
			PrincipalID:   employeeID,
			ProjectRole:   project.ProjectRoleExecutor,
			Status:        "active",
		}},
	}
	approvals := &preDispatchGateApprovalRecorder{approvalID: uuid.New()}
	inbox := &preDispatchGateInboxRecorder{}
	store := NewProjectStoreWithApprovalsInboxAndRunStarter(repo, approvals, inbox, &projectTaskRunStarterFake{}).
		WithClock(func() time.Time { return fixedNow })
	input := DispatchProjectTaskInput{TenantID: tenantID, ProjectID: projectID, TaskID: taskID}

	first, err := store.RunPreDispatchGate(context.Background(), input)
	require.NoError(t, err)
	second, err := store.RunPreDispatchGate(context.Background(), input)
	require.NoError(t, err)

	require.Equal(t, project.PreDispatchGateStatusWaitingHuman, first.Gate.Status)
	require.Equal(t, project.PreDispatchGateStatusWaitingHuman, second.Gate.Status)
	require.False(t, first.AllowRunStart)
	require.False(t, second.AllowRunStart)
	require.Len(t, approvals.requests, 1)
	require.Len(t, repo.decisionRequests, 1)
	require.Len(t, inbox.upserts, 1)
	require.NotNil(t, repo.task.WaitingRequestID)
	require.Equal(t, repo.decisionRequests[0].ID, *repo.task.WaitingRequestID)
	require.NotNil(t, second.Gate.DecisionRequestID)
	require.Equal(t, repo.decisionRequests[0].ID, *second.Gate.DecisionRequestID)
	require.Len(t, repo.gates, 1)
	require.Equal(t, "task.requires_human_approval", first.Gate.HumanActionRequest["risk_reason"])
}

func TestProjectStoreRunPreDispatchGateDoesNotCreateRunOnRetryLater(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	taskID := uuid.New()
	employeeID := uuid.New()
	demandID := uuid.New()
	retryAfter := time.Date(2026, 6, 21, 11, 15, 0, 0, time.UTC)
	repo := &preDispatchGateRepositoryFake{
		projectRecord: project.Project{ID: projectID, TenantID: tenantID, HumanOwnerUserID: uuid.New()},
		task: project.ProjectTask{
			ID:                        taskID,
			TenantID:                  tenantID,
			ProjectID:                 projectID,
			DemandID:                  &demandID,
			Title:                     "Wait for runtime slot",
			Status:                    project.ProjectTaskStatusPlanned,
			AssignedDigitalEmployeeID: &employeeID,
			AttemptCount:              0,
		},
		members: []project.ProjectMember{{
			ID:            uuid.New(),
			TenantID:      tenantID,
			ProjectID:     projectID,
			PrincipalType: project.PrincipalTypeDigitalEmployee,
			PrincipalID:   employeeID,
			ProjectRole:   project.ProjectRoleExecutor,
			Status:        "active",
		}},
	}
	reader := &preDispatchGateEmployeeRuntimeReader{
		employee: project.PreDispatchEmployeeSnapshot{
			ID:                 employeeID,
			IsProjectExecutor:  true,
			Status:             "active",
			PolicyAllowed:      true,
			RequiredLoadSlots:  1,
			AvailableLoadSlots: 1,
		},
		runtime: project.PreDispatchRuntimeSnapshot{
			NodeOnline:              true,
			ProviderAvailable:       true,
			WorkspaceReady:          true,
			SlotAvailable:           false,
			ContractVersionAccepted: true,
			RetryAfter:              retryAfter,
		},
	}
	runStarter := &projectTaskRunStarterFake{}
	store := NewProjectStoreWithApprovalsInboxAndRunStarter(repo, nil, nil, runStarter).
		WithPreDispatchGateReaders(reader, nil)

	decision, err := store.RunPreDispatchGate(context.Background(), DispatchProjectTaskInput{
		TenantID:  tenantID,
		ProjectID: projectID,
		TaskID:    taskID,
	})

	require.NoError(t, err)
	require.Equal(t, project.PreDispatchGateStatusRetryLater, decision.Gate.Status)
	require.False(t, decision.AllowRunStart)
	require.True(t, decision.Retryable)
	require.False(t, decision.Terminal)
	require.NotNil(t, decision.Gate.RetryAfter)
	require.Equal(t, retryAfter, *decision.Gate.RetryAfter)
	require.Empty(t, repo.decisionRequests)
	require.Empty(t, runStarter.requests)
	require.Len(t, repo.gates, 1)
	require.Equal(t, project.ProjectEventTaskDispatchGateRetryLater, repo.events[0].EventType)
}

type preDispatchGateRepositoryFake struct {
	project.Repository

	projectRecord    project.Project
	task             project.ProjectTask
	currentAttempt   *project.ProjectTaskAttempt
	dependencies     []project.ProjectTaskDependency
	dependencyTasks  map[uuid.UUID]project.ProjectTask
	members          []project.ProjectMember
	events           []project.ProjectEvent
	gates            []project.PreDispatchGateResult
	decisionRequests []project.DecisionRequest
}

func (r *preDispatchGateRepositoryFake) GetProject(ctx context.Context, tenantID, projectID uuid.UUID) (project.Project, error) {
	if r.projectRecord.TenantID == tenantID && r.projectRecord.ID == projectID {
		return r.projectRecord, nil
	}
	return project.Project{}, project.ErrProjectNotFound
}

func (r *preDispatchGateRepositoryFake) GetProjectTask(ctx context.Context, tenantID, projectTaskID uuid.UUID) (project.ProjectTask, error) {
	if r.task.TenantID == tenantID && r.task.ID == projectTaskID {
		return r.task, nil
	}
	if r.dependencyTasks != nil {
		if task, ok := r.dependencyTasks[projectTaskID]; ok && task.TenantID == tenantID {
			return task, nil
		}
	}
	return project.ProjectTask{}, project.ErrProjectNotFound
}

func (r *preDispatchGateRepositoryFake) GetCurrentProjectTaskAttempt(ctx context.Context, tenantID, projectTaskID uuid.UUID) (project.ProjectTaskAttempt, error) {
	if r.currentAttempt == nil || r.currentAttempt.TenantID != tenantID || r.currentAttempt.ProjectTaskID != projectTaskID {
		return project.ProjectTaskAttempt{}, project.ErrProjectNotFound
	}
	return *r.currentAttempt, nil
}

func (r *preDispatchGateRepositoryFake) ListProjectTaskDependencies(ctx context.Context, tenantID, projectID uuid.UUID, dependentTaskIDs []uuid.UUID) ([]project.ProjectTaskDependency, error) {
	requested := map[uuid.UUID]struct{}{}
	for _, taskID := range dependentTaskIDs {
		requested[taskID] = struct{}{}
	}
	result := make([]project.ProjectTaskDependency, 0)
	for _, dependency := range r.dependencies {
		if dependency.TenantID != tenantID || dependency.ProjectID != projectID {
			continue
		}
		if _, ok := requested[dependency.DependentTaskID]; ok {
			result = append(result, dependency)
		}
	}
	return result, nil
}

func (r *preDispatchGateRepositoryFake) ListProjectMembers(ctx context.Context, tenantID, projectID uuid.UUID) ([]project.ProjectMember, error) {
	result := make([]project.ProjectMember, 0)
	for _, member := range r.members {
		if member.TenantID == tenantID && member.ProjectID == projectID {
			result = append(result, member)
		}
	}
	return result, nil
}

func (r *preDispatchGateRepositoryFake) AppendProjectEvent(ctx context.Context, req project.AppendProjectEventRequest) (project.ProjectEvent, error) {
	event := project.ProjectEvent{
		ID:        uuid.New(),
		TenantID:  req.TenantID,
		ProjectID: req.ProjectID,
		EventType: req.EventType,
		ActorType: req.ActorType,
		ActorID:   req.ActorID,
		Summary:   &req.Summary,
		Payload:   req.Payload,
		CreatedAt: time.Now().UTC(),
	}
	r.events = append(r.events, event)
	return event, nil
}

func (r *preDispatchGateRepositoryFake) RecordPreDispatchGateResult(ctx context.Context, req project.RecordPreDispatchGateResultRequest) (project.PreDispatchGateResult, error) {
	if r.task.TenantID != req.TenantID || r.task.ProjectID != req.ProjectID || r.task.ID != req.ProjectTaskID {
		return project.PreDispatchGateResult{}, project.ErrProjectNotFound
	}
	for index, gate := range r.gates {
		if gate.TenantID == req.TenantID && gate.ProjectID == req.ProjectID && gate.ProjectTaskID == req.ProjectTaskID && gate.IdempotencyKey == req.IdempotencyKey {
			if gate.DecisionRequestID != nil || gate.AttemptID != nil {
				return gate, nil
			}
			gate.Status = req.Status
			gate.CheckedAt = req.CheckedAt
			gate.Checks = append([]project.PreDispatchGateCheck(nil), req.Checks...)
			gate.Blockers = append([]project.PreDispatchGateBlocker(nil), req.Blockers...)
			gate.HumanActionRequest = cloneHumanAction(req.HumanActionRequest)
			gate.RetryAfter = req.RetryAfter
			if gate.CreatedEventID == nil {
				gate.CreatedEventID = req.CreatedEventID
			}
			r.gates[index] = gate
			r.task.LatestDispatchGateResultID = &gate.ID
			return gate, nil
		}
	}
	gate := project.PreDispatchGateResult{
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
		CheckedAt:              req.CheckedAt,
		Checks:                 append([]project.PreDispatchGateCheck(nil), req.Checks...),
		Blockers:               append([]project.PreDispatchGateBlocker(nil), req.Blockers...),
		HumanActionRequest:     cloneHumanAction(req.HumanActionRequest),
		RetryAfter:             req.RetryAfter,
		CreatedEventID:         req.CreatedEventID,
		CreatedAt:              time.Now().UTC(),
		UpdatedAt:              time.Now().UTC(),
	}
	r.gates = append(r.gates, gate)
	r.task.LatestDispatchGateResultID = &gate.ID
	return gate, nil
}

func (r *preDispatchGateRepositoryFake) LinkPreDispatchGateDecisionRequest(ctx context.Context, req project.LinkPreDispatchGateDecisionRequest) (project.PreDispatchGateResult, error) {
	for gateIndex, gate := range r.gates {
		if gate.TenantID != req.TenantID || gate.ProjectID != req.ProjectID || gate.ProjectTaskID != req.ProjectTaskID || gate.ID != req.GateResultID {
			continue
		}
		for decisionIndex, decision := range r.decisionRequests {
			if decision.ID != req.DecisionRequestID || decision.TenantID != req.TenantID || decision.ProjectID != req.ProjectID {
				continue
			}
			if decision.ProjectTaskID == nil || *decision.ProjectTaskID != req.ProjectTaskID {
				return project.PreDispatchGateResult{}, project.ErrProjectNotFound
			}
			gate.DecisionRequestID = &req.DecisionRequestID
			decision.DispatchGateResultID = &req.GateResultID
			r.gates[gateIndex] = gate
			r.decisionRequests[decisionIndex] = decision
			return gate, nil
		}
	}
	return project.PreDispatchGateResult{}, project.ErrProjectNotFound
}

func (r *preDispatchGateRepositoryFake) MoveProjectTaskToWaitingHumanForPreDispatchGate(ctx context.Context, req project.MoveProjectTaskToWaitingHumanForPreDispatchGateRequest) (project.ProjectTask, error) {
	if r.task.TenantID != req.TenantID || r.task.ProjectID != req.ProjectID || r.task.ID != req.ProjectTaskID {
		return project.ProjectTask{}, project.ErrProjectNotFound
	}
	r.task.Status = project.ProjectTaskStatusWaitingHuman
	r.task.WaitingReason = stringPtr(req.WaitingReason)
	r.task.WaitingRequestID = &req.DecisionRequestID
	r.task.LatestDispatchGateResultID = &req.GateResultID
	return r.task, nil
}

func (r *preDispatchGateRepositoryFake) CreateDecisionRequest(ctx context.Context, req project.CreateDecisionRequestRequest) (project.DecisionRequest, error) {
	projectTaskID := req.ProjectTaskID
	decision := project.DecisionRequest{
		ID:                   uuid.New(),
		TenantID:             req.TenantID,
		ProjectID:            req.ProjectID,
		ApprovalRequestID:    req.ApprovalRequestID,
		CoordinationJobID:    req.CoordinationJobID,
		ProjectTaskID:        projectTaskID,
		TargetUserID:         req.TargetUserID,
		DecisionType:         req.DecisionType,
		TitleSnapshot:        req.TitleSnapshot,
		SummarySnapshot:      stringPtr(req.SummarySnapshot),
		RiskLevelSnapshot:    stringPtr(req.RiskLevelSnapshot),
		StatusSnapshot:       req.StatusSnapshot,
		CreatedEventID:       req.CreatedEventID,
		CreatedAt:            time.Now().UTC(),
		UpdatedAt:            time.Now().UTC(),
		DispatchGateResultID: nil,
	}
	r.decisionRequests = append(r.decisionRequests, decision)
	return decision, nil
}

type preDispatchGateApprovalRecorder struct {
	approvalID uuid.UUID
	requests   []approval.CreateRequestInput
}

func (r *preDispatchGateApprovalRecorder) CreateRequest(ctx context.Context, input approval.CreateRequestInput) (*approval.ApprovalRequest, error) {
	r.requests = append(r.requests, input)
	id := r.approvalID
	if id == uuid.Nil {
		id = uuid.New()
	}
	return &approval.ApprovalRequest{
		ID:             id,
		TenantID:       input.TenantID,
		ResourceType:   input.ResourceType,
		ResourceID:     input.ResourceID,
		RequesterType:  input.RequesterType,
		RequesterID:    input.RequesterID,
		TargetUserID:   input.TargetUserID,
		DecisionType:   input.DecisionType,
		Title:          input.Title,
		Summary:        stringPtr(input.Summary),
		RiskLevel:      stringPtr(input.RiskLevel),
		Options:        append([]any(nil), input.Options...),
		ContextPayload: input.ContextPayload,
		Status:         approval.ApprovalStatusPending,
	}, nil
}

type preDispatchGateInboxRecorder struct {
	upserts []project.DecisionRequest
}

func (r *preDispatchGateInboxRecorder) UpsertProjectDecisionRequest(ctx context.Context, decision project.DecisionRequest) error {
	r.upserts = append(r.upserts, decision)
	return nil
}

func (r *preDispatchGateInboxRecorder) ResolveProjectDecisionRequest(ctx context.Context, decision project.DecisionRequest) error {
	return nil
}

type preDispatchGateEmployeeRuntimeReader struct {
	employee project.PreDispatchEmployeeSnapshot
	runtime  project.PreDispatchRuntimeSnapshot
}

func (r *preDispatchGateEmployeeRuntimeReader) GetEmployeeRuntimeSnapshot(ctx context.Context, tenantID, employeeID uuid.UUID) (project.PreDispatchEmployeeSnapshot, project.PreDispatchRuntimeSnapshot, error) {
	return r.employee, r.runtime, nil
}

func cloneHumanAction(input project.HumanActionRequest) project.HumanActionRequest {
	if input == nil {
		return nil
	}
	clone := make(project.HumanActionRequest, len(input))
	for key, value := range input {
		clone[key] = value
	}
	return clone
}
