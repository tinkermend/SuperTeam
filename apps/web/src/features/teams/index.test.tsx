import type { ReactNode } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { describe, expect, it, vi } from "vitest";
import { render } from "vitest-browser-react";
import { TeamDetailView, TeamsView } from "@/features/teams";

vi.mock("@/components/layout/header", () => ({
  Header: ({ children }: { children: ReactNode }) => <header>{children}</header>,
}));

vi.mock("@/components/layout/main", () => ({
  Main: ({ children }: { children: ReactNode }) => <main>{children}</main>,
}));

vi.mock("@/components/search", () => ({
  Search: () => <button type="button">Search</button>,
}));

vi.mock("@/components/theme-switch", () => ({
  ThemeSwitch: () => <button type="button">Toggle theme</button>,
}));

function createQueryClient() {
  return new QueryClient({
    defaultOptions: {
      queries: {
        retry: false,
      },
    },
  });
}

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    headers: { "content-type": "application/json" },
    status,
  });
}

function createTeamsFetcher(options: { disabledOverview?: boolean } = {}) {
  const fetcher = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = new URL(String(input));
    const method = init?.method ?? "GET";

    if (url.pathname === "/api/v1/teams" && method === "GET") {
      return jsonResponse([
        {
          id: "team-1",
          tenant_id: "tenant-1",
          slug: "ops",
          name: "运维团队",
          status: "active",
          human_owner_user_id: "human-owner-1",
          member_count: 18,
          digital_employee_count: 6,
          capability_count: 12,
          governance_status: "active",
          current_revision: 7,
          pending_draft_count: 3,
          risk_summary: "生产写操作需审批",
        },
      ]);
    }

    if (url.pathname === "/api/v1/teams/team-1/overview" && method === "GET") {
      return jsonResponse({
        team: {
          id: "team-1",
          tenant_id: "tenant-1",
          slug: "ops",
          name: "运维团队",
          status: options.disabledOverview ? "disabled" : "active",
          human_owner_user_id: "human-owner-1",
        },
        member_count: 18,
        digital_employee_count: 6,
        capability_count: 12,
        pending_draft_count: 3,
        pending_item_count: 3,
        allowed_actions: options.disabledOverview
          ? ["team.restore"]
          : ["team.update", "team.disable", "team.archive", "team.member.add", "team.governance.edit"],
      });
    }

    if (url.pathname === "/api/v1/teams/team-1/disable" && method === "POST") {
      return jsonResponse({ id: "team-1", tenant_id: "tenant-1", slug: "ops", name: "运维团队", status: "disabled" });
    }

    if (url.pathname === "/api/v1/teams/team-1/archive" && method === "POST") {
      return jsonResponse({ id: "team-1", tenant_id: "tenant-1", slug: "ops", name: "运维团队", status: "archived" });
    }

    if (url.pathname === "/api/v1/teams/team-1/restore" && method === "POST") {
      return jsonResponse({ id: "team-1", tenant_id: "tenant-1", slug: "ops", name: "运维团队", status: "active" });
    }

    return jsonResponse({ error: `unhandled ${url.pathname}` }, 404);
  }) as unknown as typeof fetch;

  return fetcher;
}

async function renderWithQueryClient(children: ReactNode) {
  return await render(<QueryClientProvider client={createQueryClient()}>{children}</QueryClientProvider>);
}

describe("TeamsView", () => {
  it("renders a dense team summary table", async () => {
    const fetcher = createTeamsFetcher();
    const screen = await renderWithQueryClient(<TeamsView apiBaseUrl="http://control-plane.local" fetcher={fetcher} />);

    await expect.element(screen.getByRole("heading", { name: "团队管理" })).toBeVisible();
    for (const column of ["负责人", "成员", "数字员工", "能力", "治理状态", "当前版本", "待批准"]) {
      await expect.element(screen.getByText(column)).toBeVisible();
    }
    await expect.element(screen.getByText("运维团队")).toBeVisible();
    await expect.element(screen.getByText("human-owner-1")).toBeVisible();
    await expect.element(screen.getByText("18")).toBeVisible();
    await expect.element(screen.getByText("v7")).toBeVisible();
    await expect.element(screen.getByText("3")).toBeVisible();
  });
});

describe("TeamDetailView", () => {
  it("renders detail tabs for the team shell", async () => {
    const screen = await renderWithQueryClient(
      <TeamDetailView apiBaseUrl="http://control-plane.local" fetcher={createTeamsFetcher()} teamId="team-1" />,
    );

    await expect.element(screen.getByRole("heading", { name: "运维团队" })).toBeVisible();
    for (const tab of ["概览", "成员", "数字员工", "能力与知识", "治理策略", "审计记录"]) {
      await expect.element(screen.getByRole("tab", { name: tab })).toBeVisible();
    }
    await screen.getByRole("tab", { name: "成员" }).click();
    await expect.element(screen.getByText("Plan 2 会接入成员与角色管理。")).toBeVisible();
    await expect.element(screen.getByRole("button", { name: "添加成员" })).toBeVisible();
    await expect.element(screen.getByRole("button", { name: "创建治理草案" })).toBeVisible();
    await expect.element(screen.getByRole("button", { name: "禁用团队" })).toBeVisible();
    await expect.element(screen.getByRole("button", { name: "归档团队" })).toBeVisible();
  });

  it("calls lifecycle APIs from detail actions", async () => {
    const fetcher = createTeamsFetcher();
    const screen = await renderWithQueryClient(
      <TeamDetailView apiBaseUrl="http://control-plane.local" fetcher={fetcher} teamId="team-1" />,
    );

    await screen.getByRole("button", { name: "禁用团队" }).click();

    expect(fetcher).toHaveBeenCalledWith(
      "http://control-plane.local/api/v1/teams/team-1/disable",
      expect.objectContaining({
        credentials: "include",
        method: "POST",
      }),
    );

    await screen.getByRole("button", { name: "归档团队" }).click();

    expect(fetcher).toHaveBeenCalledWith(
      "http://control-plane.local/api/v1/teams/team-1/archive",
      expect.objectContaining({
        credentials: "include",
        method: "POST",
      }),
    );
  });

  it("does not show member or governance creation actions for a disabled team", async () => {
    const fetcher = createTeamsFetcher({ disabledOverview: true });
    const screen = await renderWithQueryClient(
      <TeamDetailView apiBaseUrl="http://control-plane.local" fetcher={fetcher} teamId="team-1" />,
    );

    await expect.element(screen.getByRole("heading", { name: "运维团队" })).toBeVisible();
    await expect.element(screen.getByText("已禁用")).toBeVisible();
    await expect.element(screen.getByRole("button", { name: "添加成员" })).not.toBeInTheDocument();
    await expect.element(screen.getByRole("button", { name: "创建治理草案" })).not.toBeInTheDocument();
    await screen.getByRole("button", { name: "恢复团队" }).click();
    expect(fetcher).toHaveBeenCalledWith(
      "http://control-plane.local/api/v1/teams/team-1/restore",
      expect.objectContaining({
        credentials: "include",
        method: "POST",
      }),
    );
  });
});
