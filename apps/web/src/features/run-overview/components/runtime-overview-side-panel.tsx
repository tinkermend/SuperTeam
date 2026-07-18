import { useMemo, useState } from "react";
import { Link } from "@tanstack/react-router";
import { GlassCard, V3Button } from "@/components/superteam";
import type { RuntimeOverviewActivityItem, RuntimeOverviewDTO, RuntimeOverviewEmployee } from "../runtime-overview-model";
import { aggregateLensProjectOptions, type ProjectLens } from "../runtime-overview-project-lens";
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
  selectedProjectId,
  onSelectProject,
  lens,
  lensLoading,
}: {
  overview: RuntimeOverviewDTO;
  // 优先使用 activity 端点数据；未加载/失败时回退 overview 内聚合的近似动态。
  activity?: RuntimeOverviewActivityItem[];
  // 项目透镜：选中项目后地图高亮参与者并绘制任务交接链路。
  selectedProjectId?: string;
  onSelectProject?: (projectId?: string) => void;
  lens?: ProjectLens;
  lensLoading?: boolean;
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
      {onSelectProject ? (
        <ProjectLensBlock
          overview={overview}
          selectedProjectId={selectedProjectId}
          onSelectProject={onSelectProject}
          lens={lens}
          lensLoading={lensLoading}
        />
      ) : null}
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

// 项目透镜区块：默认只列有活跃任务的项目控制长度，可展开全部；选中后展示链路摘要，
// 任务级操作一律跳项目详情，总览不承载。
function ProjectLensBlock({
  overview,
  selectedProjectId,
  onSelectProject,
  lens,
  lensLoading,
}: {
  overview: RuntimeOverviewDTO;
  selectedProjectId?: string;
  onSelectProject: (projectId?: string) => void;
  lens?: ProjectLens;
  lensLoading?: boolean;
}) {
  const [showAll, setShowAll] = useState(false);
  const options = useMemo(() => aggregateLensProjectOptions(overview), [overview]);
  const activeOptions = options.filter((option) => option.activeTaskCount > 0);
  const visibleOptions = showAll ? options : activeOptions;
  if (options.length === 0) return null;
  return (
    <div className="mt-5" data-runtime-project-lens>
      <div className="flex items-center justify-between">
        <div className="text-xs font-semibold text-v3-ink-2">项目透镜</div>
        {selectedProjectId ? (
          <button
            type="button"
            className="text-xs font-medium text-v3-brand hover:underline"
            onClick={() => onSelectProject(undefined)}
          >
            退出透镜
          </button>
        ) : null}
      </div>
      <ul className="mt-2 space-y-1.5">
        {visibleOptions.map((option) => {
          const selected = option.projectId === selectedProjectId;
          return (
            <li key={option.projectId}>
              <button
                type="button"
                data-runtime-lens-project={option.projectId}
                aria-pressed={selected}
                className={`v3-glass-inner flex w-full items-center justify-between gap-2 px-3 py-2 text-left text-sm transition ${
                  selected ? "ring-2 ring-v3-brand/60" : "hover:ring-1 hover:ring-v3-line"
                }`}
                onClick={() => onSelectProject(selected ? undefined : option.projectId)}
              >
                <span className="min-w-0 truncate font-medium text-v3-ink">{option.name}</span>
                <span className="shrink-0 text-xs text-v3-ink-3 tabular-nums">
                  {option.participantCount} 人{option.activeTaskCount > 0 ? ` · 活跃 ${option.activeTaskCount}` : ""}
                </span>
              </button>
            </li>
          );
        })}
      </ul>
      {!showAll && options.length > activeOptions.length ? (
        <button
          type="button"
          className="mt-2 text-xs font-medium text-v3-brand hover:underline"
          onClick={() => setShowAll(true)}
        >
          显示全部 {options.length} 个项目
        </button>
      ) : null}
      {selectedProjectId ? (
        <div className="v3-glass-inner mt-3 px-3 py-3 text-sm" data-runtime-lens-summary>
          {lens ? (
            <>
              <p className="flex flex-wrap gap-x-3 gap-y-1 text-v3-ink-2 tabular-nums">
                <span>参与 {lens.participantEmployeeIds.length} 人</span>
                <span>交接 {lens.edges.length} 段</span>
                {lens.blockedTaskCount > 0 ? (
                  <span className="font-semibold text-v3-danger">阻塞 {lens.blockedTaskCount}</span>
                ) : null}
                {lens.unassignedTaskCount > 0 ? <span>待派发 {lens.unassignedTaskCount}</span> : null}
              </p>
              <V3Button asChild size="sm" variant="glass" className="mt-2">
                <Link params={{ projectId: selectedProjectId }} to="/projects/$projectId">
                  查看项目详情
                </Link>
              </V3Button>
            </>
          ) : lensLoading ? (
            <p className="text-v3-ink-3">正在加载任务链路…</p>
          ) : (
            <p className="text-v3-ink-3">该项目暂无可用任务链路</p>
          )}
        </div>
      ) : null}
    </div>
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
