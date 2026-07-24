import type { ReactNode } from "react";
import { Puzzle } from "lucide-react";
import { describe, expect, it, vi } from "vitest";
import { render } from "vitest-browser-react";
import { UnimplementedPage } from "./unimplemented-page";

vi.mock("@/components/layout/header", () => ({
  Header: ({ children }: { children: ReactNode }) => <header>{children}</header>
}));

vi.mock("@/components/layout/main", () => ({
  Main: ({ children }: { children: ReactNode }) => <main>{children}</main>
}));

vi.mock("@/components/search", () => ({
  Search: () => <button type="button">Search</button>
}));

vi.mock("@/components/theme-switch", () => ({
  ThemeSwitch: () => <button type="button">Toggle theme</button>
}));

describe("UnimplementedPage", () => {
  it("uses v3 soft-flat surfaces", async () => {
    const screen = await render(
      <UnimplementedPage
        description="注册和审计企业内部系统能力。"
        icon={Puzzle}
        title="外部能力"
        tone="warning"
      />,
    );

    await expect.element(screen.getByRole("heading", { name: "外部能力" })).toBeVisible();
    await expect.element(screen.getByText("功能建设中")).toBeVisible();

    expect(document.querySelector('[data-slot="icon-tile"]')).not.toBeNull();
    expect(document.querySelector('[data-slot="soft-card"]')).not.toBeNull();
  });
});
