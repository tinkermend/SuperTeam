import { describe, expect, it, vi } from "vitest";
import { render } from "vitest-browser-react";
import type { ProjectTaskGraph } from "@/lib/api/projects";
import type { FlowLiveEdgeData } from "./flow-graph-adapter";
import { FlowHandoffOverlay } from "./flow-handoff-overlay";

function makeGraph(): ProjectTaskGraph {
  return {
    blocking_facts: [],
    decision_requests: [],
    edges: [
      {
        blocker_task_id: "task-scan",
        dependent_task_id: "task-verify",
        edge_status: "unblocked"
},
    ],
    employees: [
      {
        digital_employee_id: "employee-scan",
        display_name: "开发-小王",
        project_role: "executor",
        status: "active"
},
    ],
    execution_summaries: [
      {
        artifact_refs: ["file-stats.json"],
        conclusion: "扫描完成，统计覆盖率 99.4%",
        confidence_factors: {},
        created_at: "2026-07-27T03:20:00Z",
        digital_employee_id: "employee-scan",
        evidence_refs: [],
        id: "summary-1",
        missing_information: [],
        project_id: "project-1",
        project_task_id: "task-scan",
        requires_human_review: false,
        tenant_id: "tenant-1"
},
    ],
    nodes: [
      {
        assigned_digital_employee_id: "employee-scan",
        demand_id: "demand-1",
        expected_outputs: ["file-stats.json", "scan-log.txt"],
        handoff_contract: {
          acceptance_criteria: ["统计覆盖 ≥ 99% 的仓库文件", "扫描日志无未处理错误"]
},
        id: "task-scan",
        input_requirements: {},
        planner_metadata: {},
        project_id: "project-1",
        requires_human_approval: false,
        stage_index: 0,
        status: "completed",
        summary: "扫描项目目录并统计文件",
        tenant_id: "tenant-1",
        title: "扫描项目目录"
},
      {
        demand_id: "demand-1",
        expected_outputs: [],
        handoff_contract: {},
        id: "task-verify",
        input_requirements: {},
        planner_metadata: {},
        project_id: "project-1",
        requires_human_approval: false,
        stage_index: 1,
        status: "running",
        tenant_id: "tenant-1",
        title: "校验统计结果"
},
    ],
    recent_events: [],
    runs: []
};
}

const edge: FlowLiveEdgeData = {
  activity: "flowing",
  blockerTaskId: "task-scan",
  dependentTaskId: "task-verify"
};

