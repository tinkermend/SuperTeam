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
	Reason              string
	RequiresHumanReview bool
	BudgetEstimate      map[string]any
	TemplateKey         string
	PlannerMetadata     map[string]any
	Tasks               []PlannedTask
}

type PlannedTask struct {
	Key                      string
	Title                    string
	Summary                  string
	SelectedEmployeeID       uuid.UUID
	EmployeeSelectionReason  string
	RequiredCapabilities     []string
	MatchedCapabilities      []string
	MissingCapabilities      []string
	PermissionRequirements   []string
	ToolRequirements         []string
	RuntimeRequirements      []string
	VerificationRequirements []string
	SelectionScore           int
	TaskKind                 string
	StageIndex               *int32
	RiskLevel                string
	RequiresHumanApproval    bool
	ExpectedOutputs          []string
	InputRequirements        map[string]any
	HandoffContract          map[string]any
	BlockedByKeys            []string
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
