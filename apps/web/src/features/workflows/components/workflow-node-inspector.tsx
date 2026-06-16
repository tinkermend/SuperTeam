import type { ReactNode } from "react";
import { Link } from "@tanstack/react-router";
import { ExternalLink } from "lucide-react";
import { Button } from "@/components/ui/button";
import { LiquidCard, StatusBadge, type Tone } from "@/components/superteam";
import type { ProjectTaskGraph, ProjectTaskGraphNode } from "@/lib/api/projects";

type WorkflowNodeInspectorProps = {
  graph: ProjectTaskGraph;
  selectedTask: ProjectTaskGraphNode | undefined;
  variant?: "card" | "dialog";
};

export function WorkflowNodeInspector({
  graph,
  selectedTask,
  variant = "card",
}: WorkflowNodeInspectorProps) {
  if (!selectedTask) {
    if (variant === "dialog") {
      return (
        <div className="rounded-xl border bg-background/70 p-5 text-sm text-muted-foreground">
          选择节点查看详情
        </div>
      );
    }

    return (
      <LiquidCard className="flex min-h-[420px] items-center justify-center rounded-xl p-5 text-sm text-muted-foreground">
        选择节点查看详情
      </LiquidCard>
    );
  }

  const run = graph.runs.find((item) => item.project_task_id === selectedTask.id);
  const result = graph.execution_summaries.find(
    (item) => item.project_task_id === selectedTask.id,
  );
  const decisions = graph.decision_requests.filter(
    (item) => item.project_task_id === selectedTask.id,
  );

  const content = (
    <>
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0">
          <p className="text-xs font-medium text-muted-foreground">节点详情</p>
          <h3 className="mt-1 line-clamp-2 text-base font-semibold tracking-normal">
            {selectedTask.title}
          </h3>
        </div>
        <StatusBadge tone={taskStatusTone(selectedTask.status)}>
          {selectedTask.status}
        </StatusBadge>
      </div>

      <div className="mt-4 divide-y text-sm">
        <InspectorRow label="输入" value={formatValue(selectedTask.input_requirements)} />
        <InspectorRow label="输出" value={formatValue(selectedTask.expected_outputs)} />
        <InspectorRow
          action={
            run?.runtime_task_id ? (
              <Button asChild size="sm" variant="outline">
                <Link to="/runtime">
                  <ExternalLink className="size-3.5" />
                  Runtime
                </Link>
              </Button>
            ) : null
          }
          label="Run"
          value={
            run
              ? [run.status, run.provider_type, run.runtime_node_summary]
                  .filter(Boolean)
                  .join(" · ")
              : "暂无运行记录"
          }
        />
        <InspectorRow
          label="结果"
          value={result ? result.conclusion : "暂无执行结果"}
        />
        <InspectorRow
          action={
            decisions.length > 0 ? (
              <Button asChild size="sm" variant="outline">
                <Link to="/approvals">
                  <ExternalLink className="size-3.5" />
                  审批
                </Link>
              </Button>
            ) : null
          }
          label="人工决策"
          value={
            decisions.length > 0
              ? decisions.map((decision) => decision.status_snapshot).join(" · ")
              : "暂无人工决策"
          }
        />
      </div>
    </>
  );

  if (variant === "dialog") {
    return (
      <div className="rounded-xl border bg-background/70 p-4">
        {content}
      </div>
    );
  }

  return (
    <LiquidCard className="min-h-[420px] rounded-xl p-4">
      {content}
    </LiquidCard>
  );
}

export function taskStatusTone(status: string): Tone {
  const normalized = status.toLowerCase();

  if (["completed", "accepted", "approved", "done", "success"].includes(normalized)) {
    return "success";
  }
  if (["failed", "rejected", "cancelled", "blocked"].includes(normalized)) {
    return "danger";
  }
  if (
    ["pending", "waiting", "planning", "planning_pending", "planned", "waiting_human"].includes(
      normalized,
    )
  ) {
    return "warning";
  }
  if (["assigned", "dispatchable", "running", "in_progress"].includes(normalized)) {
    return "info";
  }

  return "neutral";
}

function InspectorRow({
  action,
  label,
  value,
}: {
  action?: ReactNode;
  label: string;
  value: string;
}) {
  return (
    <div className="grid gap-2 py-3">
      <div className="flex items-center justify-between gap-3">
        <span className="text-xs font-medium text-muted-foreground">{label}</span>
        {action}
      </div>
      <p className="min-w-0 break-words text-sm leading-6 text-foreground">{value}</p>
    </div>
  );
}

function formatValue(value: unknown): string {
  if (Array.isArray(value)) {
    if (value.length === 0) return "暂无";
    return value.map((item) => formatLeaf(item)).join(" · ");
  }

  if (value && typeof value === "object") {
    const entries = Object.entries(value);
    if (entries.length === 0) return "暂无";

    return entries
      .slice(0, 4)
      .map(([key, item]) => `${key}: ${formatLeaf(item)}`)
      .join(" · ");
  }

  return formatLeaf(value);
}

function formatLeaf(value: unknown): string {
  if (value === undefined || value === null || value === "") return "暂无";
  if (typeof value === "string") return value;
  if (typeof value === "number" || typeof value === "boolean") return String(value);

  try {
    return JSON.stringify(value);
  } catch {
    return "无法显示";
  }
}
