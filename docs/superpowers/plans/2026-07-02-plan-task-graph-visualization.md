# Plan Task Graph Visualization Implementation Plan

> 复核状态：CHANGELOG 2026-07-03 09:23记录Project task graph可视化补齐桌面编排视图

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the plain card-list rendering of a project's just-created (not-yet-running) task plan with a vertically-staged `@xyflow/react` diagram — employee avatar + role on each task card, a compact stage header summarizing stage/employee counts, and bounded desktop rows that wrap large stages instead of letting cards collide or overflow — matching the reference screenshot's look.

**Desktop acceptance target:** For this implementation, final visual acceptance is desktop Web only. The project detail page must present the plan as a large graph canvas with visible dependency lines, a reasonable centered staged layout, digital employee avatar/name/role on task cards, task description text, and task status. A generated plan can involve up to 10 digital employees/tasks, so the desktop graph must remain readable by wrapping same-stage tasks into bounded rows with no card overlap or horizontal overflow. Mobile viewport behavior is explicitly out of scope for this pass.

**Architecture:** No new rendering framework — `apps/web` already ships `@xyflow/react` and a horizontal task-graph canvas (`features/workflows/components/workflow-graph-canvas.tsx` + `workflow-graph-adapter.ts`) for the live execution view. We add a second, independent adapter function (`buildPlanTaskGraphElements`) in the same adapter file that lays the same `ProjectTaskGraph` data out vertically by `stage_index`, centers each stage on a shared x=0 axis, wraps same-stage tasks to a maximum of 3 desktop columns per row, and emits one extra node type (a stage header). The existing horizontal adapter and canvas are left behavior-identical for their current callers — this is additive, not a rewrite. The one shared piece of visual work (employee avatar + role on the task card) is added once, to the single `WorkflowTaskNode` component both the live and plan views already use, so both views benefit.

Per this session's earlier design discussion: the reference screenshot's "任务入口"/"最终汇总" nodes are NOT a distinct node type — they are ordinary planned tasks (first stage / last stage). No synthetic entry/exit node is introduced anywhere in this plan.

**Tech Stack:** React 19, `@xyflow/react` v12, existing `@/components/superteam` design-system components (`SoftCard`, `StatusPill`, `IconTile`), `@/components/ui/avatar` primitives, Vitest + `vitest-browser-react`.

## Global Constraints

