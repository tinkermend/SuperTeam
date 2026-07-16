package project

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func adversarialCriterion(id string) DemandAcceptanceCriterion {
	return DemandAcceptanceCriterion{
		CriterionID:        id,
		Statement:          "登录接口 p95 延迟低于 200ms",
		VerificationMethod: demandCriterionVerificationMethodAdversarialReview,
		Severity:           demandAcceptanceCriterionSeverityBlocking,
		SatisfiedBy:        []string{"perf_task"},
	}
}

func adversarialVerdict(id, verdict string) DemandCriterionVerdict {
	return DemandCriterionVerdict{
		CriterionID: id,
		Verdict:     verdict,
		JudgeType:   demandCriterionJudgeTypeAdversarial,
		Reason:      "2/3 判官证伪",
	}
}

func humanVerdict(id, verdict string) DemandCriterionVerdict {
	return DemandCriterionVerdict{
		CriterionID: id,
		Verdict:     verdict,
		JudgeType:   demandCriterionJudgeTypeHuman,
	}
}

func executorVerdict(id, verdict string) DemandCriterionVerdict {
	taskID := uuid.New()
	return DemandCriterionVerdict{
		CriterionID:   id,
		Verdict:       verdict,
		JudgeType:     demandCriterionJudgeTypeExecutor,
		ProjectTaskID: &taskID,
	}
}

// TestAdversarialVerdictProjectedAndConsumed: an adversarial aggregate row
// verdict=unsatisfied is (1) the criterion's effective verdict, tagged with the
// adversarial judge type (NOT miscounted as executor), and (2) holds the demand
// at the convergence gate.
func TestAdversarialVerdictProjectedAndConsumed(t *testing.T) {
	criterion := adversarialCriterion("crit_adv")
	verdicts := []DemandCriterionVerdict{adversarialVerdict("crit_adv", demandCriterionVerdictUnsatisfied)}

	verdict, judgeType, _, hasVerdict := criterionEffectiveVerdict(verdicts, "crit_adv")
	require.True(t, hasVerdict)
	require.Equal(t, demandCriterionVerdictUnsatisfied, verdict)
	require.Equal(t, demandCriterionJudgeTypeAdversarial, judgeType)

	pending := ResolveUnsatisfiedBlockingCriteria([]DemandAcceptanceCriterion{criterion}, verdicts)
	require.Equal(t, []string{"crit_adv"}, pending)
}

// TestAdversarialSatisfiedReleasesGate: a majority-not-refuted adversarial
// aggregate (satisfied) releases the blocking criterion.
func TestAdversarialSatisfiedReleasesGate(t *testing.T) {
	criterion := adversarialCriterion("crit_adv")
	verdicts := []DemandCriterionVerdict{adversarialVerdict("crit_adv", demandCriterionVerdictSatisfied)}

	verdict, judgeType, _, hasVerdict := criterionEffectiveVerdict(verdicts, "crit_adv")
	require.True(t, hasVerdict)
	require.Equal(t, demandCriterionVerdictSatisfied, verdict)
	require.Equal(t, demandCriterionJudgeTypeAdversarial, judgeType)

	pending := ResolveUnsatisfiedBlockingCriteria([]DemandAcceptanceCriterion{criterion}, verdicts)
	require.Empty(t, pending)
}

// TestHumanOverridesAdversarial: a human verdict always wins over the
// adversarial aggregate — both directions (human releases an adversarial-refuted
// criterion; human holds an adversarial-satisfied one).
func TestHumanOverridesAdversarial(t *testing.T) {
	criterion := adversarialCriterion("crit_adv")

	// Human satisfied overrides adversarial unsatisfied → released.
	overrideUp := []DemandCriterionVerdict{
		adversarialVerdict("crit_adv", demandCriterionVerdictUnsatisfied),
		humanVerdict("crit_adv", demandCriterionVerdictSatisfied),
	}
	verdict, judgeType, _, hasVerdict := criterionEffectiveVerdict(overrideUp, "crit_adv")
	require.True(t, hasVerdict)
	require.Equal(t, demandCriterionVerdictSatisfied, verdict)
	require.Equal(t, demandCriterionJudgeTypeHuman, judgeType)
	require.Empty(t, ResolveUnsatisfiedBlockingCriteria([]DemandAcceptanceCriterion{criterion}, overrideUp))

	// Human unsatisfied overrides adversarial satisfied → held.
	overrideDown := []DemandCriterionVerdict{
		adversarialVerdict("crit_adv", demandCriterionVerdictSatisfied),
		humanVerdict("crit_adv", demandCriterionVerdictUnsatisfied),
	}
	verdict, judgeType, _, hasVerdict = criterionEffectiveVerdict(overrideDown, "crit_adv")
	require.True(t, hasVerdict)
	require.Equal(t, demandCriterionVerdictUnsatisfied, verdict)
	require.Equal(t, demandCriterionJudgeTypeHuman, judgeType)
	require.Equal(t, []string{"crit_adv"}, ResolveUnsatisfiedBlockingCriteria([]DemandAcceptanceCriterion{criterion}, overrideDown))
}

// TestAdversarialCriterionHoldsWithoutAdversarialVerdict: a blocking
// adversarial_review criterion holds when no adversarial verdict exists yet —
// and a spurious executor self-report must NOT release it (only a human verdict
// or the adversarial aggregate can). This is the escalate_human / engine-error
// safety property: absent a real adversarial decision, the gate holds.
func TestAdversarialCriterionHoldsWithoutAdversarialVerdict(t *testing.T) {
	criterion := adversarialCriterion("crit_adv")

	// No verdict at all → held.
	require.Equal(t, []string{"crit_adv"},
		ResolveUnsatisfiedBlockingCriteria([]DemandAcceptanceCriterion{criterion}, nil))

	// Spurious executor "satisfied" self-report → still held (executor cannot
	// release an adversarial_review criterion).
	executorOnly := []DemandCriterionVerdict{executorVerdict("crit_adv", demandCriterionVerdictSatisfied)}
	require.Equal(t, []string{"crit_adv"},
		ResolveUnsatisfiedBlockingCriteria([]DemandAcceptanceCriterion{criterion}, executorOnly))

	// escalate_human aggregate (budget exhausted) → held.
	escalate := []DemandCriterionVerdict{adversarialVerdict("crit_adv", "escalate_human")}
	require.Equal(t, []string{"crit_adv"},
		ResolveUnsatisfiedBlockingCriteria([]DemandAcceptanceCriterion{criterion}, escalate))
}
