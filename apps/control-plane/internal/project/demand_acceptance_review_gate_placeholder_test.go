package project

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// TestReviewGatePlaceholderWrittenOnCompletion (P1.1 race fix): the completion
// path must synchronously write a `pending` review_gate aggregate row for every
// review_gate criterion the completing task satisfies — BEFORE the writeback
// recompute runs — so the convergence gate holds the demand until the
// asynchronous detector concludes. Non-review_gate criteria on the same task
// must get no placeholder.
func TestReviewGatePlaceholderWrittenOnCompletion(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	require.NoError(t, err)

	tenantID, projectID := uuid.New(), uuid.New()
	demandID, revisionID := uuid.New(), uuid.New()
	taskKey := "impl_task"

	rgCriterion := DemandAcceptanceCriterion{
		TenantID: tenantID, ProjectID: projectID, DemandID: demandID, PlanRevisionID: revisionID,
		CriterionID:        "crit_rg",
		Statement:          "交付未引入安全违规",
		VerificationMethod: demandCriterionVerificationMethodReviewGate,
		Severity:           demandAcceptanceCriterionSeverityBlocking,
		SatisfiedBy:        []string{taskKey},
	}
	atCriterion := DemandAcceptanceCriterion{
		TenantID: tenantID, ProjectID: projectID, DemandID: demandID, PlanRevisionID: revisionID,
		CriterionID:        "crit_at",
		Statement:          "单元测试全部通过",
		VerificationMethod: "automated_test",
		Severity:           demandAcceptanceCriterionSeverityBlocking,
		SatisfiedBy:        []string{taskKey},
	}
	// A review_gate criterion satisfied_by a DIFFERENT task must not get a
	// placeholder from this task's completion.
	otherRgCriterion := DemandAcceptanceCriterion{
		TenantID: tenantID, ProjectID: projectID, DemandID: demandID, PlanRevisionID: revisionID,
		CriterionID:        "crit_rg_other",
		Statement:          "另一个任务的检测门",
		VerificationMethod: demandCriterionVerificationMethodReviewGate,
		Severity:           demandAcceptanceCriterionSeverityBlocking,
		SatisfiedBy:        []string{"other_task"},
	}
	repo.demandAcceptanceCriteria = append(repo.demandAcceptanceCriteria, rgCriterion, atCriterion, otherRgCriterion)

	key := taskKey
	task := ProjectTask{
		ID:                     uuid.New(),
		TenantID:               tenantID,
		ProjectID:              projectID,
		DemandID:               &demandID,
		AcceptedPlanRevisionID: &revisionID,
		PlannedTaskKey:         &key,
	}

	require.NoError(t, service.projectReviewGatePlaceholderVerdicts(context.Background(), task))

	verdicts, err := repo.ListDemandCriterionVerdicts(context.Background(), tenantID, demandID, revisionID)
	require.NoError(t, err)
	require.Len(t, verdicts, 1, "exactly the satisfied-by review_gate criterion gets a placeholder")
	placeholder := verdicts[0]
	require.Equal(t, "crit_rg", placeholder.CriterionID)
	require.Equal(t, demandCriterionVerdictReviewGatePending, placeholder.Verdict)
	require.Equal(t, demandCriterionJudgeTypeReviewGate, placeholder.JudgeType)
	require.Nil(t, placeholder.ProjectTaskID)

	// The placeholder must HOLD the convergence gate for its criterion (the
	// other review_gate criterion, verdict-less, still default-releases).
	pending := ResolveUnsatisfiedBlockingCriteria(
		[]DemandAcceptanceCriterion{rgCriterion, otherRgCriterion}, verdicts)
	require.Equal(t, []string{"crit_rg"}, pending)
}

// TestReviewGatePlaceholderOverwritesPriorVerdict: re-completion (task retry)
// deliberately resets a previous round's real verdict back to `pending` — a new
// artifact means a new detection round, and holding until the detector
// concludes is the conservative direction.
func TestReviewGatePlaceholderOverwritesPriorVerdict(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	require.NoError(t, err)

	tenantID, projectID := uuid.New(), uuid.New()
	demandID, revisionID := uuid.New(), uuid.New()
	taskKey := "impl_task"

	criterion := DemandAcceptanceCriterion{
		TenantID: tenantID, ProjectID: projectID, DemandID: demandID, PlanRevisionID: revisionID,
		CriterionID:        "crit_rg",
		Statement:          "交付未引入安全违规",
		VerificationMethod: demandCriterionVerificationMethodReviewGate,
		Severity:           demandAcceptanceCriterionSeverityBlocking,
		SatisfiedBy:        []string{taskKey},
	}
	repo.demandAcceptanceCriteria = append(repo.demandAcceptanceCriteria, criterion)

	// Round 1: detector already concluded satisfied.
	require.NoError(t, repo.CreateReviewGateVerdict(context.Background(), CreateReviewGateVerdictRequest{
		TenantID: tenantID, ProjectID: projectID, DemandID: demandID, PlanRevisionID: revisionID,
		CriterionID: "crit_rg", Verdict: "satisfied", Reason: "检测门无命中：默认放行",
	}))

	key := taskKey
	task := ProjectTask{
		ID:                     uuid.New(),
		TenantID:               tenantID,
		ProjectID:              projectID,
		DemandID:               &demandID,
		AcceptedPlanRevisionID: &revisionID,
		PlannedTaskKey:         &key,
	}
	require.NoError(t, service.projectReviewGatePlaceholderVerdicts(context.Background(), task))

	verdicts, err := repo.ListDemandCriterionVerdicts(context.Background(), tenantID, demandID, revisionID)
	require.NoError(t, err)
	require.Len(t, verdicts, 1, "upsert must overwrite the aggregate row, not add a second one")
	require.Equal(t, demandCriterionVerdictReviewGatePending, verdicts[0].Verdict)
}
