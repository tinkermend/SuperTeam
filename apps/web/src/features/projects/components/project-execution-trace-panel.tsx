import type { ReactNode } from "react";
import { ActivitySquare, AlertTriangle, Boxes, Clock3, FileCheck2 } from "lucide-react";
import {
  LiquidCard,
  SemanticIconTile,
  StatusBadge,
  type Tone,
} from "@/components/superteam";
import type {
  ExecutionLedgerEvent,
  ProjectExecutionTrace,
  ProjectExecutionTraceAttempt,
} from "@/lib/api/projects";

type ProjectExecutionTracePanelProps = {
  trace?: ProjectExecutionTrace;
};

export function ProjectExecutionTracePanel({
  trace,
}: ProjectExecutionTracePanelProps) {
  const attempts = trace?.attempts ?? [];
  const summary = trace?.summary;
  const attemptCount = summary?.attempt_count ?? attempts.length;

  return (
    <LiquidCard className="rounded-xl">
      <div className="flex items-center justify-between gap-3 border-b p-4">
        <div className="flex min-w-0 items-center gap-3">
          <SemanticIconTile tone="info" size="sm">
            <ActivitySquare />
          </SemanticIconTile>
          <div className="min-w-0">
            <h3 className="font-semibold">执行证据链</h3>
            <p className="truncate text-xs text-muted-foreground">
              Runtime 尝试、回写事件、证据与工件引用
            </p>
          </div>
        </div>
        <StatusBadge tone="info">{attemptCount} 次</StatusBadge>
      </div>

      {attempts.length === 0 ? (
        <div className="flex min-h-32 items-center justify-center p-4 text-sm text-muted-foreground">
          暂无执行证据链
        </div>
      ) : (
        <div className="grid gap-4 p-4">
          <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-4">
            <MetricBlock
              icon={<AlertTriangle />}
              label="失败尝试"
              tone="danger"
              value={String(summary?.failed_attempt_count ?? 0)}
            />
            <MetricBlock
              icon={<FileCheck2 />}
              label="人工复核"
              tone="warning"
              value={String(summary?.human_review_required_count ?? 0)}
            />
            <MetricBlock
              icon={<Boxes />}
              label="工件引用"
              tone="artifact"
              value={String(summary?.artifact_ref_count ?? 0)}
            />
            <MetricBlock
              icon={<FileCheck2 />}
              label="证据引用"
              tone="decision"
              value={String(summary?.evidence_ref_count ?? 0)}
            />
          </div>

          {summary?.latest_error_family ? (
            <div className="flex flex-wrap items-center gap-2 rounded-lg border border-destructive/20 bg-destructive/5 px-3 py-2 text-xs">
              <span className="font-medium text-destructive">最新错误</span>
              <StatusBadge tone="danger" showDot={false}>
                {summary.latest_error_family}
              </StatusBadge>
            </div>
          ) : null}

          <div className="grid gap-3">
            {attempts.map((attempt) => (
              <AttemptRow attempt={attempt} key={attempt.attempt_id} />
            ))}
          </div>
        </div>
      )}
    </LiquidCard>
  );
}

