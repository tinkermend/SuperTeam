import { describe, expect, it } from "vitest";
import { render } from "vitest-browser-react";
import { ProjectExecutionTracePanel } from "./project-execution-trace-panel";

const trace = {
  attempts: [
    {
      attempt_id: "attempt-1",
      attempt_no: 1,
      events: [
        {
          actor_id: "runtime-agent-1",
          actor_type: "runtime_agent",
          artifact_refs: ["artifact-1"],
          created_at: "2026-06-20T08:02:00Z",
          error_code: "EXIT_1",
          error_family: "provider_exit",
          error_message: "provider exited with code 1",
          event_type: "attempt.failed",
          evidence_refs: ["evidence-1"],
          id: "event-1",
          input_summary: "读取任务上下文",
          metadata: { command: "codex" },
          occurred_at: "2026-06-20T08:01:00Z",
          output_summary: "生成失败诊断",
          project_id: "project-1",
          project_task_attempt_id: "attempt-1",
          project_task_id: "task-1",
          provider_session_id: "session-1",
          provider_type: "codex",
          retryable: false,
          runtime_node_id: "runtime-node-1",
          source_id: "runtime-node-1",
          source_type: "runtime_agent",
          tenant_id: "tenant-1",
        },
      ],
      failure_family: "provider_exit",
      finished_at: "2026-06-20T08:02:00Z",
      project_task_id: "task-1",
      provider_session_id: "session-1",
      provider_type: "codex",
      retryable: false,
      runtime_node_id: "runtime-node-1",
      started_at: "2026-06-20T08:00:00Z",
      status: "failed",
      summary: {
        artifact_refs: ["artifact-1"],
        conclusion: "Provider 进程异常退出，需要人工复核。",
        created_at: "2026-06-20T08:03:00Z",
        evidence_refs: ["evidence-1"],
        execution_summary_id: "summary-1",
        requires_human_review: true,
      },
    },
  ],
  project_id: "project-1",
  summary: {
    artifact_ref_count: 1,
    attempt_count: 1,
    evidence_ref_count: 1,
    failed_attempt_count: 1,
    human_review_required_count: 1,
    latest_error_family: "provider_exit",
  },
};

describe("ProjectExecutionTracePanel", () => {
  it("renders execution trace summary, attempt, and ledger event details", async () => {
    const screen = await render(<ProjectExecutionTracePanel trace={trace} />);

    await expect
      .element(screen.getByRole("heading", { name: "执行证据链" }))
      .toBeInTheDocument();
    await expect.element(screen.getByText("1 次")).toBeInTheDocument();
    await expect
      .element(screen.getByText("provider_exit", { exact: true }))
      .toBeInTheDocument();
    await expect
      .element(screen.getByLabelText("执行尝试 1"))
      .toBeInTheDocument();
    await expect
      .element(screen.getByText("failed", { exact: true }))
      .toBeInTheDocument();
    await expect.element(screen.getByText("读取任务上下文")).toBeInTheDocument();
    await expect.element(screen.getByText("生成失败诊断")).toBeInTheDocument();
  });
});
