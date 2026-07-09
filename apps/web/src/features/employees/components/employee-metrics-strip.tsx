import { Activity, CheckCircle2, Clock, Gauge, Hand, Server, XCircle } from "lucide-react";
import { StatusPill, V3MetricCard } from "@/components/superteam";
import type { DigitalEmployeeRunStats } from "@/lib/api/employees";

type EmployeeMetricsStripProps = {
  stats: DigitalEmployeeRunStats | undefined;
  providerType: string;
  runtimeNodeLabel: string;
  commandChannelConnected?: boolean;
  currentStatusLabel: string;
};

export function EmployeeMetricsStrip({
  stats,
  providerType,
  runtimeNodeLabel,
  commandChannelConnected,
  currentStatusLabel,
}: EmployeeMetricsStripProps) {
  const trend = formatTrend(stats);

  return (
    <section className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4 xl:grid-cols-5">
      <V3MetricCard icon={<Server />} iconTone="brand" label="Provider" value={providerType} />
      <V3MetricCard
        icon={<Server />}
        iconTone="info"
        label="Runtime 执行位置"
        meta={
          <StatusPill
            showDot={commandChannelConnected !== undefined}
            tone={commandChannelConnected === undefined ? "mute" : commandChannelConnected ? "ok" : "danger"}
          >
            {commandChannelConnected === undefined
              ? "项目调度时选择"
              : commandChannelConnected
                ? "命令通道在线"
                : "Runtime 命令通道未连接"}
          </StatusPill>
        }
        value={runtimeNodeLabel}
      />
      <V3MetricCard icon={<Activity />} iconTone="mute" label="累计执行" value={stats ? stats.total_count : "--"} />
      <V3MetricCard icon={<Activity />} iconTone="info" label="近7天" meta={trend} value={stats ? stats.last_7d_count : "--"} />
      <V3MetricCard
        icon={<Gauge />}
        iconTone="ok"
        label="成功率"
        value={stats && stats.success_rate !== null ? formatPercent(stats.success_rate) : "--"}
      />
      <V3MetricCard
        icon={<Clock />}
        iconTone="brand"
        label="平均耗时"
        meta={stats?.p90_duration_sec != null ? `P90 ${formatDuration(stats.p90_duration_sec)}` : undefined}
        value={stats?.avg_duration_sec != null ? formatDuration(stats.avg_duration_sec) : "--"}
      />
      <V3MetricCard icon={<CheckCircle2 />} iconTone="ok" label="成功" value={stats ? stats.succeeded_count : "--"} />
      <V3MetricCard icon={<XCircle />} iconTone="danger" label="失败" value={stats ? stats.failed_count : "--"} />
      <V3MetricCard icon={<Hand />} iconTone="warn" label="人工停止" value={stats ? stats.cancelled_count : "--"} />
      <V3MetricCard icon={<Activity />} iconTone="brand" label="当前状态" value={currentStatusLabel} />
    </section>
  );
}

function formatPercent(ratio: number): string {
  return `${(ratio * 100).toFixed(1)}%`;
}

function formatDuration(seconds: number): string {
  const totalSeconds = Math.round(seconds);
  const minutes = Math.floor(totalSeconds / 60);
  const remainSeconds = totalSeconds % 60;
  return `${minutes}分${remainSeconds}秒`;
}

function formatTrend(stats: DigitalEmployeeRunStats | undefined): string | undefined {
  if (!stats) {
    return undefined;
  }
  if (stats.prev_7d_count === 0) {
    return `较上周期 +${stats.last_7d_count}`;
  }
  const change = ((stats.last_7d_count - stats.prev_7d_count) / stats.prev_7d_count) * 100;
  const arrow = change >= 0 ? "↑" : "↓";
  return `较上周期 ${arrow}${Math.abs(change).toFixed(0)}%`;
}
