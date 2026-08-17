package projectcoordination

import "strings"

// Risk signal source tags (autonomy gate inventory F2). Separates planner
// self-report from platform-derived facts so later stages (F4 E6, F6 whitelist
// by source) can tell them apart. F2 itself does not change gate behaviour.
const (
	RiskSourcePlannerSelfReported            = "planner_self_reported"
	RiskSourcePlatformPolicy                 = "platform_derived:policy"
	RiskSourcePlatformProfileScore           = "platform_derived:profile_score"
	RiskSourcePlatformTemplateGovernance     = "platform_derived:template_governance"
)

// Risk signal kinds recorded on RouteDecisionPlan.RiskAttribution.
const (
	RiskSignalPlanRequiresHumanReview   = "plan_requires_human_review"
	RiskSignalTaskRequiresHumanApproval = "task_requires_human_approval"
	RiskSignalTaskRiskLevel             = "task_risk_level"
)

// RiskSignalAttribution records who authored one high-risk signal on a plan.
type RiskSignalAttribution struct {
	Kind    string `json:"kind"`
	TaskKey string `json:"task_key,omitempty"`
	Source  string `json:"source"`
}

// Criterion source tags (F2 / §6.4). E6 (F4) will only treat template_declared
// and platform_injected human_judgment as stop-human; planner_authored is not
// a pre-sign and must not park full_auto acceptance.
const (
	CriterionSourceTemplateDeclared = "template_declared"
	CriterionSourcePlatformInjected = "platform_injected"
	CriterionSourcePlannerAuthored  = "planner_authored"
)

// attributePlannerSelfReportedRisk tags every high-risk signal currently set on
// the plan as planner_self_reported. Call immediately after decode / before
// platform writers so later platform_derived marks can override.
func attributePlannerSelfReportedRisk(plan *RouteDecisionPlan) {
	if plan == nil {
		return
	}
	if plan.RequiresHumanReview {
		setRiskAttribution(plan, RiskSignalPlanRequiresHumanReview, "", RiskSourcePlannerSelfReported)
	}
	for i := range plan.Tasks {
		task := &plan.Tasks[i]
		key := strings.TrimSpace(task.Key)
		if task.RequiresHumanApproval {
			setRiskAttribution(plan, RiskSignalTaskRequiresHumanApproval, key, RiskSourcePlannerSelfReported)
		}
		if isConstitutionalHighRiskLevel(task.RiskLevel) {
			setRiskAttribution(plan, RiskSignalTaskRiskLevel, key, RiskSourcePlannerSelfReported)
		}
	}
}

// setRiskAttribution upserts one signal attribution (last writer wins).
func setRiskAttribution(plan *RouteDecisionPlan, kind, taskKey, source string) {
	if plan == nil || strings.TrimSpace(kind) == "" || strings.TrimSpace(source) == "" {
		return
	}
	taskKey = strings.TrimSpace(taskKey)
	for i := range plan.RiskAttribution {
		entry := &plan.RiskAttribution[i]
		if entry.Kind == kind && entry.TaskKey == taskKey {
			entry.Source = source
			return
		}
	}
	plan.RiskAttribution = append(plan.RiskAttribution, RiskSignalAttribution{
		Kind:    kind,
		TaskKey: taskKey,
		Source:  source,
	})
}

// markPlanRequiresHumanReviewPlatform sets RequiresHumanReview and attributes
// the signal to the given platform source.
func markPlanRequiresHumanReviewPlatform(plan *RouteDecisionPlan, source string) {
	if plan == nil {
		return
	}
	plan.RequiresHumanReview = true
	setRiskAttribution(plan, RiskSignalPlanRequiresHumanReview, "", source)
}

// markTaskRequiresHumanApprovalPlatform sets a task's RequiresHumanApproval and
// attributes it (and optionally elevates plan review) under a platform source.
func markTaskRequiresHumanApprovalPlatform(plan *RouteDecisionPlan, task *PlannedTask, source string) {
	if plan == nil || task == nil {
		return
	}
	task.RequiresHumanApproval = true
	setRiskAttribution(plan, RiskSignalTaskRequiresHumanApproval, strings.TrimSpace(task.Key), source)
}

// attributePlannerAuthoredCriteria tags every criterion lacking a Source as
// planner_authored (decode path). Platform/template writers must set Source
// explicitly before this runs, or re-set after.
func attributePlannerAuthoredCriteria(criteria []PlanAcceptanceCriterion) {
	for i := range criteria {
		if strings.TrimSpace(criteria[i].Source) == "" {
			criteria[i].Source = CriterionSourcePlannerAuthored
		}
	}
}

// stripPlannerOnlyRiskFlags clears RequiresHumanReview / RequiresHumanApproval
// when their RiskAttribution is only planner_self_reported (F6). Platform
// writers re-set both the flag and attribution; leftover planner-only flags
// must not mint risk_approval or force plan_review.
func stripPlannerOnlyRiskFlags(plan *RouteDecisionPlan) {
	if plan == nil {
		return
	}
	planSrc := riskAttributionSource(plan, RiskSignalPlanRequiresHumanReview, "")
	if plan.RequiresHumanReview && (planSrc == "" || planSrc == RiskSourcePlannerSelfReported) {
		plan.RequiresHumanReview = false
	}
	for i := range plan.Tasks {
		task := &plan.Tasks[i]
		src := riskAttributionSource(plan, RiskSignalTaskRequiresHumanApproval, task.Key)
		if task.RequiresHumanApproval && (src == "" || src == RiskSourcePlannerSelfReported) {
			task.RequiresHumanApproval = false
		}
	}
}

// riskAttributionSource looks up the source for one signal (empty if absent).
func riskAttributionSource(plan *RouteDecisionPlan, kind, taskKey string) string {
	if plan == nil {
		return ""
	}
	taskKey = strings.TrimSpace(taskKey)
	for _, entry := range plan.RiskAttribution {
		if entry.Kind == kind && entry.TaskKey == taskKey {
			return entry.Source
		}
	}
	return ""
}

func cloneRiskAttribution(in []RiskSignalAttribution) []RiskSignalAttribution {
	if len(in) == 0 {
		return nil
	}
	out := make([]RiskSignalAttribution, len(in))
	copy(out, in)
	return out
}
