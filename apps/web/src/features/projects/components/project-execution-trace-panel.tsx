import { useEffect, useMemo, useState } from "react";
import { ActivitySquare, AlertTriangle, Boxes, Clock3, FileCheck2 } from "lucide-react";
import {
  IconTile,
  SoftCard,
  StatusPill,
  Button,
  EmptyState,
  ErrorState,
  LoadingState,
  MetricCard,
  type Tone
} from "@/components/superteam";
import type {
  ExecutionLedgerEvent,
  ProjectExecutionTrace,
  ProjectExecutionTraceAttempt
} from "@/lib/api/projects";
import { failureFamilyLabel, statusLabel } from "@/lib/status-labels";
import { AttemptCapabilityProjection } from "./attempt-capability-projection";

type ProjectExecutionTracePanelProps = {
  errorMessage?: string;
  /** ?task= 深链：预选任务过滤，直接定位到该任务的执行尝试。 */
  focusTaskId?: string;
  isError?: boolean;
  isLoading?: boolean;
  onRetry?: () => void;
  /** 任务过滤下拉的显示名（id → 标题）；缺名回退 mono id（技术详情区例外）。 */
  taskTitlesById?: ReadonlyMap<string, string>;
  trace?: ProjectExecutionTrace;
};

const ALL_TASKS_FILTER = "all";

export function ProjectExecutionTracePanel({
  errorMessage,
  focusTaskId,
  isError,
  isLoading,
  onRetry,
  taskTitlesById,
  trace
}: ProjectExecutionTracePanelProps) {
  const attempts = trace?.attempts ?? [];
  const summary = trace?.summary;
  const attemptCount = summary?.attempt_count ?? attempts.length;
  const [taskFilter, setTaskFilter] = useState(focusTaskId ?? ALL_TASKS_FILTER);
  // 深链任务变化（弹层「查看执行轨迹」重新进入）时同步重置过滤。
  useEffect(() => {
    setTaskFilter(focusTaskId ?? ALL_TASKS_FILTER);
  }, [focusTaskId]);
  const attemptTaskIds = useMemo(
    () => Array.from(new Set(attempts.map((attempt) => attempt.project_task_id))),
    [attempts],
  );
  const visibleAttempts =
    taskFilter === ALL_TASKS_FILTER
      ? attempts
      : attempts.filter((attempt) => attempt.project_task_id === taskFilter);
  // 单任务且未带深链过滤时下拉没有意义，不渲染（也避免与技术区 mono id 重复展示）。
  const showTaskFilter =
    attempts.length > 0 &&
    (attemptTaskIds.length > 1 || taskFilter !== ALL_TASKS_FILTER);

  return (
    <SoftCard className="overflow-hidden">
      <div className="flex flex-wrap items-center justify-between gap-3 border-b border-line p-4">
        <div className="flex min-w-0 items-center gap-3">
          <IconTile tone="info" size="sm">
            <ActivitySquare />
          </IconTile>
          <div className="min-w-0">
            <h3 className="font-semibold text-ink">执行证据链</h3>
            <p className="truncate text-xs text-ink-2">
              Runtime 尝试、回写事件、证据与工件引用
            </p>
          </div>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          {showTaskFilter ? (
            <select
              aria-label="按任务过滤执行尝试"
              className="h-8 max-w-56 truncate rounded-inner border border-line bg-card px-2 text-xs font-semibold text-ink"
              data-testid="trace-task-filter"
              value={taskFilter}
              onChange={(event) => setTaskFilter(event.target.value)}
            >
              <option value={ALL_TASKS_FILTER}>全部任务</option>
              {/* 深链任务不在当前证据链里：仍列出选中项，让空态可解释。 */}
              {taskFilter !== ALL_TASKS_FILTER &&
              !attemptTaskIds.includes(taskFilter) ? (
                <option value={taskFilter}>
                  {taskTitlesById?.get(taskFilter) ?? taskFilter}
                </option>
              ) : null}
              {attemptTaskIds.map((taskId) => (
                <option key={taskId} value={taskId}>
                  {taskTitlesById?.get(taskId) ?? taskId}
                </option>
              ))}
            </select>
          ) : null}
          <StatusPill tone="info">{attemptCount} 次</StatusPill>
        </div>
      </div>

      {isLoading ? (
        <LoadingState className="min-h-32" label="正在加载执行证据链" />
      ) : isError ? (
        <div className="p-4">
          <ErrorState
            className="min-h-32"
            description={errorMessage || "请重试或稍后查看执行证据链。"}
            onRetry={onRetry}
            title="执行证据链加载失败"
          />
        </div>
      ) : attempts.length === 0 ? (
        <EmptyState className="min-h-32 py-12" title="暂无执行证据链" />
      ) : (
        <div className="grid gap-4 p-4">
          <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-4">
            <MetricCard
              icon={<AlertTriangle />}
              iconTone="danger"
              label="失败尝试"
              value={String(summary?.failed_attempt_count ?? 0)}
            />
            <MetricCard
              icon={<FileCheck2 />}
              iconTone="warn"
              label="人工复核"
              value={String(summary?.human_review_required_count ?? 0)}
            />
            <MetricCard
              icon={<Boxes />}
              iconTone="artifact"
              label="工件引用"
              value={String(summary?.artifact_ref_count ?? 0)}
            />
            <MetricCard
              icon={<FileCheck2 />}
              iconTone="ok"
              label="证据引用"
              value={String(summary?.evidence_ref_count ?? 0)}
            />
          </div>

          {summary?.latest_error_family ? (
            <div className="flex flex-wrap items-center gap-2 rounded-inner bg-danger-soft px-3 py-2 text-xs">
              <span className="font-medium text-danger">最新错误</span>
              <StatusPill
                tone="danger"
                showDot={false}
                title={summary.latest_error_family}
              >
                {summary.latest_error_family}
              </StatusPill>
            </div>
          ) : null}

          {visibleAttempts.length === 0 ? (
            <EmptyState
              action={
                <Button
                  size="sm"
                  type="button"
                  variant="outline"
                  onClick={() => setTaskFilter(ALL_TASKS_FILTER)}
                >
                  显示全部任务
                </Button>
              }
              className="min-h-32 py-8"
              data-testid="trace-task-filter-empty"
              title="该任务暂无执行尝试记录"
            />
          ) : (
            <div className="grid gap-3">
              {visibleAttempts.map((attempt) => (
                <AttemptRow attempt={attempt} key={attempt.attempt_id} />
              ))}
            </div>
          )}
        </div>
      )}
    </SoftCard>
  );
}

