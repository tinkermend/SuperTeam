package app

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/superteam/control-plane/internal/feishu"
	"github.com/superteam/control-plane/internal/inbox"
	"github.com/superteam/control-plane/internal/platform"
)

// 飞书 connector 断连看门狗(接入管理 P1):
// connector 正常约 30s 心跳;超过 ConnectorHeartbeatTimeout 未上报 → 向租户
// owner/admin 投递收件箱告警(只进 Console,不推飞书,避免自指)。恢复后按
// source 自动 resolve。同一 service 的告警 source_id 稳定,Upsert 幂等不刷屏。

const feishuChannelWatchInterval = 30 * time.Second

func startFeishuChannelWatchdog(ctx context.Context, feishuService *feishu.Service, inboxService *inbox.Service) {
	if feishuService == nil || inboxService == nil {
		return
	}
	run := func() {
		if err := sweepFeishuChannelHealth(ctx, feishuService, inboxService); err != nil && ctx.Err() == nil {
			slog.Warn("feishu channel watchdog sweep failed", "error", err)
		}
	}
	run()
	ticker := time.NewTicker(feishuChannelWatchInterval)
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

func sweepFeishuChannelHealth(ctx context.Context, feishuService *feishu.Service, inboxService *inbox.Service) error {
	tenantID := platform.DefaultTenantID
	health, err := feishuService.GetChannelHealth(ctx, tenantID)
	if err != nil {
		return err
	}
	sourceID := connectorSourceID(health.ServiceName)
	switch health.Status {
	case "healthy":
		return inboxService.ResolveOpenItemsBySource(ctx, tenantID, inbox.SourceTypeChannelAlert, sourceID)
	case "stale", "missing":
		return openChannelDownItems(ctx, feishuService, inboxService, tenantID, health, sourceID)
	default:
		return nil
	}
}

func connectorSourceID(serviceName string) uuid.UUID {
	if serviceName == "" {
		serviceName = feishu.DefaultConnectorServiceName
	}
	return uuid.NewSHA1(uuid.NameSpaceOID, []byte("feishu-channel:"+serviceName))
}

func openChannelDownItems(ctx context.Context, feishuService *feishu.Service, inboxService *inbox.Service, tenantID uuid.UUID, health feishu.ChannelHealth, sourceID uuid.UUID) error {
	recipients, err := feishuService.ListChannelAlertRecipients(ctx, tenantID)
	if err != nil {
		return err
	}
	if len(recipients) == 0 {
		return nil
	}
	title := "飞书通道失联"
	summary := "feishu-connector 心跳超时或从未上报。Console 是完整操作面,业务不阻断;请检查 connector 进程与 FEISHU_CONNECTOR_TOKEN。"
	if health.Status == "missing" {
		summary = "尚未收到 feishu-connector 心跳。若刚部署请确认进程已启动并完成 bootstrap;若长期如此,通道通知不会发出。"
	} else if health.AgeSeconds != nil {
		summary = fmt.Sprintf("feishu-connector 已 %d 秒未心跳(阈值 %d 秒)。请检查进程、网络与服务凭据;恢复后本告警会自动关闭。", *health.AgeSeconds, health.TimeoutSeconds)
	}
	now := time.Now().UTC()
	for _, userID := range recipients {
		if _, err := inboxService.UpsertItem(ctx, inbox.UpsertItemRequest{
			TenantID:     tenantID,
			TargetUserID: userID,
			Scope:        "personal",
			ItemType:     inbox.ItemTypeChannelAlert,
			SourceType:   inbox.SourceTypeChannelAlert,
			SourceID:     sourceID,
			Title:        title,
			Summary:      summary,
			Priority:     "high",
			Status:       inbox.StatusOpen,
			// 空动作:告警只提示去系统配置排查,不走审批动词。
			Actions: []inbox.Action{},
			DeepLink: map[string]any{
				"type":            "feishu_channel_down",
				"route":           "/system-config",
				"primary_surface": "/system-config",
			},
			ContextPayload: map[string]any{
				"channel":      "feishu",
				"service_name": health.ServiceName,
				"status":       health.Status,
			},
			LastActivityAt: now,
		}); err != nil {
			return fmt.Errorf("upsert channel down alert for user %s: %w", userID, err)
		}
	}
	return nil
}
