import type { ReactNode } from "react";
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
  IconTile,
  SignatureCard,
  SoftCard,
  StatusPill,
  V3EmptyState,
  V3ErrorState,
  V3LoadingState,
  V3MetricCard,
  V3Table,
  V3Td,
  V3Th,
  V3Tr,
  WorkSurface,
  type V3Tone,
} from "@/components/superteam";
import type {
  WorkflowInstanceStatus,
  WorkflowInstanceSummary,
} from "@/lib/api/projects";
import { workflowStatusLabel } from "../workflow-status";

type WorkflowEntranceProps = {
  instances: WorkflowInstanceSummary[];
  isError: boolean;
  isLoading: boolean;
};

export function WorkflowEntrance({
  instances,
  isError,
  isLoading,
}: WorkflowEntranceProps) {
  if (isError && instances.length === 0) {
    return (
      <SoftCard className="p-5">
        <V3ErrorState
          description="暂时无法读取流程编排入口，请稍后重试。"
          title="流程实例加载失败"
        />
      </SoftCard>
    );
  }

  if (isLoading && instances.length === 0) {
    return (
      <SoftCard>
        <V3LoadingState label="正在加载流程实例" />
      </SoftCard>
    );
  }

  if (instances.length === 0) {
    return (
      <SoftCard>
        <V3EmptyState
          description="有需求进入协调线程后，会在这里显示全局流程状态。"
          icon={<GitBranch />}
          title="暂无可见流程实例"
        />
      </SoftCard>
    );
  }

  const totals = workflowInstanceTotals(instances);

  return (
    <div className="flex min-w-0 flex-col gap-5">
      <div className="grid min-w-0 gap-4 xl:grid-cols-4">
        <SignatureCard className="flex min-h-44 flex-col justify-between gap-5 xl:col-span-1">
          <div>
            <p className="text-[13px] font-semibold text-white/80">流程实例</p>
            <p className="mt-2 text-4xl leading-none font-extrabold tabular-nums">
              {instances.length}
            </p>
            <p className="mt-3 max-w-sm text-sm text-white/80">
              个可见实例，进入单个实例查看编排画布和阶段详情。
            </p>
          </div>
          <div className="flex flex-wrap gap-2">
            {isLoading ? (
              <StatusPill className="bg-white/20 text-white" tone="info">
                同步中
              </StatusPill>
            ) : null}
            {isError ? (
              <StatusPill className="bg-white/20 text-white" tone="danger">
                刷新失败
              </StatusPill>
            ) : null}
          </div>
        </SignatureCard>
        <V3MetricCard
          icon={<PlayCircle />}
          iconTone="info"
          label="运行中"
          meta="正在推进的协调线程"
          value={totals.running}
        />
        <V3MetricCard
          icon={<UserCheck />}
          iconTone="warn"
          label="等待人工"
          loud={totals.waitingHuman > 0}
          meta="需要人类负责人处理"
          value={totals.waitingHuman}
        />
        <V3MetricCard
          icon={<AlertTriangle />}
          iconTone={totals.blocked > 0 ? "danger" : "mute"}
          label="阻断节点"
          loud={totals.blocked > 0}
          meta="当前实例内阻塞任务"
          value={totals.blocked}
        />
      </div>

      <WorkSurface>
        <div className="flex flex-col gap-3 border-b border-v3-line px-4 py-4 sm:flex-row sm:items-center sm:justify-between">
          <div className="min-w-0">
            <h2 className="text-[15px] font-bold text-v3-ink">流程实例</h2>
            <p className="mt-1 text-[13px] text-v3-ink-2">
              按最近更新时间展示实例状态、进度和人工/阻断计数。
            </p>
          </div>
          <div className="flex shrink-0 flex-wrap gap-2">
            {isLoading ? <StatusPill tone="info">同步中</StatusPill> : null}
            {isError ? <StatusPill tone="danger">刷新失败</StatusPill> : null}
          </div>
        </div>
        <V3Table aria-label="流程实例列表">
          <thead>
            <tr>
              <V3Th>流程</V3Th>
              <V3Th>状态</V3Th>
              <V3Th>进度</V3Th>
              <V3Th>人工 / 阻断</V3Th>
              <V3Th>最新状态</V3Th>
              <V3Th>更新</V3Th>
            </tr>
          </thead>
          <tbody>
            {instances.map((instance) => (
              <WorkflowInstanceRow
                instance={instance}
                key={instance.demand_id}
              />
            ))}
          </tbody>
        </V3Table>
      </WorkSurface>
    </div>
  );
}