function AttemptRow({ attempt }: { attempt: ProjectExecutionTraceAttempt }) {
  const summary = attempt.summary;

  return (
    <section
      aria-label={`执行尝试 ${attempt.attempt_no}`}
      className="grid gap-3 rounded-inner bg-card-soft p-3"
    >
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="min-w-0">
          <div className="flex flex-wrap items-center gap-2">
            <h4 className="text-sm font-semibold text-ink">执行尝试 {attempt.attempt_no}</h4>
            <StatusPill tone={attemptStatusTone(attempt.status)}>
              {statusLabel(attempt.status)}
            </StatusPill>
            {attempt.retryable !== undefined ? (
              <StatusPill tone={attempt.retryable ? "warn" : "mute"}>
                {attempt.retryable ? "可重试" : "不可重试"}
              </StatusPill>
            ) : null}
          </div>
          {attempt.failure_family ? (
            <p
              className="mt-1 break-all text-xs text-ink-2"
              title={attempt.failure_family}
            >
              失败族：{failureFamilyLabel(attempt.failure_family)}
            </p>
          ) : null}
        </div>
        <div className="flex items-center gap-1.5 text-xs text-ink-2">
          <Clock3 className="size-3.5" />
          <span>{formatTimestamp(attempt.finished_at ?? attempt.started_at)}</span>
        </div>
      </div>

      <div className="grid gap-2 md:grid-cols-2 xl:grid-cols-4">
        <MetaBlock label="任务" value={attempt.project_task_id} />
        <MetaBlock label="Provider" value={attempt.provider_type ?? "未记录"} />
        <MetaBlock
          label="会话接续"
          value={attempt.session_resume_label ?? (attempt.session_resume_status ? attempt.session_resume_status : "—")}
        />
        <MetaBlock label="Session" value={attempt.provider_session_id ?? "未记录"} />
        <MetaBlock label="Runtime" value={attempt.runtime_node_id ?? "未记录"} />
      </div>

      <AttemptCapabilityProjection projection={attempt.capability_projection} />

      {summary ? (
        <div className="grid gap-2 rounded-inner bg-card p-3">
          <div className="flex flex-wrap items-center justify-between gap-2">
            <p className="text-xs font-semibold text-ink">执行摘要</p>
            {summary.requires_human_review ? (
              <StatusPill tone="warn">需人工复核</StatusPill>
            ) : (
              <StatusPill tone="ok">已回写</StatusPill>
            )}
          </div>
          <p
            className="whitespace-pre-wrap break-words text-sm leading-6 text-ink"
            title={summary.conclusion}
          >
            {summary.conclusion}
          </p>
          <div className="flex flex-wrap gap-2 text-xs text-ink-2">
            <span>证据 {summary.evidence_refs.length}</span>
            <span>工件 {summary.artifact_refs.length}</span>
            <span>{formatTimestamp(summary.created_at)}</span>
          </div>
        </div>
      ) : null}

      <div className="grid gap-2">
        <div className="flex items-center justify-between gap-3">
          <p className="text-xs font-semibold text-ink">Ledger Events</p>
          <span className="text-xs text-ink-2">
            {attempt.events.length} 条
          </span>
        </div>
        {attempt.events.length === 0 ? (
          <div className="rounded-inner bg-card p-3 text-sm text-ink-2">
            暂无执行事件
          </div>
        ) : (
          <div className="grid gap-2">
            {attempt.events.map((event) => (
              <EventRow event={event} key={event.id} />
            ))}
          </div>
        )}
      </div>
    </section>
  );
}

