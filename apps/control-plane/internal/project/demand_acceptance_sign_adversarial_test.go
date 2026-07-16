package project

import (
	"context"
	"errors"
	"testing"
)

// seedAdversarialVerdict writes the adversarial aggregate row (judge_type
// "adversarial", project_task_id nil) for a criterion on the fixture's plan
// revision — the tier-2 outcome the human tier-3 sign-off must be able to
// override.
func seedAdversarialVerdict(t *testing.T, repo *memoryRepository, f demandAcceptanceSignFixture, criterionID, verdict string) {
	t.Helper()
	if err := repo.CreateAdversarialVerdict(context.Background(), CreateAdversarialVerdictRequest{
		TenantID:       f.tenantID,
		ProjectID:      f.projectID,
		DemandID:       f.demandID,
		PlanRevisionID: f.revisionID,
		CriterionID:    criterionID,
		Verdict:        verdict,
		Reason:         "2/3 判官证伪",
	}); err != nil {
		t.Fatalf("seed adversarial verdict: %v", err)
	}
}

// TestHumanSignsAdversarialReviewCriterionOverridesHold: an adversarial_review
// blocking criterion held unsatisfied by the tier-2 judges is resolvable by a
// human tier-3 sign-off. The human "satisfied" verdict coexists with the
// adversarial row (migration 068 disjoint indexes), wins under
// criterionEffectiveVerdict's human precedence, and the convergence gate
// releases the demand to completed.
func TestHumanSignsAdversarialReviewCriterionOverridesHold(t *testing.T) {
	repo := newMemoryRepository()
	approvals := &fakeApprovalResolver{}
	inbox := &fakeDecisionInboxProjector{}
	service, err := NewServiceWithCoordinatorApprovalsInboxAndArchiveArtifactLocker(repo, NoopCoordinatorSignalClient{}, approvals, inbox, nil)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	f := setupDemandAcceptanceSignFixture(repo, []DemandAcceptanceCriterion{
		adversarialCriterion("c1"),
	})
	seedAdversarialVerdict(t, repo, f, "c1", demandCriterionVerdictUnsatisfied)

	// Precondition: the adversarial unsatisfied verdict holds the gate — c1 is
	// pending and its effective verdict is the tier-2 adversarial unsatisfied.
	preCriteria, _ := repo.ListDemandAcceptanceCriteria(context.Background(), f.tenantID, f.demandID, f.revisionID)
	preVerdicts, _ := repo.ListDemandCriterionVerdicts(context.Background(), f.tenantID, f.demandID, f.revisionID)
	if got := ResolveUnsatisfiedBlockingCriteria(preCriteria, preVerdicts); len(got) != 1 || got[0] != "c1" {
		t.Fatalf("expected c1 held pending before sign, got %v", got)
	}
	if v, jt, _, ok := criterionEffectiveVerdict(preVerdicts, "c1"); !ok || v != demandCriterionVerdictUnsatisfied || jt != demandCriterionJudgeTypeAdversarial {
		t.Fatalf("expected adversarial unsatisfied effective verdict before sign, got v=%q jt=%q ok=%v", v, jt, ok)
	}

	result, err := service.SignDemandCriterionVerdict(context.Background(), SignDemandCriterionVerdictRequest{
		TenantID:    f.tenantID,
		DemandID:    f.demandID,
		ActorUserID: f.ownerID,
		CriterionID: "c1",
		Verdict:     demandCriterionVerdictSatisfied,
		Reason:      "人类档3覆盖：已复核证据接受",
	})
	if err != nil {
		t.Fatalf("sign adversarial criterion: %v", err)
	}
	if result.DemandStatus != ProjectDemandStatusCompleted {
		t.Fatalf("expected demand completed after human override, got %s", result.DemandStatus)
	}

	// The human verdict row persisted alongside the adversarial row.
	postVerdicts, _ := repo.ListDemandCriterionVerdicts(context.Background(), f.tenantID, f.demandID, f.revisionID)
	human := findHumanCriterionVerdict(postVerdicts, "c1")
	if human == nil {
		t.Fatalf("expected persisted human verdict row for c1, verdicts=%#v", postVerdicts)
	}
	if human.Verdict != demandCriterionVerdictSatisfied || human.JudgeID != f.ownerID {
		t.Fatalf("unexpected human verdict row: %#v", human)
	}
	var adversarialRows int
	for _, v := range postVerdicts {
		if v.CriterionID == "c1" && v.JudgeType == demandCriterionJudgeTypeAdversarial {
			adversarialRows++
		}
	}
	if adversarialRows != 1 {
		t.Fatalf("expected adversarial row to coexist with human row, adversarial rows=%d", adversarialRows)
	}

	// Human precedence: the criterion's effective verdict is now human satisfied,
	// so the gate no longer holds it.
	if v, jt, _, ok := criterionEffectiveVerdict(postVerdicts, "c1"); !ok || v != demandCriterionVerdictSatisfied || jt != demandCriterionJudgeTypeHuman {
		t.Fatalf("expected human satisfied effective verdict after sign, got v=%q jt=%q ok=%v", v, jt, ok)
	}
	if got := ResolveUnsatisfiedBlockingCriteria(preCriteria, postVerdicts); len(got) != 0 {
		t.Fatalf("expected no pending criteria after human override, got %v", got)
	}

	// The demand actually advanced in the repository, and the pending decision
	// resolved approved — convergence, not just a nil return.
	demand, _ := repo.GetProjectDemand(context.Background(), f.tenantID, f.demandID)
	if demand.Status != ProjectDemandStatusCompleted {
		t.Fatalf("expected repository demand completed, got %s", demand.Status)
	}
	if approvals.calls != 1 || approvals.last.ApprovalRequestID != f.approvalID || approvals.last.Decision != "approved" {
		t.Fatalf("expected approval resolved approved, got calls=%d last=%#v", approvals.calls, approvals.last)
	}
	decision, _ := repo.GetDecisionRequest(context.Background(), f.tenantID, f.projectID, f.decisionID)
	if decision.StatusSnapshot != "approved" {
		t.Fatalf("expected decision resolved approved, got %s", decision.StatusSnapshot)
	}
}

