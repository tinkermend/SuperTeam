import type { ReactNode } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { userEvent } from "vitest/browser";
import { describe, expect, it, vi } from "vitest";
import { render } from "vitest-browser-react";
import { CreateTeamView } from "./create-team-page";

function createQueryClient() {
  return new QueryClient({
    defaultOptions: { queries: { retry: false } }
});
}

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    headers: { "content-type": "application/json" },
    status
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

function createFetcher(
  options: {
    createStatus?: number;
    digitalEmployees?: unknown[];
  } = {},
) {
  return vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = new URL(String(input));
    const method = init?.method ?? "GET";

    if (url.pathname === "/api/auth/users" && method === "GET") {
      const q = url.searchParams.get("q")?.trim().toLowerCase();
      const users = ["owner", "member", "viewer"].map((username) => ({
        avatar: { provider: "dicebear", seed: username, style: "adventurer" },
        id: `${username}-user`,
        status: "active",
        username
}));

      return jsonResponse({
        items: q ? users.filter((user) => user.username.includes(q)) : users
});
    }

    if (url.pathname === "/api/v1/digital-employees" && method === "GET") {
      return jsonResponse(options.digitalEmployees ?? []);
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
            status: "active"
},
          member_count: 1,
          digital_employee_count: 0,
          capability_count: 0,
          pending_draft_count: 0,
          pending_item_count: 0,
          allowed_actions: []
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
  it("validates name, slug, and owner before moving to review", async () => {
    const fetcher = createFetcher();
    const screen = await renderView(
      <CreateTeamView apiBaseUrl="http://control-plane.local" fetcher={fetcher} />,
    );

    await userEvent.click(screen.getByRole("button", { name: "下一步: 确认并创建" }));

    await expect.element(screen.getByText("团队名称不能为空")).toBeVisible();
    await expect.element(screen.getByText("团队标识不能为空")).toBeVisible();
    await expect.element(screen.getByText("请至少选择一位负责人")).toBeVisible();

    await userEvent.fill(
      screen.getByRole("textbox", { name: "团队名称", exact: true }),
      "安全团队",
    );
    await userEvent.fill(
      screen.getByRole("textbox", { name: "团队标识 slug", exact: true }),
      "security",
    );
    await userEvent.click(screen.getByRole("button", { name: "下一步: 确认并创建" }));
    await expect.element(screen.getByText("请至少选择一位负责人")).toBeVisible();

    expect(
      fetchCalls(fetcher).some(
        ([url, init]) =>
          String(url).endsWith("/api/v1/teams") && init?.method === "POST",
      ),
    ).toBe(false);
  });

  it("rejects a slug that the backend tenant validation would reject", async () => {
    const fetcher = createFetcher();
    const screen = await renderView(
      <CreateTeamView apiBaseUrl="http://control-plane.local" fetcher={fetcher} />,
    );

    await userEvent.fill(
      screen.getByRole("textbox", { name: "团队名称", exact: true }),
      "2024 团队",
    );
    await userEvent.fill(
      screen.getByRole("textbox", { name: "团队标识 slug", exact: true }),
      "2024-team",
    );

    await userEvent.type(
      screen.getByRole("searchbox", { name: "搜索平台注册用户" }),
      "owner",
    );
    await userEvent.click(screen.getByRole("button", { name: "选择 owner" }));
    await userEvent.click(screen.getByRole("button", { name: "下一步: 确认并创建" }));

    await expect
      .element(
        screen.getByText(
          "团队标识需以小写字母开头，仅含小写字母、数字和中划线，且以字母或数字结尾",
        ),
      )
      .toBeVisible();

    expect(
      fetchCalls(fetcher).some(
        ([url, init]) =>
          String(url).endsWith("/api/v1/teams") && init?.method === "POST",
      ),
    ).toBe(false);
  });

  it("strips disallowed characters from slug input", async () => {
    const fetcher = createFetcher();
    const screen = await renderView(
      <CreateTeamView apiBaseUrl="http://control-plane.local" fetcher={fetcher} />,
    );

    await userEvent.fill(
      screen.getByRole("textbox", { name: "团队标识 slug", exact: true }),
      "My_Team",
    );

    await expect
      .element(screen.getByRole("textbox", { name: "团队标识 slug", exact: true }))
      .toHaveValue("myteam");
  });

  it("creates a team through the two-step wizard", async () => {
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
    await userEvent.fill(
      screen.getByRole("textbox", { name: "团队说明", exact: true }),
      "负责应用安全架构评审与风险响应。",
    );
    await userEvent.type(
      screen.getByRole("searchbox", { name: "搜索平台注册用户" }),
      "owner",
    );
    await userEvent.click(screen.getByRole("button", { name: "选择 owner" }));
    await userEvent.click(screen.getByRole("button", { name: "下一步: 确认并创建" }));

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
      description: "负责应用安全架构评审与风险响应。",
      human_owner_user_ids: ["owner-user"],
      initial_digital_employee_ids: [],
      metadata: {
        display: {
          color_tone: "teal",
          icon_key: "role-application-security"
}
},
      name: "安全团队",
      slug: "security"
});
    await expect.poll(() => onCreated.mock.calls.length).toBe(1);
    expect(onCreated.mock.calls[0][1]).toEqual({ goToConstitution: false });
  });

  it("keeps human owners and digital employees distinct on the team canvas", async () => {
    const fetcher = createFetcher({
      digitalEmployees: [
        {
          id: "employee-1",
          name: "事件分析员",
          role: "安全事件分析",
          status: "ready"
},
      ]
});
    const screen = await renderView(
      <CreateTeamView apiBaseUrl="http://control-plane.local" fetcher={fetcher} />,
    );

    await expect.element(screen.getByLabelText("团队画布")).toBeVisible();
    await expect.element(screen.getByText("数字员工库")).toBeVisible();

    await userEvent.fill(
      screen.getByRole("textbox", { name: "团队名称", exact: true }),
      "安全团队",
    );
    await userEvent.fill(
      screen.getByRole("textbox", { name: "团队标识 slug", exact: true }),
      "security",
    );
    await userEvent.fill(
      screen.getByRole("textbox", { name: "团队说明", exact: true }),
      "负责应用安全架构评审与风险响应。",
    );
    await userEvent.type(
      screen.getByRole("searchbox", { name: "搜索平台注册用户" }),
      "owner",
    );
    await userEvent.click(screen.getByRole("button", { name: "选择 owner" }));
    await userEvent.click(screen.getByRole("button", { name: "加入 事件分析员" }));

    await expect
      .element(screen.getByRole("button", { name: "移除数字员工 事件分析员" }))
      .toBeVisible();
    await expect
      .element(screen.getByRole("heading", { name: "人类负责人" }))
      .toBeVisible();

    await userEvent.click(screen.getByRole("button", { name: "下一步: 确认并创建" }));
    await userEvent.click(screen.getByRole("button", { name: "确认并创建" }));

    await expect.poll(() => createTeamPostBody(fetcher)).toEqual({
      description: "负责应用安全架构评审与风险响应。",
      human_owner_user_ids: ["owner-user"],
      initial_digital_employee_ids: ["employee-1"],
      metadata: {
        display: {
          color_tone: "teal",
          icon_key: "role-application-security"
}
},
      name: "安全团队",
      slug: "security"
});
  });

  it("paginates the digital employee library instead of extending the create page", async () => {
    const fetcher = createFetcher({
      digitalEmployees: Array.from({ length: 6 }, (_, index) => ({
        id: `employee-${index + 1}`,
        name: `数字员工 ${index + 1}`,
        role: "执行专员",
        status: "ready"
}))
});
    const screen = await renderView(
      <CreateTeamView apiBaseUrl="http://control-plane.local" fetcher={fetcher} />,
    );

    await expect
      .element(screen.getByRole("button", { name: "加入 数字员工 1" }))
      .toBeVisible();
    await expect
      .element(screen.getByRole("button", { name: "加入 数字员工 6" }))
      .not.toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: "下一页" }));

    await expect
      .element(screen.getByRole("button", { name: "加入 数字员工 6" }))
      .toBeVisible();
    await expect
      .element(screen.getByRole("button", { name: "加入 数字员工 1" }))
      .not.toBeInTheDocument();
  });

  it("keeps the wizard actions at the viewport bottom while the configuration scrolls", async () => {
    const screen = await renderView(
      <CreateTeamView apiBaseUrl="http://control-plane.local" fetcher={createFetcher()} />,
    );

    const scrollRegion = document.body.querySelector('[data-testid="create-team-scroll-region"]');
    expect(scrollRegion).toBeTruthy();
    expect(scrollRegion).toHaveClass("flex-1");
    expect(scrollRegion).toHaveClass("overflow-y-auto");

    const actions = document.body.querySelector('[data-testid="create-team-actions"]');
    expect(actions).toBeTruthy();
    expect(actions).toHaveClass("sticky");
    expect(actions).toHaveClass("bottom-0");
    await expect
      .element(screen.getByRole("button", { name: "下一步: 确认并创建" }))
      .toBeVisible();
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
    await userEvent.type(
      screen.getByRole("searchbox", { name: "搜索平台注册用户" }),
      "owner",
    );
    await userEvent.click(screen.getByRole("button", { name: "选择 owner" }));
    await userEvent.click(screen.getByRole("button", { name: "下一步: 确认并创建" }));
    await userEvent.click(screen.getByRole("button", { name: "确认并创建" }));

    await expect
      .element(screen.getByText("create team request failed with status 500"))
      .toBeVisible();
  });
});
