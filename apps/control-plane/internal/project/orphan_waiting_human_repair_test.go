package project

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestSweepOrphanWaitingHumanProjectTasksCreatesDecisionAndBinds(t *testing.T) {
	repo := newMemoryRepository()
	inbox := &fakeDecisionInboxProjector{}
	service, err := NewServiceWithCoordinatorApprovalsInboxAndArchiveArtifactLocker(repo, NoopCoordinatorSignalClient{}, nil, inbox, nil)
	require.NoError(t, err)

	tenantID := uuid.New()
	projectID := uuid.New()
	ownerID := uuid.New()
	repo.projects[projectID] = Project{
		ID:               projectID,
		TenantID:         tenantID,
		HumanOwnerUserID: ownerID,
		Status:           ProjectStatusRunning,
	}
	taskID := uuid.New()
	repo.tasks = append(repo.tasks, ProjectTask{
		ID:        taskID,
		TenantID:  tenantID,
		ProjectID: projectID,
		Title:     "orphan wait",
		Status:    ProjectTaskStatusWaitingHuman,
	})

	n, err := service.SweepOrphanWaitingHumanProjectTasks(context.Background(), 10)
	require.NoError(t, err)
	require.Equal(t, 1, n)
	require.Len(t, repo.decisionRequests, 1)
	require.NotNil(t, repo.tasks[0].WaitingRequestID)
	require.Equal(t, repo.decisionRequests[0].ID, *repo.tasks[0].WaitingRequestID)
	require.Len(t, inbox.upserts, 1)
}

func TestSweepOrphanWaitingHumanProjectTasksRebindsExistingOpenDecision(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewServiceWithCoordinatorApprovalsInboxAndArchiveArtifactLocker(repo, NoopCoordinatorSignalClient{}, nil, nil, nil)
	require.NoError(t, err)

	tenantID := uuid.New()
	projectID := uuid.New()
	ownerID := uuid.New()
	repo.projects[projectID] = Project{
		ID:               projectID,
		TenantID:         tenantID,
		HumanOwnerUserID: ownerID,
		Status:           ProjectStatusRunning,
	}
	taskID := uuid.New()
	decisionID := uuid.New()
	repo.tasks = append(repo.tasks, ProjectTask{
		ID:        taskID,
		TenantID:  tenantID,
		ProjectID: projectID,
		Title:     "unlink wait",
		Status:    ProjectTaskStatusWaitingHuman,
	})
	repo.decisionRequests = append(repo.decisionRequests, DecisionRequest{
		ID:             decisionID,
		TenantID:       tenantID,
		ProjectID:      projectID,
		ProjectTaskID:  &taskID,
		TargetUserID:   ownerID,
		DecisionType:   "project_task_clarification",
		TitleSnapshot:  "unlink wait",
		StatusSnapshot: "pending",
	})

	n, err := service.SweepOrphanWaitingHumanProjectTasks(context.Background(), 10)
	require.NoError(t, err)
	require.Equal(t, 1, n)
	require.Len(t, repo.decisionRequests, 1, "must not duplicate decision")
	require.NotNil(t, repo.tasks[0].WaitingRequestID)
	require.Equal(t, decisionID, *repo.tasks[0].WaitingRequestID)
}

func TestSweepOrphanWaitingHumanDoesNotMintZeroApprovalProjectTaskApproval(t *testing.T) {
	repo := newMemoryRepository()
	inbox := &fakeDecisionInboxProjector{}
	service, err := NewServiceWithCoordinatorApprovalsInboxAndArchiveArtifactLocker(repo, NoopCoordinatorSignalClient{}, nil, inbox, nil)
	require.NoError(t, err)

	tenantID := uuid.New()
	projectID := uuid.New()
	ownerID := uuid.New()
	repo.projects[projectID] = Project{
		ID: projectID, TenantID: tenantID, HumanOwnerUserID: ownerID, Status: ProjectStatusRunning,
	}
	reason := HumanWaitReasonApprovalRequired
	taskID := uuid.New()
	repo.tasks = append(repo.tasks, ProjectTask{
		ID: taskID, TenantID: tenantID, ProjectID: projectID,
		Title: "high risk orphan", Status: ProjectTaskStatusWaitingHuman, WaitingReason: &reason,
	})

	n, err := service.SweepOrphanWaitingHumanProjectTasks(context.Background(), 10)
	require.NoError(t, err)
	require.Equal(t, 1, n)
	require.Len(t, repo.decisionRequests, 1)
	require.Equal(t, "project_task_clarification", repo.decisionRequests[0].DecisionType,
		"must not mint project_task_approval without a real approval_request_id")
	require.Equal(t, uuid.Nil, repo.decisionRequests[0].ApprovalRequestID)
	require.NotNil(t, repo.decisionRequests[0].SummarySnapshot)
	require.Contains(t, *repo.decisionRequests[0].SummarySnapshot, "系统无法重建审批对象")
}

