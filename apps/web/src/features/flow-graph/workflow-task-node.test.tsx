import { ReactFlowProvider } from "@xyflow/react";
import { describe, expect, it } from "vitest";
import { render } from "vitest-browser-react";
import type { ProjectTaskGraphNode } from "@/lib/api/projects";
import type { WorkflowTaskNodeData } from "./flow-graph-adapter";
import { WorkflowStageLabelNode, WorkflowTaskNode } from "./workflow-task-node";

const TEST_AVATAR_SRC =
  "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAFgwJ/lm9qWAAAAABJRU5ErkJggg==";

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
    runStartedAt: undefined,
    runFinishedAt: undefined,
    showTiming: false,
    status: "planned",
    summary: "任务摘要",
    task: {} as ProjectTaskGraphNode,
    title: "任务标题",
    ...overrides
};
}

function renderTaskNode(data: WorkflowTaskNodeData) {
  return render(
    <ReactFlowProvider>
      <WorkflowTaskNode
        data={data}
        deletable={false}
        dragging={false}
        draggable={false}
        id="task:1"
        isConnectable={false}
        positionAbsoluteX={0}
        positionAbsoluteY={0}
        selectable={false}
        selected={false}
        type="workflowTask"
        zIndex={0}
      />
    </ReactFlowProvider>,
  );
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
        thumbnailUrl: TEST_AVATAR_SRC
}
});

    const screen = await render(
      <ReactFlowProvider>
        <WorkflowTaskNode
          data={data}
          deletable={false}
          dragging={false}
          draggable={false}
          id="task:1"
          isConnectable={false}
          positionAbsoluteX={0}
          positionAbsoluteY={0}
          selectable={false}
          selected={false}
          type="workflowTask"
          zIndex={0}
        />
      </ReactFlowProvider>,
    );

    await expect.element(screen.getByText("应用运维工程师·工程部")).toBeInTheDocument();
    await expect.element(screen.getByAltText("应用运维工程师")).toBeInTheDocument();
    await expect
      .element(screen.getByAltText("应用运维工程师"))
      .toHaveClass(/size-full/);
    await expect
      .element(screen.container.querySelector<HTMLElement>('[data-slot="avatar"]'))
      .toHaveClass(/size-14/);
  });

  it("renders known employee role identifiers as Chinese labels", async () => {
    const data = makeTaskData({
      employeeName: "通用执行员",
      employeeRole: "general_engineer"
});

    const screen = await render(
      <ReactFlowProvider>
        <WorkflowTaskNode
          data={data}
          deletable={false}
          dragging={false}
          draggable={false}
          id="task:1"
          isConnectable={false}
          positionAbsoluteX={0}
          positionAbsoluteY={0}
          selectable={false}
          selected={false}
          type="workflowTask"
          zIndex={0}
        />
      </ReactFlowProvider>,
    );

    await expect.element(screen.getByText("通用工程执行")).toBeInTheDocument();
    expect(screen.container.textContent).not.toContain("general_engineer");
  });

  it("falls back to a bot icon and no role line when the employee has no avatar/role", async () => {
    const data = makeTaskData({ employeeName: "需求分析师" });

    const screen = await render(
      <ReactFlowProvider>
        <WorkflowTaskNode
          data={data}
          deletable={false}
          dragging={false}
          draggable={false}
          id="task:1"
          isConnectable={false}
          positionAbsoluteX={0}
          positionAbsoluteY={0}
          selectable={false}
          selected={false}
          type="workflowTask"
          zIndex={0}
        />
      </ReactFlowProvider>,
    );

    await expect.element(screen.getByText("需求分析师")).toBeInTheDocument();
    expect(screen.container.querySelector("img")).toBeNull();
  });

  it("renders task and run statuses in Chinese labels", async () => {
    const data = makeTaskData({
      runStatus: "queued",
      status: "waiting_human"
});

    const screen = await render(
      <ReactFlowProvider>
        <WorkflowTaskNode
          data={data}
          deletable={false}
          dragging={false}
          draggable={false}
          id="task:1"
          isConnectable={false}
          positionAbsoluteX={0}
          positionAbsoluteY={0}
          selectable={false}
          selected={false}
          type="workflowTask"
          zIndex={0}
        />
      </ReactFlowProvider>,
    );

    await expect.element(screen.getByText("待人工确认")).toBeInTheDocument();
    await expect.element(screen.getByText("运行 排队中")).toBeInTheDocument();
    expect(screen.container.textContent).not.toContain("waiting_human");
    expect(screen.container.textContent).not.toContain("Run queued");
  });

  it("hides the timing section by default so existing consumers stay unchanged", async () => {
    const screen = await renderTaskNode(
      makeTaskData({
        runStartedAt: "2026-07-27T02:00:00Z",
        runFinishedAt: "2026-07-27T02:12:00Z",
        status: "completed"
}),
    );

    expect(
      screen.container.querySelector('[data-testid="task-node-timing"]'),
    ).toBeNull();
  });

  it("shows start/end and duration for a finished run in live mode", async () => {
    const screen = await renderTaskNode(
      makeTaskData({
        runStartedAt: "2026-07-27T02:00:00Z",
        runFinishedAt: "2026-07-27T02:12:00Z",
        showTiming: true,
        status: "completed"
}),
    );

    const timing = screen.getByTestId("task-node-timing");
    await expect.element(timing).toBeInTheDocument();
    await expect.element(timing.getByText("起")).toBeInTheDocument();
    await expect.element(timing.getByText("止")).toBeInTheDocument();
    await expect.element(timing.getByText("耗时")).toBeInTheDocument();
    expect(timing.element().textContent).toContain("12 分钟");
    expect(timing.element().textContent).not.toContain("已运行");
  });

  it("shows a rolling elapsed label for a running node in live mode", async () => {
    const startedAt = new Date(Date.now() - 5 * 60 * 1000 - 10_000).toISOString();
    const screen = await renderTaskNode(
      makeTaskData({
        runStartedAt: startedAt,
        showTiming: true,
        status: "running"
}),
    );

    const timing = screen.getByTestId("task-node-timing");
    await expect.element(timing).toBeInTheDocument();
    expect(timing.element().textContent).toContain("已运行 5 分钟");
    expect(timing.element().textContent).not.toContain("耗时");
  });

  it("renders no timing section in live mode when the task never started", async () => {
    const screen = await renderTaskNode(
      makeTaskData({ showTiming: true, status: "planned" }),
    );

    expect(
      screen.container.querySelector('[data-testid="task-node-timing"]'),
    ).toBeNull();
  });

  it("renders stage labels as compact centered pills for connector clearance", async () => {
    const screen = await render(
      <ReactFlowProvider>
        <WorkflowStageLabelNode
          data={{
            employeeCount: 2,
            stageIndex: 1,
            taskCount: 2,
            title: "并行审查"
}}
          deletable={false}
          dragging={false}
          draggable={false}
          id="stage-label:1"
          isConnectable={false}
          positionAbsoluteX={0}
          positionAbsoluteY={0}
          selectable={false}
          selected={false}
          type="workflowStageLabel"
          zIndex={0}
        />
      </ReactFlowProvider>,
    );

    await expect.element(screen.getByText("并行审查")).toBeInTheDocument();
    await expect
      .element(screen.container.querySelector<HTMLElement>('[data-testid="workflow-stage-label"]'))
      .toHaveClass(/w-\[168px\]/);
    await expect
      .element(screen.container.querySelector<HTMLElement>('[data-testid="workflow-stage-label"]'))
      .toHaveClass(/rounded-full/);
  });
});
