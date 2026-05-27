import "@testing-library/jest-dom/vitest";
import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { StatusBadge } from "./status-badge";

describe("StatusBadge", () => {
  it("renders a compact status label with a stable tone marker", () => {
    render(<StatusBadge tone="ok">Online</StatusBadge>);

    const badge = screen.getByText("Online");
    expect(badge).toHaveAttribute("data-tone", "ok");
    expect(badge).toHaveClass("inline-flex");
  });
});
