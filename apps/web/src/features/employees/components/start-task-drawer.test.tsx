import { userEvent } from "vitest/browser";
import { describe, expect, it, vi } from "vitest";
import { render } from "vitest-browser-react";
import { StartTaskDrawer } from "./start-task-drawer";

describe("StartTaskDrawer", () => {
  it("submits objective and prompt", async () => {
    const onSubmit = vi.fn();
    const screen = await render(
      <StartTaskDrawer
        canStartTask
        disabledReasons={[]}
        isError={false}
        isPending={false}
        onOpenChange={vi.fn()}
        onSubmit={onSubmit}
        open
      />,
    );

    await userEvent.fill(screen.getByLabelText("任务目标"), "梳理上线风险");
    await userEvent.fill(screen.getByLabelText("任务提示"), "请检查最近失败任务");
    await userEvent.click(screen.getByRole("button", { name: "开始任务" }));

    expect(onSubmit).toHaveBeenCalledWith({ objective: "梳理上线风险", prompt: "请检查最近失败任务" });
  });

  it("disables submit and shows reasons when task cannot start", async () => {
    const screen = await render(
      <StartTaskDrawer
        canStartTask={false}
        disabledReasons={["Runtime 命令通道未连接，暂不能开始任务"]}
        isError={false}
        isPending={false}
        onOpenChange={vi.fn()}
        onSubmit={vi.fn()}
        open
      />,
    );

    await expect.element(screen.getByText("Runtime 命令通道未连接，暂不能开始任务")).toBeVisible();
    await expect.element(screen.getByRole("button", { name: "开始任务" })).toBeDisabled();
  });
});
