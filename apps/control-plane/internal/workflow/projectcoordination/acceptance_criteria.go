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
// projects.coordination_policy, exempts a plan from the fallback human-judgment
// criterion injection (e.g. fully automated pipelines with no human owner
// sign-off step).
const acceptanceHumanJudgmentExemptPolicyKey = "acceptance_human_judgment_exempt"

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
// when the plan has none of its own and the coordination policy does not
// exempt it. Call after normalizeCriterionDefaults has run over every existing
// criterion, so this only has to compare against the normalized
// VerificationMethod value.
func ensureHumanJudgmentCriterion(plan *RouteDecisionPlan, policy map[string]any) {
	if plan == nil {
		return
	}
	if acceptanceHumanJudgmentExempt(policy) {
		return
	}
	for _, criterion := range plan.PlanAcceptanceCriteria {
		if criterion.VerificationMethod == VerificationMethodHumanJudgment {
			return
		}
	}
	plan.PlanAcceptanceCriteria = append(plan.PlanAcceptanceCriteria, PlanAcceptanceCriterion{
		ID:                 fallbackHumanJudgmentCriterionID,
		Statement:          fallbackHumanJudgmentCriterionStatement,
		VerificationMethod: VerificationMethodHumanJudgment,
		Severity:           CriterionSeverityBlocking,
	})
}

func acceptanceHumanJudgmentExempt(policy map[string]any) bool {
	raw, ok := policy[acceptanceHumanJudgmentExemptPolicyKey]
	if !ok {
		return false
	}
	exempt, ok := raw.(bool)
	return ok && exempt
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
