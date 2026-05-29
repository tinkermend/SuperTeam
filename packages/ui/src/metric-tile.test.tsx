import "@testing-library/jest-dom/vitest";
import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { MetricTile } from "./metric-tile";

describe("MetricTile", () => {
  it("keeps the metric label, value, and supporting status together", () => {
    render(<MetricTile label="Runtime 节点" meta="2 个离线" status="需处理" value="4" />);

    const tile = screen.getByRole("group", { name: "Runtime 节点" });
    expect(tile).toHaveTextContent("4");
    expect(tile).toHaveTextContent("2 个离线");
    expect(tile).toHaveTextContent("需处理");
  });
});
