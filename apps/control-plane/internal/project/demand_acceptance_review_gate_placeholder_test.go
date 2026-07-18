package project

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func placeholderFixtureCriterion(tenantID, projectID, demandID, revisionID uuid.UUID, id, method, satisfiedBy string) DemandAcceptanceCriterion {
	return DemandAcceptanceCriterion{
		TenantID: tenantID, ProjectID: projectID, DemandID: demandID, PlanRevisionID: revisionID,
		CriterionID:        id,
		Statement:          "判据 " + id,
		VerificationMethod: method,
		Severity:           demandAcceptanceCriterionSeverityBlocking,
		SatisfiedBy:        []string{satisfiedBy},
	}
}

// TestReviewGatePlaceholderRequestsScopedToTask (P1.1 race fix): the completion
// path builds a `pending` review_gate placeholder request for every review_gate
// criterion the completing task satisfies — and ONLY those (other tasks'
// criteria and non-review_gate methods are excluded). The requests are handed
// to the writeback so they commit in the same transaction as the completion,
// before its demand-status recompute.
func TestReviewGatePlaceholderRequestsScopedToTask(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	require.NoError(t, err)

	tenantID, projectID := uuid.New(), uuid.New()
	demandID, revisionID := uuid.New(), uuid.New()
	taskKey := "impl_task"

	rg := placeholderFixtureCriterion(tenantID, projectID, demandID, revisionID, "crit_rg", demandCriterionVerificationMethodReviewGate, taskKey)
	at := placeholderFixtureCriterion(tenantID, projectID, demandID, revisionID, "crit_at", "automated_test", taskKey)
	otherRg := placeholderFixtureCriterion(tenantID, projectID, demandID, revisionID, "crit_rg_other", demandCriterionVerificationMethodReviewGate, "other_task")
	repo.demandAcceptanceCriteria = append(repo.demandAcceptanceCriteria, rg, at, otherRg)

	key := taskKey
	task := ProjectTask{
		ID:                     uuid.New(),
		TenantID:               tenantID,
		ProjectID:              projectID,
		DemandID:               &demandID,
		AcceptedPlanRevisionID: &revisionID,
		PlannedTaskKey:         &key,
	}

	requests, err := service.reviewGatePlaceholderVerdictRequests(context.Background(), task)
	require.NoError(t, err)
	require.Len(t, requests, 1, "exactly the satisfied-by review_gate criterion gets a placeholder request")
	require.Equal(t, "crit_rg", requests[0].CriterionID)
	require.Equal(t, demandCriterionVerdictReviewGatePending, requests[0].Verdict)

	// Committed through the writeback (memory fake mirrors PgRepository), the
	// placeholder HOLDs the gate for its criterion; the other review_gate
	// criterion, verdict-less, still default-releases.
	for _, req := range requests {
		require.NoError(t, repo.CreateReviewGateVerdict(context.Background(), req))
	}
	verdicts, err := repo.ListDemandCriterionVerdicts(context.Background(), tenantID, demandID, revisionID)
	require.NoError(t, err)
	require.Len(t, verdicts, 1)
	require.Equal(t, demandCriterionJudgeTypeReviewGate, verdicts[0].JudgeType)
	require.Nil(t, verdicts[0].ProjectTaskID)
	pending := ResolveUnsatisfiedBlockingCriteria([]DemandAcceptanceCriterion{rg, otherRg}, verdicts)
	require.Equal(t, []string{"crit_rg"}, pending)
}

// TestReviewGatePlaceholderMatchesRevisionRootKey: a revision/rework task
// carries a derived planned key while the criteria's SatisfiedBy names the
// revision-root's key — the placeholder scope must match via the root key (the
// same identity rule the detector trigger applies), otherwise every revision
// completion skips the placeholder and reopens the race on exactly the
// revision path.
func TestReviewGatePlaceholderMatchesRevisionRootKey(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	require.NoError(t, err)

	tenantID, projectID := uuid.New(), uuid.New()
	demandID, revisionID := uuid.New(), uuid.New()
	rootKey := "impl_task"

	criterion := placeholderFixtureCriterion(tenantID, projectID, demandID, revisionID, "crit_rg", demandCriterionVerificationMethodReviewGate, rootKey)
	repo.demandAcceptanceCriteria = append(repo.demandAcceptanceCriteria, criterion)

	rootTaskKey := rootKey
	rootTask := ProjectTask{
		ID:             uuid.New(),
		TenantID:       tenantID,
		ProjectID:      projectID,
		DemandID:       &demandID,
		PlannedTaskKey: &rootTaskKey,
	}
	repo.tasks = append(repo.tasks, rootTask)

	revisionKey := rootKey + "#revision-1"
	rootID := rootTask.ID
	revisionTask := ProjectTask{
		ID:                     uuid.New(),
		TenantID:               tenantID,
		ProjectID:              projectID,
		DemandID:               &demandID,
		AcceptedPlanRevisionID: &revisionID,
		PlannedTaskKey:         &revisionKey,
		RevisionOfTaskID:       &rootID,
	}

	requests, err := service.reviewGatePlaceholderVerdictRequests(context.Background(), revisionTask)
	require.NoError(t, err)
	require.Len(t, requests, 1, "revision task must place the placeholder via its revision-root key")
	require.Equal(t, "crit_rg", requests[0].CriterionID)
	require.Equal(t, demandCriterionVerdictReviewGatePending, requests[0].Verdict)
}

// TestReviewGatePlaceholderOverwritesPriorVerdict: re-completion (task retry /
// new revision round) deliberately resets a previous round's real verdict back
// to `pending` — a new artifact means a new detection round, and holding until
// the detector concludes is the conservative direction.
func TestReviewGatePlaceholderOverwritesPriorVerdict(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	require.NoError(t, err)

	tenantID, projectID := uuid.New(), uuid.New()
	demandID, revisionID := uuid.New(), uuid.New()
	taskKey := "impl_task"

	criterion := placeholderFixtureCriterion(tenantID, projectID, demandID, revisionID, "crit_rg", demandCriterionVerificationMethodReviewGate, taskKey)
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
	requests, err := service.reviewGatePlaceholderVerdictRequests(context.Background(), task)
	require.NoError(t, err)
	require.Len(t, requests, 1)
	for _, req := range requests {
		require.NoError(t, repo.CreateReviewGateVerdict(context.Background(), req))
	}

	verdicts, err := repo.ListDemandCriterionVerdicts(context.Background(), tenantID, demandID, revisionID)
	require.NoError(t, err)
	require.Len(t, verdicts, 1, "upsert must overwrite the aggregate row, not add a second one")
	require.Equal(t, demandCriterionVerdictReviewGatePending, verdicts[0].Verdict)
}
