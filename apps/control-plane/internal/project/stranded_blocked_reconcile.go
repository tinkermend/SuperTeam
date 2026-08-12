package project

import (
	"context"
	"errors"
	"log/slog"

	"github.com/google/uuid"
)

const strandedBlockedCancelSummary = "系统收敛：前置任务已失败或取消，下游无法继续，取消滞留阻塞任务"

// StrandedBlockedProjectTaskRepairer lists blocked tasks whose every blocker is
// already failed/cancelled, so the watchdog can cancel them the same way
// cancelFailureDownstream does on an explicit human reject.
type StrandedBlockedProjectTaskRepairer interface {
	ListStrandedBlockedProjectTasks(ctx context.Context, limit int32) ([]ProjectTask, error)
	UpdateProjectTaskStatus(ctx context.Context, tenantID, projectTaskID uuid.UUID, status string, eventID *uuid.UUID, currentStatuses []string) (ProjectTask, error)
	AppendProjectEvent(ctx context.Context, req AppendProjectEventRequest) (ProjectEvent, error)
	RecomputeProjectDemandStatus(ctx context.Context, tenantID, projectID, demandID uuid.UUID) error
}

// SweepStrandedBlockedProjectTasks cancels blocked downstream tasks whose
// upstream blockers are all terminal-failed. Without this, demand recompute
// used to treat blocked as "still working" and the demand stayed executing.
func (s *Service) SweepStrandedBlockedProjectTasks(ctx context.Context, limit int32) (int, error) {
	repairer, ok := s.repository.(StrandedBlockedProjectTaskRepairer)
	if !ok {
		return 0, nil
	}
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	tasks, err := repairer.ListStrandedBlockedProjectTasks(ctx, limit)
	if err != nil {
		return 0, err
	}
	cancelled := 0
	recompute := map[uuid.UUID]struct {
		tenantID  uuid.UUID
		projectID uuid.UUID
	}{}
	for _, task := range tasks {
		if err := s.cancelStrandedBlockedProjectTask(ctx, repairer, task); err != nil {
			if errors.Is(err, ErrProjectNotFound) || errors.Is(err, ErrProjectConflict) {
				continue
			}
			slog.Default().Warn("stranded blocked reconciler: cancel failed",
				"project_task_id", task.ID, "error", err)
			continue
		}
		cancelled++
		if task.DemandID != nil && *task.DemandID != uuid.Nil {
			recompute[*task.DemandID] = struct {
				tenantID  uuid.UUID
				projectID uuid.UUID
			}{tenantID: task.TenantID, projectID: task.ProjectID}
		}
	}
	for demandID, ids := range recompute {
		if err := repairer.RecomputeProjectDemandStatus(ctx, ids.tenantID, ids.projectID, demandID); err != nil {
			slog.Default().Warn("stranded blocked reconciler: demand recompute failed",
				"demand_id", demandID, "error", err)
		}
	}
	if cancelled > 0 {
		slog.Default().Info("stranded blocked reconciler: cancelled downstream tasks", "count", cancelled)
	}
	return cancelled, nil
}

func (s *Service) cancelStrandedBlockedProjectTask(ctx context.Context, repairer StrandedBlockedProjectTaskRepairer, task ProjectTask) error {
	event, err := repairer.AppendProjectEvent(ctx, AppendProjectEventRequest{
		TenantID:     task.TenantID,
		ProjectID:    task.ProjectID,
		EventType:    ProjectEventTaskCancelled,
		ActorType:    "system",
		ActorID:      "stranded-blocked-reconciler",
		ResourceType: strPtr("project_task"),
		ResourceID:   strPtr(task.ID.String()),
		Summary:      strandedBlockedCancelSummary,
		Payload: map[string]any{
			"project_task_id": task.ID.String(),
			"repair":          "stranded_blocked_downstream",
			"prior_status":    task.Status,
		},
	})
	if err != nil {
		return err
	}
	updated, err := repairer.UpdateProjectTaskStatus(ctx, task.TenantID, task.ID, ProjectTaskStatusCancelled, &event.ID, []string{ProjectTaskStatusBlocked, "planned", "pending"})
	if err != nil {
		if errors.Is(err, ErrProjectNotFound) {
			return ErrProjectConflict
		}
		return err
	}
	// Best-effort: cancelled 终态也采一次工作区 git，与 attempt fail/complete 路径对齐。
	s.maybeSampleWorkspaceGitOnTaskTerminal(ctx, task.TenantID, task.ProjectID, updated)
	return nil
}
