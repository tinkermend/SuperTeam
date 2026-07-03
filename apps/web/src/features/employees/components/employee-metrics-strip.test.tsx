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
  it("renders formatted stats", async () => {
    const screen = await render(
      <EmployeeMetricsStrip
        commandChannelConnected
        currentStatusLabel="active"
        providerType="claude_code"
        runtimeNodeLabel="local-dev-node"
        stats={stats}
      />,
    );

    await expect.element(screen.getByText("76")).toBeVisible();
    await expect.element(screen.getByText("89.5%")).toBeVisible();
    await expect.element(screen.getByText("68")).toBeVisible();
    await expect.element(screen.getByText("29分0秒")).toBeVisible();
    await expect.element(screen.getByText(/P90 48分0秒/)).toBeVisible();
    await expect.element(screen.getByText(/近7天.*↑/)).toBeVisible();
  });

  it("shows placeholder dashes when stats are unavailable", async () => {
    const screen = await render(
      <EmployeeMetricsStrip
        commandChannelConnected={false}
        currentStatusLabel="active"
        providerType="claude_code"
        runtimeNodeLabel="local-dev-node"
        stats={undefined}
      />,
    );

    await expect.element(screen.getByText("Runtime 命令通道未连接")).toBeVisible();
  });
});
