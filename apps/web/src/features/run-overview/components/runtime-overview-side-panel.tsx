import { useMemo, useState } from "react";
import { Link } from "@tanstack/react-router";
import { GlassCard, Button } from "@/components/superteam";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import type { ProjectDemand } from "@/lib/api/projects";
import type { RuntimeOverviewActivityItem, RuntimeOverviewDTO, RuntimeOverviewEmployee } from "../runtime-overview-model";
import { aggregateLensProjectOptions, type ProjectLens } from "../runtime-overview-project-lens";
import { formatCompactTokens, formatTime } from "../formatters";
import { employeeStatusDotClass as statusDotClass } from "../status-maps";

const activityDotClass: Record<string, string> = {
  cancelled: "bg-mute",
  completed: "bg-ok",
  failed: "bg-danger",
  running: "bg-info"
};

export function RuntimeOverviewSidePanel({
  overview,
  activity,
  selectedProjectId,
  onSelectProject,
  lens,
  lensLoading,
  demands,
  selectedDemandId,
  onSelectDemand
}: {
  overview: RuntimeOverviewDTO;
  // 优先使用 activity 端点数据；未加载/失败时回退 overview 内聚合的近似动态。
  activity?: RuntimeOverviewActivityItem[];
  // 项目透镜：选中项目后地图高亮参与者并绘制任务交接链路。
  selectedProjectId?: string;
  onSelectProject?: (projectId?: string) => void;
  lens?: ProjectLens;
  lensLoading?: boolean;
  // 透镜链路所属的需求：显式标注当前 demand，多 demand 时可切换。
  demands?: ProjectDemand[];
  selectedDemandId?: string;
  onSelectDemand?: (demandId: string) => void;
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
        <h2 className="text-sm font-semibold text-ink">运行概况</h2>
        <span className="rounded-full bg-ok-soft px-2 py-1 text-xs font-semibold text-ok">实时读取</span>
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
          <div className="mb-2 flex items-center justify-between text-xs text-ink-2">
            <span>容量使用</span>
            <span className="font-semibold text-ink tabular-nums">
              {overview.summary.capacityUsed} / {overview.summary.capacityTotal}
            </span>
          </div>
          <div className="h-1.5 overflow-hidden rounded-full bg-[color:var(--aurora-hairline)]">
            <span
              className="block h-full rounded-full bg-brand"
              style={{
                width: `${overview.summary.capacityTotal > 0 ? Math.min(100, Math.round((overview.summary.capacityUsed / overview.summary.capacityTotal) * 100)) : 0}%`
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
          demands={demands}
          selectedDemandId={selectedDemandId}
          onSelectDemand={onSelectDemand}
        />
      ) : null}
      {recentActivity.length > 0 ? (
        <div className="mt-5">
          <div className="text-xs font-semibold text-ink-2">最新动态</div>
          <ul className="mt-2 space-y-2" data-runtime-recent-activity>
            {recentActivity.slice(0, 5).map((item, index) => (
              <li
                key={`${item.employeeId}-${item.label}-${index}`}
                className="flex items-center gap-2 rounded-lg bg-card-soft px-3 py-2 text-sm text-ink-2"
              >
                <span className={`size-2 shrink-0 rounded-full ${activityDotClass[item.status] ?? "bg-mute"}`} aria-hidden />
                <span className="shrink-0 font-medium text-ink">{item.employeeName}</span>
                <span className="min-w-0 flex-1 truncate">
                  {item.label}
                  {item.taskTitle ? ` · ${item.taskTitle}` : ""}
                </span>
                {item.occurredAt ? (
                  <span className="shrink-0 text-xs tabular-nums text-ink-3">{formatTime(item.occurredAt)}</span>
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
  demands,
  selectedDemandId,
  onSelectDemand
}: {
  overview: RuntimeOverviewDTO;
  selectedProjectId?: string;
  onSelectProject: (projectId?: string) => void;
  lens?: ProjectLens;
  lensLoading?: boolean;
  demands?: ProjectDemand[];
  selectedDemandId?: string;
  onSelectDemand?: (demandId: string) => void;
}) {
  const [showAll, setShowAll] = useState(false);
  const options = useMemo(() => aggregateLensProjectOptions(overview), [overview]);
  const activeOptions = options.filter((option) => option.activeTaskCount > 0);
  const visibleOptions = showAll ? options : activeOptions;
  if (options.length === 0) return null;
  return (
    <div className="mt-5" data-runtime-project-lens>
      <div className="flex items-center justify-between">
        <div className="text-xs font-semibold text-ink-2">项目透镜</div>
        {selectedProjectId ? (
          <button
            type="button"
            className="text-xs font-medium text-brand hover:underline"
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
                className={`glass-inner flex w-full items-center justify-between gap-2 px-3 py-2 text-left text-sm transition ${
                  selected ? "ring-2 ring-brand/60" : "hover:ring-1 hover:ring-line"
                }`}
                onClick={() => onSelectProject(selected ? undefined : option.projectId)}
              >
                <span className="min-w-0 truncate font-medium text-ink">{option.name}</span>
                <span className="shrink-0 text-xs text-ink-3 tabular-nums">
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
          className="mt-2 text-xs font-medium text-brand hover:underline"
          onClick={() => setShowAll(true)}
        >
          显示全部 {options.length} 个项目
        </button>
      ) : null}
      {selectedProjectId ? (
        <div className="glass-inner mt-3 px-3 py-3 text-sm" data-runtime-lens-summary>
          <DemandRow demands={demands} selectedDemandId={selectedDemandId} onSelectDemand={onSelectDemand} />
          {lens ? (
            <>
              <p className="flex flex-wrap gap-x-3 gap-y-1 text-ink-2 tabular-nums">
                <span>参与 {lens.participantEmployeeIds.length} 人</span>
                <span>交接 {lens.edges.length} 段</span>
                {lens.blockedTaskCount > 0 ? (
                  <span className="font-semibold text-danger">阻塞 {lens.blockedTaskCount}</span>
                ) : null}
                {lens.unassignedTaskCount > 0 ? <span>待派发 {lens.unassignedTaskCount}</span> : null}
              </p>
              <Button asChild size="sm" variant="glass" className="mt-2">
                <Link params={{ projectId: selectedProjectId }} to="/projects/$projectId">
                  查看项目详情
                </Link>
              </Button>
            </>
          ) : lensLoading ? (
            <p className="text-ink-3">正在加载任务链路…</p>
          ) : (
            <p className="text-ink-3">该项目暂无可用任务链路</p>
          )}
        </div>
      ) : null}
    </div>
  );
}

// 透镜链路所属需求行：让"链路来自哪个 demand"始终显式可见——单需求静态标注，
// 多需求提供切换（选定后由页面层钉住，不随新 demand 抢位）。
function DemandRow({
  demands,
  selectedDemandId,
  onSelectDemand
}: {
  demands?: ProjectDemand[];
  selectedDemandId?: string;
  onSelectDemand?: (demandId: string) => void;
}) {
  if (!demands || demands.length === 0) return null;
  const current = demands.find((demand) => demand.id === selectedDemandId) ?? demands[0];
  return (
    <div className="mb-2 flex items-center gap-2" data-runtime-lens-demand>
      <span className="shrink-0 text-xs text-ink-3">需求</span>
      {demands.length > 1 && onSelectDemand ? (
        <Select value={current.id} onValueChange={onSelectDemand}>
          <SelectTrigger aria-label="切换需求" size="sm" className="h-7 min-w-0 flex-1 text-xs">
            <SelectValue placeholder="选择需求" />
          </SelectTrigger>
          <SelectContent>
            {demands.map((demand) => (
              <SelectItem key={demand.id} value={demand.id}>
                {demand.title}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      ) : (
        <span className="min-w-0 truncate text-xs font-medium text-ink" data-runtime-lens-demand-title>
          {current.title}
        </span>
      )}
    </div>
  );
}

function Metric({ label, value, tone }: { label: string; value: number | string; tone?: "danger" }) {
  return (
    <div className="glass-inner p-3">
      <div className="text-xs text-ink-2">{label}</div>
      <div className={`mt-1 text-2xl font-semibold tabular-nums ${tone === "danger" ? "text-danger" : "text-ink"}`}>{value}</div>
    </div>
  );
}

function StatusRow({ label, status, value }: { label: string; status: RuntimeOverviewEmployee["status"]; value: number }) {
  return (
    <div className="flex items-center justify-between text-sm">
      <span className="inline-flex items-center gap-2 text-ink-2">
        <span className={`size-2.5 rounded-full ${statusDotClass[status]}`} aria-hidden />
        {label}
      </span>
      <span className="font-semibold text-ink tabular-nums">{value}</span>
    </div>
  );
}