function EventRow({ event }: { event: ExecutionLedgerEvent }) {
  const actorSourceLabel = `${event.actor_type}${
    event.actor_id ? `:${event.actor_id}` : ""
  } · ${event.source_type}:${event.source_id}`;
  const errorHeader = formatErrorHeader(event);
  const tool = readToolEvent(event);

  return (
    <div className="grid gap-2 rounded-inner bg-card p-3">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="min-w-0">
          <div className="flex flex-wrap items-center gap-2">
            <StatusPill tone={eventTone(event)} title={event.event_type}>
              {event.event_type}
            </StatusPill>
            {event.retryable !== undefined ? (
              <StatusPill tone={event.retryable ? "warn" : "mute"}>
                {event.retryable ? "可重试" : "不可重试"}
              </StatusPill>
            ) : null}
          </div>
          <p
            className="mt-1 break-all text-xs text-ink-2"
            title={actorSourceLabel}
          >
            {actorSourceLabel}
          </p>
        </div>
        <div className="flex flex-wrap justify-end gap-2 text-xs text-ink-2">
          <span>证据 {event.evidence_refs.length}</span>
          <span>工件 {event.artifact_refs.length}</span>
          <span>{formatTimestamp(event.occurred_at)}</span>
        </div>
      </div>

      {tool ? <ToolBlock tool={tool} /> : null}

      {!tool && (event.input_summary || event.output_summary) ? (
        <div className="grid gap-2 md:grid-cols-2">
          <SummaryBlock label="Input" value={event.input_summary} />
          <SummaryBlock label="Output" value={event.output_summary} />
        </div>
      ) : null}

      {event.error_family || event.error_code || event.error_message ? (
        <div className="grid gap-1 rounded-inner bg-danger-soft p-2 text-xs">
          <p className="break-all font-medium text-danger" title={errorHeader}>
            错误 {errorHeader}
          </p>
          {event.error_message ? (
            <p
              className="whitespace-pre-wrap break-words text-ink-2"
              title={event.error_message}
            >
              {event.error_message}
            </p>
          ) : null}
        </div>
      ) : null}
    </div>
  );
}

type ToolEvent = {
  excerpt: string;
  isError?: boolean;
  kind: "started" | "completed";
  name?: string;
  toolId: string;
  truncated: boolean;
};

/**
 * `is_error` originates from the provider process, never from model prose.
 * See docs/superpowers/specs/2026-07-09-provider-transcript-tool-event-capture-design.md §3.3.
 */
