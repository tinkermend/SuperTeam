import "@testing-library/jest-dom/vitest";
import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { StatusPill } from "./status-pill";

describe("StatusPill", () => {
  it("renders a semantic status with a stable tone and optional description", () => {
    render(<StatusPill description="等待人工确认" tone="warning">待确认</StatusPill>);

    const pill = screen.getByText("待确认").closest("span");
    expect(pill).toHaveAttribute("data-tone", "warning");
    expect(screen.getByText("等待人工确认")).toBeInTheDocument();
  });
});
