import { describe, expect, it } from "vitest";
import { render } from "vitest-browser-react";
import { Main } from "./main";

describe("Main", () => {
  it("defaults to the contained tier so untiered pages stay safe", async () => {
    const screen = await render(<Main>控制台页面</Main>);
    const main = screen.getByRole("main");

    await expect.element(main).toHaveClass("w-full");
    await expect
      .element(main)
      .toHaveClass("@7xl/content:max-w-(--layout-contained)");
    expect((main.element() as HTMLElement).dataset.width).toBe("contained");
  });

  it("maps the legacy fluid prop to the canvas tier", async () => {
    const screen = await render(<Main fluid>全宽页面</Main>);
    const main = screen.getByRole("main");

    expect((main.element() as HTMLElement).className).not.toContain("max-w-");
    expect((main.element() as HTMLElement).dataset.width).toBe("canvas");
  });

  it("keeps a contained mode for narrow pages", async () => {
    const screen = await render(<Main contained>窄版页面</Main>);
    const main = screen.getByRole("main");

    await expect
      .element(main)
      .toHaveClass("@7xl/content:max-w-(--layout-contained)");
    expect((main.element() as HTMLElement).dataset.width).toBe("contained");
  });

  it("caps master-detail workbench pages at the wide tier", async () => {
    const screen = await render(<Main width="wide">主从工作台</Main>);
    const main = screen.getByRole("main");

    await expect.element(main).toHaveClass("max-w-(--layout-wide)");
    await expect.element(main).toHaveClass("mx-auto");
    expect((main.element() as HTMLElement).dataset.width).toBe("wide");
  });

  it("keeps canvas pages full-width", async () => {
    const screen = await render(<Main width="canvas">画布页面</Main>);
    const main = screen.getByRole("main");

    expect((main.element() as HTMLElement).className).not.toContain("max-w-");
    expect((main.element() as HTMLElement).dataset.width).toBe("canvas");
  });
});
