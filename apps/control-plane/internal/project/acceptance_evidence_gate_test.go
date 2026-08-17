package project

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEvaluateAcceptanceEvidenceGate(t *testing.T) {
	t.Run("zero criteria fails E1", func(t *testing.T) {
		require.Equal(t, AcceptanceEvidenceFailNoMachineProof, EvaluateAcceptanceEvidenceGate(nil, nil))
	})

	t.Run("satisfied automated_test passes", func(t *testing.T) {
		criteria := []DemandAcceptanceCriterion{{
			CriterionID: "c1", VerificationMethod: "automated_test", Severity: "blocking",
		}}
		verdicts := []DemandCriterionVerdict{{
			CriterionID: "c1", Verdict: "satisfied", JudgeType: "executor",
		}}
		require.Equal(t, AcceptanceEvidencePass, EvaluateAcceptanceEvidenceGate(criteria, verdicts))
	})

	t.Run("planner human_judgment does not block", func(t *testing.T) {
		criteria := []DemandAcceptanceCriterion{
			{CriterionID: "c1", VerificationMethod: "automated_test", Severity: "blocking"},
			{CriterionID: "hj", VerificationMethod: "human_judgment", Severity: "blocking", Source: CriterionSourcePlannerAuthored},
		}
		verdicts := []DemandCriterionVerdict{{
			CriterionID: "c1", Verdict: "satisfied", JudgeType: "executor",
		}}
		require.Equal(t, AcceptanceEvidencePass, EvaluateAcceptanceEvidenceGate(criteria, verdicts))
		require.Empty(t, ResolveUnsatisfiedBlockingCriteria(criteria, verdicts))
	})

	t.Run("platform human_judgment blocks E6", func(t *testing.T) {
		criteria := []DemandAcceptanceCriterion{
			{CriterionID: "c1", VerificationMethod: "automated_test", Severity: "blocking"},
			{CriterionID: "human_final_confirmation", VerificationMethod: "human_judgment", Severity: "blocking"},
		}
		verdicts := []DemandCriterionVerdict{{
			CriterionID: "c1", Verdict: "satisfied", JudgeType: "executor",
		}}
		require.Equal(t, AcceptanceEvidenceFailDeclarativeHJ, EvaluateAcceptanceEvidenceGate(criteria, verdicts))
		require.Equal(t, []string{"human_final_confirmation"}, ResolveUnsatisfiedBlockingCriteria(criteria, verdicts))
	})

	t.Run("not_applicable alone is not machine proof", func(t *testing.T) {
		criteria := []DemandAcceptanceCriterion{{
			CriterionID: "c1", VerificationMethod: "automated_test", Severity: "blocking",
		}}
		verdicts := []DemandCriterionVerdict{{
			CriterionID: "c1", Verdict: "not_applicable", JudgeType: "executor",
		}}
		require.Equal(t, AcceptanceEvidenceFailNoMachineProof, EvaluateAcceptanceEvidenceGate(criteria, verdicts))
	})
}

func TestInferCriterionSource(t *testing.T) {
	require.Equal(t, CriterionSourcePlatformInjected, InferCriterionSource("human_final_confirmation", "human_judgment", ""))
	require.Equal(t, CriterionSourcePlatformInjected, InferCriterionSource("produces_delivered:t:x", "automated_test", ""))
	require.Equal(t, CriterionSourceTemplateDeclared, InferCriterionSource("template_criterion_1", "automated_test", ""))
	require.Equal(t, CriterionSourcePlannerAuthored, InferCriterionSource("custom_hj", "human_judgment", ""))
	require.Equal(t, CriterionSourcePlatformInjected, InferCriterionSource("x", "human_judgment", CriterionSourcePlatformInjected))
}
