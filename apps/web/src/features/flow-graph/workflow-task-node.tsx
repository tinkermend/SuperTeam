import { useEffect, useState } from "react";
import type { Node, NodeProps } from "@xyflow/react";
import { Handle, Position } from "@xyflow/react";
import { Bot, GitBranch, ShieldCheck, Timer } from "lucide-react";
import { Avatar, AvatarFallback, AvatarImage } from "@/components/ui/avatar";
import { SoftCard, StatusPill } from "@/components/superteam";
import { riskLevelLabel, runStatusLabel, taskStatusLabel } from "@/lib/status-labels";
import {
  formatDateTime,
  formatElapsedSince,
  formatRunDuration,
} from "@/lib/format-time";
import { cn } from "@/lib/utils";
import type {
  WorkflowAttachmentNodeData,
  WorkflowStageLabelNodeData,
  WorkflowTaskNodeData
} from "./flow-graph-adapter";
import { taskStatusTone } from "./inspector-primitives";

const EMPLOYEE_ROLE_LABELS: Record<string, string> = {
  backend_engineer: "后端开发",
  database_admin: "数据库管理",
  devops_engineer: "DevOps 运维",
  frontend_engineer: "前端开发",
  fullstack_engineer: "全栈开发",
  general_engineer: "通用工程执行",
  implementation_engineer: "实施工程师",
  qa_engineer: "测试工程",
  security_engineer: "安全工程"
};

export function WorkflowTaskNode({
  data,
  selected
}: NodeProps<Node<WorkflowTaskNodeData, "workflowTask">>) {
  const showHumanApproval = data.requiresHumanApproval || data.hasPendingDecision;
  const employeeRole = employeeRoleLabel(data.employeeRole);

  return (
    <SoftCard
      className={cn(
        "relative w-[360px] border border-line p-4 shadow-soft",
        selected && "border-brand ring-2 ring-brand/20",
      )}
    >
      <Handle
        className="!size-2 !border-brand/40 !bg-brand"
        isConnectable={false}
        position={Position.Top}
        type="target"
      />
      <div className="flex items-start gap-3.5">
        <Avatar className="size-14 shrink-0 border-2 border-white shadow-soft ring-1 ring-line">
          {data.avatarAsset ? (
            <AvatarImage src={data.avatarAsset.thumbnailUrl} alt={data.employeeName || "数字员工"} />
          ) : null}
          <AvatarFallback className="bg-brand-soft text-brand">
            <Bot className="size-6" />
          </AvatarFallback>
        </Avatar>
        <div className="min-w-0 flex-1">
          <p className="truncate text-sm font-bold text-ink">
            {data.employeeName || "未分配"}
          </p>
          {employeeRole ? (
            <p className="mt-1 truncate text-xs text-ink-3">{employeeRole}</p>
          ) : null}
        </div>
        <StatusPill className="shrink-0" tone={taskStatusTone(data.status)}>
          {taskStatusLabel(data.status)}
        </StatusPill>
      </div>

      <div className="mt-3">
        <p className="line-clamp-2 text-[15px] font-bold leading-6 tracking-normal text-ink">
          {data.title}
        </p>
        <p className="mt-1.5 line-clamp-3 text-[13px] leading-5 text-ink-2">
          {data.summary || "暂无任务摘要"}
        </p>
      </div>

      <div className="mt-4 flex flex-wrap gap-2">
        {data.riskLevel && nodeRiskTone(data.riskLevel) ? (
          <StatusPill
            className="max-w-full"
            showDot={false}
            tone={nodeRiskTone(data.riskLevel)}
          >
            <ShieldCheck className="size-3.5 shrink-0" />
            <span className="truncate">风险 {riskLevelLabel(data.riskLevel)}</span>
          </StatusPill>
        ) : null}
        {data.runStatus ? (
          <StatusPill className="max-w-full" showDot={false} tone="info">
            <Bot className="size-3.5 shrink-0" />
            <span className="truncate">运行 {runStatusLabel(data.runStatus)}</span>
          </StatusPill>
        ) : null}
        {showHumanApproval ? (
          <StatusPill className="max-w-full" showDot={false} tone="warn">
            <ShieldCheck className="size-3.5 shrink-0" />
            <span className="truncate">
              {data.hasPendingDecision ? "等待人工决策" : "需要人工审批"}
            </span>
          </StatusPill>
        ) : null}
      </div>
      {data.showTiming ? <TaskNodeTiming data={data} /> : null}
      <Handle
        className="!size-2 !border-brand/40 !bg-brand"
        isConnectable={false}
        position={Position.Bottom}
        type="source"
      />
    </SoftCard>
  );
}

const RUNNING_NODE_STATUSES = new Set(["running", "in_progress"]);
/** 运行中滚动时长的局部刷新粒度（分钟级显示，30s tick 足够，不整图重渲）。 */
const RUNNING_ELAPSED_TICK_MS = 30_000;

