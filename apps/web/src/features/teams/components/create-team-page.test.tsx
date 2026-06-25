import type { ReactNode } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { userEvent } from "vitest/browser";
import { describe, expect, it, vi } from "vitest";
import { render } from "vitest-browser-react";
import { CreateTeamView } from "./create-team-page";

function createQueryClient() {
  return new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
}

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    headers: { "content-type": "application/json" },
    status,
  });
}

function fetchCalls(fetcher: typeof fetch) {
  return (
    fetcher as unknown as {
      mock: { calls: [RequestInfo | URL, RequestInit | undefined][] };
    }
  ).mock.calls;
}

function createTeamPostBody(fetcher: typeof fetch) {
  const postCall = fetchCalls(fetcher).find(
    ([url, init]) =>
      String(url).endsWith("/api/v1/teams") && init?.method === "POST",
  );

  return JSON.parse(String(postCall?.[1]?.body));
}

function createFetcher(options: { createStatus?: number } = {}) {
  return vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = new URL(String(input));
    const method = init?.method ?? "GET";

    if (url.pathname === "/api/auth/users" && method === "GET") {
      const q = url.searchParams.get("q")?.trim().toLowerCase();
      const users = ["owner", "member", "viewer"].map((username) => ({
        avatar: { provider: "dicebear", seed: username, style: "adventurer" },
        id: `${username}-user`,
        status: "active",
        username,
      }));

      return jsonResponse({
        items: q ? users.filter((user) => user.username.includes(q)) : users,
      });
    }

    if (url.pathname === "/api/digital-employees" && method === "GET") {
      return jsonResponse({
        items: [],
      });
    }

    if (url.pathname === "/api/v1/teams" && method === "POST") {
      if (options.createStatus && options.createStatus >= 400) {
        return jsonResponse(
          { error: "create team unavailable" },
          options.createStatus,
        );
      }

      return jsonResponse(
        {
          team: {
            id: "team-security",
            tenant_id: "tenant-1",
            name: "安全团队",
            slug: "security",
            status: "active",
          },
          member_count: 1,
          digital_employee_count: 0,
          capability_count: 0,
          pending_draft_count: 0,
          pending_item_count: 0,
          allowed_actions: [],
        },
        201,
      );
    }

    return jsonResponse({ error: `unhandled ${url.pathname}` }, 404);
  }) as unknown as typeof fetch;
}

async function renderView(node: ReactNode) {
  return await render(
    <QueryClientProvider client={createQueryClient()}>
      {node}
    </QueryClientProvider>,
  );
}

describe("CreateTeamView", () => {
  it("validates name, slug, and owner before moving to next steps", async () => {
    const fetcher = createFetcher();
    const screen = await renderView(
      <CreateTeamView apiBaseUrl="http://control-plane.local" fetcher={fetcher} />,
    );

    // Try next without filling name and slug
    await userEvent.click(screen.getByRole("button", { name: "下一步: 负责人和数字员工" }));

    await expect.element(screen.getByText("团队名称不能为空")).toBeVisible();
    await expect.element(screen.getByText("团队标识不能为空")).toBeVisible();

    // Fill name and slug
    await userEvent.fill(
      screen.getByRole("textbox", { name: "团队名称", exact: true }),
      "安全团队",
    );
    await userEvent.fill(
      screen.getByRole("textbox", { name: "团队标识 slug", exact: true }),
      "security",
    );
    
    // Now next works
    await userEvent.click(screen.getByRole("button", { name: "下一步: 负责人和数字员工" }));

    // Try next without filling owner
    await userEvent.click(screen.getByRole("button", { name: "下一步: 权限与设置" }));
    await expect.element(screen.getByText("请至少选择一位负责人")).toBeVisible();

    expect(
      fetchCalls(fetcher).some(
        ([url, init]) =>
          String(url).endsWith("/api/v1/teams") && init?.method === "POST",
      ),
    ).toBe(false);
  });

  it("creates a team through the multi-step wizard", async () => {
    const fetcher = createFetcher();
    const onCreated = vi.fn();
    const screen = await renderView(
      <CreateTeamView
        apiBaseUrl="http://control-plane.local"
        fetcher={fetcher}
        onCreated={onCreated}
      />,
    );

    // Step 1
    await userEvent.fill(
      screen.getByRole("textbox", { name: "团队名称", exact: true }),
      "安全团队",
    );
    await userEvent.fill(
      screen.getByRole("textbox", { name: "团队标识 slug", exact: true }),
      "security",
    );
    await userEvent.click(screen.getByRole("button", { name: "下一步: 负责人和数字员工" }));

    // Step 2
    await userEvent.type(
      screen.getByRole("searchbox", { name: "搜索平台注册用户" }),
      "owner",
    );
    await userEvent.click(screen.getByRole("button", { name: "选择 owner" }));
    await userEvent.click(screen.getByRole("button", { name: "下一步: 权限与设置" }));

    // Step 3
    await userEvent.click(screen.getByRole("button", { name: "下一步: 确认并创建" }));

    // Step 4
    await userEvent.click(screen.getByRole("button", { name: "确认并创建" }));

    await expect
      .poll(() =>
        fetchCalls(fetcher).some(
          ([url, init]) =>
            String(url).endsWith("/api/v1/teams") && init?.method === "POST",
        ),
      )
      .toBe(true);
    expect(createTeamPostBody(fetcher)).toEqual({
      human_owner_user_ids: ["owner-user"],
      initial_digital_employee_ids: [],
      metadata: {
        display: {
          color_tone: "teal",
          icon_key: "security", } },
      name: "安全团队",
      slug: "security",
    });
    await expect.poll(() => onCreated.mock.calls.length).toBe(1);
    expect(onCreated.mock.calls[0][1]).toEqual({ goToGovernance: false });
  });

  it("surfaces a create error returned by the API", async () => {
    const fetcher = createFetcher({ createStatus: 500 });
    const screen = await renderView(
      <CreateTeamView apiBaseUrl="http://control-plane.local" fetcher={fetcher} />,
    );

    // Step 1
    await userEvent.fill(
      screen.getByRole("textbox", { name: "团队名称", exact: true }),
      "安全团队",
    );
    await userEvent.fill(
      screen.getByRole("textbox", { name: "团队标识 slug", exact: true }),
      "security",
    );
    await userEvent.click(screen.getByRole("button", { name: "下一步: 负责人和数字员工" }));

    // Step 2
    await userEvent.type(
      screen.getByRole("searchbox", { name: "搜索平台注册用户" }),
      "owner",
    );
    await userEvent.click(screen.getByRole("button", { name: "选择 owner" }));
    await userEvent.click(screen.getByRole("button", { name: "下一步: 权限与设置" }));

    // Step 3
    await userEvent.click(screen.getByRole("button", { name: "下一步: 确认并创建" }));

    // Step 4
    await userEvent.click(screen.getByRole("button", { name: "确认并创建" }));

    await expect
      .element(screen.getByText("create team request failed with status 500"))
      .toBeVisible();
  });
});
