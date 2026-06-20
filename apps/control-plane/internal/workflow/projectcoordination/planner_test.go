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
	if len(decision.Tasks) != 1 {
		t.Fatalf("expected one planned task, got %#v", decision.Tasks)
	}
	task := decision.Tasks[0]
	if task.SelectedEmployeeID != employeeID {
		t.Fatalf("expected only executor selected, got %#v", task.SelectedEmployeeID)
	}
	if len(task.BlockedByKeys) != 0 {
		t.Fatalf("expected single heuristic node with no blockers, got %#v", task.BlockedByKeys)
	}
	if len(task.ExpectedOutputs) != 3 ||
		task.ExpectedOutputs[0] != "execution_summary" ||
		task.ExpectedOutputs[1] != "evidence_refs" ||
		task.ExpectedOutputs[2] != "recommended_next_action" {
		t.Fatalf("unexpected task outputs: %#v", task.ExpectedOutputs)
	}
	if task.InputRequirements == nil || task.HandoffContract == nil {
		t.Fatalf("expected non-nil task input requirements and handoff contract, got %#v %#v", task.InputRequirements, task.HandoffContract)
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
	if len(decision.Tasks) != 1 || decision.Tasks[0].RequiresHumanApproval != decision.RequiresHumanReview {
		t.Fatalf("expected task approval flag to follow route review policy, got %#v", decision.Tasks)
	}
}

func TestPlanDemandRouteCanFanOutToAllActiveExecutorsWithoutHumanReview(t *testing.T) {
	firstEmployeeID := uuid.New()
	secondEmployeeID := uuid.New()
	reviewerID := uuid.New()
	decision, err := PlanDemandRoute(CoordinationSnapshot{
		ProjectID: uuid.New(),
		Demand: DemandSnapshot{
			ID:      uuid.New(),
			Title:   "并行验证任务链",
			Content: "请多个数字员工分别执行独立检查，并汇总各自结论。",
		},
		DigitalEmployeePool: []ProjectMemberSnapshot{
			{PrincipalID: firstEmployeeID, ProjectRole: "executor", Status: "active", DisplayName: "执行员工 A"},
			{PrincipalID: secondEmployeeID, ProjectRole: "executor", Status: "active", DisplayName: "执行员工 B"},
			{PrincipalID: reviewerID, ProjectRole: "reviewer", Status: "active", DisplayName: "复核员工"},
		},
		CoordinationPolicy: map[string]any{"dispatch_all_active_executors": true},
	})
	if err != nil {
		t.Fatalf("plan demand route: %v", err)
	}
	if decision.RequiresHumanReview {
		t.Fatal("fanout smoke route should not require human review unless policy explicitly asks for it")
	}
	if len(decision.Tasks) != 2 {
		t.Fatalf("expected one task per active executor, got %#v", decision.Tasks)
	}
	selected := map[uuid.UUID]bool{}
	for _, task := range decision.Tasks {
		selected[task.SelectedEmployeeID] = true
		if task.RequiresHumanApproval {
			t.Fatalf("fanout task should not require human approval: %#v", task)
		}
		if len(task.BlockedByKeys) != 0 {
			t.Fatalf("fanout tasks should be independently dispatchable, got blockers %#v", task.BlockedByKeys)
		}
		if task.HandoffContract["completion_path"] != "project_task_attempt_writeback" {
			t.Fatalf("expected runtime project task writeback contract, got %#v", task.HandoffContract)
		}
	}
	if !selected[firstEmployeeID] || !selected[secondEmployeeID] || selected[reviewerID] {
		t.Fatalf("unexpected selected employees: %#v", selected)
	}
	if decision.PlannerMetadata["strategy"] != "all_active_executors" {
		t.Fatalf("expected fanout planner metadata, got %#v", decision.PlannerMetadata)
	}
}

func TestValidateRouteDecisionRejectsOutOfPoolSelection(t *testing.T) {
	poolID := uuid.New()
	decision := RouteDecisionPlan{
		Reason: "错误选择",
		Tasks: []PlannedTask{{
			Key:                "execute",
			Title:              "执行",
			Summary:            "执行摘要",
			SelectedEmployeeID: uuid.New(),
			ExpectedOutputs:    []string{"执行摘要"},
			InputRequirements:  map[string]any{},
			HandoffContract:    map[string]any{},
		}},
	}
	err := ValidateRouteDecision(decision, []uuid.UUID{poolID})
	if err == nil {
		t.Fatal("expected out-of-pool route decision to fail validation")
	}
}
