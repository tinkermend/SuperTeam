import { describe, expect, it } from "vitest";
import { render } from "vitest-browser-react";
import { RunEventTimeline } from "./run-event-timeline";
import type { DigitalEmployeeRunEvent } from "@/lib/api/employees";

function event(overrides: Partial<DigitalEmployeeRunEvent>): DigitalEmployeeRunEvent {
  return { event_type: "text_delta", sequence_number: 1, payload: {}, ...overrides };
}

describe("RunEventTimeline", () => {
  it("renders Chinese lifecycle markers and merges consecutive text deltas", async () => {
    const screen = await render(
      <RunEventTimeline
        events={[
          event({ event_type: "session_started", sequence_number: 1, payload: { session_id: "sess-123" } }),
          event({ event_type: "turn_started", sequence_number: 2 }),
          event({ event_type: "text_delta", sequence_number: 3, payload: { text: "正在分析" } }),
          event({ event_type: "text_delta", sequence_number: 4, payload: { text: "需求" } }),
          event({ event_type: "turn_completed", sequence_number: 5, payload: { summary: "分析完成" } }),
          event({ event_type: "run_completed", sequence_number: 2147483000 }),
        ]}
      />,
    );

    await expect.element(screen.getByText("会话已建立")).toBeVisible();
    await expect.element(screen.getByText("sess-123")).toBeVisible();
    await expect.element(screen.getByText("回合开始")).toBeVisible();
    await expect.element(screen.getByText("正在分析需求")).toBeVisible();
    await expect.element(screen.getByText("回合完成")).toBeVisible();
    await expect.element(screen.getByText("分析完成")).toBeVisible();
    await expect.element(screen.getByText("运行完成")).toBeVisible();
  });

  it("pairs tool started/completed into one row with expandable excerpts", async () => {
    const screen = await render(
      <RunEventTimeline
        events={[
          event({
            event_type: "tool_started",
            sequence_number: 1,
            payload: { tool_id: "t1", name: "Read", input_excerpt: "path=/tmp/a.txt" }
}),
          event({
            event_type: "tool_completed",
            sequence_number: 2,
            payload: { tool_id: "t1", is_error: false, output_excerpt: "file contents", output_truncated: true }
}),
        ]}
      />,
    );

    await expect.element(screen.getByText("Read")).toBeVisible();
    await expect.element(screen.getByText("成功")).toBeVisible();
    await expect.element(screen.getByText("内容已截断。")).toBeVisible();
    expect(screen.getByText("工具调用").elements().length).toBe(1);

    await screen.getByText("输入").click();
    await expect.element(screen.getByText("path=/tmp/a.txt")).toBeVisible();
    await screen.getByText("输出").click();
    await expect.element(screen.getByText("file contents")).toBeVisible();
  });

  it("renders an orphan failed tool_completed as a standalone row", async () => {
    const screen = await render(
      <RunEventTimeline
        events={[
          event({
            event_type: "tool_completed",
            sequence_number: 7,
            payload: { tool_id: "t9", is_error: true, output_excerpt: "boom" }
}),
        ]}
      />,
    );

    await expect.element(screen.getByText("工具调用")).toBeVisible();
    await expect.element(screen.getByText("t9")).toBeVisible();
    await expect.element(screen.getByText("失败")).toBeVisible();
  });

  it("renders turn errors, unknown event types and the limit hint", async () => {
    const screen = await render(
      <RunEventTimeline
        events={[
          event({ event_type: "turn_error", sequence_number: 1, payload: { message: "provider crashed" } }),
          event({ event_type: "provider.stdout", sequence_number: 2, payload: { text: "raw" } }),
        ]}
        limitReached
      />,
    );

    await expect.element(screen.getByText("回合出错")).toBeVisible();
    await expect.element(screen.getByText("provider crashed")).toBeVisible();
    await expect.element(screen.getByText("provider.stdout")).toBeVisible();
    await expect.element(screen.getByText("仅显示前 2 条事件。")).toBeVisible();
  });

  it("reveals raw event JSON on demand", async () => {
    const screen = await render(
      <RunEventTimeline events={[event({ event_type: "turn_started", sequence_number: 1 })]} />,
    );

    await screen.getByText("原始 JSON").click();
    await expect.element(screen.getByText(/"event_type": "turn_started"/)).toBeVisible();
  });

  it("does not render a blank text row for an empty text_delta", async () => {
    const screen = await render(
      <RunEventTimeline
        events={[
          event({ event_type: "turn_started", sequence_number: 1 }),
          event({ event_type: "text_delta", sequence_number: 2, payload: {} }),
        ]}
      />,
    );

    await expect.element(screen.getByText("回合开始")).toBeVisible();
    // 空 text_delta 不应产出空白正文卡:整个时间线只有 turn_started 一张卡
    expect(screen.getByText("原始 JSON").elements().length).toBe(1);
  });

  it("still merges an empty text_delta into a preceding text block without adding a row", async () => {
    const screen = await render(
      <RunEventTimeline
        events={[
          event({ event_type: "text_delta", sequence_number: 1, payload: { text: "有内容" } }),
          event({ event_type: "text_delta", sequence_number: 2, payload: {} }),
        ]}
      />,
    );

    await expect.element(screen.getByText("有内容")).toBeVisible();
    expect(screen.getByText("原始 JSON").elements().length).toBe(1);
  });
});
