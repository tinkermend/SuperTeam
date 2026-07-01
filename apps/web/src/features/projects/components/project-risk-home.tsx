import { Link } from "@tanstack/react-router";
import {
  AlertTriangle,
  Archive,
  CircleDot,
  Clock3,
  FileWarning,
  FolderKanban,
  PlayCircle,
  ShieldAlert,
  UserCheck,
} from "lucide-react";
import {
  IconTile,
  StatusPill,
  V3Button,
  V3Chip,
  V3EmptyState,
  V3Pagination,
  V3Table,
  V3Td,
  V3Th,
  V3ToolbarSearch,
  V3Tr,
  WorkSurface,
  type V3Tone,
} from "@/components/superteam";
import type { Project, ProjectEvent, ProjectStatus } from "@/lib/api/projects";
import { cn } from "@/lib/utils";
import {
  buildRiskCounts,
  emptyProjectRiskSummary,
  matchesProjectRiskFilter,
  PROJECT_RISK_FILTERS,
  projectRiskLevelLabel,
  projectRiskLevelTone,
  sortProjectsByRisk,
  type ProjectRiskCounts,
  type ProjectRiskFilter,
  type ProjectRiskSummary,
  type ProjectRiskSummaryMap,
} from "../project-risk";

export type ProjectRiskQueueFilters = {
  q: string;
  risk: ProjectRiskFilter;
  status: "all" | ProjectStatus;
};

export type ProjectRiskQueueProps = {
  activePage: number;
  filters: ProjectRiskQueueFilters;
  isFetching: boolean;
  onFiltersChange: (filters: ProjectRiskQueueFilters) => void;
  onPageChange: (page: number) => void;
  onPageSizeChange: (pageSize: number) => void;
  onSelectProject: (projectId: string) => void;
  pageCount: number;
  pageSize: number;
  /** Unsorted current server page; this queue owns risk sorting and risk-chip filtering within that page. */
  projects: Project[];
  riskSummaries: ProjectRiskSummaryMap;
  selectedProjectId?: string;
  total: number;
};

export type ProjectSelectedContextPanelProps = {
  isLoading?: boolean;
  project?: Project;
  recentEvents: ProjectEvent[];
  riskSummary?: ProjectRiskSummary;
};

export function ProjectHomeRiskSummaryBar({
  isLoading,
  riskSummaries,
}: {
  isLoading?: boolean;
  riskSummaries: ProjectRiskSummaryMap;
}) {
  const counts = buildRiskCounts(Object.values(riskSummaries));
  const items = [
    {
      icon: ShieldAlert,
      label: "阻塞项目",
      tone: "danger",
      value: counts.blocked,
    },
    {
      icon: UserCheck,
      label: "人工决策",
      tone: "warn",
      value: counts.human_decision,
    },
    {
      icon: AlertTriangle,
      label: "执行失败",
      tone: "danger",
      value: counts.execution_failed,
    },
    {
      icon: FileWarning,
      label: "证据待补",
      tone: "warn",
      value: counts.evidence_required,
    },
    {
      icon: Clock3,
      label: "等待超时",
      tone: "warn",
      value: counts.sla_waiting,
    },
    {
      icon: PlayCircle,
      label: "协调异常",
      tone: "info",
      value: counts.runtime_or_coordination,
    },
  ] satisfies Array<{
    icon: typeof ShieldAlert;
    label: string;
    tone: V3Tone;
    value: number;
  }>;

  if (isLoading) {
    return (
      <section
        aria-label="项目风险汇总（当前页）"
        className="grid gap-3 rounded-v3-card border border-v3-line bg-v3-card p-4 shadow-v3 sm:grid-cols-[auto_minmax(0,1fr)] sm:items-center"
      >
        <IconTile tone="info" size="sm">
          <ShieldAlert />
        </IconTile>
        <div className="min-w-0">
          <StatusPill tone="info">风险识别中</StatusPill>
          <p className="mt-2 text-sm font-semibold leading-6 text-v3-ink">
            正在读取当前页项目的任务、决策和证据信号
          </p>
          <p className="mt-1 text-[12px] leading-5 text-v3-ink-3">
            摘要将在当前页风险信号稳定后一次性展示，避免首开半成品计数跳动。
          </p>
        </div>
      </section>
    );
  }

  return (
    <section
      aria-label="项目风险汇总（当前页）"
      className="grid gap-3 rounded-v3-card border border-v3-line bg-v3-card p-4 shadow-v3 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-6"
    >
      {items.map((item) => {
        const Icon = item.icon;
        return (
          <div
            key={item.label}
            className="flex min-w-0 items-center gap-3 rounded-v3-inner bg-v3-card-soft/70 px-3 py-2.5"
          >
            <IconTile tone={item.tone} size="sm">
              <Icon />
            </IconTile>
            <div className="min-w-0">
              <div className="truncate text-[12px] font-semibold text-v3-ink-2">
                {item.label}
                <span className="sr-only">（当前页）</span>
              </div>
              <div className="text-2xl font-extrabold tabular-nums text-v3-ink">
                {item.value}
              </div>
            </div>
          </div>
        );
      })}
    </section>
  );
}

