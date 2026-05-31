import "@testing-library/jest-dom/vitest";
import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import {
  PlatformEmptyState,
  PlatformErrorState,
  PlatformForbiddenState,
  PlatformLoadingState,
} from "./platform-states";

describe("platform state components", () => {
  it("renders an empty state with an optional action", () => {
    render(<PlatformEmptyState action={<button type="button">创建用户</button>} description="当前没有平台账号。" title="暂无用户" />);

    expect(screen.getByRole("heading", { name: "暂无用户" })).toBeInTheDocument();
    expect(screen.getByText("当前没有平台账号。")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "创建用户" })).toBeInTheDocument();
  });

  it("renders loading, error, and forbidden states with accessible semantics", () => {
    const { rerender } = render(<PlatformLoadingState title="正在加载用户列表" />);
    expect(screen.getByRole("status")).toHaveTextContent("正在加载用户列表");

    rerender(<PlatformErrorState title="用户列表加载失败" />);
    expect(screen.getByRole("alert")).toHaveTextContent("用户列表加载失败");

    rerender(<PlatformForbiddenState title="权限不足" />);
    expect(screen.getByRole("alert")).toHaveTextContent("权限不足");
  });
});
