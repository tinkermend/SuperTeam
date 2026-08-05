package projectcoordination

import (
	"fmt"
	"strings"

	"github.com/superteam/control-plane/internal/scenariotemplate"
)

// AnnotateAndValidateExtraTaskRoles stamps skeleton tasks with their template
// role and validates role_key on beyond-skeleton tasks (batch 3 P1).
//
// Unknown role_key on an extra task is demoted to empty + a constraint_notes
// annotation — the plan is NOT rejected (planner retry is expensive; this is a
// labeling issue, not a governance hard-fail). Reachability / casting completeness
// still only considers skeleton roles.
func AnnotateAndValidateExtraTaskRoles(snapshot CoordinationSnapshot, plan *RouteDecisionPlan) {
	if plan == nil {
		return
	}
	allowed := map[string]bool{}
	for _, r := range snapshot.RoleVocabulary {
		key := strings.TrimSpace(r.RoleKey)
		if key != "" {
			allowed[key] = true
		}
	}

	skeletonTaskKeys := map[string]string{} // task key → template role
	if snapshot.ScenarioTemplate != nil {
		if spec, err := scenariotemplate.ParseSpec(snapshot.ScenarioTemplate.Spec); err == nil && len(spec.Skeleton) > 0 {
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
				role := strings.TrimSpace(step.Role)
				task, ok := stepTask(step, producedBy, taskByKey)
				if !ok || task == nil {
					continue
				}
				skeletonTaskKeys[task.Key] = role
				if role != "" {
					task.RoleKey = role
				}
			}
		}
	}

	for i := range plan.Tasks {
		task := &plan.Tasks[i]
		if _, isSkeleton := skeletonTaskKeys[task.Key]; isSkeleton {
			continue
		}
		roleKey := strings.TrimSpace(task.RoleKey)
		if roleKey == "" {
			// Extra task without a role is allowed but noted once we know vocab exists.
			if len(allowed) > 0 {
				plan.ConstraintNotes = append(plan.ConstraintNotes, PlanConstraintNote{
					Kind:    "extra_task_missing_role",
					Message: fmt.Sprintf("骨架外任务 %s 未标注已注册角色", task.Key),
				})
			}
			continue
		}
		if len(allowed) == 0 {
			// No vocabulary loaded (tests / unwired): keep planner-provided key.
			continue
		}
		if !allowed[roleKey] {
			task.RoleKey = ""
			plan.ConstraintNotes = append(plan.ConstraintNotes, PlanConstraintNote{
				Kind:    "extra_task_unknown_role",
				Message: fmt.Sprintf("骨架外任务 %s 的角色 %s 不在词表，已降为无角色", task.Key, roleKey),
			})
		}
	}
}
