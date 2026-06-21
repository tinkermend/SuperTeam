import { userEvent } from "vitest/browser";
import { describe, expect, it, vi } from "vitest";
import { render } from "vitest-browser-react";
import type { ProjectExecutionTrace } from "@/lib/api/projects";
import { ProjectExecutionTracePanel } from "./project-execution-trace-panel";

const trace: ProjectExecutionTrace = {
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
  it("renders loading state before empty state", async () => {
    const screen = await render(<ProjectExecutionTracePanel isLoading />);

    await expect
      .element(screen.getByText("正在加载执行证据链"))
      .toBeInTheDocument();
    await expect
      .element(screen.getByText("暂无执行证据链"))
      .not.toBeInTheDocument();
  });

  it("renders error state with retry action before empty state", async () => {
    const onRetry = vi.fn();
    const screen = await render(
      <ProjectExecutionTracePanel
        errorMessage="执行证据链接口失败"
        isError
        onRetry={onRetry}
      />,
    );

    await expect
      .element(screen.getByText("执行证据链接口失败"))
      .toBeInTheDocument();
    await expect.element(screen.getByRole("button", { name: "重试" })).toBeInTheDocument();
    await expect
      .element(screen.getByText("暂无执行证据链"))
      .not.toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: "重试" }));

    expect(onRetry).toHaveBeenCalledTimes(1);
  });

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

  it("keeps long audit values accessible", async () => {
    const longTaskId = "task-2026-06-20-runtime-ledger-audit-value-full-access";
    const longSessionId = "session-codex-provider-run-with-long-session-identifier";
    const longRuntimeId = "runtime-node-macos-developer-machine-very-long-id";
    const longActorId = "runtime-agent-with-a-long-audit-principal-id";
    const longSourceId = "source-project-task-attempt-writeback-long-id";
    const longConclusion =
      "Provider 进程异常退出，需要人工复核。这里保留完整结论文本，包含失败原因、下一步动作和证据口径。";
    const longInputSummary =
      "输入摘要包含完整上下文切片、任务边界、运行参数和依赖证据，不能只显示截断后的片段。";
    const longOutputSummary =
      "输出摘要包含执行结果、缺失信息、工件引用和后续处理建议，需要可直接阅读完整内容。";
    const longErrorMessage =
      "provider exited with code 1 after streaming a long diagnostic message that should remain inspectable";
    const actorSourceLabel = `runtime_agent:${longActorId} · runtime_agent:${longSourceId}`;
    const longTrace: ProjectExecutionTrace = {
      ...trace,
      attempts: [
        {
          ...trace.attempts[0],
          project_task_id: longTaskId,
          provider_session_id: longSessionId,
          runtime_node_id: longRuntimeId,
          summary: {
            ...trace.attempts[0].summary!,
            conclusion: longConclusion,
          },
          events: [
            {
              ...trace.attempts[0].events[0],
              actor_id: longActorId,
              error_message: longErrorMessage,
              input_summary: longInputSummary,
              output_summary: longOutputSummary,
              source_id: longSourceId,
            },
          ],
        },
      ],
    };

    const screen = await render(<ProjectExecutionTracePanel trace={longTrace} />);

    await expect.element(screen.getByText(longTaskId)).toHaveAttribute("title", longTaskId);
    await expect
      .element(screen.getByText(longSessionId))
      .toHaveAttribute("title", longSessionId);
    await expect
      .element(screen.getByText(longRuntimeId))
      .toHaveAttribute("title", longRuntimeId);
    await expect
      .element(screen.getByText(actorSourceLabel))
      .toHaveAttribute("title", actorSourceLabel);
    await expect.element(screen.getByText(longConclusion)).toBeInTheDocument();
    await expect.element(screen.getByText(longInputSummary)).toBeInTheDocument();
    await expect.element(screen.getByText(longOutputSummary)).toBeInTheDocument();
    await expect.element(screen.getByText(longErrorMessage)).toBeInTheDocument();
  });
});
