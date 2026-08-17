package projectcoordination

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestAttributePlannerSelfReportedRisk(t *testing.T) {
	employeeID := uuid.New()
	plan := RouteDecisionPlan{
		RequiresHumanReview: true,
		Tasks: []PlannedTask{
			{Key: "t1", SelectedEmployeeID: employeeID, RequiresHumanApproval: true, RiskLevel: "high"},
			{Key: "t2", SelectedEmployeeID: employeeID, RiskLevel: "low"},
		},
	}
	attributePlannerSelfReportedRisk(&plan)

	require.Equal(t, RiskSourcePlannerSelfReported, riskAttributionSource(&plan, RiskSignalPlanRequiresHumanReview, ""))
	require.Equal(t, RiskSourcePlannerSelfReported, riskAttributionSource(&plan, RiskSignalTaskRequiresHumanApproval, "t1"))
	require.Equal(t, RiskSourcePlannerSelfReported, riskAttributionSource(&plan, RiskSignalTaskRiskLevel, "t1"))
	require.Empty(t, riskAttributionSource(&plan, RiskSignalTaskRiskLevel, "t2"))
}

func TestPlatformRiskWritersOverridePlannerSelfReport(t *testing.T) {
	employeeID := uuid.New()
	plan := RouteDecisionPlan{
		RequiresHumanReview: true,
		Tasks: []PlannedTask{
			{Key: "t1", SelectedEmployeeID: employeeID, RequiresHumanApproval: true},
		},
	}
	attributePlannerSelfReportedRisk(&plan)
	require.Equal(t, RiskSourcePlannerSelfReported, riskAttributionSource(&plan, RiskSignalPlanRequiresHumanReview, ""))

	markPlanRequiresHumanReviewPlatform(&plan, RiskSourcePlatformPolicy)
	markTaskRequiresHumanApprovalPlatform(&plan, &plan.Tasks[0], RiskSourcePlatformPolicy)
	require.Equal(t, RiskSourcePlatformPolicy, riskAttributionSource(&plan, RiskSignalPlanRequiresHumanReview, ""))
	require.Equal(t, RiskSourcePlatformPolicy, riskAttributionSource(&plan, RiskSignalTaskRequiresHumanApproval, "t1"))
}

func TestApplyPlanningProfileScoresAttributesProfileScore(t *testing.T) {
	employeeID := uuid.New()
	snapshot := CoordinationSnapshot{
		DigitalEmployeePool: []ProjectMemberSnapshot{{
			PrincipalID: employeeID,
			ProjectRole: "executor",
			Status:      "active",
			PlanningProfile: &DigitalEmployeePlanningProfile{
				DigitalEmployeeID: employeeID,
				HardFailures:      []string{"employee_not_dispatchable"},
			},
		}},
	}
	plan := RouteDecisionPlan{Tasks: []PlannedTask{{Key: "t1", SelectedEmployeeID: employeeID}}}

	ApplyPlanningProfileScores(snapshot, &plan)

	require.True(t, plan.RequiresHumanReview)
	require.True(t, plan.Tasks[0].RequiresHumanApproval)
	require.Equal(t, RiskSourcePlatformProfileScore, riskAttributionSource(&plan, RiskSignalPlanRequiresHumanReview, ""))
	require.Equal(t, RiskSourcePlatformProfileScore, riskAttributionSource(&plan, RiskSignalTaskRequiresHumanApproval, "t1"))
}

func TestHumanGateAttributesTemplateGovernance(t *testing.T) {
	employeeA := uuid.New()
	employeeB := uuid.New()
	develop := planTaskWithIO("develop", nil, []string{"branch_ref", "head_commit"}, nil)
	develop.SelectedEmployeeID = employeeA
	review := planTaskWithIO("review", []string{"develop"}, []string{"review_verdict"}, []string{"head_commit"})
	review.SelectedEmployeeID = employeeB
	test := planTaskWithIO("test", []string{"develop"}, []string{"test_report"}, []string{"branch_ref"})
	test.SelectedEmployeeID = employeeB
	release := planTaskWithIO("release", []string{"review", "test"}, []string{"release_record"}, []string{"review_verdict", "test_report"})
	release.SelectedEmployeeID = employeeA
	release.RequiresHumanApproval = false
	plan := RouteDecisionPlan{
		Reason:          "full chain to release",
		TemplateKey:     "software_delivery",
		ExitDeliverable: "release_record",
		Tasks:           []PlannedTask{develop, review, test, release},
	}
	snapshot := CoordinationSnapshot{
		ScenarioTemplate:    softwareDeliveryTemplateSnapshot(t),
		DigitalEmployeePool: activeExecutorPool(employeeA, employeeB),
	}

	err := EnforceScenarioTemplateGovernance(snapshot, &plan)
	require.NoError(t, err)

	releaseTask := findTaskByKey(plan.Tasks, "release")
	require.NotNil(t, releaseTask)
	require.True(t, releaseTask.RequiresHumanApproval)
	require.Equal(t, RiskSourcePlatformTemplateGovernance, riskAttributionSource(&plan, RiskSignalTaskRequiresHumanApproval, "release"))
}