describe("FlowHandoffOverlay", () => {
  it("shows the handoff contract against the actual conclusion with delivery time", async () => {
    const screen = await render(
      <FlowHandoffOverlay edge={edge} graph={makeGraph()} onClose={vi.fn()} />,
    );

    await expect.element(screen.getByText("交接对照")).toBeInTheDocument();
    // 交接双方：任务名 +（员工名）指称，不裸 UUID。
    await expect
      .element(screen.getByText("扫描项目目录（开发-小王）"))
      .toBeInTheDocument();
    await expect.element(screen.getByText("校验统计结果")).toBeInTheDocument();

    // 左列：契约 = expected_outputs + acceptance_criteria。
    const contract = screen.getByTestId("handoff-contract-column");
    await expect.element(contract.getByText("file-stats.json")).toBeInTheDocument();
    await expect.element(contract.getByText("scan-log.txt")).toBeInTheDocument();
    await expect
      .element(contract.getByText("统计覆盖 ≥ 99% 的仓库文件"))
      .toBeInTheDocument();

    // 右列：实际执行结论 + 产出引用 + 交付时间。
    const actual = screen.getByTestId("handoff-actual-column");
    await expect
      .element(actual.getByText("扫描完成，统计覆盖率 99.4%"))
      .toBeInTheDocument();
    expect(actual.element().textContent).toContain("产出：file-stats.json");
    expect(actual.element().textContent).toContain("交付时间");
    expect(actual.element().querySelector("time")?.getAttribute("dateTime")).toBe(
      "2026-07-27T03:20:00Z",
    );

    // verdict 拍板默认：不编造符合性判定。
    expect(screen.container.ownerDocument.body.textContent).not.toContain("不符");
    expect(
      screen.container.ownerDocument.body.textContent,
    ).toContain("不构成符合性判定");
  });

  it("presents empty boundaries as 暂无 instead of fabricating a verdict", async () => {
    const graph = makeGraph();
    graph.nodes[0].expected_outputs = [];
    graph.nodes[0].handoff_contract = {};
    graph.execution_summaries = [];

    const screen = await render(
      <FlowHandoffOverlay edge={edge} graph={graph} onClose={vi.fn()} />,
    );

    await expect.element(screen.getByText("暂无预期输出")).toBeInTheDocument();
    await expect.element(screen.getByText("暂无验收判据")).toBeInTheDocument();
    await expect
      .element(screen.getByText("暂无产出（上游任务尚未回写执行结论）"))
      .toBeInTheDocument();
    expect(screen.container.ownerDocument.body.textContent).not.toContain("不符");
  });

  // spec §5 P2-V：控制平面结构化 verdict → 右列逐条徽章 + 底注切换。
  it("renders per-deliverable verdict badges from the structured assessment", async () => {
    const graph = makeGraph();
    graph.handoff_assessments = [
      {
        deliverables: [
          {
            name: "file-stats.json",
            ref: "8b7f7a52-0000-0000-0000-000000000001",
            summary: "deliverables/file-stats.json",
            verdict: "delivered"
},
          { name: "scan-log", verdict: "missing" },
        ],
        project_task_id: "task-scan",
        status: "partial"
},
    ];

    const screen = await render(
      <FlowHandoffOverlay edge={edge} graph={graph} onClose={vi.fn()} />,
    );

    const verdicts = screen.getByTestId("handoff-deliverable-verdicts");
    await expect.element(verdicts.getByText("声明交付物核对")).toBeInTheDocument();
    await expect
      .element(verdicts.getByText("file-stats.json", { exact: true }))
      .toBeInTheDocument();
    await expect.element(verdicts.getByText("已交付")).toBeInTheDocument();
    await expect
      .element(verdicts.getByText("scan-log", { exact: true }))
      .toBeInTheDocument();
    await expect.element(verdicts.getByText("缺失")).toBeInTheDocument();
    // 原始路径经 summary 保留（CP Ref 回填时挪入），不裸渲 ref UUID。
    expect(verdicts.element().textContent).toContain("deliverables/file-stats.json");
    expect(verdicts.element().textContent).not.toContain(
      "8b7f7a52-0000-0000-0000-000000000001",
    );
    // 徽章走 StatusPill tone 惯例：已交付=ok / 缺失=danger。
    expect(
      verdicts.element().querySelectorAll('[data-tone="ok"]').length,
    ).toBe(1);
    expect(
      verdicts.element().querySelectorAll('[data-tone="danger"]').length,
    ).toBe(1);
    // 有结构化 verdict 时底注改为控制平面核对口径。
    expect(screen.container.ownerDocument.body.textContent).toContain(
      "符合性由控制平面按声明交付物核对",
    );
    expect(screen.container.ownerDocument.body.textContent).not.toContain(
      "不构成符合性判定",
    );
  });

  it("keeps 暂无 wording and the honest footnote when the assessment is unknown", async () => {
    const graph = makeGraph();
    graph.execution_summaries = [];
    graph.handoff_assessments = [
      { deliverables: [], project_task_id: "task-scan", status: "unknown" },
    ];

    const screen = await render(
      <FlowHandoffOverlay edge={edge} graph={graph} onClose={vi.fn()} />,
    );

    expect(
      screen.container.ownerDocument.querySelector(
        '[data-testid="handoff-deliverable-verdicts"]',
      ),
    ).toBeNull();
    await expect
      .element(screen.getByText("暂无产出（上游任务尚未回写执行结论）"))
      .toBeInTheDocument();
    expect(screen.container.ownerDocument.body.textContent).toContain(
      "不构成符合性判定",
    );
    expect(screen.container.ownerDocument.body.textContent).not.toContain(
      "符合性由控制平面按声明交付物核对",
    );
  });

  it("stays closed without an edge and closes through the dialog close button", async () => {
    const onClose = vi.fn();
    const closedScreen = await render(
      <FlowHandoffOverlay edge={undefined} graph={makeGraph()} onClose={onClose} />,
    );
    expect(
      closedScreen.container.ownerDocument.querySelector(
        '[data-testid="flow-handoff-overlay"]',
      ),
    ).toBeNull();
    closedScreen.unmount();

    const openScreen = await render(
      <FlowHandoffOverlay edge={edge} graph={makeGraph()} onClose={onClose} />,
    );
    await openScreen.getByRole("button", { name: "关闭" }).click();
    expect(onClose).toHaveBeenCalled();
  });
});
