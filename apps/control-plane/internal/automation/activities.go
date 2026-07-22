package automation

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type FireActivityInput struct {
	TenantID        uuid.UUID `json:"tenant_id"`
	RuleID          uuid.UUID `json:"rule_id"`
	ScheduledFireAt time.Time `json:"scheduled_fire_at"`
}

type Activities struct {
	service *Service
}

func NewActivities(service *Service) *Activities {
	return &Activities{service: service}
}

func (a *Activities) FireAutomationRule(ctx context.Context, input FireActivityInput) error {
	if a == nil || a.service == nil {
		return ErrInvalidInput
	}
	_, err := a.service.Fire(ctx, input.TenantID, input.RuleID, input.ScheduledFireAt)
	return err
}
