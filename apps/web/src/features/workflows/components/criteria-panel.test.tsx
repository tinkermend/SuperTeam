import { userEvent } from "vitest/browser";
import { describe, expect, it, vi } from "vitest";
import { render } from "vitest-browser-react";
import { CriteriaPanelView } from "./criteria-panel";
import type { DemandAcceptanceCriterionDetail } from "@/lib/api/projects";

function criterion(
  overrides: Partial<DemandAcceptanceCriterionDetail> = {},
): DemandAcceptanceCriterionDetail {
  return {
    criterion_id: "c1",
    statement: "接口在 500ms 内返回",
    verification_method: "human_judgment",
    severity: "blocking",
    satisfied_by: [],
    verdict: null,
    judge_type: null,
    evidence_refs: [],
    task_summaries: [],
    ...overrides
};
}

describe("CriteriaPanelView", () => {
  it("renders each criterion with method, severity, verdict and judge badges", async () => {
    const screen = await render(
      <CriteriaPanelView
        criteria={[
          criterion({
            criterion_id: "c1",
            statement: "接口在 500ms 内返回",
            verdict: "satisfied",
            judge_type: "executor",
            verification_method: "automated_test"
}),
          criterion({
            criterion_id: "c2",
            statement: "文案由负责人确认",
            verdict: "unsatisfied",
            judge_type: "human",
            severity: "non_blocking"
}),
          criterion({ criterion_id: "c3", statement: "待判定项", verdict: null }),
        ]}
        demandStatus="completed"
      />,
    );

    const row1 = screen.getByTestId("criterion-row-c1");
    const row2 = screen.getByTestId("criterion-row-c2");
    const row3 = screen.getByTestId("criterion-row-c3");

    await expect.element(row1.getByText("接口在 500ms 内返回")).toBeInTheDocument();
    await expect.element(row1.getByText("自动验证")).toBeInTheDocument();
    await expect.element(row1.getByText("已满足")).toBeInTheDocument();
    await expect.element(row1.getByText("员工判定")).toBeInTheDocument();

    await expect.element(row2.getByText("人类判定")).toBeInTheDocument();
    await expect.element(row2.getByText("非阻断")).toBeInTheDocument();
    await expect.element(row2.getByText("未满足")).toBeInTheDocument();
    await expect.element(row2.getByText("负责人判定")).toBeInTheDocument();

    await expect.element(row3.getByText("待判定", { exact: true })).toBeInTheDocument();
  });

  it("renders an executor not_applicable verdict as a distinct 不适用 state without sign controls", async () => {
    const screen = await render(
      <CriteriaPanelView
        criteria={[
          criterion({
            criterion_id: "c-na",
            statement: "自动测试通过",
            verification_method: "automated_test",
            verdict: "not_applicable",
            judge_type: "executor",
            evidence_refs: ["artifact:na-rationale"]
}),
        ]}
        demandStatus="acceptance_pending"
        onFinalAccept={vi.fn()}
      />,
    );

    const row = screen.getByTestId("criterion-row-c-na");
    await expect.element(row.getByText("不适用")).toBeInTheDocument();
    await expect.element(row.getByText("员工判定")).toBeInTheDocument();
    await expect.element(row.getByText("artifact:na-rationale")).toBeInTheDocument();
    // Automated criterion → never opens the final acceptance gate alone.
    expect(screen.container.querySelector('[data-testid="final-acceptance-gate"]')).toBeNull();
  });

  it("renders evidence refs as labeled monospace chips without navigation", async () => {
    const screen = await render(
      <CriteriaPanelView
        criteria={[
          criterion({
            criterion_id: "c1",
            evidence_refs: ["attestation:abc-123", "artifact://report.md"]
}),
        ]}
        demandStatus="completed"
      />,
    );

    const chip = screen.getByText("attestation:abc-123");
    await expect.element(chip).toBeInTheDocument();
    // Non-link: must not be an anchor element.
    expect(
      (chip.element() as HTMLElement).closest("a"),
    ).toBeNull();
    await expect.element(screen.getByText("artifact://report.md")).toBeInTheDocument();
  });

  it("shows a single final-acceptance gate for unsigned blocking human criteria", async () => {
    const screen = await render(
      <CriteriaPanelView
        criteria={[
          criterion({ criterion_id: "c1", statement: "人类确认交付意图", verdict: null, judge_type: null }),
          // already human-signed → not listed in gate
          criterion({ criterion_id: "c2", verdict: "satisfied", judge_type: "human" }),
          // automated → no sign controls
          criterion({
            criterion_id: "c3",
            verification_method: "automated_test",
            verdict: "satisfied",
            judge_type: "executor"
}),
        ]}
        demandStatus="acceptance_pending"
        onFinalAccept={vi.fn()}
      />,
    );

    await expect.element(screen.getByTestId("final-acceptance-gate")).toBeInTheDocument();
    await expect.element(screen.getByTestId("final-acceptance-item-c1")).toBeInTheDocument();
    expect(screen.container.querySelector('[data-testid="final-acceptance-item-c2"]')).toBeNull();
    expect(screen.container.querySelector('[data-testid="final-acceptance-item-c3"]')).toBeNull();
    // No per-row sign buttons.
    expect(screen.container.querySelector('[data-testid="criterion-sign-satisfied-c1"]')).toBeNull();
  });

  it("lists all unsigned blocking human criteria in the final gate for legacy snapshots", async () => {
    const screen = await render(
      <CriteriaPanelView
        criteria={[
          criterion({ criterion_id: "c1", statement: "结论可接受", verdict: null }),
          criterion({ criterion_id: "c2", statement: "范围充分", verdict: null }),
        ]}
        demandStatus="acceptance_pending"
        onFinalAccept={vi.fn()}
      />,
    );

    await expect.element(screen.getByTestId("final-acceptance-item-c1")).toBeInTheDocument();
    await expect.element(screen.getByTestId("final-acceptance-item-c2")).toBeInTheDocument();
  });

  it("does not render the final gate when demand is not acceptance_pending", async () => {
    const screen = await render(
      <CriteriaPanelView
        criteria={[criterion({ criterion_id: "c1", verdict: null, judge_type: null })]}
        demandStatus="completed"
        onFinalAccept={vi.fn()}
      />,
    );

    expect(screen.container.querySelector('[data-testid="final-acceptance-gate"]')).toBeNull();
  });

  it("submits final pass with all unsigned criterion ids", async () => {
    const onFinalAccept = vi.fn();
    const screen = await render(
      <CriteriaPanelView
        criteria={[
          criterion({ criterion_id: "c1", statement: "结论可接受", verdict: null }),
          criterion({ criterion_id: "c2", statement: "范围充分", verdict: null }),
        ]}
        demandStatus="acceptance_pending"
        onFinalAccept={onFinalAccept}
      />,
    );

    await userEvent.fill(screen.getByTestId("final-acceptance-reason"), "已核对产出");
    await userEvent.click(screen.getByTestId("final-acceptance-pass"));

    expect(onFinalAccept).toHaveBeenCalledWith("satisfied", "已核对产出", ["c1", "c2"], {
      alsoCloseProject: false
});
  });

  it("requires a reason before reject and submits unsatisfied for the gate", async () => {
    const onFinalAccept = vi.fn();
    const screen = await render(
      <CriteriaPanelView
        criteria={[criterion({ criterion_id: "c1", verdict: null })]}
        demandStatus="acceptance_pending"
        onFinalAccept={onFinalAccept}
      />,
    );

    const reject = screen.getByTestId("final-acceptance-reject");
    await expect.element(reject).toBeDisabled();

    await userEvent.fill(screen.getByTestId("final-acceptance-reason"), "证据不足");
    await userEvent.click(reject);

    expect(onFinalAccept).toHaveBeenCalledWith("unsatisfied", "证据不足", ["c1"]);
  });

  it("renders task summaries so the human sees produced work before signing", async () => {
    const screen = await render(
      <CriteriaPanelView
        criteria={[
          criterion({
            criterion_id: "c1",
            satisfied_by: ["task-1"],
            task_summaries: [
              { task_id: "task-1", summary: "已交付并通过回归", deliverables: [] },
            ]
}),
        ]}
        demandStatus="acceptance_pending"
        onFinalAccept={vi.fn()}
      />,
    );

    await expect.element(screen.getByText("已交付并通过回归")).toBeInTheDocument();
  });

  it("surfaces declared deliverables as preview/download chips beside the task summary", async () => {
    const screen = await render(
      <CriteriaPanelView
        criteria={[
          criterion({
            criterion_id: "c1",
            satisfied_by: ["task-1"],
            task_summaries: [
              {
                task_id: "task-1",
                summary: "报告已交付",
                deliverables: [
                  {
                    artifact_ref_id: "art-html",
                    title: "report.html",
                    content_type: "text/html",
                    size_bytes: 462
},
                  {
                    artifact_ref_id: "art-bin",
                    title: "data.bin",
                    content_type: "application/octet-stream"
},
                ]
},
            ]
}),
        ]}
        demandStatus="acceptance_pending"
        onFinalAccept={vi.fn()}
      />,
    );

    // 交付物随任务产出折叠块展开可见。
    await userEvent.click(screen.getByText(/查看满足任务产出/));
    await expect.element(screen.getByText("report.html")).toBeInTheDocument();
    await expect.element(screen.getByText("data.bin")).toBeInTheDocument();
    // 两个交付物各一个下载链接 + 仅 HTML 一个预览按钮。
    expect(screen.getByRole("link", { name: /下载/ }).all()).toHaveLength(2);
    expect(screen.getByRole("button", { name: /预览/ }).all()).toHaveLength(1);
  });
});
