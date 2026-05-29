import "@testing-library/jest-dom/vitest";
import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { TimelineItem } from "./timeline-item";

describe("TimelineItem", () => {
  it("renders an auditable timeline entry with time and status", () => {
    render(
      <TimelineItem description="Runtime Agent 已回传执行日志" status="运行中" time="11:24" title="执行事件" />,
    );

    expect(screen.getByRole("listitem")).toHaveTextContent("执行事件");
    expect(screen.getByText("11:24")).toBeInTheDocument();
    expect(screen.getByText("运行中")).toBeInTheDocument();
  });
});
