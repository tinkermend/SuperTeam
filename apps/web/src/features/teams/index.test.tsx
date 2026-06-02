import type { ReactNode } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { describe, expect, it, vi } from "vitest";
import { render } from "vitest-browser-react";
import { TeamsView } from "@/features/teams";

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

function createTeamsFetcher(options: { currentConfigStatus?: number; useLegacyAutomaticRounds?: boolean } = {}) {
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
        },
      ]);
    }

    if (url.pathname === "/api/v1/teams/team-1/config-revisions/current" && method === "GET") {
      if (options.currentConfigStatus && options.currentConfigStatus >= 400) {
        return jsonResponse({ error: "not ready" }, options.currentConfigStatus);
      }

      return jsonResponse({
        id: "revision-1",
        tenant_id: "tenant-1",
        team_id: "team-1",
        revision_number: 3,
        constitution: {
          hard_rules: ["禁止执行未审批的生产写操作"],
        },
        capability_policy: {
          allowed_skills: ["incident-diagnosis"],
        },
        context_policy: {},
        approval_policy: {},
        artifact_contract: {},
        internal_collaboration_policy: options.useLegacyAutomaticRounds
          ? {
              automatic_rounds: 2,
            }
          : {
              max_auto_rounds: 2,
            },
        runtime_scope_policy: {},
        human_owner_user_id: "human-owner-1",
        status: "active",
      });
    }

    return jsonResponse({ error: `unhandled ${url.pathname}` }, 404);
  }) as unknown as typeof fetch;

  return fetcher;
}

async function renderTeamsView(fetcher = createTeamsFetcher()) {
  return await render(
    <QueryClientProvider client={createQueryClient()}>
      <TeamsView apiBaseUrl="http://control-plane.local" fetcher={fetcher} />
    </QueryClientProvider>,
  );
}

describe("TeamsView", () => {
  it("renders teams with current governance config details", async () => {
    const fetcher = createTeamsFetcher();
    const screen = await renderTeamsView(fetcher);

    await expect.element(screen.getByRole("heading", { name: "团队管理" })).toBeVisible();
    await expect.element(screen.getByText("运维团队")).toBeVisible();
    await expect.element(screen.getByText("ops / active")).toBeVisible();
    await expect.element(screen.getByText("human-owner-1")).toBeVisible();
    await expect.element(screen.getByText("修订号 3")).toBeVisible();
    await expect.element(screen.getByText("禁止执行未审批的生产写操作")).toBeVisible();
    await expect.element(screen.getByText("incident-diagnosis")).toBeVisible();
    await expect.element(screen.getByText("内部协作自动轮次 2")).toBeVisible();
    expect(fetcher).toHaveBeenCalledWith(
      "http://control-plane.local/api/v1/teams/team-1/config-revisions/current",
      expect.objectContaining({
        credentials: "include",
        method: "GET",
      }),
    );
  });

  it("keeps the page usable when current config is not ready", async () => {
    const screen = await renderTeamsView(createTeamsFetcher({ currentConfigStatus: 404 }));

    await expect.element(screen.getByRole("heading", { name: "团队管理" })).toBeVisible();
    await expect.element(screen.getByText("运维团队")).toBeVisible();
    await expect.element(screen.getByText("当前治理配置未就绪")).toBeVisible();
  });

  it("falls back to legacy automatic rounds when max auto rounds is absent", async () => {
    const screen = await renderTeamsView(createTeamsFetcher({ useLegacyAutomaticRounds: true }));

    await expect.element(screen.getByText("内部协作自动轮次 2")).toBeVisible();
  });
});
