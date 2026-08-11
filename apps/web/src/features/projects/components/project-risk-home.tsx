import { useState, type KeyboardEvent } from "react";
import { Link } from "@tanstack/react-router";
import { cn } from "@/lib/utils";
import {
  AlertTriangle,
  ArrowRight,
  Clock3,
  FileWarning,
  FolderKanban,
  PlayCircle,
  UserCheck,
  UserRound,
  X
} from "lucide-react";
import {
  IconTile,
  StatusPill,
  IconButton,
  Button,
  Chip,
  EmptyState,
  Pagination,
  DataTable,
  Td,
  Th,
  ToolbarSearch,
  Tr,
  WorkSurface,
  type Tone
} from "@/components/superteam";
import type {
  Project,
  ProjectStatus,
} from "@/lib/api/projects";
import { projectStatusLabel } from "@/lib/status-labels";
import {
  buildAttentionBreakdown,
  buildProjectPortfolioCounts,
  buildRiskCounts,
  emptyProjectRiskSummary,
  formatAttentionHeadline,
  formatProjectQueueHandlerLabel,
  isActionableRiskReason,
  PROJECT_RISK_FILTERS,
  projectRiskLevelLabel,
  projectRiskLevelTone,
  projectRiskReasonLabel,
  resolveProjectOwnerLabel,
  sortProjectsByRisk,
  type ProjectPortfolioCounts,
  type ProjectRiskCounts,
  type ProjectRiskFilter,
  type ProjectRiskReason,
  type ProjectRiskReasonType,
  type ProjectRiskSummary,
  type ProjectRiskSummaryMap
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
  /** 命中 listProjects limit 上限时显示截断提示。 */
  listCapped?: boolean;
  /**
   * 已加载全量上的关注 chip 计数（run-summary 真值）。
   * 未传时回退为当前 `projects` 页内计数。
   */
  loadedRiskCounts?: ProjectRiskCounts;
  onFiltersChange: (filters: ProjectRiskQueueFilters) => void;
  onPageChange: (page: number) => void;
  onPageSizeChange: (pageSize: number) => void;
  onSelectProject: (projectId: string) => void;
  pageCount: number;
  pageSize: number;
  /** principal_id → 展示名；成员快照缺失时负责人行回退用。 */
  principalNamesById?: ReadonlyMap<string, string>;
  /**
   * 当前页行（父层已按风险筛选 + 排序 + 切片）。
   * chip 计数请用 `loadedRiskCounts`，勿再对本页二次统计冒充全量。
   */
  projects: Project[];
  riskSummaries: ProjectRiskSummaryMap;
  selectedProjectId?: string;
  /** 已加载项目数（截断提示与「已加载 N」pill）。 */
  total: number;
  /** 风险筛选后的可见条数（分页 total；默认 = projects 长度）。 */
  visibleTotal?: number;
};

/** KPI 卡语义映射：icon 圆底色 / 数字色 / 顶部装饰条实色。 */
/**
 * 项目组合紧凑真值条（S1）：单行弱样式，无 Inbox 卡。
 * 数字默认来自已加载列表廉价字段；可选 totalLabel 改「全部」。
 */
export function ProjectPortfolioSummaryBar({
  portfolioCounts,
  totalLabel = "已加载",
}: {
  portfolioCounts: ProjectPortfolioCounts;
  /** 总数标签：截断场景用「已加载」，全量 run-summary 场景可用「全部」。 */
  totalLabel?: string;
}) {
  const parts: Array<{ label: string; value: number; tone?: "warn" | "danger" }> = [
    { label: totalLabel, value: portfolioCounts.total },
    { label: projectStatusLabel("running"), value: portfolioCounts.running },
    { label: "验收中", value: portfolioCounts.acceptance },
    {
      label: "协调异常",
      value: portfolioCounts.coordinationAnomaly,
      tone: portfolioCounts.coordinationAnomaly > 0 ? "danger" : undefined,
    },
  ];

  return (
    <div
      aria-label="项目组合概览"
      className="flex min-w-0 flex-wrap items-center gap-x-3 gap-y-1.5 rounded-[14px] border border-line bg-card px-3.5 py-2.5 text-[12.5px] text-ink-3 shadow-sm"
      data-testid="project-portfolio-summary-bar"
    >
      {parts.map((part, index) => (
        <span key={part.label} className="inline-flex items-center gap-1.5">
          {index > 0 ? (
            <span aria-hidden className="mr-1.5 text-line-strong">
              ·
            </span>
          ) : null}
          <span>{part.label}</span>
          <span
            className={cn(
              "font-extrabold tabular-nums text-ink",
              part.tone === "danger" && "text-danger-text",
              part.tone === "warn" && "text-warn-text",
            )}
          >
            {part.value}
          </span>
        </span>
      ))}
    </div>
  );
}

