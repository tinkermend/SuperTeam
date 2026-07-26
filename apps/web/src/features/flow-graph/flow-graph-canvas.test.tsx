import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { render } from "vitest-browser-react";
import type { ProjectTaskGraph } from "@/lib/api/projects";
import { FlowGraphCanvas } from "./flow-graph-canvas";

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
    parentId?: string;
  };

  type MockEdge = {
    data?: Record<string, unknown>;
    id: string;
    label?: ReactNode;
    type?: string;
  };

  type MockReactFlowProps = {
    children?: ReactNode;
    edges?: MockEdge[];
    defaultViewport?: {
      x: number;
      y: number;
      zoom: number;
    };
    minZoom?: number;
    nodes?: MockNode[];
    onEdgeClick?: (event: unknown, edge: MockEdge) => void;
    onNodeClick?: (event: unknown, node: MockNode) => void;
    proOptions?: {
      hideAttribution?: boolean;
    };
  };

  return {
    Background: () => null,
    BaseEdge: () => null,
    Controls: () => null,
    Handle: () => null,
    MiniMap: () => null,
    Position: { Bottom: "bottom", Top: "top" },
    getSmoothStepPath: () => ["M 0 0 L 10 10", 5, 5],
    ReactFlow: ({
      children,
      defaultViewport,
      edges = [],
      minZoom,
      nodes = [],
      onEdgeClick,
      onNodeClick,
      proOptions
}: MockReactFlowProps) => (
      <div
        data-edge-count={String(edges.length)}
        data-hide-attribution={String(Boolean(proOptions?.hideAttribution))}
        data-min-zoom={String(minZoom ?? "")}
        data-testid="mock-react-flow"
        data-viewport-zoom={String(defaultViewport?.zoom ?? "")}
      >
        {edges.map((edge) => (
          <button
            data-edge-activity={String(edge.data?.activity ?? "")}
            data-edge-degraded={String(edge.data?.scaleDegraded ?? "")}
            data-edge-type={edge.type ?? ""}
            data-testid={`flow-graph-edge-${edge.id}`}
            key={edge.id}
            onClick={(event) => onEdgeClick?.(event, edge)}
            type="button"
          >
            {edge.label}
          </button>
        ))}
        {nodes.map((node) => (
          <button
            key={node.id}
            onClick={(event) => onNodeClick?.(event, node)}
            type="button"
          >
            {node.data?.title ? <span>{node.data.title}</span> : null}
            {node.data?.employeeName ? <span>{node.data.employeeName}</span> : null}
            {node.data?.employeeRole ? <span>{node.data.employeeRole}</span> : null}
          </button>
        ))}
        {children}
      </div>
    )
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
        planner_metadata: {}
},
      {
        id: "task-b",
        tenant_id: "tenant-1",
        project_id: "project-1",
        demand_id: "demand-1",
        title: "安全风险审查",
        summary: "确认历史数据持久化风险",
        status: "waiting_human",
        assigned_digital_employee_id: "employee-2",
        requires_human_approval: true,
        stage_index: 1,
        expected_outputs: [],
        input_requirements: {},
        handoff_contract: {},
        planner_metadata: {}
},
    ],
    edges: [
      {
        blocker_task_id: "task-a",
        dependent_task_id: "task-b",
        edge_status: "planned"
},
    ],
    employees: [
      {
        digital_employee_id: "employee-1",
        display_name: "高乐驹",
        project_role: "executor",
        status: "active",
        employee_role: "代码库入职导师·工程部"
},
      {
        digital_employee_id: "employee-2",
        display_name: "安特妍",
        project_role: "executor",
        status: "active",
        employee_role: "安全工程师·工程部"
},
    ],
    runs: [],
    execution_summaries: [],
    recent_events: [],
    decision_requests: [],
    blocking_facts: []
};
}

/**
 * 链式大图：taskCount 个 running 任务 + (taskCount-1) 条边；stageCount 控制阶段
 * 标签数，用于精确凑渲染元素总数（任务+阶段标签+边）踩 P2-S 阈值边界。
 */
function makeScaleGraph(taskCount: number, stageCount: 1 | 2): ProjectTaskGraph {
  const graph = makeGraph();
  graph.nodes = Array.from({ length: taskCount }, (_, index) => ({
    ...graph.nodes[0],
    id: `task-${index + 1}`,
    title: `扩容任务 ${index + 1}`,
    status: "running",
    assigned_digital_employee_id: undefined,
    stage_index: stageCount === 1 ? 0 : index < taskCount / 2 ? 0 : 1
}));
  graph.edges = graph.nodes.slice(1).map((task, index) => ({
    blocker_task_id: graph.nodes[index].id,
    dependent_task_id: task.id,
    edge_status: "unblocked"
}));
  graph.employees = [];
  return graph;
}

