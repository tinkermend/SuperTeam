package app

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/superteam/control-plane/internal/inbox"
	"github.com/superteam/control-plane/internal/tenant"
)

// 团队待确认删除滞留催办(生命周期收敛 P2,用户拍板"永不自动 + 超时催办"):
// pending_delete 永不因超时自动物理删除;滞留超过 staleAfter 后,周期任务向删除
// 发起人投递收件箱催办。Upsert 以 (tenant, source_type, source_id) 幂等——同团队
// 始终只有一条催办条目,重复扫描仅刷新活跃时间,不刷屏。
const (
	teamPendingDeleteStaleAfter       = 7 * 24 * time.Hour
	teamPendingDeleteReminderInterval = 24 * time.Hour
)

func startTeamPendingDeleteReminder(ctx context.Context, tenantService *tenant.Service, inboxService *inbox.Service) {
	run := func() {
		if err := remindStalePendingDeleteTeams(ctx, tenantService, inboxService); err != nil && ctx.Err() == nil {
			slog.Warn("team pending delete reminder sweep failed", "error", err)
		}
	}
	// 启动即扫一轮(部署后立即补账),此后按天巡检。
	run()
	ticker := time.NewTicker(teamPendingDeleteReminderInterval)
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

func remindStalePendingDeleteTeams(ctx context.Context, tenantService *tenant.Service, inboxService *inbox.Service) error {
	stale, err := tenantService.ListStalePendingDeleteTeams(ctx, time.Now().Add(-teamPendingDeleteStaleAfter))
	if err != nil {
		return err
	}
	for _, team := range stale {
		if team.DeleteRequestedBy == nil {
			// 无发起人可催(理论上不发生:P2 起删除必带 actor);跳过而不是失败整轮。
			continue
		}
		days := int(time.Since(team.DeletedAt).Hours() / 24)
		if _, err := inboxService.UpsertItem(ctx, inbox.UpsertItemRequest{
			TenantID:     team.TenantID,
			TargetUserID: *team.DeleteRequestedBy,
			Scope:        "personal",
			ItemType:     inbox.ItemTypeTeamPendingDelete,
			SourceType:   inbox.SourceTypeTeamPendingDelete,
			SourceID:     team.ID,
			Title:        fmt.Sprintf("团队「%s」删除待确认已滞留 %d 天", team.Name, days),
			Summary:      "该团队处于删除待确认状态,请在团队管理页恢复或确认删除;未确认前不会物理删除。",
			Priority:     "medium",
			Status:       inbox.StatusOpen,
			DeepLink: map[string]any{
				"type": "team_pending_delete",
				"path": "/teams",
			},
			LastActivityAt: time.Now(),
		}); err != nil {
			return fmt.Errorf("upsert pending delete reminder for team %s: %w", team.ID, err)
		}
	}
	return nil
}
