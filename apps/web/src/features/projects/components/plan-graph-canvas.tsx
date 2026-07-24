import { useMemo } from "react";
import { Background, ReactFlow, type NodeTypes } from "@xyflow/react";
import "@xyflow/react/dist/style.css";
import {
  WorkflowStageLabelNode,
  WorkflowTaskNode
} from "@/features/workflows/components/workflow-task-node";
import {
  PLAN_TASK_GRAPH_LAYOUT,
  buildPlanTaskGraphElements,
  type PlanWorkflowGraphElements
} from "@/features/workflows/workflow-graph-adapter";
import type { ProjectTaskGraph } from "@/lib/api/projects";

const MIN_CANVAS_HEIGHT = 820;
const CANVAS_TOP_PADDING = 36;
const CANVAS_BOTTOM_PADDING = 96;

const nodeTypes = {
  workflowStageLabel: WorkflowStageLabelNode,
  workflowTask: WorkflowTaskNode
} satisfies NodeTypes;

type PlanGraphCanvasProps = {
  graph: ProjectTaskGraph;
};

export function PlanGraphCanvas({ graph }: PlanGraphCanvasProps) {
  const elements = useMemo<PlanWorkflowGraphElements>(
    () => buildPlanTaskGraphElements(graph),
    [graph],
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
      data-testid="plan-graph-canvas"
      data-task-count={graph.nodes.length}
      style={{ height: canvasHeight }}
    >
      <ReactFlow
        defaultViewport={{ x: 700, y: 36, zoom: 1 }}
        edges={elements.edges}
        maxZoom={1.25}
        minZoom={0.65}
        nodeTypes={nodeTypes}
        nodes={elements.nodes}
        nodesConnectable={false}
        nodesDraggable={false}
        panOnDrag
        proOptions={{ hideAttribution: true }}
        zoomOnScroll
      >
        <Background gap={24} size={1} />
      </ReactFlow>
    </div>
  );
}

function measureGraphContentHeight(elements: PlanWorkflowGraphElements): number {
  return elements.nodes.reduce((maxBottom, node) => {
    const estimatedHeight =
      node.type === "workflowStageLabel"
        ? PLAN_TASK_GRAPH_LAYOUT.stageLabelHeight
        : PLAN_TASK_GRAPH_LAYOUT.taskEstimatedHeight;
    return Math.max(maxBottom, node.position.y + estimatedHeight);
  }, 0);
}
