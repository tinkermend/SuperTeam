import type { KeyboardEvent } from "react";
import { Link } from "@tanstack/react-router";
import { cn } from "@/lib/utils";
import {
  AlertTriangle,
  ArrowRight,
  Clock3,
  FileWarning,
  FolderKanban,
  PlayCircle,
  ShieldCheck,
  UserCheck,
  UserRound,
} from "lucide-react";
import {
  IconTile,
  MetricGrid,
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
import type {
  Project,
  ProjectStatus,
  WorkflowInstanceSummary,
} from "@/lib/api/projects";
import { projectStatusLabel } from "@/lib/status-labels";
import {
  buildRiskCounts,
  emptyProjectRiskSummary,
  matchesProjectRiskFilter,
  PROJECT_RISK_FILTERS,
  projectRiskLevelLabel,
  projectRiskLevelTone,
  sortProjectsByRisk,
  type ProjectPortfolioCounts,
  type ProjectRiskCounts,
  type ProjectRiskFilter,
  type ProjectRiskReason,
  type ProjectRiskReasonType,
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
  /** Optional action slot rendered right-aligned in the queue header (e.g. 新建项目). */
  createAction?: React.ReactNode;
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
  workflowInstances: WorkflowInstanceSummary[];
};

/** KPI 卡语义映射：icon 圆底色 / 数字色 / 顶部装饰条实色。 */
const kpiSoftBg: Record<V3Tone, string> = {
  brand: "bg-v3-brand-soft",
  info: "bg-v3-info-soft",
  ok: "bg-v3-ok-soft",
  warn: "bg-v3-warn-soft",
  danger: "bg-v3-danger-soft",
  mute: "bg-v3-mute-soft",
  artifact: "bg-v3-artifact-soft",
};

const kpiNumText: Record<V3Tone, string> = {
  brand: "text-v3-brand-deep",
  info: "text-v3-info-text",
  ok: "text-v3-ok-text",
  warn: "text-v3-warn-text",
  danger: "text-v3-danger-text",
  mute: "text-v3-mute-text",
  artifact: "text-v3-artifact-text",
};

const kpiAccentBar: Record<V3Tone, string> = {
  brand: "bg-v3-brand",
  info: "bg-v3-info",
  ok: "bg-v3-ok",
  warn: "bg-v3-warn",
  danger: "bg-v3-danger",
  mute: "bg-v3-mute",
  artifact: "bg-v3-artifact",
};

/**
 * 项目组合真值条：只用完整已加载列表上的廉价字段（status / coordination_status），
 * 覆盖整个已加载列表而非当前页。逐项目风险（人工决策 / 执行失败 / 证据待补）是页级派生，
 * 保留在下方队列表头与筛选 chip 中，避免把页级计数伪装成全平台真值。
 */
export function ProjectPortfolioSummaryBar({
  portfolioCounts,
}: {
  portfolioCounts: ProjectPortfolioCounts;
}) {
  const items = [
    { icon: FolderKanban, label: "已加载项目", tone: "brand" as V3Tone, value: portfolioCounts.total },
    { icon: PlayCircle, label: "运行中", tone: "ok" as V3Tone, value: portfolioCounts.running },
    { icon: ShieldCheck, label: "验收中", tone: "info" as V3Tone, value: portfolioCounts.acceptance },
    { icon: AlertTriangle, label: "协调异常", tone: "danger" as V3Tone, value: portfolioCounts.coordinationAnomaly },
  ];

  return (
    <MetricGrid aria-label="项目组合概览（已加载列表）">
      {items.map((item) => {
        const Icon = item.icon;
        return (
          <div
            key={item.label}
            className="relative flex flex-col gap-2 overflow-hidden rounded-v3-card border border-v3-line bg-v3-card px-4 py-3.5 shadow-v3 transition-all duration-200 hover:-translate-y-0.5 hover:border-v3-brand/40 hover:shadow-v3-pop"
          >
            {/* tone accent bar — mirrors the selected-state left bar in employee gallery */}
            <span
              aria-hidden
              className={cn("absolute inset-x-0 top-0 h-0.5", kpiAccentBar[item.tone])}
            />

            {/* icon in soft circle */}
            <div
              className={cn(
                "flex size-8 items-center justify-center rounded-full",
                kpiSoftBg[item.tone],
              )}
            >
              <Icon
                aria-hidden
                className={cn("size-[15px]", kpiNumText[item.tone])}
              />
            </div>

            {/* number */}
            <div
              className={cn(
                "text-2xl font-extrabold leading-none tabular-nums",
                kpiNumText[item.tone],
              )}
            >
              {item.value}
            </div>

            {/* label */}
            <div className="text-[11.5px] font-medium leading-none text-v3-ink-3">
              {item.label}
            </div>
          </div>
        );
      })}
    </MetricGrid>
  );
}

export function ProjectRiskQueue(props: ProjectRiskQueueProps) {
  const workflowByProjectId = useLatestWorkflowByProjectId(props.workflowInstances);
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
      data-density="compact"
      data-testid="project-risk-queue"
    >
      <WorkSurface className="min-w-0 rounded-[14px] shadow-sm">
        <div className="flex min-w-0 flex-col gap-2 border-b border-v3-line bg-v3-card px-3 py-3 lg:flex-row lg:items-start lg:justify-between">
          <div className="min-w-0">
            <h2 className="text-base font-extrabold text-v3-ink">项目队列</h2>
            <div className="mt-1 flex flex-wrap items-center gap-1.5">
              <StatusPill tone={props.isFetching ? "info" : "mute"}>
                {props.isFetching ? "正在识别风险" : `${props.total} 个项目`}
              </StatusPill>
              <StatusPill tone={riskCounts.blocked > 0 ? "danger" : "mute"}>
                {riskCounts.blocked} 个阻塞（当前页）
              </StatusPill>
            </div>
            <p className="mt-1 text-[11px] leading-5 text-v3-ink-3">
              按需要介入程度排序；选中项目在右侧查看待办与直达动作。风险识别基于当前页，筛选仅过滤当前页项目，分页仍对应完整项目列表。
            </p>
          </div>
          {props.createAction ? (
            <div className="shrink-0 lg:pt-0.5">{props.createAction}</div>
          ) : null}
        </div>

        <div className="flex min-w-0 flex-col gap-2 border-b border-v3-line bg-v3-card-soft/45 px-3 py-3">
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
              className="h-8 rounded-[8px] border border-v3-line bg-v3-card px-2.5 text-[12px] text-v3-ink outline-none transition-colors hover:bg-v3-card-soft focus:border-v3-brand focus:ring-2 focus:ring-v3-brand/25"
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
                className="rounded-[8px] px-2.5 py-1.5 text-[12px]"
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

        <V3Table className="min-w-[46rem] table-fixed text-[12px]">
          <colgroup>
            <col className="w-[30%]" data-column="project" />
            <col className="w-[26%]" data-column="pending" />
            <col className="w-[16%]" data-column="handler" />
            <col className="w-[12%]" data-column="last-run" />
            <col className="w-[16%]" data-column="action" />
          </colgroup>
          <thead>
            <tr>
              <V3Th className="px-3 py-2">项目</V3Th>
              <V3Th className="px-3 py-2">待处理</V3Th>
              <V3Th className="px-3 py-2">当前处理者</V3Th>
              <V3Th className="px-3 py-2">最近活动</V3Th>
              <V3Th className="px-3 py-2 text-right">操作</V3Th>
            </tr>
          </thead>
          <tbody>
            {sortedProjects.map((project) => (
              <ProjectRiskQueueRow
                isSelected={props.selectedProjectId === project.id}
                key={project.id}
                onSelect={props.onSelectProject}
                project={project}
                riskSummary={props.riskSummaries[project.id]}
                workflow={workflowByProjectId.get(project.id)}
              />
            ))}
            {sortedProjects.length === 0 ? (
              <tr>
                <V3Td className="px-3 py-3" colSpan={5}>
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

function ProjectRiskQueueRow({
  isSelected,
  onSelect,
  project,
  riskSummary,
  workflow,
}: {
  isSelected: boolean;
  onSelect: (projectId: string) => void;
  project: Project;
  riskSummary?: ProjectRiskSummary;
  workflow?: WorkflowInstanceSummary;
}) {
  const summary = riskSummary ?? emptyProjectRiskSummary(project);
  const pendingCount = summary.reasons.length;
  const ownerLabel = summary.owner?.label ?? project.human_owner_user_id ?? "未设置";
  const handler = summary.currentHandler?.label;

  const handleKeyDown = (event: KeyboardEvent<HTMLTableRowElement>) => {
    if (event.target !== event.currentTarget) {
      return;
    }
    if (event.key === "Enter" || event.key === " ") {
      event.preventDefault();
      onSelect(project.id);
    }
  };

  return (
    <V3Tr
      aria-selected={isSelected}
      className={[
        "cursor-pointer",
        isSelected ? "bg-v3-brand-soft/85" : "hover:bg-v3-card-soft/60",
        summary.level === "danger"
          ? "[&>td:first-child]:shadow-[inset_3px_0_0_var(--v3-danger)]"
          : summary.level === "warn"
            ? "[&>td:first-child]:shadow-[inset_3px_0_0_var(--v3-warn)]"
            : "",
      ]
        .filter(Boolean)
        .join(" ")}
      data-selected={isSelected}
      onClick={() => onSelect(project.id)}
      onKeyDown={handleKeyDown}
      tabIndex={0}
    >
      <V3Td className="whitespace-normal px-3 py-2">
        <div className="flex min-w-0 items-start gap-2.5">
          <IconTile tone={projectStatusTone(project.status)} size="sm">
            <FolderKanban />
          </IconTile>
          <span className="min-w-0 flex-1">
            <span
              className="block min-h-0 min-w-0 max-h-10 max-w-full line-clamp-2 break-words font-bold leading-5 text-v3-ink"
              data-testid="project-queue-project-title"
            >
              {project.name}
            </span>
            <span className="mt-1 flex min-w-0 max-w-full items-center gap-1 truncate text-[12px] text-v3-ink-3">
              <UserRound aria-hidden className="size-3 shrink-0" />
              <span className="truncate">{ownerLabel}</span>
            </span>
            <span className="mt-1.5 flex flex-wrap items-center gap-1">
              <StatusPill tone={projectStatusTone(project.status)}>
                {projectStatusLabel(project.status)}
              </StatusPill>
            </span>
          </span>
        </div>
      </V3Td>
      <V3Td className="whitespace-normal px-3 py-2">
        <div className="flex min-w-0 flex-col gap-1" data-testid="project-queue-pending">
          <StatusPill tone={projectRiskLevelTone(summary.level)}>
            {projectRiskLevelLabel(summary)}
          </StatusPill>
          <span className="block min-w-0 max-w-full text-[12px] text-v3-ink-3">
            {pendingCount > 0 ? `${pendingCount} 项待处理` : "无待处理项"}
          </span>
        </div>
      </V3Td>
      <V3Td className="whitespace-normal px-3 py-2">
        <span
          className="block min-w-0 max-h-10 max-w-full line-clamp-2 break-words text-[12px] font-semibold leading-5 text-v3-ink"
          data-testid="project-queue-current-handler"
        >
          {handler ?? "待调度"}
        </span>
      </V3Td>
      <V3Td className="whitespace-nowrap px-3 py-2">
        <span className="block min-w-0 max-w-full truncate font-mono text-[12px] text-v3-ink-2">
          {workflow ? formatRunTime(workflow.updated_at) : "暂无运行记录"}
        </span>
      </V3Td>
      <V3Td className="whitespace-nowrap px-3 py-2 text-right">
        <div
          className="flex min-w-0 flex-col items-stretch justify-end gap-1.5"
          onClick={(event) => event.stopPropagation()}
        >
          <V3Button asChild className="max-w-full justify-center" size="sm">
            <Link
              aria-label={`进入项目 ${project.name}`}
              params={{ projectId: project.id }}
              to="/projects/$projectId"
            >
              进入项目
              <ArrowRight data-icon="inline-end" />
            </Link>
          </V3Button>
        </div>
      </V3Td>
    </V3Tr>
  );
}

const REASON_META: Record<
  ProjectRiskReasonType,
  { icon: typeof UserCheck; tab: string; action: string }
> = {
  human_decision: { icon: UserCheck, tab: "approval", action: "处理决策" },
  execution_failed: { icon: AlertTriangle, tab: "tasks", action: "查看失败任务" },
  runtime_or_coordination: { icon: PlayCircle, tab: "overview", action: "查看协调状态" },
  evidence_required: { icon: FileWarning, tab: "acceptance", action: "补充证据" },
  sla_waiting: { icon: Clock3, tab: "overview", action: "查看等待原因" },
};

/**
 * 选中项目上下文（主从详情的从属侧）：复用队列已计算的风险摘要，零额外请求。
 * 按需渲染——仅在选中项目时由 MasterDetailLayout 装载（宽容器 in-flow 右栏 /
 * 窄容器 Sheet），未选中不保留空态占位栏。
 */
export function ProjectTriagePanel({
  project,
  summary,
}: {
  project: Project;
  summary?: ProjectRiskSummary;
}) {
  const resolvedSummary = summary ?? emptyProjectRiskSummary(project);
  const reasons = resolvedSummary.reasons;
  const ownerLabel =
    resolvedSummary.owner?.label ?? project.human_owner_user_id ?? "未设置";

  return (
    <aside
      aria-label="选中项目上下文"
      className="flex min-w-0 flex-col gap-3 rounded-[14px] border border-v3-line bg-v3-card p-4 shadow-sm @5xl/master-detail:sticky @5xl/master-detail:top-4"
      data-testid="project-selected-context-panel"
    >
      <div className="flex min-w-0 items-start gap-2.5">
        <IconTile tone={projectStatusTone(project.status)} size="sm">
          <FolderKanban />
        </IconTile>
        <div className="min-w-0 flex-1">
          <h3 className="min-w-0 break-words text-sm font-extrabold leading-5 text-v3-ink">
            {project.name}
          </h3>
          {project.goal ? (
            <p className="mt-0.5 line-clamp-2 text-[11.5px] leading-[1.45] text-v3-ink-3">
              {project.goal}
            </p>
          ) : null}
          <p className="mt-1 flex min-w-0 items-center gap-1 truncate text-[12px] text-v3-ink-3">
            <UserRound aria-hidden className="size-3 shrink-0" />
            负责人 <span className="truncate font-medium text-v3-ink-2">{ownerLabel}</span>
          </p>
          <div className="mt-1.5 flex flex-wrap items-center gap-1.5">
            <StatusPill tone={projectStatusTone(project.status)}>
              {projectStatusLabel(project.status)}
            </StatusPill>
            <StatusPill tone={projectRiskLevelTone(resolvedSummary.level)}>
              {projectRiskLevelLabel(resolvedSummary)}
            </StatusPill>
          </div>
        </div>
      </div>

      <div className="min-w-0 rounded-[10px] bg-v3-card-soft/60 p-3">
        <div className="text-[11px] font-semibold uppercase tracking-wide text-v3-ink-3">
          待办 {reasons.length > 0 ? `· ${reasons.length}` : ""}
        </div>
        {reasons.length === 0 ? (
          <p className="mt-1.5 text-[12px] leading-5 text-v3-ink-2">
            该项目当前没有需要人工介入的阻塞。{" "}
            <Link
              className="text-v3-brand underline underline-offset-2 hover:opacity-75"
              params={{ projectId: project.id }}
              to="/projects/$projectId"
            >
              进入项目
            </Link>{" "}
            可查看任务、证据与验收进度。
          </p>
        ) : (
          <ul className="mt-2 flex flex-col gap-2">
            {reasons.map((reason) => (
              <ProjectTriageReasonRow
                key={reason.id}
                projectId={project.id}
                reason={reason}
              />
            ))}
          </ul>
        )}
      </div>

      </aside>
    );
  }

function ProjectTriageReasonRow({
  projectId,
  reason,
}: {
  projectId: string;
  reason: ProjectRiskReason;
}) {
  const meta = REASON_META[reason.type];
  const Icon = meta.icon;
  const focus =
    reason.type === "human_decision" && reason.source === "decisions"
      ? reason.sourceId
      : undefined;

  return (
    <li className="flex min-w-0 items-start gap-2 rounded-[8px] bg-v3-card px-2.5 py-2">
      <IconTile tone={projectRiskLevelTone(reason.level)} size="sm">
        <Icon />
      </IconTile>
      <div className="min-w-0 flex-1">
        <p className="min-w-0 break-words text-[12px] font-semibold leading-5 text-v3-ink">
          {reason.title}
        </p>
        {reason.detail ? (
          <p className="mt-0.5 min-w-0 truncate font-mono text-[11px] text-v3-ink-3">
            {reason.detail}
          </p>
        ) : null}
      </div>
      <V3Button asChild className="shrink-0" size="sm" variant="outline">
        <Link
          params={{ projectId }}
          search={focus ? { focus, tab: meta.tab } : { tab: meta.tab }}
          to="/projects/$projectId"
        >
          {meta.action}
        </Link>
      </V3Button>
    </li>
  );
}

function riskFilterCount(
  filter: ProjectRiskFilter,
  counts: ProjectRiskCounts,
) {
  return counts[filter];
}

const PROJECT_STATUS_FILTER_OPTIONS: Array<{
  label: string;
  value: "all" | ProjectStatus;
}> = [
  { label: "全部状态", value: "all" },
  { label: projectStatusLabel("running"), value: "running" },
  { label: projectStatusLabel("configuring"), value: "configuring" },
  { label: projectStatusLabel("draft"), value: "draft" },
  { label: projectStatusLabel("paused"), value: "paused" },
  { label: projectStatusLabel("acceptance"), value: "acceptance" },
  { label: projectStatusLabel("archived"), value: "archived" },
];

function projectStatusTone(status: ProjectStatus | string): V3Tone {
  if (status === "running") return "ok";
  if (status === "archived") return "mute";
  if (status === "acceptance") return "brand";
  if (status === "paused") return "mute";
  if (status === "configuring" || status === "draft") return "info";
  return "mute";
}

function useLatestWorkflowByProjectId(workflows: WorkflowInstanceSummary[]) {
  const latestByProjectId = new Map<string, WorkflowInstanceSummary>();

  for (const workflow of workflows) {
    const existing = latestByProjectId.get(workflow.project_id);
    if (!existing || compareOptionalTime(workflow.updated_at, existing.updated_at) > 0) {
      latestByProjectId.set(workflow.project_id, workflow);
    }
  }

  return latestByProjectId;
}

function compareOptionalTime(left?: string, right?: string): number {
  const leftTime = left ? Date.parse(left) : undefined;
  const rightTime = right ? Date.parse(right) : undefined;
  const leftValid = leftTime !== undefined && !Number.isNaN(leftTime);
  const rightValid = rightTime !== undefined && !Number.isNaN(rightTime);
  if (!leftValid && !rightValid) {
    return 0;
  }
  if (!leftValid) {
    return -1;
  }
  if (!rightValid) {
    return 1;
  }
  return leftTime - rightTime;
}

function formatRunTime(value: string): string {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return value;
  }
  return new Intl.DateTimeFormat("zh-CN", {
    day: "2-digit",
    hour: "2-digit",
    hour12: false,
    minute: "2-digit",
    month: "2-digit",
  }).format(date);
}
