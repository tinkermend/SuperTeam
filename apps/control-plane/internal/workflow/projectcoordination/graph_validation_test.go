package projectcoordination

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestValidateTaskGraphErrorCarriesFieldLevelReason(t *testing.T) {
	employeeID := uuid.New()
	stranger := uuid.New()
	plan := validGraphPlan(employeeID)
	plan.Tasks[0].SelectedEmployeeID = stranger

	err := ValidateRouteDecisionGraph(plan, []uuid.UUID{employeeID}, GraphValidationPolicy{MaxTasks: 10})

	require.ErrorIs(t, err, ErrInvalidRouteDecision)
	require.Contains(t, err.Error(), "not in the active executor pool")
	require.Contains(t, err.Error(), stranger.String())
}

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

func TestValidateRouteDecisionPlanRejectsMissingSelectionReason(t *testing.T) {
	employeeID := uuid.New()
	snapshot := validationSnapshotWithProfile(employeeID)
	plan := validGraphPlan(employeeID)
	plan.Tasks[0].TaskKind = "database_analysis"
	plan.Tasks[0].RequiredCapabilities = []string{"database.read"}
	plan.Tasks[0].MatchedCapabilities = []string{"database.read"}

	err := ValidateRouteDecisionPlan(snapshot, plan, GraphValidationPolicy{MaxTasks: 10})

	require.ErrorIs(t, err, ErrInvalidRouteDecision)
}

func TestValidateRouteDecisionPlanRejectsHardMissingCapabilityWithoutReview(t *testing.T) {
	t.Skip("removed in Task 3")
	employeeID := uuid.New()
	snapshot := validationSnapshotWithProfile(employeeID)
	plan := validGraphPlan(employeeID)
	plan.Tasks[0].TaskKind = "database_analysis"
	plan.Tasks[0].EmployeeSelectionReason = "选择员工"
	plan.Tasks[0].RequiredCapabilities = []string{"database.write"}
	plan.Tasks[0].MissingCapabilities = []string{"database.write"}

	err := ValidateRouteDecisionPlan(snapshot, plan, GraphValidationPolicy{MaxTasks: 10})

	require.ErrorIs(t, err, ErrInvalidRouteDecision)
}

func TestValidateRouteDecisionPlanAllowsMissingCapabilityWithHumanReview(t *testing.T) {
	employeeID := uuid.New()
	snapshot := validationSnapshotWithProfile(employeeID)
	plan := validGraphPlan(employeeID)
	plan.RequiresHumanReview = true
	plan.Tasks[0].RequiresHumanApproval = true
	plan.Tasks[0].TaskKind = "database_analysis"
	plan.Tasks[0].EmployeeSelectionReason = "缺 database.write，等待人工确认"
	plan.Tasks[0].RequiredCapabilities = []string{"database.write"}
	plan.Tasks[0].MissingCapabilities = []string{"database.write"}

	err := ValidateRouteDecisionPlan(snapshot, plan, GraphValidationPolicy{MaxTasks: 10})

	require.NoError(t, err)
}

func TestValidateRouteDecisionPlanAllowsMatchedCapabilitiesInDifferentOrder(t *testing.T) {
	employeeID := uuid.New()
	snapshot := validationSnapshotWithProfile(employeeID)
	snapshot.DigitalEmployeePool[0].PlanningProfile.Capabilities = append(snapshot.DigitalEmployeePool[0].PlanningProfile.Capabilities, PlanningCapability{
		Key:        "sql.analysis",
		Level:      "strong",
		Source:     "test",
		Confidence: 1,
	})
	plan := validEvidenceGraphPlan(employeeID)
	plan.Tasks[0].RequiredCapabilities = []string{"database.read", "sql.analysis"}
	plan.Tasks[0].MatchedCapabilities = []string{"sql.analysis", "database.read"}

	err := ValidateRouteDecisionPlan(snapshot, plan, GraphValidationPolicy{MaxTasks: 10})

	require.NoError(t, err)
}

func TestValidateRouteDecisionPlanRejectsProfileIdentityMismatch(t *testing.T) {
	employeeID := uuid.New()
	snapshot := validationSnapshotWithProfile(employeeID)
	snapshot.DigitalEmployeePool[0].PlanningProfile.DigitalEmployeeID = uuid.New()
	plan := validEvidenceGraphPlan(employeeID)

	err := ValidateRouteDecisionPlan(snapshot, plan, GraphValidationPolicy{MaxTasks: 10})

	require.ErrorIs(t, err, ErrInvalidRouteDecision)
}