function WorkflowInstanceRow({ instance }: { instance: WorkflowInstanceSummary }) {
  const completed = instance.progress.completed_nodes;
  const total = instance.progress.total_nodes;
  const progressMax = Math.max(total, 1);
  const progressNow = Math.min(completed, progressMax);
  const progressPercent =
    total > 0 ? Math.min(100, Math.round((completed / total) * 100)) : 0;
  const statusTone = workflowV3StatusTone(instance.status);
  const summary = workflowInstanceSummary(instance);
  const optionalBadges = workflowOptionalBadges(instance);

  return (
    <V3Tr tone={workflowRowTone(instance)}>
      <V3Td className="min-w-[280px]">
        <Link
          className="group flex min-w-0 items-start gap-3 rounded-v3-inner outline-none focus-visible:ring-2 focus-visible:ring-v3-brand/60"
          params={{ demandId: instance.demand_id }}
          to="/workflows/$demandId"
        >
          <IconTile tone={statusTone} size="sm">
            <GitBranch />
          </IconTile>
          <span className="min-w-0">
            <span className="line-clamp-2 text-[14px] leading-5 font-bold text-v3-ink group-hover:text-v3-brand-deep">
              {instance.title}
            </span>
            <span className="mt-1 block truncate text-xs text-v3-ink-2">
              {instance.project_name}
            </span>
            {optionalBadges.length > 0 ? (
              <span className="mt-2 flex flex-wrap gap-1.5">
                {optionalBadges.map((badge) => (
                  <StatusPill
                    key={badge.key}
                    showDot={false}
                    tone={badge.tone}
                  >
                    {badge.label}
                  </StatusPill>
                ))}
              </span>
            ) : null}
          </span>
        </Link>
      </V3Td>
      <V3Td>
        <StatusPill tone={statusTone}>
          {workflowStatusLabel(instance.status)}
        </StatusPill>
      </V3Td>
      <V3Td className="min-w-[170px]">
        <div className="flex flex-col gap-2">
          <div className="flex items-center justify-between gap-3 text-xs text-v3-ink-2">
            <span className="inline-flex items-center gap-1.5">
              <CheckCircle2 className="size-3.5 text-v3-ok" />
              已完成
            </span>
            <span className="font-semibold tabular-nums text-v3-ink">
              {completed}/{total}
            </span>
          </div>
          <div
            aria-label={`完成进度 ${completed}/${total}`}
            aria-valuemax={progressMax}
            aria-valuemin={0}
            aria-valuenow={progressNow}
            aria-valuetext={`已完成 ${completed}/${total}`}
            className="h-2 overflow-hidden rounded-full bg-v3-card-soft"
            role="progressbar"
          >
            <div
              className="h-full rounded-full bg-v3-brand transition-[width]"
              style={{ width: `${progressPercent}%` }}
            />
          </div>
        </div>
      </V3Td>
      <V3Td>
        <div className="grid min-w-[170px] grid-cols-3 gap-2">
          <Counter
            icon={<PlayCircle className="size-3.5" />}
            label="运行"
            tone="info"
            value={instance.progress.running_nodes}
          />
          <Counter
            icon={<UserCheck className="size-3.5" />}
            label="人工"
            tone="warn"
            value={instance.progress.waiting_human_nodes}
          />
          <Counter
            icon={<AlertTriangle className="size-3.5" />}
            label="阻断"
            tone={instance.progress.blocked_nodes > 0 ? "danger" : "mute"}
            value={instance.progress.blocked_nodes}
          />
        </div>
      </V3Td>
      <V3Td className="min-w-[240px]">
        <div className="flex items-start gap-2 text-[13px] leading-5 text-v3-ink-2">
          {summary.icon}
          <span className="line-clamp-2">{summary.text}</span>
        </div>
      </V3Td>
      <V3Td className="whitespace-nowrap text-xs tabular-nums text-v3-ink-2">
        <span className="inline-flex items-center gap-1.5">
          <Clock3 className="size-3.5" />
          {formatWorkflowTime(instance.updated_at)}
        </span>
      </V3Td>
    </V3Tr>
  );
}

