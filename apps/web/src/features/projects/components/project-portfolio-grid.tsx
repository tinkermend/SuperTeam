import { useEffect, useState, type KeyboardEvent } from "react";
import { Link } from "@tanstack/react-router";
import { cn } from "@/lib/utils";
import { ArrowRight, FolderKanban, LayoutGrid, List, UserRound } from "lucide-react";
import {
  Button,
  Chip,
  EmptyState,
  Pagination,
  SoftCard,
  StatusPill,
  ToolbarSearch,
  WorkSurface,
  type Tone,
} from "@/components/superteam";
import type {
  ProjectPortfolioItem,
  ProjectPortfolioResponse,
  ProjectPortfolioSort,
  ProjectStatus,
  ProjectTaskPortfolioCounts,
} from "@/lib/api/projects";
import {
  projectStatusLabel,
  projectTaskPortfolioBucketLabel,
  type ProjectTaskPortfolioBucketKey,
} from "@/lib/status-labels";
import { projectPhaseDotClass } from "../project-lifecycle-display";
import {
  projectRiskLevelLabel,
  projectRiskLevelTone,
  type ProjectRiskFilter,
  type ProjectRiskSummary,
  type ProjectRiskSummaryMap,
  PROJECT_RISK_FILTERS,
} from "../project-risk";

/** 默认页大小：3 列 × 3 行 ≈ 1.5 屏（方案 C）。 */
export const PORTFOLIO_DEFAULT_PAGE_SIZE = 9;
export const PORTFOLIO_PAGE_SIZE_OPTIONS = [9, 12, 20, 50] as const;

/** 卡片 / 紧凑列表密度；只改变表现，不改变数据口径。 */
export type PortfolioDensity = "cards" | "compact";
const PORTFOLIO_DENSITY_STORAGE_KEY = "superteam.projects.portfolioDensity";

function readPortfolioDensity(): PortfolioDensity {
  try {
    const raw = window.localStorage.getItem(PORTFOLIO_DENSITY_STORAGE_KEY);
    if (raw === "compact" || raw === "cards") return raw;
  } catch {
    // SSR / 隐私模式：忽略
  }
  return "cards";
}

function writePortfolioDensity(density: PortfolioDensity) {
  try {
    window.localStorage.setItem(PORTFOLIO_DENSITY_STORAGE_KEY, density);
  } catch {
    // ignore
  }
}

/** 卡片/顶栏共用：主桶优先展示顺序（完成→执行中→排队…）。 */
export const PORTFOLIO_BAR_ORDER: ProjectTaskPortfolioBucketKey[] = [
  "completed",
  "running",
  "queued",
  "pending",
  "waiting_human",
  "blocked",
  "failed",
  "cancelled",
  "other",
];

/** 实心色块（少透明，对齐原型饱和度）。 */
export const PORTFOLIO_BAR_TONE: Record<ProjectTaskPortfolioBucketKey, string> = {
  completed: "bg-ok",
  running: "bg-brand",
  queued: "bg-ink-4",
  pending: "bg-ink-4/70",
  waiting_human: "bg-warn",
  blocked: "bg-[#a855f7]", // 阻断与失败可辨
  failed: "bg-danger",
  cancelled: "bg-ink-4/40",
  other: "bg-ink-3",
};

const PORTFOLIO_DOT_TONE: Record<ProjectTaskPortfolioBucketKey, string> = {
  completed: "bg-ok",
  running: "bg-brand",
  queued: "bg-ink-4",
  pending: "bg-ink-4/70",
  waiting_human: "bg-warn",
  blocked: "bg-[#a855f7]",
  failed: "bg-danger",
  cancelled: "bg-ink-4/40",
  other: "bg-ink-3",
};

/** 顶栏任务层默认展示的主图例（与原型五色叙事接近，仍保留阻断）。 */
const SUMMARY_LEGEND_KEYS: ProjectTaskPortfolioBucketKey[] = [
  "completed",
  "running",
  "queued",
  "waiting_human",
  "blocked",
  "failed",
];

