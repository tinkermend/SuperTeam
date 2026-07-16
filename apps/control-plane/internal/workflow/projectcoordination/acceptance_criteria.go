package projectcoordination

import (
	"strings"
	"unicode/utf8"
)

// Verification method registry for PlanAcceptanceCriterion.VerificationMethod.
// Adding a new judgment channel means adding it here; validateAcceptanceCriteriaSemantics
// rejects any non-empty value not present in this map.
const (
	VerificationMethodAutomatedTest = "automated_test"
	VerificationMethodHumanJudgment = "human_judgment"

	CriterionSeverityBlocking    = "blocking"
	CriterionSeverityNonBlocking = "non_blocking"
)

var knownVerificationMethods = map[string]bool{
	VerificationMethodAutomatedTest: true,
	VerificationMethodHumanJudgment: true,
}

var knownCriterionSeverities = map[string]bool{
	CriterionSeverityBlocking:    true,
	CriterionSeverityNonBlocking: true,
}

// fallbackHumanJudgmentCriterionID/Statement is the criterion ensureHumanJudgmentCriterion
// injects when a plan declares no human_judgment criterion of its own. It has no
// SatisfiedBy: it is a business/intent judgment the human owner makes directly, not
// backed by any task's produces.
const (
	fallbackHumanJudgmentCriterionID        = "human_final_confirmation"
	fallbackHumanJudgmentCriterionStatement = "人类负责人确认交付符合需求意图"
)

// acceptanceHumanJudgmentExemptPolicyKey, when true in
// projects.coordination_policy, exempts a plan from the *policy/template*
// triggered fallback human-judgment criterion injection. It does NOT exempt
// the high-risk trigger (see planTouchesHighRisk) — that one is constitutional
// and cannot be waived by policy.
const acceptanceHumanJudgmentExemptPolicyKey = "acceptance_human_judgment_exempt"

// requireHumanAcceptancePolicyKey, when true in projects.coordination_policy,
// opts a plan back into the fallback human-judgment criterion even when it is
// not high-risk. Default (key absent/false) is autonomy: no injection.
const requireHumanAcceptancePolicyKey = "require_human_acceptance"

// ambiguousCriterionMinRuneLength: a statement trimmed shorter than this is too
// terse to be a judgeable assertion.
const ambiguousCriterionMinRuneLength = 8

// ambiguousCriterionPhrases are vague qualifiers that make a statement
// unjudgeable as written (a criterion should read as a decidable assertion, not
// a wish).
var ambiguousCriterionPhrases = []string{"尽量", "适当", "合理", "优化一下", "等等", "大概"}

// normalizeCriterionDefaults fills in VerificationMethod (default
// automated_test) and Severity (default blocking) when the planner omitted
// them. It must run before ensureHumanJudgmentCriterion, whose
// already-declared-human-judgment check relies on every existing criterion
// having a normalized method.
func normalizeCriterionDefaults(criterion *PlanAcceptanceCriterion) {
	if criterion == nil {
		return
	}
	method := strings.TrimSpace(criterion.VerificationMethod)
	if method == "" {
		method = VerificationMethodAutomatedTest
	}
	criterion.VerificationMethod = method

	severity := strings.TrimSpace(criterion.Severity)
	if severity == "" {
		severity = CriterionSeverityBlocking
	}
	criterion.Severity = severity
}

// ensureHumanJudgmentCriterion injects the fallback human_judgment criterion
// only when injection is warranted. Autonomy is the default posture: an
// ordinary, non-high-risk plan under an empty/permissive policy gets NO
// fallback criterion. Injection happens when:
//
//  1. planTouchesHighRisk(plan) — ALWAYS, constitutional, not exemptable by
//     policy; OR
//  2. (requireHumanAcceptance(policy) OR a template-declared human checkpoint
//     — TODO(Task 4): read the selected exit's human_checkpoint declaration
//     from template_governance once it lands) AND NOT
//     acceptanceHumanJudgmentExempt(policy).
//
// A planner-authored human_judgment criterion already present always
// suppresses the fallback (never double-inject), regardless of which trigger
// fired. Call after normalizeCriterionDefaults has run over every existing
// criterion, so this only has to compare against the normalized
// VerificationMethod value.
func ensureHumanJudgmentCriterion(plan *RouteDecisionPlan, policy map[string]any) {
	if plan == nil {
		return
	}
	for _, criterion := range plan.PlanAcceptanceCriteria {
		if criterion.VerificationMethod == VerificationMethodHumanJudgment {
			return
		}
	}

	highRisk := planTouchesHighRisk(plan)
	policyOrTemplateTriggered := requireHumanAcceptance(policy) && !acceptanceHumanJudgmentExempt(policy)
	if !highRisk && !policyOrTemplateTriggered {
		return
	}

	plan.PlanAcceptanceCriteria = append(plan.PlanAcceptanceCriteria, PlanAcceptanceCriterion{
		ID:                 fallbackHumanJudgmentCriterionID,
		Statement:          fallbackHumanJudgmentCriterionStatement,
		VerificationMethod: VerificationMethodHumanJudgment,
		Severity:           CriterionSeverityBlocking,
	})
}

