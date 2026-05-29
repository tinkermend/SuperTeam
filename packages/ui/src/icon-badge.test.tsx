import "@testing-library/jest-dom/vitest";
import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { IconBadge } from "./icon-badge";

describe("IconBadge", () => {
  it("renders a labeled icon container with a tone marker", () => {
    render(
      <IconBadge label="Runtime 节点" tone="info">
        R
      </IconBadge>,
    );

    const badge = screen.getByLabelText("Runtime 节点");
    expect(badge).toHaveAttribute("data-tone", "info");
    expect(badge).toHaveTextContent("R");
  });
});
