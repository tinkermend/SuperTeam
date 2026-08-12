package project

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestSweepStrandedBlockedProjectTasksCancelsDownstreamOfFailed(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewServiceWithCoordinatorApprovalsInboxAndArchiveArtifactLocker(repo, NoopCoordinatorSignalClient{}, nil, nil, nil)
	require.NoError(t, err)

	tenantID := uuid.New()
	projectID := uuid.New()
	demandID := uuid.New()
	failedID := uuid.New()
	blockedID := uuid.New()
	runningID := uuid.New()
	repo.projects[projectID] = Project{ID: projectID, TenantID: tenantID, Status: ProjectStatusRunning}
	repo.demands = append(repo.demands, ProjectDemand{
		ID: demandID, TenantID: tenantID, ProjectID: projectID, Status: ProjectDemandStatusExecuting,
	})
	repo.tasks = append(repo.tasks,
		ProjectTask{ID: failedID, TenantID: tenantID, ProjectID: projectID, DemandID: &demandID, Title: "Create", Status: ProjectTaskStatusFailed},
		ProjectTask{ID: blockedID, TenantID: tenantID, ProjectID: projectID, DemandID: &demandID, Title: "Verify", Status: ProjectTaskStatusBlocked},
		ProjectTask{ID: runningID, TenantID: tenantID, ProjectID: projectID, DemandID: &demandID, Title: "Other", Status: ProjectTaskStatusRunning},
	)
	repo.taskDependents = map[uuid.UUID][]uuid.UUID{
		failedID: {blockedID},
	}

	n, err := service.SweepStrandedBlockedProjectTasks(context.Background(), 10)
	require.NoError(t, err)
	require.Equal(t, 1, n)
	require.Equal(t, ProjectTaskStatusCancelled, repo.tasks[1].Status)
	require.Equal(t, ProjectTaskStatusRunning, repo.tasks[2].Status)
}

func TestSweepStrandedBlockedProjectTasksSkipsWhenUpstreamStillRunnable(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewServiceWithCoordinatorApprovalsInboxAndArchiveArtifactLocker(repo, NoopCoordinatorSignalClient{}, nil, nil, nil)
	require.NoError(t, err)

	tenantID := uuid.New()
	projectID := uuid.New()
	upstreamID := uuid.New()
	blockedID := uuid.New()
	repo.projects[projectID] = Project{ID: projectID, TenantID: tenantID, Status: ProjectStatusRunning}
	repo.tasks = append(repo.tasks,
		ProjectTask{ID: upstreamID, TenantID: tenantID, ProjectID: projectID, Title: "Create", Status: ProjectTaskStatusRunning},
		ProjectTask{ID: blockedID, TenantID: tenantID, ProjectID: projectID, Title: "Verify", Status: ProjectTaskStatusBlocked},
	)
	repo.taskDependents = map[uuid.UUID][]uuid.UUID{
		upstreamID: {blockedID},
	}

	n, err := service.SweepStrandedBlockedProjectTasks(context.Background(), 10)
	require.NoError(t, err)
	require.Equal(t, 0, n)
	require.Equal(t, ProjectTaskStatusBlocked, repo.tasks[1].Status)
}
