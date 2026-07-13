package projectcoordination

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	projectmodel "github.com/superteam/control-plane/internal/project"
)

func TestBuildPlanRevisionPayloadCanonicalFingerprintStableAndDefaults(t *testing.T) {
	employeeID := uuid.MustParse("11111111-1111-4111-8111-111111111111")
	plan := RouteDecisionPlan{
		Reason:              "Plan the demand with a canonical task graph.",
		RequiresHumanReview: true,
		PlannerMetadata:     map[string]any{"request_id": uuid.NewString()},
		PlanAcceptanceCriteria: []PlanAcceptanceCriterion{
			{ID: "ac1", Statement: "Tests cover canonical payload behavior", SatisfiedBy: []string{"write_tests"}},
		},
		Tasks: []PlannedTask{
			{
				Key:                         "write_tests",
				Title:                       "Write tests",
				Summary:                     "Cover the canonical payload behavior.",
				SelectedEmployeeID:          employeeID,
				EmployeeSelectionReason:     "Owns control-plane planning tests.",
				RequiredCapabilities:        []string{"go.test", "planning"},
				MatchedCapabilities:         []string{"planning", "go.test"},
				MissingCapabilities:         []string{},
				PermissionRequirements:      []string{"repo.write"},
				ToolRequirements:            []string{"go"},
				RuntimeRequirements:         []string{"darwin"},
				VerificationRequirements:    []string{"go test"},
				SelectionScore:              95,
				PlanningProfileSnapshotHash: "profile-hash",
				TaskKind:                    "test",
				RiskLevel:                   "medium",
				RequiresHumanApproval:       true,
				ExpectedOutputs:             []string{"test_report"},
				Produces:                    []string{"test_report"},
				InputRequirements:           map[string]any{"context_refs": []any{"demand", "repo"}},
				HandoffContract:             map[string]any{"acceptance_criteria": []any{"tests fail first", "tests pass after implementation"}},
			},
			{
				Key:                     "implement",
				Title:                   "Implement",
				Summary:                 "Implement the canonical payload behavior.",
				SelectedEmployeeID:      employeeID,
				EmployeeSelectionReason: "Owns Go implementation.",
				RequiredCapabilities:    []string{"planning", "go.code"},
				ExpectedOutputs:         []string{"patch"},
				Produces:                []string{"patch"},
				InputRequirements:       map[string]any{"required_context": []string{"tests"}, "scope": "payload"},
				HandoffContract:         map[string]any{"acceptance_criteria": "string criterion from contract"},
				BlockedByKeys:           []string{"write_tests"},
			},
		},
	}

	payload := BuildPlanRevisionPayload(plan)
	fingerprint, err := CanonicalPlanFingerprint(payload)
	require.NoError(t, err)

	reordered := plan
	reordered.PlannerMetadata = map[string]any{"request_id": uuid.NewString()}
	reordered.Tasks = []PlannedTask{plan.Tasks[1], plan.Tasks[0]}
	reordered.Tasks[1].RequiredCapabilities = []string{"planning", "go.test"}
	reordered.Tasks[1].MatchedCapabilities = []string{"go.test", "planning"}
	reorderedPayload := BuildPlanRevisionPayload(reordered)
	reorderedFingerprint, err := CanonicalPlanFingerprint(reorderedPayload)
	require.NoError(t, err)

	require.Len(t, fingerprint, 64)
	require.Equal(t, fingerprint, reorderedFingerprint)
	require.Equal(t, plan.Reason, payload.Summary)
	require.Equal(t, []PlanAcceptanceCriterion{
		{ID: "ac1", Statement: "Tests cover canonical payload behavior", SatisfiedBy: []string{"write_tests"}},
	}, payload.PlanAcceptanceCriteria)
	require.Equal(t, []string{"conclusion", "evidence", "risks", "next_steps"}, payload.FinalSummaryContract.RequiredSections)
	require.Equal(t, "write_tests", payload.Tasks[0].PlannedTaskKey)
	require.Equal(t, "Cover the canonical payload behavior.", payload.Tasks[0].Objective)
	require.Equal(t, "Owns control-plane planning tests.", payload.Tasks[0].EmployeeSelectionReason)
	require.Equal(t, []string{"tests fail first", "tests pass after implementation"}, payload.Tasks[0].AcceptanceCriteria)
	require.Equal(t, []string{"string criterion from contract"}, payload.Tasks[1].AcceptanceCriteria)
	require.Equal(t, []string{"demand", "repo"}, payload.Tasks[0].InputContextRefs)
	require.Equal(t, []string{"test_report"}, payload.Tasks[0].Produces)
	require.Equal(t, map[string]any{"required_context": []string{"tests"}, "scope": "payload"}, payload.Tasks[1].InputRequirements)
	roundTripped := PlanRevisionPayloadToPlannedTasks(payload)
	require.Equal(t, []string{"patch"}, roundTripped[1].Produces)
	require.Equal(t, map[string]any{"required_context": []string{"tests"}, "scope": "payload"}, roundTripped[1].InputRequirements)
	require.True(t, payload.HumanReview.Required)
	require.Contains(t, payload.HumanReview.Reasons, "plan_requires_human_review")
	require.Contains(t, payload.HumanReview.Reasons, "task_requires_human_approval:write_tests")
}

