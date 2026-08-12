package project

import (
	"context"
	"errors"
	"log/slog"
	"strings"

	"github.com/google/uuid"
)

const orphanDecisionInboxCancelComment = "系统收敛：决策仍待处理但收件箱投影缺失，关联对象已终态或不可再处理"

// DecisionInboxProjectionReconciler lists pending decisions that have no open
// inbox projection so the watchdog can reproject or cancel them.
type DecisionInboxProjectionReconciler interface {
	ListPendingDecisionsMissingOpenInbox(ctx context.Context, limit int32) ([]DecisionRequest, error)
	ResolveDecisionRequest(ctx context.Context, req ResolveDecisionRequestRepositoryRequest) (DecisionRequest, error)
	GetProjectTask(ctx context.Context, tenantID, projectTaskID uuid.UUID) (ProjectTask, error)
	GetProjectDemand(ctx context.Context, tenantID, demandID uuid.UUID) (ProjectDemand, error)
	GetProjectEvent(ctx context.Context, tenantID, projectID, eventID uuid.UUID) (ProjectEvent, error)
}

// SweepOrphanDecisionInboxProjections heals pending decision SoT rows that have
// no open inbox card: reproject when still actionable, cancel when stale.
// Returns the number of decisions healed (reprojected or cancelled).
func (s *Service) SweepOrphanDecisionInboxProjections(ctx context.Context, limit int32) (int, error) {
	reconciler, ok := s.repository.(DecisionInboxProjectionReconciler)
	if !ok {
		return 0, nil
	}
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	orphans, err := reconciler.ListPendingDecisionsMissingOpenInbox(ctx, limit)
	if err != nil {
		return 0, err
	}
	healed := 0
	for _, decision := range orphans {
		action, healErr := s.healOrphanDecisionInboxProjection(ctx, reconciler, decision)
		if healErr != nil {
			if errors.Is(healErr, ErrProjectNotFound) || errors.Is(healErr, ErrProjectConflict) {
				continue
			}
			slog.Default().Warn("decision inbox projection reconciler: heal failed",
				"decision_request_id", decision.ID,
				"decision_type", decision.DecisionType,
				"error", healErr)
			continue
		}
		if action == "" {
			continue
		}
		healed++
		slog.Default().Info("decision inbox projection reconciler: healed",
			"decision_request_id", decision.ID,
			"decision_type", decision.DecisionType,
			"action", action)
	}
	if healed > 0 {
		slog.Default().Info("decision inbox projection reconciler: healed decisions", "count", healed)
	}
	return healed, nil
}

func (s *Service) healOrphanDecisionInboxProjection(ctx context.Context, reconciler DecisionInboxProjectionReconciler, decision DecisionRequest) (string, error) {
	if !isPendingDecisionStatus(decision.StatusSnapshot) {
		return "", nil
	}
	if shouldCancelOrphanDecisionInbox(ctx, s, reconciler, decision) {
		return s.cancelOrphanDecisionMissingInbox(ctx, reconciler, decision)
	}
	return s.reprojectOrphanDecisionInbox(ctx, reconciler, decision)
}

func shouldCancelOrphanDecisionInbox(ctx context.Context, s *Service, reconciler DecisionInboxProjectionReconciler, decision DecisionRequest) bool {
	if decision.ProjectTaskID != nil && *decision.ProjectTaskID != uuid.Nil {
		task, err := reconciler.GetProjectTask(ctx, decision.TenantID, *decision.ProjectTaskID)
		if err != nil {
			// Missing task: decision can never be acted in task context.
			return true
		}
		if isTerminalProjectTaskStatus(task.Status) {
			return true
		}
		// Human-wait family cards only make sense while the task is waiting_human.
		// If the task moved on (running/queued/planned) without resolving the card,
		// the SoT row is stale relative to execution state.
		if isTaskHumanWaitRedispatchDecisionType(decision.DecisionType) &&
			task.Status != ProjectTaskStatusWaitingHuman {
			return true
		}
		return false
	}

	if decision.DecisionType == DecisionTypeCastingExpansion {
		payload := orphanDecisionContextPayload(ctx, s, reconciler, decision)
		if demandID := demandIDFromPayloadMap(payload); demandID != uuid.Nil {
			demand, err := reconciler.GetProjectDemand(ctx, decision.TenantID, demandID)
			if err != nil || isTerminalProjectDemandStatus(demand.Status) {
				return true
			}
		}
		// Approve path needs approval context (scenario_template_key / demand_id).
		if decision.ApprovalRequestID == uuid.Nil {
			return true
		}
		if s.approvals == nil {
			return true
		}
		if _, err := s.approvals.GetRequestContextPayload(ctx, decision.TenantID, decision.ApprovalRequestID); err != nil {
			return true
		}
		return false
	}

	return false
}

