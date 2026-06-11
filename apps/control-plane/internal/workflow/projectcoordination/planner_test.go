package projectcoordination

import (
	"testing"

	"github.com/google/uuid"
)

func TestPlanDemandRouteSelectsOnlyActiveExecutorPoolMembers(t *testing.T) {
	employeeID := uuid.New()
	reviewerID := uuid.New()
	snapshot := CoordinationSnapshot{
		ProjectID: uuid.New(),
		Demand: DemandSnapshot{
			ID:      uuid.New(),
			Title:   "补充回归证据",
			Content: "整理日志并给出结论",
		},
		DigitalEmployeePool: []ProjectMemberSnapshot{
			{PrincipalID: employeeID, ProjectRole: "executor", Status: "active", DisplayName: "执行员工"},
			{PrincipalID: reviewerID, ProjectRole: "reviewer", Status: "active", DisplayName: "复核员工"},
		},
	}

	decision, err := PlanDemandRoute(snapshot)
	if err != nil {
		t.Fatalf("plan demand route: %v", err)
	}
	if len(decision.SelectedDigitalEmployeeIDs) != 1 || decision.SelectedDigitalEmployeeIDs[0] != employeeID {
		t.Fatalf("expected only executor selected, got %#v", decision.SelectedDigitalEmployeeIDs)
	}
	if decision.RequiresHumanReview {
		t.Fatalf("ordinary demand should not require human review")
	}
}

func TestPlanDemandRouteRequiresHumanReviewWhenPolicySaysSo(t *testing.T) {
	employeeID := uuid.New()
	decision, err := PlanDemandRoute(CoordinationSnapshot{
		ProjectID: uuid.New(),
		Demand: DemandSnapshot{
			ID:      uuid.New(),
			Title:   "删除生产数据",
			Content: "需要先确认风险",
		},
		DigitalEmployeePool: []ProjectMemberSnapshot{
			{PrincipalID: employeeID, ProjectRole: "executor", Status: "active"},
		},
		CoordinationPolicy: map[string]any{"require_human_review_for_new_demands": true},
	})
	if err != nil {
		t.Fatalf("plan demand route: %v", err)
	}
	if !decision.RequiresHumanReview {
		t.Fatal("expected policy to require human review")
	}
}

func TestValidateRouteDecisionRejectsOutOfPoolSelection(t *testing.T) {
	poolID := uuid.New()
	decision := RouteDecisionPlan{
		CandidateDigitalEmployeeIDs: []uuid.UUID{poolID},
		SelectedDigitalEmployeeIDs:  []uuid.UUID{uuid.New()},
		Reason:                      "错误选择",
		ExpectedOutputs:             []string{"执行摘要"},
	}
	err := ValidateRouteDecision(decision, []uuid.UUID{poolID})
	if err == nil {
		t.Fatal("expected out-of-pool route decision to fail validation")
	}
}
