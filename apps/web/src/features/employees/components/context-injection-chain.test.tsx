import { describe, expect, it } from "vitest";
import { render } from "vitest-browser-react";
import { ContextInjectionChain } from "./context-injection-chain";

describe("ContextInjectionChain", () => {
  it("renders 8 ordered nodes with project constitution, persona memory and counts", async () => {
    const screen = await render(
      <ContextInjectionChain
        envConfiguredCount={7}
        envTotalCount={9}
        hasPersonaMemory
        mcpCount={2}
        roleLabel="backend_engineer"
        personalSkillCount={3}
        inheritedSkillCount={6}
      />,
    );

    await expect.element(screen.getByText("角色说明")).toBeVisible();
    await expect.element(screen.getByText("项目宪法")).toBeVisible();
    await expect.element(screen.getByText("人格记忆")).toBeVisible();
    await expect.element(screen.getByText("已配置")).toBeVisible();
    // Distinct counts per skill node — no .first() workaround needed.
    await expect.element(screen.getByText("3 项")).toBeVisible();
    await expect.element(screen.getByText("6 项")).toBeVisible();
    await expect.element(screen.getByText("7 / 9")).toBeVisible();
  });
});
