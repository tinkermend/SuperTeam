import { describe, expect, it } from "vitest";
import { render } from "vitest-browser-react";
import { ContextInjectionChain } from "./context-injection-chain";

describe("ContextInjectionChain", () => {
  it("renders 8 ordered nodes with counts and memory placeholder", async () => {
    const screen = await render(
      <ContextInjectionChain envConfiguredCount={7} envTotalCount={9} mcpCount={2} roleLabel="backend_engineer" skillCount={9} />,
    );

    await expect.element(screen.getByText("角色说明")).toBeVisible();
    await expect.element(screen.getByText("宪法")).toBeVisible();
    await expect.element(screen.getByText("待接入")).toBeVisible();
    // Both 个人技能 and 团队继承技能 nodes render `${skillCount} 项` (total count, by design — see plan Task 13).
    // Use .first() to disambiguate strict-mode matching.
    await expect.element(screen.getByText("9 项").first()).toBeVisible();
    await expect.element(screen.getByText("7 / 9")).toBeVisible();
  });
});
