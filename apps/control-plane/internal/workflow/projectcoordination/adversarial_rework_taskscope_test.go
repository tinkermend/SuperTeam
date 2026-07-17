package projectcoordination

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/superteam/control-plane/internal/project"
)

// TestReworkFromAdversarialAggregatesHeldCriteria: a reviewed task held on TWO
// adversarial criteria produces ONE rework task whose requested_changes is the
// UNION of both criteria's refuted-lens reasons and whose revision_reason carries
// both criterion statements. Task-scoped merge (Phase C1 Task 4).
func TestReworkFromAdversarialAggregatesHeldCriteria(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	demandID := uuid.New()
	planRevisionID := uuid.New()
	coordinationJobID := uuid.New()
	reviewedTaskID := uuid.New()
	resultID := uuid.New()
	employeeID := uuid.New()
	maxAttempts := int32(3)

	repo := &projectStoreMemoryRepository{
		tasks: []project.ProjectTask{{
			ID:                        reviewedTaskID,
			TenantID:                  tenantID,
			ProjectID:                 projectID,
			DemandID:                  &demandID,
			CoordinationJobID:         &coordinationJobID,
			Title:                     "Implement login",
			Summary:                   strPtr("Wire redirect flow"),
			Status:                    project.ProjectTaskStatusCompleted,
			AttemptCount:              1,
			MaxAttempts:               &maxAttempts,
			AssignedDigitalEmployeeID: &employeeID,
			PlannerMetadata:           map[string]any{"iteration_key": "wi-login"},
			LatestTaskResultID:        &resultID,
		}},
		projectTaskResults: []project.ProjectTaskResult{{
			ID:            resultID,
			TenantID:      tenantID,
			ProjectID:     projectID,
			ProjectTaskID: reviewedTaskID,
			ResultStatus:  project.TaskResultStatusCompleted,
			Decision:      project.TaskResultDecisionCompleteAccepted,
			Contract:      project.TaskResultContract{Status: project.TaskResultStatusCompleted, Summary: "login implemented"},
		}},
		adversarialJudgements: []project.DemandAdversarialJudgement{
			{
				TenantID: tenantID, ProjectID: projectID, DemandID: demandID, PlanRevisionID: planRevisionID,
				CriterionID: "crit-secure", ReviewedTaskID: reviewedTaskID,
				Lens: "security", Verdict: AdversarialVerdictRefuted, Reason: "缺少 nonce 校验",
			},
			{
				TenantID: tenantID, ProjectID: projectID, DemandID: demandID, PlanRevisionID: planRevisionID,
				CriterionID: "crit-secure", ReviewedTaskID: reviewedTaskID,
				Lens: "clarity", Verdict: AdversarialVerdictAccepted, Reason: "文档清晰",
			},
			{
				TenantID: tenantID, ProjectID: projectID, DemandID: demandID, PlanRevisionID: planRevisionID,
				CriterionID: "crit-perf", ReviewedTaskID: reviewedTaskID,
				Lens: "robustness", Verdict: AdversarialVerdictRefuted, Reason: "并发登录未加锁",
			},
		},
	}
	store := NewProjectStore(repo)

	created, err := store.CreateReworkTaskFromAdversarial(context.Background(), CreateReworkTaskFromAdversarialInput{
		TenantID:       tenantID,
		ProjectID:      projectID,
		ReviewedTaskID: reviewedTaskID,
		DemandID:       demandID,
		PlanRevisionID: planRevisionID,
		HeldCriteria: []HeldAdversarialCriterion{
			{CriterionID: "crit-secure", Statement: "登录流程必须防重放"},
			{CriterionID: "crit-perf", Statement: "登录接口 p95 低于 200ms"},
		},
	})
	require.NoError(t, err)
	require.False(t, created.Exhausted)
	require.NotEqual(t, uuid.Nil, created.TaskID)

	// Exactly ONE rework task merges both held criteria.
	require.Len(t, repo.projectTaskRequests, 1, "two held criteria must yield ONE rework task")

	rework := repo.mustTask(created.TaskID)
	// The union of both criteria's REFUTED-lens reasons (accepted lens excluded).
	require.Equal(t, []string{
		"security: 缺少 nonce 校验",
		"robustness: 并发登录未加锁",
	}, rework.InputRequirements["requested_changes"])
	// The combined reason carries BOTH criterion statements.
	reason, _ := rework.InputRequirements["revision_reason"].(string)
	require.Contains(t, reason, "登录流程必须防重放")
	require.Contains(t, reason, "登录接口 p95 低于 200ms")
}

// TestReworkFromAdversarialDeterministicKeyOnRetry: when the reviewed task has no
// execution result yet, two invocations with identical inputs must yield the SAME
// PlannedTaskKey (deterministic seed from reviewed-task-id + plan-revision), so a
// Temporal retry never spawns a duplicate rework. Idempotency (Task 4 step 3).
func TestReworkFromAdversarialDeterministicKeyOnRetry(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	demandID := uuid.New()
	planRevisionID := uuid.New()
	coordinationJobID := uuid.New()
	reviewedTaskID := uuid.New()

	newRepo := func() *projectStoreMemoryRepository {
		return &projectStoreMemoryRepository{
			tasks: []project.ProjectTask{{
				ID:                reviewedTaskID,
				TenantID:          tenantID,
				ProjectID:         projectID,
				DemandID:          &demandID,
				CoordinationJobID: &coordinationJobID,
				Title:             "Implement login",
				Status:            project.ProjectTaskStatusCompleted,
				PlannerMetadata:   map[string]any{"iteration_key": "wi-login"},
				// NO LatestTaskResultID → latestTaskResult returns nil → deterministic seed path.
			}},
			adversarialJudgements: []project.DemandAdversarialJudgement{{
				TenantID: tenantID, ProjectID: projectID, DemandID: demandID, PlanRevisionID: planRevisionID,
				CriterionID: "crit-secure", ReviewedTaskID: reviewedTaskID,
				Lens: "security", Verdict: AdversarialVerdictRefuted, Reason: "缺少 nonce 校验",
			}},
		}
	}

	input := CreateReworkTaskFromAdversarialInput{
		TenantID:       tenantID,
		ProjectID:      projectID,
		ReviewedTaskID: reviewedTaskID,
		DemandID:       demandID,
		PlanRevisionID: planRevisionID,
		HeldCriteria:   []HeldAdversarialCriterion{{CriterionID: "crit-secure", Statement: "登录流程必须防重放"}},
	}

	repoA := newRepo()
	createdA, err := NewProjectStore(repoA).CreateReworkTaskFromAdversarial(context.Background(), input)
	require.NoError(t, err)
	require.Len(t, repoA.projectTaskRequests, 1)
	keyA := repoA.projectTaskRequests[0].PlannedTaskKey
	require.NotNil(t, keyA)

	repoB := newRepo()
	createdB, err := NewProjectStore(repoB).CreateReworkTaskFromAdversarial(context.Background(), input)
	require.NoError(t, err)
	require.Len(t, repoB.projectTaskRequests, 1)
	keyB := repoB.projectTaskRequests[0].PlannedTaskKey
	require.NotNil(t, keyB)

	require.Equal(t, *keyA, *keyB, "identical inputs must derive the SAME deterministic planned key")
	require.NotEqual(t, uuid.Nil, createdA.TaskID)
	require.NotEqual(t, uuid.Nil, createdB.TaskID)
}
