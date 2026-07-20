package inbox

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/superteam/control-plane/internal/employee"
)

// RunRecoveryProjectorAdapter projects standalone digital-employee run failures
// into inbox items and resolves them when humans retry or acknowledge.
type RunRecoveryProjectorAdapter struct {
	service    *Service
	runService *employee.DigitalEmployeeRunService
}

func NewRunRecoveryProjectorAdapter(service *Service, runService *employee.DigitalEmployeeRunService) *RunRecoveryProjectorAdapter {
	return &RunRecoveryProjectorAdapter{service: service, runService: runService}
}

func (a *RunRecoveryProjectorAdapter) UpsertStandaloneRunFailure(ctx context.Context, input employee.StandaloneRunFailureInboxInput) error {
	if a == nil || a.service == nil {
		return ErrSourceUnavailable
	}
	now := time.Now().UTC()
	_, err := a.service.UpsertItem(ctx, UpsertItemRequest{
		TenantID:     input.TenantID,
		TargetUserID: input.TargetUserID,
		Scope:        "personal",
		ItemType:     ItemTypeDigitalEmployeeRunRecovery,
		SourceType:   SourceTypeDigitalEmployeeRun,
		SourceID:     input.RunID,
		Title:        input.Title,
		Summary:      input.Summary,
		RiskLevel:    "high",
		Status:       StatusOpen,
		Actions:      DefaultActions(ItemTypeDigitalEmployeeRunRecovery),
		ContextPayload: map[string]any{
			"digital_employee_id":   input.DigitalEmployeeID.String(),
			"digital_employee_name": input.EmployeeName,
			"run_id":                input.RunID.String(),
			"run_kind":              input.RunKind,
		},
		DeepLink: map[string]any{
			"route": "/employees/" + input.DigitalEmployeeID.String(),
			"run_id": input.RunID.String(),
		},
		LastActivityAt: now,
	})
	return err
}

func (a *RunRecoveryProjectorAdapter) ResolveStandaloneRunFailure(ctx context.Context, tenantID, runID, actorUserID uuid.UUID, action string) error {
	if a == nil || a.service == nil {
		return ErrSourceUnavailable
	}
	now := time.Now().UTC()
	_, err := a.service.UpsertItem(ctx, UpsertItemRequest{
		TenantID:       tenantID,
		TargetUserID:   actorUserID,
		Scope:          "personal",
		ItemType:       ItemTypeDigitalEmployeeRunRecovery,
		SourceType:     SourceTypeDigitalEmployeeRun,
		SourceID:       runID,
		Title:          "运行失败已处理",
		Summary:        action,
		Status:         StatusResolved,
		Actions:        []Action{},
		ResolvedAt:     &now,
		LastActivityAt: now,
	})
	return err
}

func (a *RunRecoveryProjectorAdapter) ResolveRunRecoveryAction(ctx context.Context, req SourceActionRequest) (SourceActionResult, error) {
	if a == nil || a.runService == nil {
		return SourceActionResult{}, ErrSourceUnavailable
	}
	run, err := a.runService.GetRunByIDForRecovery(ctx, req.TenantID, req.SourceID)
	if err != nil || run == nil {
		return SourceActionResult{}, ErrSourceUnavailable
	}
	switch req.Action {
	case "acknowledge":
		if _, err := a.runService.AcknowledgeFailedRun(ctx, req.TenantID, run.DigitalEmployeeID, run.ID, req.ActorUserID); err != nil {
			return SourceActionResult{}, normalizeSourceActionError(err)
		}
	case "retry":
		if _, err := a.runService.RetryFailedRun(ctx, req.TenantID, run.DigitalEmployeeID, run.ID, req.ActorUserID); err != nil {
			return SourceActionResult{}, normalizeSourceActionError(err)
		}
	default:
		return SourceActionResult{}, ErrInvalidAction
	}
	return SourceActionResult{
		SourceType: string(SourceTypeDigitalEmployeeRun),
		SourceID:   req.SourceID,
		Status:     req.Action,
	}, nil
}
