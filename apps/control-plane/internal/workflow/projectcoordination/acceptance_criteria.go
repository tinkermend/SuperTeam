package projectcoordination

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/superteam/control-plane/internal/scenariotemplate"
)

// Verification method registry for PlanAcceptanceCriterion.VerificationMethod.
// Adding a new judgment channel means adding it here; validateAcceptanceCriteriaSemantics
// rejects any non-empty value not present in this map.
const (
	VerificationMethodAutomatedTest     = "automated_test"
	VerificationMethodHumanJudgment     = "human_judgment"
	VerificationMethodAdversarialReview = "adversarial_review"
	// VerificationMethodReviewGate is the violation-detection gate channel: a
	// criterion reviewed by the review-gate detector (Task 4), which reviews a
	// specific task's output and therefore requires satisfied_by like
	// automated_test/adversarial_review. Its convergence-gate semantics are
	// default-release (holds only on a detected violation) — see the project
	// package's ResolveUnsatisfiedBlockingCriteria.
	VerificationMethodReviewGate = "review_gate"

	CriterionSeverityBlocking    = "blocking"
	CriterionSeverityNonBlocking = "non_blocking"
)

var knownVerificationMethods = map[string]bool{
	VerificationMethodAutomatedTest:     true,
	VerificationMethodHumanJudgment:     true,
	VerificationMethodAdversarialReview: true,
	VerificationMethodReviewGate:        true,
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

// requireHumanAcceptancePolicyKey, when true in projects.coordination_policy,
// opts a plan back into the fallback human-judgment criterion even when it is
// not high-risk. Default (key absent/false) is autonomy: no injection.
const requireHumanAcceptancePolicyKey = "require_human_acceptance"

// acceptance_human_judgment_exempt was retired in F6 (unidirectional valve:
// exemptions that loosen declarative injection are gone).

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
//  1. planTouchesHighRisk(plan) — platform_derived risk only (F6); OR
//  2. requireHumanAcceptance(policy) — project declarative acceptance.
//
// Template exit-tier human_judgment is folded server-side (F5) and does not
// need a second inject here. A human_judgment criterion already present always
// suppresses the fallback. Call after normalizeCriterionDefaults.
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
	policyTriggered := requireHumanAcceptance(policy)
	if !highRisk && !policyTriggered {
		return
	}

	plan.PlanAcceptanceCriteria = append(plan.PlanAcceptanceCriteria, PlanAcceptanceCriterion{
		ID:                 fallbackHumanJudgmentCriterionID,
		Statement:          fallbackHumanJudgmentCriterionStatement,
		VerificationMethod: VerificationMethodHumanJudgment,
		Severity:           CriterionSeverityBlocking,
		Source:             CriterionSourcePlatformInjected,
	})
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

// planTouchesHighRisk reports whether the plan carries a platform-derived
// high-risk signal (F6 / §4.2): only RiskAttribution sources under
// platform_derived:* count. Planner self-reported RequiresHumanReview /
// RequiresHumanApproval / high RiskLevel are display-only and must not inject
// the constitutional human_judgment criterion.
func planTouchesHighRisk(plan *RouteDecisionPlan) bool {
	if plan == nil {
		return false
	}
	for _, entry := range plan.RiskAttribution {
		if strings.HasPrefix(entry.Source, "platform_derived:") {
			return true
		}
	}
	return false
}

// isConstitutionalHighRiskLevel classifies a free-form risk_level label as
// high-risk for the constitutional (unwaivable) injection trigger. risk_level
// is free-form LLM output, so the set must include the Chinese labels that
// appear elsewhere in the system — it deliberately mirrors project package's
// highRiskLevel (task_result_contract.go), NOT the narrower isHighRiskLevel
// (high/critical only) used for the plan-revision risk *summary*. Kept local to
// projectcoordination to avoid cross-importing the project package solely for
// this check.
func isConstitutionalHighRiskLevel(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "high", "critical", "高", "高风险", "严重":
		return true
	default:
		return false
	}
}

