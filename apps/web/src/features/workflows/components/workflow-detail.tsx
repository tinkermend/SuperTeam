import { useEffect, useMemo, useState } from "react";
import { Link } from "@tanstack/react-router";
import { FolderKanban, ListChecks } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { LiquidCard, SemanticIconTile, StatusBadge } from "@/components/superteam";
import type {
  ProjectDemandLaunchDetail,
  ProjectTaskGraph,
  WorkflowInstanceSummary,
} from "@/lib/api/projects";
import { taskNodeId } from "../workflow-graph-adapter";
import { workflowStatusLabel, workflowStatusTone } from "../workflow-status";
import { WorkflowGraphCanvas } from "./workflow-graph-canvas";
import { WorkflowNodeInspector } from "./workflow-node-inspector";

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
  const [selectedNodeId, setSelectedNodeId] = useState<string | undefined>();

  useEffect(() => {
    setSelectedNodeId((current) => {
      if (!graph?.nodes.length) return undefined;
      if (current && graph.nodes.some((node) => taskNodeId(node.id) === current)) {
        return current;
      }

      return undefined;
    });
  }, [graph]);

  const selectedTask = useMemo(
    () => graph?.nodes.find((node) => taskNodeId(node.id) === selectedNodeId),
    [graph, selectedNodeId],
  );

  const isInspectorOpen = Boolean(selectedNodeId && selectedTask);

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
    <div className="min-w-0">
      <LiquidCard className="@container/workflow-graph min-w-0 overflow-hidden rounded-xl">
        <DemandSummaryBar detail={detail} instance={instance} />

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

        {isGraphReady && graph ? (
          <div className="min-w-0 p-4">
            <WorkflowGraphCanvas
              graph={graph}
              onNodeOpen={setSelectedNodeId}
              onSelectedNodeChange={setSelectedNodeId}
              selectedNodeId={selectedNodeId}
            />
          </div>
        ) : (
          <div className="p-5 text-sm leading-6 text-muted-foreground">
            当前需求已进入流程实例，任务节点会在协调线程规划完成后显示。
          </div>
        )}
      </LiquidCard>

      {isGraphReady && graph ? (
        <Dialog
          onOpenChange={(open) => {
            if (!open) setSelectedNodeId(undefined);
          }}
          open={isInspectorOpen}
        >
          <DialogContent className="flex max-h-[85vh] w-full flex-col gap-0 p-0 sm:max-w-xl">
            <DialogHeader className="shrink-0 border-b px-5 py-4">
              <DialogTitle className="text-base font-semibold tracking-normal">
                节点详情
              </DialogTitle>
              <DialogDescription className="text-xs">
                查看任务节点的负责人、阻塞、输入输出与执行结果
              </DialogDescription>
            </DialogHeader>
            <div className="min-h-0 overflow-y-auto px-5 py-4">
              <WorkflowNodeInspector
                graph={graph}
                selectedTask={selectedTask}
                variant="dialog"
              />
            </div>
          </DialogContent>
        </Dialog>
      ) : null}
    </div>
  );
}

function DemandSummaryBar({
  detail,
  instance,
}: {
  detail: ProjectDemandLaunchDetail;
  instance: WorkflowInstanceSummary;
}) {
  return (
    <div className="border-b bg-white/75 px-4 py-3 backdrop-blur">
      <div className="flex flex-col gap-3 @lg/workflow-graph:flex-row @lg/workflow-graph:items-start @lg/workflow-graph:justify-between">
        <div className="min-w-0">
          <div className="flex flex-wrap items-center gap-x-3 gap-y-1">
            <p className="text-xs font-medium text-muted-foreground">需求摘要</p>
            <p className="truncate text-xs text-muted-foreground">
              {detail.project.name}
            </p>
          </div>
          <h2 className="mt-1 line-clamp-2 text-base font-semibold leading-6 tracking-normal">
            {detail.demand.title}
          </h2>
          <p className="mt-2 line-clamp-2 text-sm leading-6 text-muted-foreground">
            {detail.demand.content || "暂无需求正文"}
          </p>
        </div>

        <div className="flex shrink-0 flex-wrap items-center gap-2 @lg/workflow-graph:justify-end">
          <StatusBadge tone={workflowStatusTone(instance.status)}>
            {workflowStatusLabel(instance.status)}
          </StatusBadge>
          <StatusBadge tone="neutral">
            已完成 {instance.progress.completed_nodes}/{instance.progress.total_nodes}
          </StatusBadge>
          <StatusBadge tone={instance.progress.running_nodes > 0 ? "info" : "neutral"}>
            运行中 {instance.progress.running_nodes}
          </StatusBadge>
          <StatusBadge
            tone={instance.progress.waiting_human_nodes > 0 ? "warning" : "neutral"}
          >
            等待人工 {instance.progress.waiting_human_nodes}
          </StatusBadge>
          <StatusBadge tone={instance.progress.blocked_nodes > 0 ? "danger" : "neutral"}>
            阻塞 {instance.progress.blocked_nodes}
          </StatusBadge>
          <Button asChild size="sm" variant="outline">
            <Link params={{ projectId: detail.project.id }} to="/projects/$projectId">
              <FolderKanban className="size-4" />
              打开项目
            </Link>
          </Button>
        </div>
      </div>
    </div>
  );
}
