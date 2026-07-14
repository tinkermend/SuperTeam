import { describe, expect, it } from "vitest";
import { render } from "vitest-browser-react";
import { EmployeeMetricsStrip } from "./employee-metrics-strip";

const stats = {
  total_count: 76,
  succeeded_count: 68,
  failed_count: 5,
  cancelled_count: 3,
  success_rate: 68 / 76,
  avg_duration_sec: 29 * 60,
  p90_duration_sec: 48 * 60,
  last_7d_count: 12,
  prev_7d_count: 10,
};

describe("EmployeeMetricsStrip", () => {
  it("renders formatted stats without runtime/state cards", async () => {
    const screen = await render(<EmployeeMetricsStrip providerType="Claude Code" stats={stats} />);

    await expect.element(screen.getByText("76")).toBeVisible();
    await expect.element(screen.getByText("89.5%")).toBeVisible();
    await expect.element(screen.getByText("68")).toBeVisible();
    await expect.element(screen.getByText("29分0秒")).toBeVisible();
    await expect.element(screen.getByText(/P90 48分0秒/)).toBeVisible();
    await expect.element(screen.getByText(/较上周期/)).toBeVisible();
    expect(screen.getByText("Runtime 执行位置").query()).toBeNull();
    expect(screen.getByText("当前状态").query()).toBeNull();
  });

  it("shows placeholder dashes when stats are unavailable", async () => {
    const screen = await render(<EmployeeMetricsStrip providerType="Codex" stats={undefined} />);

    await expect.element(screen.getByText("成功率")).toBeVisible();
    expect(screen.getByText("--").elements().length).toBeGreaterThan(0);
  });
});
