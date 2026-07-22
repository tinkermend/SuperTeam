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

// TestFallbackNotInjectedByDefault: autonomy is the default posture — an
// ordinary plan with no high-risk signal and an empty policy must NOT get the
// fallback human_judgment criterion injected.
func TestFallbackNotInjectedByDefault(t *testing.T) {
	plan := &RouteDecisionPlan{
		PlanAcceptanceCriteria: []PlanAcceptanceCriterion{
			{ID: "ac1", Statement: "登录失败返回 401", VerificationMethod: VerificationMethodAutomatedTest, SatisfiedBy: []string{"a"}},
		},
	}

	ensureHumanJudgmentCriterion(plan, nil)

	require.Len(t, plan.PlanAcceptanceCriteria, 1, "no injection expected by default (autonomy posture)")
}

// TestFallbackInjectedWhenPolicyRequires: policy can opt a plan back into the
// human-judgment fallback via require_human_acceptance.
func TestFallbackInjectedWhenPolicyRequires(t *testing.T) {
	plan := &RouteDecisionPlan{
		PlanAcceptanceCriteria: []PlanAcceptanceCriterion{
			{ID: "ac1", Statement: "登录失败返回 401", VerificationMethod: VerificationMethodAutomatedTest, SatisfiedBy: []string{"a"}},
		},
	}

	ensureHumanJudgmentCriterion(plan, map[string]any{"require_human_acceptance": true})

	require.Len(t, plan.PlanAcceptanceCriteria, 2)
	injected := plan.PlanAcceptanceCriteria[1]
	require.Equal(t, "human_final_confirmation", injected.ID)
	require.Equal(t, "人类负责人确认交付符合需求意图", injected.Statement)
	require.Equal(t, VerificationMethodHumanJudgment, injected.VerificationMethod)
	require.Equal(t, CriterionSeverityBlocking, injected.Severity)
	require.Empty(t, injected.SatisfiedBy)
}

// TestFallbackInjectedWhenHighRisk: a high-risk plan (plan-level
// RequiresHumanReview here) injects the fallback even with an empty policy.
func TestFallbackInjectedWhenHighRisk(t *testing.T) {
	plan := &RouteDecisionPlan{
		RequiresHumanReview: true,
		PlanAcceptanceCriteria: []PlanAcceptanceCriterion{
			{ID: "ac1", Statement: "登录失败返回 401", VerificationMethod: VerificationMethodAutomatedTest, SatisfiedBy: []string{"a"}},
		},
	}

	ensureHumanJudgmentCriterion(plan, nil)

	require.Len(t, plan.PlanAcceptanceCriteria, 2)
	require.Equal(t, "human_final_confirmation", plan.PlanAcceptanceCriteria[1].ID)
}

// TestHighRiskInjectionNotExemptable: the high-risk trigger is constitutional
// — acceptance_human_judgment_exempt must NOT suppress it, even set to true.
func TestHighRiskInjectionNotExemptable(t *testing.T) {
	plan := &RouteDecisionPlan{
		Tasks: []PlannedTask{
			{Key: "t1", RequiresHumanApproval: true},
		},
		PlanAcceptanceCriteria: []PlanAcceptanceCriterion{
			{ID: "ac1", Statement: "登录失败返回 401", VerificationMethod: VerificationMethodAutomatedTest, SatisfiedBy: []string{"a"}},
		},
	}

	ensureHumanJudgmentCriterion(plan, map[string]any{"acceptance_human_judgment_exempt": true})

	require.Len(t, plan.PlanAcceptanceCriteria, 2, "high-risk injection must not be exemptable")
	require.Equal(t, "human_final_confirmation", plan.PlanAcceptanceCriteria[1].ID)
}

// TestPolicyInjectionExemptable: the policy-driven trigger IS exemptable —
// exempt=true suppresses require_human_acceptance when the plan is not
// high-risk.
func TestPolicyInjectionExemptable(t *testing.T) {
	plan := &RouteDecisionPlan{
		PlanAcceptanceCriteria: []PlanAcceptanceCriterion{
			{ID: "ac1", Statement: "登录失败返回 401", VerificationMethod: VerificationMethodAutomatedTest, SatisfiedBy: []string{"a"}},
		},
	}

	ensureHumanJudgmentCriterion(plan, map[string]any{
		"require_human_acceptance":         true,
		"acceptance_human_judgment_exempt": true,
	})

	require.Len(t, plan.PlanAcceptanceCriteria, 1, "exemption should suppress the policy-driven trigger")
}

