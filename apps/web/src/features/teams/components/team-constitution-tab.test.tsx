import type { ReactNode } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { userEvent } from "vitest/browser";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { render } from "vitest-browser-react";
import type { ApiClientOptions } from "@/lib/api/client";
import { TeamConstitutionTab } from "./team-constitution-tab";

const { updateTeamConstitution } = vi.hoisted(() => ({
  updateTeamConstitution: vi.fn()
}));

vi.mock("@/lib/api/teams", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/api/teams")>();
  return {
    ...actual,
    updateTeamConstitution
};
});

function createQueryClient() {
  return new QueryClient({
    defaultOptions: {
      queries: { retry: false }
}
});
}

async function renderView(node: ReactNode) {
  return await render(
    <QueryClientProvider client={createQueryClient()}>
      {node}
    </QueryClientProvider>,
  );
}

const apiOptions: ApiClientOptions = {
  baseUrl: "http://control-plane.local",
  fetcher: vi.fn() as unknown as typeof fetch
};

describe("TeamConstitutionTab", () => {
  beforeEach(() => {
    updateTeamConstitution.mockReset();
    updateTeamConstitution.mockResolvedValue({
      constitution: {
        hard_rules: ["规则一", "规则二"]
}
});
  });

  it("renders hard rules and saves edited constitution", async () => {
    const onSaved = vi.fn();
    const screen = await renderView(
      <TeamConstitutionTab
        apiOptions={apiOptions}
        canEdit
        constitution={{ hard_rules: ["规则一"] }}
        onSaved={onSaved}
        teamId="team-1"
      />,
    );

    await expect.element(screen.getByLabelText("团队宪法")).toHaveValue("规则一");

    await userEvent.fill(screen.getByLabelText("团队宪法"), "规则一\n规则二");
    await userEvent.click(screen.getByRole("button", { name: "保存宪法" }));

    await expect.poll(() => updateTeamConstitution.mock.calls.length).toBe(1);
    expect(updateTeamConstitution).toHaveBeenCalledWith(apiOptions, "team-1", {
      hard_rules: ["规则一", "规则二"]
});
    await expect.poll(() => onSaved.mock.calls.length).toBe(1);
  });

  it("preserves existing constitution keys when saving hard rules", async () => {
    const screen = await renderView(
      <TeamConstitutionTab
        apiOptions={apiOptions}
        canEdit
        constitution={{
          approval_policy: { high_risk: "required" },
          hard_rules: ["规则一"],
          principles: ["安全优先"]
}}
        teamId="team-1"
      />,
    );

    await userEvent.fill(screen.getByLabelText("团队宪法"), "规则一\n规则二");
    await userEvent.click(screen.getByRole("button", { name: "保存宪法" }));

    await expect.poll(() => updateTeamConstitution.mock.calls.length).toBe(1);
    expect(updateTeamConstitution).toHaveBeenCalledWith(apiOptions, "team-1", {
      approval_policy: { high_risk: "required" },
      hard_rules: ["规则一", "规则二"],
      principles: ["安全优先"]
});
  });

  it("does not render approval or diff UI", async () => {
    const screen = await renderView(
      <TeamConstitutionTab
        apiOptions={apiOptions}
        canEdit
        constitution={{ hard_rules: ["规则一"] }}
        teamId="team-1"
      />,
    );

    await expect.element(screen.getByLabelText("团队宪法")).toBeVisible();
    await expect.element(screen.getByText("1 条硬性规则")).toBeVisible();
    await expect.element(screen.getByText("审批策略")).not.toBeInTheDocument();
    await expect.element(screen.getByText("JSON 快照预览")).not.toBeInTheDocument();
    await expect.element(screen.getByText("相对当前版本的变更")).not.toBeInTheDocument();
  });
});
