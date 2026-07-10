package projectcoordination

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

type GraphValidationPolicy struct {
	MaxTasks int
}

// defaultSelectionConfidenceThreshold is the fallback floor. The real knob is
// projects.coordination_policy.selection_confidence_threshold, not an ops
// constant.
const defaultSelectionConfidenceThreshold = 0.7

// ErrNoSuitableEmployee means the planner could not find an employee it believed
// fit the task. The demand goes back to the human with the planner's reasons; it
// is not a plan defect to be repaired.
var ErrNoSuitableEmployee = errors.New("no suitable employee")

// invalidRouteDecision wraps the ErrInvalidRouteDecision sentinel with a
// field-level reason so a rejected planner output can be diagnosed from logs,
// while errors.Is(err, ErrInvalidRouteDecision) still holds for callers.
func invalidRouteDecision(format string, args ...any) error {
	return fmt.Errorf("invalid route decision: "+fmt.Sprintf(format, args...)+": %w", ErrInvalidRouteDecision)
}

func selectionConfidenceThreshold(policy map[string]any) float64 {
	raw, ok := policy["selection_confidence_threshold"]
	if !ok {
		return defaultSelectionConfidenceThreshold
	}
	switch value := raw.(type) {
	case float64:
		if value > 0 && value <= 1 {
			return value
		}
	case json.Number:
		if parsed, err := value.Float64(); err == nil && parsed > 0 && parsed <= 1 {
			return parsed
		}
	}
	return defaultSelectionConfidenceThreshold
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
		if task.SelectionConfidence < selectionConfidenceThreshold(snapshot.CoordinationPolicy) {
			return fmt.Errorf("%w: task %q: employee %s scored %.2f", ErrNoSuitableEmployee, task.Key, task.SelectedEmployeeID, task.SelectionConfidence)
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
		if len(score.HardFailures) > 0 {
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
	producers := map[string]string{}
	for _, task := range plan.Tasks {
		for _, key := range task.Produces {
			trimmed := strings.TrimSpace(key)
			if trimmed == "" {
				return invalidRouteDecision("task %q: produces contains an empty key", task.Key)
			}
			if owner, exists := producers[trimmed]; exists {
				return invalidRouteDecision("produces key %q is claimed by both task %q and task %q; a key must have exactly one producer", trimmed, owner, task.Key)
			}
			producers[trimmed] = task.Key
		}
	}
	for _, task := range plan.Tasks {
		ancestors := ancestorKeys(plan.Tasks, task.Key)
		for _, required := range plannerRequiredInputs(task.InputRequirements) {
			producer, ok := producers[required]
			if !ok {
				return invalidRouteDecision("task %q: required input %q is produced by no task in this plan", task.Key, required)
			}
			if _, reachable := ancestors[producer]; !reachable {
				return invalidRouteDecision("task %q: required input %q is produced by task %q, which is not an ancestor", task.Key, required, producer)
			}
		}
	}
	// Plan-level acceptance criteria: each satisfied_by must name a real task key.
	// produces-key uniqueness is already enforced, so a satisfier resolves to one
	// task — but satisfied_by references the task key directly, not a produces key,
	// so we check membership in the task set.
	taskKeys := map[string]struct{}{}
	for _, task := range plan.Tasks {
		taskKeys[task.Key] = struct{}{}
	}
	for _, criterion := range plan.PlanAcceptanceCriteria {
		if len(criterion.SatisfiedBy) == 0 {
			return invalidRouteDecision("plan acceptance criterion %q has no satisfied_by task; a criterion must be backed by at least one task", criterion.ID)
		}
		for _, satisfier := range criterion.SatisfiedBy {
			if _, ok := taskKeys[satisfier]; !ok {
				return invalidRouteDecision("plan acceptance criterion %q satisfied_by %q is not a task in this plan", criterion.ID, satisfier)
			}
		}
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

// ancestorKeys returns every task key reachable by walking BlockedByKeys upward
// from key, excluding key itself.
func ancestorKeys(tasks []PlannedTask, key string) map[string]struct{} {
	dependencies := make(map[string][]string, len(tasks))
	for _, task := range tasks {
		dependencies[task.Key] = task.BlockedByKeys
	}
	ancestors := map[string]struct{}{}
	var visit func(string)
	visit = func(current string) {
		for _, blocker := range dependencies[current] {
			if _, seen := ancestors[blocker]; seen {
				continue
			}
			ancestors[blocker] = struct{}{}
			visit(blocker)
		}
	}
	visit(key)
	delete(ancestors, key)
	return ancestors
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
