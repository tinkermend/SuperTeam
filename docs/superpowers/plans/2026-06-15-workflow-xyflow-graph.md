# Workflow Xyflow Graph Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the workflow detail's non-graph task summary with a read-only `@xyflow/react` ProjectTask DAG, node inspector, and final real-chain workflow smoke.

**Architecture:** Keep backend facts unchanged. Convert `ProjectTaskGraph` into controlled xyflow nodes and edges in a pure adapter, render task nodes as SuperTeam-style cards, and keep all mutation actions as external navigation. The canvas never saves layout or changes workflow state.

**Tech Stack:** React 19, `@xyflow/react`, TanStack Query, TanStack Router, SuperTeam liquid components, lucide-react, Vitest Browser, Playwright/browser verification through the repo's Web workflow.

---

## Source Spec

Implement this subplan against:

- `docs/superpowers/specs/2026-06-15-workflow-orchestration-graph-design.md`
- `docs/superpowers/plans/2026-06-15-workflow-instances-read-model.md`
- `docs/superpowers/plans/2026-06-15-workflow-shell-and-list.md`

This subplan assumes `/workflows/$demandId` already loads workflow instances, launch detail, and task graph data.

**Hard prerequisite — cannot compile standalone:** `ProjectTaskGraph`, `ProjectTaskGraphNode`, `ProjectTaskGraphEdge`, `ProjectTaskGraphEmployee`, and `ProjectTaskGraphRun` are introduced by `2026-06-15-workflow-shell-and-list.md` and are **not** present in `apps/web/src/lib/api/projects.ts` until that plan lands. This plan's adapter and tests (`import type { ProjectTaskGraph, ProjectTaskGraphNode }`) will not compile until the shell-and-list plan is merged. The node field shapes (`stage_index`, `expected_outputs`, `input_requirements`, `handoff_contract`, `planner_metadata`, `risk_level`, `summary`, …) must match both that plan's types and what the Control Plane `task-graph` endpoint actually returns. Execute in order: read-model → shell-and-list → xyflow.

## File Structure

Modify:

- `apps/web/package.json` and `pnpm-lock.yaml`
  Add `@xyflow/react`.
- `apps/web/src/features/workflows/components/workflow-detail.tsx`
  Replace the task summary panel with the graph canvas plus inspector.
- `apps/web/src/features/workflows/index.test.tsx`
  Cover graph rendering and selected node detail behavior.

Create:

- `apps/web/src/features/workflows/workflow-graph-adapter.ts`
- `apps/web/src/features/workflows/workflow-graph-adapter.test.ts`
- `apps/web/src/features/workflows/components/workflow-graph-canvas.tsx`
- `apps/web/src/features/workflows/components/workflow-task-node.tsx`
- `apps/web/src/features/workflows/components/workflow-node-inspector.tsx`

## Task 1: Install Xyflow Dependency

**Files:**

- Modify: `apps/web/package.json`
- Modify: `pnpm-lock.yaml`

- [ ] **Step 1: Add dependency**

Run:

```bash
corepack pnpm --filter @superteam/web add @xyflow/react
```

Expected: `apps/web/package.json` lists `@xyflow/react`, and `pnpm-lock.yaml` changes.

- [ ] **Step 2: Verify dependency resolves**

Run:

```bash
corepack pnpm --filter @superteam/web exec node -e "import('@xyflow/react').then(() => console.log('xyflow ok'))"
```

Expected output contains:

```text
xyflow ok
```

- [ ] **Step 3: Confirm xyflow major version and node typing**

Run:

```bash
corepack pnpm --filter @superteam/web list @xyflow/react
```

Expected: record the installed `@xyflow/react` major version. `@xyflow/react` v12 changed `NodeProps` to be generic over the **Node type**, not the data type:

- v12: custom node props are `NodeProps<Node<YourData>>`, and `Node<YourData>` requires `YourData` to satisfy xyflow's `ElementData` constraint.
- v11: `NodeProps<YourData>` was correct.