const PROJECT_STATUS_FILTER_OPTIONS: Array<{
  value: "all" | ProjectStatus;
  label: string;
}> = [
  { value: "all", label: "全部状态" },
  { value: "running", label: projectStatusLabel("running") },
  { value: "acceptance", label: projectStatusLabel("acceptance") },
  { value: "paused", label: projectStatusLabel("paused") },
  { value: "configuring", label: projectStatusLabel("configuring") },
  { value: "archived", label: projectStatusLabel("archived") },
];

/** 任务态一线 chip（映射 task_state）。 */
const TASK_STATE_CHIPS: Array<{
  value: "" | ProjectTaskPortfolioBucketKey;
  label: string;
}> = [
  { value: "", label: "全部" },
  { value: "running", label: "含运行" },
  { value: "waiting_human", label: "含等待" },
  { value: "blocked", label: "含阻断" },
  { value: "failed", label: "含失败" },
];

function initials(name: string): string {
  const t = name.trim();
  if (!t) return "?";
  // 中文取末字，英文取首字母
  if (/[\u4e00-\u9fff]/.test(t)) return t.slice(-1);
  return t.slice(0, 1).toUpperCase();
}

export function TaskCompositionBar({
  counts,
  className,
  size = "card",
  showTitle = true,
  legendMode = "nonzero",
}: {
  counts: ProjectTaskPortfolioCounts;
  className?: string;
  /** card: 卡片细条；summary: 顶栏粗条 */
  size?: "card" | "summary";
  showTitle?: boolean;
  /** nonzero: 仅非零；primary: 固定主图例（含 0） */
  legendMode?: "nonzero" | "primary";
}) {
  if (counts.total <= 0) {
    return (
      <p
        className={cn("text-[11.5px] text-ink-3", className)}
        data-testid="task-composition-empty"
      >
        尚无项目任务
      </p>
    );
  }

  const segments = PORTFOLIO_BAR_ORDER.filter((k) => (counts[k] ?? 0) > 0);
  const legendKeys =
    legendMode === "primary"
      ? SUMMARY_LEGEND_KEYS
      : segments.filter((k) => k !== "other" || (counts.other ?? 0) > 0);

  const barH = size === "summary" ? "h-4" : "h-3.5";
  const numThreshold = size === "summary" ? 6 : 8; // 段宽百分比阈值

  return (
    <div className={cn("min-w-0", className)} data-testid="task-composition-bar">
      {showTitle ? (
        <div className="mb-1.5 flex min-w-0 items-center justify-between gap-2">
          <span className="text-[11px] font-semibold text-ink-3">
            {size === "summary" ? "任务总数" : "任务构成"}
            <span className="ml-1 font-extrabold tabular-nums text-ink">
              {counts.total}
            </span>
          </span>
          {counts.other > 0 ? (
            <span className="text-[11px] text-warn-text">
              {projectTaskPortfolioBucketLabel("other")} {counts.other}
            </span>
          ) : null}
        </div>
      ) : null}
      <div
        aria-label="任务状态构成"
        className={cn(
          "flex w-full overflow-hidden rounded-full bg-card-soft",
          barH,
        )}
        role="img"
      >
        {segments.map((key) => {
          const n = counts[key] ?? 0;
          const pct = Math.max((n / counts.total) * 100, n > 0 ? 1.5 : 0);
          const showNum = pct >= numThreshold && n > 0;
          return (
            <span
              key={key}
              className={cn(
                "relative flex h-full min-w-[2px] items-center justify-center transition-[width] duration-180 motion-reduce:transition-none",
                PORTFOLIO_BAR_TONE[key],
              )}
              style={{ width: `${pct}%` }}
              title={`${projectTaskPortfolioBucketLabel(key)} ${n}`}
            >
              {showNum ? (
                <span className="px-0.5 text-[10px] font-extrabold leading-none tabular-nums text-white drop-shadow-sm">
                  {n}
                </span>
              ) : null}
            </span>
          );
        })}
      </div>
      <div className="mt-1.5 flex flex-wrap gap-x-2.5 gap-y-0.5 text-[10.5px] text-ink-3">
        {legendKeys.map((key) => (
          <span key={key} className="inline-flex items-center gap-1 tabular-nums">
            <span
              aria-hidden
              className={cn(
                "inline-block size-1.5 shrink-0 rounded-full",
                PORTFOLIO_DOT_TONE[key],
              )}
            />
            {projectTaskPortfolioBucketLabel(key)}{" "}
            <span className="font-semibold text-ink-2">{counts[key] ?? 0}</span>
          </span>
        ))}
      </div>
    </div>
  );
}

