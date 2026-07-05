import { Link } from "@tanstack/react-router";
import {
  AlertTriangle,
  Clock3,
  FileWarning,
  FolderKanban,
  GitBranch,
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
import type {
  Project,
  ProjectStatus,
  WorkflowInstanceSummary,
} from "@/lib/api/projects";
import {
  workflowStatusLabel,
  workflowStatusTone,
} from "@/features/workflows/workflow-status";
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
  pageCount: number;
  pageSize: number;
  /** Unsorted current server page; this queue owns risk sorting and risk-chip filtering within that page. */
  projects: Project[];
  riskSummaries: ProjectRiskSummaryMap;
  total: number;
  workflowInstances: WorkflowInstanceSummary[];
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

        <V3Table className="min-w-[76rem] table-fixed">
          <colgroup>
            <col className="w-[21%]" data-column="project" />
            <col className="w-[22%]" data-column="task" />
            <col className="w-[17%]" data-column="node" />
            <col className="w-[12%]" data-column="handler" />
            <col className="w-[9%]" data-column="status" />
            <col className="w-[9%]" data-column="last-run" />
            <col className="w-[10%]" data-column="action" />
          </colgroup>
          <thead>
            <tr>
              <V3Th>项目</V3Th>
              <V3Th>当前任务</V3Th>
              <V3Th>当前节点/能力</V3Th>
              <V3Th>当前处理者</V3Th>
              <V3Th>执行状态</V3Th>
              <V3Th>最后运行时间</V3Th>
              <V3Th className="text-right">操作</V3Th>
            </tr>
          </thead>
          <tbody>
            {sortedProjects.map((project) => (
              <ProjectRiskQueueRow
                key={project.id}
                project={project}
                riskSummary={props.riskSummaries[project.id]}
                workflow={workflowByProjectId.get(project.id)}
              />
            ))}
            {sortedProjects.length === 0 ? (
              <tr>
                <V3Td colSpan={7}>
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
  project,
  riskSummary,
  workflow,
}: {
  project: Project;
  riskSummary?: ProjectRiskSummary;
  workflow?: WorkflowInstanceSummary;
}) {
  const summary = riskSummary ?? emptyProjectRiskSummary(project);
  const currentTask = summary.currentTask;
  const taskTitle = workflow?.title ?? currentTask?.summary ?? currentTask?.title;
  const nodeTitle = currentTask?.title ?? currentTask?.task_kind;
  const handler = summary.currentHandler?.label;

  return (
    <V3Tr>
      <V3Td className="whitespace-normal">
        <div className="flex min-w-0 items-start gap-3">
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
            <span className="mt-0.5 block min-w-0 max-w-full truncate font-mono text-[12px] text-v3-ink-3">
              {project.id}
            </span>
            <span className="mt-1 block min-w-0 max-w-full truncate text-[12px] text-v3-ink-3">
              负责人{" "}
              <span className="font-mono">
                {project.human_owner_user_id || "未设置"}
              </span>
            </span>
            <span className="mt-2 block">
              <StatusPill tone={projectRiskLevelTone(summary.level)}>
                {projectRiskLevelLabel(summary)}
              </StatusPill>
            </span>
          </span>
        </div>
      </V3Td>
      <V3Td className="whitespace-normal">
        <div className="flex min-w-0 flex-col gap-1">
          <span
            className="max-h-10 line-clamp-2 break-words text-sm font-bold leading-5 text-v3-ink"
            data-testid="project-queue-current-task"
          >
            {taskTitle ?? "暂无任务发起记录"}
          </span>
          <span className="block min-w-0 max-w-full truncate text-[12px] text-v3-ink-3">
            {workflow ? `需求 ${workflow.demand_id}` : project.goal}
          </span>
        </div>
      </V3Td>
      <V3Td className="whitespace-normal">
        <div className="flex min-w-0 flex-col gap-1">
          <span className="line-clamp-2 break-words text-[13px] font-semibold leading-5 text-v3-ink">
            {nodeTitle ?? "暂无执行节点"}
          </span>
          <span className="block min-w-0 max-w-full truncate font-mono text-[11px] text-v3-ink-3">
            {currentTask?.task_kind ?? currentTask?.planned_task_key ?? "未登记能力"}
          </span>
        </div>
      </V3Td>
      <V3Td className="whitespace-normal">
        <span
          className="block min-w-0 max-h-10 max-w-full line-clamp-2 break-words text-[12px] font-semibold leading-5 text-v3-ink"
          data-testid="project-queue-current-handler"
        >
          {handler ?? "待调度"}
        </span>
      </V3Td>
      <V3Td className="whitespace-normal">
        <div className="flex min-w-0 flex-col items-start gap-1">
          <StatusPill
            tone={workflow ? workflowStatusTone(workflow.status) : projectStatusTone(project.status)}
          >
            {workflow ? workflowStatusLabel(workflow.status) : projectStatusLabel(project.status)}
          </StatusPill>
          {workflow?.status_reason ? (
            <span className="line-clamp-1 max-w-full break-words text-[11px] text-v3-ink-3">
              {workflow.status_reason}
            </span>
          ) : null}
        </div>
      </V3Td>
      <V3Td className="whitespace-nowrap">
        <span className="block min-w-0 max-w-full truncate font-mono text-[12px] text-v3-ink-2">
          {workflow ? formatRunTime(workflow.updated_at) : "暂无运行记录"}
        </span>
      </V3Td>
      <V3Td className="whitespace-nowrap text-right">
        <div className="flex min-w-0 justify-end gap-2">
          {workflow ? (
            <V3Button asChild className="max-w-full" size="sm" variant="outline">
              <Link
                aria-label={`进入流程编排 ${project.name}`}
                params={{ demandId: workflow.demand_id }}
                to="/workflows/$demandId"
              >
                <GitBranch data-icon="inline-start" />
                进入流程编排
              </Link>
            </V3Button>
          ) : (
            <V3Button asChild className="max-w-full" size="sm" variant="ghost">
              <Link aria-label={`进入流程编排 ${project.name}`} to="/workflows">
                <GitBranch data-icon="inline-start" />
                进入流程编排
              </Link>
            </V3Button>
          )}
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
