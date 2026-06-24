package projectcoordination

import (
	"fmt"
	"strings"

	"github.com/google/uuid"
)

type GraphValidationPolicy struct {
	MaxTasks int
}

// invalidRouteDecision wraps the ErrInvalidRouteDecision sentinel with a
// field-level reason so a rejected planner output can be diagnosed from logs,
// while errors.Is(err, ErrInvalidRouteDecision) still holds for callers.
func invalidRouteDecision(format string, args ...any) error {
	return fmt.Errorf("invalid route decision: "+fmt.Sprintf(format, args...)+": %w", ErrInvalidRouteDecision)
}

func ValidateRouteDecision(decision RouteDecisionPlan, poolIDs []uuid.UUID) error {
	return ValidateRouteDecisionGraph(decision, poolIDs, GraphValidationPolicy{})
}

func ValidateRouteDecisionPlan(snapshot CoordinationSnapshot, plan RouteDecisionPlan, policy GraphValidationPolicy) error {
	if err := ValidateRouteDecisionGraph(plan, activeExecutorIDs(snapshot.DigitalEmployeePool), policy); err != nil {
		return err
	}
	profiles := planningProfilesByEmployeeID(snapshot.DigitalEmployeePool)
	for _, task := range plan.Tasks {
		if strings.TrimSpace(task.EmployeeSelectionReason) == "" {
			return invalidRouteDecision("task %q: employee_selection_reason is empty", task.Key)
		}
		if !plan.RequiresHumanReview && !task.RequiresHumanApproval && len(task.RequiredCapabilities) == 0 {
			return invalidRouteDecision("task %q: required_capabilities is empty and the task is not flagged for human review", task.Key)
		}
		if hasInvalidRequirementString(task.RequiredCapabilities) ||
			hasInvalidRequirementString(task.MatchedCapabilities) ||
			hasInvalidRequirementString(task.MissingCapabilities) ||
			hasInvalidRequirementString(task.PermissionRequirements) ||
			hasInvalidRequirementString(task.ToolRequirements) ||
			hasInvalidRequirementString(task.RuntimeRequirements) ||
			hasInvalidRequirementString(task.VerificationRequirements) {
			return invalidRouteDecision("task %q: a requirement list contains an empty or untrimmed entry", task.Key)
		}
		profile, ok := profiles[task.SelectedEmployeeID]
		if !ok || profile.DigitalEmployeeID == uuid.Nil || profile.DigitalEmployeeID != task.SelectedEmployeeID {
			return invalidRouteDecision("task %q: selected_employee_id %s has no planning profile in the executor pool", task.Key, task.SelectedEmployeeID)
		}
		score := ScorePlanningProfile(profile, planningTaskRequirements(task))
		reviewRequired := plan.RequiresHumanReview || task.RequiresHumanApproval
		if len(score.HardFailures) > 0 && !reviewRequired {
			return invalidRouteDecision("task %q: selected employee %s has %d capability hard-failure(s) but the task is not flagged for human review", task.Key, task.SelectedEmployeeID, len(score.HardFailures))
		}
		if len(score.MissingCapabilities) > 0 && !reviewRequired {
			return invalidRouteDecision("task %q: selected employee %s is missing %d required capability/-ies (%s) but the task is not flagged for human review", task.Key, task.SelectedEmployeeID, len(score.MissingCapabilities), strings.Join(score.MissingCapabilities, ", "))
		}
	}
	return nil
}

func ApplyPlanningProfileScores(snapshot CoordinationSnapshot, plan *RouteDecisionPlan) {
	if plan == nil {
		return
	}
	profiles := planningProfilesByEmployeeID(snapshot.DigitalEmployeePool)
	for index := range plan.Tasks {
		task := &plan.Tasks[index]
		profile, ok := profiles[task.SelectedEmployeeID]
		if !ok || profile.DigitalEmployeeID == uuid.Nil || profile.DigitalEmployeeID != task.SelectedEmployeeID {
			continue
		}
		score := ScorePlanningProfile(profile, planningTaskRequirements(*task))
		task.SelectionScore = score.Score
		task.MatchedCapabilities = append([]string(nil), score.MatchedCapabilities...)
		task.MissingCapabilities = append([]string(nil), score.MissingCapabilities...)
		task.PlanningProfileSnapshotHash = PlanningProfileSnapshotHash(profile)
		if len(score.HardFailures) > 0 || len(score.MissingCapabilities) > 0 {
			task.RequiresHumanApproval = true
			plan.RequiresHumanReview = true
		}
	}
}