// TestHighRiskInjectedForChineseRiskLabel: risk_level is free-form LLM output
// and Chinese high-risk labels are anticipated. A benign plan (empty policy, no
// RequiresHumanReview/Approval) whose sole risk signal is a task RiskLevel in
// the Chinese-inclusive high set must still inject the fallback — otherwise
// high-risk work escapes human oversight (constitutional under-injection).
func TestHighRiskInjectedForChineseRiskLabel(t *testing.T) {
	for _, label := range []string{"严重", "高风险", "高", "high", "critical"} {
		t.Run(label, func(t *testing.T) {
			plan := &RouteDecisionPlan{
				Tasks: []PlannedTask{
					{Key: "t1", RiskLevel: label},
				},
				PlanAcceptanceCriteria: []PlanAcceptanceCriterion{
					{ID: "ac1", Statement: "登录失败返回 401", VerificationMethod: VerificationMethodAutomatedTest, SatisfiedBy: []string{"a"}},
				},
			}

			ensureHumanJudgmentCriterion(plan, nil)

			require.Len(t, plan.PlanAcceptanceCriteria, 2, "risk_level %q must trigger high-risk injection", label)
			require.Equal(t, "human_final_confirmation", plan.PlanAcceptanceCriteria[1].ID)
		})
	}
}

