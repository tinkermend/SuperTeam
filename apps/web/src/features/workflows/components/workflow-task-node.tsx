import type { Node, NodeProps } from "@xyflow/react";
import { Handle, Position } from "@xyflow/react";
import { Bot, ShieldCheck } from "lucide-react";
import { SoftCard, StatusPill } from "@/components/superteam";
import { cn } from "@/lib/utils";
import type {
  WorkflowAttachmentNodeData,
  WorkflowTaskNodeData,
} from "../workflow-graph-adapter";
import { taskStatusTone } from "./workflow-node-inspector";

export function WorkflowTaskNode({
  data,
  selected,
}: NodeProps<Node<WorkflowTaskNodeData, "workflowTask">>) {
  const showHumanApproval = data.requiresHumanApproval || data.hasPendingDecision;

  return (
    <SoftCard
      className={cn(
        "relative w-[300px] border border-v3-line p-3",
        selected && "border-v3-brand ring-2 ring-v3-brand/20",
      )}
    >
      <Handle
        className="!size-2 !border-v3-brand/40 !bg-v3-brand"
        isConnectable={false}
        position={Position.Top}
        type="target"
      />
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0">
          <p className="line-clamp-2 text-sm font-bold tracking-normal text-v3-ink">
            {data.title}
          </p>
          <p className="mt-1 line-clamp-2 text-xs leading-5 text-v3-ink-2">
            {data.summary || "暂无任务摘要"}
          </p>
        </div>
        <StatusPill className="shrink-0" tone={taskStatusTone(data.status)}>
          {data.status}
        </StatusPill>
      </div>

      <div className="mt-3 flex flex-wrap gap-2">
        <StatusPill className="max-w-full" showDot={false} tone="mute">
          <Bot className="size-3.5 shrink-0" />
          <span className="truncate">{data.employeeName || "未分配"}</span>
        </StatusPill>
        {data.riskLevel ? (
          <StatusPill className="max-w-full" showDot={false} tone="danger">
            <ShieldCheck className="size-3.5 shrink-0" />
            <span className="truncate">风险 {data.riskLevel}</span>
          </StatusPill>
        ) : null}
        {data.runStatus ? (
          <StatusPill className="max-w-full" showDot={false} tone="info">
            <Bot className="size-3.5 shrink-0" />
            <span className="truncate">Run {data.runStatus}</span>
          </StatusPill>
        ) : null}
        {showHumanApproval ? (
          <StatusPill className="max-w-full" showDot={false} tone="ok">
            <ShieldCheck className="size-3.5 shrink-0" />
            <span className="truncate">
              {data.hasPendingDecision ? "等待人工决策" : "需要人工审批"}
            </span>
          </StatusPill>
        ) : null}
      </div>
      <Handle
        className="!size-2 !border-v3-brand/40 !bg-v3-brand"
        isConnectable={false}
        position={Position.Bottom}
        type="source"
      />
    </SoftCard>
  );
}

export function WorkflowAttachmentNode({
  data,
  selected,
}: NodeProps<Node<WorkflowAttachmentNodeData, "workflowAttachment">>) {
  return (
    <SoftCard
      className={cn(
        "relative w-[220px] border border-v3-ok/25 p-3 text-xs",
        selected && "ring-2 ring-v3-ok/25",
      )}
    >
      <Handle
        className="!size-2 !border-v3-ok/40 !bg-v3-ok"
        isConnectable={false}
        position={Position.Top}
        type="target"
      />
      <div className="flex items-start gap-2">
        <ShieldCheck className="mt-0.5 size-4 shrink-0 text-v3-ok" />
        <div className="min-w-0">
          <p className="line-clamp-2 font-semibold text-v3-ink">{data.title}</p>
          <StatusPill className="mt-2" tone={taskStatusTone(data.status)}>
            {data.status}
          </StatusPill>
        </div>
      </div>
      <Handle
        className="!size-2 !border-v3-ok/40 !bg-v3-ok"
        isConnectable={false}
        position={Position.Bottom}
        type="source"
      />
    </SoftCard>
  );
}
