import "@testing-library/jest-dom/vitest";
import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { ConsoleHealthView } from "./console-health-view";

describe("ConsoleHealthView", () => {
  it("shows the control plane health summary without depending on a platform router", () => {
    render(<ConsoleHealthView summary="control-plane is ok" />);

    expect(screen.getByRole("heading", { name: "Control Plane" })).toBeInTheDocument();
    expect(screen.getByText("control-plane is ok")).toBeInTheDocument();
    expect(screen.getByText("Live")).toHaveAttribute("data-tone", "ok");
  });
});
