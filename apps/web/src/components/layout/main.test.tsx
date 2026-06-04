import { describe, expect, it } from "vitest";
import { render } from "vitest-browser-react";
import { Main } from "./main";

describe("Main", () => {
  it("uses a full-width content area by default", async () => {
    const screen = await render(<Main>控制台页面</Main>);
    const main = screen.getByRole("main");

    await expect.element(main).toHaveClass("w-full");
    expect((main.element() as HTMLElement).className).not.toContain("@7xl/content:max-w-7xl");
  });

  it("keeps a contained mode for narrow pages", async () => {
    const screen = await render(<Main contained>窄版页面</Main>);
    const main = screen.getByRole("main");

    await expect.element(main).toHaveClass("@7xl/content:max-w-7xl");
  });
});
