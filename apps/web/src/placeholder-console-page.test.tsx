import "@testing-library/jest-dom/vitest";
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { TooltipProvider } from "@/components/ui/tooltip";

import { PlaceholderConsolePage } from "./placeholder-console-page";

let pathname = "/tasks";

vi.mock("next/navigation", () => ({
  usePathname: () => pathname,
}));

vi.mock("@superteam/core/auth", () => ({
  useAuth: () => ({
    logout: vi.fn(),
    user: null,
  }),
}));

describe("PlaceholderConsolePage", () => {
  afterEach(() => {
    cleanup();
  });

  it("renders an undeveloped top-level menu placeholder", () => {
    pathname = "/tasks";

    render(
      <TooltipProvider>
        <PlaceholderConsolePage
          description="后续承载任务输入、执行状态、工件回传和验收闭环。"
          title="任务中心"
        />
      </TooltipProvider>,
    );

    expect(screen.getByRole("heading", { name: "任务中心" })).toBeInTheDocument();
    expect(screen.getByText("页面功能暂不开发")).toBeInTheDocument();
    expect(screen.getAllByText("后续承载任务输入、执行状态、工件回传和验收闭环。").length).toBeGreaterThan(0);
  });
});