export function ProjectRiskQueue(props: ProjectRiskQueueProps) {
  const [showMoreFilters, setShowMoreFilters] = useState(false);
  // 父层已做风险筛选+排序+分页；本组件只渲染当前页行。
  const sortedProjects = props.projects;
  const riskCounts =
    props.loadedRiskCounts ??
    buildRiskCounts(
      props.projects.map(
        (project) =>
          props.riskSummaries[project.id] ?? emptyProjectRiskSummary(project),
      ),
    );
  const paginationTotal = props.visibleTotal ?? props.projects.length;

  return (
    <section
      aria-label="项目队列"
      className="min-w-0"
      data-density="compact"
      data-testid="project-risk-queue"
    >
      <WorkSurface className="min-w-0 rounded-[14px] shadow-sm">
        <div className="flex min-w-0 flex-col gap-2 border-b border-line bg-card px-3 py-3 lg:flex-row lg:items-start lg:justify-between">
          <div className="min-w-0">
            <h2 className="text-base font-extrabold text-ink">项目队列</h2>
            <div className="mt-1 flex flex-wrap items-center gap-1.5">
              <StatusPill tone={props.isFetching ? "info" : "mute"}>
                {props.isFetching ? "加载中" : `已加载 ${props.total} 个项目`}
              </StatusPill>
              <StatusPill tone={riskCounts.blocked > 0 ? "danger" : "mute"}>
                {riskCounts.blocked} 个有关注信号
              </StatusPill>
            </div>
            {props.listCapped ? (
              <p className="mt-1 text-[11px] leading-5 text-warn-text" data-testid="project-list-capped-hint">
                已加载前 50 个项目，请用搜索或状态筛选缩小范围
              </p>
            ) : null}
          </div>
          {props.createAction ? (
            <div className="shrink-0 lg:pt-0.5">{props.createAction}</div>
          ) : null}
        </div>

        <div className="flex min-w-0 flex-col gap-2 border-b border-line bg-card-soft/45 px-3 py-3">
          <div className="flex min-w-0 flex-col gap-2 lg:flex-row lg:items-center">
            <ToolbarSearch
              aria-label="搜索项目"
              className="min-w-0 lg:max-w-md"
              onChange={(event) =>
                props.onFiltersChange({
                  ...props.filters,
                  q: event.target.value
})
              }
              placeholder="搜索项目名称或目标"
              value={props.filters.q}
            />
            <select
              aria-label="项目状态筛选"
              className="h-8 rounded-[8px] border border-line bg-card px-2.5 text-[12px] text-ink outline-none transition-colors hover:bg-card-soft focus:border-brand focus:ring-2 focus:ring-brand/25"
              onChange={(event) =>
                props.onFiltersChange({
                  ...props.filters,
                  status: event.target
                    .value as ProjectRiskQueueProps["filters"]["status"]
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
            {PROJECT_RISK_FILTERS.filter(
              (filter) =>
                // 列表态不可用的桶（如 sla_waiting：run-summary 无等待起点，恒 0）不渲染，
                // 否则是个点了必空的死 chip。
                filter.listAvailable !== false &&
                (filter.defaultVisible !== false || showMoreFilters || props.filters.risk === filter.value),
            ).map((filter) => (
              <Chip
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
              </Chip>
            ))}
            <button
              className="rounded-[8px] px-2.5 py-1.5 text-[12px] font-semibold text-brand hover:opacity-75"
              onClick={() => setShowMoreFilters((v) => !v)}
              type="button"
            >
              {showMoreFilters ? "收起筛选" : "更多筛选"}
            </button>
          </div>
        </div>

        <DataTable className="min-w-[46rem] table-fixed text-[12px]">
          <colgroup>
            <col className="w-[34%]" data-column="project" />
            <col className="w-[20%]" data-column="pending" />
            <col className="w-[20%]" data-column="handler" />
            <col className="w-[13%]" data-column="last-run" />
            <col className="w-[13%]" data-column="action" />
          </colgroup>
          <thead>
            <tr>
              <Th className="px-3 py-2">项目</Th>
              <Th className="px-3 py-2">关注摘要</Th>
              <Th className="px-3 py-2">执行摘要</Th>
              <Th className="px-3 py-2">最近活动</Th>
              <Th className="px-3 py-2 text-right">操作</Th>
            </tr>
          </thead>
          <tbody>
            {sortedProjects.map((project) => (
              <ProjectRiskQueueRow
                isSelected={props.selectedProjectId === project.id}
                key={project.id}
                onSelect={props.onSelectProject}
                principalNamesById={props.principalNamesById}
                project={project}
                riskSummary={props.riskSummaries[project.id]}
              />
            ))}
            {sortedProjects.length === 0 ? (
              <tr>
                <Td className="px-3 py-3" colSpan={5}>
                  <EmptyState
                    description="调整风险筛选、搜索关键词或项目状态后重试。"
                    icon={<FolderKanban />}
                    title="没有符合筛选条件的项目"
                  />
                </Td>
              </tr>
            ) : null}
          </tbody>
        </DataTable>

        <Pagination
          onPageChange={props.onPageChange}
          onPageSizeChange={props.onPageSizeChange}
          page={props.activePage}
          pageCount={props.pageCount}
          pageSize={props.pageSize}
          pageSizeOptions={[10, 20]}
          total={paginationTotal}
          totalLabel="已加载范围内"
        />
      </WorkSurface>
    </section>
  );
}

function ProjectRiskQueueRow({
  isSelected,
  onSelect,
  principalNamesById,
  project,
  riskSummary,
}: {
  isSelected: boolean;
  onSelect: (projectId: string) => void;
  principalNamesById?: ReadonlyMap<string, string>;
  project: Project;
  riskSummary?: ProjectRiskSummary;
}) {
  const summary = riskSummary ?? emptyProjectRiskSummary(project);
  const attention = formatAttentionHeadline(summary);
  const ownerLabel = resolveProjectOwnerLabel(
    project,
    summary.owner,
    principalNamesById,
  );
  const activityAt = summary.lastActivityAt ?? project.updated_at;

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
    <Tr
      aria-selected={isSelected}
      className={[
        "cursor-pointer",
        isSelected ? "bg-brand-soft/85" : "hover:bg-card-soft/60",
        summary.level === "danger"
          ? "[&>td:first-child]:shadow-[inset_3px_0_0_var(--danger)]"
          : summary.level === "warn"
            ? "[&>td:first-child]:shadow-[inset_3px_0_0_var(--warn)]"
            : "",
      ]
        .filter(Boolean)
        .join(" ")}
      data-selected={isSelected}
      onClick={() => onSelect(project.id)}
      onKeyDown={handleKeyDown}
      tabIndex={0}
    >
      <Td className="whitespace-normal px-3 py-2">
        <div className="flex min-w-0 items-start gap-2.5">
          <IconTile tone={projectStatusTone(project.status)} size="sm">
            <FolderKanban />
          </IconTile>
          <span className="min-w-0 flex-1">
            <span className="flex min-w-0 flex-wrap items-center gap-1.5">
              <span
                className="min-w-0 max-w-full truncate font-bold leading-5 text-ink"
                data-testid="project-queue-project-title"
                title={project.name}
              >
                {project.name}
              </span>
              <StatusPill tone={projectStatusTone(project.status)}>
                {projectStatusLabel(project.status)}
              </StatusPill>
            </span>
            <span className="mt-1 flex min-w-0 max-w-full items-center gap-1 truncate text-[12px] text-ink-3">
              <UserRound aria-hidden className="size-3 shrink-0" />
              <span className="truncate" data-testid="project-queue-owner">
                {ownerLabel}
              </span>
            </span>
          </span>
        </div>
      </Td>
      <Td className="whitespace-normal px-3 py-2">
        <div
          className="flex min-w-0 flex-col items-start gap-1"
          data-testid="project-queue-pending"
        >
          {summary.level === "none" && !attention.primary ? (
            <StatusPill tone="mute">暂无阻塞</StatusPill>
          ) : (
            <StatusPill tone={projectRiskLevelTone(summary.level)}>
              {projectRiskLevelLabel(summary)}
            </StatusPill>
          )}
          {attention.primary ? (
            <span
              className="block min-w-0 max-w-full text-[12px] text-ink-3"
              data-testid="project-queue-attention-headline"
              title={
                attention.detail
                  ? `${attention.primary}；${attention.detail}`
                  : attention.primary
              }
            >
              {attention.primary}
              {attention.detail ? (
                <span className="mt-0.5 block truncate text-[11px] text-ink-3/80">
                  {attention.detail}
                </span>
              ) : null}
            </span>
          ) : null}
        </div>
      </Td>
      <Td className="whitespace-normal px-3 py-2">
        <span
          className="block min-w-0 max-h-10 max-w-full line-clamp-2 break-words text-[12px] font-semibold leading-5 text-ink"
          data-testid="project-queue-current-handler"
          title="多任务时为摘要，非全部任务状态"
        >
          {formatProjectQueueHandlerLabel(summary, project)}
        </span>
      </Td>
      <Td className="whitespace-nowrap px-3 py-2">
        <span className="block min-w-0 max-w-full truncate font-mono text-[12px] text-ink-2">
          {activityAt ? formatRunTime(activityAt) : "暂无运行记录"}
        </span>
      </Td>
      <Td className="whitespace-nowrap px-3 py-2 text-right">
        <div
          className="flex min-w-0 flex-col items-stretch justify-end gap-1.5"
          onClick={(event) => event.stopPropagation()}
        >
          <Button asChild className="max-w-full justify-center" size="sm">
            <Link
              aria-label={`进入项目 ${project.name}`}
              params={{ projectId: project.id }}
              to="/projects/$projectId"
            >
              进入项目
              <ArrowRight data-icon="inline-end" />
            </Link>
          </Button>
        </div>
      </Td>
    </Tr>
  );
}

const REASON_META: Record<
  ProjectRiskReasonType,
  { icon: typeof UserCheck; tab: string; action: string }
> = {
  human_decision: { icon: UserCheck, tab: "approval", action: "处理决策" },
  waiting_human: { icon: UserCheck, tab: "tasks", action: "查看等人任务" },
  execution_failed: { icon: AlertTriangle, tab: "tasks", action: "查看失败任务" },
  runtime_or_coordination: { icon: PlayCircle, tab: "overview", action: "查看协调状态" },
  evidence_required: { icon: FileWarning, tab: "assets", action: "查看证据核验" },
  sla_waiting: { icon: Clock3, tab: "overview", action: "查看等待原因" }
};

/**
 * 选中项目上下文（主从详情的从属侧）：复用队列已计算的风险摘要，零额外请求。
 * 按需渲染——仅在选中项目时由 MasterDetailLayout 装载（宽容器 in-flow 右栏 /
 * 窄容器 Sheet），未选中不保留空态占位栏。
 */
export function ProjectTriagePanel({
  detailState = "ready",
  onClose,
  principalNamesById,
  project,
  summary
}: {
  /**
   * 单项目明细（deriveProjectRiskSummary）的加载态。列表态摘要只有计数桶、
   * 其 reasons 是按桶合成的占位（title 就是项目名、无 sourceId），
   * 直接渲染会变成看着像真条目的假行——明细未就绪时必须走占位分支。
   */
  detailState?: "pending" | "ready" | "error";
  /** 关闭选中态（宽容器 in-flow 右栏需要显式返回驾驶舱面板；Sheet 模式有自带关闭钮可不传）。 */
  onClose?: () => void;
  principalNamesById?: ReadonlyMap<string, string>;
  project: Project;
  summary?: ProjectRiskSummary;
}) {
  const resolvedSummary = summary ?? emptyProjectRiskSummary(project);
  // countBuckets 只在列表态（run-summary 计数）路径存在，是「明细尚未接管」的判别器。
  const isCountsOnly = Boolean(resolvedSummary.countBuckets);
  const reasons = isCountsOnly ? [] : resolvedSummary.reasons;
  const actionableReasons = reasons.filter((reason) => isActionableRiskReason(reason.type));
  const signalReasons = reasons.filter((reason) => !isActionableRiskReason(reason.type));
  const attention = formatAttentionHeadline(resolvedSummary);
  const breakdown = buildAttentionBreakdown(resolvedSummary);
  const ownerLabel = resolveProjectOwnerLabel(
    project,
    resolvedSummary.owner,
    principalNamesById,
  );
  // 与队列「关注摘要」重复的明细默认收起，避免同屏双读。
  const [signalsOpen, setSignalsOpen] = useState(false);
  const [actionableOpen, setActionableOpen] = useState(true);

  return (
    <aside
      aria-label="选中项目上下文"
      className="flex min-w-0 flex-col gap-3 rounded-[14px] border border-line bg-card p-4 shadow-sm @5xl/master-detail:sticky @5xl/master-detail:top-4 @5xl/master-detail:max-h-[calc(100svh-2rem)] @5xl/master-detail:overflow-y-auto"
      data-testid="project-selected-context-panel"
    >
      <div className="flex min-w-0 items-start gap-2.5">
        <IconTile tone={projectStatusTone(project.status)} size="sm">
          <FolderKanban />
        </IconTile>
        {onClose ? (
          <IconButton
            aria-label="关闭项目待办详情"
            // Sheet 模式有自带关闭钮且位置重叠，此钮仅 in-flow（master-detail 容器内）显示
            className="order-last hidden size-7 shrink-0 @5xl/master-detail:inline-grid"
            onClick={onClose}
            type="button"
          >
            <X aria-hidden className="size-3.5" />
          </IconButton>
        ) : null}
        <div className="min-w-0 flex-1">
          <h3 className="min-w-0 break-words text-sm font-extrabold leading-5 text-ink">
            {project.name}
          </h3>
          {project.goal ? (
            <p className="mt-0.5 line-clamp-2 text-[11.5px] leading-[1.45] text-ink-3">
              {project.goal}
            </p>
          ) : null}
          <p className="mt-1 flex min-w-0 items-center gap-1 truncate text-[12px] text-ink-3">
            <UserRound aria-hidden className="size-3 shrink-0" />
            负责人 <span className="truncate font-medium text-ink-2">{ownerLabel}</span>
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

      <div className="min-w-0 rounded-[10px] bg-card-soft/60 p-3" data-testid="project-triage-attention">
        <div className="flex min-w-0 items-start justify-between gap-2">
          <button
            className="text-left text-[11px] font-semibold uppercase tracking-wide text-ink-3 hover:text-ink"
            onClick={() => setActionableOpen((v) => !v)}
            type="button"
          >
            可行动
            {breakdown.actionableTotal > 0 ? ` · ${breakdown.actionableTotal}` : ""}
            <span className="ml-1 font-normal normal-case text-ink-4">
              {actionableOpen ? "收起" : "展开"}
            </span>
          </button>
          {attention.primary ? (
            <span className="min-w-0 max-w-[55%] text-right text-[11px] leading-4 text-ink-3">
              {attention.primary}
            </span>
          ) : null}
        </div>
        {isCountsOnly ? (
          <p
            className={cn(
              "mt-1.5 text-[12px] leading-5",
              detailState === "error" ? "text-warn-text" : "text-ink-3",
            )}
            data-testid="project-triage-detail-placeholder"
          >
            {detailState === "error" ? (
              <>
                明细加载失败，上方仅为计数。{" "}
                <Link
                  className="text-brand underline underline-offset-2 hover:opacity-75"
                  params={{ projectId: project.id }}
                  to="/projects/$projectId"
                >
                  进入项目
                </Link>{" "}
                查看具体条目。
              </>
            ) : (
              "正在加载明细…"
            )}
          </p>
        ) : actionableOpen ? (
          actionableReasons.length === 0 ? (
            <p className="mt-1.5 text-[12px] leading-5 text-ink-2">
              当前没有可下钻的决策/等人/失败/协调项。{" "}
              <Link
                className="text-brand underline underline-offset-2 hover:opacity-75"
                params={{ projectId: project.id }}
                to="/projects/$projectId"
              >
                进入项目
              </Link>
              {signalReasons.length > 0
                ? "；其它信号见下方折叠区。"
                : " 可查看任务与验收进度。"}
            </p>
          ) : (
            <ul className="mt-2 flex flex-col gap-2">
              {actionableReasons.map((reason) => (
                <ProjectTriageReasonRow
                  key={reason.id}
                  projectId={project.id}
                  reason={reason}
                />
              ))}
            </ul>
          )
        ) : null}
        {signalReasons.length > 0 ? (
          <div className="mt-3 border-t border-line/70 pt-2.5">
            <button
              className="text-left text-[11px] font-semibold text-ink-3 hover:text-ink"
              onClick={() => setSignalsOpen((v) => !v)}
              type="button"
            >
              其它信号 · {signalReasons.length}
              <span className="ml-1 font-normal text-ink-4">
                {signalsOpen ? "收起" : "展开明细"}
              </span>
            </button>
            {signalsOpen ? (
              <>
                <p className="mt-1 text-[11px] leading-4 text-ink-3">
                  证据核验与等待超时不计入可行动项；进入资产/概览查看。
                </p>
                <ul className="mt-2 flex flex-col gap-2">
                  {signalReasons.map((reason) => (
                    <ProjectTriageReasonRow
                      key={reason.id}
                      projectId={project.id}
                      reason={reason}
                    />
                  ))}
                </ul>
              </>
            ) : null}
          </div>
        ) : null}
      </div>

      <p className="text-[11px] leading-4 text-ink-3">
        完整审批与决策处理请在收件箱或项目内完成；本页仅导流。
      </p>
      <div className="flex flex-col gap-2">
        <Button asChild className="w-full justify-center">
          <Link params={{ projectId: project.id }} to="/projects/$projectId">
            进入项目
            <ArrowRight data-icon="inline-end" />
          </Link>
        </Button>
        <Button asChild className="w-full justify-center" variant="ghost">
          <Link to="/inbox">我的待办</Link>
        </Button>
      </div>

      </aside>
    );
  }

function ProjectTriageReasonRow({
  projectId,
  reason
}: {
  projectId: string;
  reason: ProjectRiskReason;
}) {
  const meta = REASON_META[reason.type];
  const Icon = meta.icon;
  const focus =
    reason.type === "human_decision" && reason.source === "decisions"
      ? reason.sourceId
      : reason.type === "waiting_human" || reason.type === "execution_failed"
        ? reason.sourceId
        : undefined;

  return (
    <li className="flex min-w-0 items-start gap-2 rounded-[8px] bg-card px-2.5 py-2">
      <IconTile tone={projectRiskLevelTone(reason.level)} size="sm">
        <Icon />
      </IconTile>
      <div className="min-w-0 flex-1">
        <p className="text-[10.5px] font-bold text-ink-3">
          {projectRiskReasonLabel(reason.type)}
        </p>
        <p
          className="min-w-0 line-clamp-2 break-words text-[12px] font-semibold leading-5 text-ink"
          title={reason.title}
        >
          {reason.title}
        </p>
        {reason.detail ? (
          <p className="mt-0.5 min-w-0 truncate font-mono text-[11px] text-ink-3">
            {reason.detail}
          </p>
        ) : null}
      </div>
      <Button asChild className="shrink-0" size="sm" variant="outline">
        <Link
          params={{ projectId }}
          search={
            reason.type === "waiting_human" || reason.type === "execution_failed"
              ? reason.sourceId
                ? { tab: meta.tab, task: reason.sourceId }
                : { tab: meta.tab }
              : focus
                ? { focus, tab: meta.tab }
                : { tab: meta.tab }
          }
          to="/projects/$projectId"
        >
          {meta.action}
        </Link>
      </Button>
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

function projectStatusTone(status: ProjectStatus | string): Tone {
  if (status === "running") return "ok";
  if (status === "archived") return "mute";
  if (status === "acceptance") return "brand";
  if (status === "paused") return "mute";
  if (status === "configuring" || status === "draft") return "info";
  return "mute";
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
    month: "2-digit"
}).format(date);
}


/**
 * 未选中时的右栏：项目组合透视（方案 C）。
 * 状态分布 / 需关注 / 协调异常名单 / 今日完成 / 长期无活动。
 * 数据来自已加载列表 + run-summary，零额外请求。
 */
export function ProjectPortfolioPerspectivePanel({
  completedTodayCount,
  projects,
  riskSummaries,
  runSummaryItems,
  onSelectProject,
}: {
  /** 租户级今日完成运行数（run-summary.today_completed_run_count）。 */
  completedTodayCount?: number;
  projects: Project[];
  riskSummaries: ProjectRiskSummaryMap;
  /** 可选：run-summary items，用于今日完成按项目展示（有则优先）。 */
  runSummaryItems?: Array<{
    project_id: string;
    completed_today_count: number;
    name?: string;
  }>;
  onSelectProject: (projectId: string) => void;
}) {
  const portfolio = buildProjectPortfolioCounts(projects);
  // 按风险优先级取前 6，避免 created_at 序把新低危挤掉旧高危。
  const needAttention = sortProjectsByRisk(projects, riskSummaries)
    .map((p) => ({
      project: p,
      summary: riskSummaries[p.id] ?? emptyProjectRiskSummary(p),
    }))
    .filter(
      ({ summary, project }) =>
        project.status !== "archived" &&
        (summary.level === "danger" || summary.level === "warn"),
    )
    .slice(0, 6);

  const coordinationAnomalies = sortProjectsByRisk(projects, riskSummaries)
    .filter((p) => {
      if (p.status === "archived") return false;
      const summary = riskSummaries[p.id] ?? emptyProjectRiskSummary(p);
      const b = buildAttentionBreakdown(summary);
      return b.coordination > 0;
    })
    .slice(0, 6);

  const stale = projects
    .filter((p) => p.status !== "archived")
    .map((p) => {
      const summary = riskSummaries[p.id] ?? emptyProjectRiskSummary(p);
      const at = summary.lastActivityAt ?? p.updated_at;
      return { project: p, at };
    })
    .filter((item) => {
      if (!item.at) return true;
      const ms = Date.parse(item.at);
      if (Number.isNaN(ms)) return true;
      return Date.now() - ms > 7 * 24 * 60 * 60 * 1000;
    })
    .sort((a, b) => {
      const aMs = a.at ? Date.parse(a.at) : 0;
      const bMs = b.at ? Date.parse(b.at) : 0;
      const aOk = !Number.isNaN(aMs) ? aMs : 0;
      const bOk = !Number.isNaN(bMs) ? bMs : 0;
      return aOk - bOk; // 最久无活动优先
    })
    .slice(0, 5);

  const todayByProject = (runSummaryItems ?? [])
    .filter((item) => (item.completed_today_count ?? 0) > 0)
    .sort((a, b) => b.completed_today_count - a.completed_today_count)
    .slice(0, 5);
  const todayTotal =
    completedTodayCount ??
    todayByProject.reduce((acc, item) => acc + item.completed_today_count, 0);

  return (
    <aside
      aria-label="项目组合透视"
      className="flex min-w-0 flex-col gap-3 rounded-[14px] border border-line bg-card p-4 shadow-sm @5xl/master-detail:sticky @5xl/master-detail:top-4 @5xl/master-detail:max-h-[calc(100svh-2rem)] @5xl/master-detail:overflow-y-auto"
      data-testid="projects-portfolio-perspective"
    >
      <div>
        <h3 className="text-sm font-extrabold text-ink">组合透视</h3>
        <p className="mt-0.5 text-[11.5px] leading-4 text-ink-3">
          状态分布与需关注项目（组合视角，非收件箱）
        </p>
      </div>

      <div className="rounded-[10px] bg-card-soft/60 p-3">
        <div className="text-[11px] font-semibold text-ink-3">状态分布</div>
        <div className="mt-2 flex flex-col gap-1.5 text-[12px] text-ink-2">
          <div className="flex justify-between gap-2">
            <span>{projectStatusLabel("running")}</span>
            <span className="font-bold tabular-nums">{portfolio.running}</span>
          </div>
          <div className="flex justify-between gap-2">
            <span>验收中</span>
            <span className="font-bold tabular-nums">{portfolio.acceptance}</span>
          </div>
          <div className="flex justify-between gap-2">
            <span>协调异常</span>
            <span
              className={cn(
                "font-bold tabular-nums",
                portfolio.coordinationAnomaly > 0 && "text-danger-text",
              )}
            >
              {portfolio.coordinationAnomaly}
            </span>
          </div>
          <div className="flex justify-between gap-2">
            <span>已归档</span>
            <span className="font-bold tabular-nums text-ink-3">{portfolio.archived}</span>
          </div>
        </div>
      </div>

      <div className="rounded-[10px] bg-card-soft/60 p-3">
        <div className="text-[11px] font-semibold text-ink-3">
          需要关注 · {needAttention.length}
        </div>
        {needAttention.length === 0 ? (
          <p className="mt-1.5 text-[12px] text-ink-3">当前列表没有高关注信号项目。</p>
        ) : (
          <ul className="mt-2 flex flex-col gap-1.5">
            {needAttention.map(({ project, summary }) => {
              const headline = formatAttentionHeadline(summary);
              return (
                <li key={project.id}>
                  <button
                    className="w-full rounded-[8px] bg-card px-2.5 py-2 text-left hover:bg-brand-soft/40"
                    onClick={() => onSelectProject(project.id)}
                    type="button"
                  >
                    <div className="truncate text-[12px] font-semibold text-ink">
                      {project.name}
                    </div>
                    {headline.primary ? (
                      <div className="mt-0.5 truncate text-[11px] text-ink-3">
                        {headline.primary}
                      </div>
                    ) : null}
                  </button>
                </li>
              );
            })}
          </ul>
        )}
      </div>

      <div className="rounded-[10px] bg-card-soft/60 p-3">
        <div className="text-[11px] font-semibold text-ink-3">
          协调异常 · {coordinationAnomalies.length}
        </div>
        {coordinationAnomalies.length === 0 ? (
          <p className="mt-1.5 text-[12px] text-ink-3">无协调异常项目。</p>
        ) : (
          <ul className="mt-2 flex flex-col gap-1 text-[12px] text-ink-2">
            {coordinationAnomalies.map((project) => (
              <li key={project.id}>
                <button
                  className="w-full truncate text-left font-medium text-danger-text hover:opacity-80"
                  onClick={() => onSelectProject(project.id)}
                  type="button"
                >
                  {project.name}
                </button>
              </li>
            ))}
          </ul>
        )}
      </div>

      <div className="rounded-[10px] bg-card-soft/60 p-3">
        <div className="flex items-center justify-between gap-2">
          <div className="text-[11px] font-semibold text-ink-3">今日完成运行</div>
          <span className="font-extrabold tabular-nums text-ink">{todayTotal}</span>
        </div>
        {todayByProject.length === 0 ? (
          <p className="mt-1.5 text-[12px] text-ink-3">今日暂无项目侧完成运行。</p>
        ) : (
          <ul className="mt-2 flex flex-col gap-1 text-[12px] text-ink-2">
            {todayByProject.map((item) => {
              const name =
                item.name ||
                projects.find((p) => p.id === item.project_id)?.name ||
                item.project_id;
              return (
                <li key={item.project_id} className="flex justify-between gap-2">
                  <button
                    className="min-w-0 truncate text-left font-medium hover:text-brand"
                    onClick={() => onSelectProject(item.project_id)}
                    type="button"
                  >
                    {name}
                  </button>
                  <span className="shrink-0 tabular-nums font-semibold">
                    {item.completed_today_count}
                  </span>
                </li>
              );
            })}
          </ul>
        )}
      </div>

      <div className="rounded-[10px] bg-card-soft/60 p-3">
        <div className="text-[11px] font-semibold text-ink-3">长期无活动（约 7 日+）</div>
        {stale.length === 0 ? (
          <p className="mt-1.5 text-[12px] text-ink-3">暂无长期静默项目。</p>
        ) : (
          <ul className="mt-2 flex flex-col gap-1 text-[12px] text-ink-2">
            {stale.map(({ project }) => (
              <li key={project.id}>
                <button
                  className="w-full truncate text-left font-medium hover:text-brand"
                  onClick={() => onSelectProject(project.id)}
                  type="button"
                >
                  {project.name}
                </button>
              </li>
            ))}
          </ul>
        )}
      </div>
    </aside>
  );
}