// acceptanceHumanJudgmentExempt reads the legacy exemption key. It only
// suppresses the requireHumanAcceptance/template-checkpoint trigger — never
// the high-risk trigger, which is constitutional and unwaivable by policy.
func acceptanceHumanJudgmentExempt(policy map[string]any) bool {
	raw, ok := policy[acceptanceHumanJudgmentExemptPolicyKey]
	if !ok {
		return false
	}
	exempt, ok := raw.(bool)
	return ok && exempt
}

// requireHumanAcceptance reads the require_human_acceptance policy key.
// Default (key absent or not a bool true) is false — autonomy.
func requireHumanAcceptance(policy map[string]any) bool {
	raw, ok := policy[requireHumanAcceptancePolicyKey]
	if !ok {
		return false
	}
	required, ok := raw.(bool)
	return ok && required
}

// planTouchesHighRisk reports whether the plan carries any high-risk signal:
// plan-level RequiresHumanReview, any task's RequiresHumanApproval, or any
// task's RiskLevel classifying as high (see isHighRiskLevel). This trigger is
// constitutional — it is never exemptable by policy.
func planTouchesHighRisk(plan *RouteDecisionPlan) bool {
	if plan == nil {
		return false
	}
	if plan.RequiresHumanReview {
		return true
	}
	for _, task := range plan.Tasks {
		if task.RequiresHumanApproval {
			return true
		}
		if isHighRiskLevel(task.RiskLevel) {
			return true
		}
	}
	return false
}

// markAmbiguousCriteria flags (but never rejects) criteria whose statement is
// too vague to judge: shorter than ambiguousCriterionMinRuneLength once
// trimmed, or containing a vague qualifier phrase.
func markAmbiguousCriteria(plan *RouteDecisionPlan) {
	if plan == nil {
		return
	}
	for i := range plan.PlanAcceptanceCriteria {
		plan.PlanAcceptanceCriteria[i].AmbiguityFlag = isAmbiguousCriterionStatement(plan.PlanAcceptanceCriteria[i].Statement)
	}
}

func isAmbiguousCriterionStatement(statement string) bool {
	trimmed := strings.TrimSpace(statement)
	if utf8.RuneCountInString(trimmed) < ambiguousCriterionMinRuneLength {
		return true
	}
	for _, phrase := range ambiguousCriterionPhrases {
		if strings.Contains(trimmed, phrase) {
			return true
		}
	}
	return false
}

// applyAcceptanceCriteriaDefaults runs the full plan-level acceptance-criteria
// pipeline in order: normalize every criterion's method/severity defaults,
// inject the fallback human-judgment criterion if none exists (unless
// policy-exempt), then flag ambiguous statements. Both planner production
// paths (the primary decode and the required-review repair synthesis, which
// starts from zero criteria of its own) must call this before
// ValidateRouteDecisionPlan.
func applyAcceptanceCriteriaDefaults(plan *RouteDecisionPlan, policy map[string]any) {
	if plan == nil {
		return
	}
	for i := range plan.PlanAcceptanceCriteria {
		normalizeCriterionDefaults(&plan.PlanAcceptanceCriteria[i])
	}
	ensureHumanJudgmentCriterion(plan, policy)
	markAmbiguousCriteria(plan)
}

// validateAcceptanceCriteriaSemantics rejects a plan whose acceptance criteria
// are semantically inconsistent: an unrecognized verification_method, an
// automated_test criterion with no satisfied_by task, or an unrecognized
// severity. An empty VerificationMethod/Severity is tolerated here (treated as
// not-yet-defaulted); production always normalizes before validating, so this
// only bites on an explicit, unrecognized value.
func validateAcceptanceCriteriaSemantics(plan RouteDecisionPlan) error {
	for _, criterion := range plan.PlanAcceptanceCriteria {
		method := strings.TrimSpace(criterion.VerificationMethod)
		if method != "" && !knownVerificationMethods[method] {
			return invalidRouteDecision("unknown_verification_method: plan acceptance criterion %q declares unrecognized verification_method %q", criterion.ID, criterion.VerificationMethod)
		}
		if method == VerificationMethodAutomatedTest && len(criterion.SatisfiedBy) == 0 {
			return invalidRouteDecision("automated_test_requires_satisfied_by: plan acceptance criterion %q declares verification_method automated_test but has no satisfied_by task", criterion.ID)
		}
		severity := strings.TrimSpace(criterion.Severity)
		if severity != "" && !knownCriterionSeverities[severity] {
			return invalidRouteDecision("unknown_criterion_severity: plan acceptance criterion %q declares unrecognized severity %q", criterion.ID, criterion.Severity)
		}
	}
	return nil
}