func TestDecodePlannerJSONAttributesSelfReportAndCriteria(t *testing.T) {
	employeeID := uuid.New()
	content := `{
		"reason":"demo",
		"requires_human_review":true,
		"plan_acceptance_criteria":[{"id":"c1","statement":"人类确认意图","verification_method":"human_judgment","satisfied_by":[]}],
		"tasks":[{"key":"t1","title":"T","summary":"S","selected_employee_id":"` + employeeID.String() + `","risk_level":"high","requires_human_approval":true,"selection_confidence":0.9,"expected_outputs":["o"],"input_requirements":{},"handoff_contract":{}}]
	}`
	plan, err := decodePlannerJSON(content)
	require.NoError(t, err)
	require.Equal(t, RiskSourcePlannerSelfReported, riskAttributionSource(&plan, RiskSignalPlanRequiresHumanReview, ""))
	require.Equal(t, RiskSourcePlannerSelfReported, riskAttributionSource(&plan, RiskSignalTaskRequiresHumanApproval, "t1"))
	require.Equal(t, RiskSourcePlannerSelfReported, riskAttributionSource(&plan, RiskSignalTaskRiskLevel, "t1"))
	require.Len(t, plan.PlanAcceptanceCriteria, 1)
	require.Equal(t, CriterionSourcePlannerAuthored, plan.PlanAcceptanceCriteria[0].Source)
}

func TestEnsureProducesDeliverableCriteriaInjectsPerProduce(t *testing.T) {
	plan := &RouteDecisionPlan{
		Tasks: []PlannedTask{
			{Key: "develop", Produces: []string{"branch_ref", "head_commit"}},
			{Key: "test", Produces: []string{"test_report"}},
		},
		PlanAcceptanceCriteria: []PlanAcceptanceCriterion{
			{ID: "existing", Statement: "已有判据", VerificationMethod: VerificationMethodAutomatedTest, SatisfiedBy: []string{"develop"}},
		},
	}
	ensureProducesDeliverableCriteria(plan)
	require.Len(t, plan.PlanAcceptanceCriteria, 4)
	byID := map[string]PlanAcceptanceCriterion{}
	for _, c := range plan.PlanAcceptanceCriteria {
		byID[c.ID] = c
	}
	require.Contains(t, byID, "produces_delivered:develop:branch_ref")
	require.Contains(t, byID, "produces_delivered:develop:head_commit")
	require.Contains(t, byID, "produces_delivered:test:test_report")
	require.Equal(t, CriterionSourcePlatformInjected, byID["produces_delivered:develop:branch_ref"].Source)
	require.Equal(t, []string{"develop"}, byID["produces_delivered:develop:branch_ref"].SatisfiedBy)

	// Idempotent.
	ensureProducesDeliverableCriteria(plan)
	require.Len(t, plan.PlanAcceptanceCriteria, 4)
}

func TestEnsureHumanJudgmentCriterionTagsPlatformInjected(t *testing.T) {
	plan := &RouteDecisionPlan{
		RequiresHumanReview: true,
		RiskAttribution: []RiskSignalAttribution{{
			Kind: RiskSignalPlanRequiresHumanReview, Source: RiskSourcePlatformPolicy,
		}},
		PlanAcceptanceCriteria: []PlanAcceptanceCriterion{
			{ID: "a1", Statement: "自动化测试通过", VerificationMethod: VerificationMethodAutomatedTest},
		},
	}
	normalizeCriterionDefaults(&plan.PlanAcceptanceCriteria[0])
	ensureHumanJudgmentCriterion(plan, nil)
	require.Len(t, plan.PlanAcceptanceCriteria, 2)
	require.Equal(t, CriterionSourcePlatformInjected, plan.PlanAcceptanceCriteria[1].Source)
}

func TestPlanTouchesHighRiskOnlyPlatformDerived(t *testing.T) {
	selfReport := &RouteDecisionPlan{
		RequiresHumanReview: true,
		RiskAttribution: []RiskSignalAttribution{{
			Kind: RiskSignalPlanRequiresHumanReview, Source: RiskSourcePlannerSelfReported,
		}},
	}
	require.False(t, planTouchesHighRisk(selfReport))

	platform := &RouteDecisionPlan{
		RequiresHumanReview: true,
		RiskAttribution: []RiskSignalAttribution{{
			Kind: RiskSignalPlanRequiresHumanReview, Source: RiskSourcePlatformTemplateGovernance,
		}},
	}
	require.True(t, planTouchesHighRisk(platform))

	selfReportOnly := &RouteDecisionPlan{
		Tasks: []PlannedTask{{Key: "t1", RiskLevel: "high"}},
	}
	attributePlannerSelfReportedRisk(selfReportOnly)
	require.False(t, planTouchesHighRisk(selfReportOnly), "F6: planner risk_level is display-only")
}

func TestApplyRequiredHumanReviewPolicyAttributesPolicy(t *testing.T) {
	employeeID := uuid.New()
	plan := &RouteDecisionPlan{
		Tasks: []PlannedTask{{Key: "t1", SelectedEmployeeID: employeeID}},
	}
	snapshot := CoordinationSnapshot{
		CoordinationPolicy: map[string]any{"require_human_review_for_new_demands": true},
	}
	applyRequiredHumanReviewPolicy(snapshot, plan)
	require.True(t, plan.RequiresHumanReview)
	require.True(t, plan.Tasks[0].RequiresHumanApproval)
	require.Equal(t, RiskSourcePlatformPolicy, riskAttributionSource(plan, RiskSignalPlanRequiresHumanReview, ""))
	require.Equal(t, RiskSourcePlatformPolicy, riskAttributionSource(plan, RiskSignalTaskRequiresHumanApproval, "t1"))
}
