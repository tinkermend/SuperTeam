package automation

import (
	"context"
	"fmt"
	"time"

	enumspb "go.temporal.io/api/enums/v1"
	"go.temporal.io/sdk/client"
)

// TemporalScheduler syncs automation rules to Temporal Schedules.
// When temporal client is nil, all methods no-op so manual Trigger still works.
type TemporalScheduler struct {
	client    client.Client
	taskQueue string
}

func NewTemporalScheduler(c client.Client, taskQueue string) *TemporalScheduler {
	return &TemporalScheduler{client: c, taskQueue: taskQueue}
}

func (s *TemporalScheduler) Create(ctx context.Context, rule Rule) (string, error) {
	if s == nil || s.client == nil {
		return "", nil
	}
	scheduleID := scheduleIDFor(rule)
	spec, err := scheduleSpecFor(rule)
	if err != nil {
		return "", err
	}
	_, err = s.client.ScheduleClient().Create(ctx, client.ScheduleOptions{
		ID:   scheduleID,
		Spec: spec,
		Action: &client.ScheduleWorkflowAction{
			ID:        scheduleID + "-fire",
			Workflow:  AutomationFireWorkflow,
			Args:      []any{FireWorkflowInput{TenantID: rule.TenantID, RuleID: rule.ID}},
			TaskQueue: s.taskQueue,
		},
		Overlap: enumspb.SCHEDULE_OVERLAP_POLICY_SKIP,
		Paused:  !rule.Enabled,
		Note:    "automation rule " + rule.ID.String(),
	})
	if err != nil {
		return "", fmt.Errorf("create temporal schedule: %w", err)
	}
	return scheduleID, nil
}

func (s *TemporalScheduler) Update(ctx context.Context, rule Rule) error {
	if s == nil || s.client == nil {
		return nil
	}
	scheduleID := ""
	if rule.TemporalScheduleID != nil {
		scheduleID = *rule.TemporalScheduleID
	}
	if scheduleID == "" {
		scheduleID = scheduleIDFor(rule)
	}
	spec, err := scheduleSpecFor(rule)
	if err != nil {
		return err
	}
	handle := s.client.ScheduleClient().GetHandle(ctx, scheduleID)
	return handle.Update(ctx, client.ScheduleUpdateOptions{
		DoUpdate: func(input client.ScheduleUpdateInput) (*client.ScheduleUpdate, error) {
			schedule := input.Description.Schedule
			schedule.Spec = &spec
			schedule.Action = &client.ScheduleWorkflowAction{
				ID:        scheduleID + "-fire",
				Workflow:  AutomationFireWorkflow,
				Args:      []any{FireWorkflowInput{TenantID: rule.TenantID, RuleID: rule.ID}},
				TaskQueue: s.taskQueue,
			}
			if schedule.State == nil {
				schedule.State = &client.ScheduleState{}
			}
			schedule.State.Paused = !rule.Enabled
			return &client.ScheduleUpdate{Schedule: &schedule}, nil
		},
	})
}

func (s *TemporalScheduler) Pause(ctx context.Context, scheduleID string, note string) error {
	if s == nil || s.client == nil || scheduleID == "" {
		return nil
	}
	return s.client.ScheduleClient().GetHandle(ctx, scheduleID).Pause(ctx, client.SchedulePauseOptions{Note: note})
}

func (s *TemporalScheduler) Unpause(ctx context.Context, scheduleID string, note string) error {
	if s == nil || s.client == nil || scheduleID == "" {
		return nil
	}
	return s.client.ScheduleClient().GetHandle(ctx, scheduleID).Unpause(ctx, client.ScheduleUnpauseOptions{Note: note})
}

func (s *TemporalScheduler) Delete(ctx context.Context, scheduleID string) error {
	if s == nil || s.client == nil || scheduleID == "" {
		return nil
	}
	return s.client.ScheduleClient().GetHandle(ctx, scheduleID).Delete(ctx)
}

func scheduleIDFor(rule Rule) string {
	return fmt.Sprintf("automation-rule:%s", rule.ID.String())
}

func scheduleSpecFor(rule Rule) (client.ScheduleSpec, error) {
	tz := rule.Timezone
	if tz == "" {
		tz = DefaultTimezone
	}
	switch rule.ScheduleKind {
	case ScheduleCron:
		if rule.CronExpr == nil || *rule.CronExpr == "" {
			return client.ScheduleSpec{}, fmt.Errorf("%w: cron_expr is required", ErrInvalidInput)
		}
		return client.ScheduleSpec{
			CronExpressions: []string{*rule.CronExpr},
			TimeZoneName:    tz,
		}, nil
	case ScheduleInterval:
		if rule.IntervalSeconds == nil || *rule.IntervalSeconds < MinIntervalSeconds {
			return client.ScheduleSpec{}, fmt.Errorf("%w: interval_seconds must be >= %d", ErrInvalidInput, MinIntervalSeconds)
		}
		return client.ScheduleSpec{
			Intervals: []client.ScheduleIntervalSpec{{
				Every: time.Duration(*rule.IntervalSeconds) * time.Second,
			}},
			TimeZoneName: tz,
		}, nil
	default:
		return client.ScheduleSpec{}, fmt.Errorf("%w: unknown schedule_kind", ErrInvalidInput)
	}
}

var _ ScheduleSyncer = (*TemporalScheduler)(nil)
