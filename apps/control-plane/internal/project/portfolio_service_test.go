package project

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

type portfolioBumpRepo struct {
	*memoryRepository
}

func (r *portfolioBumpRepo) GetProjectPortfolio(ctx context.Context, req GetProjectPortfolioRequest) (ProjectPortfolioResponse, error) {
	resp, err := r.memoryRepository.GetProjectPortfolio(ctx, req)
	if err != nil {
		return resp, err
	}
	// Inject a broken total so service must degrade (§9).
	resp.Summary.ActiveTaskCounts.Total = resp.Summary.ActiveTaskCounts.Sum() + 99
	return resp, nil
}

func TestGetProjectPortfolioBucketInvariantDegrades(t *testing.T) {
	inner := newMemoryRepository()
	svc, err := NewService(&portfolioBumpRepo{memoryRepository: inner})
	require.NoError(t, err)

	tenantID := uuid.New()
	actorID := uuid.New()
	projectID := uuid.New()
	inner.projects[projectID] = Project{
		ID: projectID, TenantID: tenantID, Name: "p", Status: ProjectStatusRunning,
		HumanOwnerUserID: actorID,
	}
	inner.tasks = append(inner.tasks, ProjectTask{
		ID: uuid.New(), TenantID: tenantID, ProjectID: projectID, Status: "running",
	})

	resp, err := svc.GetProjectPortfolio(context.Background(), GetProjectPortfolioRequest{
		TenantID: tenantID, ActorUserID: actorID, Limit: 12,
	})
	require.NoError(t, err)
	require.True(t, resp.CountsDegraded, "bucket mismatch must degrade not 5xx")
	require.Equal(t, resp.Summary.ActiveTaskCounts.Sum(), resp.Summary.ActiveTaskCounts.Total)
}

func TestGetProjectPortfolioWaitingHumanDualCount(t *testing.T) {
	repo := newMemoryRepository()
	svc, err := NewService(repo)
	require.NoError(t, err)

	tenantID := uuid.New()
	actorID := uuid.New()
	projectID := uuid.New()
	repo.projects[projectID] = Project{
		ID: projectID, TenantID: tenantID, Name: "dual", Status: ProjectStatusRunning,
		HumanOwnerUserID: actorID,
	}

	// 19 waiting_human tasks; 17 linked to open decisions → orphan 2.
	for i := 0; i < 19; i++ {
		taskID := uuid.New()
		repo.tasks = append(repo.tasks, ProjectTask{
			ID: taskID, TenantID: tenantID, ProjectID: projectID, Status: ProjectTaskStatusWaitingHuman,
		})
		if i < 17 {
			tid := taskID
			repo.decisionRequests = append(repo.decisionRequests, DecisionRequest{
				ID: uuid.New(), TenantID: tenantID, ProjectID: projectID,
				ProjectTaskID: &tid, StatusSnapshot: "pending",
			})
		}
	}
	// One extra open decision without task link (still counts as open_decision).
	repo.decisionRequests = append(repo.decisionRequests, DecisionRequest{
		ID: uuid.New(), TenantID: tenantID, ProjectID: projectID, StatusSnapshot: "open",
	})

	resp, err := svc.GetProjectPortfolio(context.Background(), GetProjectPortfolioRequest{
		TenantID: tenantID, ActorUserID: actorID, Limit: 12,
	})
	require.NoError(t, err)
	require.Len(t, resp.Items, 1)
	item := resp.Items[0]
	require.Equal(t, 19, item.TaskCounts.WaitingHuman, "task_counts.waiting_human must be wide")
	require.Equal(t, 2, item.Attention.WaitingHumanUnlinkedCount, "attention must be orphan")
	require.Equal(t, 18, item.Attention.OpenDecisionCount)
}

func TestGetProjectPortfolioMineOnly(t *testing.T) {
	repo := newMemoryRepository()
	svc, err := NewService(repo)
	require.NoError(t, err)

	tenantID := uuid.New()
	actorID := uuid.New()
	otherID := uuid.New()
	mine := uuid.New()
	theirs := uuid.New()
	memberOnly := uuid.New()
	inactiveMember := uuid.New()

	repo.projects[mine] = Project{
		ID: mine, TenantID: tenantID, Name: "mine", Status: ProjectStatusRunning,
		HumanOwnerUserID: actorID, HumanOwnerUserIDs: []uuid.UUID{actorID},
	}
	repo.projects[theirs] = Project{
		ID: theirs, TenantID: tenantID, Name: "theirs", Status: ProjectStatusRunning,
		HumanOwnerUserID: otherID, HumanOwnerUserIDs: []uuid.UUID{otherID},
	}
	repo.projects[memberOnly] = Project{
		ID: memberOnly, TenantID: tenantID, Name: "member", Status: ProjectStatusRunning,
		HumanOwnerUserID: otherID, HumanOwnerUserIDs: []uuid.UUID{otherID},
	}
	repo.members[memberOnly] = []ProjectMember{{
		ProjectID: memberOnly, PrincipalType: PrincipalTypeHumanUser,
		PrincipalID: actorID, Status: "active",
	}}
	repo.projects[inactiveMember] = Project{
		ID: inactiveMember, TenantID: tenantID, Name: "inactive", Status: ProjectStatusRunning,
		HumanOwnerUserID: otherID, HumanOwnerUserIDs: []uuid.UUID{otherID},
	}
	repo.members[inactiveMember] = []ProjectMember{{
		ProjectID: inactiveMember, PrincipalType: PrincipalTypeHumanUser,
		PrincipalID: actorID, Status: "inactive",
	}}

	// mine_only=false → tenant full set
	all, err := svc.GetProjectPortfolio(context.Background(), GetProjectPortfolioRequest{
		TenantID: tenantID, ActorUserID: actorID, Limit: 50, MineOnly: false,
	})
	require.NoError(t, err)
	require.Equal(t, 4, all.Summary.TotalProjects)
	require.Equal(t, int32(4), all.Pagination.Total)

	// mine_only=true → owner + active member only
	mineResp, err := svc.GetProjectPortfolio(context.Background(), GetProjectPortfolioRequest{
		TenantID: tenantID, ActorUserID: actorID, Limit: 50, MineOnly: true,
	})
	require.NoError(t, err)
	require.Equal(t, 2, mineResp.Summary.TotalProjects, "summary must narrow with mine_only")
	require.Equal(t, int32(2), mineResp.Pagination.Total)
	names := map[string]bool{}
	for _, it := range mineResp.Items {
		names[it.Project.Name] = true
	}
	require.True(t, names["mine"])
	require.True(t, names["member"])
	require.False(t, names["theirs"])
	require.False(t, names["inactive"])
}

