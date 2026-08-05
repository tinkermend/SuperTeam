package scenariotemplate

import (
	"fmt"
	"strings"
)

// PruneSkeletonForExit returns the subset of spec.Skeleton reachable via
// depends_on from the step that produces exitDeliverable, preserving the
// skeleton's declared order. It errors if exitDeliverable is not a declared
// exit, or if no skeleton step produces it.
//
// Shared by planning governance and playbook-readiness; do not reimplement.
func PruneSkeletonForExit(spec SpecV2, exitDeliverable string) ([]SpecSkeletonStep, error) {
	if spec.ExitIndex(exitDeliverable) < 0 {
		return nil, fmt.Errorf("exit deliverable %q is not a declared exit", exitDeliverable)
	}
	target, ok := spec.StepByProduce(exitDeliverable)
	if !ok {
		return nil, fmt.Errorf("no skeleton step produces %q", exitDeliverable)
	}
	byStep := map[string]SpecSkeletonStep{}
	for _, s := range spec.Skeleton {
		byStep[s.Step] = s
	}
	included := map[string]bool{}
	var visit func(step SpecSkeletonStep)
	visit = func(step SpecSkeletonStep) {
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
	var pruned []SpecSkeletonStep
	for _, s := range spec.Skeleton {
		if included[s.Step] {
			pruned = append(pruned, s) // 保持声明序
		}
	}
	return pruned, nil
}

// ExitCondMet evaluates a SpecConstraint's When clause against the plan's
// chosen exit. An empty clause is unconditional (always met). Otherwise the
// constraint is met once the plan's exit is at-or-beyond the referenced exit
// in the template's declared exit ordering. Unresolvable exits are treated as
// met — sooner enforce a constraint than silently skip one.
//
// Shared by planning governance and playbook-readiness; do not reimplement.
func ExitCondMet(spec SpecV2, cond SpecConstraintWhen, exit string) bool {
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
