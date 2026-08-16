package project

import (
	"context"
	"testing"
	"time"

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
	// 取消必须落 system_stranded：人类点重试时据此复活重挂边。
	require.NotNil(t, repo.tasks[1].CancelReason)
	require.Equal(t, ProjectTaskCancelReasonSystemStranded, *repo.tasks[1].CancelReason)
}

// A 期真链现场：任务 21:10:06 失败、看门狗 21:10:08 收口、恢复卡 21:10:09 才建好。
// 只看「卡是否 pending」挡不住这几秒竞态，所以上游刚进终态时先不收口。
func TestSweepStrandedBlockedProjectTasksHonorsFailureGrace(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewServiceWithCoordinatorApprovalsInboxAndArchiveArtifactLocker(repo, NoopCoordinatorSignalClient{}, nil, nil, nil)
	require.NoError(t, err)

	tenantID := uuid.New()
	projectID := uuid.New()
	demandID := uuid.New()
	failedID := uuid.New()
	blockedID := uuid.New()
	repo.projects[projectID] = Project{ID: projectID, TenantID: tenantID, Status: ProjectStatusRunning}
	repo.demands = append(repo.demands, ProjectDemand{
		ID: demandID, TenantID: tenantID, ProjectID: projectID, Status: ProjectDemandStatusExecuting,
	})
	// 刚失败（updated_at = 现在）：宽限内不得收口，哪怕卡还没建出来。
	repo.tasks = append(repo.tasks,
		ProjectTask{ID: failedID, TenantID: tenantID, ProjectID: projectID, DemandID: &demandID, Title: "开发实现", Status: ProjectTaskStatusFailed, UpdatedAt: time.Now().UTC()},
		ProjectTask{ID: blockedID, TenantID: tenantID, ProjectID: projectID, DemandID: &demandID, Title: "代码审查", Status: ProjectTaskStatusBlocked},
	)
	repo.taskDependents = map[uuid.UUID][]uuid.UUID{failedID: {blockedID}}

	n, err := service.SweepStrandedBlockedProjectTasks(context.Background(), 10)
	require.NoError(t, err)
	require.Equal(t, 0, n)
	require.Equal(t, ProjectTaskStatusBlocked, repo.tasks[1].Status)

	// 宽限过后（失败发生在很久以前）才收口。
	repo.tasks[0].UpdatedAt = time.Now().UTC().Add(-2 * strandedBlockedFailureGrace)
	n, err = service.SweepStrandedBlockedProjectTasks(context.Background(), 10)
	require.NoError(t, err)
	require.Equal(t, 1, n)
	require.Equal(t, ProjectTaskStatusCancelled, repo.tasks[1].Status)
}

// 复跑 3 现场：失败任务上还挂着 pending 恢复卡时看门狗不得收口——人类随时能点重试，
// 此刻取消下游会让重试接不回图（rewireRecoverableDependents 跳过终态任务）。
func TestSweepStrandedBlockedProjectTasksSkipsWhileRecoveryDecisionPending(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewServiceWithCoordinatorApprovalsInboxAndArchiveArtifactLocker(repo, NoopCoordinatorSignalClient{}, nil, nil, nil)
	require.NoError(t, err)

	tenantID := uuid.New()
	projectID := uuid.New()
	demandID := uuid.New()
	failedID := uuid.New()
	blockedID := uuid.New()
	repo.projects[projectID] = Project{ID: projectID, TenantID: tenantID, Status: ProjectStatusRunning}
	repo.demands = append(repo.demands, ProjectDemand{
		ID: demandID, TenantID: tenantID, ProjectID: projectID, Status: ProjectDemandStatusExecuting,
	})
	repo.tasks = append(repo.tasks,
		ProjectTask{ID: failedID, TenantID: tenantID, ProjectID: projectID, DemandID: &demandID, Title: "开发实现", Status: ProjectTaskStatusFailed},
		ProjectTask{ID: blockedID, TenantID: tenantID, ProjectID: projectID, DemandID: &demandID, Title: "代码审查", Status: ProjectTaskStatusBlocked},
	)
	repo.taskDependents = map[uuid.UUID][]uuid.UUID{failedID: {blockedID}}
	failedIDPtr := failedID
	repo.decisionRequests = append(repo.decisionRequests, DecisionRequest{
		ID:             uuid.New(),
		TenantID:       tenantID,
		ProjectID:      projectID,
		ProjectTaskID:  &failedIDPtr,
		DecisionType:   "task_failure_recovery",
		StatusSnapshot: "pending",
	})

	n, err := service.SweepStrandedBlockedProjectTasks(context.Background(), 10)
	require.NoError(t, err)
	require.Equal(t, 0, n)
	require.Equal(t, ProjectTaskStatusBlocked, repo.tasks[1].Status)

	// 恢复卡收敛（人类已处置）后才轮到看门狗收口。
	repo.decisionRequests[0].StatusSnapshot = "approved"
	n, err = service.SweepStrandedBlockedProjectTasks(context.Background(), 10)
	require.NoError(t, err)
	require.Equal(t, 1, n)
	require.Equal(t, ProjectTaskStatusCancelled, repo.tasks[1].Status)
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
