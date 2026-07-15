package projectcoordination

import (
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/superteam/control-plane/internal/scenariotemplate"
)

// noSuitableEmployeeStructuralGapMessage is the fixed message surfaced when a
// role_independence violation cannot be repaired by re-planning because the
// active executor pool is too small to make the two roles independent — see
// EnforceScenarioTemplateGovernance.
const noSuitableEmployeeStructuralGapMessage = "项目员工池无法满足审查独立性约束（需≥2名可调度员工）；可改选更浅出口、为项目补充员工、或换用模板"

// pruneSkeletonForExit returns the subset of spec.Skeleton reachable via
// depends_on from the step that produces exitDeliverable, preserving the
// skeleton's declared order. It errors if exitDeliverable is not a declared
// exit, or if no skeleton step produces it.
func pruneSkeletonForExit(spec scenariotemplate.SpecV2, exitDeliverable string) ([]scenariotemplate.SpecSkeletonStep, error) {
	if spec.ExitIndex(exitDeliverable) < 0 {
		return nil, fmt.Errorf("exit deliverable %q is not a declared exit", exitDeliverable)
	}
	target, ok := spec.StepByProduce(exitDeliverable)
	if !ok {
		return nil, fmt.Errorf("no skeleton step produces %q", exitDeliverable)
	}
	byStep := map[string]scenariotemplate.SpecSkeletonStep{}
	for _, s := range spec.Skeleton {
		byStep[s.Step] = s
	}
	included := map[string]bool{}
	var visit func(step scenariotemplate.SpecSkeletonStep)
	visit = func(step scenariotemplate.SpecSkeletonStep) {
		if included[step.Step] {
			return
		}
		included[step.Step] = true
		for _, dep := range step.DependsOn {
			if d, ok := byStep[dep]; ok {
				visit(d)
			}
		}
	}
	visit(target)
	var pruned []scenariotemplate.SpecSkeletonStep
	for _, s := range spec.Skeleton {
		if included[s.Step] {
			pruned = append(pruned, s) // 保持声明序
		}
	}
	return pruned, nil
}

// validateSkeletonAdherence checks that plan's tasks collectively produce
// every deliverable required by the scenario template's skeleton, pruned to
// the steps reachable from plan.ExitDeliverable. A spec with no skeleton
// (generic/unbound fallback) is a no-op.
func validateSkeletonAdherence(spec scenariotemplate.SpecV2, plan RouteDecisionPlan) error {
	if len(spec.Skeleton) == 0 {
		return nil
	}
	if len(spec.Exits) > 0 && strings.TrimSpace(plan.ExitDeliverable) == "" {
		names := make([]string, 0, len(spec.Exits))
		for _, e := range spec.Exits {
			names = append(names, e.Deliverable)
		}
		return invalidRouteDecision("exit_deliverable required: template declares exits %v", names)
	}
	steps := spec.Skeleton
	if len(spec.Exits) > 0 {
		pruned, err := pruneSkeletonForExit(spec, plan.ExitDeliverable)
		if err != nil {
			return invalidRouteDecision("invalid exit_deliverable %q: %v", plan.ExitDeliverable, err)
		}
		steps = pruned
	}
	producedBy := map[string]string{} // produce name -> task key（全局唯一性已由 ValidateRouteDecisionGraph 保证）
	for _, task := range plan.Tasks {
		for _, p := range task.Produces {
			producedBy[p] = task.Key
		}
	}
	for _, step := range steps {
		for _, p := range step.ProducesDefaults {
			if _, ok := producedBy[p.Name]; !ok {
				return invalidRouteDecision("skeleton step %q deliverable %q missing from plan produces", step.Step, p.Name)
			}
		}
	}
	return nil
}

// exitCondMet evaluates a SpecConstraint's When clause against the plan's
// chosen exit. An empty clause is unconditional (always met). Otherwise the
// constraint is met once the plan's exit is at-or-beyond the referenced exit
// in the template's declared exit ordering. If either exit cannot be
// resolved in the spec's Exits list (should not happen for a spec that has
// already passed scenariotemplate.ParseSpec, but defends against a stale
// caller), the condition is treated as met — sooner enforce a constraint
// than silently skip one on unrecognized input.
func exitCondMet(spec scenariotemplate.SpecV2, cond scenariotemplate.SpecConstraintWhen, exit string) bool {
	if strings.TrimSpace(cond.ExitAtOrBeyond) == "" {
		return true
	}
	target := spec.ExitIndex(cond.ExitAtOrBeyond)
	current := spec.ExitIndex(exit)
	if target < 0 || current < 0 {
		return true
	}
	return current >= target
}

