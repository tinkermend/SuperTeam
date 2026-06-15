import type { Node, NodeProps } from "@xyflow/react";
import { Handle, Position } from "@xyflow/react";
import { Bot, ShieldCheck } from "lucide-react";
import { StatusBadge } from "@/components/superteam";
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
    <div
      className={cn(
        "relative w-[300px] rounded-lg border bg-card/95 p-3 text-card-foreground shadow-sm backdrop-blur-sm",
        selected && "border-primary/70 ring-2 ring-primary/20",
      )}
    >
      <Handle
        className="!size-2 !border-primary/40 !bg-primary"
        isConnectable={false}
        position={Position.Top}
        type="target"
      />
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0">
          <h3 className="line-clamp-2 text-sm font-semibold tracking-normal">
            {data.title}
          </h3>
          <p className="mt-1 line-clamp-2 text-xs leading-5 text-muted-foreground">
            {data.summary || "暂无任务摘要"}
          </p>
        </div>
        <StatusBadge className="shrink-0" tone={taskStatusTone(data.status)}>
          {data.status}
        </StatusBadge>
      </div>

      <div className="mt-3 flex flex-wrap gap-2">
        <span className="inline-flex max-w-full items-center gap-1.5 rounded-full border bg-background/70 px-2.5 py-1 text-xs text-muted-foreground">
          <Bot className="size-3.5 shrink-0 text-[color:var(--superteam-info)]" />
          <span className="truncate">{data.employeeName || "未分配"}</span>
        </span>
        {showHumanApproval ? (
          <span className="inline-flex max-w-full items-center gap-1.5 rounded-full border bg-background/70 px-2.5 py-1 text-xs text-[color:var(--superteam-decision)]">
            <ShieldCheck className="size-3.5 shrink-0" />
            <span className="truncate">
              {data.hasPendingDecision ? "等待人工决策" : "需要人工审批"}
            </span>
          </span>
        ) : null}
      </div>
      <Handle
        className="!size-2 !border-primary/40 !bg-primary"
        isConnectable={false}
        position={Position.Bottom}
        type="source"
      />
    </div>
  );
}

export function WorkflowAttachmentNode({
  data,
  selected,
}: NodeProps<Node<WorkflowAttachmentNodeData, "workflowAttachment">>) {
  return (
    <div
      className={cn(
        "relative w-[220px] rounded-lg border border-[color:var(--superteam-decision)]/25 bg-background/90 p-3 text-xs shadow-sm backdrop-blur-sm",
        selected && "ring-2 ring-[color:var(--superteam-decision)]/25",
      )}
    >
      <Handle
        className="!size-2 !border-[color:var(--superteam-decision)]/40 !bg-[color:var(--superteam-decision)]"
        isConnectable={false}
        position={Position.Top}
        type="target"
      />
      <div className="flex items-start gap-2">
        <ShieldCheck className="mt-0.5 size-4 shrink-0 text-[color:var(--superteam-decision)]" />
        <div className="min-w-0">
          <p className="line-clamp-2 font-medium text-foreground">{data.title}</p>
          <StatusBadge className="mt-2" tone={taskStatusTone(data.status)}>
            {data.status}
          </StatusBadge>
        </div>
      </div>
      <Handle
        className="!size-2 !border-[color:var(--superteam-decision)]/40 !bg-[color:var(--superteam-decision)]"
        isConnectable={false}
        position={Position.Bottom}
        type="source"
      />
    </div>
  );
}