If v12 is installed, the node components below must use `NodeProps<Node<WorkflowTaskNodeData>>` / `NodeProps<Node<WorkflowAttachmentNodeData>>` instead of `NodeProps<WorkflowTaskNodeData>`. The unit tests mock `@xyflow/react`, so only `pnpm typecheck`/`build` enforce this — fix it before Task 5.

- [ ] **Step 4: Commit dependency slice**

Run:

```bash
git add apps/web/package.json pnpm-lock.yaml
git commit -m "chore(web): add xyflow"
```

Expected: commit succeeds with only dependency files.

## Task 2: Graph Adapter

**Files:**

- Create: `apps/web/src/features/workflows/workflow-graph-adapter.ts`
- Create: `apps/web/src/features/workflows/workflow-graph-adapter.test.ts`

- [ ] **Step 1: Write failing adapter tests**

Create `apps/web/src/features/workflows/workflow-graph-adapter.test.ts`:

```ts
import { describe, expect, it } from "vitest";
import { buildWorkflowGraphElements, selectInitialWorkflowNodeId } from "./workflow-graph-adapter";
import type { ProjectTaskGraph } from "@/lib/api/projects";

function makeGraph(): ProjectTaskGraph {
  return {
    nodes: [
      {
        id: "task-root",
        tenant_id: "tenant-1",
        project_id: "project-1",
        demand_id: "demand-1",
        title: "任务入口",
        summary: "整理需求和上下文",
        status: "completed",
        assigned_digital_employee_id: "employee-1",
        requires_human_approval: false,
        stage_index: 1,
        expected_outputs: ["需求清单"],
        input_requirements: {},
        handoff_contract: {},
        planner_metadata: {},
      },
      {
        id: "task-run",
        tenant_id: "tenant-1",
        project_id: "project-1",
        demand_id: "demand-1",
        title: "服务健康巡检",
        summary: "检查 payment-api 和队列",
        status: "assigned",
        assigned_digital_employee_id: "employee-2",
        requires_human_approval: true,
        stage_index: 2,
        expected_outputs: ["服务健康报告"],
        input_requirements: { scope: "payment" },
        handoff_contract: {},
        planner_metadata: {},
      },
    ],
    edges: [
      {
        blocker_task_id: "task-root",
        dependent_task_id: "task-run",
        edge_status: "unblocked",
      },
    ],
    employees: [
      {
        digital_employee_id: "employee-1",
        display_name: "需求分析师",
        project_role: "executor",
        status: "active",
      },
      {
        digital_employee_id: "employee-2",
        display_name: "应用运维工程师",
        project_role: "executor",
        status: "active",
      },
    ],
    runs: [
      {
        project_task_id: "task-run",
        runtime_node_summary: "runtime-east-1",
        status: "assigned",
        provider_type: "codex",
      },
    ],
    execution_summaries: [],
    recent_events: [],
    decision_requests: [
      {
        id: "decision-1",
        tenant_id: "tenant-1",
        project_id: "project-1",
        approval_request_id: "approval-1",
        project_task_id: "task-run",
        target_user_id: "reviewer-1",
        decision_type: "route_review",
        title_snapshot: "需要确认巡检范围",
        status_snapshot: "pending",
      },
    ],
  };
}

describe("workflow graph adapter", () => {
  it("maps project task graph nodes and edges into xyflow elements", () => {
    const result = buildWorkflowGraphElements(makeGraph());

    expect(result.nodes.map((node) => node.id)).toEqual(["task:task-root", "task:task-run", "attachment:decision:decision-1"]);
    expect(result.edges).toEqual([
      expect.objectContaining({
        id: "edge:task-root:task-run",
        source: "task:task-root",
        target: "task:task-run",
      }),
    ]);
    expect(result.nodes[0].position.x).toBe(0);
    expect(result.nodes[1].position.x).toBeGreaterThan(result.nodes[0].position.x);
    expect(result.nodes[1].data.employeeName).toBe("应用运维工程师");
    expect(result.nodes[1].data.runStatus).toBe("assigned");
  });

  it("selects failed, waiting, running, then first node", () => {
    expect(selectInitialWorkflowNodeId(makeGraph())).toBe("task:task-run");
    const completed = makeGraph();
    completed.nodes[1].status = "completed";
    expect(selectInitialWorkflowNodeId(completed)).toBe("task:task-root");
  });
});
```