export type ProjectPortfolioToolbarFilters = {
  q: string;
  status: "all" | ProjectStatus;
  risk: ProjectRiskFilter;
  taskState: "" | ProjectTaskPortfolioBucketKey;
  sort: ProjectPortfolioSort;
  mineOnly: boolean;
};

export function ProjectPortfolioToolbar({
  createAction,
  density = "cards",
  filters,
  isFetching,
  loadedRiskCounts,
  onDensityChange,
  onFiltersChange,
  totalLabel,
  totalProjects,
}: {
  createAction?: React.ReactNode;
  density?: PortfolioDensity;
  filters: ProjectPortfolioToolbarFilters;
  isFetching: boolean;
  loadedRiskCounts?: { blocked: number };
  onDensityChange?: (density: PortfolioDensity) => void;
  onFiltersChange: (next: ProjectPortfolioToolbarFilters) => void;
  totalLabel: string;
  totalProjects: number;
}) {
  const [showMoreFilters, setShowMoreFilters] = useState(false);

  return (
    <div
      className="flex min-w-0 flex-col gap-2.5 border-b border-line bg-card px-3.5 py-3"
      data-testid="project-portfolio-toolbar"
    >
      <div className="flex min-w-0 flex-wrap items-center justify-between gap-2">
        <div className="flex min-w-0 flex-wrap items-center gap-2">
          <h2 className="text-[15px] font-extrabold text-ink">项目组合</h2>
          <StatusPill tone={isFetching ? "info" : "mute"}>
            {isFetching ? "加载中" : `${totalLabel} ${totalProjects} 个`}
          </StatusPill>
          {loadedRiskCounts && loadedRiskCounts.blocked > 0 ? (
            <StatusPill tone="danger">
              {loadedRiskCounts.blocked} 个需关注
            </StatusPill>
          ) : null}
        </div>
        <div className="flex shrink-0 flex-wrap items-center gap-2">
          {onDensityChange ? (
            <div
              aria-label="显示密度"
              className="inline-flex items-center gap-0.5 rounded-[8px] border border-line bg-card-soft p-0.5"
              data-testid="portfolio-density-switch"
              role="group"
            >
              <Chip
                active={density === "cards"}
                aria-label="卡片视图"
                className="rounded-[6px] px-2 py-1 text-[11px]"
                onClick={() => onDensityChange("cards")}
                type="button"
              >
                <LayoutGrid aria-hidden className="size-3.5" />
                <span className="ml-1">卡片</span>
              </Chip>
              <Chip
                active={density === "compact"}
                aria-label="紧凑列表"
                className="rounded-[6px] px-2 py-1 text-[11px]"
                onClick={() => onDensityChange("compact")}
                type="button"
              >
                <List aria-hidden className="size-3.5" />
                <span className="ml-1">列表</span>
              </Chip>
            </div>
          ) : null}
          {createAction ? <div className="shrink-0">{createAction}</div> : null}
        </div>
      </div>

      {/* 一线：搜索 + 状态下拉 + 仅我 + 任务态 chip */}
      <div className="flex min-w-0 flex-col gap-2 lg:flex-row lg:items-center">
        <ToolbarSearch
          aria-label="搜索项目"
          className="min-w-0 lg:max-w-xs"
          onChange={(event) =>
            onFiltersChange({ ...filters, q: event.target.value })
          }
          placeholder="搜索项目名称或目标"
          value={filters.q}
        />
        <select
          aria-label="项目状态筛选"
          className="h-8 shrink-0 rounded-[8px] border border-line bg-card px-2.5 text-[12px] text-ink outline-none transition-colors hover:bg-card-soft focus:border-brand focus:ring-2 focus:ring-brand/25"
          onChange={(event) =>
            onFiltersChange({
              ...filters,
              status: event.target
                .value as ProjectPortfolioToolbarFilters["status"],
            })
          }
          value={filters.status}
        >
          {PROJECT_STATUS_FILTER_OPTIONS.map((option) => (
            <option key={option.value} value={option.value}>
              {option.label}
            </option>
          ))}
        </select>
        <Chip
          active={filters.mineOnly}
          className="rounded-[8px] px-2.5 py-1.5 text-[12px]"
          onClick={() =>
            onFiltersChange({ ...filters, mineOnly: !filters.mineOnly })
          }
          type="button"
        >
          仅我参与
        </Chip>
        <div className="flex min-w-0 flex-wrap items-center gap-1.5 lg:ml-1">
          {TASK_STATE_CHIPS.map((chip) => (
            <Chip
              active={filters.taskState === chip.value}
              className="rounded-[8px] px-2.5 py-1.5 text-[12px]"
              key={chip.value || "task-all"}
              onClick={() =>
                onFiltersChange({ ...filters, taskState: chip.value })
              }
              type="button"
            >
              {chip.label}
            </Chip>
          ))}
        </div>
        <button
          className="rounded-[8px] px-2 py-1.5 text-[12px] font-semibold text-brand hover:opacity-75 lg:ml-auto"
          onClick={() => setShowMoreFilters((v) => !v)}
          type="button"
        >
          {showMoreFilters ? "收起" : "更多"}
        </button>
      </div>

      {showMoreFilters ? (
        <div className="flex min-w-0 flex-wrap items-center gap-1.5 border-t border-line/60 pt-2">
          {PROJECT_RISK_FILTERS.filter(
            (filter) =>
              filter.listAvailable !== false &&
              (filter.defaultVisible !== false ||
                filters.risk === filter.value),
          ).map((filter) => (
            <Chip
              active={filters.risk === filter.value}
              className="rounded-[8px] px-2.5 py-1.5 text-[12px]"
              key={filter.value}
              onClick={() =>
                onFiltersChange({ ...filters, risk: filter.value })
              }
              type="button"
            >
              {filter.label}
            </Chip>
          ))}
          {(
            [
              ["attention", "关注优先"],
              ["recent", "最近活动"],
              ["created", "创建时间"],
            ] as const
          ).map(([value, label]) => (
            <Chip
              active={filters.sort === value}
              className="rounded-[8px] px-2.5 py-1.5 text-[12px]"
              key={value}
              onClick={() => onFiltersChange({ ...filters, sort: value })}
              type="button"
            >
              {label}
            </Chip>
          ))}
        </div>
      ) : null}
    </div>
  );
}

