import { lazy, Suspense, useEffect, useMemo, useState } from "react";
import { Link } from "@tanstack/react-router";
import { FolderKanban, ListChecks } from "lucide-react";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle
} from "@/components/ui/dialog";
import {
  IconTile,
  SoftCard,
  StatusPill,
  Button,
  ErrorState,
  LoadingState
} from "@/components/superteam";
import type {
  ProjectDecisionRequest,
  ProjectDemandLaunchDetail,
  ProjectTaskGraph,
  WorkflowInstanceSummary
} from "@/lib/api/projects";
import { decisionTypeLabel, riskLevelLabel } from "@/lib/status-labels";
import { taskNodeId } from "../workflow-graph-adapter";
import { workflowStatusLabel, workflowStatusTone } from "../workflow-status";
import { WorkflowNodeInspector } from "./workflow-node-inspector";

// 流程图画布依赖 @xyflow/react（重）。懒加载让它离开入口包——图只在有节点内容的
// 流程详情才渲染，首屏不需要（P1-D Step 1）。
const WorkflowGraphCanvas = lazy(() =>
  import("./workflow-graph-canvas").then((m) => ({ default: m.WorkflowGraphCanvas })),
);

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
  isError
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
        <ErrorState title="流程详情加载失败" />
      </SoftCard>
    );
  }

  if (!detail) {
    return (
      <SoftCard>
        <LoadingState label="正在加载流程详情" />
      </SoftCard>
    );
  }

  const nodes = graph?.nodes ?? [];
  const blockingFacts = graph?.blocking_facts ?? [];
  const hasBlockingFacts = blockingFacts.length > 0;
  const hasGraphContent = nodes.length > 0 || hasBlockingFacts;
  // 空图且有待处理人工决策（如计划版本 pending_review）时，必须如实呈现"等待人工决策"，
  // 不能用"任务正在规划"误导——此时协调线程挂起等审批，不批准永远不会分解出任务节点。
  const pendingDecision = detail.decision_requests.find(
    (decision) => decision.status_snapshot === "pending",
  );
  const orchestrationTitle = nodes.length > 0
    ? "流程图已就绪"
    : hasBlockingFacts
      ? "协调已阻塞"
      : pendingDecision
        ? pendingDecision.decision_type === "plan_review"
          ? "计划版本待确认"
          : "等待人工决策"
        : detail.coordination_jobs.length === 0
          ? "等待项目协调线程接收"
          : "任务正在规划";
  const orchestrationSubtitle = nodes.length > 0
    ? `${nodes.length} 个任务节点已从真实 task graph 读取`
    : hasBlockingFacts
      ? "协调线程已写入阻塞事实，等待处理后继续规划"
      : pendingDecision
        ? "任务节点将在人工决策完成后分解生成"
        : "等待 Control Plane 继续写入协调事实";

  return (
    <div className="min-w-0">
      <SoftCard className="@container/workflow-graph min-w-0 overflow-hidden">
        <DemandSummaryBar detail={detail} instance={instance} />

        <div className="flex items-start justify-between gap-3 border-b border-line p-4">
          <div className="flex min-w-0 items-center gap-3">
            <IconTile tone="brand" size="sm">
              <ListChecks />
            </IconTile>
            <div className="min-w-0">
              <h2 className="text-base font-bold tracking-normal text-ink">
                {orchestrationTitle}
              </h2>
              <p className="mt-1 text-xs text-ink-2">
                {orchestrationSubtitle}
              </p>
            </div>
          </div>
        </div>

        {hasGraphContent && graph ? (
          <div className="min-w-0 p-4">
            <Suspense fallback={<LoadingState />}>
              <WorkflowGraphCanvas
                graph={graph}
                onNodeOpen={setSelectedNodeId}
                onSelectedNodeChange={setSelectedNodeId}
                selectedNodeId={selectedNodeId}
              />
            </Suspense>
          </div>
        ) : pendingDecision ? (
          <PendingDecisionPanel decision={pendingDecision} />
        ) : (
          <div className="p-5 text-sm leading-6 text-ink-2">
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
            <DialogHeader className="shrink-0 border-b border-line px-5 py-4">
              <DialogTitle className="text-base font-bold tracking-normal text-ink">
                节点详情
              </DialogTitle>
              <DialogDescription className="text-xs text-ink-2">
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
  instance
}: {
  detail: ProjectDemandLaunchDetail;
  instance?: WorkflowInstanceSummary;
}) {
  return (
    <div className="border-b border-line bg-card px-4 py-3">
      <div className="flex flex-col gap-3 @lg/workflow-graph:flex-row @lg/workflow-graph:items-start @lg/workflow-graph:justify-between">
        <div className="min-w-0">
          <div className="flex flex-wrap items-center gap-x-3 gap-y-1">
            <p className="text-xs font-bold text-ink-3">需求摘要</p>
            <p className="truncate text-xs text-ink-2">
              {detail.project.name}
            </p>
          </div>
          <h2 className="mt-1 line-clamp-2 text-base font-bold leading-6 tracking-normal text-ink">
            {detail.demand.title}
          </h2>
          <p className="mt-2 line-clamp-2 text-sm leading-6 text-ink-2">
            {detail.demand.content || "暂无需求正文"}
          </p>
        </div>

        <div className="flex shrink-0 flex-wrap items-center gap-2 @lg/workflow-graph:justify-end">
          {/* instance 来自首页 50 条流程实例列表的命中，直链访问不在该页范围内的需求时
              (例如已排到河道末尾的失败需求) 拿不到这份进度快照；由 launch-detail 已经
              提供的需求/项目信息渲染详情主体，这里的状态/进度 pill 没有真实数据时不伪造。 */}
          {instance ? (
            <>
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
            </>
          ) : null}
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

// 空图 + 待处理人工决策：展示决策类型/标题/摘要与审批入口，
// 让人能直接判断"该去批准还是驳回"，而不是面对一句"任务正在规划"。
function PendingDecisionPanel({ decision }: { decision: ProjectDecisionRequest }) {
  return (
    <div className="p-5">
      <div className="rounded-inner border border-line p-4">
        <div className="flex flex-wrap items-center gap-2">
          <StatusPill tone="warn">{decisionTypeLabel(decision.decision_type)}</StatusPill>
          {decision.risk_level_snapshot ? (
            <StatusPill tone={decisionRiskTone(decision.risk_level_snapshot)}>
              {riskLevelLabel(decision.risk_level_snapshot)}
            </StatusPill>
          ) : null}
        </div>
        <p className="mt-3 text-sm font-semibold text-ink">{decision.title_snapshot}</p>
        {decision.summary_snapshot ? (
          <p className="mt-1 line-clamp-3 text-xs leading-5 text-ink-2">
            {decision.summary_snapshot}
          </p>
        ) : null}
        <div className="mt-4">
          <Button asChild size="sm" variant="primary">
            <Link to="/inbox">前往收件箱处理</Link>
          </Button>
        </div>
      </div>
    </div>
  );
}

function decisionRiskTone(level: string | undefined): "danger" | "warn" | "mute" {
  const normalized = level?.trim().toLowerCase();
  if (normalized === "high" || normalized === "blocked") return "danger";
  if (normalized === "medium") return "warn";
  return "mute";
}