// TestPlannerAuthoredHumanCriterionSuppressesFallback: a planner-authored
// human_judgment criterion still suppresses the fallback, even when the plan
// is high-risk (no double-injection).
func TestPlannerAuthoredHumanCriterionSuppressesFallback(t *testing.T) {
	plan := &RouteDecisionPlan{
		RequiresHumanReview: true,
		PlanAcceptanceCriteria: []PlanAcceptanceCriterion{
			{ID: "ac_human", Statement: "人类审阅并确认结果符合预期", VerificationMethod: VerificationMethodHumanJudgment},
		},
	}

	ensureHumanJudgmentCriterion(plan, nil)

	require.Len(t, plan.PlanAcceptanceCriteria, 1)
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

// TestAdversarialReviewMethodRegistered: adversarial_review is a known
// verification_method — a criterion declaring it (with satisfied_by) passes
// semantic validation like automated_test does.
func TestAdversarialReviewMethodRegistered(t *testing.T) {
	require.True(t, knownVerificationMethods[VerificationMethodAdversarialReview])
	require.Equal(t, "adversarial_review", VerificationMethodAdversarialReview)

	plan := RouteDecisionPlan{
		PlanAcceptanceCriteria: []PlanAcceptanceCriterion{
			{ID: "ac1", Statement: "对抗式评审确认输出无重大缺陷", VerificationMethod: VerificationMethodAdversarialReview, SatisfiedBy: []string{"a"}},
		},
	}

	err := validateAcceptanceCriteriaSemantics(plan)

	require.NoError(t, err)
}

// TestAdversarialReviewRequiresSatisfiedBy: adversarial_review reviews a
// specific task's output, so — same as automated_test — it must declare at
// least one satisfied_by task. It is NOT exempted the way human_judgment is.
func TestAdversarialReviewRequiresSatisfiedBy(t *testing.T) {
	plan := RouteDecisionPlan{
		PlanAcceptanceCriteria: []PlanAcceptanceCriterion{
			{ID: "ac1", Statement: "对抗式评审确认输出无重大缺陷", VerificationMethod: VerificationMethodAdversarialReview, SatisfiedBy: nil},
		},
	}

	err := validateAcceptanceCriteriaSemantics(plan)

	require.Error(t, err)
	require.Contains(t, err.Error(), "automated_test_requires_satisfied_by")
	require.Contains(t, err.Error(), "ac1")
}

// TestUnknownMethodStillRejected: regression — adding adversarial_review to
// the registry must not loosen rejection of genuinely unrecognized methods.
func TestUnknownMethodStillRejected(t *testing.T) {
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

// TestCollapseBlockingHumanJudgmentKeepsOne: three blocking human criteria
// collapse to one blocking + two non_blocking checklist items.
func TestCollapseBlockingHumanJudgmentKeepsOne(t *testing.T) {
	plan := &RouteDecisionPlan{
		PlanAcceptanceCriteria: []PlanAcceptanceCriterion{
			{ID: "h1", Statement: "结论业务上可接受", VerificationMethod: VerificationMethodHumanJudgment, Severity: CriterionSeverityBlocking},
			{ID: "h2", Statement: "范围说明充分", VerificationMethod: VerificationMethodHumanJudgment, Severity: CriterionSeverityBlocking},
			{ID: "h3", Statement: "风险已沟通", VerificationMethod: VerificationMethodHumanJudgment, Severity: CriterionSeverityBlocking},
			{ID: "a1", Statement: "登录失败返回 401", VerificationMethod: VerificationMethodAutomatedTest, Severity: CriterionSeverityBlocking, SatisfiedBy: []string{"t1"}},
		},
	}

	collapseBlockingHumanJudgment(plan)

	require.Equal(t, CriterionSeverityBlocking, plan.PlanAcceptanceCriteria[0].Severity)
	require.Equal(t, "h1", plan.PlanAcceptanceCriteria[0].ID)
	require.Equal(t, CriterionSeverityNonBlocking, plan.PlanAcceptanceCriteria[1].Severity)
	require.Equal(t, CriterionSeverityNonBlocking, plan.PlanAcceptanceCriteria[2].Severity)
	require.Equal(t, CriterionSeverityBlocking, plan.PlanAcceptanceCriteria[3].Severity, "automated criteria untouched")
}

// TestCollapsePrefersHumanFinalConfirmation: when the fallback id is present,
// it is the kept blocking gate even if it is not first.
func TestCollapsePrefersHumanFinalConfirmation(t *testing.T) {
	plan := &RouteDecisionPlan{
		PlanAcceptanceCriteria: []PlanAcceptanceCriterion{
			{ID: "h1", Statement: "planner 业务判断", VerificationMethod: VerificationMethodHumanJudgment, Severity: CriterionSeverityBlocking},
			{ID: fallbackHumanJudgmentCriterionID, Statement: fallbackHumanJudgmentCriterionStatement, VerificationMethod: VerificationMethodHumanJudgment, Severity: CriterionSeverityBlocking},
			{ID: "h2", Statement: "另一条人类判断", VerificationMethod: VerificationMethodHumanJudgment, Severity: CriterionSeverityBlocking},
		},
	}

	collapseBlockingHumanJudgment(plan)

	require.Equal(t, CriterionSeverityNonBlocking, plan.PlanAcceptanceCriteria[0].Severity)
	require.Equal(t, CriterionSeverityBlocking, plan.PlanAcceptanceCriteria[1].Severity)
	require.Equal(t, fallbackHumanJudgmentCriterionID, plan.PlanAcceptanceCriteria[1].ID)
	require.Equal(t, CriterionSeverityNonBlocking, plan.PlanAcceptanceCriteria[2].Severity)
}

// TestApplyDefaultsCollapsesThenInjects: high-risk + three planner human
// criteria → collapse to one blocking human, no double-inject of fallback.
func TestApplyDefaultsCollapsesThenInjects(t *testing.T) {
	plan := &RouteDecisionPlan{
		RequiresHumanReview: true,
		PlanAcceptanceCriteria: []PlanAcceptanceCriterion{
			{ID: "h1", Statement: "结论业务上可接受", VerificationMethod: VerificationMethodHumanJudgment},
			{ID: "h2", Statement: "范围说明充分", VerificationMethod: VerificationMethodHumanJudgment},
			{ID: "h3", Statement: "风险已沟通", VerificationMethod: VerificationMethodHumanJudgment},
		},
	}

	applyAcceptanceCriteriaDefaults(plan, nil)

	blockingHuman := 0
	nonBlockingHuman := 0
	for _, c := range plan.PlanAcceptanceCriteria {
		if c.VerificationMethod != VerificationMethodHumanJudgment {
			continue
		}
		switch c.Severity {
		case CriterionSeverityBlocking:
			blockingHuman++
		case CriterionSeverityNonBlocking:
			nonBlockingHuman++
		}
	}
	require.Equal(t, 1, blockingHuman, "exactly one blocking human gate")
	require.Equal(t, 2, nonBlockingHuman, "extras demoted to checklist")
	require.Len(t, plan.PlanAcceptanceCriteria, 3, "no fallback double-inject when planner already authored human")
}

// TestApplyDefaultsHighRiskInjectsSingleFallback: high-risk with only
// automated criteria still gets exactly one human_final_confirmation.
func TestApplyDefaultsHighRiskInjectsSingleFallback(t *testing.T) {
	plan := &RouteDecisionPlan{
		RequiresHumanReview: true,
		PlanAcceptanceCriteria: []PlanAcceptanceCriterion{
			{ID: "a1", Statement: "登录失败返回 401", SatisfiedBy: []string{"t1"}},
		},
	}

	applyAcceptanceCriteriaDefaults(plan, nil)

	require.Len(t, plan.PlanAcceptanceCriteria, 2)
	injected := plan.PlanAcceptanceCriteria[1]
	require.Equal(t, fallbackHumanJudgmentCriterionID, injected.ID)
	require.Equal(t, VerificationMethodHumanJudgment, injected.VerificationMethod)
	require.Equal(t, CriterionSeverityBlocking, injected.Severity)
}