function healthDisplay(
  projectStatus: string,
  riskSummary?: ProjectRiskSummary,
): { label: string; tone: Tone } {
  if (projectStatus === "archived") {
    return { label: "中性", tone: "mute" };
  }
  if (!riskSummary) {
    return { label: "正常", tone: "ok" };
  }
  if (riskSummary.state === "pending") {
    return { label: "识别中", tone: "info" };
  }
  if (riskSummary.state === "error") {
    return { label: "风险待确认", tone: "warn" };
  }
  const level = riskSummary.level;
  if (level === "danger" || level === "warn") {
    return { label: "需关注", tone: projectRiskLevelTone(level) };
  }
  // 细粒度文案：暂无阻塞 → 正常（原型用语）
  const raw = projectRiskLevelLabel(riskSummary);
  if (raw === "暂无阻塞" || raw === "正常") {
    return { label: "正常", tone: "ok" };
  }
  return { label: raw, tone: projectRiskLevelTone(level) };
}

function portfolioOwnerName(
  item: ProjectPortfolioItem,
  principalNamesById?: ReadonlyMap<string, string>,
): string {
  const project = item.project;
  return (
    item.owner?.display_name?.trim() ||
    (project.human_owner_user_id
      ? principalNamesById?.get(project.human_owner_user_id)
      : undefined) ||
    "未指定负责人"
  );
}

function portfolioActivityLabel(item: ProjectPortfolioItem): string | undefined {
  if (!item.last_activity_at) return undefined;
  return new Date(item.last_activity_at).toLocaleString("zh-CN", {
    month: "numeric",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  });
}