/**
 * live 模式节点卡时间区（spec 2026-07-27 §1.2）：已结束显示起止+耗时；
 * 运行中显示"已运行 X 分"，由 5s 轮询数据刷新驱动，另加单节点局部 tick
 * 保证轮询数据未变时时长仍推进。无时间数据不渲染。
 */
function TaskNodeTiming({ data }: { data: WorkflowTaskNodeData }) {
  const startedAt = data.runStartedAt;
  const finishedAt = data.runFinishedAt;
  const isRunning =
    !finishedAt &&
    (RUNNING_NODE_STATUSES.has(data.status.toLowerCase()) ||
      RUNNING_NODE_STATUSES.has((data.runStatus ?? "").toLowerCase()));
  const [nowMs, setNowMs] = useState(() => Date.now());

  useEffect(() => {
    if (!isRunning || !startedAt) return;
    const timer = window.setInterval(
      () => setNowMs(Date.now()),
      RUNNING_ELAPSED_TICK_MS,
    );
    return () => window.clearInterval(timer);
  }, [isRunning, startedAt]);

  if (!startedAt) return null;

  const elapsed = isRunning ? formatElapsedSince(startedAt, nowMs) : undefined;
  const duration = formatRunDuration(startedAt, finishedAt);

  return (
    <p
      className="mt-3 flex flex-wrap items-center gap-x-2 gap-y-0.5 border-t border-line pt-2.5 text-[11.5px] tabular-nums leading-4 text-ink-2"
      data-testid="task-node-timing"
    >
      <Timer aria-hidden className="size-3.5 shrink-0 text-ink-3" />
      <span>
        <span className="text-ink-3">起</span>{" "}
        <time dateTime={startedAt}>{formatDateTime(startedAt)}</time>
      </span>
      {finishedAt ? (
        <span>
          <span className="text-ink-3">止</span>{" "}
          <time dateTime={finishedAt}>{formatDateTime(finishedAt)}</time>
        </span>
      ) : null}
      {duration ? (
        <span>
          <span className="text-ink-3">耗时</span> {duration}
        </span>
      ) : null}
      {elapsed ? <span className="font-semibold text-brand-deep">{elapsed}</span> : null}
    </p>
  );
}

function employeeRoleLabel(role: string | undefined): string | undefined {
  if (!role) return undefined;
  const normalized = role.trim().toLowerCase();
  return EMPLOYEE_ROLE_LABELS[normalized] ?? role;
}

/** 风险 pill 按级别取语义色；低风险无信息量，不渲染。 */
function nodeRiskTone(level: string): "danger" | "warn" | undefined {
  const normalized = level.trim().toLowerCase();
  if (["critical", "high", "severe", "高", "高风险"].includes(normalized)) {
    return "danger";
  }
  if (["medium", "moderate", "中", "中风险"].includes(normalized)) {
    return "warn";
  }
  return undefined;
}

export function WorkflowAttachmentNode({
  data,
  selected
}: NodeProps<Node<WorkflowAttachmentNodeData, "workflowAttachment">>) {
  return (
    <SoftCard
      className={cn(
        "relative w-[220px] border border-line p-3 text-xs",
        selected && "border-brand ring-2 ring-brand/20",
      )}
    >
      <Handle
        className="!size-2 !border-brand/40 !bg-brand"
        isConnectable={false}
        position={Position.Top}
        type="target"
      />
      <div className="flex items-start gap-2">
        <ShieldCheck className="mt-0.5 size-4 shrink-0 text-ink-3" />
        <div className="min-w-0">
          <p className="line-clamp-2 font-semibold text-ink">{data.title}</p>
          <StatusPill className="mt-2" tone={taskStatusTone(data.status)}>
            {taskStatusLabel(data.status)}
          </StatusPill>
        </div>
      </div>
      <Handle
        className="!size-2 !border-brand/40 !bg-brand"
        isConnectable={false}
        position={Position.Bottom}
        type="source"
      />
    </SoftCard>
  );
}

export function WorkflowStageLabelNode({
  data
}: NodeProps<Node<WorkflowStageLabelNodeData, "workflowStageLabel">>) {
  return (
    <SoftCard
      className="flex w-[168px] items-center justify-center rounded-full border border-line bg-card/92 px-3 py-1.5 shadow-sm"
      data-testid="workflow-stage-label"
    >
      <GitBranch className="mr-1.5 size-3.5 shrink-0 text-ink-3" />
      <div className="min-w-0 text-center">
        <p className="truncate text-[11px] font-bold leading-4 text-ink">{data.title}</p>
        <p className="truncate text-[10px] leading-3 text-ink-3">
          {`${data.taskCount} 任务 · ${data.employeeCount} 人`}
        </p>
      </div>
    </SoftCard>
  );
}
