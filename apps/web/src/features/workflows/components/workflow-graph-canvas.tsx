import { useMemo } from "react";
import {
  Background,
  Controls,
  MiniMap,
  ReactFlow,
  type NodeTypes,
} from "@xyflow/react";
import "@xyflow/react/dist/style.css";
import type { ProjectTaskGraph } from "@/lib/api/projects";
import {
  buildWorkflowGraphElements,
  selectInitialWorkflowNodeId,
  type WorkflowGraphElements,
} from "../workflow-graph-adapter";
import { WorkflowAttachmentNode, WorkflowTaskNode } from "./workflow-task-node";

const nodeTypes = {
  workflowAttachment: WorkflowAttachmentNode,
  workflowTask: WorkflowTaskNode,
} satisfies NodeTypes;

type WorkflowGraphCanvasProps = {
  graph: ProjectTaskGraph;
  onSelectedNodeChange: (nodeId: string | undefined) => void;
  selectedNodeId: string | undefined;
};

export function WorkflowGraphCanvas({
  graph,
  onSelectedNodeChange,
  selectedNodeId,
}: WorkflowGraphCanvasProps) {
  const elements = useMemo<WorkflowGraphElements>(
    () => buildWorkflowGraphElements(graph),
    [graph],
  );
  const nodes = useMemo(
    () =>
      elements.nodes.map((node) => ({
        ...node,
        selected: node.id === selectedNodeId,
      })),
    [elements.nodes, selectedNodeId],
  );

  return (
    <div className="h-[620px] min-h-[420px] overflow-hidden rounded-xl border bg-[linear-gradient(180deg,rgba(248,251,255,0.95),rgba(255,255,255,0.9))]">
      <ReactFlow
        edges={elements.edges}
        fitView
        maxZoom={1.25}
        minZoom={0.45}
        nodeTypes={nodeTypes}
        nodes={nodes}
        nodesConnectable={false}
        nodesDraggable={false}
        onNodeClick={(_, node) => onSelectedNodeChange(node.parentId ?? node.id)}
        onPaneClick={() => onSelectedNodeChange(selectInitialWorkflowNodeId(graph))}
        proOptions={{ hideAttribution: true }}
      >
        <Background />
        <MiniMap pannable={false} zoomable={false} />
        <Controls showInteractive={false} />
      </ReactFlow>
    </div>
  );
}