function readToolEvent(event: ExecutionLedgerEvent): ToolEvent | undefined {
  const kind =
    event.input_summary === "tool_started"
      ? "started"
      : event.input_summary === "tool_completed"
        ? "completed"
        : undefined;
  if (!kind) {
    return undefined;
  }

  const metadata = event.metadata ?? {};
  const toolId = typeof metadata.tool_id === "string" ? metadata.tool_id : "";
  if (!toolId) {
    return undefined;
  }

  const rawExcerpt =
    kind === "started" ? metadata.input_excerpt : metadata.output_excerpt;
  const rawTruncated =
    kind === "started" ? metadata.input_truncated : metadata.output_truncated;

  return {
    excerpt: typeof rawExcerpt === "string" ? rawExcerpt : "",
    isError: typeof metadata.is_error === "boolean" ? metadata.is_error : undefined,
    kind,
    name: typeof metadata.name === "string" ? metadata.name : undefined,
    toolId,
    truncated: rawTruncated === true
};
}

function ToolBlock({ tool }: { tool: ToolEvent }) {
  const label = tool.kind === "started" ? "工具调用" : "工具结果";

  return (
    <div className="grid gap-2 rounded-inner bg-card-soft p-2">
      <div className="flex flex-wrap items-center gap-2">
        <span className="text-[11px] text-ink-3">{label}</span>
        {tool.name ? (
          <span className="font-mono text-xs text-ink">{tool.name}</span>
        ) : null}
        {tool.isError !== undefined ? (
          <StatusPill tone={tool.isError ? "danger" : "ok"}>
            {tool.isError ? "失败" : "成功"}
          </StatusPill>
        ) : null}
      </div>
      {tool.excerpt ? (
        <details className="min-w-0">
          <summary className="cursor-pointer truncate font-mono text-xs text-ink-2">
            {tool.excerpt}
          </summary>
          <pre className="mt-2 max-h-64 overflow-auto whitespace-pre-wrap break-words rounded-inner bg-card p-2 font-mono text-xs leading-5 text-ink-2">
            {tool.excerpt}
          </pre>
        </details>
      ) : null}
      {tool.truncated ? (
        <p className="text-[11px] text-ink-3">
          内容已截断，完整日志将在证据地基落地后可下载。
        </p>
      ) : null}
    </div>
  );
}

function MetaBlock({ label, value }: { label: string; value: string }) {
  return (
    <div className="min-w-0 rounded-inner bg-card p-2">
      <p className="text-[11px] text-ink-3">{label}</p>
      <p className="mt-1 break-all font-mono text-xs text-ink" title={value}>
        {value}
      </p>
    </div>
  );
}

function SummaryBlock({ label, value }: { label: string; value?: string }) {
  return (
    <div className="min-w-0 rounded-inner bg-card-soft p-2">
      <p className="text-[11px] text-ink-3">{label}</p>
      <p
        className="mt-1 whitespace-pre-wrap break-words text-xs leading-5 text-ink-2"
        title={value}
      >
        {value || "未记录"}
      </p>
    </div>
  );
}

function attemptStatusTone(status: string): Tone {
  if (status === "completed" || status === "succeeded") {
    return "ok";
  }
  if (status === "failed" || status === "cancelled") {
    return "danger";
  }
  if (status === "running" || status === "started") {
    return "info";
  }
  if (status === "queued" || status === "waiting_human") {
    return "warn";
  }
  return "mute";
}

function eventTone(event: ExecutionLedgerEvent): Tone {
  if (event.error_family || event.error_message || event.event_type.includes("failed")) {
    return "danger";
  }
  if (
    event.event_type.includes("completed") ||
    event.event_type.includes("succeeded") ||
    event.event_type.includes("summary")
  ) {
    return "ok";
  }
  if (event.event_type.includes("human") || event.event_type.includes("wait")) {
    return "warn";
  }
  return "info";
}

function formatErrorHeader(event: ExecutionLedgerEvent) {
  return [event.error_family, event.error_code].filter(Boolean).join(" · ") || "未分类";
}

function formatTimestamp(value?: string) {
  if (!value) {
    return "未记录时间";
  }

  return value.replace("T", " ").replace(/\.\d+Z$/, "Z").replace("Z", " UTC");
}
