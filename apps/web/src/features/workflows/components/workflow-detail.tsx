import { Link } from "@tanstack/react-router";
import { FolderKanban, ListChecks } from "lucide-react";
import { Button } from "@/components/ui/button";
import { LiquidCard, SemanticIconTile, StatusBadge, type Tone } from "@/components/superteam";
import type {
  ProjectDemandLaunchDetail,
  ProjectTaskGraph,
  ProjectTaskGraphNode,
  WorkflowInstanceSummary,
} from "@/lib/api/projects";
import { workflowStatusLabel, workflowStatusTone } from "../workflow-status";

type WorkflowDetailProps = {
  detail?: ProjectDemandLaunchDetail;
  graph?: ProjectTaskGraph;
  instance?: WorkflowInstanceSummary;
  isError?: boolean;
};

export function WorkflowDetail({
  detail,
  graph,
  instance,
  isError,
}: WorkflowDetailProps) {
  if (isError) {
    return (
      <LiquidCard className="rounded-xl p-6 text-sm text-destructive">
        流程详情加载失败
      </LiquidCard>
    );
  }

  if (!detail || !instance) {
    return (
      <LiquidCard className="rounded-xl p-6 text-sm text-muted-foreground">
        正在加载流程详情
      </LiquidCard>
    );
  }

  const nodes = graph?.nodes ?? [];
  const isGraphReady = nodes.length > 0;
  const orchestrationTitle = isGraphReady
    ? "流程图已就绪"
    : detail.coordination_jobs.length === 0
      ? "等待项目协调线程接收"
      : "任务正在规划";

  return (
    <div className="grid items-start gap-4 lg:grid-cols-[320px_minmax(0,1fr)]">
      <LiquidCard className="rounded-xl p-4">
        <div className="flex items-start justify-between gap-3">
          <div className="min-w-0">
            <p className="text-xs font-medium text-muted-foreground">需求摘要</p>
            <h2 className="mt-1 line-clamp-2 text-lg font-semibold tracking-normal">
              {detail.demand.title}
            </h2>
          </div>
          <StatusBadge tone={workflowStatusTone(instance.status)}>
            {workflowStatusLabel(instance.status)}
          </StatusBadge>
        </div>

        <p className="mt-3 line-clamp-5 text-sm leading-6 text-muted-foreground">
          {detail.demand.content || "暂无需求正文"}
        </p>

        <div className="mt-4 grid gap-2 border-y py-3 text-sm">
          <SummaryRow label="项目" value={detail.project.name} />
          <SummaryRow label="流程" value={instance.demand_id} />
          <SummaryRow label="进度" value={`${instance.progress.completed_nodes}/${instance.progress.total_nodes} 已完成`} />
        </div>

        <Button asChild className="mt-4 w-full justify-start" variant="outline">
          <Link params={{ projectId: detail.project.id }} to="/projects/$projectId">
            <FolderKanban className="size-4" />
            打开项目
          </Link>
        </Button>
      </LiquidCard>

      <LiquidCard className="rounded-xl">
        <div className="flex items-start justify-between gap-3 border-b p-4">
          <div className="flex min-w-0 items-center gap-3">
            <SemanticIconTile tone={isGraphReady ? "info" : "warning"} size="sm">
              <ListChecks />
            </SemanticIconTile>
            <div className="min-w-0">
              <h2 className="text-base font-semibold tracking-normal">
                {orchestrationTitle}
              </h2>
              <p className="mt-1 text-xs text-muted-foreground">
                {isGraphReady
                  ? `${nodes.length} 个任务节点已从真实 task graph 读取`
                  : "等待 Control Plane 继续写入协调事实"}
              </p>
            </div>
          </div>
          <StatusBadge tone={workflowStatusTone(instance.status)}>
            {workflowStatusLabel(instance.status)}
          </StatusBadge>
        </div>

        {isGraphReady ? (
          <div className="divide-y">
            {nodes.slice(0, 4).map((node) => (
              <TaskNodeRow key={node.id} node={node} />
            ))}
          </div>
        ) : (
          <div className="p-5 text-sm leading-6 text-muted-foreground">
            当前需求已进入流程实例，任务节点会在协调线程规划完成后显示。
          </div>
        )}
      </LiquidCard>
    </div>
  );
}

function SummaryRow({ label, value }: { label: string; value: string }) {
  return (
    <div className="grid grid-cols-[56px_minmax(0,1fr)] gap-3">
      <span className="text-muted-foreground">{label}</span>
      <span className="min-w-0 truncate font-medium text-foreground">{value}</span>
    </div>
  );
}

function TaskNodeRow({ node }: { node: ProjectTaskGraphNode }) {
  return (
    <div className="flex min-w-0 items-start justify-between gap-3 p-4">
      <div className="min-w-0">
        <p className="line-clamp-2 text-sm font-medium">{node.title}</p>
        {node.summary ? (
          <p className="mt-1 line-clamp-2 text-xs text-muted-foreground">{node.summary}</p>
        ) : null}
      </div>
      <StatusBadge tone={taskStatusTone(node.status)}>{node.status}</StatusBadge>
    </div>
  );
}

function taskStatusTone(status: string): Tone {
  if (["completed", "accepted", "approved", "done", "success"].includes(status)) {
    return "success";
  }
  if (["failed", "rejected", "cancelled", "blocked"].includes(status)) {
    return "danger";
  }
  if (["pending", "waiting", "planning", "planning_pending", "waiting_human"].includes(status)) {
    return "warning";
  }
  if (["dispatchable", "running", "in_progress"].includes(status)) {
    return "info";
  }

  return "neutral";
}
