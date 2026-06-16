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
import { selectInitialWorkflowNodeId, taskNodeId } from "../workflow-graph-adapter";
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
  const initialSelectedNodeId = useMemo(
    () => (graph?.nodes.length ? selectInitialWorkflowNodeId(graph) : undefined),
    [graph],
  );
  const [selectedNodeId, setSelectedNodeId] = useState<string | undefined>(
    initialSelectedNodeId,
  );
  const [isNodeDialogOpen, setIsNodeDialogOpen] = useState(false);

  useEffect(() => {
    setSelectedNodeId((current) => {
      if (!graph?.nodes.length) return undefined;
      if (current && graph.nodes.some((node) => taskNodeId(node.id) === current)) {
        return current;
      }

      return initialSelectedNodeId;
    });
  }, [graph, initialSelectedNodeId]);

  const selectedTask = useMemo(
    () => graph?.nodes.find((node) => taskNodeId(node.id) === selectedNodeId),
    [graph, selectedNodeId],
  );

  useEffect(() => {
    if (!selectedTask) {
      setIsNodeDialogOpen(false);
    }
  }, [selectedTask]);

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
          <div className="mt-4 min-w-0 p-4 pt-0">
            <WorkflowGraphCanvas
              graph={graph}
              onNodeOpen={(nodeId) => {
                setSelectedNodeId(nodeId);
                setIsNodeDialogOpen(true);
              }}
              onSelectedNodeChange={setSelectedNodeId}
              selectedNodeId={selectedNodeId}
            />
            <NodeDetailDialog
              graph={graph}
              open={isNodeDialogOpen}
              selectedTask={selectedTask}
              onOpenChange={setIsNodeDialogOpen}
            />
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

function NodeDetailDialog({
  graph,
  onOpenChange,
  open,
  selectedTask,
}: {
  graph: ProjectTaskGraph;
  onOpenChange: (open: boolean) => void;
  open: boolean;
  selectedTask: ProjectTaskGraph["nodes"][number] | undefined;
}) {
  return (
    <Dialog open={open && Boolean(selectedTask)} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[calc(100vh-4rem)] overflow-y-auto border-[color:var(--superteam-glass-border)] bg-[color:var(--superteam-glass-strong-bg)] sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle>节点详情</DialogTitle>
          <DialogDescription>
            查看当前流程节点的输入、输出、运行记录和人工决策。
          </DialogDescription>
        </DialogHeader>
        <WorkflowNodeInspector
          graph={graph}
          selectedTask={selectedTask}
          variant="dialog"
        />
      </DialogContent>
    </Dialog>
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