// collapseBlockingHumanJudgment enforces at most one blocking human_judgment
// criterion per plan. Extra blocking human criteria are demoted to
// non_blocking (kept as a readable checklist, no longer signable/gating).
// Keep preference: id == human_final_confirmation, else the first blocking
// human in list order. Call after normalizeCriterionDefaults and before
// ensureHumanJudgmentCriterion so the fallback inject still sees a clean
// "zero vs one" human set.
func collapseBlockingHumanJudgment(plan *RouteDecisionPlan) {
	if plan == nil {
		return
	}
	keepIdx := -1
	for i, criterion := range plan.PlanAcceptanceCriteria {
		if criterion.VerificationMethod != VerificationMethodHumanJudgment {
			continue
		}
		if criterion.Severity != CriterionSeverityBlocking {
			continue
		}
		if criterion.ID == fallbackHumanJudgmentCriterionID {
			keepIdx = i
			break
		}
		if keepIdx < 0 {
			keepIdx = i
		}
	}
	if keepIdx < 0 {
		return
	}
	for i := range plan.PlanAcceptanceCriteria {
		if i == keepIdx {
			continue
		}
		c := &plan.PlanAcceptanceCriteria[i]
		if c.VerificationMethod == VerificationMethodHumanJudgment && c.Severity == CriterionSeverityBlocking {
			c.Severity = CriterionSeverityNonBlocking
		}
	}
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
// collapse excess blocking human_judgment criteria to a single gate, inject
// platform produces-deliverable criteria (F3 / E2), inject the fallback
// human-judgment criterion if warranted, then flag ambiguous statements.
func applyAcceptanceCriteriaDefaults(plan *RouteDecisionPlan, policy map[string]any) {
	if plan == nil {
		return
	}
	for i := range plan.PlanAcceptanceCriteria {
		normalizeCriterionDefaults(&plan.PlanAcceptanceCriteria[i])
	}
	collapseBlockingHumanJudgment(plan)
	ensureProducesDeliverableCriteria(plan)
	ensureHumanJudgmentCriterion(plan, policy)
	markAmbiguousCriteria(plan)
}

// producesDeliverableCriterionIDPrefix marks platform-injected criteria that
// assert a planner-declared produces key was delivered (autonomy F3 / E2).
const producesDeliverableCriterionIDPrefix = "produces_delivered:"

// ensureProducesDeliverableCriteria injects one blocking automated_test
// criterion per (task, produces name) so demand-level acceptance cannot
// complete while a declared handoff deliverable is missing. Task completion
// already rejects missing produces; these criteria lift that check into the
// demand gate and count as platform-generated machine evidence for E1.
func ensureProducesDeliverableCriteria(plan *RouteDecisionPlan) {
	if plan == nil {
		return
	}
	existing := map[string]bool{}
	for _, c := range plan.PlanAcceptanceCriteria {
		existing[c.ID] = true
	}
	for _, task := range plan.Tasks {
		key := strings.TrimSpace(task.Key)
		if key == "" {
			continue
		}
		for _, name := range task.Produces {
			name = strings.TrimSpace(name)
			if name == "" {
				continue
			}
			id := producesDeliverableCriterionIDPrefix + key + ":" + name
			if existing[id] {
				continue
			}
			existing[id] = true
			plan.PlanAcceptanceCriteria = append(plan.PlanAcceptanceCriteria, PlanAcceptanceCriterion{
				ID:                 id,
				Statement:          "任务 " + key + " 已交付 " + name,
				SatisfiedBy:        []string{key},
				VerificationMethod: VerificationMethodAutomatedTest,
				Severity:           CriterionSeverityBlocking,
				Source:             CriterionSourcePlatformInjected,
				EvidenceHint:       "platform_produces_check",
			})
		}
	}
}

// foldTemplateAcceptanceCriteria merges scenario_template.spec.default_acceptance_criteria
// that apply to the plan's chosen exit into PlanAcceptanceCriteria (F5). Runs
// server-side so planner cannot omit or rewrite template-declared criteria.
func foldTemplateAcceptanceCriteria(snapshot CoordinationSnapshot, plan *RouteDecisionPlan) {
	if plan == nil || snapshot.ScenarioTemplate == nil {
		return
	}
	spec, err := scenariotemplate.ParseSpec(snapshot.ScenarioTemplate.Spec)
	if err != nil || len(spec.DefaultAcceptanceCriteria) == 0 {
		return
	}
	exit := strings.TrimSpace(plan.ExitDeliverable)
	if exit == "" {
		return
	}
	taskByStep := map[string]string{}
	for _, task := range plan.Tasks {
		if key := strings.TrimSpace(task.Key); key != "" {
			taskByStep[key] = key
		}
	}
	existing := map[string]bool{}
	for _, c := range plan.PlanAcceptanceCriteria {
		existing[c.ID] = true
	}
	for i, criterion := range spec.DefaultAcceptanceCriteria {
		if !scenariotemplate.ExitCondMet(spec, scenariotemplate.SpecConstraintWhen{ExitAtOrBeyond: criterion.AppliesFromExit}, exit) {
			continue
		}
		id := fmt.Sprintf("template_criterion_%d", i+1)
		if existing[id] {
			continue
		}
		satisfied := make([]string, 0)
		if step, ok := spec.StepByProduce(criterion.AppliesFromExit); ok {
			if key, found := taskByStep[step.Step]; found {
				satisfied = append(satisfied, key)
			}
		}
		if len(satisfied) == 0 && len(plan.Tasks) > 0 {
			satisfied = []string{plan.Tasks[len(plan.Tasks)-1].Key}
		}
		entry := PlanAcceptanceCriterion{
			ID:                 id,
			Statement:          criterion.Statement,
			SatisfiedBy:        satisfied,
			VerificationMethod: strings.TrimSpace(criterion.VerificationMethod),
			Severity:           strings.TrimSpace(criterion.Severity),
			Source:             CriterionSourceTemplateDeclared,
		}
		normalizeCriterionDefaults(&entry)
		plan.PlanAcceptanceCriteria = append(plan.PlanAcceptanceCriteria, entry)
		existing[id] = true
	}
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
		if (method == VerificationMethodAutomatedTest || method == VerificationMethodAdversarialReview || method == VerificationMethodReviewGate) && len(criterion.SatisfiedBy) == 0 {
			return invalidRouteDecision("automated_test_requires_satisfied_by: plan acceptance criterion %q declares verification_method %q but has no satisfied_by task", criterion.ID, method)
		}
		severity := strings.TrimSpace(criterion.Severity)
		if severity != "" && !knownCriterionSeverities[severity] {
			return invalidRouteDecision("unknown_criterion_severity: plan acceptance criterion %q declares unrecognized severity %q", criterion.ID, criterion.Severity)
		}
	}
	return nil
}
