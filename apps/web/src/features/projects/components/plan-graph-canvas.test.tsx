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

  type MockEdge = {
    id: string;
    markerEnd?: unknown;
    style?: unknown;
  };

  type MockReactFlowProps = {
    children?: ReactNode;
    edges?: MockEdge[];
    defaultViewport?: {
      x: number;
      y: number;
      zoom: number;
    };
    fitView?: boolean;
    minZoom?: number;
    nodes?: MockNode[];
    proOptions?: {
      hideAttribution?: boolean;
    };
  };

  return {
    Background: () => null,
    Handle: () => null,
    MarkerType: { ArrowClosed: "arrowclosed" },
    Position: { Bottom: "bottom", Top: "top" },
    ReactFlow: ({
      children,
      defaultViewport,
      edges = [],
      fitView,
      minZoom,
      nodes = [],
      proOptions,
    }: MockReactFlowProps) => (
      <div
        data-edge-count={String(edges.length)}
        data-fit-view={String(Boolean(fitView))}
        data-hide-attribution={String(Boolean(proOptions?.hideAttribution))}
        data-min-zoom={String(minZoom ?? "")}
        data-testid="mock-react-flow"
        data-viewport-zoom={String(defaultViewport?.zoom ?? "")}
      >
        {edges.map((edge) => (
          <p
            data-has-marker={String(Boolean(edge.markerEnd))}
            data-has-style={String(Boolean(edge.style))}
            data-testid={`plan-graph-edge-${edge.id}`}
            key={edge.id}
          >
            {edge.id}
          </p>
        ))}
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
        planner_metadata: {},
      },
    ],
    edges: [
      {
        blocker_task_id: "task-a",
        dependent_task_id: "task-b",
        edge_status: "planned",
      },
    ],
    employees: [
      {
        digital_employee_id: "employee-1",
        display_name: "高乐驹",
        project_role: "executor",
        status: "active",
        employee_role: "代码库入职导师·工程部",
      },
      {
        digital_employee_id: "employee-2",
        display_name: "安特妍",
        project_role: "executor",
        status: "active",
        employee_role: "安全工程师·工程部",
      },
    ],
    runs: [],
    execution_summaries: [],
    recent_events: [],
    decision_requests: [],
    blocking_facts: [],
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
      {
        stage_index: 1,
        title: "第 2 阶段",
        total_nodes: 1,
        completed_nodes: 0,
        running_nodes: 0,
        waiting_human_nodes: 1,
        blocked_nodes: 0,
      },
    ],
  };
}

describe("PlanGraphCanvas", () => {
  it("renders a task card and its stage label from the graph", async () => {
    const screen = await render(<PlanGraphCanvas graph={makeGraph()} />);

    await expect.element(screen.getByText("PR 上下文盘点")).toBeInTheDocument();
    await expect
      .element(screen.getByTestId("mock-react-flow"))
      .toHaveAttribute("data-hide-attribution", "true");
    await expect
      .element(screen.getByTestId("mock-react-flow"))
      .toHaveAttribute("data-fit-view", "false");
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
      .element(screen.getByTestId("plan-graph-edge-edge:task-a:task-b"))
      .toHaveAttribute("data-has-marker", "true");
    await expect
      .element(screen.getByTestId("plan-graph-edge-edge:task-a:task-b"))
      .toHaveAttribute("data-has-style", "true");
    await expect.element(screen.getByText("高乐驹")).toBeInTheDocument();
    await expect.element(screen.getByText("代码库入职导师·工程部")).toBeInTheDocument();
    await expect.element(screen.getByText("安全风险审查")).toBeInTheDocument();
    await expect.element(screen.getByText("安特妍")).toBeInTheDocument();
    await expect.element(screen.getByText("安全工程师·工程部")).toBeInTheDocument();
    await expect.element(screen.getByText("第 1 阶段")).toBeInTheDocument();
    await expect.element(screen.getByText("第 2 阶段")).toBeInTheDocument();
    expect(screen.container.textContent).toContain("1 个任务 · 1 位同事");
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

    const screen = await render(<PlanGraphCanvas graph={graph} />);

    await expect
      .element(screen.getByTestId("plan-graph-canvas"))
      .toHaveAttribute("data-stage-count", "3");
    await expect
      .element(screen.getByTestId("plan-graph-canvas"))
      .toHaveAttribute("data-task-count", "4");
    expect(screen.getByTestId("plan-graph-canvas").element().clientHeight).toBeGreaterThan(1300);
  });

  it("grows the desktop canvas for a ten-task wrapped plan stage", async () => {
    const graph = makeGraph();
    graph.nodes = Array.from({ length: 10 }, (_, index) => ({
      ...graph.nodes[0],
      id: `task-${index + 1}`,
      assigned_digital_employee_id: `employee-${index + 1}`,
      stage_index: 0,
      title: `计划任务 ${index + 1}`,
    }));
    graph.edges = [];
    graph.employees = graph.nodes.map((task, index) => ({
      digital_employee_id: task.assigned_digital_employee_id ?? `employee-${index + 1}`,
      display_name: `数字员工 ${index + 1}`,
      project_role: "executor",
      status: "active",
      employee_role: "通用工程执行",
    }));
    graph.stage_summaries = [
      {
        stage_index: 0,
        title: "并行规划执行",
        total_nodes: 10,
        completed_nodes: 0,
        running_nodes: 0,
        waiting_human_nodes: 0,
        blocked_nodes: 0,
      },
    ];

    const screen = await render(<PlanGraphCanvas graph={graph} />);

    await expect.element(screen.getByText("计划任务 10")).toBeInTheDocument();
    await expect
      .element(screen.getByTestId("plan-graph-canvas"))
      .toHaveAttribute("data-stage-count", "1");
    await expect
      .element(screen.getByTestId("plan-graph-canvas"))
      .toHaveAttribute("data-task-count", "10");
    expect(screen.getByTestId("plan-graph-canvas").element().clientHeight).toBeGreaterThan(2100);
  });
});
