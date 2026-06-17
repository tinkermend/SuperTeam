import { Link } from "@tanstack/react-router";
import {
  AlertCircle,
  AlertTriangle,
  CheckCircle2,
  Clock3,
  GitBranch,
  PlayCircle,
  UserCheck,
} from "lucide-react";
import {
  LiquidCard,
  SemanticIconTile,
  StatusBadge,
  type Tone,
} from "@/components/superteam";
import type { WorkflowInstanceSummary } from "@/lib/api/projects";
import { cn } from "@/lib/utils";
import { workflowStatusLabel, workflowStatusTone } from "../workflow-status";

type WorkflowInstanceCardProps = {
  instance: WorkflowInstanceSummary;
};

export function WorkflowInstanceCard({ instance }: WorkflowInstanceCardProps) {
  const completed = instance.progress.completed_nodes;
  const total = instance.progress.total_nodes;
  const progressPercent = total > 0 ? Math.min(100, Math.round((completed / total) * 100)) : 0;
  const progressMax = Math.max(total, 1);
  const progressNow = Math.min(completed, progressMax);
  const summary = workflowInstanceSummary(instance);
  const optionalBadges = workflowOptionalBadges(instance);

  return (
    <Link
      className="group block h-full rounded-xl outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2"
      params={{ demandId: instance.demand_id }}
      to="/workflows/$demandId"
    >
      <LiquidCard className="h-full gap-0 overflow-hidden rounded-xl p-0 transition duration-200 hover:-translate-y-0.5 hover:border-[color:var(--superteam-menu-accent)] hover:shadow-[var(--superteam-shadow-mid)]">
        <div className="flex h-full min-w-0 flex-col gap-4 p-5">
          <div className="flex min-w-0 items-start justify-between gap-3">
            <div className="flex min-w-0 items-start gap-3">
              <SemanticIconTile tone={workflowStatusTone(instance.status)} size="sm">
                <GitBranch />
              </SemanticIconTile>
              <div className="min-w-0">
                <h3 className="line-clamp-2 text-base font-semibold leading-6 tracking-normal text-foreground">
                  {instance.title}
                </h3>
                <p className="mt-1 truncate text-sm text-muted-foreground">
                  {instance.project_name}
                </p>
              </div>
            </div>
            <StatusBadge
              className="shrink-0"
              tone={workflowStatusTone(instance.status)}
            >
              {workflowStatusLabel(instance.status)}
            </StatusBadge>
          </div>

          {optionalBadges.length > 0 ? (
            <div className="flex flex-wrap gap-2">
              {optionalBadges.map((badge) => (
                <StatusBadge key={badge.key} showDot={false} tone={badge.tone}>
                  {badge.label}
                </StatusBadge>
              ))}
            </div>
          ) : null}

          <div className="rounded-lg border border-border/70 bg-white/65 p-3">
            <div className="flex items-start gap-2">
              {summary.icon}
              <p className="line-clamp-2 min-w-0 text-sm leading-6 text-muted-foreground">
                {summary.text}
              </p>
            </div>
          </div>

          <div className="mt-auto flex flex-col gap-3">
            <div className="flex flex-wrap items-center justify-between gap-2 text-xs text-muted-foreground">
              <span className="inline-flex items-center gap-1.5">
                <CheckCircle2 className="size-3.5 text-[color:var(--superteam-success)]" />
                已完成 {completed}/{total}
              </span>
              <span className="inline-flex items-center gap-1.5">
                <Clock3 className="size-3.5" />
                更新 {formatWorkflowTime(instance.updated_at)}
              </span>
            </div>

            <div
              aria-label={`完成进度 ${completed}/${total}`}
              aria-valuetext={`已完成 ${completed}/${total}`}
              className="h-2 overflow-hidden rounded-full bg-slate-100"
              role="progressbar"
              aria-valuemax={progressMax}
              aria-valuemin={0}
              aria-valuenow={progressNow}
            >
              <div
                className="h-full rounded-full bg-[color:var(--superteam-menu-accent)] transition-[width]"
                style={{ width: `${progressPercent}%` }}
              />
            </div>

            <div className="grid grid-cols-3 gap-2">
              <Counter
                icon={<PlayCircle className="size-3.5" />}
                label="运行"
                tone="info"
                value={instance.progress.running_nodes}
              />
              <Counter
                icon={<UserCheck className="size-3.5" />}
                label="人工"
                tone="decision"
                value={instance.progress.waiting_human_nodes}
              />
              <Counter
                icon={<AlertTriangle className="size-3.5" />}
                label="阻断"
                tone={instance.progress.blocked_nodes > 0 ? "danger" : "neutral"}
                value={instance.progress.blocked_nodes}
              />
            </div>
          </div>
        </div>
      </LiquidCard>
    </Link>
  );
}

