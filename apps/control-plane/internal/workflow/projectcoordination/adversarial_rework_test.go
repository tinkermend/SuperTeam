package projectcoordination

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/superteam/control-plane/internal/project"
)

func TestSynthesizeAdversarialRevisionFromJudgements(t *testing.T) {
	criterionID := "crit-login-secure"
	statement := "登录流程必须防重放"
	judgements := []project.DemandAdversarialJudgement{
		{Lens: "security", Verdict: AdversarialVerdictRefuted, Reason: "缺少 nonce 校验"},
		{Lens: "robustness", Verdict: AdversarialVerdictRefuted, Reason: "并发登录未加锁"},
		{Lens: "clarity", Verdict: AdversarialVerdictAccepted, Reason: "文档清晰"},
	}

	rev := synthesizeAdversarialRevision(criterionID, statement, judgements)

	require.Contains(t, rev.Reason, "证伪")
	require.Contains(t, rev.Reason, "2/3")
	require.Contains(t, rev.Reason, statement)
	require.Equal(t, []string{
		"security: 缺少 nonce 校验",
		"robustness: 并发登录未加锁",
	}, rev.RequestedChanges)
	// The accepted lens's reason must NOT leak into the rework input.
	for _, change := range rev.RequestedChanges {
		require.NotContains(t, change, "文档清晰")
	}
}

func TestCreateReworkTaskFromAdversarialCarriesReasons(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	demandID := uuid.New()
	planRevisionID := uuid.New()
	coordinationJobID := uuid.New()
	routeDecisionID := uuid.New()
	reviewedTaskID := uuid.New()
	resultID := uuid.New()
	employeeID := uuid.New()
	criterionID := "crit-login-secure"
	maxAttempts := int32(3)

	repo := &projectStoreMemoryRepository{
		tasks: []project.ProjectTask{{
			ID:                        reviewedTaskID,
			TenantID:                  tenantID,
			ProjectID:                 projectID,
			DemandID:                  &demandID,
			CoordinationJobID:         &coordinationJobID,
			RouteDecisionID:           &routeDecisionID,
			Title:                     "Implement login",
			Summary:                   strPtr("Wire redirect flow"),
			Status:                    project.ProjectTaskStatusCompleted,
			AttemptCount:              1,
			MaxAttempts:               &maxAttempts,
			AssignedDigitalEmployeeID: &employeeID,
			TaskKind:                  strPtr("implementation"),
			RiskLevel:                 strPtr("medium"),
			ExpectedOutputs:           []any{"patch", "test_evidence"},
			InputRequirements:         map[string]any{"existing": "context"},
			PlannerMetadata:           map[string]any{"iteration_key": "wi-login"},
			HandoffContract:           map[string]any{"completion_path": "project_task_attempt_writeback"},
			LatestTaskResultID:        &resultID,
		}},
		projectTaskResults: []project.ProjectTaskResult{{
			ID:            resultID,
			TenantID:      tenantID,
			ProjectID:     projectID,
			ProjectTaskID: reviewedTaskID,
			ResultStatus:  project.TaskResultStatusCompleted,
			Decision:      project.TaskResultDecisionCompleteAccepted,
			Contract: project.TaskResultContract{
				Status:  project.TaskResultStatusCompleted,
				Summary: "login implemented",
			},
		}},
		adversarialJudgements: []project.DemandAdversarialJudgement{
			{
				TenantID: tenantID, ProjectID: projectID, DemandID: demandID, PlanRevisionID: planRevisionID,
				CriterionID: criterionID, ReviewedTaskID: reviewedTaskID,
				Lens: "security", Verdict: AdversarialVerdictRefuted, Reason: "缺少 nonce 校验",
			},
			{
				TenantID: tenantID, ProjectID: projectID, DemandID: demandID, PlanRevisionID: planRevisionID,
				CriterionID: criterionID, ReviewedTaskID: reviewedTaskID,
				Lens: "robustness", Verdict: AdversarialVerdictRefuted, Reason: "并发登录未加锁",
			},
			{
				TenantID: tenantID, ProjectID: projectID, DemandID: demandID, PlanRevisionID: planRevisionID,
				CriterionID: criterionID, ReviewedTaskID: reviewedTaskID,
				Lens: "clarity", Verdict: AdversarialVerdictAccepted, Reason: "文档清晰",
			},
			// A judgement for a DIFFERENT criterion must be filtered out.
			{
				TenantID: tenantID, ProjectID: projectID, DemandID: demandID, PlanRevisionID: planRevisionID,
				CriterionID: "crit-other", ReviewedTaskID: reviewedTaskID,
				Lens: "security", Verdict: AdversarialVerdictRefuted, Reason: "不相关的驳斥",
			},
		},
	}
	store := NewProjectStore(repo)

	created, err := store.CreateReworkTaskFromAdversarial(context.Background(), CreateReworkTaskFromAdversarialInput{
		TenantID:           tenantID,
		ProjectID:          projectID,
		ReviewedTaskID:     reviewedTaskID,
		CriterionID:        criterionID,
		CriterionStatement: "登录流程必须防重放",
		DemandID:           demandID,
		PlanRevisionID:     planRevisionID,
	})

	require.NoError(t, err)
	require.False(t, created.Exhausted)
	require.NotEqual(t, uuid.Nil, created.TaskID)

	rework := repo.mustTask(created.TaskID)
	require.Equal(t, project.ProjectTaskStatusPlanned, rework.Status)
	require.Equal(t, &reviewedTaskID, rework.RevisionOfTaskID)

	// The judges' refutation reasons are fed back as the rework input.
	require.Contains(t, rework.InputRequirements["revision_reason"], "证伪")
	require.Contains(t, rework.InputRequirements["revision_reason"], "登录流程必须防重放")
	require.Equal(t, []string{
		"security: 缺少 nonce 校验",
		"robustness: 并发登录未加锁",
	}, rework.InputRequirements["requested_changes"])
	require.Equal(t, reviewedTaskID.String(), rework.InputRequirements["source_task_id"])
	require.Equal(t, resultID.String(), rework.InputRequirements["source_result_id"])

	// Revision lineage metadata: attempt count incremented off the source.
	require.Equal(t, int32(2), rework.PlannerMetadata["revision_attempt_count"])
	require.Equal(t, reviewedTaskID.String(), rework.PlannerMetadata["revision_root_task_id"])

	// The latest result is linked to the rework task for lineage.
	require.Equal(t, &created.TaskID, repo.projectTaskResults[0].RevisionTaskID)
}

