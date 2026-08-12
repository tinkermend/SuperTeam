package project

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
)

// enqueueWorkspaceDeleteOnNodes records one pending delete-confirmation row per
// runtime node. Does not touch disk (spec 2026-08-12 §5.5). Offline nodes still
// get a queue row; confirm retries when the node is back.
func (s *Service) enqueueWorkspaceDeleteOnNodes(
	ctx context.Context,
	tenantID, projectID, actorUserID uuid.UUID,
	directoryName string,
	ownership WorkspaceOwnership,
	repoSummary map[string]any,
	runtimeNodeIDs []uuid.UUID,
	reason string,
) error {
	if len(runtimeNodeIDs) == 0 {
		return nil
	}
	if ownership == "" {
		ownership = WorkspaceOwnershipPlatformManaged
	}
	var firstErr error
	for _, runtimeNodeID := range runtimeNodeIDs {
		nodeIDSnapshot := runtimeNodeID.String()
		if s.workspaceCommander != nil {
			if node, err := s.workspaceCommander.GetNodeByID(ctx, runtimeNodeID); err == nil {
				if strings.TrimSpace(node.NodeID) != "" {
					nodeIDSnapshot = strings.TrimSpace(node.NodeID)
				}
			}
		}
		req, err := s.repository.EnqueueWorkspaceDeleteRequest(ctx, EnqueueWorkspaceDeleteRequestParams{
			TenantID:       tenantID,
			ProjectID:      projectID,
			RuntimeNodeID:  runtimeNodeID,
			DirectoryName:  directoryName,
			NodeIDSnapshot: nodeIDSnapshot,
			Ownership:      ownership,
			RepoSummary:    repoSummary,
			RequestedBy:    actorUserID,
			Reason:         reason,
		})
		if err != nil {
			slog.Default().Error("workspace delete queue: enqueue failed",
				"project_id", projectID.String(),
				"runtime_node_id", runtimeNodeID.String(),
				"directory_name", directoryName,
				"error", err.Error(),
			)
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		if actorUserID != uuid.Nil {
			_ = s.repository.CreateWorkspaceDeleteAuditEvent(ctx, tenantID, actorUserID, "project.workspace_delete.requested", req, map[string]any{
				"reason": reason,
			})
		}
	}
	return firstErr
}

func projectRepoSummary(project Project) map[string]any {
	summary := map[string]any{
		"project_name": project.Name,
		"repo_status":  string(project.RepoBinding.Status),
	}
	if project.RepoBinding.Status == ProjectRepoBindingStatusBound {
		summary["repo_url"] = project.RepoBinding.URL
		summary["default_branch"] = project.RepoBinding.DefaultBranch
		if len(project.RepoBinding.Scope) > 0 {
			summary["scope"] = append([]string(nil), project.RepoBinding.Scope...)
		}
	}
	return summary
}

// projectWorkspaceOwnership returns the ownership snapshot for delete queue rows.
func projectWorkspaceOwnership(project Project) WorkspaceOwnership {
	if project.WorkspaceOwnership == WorkspaceOwnershipAttached {
		return WorkspaceOwnershipAttached
	}
	return WorkspaceOwnershipPlatformManaged
}

// ListPendingWorkspaceDeleteRequests returns the admin confirmation queue.
func (s *Service) ListPendingWorkspaceDeleteRequests(ctx context.Context, tenantID uuid.UUID) ([]WorkspaceDeleteRequest, error) {
	if tenantID == uuid.Nil {
		return nil, ErrInvalidProject
	}
	return s.repository.ListPendingWorkspaceDeleteRequests(ctx, tenantID)
}

// ListStalePendingWorkspaceDeleteRequests is used by the retention reminder
// sweep (cross-tenant).
func (s *Service) ListStalePendingWorkspaceDeleteRequests(ctx context.Context, staleBefore time.Time) ([]WorkspaceDeleteRequest, error) {
	return s.repository.ListStalePendingWorkspaceDeleteRequests(ctx, staleBefore)
}

// ResolveOrphanWorkspaceDeleteReminders closes inbox reminders for resolved
// queue items.
func (s *Service) ResolveOrphanWorkspaceDeleteReminders(ctx context.Context) error {
	return s.repository.ResolveOrphanWorkspaceDeleteReminders(ctx)
}

// ConfirmWorkspaceDelete dispatches remove_project_directory for a pending
// queue item. If the node is offline the item stays pending (no overall failure).
func (s *Service) ConfirmWorkspaceDelete(ctx context.Context, tenantID, requestID, actorUserID uuid.UUID) (*WorkspaceDeleteRequest, error) {
	if tenantID == uuid.Nil || requestID == uuid.Nil || actorUserID == uuid.Nil {
		return nil, ErrInvalidProject
	}
	pending, err := s.repository.GetWorkspaceDeleteRequest(ctx, tenantID, requestID)
	if err != nil {
		return nil, err
	}
	if pending.Status != WorkspaceDeleteRequestStatusPending {
		return nil, ErrProjectWorkspaceDeleteRequestNotFound
	}

	// Disk removal first: only mark confirmed after success so offline nodes
	// remain pending for retry (spec §5.5).
	if err := s.removeProjectDirectoriesOnNodes(ctx, tenantID, pending.ProjectID, pending.DirectoryName, []uuid.UUID{pending.RuntimeNodeID}); err != nil {
		return nil, fmt.Errorf("%w: 节点 %s 上删除目录 %q 失败(离线则保持待确认,可稍后重试): %v",
			ErrProjectWorkspaceProvision, pending.NodeIDSnapshot, pending.DirectoryName, err)
	}

	confirmed, err := s.repository.ConfirmWorkspaceDeleteRequest(ctx, tenantID, requestID, actorUserID)
	if err != nil {
		return nil, err
	}
	_ = s.repository.CreateWorkspaceDeleteAuditEvent(ctx, tenantID, actorUserID, "project.workspace_delete.confirmed", confirmed, nil)
	return &confirmed, nil
}

// RejectWorkspaceDelete closes the queue item without deleting disk — platform
// hands off (spec §5.5 / invariant 6: no hanging orphan work items).
func (s *Service) RejectWorkspaceDelete(ctx context.Context, tenantID, requestID, actorUserID uuid.UUID, reason string) (*WorkspaceDeleteRequest, error) {
	if tenantID == uuid.Nil || requestID == uuid.Nil || actorUserID == uuid.Nil {
		return nil, ErrInvalidProject
	}
	pending, err := s.repository.GetWorkspaceDeleteRequest(ctx, tenantID, requestID)
	if err != nil {
		return nil, err
	}
	if pending.Status != WorkspaceDeleteRequestStatusPending {
		return nil, ErrProjectWorkspaceDeleteRequestNotFound
	}
	rejected, err := s.repository.RejectWorkspaceDeleteRequest(ctx, tenantID, requestID, actorUserID, strings.TrimSpace(reason))
	if err != nil {
		return nil, err
	}
	_ = s.repository.CreateWorkspaceDeleteAuditEvent(ctx, tenantID, actorUserID, "project.workspace_delete.rejected", rejected, map[string]any{
		"hand_off": true,
	})
	return &rejected, nil
}

// ensureDirectoryNameNotPendingDelete rejects create when the directory name is
// still occupied by an unconfirmed disk-delete queue item (§5.5).
func (s *Service) ensureDirectoryNameNotPendingDelete(ctx context.Context, directoryName string) error {
	count, err := s.repository.CountPendingWorkspaceDeleteByDirectoryName(ctx, directoryName)
	if err != nil {
		return err
	}
	if count > 0 {
		return fmt.Errorf("%w: 目录名 %q 有 %d 条未决删除确认", ErrProjectDirectoryNamePendingDelete, directoryName, count)
	}
	return nil
}