function AttemptRow({ attempt }: { attempt: ProjectExecutionTraceAttempt }) {
  const summary = attempt.summary;

  return (
    <section
      aria-label={`执行尝试 ${attempt.attempt_no}`}
      className="grid gap-3 rounded-lg border bg-white/55 p-3"
    >
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="min-w-0">
          <div className="flex flex-wrap items-center gap-2">
            <h4 className="text-sm font-semibold">执行尝试 {attempt.attempt_no}</h4>
            <StatusBadge tone={attemptStatusTone(attempt.status)}>
              {attempt.status}
            </StatusBadge>
            {attempt.retryable !== undefined ? (
              <StatusBadge tone={attempt.retryable ? "warning" : "neutral"}>
                {attempt.retryable ? "可重试" : "不可重试"}
              </StatusBadge>
            ) : null}
          </div>
          {attempt.failure_family ? (
            <p className="mt-1 text-xs text-muted-foreground">
              失败族：{attempt.failure_family}
            </p>
          ) : null}
        </div>
        <div className="flex items-center gap-1.5 text-xs text-muted-foreground">
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
        <div className="grid gap-2 rounded-md border bg-white/60 p-3">
          <div className="flex flex-wrap items-center justify-between gap-2">
            <p className="text-xs font-semibold">执行摘要</p>
            {summary.requires_human_review ? (
              <StatusBadge tone="warning">需人工复核</StatusBadge>
            ) : (
              <StatusBadge tone="success">已回写</StatusBadge>
            )}
          </div>
          <p className="line-clamp-3 text-sm leading-6">{summary.conclusion}</p>
          <div className="flex flex-wrap gap-2 text-xs text-muted-foreground">
            <span>证据 {summary.evidence_refs.length}</span>
            <span>工件 {summary.artifact_refs.length}</span>
            <span>{formatTimestamp(summary.created_at)}</span>
          </div>
        </div>
      ) : null}

      <div className="grid gap-2">
        <div className="flex items-center justify-between gap-3">
          <p className="text-xs font-semibold">Ledger Events</p>
          <span className="text-xs text-muted-foreground">
            {attempt.events.length} 条
          </span>
        </div>
        {attempt.events.length === 0 ? (
          <div className="rounded-md border bg-white/60 p-3 text-sm text-muted-foreground">
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
  return (
    <div className="grid gap-2 rounded-md border bg-white/70 p-3">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="min-w-0">
          <div className="flex flex-wrap items-center gap-2">
            <StatusBadge tone={eventTone(event)}>{event.event_type}</StatusBadge>
            {event.retryable !== undefined ? (
              <StatusBadge tone={event.retryable ? "warning" : "neutral"}>
                {event.retryable ? "可重试" : "不可重试"}
              </StatusBadge>
            ) : null}
          </div>
          <p className="mt-1 truncate text-xs text-muted-foreground">
            {event.actor_type}
            {event.actor_id ? `:${event.actor_id}` : ""} · {event.source_type}:
            {event.source_id}
          </p>
        </div>
        <div className="flex flex-wrap justify-end gap-2 text-xs text-muted-foreground">
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
        <div className="grid gap-1 rounded-md border border-destructive/20 bg-destructive/5 p-2 text-xs">
          <p className="font-medium text-destructive">
            错误 {formatErrorHeader(event)}
          </p>
          {event.error_message ? (
            <p className="line-clamp-2 text-muted-foreground">
              {event.error_message}
            </p>
          ) : null}
        </div>
      ) : null}
    </div>
  );
}

function MetricBlock({
  icon,
  label,
  tone,
  value,
}: {
  icon: ReactNode;
  label: string;
  tone: Tone;
  value: string;
}) {
  return (
    <div className="min-w-0 rounded-lg border bg-white/55 p-3">
      <div className="flex items-center gap-2 text-xs text-muted-foreground">
        <span className="[&_svg]:size-3.5">{icon}</span>
        {label}
      </div>
      <p className={`mt-2 truncate text-lg font-semibold ${toneTextClass(tone)}`}>
        {value}
      </p>
    </div>
  );
}

function MetaBlock({ label, value }: { label: string; value: string }) {
  return (
    <div className="min-w-0 rounded-md border bg-white/60 p-2">
      <p className="text-[11px] text-muted-foreground">{label}</p>
      <p className="mt-1 truncate font-mono text-xs">{value}</p>
    </div>
  );
}

function SummaryBlock({ label, value }: { label: string; value?: string }) {
  return (
    <div className="min-w-0 rounded-md border bg-white/60 p-2">
      <p className="text-[11px] text-muted-foreground">{label}</p>
      <p className="mt-1 line-clamp-2 text-xs leading-5">
        {value || "未记录"}
      </p>
    </div>
  );
}

function attemptStatusTone(status: string): Tone {
  if (status === "completed" || status === "succeeded") {
    return "success";
  }
  if (status === "failed" || status === "cancelled") {
    return "danger";
  }
  if (status === "running" || status === "started") {
    return "info";
  }
  if (status === "queued" || status === "waiting_human") {
    return "warning";
  }
  return "neutral";
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
    return "success";
  }
  if (event.event_type.includes("human") || event.event_type.includes("wait")) {
    return "warning";
  }
  return "info";
}

function toneTextClass(tone: Tone) {
  if (tone === "danger") {
    return "text-[color:var(--superteam-danger)]";
  }
  if (tone === "warning") {
    return "text-[color:var(--superteam-warning)]";
  }
  if (tone === "artifact") {
    return "text-[color:var(--superteam-artifact)]";
  }
  if (tone === "decision") {
    return "text-[color:var(--superteam-decision)]";
  }
  if (tone === "info") {
    return "text-[color:var(--superteam-info)]";
  }
  if (tone === "success") {
    return "text-[color:var(--superteam-success)]";
  }
  return "text-foreground";
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