- [ ] **Step 2: Run adapter tests and verify RED**

Run:

```bash
pnpm --dir apps/web test src/features/workflows/workflow-graph-adapter.test.ts
```

Expected: FAIL because the adapter file does not exist.

- [ ] **Step 3: Implement adapter**

Create `apps/web/src/features/workflows/workflow-graph-adapter.ts`:

```ts
import type { Edge, Node } from "@xyflow/react";
import type {
  ProjectDecisionRequest,
  ProjectTaskGraph,
  ProjectTaskGraphNode,
} from "@/lib/api/projects";

export type WorkflowTaskNodeData = {
  employeeName?: string;
  expectedOutputs: unknown[];
  hasPendingDecision: boolean;
  requiresHumanApproval: boolean;
  riskLevel?: string;
  runStatus?: string;
  status: string;
  summary?: string;
  task: ProjectTaskGraphNode;
  title: string;
};

export type WorkflowAttachmentNodeData = {
  status: string;
  title: string;
  type: "decision";
};

export type WorkflowGraphElements = {
  edges: Edge[];
  nodes: Array<Node<WorkflowTaskNodeData> | Node<WorkflowAttachmentNodeData>>;
};

const STAGE_X = 360;
const ROW_Y = 170;
const ATTACHMENT_OFFSET_Y = 116;

export function buildWorkflowGraphElements(graph: ProjectTaskGraph): WorkflowGraphElements {
  const employees = new Map(graph.employees.map((employee) => [employee.digital_employee_id, employee]));
  const runsByTask = new Map(graph.runs.map((run) => [run.project_task_id, run]));
  const decisionsByTask = pendingDecisionsByTask(graph.decision_requests);
  const stageRows = new Map<number, number>();

  const nodes: WorkflowGraphElements["nodes"] = graph.nodes.map((task) => {
    const stage = task.stage_index ?? lastKnownStage(graph.nodes);
    const row = stageRows.get(stage) ?? 0;
    stageRows.set(stage, row + 1);
    const employee = task.assigned_digital_employee_id ? employees.get(task.assigned_digital_employee_id) : undefined;
    const run = runsByTask.get(task.id);
    const decisions = decisionsByTask.get(task.id) ?? [];
    return {
      id: taskNodeId(task.id),
      type: "workflowTask",
      position: { x: Math.max(stage - 1, 0) * STAGE_X, y: row * ROW_Y },
      data: {
        employeeName: employee?.display_name,
        expectedOutputs: task.expected_outputs,
        hasPendingDecision: decisions.length > 0,
        requiresHumanApproval: task.requires_human_approval,
        riskLevel: task.risk_level,
        runStatus: run?.status,
        status: task.status,
        summary: task.summary,
        task,
        title: task.title,
      },
    };
  });

  for (const decision of graph.decision_requests) {
    if (!decision.project_task_id || !isPendingDecision(decision)) {
      continue;
    }
    const parent = nodes.find((node) => node.id === taskNodeId(decision.project_task_id));
    if (!parent) {
      continue;
    }
    nodes.push({
      id: `attachment:decision:${decision.id}`,
      type: "workflowAttachment",
      position: { x: parent.position.x + 36, y: parent.position.y + ATTACHMENT_OFFSET_Y },
      data: {
        status: decision.status_snapshot,
        title: decision.title_snapshot,
        type: "decision",
      },
      parentId: parent.id,
    });
  }

  const edges = graph.edges.map((edge) => ({
    animated: edge.edge_status === "unblocked" || edge.edge_status === "ready",
    id: `edge:${edge.blocker_task_id}:${edge.dependent_task_id}`,
    source: taskNodeId(edge.blocker_task_id),
    target: taskNodeId(edge.dependent_task_id),
    type: "smoothstep",
    label: edge.edge_status,
  }));

  return { edges, nodes };
}

export function selectInitialWorkflowNodeId(graph: ProjectTaskGraph): string | undefined {
  const priority = ["failed", "blocked", "waiting_human", "pending", "assigned", "running", "in_progress"];
  for (const status of priority) {
    const task = graph.nodes.find((node) => node.status === status);
    if (task) {
      return taskNodeId(task.id);
    }
  }
  // No status-priority match: fall back to the first node. Do NOT fold pending
  // decisions into this loop — that clause was status-independent and made any task
  // with a pending decision win on the first ("failed") iteration, defeating both
  // the priority order and the first-node fallback. Pending decisions are surfaced
  // as attachment nodes on their task instead.
  return graph.nodes[0] ? taskNodeId(graph.nodes[0].id) : undefined;
}

export function taskNodeId(taskId: string): string {
  return `task:${taskId}`;
}

function lastKnownStage(nodes: ProjectTaskGraphNode[]): number {
  return nodes.reduce((max, node) => Math.max(max, node.stage_index ?? 1), 1);
}

function pendingDecisionsByTask(decisions: ProjectDecisionRequest[]): Map<string, ProjectDecisionRequest[]> {
  const byTask = new Map<string, ProjectDecisionRequest[]>();
  for (const decision of decisions) {
    if (!decision.project_task_id || !isPendingDecision(decision)) {
      continue;
    }
    byTask.set(decision.project_task_id, [...(byTask.get(decision.project_task_id) ?? []), decision]);
  }
  return byTask;
}

function isPendingDecision(decision: ProjectDecisionRequest): boolean {
  return ["pending", "requested", "open"].includes(decision.status_snapshot);
}
```