export function ProjectRiskQueue(props: ProjectRiskQueueProps) {
  const currentPageSummaries = props.projects.map(
    (project) =>
      props.riskSummaries[project.id] ?? emptyProjectRiskSummary(project),
  );
  const riskCounts = buildRiskCounts(currentPageSummaries);
  const sortedProjects = sortProjectsByRisk(
    props.projects,
    props.riskSummaries,
  ).filter((project) => {
    const summary =
      props.riskSummaries[project.id] ?? emptyProjectRiskSummary(project);
    return matchesProjectRiskFilter(summary, props.filters.risk);
  });

  return (
    <section
      aria-label="项目队列"
      className="min-w-0"
      data-testid="project-risk-queue"
    >
      <WorkSurface className="min-w-0">
        <div className="flex min-w-0 flex-col gap-3 border-b border-v3-line p-4 lg:flex-row lg:items-start lg:justify-between">
          <div className="min-w-0">
            <h2 className="text-base font-extrabold text-v3-ink">项目队列</h2>
            <div className="mt-1 flex flex-wrap items-center gap-2">
              <StatusPill tone={props.isFetching ? "info" : "mute"}>
                {props.isFetching ? "正在识别风险" : `${props.total} 个项目`}
              </StatusPill>
              <StatusPill tone={riskCounts.blocked > 0 ? "danger" : "mute"}>
                {riskCounts.blocked} 个阻塞（当前页）
              </StatusPill>
            </div>
            <p className="mt-1 text-[11px] leading-5 text-v3-ink-3">
              风险识别与排序基于当前页；风险筛选仅过滤当前页项目，分页仍对应完整项目列表。
            </p>
          </div>
        </div>

        <div className="flex min-w-0 flex-col gap-3 border-b border-v3-line p-4">
          <div className="flex min-w-0 flex-col gap-2 lg:flex-row lg:items-center">
            <V3ToolbarSearch
              aria-label="搜索项目"
              className="min-w-0 lg:max-w-md"
              onChange={(event) =>
                props.onFiltersChange({
                  ...props.filters,
                  q: event.target.value,
                })
              }
              placeholder="搜索项目名称或目标"
              value={props.filters.q}
            />
            <select
              aria-label="项目状态筛选"
              className="h-9 rounded-[10px] border border-v3-line bg-v3-card px-3 text-[13px] text-v3-ink outline-none transition-colors hover:bg-v3-card-soft focus:border-v3-brand focus:ring-2 focus:ring-v3-brand/25"
              onChange={(event) =>
                props.onFiltersChange({
                  ...props.filters,
                  status: event.target
                    .value as ProjectRiskQueueProps["filters"]["status"],
                })
              }
              value={props.filters.status}
            >
              {PROJECT_STATUS_FILTER_OPTIONS.map((option) => (
                <option key={option.value} value={option.value}>
                  {option.label}
                </option>
              ))}
            </select>
          </div>
          <div className="flex flex-wrap gap-2">
            {PROJECT_RISK_FILTERS.map((filter) => (
              <V3Chip
                active={props.filters.risk === filter.value}
                count={riskFilterCount(filter.value, riskCounts)}
                key={filter.value}
                onClick={() =>
                  props.onFiltersChange({
                    ...props.filters,
                    risk: filter.value,
                  })
                }
                type="button"
              >
                {filter.label}
              </V3Chip>
            ))}
          </div>
        </div>

        <V3Table>
          <thead>
            <tr>
              <V3Th className="min-w-[20rem]">项目</V3Th>
              <V3Th className="min-w-[18rem]">风险与落点</V3Th>
              <V3Th className="min-w-[7.5rem]">状态</V3Th>
              <V3Th className="min-w-[8.5rem] text-right">操作</V3Th>
            </tr>
          </thead>
          <tbody>
            {sortedProjects.map((project) => (
              <ProjectRiskQueueRow
                key={project.id}
                onSelectProject={props.onSelectProject}
                project={project}
                riskSummary={props.riskSummaries[project.id]}
                selected={project.id === props.selectedProjectId}
              />
            ))}
            {sortedProjects.length === 0 ? (
              <tr>
                <V3Td colSpan={4}>
                  <V3EmptyState
                    description="调整风险筛选、搜索关键词或项目状态后重试。"
                    icon={<FolderKanban />}
                    title="没有符合筛选条件的项目"
                  />
                </V3Td>
              </tr>
            ) : null}
          </tbody>
        </V3Table>

        <V3Pagination
          onPageChange={props.onPageChange}
          onPageSizeChange={props.onPageSizeChange}
          page={props.activePage}
          pageCount={props.pageCount}
          pageSize={props.pageSize}
          pageSizeOptions={[10, 20]}
          total={props.total}
        />
      </WorkSurface>
    </section>
  );
}

