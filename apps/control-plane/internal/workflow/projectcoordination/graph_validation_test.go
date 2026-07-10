package projectcoordination

import (
	"encoding/json"
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

func TestValidateRouteDecisionPlanAcceptsRequiredInputFromAncestor(t *testing.T) {
	plan := RouteDecisionPlan{Reason: "required input from ancestor", Tasks: []PlannedTask{
		planTaskWithIO("a", nil, []string{"load_test_report"}, nil),
		planTaskWithIO("b", []string{"a"}, nil, []string{"load_test_report"}),
	}}
	snapshot := snapshotForPlan(plan)

	require.NoError(t, ValidateRouteDecisionPlan(snapshot, plan, GraphValidationPolicy{MaxTasks: 12}))
}

func TestValidateRouteDecisionPlanRejectsRequiredInputWithNoProducer(t *testing.T) {
	plan := RouteDecisionPlan{Reason: "required input without producer", Tasks: []PlannedTask{
		planTaskWithIO("a", nil, []string{"something_else"}, nil),
		planTaskWithIO("b", []string{"a"}, nil, []string{"load_test_report"}),
	}}
	snapshot := snapshotForPlan(plan)

	err := ValidateRouteDecisionPlan(snapshot, plan, GraphValidationPolicy{MaxTasks: 12})

	require.Error(t, err)
	require.Contains(t, err.Error(), "load_test_report")
}

func TestValidateRouteDecisionPlanRejectsDuplicateProducesKey(t *testing.T) {
	plan := RouteDecisionPlan{Reason: "duplicate produces key", Tasks: []PlannedTask{
		planTaskWithIO("a", nil, []string{"report"}, nil),
		planTaskWithIO("b", nil, []string{"report"}, nil),
	}}
	snapshot := snapshotForPlan(plan)

	err := ValidateRouteDecisionPlan(snapshot, plan, GraphValidationPolicy{MaxTasks: 12})

	require.Error(t, err)
	require.Contains(t, err.Error(), "report")
}

func TestValidateRouteDecisionPlanRejectsBlankProducesKey(t *testing.T) {
	plan := RouteDecisionPlan{Reason: "blank produces key", Tasks: []PlannedTask{
		planTaskWithIO("a", nil, []string{"  "}, nil),
	}}
	snapshot := snapshotForPlan(plan)

	require.Error(t, ValidateRouteDecisionPlan(snapshot, plan, GraphValidationPolicy{MaxTasks: 12}))
}

func TestValidateRouteDecisionPlanRejectsRequiredInputFromNonAncestor(t *testing.T) {
	plan := RouteDecisionPlan{Reason: "required input from non ancestor", Tasks: []PlannedTask{
		planTaskWithIO("a", nil, nil, nil),
		planTaskWithIO("b", []string{"a"}, nil, []string{"load_test_report"}),
		planTaskWithIO("c", []string{"a"}, []string{"load_test_report"}, nil),
	}}
	snapshot := snapshotForPlan(plan)

	err := ValidateRouteDecisionPlan(snapshot, plan, GraphValidationPolicy{MaxTasks: 12})

	require.Error(t, err)
	require.Contains(t, err.Error(), "load_test_report")
}

func TestValidateRouteDecisionPlanRejectsCriterionWithUnknownSatisfier(t *testing.T) {
	employeeID := uuid.New()
	plan := RouteDecisionPlan{
		Reason: "criterion with unknown satisfier",
		PlanAcceptanceCriteria: []PlanAcceptanceCriterion{
			{ID: "ac1", Statement: "done", SatisfiedBy: []string{"no_such_task"}},
		},
		Tasks: []PlannedTask{
			{Key: "real_task", Title: "real_task", Summary: "real_task", SelectedEmployeeID: employeeID, EmployeeSelectionReason: "only one", SelectionConfidence: 0.9, ExpectedOutputs: []string{"execution_summary"}, InputRequirements: map[string]any{}, HandoffContract: map[string]any{}},
		},
	}
	snapshot := snapshotForPlan(plan)

	err := ValidateRouteDecisionPlan(snapshot, plan, GraphValidationPolicy{MaxTasks: 12})

	require.Error(t, err)
	require.Contains(t, err.Error(), "satisfied_by_task_not_found")
	require.Contains(t, err.Error(), "no_such_task")
}

func TestValidateRouteDecisionPlanRejectsCriterionWithNoSatisfier(t *testing.T) {
	employeeID := uuid.New()
	plan := RouteDecisionPlan{
		Reason: "criterion with no satisfier",
		PlanAcceptanceCriteria: []PlanAcceptanceCriterion{
			{ID: "ac1", Statement: "done", SatisfiedBy: nil},
		},
		Tasks: []PlannedTask{
			{Key: "a", Title: "a", Summary: "a", SelectedEmployeeID: employeeID, EmployeeSelectionReason: "only one", SelectionConfidence: 0.9, ExpectedOutputs: []string{"execution_summary"}, InputRequirements: map[string]any{}, HandoffContract: map[string]any{}},
		},
	}
	snapshot := snapshotForPlan(plan)

	err := ValidateRouteDecisionPlan(snapshot, plan, GraphValidationPolicy{MaxTasks: 12})

	require.Error(t, err)
	require.Contains(t, err.Error(), "acceptance_criterion_has_no_satisfier")
}

func TestValidateRouteDecisionPlanAcceptsCriterionWithRealSatisfier(t *testing.T) {
	employeeID := uuid.New()
	plan := RouteDecisionPlan{
		Reason: "criterion with real satisfier",
		PlanAcceptanceCriteria: []PlanAcceptanceCriterion{
			{ID: "ac1", Statement: "done", SatisfiedBy: []string{"a"}},
		},
		Tasks: []PlannedTask{
			{Key: "a", Title: "a", Summary: "a", SelectedEmployeeID: employeeID, EmployeeSelectionReason: "only one", SelectionConfidence: 0.9, ExpectedOutputs: []string{"execution_summary"}, InputRequirements: map[string]any{}, HandoffContract: map[string]any{}},
		},
	}
	snapshot := snapshotForPlan(plan)

	require.NoError(t, ValidateRouteDecisionPlan(snapshot, plan, GraphValidationPolicy{MaxTasks: 12}))
}

func TestAncestorKeysWalksTransitively(t *testing.T) {
	tasks := []PlannedTask{
		{Key: "a"},
		{Key: "b", BlockedByKeys: []string{"a"}},
		{Key: "c", BlockedByKeys: []string{"b"}},
	}

	ancestors := ancestorKeys(tasks, "c")

	require.Contains(t, ancestors, "a")
	require.Contains(t, ancestors, "b")
	require.NotContains(t, ancestors, "c")
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

func TestValidateRouteDecisionPlanAcceptsEmptyRequiredCapabilities(t *testing.T) {
	employeeID := uuid.New()
	snapshot := validationSnapshotWithProfile(employeeID)
	plan := validGraphPlan(employeeID)
	plan.Tasks[0].EmployeeSelectionReason = "only executor in pool"
	plan.Tasks[0].RequiredCapabilities = nil

	require.NoError(t, ValidateRouteDecisionPlan(snapshot, plan, GraphValidationPolicy{MaxTasks: 12}))
}

func TestValidateRouteDecisionPlanRejectsLowConfidence(t *testing.T) {
	employeeID := uuid.New()
	snapshot := validationSnapshotWithProfile(employeeID)
	plan := validEvidenceGraphPlan(employeeID)
	plan.Tasks[0].EmployeeSelectionReason = "closest match, but weak"
	plan.Tasks[0].SelectionConfidence = 0.4

	err := ValidateRouteDecisionPlan(snapshot, plan, GraphValidationPolicy{MaxTasks: 12})

	require.ErrorIs(t, err, ErrNoSuitableEmployee)
}

func TestValidateRouteDecisionPlanAcceptsConfidenceAtThreshold(t *testing.T) {
	employeeID := uuid.New()
	snapshot := validationSnapshotWithProfile(employeeID)
	plan := validEvidenceGraphPlan(employeeID)
	plan.Tasks[0].EmployeeSelectionReason = "exact match"
	plan.Tasks[0].SelectionConfidence = 0.7

	require.NoError(t, ValidateRouteDecisionPlan(snapshot, plan, GraphValidationPolicy{MaxTasks: 12}))
}

func TestSelectionConfidenceThresholdPrefersProjectPolicy(t *testing.T) {
	require.InDelta(t, 0.9,
		selectionConfidenceThreshold(map[string]any{"selection_confidence_threshold": 0.9}), 1e-9)
	require.InDelta(t, 0.8,
		selectionConfidenceThreshold(map[string]any{"selection_confidence_threshold": json.Number("0.8")}), 1e-9)
	require.InDelta(t, defaultSelectionConfidenceThreshold,
		selectionConfidenceThreshold(nil), 1e-9)
	require.InDelta(t, defaultSelectionConfidenceThreshold,
		selectionConfidenceThreshold(map[string]any{"selection_confidence_threshold": "not a number"}), 1e-9)
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

func TestValidateRouteDecisionPlanAcceptsAuthoritativeMissingCapabilitiesWithoutReview(t *testing.T) {
	employeeID := uuid.New()
	snapshot := validationSnapshotWithProfile(employeeID)
	plan := validEvidenceGraphPlan(employeeID)
	plan.Tasks[0].RequiredCapabilities = []string{"database.write"}

	err := ValidateRouteDecisionPlan(snapshot, plan, GraphValidationPolicy{MaxTasks: 10})

	require.NoError(t, err)
}

func TestApplyPlanningProfileScoresDoesNotForceApprovalOnMissingCapability(t *testing.T) {
	employeeID := uuid.New()
	snapshot := CoordinationSnapshot{
		DigitalEmployeePool: []ProjectMemberSnapshot{{
			PrincipalID: employeeID,
			ProjectRole: "executor",
			Status:      "active",
			PlanningProfile: &DigitalEmployeePlanningProfile{
				DigitalEmployeeID: employeeID,
				Capabilities:      []PlanningCapability{{Key: "bash_execution"}},
			},
		}},
	}
	plan := RouteDecisionPlan{Tasks: []PlannedTask{{
		Key:                  "t1",
		SelectedEmployeeID:   employeeID,
		RequiredCapabilities: []string{"invented.capability"},
	}}}

	ApplyPlanningProfileScores(snapshot, &plan)

	require.False(t, plan.RequiresHumanReview, "a fictional vocabulary must not trigger human review")
	require.False(t, plan.Tasks[0].RequiresHumanApproval)
	require.Equal(t, []string{"invented.capability"}, plan.Tasks[0].MissingCapabilities,
		"still recorded for display")
}

func TestApplyPlanningProfileScoresStillForcesApprovalOnProfileHardFailure(t *testing.T) {
	employeeID := uuid.New()
	snapshot := CoordinationSnapshot{
		DigitalEmployeePool: []ProjectMemberSnapshot{{
			PrincipalID: employeeID,
			ProjectRole: "executor",
			Status:      "active",
			PlanningProfile: &DigitalEmployeePlanningProfile{
				DigitalEmployeeID: employeeID,
				// A real, server-derived fact — not a capability name.
				HardFailures: []string{"employee_not_dispatchable"},
			},
		}},
	}
	plan := RouteDecisionPlan{Tasks: []PlannedTask{{Key: "t1", SelectedEmployeeID: employeeID}}}

	ApplyPlanningProfileScores(snapshot, &plan)

	require.True(t, plan.RequiresHumanReview)
	require.True(t, plan.Tasks[0].RequiresHumanApproval)
}

func TestApplyPlanningProfileScoresRecordsMissingCapabilitiesWithoutForcingReview(t *testing.T) {
	employeeID := uuid.New()
	snapshot := validationSnapshotWithProfile(employeeID)
	plan := validEvidenceGraphPlan(employeeID)
	plan.Tasks[0].RequiredCapabilities = []string{"database.write"}

	ApplyPlanningProfileScores(snapshot, &plan)

	require.False(t, plan.RequiresHumanReview)
	require.False(t, plan.Tasks[0].RequiresHumanApproval)
	require.Equal(t, []string{"database.write"}, plan.Tasks[0].MissingCapabilities)
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
			Key:                 "root",
			Title:               "Root",
			Summary:             "Root task",
			SelectedEmployeeID:  employeeID,
			SelectionConfidence: 0.9,
			ExpectedOutputs:     []string{"execution_summary"},
			InputRequirements:   map[string]any{},
			HandoffContract:     map[string]any{},
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

func planTaskWithIO(key string, blockedBy []string, produces []string, requires []string) PlannedTask {
	return PlannedTask{
		Key:                     key,
		Title:                   key,
		Summary:                 key,
		SelectedEmployeeID:      uuid.New(),
		EmployeeSelectionReason: "test",
		SelectionConfidence:     0.9,
		ExpectedOutputs:         []string{"execution_summary"},
		Produces:                produces,
		InputRequirements:       map[string]any{"required_inputs": stringsToAny(requires)},
		HandoffContract:         map[string]any{},
		BlockedByKeys:           blockedBy,
	}
}

func snapshotForPlan(plan RouteDecisionPlan) CoordinationSnapshot {
	pool := make([]ProjectMemberSnapshot, 0, len(plan.Tasks))
	for _, task := range plan.Tasks {
		pool = append(pool, ProjectMemberSnapshot{
			PrincipalID: task.SelectedEmployeeID,
			ProjectRole: "executor",
			Status:      "active",
			PlanningProfile: &DigitalEmployeePlanningProfile{
				DigitalEmployeeID: task.SelectedEmployeeID,
			},
		})
	}
	return CoordinationSnapshot{DigitalEmployeePool: pool}
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
				Key:                 "root",
				Title:               "Root",
				Summary:             "Root task",
				SelectedEmployeeID:  employeeID,
				SelectionConfidence: 0.9,
				ExpectedOutputs:     []string{"execution_summary"},
				InputRequirements:   map[string]any{},
				HandoffContract:     map[string]any{},
			},
			{
				Key:                 "child",
				Title:               "Child",
				Summary:             "Child task",
				SelectedEmployeeID:  employeeID,
				SelectionConfidence: 0.9,
				ExpectedOutputs:     []string{"execution_summary"},
				InputRequirements:   map[string]any{},
				HandoffContract:     map[string]any{},
				BlockedByKeys:       []string{"root"},
			},
		},
	}
}
