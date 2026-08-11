import { useState } from "react";
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
} from "@/components/superteam";
import type {
  Project,
  ProjectStatus,
} from "@/lib/api/projects";
import { projectStatusLabel } from "@/lib/status-labels";
import {
  buildAttentionBreakdown,
  buildProjectPortfolioCounts,
  emptyProjectRiskSummary,
  formatAttentionHeadline,
  isActionableRiskReason,
  projectRiskLevelLabel,
  projectRiskLevelTone,
  projectRiskReasonLabel,
  resolveProjectOwnerLabel,
  sortProjectsByRisk,
  type ProjectPortfolioCounts,
  type ProjectRiskReason,
  type ProjectRiskReasonType,
  type ProjectRiskSummary,
  type ProjectRiskSummaryMap
} from "../project-risk";
import { TaskCompositionBar } from "./project-portfolio-grid";

/** 项目层环图色（与生命周期叙事对齐，CSS conic-gradient）。 */
const PROJECT_LAYER_COLORS: Array<{ key: string; label: string; color: string }> = [
  { key: "running", label: projectStatusLabel("running"), color: "var(--brand)" },
  { key: "acceptance", label: "验收中", color: "var(--info)" },
  { key: "paused", label: projectStatusLabel("paused"), color: "var(--ink-4)" },
  { key: "configuring", label: projectStatusLabel("configuring"), color: "var(--warn)" },
  { key: "draft", label: projectStatusLabel("draft"), color: "#94a3b8" },
  { key: "archived", label: "已归档", color: "#cbd5e1" },
];

/**
 * 项目组合双层摘要：左项目层环图 + 右任务层构成条（portfolio summary 数据）。
 * 兼容旧 `portfolioCounts` 入参。
 */
