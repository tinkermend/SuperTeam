import { describe, expect, it } from "vitest";
import { page } from "vitest/browser";
import { render } from "vitest-browser-react";
import { MasterDetailLayout, MetricGrid } from "./layout";

describe("MasterDetailLayout", () => {
  it("renders master full-width without a rail when no detail is provided", async () => {
    const screen = await render(
      <MasterDetailLayout data-testid="layout-root" master={<section>队列</section>} />,
    );

    await expect.element(screen.getByText("队列")).toBeInTheDocument();
    const root = screen.getByTestId("layout-root").element() as HTMLElement;
    expect(root.className).toContain("@container/master-detail");
    expect(root.firstElementChild?.className).not.toContain("grid-cols");
    expect(document.querySelector('[data-slot="sheet-content"]')).toBeNull();
  });

  it("renders detail as an in-flow rail on wide containers", async () => {
    await page.viewport(1400, 900);
    try {
      const screen = await render(
        <MasterDetailLayout
          data-testid="layout-root"
          detail={<aside>右栏</aside>}
          master={<section>队列</section>}
          rail="lg"
        />,
      );

      await expect.element(screen.getByText("右栏")).toBeInTheDocument();
      const grid = (screen.getByTestId("layout-root").element() as HTMLElement)
        .firstElementChild as HTMLElement;
      expect(grid.className).toContain(
        "@5xl/master-detail:grid-cols-[minmax(0,1fr)_minmax(min(100%,18rem),var(--v3-layout-rail-lg))]",
      );
      expect(document.querySelector('[data-slot="sheet-content"]')).toBeNull();
    } finally {
      await page.viewport(414, 896);
    }
  });

  it("renders detail in a right sheet on narrow containers and dismisses on close", async () => {
    let dismissed = 0;
    const screen = await render(
      <MasterDetailLayout
        data-testid="layout-root"
        detail={<aside>右栏内容</aside>}
        master={<section>队列</section>}
        detailLabel="选中上下文"
        onDetailDismiss={() => {
          dismissed += 1;
        }}
        rail="lg"
      />,
    );

    await expect.element(screen.getByText("右栏内容")).toBeInTheDocument();
    const sheet = document.querySelector('[data-slot="sheet-content"]');
    expect(sheet).toBeTruthy();
    expect(sheet?.textContent).toContain("右栏内容");
    // in-flow 栅格保持单列，不保留空轨道
    const grid = (screen.getByTestId("layout-root").element() as HTMLElement)
      .firstElementChild as HTMLElement;
    expect(grid.className).not.toContain("grid-cols");

    const close = document.querySelector(
      '[data-slot="sheet-content"] button',
    ) as HTMLElement;
    close.click();
    await expect.poll(() => dismissed).toBe(1);
  });
});

describe("MetricGrid", () => {
  it("fills the row with elastic cards at a constant gap", async () => {
    const screen = await render(
      <MetricGrid aria-label="项目组合概览" data-testid="metric-grid">
        <div>卡片一</div>
        <div>卡片二</div>
      </MetricGrid>,
    );

    const grid = screen.getByTestId("metric-grid").element() as HTMLElement;
    expect(grid.tagName).toBe("SECTION");
    expect(grid.className).toContain("flex-wrap");
    expect(grid.className).toContain("gap-3");
    expect(grid.className).toContain("[&>*]:flex-[1_1_var(--v3-metric-min)]");
    // 宽屏由卡片变宽吸收空间：无宽度上限、无间距分布
    expect(grid.className).not.toContain("max-w-");
    expect(grid.className).not.toContain("justify-between");
    await expect.element(screen.getByText("卡片一")).toBeInTheDocument();
  });
});