func TestValidatePlanRevisionPayloadRejectsDuplicateKeyDanglingDependencyAndCycle(t *testing.T) {
	employeeID := uuid.New()
	t.Run("duplicate key and dangling dependency", func(t *testing.T) {
		payload := validPlanRevisionPayload(employeeID)
		payload.Tasks = append(payload.Tasks, PlanRevisionTask{
			PlannedTaskKey:           "root",
			Title:                    "Duplicate root",
			Objective:                "Duplicate root",
			SelectedEmployeeID:       employeeID.String(),
			ExpectedOutputs:          []string{"duplicate_report"},
			AcceptanceCriteria:       []string{"duplicate_report"},
			DependsOn:                []string{"missing"},
			HumanReviewRequired:      false,
			VerificationRequirements: []string{"go test"},
		})

		result := ValidatePlanRevisionPayload(payload)

		require.False(t, result.Acceptable)
		require.Contains(t, result.Errors, "duplicate_planned_task_key:root")
		require.Contains(t, result.Errors, "unknown_dependency:root->missing")
		require.Len(t, result.PlanFingerprint, 64)
	})

	t.Run("cycle", func(t *testing.T) {
		payload := validPlanRevisionPayload(employeeID)
		payload.Tasks = append(payload.Tasks, PlanRevisionTask{
			PlannedTaskKey:     "child",
			Title:              "Child",
			Objective:          "Child",
			SelectedEmployeeID: employeeID.String(),
			ExpectedOutputs:    []string{"child_report"},
			AcceptanceCriteria: []string{"child_report"},
			DependsOn:          []string{"root"},
		})
		payload.Tasks[0].DependsOn = []string{"child"}

		result := ValidatePlanRevisionPayload(payload)

		require.False(t, result.Acceptable)
		require.Contains(t, result.Errors, "cycle_detected")
		require.Len(t, result.PlanFingerprint, 64)
	})
}

func TestValidatePlanRevisionPayloadHighRiskRequiresReviewButRemainsAcceptable(t *testing.T) {
	payload := validPlanRevisionPayload(uuid.New())
	payload.Tasks[0].RiskLevel = "critical"

	result := ValidatePlanRevisionPayload(payload)

	require.True(t, result.Acceptable)
	require.True(t, result.ReviewRequired)
	require.Contains(t, result.ReviewReasons, "high_risk_task:root")
	require.Len(t, result.PlanFingerprint, 64)
}

func TestValidatePlanRevisionPayloadRejectsMissingStructuralRequirements(t *testing.T) {
	payload := validPlanRevisionPayload(uuid.New())
	payload.FinalSummaryContract.RequiredSections = []string{"conclusion", "evidence"}
	payload.Tasks[0].SelectedEmployeeID = uuid.Nil.String()
	payload.Tasks[0].ExpectedOutputs = nil
	payload.Tasks[0].AcceptanceCriteria = nil

	result := ValidatePlanRevisionPayload(payload)

	require.False(t, result.Acceptable)
	require.Contains(t, result.Errors, "missing_final_summary_required_section:risks")
	require.Contains(t, result.Errors, "missing_final_summary_required_section:next_steps")
	require.Contains(t, result.Errors, "missing_selected_employee_id:root")
	require.Contains(t, result.Errors, "missing_expected_outputs:root")
	require.Contains(t, result.Errors, "missing_acceptance_criteria:root")
	require.Len(t, result.PlanFingerprint, 64)
}

func TestValidatePlanRevisionPayloadRejectsMalformedEmployeeIDAndNonCanonicalKeys(t *testing.T) {
	payload := validPlanRevisionPayload(uuid.New())
	payload.Tasks[0].PlannedTaskKey = " root "
	payload.Tasks[0].SelectedEmployeeID = "not-a-uuid"
	payload.Tasks = append(payload.Tasks, PlanRevisionTask{
		PlannedTaskKey:     "child",
		Title:              "Child",
		Objective:          "Child",
		SelectedEmployeeID: uuid.NewString(),
		ExpectedOutputs:    []string{"child_report"},
		AcceptanceCriteria: []string{"child_report"},
		DependsOn:          []string{" root "},
	})

	result := ValidatePlanRevisionPayload(payload)

	require.False(t, result.Acceptable)
	require.Contains(t, result.Errors, "non_canonical_planned_task_key:root")
	require.Contains(t, result.Errors, "invalid_selected_employee_id:root")
	require.Contains(t, result.Errors, "non_canonical_dependency:child->root")
}

