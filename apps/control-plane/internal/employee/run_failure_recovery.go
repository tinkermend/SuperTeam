package employee

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/superteam/control-plane/internal/storage/queries"
)

// RunFailureInboxProjector projects standalone run failures into the human inbox.
type RunFailureInboxProjector interface {
	UpsertStandaloneRunFailure(ctx context.Context, input StandaloneRunFailureInboxInput) error
	ResolveStandaloneRunFailure(ctx context.Context, tenantID, runID, actorUserID uuid.UUID, action string) error
}

type StandaloneRunFailureInboxInput struct {
	TenantID          uuid.UUID
	TargetUserID      uuid.UUID
	RunID             uuid.UUID
	DigitalEmployeeID uuid.UUID
	EmployeeName      string
	Title             string
	Summary           string
	RunKind           string
	// ProjectID is the chat/task-hub anchor project when present; stored as
	// inbox source_project_id so project delete can cascade-cancel the card.
	ProjectID *uuid.UUID
}

func (s *DigitalEmployeeRunService) WithFailureInboxProjector(projector RunFailureInboxProjector) *DigitalEmployeeRunService {
	s.failureInbox = projector
	return s
}

func (s *DigitalEmployeeRunWritebackService) WithFailureInboxProjector(projector RunFailureInboxProjector) *DigitalEmployeeRunWritebackService {
	s.failureInbox = projector
	return s
}

// GetRunByIDForRecovery loads a run by id for inbox recovery actions (no employee id in path).
func (s *DigitalEmployeeRunService) GetRunByIDForRecovery(ctx context.Context, tenantID, runID uuid.UUID) (*DigitalEmployeeRun, error) {
	return s.repository.GetRunByID(ctx, tenantID, runID)
}

// AcknowledgeFailedRun marks a failed/timed_out standalone run as human-acknowledged
// and closes the matching inbox item so operational status can return to idle.
func (s *DigitalEmployeeRunService) AcknowledgeFailedRun(ctx context.Context, tenantID, employeeID, runID, actorUserID uuid.UUID) (*DigitalEmployeeRun, error) {
	if tenantID == uuid.Nil || employeeID == uuid.Nil || runID == uuid.Nil || actorUserID == uuid.Nil {
		return nil, fmt.Errorf("%w: tenant_id, employee_id, run_id and actor are required", ErrInvalidInput)
	}
	run, err := s.repository.GetRun(ctx, tenantID, employeeID, runID)
	if err != nil {
		return nil, err
	}
	if run == nil {
		return nil, fmt.Errorf("%w: run not found", ErrNotFound)
	}
	if run.Status != DigitalEmployeeRunStatusFailed && run.Status != DigitalEmployeeRunStatusTimedOut {
		return nil, fmt.Errorf("%w: run is not failed", ErrInvalidInput)
	}
	if err := ensureStandaloneRun(ctx, s.repository, run); err != nil {
		return nil, err
	}
	acked, err := s.repository.AcknowledgeRunFailure(ctx, tenantID, runID, actorUserID)
	if err != nil {
		return nil, err
	}
	if s.failureInbox != nil {
		_ = s.failureInbox.ResolveStandaloneRunFailure(ctx, tenantID, runID, actorUserID, "acknowledge")
	}
	return acked, nil
}

