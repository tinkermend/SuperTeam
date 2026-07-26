import type { ReactNode } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { userEvent } from "vitest/browser";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { render } from "vitest-browser-react";
import type { ApiClientOptions } from "@/lib/api/client";
import { TeamConstitutionTab } from "./team-constitution-tab";

const { listTeamConstitutionRevisions, rollbackTeamConstitution, saveTeamConstitution } =
  vi.hoisted(() => ({
    listTeamConstitutionRevisions: vi.fn(),
    rollbackTeamConstitution: vi.fn(),
    saveTeamConstitution: vi.fn()
  }));

vi.mock("@/lib/api/teams", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/api/teams")>();
  return {
    ...actual,
    listTeamConstitutionRevisions,
    rollbackTeamConstitution,
    saveTeamConstitution
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
    <QueryClientProvider client={createQueryClient()}>{node}</QueryClientProvider>,
  );
}

const apiOptions: ApiClientOptions = {
  baseUrl: "http://control-plane.local",
  fetcher: vi.fn() as unknown as typeof fetch
};

describe("TeamConstitutionTab", () => {
  beforeEach(() => {
    saveTeamConstitution.mockReset();
    rollbackTeamConstitution.mockReset();
    listTeamConstitutionRevisions.mockReset();
    saveTeamConstitution.mockResolvedValue({
      id: "rev-2",
      revision_number: 2,
      rules: [],
      change_note: "note"
    });
    rollbackTeamConstitution.mockResolvedValue({
      id: "rev-3",
      revision_number: 3,
      rules: [{ id: "r1", text: "规则一", category: "must" }],
      change_note: "回滚到版本 v1"
    });
    listTeamConstitutionRevisions.mockResolvedValue([
      {
        id: "rev-2",
        tenant_id: "t",
        team_id: "team-1",
        revision_number: 2,
        rules: [{ id: "r2", text: "规则二", category: "forbid" }],
        change_note: "收紧",
        created_at: "2026-07-26T02:00:00Z"
      },
      {
        id: "rev-1",
        tenant_id: "t",
        team_id: "team-1",
        revision_number: 1,
        rules: [{ id: "r1", text: "规则一", category: "must" }],
        change_note: "初始版本",
        created_at: "2026-07-26T01:00:00Z"
      }
    ]);
  });

  it("saves edited rules as a new revision and requires a change note", async () => {
    const onSaved = vi.fn();
    const screen = await renderView(
      <TeamConstitutionTab
        apiOptions={apiOptions}
        canEdit
        constitution={{ rules: [{ id: "r1", text: "规则一", category: "must" }] }}
        onSaved={onSaved}
        teamId="team-1"
      />,
    );

    await expect.element(screen.getByRole("textbox", { name: "第 1 条规则" })).toHaveValue("规则一");
    await userEvent.fill(screen.getByRole("textbox", { name: "第 1 条规则" }), "规则一改");
    await userEvent.click(screen.getByRole("button", { name: "预览并保存" }));

    // 变更说明必填：宪法改动对全队所有派发生效，事后要能回答"为什么改"。
    await expect.element(screen.getByRole("button", { name: "保存为新版本" })).toBeDisabled();
    await userEvent.fill(screen.getByLabelText("变更说明"), "收紧访问");
    await userEvent.click(screen.getByRole("button", { name: "保存为新版本" }));

    await expect.poll(() => saveTeamConstitution.mock.calls.length).toBe(1);
    expect(saveTeamConstitution).toHaveBeenCalledWith(apiOptions, "team-1", {
      rules: [{ id: "r1", text: "规则一改", category: "must" }],
      change_note: "收紧访问"
    });
    await expect.poll(() => onSaved.mock.calls.length).toBe(1);
  });

  it("previews the diff before saving", async () => {
    const screen = await renderView(
      <TeamConstitutionTab
        apiOptions={apiOptions}
        canEdit
        constitution={{ rules: [{ id: "r1", text: "规则一", category: "must" }] }}
        teamId="team-1"
      />,
    );

    await userEvent.click(screen.getByRole("button", { name: "添加规则" }));
    await userEvent.fill(screen.getByRole("textbox", { name: "第 2 条规则" }), "新增规则");
    await userEvent.click(screen.getByRole("button", { name: "预览并保存" }));

    await expect.element(screen.getByText("+ 新增规则")).toBeVisible();
  });

  it("falls back to legacy hard_rules for teams saved before structured rules", async () => {
    const screen = await renderView(
      <TeamConstitutionTab
        apiOptions={apiOptions}
        canEdit
        constitution={{ hard_rules: ["旧规则一", "旧规则二"] }}
        teamId="team-1"
      />,
    );

    await expect.element(screen.getByText("2 条规则")).toBeVisible();
    await expect.element(screen.getByRole("textbox", { name: "第 1 条规则" })).toHaveValue("旧规则一");
  });

  it("offers rollback only for revisions that are not currently in effect", async () => {
    const screen = await renderView(
      <TeamConstitutionTab
        apiOptions={apiOptions}
        canEdit
        constitution={{ rules: [{ id: "r2", text: "规则二", category: "forbid" }] }}
        teamId="team-1"
      />,
    );

    await expect.element(screen.getByRole("heading", { name: "版本历史" })).toBeVisible();
    await expect.element(screen.getByText("当前生效")).toBeVisible();

    await userEvent.click(screen.getByRole("button", { name: "回滚到此版本" }));
    await expect.poll(() => rollbackTeamConstitution.mock.calls.length).toBe(1);
    expect(rollbackTeamConstitution).toHaveBeenCalledWith(apiOptions, "team-1", 1);
  });

  it("hides editing affordances without permission", async () => {
    const screen = await renderView(
      <TeamConstitutionTab
        apiOptions={apiOptions}
        canEdit={false}
        constitution={{ rules: [{ id: "r1", text: "规则一", category: "must" }] }}
        teamId="team-1"
      />,
    );

    await expect.element(screen.getByRole("button", { name: "添加规则" })).not.toBeInTheDocument();
    await expect.element(screen.getByRole("button", { name: "预览并保存" })).toBeDisabled();
  });
});
