package project

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// reviewGateCriterion builds a blocking review_gate criterion (a violation
// DETECTOR bound to a specific task's output, so it carries satisfied_by).
func reviewGateCriterion(id string) DemandAcceptanceCriterion {
	return DemandAcceptanceCriterion{
		CriterionID:        id,
		Statement:          "交付未引入安全违规",
		VerificationMethod: demandCriterionVerificationMethodReviewGate,
		Severity:           demandAcceptanceCriterionSeverityBlocking,
		SatisfiedBy:        []string{"impl_task"},
	}
}

// reviewGateVerdict builds a review_gate aggregate verdict row (judge_type
// review_gate), the detector's projected outcome for a criterion.
func reviewGateVerdict(id, verdict string) DemandCriterionVerdict {
	return DemandCriterionVerdict{
		CriterionID: id,
		Verdict:     verdict,
		JudgeType:   demandCriterionJudgeTypeReviewGate,
		Reason:      "检测门",
	}
}

func humanJudgmentCriterion(id string) DemandAcceptanceCriterion {
	return DemandAcceptanceCriterion{
		CriterionID:        id,
		Statement:          "人类负责人确认交付符合需求意图",
		VerificationMethod: demandCriterionVerificationMethodHumanJudgment,
		Severity:           demandAcceptanceCriterionSeverityBlocking,
	}
}

func automatedTestCriterion(id string) DemandAcceptanceCriterion {
	return DemandAcceptanceCriterion{
		CriterionID:        id,
		Statement:          "单元测试全部通过",
		VerificationMethod: "automated_test",
		Severity:           demandAcceptanceCriterionSeverityBlocking,
		SatisfiedBy:        []string{"test_task"},
	}
}

// TestReviewGateNoVerdictReleases is the core reversal: a blocking review_gate
// criterion with NO verdict (detector not run / nothing detected) is RELEASED,
// the opposite of automated_test/human_judgment's held-by-default.
func TestReviewGateNoVerdictReleases(t *testing.T) {
	criterion := reviewGateCriterion("crit_rg")
	pending := ResolveUnsatisfiedBlockingCriteria([]DemandAcceptanceCriterion{criterion}, nil)
	require.Empty(t, pending)
}

// TestReviewGateViolationHolds: a detected violation (review_gate unsatisfied
// verdict) holds the demand at the convergence gate.
func TestReviewGateViolationHolds(t *testing.T) {
	criterion := reviewGateCriterion("crit_rg")
	verdicts := []DemandCriterionVerdict{reviewGateVerdict("crit_rg", demandCriterionVerdictUnsatisfied)}
	pending := ResolveUnsatisfiedBlockingCriteria([]DemandAcceptanceCriterion{criterion}, verdicts)
	require.Equal(t, []string{"crit_rg"}, pending)
}

// TestReviewGateCleanReleases: a review_gate satisfied verdict (detector ran,
// clean) releases the criterion.
func TestReviewGateCleanReleases(t *testing.T) {
	criterion := reviewGateCriterion("crit_rg")
	verdicts := []DemandCriterionVerdict{reviewGateVerdict("crit_rg", demandCriterionVerdictSatisfied)}
	pending := ResolveUnsatisfiedBlockingCriteria([]DemandAcceptanceCriterion{criterion}, verdicts)
	require.Empty(t, pending)
}

// TestReviewGatePendingPlaceholderHolds (P1.1 race fix): the synchronous
// `pending` placeholder written at the reviewed task's completion HOLDS the
// criterion until the asynchronous detector flips it — closing the window where
// a review_gate-only demand auto-completed before the detector concluded.
func TestReviewGatePendingPlaceholderHolds(t *testing.T) {
	criterion := reviewGateCriterion("crit_rg")
	verdicts := []DemandCriterionVerdict{reviewGateVerdict("crit_rg", demandCriterionVerdictReviewGatePending)}
	pending := ResolveUnsatisfiedBlockingCriteria([]DemandAcceptanceCriterion{criterion}, verdicts)
	require.Equal(t, []string{"crit_rg"}, pending)
}

// TestReviewGateUnknownVerdictHolds: once ANY review_gate verdict exists, only
// `satisfied` releases — an unexpected value fails toward the human instead of
// releasing on garbage.
func TestReviewGateUnknownVerdictHolds(t *testing.T) {
	criterion := reviewGateCriterion("crit_rg")
	verdicts := []DemandCriterionVerdict{reviewGateVerdict("crit_rg", "garbage")}
	pending := ResolveUnsatisfiedBlockingCriteria([]DemandAcceptanceCriterion{criterion}, verdicts)
	require.Equal(t, []string{"crit_rg"}, pending)
}