func TestSweepOrphanWaitingHumanHealsZombieWhenApprovedGateCardExists(t *testing.T) {
	repo := newMemoryRepository()
	coordinator := &fakeCoordinatorSignalClient{}
	service, err := NewServiceWithCoordinatorApprovalsInboxAndArchiveArtifactLocker(repo, coordinator, nil, nil, nil)
	require.NoError(t, err)

	tenantID := uuid.New()
	projectID := uuid.New()
	ownerID := uuid.New()
	repo.projects[projectID] = Project{
		ID: projectID, TenantID: tenantID, HumanOwnerUserID: ownerID, Status: ProjectStatusRunning,
	}
	taskID := uuid.New()
	gateID := uuid.New()
	realID := uuid.New()
	zombieID := uuid.New()
	summary := orphanWaitingHumanRepairSummary + "（原因：需要人工审批）"
	repo.decisionRequests = append(repo.decisionRequests,
		DecisionRequest{
			ID: realID, TenantID: tenantID, ProjectID: projectID, ProjectTaskID: &taskID,
			ApprovalRequestID: uuid.New(), DecisionType: "project_task_approval",
			StatusSnapshot: "approved", DispatchGateResultID: &gateID,
		},
		DecisionRequest{
			ID: zombieID, TenantID: tenantID, ProjectID: projectID, ProjectTaskID: &taskID,
			ApprovalRequestID: uuid.Nil, DecisionType: "project_task_approval",
			StatusSnapshot: "pending", SummarySnapshot: &summary,
		},
	)
	reason := HumanWaitReasonApprovalRequired
	repo.tasks = append(repo.tasks, ProjectTask{
		ID: taskID, TenantID: tenantID, ProjectID: projectID,
		Title: "stuck after approve", Status: ProjectTaskStatusWaitingHuman,
		WaitingReason: &reason, WaitingRequestID: &zombieID,
	})

	n, err := service.SweepOrphanWaitingHumanProjectTasks(context.Background(), 10)
	require.NoError(t, err)
	require.Equal(t, 1, n)
	require.Equal(t, ProjectTaskStatusPlanned, repo.tasks[0].Status)
	require.Nil(t, repo.tasks[0].WaitingRequestID)
	// zombie cancelled
	var zombie DecisionRequest
	for _, d := range repo.decisionRequests {
		if d.ID == zombieID {
			zombie = d
		}
	}
	require.Equal(t, "cancelled", zombie.StatusSnapshot)
	require.Equal(t, 1, coordinator.retrySignals)
	require.Equal(t, taskID, coordinator.lastRetry.ProjectTaskID)
}

func TestSweepOrphanWaitingHumanHealsWhenPointerOnApprovedGateCard(t *testing.T) {
	// Intermediate state after approve before orphan stole the pointer: heal → planned + redispatch.
	repo := newMemoryRepository()
	coordinator := &fakeCoordinatorSignalClient{}
	service, err := NewServiceWithCoordinatorApprovalsInboxAndArchiveArtifactLocker(repo, coordinator, nil, nil, nil)
	require.NoError(t, err)

	tenantID := uuid.New()
	projectID := uuid.New()
	ownerID := uuid.New()
	repo.projects[projectID] = Project{
		ID: projectID, TenantID: tenantID, HumanOwnerUserID: ownerID, Status: ProjectStatusRunning,
	}
	taskID := uuid.New()
	gateID := uuid.New()
	realID := uuid.New()
	repo.decisionRequests = append(repo.decisionRequests, DecisionRequest{
		ID: realID, TenantID: tenantID, ProjectID: projectID, ProjectTaskID: &taskID,
		ApprovalRequestID: uuid.New(), DecisionType: "project_task_approval",
		StatusSnapshot: "approved", DispatchGateResultID: &gateID,
	})
	reason := HumanWaitReasonApprovalRequired
	repo.tasks = append(repo.tasks, ProjectTask{
		ID: taskID, TenantID: tenantID, ProjectID: projectID,
		Status: ProjectTaskStatusWaitingHuman, WaitingReason: &reason, WaitingRequestID: &realID,
	})

	n, err := service.SweepOrphanWaitingHumanProjectTasks(context.Background(), 10)
	require.NoError(t, err)
	require.Equal(t, 1, n)
	require.Len(t, repo.decisionRequests, 1, "must not mint another decision")
	require.Equal(t, ProjectTaskStatusPlanned, repo.tasks[0].Status)
	require.Equal(t, 1, coordinator.retrySignals)
}

