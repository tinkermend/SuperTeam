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