func TestGetProjectPortfolioLimitBounds(t *testing.T) {
	repo := newMemoryRepository()
	svc, err := NewService(repo)
	require.NoError(t, err)
	tenantID := uuid.New()

	_, err = svc.GetProjectPortfolio(context.Background(), GetProjectPortfolioRequest{
		TenantID: tenantID, Limit: 51,
	})
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrInvalidProject))

	_, err = svc.GetProjectPortfolio(context.Background(), GetProjectPortfolioRequest{
		TenantID: tenantID, Limit: 12, Sort: "nope",
	})
	require.Error(t, err)
}

func TestArchiveReadinessUnchangedWithBlockedAndError(t *testing.T) {
	// §5.2.1: blocked and error remain ActiveTasks → archive blocked.
	repo := newMemoryRepository()
	svc, err := NewService(repo)
	require.NoError(t, err)

	tenantID := uuid.New()
	projectID := uuid.New()
	actorID := uuid.New()
	repo.projects[projectID] = Project{
		ID: projectID, TenantID: tenantID, Name: "gate", Status: ProjectStatusRunning,
		HumanOwnerUserID: actorID,
	}
	repo.tasks = append(repo.tasks,
		ProjectTask{ID: uuid.New(), TenantID: tenantID, ProjectID: projectID, Status: "blocked"},
		ProjectTask{ID: uuid.New(), TenantID: tenantID, ProjectID: projectID, Status: "error"},
		ProjectTask{ID: uuid.New(), TenantID: tenantID, ProjectID: projectID, Status: "completed"},
	)

	summary, err := repo.GetProjectTaskStatusCounts(context.Background(), tenantID, projectID)
	require.NoError(t, err)
	// Display: blocked=1, failed(error)=1, completed=1; ActiveTasks includes blocked+error.
	require.Equal(t, 1, summary.BlockedTasks)
	require.Equal(t, 1, summary.FailedTasks)
	require.Equal(t, 1, summary.CompletedTasks)
	require.Equal(t, 2, summary.ActiveTasks, "blocked+error must stay active for gate")

	ready, err := svc.evaluateArchiveReadiness(context.Background(), tenantID, projectID, repo.projects[projectID], 1, 1)
	require.NoError(t, err)
	require.False(t, ready.CanArchive())
	var found bool
	for _, b := range ready.Blockers {
		if b.Code == "active_tasks" {
			found = true
			require.Equal(t, 2, b.Count)
		}
	}
	require.True(t, found, "active_tasks blocker required")

	// Only terminal (failed/completed/cancelled) — error is NOT terminal for gate,
	// but pure failed IS terminal for gate (failed is in NOT IN set).
	repo2 := newMemoryRepository()
	svc2, err := NewService(repo2)
	require.NoError(t, err)
	pid2 := uuid.New()
	repo2.projects[pid2] = Project{
		ID: pid2, TenantID: tenantID, Name: "ok", Status: ProjectStatusRunning,
		HumanOwnerUserID: actorID,
	}
	repo2.tasks = append(repo2.tasks,
		ProjectTask{ID: uuid.New(), TenantID: tenantID, ProjectID: pid2, Status: "failed"},
		ProjectTask{ID: uuid.New(), TenantID: tenantID, ProjectID: pid2, Status: "completed"},
		ProjectTask{ID: uuid.New(), TenantID: tenantID, ProjectID: pid2, Status: "cancelled"},
	)
	ready2, err := svc2.evaluateArchiveReadiness(context.Background(), tenantID, pid2, repo2.projects[pid2], 1, 1)
	require.NoError(t, err)
	for _, b := range ready2.Blockers {
		require.NotEqual(t, "active_tasks", b.Code)
	}
}

func TestListProjectRunSummariesStillWideFailedAndDualWaiting(t *testing.T) {
	// §3.5 / §8.1: run-summary contract must not be regressed by portfolio work.
	repo := newMemoryRepository()
	svc, err := NewService(repo)
	require.NoError(t, err)
	tenantID := uuid.New()
	repo.runSummaries = []ProjectRunSummary{{
		ProjectID:                 uuid.New(),
		Name:                      "x",
		Status:                    ProjectStatusRunning,
		FailedCount:               3, // includes blocked in real SQL
		WaitingHumanCount:         19,
		WaitingHumanUnlinkedCount: 2,
		OpenDecisionCount:         18,
	}}
	list, err := svc.ListProjectRunSummaries(context.Background(), ListProjectRunSummariesRequest{
		TenantID: tenantID, Limit: 50,
	})
	require.NoError(t, err)
	require.Len(t, list.Items, 1)
	require.Equal(t, int32(3), list.Items[0].FailedCount)
	require.Equal(t, int32(19), list.Items[0].WaitingHumanCount)
	require.Equal(t, int32(2), list.Items[0].WaitingHumanUnlinkedCount)
}
