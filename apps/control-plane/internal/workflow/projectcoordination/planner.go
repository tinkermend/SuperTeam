package projectcoordination

import (
	"context"
	"errors"

	"github.com/google/uuid"
)

var ErrInvalidRouteDecision = errors.New("invalid route decision")

type CoordinationSnapshot struct {
	ProjectID            uuid.UUID
	Demand               DemandSnapshot
	DigitalEmployeePool  []ProjectMemberSnapshot
	CoordinationPolicy   map[string]any
	PreviousRouteContext map[string]any
	// ScenarioTemplate is the project's bound scenario template, injected into
	// the planner prompt. nil means unbound (generic fallback): the planner
	// behaves exactly as before templates existed.
	ScenarioTemplate *ScenarioTemplateSnapshot
}

// ScenarioTemplateSnapshot carries the bound template's content into planning.
type ScenarioTemplateSnapshot struct {
	Key     string         `json:"key"`
	Name    string         `json:"name"`
	Version int            `json:"version,omitempty"`
	Spec    map[string]any `json:"spec,omitempty"`
}

type DemandSnapshot struct {
	ID      uuid.UUID
	Title   string
	Content string
}

type ProjectMemberSnapshot struct {
	PrincipalID     uuid.UUID                       `json:"principal_id"`
	ProjectRole     string                          `json:"project_role"`
	Status          string                          `json:"status"`
	DisplayName     string                          `json:"display_name,omitempty"`
	PlanningProfile *DigitalEmployeePlanningProfile `json:"planning_profile,omitempty"`
}

// RoutePlanner plans a demand's execution route. The only supported implementation
// is a reasoning-model planner (see NewOpenAICompatibleRoutePlanner); there is deliberately
// no non-reasoning / heuristic fallback in production — planning failures surface as
// errors instead of silently degrading to a fan-out.
type RoutePlanner interface {
	Plan(ctx context.Context, snapshot CoordinationSnapshot) (RouteDecisionPlan, error)
}

type RouteDecisionPlan struct {
	Reason                 string
	RequiresHumanReview    bool
	BudgetEstimate         map[string]any
	TemplateKey            string
	PlannerMetadata        map[string]any
	PlanAcceptanceCriteria []PlanAcceptanceCriterion
	Tasks                  []PlannedTask
}

type PlannedTask struct {
	Key                         string
	Title                       string
	Summary                     string
	SelectedEmployeeID          uuid.UUID
	EmployeeSelectionReason     string
	RequiredCapabilities        []string
	MatchedCapabilities         []string
	MissingCapabilities         []string
	PermissionRequirements      []string
	ToolRequirements            []string
	RuntimeRequirements         []string
	VerificationRequirements    []string
	SelectionScore              int
	SelectionConfidence         float64
	PlanningProfileSnapshotHash string
	TaskKind                    string
	StageIndex                  *int32
	RiskLevel                   string
	RequiresHumanApproval       bool
	ExpectedOutputs             []string
	// Produces are plan-scoped output keys other tasks may declare as
	// required_inputs. Unlike ExpectedOutputs (prose, for humans), these are
	// matched by the validator.
	Produces          []string
	InputRequirements map[string]any
	HandoffContract   map[string]any
	BlockedByKeys     []string
}

// activeExecutorIDs returns the active executor members of a coordination snapshot's
// digital-employee pool. Shared by the reasoning planner and the test double.
func activeExecutorIDs(members []ProjectMemberSnapshot) []uuid.UUID {
	ids := make([]uuid.UUID, 0, len(members))
	for _, member := range members {
		if member.PrincipalID != uuid.Nil && member.ProjectRole == "executor" && member.Status == "active" {
			ids = append(ids, member.PrincipalID)
		}
	}
	return ids
}
