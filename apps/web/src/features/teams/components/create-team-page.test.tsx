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

async function selectOwner(screen: Awaited<ReturnType<typeof renderView>>) {
  await userEvent.type(screen.getByRole("searchbox", { name: "负责人" }), "owner");
  await userEvent.click(screen.getByRole("button", { name: "选择 owner" }));
}

describe("CreateTeamView", () => {
  it("validates name, slug, and owner before submitting", async () => {
    const fetcher = createFetcher();
    const screen = await renderView(
      <CreateTeamView apiBaseUrl="http://control-plane.local" fetcher={fetcher} />,
    );

    await userEvent.click(screen.getByRole("button", { name: "创建团队" }));

    await expect.element(screen.getByText("团队名称不能为空")).toBeVisible();
    await expect.element(screen.getByText("团队标识不能为空")).toBeVisible();
    await expect.element(screen.getByText("请选择负责人")).toBeVisible();
    expect(
      fetchCalls(fetcher).some(
        ([url, init]) =>
          String(url).endsWith("/api/v1/teams") && init?.method === "POST",
      ),
    ).toBe(false);
  });

  it("creates a team with display metadata, owner, and initial members", async () => {
    const fetcher = createFetcher();
    const onCreated = vi.fn();
    const screen = await renderView(
      <CreateTeamView
        apiBaseUrl="http://control-plane.local"
        fetcher={fetcher}
        onCreated={onCreated}
      />,
    );

    await userEvent.fill(
      screen.getByRole("textbox", { name: "团队名称", exact: true }),
      "安全团队",
    );
    await userEvent.fill(
      screen.getByRole("textbox", { name: "团队标识 slug", exact: true }),
      "security",
    );
    await selectOwner(screen);

    await userEvent.click(screen.getByRole("button", { name: "添加初始成员" }));
    await userEvent.click(screen.getByRole("button", { name: "添加 member" }));
    await userEvent.click(screen.getByRole("button", { name: "添加 viewer" }));
    await userEvent.click(screen.getByRole("button", { name: "移除 viewer" }));
    await userEvent.click(screen.getByRole("button", { name: "创建团队" }));

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
      initial_members: [{ role: "member", user_id: "member-user" }],
      metadata: { display: { color_tone: "teal", icon_key: "security" } },
      name: "安全团队",
      slug: "security",
    });
    await expect.poll(() => onCreated.mock.calls.length).toBe(1);
    expect(onCreated.mock.calls[0][1]).toEqual({ goToGovernance: false });
  });

  it("keeps the inferred icon until one is manually selected", async () => {
    const fetcher = createFetcher();
    const screen = await renderView(
      <CreateTeamView apiBaseUrl="http://control-plane.local" fetcher={fetcher} />,
    );

    const iconTrigger = screen.getByRole("button", { name: "选择团队图标" });

    await userEvent.fill(
      screen.getByRole("textbox", { name: "团队名称", exact: true }),
      "安全团队",
    );
    await expect.element(iconTrigger).toHaveTextContent("security");

    await userEvent.fill(
      screen.getByRole("textbox", { name: "团队标识 slug", exact: true }),
      "ops",
    );
    await expect.element(iconTrigger).toHaveTextContent("ops");

    // 手动从图标库选择后，后续名称/标识变化不再覆盖。
    await userEvent.click(iconTrigger);
    await userEvent.fill(screen.getByRole("searchbox", { name: "搜索图标" }), "rocket");
    await userEvent.click(screen.getByRole("button", { name: "选择图标 rocket" }));
    await expect.element(iconTrigger).toHaveTextContent("rocket");

    await userEvent.fill(
      screen.getByRole("textbox", { name: "团队标识 slug", exact: true }),
      "dev",
    );
    await expect.element(iconTrigger).toHaveTextContent("rocket");
  });

  it("surfaces a create error returned by the API", async () => {
    const fetcher = createFetcher({ createStatus: 500 });
    const screen = await renderView(
      <CreateTeamView apiBaseUrl="http://control-plane.local" fetcher={fetcher} />,
    );

    await userEvent.fill(
      screen.getByRole("textbox", { name: "团队名称", exact: true }),
      "安全团队",
    );
    await userEvent.fill(
      screen.getByRole("textbox", { name: "团队标识 slug", exact: true }),
      "security",
    );
    await selectOwner(screen);
    await userEvent.click(screen.getByRole("button", { name: "创建团队" }));

    await expect
      .element(screen.getByText("create team request failed with status 500"))
      .toBeVisible();
  });

  it("drops the chosen owner from initial members", async () => {
    const fetcher = createFetcher();
    const screen = await renderView(
      <CreateTeamView apiBaseUrl="http://control-plane.local" fetcher={fetcher} />,
    );

    await userEvent.fill(
      screen.getByRole("textbox", { name: "团队名称", exact: true }),
      "安全团队",
    );
    await userEvent.fill(
      screen.getByRole("textbox", { name: "团队标识 slug", exact: true }),
      "security",
    );
    await userEvent.click(screen.getByRole("button", { name: "添加初始成员" }));
    await userEvent.click(screen.getByRole("button", { name: "添加 member" }));
    await userEvent.type(
      screen.getByRole("searchbox", { name: "负责人" }),
      "member",
    );
    await userEvent.click(screen.getByRole("button", { name: "选择 member" }));
    await userEvent.click(screen.getByRole("button", { name: "创建团队" }));

    await expect
      .poll(() =>
        fetchCalls(fetcher).some(
          ([url, init]) =>
            String(url).endsWith("/api/v1/teams") && init?.method === "POST",
        ),
      )
      .toBe(true);
    expect(createTeamPostBody(fetcher)).toEqual({
      human_owner_user_ids: ["member-user"],
      initial_members: [],
      metadata: { display: { color_tone: "teal", icon_key: "security" } },
      name: "安全团队",
      slug: "security",
    });
  });
});
