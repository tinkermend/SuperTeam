package app

import (
	"context"
	"log/slog"
	"time"

	"github.com/superteam/control-plane/internal/platform"
	"github.com/superteam/control-plane/internal/project"
	"github.com/superteam/control-plane/internal/systemconfig"
)

// 僵尸任务收敛看门狗(卡死任务收敛 spec P1):周期性兜底两类"任务卡在执行中但无自愈"
// 的场景,补齐既有恢复能力从未接线的缺口——
//  1. 孤儿任务:running/in_progress 但无活跃 attempt(无 attempt、无 run),滞留超过
//     可配阈值 task.stuck_running_timeout。协调线程从未派发成功、协调线程死亡或异常
//     数据直插都会落到这里。SweepStuckOrphanProjectTasks 置 failed + 发失败信号收敛。
//  2. waiting_human orphan:任务停在 waiting_human 但 waiting_request_id 空/失效且
//     无可行动 open decision。SweepOrphanWaitingHumanProjectTasks 补绑或补建决策卡。
//  3. attempt 卡死:queued attempt 派发未确认、running attempt 租约过期被死亡 runtime
//     抛弃(遗留缺陷#1:runtime 写回丢失致任务永久 running)。既有的 per-tenant sweep
//     方法此前无任何调度者;这里逐租户驱动它们做有界重试/转人工。
//  4. 决策 SoT 与收件箱投影分裂:pending decision 无 open inbox 卡(创建后 Upsert
//     失败、或 inbox 被取消未收敛决策)。SweepOrphanDecisionInboxProjections
//     对仍可处理的补投影、对已终态关联对象的取消决策。
//  5. 滞留下游 blocked:上游已 failed/cancelled，下游仍 blocked（失败恢复未走驳回
//     就不会 cancelFailureDownstream）。SweepStrandedBlockedProjectTasks 取消这些
//     下游并重算需求状态，避免需求永远「执行中」。
//
// 与派发路径的按需恢复(同项目下一次协调触发)互补:runtime 整个死掉、协调线程不再运转
// 时,只有这个平台级看门狗能把任务收走。逐条/逐租户失败只记日志,下一轮重试。
const (
	stuckTaskReconcileInterval = time.Minute
	// 单轮孤儿收敛上限:看门狗周期跑,积压在后续轮次消化,避免单轮长事务扫全表。
	stuckTaskReapBatchLimit = 50
)

func startStuckTaskReconciler(ctx context.Context, projectService *project.Service, config systemconfig.Reader) {
	run := func() {
		now := time.Now().UTC()
		// 阈值读平台默认租户配置(与登录会话 TTL 等平台级读取一致);读路径永不失败,
		// 存储故障回退注册表默认值。
		timeout := config.Duration(ctx, platform.DefaultTenantID, systemconfig.KeyTaskStuckRunningTimeoutSeconds)
		if timeout <= 0 {
			timeout = systemconfig.DefaultDurationFor(systemconfig.KeyTaskStuckRunningTimeoutSeconds)
		}
		staleBefore := now.Add(-timeout)
		if _, err := projectService.SweepStuckOrphanProjectTasks(ctx, staleBefore, stuckTaskReapBatchLimit); err != nil && ctx.Err() == nil {
			slog.Warn("stuck task reconciler: orphan sweep failed", "error", err)
		}
		if _, err := projectService.SweepOrphanWaitingHumanProjectTasks(ctx, stuckTaskReapBatchLimit); err != nil && ctx.Err() == nil {
			slog.Warn("stuck task reconciler: waiting_human orphan repair failed", "error", err)
		}
		if _, err := projectService.SweepOrphanDecisionInboxProjections(ctx, stuckTaskReapBatchLimit); err != nil && ctx.Err() == nil {
			slog.Warn("stuck task reconciler: decision inbox projection repair failed", "error", err)
		}
		if _, err := projectService.SweepStrandedBlockedProjectTasks(ctx, stuckTaskReapBatchLimit); err != nil && ctx.Err() == nil {
			slog.Warn("stuck task reconciler: stranded blocked downstream cancel failed", "error", err)
		}
		if _, err := projectService.SweepStuckProjectTaskAttemptsAllTenants(ctx, now); err != nil && ctx.Err() == nil {
			slog.Warn("stuck task reconciler: attempt recovery sweep failed", "error", err)
		}
	}
	// 启动即扫一轮(部署后立即补账),此后按 interval 巡检。
	run()
	ticker := time.NewTicker(stuckTaskReconcileInterval)
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
