import type { Node, NodeProps } from "@xyflow/react";
import { Handle, Position } from "@xyflow/react";
import { AlertTriangle, ArrowRightCircle } from "lucide-react";
import { SoftCard, StatusPill } from "@/components/superteam";
import { cn } from "@/lib/utils";
import type { WorkflowBlockingNodeData } from "../workflow-graph-adapter";

export function WorkflowBlockingNode({
  data,
  selected,
}: NodeProps<Node<WorkflowBlockingNodeData, "workflowBlocking">>) {
  return (
    <SoftCard
      className={cn(
        "relative w-[360px] border border-v3-danger/25 p-4 shadow-v3-soft",
        selected && "border-v3-danger ring-2 ring-v3-danger/20",
      )}
      data-testid="workflow-blocking-node"
    >
      <Handle
        className="!size-2 !border-v3-danger/40 !bg-v3-danger"
        isConnectable={false}
        position={Position.Top}
        type="target"
      />
      <div className="flex items-start gap-3.5">
        <div className="flex size-12 shrink-0 items-center justify-center rounded-2xl bg-v3-danger/10 text-v3-danger ring-1 ring-v3-danger/20">
          <AlertTriangle className="size-6" />
        </div>
        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-center gap-2">
            <StatusPill tone="danger">已阻塞</StatusPill>
            <span className="truncate text-xs font-semibold text-v3-ink-3">
              {data.reasonCode}
            </span>
          </div>
          <p className="mt-2 line-clamp-3 text-[15px] font-bold leading-6 text-v3-ink">
            {data.message}
          </p>
        </div>
      </div>

      {data.recommendedAction ? (
        <div className="mt-4 flex items-start gap-2 rounded-xl border border-v3-line bg-v3-card-soft px-3 py-2.5 text-[13px] leading-5 text-v3-ink-2">
          <ArrowRightCircle className="mt-0.5 size-4 shrink-0 text-v3-danger" />
          <p className="min-w-0 break-words">
            <span className="font-bold text-v3-ink">下一步：</span>
            {data.recommendedAction}
          </p>
        </div>
      ) : null}
      <Handle
        className="!size-2 !border-v3-danger/40 !bg-v3-danger"
        isConnectable={false}
        position={Position.Bottom}
        type="source"
      />
    </SoftCard>
  );
}
