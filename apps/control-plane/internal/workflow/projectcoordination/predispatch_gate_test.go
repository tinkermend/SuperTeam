package projectcoordination

import (
	"context"
	"errors"
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
	require.Nil(t, repo.gates[0].CreatedEventID)
	require.NotEmpty(t, repo.operations)
	require.Equal(t, "record_gate", repo.operations[0])
	require.Equal(t, project.ProjectEventTaskDispatchGateChecked, repo.events[0].EventType)
}

func TestRunPreDispatchGateBlocksMissingProjectPlacement(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	taskID := uuid.New()
	employeeID := uuid.New()
	demandID := uuid.New()
	fixedNow := time.Date(2026, 6, 30, 9, 0, 0, 0, time.UTC)
	repo := &preDispatchGateRepositoryFake{
		missingActivePlacement: true,
		projectRecord:          project.Project{ID: projectID, TenantID: tenantID, HumanOwnerUserID: uuid.New()},
		task: project.ProjectTask{
			ID:                        taskID,
			TenantID:                  tenantID,
			ProjectID:                 projectID,
			DemandID:                  &demandID,
			Title:                     "Dispatch without placement",
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
	store := NewProjectStoreWithApprovalsInboxAndRunStarter(repo, nil, nil, &projectTaskRunStarterFake{}).
		WithClock(func() time.Time { return fixedNow })

	gate, err := store.RunPreDispatchGate(context.Background(), DispatchProjectTaskInput{
		TenantID:  tenantID,
		ProjectID: projectID,
		TaskID:    taskID,
	})

	require.NoError(t, err)
	require.False(t, gate.AllowRunStart)
	require.False(t, gate.Terminal)
	require.False(t, gate.Retryable)
	require.Contains(t, preDispatchBlockerKeys(gate.Gate.Blockers), "runtime.placement_missing")
	reasonCode, recommendedAction := dispatchBlockReasonFromGate(gate.Gate)
	require.Equal(t, "runtime_placement_missing", reasonCode)
	require.Equal(t, "bind_runtime", recommendedAction)
}

func TestProjectStoreRunPreDispatchGateRequiresAcceptedDependencyResult(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	taskID := uuid.New()
	dependencyID := uuid.New()
	employeeID := uuid.New()
	fixedNow := time.Date(2026, 6, 21, 11, 30, 0, 0, time.UTC)
	repo := &preDispatchGateRepositoryFake{
		projectRecord: project.Project{ID: projectID, TenantID: tenantID, HumanOwnerUserID: uuid.New()},
		task: project.ProjectTask{
			ID:                        taskID,
			TenantID:                  tenantID,
			ProjectID:                 projectID,
			Title:                     "Dispatch after dependency acceptance",
			Status:                    project.ProjectTaskStatusPlanned,
			AssignedDigitalEmployeeID: &employeeID,
			AttemptCount:              0,
		},
		dependencies: []project.ProjectTaskDependency{{
			ID:              uuid.New(),
			TenantID:        tenantID,
			ProjectID:       projectID,
			DependentTaskID: taskID,
			BlockerTaskID:   dependencyID,
		}},
		dependencyTasks: map[uuid.UUID]project.ProjectTask{
			dependencyID: {
				ID:        dependencyID,
				TenantID:  tenantID,
				ProjectID: projectID,
				Title:     "Dependency",
				Status:    project.ProjectTaskStatusCompleted,
				UpdatedAt: fixedNow,
			},
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
	input := DispatchProjectTaskInput{TenantID: tenantID, ProjectID: projectID, TaskID: taskID}

	missing, err := store.RunPreDispatchGate(context.Background(), input)
	require.NoError(t, err)
	require.Equal(t, project.PreDispatchGateStatusBlocked, missing.Gate.Status)
	require.False(t, missing.AllowRunStart)
	require.Len(t, missing.Gate.Blockers, 1)
	require.Equal(t, "dependency.not_ready", missing.Gate.Blockers[0].Key)

	repo.setDependencyLatestResult(dependencyID, preDispatchGateTaskResult(tenantID, projectID, dependencyID, project.TaskResultDecisionWaitingHumanReview, "accepted", fixedNow.Add(time.Minute)))
	waitingHuman, err := store.RunPreDispatchGate(context.Background(), input)
	require.NoError(t, err)
	require.Equal(t, project.PreDispatchGateStatusBlocked, waitingHuman.Gate.Status)
	require.False(t, waitingHuman.AllowRunStart)

	repo.setDependencyLatestResult(dependencyID, preDispatchGateTaskResult(tenantID, projectID, dependencyID, project.TaskResultDecisionCompleteAccepted, "accepted", fixedNow.Add(2*time.Minute)))
	accepted, err := store.RunPreDispatchGate(context.Background(), input)
	require.NoError(t, err)
	require.Equal(t, project.PreDispatchGateStatusPassed, accepted.Gate.Status)
	require.True(t, accepted.AllowRunStart)
}

func TestProjectStoreRunPreDispatchGateDoesNotDuplicateGateEventOnReplay(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	taskID := uuid.New()
	employeeID := uuid.New()
	fixedNow := time.Date(2026, 6, 21, 11, 0, 0, 0, time.UTC)
	repo := &preDispatchGateRepositoryFake{
		projectRecord: project.Project{ID: projectID, TenantID: tenantID, HumanOwnerUserID: uuid.New()},
		task: project.ProjectTask{
			ID:                        taskID,
			TenantID:                  tenantID,
			ProjectID:                 projectID,
			Title:                     "Replay gate",
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
	store := NewProjectStoreWithApprovalsInboxAndRunStarter(repo, nil, nil, &projectTaskRunStarterFake{}).
		WithClock(func() time.Time { return fixedNow })
	input := DispatchProjectTaskInput{TenantID: tenantID, ProjectID: projectID, TaskID: taskID}

	first, err := store.RunPreDispatchGate(context.Background(), input)
	require.NoError(t, err)
	second, err := store.RunPreDispatchGate(context.Background(), input)
	require.NoError(t, err)

	require.Equal(t, first.Gate.ID, second.Gate.ID)
	require.Len(t, repo.gates, 1)
	require.Len(t, repo.events, 1)
	require.Equal(t, project.ProjectEventTaskDispatchGateChecked, repo.events[0].EventType)
	require.Nil(t, repo.gates[0].CreatedEventID)
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
	require.Len(t, inbox.upserts, 2)
	require.NotNil(t, repo.task.WaitingRequestID)
	require.Equal(t, repo.decisionRequests[0].ID, *repo.task.WaitingRequestID)
	require.NotNil(t, second.Gate.DecisionRequestID)
	require.Equal(t, repo.decisionRequests[0].ID, *second.Gate.DecisionRequestID)
	require.Len(t, repo.gates, 1)
	require.Equal(t, "task.requires_human_approval", first.Gate.HumanActionRequest["risk_reason"])
}

func TestProjectStoreRunPreDispatchGatePassesAfterApprovalDecision(t *testing.T) {
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
	store := NewProjectStoreWithApprovalsInboxAndRunStarter(repo, approvals, nil, &projectTaskRunStarterFake{}).
		WithClock(func() time.Time { return fixedNow })
	input := DispatchProjectTaskInput{TenantID: tenantID, ProjectID: projectID, TaskID: taskID}

	first, err := store.RunPreDispatchGate(context.Background(), input)
	require.NoError(t, err)
	require.Equal(t, project.PreDispatchGateStatusWaitingHuman, first.Gate.Status)
	require.Len(t, repo.decisionRequests, 1)
	repo.decisionRequests[0].StatusSnapshot = "approved"
	resolvedAt := fixedNow.Add(time.Minute)
	repo.decisionRequests[0].ResolvedAt = &resolvedAt

	second, err := store.RunPreDispatchGate(context.Background(), DispatchProjectTaskInput{
		TenantID:       tenantID,
		ProjectID:      projectID,
		TaskID:         taskID,
		DispatchReason: project.DispatchReasonHumanResolved,
	})

	require.NoError(t, err)
	require.Equal(t, project.PreDispatchGateStatusPassed, second.Gate.Status)
	require.True(t, second.AllowRunStart)
	require.Len(t, repo.gates, 2)
	require.Equal(t, project.ProjectEventTaskDispatchGateChecked, repo.events[len(repo.events)-1].EventType)
}

func TestProjectStoreApplyPreDispatchGateDecisionIgnoresNonApprovalGateDecision(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	taskID := uuid.New()
	employeeID := uuid.New()
	gateID := uuid.New()
	taskIDCopy := taskID
	repo := &preDispatchGateRepositoryFake{
		projectRecord: project.Project{ID: projectID, TenantID: tenantID, HumanOwnerUserID: uuid.New()},
		task: project.ProjectTask{
			ID:                        taskID,
			TenantID:                  tenantID,
			ProjectID:                 projectID,
			Title:                     "Budget approval should not redispatch",
			Status:                    project.ProjectTaskStatusWaitingHuman,
			AssignedDigitalEmployeeID: &employeeID,
			AttemptCount:              0,
		},
		decisionRequests: []project.DecisionRequest{{
			ID:                   uuid.New(),
			TenantID:             tenantID,
			ProjectID:            projectID,
			ApprovalRequestID:    uuid.New(),
			ProjectTaskID:        &taskIDCopy,
			TargetUserID:         uuid.New(),
			DecisionType:         "project_task_budget_approval",
			TitleSnapshot:        "Budget approval",
			StatusSnapshot:       "approved",
			DispatchGateResultID: &gateID,
		}},
	}
	store := NewProjectStoreWithApprovalsInboxAndRunStarter(repo, nil, nil, &projectTaskRunStarterFake{})

	result, err := store.ApplyPreDispatchGateDecision(context.Background(), ApplyPreDispatchGateDecisionInput{
		TenantID:          tenantID,
		ProjectID:         projectID,
		DecisionRequestID: repo.decisionRequests[0].ID,
		Decision:          "approved",
	})

	require.NoError(t, err)
	require.Empty(t, result.ReadyTaskIDs)
}

func TestProjectStoreRunPreDispatchGateFailsClosedWhenApprovalsMissing(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	taskID := uuid.New()
	employeeID := uuid.New()
	repo := &preDispatchGateRepositoryFake{
		projectRecord: project.Project{ID: projectID, TenantID: tenantID, HumanOwnerUserID: uuid.New()},
		task: project.ProjectTask{
			ID:                        taskID,
			TenantID:                  tenantID,
			ProjectID:                 projectID,
			Title:                     "Needs approval",
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
	store := NewProjectStoreWithApprovalsInboxAndRunStarter(repo, nil, nil, &projectTaskRunStarterFake{})

	_, err := store.RunPreDispatchGate(context.Background(), DispatchProjectTaskInput{
		TenantID:  tenantID,
		ProjectID: projectID,
		TaskID:    taskID,
	})

	require.Error(t, err)
	require.Empty(t, repo.gates)
	require.Empty(t, repo.events)
	require.Empty(t, repo.decisionRequests)
	require.Nil(t, repo.task.LatestDispatchGateResultID)
	require.Nil(t, repo.task.WaitingReason)
	require.Nil(t, repo.task.WaitingRequestID)
}

func TestProjectStoreRunPreDispatchGateReusesApprovalAfterDecisionCreateFailure(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	taskID := uuid.New()
	employeeID := uuid.New()
	ownerID := uuid.New()
	approvalID := uuid.New()
	fixedNow := time.Date(2026, 6, 21, 11, 0, 0, 0, time.UTC)
	createDecisionErr := errors.New("create decision failed")
	repo := &preDispatchGateRepositoryFake{
		projectRecord:     project.Project{ID: projectID, TenantID: tenantID, HumanOwnerUserID: ownerID},
		createDecisionErr: createDecisionErr,
		task: project.ProjectTask{
			ID:                        taskID,
			TenantID:                  tenantID,
			ProjectID:                 projectID,
			Title:                     "Approve after partial failure",
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
	approvals := &preDispatchGateApprovalRecorder{approvalID: approvalID}
	store := NewProjectStoreWithApprovalsInboxAndRunStarter(repo, approvals, nil, &projectTaskRunStarterFake{}).
		WithClock(func() time.Time { return fixedNow })
	input := DispatchProjectTaskInput{TenantID: tenantID, ProjectID: projectID, TaskID: taskID}

	_, err := store.RunPreDispatchGate(context.Background(), input)
	require.ErrorIs(t, err, createDecisionErr)

	decision, err := store.RunPreDispatchGate(context.Background(), input)
	require.NoError(t, err)

	require.Equal(t, project.PreDispatchGateStatusWaitingHuman, decision.Gate.Status)
	require.Len(t, approvals.requests, 1)
	require.Len(t, approvals.records, 1)
	require.Len(t, repo.decisionRequests, 1)
	require.Equal(t, approvalID, repo.decisionRequests[0].ApprovalRequestID)
	require.NotNil(t, repo.task.WaitingRequestID)
	require.Equal(t, repo.decisionRequests[0].ID, *repo.task.WaitingRequestID)
}

func TestProjectStoreRunPreDispatchGateReusesDecisionAfterLinkFailure(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	taskID := uuid.New()
	employeeID := uuid.New()
	ownerID := uuid.New()
	approvalID := uuid.New()
	fixedNow := time.Date(2026, 6, 21, 11, 0, 0, 0, time.UTC)
	linkErr := errors.New("link gate decision failed")
	repo := &preDispatchGateRepositoryFake{
		projectRecord:   project.Project{ID: projectID, TenantID: tenantID, HumanOwnerUserID: ownerID},
		linkDecisionErr: linkErr,
		task: project.ProjectTask{
			ID:                        taskID,
			TenantID:                  tenantID,
			ProjectID:                 projectID,
			Title:                     "Link after partial failure",
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
	approvals := &preDispatchGateApprovalRecorder{approvalID: approvalID}
	inbox := &preDispatchGateInboxRecorder{}
	store := NewProjectStoreWithApprovalsInboxAndRunStarter(repo, approvals, inbox, &projectTaskRunStarterFake{}).
		WithClock(func() time.Time { return fixedNow })
	input := DispatchProjectTaskInput{TenantID: tenantID, ProjectID: projectID, TaskID: taskID}

	_, err := store.RunPreDispatchGate(context.Background(), input)
	require.ErrorIs(t, err, linkErr)
	require.Len(t, approvals.requests, 1)
	require.Len(t, repo.decisionRequests, 1)
	require.Nil(t, repo.decisionRequests[0].DispatchGateResultID)

	repo.listDecisionErr = errors.New("ListDecisionRequests should not be used for gate decision retry lookup")
	decision, err := store.RunPreDispatchGate(context.Background(), input)
	require.NoError(t, err)

	require.Equal(t, project.PreDispatchGateStatusWaitingHuman, decision.Gate.Status)
	require.Zero(t, repo.listDecisionCalls)
	require.GreaterOrEqual(t, repo.directDecisionCalls, 2)
	require.Len(t, approvals.requests, 1)
	require.Len(t, approvals.records, 1)
	require.Len(t, repo.decisionRequests, 1)
	require.NotNil(t, repo.decisionRequests[0].DispatchGateResultID)
	require.Equal(t, decision.Gate.ID, *repo.decisionRequests[0].DispatchGateResultID)
	require.NotNil(t, repo.task.WaitingRequestID)
	require.Equal(t, repo.decisionRequests[0].ID, *repo.task.WaitingRequestID)
	require.Len(t, inbox.upserts, 1)
	require.Equal(t, repo.decisionRequests[0].ID, inbox.upserts[0].ID)
}

func TestProjectStoreRunPreDispatchGateReusesExistingDecisionRequestedEventID(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	taskID := uuid.New()
	employeeID := uuid.New()
	ownerID := uuid.New()
	gateID := uuid.New()
	decisionID := uuid.New()
	approvalID := uuid.New()
	eventID := uuid.New()
	checkedAt := time.Date(2026, 6, 21, 11, 0, 0, 0, time.UTC)
	gateInput := project.PreDispatchGateInput{
		ProjectID:          projectID,
		ProjectTaskID:      taskID,
		SelectedEmployeeID: employeeID,
		AttemptNo:          1,
		DispatchReason:     project.DispatchReasonRootReady,
	}
	taskIDCopy := taskID
	repo := &preDispatchGateRepositoryFake{
		projectRecord: project.Project{ID: projectID, TenantID: tenantID, HumanOwnerUserID: ownerID},
		task: project.ProjectTask{
			ID:                        taskID,
			TenantID:                  tenantID,
			ProjectID:                 projectID,
			Title:                     "Recover event id",
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
		gates: []project.PreDispatchGateResult{{
			ID:                 gateID,
			TenantID:           tenantID,
			ProjectID:          projectID,
			ProjectTaskID:      taskID,
			SelectedEmployeeID: employeeID,
			AttemptNo:          1,
			DispatchReason:     project.DispatchReasonRootReady,
			IdempotencyKey:     project.PreDispatchGateIdempotencyKey(gateInput),
			DispatchToken:      project.PreDispatchGateDispatchToken(gateInput),
			Status:             project.PreDispatchGateStatusWaitingHuman,
			CheckedAt:          checkedAt,
			DecisionRequestID:  &decisionID,
			HumanActionRequest: project.HumanActionRequest{"waiting_reason": project.HumanWaitReasonApprovalRequired},
		}},
		decisionRequests: []project.DecisionRequest{{
			ID:                   decisionID,
			TenantID:             tenantID,
			ProjectID:            projectID,
			ApprovalRequestID:    approvalID,
			ProjectTaskID:        &taskIDCopy,
			TargetUserID:         ownerID,
			DecisionType:         "project_task_approval",
			TitleSnapshot:        "High risk action requires confirmation",
			StatusSnapshot:       "pending",
			DispatchGateResultID: &gateID,
		}},
		events: []project.ProjectEvent{{
			ID:        eventID,
			TenantID:  tenantID,
			ProjectID: projectID,
			EventType: project.ProjectEventDecisionRequested,
			ActorType: "project_coordinator",
			ActorID:   gateID.String(),
			CreatedAt: checkedAt,
		}},
	}
	store := NewProjectStoreWithApprovalsInboxAndRunStarter(repo, &preDispatchGateApprovalRecorder{}, nil, &projectTaskRunStarterFake{}).
		WithClock(func() time.Time { return checkedAt })

	decision, err := store.RunPreDispatchGate(context.Background(), DispatchProjectTaskInput{
		TenantID:  tenantID,
		ProjectID: projectID,
		TaskID:    taskID,
	})

	require.NoError(t, err)
	require.Equal(t, gateID, decision.Gate.ID)
	require.NotNil(t, repo.lastMoveWaitingReq)
	require.NotNil(t, repo.lastMoveWaitingReq.EventID)
	require.Equal(t, eventID, *repo.lastMoveWaitingReq.EventID)
	require.Equal(t, 1, countProjectEvents(repo.events, project.ProjectEventDecisionRequested))
}

func TestProjectStoreRunPreDispatchGateLinkedDecisionRetryCompletesWaitAndInbox(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	taskID := uuid.New()
	employeeID := uuid.New()
	ownerID := uuid.New()
	gateID := uuid.New()
	decisionID := uuid.New()
	approvalID := uuid.New()
	checkedAt := time.Date(2026, 6, 21, 11, 0, 0, 0, time.UTC)
	gateInput := project.PreDispatchGateInput{
		ProjectID:          projectID,
		ProjectTaskID:      taskID,
		SelectedEmployeeID: employeeID,
		AttemptNo:          1,
		DispatchReason:     project.DispatchReasonRootReady,
	}
	taskIDCopy := taskID
	repo := &preDispatchGateRepositoryFake{
		projectRecord: project.Project{ID: projectID, TenantID: tenantID, HumanOwnerUserID: ownerID},
		task: project.ProjectTask{
			ID:                        taskID,
			TenantID:                  tenantID,
			ProjectID:                 projectID,
			Title:                     "Recover waiting state",
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
		gates: []project.PreDispatchGateResult{{
			ID:                 gateID,
			TenantID:           tenantID,
			ProjectID:          projectID,
			ProjectTaskID:      taskID,
			SelectedEmployeeID: employeeID,
			AttemptNo:          1,
			DispatchReason:     project.DispatchReasonRootReady,
			IdempotencyKey:     project.PreDispatchGateIdempotencyKey(gateInput),
			DispatchToken:      project.PreDispatchGateDispatchToken(gateInput),
			Status:             project.PreDispatchGateStatusWaitingHuman,
			CheckedAt:          checkedAt,
			DecisionRequestID:  &decisionID,
			HumanActionRequest: project.HumanActionRequest{"waiting_reason": project.HumanWaitReasonApprovalRequired},
		}},
		decisionRequests: []project.DecisionRequest{{
			ID:                   decisionID,
			TenantID:             tenantID,
			ProjectID:            projectID,
			ApprovalRequestID:    approvalID,
			ProjectTaskID:        &taskIDCopy,
			TargetUserID:         ownerID,
			DecisionType:         "project_task_approval",
			TitleSnapshot:        "High risk action requires confirmation",
			StatusSnapshot:       "pending",
			DispatchGateResultID: &gateID,
		}},
	}
	inbox := &preDispatchGateInboxRecorder{}
	store := NewProjectStoreWithApprovalsInboxAndRunStarter(repo, &preDispatchGateApprovalRecorder{approvalID: uuid.New()}, inbox, &projectTaskRunStarterFake{}).
		WithClock(func() time.Time { return checkedAt })

	decision, err := store.RunPreDispatchGate(context.Background(), DispatchProjectTaskInput{
		TenantID:  tenantID,
		ProjectID: projectID,
		TaskID:    taskID,
	})

	require.NoError(t, err)
	require.Equal(t, gateID, decision.Gate.ID)
	require.NotNil(t, repo.task.WaitingRequestID)
	require.Equal(t, decisionID, *repo.task.WaitingRequestID)
	require.Equal(t, project.ProjectTaskStatusWaitingHuman, repo.task.Status)
	require.Len(t, repo.decisionRequests, 1)
	require.Len(t, inbox.upserts, 1)
	require.NotNil(t, inbox.upserts[0].DispatchGateResultID)
	require.Equal(t, gateID, *inbox.upserts[0].DispatchGateResultID)
}

func TestProjectStoreRunPreDispatchGateWaitingTaskRetryLinksGateAndInbox(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	taskID := uuid.New()
	employeeID := uuid.New()
	ownerID := uuid.New()
	decisionID := uuid.New()
	approvalID := uuid.New()
	checkedAt := time.Date(2026, 6, 21, 11, 0, 0, 0, time.UTC)
	taskIDCopy := taskID
	waitingReason := project.HumanWaitReasonApprovalRequired
	repo := &preDispatchGateRepositoryFake{
		projectRecord: project.Project{ID: projectID, TenantID: tenantID, HumanOwnerUserID: ownerID},
		task: project.ProjectTask{
			ID:                        taskID,
			TenantID:                  tenantID,
			ProjectID:                 projectID,
			Title:                     "Link waiting decision",
			Status:                    project.ProjectTaskStatusWaitingHuman,
			WaitingReason:             &waitingReason,
			WaitingRequestID:          &decisionID,
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
		decisionRequests: []project.DecisionRequest{{
			ID:                decisionID,
			TenantID:          tenantID,
			ProjectID:         projectID,
			ApprovalRequestID: approvalID,
			ProjectTaskID:     &taskIDCopy,
			TargetUserID:      ownerID,
			DecisionType:      "project_task_approval",
			TitleSnapshot:     "High risk action requires confirmation",
			StatusSnapshot:    "pending",
		}},
	}
	inbox := &preDispatchGateInboxRecorder{}
	store := NewProjectStoreWithApprovalsInboxAndRunStarter(repo, &preDispatchGateApprovalRecorder{approvalID: uuid.New()}, inbox, &projectTaskRunStarterFake{}).
		WithClock(func() time.Time { return checkedAt })

	decision, err := store.RunPreDispatchGate(context.Background(), DispatchProjectTaskInput{
		TenantID:  tenantID,
		ProjectID: projectID,
		TaskID:    taskID,
	})

	require.NoError(t, err)
	require.NotNil(t, decision.Gate.DecisionRequestID)
	require.Equal(t, decisionID, *decision.Gate.DecisionRequestID)
	require.Len(t, repo.decisionRequests, 1)
	require.NotNil(t, repo.decisionRequests[0].DispatchGateResultID)
	require.Equal(t, decision.Gate.ID, *repo.decisionRequests[0].DispatchGateResultID)
	require.Len(t, inbox.upserts, 1)
	require.NotNil(t, inbox.upserts[0].DispatchGateResultID)
	require.Equal(t, decision.Gate.ID, *inbox.upserts[0].DispatchGateResultID)
}

func TestProjectStoreRunPreDispatchGateReaderMergeKeepsProjectExecutorStatus(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	taskID := uuid.New()
	employeeID := uuid.New()
	repo := &preDispatchGateRepositoryFake{
		projectRecord: project.Project{ID: projectID, TenantID: tenantID, HumanOwnerUserID: uuid.New()},
		task: project.ProjectTask{
			ID:                        taskID,
			TenantID:                  tenantID,
			ProjectID:                 projectID,
			Title:                     "Merge employee facts",
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
			PolicyAllowed:      true,
			RequiredLoadSlots:  2,
			AvailableLoadSlots: 2,
		},
		runtime: project.PreDispatchRuntimeSnapshot{
			NodeOnline:              true,
			ProviderAvailable:       true,
			WorkspaceReady:          true,
			SlotAvailable:           true,
			ContractVersionAccepted: true,
		},
	}
	store := NewProjectStoreWithApprovalsInboxAndRunStarter(repo, nil, nil, &projectTaskRunStarterFake{}).
		WithPreDispatchGateReaders(reader, nil)

	decision, err := store.RunPreDispatchGate(context.Background(), DispatchProjectTaskInput{
		TenantID:  tenantID,
		ProjectID: projectID,
		TaskID:    taskID,
	})

	require.NoError(t, err)
	require.Equal(t, projectID, reader.projectID)
	require.Equal(t, project.PreDispatchGateStatusPassed, decision.Gate.Status)
	require.True(t, decision.AllowRunStart)
}

func TestProjectStoreRunPreDispatchGateCapabilityReaderPreservesPlannerHardMissing(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	taskID := uuid.New()
	employeeID := uuid.New()
	repo := &preDispatchGateRepositoryFake{
		projectRecord: project.Project{ID: projectID, TenantID: tenantID, HumanOwnerUserID: uuid.New()},
		task: project.ProjectTask{
			ID:                        taskID,
			TenantID:                  tenantID,
			ProjectID:                 projectID,
			Title:                     "Preserve capability evidence",
			Status:                    project.ProjectTaskStatusPlanned,
			AssignedDigitalEmployeeID: &employeeID,
			AttemptCount:              0,
			InputRequirements: map[string]any{
				"required_capabilities": []any{"database.read"},
			},
			PlannerMetadata: map[string]any{
				"employee_selection": map[string]any{
					"required_capabilities": []any{"database.read", "database.write"},
					"missing_capabilities":  []any{"database.write"},
				},
			},
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
	capabilityReader := preDispatchGateCapabilityReader{
		capabilities: project.PreDispatchCapabilitySnapshot{
			Required: []string{"database.read"},
		},
	}
	store := NewProjectStoreWithApprovalsInboxAndRunStarter(repo, nil, nil, &projectTaskRunStarterFake{}).
		WithPreDispatchGateReaders(nil, capabilityReader)

	decision, err := store.RunPreDispatchGate(context.Background(), DispatchProjectTaskInput{
		TenantID:  tenantID,
		ProjectID: projectID,
		TaskID:    taskID,
	})

	require.NoError(t, err)
	require.Equal(t, project.PreDispatchGateStatusReplanRequired, decision.Gate.Status)
	require.True(t, decision.Terminal)
	require.Len(t, decision.Gate.Blockers, 1)
	require.Equal(t, "capability.hard_missing", decision.Gate.Blockers[0].Key)
	require.Equal(t, []string{"database.write"}, decision.Gate.Blockers[0].Details["hard_missing"])
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

	projectRecord          project.Project
	task                   project.ProjectTask
	currentAttempt         *project.ProjectTaskAttempt
	dependencies           []project.ProjectTaskDependency
	dependencyTasks        map[uuid.UUID]project.ProjectTask
	projectTaskResults     []project.ProjectTaskResult
	members                []project.ProjectMember
	events                 []project.ProjectEvent
	gates                  []project.PreDispatchGateResult
	decisionRequests       []project.DecisionRequest
	operations             []string
	createDecisionErr      error
	linkDecisionErr        error
	listDecisionErr        error
	listDecisionCalls      int
	directDecisionCalls    int
	missingActivePlacement bool
	lastMoveWaitingReq     *project.MoveProjectTaskToWaitingHumanForPreDispatchGateRequest
}

func preDispatchBlockerKeys(blockers []project.PreDispatchGateBlocker) []string {
	keys := make([]string, 0, len(blockers))
	for _, blocker := range blockers {
		keys = append(keys, blocker.Key)
	}
	return keys
}

func (r *preDispatchGateRepositoryFake) GetProject(ctx context.Context, tenantID, projectID uuid.UUID) (project.Project, error) {
	if r.projectRecord.TenantID == tenantID && r.projectRecord.ID == projectID {
		return r.projectRecord, nil
	}
	return project.Project{}, project.ErrProjectNotFound
}

func (r *preDispatchGateRepositoryFake) GetActiveProjectPlacement(ctx context.Context, tenantID, projectID uuid.UUID) (project.ProjectRuntimePlacement, error) {
	if r.missingActivePlacement {
		return project.ProjectRuntimePlacement{}, project.ErrProjectNotFound
	}
	if r.projectRecord.TenantID != tenantID || r.projectRecord.ID != projectID {
		return project.ProjectRuntimePlacement{}, project.ErrProjectNotFound
	}
	return project.ProjectRuntimePlacement{
		ID:              uuid.New(),
		TenantID:        tenantID,
		ProjectID:       projectID,
		RuntimeNodeID:   uuid.New(),
		PlacementStatus: project.ProjectRuntimePlacementStateActive,
	}, nil
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

func (r *preDispatchGateRepositoryFake) ListProjectTaskResults(ctx context.Context, req project.ListProjectTaskResultsRequest) ([]project.ProjectTaskResult, error) {
	results := make([]project.ProjectTaskResult, 0, len(r.projectTaskResults))
	for _, result := range r.projectTaskResults {
		if result.TenantID == req.TenantID && result.ProjectID == req.ProjectID && result.ProjectTaskID == req.ProjectTaskID {
			results = append(results, result)
		}
	}
	return results, nil
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

func preDispatchGateTaskResult(tenantID, projectID, taskID uuid.UUID, decision project.TaskResultDecision, validationStatus string, at time.Time) project.ProjectTaskResult {
	return project.ProjectTaskResult{
		ID:               uuid.New(),
		TenantID:         tenantID,
		ProjectID:        projectID,
		ProjectTaskID:    taskID,
		ResultStatus:     project.TaskResultStatusCompleted,
		ValidationStatus: validationStatus,
		Decision:         decision,
		Contract: project.TaskResultContract{
			Status:  project.TaskResultStatusCompleted,
			Summary: "dependency result",
		},
		CreatedAt: at,
		UpdatedAt: at,
	}
}

func (r *preDispatchGateRepositoryFake) setDependencyLatestResult(taskID uuid.UUID, result project.ProjectTaskResult) {
	r.projectTaskResults = append(r.projectTaskResults, result)
	if r.dependencyTasks == nil {
		return
	}
	task, ok := r.dependencyTasks[taskID]
	if !ok {
		return
	}
	task.LatestTaskResultID = &result.ID
	task.UpdatedAt = result.UpdatedAt
	r.dependencyTasks[taskID] = task
}

func (r *preDispatchGateRepositoryFake) AppendProjectEvent(ctx context.Context, req project.AppendProjectEventRequest) (project.ProjectEvent, error) {
	r.operations = append(r.operations, "append_event")
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

func (r *preDispatchGateRepositoryFake) ProjectTaskEventExists(ctx context.Context, tenantID, projectID uuid.UUID, eventType project.ProjectEventType, actorID string) (bool, error) {
	for _, event := range r.events {
		if event.TenantID == tenantID && event.ProjectID == projectID && event.EventType == eventType && event.ActorID == actorID {
			return true, nil
		}
	}
	return false, nil
}

func (r *preDispatchGateRepositoryFake) GetProjectEventByTypeAndActor(ctx context.Context, tenantID, projectID uuid.UUID, eventType project.ProjectEventType, actorID string) (project.ProjectEvent, error) {
	for _, event := range r.events {
		if event.TenantID == tenantID && event.ProjectID == projectID && event.EventType == eventType && event.ActorID == actorID {
			return event, nil
		}
	}
	return project.ProjectEvent{}, project.ErrProjectNotFound
}

func (r *preDispatchGateRepositoryFake) RecordPreDispatchGateResult(ctx context.Context, req project.RecordPreDispatchGateResultRequest) (project.PreDispatchGateResult, error) {
	r.operations = append(r.operations, "record_gate")
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

func (r *preDispatchGateRepositoryFake) GetDecisionRequest(ctx context.Context, tenantID, projectID, decisionRequestID uuid.UUID) (project.DecisionRequest, error) {
	for _, decision := range r.decisionRequests {
		if decision.TenantID == tenantID && decision.ProjectID == projectID && decision.ID == decisionRequestID {
			return decision, nil
		}
	}
	return project.DecisionRequest{}, project.ErrProjectNotFound
}

func (r *preDispatchGateRepositoryFake) GetDecisionRequestByApprovalAndTask(ctx context.Context, tenantID, projectID, approvalRequestID, projectTaskID uuid.UUID) (project.DecisionRequest, error) {
	r.directDecisionCalls++
	for _, decision := range r.decisionRequests {
		if decision.TenantID != tenantID || decision.ProjectID != projectID || decision.ApprovalRequestID != approvalRequestID {
			continue
		}
		if decision.ProjectTaskID == nil || *decision.ProjectTaskID != projectTaskID {
			continue
		}
		return decision, nil
	}
	return project.DecisionRequest{}, project.ErrProjectNotFound
}

func (r *preDispatchGateRepositoryFake) LinkPreDispatchGateDecisionRequest(ctx context.Context, req project.LinkPreDispatchGateDecisionRequest) (project.PreDispatchGateResult, error) {
	if r.linkDecisionErr != nil {
		err := r.linkDecisionErr
		r.linkDecisionErr = nil
		return project.PreDispatchGateResult{}, err
	}
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
	reqCopy := req
	r.lastMoveWaitingReq = &reqCopy
	r.task.Status = project.ProjectTaskStatusWaitingHuman
	r.task.WaitingReason = stringPtr(req.WaitingReason)
	r.task.WaitingRequestID = &req.DecisionRequestID
	r.task.LatestDispatchGateResultID = &req.GateResultID
	return r.task, nil
}

func (r *preDispatchGateRepositoryFake) CreateDecisionRequest(ctx context.Context, req project.CreateDecisionRequestRequest) (project.DecisionRequest, error) {
	if r.createDecisionErr != nil {
		err := r.createDecisionErr
		r.createDecisionErr = nil
		return project.DecisionRequest{}, err
	}
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

func (r *preDispatchGateRepositoryFake) ListDecisionRequests(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]project.DecisionRequest, error) {
	r.listDecisionCalls++
	if r.listDecisionErr != nil {
		return nil, r.listDecisionErr
	}
	matches := make([]project.DecisionRequest, 0)
	for _, decision := range r.decisionRequests {
		if decision.TenantID == tenantID && decision.ProjectID == projectID {
			matches = append(matches, decision)
		}
	}
	if offset >= int32(len(matches)) {
		return []project.DecisionRequest{}, nil
	}
	end := int(offset + limit)
	if limit <= 0 || end > len(matches) {
		end = len(matches)
	}
	return append([]project.DecisionRequest(nil), matches[offset:end]...), nil
}

type preDispatchGateApprovalRecorder struct {
	approvalID uuid.UUID
	requests   []approval.CreateRequestInput
	records    []approval.ApprovalRequest
}

func (r *preDispatchGateApprovalRecorder) CreateRequest(ctx context.Context, input approval.CreateRequestInput) (*approval.ApprovalRequest, error) {
	r.requests = append(r.requests, input)
	id := r.approvalID
	if id == uuid.Nil {
		id = uuid.New()
	}
	request := approval.ApprovalRequest{
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
	}
	r.records = append(r.records, request)
	return &request, nil
}

func (r *preDispatchGateApprovalRecorder) GetRequestByResource(ctx context.Context, tenantID uuid.UUID, resourceType string, resourceID uuid.UUID) (*approval.ApprovalRequest, error) {
	for index := len(r.records) - 1; index >= 0; index-- {
		request := r.records[index]
		if request.TenantID == tenantID && request.ResourceType == resourceType && request.ResourceID == resourceID && request.Status == approval.ApprovalStatusPending {
			return &request, nil
		}
	}
	return nil, approval.ErrApprovalNotFound
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
	employee  project.PreDispatchEmployeeSnapshot
	runtime   project.PreDispatchRuntimeSnapshot
	projectID uuid.UUID
}

func (r *preDispatchGateEmployeeRuntimeReader) GetEmployeeRuntimeSnapshot(ctx context.Context, tenantID, projectID, employeeID uuid.UUID) (project.PreDispatchEmployeeSnapshot, project.PreDispatchRuntimeSnapshot, error) {
	r.projectID = projectID
	return r.employee, r.runtime, nil
}

type preDispatchGateCapabilityReader struct {
	capabilities project.PreDispatchCapabilitySnapshot
	tools        project.PreDispatchToolSnapshot
}

func (r preDispatchGateCapabilityReader) GetEmployeeCapabilitySnapshot(ctx context.Context, tenantID, employeeID uuid.UUID, task project.ProjectTask) (project.PreDispatchCapabilitySnapshot, project.PreDispatchToolSnapshot, error) {
	return r.capabilities, r.tools, nil
}

func countProjectEvents(events []project.ProjectEvent, eventType project.ProjectEventType) int {
	count := 0
	for _, event := range events {
		if event.EventType == eventType {
			count++
		}
	}
	return count
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