// RetryFailedRun creates a replacement run from a failed standalone run and
// closes the recovery inbox item.
func (s *DigitalEmployeeRunService) RetryFailedRun(ctx context.Context, tenantID, employeeID, runID, actorUserID uuid.UUID) (*DigitalEmployeeRun, error) {
	if tenantID == uuid.Nil || employeeID == uuid.Nil || runID == uuid.Nil || actorUserID == uuid.Nil {
		return nil, fmt.Errorf("%w: tenant_id, employee_id, run_id and actor are required", ErrInvalidInput)
	}
	prior, err := s.repository.GetRun(ctx, tenantID, employeeID, runID)
	if err != nil {
		return nil, err
	}
	if prior == nil {
		return nil, fmt.Errorf("%w: run not found", ErrNotFound)
	}
	if prior.Status != DigitalEmployeeRunStatusFailed && prior.Status != DigitalEmployeeRunStatusTimedOut {
		return nil, fmt.Errorf("%w: run is not failed", ErrInvalidInput)
	}
	if err := ensureStandaloneRun(ctx, s.repository, prior); err != nil {
		return nil, err
	}
	meta, _ := s.repository.GetRunTaskMetadata(ctx, tenantID, prior.TaskID)
	objective := objectiveFromRunMetadata(meta)
	if objective == "" {
		objective = "重试失败任务"
	}
	req := CreateDigitalEmployeeRunRequest{
		TenantID:          tenantID,
		UserID:            actorUserID,
		DigitalEmployeeID: employeeID,
		Objective:         objective,
		Prompt:            objective,
		RunKind:           prior.RunKind,
	}
	if prior.RunKind == "" {
		req.RunKind = RunKindTask
	}
	if req.RunKind == RunKindChat {
		req.ResumeOfRunID = &prior.ID
		if projectID := projectIDFromRunMetadata(meta); projectID != nil {
			req.ProjectID = projectID
		}
	}
	created, err := s.CreateRun(ctx, req)
	if err != nil && req.RunKind == RunKindChat && req.ResumeOfRunID != nil {
		req.ResumeOfRunID = nil
		created, err = s.CreateRun(ctx, req)
	}
	if err != nil {
		return nil, err
	}
	_, _ = s.repository.AcknowledgeRunFailure(ctx, tenantID, runID, actorUserID)
	if s.failureInbox != nil {
		_ = s.failureInbox.ResolveStandaloneRunFailure(ctx, tenantID, runID, actorUserID, "retry")
	}
	return created, nil
}

func ensureStandaloneRun(ctx context.Context, repo DigitalEmployeeRunRepository, run *DigitalEmployeeRun) error {
	meta, err := repo.GetRunTaskMetadata(ctx, run.TenantID, run.TaskID)
	if err != nil {
		return err
	}
	if projectTaskID, _ := meta["project_task_id"].(string); strings.TrimSpace(projectTaskID) != "" {
		return fmt.Errorf("%w: 项目任务失败请在项目详情或收件箱处理恢复", ErrInvalidInput)
	}
	return nil
}

func objectiveFromRunMetadata(meta map[string]any) string {
	if meta == nil {
		return ""
	}
	for _, key := range []string{"objective", "prompt", "title"} {
		if value, _ := meta[key].(string); strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func projectIDFromRunMetadata(meta map[string]any) *uuid.UUID {
	if meta == nil {
		return nil
	}
	for _, key := range []string{"anchor_project_id", "project_id"} {
		if text, _ := meta[key].(string); strings.TrimSpace(text) != "" {
			id, err := uuid.Parse(strings.TrimSpace(text))
			if err == nil {
				return &id
			}
		}
	}
	return nil
}

func (r *PgRunRepository) AcknowledgeRunFailure(ctx context.Context, tenantID, runID, actorUserID uuid.UUID) (*DigitalEmployeeRun, error) {
	row, err := r.q.AcknowledgeDigitalEmployeeRunFailure(ctx, queries.AcknowledgeDigitalEmployeeRunFailureParams{
		AcknowledgedBy: actorUserID,
		TenantID:       tenantID,
		RunID:          runID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			existing, getErr := r.GetRunByID(ctx, tenantID, runID)
			if getErr != nil {
				return nil, getErr
			}
			if existing != nil && existing.FailureAcknowledgedAt != nil {
				return existing, nil
			}
			return nil, fmt.Errorf("%w: run not found or not failed", ErrNotFound)
		}
		return nil, err
	}
	return digitalEmployeeRunFromQuery(row), nil
}