- [ ] **Step 4: Run adapter tests and verify GREEN**

Run:

```bash
pnpm --dir apps/web test src/features/workflows/workflow-graph-adapter.test.ts
```

Expected: PASS.

- [ ] **Step 5: Commit adapter slice**

Run:

```bash
git add apps/web/src/features/workflows/workflow-graph-adapter.ts apps/web/src/features/workflows/workflow-graph-adapter.test.ts
git commit -m "feat: map workflow graph elements"
```

Expected: commit succeeds with adapter and adapter tests.

## Task 3: Canvas And Node Components

**Files:**

- Create: `apps/web/src/features/workflows/components/workflow-graph-canvas.tsx`
- Create: `apps/web/src/features/workflows/components/workflow-task-node.tsx`
- Create: `apps/web/src/features/workflows/components/workflow-node-inspector.tsx`
- Modify: `apps/web/src/features/workflows/index.test.tsx`

- [ ] **Step 1: Mock xyflow in workflow tests**

Add this mock to `apps/web/src/features/workflows/index.test.tsx` before the router mock:

```tsx
vi.mock("@xyflow/react", () => ({
  Background: () => <div data-testid="xyflow-background" />,
  Controls: () => <div data-testid="xyflow-controls" />,
  Handle: () => <span data-testid="xyflow-handle" />,
  MiniMap: () => <div data-testid="xyflow-minimap" />,
  Position: { Bottom: "bottom", Top: "top" },
  ReactFlow: ({
    children,
    nodes,
    onNodeClick,
  }: {
    children: ReactNode;
    nodes: Array<{ id: string; data: { title?: string } }>;
    onNodeClick?: (event: unknown, node: { id: string }) => void;
  }) => (
    <div data-testid="workflow-canvas">
      {nodes.map((node) => (
        <button key={node.id} type="button" onClick={() => onNodeClick?.({}, { id: node.id })}>
          {node.data.title ?? node.id}
        </button>
      ))}
      {children}
    </div>
  ),
}));
```

- [ ] **Step 2: Add failing canvas test**

Add to the first workflow test in `apps/web/src/features/workflows/index.test.tsx` or create a new test:

```tsx
it("renders the task graph canvas and selected node inspector", async () => {
  const fetcher = createWorkflowFetcher();
  await renderWithQueryClient(
    <WorkflowView apiBaseUrl="http://control-plane.local" demandId="demand-running" fetcher={fetcher} />,
  );

  await vi.waitFor(() => expect(getByText("服务健康巡检")).toBeTruthy());
  expect(document.body.querySelector("[data-testid='workflow-canvas']")).toBeTruthy();
  expect(getByText("节点详情")).toBeTruthy();
  expect(getByText("assigned")).toBeTruthy();
});
```

