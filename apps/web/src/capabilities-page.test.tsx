import "@testing-library/jest-dom/vitest";
import { cleanup, render, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { TooltipProvider } from "@/components/ui/tooltip";

import { CapabilitiesPage } from "./capabilities-page";

vi.mock("next/navigation", () => ({
  usePathname: () => "/capabilities",
}));

vi.mock("@superteam/core/auth", () => ({
  useAuth: () => ({
    logout: vi.fn(),
    user: {
      id: 1,
      status: "active",
      username: "admin",
    },
  }),
}));

describe("CapabilitiesPage", () => {
  afterEach(() => {
    cleanup();
  });

  it("renders an external capability placeholder with planned extension targets", () => {
    render(
      <TooltipProvider>
        <CapabilitiesPage />
      </TooltipProvider>,
    );

    expect(screen.getByRole("heading", { name: "外部能力" })).toBeInTheDocument();
    expect(screen.getByText("页面功能暂不开发，后续用于统一注册、授权、调用和审计外部能力。")).toBeInTheDocument();

    const plannedTargets = screen.getByRole("list", { name: "外部能力后续扩展对象" });
    for (const target of [
      "Dify Workflow",
      "Deephub Agent",
      "企业内部 HTTP 接口",
      "数据分析服务",
      "ITSM 工单接口",
      "CMDB / 监控 / 日志平台",
      "自研脚本服务",
      "后续的 MCP Server / Connector",
    ]) {
      expect(within(plannedTargets).getByText(target)).toBeInTheDocument();
    }
  });
});
