package projectcoordination

import (
	"strings"

	"github.com/google/uuid"
)

type GraphValidationPolicy struct {
	MaxTasks int
}

func ValidateRouteDecision(decision RouteDecisionPlan, poolIDs []uuid.UUID) error {
	return ValidateRouteDecisionGraph(decision, poolIDs, GraphValidationPolicy{})
}

func ValidateRouteDecisionGraph(plan RouteDecisionPlan, poolIDs []uuid.UUID, policy GraphValidationPolicy) error {
	if strings.TrimSpace(plan.Reason) == "" || len(plan.Tasks) == 0 {
		return ErrInvalidRouteDecision
	}
	if policy.MaxTasks <= 0 {
		policy.MaxTasks = 12
	}
	if len(plan.Tasks) > policy.MaxTasks {
		return ErrInvalidRouteDecision
	}
	pool := uuidSet(poolIDs)
	keys := map[string]PlannedTask{}
	for _, task := range plan.Tasks {
		key := task.Key
		if key != strings.TrimSpace(key) {
			return ErrInvalidRouteDecision
		}
		if key == "" || strings.TrimSpace(task.Title) == "" || strings.TrimSpace(task.Summary) == "" {
			return ErrInvalidRouteDecision
		}
		if _, exists := keys[key]; exists {
			return ErrInvalidRouteDecision
		}
		if _, ok := pool[task.SelectedEmployeeID]; !ok {
			return ErrInvalidRouteDecision
		}
		if len(task.ExpectedOutputs) == 0 || task.InputRequirements == nil || task.HandoffContract == nil {
			return ErrInvalidRouteDecision
		}
		for _, output := range task.ExpectedOutputs {
			if output == "" || output != strings.TrimSpace(output) {
				return ErrInvalidRouteDecision
			}
		}
		keys[key] = task
	}
	for _, task := range plan.Tasks {
		taskKey := task.Key
		blockers := map[string]struct{}{}
		for _, blocker := range task.BlockedByKeys {
			if blocker != strings.TrimSpace(blocker) {
				return ErrInvalidRouteDecision
			}
			if blocker == "" || blocker == taskKey {
				return ErrInvalidRouteDecision
			}
			if _, exists := blockers[blocker]; exists {
				return ErrInvalidRouteDecision
			}
			if _, ok := keys[blocker]; !ok {
				return ErrInvalidRouteDecision
			}
			blockers[blocker] = struct{}{}
		}
	}
	if !hasRoot(plan.Tasks) || hasCycle(plan.Tasks) {
		return ErrInvalidRouteDecision
	}
	return nil
}

func hasRoot(tasks []PlannedTask) bool {
	for _, task := range tasks {
		if len(task.BlockedByKeys) == 0 {
			return true
		}
	}
	return false
}

func hasCycle(tasks []PlannedTask) bool {
	dependencies := make(map[string][]string, len(tasks))
	for _, task := range tasks {
		dependencies[task.Key] = task.BlockedByKeys
	}
	state := map[string]int{}
	var visit func(string) bool
	visit = func(key string) bool {
		switch state[key] {
		case 1:
			return true
		case 2:
			return false
		}
		state[key] = 1
		for _, blocker := range dependencies[key] {
			if visit(blocker) {
				return true
			}
		}
		state[key] = 2
		return false
	}
	for key := range dependencies {
		if visit(key) {
			return true
		}
	}
	return false
}

func uuidSet(ids []uuid.UUID) map[uuid.UUID]struct{} {
	set := make(map[uuid.UUID]struct{}, len(ids))
	for _, id := range ids {
		if id == uuid.Nil {
			continue
		}
		set[id] = struct{}{}
	}
	return set
}