- [ ] **Step 3: Run workflow test and verify RED**

Run:

```bash
pnpm --dir apps/web test src/features/workflows/index.test.tsx
```

Expected: FAIL because the workflow detail still renders the non-graph summary panel.

- [ ] **Step 4: Add task node component**

Create `apps/web/src/features/workflows/components/workflow-task-node.tsx`:

```tsx
import { Handle, Position, type NodeProps } from "@xyflow/react";
import { Bot, ShieldCheck } from "lucide-react";
import { StatusBadge } from "@/components/superteam";
import type { WorkflowAttachmentNodeData, WorkflowTaskNodeData } from "../workflow-graph-adapter";
import { taskStatusTone } from "./workflow-node-inspector";

export function WorkflowTaskNode({ data, selected }: NodeProps<WorkflowTaskNodeData>) {
  return (
    <div className={[
      "w-[300px] rounded-lg border bg-background p-3 shadow-sm",
      selected ? "border-primary ring-2 ring-primary/20" : "border-border",
    ].join(" ")}>
      <Handle type="target" position={Position.Top} />
      <div className="flex items-start justify-between gap-2">
        <div className="min-w-0">
          <p className="line-clamp-2 text-sm font-semibold tracking-normal">{data.title}</p>
          <p className="mt-1 line-clamp-2 text-xs text-muted-foreground">{data.summary || "暂无摘要"}</p>
        </div>
        <StatusBadge tone={taskStatusTone(data.status)}>{data.status}</StatusBadge>
      </div>
      <div className="mt-3 flex flex-wrap gap-2 text-xs text-muted-foreground">
        {data.employeeName ? (
          <span className="inline-flex items-center gap-1 rounded-md bg-muted px-2 py-1">
            <Bot className="size-3" />
            {data.employeeName}
          </span>
        ) : null}
        {data.requiresHumanApproval || data.hasPendingDecision ? (
          <span className="inline-flex items-center gap-1 rounded-md bg-amber-50 px-2 py-1 text-amber-700">
            <ShieldCheck className="size-3" />
            人工确认
          </span>
        ) : null}
      </div>
      <Handle type="source" position={Position.Bottom} />
    </div>
  );
}

export function WorkflowAttachmentNode({ data, selected }: NodeProps<WorkflowAttachmentNodeData>) {
  return (
    <div className={[
      "w-[240px] rounded-lg border bg-background p-2 shadow-sm",
      selected ? "border-primary ring-2 ring-primary/20" : "border-amber-300",
    ].join(" ")}>
      <Handle type="target" position={Position.Top} />
      <div className="flex items-start gap-2">
        <ShieldCheck className="mt-0.5 size-3.5 text-amber-600" />
        <div className="min-w-0">
          <p className="line-clamp-2 text-xs font-medium">{data.title}</p>
          <p className="text-[10px] text-muted-foreground">{data.status}</p>
        </div>
      </div>
      <Handle type="source" position={Position.Bottom} />
    </div>
  );
}
```

- [ ] **Step 5: Add inspector component**

Create `apps/web/src/features/workflows/components/workflow-node-inspector.tsx`:

```tsx
import { Link } from "@tanstack/react-router";
import { ExternalLink } from "lucide-react";
import { Button } from "@/components/ui/button";
import { LiquidCard, StatusBadge, type Tone } from "@/components/superteam";
import type { ProjectTaskGraph, ProjectTaskGraphNode } from "@/lib/api/projects";

type WorkflowNodeInspectorProps = {
  graph: ProjectTaskGraph;
  selectedTask?: ProjectTaskGraphNode;
};

export function WorkflowNodeInspector({ graph, selectedTask }: WorkflowNodeInspectorProps) {
  if (!selectedTask) {
    return <LiquidCard className="rounded-xl p-4 text-sm text-muted-foreground">选择节点查看详情</LiquidCard>;
  }
  const run = graph.runs.find((item) => item.project_task_id === selectedTask.id);
  const summary = graph.execution_summaries.find((item) => item.project_task_id === selectedTask.id);
  const decisions = graph.decision_requests.filter((item) => item.project_task_id === selectedTask.id);

  return (
    <LiquidCard className="rounded-xl p-4">
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0">
          <p className="text-xs font-medium text-muted-foreground">节点详情</p>
          <h3 className="mt-1 line-clamp-2 text-sm font-semibold tracking-normal">{selectedTask.title}</h3>
        </div>
        <StatusBadge tone={taskStatusTone(selectedTask.status)}>{selectedTask.status}</StatusBadge>
      </div>
      <div className="mt-4 grid gap-2 text-sm">
        <InspectorRow label="输入" value={objectSummary(selectedTask.input_requirements)} />
        <InspectorRow label="输出" value={selectedTask.expected_outputs.length ? `${selectedTask.expected_outputs.length} 项` : "尚未定义"} />
        <InspectorRow label="Run" value={run ? `${run.provider_type} / ${run.status}` : "未绑定"} />
        <InspectorRow label="结果" value={summary?.conclusion || "尚未产生"} />
        <InspectorRow label="人工决策" value={decisions.length ? `${decisions.length} 项待查看` : "无"} />
      </div>
      <div className="mt-4 flex flex-wrap gap-2">
        {run?.runtime_task_id ? (
          <Button asChild size="sm" variant="outline">
            <Link to="/runtime">
              <ExternalLink className="size-4" />
              打开 Runtime
            </Link>
          </Button>
        ) : null}
        {decisions.length ? (
          <Button asChild size="sm" variant="outline">
            <Link to="/approvals">
              <ExternalLink className="size-4" />
              去审批
            </Link>
          </Button>
        ) : null}
      </div>
    </LiquidCard>
  );
}

export function taskStatusTone(status: string): Tone {
  if (["completed", "done", "success"].includes(status)) {
    return "success";
  }
  if (["failed", "cancelled"].includes(status)) {
    return "danger";
  }
  if (["blocked", "waiting_human", "pending"].includes(status)) {
    return "warning";
  }
  if (["assigned", "running", "in_progress"].includes(status)) {
    return "info";
  }
  return "neutral";
}

function InspectorRow({ label, value }: { label: string; value: string }) {
  return (
    <div className="grid grid-cols-[72px_minmax(0,1fr)] gap-3">
      <span className="text-muted-foreground">{label}</span>
      <span className="min-w-0 break-words font-medium text-foreground">{value}</span>
    </div>
  );
}

function objectSummary(value: Record<string, unknown>): string {
  const keys = Object.keys(value);
  return keys.length ? keys.join(", ") : "无";
}
```

- [ ] **Step 6: Add graph canvas component**

Create `apps/web/src/features/workflows/components/workflow-graph-canvas.tsx`:

```tsx
// Requires @xyflow/react to be installed (Task 1). This specifier is NOT covered by the
// vi.mock("@xyflow/react") in tests, so the package must be present before tests run.
import "@xyflow/react/dist/style.css";
import { Background, Controls, MiniMap, ReactFlow } from "@xyflow/react";
import { useMemo } from "react";
import type { ProjectTaskGraph } from "@/lib/api/projects";
import {
  buildWorkflowGraphElements,
  selectInitialWorkflowNodeId,
} from "../workflow-graph-adapter";
import { WorkflowAttachmentNode, WorkflowTaskNode } from "./workflow-task-node";

type WorkflowGraphCanvasProps = {
  graph: ProjectTaskGraph;
  onSelectedNodeChange: (nodeId: string | undefined) => void;
  selectedNodeId?: string;
};

const nodeTypes = {
  workflowTask: WorkflowTaskNode,
  workflowAttachment: WorkflowAttachmentNode,
};

export function WorkflowGraphCanvas({
  graph,
  onSelectedNodeChange,
  selectedNodeId,
}: WorkflowGraphCanvasProps) {
  const elements = useMemo(() => buildWorkflowGraphElements(graph), [graph]);
  const nodes = useMemo(
    () => elements.nodes.map((node) => ({ ...node, selected: node.id === selectedNodeId })),
    [elements.nodes, selectedNodeId],
  );

  return (
    <div className="h-[620px] min-h-[420px] overflow-hidden rounded-xl border bg-background">
      <ReactFlow
        edges={elements.edges}
        fitView
        nodeTypes={nodeTypes}
        nodes={nodes}
        onNodeClick={(_, node) => onSelectedNodeChange(node.id)}
        onPaneClick={() => onSelectedNodeChange(undefined)}
      >
        <Background />
        <Controls />
        <MiniMap pannable zoomable />
      </ReactFlow>
    </div>
  );
}
```

