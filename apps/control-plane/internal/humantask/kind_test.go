package humantask

import "testing"

func TestKindAndLayer(t *testing.T) {
	cases := []struct {
		decisionType string
		kind         string
		layer        string
	}{
		{"plan_review", "plan_review", "demand"},
		{"project_task_approval", "dispatch_release", "task"},
		{"project_task_acceptance", "downstream_release", "task"},
		{"demand_acceptance", "acceptance_sign", "demand"},
		{"project_acceptance", "closure_confirm", "project"},
		{"planning_failed", "planning_failed", "demand"},
		{"planning_gap", "planning_gap", "demand"},
		{"task_failure_recovery", "task_failure_recovery", "task"},
		{"project_task_runtime_recovery", "project_task_runtime_recovery", "task"},
		{"", "", ""},
	}
	for _, tc := range cases {
		kind, layer := KindAndLayer(tc.decisionType)
		if kind != tc.kind || layer != tc.layer {
			t.Fatalf("%q => (%q,%q), want (%q,%q)", tc.decisionType, kind, layer, tc.kind, tc.layer)
		}
	}
}