func TestCanonicalPlanFingerprintPreservesArbitraryContractArrayOrder(t *testing.T) {
	payload := validPlanRevisionPayload(uuid.New())
	payload.Tasks[0].HandoffContract = map[string]any{"ordered_steps": []any{"build", "deploy"}}
	first, err := CanonicalPlanFingerprint(payload)
	require.NoError(t, err)

	payload.Tasks[0].HandoffContract = map[string]any{"ordered_steps": []any{"deploy", "build"}}
	second, err := CanonicalPlanFingerprint(payload)
	require.NoError(t, err)

	require.NotEqual(t, first, second)
}

func TestValidatePlanRevisionPayloadDomainStatusHelpers(t *testing.T) {
	require.True(t, projectmodel.CanTransitionPlanRevisionStatus(projectmodel.PlanRevisionStatusDraft, projectmodel.PlanRevisionStatusPendingReview))
	require.True(t, projectmodel.CanTransitionPlanRevisionStatus(projectmodel.PlanRevisionStatusPendingReview, projectmodel.PlanRevisionStatusAccepted))
	require.True(t, projectmodel.CanTransitionPlanRevisionStatus(projectmodel.PlanRevisionStatusDecomposed, projectmodel.PlanRevisionStatusDecomposed))
	require.False(t, projectmodel.CanTransitionPlanRevisionStatus(projectmodel.PlanRevisionStatusAccepted, projectmodel.PlanRevisionStatusRejected))

	require.True(t, projectmodel.IsAcceptedPlanRevisionStatus(projectmodel.PlanRevisionStatusAccepted))
	require.True(t, projectmodel.IsAcceptedPlanRevisionStatus(projectmodel.PlanRevisionStatusDecomposing))
	require.True(t, projectmodel.IsAcceptedPlanRevisionStatus(projectmodel.PlanRevisionStatusDecomposed))
	require.False(t, projectmodel.IsAcceptedPlanRevisionStatus(projectmodel.PlanRevisionStatusPendingReview))

	require.True(t, projectmodel.IsMutablePlanRevisionStatus(projectmodel.PlanRevisionStatusDraft))
	require.True(t, projectmodel.IsMutablePlanRevisionStatus(projectmodel.PlanRevisionStatusValidationFailed))
	require.True(t, projectmodel.IsMutablePlanRevisionStatus(projectmodel.PlanRevisionStatusPendingReview))
	require.False(t, projectmodel.IsMutablePlanRevisionStatus(projectmodel.PlanRevisionStatusAccepted))

	require.Equal(t, "approved", projectmodel.PlanReviewDecisionAccept)
	require.Equal(t, "rejected", projectmodel.PlanReviewDecisionReject)
	require.Equal(t, "request_changes", projectmodel.PlanReviewDecisionRequestChanges)
	require.Equal(t, "cancelled", projectmodel.PlanReviewDecisionCancel)
	require.Equal(t, "in_flight", projectmodel.PlanDecompositionClaimStatusInFlight)
	require.Equal(t, "completed", projectmodel.PlanDecompositionClaimStatusCompleted)
	require.Equal(t, "failed", projectmodel.PlanDecompositionClaimStatusFailed)
}

func validPlanRevisionPayload(employeeID uuid.UUID) PlanRevisionPayload {
	return PlanRevisionPayload{
		Summary: "Valid canonical plan",
		HumanReview: PlanRevisionHumanReview{
			Required: false,
			Reasons:  []string{},
		},
		Tasks: []PlanRevisionTask{{
			PlannedTaskKey:           "root",
			Title:                    "Root",
			Objective:                "Root objective",
			TaskType:                 "analysis",
			SelectedEmployeeID:       employeeID.String(),
			ExpectedOutputs:          []string{"analysis_report"},
			AcceptanceCriteria:       []string{"analysis_report"},
			VerificationRequirements: []string{"go test"},
		}},
		FinalSummaryContract: PlanRevisionFinalSummaryContract{
			RequiredSections: []string{"conclusion", "evidence", "risks", "next_steps"},
		},
	}
}

func TestBuildPlanRevisionPayloadCarriesTemplateKey(t *testing.T) {
	plan := RouteDecisionPlan{
		Reason:      "template driven",
		TemplateKey: "ops_analysis",
	}
	payload := BuildPlanRevisionPayload(plan)
	require.Equal(t, "ops_analysis", payload.TemplateKey)

	encoded, err := json.Marshal(payload)
	require.NoError(t, err)
	require.Contains(t, string(encoded), `"template_key":"ops_analysis"`)
}

func TestCanonicalPlanFingerprintStableWithoutTemplateKey(t *testing.T) {
	payload := BuildPlanRevisionPayload(RouteDecisionPlan{Reason: "no template"})
	fingerprint, err := CanonicalPlanFingerprint(payload)
	require.NoError(t, err)

	// omitempty: an empty template key must not change historical fingerprints.
	canonical := canonicalPlanRevisionPayload(payload)
	encoded, err := json.Marshal(canonical)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "template_key")
	require.NotEmpty(t, fingerprint)
}