func TestValidateRouteDecisionPlanAllowsModelSelectionEvidenceDrift(t *testing.T) {
	employeeID := uuid.New()

	for _, tc := range []struct {
		name   string
		mutate func(*PlannedTask)
	}{
		{
			name: "matched capabilities mismatch",
			mutate: func(task *PlannedTask) {
				task.MatchedCapabilities = []string{"model.guess"}
			},
		},
		{
			name: "missing capabilities mismatch",
			mutate: func(task *PlannedTask) {
				task.MissingCapabilities = []string{"model.guess"}
			},
		},
		{
			name: "selection score mismatch",
			mutate: func(task *PlannedTask) {
				task.SelectionScore = 7
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			snapshot := validationSnapshotWithProfile(employeeID)
			plan := validEvidenceGraphPlan(employeeID)
			tc.mutate(&plan.Tasks[0])

			err := ValidateRouteDecisionPlan(snapshot, plan, GraphValidationPolicy{MaxTasks: 10})

			require.NoError(t, err)
			ApplyPlanningProfileScores(snapshot, &plan)
			require.Equal(t, []string{"database.read"}, plan.Tasks[0].MatchedCapabilities)
			require.Empty(t, plan.Tasks[0].MissingCapabilities)
			require.Equal(t, 100, plan.Tasks[0].SelectionScore)
		})
	}
}

func TestValidateRouteDecisionPlanRejectsAuthoritativeMissingCapabilitiesWithoutReview(t *testing.T) {
	employeeID := uuid.New()
	snapshot := validationSnapshotWithProfile(employeeID)
	plan := validEvidenceGraphPlan(employeeID)
	plan.Tasks[0].RequiredCapabilities = []string{"database.write"}

	err := ValidateRouteDecisionPlan(snapshot, plan, GraphValidationPolicy{MaxTasks: 10})

	require.ErrorIs(t, err, ErrInvalidRouteDecision)
}

func TestApplyPlanningProfileScoresMarksMissingCapabilitiesForHumanReview(t *testing.T) {
	employeeID := uuid.New()
	snapshot := validationSnapshotWithProfile(employeeID)
	plan := validEvidenceGraphPlan(employeeID)
	plan.Tasks[0].RequiredCapabilities = []string{"database.write"}

	ApplyPlanningProfileScores(snapshot, &plan)

	require.True(t, plan.RequiresHumanReview)
	require.True(t, plan.Tasks[0].RequiresHumanApproval)
	require.Equal(t, []string{"database.write"}, plan.Tasks[0].MissingCapabilities)
	require.NoError(t, ValidateRouteDecisionPlan(snapshot, plan, GraphValidationPolicy{MaxTasks: 10}))
}

func TestApplyPlanningProfileScoresSkipsProfileIdentityMismatch(t *testing.T) {
	employeeID := uuid.New()
	snapshot := validationSnapshotWithProfile(employeeID)
	snapshot.DigitalEmployeePool[0].PlanningProfile.DigitalEmployeeID = uuid.New()
	plan := validEvidenceGraphPlan(employeeID)

	ApplyPlanningProfileScores(snapshot, &plan)

	require.Zero(t, plan.Tasks[0].SelectionScore)
	require.Empty(t, plan.Tasks[0].PlanningProfileSnapshotHash)
	require.Empty(t, plan.Tasks[0].MatchedCapabilities)
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

func validEvidenceGraphPlan(employeeID uuid.UUID) RouteDecisionPlan {
	plan := validGraphPlan(employeeID)
	plan.Tasks[0].TaskKind = "database_analysis"
	plan.Tasks[0].EmployeeSelectionReason = "具备 database.read 能力"
	plan.Tasks[0].RequiredCapabilities = []string{"database.read"}
	return plan
}

func validationSnapshotWithProfile(employeeID uuid.UUID) CoordinationSnapshot {
	return CoordinationSnapshot{
		ProjectID: uuid.New(),
		Demand:    DemandSnapshot{ID: uuid.New(), Title: "分析数据库"},
		DigitalEmployeePool: []ProjectMemberSnapshot{{
			PrincipalID: employeeID,
			ProjectRole: "executor",
			Status:      "active",
			PlanningProfile: &DigitalEmployeePlanningProfile{
				DigitalEmployeeID: employeeID,
				RoleProfile:       PlanningRoleProfile{PrimaryRole: "data_analyst"},
				Capabilities: []PlanningCapability{{
					Key:        "database.read",
					Level:      "strong",
					Source:     "test",
					Confidence: 1,
				}},
				Skills: []PlanningSkill{{Key: "sql.analysis", Source: "test"}},
				RuntimeRequirements: PlanningRuntimeRequirements{
					ProviderTypes:  []string{"codex"},
					ProviderStatus: "ready",
				},
				Permissions:      []PlanningPermission{{Scope: "database.read", Resource: "dev_database", Status: "granted"}},
				LoadState:        PlanningLoadState{AvailableSlots: 1, Lendable: true},
				ProfileFreshness: PlanningProfileFreshness{SourceState: "ready"},
			},
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
