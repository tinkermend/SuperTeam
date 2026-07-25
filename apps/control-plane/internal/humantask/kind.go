// Package humantask holds the canonical HumanTask read-model vocabulary.
// decision_type stays the write-path technical enum; kind/layer are additive
// display/contract metadata (2026-07-24 baseline + 2026-07-25 load-budget spec).
package humantask

import "strings"

// KindAndLayer maps an internal decision_type to the canonical HumanTask kind
// and layer. Unknown project_task_* gates keep their decision_type as kind.
func KindAndLayer(decisionType string) (kind string, layer string) {
	switch strings.TrimSpace(decisionType) {
	case "plan_review":
		return "plan_review", "demand"
	case "project_task_approval":
		return "dispatch_release", "task"
	case "project_task_acceptance":
		return "downstream_release", "task"
	case "demand_acceptance":
		return "acceptance_sign", "demand"
	case "project_acceptance":
		return "closure_confirm", "project"
	case "planning_failed":
		return "planning_failed", "demand"
	case "planning_gap":
		return "planning_gap", "demand"
	case "task_failure_recovery":
		return "task_failure_recovery", "task"
	case "":
		return "", ""
	default:
		if strings.HasPrefix(decisionType, "project_task_") {
			return decisionType, "task"
		}
		return decisionType, ""
	}
}