/** 覆写 matchMedia 控制 prefers-reduced-motion；返回还原函数。 */
function stubReducedMotion(matches: boolean): () => void {
  const original = window.matchMedia;
  window.matchMedia = ((query: string) =>
    ({
      addEventListener: () => undefined,
      addListener: () => undefined,
      dispatchEvent: () => false,
      matches: query.includes("prefers-reduced-motion") ? matches : false,
      media: query,
      onchange: null,
      removeEventListener: () => undefined,
      removeListener: () => undefined
}) as MediaQueryList) as typeof window.matchMedia;
  return () => {
    window.matchMedia = original;
  };
}

let restoreMatchMedia: (() => void) | undefined;

afterEach(() => {
  restoreMatchMedia?.();
  restoreMatchMedia = undefined;
});

function renderCanvas(
  graph: ProjectTaskGraph,
  onNodeOpen = vi.fn(),
  live?: boolean,
) {
  return render(
    <FlowGraphCanvas
      graph={graph}
      live={live}
      onNodeOpen={onNodeOpen}
      onSelectedNodeChange={vi.fn()}
      selectedNodeId={undefined}
    />,
  );
}

describe("FlowGraphCanvas", () => {
  it("renders task cards, Chinese edge labels and canvas config from the graph", async () => {
    const screen = await renderCanvas(makeGraph());

    await expect.element(screen.getByText("PR 上下文盘点")).toBeInTheDocument();
    await expect.element(screen.getByText("安全风险审查")).toBeInTheDocument();
    await expect.element(screen.getByText("高乐驹")).toBeInTheDocument();
    await expect.element(screen.getByText("安特妍")).toBeInTheDocument();
    await expect
      .element(screen.getByTestId("mock-react-flow"))
      .toHaveAttribute("data-hide-attribution", "true");
    await expect
      .element(screen.getByTestId("mock-react-flow"))
      .toHaveAttribute("data-viewport-zoom", "1");
    await expect
      .element(screen.getByTestId("mock-react-flow"))
      .toHaveAttribute("data-min-zoom", "0.65");
    await expect
      .element(screen.getByTestId("mock-react-flow"))
      .toHaveAttribute("data-edge-count", "1");
    await expect
      .element(screen.getByTestId("flow-graph-edge-edge:task-a:task-b"))
      .toHaveTextContent("已计划");
  });

  it("keeps smoothstep edges and no live surface for existing consumers by default", async () => {
    const screen = await renderCanvas(makeGraph());

    await expect
      .element(screen.getByTestId("flow-graph-canvas"))
      .toHaveAttribute("data-live", "false");
    await expect
      .element(screen.getByTestId("flow-graph-edge-edge:task-a:task-b"))
      .toHaveAttribute("data-edge-type", "smoothstep");
    // 非 live 点边不弹交接对照。
    await screen.getByTestId("flow-graph-edge-edge:task-a:task-b").click();
    expect(
      screen.container.ownerDocument.querySelector(
        '[data-testid="flow-handoff-overlay"]',
      ),
    ).toBeNull();
  });

  it("switches edges to flowLive and opens the handoff overlay on edge click in live mode", async () => {
    const screen = await renderCanvas(makeGraph(), vi.fn(), true);

    await expect
      .element(screen.getByTestId("flow-graph-canvas"))
      .toHaveAttribute("data-live", "true");
    const edge = screen.getByTestId("flow-graph-edge-edge:task-a:task-b");
    await expect.element(edge).toHaveAttribute("data-edge-type", "flowLive");
    await expect.element(edge).toHaveAttribute("data-edge-activity", "idle");

    await edge.click();
    await expect.element(screen.getByText("交接对照")).toBeInTheDocument();
    await expect
      .element(screen.getByText("PR 上下文盘点（高乐驹）"))
      .toBeInTheDocument();
  });

  it("marks the live canvas scale-degraded above the animation element threshold", async () => {
    // 20 任务 + 2 阶段标签 + 19 条边 = 41 元素 > 40（LIVE_ANIMATION_MAX_ELEMENTS）。
    const screen = await renderCanvas(makeScaleGraph(20, 2), vi.fn(), true);

    await expect
      .element(screen.getByTestId("flow-graph-canvas"))
      .toHaveAttribute("data-live-degraded", "scale");
    await expect
      .element(screen.getByTestId("flow-graph-edge-edge:task-1:task-2"))
      .toHaveAttribute("data-edge-degraded", "true");
  });

  it("keeps the live canvas unmarked and edges undegraded at exactly the threshold", async () => {
    // 20 任务 + 1 阶段标签 + 19 条边 = 40 元素，阈值内零变化。
    const screen = await renderCanvas(makeScaleGraph(20, 1), vi.fn(), true);

    const canvas = screen.getByTestId("flow-graph-canvas");
    await expect.element(canvas).toHaveAttribute("data-live", "true");
    expect(canvas.element().hasAttribute("data-live-degraded")).toBe(false);
    await expect
      .element(screen.getByTestId("flow-graph-edge-edge:task-1:task-2"))
      .toHaveAttribute("data-edge-degraded", "");
  });

  it("marks motion degradation under reduced motion within the threshold", async () => {
    restoreMatchMedia = stubReducedMotion(true);
    const screen = await renderCanvas(makeGraph(), vi.fn(), true);

    await expect
      .element(screen.getByTestId("flow-graph-canvas"))
      .toHaveAttribute("data-live-degraded", "motion");
  });

  it("lets the scale marker win when reduced motion and a large graph coexist", async () => {
    restoreMatchMedia = stubReducedMotion(true);
    const screen = await renderCanvas(makeScaleGraph(20, 2), vi.fn(), true);

    await expect
      .element(screen.getByTestId("flow-graph-canvas"))
      .toHaveAttribute("data-live-degraded", "scale");
  });

  it("adds no degradation marker outside live mode even for large graphs", async () => {
    restoreMatchMedia = stubReducedMotion(true);
    const screen = await renderCanvas(makeScaleGraph(20, 2));

    await expect
      .element(screen.getByTestId("flow-graph-canvas"))
      .toHaveAttribute("data-live", "false");
    expect(
      screen.getByTestId("flow-graph-canvas").element().hasAttribute("data-live-degraded"),
    ).toBe(false);
  });

  it("reports node opens for task nodes when clicked", async () => {
    const onNodeOpen = vi.fn();
    const screen = await renderCanvas(makeGraph(), onNodeOpen);

    const button = screen.getByText("PR 上下文盘点");
    await button.click();

    expect(onNodeOpen).toHaveBeenCalledWith("task:task-a");
  });

  it("sizes the desktop canvas to avoid clipping the final stage", async () => {
    const graph = makeGraph();
    graph.nodes = [
      { ...graph.nodes[0], id: "task-a", stage_index: 0 },
      { ...graph.nodes[0], id: "task-b", stage_index: 1 },
      { ...graph.nodes[1], id: "task-c", stage_index: 1 },
      { ...graph.nodes[1], id: "task-d", stage_index: 2 },
    ];
    graph.edges = [
      { blocker_task_id: "task-a", dependent_task_id: "task-b", edge_status: "planned" },
      { blocker_task_id: "task-a", dependent_task_id: "task-c", edge_status: "planned" },
      { blocker_task_id: "task-b", dependent_task_id: "task-d", edge_status: "planned" },
      { blocker_task_id: "task-c", dependent_task_id: "task-d", edge_status: "planned" },
    ];

    const screen = await renderCanvas(graph);

    await expect
      .element(screen.getByTestId("flow-graph-canvas"))
      .toHaveAttribute("data-stage-count", "3");
    await expect
      .element(screen.getByTestId("flow-graph-canvas"))
      .toHaveAttribute("data-task-count", "4");
    expect(
      screen.getByTestId("flow-graph-canvas").element().clientHeight,
    ).toBeGreaterThan(1300);
  });

  it("grows the desktop canvas for a ten-task wrapped stage", async () => {
    const graph = makeGraph();
    graph.nodes = Array.from({ length: 10 }, (_, index) => ({
      ...graph.nodes[0],
      id: `task-${index + 1}`,
      assigned_digital_employee_id: `employee-${index + 1}`,
      stage_index: 0,
      title: `计划任务 ${index + 1}`
}));
    graph.edges = [];
    graph.employees = graph.nodes.map((task, index) => ({
      digital_employee_id: task.assigned_digital_employee_id ?? `employee-${index + 1}`,
      display_name: `数字员工 ${index + 1}`,
      project_role: "executor",
      status: "active",
      employee_role: "通用工程执行"
}));

    const screen = await renderCanvas(graph);

    await expect.element(screen.getByText("计划任务 10")).toBeInTheDocument();
    await expect
      .element(screen.getByTestId("flow-graph-canvas"))
      .toHaveAttribute("data-stage-count", "1");
    await expect
      .element(screen.getByTestId("flow-graph-canvas"))
      .toHaveAttribute("data-task-count", "10");
    expect(
      screen.getByTestId("flow-graph-canvas").element().clientHeight,
    ).toBeGreaterThan(2100);
  });
});