function Counter({
  icon,
  label,
  tone,
  value,
}: {
  icon: React.ReactNode;
  label: string;
  tone: Tone;
  value: number;
}) {
  return (
    <div
      className={cn(
        "flex min-w-0 items-center justify-center gap-1.5 rounded-lg border border-current/15 bg-white/55 px-2 py-2 text-xs font-medium",
        counterToneClass[tone],
      )}
    >
      {icon}
      <span className="truncate">{label}</span>
      <span className="font-semibold tabular-nums">{value}</span>
    </div>
  );
}

function workflowInstanceSummary(instance: WorkflowInstanceSummary) {
  if (instance.current_blocker?.title) {
    return {
      icon: (
        <AlertTriangle className="mt-1 size-4 shrink-0 text-[color:var(--superteam-danger)]" />
      ),
      text: `阻塞：${instance.current_blocker.title}`,
    };
  }

  if (instance.recent_event?.summary) {
    return {
      icon: (
        <AlertCircle className="mt-1 size-4 shrink-0 text-[color:var(--superteam-info)]" />
      ),
      text: `最新事件：${instance.recent_event.summary}`,
    };
  }

  return {
    icon: <AlertCircle className="mt-1 size-4 shrink-0 text-[color:var(--superteam-neutral)]" />,
    text: instance.status_reason || "等待协调线程更新状态",
  };
}

function workflowOptionalBadges(instance: WorkflowInstanceSummary) {
  return [
    instance.priority
      ? {
          key: "priority",
          label: instance.priority.label,
          tone: priorityTone(instance.priority.value),
        }
      : undefined,
    instance.risk
      ? {
          key: "risk",
          label: instance.risk.label,
          tone: riskTone(instance.risk.level),
        }
      : undefined,
    instance.sla
      ? {
          key: "sla",
          label: instance.sla.label,
          tone: instance.sla.breached ? "danger" : "warning",
        }
      : undefined,
  ].filter((badge): badge is { key: string; label: string; tone: Tone } => Boolean(badge));
}

function priorityTone(priority: string): Tone {
  const normalized = priority.toLowerCase();
  if (["p0", "p1", "critical", "urgent"].includes(normalized)) {
    return "danger";
  }
  if (["p2", "high"].includes(normalized)) {
    return "warning";
  }
  return "neutral";
}

function riskTone(risk: string): Tone {
  const normalized = risk.toLowerCase();
  if (["critical", "high", "severe"].includes(normalized)) {
    return "danger";
  }
  if (["medium", "moderate"].includes(normalized)) {
    return "warning";
  }
  return "neutral";
}

function formatWorkflowTime(value: string): string {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return "未知";
  }

  return new Intl.DateTimeFormat("zh-CN", {
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    month: "2-digit",
  }).format(date);
}

const counterToneClass: Record<Tone, string> = {
  artifact: "text-[color:var(--superteam-artifact)]",
  danger: "text-[color:var(--superteam-danger)]",
  decision: "text-[color:var(--superteam-decision)]",
  info: "text-[color:var(--superteam-info)]",
  neutral: "text-[color:var(--superteam-neutral)]",
  primary: "text-primary",
  success: "text-[color:var(--superteam-success)]",
  warning: "text-[color:var(--superteam-warning)]",
};
