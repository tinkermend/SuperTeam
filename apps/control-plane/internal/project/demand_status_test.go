package project

import "testing"

func TestProjectDemandStatusCanAdvance(t *testing.T) {
	cases := []struct {
		name    string
		current ProjectDemandStatus
		next    ProjectDemandStatus
		want    bool
	}{
		{"intake to planned", ProjectDemandStatusPlanningPending, ProjectDemandStatusPlanned, true},
		{"planned to executing", ProjectDemandStatusPlanned, ProjectDemandStatusExecuting, true},
		{"executing to completed", ProjectDemandStatusExecuting, ProjectDemandStatusCompleted, true},
		{"executing to failed", ProjectDemandStatusExecuting, ProjectDemandStatusFailed, true},
		{"planning_pending straight to completed", ProjectDemandStatusPlanningPending, ProjectDemandStatusCompleted, true},
		{"no self re-apply on executing", ProjectDemandStatusExecuting, ProjectDemandStatusExecuting, false},
		{"no regress completed to executing", ProjectDemandStatusCompleted, ProjectDemandStatusExecuting, false},
		{"no regress executing to planned", ProjectDemandStatusExecuting, ProjectDemandStatusPlanned, false},
		{"no terminal to terminal flip", ProjectDemandStatusCompleted, ProjectDemandStatusFailed, false},
		{"submitted ranks as intake", ProjectDemandStatusSubmitted, ProjectDemandStatusPlanned, true},
		{"executing to acceptance_pending", ProjectDemandStatusExecuting, ProjectDemandStatusAcceptancePending, true},
		{"acceptance_pending to completed", ProjectDemandStatusAcceptancePending, ProjectDemandStatusCompleted, true},
		{"acceptance_pending to failed", ProjectDemandStatusAcceptancePending, ProjectDemandStatusFailed, true},
		{"no regress acceptance_pending to executing", ProjectDemandStatusAcceptancePending, ProjectDemandStatusExecuting, false},
		{"no self re-apply on acceptance_pending", ProjectDemandStatusAcceptancePending, ProjectDemandStatusAcceptancePending, false},
		{"planning_pending straight to acceptance_pending", ProjectDemandStatusPlanningPending, ProjectDemandStatusAcceptancePending, true},
		{"planning_pending to planning_failed", ProjectDemandStatusPlanningPending, ProjectDemandStatusPlanningFailed, true},
		{"no regress planning_failed to planning_pending via Advance", ProjectDemandStatusPlanningFailed, ProjectDemandStatusPlanningPending, false},
		{"planning_failed to cancelled via Advance", ProjectDemandStatusPlanningFailed, ProjectDemandStatusCancelled, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ProjectDemandStatusCanAdvance(tc.current, tc.next); got != tc.want {
				t.Fatalf("CanAdvance(%s, %s) = %v, want %v", tc.current, tc.next, got, tc.want)
			}
		})
	}
}

func TestDeriveDemandStatusFromTaskCounts(t *testing.T) {
	cases := []struct {
		name       string
		counts     demandTaskStatusCounts
		want       ProjectDemandStatus
		wantGated  bool
	}{
		{
			name:   "runnable keeps executing even with a failure",
			counts: demandTaskStatusCounts{Total: 2, Failed: 1, Runnable: 1},
			want:   ProjectDemandStatusExecuting,
		},
		{
			name:   "failed plus stranded blocked is demand failed",
			counts: demandTaskStatusCounts{Total: 2, Failed: 1, Blocked: 1},
			want:   ProjectDemandStatusFailed,
		},
		{
			name:   "only failed is demand failed",
			counts: demandTaskStatusCounts{Total: 1, Failed: 1},
			want:   ProjectDemandStatusFailed,
		},
		{
			name:      "all completed goes to acceptance gate",
			counts:    demandTaskStatusCounts{Total: 1, Completed: 1},
			wantGated: true,
		},
		{
			name:   "blocked leftover without failure is promote lag",
			counts: demandTaskStatusCounts{Total: 1, Blocked: 1},
			want:   ProjectDemandStatusExecuting,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, gated := deriveDemandStatusFromTaskCounts(tc.counts)
			if gated != tc.wantGated {
				t.Fatalf("gated=%v want %v (status=%s)", gated, tc.wantGated, got)
			}
			if !tc.wantGated && got != tc.want {
				t.Fatalf("status=%s want %s", got, tc.want)
			}
		})
	}
}
