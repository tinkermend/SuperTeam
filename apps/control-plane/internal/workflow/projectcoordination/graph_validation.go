package projectcoordination

import (
	"fmt"
	"strings"

	"github.com/google/uuid"
)

type GraphValidationPolicy struct {
	MaxTasks int
}

var errIncoherentSelectionEvidence = fmt.Errorf("%w: incoherent selection evidence", ErrInvalidRouteDecision)

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
			return ErrInvalidRouteDecision
		}
		if !plan.RequiresHumanReview && !task.RequiresHumanApproval && len(task.RequiredCapabilities) == 0 {
			return ErrInvalidRouteDecision
		}
		if hasInvalidRequirementString(task.RequiredCapabilities) ||
			hasInvalidRequirementString(task.MatchedCapabilities) ||
			hasInvalidRequirementString(task.MissingCapabilities) ||
			hasInvalidRequirementString(task.PermissionRequirements) ||
			hasInvalidRequirementString(task.ToolRequirements) ||
			hasInvalidRequirementString(task.RuntimeRequirements) ||
			hasInvalidRequirementString(task.VerificationRequirements) {
			return ErrInvalidRouteDecision
		}
		profile, ok := profiles[task.SelectedEmployeeID]
		if !ok || profile.DigitalEmployeeID == uuid.Nil || profile.DigitalEmployeeID != task.SelectedEmployeeID {
			return ErrInvalidRouteDecision
		}
		score := ScorePlanningProfile(profile, planningTaskRequirements(task))
		reviewRequired := plan.RequiresHumanReview || task.RequiresHumanApproval
		if len(task.MatchedCapabilities) > 0 && !stringSetsEqual(task.MatchedCapabilities, score.MatchedCapabilities) {
			return errIncoherentSelectionEvidence
		}
		if len(task.MissingCapabilities) > 0 && !stringSetsEqual(task.MissingCapabilities, score.MissingCapabilities) {
			return errIncoherentSelectionEvidence
		}
		if task.SelectionScore > 0 && task.SelectionScore != score.Score {
			return errIncoherentSelectionEvidence
		}
		if len(score.HardFailures) > 0 && !reviewRequired {
			return ErrInvalidRouteDecision
		}
		if len(task.MissingCapabilities) > 0 && !reviewRequired {
			return ErrInvalidRouteDecision
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
		if len(task.MissingCapabilities) == 0 {
			task.MissingCapabilities = append([]string(nil), score.MissingCapabilities...)
		}
		task.PlanningProfileSnapshotHash = PlanningProfileSnapshotHash(profile)
	}
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
