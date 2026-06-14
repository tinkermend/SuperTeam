package projectcoordination

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestValidateTaskGraphRejectsCycle(t *testing.T) {
	employeeID := uuid.New()
	plan := RouteDecisionPlan{Reason: "cycle", Tasks: []PlannedTask{
		{Key: "a", Title: "A", Summary: "A", SelectedEmployeeID: employeeID, ExpectedOutputs: []string{"execution_summary"}, InputRequirements: map[string]any{}, HandoffContract: map[string]any{}, BlockedByKeys: []string{"b"}},
		{Key: "b", Title: "B", Summary: "B", SelectedEmployeeID: employeeID, ExpectedOutputs: []string{"execution_summary"}, InputRequirements: map[string]any{}, HandoffContract: map[string]any{}, BlockedByKeys: []string{"a"}},
	}}
	err := ValidateRouteDecisionGraph(plan, []uuid.UUID{employeeID}, GraphValidationPolicy{MaxTasks: 10})
	require.ErrorIs(t, err, ErrInvalidRouteDecision)
}

func TestValidateTaskGraphAcceptsParallelRoots(t *testing.T) {
	first := uuid.New()
	second := uuid.New()
	plan := RouteDecisionPlan{
		Reason: "parallel work",
		Tasks: []PlannedTask{
			{Key: "a", Title: "A", Summary: "A", SelectedEmployeeID: first, ExpectedOutputs: []string{"execution_summary"}, InputRequirements: map[string]any{}, HandoffContract: map[string]any{}},
			{Key: "b", Title: "B", Summary: "B", SelectedEmployeeID: second, ExpectedOutputs: []string{"execution_summary"}, InputRequirements: map[string]any{}, HandoffContract: map[string]any{}},
		},
	}
	require.NoError(t, ValidateRouteDecisionGraph(plan, []uuid.UUID{first, second}, GraphValidationPolicy{MaxTasks: 10}))
}

func TestValidateTaskGraphRejectsDuplicateKey(t *testing.T) {
	employeeID := uuid.New()
	plan := validGraphPlan(employeeID)
	plan.Tasks = append(plan.Tasks, PlannedTask{Key: "root", Title: "Duplicate", Summary: "Duplicate", SelectedEmployeeID: employeeID, ExpectedOutputs: []string{"execution_summary"}, InputRequirements: map[string]any{}, HandoffContract: map[string]any{}})

	err := ValidateRouteDecisionGraph(plan, []uuid.UUID{employeeID}, GraphValidationPolicy{MaxTasks: 10})

	require.ErrorIs(t, err, ErrInvalidRouteDecision)
}

func TestValidateTaskGraphRejectsEmptyExpectedOutputs(t *testing.T) {
	employeeID := uuid.New()
	plan := validGraphPlan(employeeID)
	plan.Tasks[0].ExpectedOutputs = nil

	err := ValidateRouteDecisionGraph(plan, []uuid.UUID{employeeID}, GraphValidationPolicy{MaxTasks: 10})

	require.ErrorIs(t, err, ErrInvalidRouteDecision)
}

func TestValidateTaskGraphRejectsBlankExpectedOutputName(t *testing.T) {
	employeeID := uuid.New()
	plan := validGraphPlan(employeeID)
	plan.Tasks[0].ExpectedOutputs = []string{"   "}

	err := ValidateRouteDecisionGraph(plan, []uuid.UUID{employeeID}, GraphValidationPolicy{MaxTasks: 10})

	require.ErrorIs(t, err, ErrInvalidRouteDecision)
}

func TestValidateTaskGraphRejectsWhitespacePaddedExpectedOutputName(t *testing.T) {
	employeeID := uuid.New()
	plan := validGraphPlan(employeeID)
	plan.Tasks[0].ExpectedOutputs = []string{" execution_summary "}

	err := ValidateRouteDecisionGraph(plan, []uuid.UUID{employeeID}, GraphValidationPolicy{MaxTasks: 10})

	require.ErrorIs(t, err, ErrInvalidRouteDecision)
}

func TestValidateTaskGraphRejectsWhitespacePaddedTaskKey(t *testing.T) {
	employeeID := uuid.New()
	plan := validGraphPlan(employeeID)
	plan.Tasks[0].Key = " root "

	err := ValidateRouteDecisionGraph(plan, []uuid.UUID{employeeID}, GraphValidationPolicy{MaxTasks: 10})

	require.ErrorIs(t, err, ErrInvalidRouteDecision)
}

func TestValidateTaskGraphRejectsDanglingBlocker(t *testing.T) {
	employeeID := uuid.New()
	plan := validGraphPlan(employeeID)
	plan.Tasks[0].BlockedByKeys = []string{"missing"}

	err := ValidateRouteDecisionGraph(plan, []uuid.UUID{employeeID}, GraphValidationPolicy{MaxTasks: 10})

	require.ErrorIs(t, err, ErrInvalidRouteDecision)
}

