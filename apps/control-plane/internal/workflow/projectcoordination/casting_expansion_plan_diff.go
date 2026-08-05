package projectcoordination

import (
	"strings"
)

// CastingExpansionPlanDiffResult describes whether a replan after casting
// expansion may auto-run or must return to human confirmation (§7.4).
type CastingExpansionPlanDiffResult struct {
	Overbound bool
	Reasons   []string
}

// EvaluateCastingExpansionPlanDiff compares the previously effective plan with
// the newly planned revision. Auto-run is allowed only when differences are
// confined to new tasks for the expansion employee; exit changes, reassignment
// of existing keys, or unrelated new tasks force overbound.
func EvaluateCastingExpansionPlanDiff(oldPlan, newPlan PlanRevisionPayload, expansionEmployeeID string) CastingExpansionPlanDiffResult {
	var reasons []string
	oldExit := strings.TrimSpace(oldPlan.ExitDeliverable)
	newExit := strings.TrimSpace(newPlan.ExitDeliverable)
	if oldExit != "" && newExit != "" && oldExit != newExit {
		reasons = append(reasons, "exit_changed:"+oldExit+"→"+newExit)
	}
	oldByKey := map[string]PlanRevisionTask{}
	for _, t := range oldPlan.Tasks {
		key := strings.TrimSpace(t.PlannedTaskKey)
		if key != "" {
			oldByKey[key] = t
		}
	}
	newByKey := map[string]PlanRevisionTask{}
	for _, t := range newPlan.Tasks {
		key := strings.TrimSpace(t.PlannedTaskKey)
		if key != "" {
			newByKey[key] = t
		}
	}
	expansionEmployeeID = strings.TrimSpace(expansionEmployeeID)
	for key, oldT := range oldByKey {
		newT, ok := newByKey[key]
		if !ok {
			reasons = append(reasons, "existing_task_removed:"+key)
			continue
		}
		if strings.TrimSpace(oldT.SelectedEmployeeID) != "" &&
			strings.TrimSpace(newT.SelectedEmployeeID) != "" &&
			strings.TrimSpace(oldT.SelectedEmployeeID) != strings.TrimSpace(newT.SelectedEmployeeID) {
			reasons = append(reasons, "existing_task_reassigned:"+key)
		}
	}
	for key, newT := range newByKey {
		if _, ok := oldByKey[key]; ok {
			continue
		}
		// New task: must belong to the expansion hire to auto-run.
		if expansionEmployeeID == "" || strings.TrimSpace(newT.SelectedEmployeeID) != expansionEmployeeID {
			reasons = append(reasons, "unrelated_new_task:"+key)
		}
	}
	return CastingExpansionPlanDiffResult{
		Overbound: len(reasons) > 0,
		Reasons:   reasons,
	}
}
