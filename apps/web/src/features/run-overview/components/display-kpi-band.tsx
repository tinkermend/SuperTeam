import { GlassCard } from "@/components/superteam";
import type { InboxBadge } from "@/lib/api/inbox";
import type { RuntimeOverview } from "@/lib/api/runtime";
import { formatCompactTokens } from "../formatters";
import type { RuntimeOverviewSummary } from "../runtime-overview-model";

// 大屏 KPI 带:远距可读的大字指标行,排序按"要人处理的事优先"。
// 数据全部取自权威端点(收件箱徽标/员工总览/run-summary/runtime 总览),不做前端二次推导;
// 语义色只在需要人介入(非零/掉线)时点亮。
export function DisplayKpiBand({
  badge,
  summary,
  todayCompletedRunCount,
  runtime
}: {
  badge?: InboxBadge;
  summary: RuntimeOverviewSummary;
  todayCompletedRunCount?: number;
  runtime?: RuntimeOverview["summary"];
}) {
  const runtimeOffline = runtime !== undefined && runtime.online_nodes < runtime.total_nodes;
  return (
    <GlassCard className="mb-4 p-4" data-display-kpi-band>
      <div className="grid grid-cols-3 gap-3 lg:grid-cols-6">
        <KpiCell
          label="待人工处理"
          value={badge ? badge.team_open_count : "—"}
          tone={badge && badge.team_open_count > 0 ? "warn" : undefined}
          badge={badge && badge.high_risk_count > 0 ? `高危 ${badge.high_risk_count}` : undefined}
        />
        <KpiCell label="异常" value={summary.errorCount} tone={summary.errorCount > 0 ? "danger" : undefined} />
        <KpiCell label="运行中" value={summary.workingCount} />
        <KpiCell label="今日完成运行" value={todayCompletedRunCount ?? "—"} />
        <KpiCell
          label="Runtime 在线"
          value={runtime ? `${runtime.online_nodes}/${runtime.total_nodes}` : "—"}
          tone={runtimeOffline ? "danger" : undefined}
        />
        <KpiCell label="今日消耗 tokens" value={formatCompactTokens(summary.todayTokensTotal)} />
      </div>
    </GlassCard>
  );
}

function KpiCell({
  label,
  value,
  tone,
  badge
}: {
  label: string;
  value: number | string;
  tone?: "danger" | "warn";
  badge?: string;
}) {
  const valueClass = tone === "danger" ? "text-danger" : tone === "warn" ? "text-warn-text" : "text-ink";
  return (
    <div className="glass-inner relative p-4" data-display-kpi={label}>
      <div className="text-sm text-ink-2">{label}</div>
      <div className={`mt-1 text-4xl font-semibold tabular-nums ${valueClass}`}>{value}</div>
      {badge ? (
        <span className="absolute right-3 top-3 rounded-full bg-danger-soft px-2 py-0.5 text-xs font-semibold text-danger-text">
          {badge}
        </span>
      ) : null}
    </div>
  );
}
