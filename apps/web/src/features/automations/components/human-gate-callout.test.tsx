import { describe, expect, it } from "vitest";
import { render } from "vitest-browser-react";
import { page } from "vitest/browser";
import { HumanGateCallout } from "./human-gate-callout";

describe("HumanGateCallout", () => {
  it("shows unmanned-not-equal copy for loop mode", async () => {
    render(<HumanGateCallout mode="loop" />);
    await expect.element(page.getByText("自动触发 ≠ 无人值守")).toBeInTheDocument();
    await expect.element(page.getByText(/终态验收仍需人类处理/)).toBeInTheDocument();
  });

  it("shows chat isolation copy", async () => {
    render(<HumanGateCallout mode="chat" />);
    await expect.element(page.getByText("定时对话，不进项目验收")).toBeInTheDocument();
  });
});