- [ ] **Step 7: Run component tests and verify current failure**

Run:

```bash
pnpm --dir apps/web test src/features/workflows/index.test.tsx
```

Expected: still FAIL until `WorkflowDetail` integrates the canvas.

## Task 4: Integrate Canvas Into Workflow Detail

**Files:**

- Modify: `apps/web/src/features/workflows/components/workflow-detail.tsx`
- Modify: `apps/web/src/features/workflows/index.test.tsx`

- [ ] **Step 1: Update detail component state and imports**

In `apps/web/src/features/workflows/components/workflow-detail.tsx`, add:

```tsx
import { useEffect, useMemo, useState } from "react";
import { WorkflowGraphCanvas } from "./workflow-graph-canvas";
import { WorkflowNodeInspector } from "./workflow-node-inspector";
import { selectInitialWorkflowNodeId, taskNodeId } from "../workflow-graph-adapter";
```

Inside `WorkflowDetail`, after `hasGraphNodes`:

```tsx
  const initialSelectedNodeId = useMemo(
    () => (graph ? selectInitialWorkflowNodeId(graph) : undefined),
    [graph],
  );
  const [selectedNodeId, setSelectedNodeId] = useState<string | undefined>(initialSelectedNodeId);

  useEffect(() => {
    if (!graph?.nodes.length) {
      setSelectedNodeId(undefined);
      return;
    }
    if (!selectedNodeId || !graph.nodes.some((node) => taskNodeId(node.id) === selectedNodeId)) {
      setSelectedNodeId(initialSelectedNodeId);
    }
  }, [graph, initialSelectedNodeId, selectedNodeId]);

  const selectedTask = graph?.nodes.find((node) => taskNodeId(node.id) === selectedNodeId);
```

- [ ] **Step 2: Replace task summary with canvas plus inspector**

Replace the `hasGraphNodes ? graph.nodes.slice(0, 4).map(...)` block with:

```tsx
        {hasGraphNodes && graph ? (
          <div className="mt-4 grid gap-4 xl:grid-cols-[minmax(0,1fr)_320px]">
            <WorkflowGraphCanvas
              graph={graph}
              onSelectedNodeChange={setSelectedNodeId}
              selectedNodeId={selectedNodeId}
            />
            <WorkflowNodeInspector graph={graph} selectedTask={selectedTask} />
          </div>
        ) : null}
```

Keep the planning state text for empty graphs unchanged.

- [ ] **Step 3: Run workflow tests and verify GREEN**

Run:

```bash
pnpm --dir apps/web test src/features/workflows/index.test.tsx src/features/workflows/workflow-graph-adapter.test.ts
```

Expected: PASS.

- [ ] **Step 4: Commit canvas integration slice**

Run:

```bash
git add apps/web/src/features/workflows/components/workflow-detail.tsx apps/web/src/features/workflows/components/workflow-graph-canvas.tsx apps/web/src/features/workflows/components/workflow-task-node.tsx apps/web/src/features/workflows/components/workflow-node-inspector.tsx apps/web/src/features/workflows/index.test.tsx
git commit -m "feat: render workflow task graph"
```

Expected: commit succeeds with canvas components and integration tests.

## Task 5: Local Web Verification

**Files:**

- No source changes expected.

- [ ] **Step 1: Run focused Web tests**

Run:

```bash
pnpm --dir apps/web test src/features/workflows/index.test.tsx src/features/workflows/workflow-graph-adapter.test.ts src/lib/api/projects.test.ts src/features/task-launches/index.test.tsx
```

Expected: PASS.

- [ ] **Step 2: Run Web typecheck**

Run:

