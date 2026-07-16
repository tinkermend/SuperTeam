import { GlassCard } from "@/components/superteam";
import type { RuntimeOverviewActivityItem, RuntimeOverviewDTO, RuntimeOverviewEmployee } from "../runtime-overview-model";
import { formatCompactTokens, formatTime } from "../formatters";
import { employeeStatusDotClass as statusDotClass } from "../status-maps";

const activityDotClass: Record<string, string> = {
  cancelled: "bg-v3-mute",
  completed: "bg-v3-ok",
  failed: "bg-v3-danger",
  running: "bg-v3-info",
};

export function RuntimeOverviewSidePanel({
  overview,
  activity,
}: {
  overview: RuntimeOverviewDTO;
  // 优先使用 activity 端点数据；未加载/失败时回退 overview 内聚合的近似动态。
  activity?: RuntimeOverviewActivityItem[];
}) {
  const recentActivity = activity ?? overview.recentActivity;
  // 运行态分布：覆盖全部 7 种运行状态，过滤 0 值后可见行之和恒等于「数字员工」。
  const statusBreakdown: Array<{ label: string; status: RuntimeOverviewEmployee["status"]; value: number }> = [
    { label: "正在工作", status: "working", value: overview.summary.workingCount },
    { label: "空闲", status: "idle", value: overview.summary.idleCount },
    { label: "排队", status: "queued", value: overview.summary.queuedCount },
    { label: "待配置", status: "needs_configuration", value: overview.summary.needsConfigurationCount },
    { label: "待人工确认", status: "waiting_human", value: overview.summary.waitingHumanCount },
    { label: "不可用", status: "unavailable", value: overview.summary.unavailableCount },
    { label: "异常", status: "error", value: overview.summary.errorCount },
  ];
  return (
    <GlassCard className="p-4">
      <div className="flex items-center justify-between gap-3">
        <h2 className="text-sm font-semibold text-v3-ink">运行概况</h2>
        <span className="rounded-full bg-v3-ok-soft px-2 py-1 text-xs font-semibold text-v3-ok">实时读取</span>
      </div>
      <div className="mt-4 grid grid-cols-2 gap-3">
        <Metric label="团队" value={overview.summary.teamCount} />
        <Metric label="数字员工" value={overview.summary.employeeCount} />
        <Metric label="容量使用" value={`${overview.summary.capacityUsed}/${overview.summary.capacityTotal}`} />
        <Metric label="异常" value={overview.summary.errorCount} tone={overview.summary.errorCount > 0 ? "danger" : undefined} />
        <Metric label="关联项目" value={overview.summary.linkedProjectCount} />
        <Metric label="今日消耗 tokens" value={formatCompactTokens(overview.summary.todayTokensTotal)} />
      </div>
      <div className="mt-4 space-y-3">
        <div>
          <div className="mb-2 flex items-center justify-between text-xs text-v3-ink-2">
            <span>容量使用</span>
            <span className="font-semibold text-v3-ink tabular-nums">
              {overview.summary.capacityUsed} / {overview.summary.capacityTotal}
            </span>
          </div>
          <div className="h-1.5 overflow-hidden rounded-full bg-[color:var(--v3-aurora-hairline)]">
            <span
              className="block h-full rounded-full bg-v3-brand"
              style={{
                width: `${overview.summary.capacityTotal > 0 ? Math.min(100, Math.round((overview.summary.capacityUsed / overview.summary.capacityTotal) * 100)) : 0}%`,
              }}
            />
          </div>
        </div>
        {statusBreakdown
          .filter((row) => row.value > 0)
          .map((row) => (
            <StatusRow key={row.status} label={row.label} value={row.value} status={row.status} />
          ))}
      </div>
      {recentActivity.length > 0 ? (
        <div className="mt-5">
          <div className="text-xs font-semibold text-v3-ink-2">最新动态</div>
          <ul className="mt-2 space-y-2" data-runtime-recent-activity>
            {recentActivity.slice(0, 5).map((item, index) => (
              <li
                key={`${item.employeeId}-${item.label}-${index}`}
                className="flex items-center gap-2 rounded-lg bg-v3-card-soft px-3 py-2 text-sm text-v3-ink-2"
              >
                <span className={`size-2 shrink-0 rounded-full ${activityDotClass[item.status] ?? "bg-v3-mute"}`} aria-hidden />
                <span className="shrink-0 font-medium text-v3-ink">{item.employeeName}</span>
                <span className="min-w-0 flex-1 truncate">
                  {item.label}
                  {item.taskTitle ? ` · ${item.taskTitle}` : ""}
                </span>
                {item.occurredAt ? (
                  <span className="shrink-0 text-xs tabular-nums text-v3-ink-3">{formatTime(item.occurredAt)}</span>
                ) : null}
              </li>
            ))}
          </ul>
        </div>
      ) : null}
    </GlassCard>
  );
}

function Metric({ label, value, tone }: { label: string; value: number | string; tone?: "danger" }) {
  return (
    <div className="v3-glass-inner p-3">
      <div className="text-xs text-v3-ink-2">{label}</div>
      <div className={`mt-1 text-2xl font-semibold tabular-nums ${tone === "danger" ? "text-v3-danger" : "text-v3-ink"}`}>{value}</div>
    </div>
  );
}

function StatusRow({ label, status, value }: { label: string; status: RuntimeOverviewEmployee["status"]; value: number }) {
  return (
    <div className="flex items-center justify-between text-sm">
      <span className="inline-flex items-center gap-2 text-v3-ink-2">
        <span className={`size-2.5 rounded-full ${statusDotClass[status]}`} aria-hidden />
        {label}
      </span>
      <span className="font-semibold text-v3-ink tabular-nums">{value}</span>
    </div>
  );
}
