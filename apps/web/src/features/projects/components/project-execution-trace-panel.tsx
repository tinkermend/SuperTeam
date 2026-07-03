import { ActivitySquare, AlertTriangle, Boxes, Clock3, FileCheck2 } from "lucide-react";
import {
  IconTile,
  SoftCard,
  StatusPill,
  V3EmptyState,
  V3ErrorState,
  V3LoadingState,
  V3MetricCard,
  type V3Tone,
} from "@/components/superteam";
import type {
  ExecutionLedgerEvent,
  ProjectExecutionTrace,
  ProjectExecutionTraceAttempt,
} from "@/lib/api/projects";
import { statusLabel } from "@/lib/status-labels";

type ProjectExecutionTracePanelProps = {
  errorMessage?: string;
  isError?: boolean;
  isLoading?: boolean;
  onRetry?: () => void;
  trace?: ProjectExecutionTrace;
};

export function ProjectExecutionTracePanel({
  errorMessage,
  isError,
  isLoading,
  onRetry,
  trace,
}: ProjectExecutionTracePanelProps) {
  const attempts = trace?.attempts ?? [];
  const summary = trace?.summary;
  const attemptCount = summary?.attempt_count ?? attempts.length;

  return (
    <SoftCard className="overflow-hidden">
      <div className="flex items-center justify-between gap-3 border-b border-v3-line p-4">
        <div className="flex min-w-0 items-center gap-3">
          <IconTile tone="info" size="sm">
            <ActivitySquare />
          </IconTile>
          <div className="min-w-0">
            <h3 className="font-semibold text-v3-ink">执行证据链</h3>
            <p className="truncate text-xs text-v3-ink-2">
              Runtime 尝试、回写事件、证据与工件引用
            </p>
          </div>
        </div>
        <StatusPill tone="info">{attemptCount} 次</StatusPill>
      </div>

      {isLoading ? (
        <V3LoadingState className="min-h-32" label="正在加载执行证据链" />
      ) : isError ? (
        <div className="p-4">
          <V3ErrorState
            className="min-h-32"
            description={errorMessage || "请重试或稍后查看执行证据链。"}
            onRetry={onRetry}
            title="执行证据链加载失败"
          />
        </div>
      ) : attempts.length === 0 ? (
        <V3EmptyState className="min-h-32 py-12" title="暂无执行证据链" />
      ) : (
        <div className="grid gap-4 p-4">
          <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-4">
            <V3MetricCard
              icon={<AlertTriangle />}
              iconTone="danger"
              label="失败尝试"
              value={String(summary?.failed_attempt_count ?? 0)}
            />
            <V3MetricCard
              icon={<FileCheck2 />}
              iconTone="warn"
              label="人工复核"
              value={String(summary?.human_review_required_count ?? 0)}
            />
            <V3MetricCard
              icon={<Boxes />}
              iconTone="artifact"
              label="工件引用"
              value={String(summary?.artifact_ref_count ?? 0)}
            />
            <V3MetricCard
              icon={<FileCheck2 />}
              iconTone="ok"
              label="证据引用"
              value={String(summary?.evidence_ref_count ?? 0)}
            />
          </div>

          {summary?.latest_error_family ? (
            <div className="flex flex-wrap items-center gap-2 rounded-v3-inner bg-v3-danger-soft px-3 py-2 text-xs">
              <span className="font-medium text-v3-danger">最新错误</span>
              <StatusPill
                tone="danger"
                showDot={false}
                title={summary.latest_error_family}
              >
                {summary.latest_error_family}
              </StatusPill>
            </div>
          ) : null}

          <div className="grid gap-3">
            {attempts.map((attempt) => (
              <AttemptRow attempt={attempt} key={attempt.attempt_id} />
            ))}
          </div>
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
      className="grid gap-3 rounded-v3-inner bg-v3-card-soft p-3"
    >
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="min-w-0">
          <div className="flex flex-wrap items-center gap-2">
            <h4 className="text-sm font-semibold text-v3-ink">执行尝试 {attempt.attempt_no}</h4>
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
              className="mt-1 break-all text-xs text-v3-ink-2"
              title={attempt.failure_family}
            >
              失败族：{attempt.failure_family}
            </p>
          ) : null}
        </div>
        <div className="flex items-center gap-1.5 text-xs text-v3-ink-2">
          <Clock3 className="size-3.5" />
          <span>{formatTimestamp(attempt.finished_at ?? attempt.started_at)}</span>
        </div>
      </div>

      <div className="grid gap-2 md:grid-cols-2 xl:grid-cols-4">
        <MetaBlock label="任务" value={attempt.project_task_id} />
        <MetaBlock label="Provider" value={attempt.provider_type ?? "未记录"} />
        <MetaBlock label="Session" value={attempt.provider_session_id ?? "未记录"} />
        <MetaBlock label="Runtime" value={attempt.runtime_node_id ?? "未记录"} />
      </div>

      {summary ? (
        <div className="grid gap-2 rounded-v3-inner bg-v3-card p-3">
          <div className="flex flex-wrap items-center justify-between gap-2">
            <p className="text-xs font-semibold text-v3-ink">执行摘要</p>
            {summary.requires_human_review ? (
              <StatusPill tone="warn">需人工复核</StatusPill>
            ) : (
              <StatusPill tone="ok">已回写</StatusPill>
            )}
          </div>
          <p
            className="whitespace-pre-wrap break-words text-sm leading-6 text-v3-ink"
            title={summary.conclusion}
          >
            {summary.conclusion}
          </p>
          <div className="flex flex-wrap gap-2 text-xs text-v3-ink-2">
            <span>证据 {summary.evidence_refs.length}</span>
            <span>工件 {summary.artifact_refs.length}</span>
            <span>{formatTimestamp(summary.created_at)}</span>
          </div>
        </div>
      ) : null}

      <div className="grid gap-2">
        <div className="flex items-center justify-between gap-3">
          <p className="text-xs font-semibold text-v3-ink">Ledger Events</p>
          <span className="text-xs text-v3-ink-2">
            {attempt.events.length} 条
          </span>
        </div>
        {attempt.events.length === 0 ? (
          <div className="rounded-v3-inner bg-v3-card p-3 text-sm text-v3-ink-2">
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

  return (
    <div className="grid gap-2 rounded-v3-inner bg-v3-card p-3">
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
            className="mt-1 break-all text-xs text-v3-ink-2"
            title={actorSourceLabel}
          >
            {actorSourceLabel}
          </p>
        </div>
        <div className="flex flex-wrap justify-end gap-2 text-xs text-v3-ink-2">
          <span>证据 {event.evidence_refs.length}</span>
          <span>工件 {event.artifact_refs.length}</span>
          <span>{formatTimestamp(event.occurred_at)}</span>
        </div>
      </div>

      {event.input_summary || event.output_summary ? (
        <div className="grid gap-2 md:grid-cols-2">
          <SummaryBlock label="Input" value={event.input_summary} />
          <SummaryBlock label="Output" value={event.output_summary} />
        </div>
      ) : null}

      {event.error_family || event.error_code || event.error_message ? (
        <div className="grid gap-1 rounded-v3-inner bg-v3-danger-soft p-2 text-xs">
          <p className="break-all font-medium text-v3-danger" title={errorHeader}>
            错误 {errorHeader}
          </p>
          {event.error_message ? (
            <p
              className="whitespace-pre-wrap break-words text-v3-ink-2"
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

function MetaBlock({ label, value }: { label: string; value: string }) {
  return (
    <div className="min-w-0 rounded-v3-inner bg-v3-card p-2">
      <p className="text-[11px] text-v3-ink-3">{label}</p>
      <p className="mt-1 break-all font-mono text-xs text-v3-ink" title={value}>
        {value}
      </p>
    </div>
  );
}

function SummaryBlock({ label, value }: { label: string; value?: string }) {
  return (
    <div className="min-w-0 rounded-v3-inner bg-v3-card-soft p-2">
      <p className="text-[11px] text-v3-ink-3">{label}</p>
      <p
        className="mt-1 whitespace-pre-wrap break-words text-xs leading-5 text-v3-ink-2"
        title={value}
      >
        {value || "未记录"}
      </p>
    </div>
  );
}

function attemptStatusTone(status: string): V3Tone {
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

function eventTone(event: ExecutionLedgerEvent): V3Tone {
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
