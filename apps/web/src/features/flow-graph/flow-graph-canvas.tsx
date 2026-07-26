import { useMemo, useState } from "react";
import {
  Background,
  Controls,
  MiniMap,
  ReactFlow,
  type EdgeTypes,
  type NodeTypes
} from "@xyflow/react";
import "@xyflow/react/dist/style.css";
import { History } from "lucide-react";
import { Button } from "@/components/superteam";
import type { ProjectTaskGraph } from "@/lib/api/projects";
import { formatRunDuration } from "@/lib/format-time";
import {
  PLAN_TASK_GRAPH_LAYOUT,
  buildFlowGraphElements,
  type FlowGraphElements,
  type FlowLiveEdgeData
} from "./flow-graph-adapter";
import { FlowHandoffOverlay } from "./flow-handoff-overlay";
import { FlowLiveEdge } from "./flow-live-edge";
import { REPLAY_WINDOW_MS } from "./flow-replay";
import { useFlowReplay } from "./use-flow-replay";
import { usePrefersReducedMotion } from "./use-prefers-reduced-motion";
import { WorkflowBlockingNode } from "./workflow-blocking-node";
import {
  WorkflowAttachmentNode,
  WorkflowStageLabelNode,
  WorkflowTaskNode
} from "./workflow-task-node";

const MIN_CANVAS_HEIGHT = 820;
const CANVAS_TOP_PADDING = 36;
const CANVAS_BOTTOM_PADDING = 96;

const nodeTypes = {
  workflowAttachment: WorkflowAttachmentNode,
  workflowBlocking: WorkflowBlockingNode,
  workflowStageLabel: WorkflowStageLabelNode,
  workflowTask: WorkflowTaskNode
} satisfies NodeTypes;

const edgeTypes = {
  flowLive: FlowLiveEdge
} satisfies EdgeTypes;

type FlowGraphCanvasProps = {
  graph: ProjectTaskGraph;
  /**
   * 活图模式（spec 2026-07-27）：概念 A 粒子活性边 + 节点卡时间区 + 点边交接
   * 对照浮层。默认关，仅项目需求流程区开启；既有消费方零行为变化。
   */
  live?: boolean;
  onNodeOpen: (nodeId: string) => void;
  onSelectedNodeChange: (nodeId: string | undefined) => void;
  selectedNodeId: string | undefined;
};