- This plan assumes the backend/contract work in `docs/superpowers/plans/2026-07-02-project-task-graph-employee-identity.md` is complete and verified end-to-end (real `employee_role`/`avatar_asset` values come back from `GET /projects/{projectId}/task-graph`) before Task 3's page wiring is considered done. Tasks 1 and 2 only need the TypeScript types to exist (already true once that plan's Task 7 lands) — they can proceed against those types even if the real backend enrichment isn't deployed yet, since they're pure/unit-tested against fixture data.
- Read `DESIGN.md` before touching any JSX in this plan — in particular "概念到代码" (`DESIGN.md:45`): compose existing `@/components/superteam` components, don't hand-roll cards/status chips with raw `div`s.
- Do not invent new `--v3-*` design tokens. Every class used below already appears in `workflow-task-node.tsx` or `plan-task-graph.tsx` today.
- Do not modify `WorkflowGraphCanvas` (`features/workflows/components/workflow-graph-canvas.tsx`) or its existing `buildWorkflowGraphElements` layout math — the live execution view it serves keeps its current horizontal layout and interaction model (click-to-inspect) unchanged. The new vertical plan view is deliberately read-only (no click/inspector), because its job is a pre-execution overview, not a live debugging surface.
- `apps/web` tests run via `corepack pnpm --filter ./apps/web run test` (never `npx vitest run` directly — see this repo's web testing rule).
- Web-only page-level changes in Task 3 still require the real end-to-end check called out in this repo's completion rules: after wiring, load the actual project detail page against a running Control Plane and confirm the graph renders from a real API response, not fixture data.

---

### Task 1: Employee identity on the shared task-node data model and card

**Files:**
- Modify: `apps/web/src/features/workflows/workflow-graph-adapter.ts`
- Modify: `apps/web/src/features/workflows/components/workflow-task-node.tsx`
- Modify: `apps/web/src/features/workflows/workflow-graph-adapter.test.ts`
- Test (new): `apps/web/src/features/workflows/components/workflow-task-node.test.tsx`

**Interfaces:**
- Consumes: `ProjectTaskGraphEmployee.employee_role?: string` and `.avatar_asset?: ProjectTaskGraphEmployeeAvatarAsset` from `@/lib/api/projects` (added by the backend plan's Task 7).
- Produces: `WorkflowTaskNodeData` gains `employeeRole: string | undefined` and `avatarAsset: WorkflowTaskNodeAvatarAsset | undefined` (a new, locally-defined camelCase type — see Step 3) — consumed by Task 2's new adapter function.

- [ ] **Step 1: Write the failing adapter test**

In `apps/web/src/features/workflows/workflow-graph-adapter.test.ts`, update the `employees` array inside `makeGraph()` (around line 53) from:

```ts
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
```

to:

```ts
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
        employee_role: "应用运维工程师·工程部",
        avatar_asset: {
          id: "avatar-1",
          label: "Adventurer 1",
          image_url: "https://example.com/avatar-1.png",
          thumbnail_url: "https://example.com/avatar-1-thumb.png",
        },
      },
    ],
```

Then extend the first test (`"maps project task graph nodes and edges into xyflow elements"`, around line 94) by adding these two assertions right after the existing `expect(runTaskData.runStatus).toBe("assigned");` line:

```ts
    expect(runTaskData.employeeRole).toBe("应用运维工程师·工程部");
    expect(runTaskData.avatarAsset?.thumbnailUrl).toBe("https://example.com/avatar-1-thumb.png");
```

Note the assertion uses `thumbnailUrl` (camelCase) — the adapter normalizes the wire snake_case shape into the same camelCase convention every other field in `WorkflowTaskNodeData` already uses (`employeeName`, `runStatus`, etc.), so define `avatarAsset` on `WorkflowTaskNodeData` as `{ id: string; label: string; imageUrl: string; thumbnailUrl: string } | undefined` (camelCase), not a re-export of the wire type.

- [ ] **Step 2: Run test to verify it fails**

Run: `corepack pnpm --filter ./apps/web run test -- workflow-graph-adapter`
Expected: FAIL — `runTaskData.employeeRole` is `undefined`, not `"应用运维工程师·工程部"`

- [ ] **Step 3: Extend `WorkflowTaskNodeData` and populate it in `buildWorkflowGraphElements`**

In `apps/web/src/features/workflows/workflow-graph-adapter.ts`, replace the `WorkflowTaskNodeData` type (near the top of the file):

```ts
export type WorkflowTaskNodeData = {
  employeeName: string | undefined;
  expectedOutputs: unknown[];
  hasPendingDecision: boolean;
  requiresHumanApproval: boolean;
  riskLevel: string | undefined;
  runStatus: string | undefined;
  status: string;
  summary: string | undefined;
  task: ProjectTaskGraphNode;
  title: string;
};
```

with:

```ts
export type WorkflowTaskNodeAvatarAsset = {
  id: string;
  label: string;
  imageUrl: string;
  thumbnailUrl: string;
};

export type WorkflowTaskNodeData = {
  avatarAsset: WorkflowTaskNodeAvatarAsset | undefined;
  employeeName: string | undefined;
  employeeRole: string | undefined;
  expectedOutputs: unknown[];
  hasPendingDecision: boolean;
  requiresHumanApproval: boolean;
  riskLevel: string | undefined;
  runStatus: string | undefined;
  status: string;
  summary: string | undefined;
  task: ProjectTaskGraphNode;
  title: string;
};
```

Then, inside `buildWorkflowGraphElements`, find:

```ts
  const employeesById = new Map(
    graph.employees.map((employee) => [employee.digital_employee_id, employee.display_name]),
  );
```

and replace it with (keep the full employee record, not just the name):

```ts
  const employeesById = new Map(
    graph.employees.map((employee) => [employee.digital_employee_id, employee]),
  );
```

Then find the `taskNodes` construction's `data` block:

```ts
      data: {
        employeeName: task.assigned_digital_employee_id
          ? employeesById.get(task.assigned_digital_employee_id)
          : undefined,
        expectedOutputs: task.expected_outputs,
        hasPendingDecision: pendingDecisionsByTaskId.has(task.id),
        requiresHumanApproval: task.requires_human_approval,
        riskLevel: task.risk_level,
        runStatus: runStatusByTaskId.get(task.id),
        status: task.status,
        summary: task.summary,
        task,
        title: task.title,
      },
```

with:

```ts
      data: {
        avatarAsset: toWorkflowTaskNodeAvatarAsset(
          task.assigned_digital_employee_id
            ? employeesById.get(task.assigned_digital_employee_id)?.avatar_asset
            : undefined,
        ),
        employeeName: task.assigned_digital_employee_id
          ? employeesById.get(task.assigned_digital_employee_id)?.display_name
          : undefined,
        employeeRole: task.assigned_digital_employee_id
          ? employeesById.get(task.assigned_digital_employee_id)?.employee_role
          : undefined,
        expectedOutputs: task.expected_outputs,
        hasPendingDecision: pendingDecisionsByTaskId.has(task.id),
        requiresHumanApproval: task.requires_human_approval,
        riskLevel: task.risk_level,
        runStatus: runStatusByTaskId.get(task.id),
        status: task.status,
        summary: task.summary,
        task,
        title: task.title,
      },
```

Finally, add this helper near the other module-level helper functions (e.g. right after `taskNodeId`):

```ts
function toWorkflowTaskNodeAvatarAsset(
  asset: ProjectTaskGraphEmployee["avatar_asset"],
): WorkflowTaskNodeAvatarAsset | undefined {
  if (!asset) return undefined;
  return {
    id: asset.id,
    label: asset.label,
    imageUrl: asset.image_url,
    thumbnailUrl: asset.thumbnail_url,
  };
}
```

This needs `ProjectTaskGraphEmployee` added to the file's top import block. Replace:

```ts
import type { Edge, Node } from "@xyflow/react";

import type {
  ProjectDecisionRequest,
  ProjectTaskGraph,
  ProjectTaskGraphNode,
} from "@/lib/api/projects";
```

with:

```ts
import type { Edge, Node } from "@xyflow/react";

import type {
  ProjectDecisionRequest,
  ProjectTaskGraph,
  ProjectTaskGraphEmployee,
  ProjectTaskGraphNode,
} from "@/lib/api/projects";
```

- [ ] **Step 4: Run test to verify it passes**

Run: `corepack pnpm --filter ./apps/web run test -- workflow-graph-adapter`
Expected: PASS, including the two new assertions and every pre-existing assertion in the file (this is a purely additive change — no existing assertion's expected value changes)

- [ ] **Step 5: Write the failing card-rendering test**

Create `apps/web/src/features/workflows/components/workflow-task-node.test.tsx`:

```tsx
import { describe, expect, it } from "vitest";
import { render } from "vitest-browser-react";
import { ReactFlowProvider } from "@xyflow/react";
import { WorkflowTaskNode } from "./workflow-task-node";
import type { WorkflowTaskNodeData } from "../workflow-graph-adapter";
import type { ProjectTaskGraphNode } from "@/lib/api/projects";

const TEST_AVATAR_SRC =
  "data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 24 24'%3E%3Crect width='24' height='24' fill='%232F5FFF'/%3E%3C/svg%3E";

function makeTaskData(overrides: Partial<WorkflowTaskNodeData> = {}): WorkflowTaskNodeData {
  return {
    avatarAsset: undefined,
    employeeName: undefined,
    employeeRole: undefined,
    expectedOutputs: [],
    hasPendingDecision: false,
    requiresHumanApproval: false,
    riskLevel: undefined,
    runStatus: undefined,
    status: "planned",
    summary: "任务摘要",
    task: {} as ProjectTaskGraphNode,
    title: "任务标题",
    ...overrides,
  };
}

describe("WorkflowTaskNode", () => {
  it("renders the employee avatar image and role when present", async () => {
    const data = makeTaskData({
      employeeName: "应用运维工程师",
      employeeRole: "应用运维工程师·工程部",
      avatarAsset: {
        id: "avatar-1",
        label: "Adventurer 1",
        imageUrl: TEST_AVATAR_SRC,
        thumbnailUrl: TEST_AVATAR_SRC,
      },
    });
    const screen = await render(
      <ReactFlowProvider>
        <WorkflowTaskNode
          data={data}
          id="task:1"
          type="workflowTask"
          selected={false}
          dragging={false}
          zIndex={0}
          isConnectable={false}
          positionAbsoluteX={0}
          positionAbsoluteY={0}
        />
      </ReactFlowProvider>,
    );
    await expect.element(screen.getByText("应用运维工程师·工程部")).toBeInTheDocument();
    const image = screen.container.querySelector(`img[src="${TEST_AVATAR_SRC}"]`);
    expect(image).not.toBeNull();
  });

  it("falls back to a bot icon and no role line when the employee has no avatar/role", async () => {
    const data = makeTaskData({ employeeName: "需求分析师" });
    const screen = await render(
      <ReactFlowProvider>
        <WorkflowTaskNode
          data={data}
          id="task:1"
          type="workflowTask"
          selected={false}
          dragging={false}
          zIndex={0}
          isConnectable={false}
          positionAbsoluteX={0}
          positionAbsoluteY={0}
        />
      </ReactFlowProvider>,
    );
    await expect.element(screen.getByText("需求分析师")).toBeInTheDocument();
    expect(screen.container.querySelector("img")).toBeNull();
  });
});
```

- [ ] **Step 6: Run test to verify it fails**

Run: `corepack pnpm --filter ./apps/web run test -- workflow-task-node`
Expected: FAIL — `screen.getByText("应用运维工程师·工程部")` not found (component doesn't render `employeeRole` yet)

- [ ] **Step 7: Update the `WorkflowTaskNode` component**

In `apps/web/src/features/workflows/components/workflow-task-node.tsx`, add the avatar imports at the top:

```ts
import { Avatar, AvatarFallback, AvatarImage } from "@/components/ui/avatar";
```

Then replace the component body's header section:

```tsx
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0">
          <p className="line-clamp-2 text-sm font-bold tracking-normal text-v3-ink">
            {data.title}
          </p>
          <p className="mt-1 line-clamp-2 text-xs leading-5 text-v3-ink-2">
            {data.summary || "暂无任务摘要"}
          </p>
        </div>
        <StatusPill className="shrink-0" tone={taskStatusTone(data.status)}>
          {data.status}
        </StatusPill>
      </div>

      <div className="mt-3 flex flex-wrap gap-2">
        <StatusPill className="max-w-full" showDot={false} tone="mute">
          <Bot className="size-3.5 shrink-0" />
          <span className="truncate">{data.employeeName || "未分配"}</span>
        </StatusPill>
        {data.riskLevel ? (
```

with:

```tsx
      <div className="flex items-start gap-3">
        <Avatar className="size-9 shrink-0 border border-v3-line">
          {data.avatarAsset ? (
            <AvatarImage src={data.avatarAsset.thumbnailUrl} alt={data.employeeName || "数字员工"} />
          ) : null}
          <AvatarFallback>
            <Bot className="size-4" />
          </AvatarFallback>
        </Avatar>
        <div className="min-w-0 flex-1">
          <p className="truncate text-xs font-semibold text-v3-ink">
            {data.employeeName || "未分配"}
          </p>
          {data.employeeRole ? (
            <p className="truncate text-[11px] text-v3-ink-3">{data.employeeRole}</p>
          ) : null}
        </div>
        <StatusPill className="shrink-0" tone={taskStatusTone(data.status)}>
          {data.status}
        </StatusPill>
      </div>

      <div className="mt-2">
        <p className="line-clamp-2 text-sm font-bold tracking-normal text-v3-ink">
          {data.title}
        </p>
        <p className="mt-1 line-clamp-2 text-xs leading-5 text-v3-ink-2">
          {data.summary || "暂无任务摘要"}
        </p>
      </div>

      <div className="mt-3 flex flex-wrap gap-2">
        {data.riskLevel ? (
```

(The remainder of the `mt-3 flex flex-wrap gap-2` block — the `riskLevel`, `runStatus`, and `showHumanApproval` pills — is unchanged; only the employee-name pill that used to open that block is removed, since the employee name now lives in the avatar row above.)

- [ ] **Step 8: Run tests to verify they pass**

Run: `corepack pnpm --filter ./apps/web run test -- workflow-task-node workflow-graph-adapter`
Expected: PASS

- [ ] **Step 9: Run the full web test suite and typecheck to catch any other consumer**

Run: `corepack pnpm --filter ./apps/web run test`
Run: `corepack pnpm --filter ./apps/web run typecheck`
Expected: PASS — `WorkflowTaskNodeData`'s new fields are additive, and no other file constructs a `WorkflowTaskNodeData` object literal outside the adapter (only `buildWorkflowGraphElements` does)

- [ ] **Step 10: Commit**

```bash
git add apps/web/src/features/workflows/workflow-graph-adapter.ts apps/web/src/features/workflows/workflow-graph-adapter.test.ts apps/web/src/features/workflows/components/workflow-task-node.tsx apps/web/src/features/workflows/components/workflow-task-node.test.tsx
git commit -m "feat(web): show employee avatar and role on workflow task cards"
```

---

### Task 2: Plan graph layout adapter and stage header node

**Files:**
- Modify: `apps/web/src/features/workflows/workflow-graph-adapter.ts`
- Modify: `apps/web/src/features/workflows/components/workflow-task-node.tsx`
- Test: `apps/web/src/features/workflows/workflow-graph-adapter.test.ts`

**Interfaces:**
- Consumes: `WorkflowTaskNodeData` (Task 1), `ProjectTaskGraph` (`@/lib/api/projects`), the module-private helpers already in `workflow-graph-adapter.ts` (`taskNodeId`, `finiteStage`, `knownStageRange`, `groupDecisionsByTaskId`, `isPendingTaskDecision`, `normalizeStatus`, `ANIMATED_EDGE_STATUSES`).
- Produces:
  - `stageLabelNodeId(stageIndex: number): string`
  - `WorkflowStageLabelNodeData = { employeeCount: number; stageIndex: number; title: string; taskCount: number }`
  - `PlanWorkflowGraphElements = { nodes: (WorkflowTaskNode | WorkflowStageLabelNode)[]; edges: Edge[] }`
  - `buildPlanTaskGraphElements(graph: ProjectTaskGraph): PlanWorkflowGraphElements` — consumed by Task 3's canvas component.
  - `WorkflowStageLabelNode` React component — consumed by Task 3's canvas component's `nodeTypes` map.

This task also extracts the existing dependency-edge-building logic (currently inlined in `buildWorkflowGraphElements`) into a small shared helper, since `buildPlanTaskGraphElements` needs the identical logic. This is a pure refactor of already-tested code — Step 2 proves it doesn't change `buildWorkflowGraphElements`'s output.

- [ ] **Step 1: Extract the shared edge-building helper (refactor, not a behavior change)**

In `apps/web/src/features/workflows/workflow-graph-adapter.ts`, find the `return` statement at the end of `buildWorkflowGraphElements`:

```ts
  return {
    nodes: [...taskNodes, ...attachmentNodes],
    edges: graph.edges
      .filter(
        (edge) => taskIds.has(edge.blocker_task_id) && taskIds.has(edge.dependent_task_id),
      )
      .map((edge) => ({
        id: `edge:${edge.blocker_task_id}:${edge.dependent_task_id}`,
        source: taskNodeId(edge.blocker_task_id),
        target: taskNodeId(edge.dependent_task_id),
        type: "smoothstep",
        label: edge.edge_status,
        animated: ANIMATED_EDGE_STATUSES.has(normalizeStatus(edge.edge_status)),
      })),
  };
```

Replace it with:

```ts
  return {
    nodes: [...taskNodes, ...attachmentNodes],
    edges: buildTaskDependencyEdges(graph, taskIds, { includeLabel: true }),
  };
```

Then add this function near the other module-level helpers (e.g. right after `buildWorkflowGraphElements`):

```ts
function buildTaskDependencyEdges(
  graph: ProjectTaskGraph,
  taskIds: Set<string>,
  options: { includeLabel: boolean },
): Edge[] {
  return graph.edges
    .filter((edge) => taskIds.has(edge.blocker_task_id) && taskIds.has(edge.dependent_task_id))
    .map((edge) => ({
      id: `edge:${edge.blocker_task_id}:${edge.dependent_task_id}`,
      source: taskNodeId(edge.blocker_task_id),
      target: taskNodeId(edge.dependent_task_id),
      type: "smoothstep",
      label: options.includeLabel ? edge.edge_status : undefined,
      animated: ANIMATED_EDGE_STATUSES.has(normalizeStatus(edge.edge_status)),
    }));
}
```

- [ ] **Step 2: Run the existing adapter test suite to prove the refactor is behavior-preserving**

Run: `corepack pnpm --filter ./apps/web run test -- workflow-graph-adapter`
Expected: PASS — every existing test (edge ids, edge animation, orphaned-edge skipping) still passes unchanged, since `buildTaskDependencyEdges(graph, taskIds, { includeLabel: true })` produces byte-identical output to the inlined code it replaced

- [ ] **Step 3: Write the failing test for the new plan layout adapter**

First, update the two import statements at the top of `apps/web/src/features/workflows/workflow-graph-adapter.test.ts`. Replace:

```ts
import {
  buildWorkflowGraphElements,
  selectInitialWorkflowNodeId,
} from "./workflow-graph-adapter";
import type { WorkflowTaskNodeData } from "./workflow-graph-adapter";
```

with:

```ts
import {
  buildPlanTaskGraphElements,
  buildWorkflowGraphElements,
  selectInitialWorkflowNodeId,
  stageLabelNodeId,
} from "./workflow-graph-adapter";
import type { WorkflowStageLabelNodeData, WorkflowTaskNodeData } from "./workflow-graph-adapter";
```

Then add a new `describe` block immediately after the existing `describe("workflow graph adapter", ...)` block's closing `});` (i.e. right before the `function makeTask(...)` declaration at the bottom of the file):

```ts
describe("plan task graph adapter", () => {
  it("lays out each stage on a shared x=0 axis and wraps large stage rows", () => {
    const graph = makeGraph();
    graph.nodes = [
      makeTask("task-a", "planned", { stage_index: 0 }),
      makeTask("task-b", "planned", {
        assigned_digital_employee_id: "employee-1",
        stage_index: 1,
      }),
      makeTask("task-c", "planned", {
        assigned_digital_employee_id: "employee-1",
        stage_index: 1,
      }),
    ];
    graph.edges = [];
    graph.employees = [
      {
        digital_employee_id: "employee-1",
        display_name: "同一执行人",
        project_role: "executor",
        status: "active",
      },
    ];

    const result = buildPlanTaskGraphElements(graph);

    const stageZeroLabel = result.nodes.find((node) => node.id === stageLabelNodeId(0));
    const stageOneLabel = result.nodes.find((node) => node.id === stageLabelNodeId(1));
    const taskA = result.nodes.find((node) => node.id === "task:task-a");
    const taskB = result.nodes.find((node) => node.id === "task:task-b");
    const taskC = result.nodes.find((node) => node.id === "task:task-c");

    expect(stageZeroLabel).toBeDefined();
    expect(stageOneLabel).toBeDefined();
    expect((stageZeroLabel?.data as WorkflowStageLabelNodeData).taskCount).toBe(1);
    expect((stageOneLabel?.data as WorkflowStageLabelNodeData).taskCount).toBe(2);
    expect((stageOneLabel?.data as WorkflowStageLabelNodeData).employeeCount).toBe(1);

    // stage 0 has a single task: it sits on the shared center axis
    expect(taskA?.position.x).toBe(0);
    // stage 1 has two tasks: they straddle the same center axis symmetrically
    expect((taskB?.position.x ?? 0) + (taskC?.position.x ?? 0)).toBe(0);
    expect(taskB?.position.x).not.toBe(taskC?.position.x);

    // stage 1's row sits below stage 0's row
    expect(taskB?.position.y ?? 0).toBeGreaterThan(taskA?.position.y ?? 0);
    // the stage label sits above its own row's tasks
    expect(stageOneLabel?.position.y ?? 0).toBeLessThan(taskB?.position.y ?? 0);
  });

  it("falls back to a generated stage title when stage_summaries has no matching entry", () => {
    const graph = makeGraph();
    graph.nodes = [makeTask("task-a", "planned", { stage_index: 4 })];
    graph.edges = [];
    graph.employees = [];
    graph.stage_summaries = [];

    const result = buildPlanTaskGraphElements(graph);
    const label = result.nodes.find((node) => node.id === stageLabelNodeId(4));

    expect((label?.data as WorkflowStageLabelNodeData).title).toBe("第 5 阶段");
  });

  it("uses the stage title from stage_summaries when present", () => {
    const graph = makeGraph();
    graph.nodes = [makeTask("task-a", "planned", { stage_index: 0 })];
    graph.edges = [];
    graph.employees = [];
    graph.stage_summaries = [
      {
        stage_index: 0,
        title: "并行审查",
        total_nodes: 1,
        completed_nodes: 0,
        running_nodes: 0,
        waiting_human_nodes: 0,
        blocked_nodes: 0,
      },
    ];

    const result = buildPlanTaskGraphElements(graph);
    const label = result.nodes.find((node) => node.id === stageLabelNodeId(0));

    expect((label?.data as WorkflowStageLabelNodeData).title).toBe("并行审查");
  });

  it("does not label plan-view dependency edges", () => {
    const graph = makeGraph();
    graph.nodes = [
      makeTask("task-a", "planned", { stage_index: 0 }),
      makeTask("task-b", "planned", { stage_index: 1 }),
    ];
    graph.edges = [
      { blocker_task_id: "task-a", dependent_task_id: "task-b", edge_status: "planned" },
    ];
    graph.employees = [];

    const result = buildPlanTaskGraphElements(graph);

    expect(result.edges[0].label).toBeUndefined();
  });
});
```

- [ ] **Step 4: Run test to verify it fails**

Run: `corepack pnpm --filter ./apps/web run test -- workflow-graph-adapter`
Expected: FAIL — `buildPlanTaskGraphElements`/`stageLabelNodeId`/`WorkflowStageLabelNodeData` are undefined

- [ ] **Step 5: Implement `buildPlanTaskGraphElements`**

Add to `apps/web/src/features/workflows/workflow-graph-adapter.ts` (e.g. after `buildTaskDependencyEdges`):

```ts
export type WorkflowStageLabelNodeData = {
  employeeCount: number;
  stageIndex: number;
  taskCount: number;
  title: string;
};

type WorkflowStageLabelNode = Node<WorkflowStageLabelNodeData, "workflowStageLabel">;

export type PlanWorkflowGraphElements = {
  edges: Edge[];
  nodes: (WorkflowTaskNode | WorkflowStageLabelNode)[];
};

const PLAN_STAGE_ROW_HEIGHT = 220;
const PLAN_STAGE_LABEL_HEIGHT = 56;
const PLAN_TASK_COLUMN_WIDTH = 340;

export function stageLabelNodeId(stageIndex: number): string {
  return `stage-label:${stageIndex}`;
}

export function buildPlanTaskGraphElements(graph: ProjectTaskGraph): PlanWorkflowGraphElements {
  const taskIds = new Set(graph.nodes.map((task) => task.id));
  const employeesById = new Map(
    graph.employees.map((employee) => [employee.digital_employee_id, employee]),
  );
  const runStatusByTaskId = new Map(graph.runs.map((run) => [run.project_task_id, run.status]));
  const pendingDecisions = graph.decision_requests.filter(
    (decision) => isPendingTaskDecision(decision) && taskIds.has(decision.project_task_id ?? ""),
  );
  const pendingDecisionsByTaskId = groupDecisionsByTaskId(pendingDecisions);
  const stageTitleByIndex = new Map(
    (graph.stage_summaries ?? []).map((stage) => [stage.stage_index, stage.title]),
  );
  const stageRange = knownStageRange(graph.nodes);

  const tasksByStage = new Map<number, ProjectTaskGraphNode[]>();
  for (const task of graph.nodes) {
    const stage = finiteStage(task.stage_index) ?? stageRange.max;
    const list = tasksByStage.get(stage) ?? [];
    list.push(task);
    tasksByStage.set(stage, list);
  }
  const orderedStages = [...tasksByStage.keys()].sort((a, b) => a - b);

  const nodes: (WorkflowTaskNode | WorkflowStageLabelNode)[] = [];
  orderedStages.forEach((stage, stageOrder) => {
    const tasks = tasksByStage.get(stage) ?? [];
    const assignedEmployeeIds = new Set(
      tasks.flatMap((task) =>
        task.assigned_digital_employee_id ? [task.assigned_digital_employee_id] : [],
      ),
    );
    const rowY = stageOrder * PLAN_STAGE_ROW_HEIGHT;

    nodes.push({
      id: stageLabelNodeId(stage),
      type: "workflowStageLabel",
      draggable: false,
      selectable: false,
      position: { x: 0, y: rowY },
      data: {
        employeeCount: assignedEmployeeIds.size,
        stageIndex: stage,
        taskCount: tasks.length,
        title: stageTitleByIndex.get(stage) || `第 ${stage + 1} 阶段`,
      },
    });

    tasks.forEach((task, taskOrder) => {
      const employee = task.assigned_digital_employee_id
        ? employeesById.get(task.assigned_digital_employee_id)
        : undefined;
      const offset = (taskOrder - (tasks.length - 1) / 2) * PLAN_TASK_COLUMN_WIDTH;

      nodes.push({
        id: taskNodeId(task.id),
        type: "workflowTask",
        position: { x: offset, y: rowY + PLAN_STAGE_LABEL_HEIGHT },
        data: {
          avatarAsset: toWorkflowTaskNodeAvatarAsset(employee?.avatar_asset),
          employeeName: employee?.display_name,
          employeeRole: employee?.employee_role,
          expectedOutputs: task.expected_outputs,
          hasPendingDecision: pendingDecisionsByTaskId.has(task.id),
          requiresHumanApproval: task.requires_human_approval,
          riskLevel: task.risk_level,
          runStatus: runStatusByTaskId.get(task.id),
          status: task.status,
          summary: task.summary,
          task,
          title: task.title,
        },
      });
    });
  });

  return {
    edges: buildTaskDependencyEdges(graph, taskIds, { includeLabel: false }),
    nodes,
  };
}
```

`toWorkflowTaskNodeAvatarAsset` already exists from Task 1 — reused here as-is.

- [ ] **Step 6: Run test to verify it passes**

Run: `corepack pnpm --filter ./apps/web run test -- workflow-graph-adapter`
Expected: PASS for all four new tests plus every pre-existing test in the file

- [ ] **Step 7: Add the `WorkflowStageLabelNode` component**

In `apps/web/src/features/workflows/components/workflow-task-node.tsx`, replace the top of the file:

```ts
import type { Node, NodeProps } from "@xyflow/react";
import { Handle, Position } from "@xyflow/react";
import { Bot, ShieldCheck } from "lucide-react";
import { SoftCard, StatusPill } from "@/components/superteam";
import { cn } from "@/lib/utils";
import type {
  WorkflowAttachmentNodeData,
  WorkflowTaskNodeData,
} from "../workflow-graph-adapter";
import { taskStatusTone } from "./workflow-node-inspector";
```

with:

```ts
import type { Node, NodeProps } from "@xyflow/react";
import { Handle, Position } from "@xyflow/react";
import { Bot, GitBranch, ShieldCheck } from "lucide-react";
import { Avatar, AvatarFallback, AvatarImage } from "@/components/ui/avatar";
import { IconTile, SoftCard, StatusPill } from "@/components/superteam";
import { cn } from "@/lib/utils";
import type {
  WorkflowAttachmentNodeData,
  WorkflowStageLabelNodeData,
  WorkflowTaskNodeData,
} from "../workflow-graph-adapter";
import { taskStatusTone } from "./workflow-node-inspector";
```

Then add this component at the end of the file:

```tsx
export function WorkflowStageLabelNode({
  data,
}: NodeProps<Node<WorkflowStageLabelNodeData, "workflowStageLabel">>) {
  return (
    <SoftCard className="flex w-[260px] items-center gap-2 border border-v3-line px-3 py-2">
      <IconTile tone="artifact" size="sm">
        <GitBranch />
      </IconTile>
      <div className="min-w-0">
        <p className="truncate text-xs font-semibold text-v3-ink">{data.title}</p>
        <p className="truncate text-[11px] text-v3-ink-3">
          {`${data.taskCount} 个任务 · ${data.employeeCount} 位同事`}
        </p>
      </div>
    </SoftCard>
  );
}
```

- [ ] **Step 8: Run the full web test suite and typecheck**

Run: `corepack pnpm --filter ./apps/web run test`
Run: `corepack pnpm --filter ./apps/web run typecheck`
Expected: PASS

- [ ] **Step 9: Commit**

```bash
git add apps/web/src/features/workflows/workflow-graph-adapter.ts apps/web/src/features/workflows/workflow-graph-adapter.test.ts apps/web/src/features/workflows/components/workflow-task-node.tsx
git commit -m "feat(web): add vertical plan graph layout adapter and stage label node"
```

---

### Task 3: Plan graph canvas and page wiring

**Files:**
- Create: `apps/web/src/features/projects/components/plan-graph-canvas.tsx`
- Create: `apps/web/src/features/projects/components/plan-graph-canvas.test.tsx`
- Modify: `apps/web/src/features/projects/components/project-operational-detail.tsx`

**Interfaces:**
- Consumes: `buildPlanTaskGraphElements`, `WorkflowStageLabelNode` (Task 2), `WorkflowTaskNode` (existing), `ProjectTaskGraph` (`@/lib/api/projects`).
- Produces: `PlanGraphCanvas({ graph: ProjectTaskGraph }): JSX.Element` — a read-only canvas — plus a summary line above it, both wired into `project-operational-detail.tsx` in place of the current `<PlanTaskGraph>` list view.

`plan-task-graph.tsx`/`plan-task-graph.test.tsx` are left in place (untouched) — they may still be useful as a plain-list fallback or for other future non-graph surfaces per their own docstring's stated reuse intent; this task only changes what `project-operational-detail.tsx` renders for the "当前执行" section.

- [ ] **Step 1: Write the failing canvas test**

Create `apps/web/src/features/projects/components/plan-graph-canvas.test.tsx`:

```tsx
import type { ReactNode } from "react";
import { describe, expect, it, vi } from "vitest";
import { render } from "vitest-browser-react";
import type { ProjectTaskGraph } from "@/lib/api/projects";
import { PlanGraphCanvas } from "./plan-graph-canvas";

vi.mock("@xyflow/react", () => {
  type MockNode = {
    data?: {
      employeeCount?: number;
      employeeName?: string;
      employeeRole?: string;
      taskCount?: number;
      title?: string;
    };
    id: string;
  };

  type MockReactFlowProps = {
    children?: ReactNode;
    nodes?: MockNode[];
  };

  return {
    Background: () => null,
    Handle: () => null,
    Position: { Bottom: "bottom", Top: "top" },
    ReactFlow: ({ children, nodes = [] }: MockReactFlowProps) => (
      <div data-testid="plan-graph-canvas">
        {nodes.map((node) => (
          <div key={node.id}>
            {node.data?.title ? <p>{node.data.title}</p> : null}
            {node.data?.employeeName ? <p>{node.data.employeeName}</p> : null}
            {node.data?.employeeRole ? <p>{node.data.employeeRole}</p> : null}
            {typeof node.data?.taskCount === "number" &&
            typeof node.data?.employeeCount === "number" ? (
              <p>{`${node.data.taskCount} 个任务 · ${node.data.employeeCount} 位同事`}</p>
            ) : null}
          </div>
        ))}
        {children}
      </div>
    ),
  };
});

function makeGraph(): ProjectTaskGraph {
  return {
    nodes: [
      {
        id: "task-a",
        tenant_id: "tenant-1",
        project_id: "project-1",
        demand_id: "demand-1",
        title: "PR 上下文盘点",
        summary: "盘点改动文件和历史记录",
        status: "planned",
        assigned_digital_employee_id: "employee-1",
        requires_human_approval: false,
        stage_index: 0,
        expected_outputs: [],
        input_requirements: {},
        handoff_contract: {},
        planner_metadata: {},
      },
    ],
    edges: [],
    employees: [
      {
        digital_employee_id: "employee-1",
        display_name: "高乐驹",
        project_role: "executor",
        status: "active",
        employee_role: "代码库入职导师·工程部",
      },
    ],
    runs: [],
    execution_summaries: [],
    recent_events: [],
    decision_requests: [],
    stage_summaries: [
      {
        stage_index: 0,
        title: "第 1 阶段",
        total_nodes: 1,
        completed_nodes: 0,
        running_nodes: 0,
        waiting_human_nodes: 0,
        blocked_nodes: 0,
      },
    ],
  };
}

describe("PlanGraphCanvas", () => {
  it("renders a task card and its stage label from the graph", async () => {
    const screen = await render(<PlanGraphCanvas graph={makeGraph()} />);
    await expect.element(screen.getByText("PR 上下文盘点")).toBeInTheDocument();
    await expect.element(screen.getByText("高乐驹")).toBeInTheDocument();
    await expect.element(screen.getByText("代码库入职导师·工程部")).toBeInTheDocument();
    await expect.element(screen.getByText("第 1 阶段")).toBeInTheDocument();
    await expect.element(screen.getByText("1 个任务 · 1 位同事")).toBeInTheDocument();
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `corepack pnpm --filter ./apps/web run test -- plan-graph-canvas`
Expected: FAIL — `./plan-graph-canvas` does not exist

- [ ] **Step 3: Implement `PlanGraphCanvas`**

Create `apps/web/src/features/projects/components/plan-graph-canvas.tsx`:

```tsx
import { useMemo } from "react";
import { Background, ReactFlow, type NodeTypes } from "@xyflow/react";
import "@xyflow/react/dist/style.css";
import type { ProjectTaskGraph } from "@/lib/api/projects";
import {
  buildPlanTaskGraphElements,
  type PlanWorkflowGraphElements,
} from "@/features/workflows/workflow-graph-adapter";
import {
  WorkflowStageLabelNode,
  WorkflowTaskNode,
} from "@/features/workflows/components/workflow-task-node";

const nodeTypes = {
  workflowStageLabel: WorkflowStageLabelNode,
  workflowTask: WorkflowTaskNode,
} satisfies NodeTypes;

type PlanGraphCanvasProps = {
  graph: ProjectTaskGraph;
};

export function PlanGraphCanvas({ graph }: PlanGraphCanvasProps) {
  const elements = useMemo<PlanWorkflowGraphElements>(
    () => buildPlanTaskGraphElements(graph),
    [graph],
  );

  return (
    <div className="h-[620px] min-h-[420px] w-full min-w-0 overflow-hidden rounded-xl border bg-[linear-gradient(180deg,rgba(248,251,255,0.95),rgba(255,255,255,0.9))]">
      <ReactFlow
        edges={elements.edges}
        fitView
        maxZoom={1.25}
        minZoom={0.45}
        nodeTypes={nodeTypes}
        nodes={elements.nodes}
        nodesConnectable={false}
        nodesDraggable={false}
        panOnDrag
        zoomOnScroll
      >
        <Background gap={24} size={1} />
      </ReactFlow>
    </div>
  );
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `corepack pnpm --filter ./apps/web run test -- plan-graph-canvas`
Expected: PASS

- [ ] **Step 5: Add the plan summary line and wire the canvas into `project-operational-detail.tsx`**

In `apps/web/src/features/projects/components/project-operational-detail.tsx`, replace the import (line 64):

```ts
import { PlanTaskGraph } from "./plan-task-graph";
```

with:

```ts
import { PlanGraphCanvas } from "./plan-graph-canvas";
```

Then find the block (around line 387-400):

```tsx
          {taskGraph && taskGraph.nodes.length > 0 ? (
            <div className="grid gap-2">
              <div className="flex items-center gap-2 px-1">
                <ClipboardList className="size-4 text-v3-ink-2" />
                <h3 className="text-sm font-semibold tracking-normal">当前执行</h3>
                <StatusPill tone="mute">{`${taskGraph.nodes.length} 项`}</StatusPill>
              </div>
              <PlanTaskGraph
                nodes={taskGraph.nodes}
                edges={taskGraph.edges}
                employees={taskGraph.employees}
                stageSummaries={taskGraph.stage_summaries}
              />
            </div>
          ) : (
```

Replace it with:

```tsx
          {taskGraph && taskGraph.nodes.length > 0 ? (
            <div className="grid gap-2">
              <div className="flex items-center gap-2 px-1">
                <ClipboardList className="size-4 text-v3-ink-2" />
                <h3 className="text-sm font-semibold tracking-normal">当前执行</h3>
                <StatusPill tone="mute">
                  {planTaskGraphSummaryLabel(taskGraph)}
                </StatusPill>
              </div>
              <PlanGraphCanvas graph={taskGraph} />
            </div>
          ) : (
```

Then add this helper function at module scope, near the bottom of the file (after the last exported component):

```ts
function planTaskGraphSummaryLabel(graph: ProjectTaskGraph): string {
  const stageCount = new Set(graph.nodes.map((node) => node.stage_index ?? -1)).size;
  const assignedEmployeeIds = new Set(
    graph.nodes.flatMap((node) =>
      node.assigned_digital_employee_id ? [node.assigned_digital_employee_id] : [],
    ),
  );
  const employeeCount = assignedEmployeeIds.size;
  const tasksByStage = new Map<number, string[]>();
  for (const node of graph.nodes) {
    const stage = node.stage_index ?? -1;
    const list = tasksByStage.get(stage) ?? [];
    list.push(node.assigned_digital_employee_id ?? "");
    tasksByStage.set(stage, list);
  }
  const maxParallel = Math.max(
    0,
    ...[...tasksByStage.values()].map((ids) => new Set(ids.filter(Boolean)).size),
  );
  return `${employeeCount} 位同事 · 分 ${stageCount} 个阶段协作 · 最多 ${maxParallel} 人同时进行`;
}
```

`ProjectTaskGraph` is already imported in this file's `import type { ... } from "@/lib/api/projects";` block (it is, since `taskGraph` is already typed with it) — no new import needed, just reuse it in this function's signature.

- [ ] **Step 6: Run the full web test suite and typecheck**

Run: `corepack pnpm --filter ./apps/web run test`
Run: `corepack pnpm --filter ./apps/web run typecheck`
Expected: PASS. If `project-operational-detail.test.tsx` (or similar) asserts on the old `PlanTaskGraph` list markup (e.g. `依赖：` text, `[data-slot="v3-soft-card"]` counts) for the "当前执行" section specifically, update those assertions to match the new graph canvas's rendered text instead — check for such a test file first:

Run: `rg -n "当前执行" apps/web/src/features/projects/components -g "*.test.tsx"`

If any matches reference the replaced block, update them to assert on `PlanGraphCanvas`'s rendered output (task titles, stage labels, the new summary line) the same way `plan-graph-canvas.test.tsx` does, rather than on `PlanTaskGraph`-specific text like `依赖：`.

- [ ] **Step 7: Commit**

```bash
git add apps/web/src/features/projects/components/plan-graph-canvas.tsx apps/web/src/features/projects/components/plan-graph-canvas.test.tsx apps/web/src/features/projects/components/project-operational-detail.tsx
git commit -m "feat(web): render the plan task graph as a vertical diagram instead of a list"
```

---

## Done Criteria

- `corepack pnpm --filter ./apps/web run test` passes.
- `corepack pnpm --filter ./apps/web run typecheck` passes.
- Real end-to-end check (per this repo's completion rules — required before calling this done, not optional): with Control Plane and Web running (`scripts/dev-services.sh status`/`restart`), create or open a project whose demand has already produced a materialized (not-yet-running) task plan, navigate to its operational detail page, and visually confirm: tasks are laid out top-to-bottom by stage, each stage has a header showing its title, task count, and unique assigned-employee count; the page summary uses unique assigned employees rather than raw task count; and at least one task card shows a real employee avatar and role sourced from the live API response (not a placeholder/fallback icon) — this last point depends on `2026-07-02-project-task-graph-employee-identity.md` being deployed first.
