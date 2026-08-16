package project

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/google/uuid"
)

const strandedBlockedCancelSummary = "系统收敛：前置任务已失败或取消，下游无法继续，取消滞留阻塞任务"

// strandedBlockedFailureGrace 是上游进终态后的收口宽限。A 期真链现场：任务 21:10:06
// 失败、看门狗 21:10:08 就把下游取消、恢复卡 21:10:09 才建好——只看「卡是否 pending」
// 挡不住这几秒竞态。看门狗是兜底，不需要秒级回收，给协调线程留足开卡窗口。
const strandedBlockedFailureGrace = 5 * time.Minute

// StrandedBlockedProjectTaskRepairer lists blocked tasks whose every blocker is
// already failed/cancelled, so the watchdog can cancel them the same way
// cancelFailureDownstream does on an explicit human reject.
type StrandedBlockedProjectTaskRepairer interface {
	ListStrandedBlockedProjectTasks(ctx context.Context, limit int32, failureGrace time.Duration) ([]ProjectTask, error)
	CancelProjectTaskWithReason(ctx context.Context, tenantID, projectTaskID uuid.UUID, cancelReason string, eventID *uuid.UUID, currentStatuses []string) (ProjectTask, error)
	AppendProjectEvent(ctx context.Context, req AppendProjectEventRequest) (ProjectEvent, error)
	RecomputeProjectDemandStatus(ctx context.Context, tenantID, projectID, demandID uuid.UUID) error
}

// SweepStrandedBlockedProjectTasks cancels blocked downstream tasks whose
// upstream blockers are all terminal-failed. Without this, demand recompute
// used to treat blocked as "still working" and the demand stayed executing.
//
// 取消一律落 cancel_reason=system_stranded：这是平台动作而非业务判死，人类点重试时
// 这些下游会被复活重挂边（ListStrandedBlockedProjectTasks 同时排除「上游还挂着
// pending 人类决策」的任务，所以正常情况下轮不到这里去和重试抢）。
func (s *Service) SweepStrandedBlockedProjectTasks(ctx context.Context, limit int32) (int, error) {
	repairer, ok := s.repository.(StrandedBlockedProjectTaskRepairer)
	if !ok {
		return 0, nil
	}
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	tasks, err := repairer.ListStrandedBlockedProjectTasks(ctx, limit, strandedBlockedFailureGrace)
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
			"cancel_reason":   ProjectTaskCancelReasonSystemStranded,
		},
	})
	if err != nil {
		return err
	}
	updated, err := repairer.CancelProjectTaskWithReason(ctx, task.TenantID, task.ID, ProjectTaskCancelReasonSystemStranded, &event.ID, []string{ProjectTaskStatusBlocked, "planned", "pending"})
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