/** 流程图单一权威画布：项目详情与流程编排详情共用（spec 2026-07-26 §4.2）。 */
export function FlowGraphCanvas({
  graph,
  live = false,
  onNodeOpen,
  onSelectedNodeChange,
  selectedNodeId
}: FlowGraphCanvasProps) {
  const [handoffEdge, setHandoffEdge] = useState<FlowLiveEdgeData | undefined>(
    undefined,
  );
  const prefersReducedMotion = usePrefersReducedMotion();
  // 回放执行（spec §5 P2-R）：回放中渲染"开始瞬间快照 + 虚拟状态"图；新数据
  // 停留在 react-query 缓存，回放结束自动回实时。虚拟图走同一 adapter 推导，
  // reduced-motion / 大图降级分支对回放同样生效。
  const replay = useFlowReplay(graph, live);
  const effectiveGraph = replay.replayGraph ?? graph;
  const elements = useMemo<FlowGraphElements>(
    () => buildFlowGraphElements(effectiveGraph, { live }),
    [effectiveGraph, live],
  );
  // 降级标记（spec §5 P2-S，可测）：大图降级标 "scale"，reduced-motion 标 "motion"，
  // 两者并存时 scale 优先；非 live 或未降级不打标记。
  const liveDegraded = !live
    ? undefined
    : elements.scaleDegraded
      ? "scale"
      : prefersReducedMotion
        ? "motion"
        : undefined;
  const nodes = useMemo(
    () =>
      elements.nodes.map((node) => ({
        ...node,
        selected: node.id === selectedNodeId
})),
    [elements.nodes, selectedNodeId],
  );
  const stageCount = useMemo(
    () =>
      new Set(
        graph.nodes.map((node) =>
          Number.isFinite(node.stage_index) ? Number(node.stage_index) : 0,
        ),
      ).size,
    [graph.nodes],
  );
  const canvasHeight = Math.max(
    MIN_CANVAS_HEIGHT,
    CANVAS_TOP_PADDING +
      measureGraphContentHeight(elements) +
      CANVAS_BOTTOM_PADDING,
  );

  return (
    <div
      className="relative min-h-[620px] w-full min-w-0 overflow-hidden rounded-xl border bg-[linear-gradient(180deg,rgba(248,251,255,0.95),rgba(255,255,255,0.9))]"
      data-live={live ? "true" : "false"}
      data-live-degraded={liveDegraded}
      data-replay={replay.isReplaying ? "true" : undefined}
      data-stage-count={stageCount}
      data-task-count={graph.nodes.length}
      data-testid="flow-graph-canvas"
      style={{ height: canvasHeight }}
    >
      {live ? (
        <div className="absolute right-3 top-3 z-10 flex items-center gap-2">
          {replay.isReplaying && replay.timeline ? (
            <span
              className="rounded-[8px] border border-line bg-card/92 px-2 py-1 text-[11px] tabular-nums leading-4 text-ink-2 shadow-sm"
              data-testid="flow-replay-progress"
            >
              {`T+${((replay.progress * REPLAY_WINDOW_MS) / 1000).toFixed(1)} 秒 / ${
                REPLAY_WINDOW_MS / 1000
              } 秒 · 压缩自 ${
                formatRunDuration(replay.timeline.t0Iso, replay.timeline.tEndIso) ??
                "不足 1 秒"
              }`}
            </span>
          ) : null}
          {/* disabled 按钮 pointer-events 关闭，title 提示挂外层 span 才能悬停可见。 */}
          <span
            title={
              !replay.available && !replay.isReplaying
                ? "任务节点暂无起止时间数据，无法回放"
                : undefined
            }
          >
            <Button
              data-testid="flow-replay-toggle"
              disabled={!replay.available && !replay.isReplaying}
              onClick={() => (replay.isReplaying ? replay.stop() : replay.start())}
              size="sm"
              variant="outline"
            >
              <History aria-hidden className="size-3.5" />
              {replay.isReplaying ? "回放中…点击停止" : "回放执行"}
            </Button>
          </span>
        </div>
      ) : null}
      <ReactFlow
        defaultViewport={{ x: 700, y: 36, zoom: 1 }}
        edgeTypes={edgeTypes}
        edges={elements.edges}
        maxZoom={1.25}
        minZoom={0.65}
        nodeTypes={nodeTypes}
        nodes={nodes}
        nodesConnectable={false}
        nodesDraggable={false}
        onEdgeClick={
          live
            ? (_, edge) => {
                if (edge.data) setHandoffEdge(edge.data as FlowLiveEdgeData);
              }
            : undefined
        }
        onNodeClick={(_, node) => {
          const selectedId = node.parentId ?? node.id;
          onSelectedNodeChange(selectedId);
          onNodeOpen(selectedId);
        }}
        onPaneClick={() => onSelectedNodeChange(undefined)}
        proOptions={{ hideAttribution: true }}
      >
        <Background />
        <MiniMap pannable={false} zoomable={false} />
        <Controls showInteractive={false} />
      </ReactFlow>
      {live ? (
        // 回放中保持可点（最简取舍）：浮层数据取回放快照，与画面一致。
        <FlowHandoffOverlay
          edge={handoffEdge}
          graph={effectiveGraph}
          onClose={() => setHandoffEdge(undefined)}
        />
      ) : null}
    </div>
  );
}

function measureGraphContentHeight(elements: FlowGraphElements): number {
  return elements.nodes.reduce((maxBottom, node) => {
    const estimatedHeight =
      node.type === "workflowStageLabel"
        ? PLAN_TASK_GRAPH_LAYOUT.stageLabelHeight
        : node.type === "workflowBlocking"
          ? 220
        : PLAN_TASK_GRAPH_LAYOUT.taskEstimatedHeight;
    return Math.max(maxBottom, node.position.y + estimatedHeight);
  }, 0);
}
