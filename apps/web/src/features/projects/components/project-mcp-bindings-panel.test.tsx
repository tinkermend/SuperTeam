import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { userEvent } from "vitest/browser";
import { describe, expect, it, vi } from "vitest";
import { render } from "vitest-browser-react";
import { ProjectMcpBindingsPanel } from "@/features/projects/components/project-mcp-bindings-panel";
import type { McpBinding, McpServerDefinition } from "@/lib/api/capabilities";

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

function makeDefinitions(): McpServerDefinition[] {
  return [
    {
      auth_strategy: "bearer_env",
      description: "GitHub 仓库操作",
      id: "server-github",
      name: "GitHub MCP",
      optional_env_vars: [],
      required_env_vars: ["GITHUB_TOKEN"],
      risk_level: "medium",
      server_key: "github",
      status: "active",
      tenant_id: "tenant-1",
      tool_allowlist: [],
      transport: "http",
      url: "https://mcp.github.test",
    },
    {
      auth_strategy: "none",
      description: "内部搜索",
      id: "server-search",
      name: "Search MCP",
      optional_env_vars: [],
      required_env_vars: [],
      risk_level: "low",
      server_key: "search",
      status: "active",
      tenant_id: "tenant-1",
      tool_allowlist: [],
      transport: "streamable_http",
      url: "https://mcp.search.test",
    },
  ];
}

function makeBindings(): McpBinding[] {
  return [
    {
      credential_env_var: "GITHUB_TOKEN",
      id: "binding-1",
      mcp_server_id: "server-github",
      project_id: "project-1",
      server_key: "github",
      server_name: "GitHub MCP",
      status: "active",
      tenant_id: "tenant-1",
      transport: "http",
    } as McpBinding,
  ];
}

function createFetcher(bindings: McpBinding[] = makeBindings()) {
  return vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = new URL(String(input));
    const method = init?.method ?? "GET";

    if (url.pathname === "/api/v1/mcp-servers" && method === "GET") {
      return jsonResponse(makeDefinitions());
    }
    if (
      url.pathname === "/api/v1/projects/project-1/mcp-bindings" &&
      method === "GET"
    ) {
      return jsonResponse(bindings);
    }
    if (
      url.pathname === "/api/v1/projects/project-1/mcp-bindings" &&
      method === "PUT"
    ) {
      const body = JSON.parse(String(init?.body)) as {
        items: { credential_env_var?: string; mcp_server_id: string }[];
      };
      return jsonResponse(
        body.items.map((item, index) => ({
          credential_env_var: item.credential_env_var,
          id: `binding-put-${index}`,
          mcp_server_id: item.mcp_server_id,
          project_id: "project-1",
          status: "active",
          tenant_id: "tenant-1",
        })),
      );
    }

    return jsonResponse({ error: `unhandled ${method} ${url.pathname}` }, 500);
  });
}

function fetchCalls(fetcher: typeof fetch) {
  return (
    fetcher as unknown as {
      mock: { calls: [RequestInfo | URL, RequestInit | undefined][] };
    }
  ).mock.calls;
}

function findPutCall(fetcher: typeof fetch) {
  return fetchCalls(fetcher).find(
    ([url, init]) =>
      String(url).endsWith("/api/v1/projects/project-1/mcp-bindings") &&
      init?.method === "PUT",
  );
}

async function renderPanel(fetcher: typeof fetch, disabled = false) {
  const queryClient = createQueryClient();
  return await render(
    <QueryClientProvider client={queryClient}>
      <ProjectMcpBindingsPanel
        apiOptions={{ baseUrl: "http://control-plane.test", fetcher }}
        disabled={disabled}
        projectId="project-1"
      />
    </QueryClientProvider>,
  );
}

describe("ProjectMcpBindingsPanel", () => {
  it("renders binding rows with server key, transport and credential env var", async () => {
    const fetcher = createFetcher();
    const screen = await renderPanel(fetcher);

    await expect.element(screen.getByText("GitHub MCP")).toBeInTheDocument();
    await expect.element(screen.getByText("github", { exact: true })).toBeInTheDocument();
    await expect.element(screen.getByText("http", { exact: true })).toBeInTheDocument();
    await expect.element(screen.getByText("GITHUB_TOKEN")).toBeInTheDocument();
    await expect
      .element(screen.getByText("同 server_key 时，项目绑定覆盖员工绑定。"))
      .toBeInTheDocument();
    await expect.element(screen.getByText("1 个绑定")).toBeInTheDocument();
  });

  it("adds a registry server and saves via declarative PUT with items", async () => {
    const fetcher = createFetcher();
    const screen = await renderPanel(fetcher);

    await expect.element(screen.getByText("GitHub MCP")).toBeInTheDocument();

    await userEvent.click(screen.getByRole("combobox", { name: "注册表 MCP" }));
    // 已绑定的 GitHub MCP 不应出现在注册表候选中
    await expect
      .element(screen.getByRole("option", { name: "GitHub MCP（github）" }))
      .not.toBeInTheDocument();
    await userEvent.click(screen.getByRole("option", { name: "Search MCP（search）" }));
    await userEvent.click(screen.getByRole("button", { name: "添加到绑定列表" }));

    await expect
      .element(screen.getByText("未保存", { exact: true }))
      .toBeInTheDocument();
    await expect.element(screen.getByText("Search MCP")).toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: "保存绑定" }));

    await vi.waitFor(() => {
      const putCall = findPutCall(fetcher);
      expect(putCall).toBeTruthy();
      expect(JSON.parse(String(putCall?.[1]?.body))).toEqual({
        items: [
          {
            credential_env_var: "GITHUB_TOKEN",
            mcp_server_id: "server-github",
          },
          { mcp_server_id: "server-search" },
        ],
      });
    });
  });

  it("removes a binding and saves an empty items list", async () => {
    const fetcher = createFetcher();
    const screen = await renderPanel(fetcher);

    await expect.element(screen.getByText("GitHub MCP")).toBeInTheDocument();

    await userEvent.click(
      screen.getByRole("button", { name: "移除 MCP GitHub MCP" }),
    );
    await expect.element(screen.getByText("暂无 MCP 绑定")).toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: "保存绑定" }));

    await vi.waitFor(() => {
      const putCall = findPutCall(fetcher);
      expect(putCall).toBeTruthy();
      expect(JSON.parse(String(putCall?.[1]?.body))).toEqual({ items: [] });
    });
  });

  it("renders the empty state and keeps save disabled until dirty", async () => {
    const fetcher = createFetcher([]);
    const screen = await renderPanel(fetcher);

    await expect.element(screen.getByText("暂无 MCP 绑定")).toBeInTheDocument();
    await expect.element(screen.getByText("0 个绑定")).toBeInTheDocument();
    await expect
      .element(screen.getByRole("button", { name: "保存绑定" }))
      .toBeDisabled();
  });

  it("disables editing when the panel is disabled", async () => {
    const fetcher = createFetcher();
    const screen = await renderPanel(fetcher, true);

    await expect.element(screen.getByText("GitHub MCP")).toBeInTheDocument();
    await expect
      .element(screen.getByRole("button", { name: "保存绑定" }))
      .toBeDisabled();
    await expect
      .element(screen.getByRole("button", { name: "移除 MCP GitHub MCP" }))
      .toBeDisabled();
    await expect
      .element(screen.getByRole("combobox", { name: "注册表 MCP" }))
      .toBeDisabled();
  });
});