// TestHumanSignsAdversarialEscalateHumanOverridesHold: the escalate_human path
// (budget exhaustion / engine error) is not a dead end — a human tier-3 sign-off
// overrides the escalate_human aggregate and releases the demand. Mechanically
// identical to the unsatisfied override (human precedence), but asserts the
// escalate_human value specifically is overridable.
func TestHumanSignsAdversarialEscalateHumanOverridesHold(t *testing.T) {
	repo := newMemoryRepository()
	approvals := &fakeApprovalResolver{}
	inbox := &fakeDecisionInboxProjector{}
	service, err := NewServiceWithCoordinatorApprovalsInboxAndArchiveArtifactLocker(repo, NoopCoordinatorSignalClient{}, approvals, inbox, nil)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	f := setupDemandAcceptanceSignFixture(repo, []DemandAcceptanceCriterion{
		adversarialCriterion("c1"),
	})
	// escalate_human: budget exhausted / engine error — the tier-2 judges could
	// not conclude, so the gate holds pending until a human resolves it.
	seedAdversarialVerdict(t, repo, f, "c1", "escalate_human")

	preCriteria, _ := repo.ListDemandAcceptanceCriteria(context.Background(), f.tenantID, f.demandID, f.revisionID)
	preVerdicts, _ := repo.ListDemandCriterionVerdicts(context.Background(), f.tenantID, f.demandID, f.revisionID)
	if v, _, _, ok := criterionEffectiveVerdict(preVerdicts, "c1"); !ok || v != "escalate_human" {
		t.Fatalf("expected escalate_human effective verdict before sign, got v=%q ok=%v", v, ok)
	}
	if got := ResolveUnsatisfiedBlockingCriteria(preCriteria, preVerdicts); len(got) != 1 || got[0] != "c1" {
		t.Fatalf("expected c1 held pending on escalate_human before sign, got %v", got)
	}

	result, err := service.SignDemandCriterionVerdict(context.Background(), SignDemandCriterionVerdictRequest{
		TenantID:    f.tenantID,
		DemandID:    f.demandID,
		ActorUserID: f.ownerID,
		CriterionID: "c1",
		Verdict:     demandCriterionVerdictSatisfied,
		Reason:      "判官升级人类：预算耗尽后人工验收通过",
	})
	if err != nil {
		t.Fatalf("sign escalate_human criterion: %v", err)
	}
	if result.DemandStatus != ProjectDemandStatusCompleted {
		t.Fatalf("expected demand completed after escalate_human override, got %s", result.DemandStatus)
	}
	postVerdicts, _ := repo.ListDemandCriterionVerdicts(context.Background(), f.tenantID, f.demandID, f.revisionID)
	if human := findHumanCriterionVerdict(postVerdicts, "c1"); human == nil || human.Verdict != demandCriterionVerdictSatisfied {
		t.Fatalf("expected persisted human satisfied verdict overriding escalate_human, got %#v", human)
	}
	demand, _ := repo.GetProjectDemand(context.Background(), f.tenantID, f.demandID)
	if demand.Status != ProjectDemandStatusCompleted {
		t.Fatalf("expected repository demand completed after escalate_human override, got %s", demand.Status)
	}
}

// TestHumanSignRejectsNonSignableMethod: only human_judgment and
// adversarial_review are human-signable. A blocking automated_test criterion
// still returns ErrInvalidProject (regression guard for the broadened method
// check).
func TestHumanSignRejectsNonSignableMethod(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	f := setupDemandAcceptanceSignFixture(repo, []DemandAcceptanceCriterion{
		{CriterionID: "c1", Statement: "自动化判据", VerificationMethod: "automated_test", Severity: demandAcceptanceCriterionSeverityBlocking},
	})

	_, err = service.SignDemandCriterionVerdict(context.Background(), SignDemandCriterionVerdictRequest{
		TenantID:    f.tenantID,
		DemandID:    f.demandID,
		ActorUserID: f.ownerID,
		CriterionID: "c1",
		Verdict:     demandCriterionVerdictSatisfied,
	})
	if !errors.Is(err, ErrInvalidProject) {
		t.Fatalf("expected ErrInvalidProject for automated_test criterion, got %v", err)
	}
}