/** 紧凑列表行：名称+阶段 pill 同行、构成条单行、健康度右对齐。与卡片态同源数据。 */
export function ProjectPortfolioCompactRow({
  countsDegraded,
  item,
  onSelect,
  principalNamesById,
  riskSummary,
  selected,
}: {
  countsDegraded?: boolean;
  item: ProjectPortfolioItem;
  onSelect: (projectId: string) => void;
  principalNamesById?: ReadonlyMap<string, string>;
  riskSummary?: ProjectRiskSummary;
  selected?: boolean;
}) {
  const project = item.project;
  const ownerName = portfolioOwnerName(item, principalNamesById);
  const health = healthDisplay(project.status, riskSummary);
  const activity = portfolioActivityLabel(item);

  const onKeyDown = (event: KeyboardEvent<HTMLDivElement>) => {
    if (event.key === "Enter" || event.key === " ") {
      event.preventDefault();
      onSelect(project.id);
    }
  };

  return (
    <div
      className={cn(
        "flex min-w-0 cursor-pointer items-center gap-3 border-b border-line/70 px-3.5 py-2.5 transition-colors hover:bg-card-soft/70",
        selected && "bg-brand-soft/40",
      )}
      data-project-id={project.id}
      data-testid="project-portfolio-compact-row"
      onClick={() => onSelect(project.id)}
      onKeyDown={onKeyDown}
      role="button"
      tabIndex={0}
    >
      <div className="flex size-8 shrink-0 items-center justify-center rounded-[9px] bg-card-soft text-ink-3">
        <FolderKanban className="size-3.5" />
      </div>
      <div className="min-w-0 flex-1">
        <div className="flex min-w-0 flex-wrap items-center gap-2">
          <h3 className="min-w-0 truncate text-[13px] font-extrabold leading-5 text-ink">
            {project.name}
          </h3>
          <StatusPill
            className="shrink-0"
            dotClassName={projectPhaseDotClass(project.status)}
            tone="mute"
          >
            {projectStatusLabel(project.status)}
          </StatusPill>
        </div>
        <div className="mt-1 flex min-w-0 items-center gap-2">
          <span className="min-w-0 max-w-[8rem] truncate text-[11px] text-ink-3">
            {ownerName}
          </span>
          <TaskCompositionBar
            className="min-w-0 flex-1"
            counts={item.task_counts}
            showTitle={false}
            size="card"
          />
        </div>
      </div>
      <div className="flex shrink-0 flex-col items-end gap-1">
        <StatusPill tone={health.tone}>{health.label}</StatusPill>
        {countsDegraded ? (
          <span className="text-[10px] text-ink-3">计数可能不准</span>
        ) : null}
        <p className="text-[10.5px] text-ink-4">
          {activity ? `最近 ${activity}` : "暂无活动"}
        </p>
      </div>
      <Button asChild className="h-8 shrink-0 px-2.5 text-[12px]" size="sm">
        <Link
          onClick={(e) => e.stopPropagation()}
          params={{ projectId: project.id }}
          to="/projects/$projectId"
        >
          进入
          <ArrowRight className="ml-1 size-3.5" />
        </Link>
      </Button>
    </div>
  );
}

