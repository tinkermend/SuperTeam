package project

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// seedReviewGateVerdict writes the review_gate aggregate row (judge_type
// "review_gate", project_task_id nil) for a criterion on the fixture's plan
// revision — the detector-projected hold the human final acceptance must be able
// to waive/confirm.
func seedReviewGateVerdict(t *testing.T, repo *memoryRepository, f demandAcceptanceSignFixture, criterionID, verdict string) {
	t.Helper()
	if err := repo.CreateReviewGateVerdict(context.Background(), CreateReviewGateVerdictRequest{
		TenantID:       f.tenantID,
		ProjectID:      f.projectID,
		DemandID:       f.demandID,
		PlanRevisionID: f.revisionID,
		CriterionID:    criterionID,
		Verdict:        verdict,
		Reason:         "检测门 HOLD（动作档 block）",
	}); err != nil {
		t.Fatalf("seed review_gate verdict: %v", err)
	}
}

// TestHumanCanSignReviewGate: a review_gate blocking criterion held unsatisfied by
// the detector gate is resolvable by a human at final acceptance. The human
// "satisfied" verdict wins under criterionEffectiveVerdict's human precedence and
// releases the demand — SignDemandCriterionVerdict must NOT reject the review_gate
// method with ErrInvalidProject (the review-flagged allow-list addition).
func TestHumanCanSignReviewGate(t *testing.T) {
	repo := newMemoryRepository()
	approvals := &fakeApprovalResolver{}
	inbox := &fakeDecisionInboxProjector{}
	service, err := NewServiceWithCoordinatorApprovalsInboxAndArchiveArtifactLocker(repo, NoopCoordinatorSignalClient{}, approvals, inbox, nil)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	f := setupDemandAcceptanceSignFixture(repo, []DemandAcceptanceCriterion{
		reviewGateCriterion("c1"),
	})
	seedReviewGateVerdict(t, repo, f, "c1", demandCriterionVerdictUnsatisfied)

	// Precondition: the review_gate unsatisfied verdict holds the gate.
	preCriteria, _ := repo.ListDemandAcceptanceCriteria(context.Background(), f.tenantID, f.demandID, f.revisionID)
	preVerdicts, _ := repo.ListDemandCriterionVerdicts(context.Background(), f.tenantID, f.demandID, f.revisionID)
	require.Equal(t, []string{"c1"}, ResolveUnsatisfiedBlockingCriteria(preCriteria, preVerdicts))

	result, err := service.SignDemandCriterionVerdict(context.Background(), SignDemandCriterionVerdictRequest{
		TenantID:    f.tenantID,
		DemandID:    f.demandID,
		ActorUserID: f.ownerID,
		CriterionID: "c1",
		Verdict:     demandCriterionVerdictSatisfied,
		Reason:      "人类最终验收：已复核检测命中并接受",
	})
	require.NoError(t, err)
	require.Equal(t, ProjectDemandStatusCompleted, result.DemandStatus)

	// Human verdict persisted and won precedence; the gate no longer holds c1.
	postVerdicts, _ := repo.ListDemandCriterionVerdicts(context.Background(), f.tenantID, f.demandID, f.revisionID)
	human := findHumanCriterionVerdict(postVerdicts, "c1")
	require.NotNil(t, human)
	require.Equal(t, demandCriterionVerdictSatisfied, human.Verdict)
	verdict, judgeType, _, ok := criterionEffectiveVerdict(postVerdicts, "c1")
	require.True(t, ok)
	require.Equal(t, demandCriterionVerdictSatisfied, verdict)
	require.Equal(t, demandCriterionJudgeTypeHuman, judgeType)
	require.Empty(t, ResolveUnsatisfiedBlockingCriteria(preCriteria, postVerdicts))
}

// TestHumanSignRejectsAutomatedTest (regression guard for the broadened allow-list):
// review_gate joins human_judgment and adversarial_review as human-signable, but a
// blocking automated_test criterion still returns ErrInvalidProject.
func TestHumanSignRejectsAutomatedTest(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	f := setupDemandAcceptanceSignFixture(repo, []DemandAcceptanceCriterion{
		automatedTestCriterion("c1"),
	})

	_, err = service.SignDemandCriterionVerdict(context.Background(), SignDemandCriterionVerdictRequest{
		TenantID:    f.tenantID,
		DemandID:    f.demandID,
		ActorUserID: f.ownerID,
		CriterionID: "c1",
		Verdict:     demandCriterionVerdictSatisfied,
	})
	require.ErrorIs(t, err, ErrInvalidProject)
}
