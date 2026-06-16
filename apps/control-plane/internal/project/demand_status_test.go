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
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ProjectDemandStatusCanAdvance(tc.current, tc.next); got != tc.want {
				t.Fatalf("CanAdvance(%s, %s) = %v, want %v", tc.current, tc.next, got, tc.want)
			}
		})
	}
}
