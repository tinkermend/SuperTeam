import { useEffect, useMemo, useState } from "react";
import { Link } from "@tanstack/react-router";
import { FolderKanban, ListChecks } from "lucide-react";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  IconTile,
  SoftCard,
  StatusPill,
  V3Button,
  V3ErrorState,
  V3LoadingState,
} from "@/components/superteam";
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
      <SoftCard className="p-4">
        <V3ErrorState title="流程详情加载失败" />
      </SoftCard>
    );
  }

  if (!detail || !instance) {
    return (
      <SoftCard>
        <V3LoadingState label="正在加载流程详情" />
      </SoftCard>
    );
  }

  const nodes = graph?.nodes ?? [];
  const blockingFacts = graph?.blocking_facts ?? [];
  const hasBlockingFacts = blockingFacts.length > 0;
  const hasGraphContent = nodes.length > 0 || hasBlockingFacts;
  const orchestrationTitle = nodes.length > 0
    ? "流程图已就绪"
    : hasBlockingFacts
      ? "协调已阻塞"
    : detail.coordination_jobs.length === 0
      ? "等待项目协调线程接收"
      : "任务正在规划";

  return (
    <div className="min-w-0">
      <SoftCard className="@container/workflow-graph min-w-0 overflow-hidden">
        <DemandSummaryBar detail={detail} instance={instance} />

        <div className="flex items-start justify-between gap-3 border-b border-v3-line p-4">
          <div className="flex min-w-0 items-center gap-3">
            <IconTile tone="brand" size="sm">
              <ListChecks />
            </IconTile>
            <div className="min-w-0">
              <h2 className="text-base font-bold tracking-normal text-v3-ink">
                {orchestrationTitle}
              </h2>
              <p className="mt-1 text-xs text-v3-ink-2">
                {nodes.length > 0
                  ? `${nodes.length} 个任务节点已从真实 task graph 读取`
                  : hasBlockingFacts
                    ? "协调线程已写入阻塞事实，等待处理后继续规划"
                  : "等待 Control Plane 继续写入协调事实"}
              </p>
            </div>
          </div>
        </div>

        {hasGraphContent && graph ? (
          <div className="min-w-0 p-4">
            <WorkflowGraphCanvas
              graph={graph}
              onNodeOpen={setSelectedNodeId}
              onSelectedNodeChange={setSelectedNodeId}
              selectedNodeId={selectedNodeId}
            />
          </div>
        ) : (
          <div className="p-5 text-sm leading-6 text-v3-ink-2">
            当前需求已进入流程实例，任务节点会在协调线程规划完成后显示。
          </div>
        )}
      </SoftCard>

      {nodes.length > 0 && graph ? (
        <Dialog
          onOpenChange={(open) => {
            if (!open) setSelectedNodeId(undefined);
          }}
          open={isInspectorOpen}
        >
          <DialogContent className="flex max-h-[85vh] w-full flex-col gap-0 p-0 sm:max-w-xl">
            <DialogHeader className="shrink-0 border-b border-v3-line px-5 py-4">
              <DialogTitle className="text-base font-bold tracking-normal text-v3-ink">
                节点详情
              </DialogTitle>
              <DialogDescription className="text-xs text-v3-ink-2">
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
    <div className="border-b border-v3-line bg-v3-card px-4 py-3">
      <div className="flex flex-col gap-3 @lg/workflow-graph:flex-row @lg/workflow-graph:items-start @lg/workflow-graph:justify-between">
        <div className="min-w-0">
          <div className="flex flex-wrap items-center gap-x-3 gap-y-1">
            <p className="text-xs font-bold text-v3-ink-3">需求摘要</p>
            <p className="truncate text-xs text-v3-ink-2">
              {detail.project.name}
            </p>
          </div>
          <h2 className="mt-1 line-clamp-2 text-base font-bold leading-6 tracking-normal text-v3-ink">
            {detail.demand.title}
          </h2>
          <p className="mt-2 line-clamp-2 text-sm leading-6 text-v3-ink-2">
            {detail.demand.content || "暂无需求正文"}
          </p>
        </div>

        <div className="flex shrink-0 flex-wrap items-center gap-2 @lg/workflow-graph:justify-end">
          <StatusPill tone={workflowStatusTone(instance.status)}>
            {workflowStatusLabel(instance.status)}
          </StatusPill>
          <StatusPill showDot={false} tone="mute">
            已完成 {instance.progress.completed_nodes}/{instance.progress.total_nodes}
          </StatusPill>
          {instance.progress.running_nodes > 0 ? (
            <StatusPill tone="info">运行中 {instance.progress.running_nodes}</StatusPill>
          ) : null}
          {instance.progress.waiting_human_nodes > 0 ? (
            <StatusPill tone="warn">
              等待人工 {instance.progress.waiting_human_nodes}
            </StatusPill>
          ) : null}
          {instance.progress.blocked_nodes > 0 ? (
            <StatusPill tone="danger">阻塞 {instance.progress.blocked_nodes}</StatusPill>
          ) : null}
          <V3Button asChild size="sm" variant="outline">
            <Link params={{ projectId: detail.project.id }} to="/projects/$projectId">
              <FolderKanban className="size-4" />
              打开项目
            </Link>
          </V3Button>
        </div>
      </div>
    </div>
  );
}