func TestCreateReworkExhaustedReturnsNoTask(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	demandID := uuid.New()
	planRevisionID := uuid.New()
	coordinationJobID := uuid.New()
	reviewedTaskID := uuid.New()

	repo := &projectStoreMemoryRepository{
		tasks: []project.ProjectTask{{
			ID:                reviewedTaskID,
			TenantID:          tenantID,
			ProjectID:         projectID,
			DemandID:          &demandID,
			CoordinationJobID: &coordinationJobID,
			Title:             "Implement login",
			Status:            project.ProjectTaskStatusCompleted,
			PlannerMetadata: map[string]any{
				"revision_attempt_count": int32(3),
				"revision_max_attempts":  int32(3),
			},
		}},
		adversarialJudgements: []project.DemandAdversarialJudgement{{
			TenantID: tenantID, ProjectID: projectID, DemandID: demandID, PlanRevisionID: planRevisionID,
			CriterionID: "crit-login-secure", ReviewedTaskID: reviewedTaskID,
			Lens: "security", Verdict: AdversarialVerdictRefuted, Reason: "缺少 nonce 校验",
		}},
	}
	store := NewProjectStore(repo)

	created, err := store.CreateReworkTaskFromAdversarial(context.Background(), CreateReworkTaskFromAdversarialInput{
		TenantID:           tenantID,
		ProjectID:          projectID,
		ReviewedTaskID:     reviewedTaskID,
		CriterionID:        "crit-login-secure",
		CriterionStatement: "登录流程必须防重放",
		DemandID:           demandID,
		PlanRevisionID:     planRevisionID,
	})

	require.NoError(t, err)
	require.True(t, created.Exhausted)
	require.Equal(t, uuid.Nil, created.TaskID)
	require.Empty(t, repo.projectTaskRequests, "no rework task must be created when budget is exhausted")
}
