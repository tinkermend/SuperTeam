package project

import "strings"

// ProjectTaskPortfolioBucket is the exclusive display bucket for a project task
// on the portfolio home and overview composition bar.
//
// Priority (first match wins) matches SQL function project_task_portfolio_bucket
// (migration 20260811180000) — keep them in lockstep.
type ProjectTaskPortfolioBucket string

const (
	PortfolioBucketCancelled     ProjectTaskPortfolioBucket = "cancelled"
	PortfolioBucketCompleted     ProjectTaskPortfolioBucket = "completed"
	PortfolioBucketFailed        ProjectTaskPortfolioBucket = "failed"
	PortfolioBucketBlocked       ProjectTaskPortfolioBucket = "blocked"
	PortfolioBucketWaitingHuman  ProjectTaskPortfolioBucket = "waiting_human"
	PortfolioBucketRunning       ProjectTaskPortfolioBucket = "running"
	PortfolioBucketQueued        ProjectTaskPortfolioBucket = "queued"
	PortfolioBucketPending       ProjectTaskPortfolioBucket = "pending"
	PortfolioBucketOther         ProjectTaskPortfolioBucket = "other"
)

// ProjectTaskPortfolioCounts is the exclusive task-state composition for one
// project (or the tenant active-task summary). All fields are required zeros.
//
// Invariant: Total == Pending+Queued+Running+WaitingHuman+Blocked+Failed+Completed+Cancelled+Other
// When the invariant fails at the service boundary, Total is corrected to the
// sum and CountsDegraded is set (see §9).
type ProjectTaskPortfolioCounts struct {
	Total         int `json:"total"`
	Pending       int `json:"pending"`
	Queued        int `json:"queued"`
	Running       int `json:"running"`
	WaitingHuman  int `json:"waiting_human"`
	Blocked       int `json:"blocked"`
	Failed        int `json:"failed"`
	Completed     int `json:"completed"`
	Cancelled     int `json:"cancelled"`
	Other         int `json:"other"`
}

// Sum returns the sum of exclusive display buckets (excludes Total).
func (c ProjectTaskPortfolioCounts) Sum() int {
	return c.Pending + c.Queued + c.Running + c.WaitingHuman +
		c.Blocked + c.Failed + c.Completed + c.Cancelled + c.Other
}

// EnsureInvariant sets Total to Sum() when they disagree and returns whether
// degradation was required.
func (c *ProjectTaskPortfolioCounts) EnsureInvariant() (degraded bool) {
	sum := c.Sum()
	if c.Total != sum {
		c.Total = sum
		return true
	}
	return false
}

// terminalForWaitingApproval matches ListProjectRunSummaries: a task with
// requires_human_approval only enters waiting_human when not already in a
// failed/blocked/completed/cancelled terminal family.
var terminalForWaitingApproval = map[string]struct{}{
	"cancelled": {},
	"completed": {},
	"done":      {},
	"success":   {},
	"failed":    {},
	"error":     {},
	"blocked":   {},
}

// ClassifyProjectTaskPortfolioBucket maps a project_tasks row into exactly one
// exclusive display bucket. Defensive aliases (in_progress, done, …) are
// recorded for alignment with web status sets; they are not "narrow-bucket
// fixes" — project_tasks writers do not currently emit most of them.
func ClassifyProjectTaskPortfolioBucket(status string, requiresHumanApproval bool) ProjectTaskPortfolioBucket {
	s := strings.ToLower(strings.TrimSpace(status))
	switch s {
	case "cancelled":
		return PortfolioBucketCancelled
	case "completed", "done", "success":
		return PortfolioBucketCompleted
	case "failed", "error":
		return PortfolioBucketFailed
	case "blocked":
		return PortfolioBucketBlocked
	case "waiting_human", "pending_human", "pending_review", "approval_required":
		return PortfolioBucketWaitingHuman
	}

	if requiresHumanApproval {
		if _, terminal := terminalForWaitingApproval[s]; !terminal {
			return PortfolioBucketWaitingHuman
		}
	}

	switch s {
	case "running", "in_progress":
		return PortfolioBucketRunning
	case "queued":
		return PortfolioBucketQueued
	case "pending", "planned", "assigned":
		return PortfolioBucketPending
	default:
		return PortfolioBucketOther
	}
}

// isActiveTasksGateStatus reports whether a status is counted in ActiveTasks
// (archive gate). FROZEN per §5.2.1:
//
//	status NOT IN ('completed','done','success','failed','cancelled')
//
// Note: blocked and error count as active under this definition. Display
// buckets must never be used to derive ActiveTasks.
func isActiveTasksGateStatus(status string) bool {
	s := strings.ToLower(strings.TrimSpace(status))
	switch s {
	case "completed", "done", "success", "failed", "cancelled":
		return false
	default:
		return true
	}
}
