package automation

import (
	"time"

	"github.com/google/uuid"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

type FireWorkflowInput struct {
	TenantID        uuid.UUID `json:"tenant_id"`
	RuleID          uuid.UUID `json:"rule_id"`
	ScheduledFireAt time.Time `json:"scheduled_fire_at,omitempty"`
}

func AutomationFireWorkflow(ctx workflow.Context, input FireWorkflowInput) error {
	ao := workflow.ActivityOptions{
		StartToCloseTimeout: 2 * time.Minute,
		RetryPolicy: &temporal.RetryPolicy{
			MaximumAttempts: 3,
		},
	}
	ctx = workflow.WithActivityOptions(ctx, ao)

	scheduledFireAt := input.ScheduledFireAt
	if scheduledFireAt.IsZero() {
		scheduledFireAt = workflow.Now(ctx).UTC()
	}

	return workflow.ExecuteActivity(ctx, (*Activities).FireAutomationRule, FireActivityInput{
		TenantID:        input.TenantID,
		RuleID:          input.RuleID,
		ScheduledFireAt: scheduledFireAt,
	}).Get(ctx, nil)
}