export function ProjectPortfolioSummaryBar({
  activeTaskCounts,
  mineOnly = false,
  portfolioCounts,
  projectStatusCounts,
  totalLabel,
  totalProjects,
}: {
  /** 非归档项目任务互斥桶（portfolio summary.active_project_task_counts）。 */
  activeTaskCounts?: {
    total: number;
    pending: number;
    queued: number;
    running: number;
    waiting_human: number;
    blocked: number;
    failed: number;
    completed: number;
    cancelled: number;
    other: number;
  };
  mineOnly?: boolean;
  /** 旧路径：从已加载项目列表推导。 */
  portfolioCounts?: ProjectPortfolioCounts;
  projectStatusCounts?: Record<string, number>;
  totalLabel?: string;
  totalProjects?: number;
}) {
  const projectTotal =
    totalProjects ?? portfolioCounts?.total ?? 0;
  const projectLabel =
    totalLabel ?? (mineOnly ? "我参与的项目" : "全部项目");
  const status = projectStatusCounts ?? {
    running: portfolioCounts?.running ?? 0,
    acceptance: portfolioCounts?.acceptance ?? 0,
    paused: portfolioCounts?.paused ?? 0,
    configuring: portfolioCounts?.configuring ?? 0,
    archived: portfolioCounts?.archived ?? 0,
    draft: 0,
  };

  const layerRows = PROJECT_LAYER_COLORS.map((row) => ({
    ...row,
    value: status[row.key] ?? 0,
  })).filter((row) => row.value > 0 || ["running", "acceptance", "archived"].includes(row.key));

  // conic-gradient segments
  let acc = 0;
  const stops: string[] = [];
  if (projectTotal > 0) {
    for (const row of layerRows) {
      if (row.value <= 0) continue;
      const start = (acc / projectTotal) * 360;
      acc += row.value;
      const end = (acc / projectTotal) * 360;
      stops.push(`${row.color} ${start}deg ${end}deg`);
    }
  }
  const donutBg =
    stops.length > 0
      ? `conic-gradient(${stops.join(", ")})`
      : "conic-gradient(var(--line) 0deg 360deg)";

  const taskCounts = activeTaskCounts
    ? {
        total: activeTaskCounts.total,
        pending: activeTaskCounts.pending,
        queued: activeTaskCounts.queued,
        running: activeTaskCounts.running,
        waiting_human: activeTaskCounts.waiting_human,
        blocked: activeTaskCounts.blocked,
        failed: activeTaskCounts.failed,
        completed: activeTaskCounts.completed,
        cancelled: activeTaskCounts.cancelled,
        other: activeTaskCounts.other,
      }
    : null;

  return (
    <div
      aria-label="项目组合概览"
      className="grid min-w-0 gap-3 lg:grid-cols-2"
      data-testid="project-portfolio-summary-bar"
    >
      {/* 项目层 */}
      <div className="flex min-w-0 items-center gap-4 rounded-[14px] border border-line bg-card px-4 py-3.5 shadow-sm">
        <div className="relative size-[88px] shrink-0">
          <div
            aria-hidden
            className="size-full rounded-full"
            style={{ background: donutBg }}
          />
          <div className="absolute inset-[18%] flex flex-col items-center justify-center rounded-full bg-card text-center">
            <span className="text-xl font-extrabold tabular-nums leading-none text-ink">
              {projectTotal}
            </span>
            <span className="mt-0.5 text-[10px] font-medium text-ink-3">
              项目总数
            </span>
          </div>
        </div>
        <div className="min-w-0 flex-1">
          <div className="text-[12px] font-extrabold text-ink">项目层</div>
          <p className="mt-0.5 text-[11px] text-ink-4">{projectLabel}</p>
          <ul className="mt-2 flex flex-col gap-1">
            {layerRows.map((row) => (
              <li
                key={row.key}
                className="flex items-center justify-between gap-2 text-[12px] text-ink-3"
              >
                <span className="inline-flex items-center gap-1.5">
                  <span
                    aria-hidden
                    className="size-2 shrink-0 rounded-full"
                    style={{ background: row.color }}
                  />
                  {row.label}
                </span>
                <span className="font-extrabold tabular-nums text-ink">
                  {row.value}
                </span>
              </li>
            ))}
          </ul>
        </div>
      </div>

      {/* 任务层 */}
      <div
        className="flex min-w-0 flex-col justify-center rounded-[14px] border border-line bg-card px-4 py-3.5 shadow-sm"
        data-testid="project-portfolio-task-summary"
      >
        <div className="text-[12px] font-extrabold text-ink">任务层</div>
        <p className="mt-0.5 text-[11px] text-ink-4">非归档项目任务</p>
        {taskCounts ? (
          <div className="mt-2.5">
            <TaskCompositionBar
              counts={taskCounts}
              legendMode="primary"
              showTitle
              size="summary"
            />
          </div>
        ) : (
          <p className="mt-3 text-[12px] text-ink-3">暂无任务统计</p>
        )}
      </div>
    </div>
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

  const statusRows: Array<{
    label: string;
    value: number;
    dot: string;
    valueClass?: string;
  }> = [
    {
      label: projectStatusLabel("running"),
      value: portfolio.running,
      dot: "bg-brand",
      valueClass: portfolio.running > 0 ? "text-brand-deep" : undefined,
    },
    {
      label: "验收中",
      value: portfolio.acceptance,
      dot: "bg-info",
      valueClass: portfolio.acceptance > 0 ? "text-info-text" : undefined,
    },
    {
      label: "协调异常",
      value: portfolio.coordinationAnomaly,
      dot: portfolio.coordinationAnomaly > 0 ? "bg-danger" : "bg-ink-4",
      valueClass:
        portfolio.coordinationAnomaly > 0 ? "text-danger-text" : "text-ink-3",
    },
    {
      label: "已归档",
      value: portfolio.archived,
      dot: "bg-ink-4",
      valueClass: "text-ink-3",
    },
  ];

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

      {/* 状态分布：色点与顶栏项目层同源 */}
      <div className="rounded-[10px] border border-line/70 bg-card-soft/50 p-3">
        <div className="text-[11px] font-semibold text-ink-3">状态分布</div>
        <div className="mt-2 flex flex-col gap-1.5 text-[12px] text-ink-2">
          {statusRows.map((row) => (
            <div key={row.label} className="flex items-center justify-between gap-2">
              <span className="inline-flex items-center gap-1.5">
                <span
                  aria-hidden
                  className={cn("size-2 shrink-0 rounded-full", row.dot)}
                />
                {row.label}
              </span>
              <span
                className={cn(
                  "font-extrabold tabular-nums text-ink",
                  row.valueClass,
                )}
              >
                {row.value}
              </span>
            </div>
          ))}
        </div>
      </div>

      {/* 需关注：中性区块底（贴合冷紫主调），语义只落在左边条 / 标题点 / 副文案 */}
      <div className="rounded-[10px] border border-line/70 bg-card-soft/50 p-3">
        <div className="flex items-center gap-1.5 text-[11px] font-semibold text-ink-3">
          {needAttention.length > 0 ? (
            <span aria-hidden className="size-1.5 shrink-0 rounded-full bg-danger" />
          ) : null}
          需要关注
          <span
            className={cn(
              "tabular-nums",
              needAttention.length > 0 ? "font-extrabold text-danger-text" : "",
            )}
          >
            · {needAttention.length}
          </span>
        </div>
        {needAttention.length === 0 ? (
          <p className="mt-1.5 text-[12px] text-ink-3">
            当前列表没有高关注信号项目。
          </p>
        ) : (
          <ul className="mt-2 flex flex-col gap-1.5">
            {needAttention.map(({ project, summary }) => {
              const headline = formatAttentionHeadline(summary);
              const isDanger = summary.level === "danger";
              return (
                <li key={project.id}>
                  <button
                    className={cn(
                      "w-full rounded-[8px] border border-line/60 bg-card px-2.5 py-2 text-left transition-colors",
                      "hover:border-brand/30 hover:bg-brand-soft/25",
                      isDanger
                        ? "border-l-[3px] border-l-danger"
                        : "border-l-[3px] border-l-warn",
                    )}
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

      {/* 协调异常：同样中性底，计数/名单用 danger 字色 */}
      <div className="rounded-[10px] border border-line/70 bg-card-soft/50 p-3">
        <div className="flex items-center gap-1.5 text-[11px] font-semibold text-ink-3">
          {coordinationAnomalies.length > 0 ? (
            <span aria-hidden className="size-1.5 shrink-0 rounded-full bg-danger" />
          ) : null}
          协调异常
          <span
            className={cn(
              "tabular-nums",
              coordinationAnomalies.length > 0
                ? "font-extrabold text-danger-text"
                : "",
            )}
          >
            · {coordinationAnomalies.length}
          </span>
        </div>
        {coordinationAnomalies.length === 0 ? (
          <p className="mt-1.5 text-[12px] text-ink-3">无协调异常项目。</p>
        ) : (
          <ul className="mt-2 flex flex-col gap-1 text-[12px]">
            {coordinationAnomalies.map((project) => (
              <li key={project.id}>
                <button
                  className="w-full truncate text-left font-medium text-ink-2 hover:text-danger-text"
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

      {/* 今日完成：中性底，有数时数字用 ok */}
      <div className="rounded-[10px] border border-line/70 bg-card-soft/50 p-3">
        <div className="flex items-center justify-between gap-2">
          <div className="text-[11px] font-semibold text-ink-3">今日完成运行</div>
          <span
            className={cn(
              "font-extrabold tabular-nums",
              todayTotal > 0 ? "text-ok-text" : "text-ink",
            )}
          >
            {todayTotal}
          </span>
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
                  <span className="shrink-0 font-semibold tabular-nums text-ok-text">
                    {item.completed_today_count}
                  </span>
                </li>
              );
            })}
          </ul>
        )}
      </div>

      {/* 长期静默：中性底 */}
      <div className="rounded-[10px] border border-line/70 bg-card-soft/50 p-3">
        <div className="text-[11px] font-semibold text-ink-3">
          长期无活动（约 7 日+）
          {stale.length > 0 ? (
            <span className="tabular-nums text-ink-2"> · {stale.length}</span>
          ) : null}
        </div>
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