export function ProjectPortfolioCard({
  countsDegraded,
  item,
  onSelect,
  principalNamesById,
  riskSummary,
  selected,
}: {
  countsDegraded?: boolean;
  item: ProjectPortfolioItem;
  onSelect: (projectId: string) => void;
  principalNamesById?: ReadonlyMap<string, string>;
  riskSummary?: ProjectRiskSummary;
  selected?: boolean;
}) {
  const project = item.project;
  const ownerName = portfolioOwnerName(item, principalNamesById);
  const employeeN = item.participants.active_digital_employee_count;
  const health = healthDisplay(project.status, riskSummary);

  const onKeyDown = (event: KeyboardEvent<HTMLDivElement>) => {
    if (event.key === "Enter" || event.key === " ") {
      event.preventDefault();
      onSelect(project.id);
    }
  };

  const activity = portfolioActivityLabel(item);

  return (
    <div data-project-id={project.id} data-testid="project-portfolio-card">
      <SoftCard
        className={cn(
          "flex h-full min-w-0 cursor-pointer flex-col gap-2.5 p-4 transition-shadow hover:shadow-md",
          selected && "ring-2 ring-brand/45 border-brand/30",
        )}
        interactive
        onClick={() => onSelect(project.id)}
        onKeyDown={onKeyDown}
        role="button"
        tabIndex={0}
      >
        {/* 行1：图标 | 标题 | 阶段 pill（中性体 + phase 色点，非紧迫度） */}
        <div className="flex min-w-0 items-start gap-2.5">
          <div className="mt-0.5 flex size-9 shrink-0 items-center justify-center rounded-[11px] bg-card-soft text-ink-3">
            <FolderKanban className="size-4" />
          </div>
          <div className="min-w-0 flex-1">
            <div className="flex min-w-0 items-start justify-between gap-2">
              <h3 className="min-w-0 flex-1 truncate text-[13.5px] font-extrabold leading-5 text-ink">
                {project.name}
              </h3>
              <StatusPill
                className="shrink-0"
                dotClassName={projectPhaseDotClass(project.status)}
                tone="mute"
              >
                {projectStatusLabel(project.status)}
              </StatusPill>
            </div>
            {project.goal ? (
              <p className="mt-0.5 line-clamp-2 text-[12px] leading-[1.45] text-ink-3">
                {project.goal}
              </p>
            ) : null}
          </div>
        </div>

        {/* 行3：负责人 + initials + N 在办 */}
        <div className="flex min-w-0 items-center gap-2 text-[12px] text-ink-3">
          <span
            aria-hidden
            className="inline-flex size-6 shrink-0 items-center justify-center rounded-full bg-brand-soft text-[11px] font-bold text-brand-deep"
          >
            {initials(ownerName)}
          </span>
          <span className="min-w-0 truncate font-medium text-ink-2">
            {ownerName}
          </span>
          {employeeN > 0 ? (
            <span className="inline-flex shrink-0 items-center gap-1 text-ink-4">
              <span
                aria-hidden
                className="inline-flex size-5 items-center justify-center rounded-full bg-card-soft text-[10px] font-bold text-ink-3"
              >
                {employeeN}
              </span>
              名在办
            </span>
          ) : null}
          <UserRound aria-hidden className="ml-auto size-3 shrink-0 opacity-40" />
        </div>

        {/* 行4：任务构成（与 attention 分区，§3.3） */}
        <TaskCompositionBar counts={item.task_counts} />

        {/* 行5：健康度 */}
        <div className="flex flex-wrap items-center gap-1.5">
          <StatusPill tone={health.tone}>{health.label}</StatusPill>
          {countsDegraded ? (
            <span
              className="text-[11px] text-ink-3"
              data-testid="portfolio-counts-degraded"
            >
              计数可能不准
            </span>
          ) : null}
        </div>

        {/* footer：时间 + 主 CTA */}
        <div className="mt-auto flex min-w-0 items-center justify-between gap-2 border-t border-line/60 pt-2.5">
          <p className="min-w-0 truncate text-[11px] text-ink-4">
            {activity ? `最近活动 ${activity}` : "暂无活动"}
          </p>
          <Button asChild className="h-8 shrink-0 px-3 text-[12px]" size="sm">
            <Link
              onClick={(e) => e.stopPropagation()}
              params={{ projectId: project.id }}
              to="/projects/$projectId"
            >
              进入项目
              <ArrowRight className="ml-1 size-3.5" />
            </Link>
          </Button>
        </div>
      </SoftCard>
    </div>
  );
}

