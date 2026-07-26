import { useMemo } from "react";
import {
  Background,
  Controls,
  MiniMap,
  ReactFlow,
  type NodeTypes
} from "@xyflow/react";
import "@xyflow/react/dist/style.css";
import type { ProjectTaskGraph } from "@/lib/api/projects";
import {
  PLAN_TASK_GRAPH_LAYOUT,
  buildFlowGraphElements,
  type FlowGraphElements
} from "./flow-graph-adapter";
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

type FlowGraphCanvasProps = {
  graph: ProjectTaskGraph;
  onNodeOpen: (nodeId: string) => void;
  onSelectedNodeChange: (nodeId: string | undefined) => void;
  selectedNodeId: string | undefined;
};

/** 流程图单一权威画布：项目详情与流程编排详情共用（spec 2026-07-26 §4.2）。 */
export function FlowGraphCanvas({
  graph,
  onNodeOpen,
  onSelectedNodeChange,
  selectedNodeId
}: FlowGraphCanvasProps) {
  const elements = useMemo<FlowGraphElements>(
    () => buildFlowGraphElements(graph),
    [graph],
  );
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
      className="min-h-[620px] w-full min-w-0 overflow-hidden rounded-xl border bg-[linear-gradient(180deg,rgba(248,251,255,0.95),rgba(255,255,255,0.9))]"
      data-stage-count={stageCount}
      data-task-count={graph.nodes.length}
      data-testid="flow-graph-canvas"
      style={{ height: canvasHeight }}
    >
      <ReactFlow
        defaultViewport={{ x: 700, y: 36, zoom: 1 }}
        edges={elements.edges}
        maxZoom={1.25}
        minZoom={0.65}
        nodeTypes={nodeTypes}
        nodes={nodes}
        nodesConnectable={false}
        nodesDraggable={false}
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
