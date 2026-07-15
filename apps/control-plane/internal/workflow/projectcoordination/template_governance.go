package projectcoordination

import (
	"fmt"
	"strings"

	"github.com/superteam/control-plane/internal/scenariotemplate"
)

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
