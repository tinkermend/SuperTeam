import "@testing-library/jest-dom/vitest";
import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { SectionPanel } from "./section-panel";

describe("SectionPanel", () => {
  it("renders a titled panel with description, action, and content", () => {
    render(
      <SectionPanel action={<button type="button">查看全部</button>} description="最近 24 小时" title="审计事件">
        <p>暂无异常</p>
      </SectionPanel>,
    );

    expect(screen.getByRole("heading", { name: "审计事件" })).toBeInTheDocument();
    expect(screen.getByText("最近 24 小时")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "查看全部" })).toBeInTheDocument();
    expect(screen.getByText("暂无异常")).toBeInTheDocument();
  });
});