// Regression (2026-08-11): a task that already converged onto an honest
// recovery card must be left alone, even though an approved gate-linked
// approval still exists on it. Healing it released to planned, dispatch hit the
// same wall, recovery minted yet another card and rebound the pointer, and the
// next watchdog tick repeated — a once-a-minute self-sustaining loop that
// accumulated 99 pending cards on a single task in the dev database.
func TestSweepOrphanWaitingHumanDoesNotHealTaskOnHonestRecoveryCard(t *testing.T) {
	repo := newMemoryRepository()
	coordinator := &fakeCoordinatorSignalClient{}
	service, err := NewServiceWithCoordinatorApprovalsInboxAndArchiveArtifactLocker(repo, coordinator, nil, nil, nil)
	require.NoError(t, err)

	tenantID := uuid.New()
	projectID := uuid.New()
	ownerID := uuid.New()
	repo.projects[projectID] = Project{
		ID: projectID, TenantID: tenantID, HumanOwnerUserID: ownerID, Status: ProjectStatusRunning,
	}
	taskID := uuid.New()
	gateID := uuid.New()
	realID := uuid.New()
	recoveryID := uuid.New()
	repo.decisionRequests = append(repo.decisionRequests,
		// The durable grant from the human's earlier gate approval.
		DecisionRequest{
			ID: realID, TenantID: tenantID, ProjectID: projectID, ProjectTaskID: &taskID,
			ApprovalRequestID: uuid.New(), DecisionType: "project_task_approval",
			StatusSnapshot: "approved", DispatchGateResultID: &gateID,
		},
		// The honest recovery card the task has since converged onto.
		DecisionRequest{
			ID: recoveryID, TenantID: tenantID, ProjectID: projectID, ProjectTaskID: &taskID,
			ApprovalRequestID: uuid.Nil, DecisionType: "project_task_recovery",
			StatusSnapshot: "pending",
		},
	)
	reason := HumanWaitReasonPlanInvalid
	maxAttempts := int32(3)
	repo.tasks = append(repo.tasks, ProjectTask{
		ID: taskID, TenantID: tenantID, ProjectID: projectID,
		Title: "converged on recovery card", Status: ProjectTaskStatusWaitingHuman,
		WaitingReason: &reason, WaitingRequestID: &recoveryID,
		AttemptCount: 3, MaxAttempts: &maxAttempts,
	})

	n, err := service.SweepOrphanWaitingHumanProjectTasks(context.Background(), 10)
	require.NoError(t, err)
	require.Equal(t, 0, n, "converged task must not be touched")
	require.Len(t, repo.decisionRequests, 2, "must not mint another decision card")
	require.Equal(t, ProjectTaskStatusWaitingHuman, repo.tasks[0].Status, "must stay parked for the human")
	require.Equal(t, recoveryID, *repo.tasks[0].WaitingRequestID, "pointer must stay on the recovery card")
	require.Equal(t, 0, coordinator.retrySignals, "must not re-arm the redispatch loop")
	require.Equal(t, "pending", repo.decisionRequests[1].StatusSnapshot, "recovery card must stay actionable")
}

// The heal path stays reachable for the shape it was built for: pointer absent
// with waiting_reason=approval_required (post-approve intermediate state).
func TestSweepOrphanWaitingHumanHealsWhenPointerAbsentAndApprovalRequired(t *testing.T) {
	repo := newMemoryRepository()
	coordinator := &fakeCoordinatorSignalClient{}
	service, err := NewServiceWithCoordinatorApprovalsInboxAndArchiveArtifactLocker(repo, coordinator, nil, nil, nil)
	require.NoError(t, err)

	tenantID := uuid.New()
	projectID := uuid.New()
	ownerID := uuid.New()
	repo.projects[projectID] = Project{
		ID: projectID, TenantID: tenantID, HumanOwnerUserID: ownerID, Status: ProjectStatusRunning,
	}
	taskID := uuid.New()
	gateID := uuid.New()
	realID := uuid.New()
	repo.decisionRequests = append(repo.decisionRequests, DecisionRequest{
		ID: realID, TenantID: tenantID, ProjectID: projectID, ProjectTaskID: &taskID,
		ApprovalRequestID: uuid.New(), DecisionType: "project_task_approval",
		StatusSnapshot: "approved", DispatchGateResultID: &gateID,
	})
	reason := HumanWaitReasonApprovalRequired
	repo.tasks = append(repo.tasks, ProjectTask{
		ID: taskID, TenantID: tenantID, ProjectID: projectID,
		Title: "approved, pointer not yet stolen", Status: ProjectTaskStatusWaitingHuman,
		WaitingReason: &reason,
	})

	n, err := service.SweepOrphanWaitingHumanProjectTasks(context.Background(), 10)
	require.NoError(t, err)
	require.Equal(t, 1, n)
	require.Len(t, repo.decisionRequests, 1, "must not mint another decision")
	require.Equal(t, ProjectTaskStatusPlanned, repo.tasks[0].Status)
	require.Equal(t, 1, coordinator.retrySignals)
}