func (s *Service) reprojectOrphanDecisionInbox(ctx context.Context, reconciler DecisionInboxProjectionReconciler, decision DecisionRequest) (string, error) {
	if s.inbox == nil {
		return "", nil
	}
	decision.InboxContext = orphanDecisionContextPayload(ctx, s, reconciler, decision)
	if err := s.inbox.UpsertProjectDecisionRequest(ctx, decision); err != nil {
		return "", err
	}
	return "reprojected", nil
}

func (s *Service) cancelOrphanDecisionMissingInbox(ctx context.Context, reconciler DecisionInboxProjectionReconciler, decision DecisionRequest) (string, error) {
	resolved, err := reconciler.ResolveDecisionRequest(ctx, ResolveDecisionRequestRepositoryRequest{
		TenantID:          decision.TenantID,
		ProjectID:         decision.ProjectID,
		ID:                decision.ID,
		StatusSnapshot:    "cancelled",
		ResolutionComment: orphanDecisionInboxCancelComment,
	})
	if err != nil {
		return "", err
	}
	if s.inbox != nil {
		// Best-effort: close any non-open leftover projection shape.
		_ = s.inbox.ResolveProjectDecisionRequest(ctx, resolved)
	}
	return "cancelled", nil
}

func orphanDecisionContextPayload(ctx context.Context, s *Service, reconciler DecisionInboxProjectionReconciler, decision DecisionRequest) map[string]any {
	if len(decision.InboxContext) > 0 {
		return mapOrEmptyAny(decision.InboxContext)
	}
	if s.approvals != nil && decision.ApprovalRequestID != uuid.Nil {
		if payload, err := s.approvals.GetRequestContextPayload(ctx, decision.TenantID, decision.ApprovalRequestID); err == nil {
			return mapOrEmptyAny(payload)
		}
	}
	if decision.CreatedEventID != nil && *decision.CreatedEventID != uuid.Nil {
		event, err := reconciler.GetProjectEvent(ctx, decision.TenantID, decision.ProjectID, *decision.CreatedEventID)
		if err == nil {
			return mapOrEmptyAny(event.Payload)
		}
	}
	out := map[string]any{}
	if strings.TrimSpace(decision.DecisionType) != "" {
		out["decision_type"] = decision.DecisionType
	}
	return out
}

func demandIDFromPayloadMap(payload map[string]any) uuid.UUID {
	raw := strings.TrimSpace(stringFromAny(payload["demand_id"]))
	if raw == "" {
		return uuid.Nil
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		return uuid.Nil
	}
	return id
}

// compensateDecisionInboxProjectionFailure cancels a just-created decision when
// the inbox upsert fails after the SoT write committed. Prevents permanent
// project-UI pending / inbox-empty splits on create paths that cannot roll back
// the decision insert (casting_expansion and similar).
func (s *Service) compensateDecisionInboxProjectionFailure(ctx context.Context, decision DecisionRequest, projectionErr error) error {
	if decision.ID == uuid.Nil {
		return projectionErr
	}
	resolved, err := s.repository.ResolveDecisionRequest(ctx, ResolveDecisionRequestRepositoryRequest{
		TenantID:          decision.TenantID,
		ProjectID:         decision.ProjectID,
		ID:                decision.ID,
		StatusSnapshot:    "cancelled",
		ResolutionComment: "系统回滚：收件箱投影失败，取消未投影的决策以免项目侧悬挂待处理",
	})
	if err != nil {
		slog.Default().Error("decision inbox projection compensate: cancel decision failed",
			"decision_request_id", decision.ID,
			"projection_error", projectionErr,
			"cancel_error", err)
		return projectionErr
	}
	if s.inbox != nil {
		_ = s.inbox.ResolveProjectDecisionRequest(ctx, resolved)
	}
	return projectionErr
}
