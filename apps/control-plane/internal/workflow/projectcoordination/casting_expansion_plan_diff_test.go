package projectcoordination

import "testing"

func TestEvaluateCastingExpansionPlanDiff_OnlyNewExpansionTasksOK(t *testing.T) {
	old := PlanRevisionPayload{
		ExitDeliverable: "root_cause",
		Tasks: []PlanRevisionTask{
			{PlannedTaskKey: "diag", SelectedEmployeeID: "emp-diag"},
		},
	}
	newPlan := PlanRevisionPayload{
		ExitDeliverable: "root_cause",
		Tasks: []PlanRevisionTask{
			{PlannedTaskKey: "diag", SelectedEmployeeID: "emp-diag"},
			{PlannedTaskKey: "fix", SelectedEmployeeID: "emp-op"},
		},
	}
	got := EvaluateCastingExpansionPlanDiff(old, newPlan, "emp-op")
	if got.Overbound {
		t.Fatalf("expected in-bounds, got overbound %#v", got.Reasons)
	}
}

func TestEvaluateCastingExpansionPlanDiff_ExitChangeOverbound(t *testing.T) {
	old := PlanRevisionPayload{ExitDeliverable: "root_cause", Tasks: []PlanRevisionTask{{PlannedTaskKey: "diag", SelectedEmployeeID: "a"}}}
	newPlan := PlanRevisionPayload{ExitDeliverable: "fix_record", Tasks: []PlanRevisionTask{{PlannedTaskKey: "diag", SelectedEmployeeID: "a"}}}
	got := EvaluateCastingExpansionPlanDiff(old, newPlan, "emp-op")
	if !got.Overbound {
		t.Fatal("expected overbound on exit change")
	}
}

func TestEvaluateCastingExpansionPlanDiff_ReassignOverbound(t *testing.T) {
	old := PlanRevisionPayload{Tasks: []PlanRevisionTask{{PlannedTaskKey: "diag", SelectedEmployeeID: "a"}}}
	newPlan := PlanRevisionPayload{Tasks: []PlanRevisionTask{{PlannedTaskKey: "diag", SelectedEmployeeID: "b"}}}
	got := EvaluateCastingExpansionPlanDiff(old, newPlan, "emp-op")
	if !got.Overbound {
		t.Fatal("expected overbound on reassignment")
	}
}

func TestEvaluateCastingExpansionPlanDiff_UnrelatedNewTaskOverbound(t *testing.T) {
	old := PlanRevisionPayload{Tasks: []PlanRevisionTask{{PlannedTaskKey: "diag", SelectedEmployeeID: "a"}}}
	newPlan := PlanRevisionPayload{Tasks: []PlanRevisionTask{
		{PlannedTaskKey: "diag", SelectedEmployeeID: "a"},
		{PlannedTaskKey: "extra", SelectedEmployeeID: "other"},
	}}
	got := EvaluateCastingExpansionPlanDiff(old, newPlan, "emp-op")
	if !got.Overbound {
		t.Fatal("expected overbound for unrelated new task")
	}
}