export function ProjectPortfolioGrid({
  activePage,
  createAction,
  empty,
  error,
  filters,
  isFetching,
  items,
  loadedRiskCounts,
  onFiltersChange,
  onPageChange,
  onPageSizeChange,
  onSelectProject,
  pageCount,
  pageSize,
  paginationTotal,
  portfolio,
  principalNamesById,
  riskSummaries,
  selectedProjectId,
  totalLabel,
}: {
  activePage: number;
  createAction?: React.ReactNode;
  empty?: boolean;
  error?: boolean;
  filters: ProjectPortfolioToolbarFilters;
  isFetching: boolean;
  items: ProjectPortfolioItem[];
  loadedRiskCounts?: { blocked: number };
  onFiltersChange: (next: ProjectPortfolioToolbarFilters) => void;
  onPageChange: (page: number) => void;
  onPageSizeChange: (size: number) => void;
  onSelectProject: (projectId: string) => void;
  pageCount: number;
  pageSize: number;
  paginationTotal: number;
  portfolio?: ProjectPortfolioResponse;
  principalNamesById?: ReadonlyMap<string, string>;
  riskSummaries: ProjectRiskSummaryMap;
  selectedProjectId?: string;
  totalLabel: string;
}) {
  // 密度只影响表现层：同一份 items/riskSummaries，不另发请求。
  const [density, setDensity] = useState<PortfolioDensity>("cards");
  useEffect(() => {
    setDensity(readPortfolioDensity());
  }, []);
  const handleDensityChange = (next: PortfolioDensity) => {
    setDensity(next);
    writePortfolioDensity(next);
  };

  if (error) {
    return (
      <WorkSurface className="min-w-0 rounded-[14px] p-6 shadow-sm">
        <EmptyState
          description="请稍后重试，或检查网络与权限。"
          title="项目组合加载失败"
        />
      </WorkSurface>
    );
  }

  return (
    <section
      aria-label="项目卡片网格"
      className="min-w-0"
      data-density={density}
      data-testid="project-portfolio-grid"
    >
      <WorkSurface className="min-w-0 rounded-[14px] shadow-sm">
        <ProjectPortfolioToolbar
          createAction={createAction}
          density={density}
          filters={filters}
          isFetching={isFetching}
          loadedRiskCounts={loadedRiskCounts}
          onDensityChange={handleDensityChange}
          onFiltersChange={onFiltersChange}
          totalLabel={totalLabel}
          totalProjects={
            portfolio?.pagination.total ??
            portfolio?.summary.total_projects ??
            paginationTotal
          }
        />

        {empty ? (
          <div className="p-6">
            <EmptyState
              description="创建项目后，这里会显示生命周期与任务构成。"
              title="还没有项目"
            />
          </div>
        ) : isFetching && items.length === 0 ? (
          <div
            className="p-6 text-center text-[13px] text-ink-3"
            data-testid="portfolio-grid-loading"
          >
            正在加载项目组合…
          </div>
        ) : items.length === 0 ? (
          <div className="p-6">
            <EmptyState
              description="试试调整状态、关注筛选或关键词。"
              title="没有符合条件的项目"
            />
          </div>
        ) : density === "compact" ? (
          <div
            className="min-w-0 divide-y-0 border-t border-line/40"
            data-testid="portfolio-compact-list"
          >
            {items.map((item) => (
              <ProjectPortfolioCompactRow
                countsDegraded={portfolio?.counts_degraded}
                item={item}
                key={item.project.id}
                onSelect={onSelectProject}
                principalNamesById={principalNamesById}
                riskSummary={riskSummaries[item.project.id]}
                selected={selectedProjectId === item.project.id}
              />
            ))}
          </div>
        ) : (
          <div className="grid min-w-0 grid-cols-1 gap-3.5 p-3.5 sm:grid-cols-2 xl:grid-cols-3">
            {items.map((item) => (
              <ProjectPortfolioCard
                countsDegraded={portfolio?.counts_degraded}
                item={item}
                key={item.project.id}
                onSelect={onSelectProject}
                principalNamesById={principalNamesById}
                riskSummary={riskSummaries[item.project.id]}
                selected={selectedProjectId === item.project.id}
              />
            ))}
          </div>
        )}

        {!empty && paginationTotal > 0 ? (
          <div className="border-t border-line px-3 py-2">
            <Pagination
              page={activePage}
              onPageChange={onPageChange}
              onPageSizeChange={onPageSizeChange}
              pageCount={pageCount}
              pageSize={pageSize}
              pageSizeOptions={[...PORTFOLIO_PAGE_SIZE_OPTIONS]}
              total={paginationTotal}
            />
          </div>
        ) : null}
      </WorkSurface>
    </section>
  );
}