export function ProjectSelectedContextPanel({
  isLoading,
  project,
  recentEvents,
  riskSummary,
}: ProjectSelectedContextPanelProps) {
  const primaryReason = riskSummary?.primaryReason;
  const launchTaskSearch = project
    ? ({ projectId: project.id } as Record<string, string>)
    : undefined;

  return (
    <aside
      className="hidden min-w-0 xl:block"
      aria-label="选中项目上下文"
      data-testid="project-selected-context-panel"
    >
      <WorkSurface className="p-4">
        {!project ? (
          <V3EmptyState
            description="从项目队列中选择项目后查看上下文。"
            title="选择一个项目"
          />
        ) : (
          <div className="flex min-w-0 flex-col gap-4">
            <div className="flex min-w-0 items-start justify-between gap-3">
              <div className="min-w-0">
                <h2 className="line-clamp-2 text-base font-extrabold leading-6 text-v3-ink">
                  {project.name}
                </h2>
                <p className="mt-1 truncate font-mono text-xs text-v3-ink-3">
                  {project.coordination_workflow_id}
                </p>
              </div>
              <StatusPill
                tone={projectRiskLevelTone(riskSummary?.level ?? "none")}
              >
                {riskSummary ? projectRiskLevelLabel(riskSummary) : "识别中"}
              </StatusPill>
            </div>

            <div className="rounded-v3-inner border border-v3-line bg-v3-card-soft p-3">
              <div className="text-[11px] font-bold tracking-wide text-v3-ink-3 uppercase">
                主要风险
              </div>
              <p className="mt-2 line-clamp-2 text-sm font-bold text-v3-ink">
                {primaryReason?.title ?? "暂无阻塞"}
              </p>
              <p className="mt-1 line-clamp-3 text-[13px] leading-5 text-v3-ink-2">
                {primaryReason?.detail ?? "当前页未识别到需要首页处置的风险。"}
              </p>
            </div>

            <dl className="grid gap-3 text-[13px]">
              <div className="grid min-w-0 grid-cols-[4.5rem_minmax(0,1fr)] gap-2">
                <dt className="text-v3-ink-3">负责人</dt>
                <dd className="truncate font-mono text-v3-ink">
                  {project.human_owner_user_id || "未设置"}
                </dd>
              </div>
              <div className="grid min-w-0 grid-cols-[4.5rem_minmax(0,1fr)] gap-2">
                <dt className="text-v3-ink-3">协调状态</dt>
                <dd>
                  <StatusPill tone={coordinationStatusTone(project.coordination_status)}>
                    {project.coordination_status || "未登记"}
                  </StatusPill>
                </dd>
              </div>
            </dl>

            <div>
              <div className="mb-2 flex items-center justify-between gap-3">
                <h3 className="text-sm font-extrabold text-v3-ink">最近事件</h3>
                {isLoading ? (
                  <StatusPill tone="info">加载中</StatusPill>
                ) : null}
              </div>
              <div className="flex min-w-0 flex-col gap-2">
                {recentEvents.slice(0, 3).map((event) => (
                  <div
                    className="min-w-0 rounded-v3-inner border border-v3-line bg-v3-card px-3 py-2"
                    key={event.id}
                  >
                    <div className="line-clamp-2 text-[13px] font-semibold text-v3-ink">
                      {event.summary || event.event_type}
                    </div>
                    <div className="mt-1 truncate font-mono text-[11px] text-v3-ink-3">
                      #{event.sequence_number} · {event.event_type}
                    </div>
                  </div>
                ))}
                {recentEvents.length === 0 ? (
                  <div className="rounded-v3-inner border border-v3-line bg-v3-card px-3 py-3 text-sm text-v3-ink-2">
                    暂无最近事件
                  </div>
                ) : null}
              </div>
            </div>

            <div className="flex flex-wrap justify-end gap-2 border-t border-v3-line pt-3">
              <V3Button asChild size="sm" variant="outline">
                <Link params={{ projectId: project.id }} to="/projects/$projectId">
                  详情
                </Link>
              </V3Button>
              <V3Button asChild size="sm" variant="ghost">
                <Link search={launchTaskSearch} to="/task-launches">
                  发起任务
                </Link>
              </V3Button>
            </div>
          </div>
        )}
      </WorkSurface>
    </aside>
  );
}

