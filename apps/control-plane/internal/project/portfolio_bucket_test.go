package project

import "testing"

func TestClassifyProjectTaskPortfolioBucketExclusive(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		status   string
		requires bool
		want     ProjectTaskPortfolioBucket
	}{
		{"cancelled", "cancelled", false, PortfolioBucketCancelled},
		{"completed", "completed", false, PortfolioBucketCompleted},
		{"done defensive", "done", false, PortfolioBucketCompleted},
		{"success defensive", "success", false, PortfolioBucketCompleted},
		{"failed", "failed", false, PortfolioBucketFailed},
		{"error defensive", "error", false, PortfolioBucketFailed},
		{"blocked", "blocked", false, PortfolioBucketBlocked},
		{"waiting_human", "waiting_human", false, PortfolioBucketWaitingHuman},
		{"pending_human defensive", "pending_human", false, PortfolioBucketWaitingHuman},
		{"pending_review defensive", "pending_review", false, PortfolioBucketWaitingHuman},
		{"approval_required defensive", "approval_required", false, PortfolioBucketWaitingHuman},
		// requires_human_approval elevates non-terminal statuses
		{"pending+approval", "pending", true, PortfolioBucketWaitingHuman},
		{"planned+approval", "planned", true, PortfolioBucketWaitingHuman},
		{"assigned+approval", "assigned", true, PortfolioBucketWaitingHuman},
		{"queued+approval", "queued", true, PortfolioBucketWaitingHuman},
		{"running+approval", "running", true, PortfolioBucketWaitingHuman},
		// terminal family not elevated by approval flag
		{"failed+approval still failed", "failed", true, PortfolioBucketFailed},
		{"blocked+approval still blocked", "blocked", true, PortfolioBucketBlocked},
		{"completed+approval still completed", "completed", true, PortfolioBucketCompleted},
		{"cancelled+approval still cancelled", "cancelled", true, PortfolioBucketCancelled},
		{"error+approval still failed", "error", true, PortfolioBucketFailed},
		// plain execution buckets
		{"running", "running", false, PortfolioBucketRunning},
		{"in_progress defensive", "in_progress", false, PortfolioBucketRunning},
		{"queued", "queued", false, PortfolioBucketQueued},
		{"pending", "pending", false, PortfolioBucketPending},
		{"planned", "planned", false, PortfolioBucketPending},
		{"assigned", "assigned", false, PortfolioBucketPending},
		{"unknown → other", "weird_status", false, PortfolioBucketOther},
		{"empty → other", "", false, PortfolioBucketOther},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := ClassifyProjectTaskPortfolioBucket(tc.status, tc.requires)
			if got != tc.want {
				t.Fatalf("status=%q requires=%v: got %s want %s", tc.status, tc.requires, got, tc.want)
			}
		})
	}
}

func TestActiveTasksGateFrozenIncludesBlockedAndError(t *testing.T) {
	t.Parallel()
	// §5.2.1: blocked and error remain active for archive gate.
	for _, status := range []string{"blocked", "error", "running", "waiting_human", "queued", "pending", "planned"} {
		if !isActiveTasksGateStatus(status) {
			t.Fatalf("%s must count as ActiveTasks (gate frozen)", status)
		}
	}
	for _, status := range []string{"completed", "done", "success", "failed", "cancelled"} {
		if isActiveTasksGateStatus(status) {
			t.Fatalf("%s must NOT count as ActiveTasks", status)
		}
	}
}

func TestPortfolioCountsInvariant(t *testing.T) {
	t.Parallel()
	c := ProjectTaskPortfolioCounts{
		Total: 10, Pending: 1, Queued: 1, Running: 1, WaitingHuman: 1,
		Blocked: 1, Failed: 1, Completed: 1, Cancelled: 1, Other: 1,
	}
	if c.Sum() != 9 {
		t.Fatalf("sum=%d", c.Sum())
	}
	if !c.EnsureInvariant() {
		t.Fatal("expected degraded when total disagrees")
	}
	if c.Total != 9 {
		t.Fatalf("total corrected to %d", c.Total)
	}
	if c.EnsureInvariant() {
		t.Fatal("second pass should not degrade")
	}
}
