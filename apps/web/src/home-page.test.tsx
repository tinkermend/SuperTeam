import "@testing-library/jest-dom/vitest";
import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { TooltipProvider } from "@/components/ui/tooltip";

import { HomePage } from "./home-page";

describe("HomePage", () => {
  it("renders the external system shell with navigation, search, and a placeholder workspace", () => {
    render(
      <TooltipProvider>
        <HomePage summary="control-plane is ok" />
      </TooltipProvider>,
    );

    expect(screen.getByRole("heading", { name: "首页" })).toBeInTheDocument();
    expect(screen.getByRole("navigation", { name: "主导航" })).toBeInTheDocument();
    expect(screen.getByRole("searchbox", { name: "全局搜索" })).toHaveAttribute(
      "placeholder",
      "搜索任务、工件、员工、流程...",
    );
    expect(screen.getByRole("heading", { name: "页面正在开发中" })).toBeInTheDocument();
    expect(screen.getByText("control-plane is ok")).toBeInTheDocument();
  });
});