// stepTask resolves the plan task that fulfils a skeleton step, via the
// step's first declared produces_defaults name. Steps with no
// produces_defaults, or whose declared output has no producing task in the
// plan, resolve to (nil, false).
func stepTask(step scenariotemplate.SpecSkeletonStep, producedBy map[string]string, taskByKey map[string]*PlannedTask) (*PlannedTask, bool) {
	if len(step.ProducesDefaults) == 0 {
		return nil, false
	}
	key, ok := producedBy[step.ProducesDefaults[0].Name]
	if !ok {
		return nil, false
	}
	task, ok := taskByKey[key]
	return task, ok
}

// enforceRoleIndependence checks every pair of roles named by a
// role_independence constraint for a shared employee. A shared employee is
// only ever a violation to reject-and-replan (invalidRouteDecision) when the
// project's active executor pool has at least 2 members — meaning a
// different assignment could in principle satisfy the constraint. When the
// pool cannot possibly make the two roles independent (fewer than 2 active
// executors), rejecting-and-replanning would just have the planner reselect
// the same sole employee forever; this is a structural pool gap, not a plan
// defect, so it escalates through the ErrNoSuitableEmployee family straight
// to a human instead (see graph_validation.go's confidence-threshold
// handling for the same "non-plan-defect" channel).
func enforceRoleIndependence(constraint scenariotemplate.SpecConstraint, roleEmployees map[string]map[uuid.UUID]bool, activeExecutorCount int) error {
	for i := 0; i < len(constraint.Roles); i++ {
		for j := i + 1; j < len(constraint.Roles); j++ {
			left := roleEmployees[constraint.Roles[i]]
			right := roleEmployees[constraint.Roles[j]]
			for id := range left {
				if !right[id] {
					continue
				}
				if activeExecutorCount >= 2 {
					return invalidRouteDecision("constraint role_independence violated: roles %v share employee %s", constraint.Roles, id)
				}
				return fmt.Errorf("%w: %s", ErrNoSuitableEmployee, noSuitableEmployeeStructuralGapMessage)
			}
		}
	}
	return nil
}

