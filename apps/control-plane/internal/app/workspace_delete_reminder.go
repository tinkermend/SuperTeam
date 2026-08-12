package app

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/superteam/control-plane/internal/inbox"
	"github.com/superteam/control-plane/internal/project"
)

// 工作区目录删除确认滞留催办(spec 2026-08-12 P0):
// 待确认项永不因超时自动删盘;滞留超过 staleAfter 后,周期任务向删除发起人投递收件箱催办。
// Upsert 以 (tenant, source_type, source_id=request_id) 幂等。
const (
	workspaceDeleteStaleAfter       = 7 * 24 * time.Hour
	workspaceDeleteReminderInterval = 24 * time.Hour
)

func startWorkspaceDeleteReminder(ctx context.Context, projectService *project.Service, inboxService *inbox.Service) {
	run := func() {
		if err := remindStaleWorkspaceDeleteRequests(ctx, projectService, inboxService); err != nil && ctx.Err() == nil {
			slog.Warn("workspace delete reminder sweep failed", "error", err)
		}
	}
	run()
	ticker := time.NewTicker(workspaceDeleteReminderInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			run()
		}
	}
}

func remindStaleWorkspaceDeleteRequests(ctx context.Context, projectService *project.Service, inboxService *inbox.Service) error {
	if err := projectService.ResolveOrphanWorkspaceDeleteReminders(ctx); err != nil {
		return err
	}
	stale, err := projectService.ListStalePendingWorkspaceDeleteRequests(ctx, time.Now().Add(-workspaceDeleteStaleAfter))
	if err != nil {
		return err
	}
	for _, item := range stale {
		if item.RequestedBy.String() == "" {
			continue
		}
		days := int(time.Since(item.RequestedAt).Hours() / 24)
		ownershipHint := "平台创建"
		if item.Ownership == project.WorkspaceOwnershipAttached {
			ownershipHint = "认领已有目录"
		}
		if _, err := inboxService.UpsertItem(ctx, inbox.UpsertItemRequest{
			TenantID:     item.TenantID,
			TargetUserID: item.RequestedBy,
			Scope:        "personal",
			ItemType:     inbox.ItemTypeProjectWorkspacePendingDelete,
			SourceType:   inbox.SourceTypeProjectWorkspacePendingDelete,
			SourceID:     item.ID,
			Title:        fmt.Sprintf("工作区目录「%s」删除待确认已滞留 %d 天", item.DirectoryName, days),
			Summary: fmt.Sprintf(
				"节点 %s 上的目录仍待管理员确认删除（来源：%s）。确认后才会从磁盘删除；拒绝则平台放手不再管理该目录。",
				item.NodeIDSnapshot,
				ownershipHint,
			),
			Priority: "medium",
			Status:   inbox.StatusOpen,
			DeepLink: map[string]any{
				"type": "project_workspace_pending_delete",
				"path": "/runtime",
			},
			LastActivityAt: time.Now(),
		}); err != nil {
			return fmt.Errorf("upsert workspace delete reminder for request %s: %w", item.ID, err)
		}
	}
	return nil
}
