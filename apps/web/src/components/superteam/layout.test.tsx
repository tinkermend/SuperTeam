import { describe, expect, it } from "vitest";
import { render } from "vitest-browser-react";
import { MasterDetailLayout, MetricGrid } from "./layout";

describe("MasterDetailLayout", () => {
  it("renders master and detail slots inside a named container", async () => {
    const screen = await render(
      <MasterDetailLayout
        data-testid="layout-root"
        detail={<aside>右栏</aside>}
        master={<section>队列</section>}
      />,
    );

    await expect.element(screen.getByText("队列")).toBeInTheDocument();
    await expect.element(screen.getByText("右栏")).toBeInTheDocument();
    const root = screen.getByTestId("layout-root").element() as HTMLElement;
    expect(root.className).toContain("@container/master-detail");
  });

  it("expands the standard rail at the @4xl container breakpoint", async () => {
    const screen = await render(
      <MasterDetailLayout
        data-testid="layout-root"
        detail={<aside>右栏</aside>}
        master={<section>队列</section>}
      />,
    );

    const grid = (screen.getByTestId("layout-root").element() as HTMLElement)
      .firstElementChild as HTMLElement;
    expect(grid.className).toContain(
      "@4xl/master-detail:grid-cols-[minmax(0,1fr)_var(--v3-layout-rail)]",
    );
  });

  it("expands the large rail at the @5xl container breakpoint", async () => {
    const screen = await render(
      <MasterDetailLayout
        data-testid="layout-root"
        detail={<aside>右栏</aside>}
        master={<section>队列</section>}
        rail="lg"
      />,
    );

    const grid = (screen.getByTestId("layout-root").element() as HTMLElement)
      .firstElementChild as HTMLElement;
    expect(grid.className).toContain(
      "@5xl/master-detail:grid-cols-[minmax(0,1fr)_var(--v3-layout-rail-lg)]",
    );
  });
});

describe("MetricGrid", () => {
  it("bounds metric card width with the auto-fit token track", async () => {
    const screen = await render(
      <MetricGrid aria-label="项目组合概览" data-testid="metric-grid">
        <div>卡片一</div>
        <div>卡片二</div>
      </MetricGrid>,
    );

    const grid = screen.getByTestId("metric-grid").element() as HTMLElement;
    expect(grid.tagName).toBe("SECTION");
    expect(grid.className).toContain("flex-wrap");
    expect(grid.className).toContain("[&>*]:max-w-(--v3-metric-max)");
    expect(grid.className).toContain("[&>*]:flex-[1_1_var(--v3-metric-min)]");
    await expect.element(screen.getByText("卡片一")).toBeInTheDocument();
  });
});