// TestHumanOverridesReviewGatePending: the human can release a placeholder-held
// criterion directly (e.g. the detector never concluded) — human satisfied wins
// over the pending placeholder.
func TestHumanOverridesReviewGatePending(t *testing.T) {
	criterion := reviewGateCriterion("crit_rg")
	verdicts := []DemandCriterionVerdict{
		reviewGateVerdict("crit_rg", demandCriterionVerdictReviewGatePending),
		humanVerdict("crit_rg", demandCriterionVerdictSatisfied),
	}
	require.Empty(t, ResolveUnsatisfiedBlockingCriteria([]DemandAcceptanceCriterion{criterion}, verdicts))
}

// TestHumanOverridesReviewGate: a human verdict always wins over the review_gate
// aggregate — human satisfied releases a detector-flagged criterion; human
// unsatisfied holds it.
func TestHumanOverridesReviewGate(t *testing.T) {
	criterion := reviewGateCriterion("crit_rg")

	// Human satisfied overrides a review_gate unsatisfied (detected) → released.
	overrideUp := []DemandCriterionVerdict{
		reviewGateVerdict("crit_rg", demandCriterionVerdictUnsatisfied),
		humanVerdict("crit_rg", demandCriterionVerdictSatisfied),
	}
	verdict, judgeType, _, hasVerdict := criterionEffectiveVerdict(overrideUp, "crit_rg")
	require.True(t, hasVerdict)
	require.Equal(t, demandCriterionVerdictSatisfied, verdict)
	require.Equal(t, demandCriterionJudgeTypeHuman, judgeType)
	require.Empty(t, ResolveUnsatisfiedBlockingCriteria([]DemandAcceptanceCriterion{criterion}, overrideUp))

	// Human unsatisfied → held (even with no detector verdict present).
	overrideDown := []DemandCriterionVerdict{
		humanVerdict("crit_rg", demandCriterionVerdictUnsatisfied),
	}
	verdict, judgeType, _, hasVerdict = criterionEffectiveVerdict(overrideDown, "crit_rg")
	require.True(t, hasVerdict)
	require.Equal(t, demandCriterionVerdictUnsatisfied, verdict)
	require.Equal(t, demandCriterionJudgeTypeHuman, judgeType)
	require.Equal(t, []string{"crit_rg"}, ResolveUnsatisfiedBlockingCriteria([]DemandAcceptanceCriterion{criterion}, overrideDown))
}

// TestReviewGateVerdictNotMiscountedAsExecutor: a review_gate row is resolved
// with judge_type=review_gate, never miscounted into the executor accumulators.
func TestReviewGateVerdictNotMiscountedAsExecutor(t *testing.T) {
	verdicts := []DemandCriterionVerdict{reviewGateVerdict("crit_rg", demandCriterionVerdictUnsatisfied)}
	verdict, judgeType, _, hasVerdict := criterionEffectiveVerdict(verdicts, "crit_rg")
	require.True(t, hasVerdict)
	require.Equal(t, demandCriterionVerdictUnsatisfied, verdict)
	require.Equal(t, demandCriterionJudgeTypeReviewGate, judgeType)
}

// TestHumanJudgmentStillHeldByDefault (regression): a blocking human_judgment
// criterion with NO verdict is STILL held — the reversal must not leak into the
// human backstop.
func TestHumanJudgmentStillHeldByDefault(t *testing.T) {
	criterion := humanJudgmentCriterion("crit_hj")
	pending := ResolveUnsatisfiedBlockingCriteria([]DemandAcceptanceCriterion{criterion}, nil)
	require.Equal(t, []string{"crit_hj"}, pending)
}

// TestAutomatedTestUnchanged (regression): automated_test remains held-by-default
// — no verdict holds, executor satisfied/not_applicable releases, executor
// unsatisfied holds.
func TestAutomatedTestUnchanged(t *testing.T) {
	criterion := automatedTestCriterion("crit_at")

	// No verdict → held.
	require.Equal(t, []string{"crit_at"},
		ResolveUnsatisfiedBlockingCriteria([]DemandAcceptanceCriterion{criterion}, nil))

	// Executor satisfied → released.
	sat := []DemandCriterionVerdict{executorVerdict("crit_at", demandCriterionVerdictSatisfied)}
	require.Empty(t, ResolveUnsatisfiedBlockingCriteria([]DemandAcceptanceCriterion{criterion}, sat))

	// Executor not_applicable → released.
	na := []DemandCriterionVerdict{executorVerdict("crit_at", demandCriterionVerdictNotApplicable)}
	require.Empty(t, ResolveUnsatisfiedBlockingCriteria([]DemandAcceptanceCriterion{criterion}, na))

	// Executor unsatisfied → held.
	unsat := []DemandCriterionVerdict{executorVerdict("crit_at", demandCriterionVerdictUnsatisfied)}
	require.Equal(t, []string{"crit_at"},
		ResolveUnsatisfiedBlockingCriteria([]DemandAcceptanceCriterion{criterion}, unsat))
}