function ProjectRiskQueueRow({
  onSelectProject,
  project,
  riskSummary,
  selected,
}: {
  onSelectProject: (projectId: string) => void;
  project: Project;
  riskSummary?: ProjectRiskSummary;
  selected: boolean;
}) {
  const summary = riskSummary ?? emptyProjectRiskSummary(project);

  return (
    <V3Tr className={cn(selected && "[&>td]:bg-v3-brand-soft/60")}>
      <V3Td className="whitespace-normal">
        <button
          aria-current={selected ? "true" : undefined}
          aria-label={`查看项目上下文 ${project.name}`}
          className="flex min-w-0 items-start gap-3 text-left"
          onClick={() => onSelectProject(project.id)}
          type="button"
        >
          <IconTile tone={projectStatusTone(project.status)} size="sm">
            {project.status === "archived" ? <Archive /> : <CircleDot />}
          </IconTile>
          <span className="min-w-0">
            <span className="block truncate font-bold text-v3-ink">
              {project.name}
            </span>
            <span className="mt-0.5 block truncate font-mono text-[12px] text-v3-ink-3">
              {project.id}
            </span>
            <span className="mt-1 block truncate text-[12px] text-v3-ink-3">
              负责人{" "}
              <span className="font-mono">
                {project.human_owner_user_id || "未设置"}
              </span>
            </span>
          </span>
        </button>
      </V3Td>
      <V3Td className="whitespace-normal">
        <div className="flex min-w-0 flex-col gap-1">
          <StatusPill tone={projectRiskLevelTone(summary.level)}>
            {projectRiskLevelLabel(summary)}
          </StatusPill>
          <span className="line-clamp-2 text-xs text-v3-ink-3">
            {summary.primaryReason?.detail ?? summary.primaryReason?.title ?? "当前页暂无风险"}
          </span>
          <span className="line-clamp-1 text-[12px] text-v3-ink-2">
            处置：{disposalTargetLabel(summary)}
          </span>
        </div>
      </V3Td>
      <V3Td>
        <StatusPill tone={projectStatusTone(project.status)}>
          {projectStatusLabel(project.status)}
        </StatusPill>
      </V3Td>
      <V3Td>
        <div className="flex justify-end gap-2">
          <V3Button
            aria-label={`选择项目 ${project.name}`}
            onClick={() => onSelectProject(project.id)}
            size="sm"
            type="button"
            variant={selected ? "outline" : "ghost"}
          >
            选择
          </V3Button>
          <V3Button asChild size="sm" variant="ghost">
            <Link params={{ projectId: project.id }} to="/projects/$projectId">
              详情
            </Link>
          </V3Button>
        </div>
      </V3Td>
    </V3Tr>
  );
}

function riskFilterCount(
  filter: ProjectRiskFilter,
  counts: ProjectRiskCounts,
) {
  return counts[filter];
}

function disposalTargetLabel(summary: ProjectRiskSummary) {
  if (summary.state === "pending") {
    return "等待风险识别完成";
  }
  if (summary.state === "error") {
    return "进入详情确认风险信号";
  }
  if (!summary.primaryReason) {
    return "进入详情查看项目上下文";
  }
  if (summary.requiresHuman) {
    return "human_owner 判断";
  }
  return summary.primaryReason.detail || summary.primaryReason.title;
}

const PROJECT_STATUS_FILTER_OPTIONS: Array<{
  label: string;
  value: "all" | ProjectStatus;
}> = [
  { label: "全部状态", value: "all" },
  { label: "运行中", value: "running" },
  { label: "配置中", value: "configuring" },
  { label: "草稿", value: "draft" },
  { label: "已暂停", value: "paused" },
  { label: "验收中", value: "acceptance" },
  { label: "已归档", value: "archived" },
];

function projectStatusLabel(status: ProjectStatus | string) {
  const labels: Record<string, string> = {
    acceptance: "验收中",
    all: "全部状态",
    archived: "已归档",
    configuring: "配置中",
    draft: "草稿",
    paused: "已暂停",
    running: "运行中",
  };
  return labels[status] ?? status;
}

function projectStatusTone(status: ProjectStatus | string): V3Tone {
  if (status === "running") return "ok";
  if (status === "archived") return "mute";
  if (status === "paused" || status === "acceptance") return "warn";
  if (status === "configuring" || status === "draft") return "info";
  return "mute";
}

function coordinationStatusTone(status?: string): V3Tone {
  const normalized = (status ?? "").trim().toLowerCase();
  if (
    normalized === "" ||
    normalized === "active" ||
    normalized === "idle" ||
    normalized === "ready" ||
    normalized === "registered" ||
    normalized === "running" ||
    normalized === "started"
  ) {
    return "ok";
  }
  return "warn";
}
