import type { ReactNode } from "react";
import { render } from "vitest-browser-react";
import { page, userEvent } from "vitest/browser";
import { describe, expect, it, vi } from "vitest";
import {
  EmployeeWorkCalendar,
  employeeWeekQueryWindow,
  employeeWeekStart,
} from "./employee-work-calendar";
import type { DigitalEmployeeRunCalendarItem } from "@/lib/api/employees";

vi.mock("@tanstack/react-router", () => ({
  Link: ({ children, to }: { children: ReactNode; to: string }) => <a href={to}>{children}</a>,
}));
function item(overrides: Partial<DigitalEmployeeRunCalendarItem> = {}): DigitalEmployeeRunCalendarItem {
  return {
    id: overrides.id ?? "run-1",
    task_title: overrides.task_title ?? "梳理需求",
    status: overrides.status ?? "completed",
    run_kind: overrides.run_kind ?? "task",
    created_at: overrides.created_at ?? new Date().toISOString(),
    ...overrides,
  };
}

describe("employeeWeekQueryWindow", () => {
  it("encodes local Monday 00:00 through next Monday 00:00 as absolute ISO bounds", () => {
    const weekStart = employeeWeekStart(new Date(2026, 6, 22, 15, 30, 0));
    const window = employeeWeekQueryWindow(weekStart);
    const from = new Date(window.from);
    const to = new Date(window.to);
    const localMondayMorning = new Date(2026, 6, 20, 0, 30, 0);
    const localNextMondayMorning = new Date(2026, 6, 27, 0, 30, 0);

    expect(weekStart.getDay()).toBe(1);
    expect(weekStart.getHours()).toBe(0);
    expect(from.getTime()).toBe(weekStart.getTime());
    expect(to.getTime() - from.getTime()).toBe(7 * 24 * 60 * 60 * 1000);
    expect(localMondayMorning.getTime()).toBeGreaterThanOrEqual(from.getTime());
    expect(localMondayMorning.getTime()).toBeLessThan(to.getTime());
    expect(localNextMondayMorning.getTime()).toBeGreaterThanOrEqual(to.getTime());
  });
});

describe("EmployeeWorkCalendar", () => {
  it("renders week columns and opens an item", async () => {
    const weekStart = employeeWeekStart(new Date(2026, 6, 20));
    const onItemClick = vi.fn();
    const screen = await render(
      <EmployeeWorkCalendar
        items={[
          item({
            id: "run-a",
            task_title: "新增知识库条目",
            created_at: new Date(2026, 6, 21, 10, 0, 0).toISOString(),
          }),
        ]}
        onItemClick={onItemClick}
        onRetry={() => undefined}
        onWeekChange={() => undefined}
        totalCount={1}
        weekStart={weekStart}
      />,
    );

    await expect.element(page.getByText("新增知识库条目")).toBeVisible();
    await expect.element(page.getByText("10:00")).toBeVisible();
    await userEvent.click(screen.getByRole("button", { name: /新增知识库条目/ }));
    expect(onItemClick).toHaveBeenCalledWith(
      expect.objectContaining({ id: "run-a", task_title: "新增知识库条目" }),
    );
  });

  it("expands overflow items for a day", async () => {
    const weekStart = employeeWeekStart(new Date(2026, 6, 20));
    const items = Array.from({ length: 7 }, (_, index) =>
      item({
        id: `run-${index}`,
        task_title: `条目 ${index + 1}`,
        created_at: new Date(2026, 6, 21, 8, index, 0).toISOString(),
      }),
    );

    const screen = await render(
      <EmployeeWorkCalendar
        items={items}
        onItemClick={() => undefined}
        onRetry={() => undefined}
        onWeekChange={() => undefined}
        totalCount={7}
        weekStart={weekStart}
      />,
    );

    await expect.element(page.getByRole("button", { name: "还有 2 项" })).toBeVisible();
    await userEvent.click(screen.getByRole("button", { name: "还有 2 项" }));
    await expect.element(page.getByText("条目 1")).toBeVisible();
  });

  it("renders a compact empty week with a task-hub link instead of empty day columns", async () => {
    const weekStart = employeeWeekStart(new Date(2026, 6, 20));
    const screen = await render(
      <EmployeeWorkCalendar
        items={[]}
        onItemClick={() => undefined}
        onRetry={() => undefined}
        onWeekChange={() => undefined}
        totalCount={0}
        weekStart={weekStart}
      />,
    );

    await expect.element(screen.getByText("本周暂无运行记录")).toBeVisible();
    expect(screen.getByText("无记录").query()).toBeNull();
    await expect.element(screen.getByRole("link", { name: "去任务中枢" })).toHaveAttribute("href", "/");
  });

  it("clamps long task titles to two lines with an ellipsis", async () => {
    const weekStart = employeeWeekStart(new Date(2026, 6, 20));
    const longTitle =
      "这是一条非常非常长的任务目标描述，用来验证日历条目只保留两行并在末尾用省略号收束，避免把整列撑破";
    const screen = await render(
      <EmployeeWorkCalendar
        items={[
          item({
            id: "run-long",
            task_title: longTitle,
            created_at: new Date(2026, 6, 21, 15, 54, 0).toISOString(),
          }),
        ]}
        onItemClick={() => undefined}
        onRetry={() => undefined}
        onWeekChange={() => undefined}
        totalCount={1}
        weekStart={weekStart}
      />,
    );

    const title = screen.getByText(longTitle);
    await expect.element(title).toBeVisible();
    expect(title.element().className).toContain("line-clamp-2");
    await expect.element(screen.getByText("15:54")).toBeVisible();
    await expect.element(screen.getByText("已完成")).toBeVisible();
  });
});
