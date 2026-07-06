import { describe, expect, it, vi } from "vitest";
import { render } from "vitest-browser-react";
import { Route } from "./index";

vi.mock("@/features/run-overview", () => ({
  RunOverviewPage: () => <main>运行总览真实页面</main>,
}));

vi.mock("@/features/shared/unimplemented-page", () => ({
  UnimplementedPage: () => <main>运行总览占位页面</main>,
}));

describe("RunOverviewRoute", () => {
  it("mounts the production runtime overview page instead of the placeholder", async () => {
    const Component = Route.options.component!;
    const screen = await render(<Component />);

    await expect.element(screen.getByText("运行总览真实页面")).toBeVisible();
  });
});
