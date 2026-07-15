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
	// PinnedExitDeliverable, when set, forces the planner to use this exact
	// exit_deliverable instead of choosing one itself. Consumed by Task 11's
	// revision-loop re-planning; this task only carries the field.
	PinnedExitDeliverable string `json:"pinned_exit_deliverable,omitempty"`
	// DemandConstraintExemptions carries the demand's first-class governance
	// exemptions (project_demand_constraint_exemptions, loaded by
	// LoadProjectCoordinationSnapshot), so EnforceScenarioTemplateGovernance can
	// skip an exempted constraint instead of rejecting the plan.
	DemandConstraintExemptions []DemandConstraintExemption `json:"demand_constraint_exemptions,omitempty"`
}

// DemandConstraintExemption is the coordination snapshot's local projection of
// project.DemandConstraintExemption — just enough for the governance evaluator to
// match an exemption to a violated constraint (kind + roles), independent of the
// persistence-layer record's id/grantor/audit fields.
type DemandConstraintExemption struct {
	ConstraintKind string   `json:"constraint_kind"`
	Roles          []string `json:"roles,omitempty"`
}

// ScenarioTemplateSnapshot carries the bound template's content into planning.
type ScenarioTemplateSnapshot struct {
	Key     string         `json:"key"`
	Name    string         `json:"name"`
	Version int            `json:"version,omitempty"`
	Spec    map[string]any `json:"spec,omitempty"`
}

type DemandSnapshot struct {
	ID                  uuid.UUID
	Title               string
	Content             string
	ScenarioTemplateKey string `json:"scenario_template_key,omitempty"`
	// CoordinationMode is the demand's coordination_mode ("plan", "loop", ...),
	// carried through to gate the plan-confirmation status in PersistPlanRevision.
	CoordinationMode string `json:"coordination_mode,omitempty"`
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
	// ExitDeliverable is the produces key of the scenario_template exit the planner
	// chose to satisfy the demand — see Task 7/8 for selection and validation.
	ExitDeliverable string
	// TemplateVersion is the scenario_template.version the plan was generated against.
	TemplateVersion int
	// AvailableExits mirrors scenario_template.spec.exits at plan time, for human
	// review context.
	AvailableExits []PlanExitOption
	// ConstraintNotes are server-authored annotations surfaced alongside the plan
	// (e.g. forced human gates); they do not affect the plan fingerprint.
	ConstraintNotes []PlanConstraintNote
}

// PlanExitOption is one scenario_template exit choice, carried into the plan for
// human review context.
type PlanExitOption struct {
	Deliverable string `json:"deliverable"`
	Label       string `json:"label"`
}

// PlanConstraintNote is a server-authored annotation attached to a plan (e.g. a
// forced human-approval gate). It is informational only and excluded from the
// plan fingerprint — see canonicalPlanRevisionPayload.
type PlanConstraintNote struct {
	Kind    string `json:"kind"`
	Message string `json:"message"`
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
