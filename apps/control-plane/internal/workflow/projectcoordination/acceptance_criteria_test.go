package projectcoordination

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizeCriterionDefaults(t *testing.T) {
	criterion := PlanAcceptanceCriterion{ID: "ac1", Statement: "登录失败返回 401", SatisfiedBy: []string{"a"}}

	normalizeCriterionDefaults(&criterion)

	require.Equal(t, VerificationMethodAutomatedTest, criterion.VerificationMethod)
	require.Equal(t, CriterionSeverityBlocking, criterion.Severity)

	explicit := PlanAcceptanceCriterion{ID: "ac2", Statement: "人类确认交付物", VerificationMethod: VerificationMethodHumanJudgment, Severity: CriterionSeverityNonBlocking}
	normalizeCriterionDefaults(&explicit)
	require.Equal(t, VerificationMethodHumanJudgment, explicit.VerificationMethod)
	require.Equal(t, CriterionSeverityNonBlocking, explicit.Severity)
}

func TestEnsureHumanJudgmentInjectsFallback(t *testing.T) {
	t.Run("no human judgment criterion injects fallback", func(t *testing.T) {
		plan := &RouteDecisionPlan{
			PlanAcceptanceCriteria: []PlanAcceptanceCriterion{
				{ID: "ac1", Statement: "登录失败返回 401", VerificationMethod: VerificationMethodAutomatedTest, SatisfiedBy: []string{"a"}},
			},
		}

		ensureHumanJudgmentCriterion(plan, nil)

		require.Len(t, plan.PlanAcceptanceCriteria, 2)
		injected := plan.PlanAcceptanceCriteria[1]
		require.Equal(t, "human_final_confirmation", injected.ID)
		require.Equal(t, "人类负责人确认交付符合需求意图", injected.Statement)
		require.Equal(t, VerificationMethodHumanJudgment, injected.VerificationMethod)
		require.Equal(t, CriterionSeverityBlocking, injected.Severity)
		require.Empty(t, injected.SatisfiedBy)
	})

	t.Run("existing human judgment criterion is not duplicated", func(t *testing.T) {
		plan := &RouteDecisionPlan{
			PlanAcceptanceCriteria: []PlanAcceptanceCriterion{
				{ID: "ac_human", Statement: "人类审阅并确认结果符合预期", VerificationMethod: VerificationMethodHumanJudgment},
			},
		}

		ensureHumanJudgmentCriterion(plan, nil)

		require.Len(t, plan.PlanAcceptanceCriteria, 1)
	})

	t.Run("policy exemption skips injection", func(t *testing.T) {
		plan := &RouteDecisionPlan{
			PlanAcceptanceCriteria: []PlanAcceptanceCriterion{
				{ID: "ac1", Statement: "登录失败返回 401", VerificationMethod: VerificationMethodAutomatedTest, SatisfiedBy: []string{"a"}},
			},
		}

		ensureHumanJudgmentCriterion(plan, map[string]any{"acceptance_human_judgment_exempt": true})

		require.Len(t, plan.PlanAcceptanceCriteria, 1)
	})
}

func TestMarkAmbiguousCriteria(t *testing.T) {
	plan := &RouteDecisionPlan{
		PlanAcceptanceCriteria: []PlanAcceptanceCriterion{
			{ID: "ac1", Statement: "尽量优化性能"},
			{ID: "ac2", Statement: "登录失败返回 401"},
		},
	}

	markAmbiguousCriteria(plan)

	require.True(t, plan.PlanAcceptanceCriteria[0].AmbiguityFlag, "vague qualifier should be flagged")
	require.False(t, plan.PlanAcceptanceCriteria[1].AmbiguityFlag, "concrete, judgeable assertion should not be flagged")
}

func TestValidateCriteriaSemanticsRejectsUnknownMethod(t *testing.T) {
	plan := RouteDecisionPlan{
		PlanAcceptanceCriteria: []PlanAcceptanceCriterion{
			{ID: "ac1", Statement: "登录失败返回 401", VerificationMethod: "vibe_check", SatisfiedBy: []string{"a"}},
		},
	}

	err := validateAcceptanceCriteriaSemantics(plan)

	require.Error(t, err)
	require.Contains(t, err.Error(), "unknown_verification_method")
	require.Contains(t, err.Error(), "vibe_check")
}

func TestValidateCriteriaSemanticsAutomatedRequiresSatisfiedBy(t *testing.T) {
	plan := RouteDecisionPlan{
		PlanAcceptanceCriteria: []PlanAcceptanceCriterion{
			{ID: "ac1", Statement: "登录失败返回 401", VerificationMethod: VerificationMethodAutomatedTest, SatisfiedBy: nil},
		},
	}

	err := validateAcceptanceCriteriaSemantics(plan)

	require.Error(t, err)
	require.Contains(t, err.Error(), "automated_test_requires_satisfied_by")
	require.Contains(t, err.Error(), "ac1")
}