func TestValidateTaskGraphRejectsWhitespacePaddedBlockerKey(t *testing.T) {
	employeeID := uuid.New()
	plan := validBlockedGraphPlan(employeeID)
	plan.Tasks[1].BlockedByKeys = []string{" root "}

	err := ValidateRouteDecisionGraph(plan, []uuid.UUID{employeeID}, GraphValidationPolicy{MaxTasks: 10})

	require.ErrorIs(t, err, ErrInvalidRouteDecision)
}

func TestValidateTaskGraphRejectsBlankBlockerKey(t *testing.T) {
	employeeID := uuid.New()
	plan := validGraphPlan(employeeID)
	plan.Tasks[0].BlockedByKeys = []string{"   "}

	err := ValidateRouteDecisionGraph(plan, []uuid.UUID{employeeID}, GraphValidationPolicy{MaxTasks: 10})

	require.ErrorIs(t, err, ErrInvalidRouteDecision)
}

func TestValidateTaskGraphRejectsSelfBlockingTask(t *testing.T) {
	employeeID := uuid.New()
	plan := validGraphPlan(employeeID)
	plan.Tasks[0].BlockedByKeys = []string{"root"}

	err := ValidateRouteDecisionGraph(plan, []uuid.UUID{employeeID}, GraphValidationPolicy{MaxTasks: 10})

	require.ErrorIs(t, err, ErrInvalidRouteDecision)
}

func TestValidateTaskGraphRejectsDuplicateBlockerOnTask(t *testing.T) {
	employeeID := uuid.New()
	plan := validBlockedGraphPlan(employeeID)
	plan.Tasks[1].BlockedByKeys = []string{"root", "root"}

	err := ValidateRouteDecisionGraph(plan, []uuid.UUID{employeeID}, GraphValidationPolicy{MaxTasks: 10})

	require.ErrorIs(t, err, ErrInvalidRouteDecision)
}

func TestValidateTaskGraphRejectsNoRoot(t *testing.T) {
	employeeID := uuid.New()
	plan := RouteDecisionPlan{Reason: "no root", Tasks: []PlannedTask{
		{Key: "a", Title: "A", Summary: "A", SelectedEmployeeID: employeeID, ExpectedOutputs: []string{"execution_summary"}, InputRequirements: map[string]any{}, HandoffContract: map[string]any{}, BlockedByKeys: []string{"b"}},
		{Key: "b", Title: "B", Summary: "B", SelectedEmployeeID: employeeID, ExpectedOutputs: []string{"execution_summary"}, InputRequirements: map[string]any{}, HandoffContract: map[string]any{}, BlockedByKeys: []string{"a"}},
	}}

	err := ValidateRouteDecisionGraph(plan, []uuid.UUID{employeeID}, GraphValidationPolicy{MaxTasks: 10})

	require.ErrorIs(t, err, ErrInvalidRouteDecision)
}

func TestValidateTaskGraphRejectsNilEmployee(t *testing.T) {
	employeeID := uuid.New()
	plan := validGraphPlan(employeeID)
	plan.Tasks[0].SelectedEmployeeID = uuid.Nil

	err := ValidateRouteDecisionGraph(plan, []uuid.UUID{employeeID}, GraphValidationPolicy{MaxTasks: 10})

	require.ErrorIs(t, err, ErrInvalidRouteDecision)
}

func TestValidateTaskGraphRejectsTaskCountOverLimit(t *testing.T) {
	employeeID := uuid.New()
	plan := validGraphPlan(employeeID)
	plan.Tasks = append(plan.Tasks, PlannedTask{Key: "extra", Title: "Extra", Summary: "Extra", SelectedEmployeeID: employeeID, ExpectedOutputs: []string{"execution_summary"}, InputRequirements: map[string]any{}, HandoffContract: map[string]any{}})

	err := ValidateRouteDecisionGraph(plan, []uuid.UUID{employeeID}, GraphValidationPolicy{MaxTasks: 1})

	require.ErrorIs(t, err, ErrInvalidRouteDecision)
}

func validGraphPlan(employeeID uuid.UUID) RouteDecisionPlan {
	return RouteDecisionPlan{
		Reason: "valid",
		Tasks: []PlannedTask{{
			Key:                "root",
			Title:              "Root",
			Summary:            "Root task",
			SelectedEmployeeID: employeeID,
			ExpectedOutputs:    []string{"execution_summary"},
			InputRequirements:  map[string]any{},
			HandoffContract:    map[string]any{},
		}},
	}
}

func validBlockedGraphPlan(employeeID uuid.UUID) RouteDecisionPlan {
	return RouteDecisionPlan{
		Reason: "valid blocked graph",
		Tasks: []PlannedTask{
			{
				Key:                "root",
				Title:              "Root",
				Summary:            "Root task",
				SelectedEmployeeID: employeeID,
				ExpectedOutputs:    []string{"execution_summary"},
				InputRequirements:  map[string]any{},
				HandoffContract:    map[string]any{},
			},
			{
				Key:                "child",
				Title:              "Child",
				Summary:            "Child task",
				SelectedEmployeeID: employeeID,
				ExpectedOutputs:    []string{"execution_summary"},
				InputRequirements:  map[string]any{},
				HandoffContract:    map[string]any{},
				BlockedByKeys:      []string{"root"},
			},
		},
	}
}