func ValidateRouteDecisionGraph(plan RouteDecisionPlan, poolIDs []uuid.UUID, policy GraphValidationPolicy) error {
	if strings.TrimSpace(plan.Reason) == "" {
		return invalidRouteDecision("plan reason is empty")
	}
	if len(plan.Tasks) == 0 {
		return invalidRouteDecision("plan has no tasks")
	}
	if policy.MaxTasks <= 0 {
		policy.MaxTasks = 12
	}
	if len(plan.Tasks) > policy.MaxTasks {
		return invalidRouteDecision("plan has %d tasks which exceeds the limit of %d", len(plan.Tasks), policy.MaxTasks)
	}
	pool := uuidSet(poolIDs)
	keys := map[string]PlannedTask{}
	for _, task := range plan.Tasks {
		key := task.Key
		if key != strings.TrimSpace(key) {
			return invalidRouteDecision("task key %q has leading or trailing whitespace", key)
		}
		if key == "" || strings.TrimSpace(task.Title) == "" || strings.TrimSpace(task.Summary) == "" {
			return invalidRouteDecision("task %q: key, title, or summary is empty", key)
		}
		if _, exists := keys[key]; exists {
			return invalidRouteDecision("duplicate task key %q", key)
		}
		if _, ok := pool[task.SelectedEmployeeID]; !ok {
			return invalidRouteDecision("task %q: selected_employee_id %s is not in the active executor pool", key, task.SelectedEmployeeID)
		}
		if len(task.ExpectedOutputs) == 0 || task.InputRequirements == nil || task.HandoffContract == nil {
			return invalidRouteDecision("task %q: expected_outputs, input_requirements, or handoff_contract is missing", key)
		}
		for _, output := range task.ExpectedOutputs {
			if output == "" || output != strings.TrimSpace(output) {
				return invalidRouteDecision("task %q: an expected_outputs entry is empty or untrimmed", key)
			}
		}
		keys[key] = task
	}
	for _, task := range plan.Tasks {
		taskKey := task.Key
		blockers := map[string]struct{}{}
		for _, blocker := range task.BlockedByKeys {
			if blocker != strings.TrimSpace(blocker) {
				return invalidRouteDecision("task %q: blocked_by key %q has leading or trailing whitespace", taskKey, blocker)
			}
			if blocker == "" || blocker == taskKey {
				return invalidRouteDecision("task %q: a blocked_by entry is empty or references the task itself", taskKey)
			}
			if _, exists := blockers[blocker]; exists {
				return invalidRouteDecision("task %q: duplicate blocked_by key %q", taskKey, blocker)
			}
			if _, ok := keys[blocker]; !ok {
				return invalidRouteDecision("task %q: blocked_by key %q does not match any task", taskKey, blocker)
			}
			blockers[blocker] = struct{}{}
		}
	}
	if !hasRoot(plan.Tasks) {
		return invalidRouteDecision("plan has no root task (every task is blocked by another)")
	}
	if hasCycle(plan.Tasks) {
		return invalidRouteDecision("plan dependency graph contains a cycle")
	}
	return nil
}

func planningProfilesByEmployeeID(members []ProjectMemberSnapshot) map[uuid.UUID]DigitalEmployeePlanningProfile {
	profiles := make(map[uuid.UUID]DigitalEmployeePlanningProfile, len(members))
	for _, member := range members {
		if member.PlanningProfile == nil {
			continue
		}
		profiles[member.PrincipalID] = *member.PlanningProfile
	}
	return profiles
}

func planningTaskRequirements(task PlannedTask) PlanningTaskRequirements {
	return PlanningTaskRequirements{
		TaskType:               task.TaskKind,
		RequiredCapabilities:   task.RequiredCapabilities,
		PermissionRequirements: task.PermissionRequirements,
		ToolRequirements:       task.ToolRequirements,
		RuntimeRequirements:    task.RuntimeRequirements,
	}
}

func hasInvalidRequirementString(values []string) bool {
	for _, value := range values {
		if value == "" || value != strings.TrimSpace(value) {
			return true
		}
	}
	return false
}

func stringSetsEqual(left []string, right []string) bool {
	leftSet := normalizedStringSet(left)
	rightSet := normalizedStringSet(right)
	if len(leftSet) != len(rightSet) {
		return false
	}
	for value := range leftSet {
		if _, ok := rightSet[value]; !ok {
			return false
		}
	}
	return true
}

func normalizedStringSet(values []string) map[string]struct{} {
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		normalized := normalizePlanningString(value)
		if normalized != "" {
			set[normalized] = struct{}{}
		}
	}
	return set
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