```bash
pnpm --dir apps/web typecheck
```

Expected: PASS.

- [ ] **Step 3: Run Web build if typecheck passes**

Run:

```bash
pnpm --dir apps/web build
```

Expected: PASS.

- [ ] **Step 4: Run whitespace check**

Run:

```bash
git diff --check
```

Expected: no output.

## Task 6: Real-Chain Smoke

**Files:**

- No source changes expected unless smoke finds a bug.

- [ ] **Step 1: Confirm service status**

Run:

```bash
scripts/dev-services.sh status
```

Expected: status output shows whether Web, Control Plane, database, Temporal, and Runtime are running. Record the Web URL and Control Plane URL.

- [ ] **Step 2: Start or restart affected services when safe**

If Web or Control Plane is not running under `scripts/dev-services.sh`, run the repo-supported command for the missing service. If services are running old code, restart only the affected managed services:

```bash
scripts/dev-services.sh restart web
scripts/dev-services.sh restart control-plane
```

Expected: services restart without port conflicts. If the script does not support these exact service names, inspect `scripts/dev-services.sh` and use the supported names.

- [ ] **Step 3: Submit a real demand through the running UI or API**

Use the browser on `/task-launches` or an authenticated curl request against the running Control Plane to create a demand. Record the returned `demand_id`.

Expected: after submit, the browser navigates to:

```text
/workflows/{demand_id}
```

- [ ] **Step 4: Verify planning state before graph exists**

Open:

```text
/workflows/{demand_id}
```

Expected: the page shows the real demand title and either “等待项目协调线程接收” or “任务正在规划”. It must not render fake task nodes.

- [ ] **Step 5: Verify graph state after graph exists**

When Control Plane has produced task graph nodes, refresh or wait for polling.

Expected:

- The workflow instance remains selected in the left list.
- The right panel renders the xyflow canvas.
- At least one ProjectTask node title matches `GET /api/v1/projects/{projectId}/task-graph?demand_id={demandId}`.
- Clicking a node updates the inspector.

- [ ] **Step 6: Capture real API proof**

Run an authenticated curl or browser network check for:

```text
GET /api/v1/workflow-instances
GET /api/v1/project-demands/{demandId}/launch-detail
GET /api/v1/projects/{projectId}/task-graph?demand_id={demandId}
```

Expected: non-5xx responses from the running Control Plane, and response bodies contain the same demand and task IDs shown in the browser.

- [ ] **Step 7: Record final verification status**

Record:

```text
真实链路验证:
- Web URL:
- Control Plane URL:
- demand_id:
- project_id:
- workflow-instances status:
- launch-detail status:
- task-graph status:
- Browser state:

Runtime/Provider caveat:
- If no real Runtime/Provider executed the ProjectTasks, claim only workflow read/display verification, not full execution-chain availability.
```

## Task 7: Final Commit And Handoff

**Files:**

- Modify: `CHANGELOG.md` only if the implementation is being delivered as completed feature work in this branch.

- [ ] **Step 1: Add changelog entry when feature work is complete**

Run:

```bash
TZ=Asia/Shanghai date '+%Y-%m-%d %H:%M'
```

Add one entry to `CHANGELOG.md` using that timestamp:

```md
- YYYY-MM-DD HH:MM: 新增流程编排运行页，支持可见流程实例列表、任务发起后跳转、ProjectTask DAG 画布和节点详情。
```

- [ ] **Step 2: Run final gates**

Run:

```bash
pnpm verify:web
go test ./apps/control-plane/internal/project ./apps/control-plane/internal/api -count=1
pnpm verify:contracts
git diff --check
```

Expected: all commands exit 0.

- [ ] **Step 3: Commit final changelog or fixes**

Run:

```bash
# This plan only touches apps/web (+ dependency lock). Do not stage control-plane/ or
# contracts/ here — those come from the read-model plan and should already be committed.
git add CHANGELOG.md apps/web pnpm-lock.yaml
git commit -m "feat: add workflow orchestration graph"
```

Expected: commit succeeds. If there are no remaining changes because previous task commits already included everything and changelog was intentionally not required, record that no final commit was needed.
