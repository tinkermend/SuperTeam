package projectcoordination

import (
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/superteam/control-plane/internal/scenariotemplate"
)

// DeepestExitDeliverableWithCasting returns the deepest scenario exit whose
// pruned skeleton roles are all present in PlaybookCasting. Used by casting-
// expansion replan to pin the planner to the exit newly unlocked by 编制
// (design G10: expand → new tasks for the hire; G11: exit change → overbound).
// Returns "" when unbound / no casting / no fully-cast exit.
//
// Does not evaluate role_independence feasibility — that stays in
// EnforceScenarioTemplateGovernance so a cast-locked SoD still fails hard (G12).
func DeepestExitDeliverableWithCasting(snapshot CoordinationSnapshot) string {
	if snapshot.ScenarioTemplate == nil || len(snapshot.PlaybookCasting) == 0 {
		return ""
	}
	spec, err := scenariotemplate.ParseSpec(snapshot.ScenarioTemplate.Spec)
	if err != nil || len(spec.Exits) == 0 {
		return ""
	}
	castRoles := map[string]bool{}
	for _, c := range snapshot.PlaybookCasting {
		role := strings.TrimSpace(c.RoleKey)
		if role != "" && c.DigitalEmployeeID != uuid.Nil {
			castRoles[role] = true
		}
	}
	if len(castRoles) == 0 {
		return ""
	}
	deepest := ""
	for _, exit := range spec.Exits {
		steps, err := pruneSkeletonForExit(spec, exit.Deliverable)
		if err != nil {
			continue
		}
		ok := true
		for _, step := range steps {
			role := strings.TrimSpace(step.Role)
			if role == "" {
				continue
			}
			if !castRoles[role] {
				ok = false
				break
			}
		}
		if ok {
			deepest = exit.Deliverable
		}
	}
	return deepest
}

// ApplyPlaybookCasting forces each skeleton-bound plan task's selected employee
// from project playbook casting (编制). Design: 编制由人定 — planner may suggest, but
// casting is the assignment authority. Called after planner decode and before
// EnforceScenarioTemplateGovernance so G12 SoD still evaluates the real cast.
//
// No-op when casting is empty, template unbound, or a role has no cast row.
func ApplyPlaybookCasting(snapshot CoordinationSnapshot, plan *RouteDecisionPlan) {
	if plan == nil || len(snapshot.PlaybookCasting) == 0 || snapshot.ScenarioTemplate == nil {
		return
	}
	byRole := map[string]uuid.UUID{}
	for _, c := range snapshot.PlaybookCasting {
		role := strings.TrimSpace(c.RoleKey)
		if role == "" || c.DigitalEmployeeID == uuid.Nil {
			continue
		}
		byRole[role] = c.DigitalEmployeeID
	}
	if len(byRole) == 0 {
		return
	}
	spec, err := scenariotemplate.ParseSpec(snapshot.ScenarioTemplate.Spec)
	if err != nil || len(spec.Skeleton) == 0 {
		return
	}
	steps := spec.Skeleton
	if len(spec.Exits) > 0 && strings.TrimSpace(plan.ExitDeliverable) != "" {
		if pruned, pruneErr := pruneSkeletonForExit(spec, plan.ExitDeliverable); pruneErr == nil {
			steps = pruned
		}
	}
	producedBy := map[string]string{}
	taskByKey := make(map[string]*PlannedTask, len(plan.Tasks))
	for i := range plan.Tasks {
		task := &plan.Tasks[i]
		taskByKey[task.Key] = task
		for _, p := range task.Produces {
			producedBy[p] = task.Key
		}
	}
	for _, step := range steps {
		emp, ok := byRole[strings.TrimSpace(step.Role)]
		if !ok {
			continue
		}
		task, ok := stepTask(step, producedBy, taskByKey)
		if !ok {
			continue
		}
		task.SelectedEmployeeID = emp
		task.EmployeeSelectionReason = fmt.Sprintf("剧本编制指定角色 %s", step.Role)
	}
}