// EnforceScenarioTemplateGovernance applies a bound scenario template's
// constraints and collapse rules to a plan that has already passed
// ValidateRouteDecisionPlan. It must be called immediately after every
// production ValidateRouteDecisionPlan call succeeds.
//
//   - A constraint violation is a plan defect: returns an error wrapping
//     ErrInvalidRouteDecision (or, for a role_independence violation that no
//     re-plan could fix, ErrNoSuitableEmployee — see enforceRoleIndependence).
//   - human_gate forces the target step's task RequiresHumanApproval=true and
//     appends a ConstraintNote.
//   - A collapse_rules hit is server-detected (not planner-self-reported —
//     stronger than requiring the planner to declare it, and cannot be
//     omitted by an uncooperative or careless planner output) and sets
//     plan.RequiresHumanReview, with a ConstraintNote naming both role titles.
//   - plan.TemplateVersion and plan.AvailableExits are always populated from
//     the bound template, for payload/human-review context — this is why the
//     function takes a pointer.
//
// A nil ScenarioTemplate (unbound/generic project) or an empty spec.Skeleton
// (generic fallback spec) is a no-op beyond populating TemplateVersion/
// AvailableExits, matching validateSkeletonAdherence's own generic-noop rule.
// The function is mode-agnostic: it applies identically to plan-mode and
// loop-mode plans, since neither RouteDecisionPlan nor CoordinationSnapshot
// carries a "mode" the evaluator branches on.
func EnforceScenarioTemplateGovernance(snapshot CoordinationSnapshot, plan *RouteDecisionPlan) error {
	if plan == nil || snapshot.ScenarioTemplate == nil {
		return nil
	}
	spec, err := scenariotemplate.ParseSpec(snapshot.ScenarioTemplate.Spec)
	if err != nil {
		return invalidRouteDecision("scenario template spec unparsable: %v", err)
	}

	plan.TemplateVersion = snapshot.ScenarioTemplate.Version
	plan.AvailableExits = make([]PlanExitOption, 0, len(spec.Exits))
	for _, exit := range spec.Exits {
		plan.AvailableExits = append(plan.AvailableExits, PlanExitOption{Deliverable: exit.Deliverable, Label: exit.Label})
	}

	if len(spec.Skeleton) == 0 {
		return nil
	}

	steps := spec.Skeleton
	if len(spec.Exits) > 0 {
		pruned, err := pruneSkeletonForExit(spec, plan.ExitDeliverable)
		if err != nil {
			return invalidRouteDecision("invalid exit_deliverable %q: %v", plan.ExitDeliverable, err)
		}
		steps = pruned
	}
	prunedSet := make(map[string]scenariotemplate.SpecSkeletonStep, len(steps))
	for _, s := range steps {
		prunedSet[s.Step] = s
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

	roleTitle := make(map[string]string, len(spec.Roles))
	for _, role := range spec.Roles {
		roleTitle[role.Key] = role.Title
	}

	// A role may span multiple steps within the pruned skeleton (e.g.
	// "developer" both develops and releases); union all employees observed
	// for each role.
	roleEmployees := map[string]map[uuid.UUID]bool{}
	for _, step := range steps {
		task, ok := stepTask(step, producedBy, taskByKey)
		if !ok {
			continue
		}
		if roleEmployees[step.Role] == nil {
			roleEmployees[step.Role] = map[uuid.UUID]bool{}
		}
		roleEmployees[step.Role][task.SelectedEmployeeID] = true
	}

	activeExecutorCount := len(activeExecutorIDs(snapshot.DigitalEmployeePool))

	for _, constraint := range spec.Constraints {
		if !exitCondMet(spec, constraint.When, plan.ExitDeliverable) {
			continue
		}
		switch constraint.Kind {
		case "role_independence":
			if err := enforceRoleIndependence(constraint, roleEmployees, activeExecutorCount); err != nil {
				return err
			}
		case "stage_required":
			step, ok := prunedSet[constraint.Step]
			if !ok {
				return invalidRouteDecision("constraint stage_required violated: step %q is not reachable from exit %q", constraint.Step, plan.ExitDeliverable)
			}
			if _, ok := stepTask(step, producedBy, taskByKey); !ok {
				return invalidRouteDecision("constraint stage_required violated: step %q has no corresponding task in the plan", constraint.Step)
			}
		case "human_gate":
			step, ok := prunedSet[constraint.Target]
			if !ok {
				return invalidRouteDecision("constraint human_gate violated: target step %q is not reachable from exit %q", constraint.Target, plan.ExitDeliverable)
			}
			task, ok := stepTask(step, producedBy, taskByKey)
			if !ok {
				return invalidRouteDecision("constraint human_gate violated: target step %q has no corresponding task in the plan", constraint.Target)
			}
			task.RequiresHumanApproval = true
			plan.ConstraintNotes = append(plan.ConstraintNotes, PlanConstraintNote{
				Kind:    "human_gate",
				Message: fmt.Sprintf("发布任务已强制人类审批：由 human_gate@%s v%d 触发", snapshot.ScenarioTemplate.Key, snapshot.ScenarioTemplate.Version),
			})
		}
	}

	// Collapse annotation is server-generated, unconditionally, from the
	// plan's actual employee assignments — see the doc comment above for why
	// this is deliberately stronger than requiring the planner to self-report
	// a collapse.
	for _, rule := range spec.CollapseRules {
		if len(rule.Roles) != 2 {
			continue
		}
		left, right := roleEmployees[rule.Roles[0]], roleEmployees[rule.Roles[1]]
		shared := false
		for id := range left {
			if right[id] {
				shared = true
				break
			}
		}
		if !shared {
			continue
		}
		plan.RequiresHumanReview = true
		plan.ConstraintNotes = append(plan.ConstraintNotes, PlanConstraintNote{
			Kind:    "collapse",
			Message: fmt.Sprintf("角色折叠：%s 与 %s 由同一员工承担，已自动标注待人工复核", roleTitleOrKey(roleTitle, rule.Roles[0]), roleTitleOrKey(roleTitle, rule.Roles[1])),
		})
	}

	return nil
}

func roleTitleOrKey(titles map[string]string, key string) string {
	if title, ok := titles[key]; ok && strings.TrimSpace(title) != "" {
		return title
	}
	return key
}