function Counter({
  icon,
  label,
  tone,
  value,
}: {
  icon: ReactNode;
  label: string;
  tone: V3Tone;
  value: number;
}) {
  return (
    <div className="flex min-w-0 items-center justify-center gap-1 rounded-lg bg-v3-card-soft px-2 py-1.5 text-xs font-semibold text-v3-ink-2">
      <span className={counterToneClass[tone]}>{icon}</span>
      <span className="truncate">{label}</span>
      <span className="tabular-nums text-v3-ink">{value}</span>
    </div>
  );
}

function workflowInstanceTotals(instances: WorkflowInstanceSummary[]) {
  return instances.reduce(
    (totals, instance) => ({
      blocked: totals.blocked + instance.progress.blocked_nodes,
      running:
        totals.running +
        (instance.status === "running" ? 1 : 0),
      waitingHuman:
        totals.waitingHuman + instance.progress.waiting_human_nodes,
    }),
    { blocked: 0, running: 0, waitingHuman: 0 },
  );
}

function workflowV3StatusTone(status: WorkflowInstanceStatus): V3Tone {
  switch (status) {
    case "completed":
      return "ok";
    case "cancelled":
    case "failed":
      return "danger";
    case "planning":
    case "waiting_human":
      return "warn";
    case "running":
      return "info";
    case "unknown":
      return "mute";
    default:
      return status satisfies never;
  }
}

function workflowRowTone(
  instance: WorkflowInstanceSummary,
): "danger" | "warn" | undefined {
  if (
    instance.status === "failed" ||
    instance.status === "cancelled" ||
    instance.progress.blocked_nodes > 0
  ) {
    return "danger";
  }
  if (instance.status === "waiting_human") {
    return "warn";
  }
  return undefined;
}

function workflowInstanceSummary(instance: WorkflowInstanceSummary) {
  if (instance.current_blocker?.title) {
    return {
      icon: <AlertTriangle className="mt-0.5 size-4 shrink-0 text-v3-danger" />,
      text: `阻塞：${instance.current_blocker.title}`,
    };
  }

  if (instance.recent_event?.summary) {
    return {
      icon: <AlertCircle className="mt-0.5 size-4 shrink-0 text-v3-info" />,
      text: `最新事件：${instance.recent_event.summary}`,
    };
  }

  return {
    icon: <AlertCircle className="mt-0.5 size-4 shrink-0 text-v3-mute" />,
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
          tone: instance.sla.breached ? "danger" : "warn",
        }
      : undefined,
  ].filter(
    (badge): badge is { key: string; label: string; tone: V3Tone } =>
      Boolean(badge),
  );
}

function priorityTone(priority: string): V3Tone {
  const normalized = priority.toLowerCase();
  if (["p0", "p1", "critical", "urgent"].includes(normalized)) {
    return "danger";
  }
  if (["p2", "high"].includes(normalized)) {
    return "warn";
  }
  return "mute";
}

function riskTone(risk: string): V3Tone {
  const normalized = risk.toLowerCase();
  if (["critical", "high", "severe"].includes(normalized)) {
    return "danger";
  }
  if (["medium", "moderate"].includes(normalized)) {
    return "warn";
  }
  return "mute";
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

const counterToneClass: Record<V3Tone, string> = {
  artifact: "text-v3-artifact",
  brand: "text-v3-brand",
  danger: "text-v3-danger",
  info: "text-v3-info",
  mute: "text-v3-mute",
  ok: "text-v3-ok",
  warn: "text-v3-warn",
};
