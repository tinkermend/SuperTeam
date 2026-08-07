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
          tenant_id: "tenant-1"
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
      capability_projection: {
        available: true,
        skills: [
          {
            skill_id: "skill-1",
            skill_key: "linux",
            skill_name: "Linux 排障",
            source_scope: "project",
          },
        ],
        mcp_servers: [
          {
            server_id: "mcp-1",
            server_key: "github-mcp",
            server_name: "GitHub",
            source_scope: "dependency_closure",
          },
        ],
        skill_conflicts: [],
        summary: {
          skill_count: 1,
          mcp_count: 1,
          conflict_count: 0,
          by_source: { project: 1, dependency_closure: 1 },
        },
      },
      summary: {
        artifact_refs: ["artifact-1"],
        conclusion: "Provider 进程异常退出，需要人工复核。",
        created_at: "2026-06-20T08:03:00Z",
        evidence_refs: ["evidence-1"],
        execution_summary_id: "summary-1",
        requires_human_review: true
}
},
  ],
  project_id: "project-1",
  summary: {
    artifact_ref_count: 1,
    attempt_count: 1,
    evidence_ref_count: 1,
    failed_attempt_count: 1,
    human_review_required_count: 1,
    latest_error_family: "provider_exit"
}
};

describe("ProjectExecutionTracePanel", () => {
  it("renders loading state before empty state", async () => {
    const screen = await render(<ProjectExecutionTracePanel isLoading />);

    await expect
      .element(screen.getByText("正在加载执行证据链"))
      .toBeInTheDocument();
    await expect
      .element(screen.container.querySelector<HTMLElement>('[data-slot="soft-card"]'))
      .toBeInTheDocument();
    await expect
      .element(screen.container.querySelector<HTMLElement>('[data-slot="loading-state"]'))
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
    await expect
      .element(screen.container.querySelector<HTMLElement>('[data-slot="error-state"]'))
      .toBeInTheDocument();
    await expect.element(screen.getByRole("button", { name: "重试" })).toBeInTheDocument();
    await expect
      .element(screen.getByText("暂无执行证据链"))
      .not.toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: "重试" }));

    expect(onRetry).toHaveBeenCalledTimes(1);
  });


  it("renders capability projection block on attempt row", async () => {
    const screen = await render(<ProjectExecutionTracePanel trace={trace} />);
    await expect
      .element(screen.getByTestId("attempt-capability-projection"))
      .toBeInTheDocument();
    await expect.element(screen.getByText("技能 1 · MCP 1 · 冲突 0")).toBeInTheDocument();
    await expect.element(screen.getByText("Linux 排障")).toBeInTheDocument();
    await expect.element(screen.getByText("依赖补全")).toBeInTheDocument();
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
      .element(screen.container.querySelector<HTMLElement>('[data-slot="soft-card"]'))
      .toBeInTheDocument();
    expect(screen.container.querySelectorAll<HTMLElement>('[data-slot="soft-card"]').length).toBeGreaterThanOrEqual(1);
    expect(screen.container.querySelectorAll<HTMLElement>('[data-slot="status-pill"]').length).toBeGreaterThan(0);
    await expect
      .element(screen.getByLabelText("执行尝试 1"))
      .toBeInTheDocument();
    await expect
      .element(screen.getByText("失败", { exact: true }))
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
            conclusion: longConclusion
},
          events: [
            {
              ...trace.attempts[0].events[0],
              actor_id: longActorId,
              error_message: longErrorMessage,
              input_summary: longInputSummary,
              output_summary: longOutputSummary,
              source_id: longSourceId
},
          ]
},
      ]
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

  it("renders a failed tool result using the provider-written is_error flag", async () => {
    const toolTrace: ProjectExecutionTrace = {
      ...trace,
      attempts: [
        {
          ...trace.attempts[0],
          // A succeeded attempt keeps "失败" unique to the tool result pill.
          failure_family: undefined,
          status: "completed",
          events: [
            {
              ...trace.attempts[0].events[0],
              error_code: undefined,
              error_family: undefined,
              error_message: undefined,
              event_type: "provider.event",
              id: "event-tool-completed",
              input_summary: "tool_completed",
              metadata: {
                is_error: true,
                output_excerpt: "bash: false: exit 1",
                output_truncated: false,
                tool_id: "toolu_2"
},
              output_summary: "bash: false: exit 1"
},
          ]
},
      ]
};

    const screen = await render(<ProjectExecutionTracePanel trace={toolTrace} />);

    await expect.element(screen.getByText("工具结果")).toBeInTheDocument();
    await expect.element(screen.getByText("失败", { exact: true })).toBeInTheDocument();
  });

  it("filters attempts by task through the dropdown with task titles", async () => {
    const twoTaskTrace: ProjectExecutionTrace = {
      ...trace,
      attempts: [
        trace.attempts[0],
        {
          ...trace.attempts[0],
          attempt_id: "attempt-2",
          attempt_no: 2,
          events: [],
          project_task_id: "task-2",
          status: "completed",
          summary: undefined
},
      ],
      summary: { ...trace.summary!, attempt_count: 2 }
};

    const screen = await render(
      <ProjectExecutionTracePanel
        taskTitlesById={
          new Map([
            ["task-1", "整理接入证据"],
            ["task-2", "复核接入证据"],
          ])
        }
        trace={twoTaskTrace}
      />,
    );

    await expect.element(screen.getByLabelText("执行尝试 1")).toBeInTheDocument();
    await expect.element(screen.getByLabelText("执行尝试 2")).toBeInTheDocument();

    // 选项用任务标题显示名（id → 标题）。
    await expect
      .element(screen.getByRole("option", { name: "整理接入证据" }))
      .toBeInTheDocument();
    await userEvent.selectOptions(screen.getByTestId("trace-task-filter"), "task-2");

    await expect.element(screen.getByLabelText("执行尝试 2")).toBeInTheDocument();
    await expect.element(screen.getByLabelText("执行尝试 1")).not.toBeInTheDocument();
  });

  it("preselects the deep-linked task and resets when it has no attempts", async () => {
    const screen = await render(
      <ProjectExecutionTracePanel focusTaskId="task-missing" trace={trace} />,
    );

    // 深链任务不在证据链里：可解释空态 + 一键回到全部任务。
    await expect
      .element(screen.getByTestId("trace-task-filter-empty"))
      .toBeInTheDocument();
    await expect
      .element(screen.getByText("该任务暂无执行尝试记录"))
      .toBeInTheDocument();
    await expect.element(screen.getByLabelText("执行尝试 1")).not.toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: "显示全部任务" }));
    await expect.element(screen.getByLabelText("执行尝试 1")).toBeInTheDocument();
  });

  it("preselects the deep-linked task that has attempts", async () => {
    const screen = await render(
      <ProjectExecutionTracePanel focusTaskId="task-1" trace={trace} />,
    );

    await expect.element(screen.getByTestId("trace-task-filter")).toHaveValue("task-1");
    await expect.element(screen.getByLabelText("执行尝试 1")).toBeInTheDocument();
  });

  it("renders a tool call with its name and truncation notice", async () => {
    const toolTrace: ProjectExecutionTrace = {
      ...trace,
      attempts: [
        {
          ...trace.attempts[0],
          events: [
            {
              ...trace.attempts[0].events[0],
              error_code: undefined,
              error_family: undefined,
              error_message: undefined,
              event_type: "provider.event",
              id: "event-tool-started",
              input_summary: "tool_started",
              metadata: {
                input_excerpt: '{"command":"git status"}',
                input_truncated: true,
                name: "Bash",
                tool_id: "toolu_1"
},
              output_summary: "tool_started"
},
          ]
},
      ]
};

    const screen = await render(<ProjectExecutionTracePanel trace={toolTrace} />);

    await expect.element(screen.getByText("工具调用")).toBeInTheDocument();
    await expect.element(screen.getByText("Bash")).toBeInTheDocument();
    await expect
      .element(screen.getByText("内容已截断，完整日志将在证据地基落地后可下载。"))
      .toBeInTheDocument();
  });
});
