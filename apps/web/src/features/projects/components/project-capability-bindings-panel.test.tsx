import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { userEvent } from "vitest/browser";
import { describe, expect, it, vi } from "vitest";
import { render } from "vitest-browser-react";
import { ProjectCapabilityBindingsPanel } from "./project-capability-bindings-panel";

function createQueryClient() {
  return new QueryClient({ defaultOptions: { queries: { retry: false } } });
}

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    headers: { "content-type": "application/json" },
    status,
  });
}

const skillLinux = {
  id: "skill-linux",
  tenant_id: "t1",
  slug: "linux",
  name: "Linux 巡检",
  description: "d",
  version: "v1",
  source: "upload",
  risk_level: "medium",
  icon_key: "blocks",
  color_token: "teal",
  tags: [],
  archive_object_ref: "s3://x",
  archive_filename: "x.zip",
  archive_size_bytes: 1,
  archive_checksum_sha256: "abc",
  archive_file_count: 1,
  created_by: "u1",
  created_by_name: "admin",
  team_bindings: [],
  agent_bindings: [],
  project_bindings: [],
  runtime_dependencies: {
    tools: [],
    env: [],
    mcp_servers: [
      { mcp_server_id: "mcp-1", server_key: "github-mcp", server_name: "GitHub MCP" },
    ],
  },
};

describe("ProjectCapabilityBindingsPanel", () => {
  it("草稿态：添加技能后出现未保存，刷新前不写 PUT", async () => {
    const calls: Array<{ method: string; url: string; body?: string }> = [];
    const fetcher = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      const method = (init?.method ?? "GET").toUpperCase();
      const body = typeof init?.body === "string" ? init.body : undefined;
      calls.push({ method, url, body });
      if (url.endsWith("/skill-bindings") && method === "GET") {
        return jsonResponse([]);
      }
      if (url.endsWith("/mcp-bindings") && method === "GET") {
        return jsonResponse([]);
      }
      if (url.includes("/api/v1/skills") && method === "GET") {
        return jsonResponse([skillLinux]);
      }
      if (url.endsWith("/mcp-servers") && method === "GET") {
        return jsonResponse([]);
      }
      if (url.endsWith("/skill-bindings") && method === "PUT") {
        return jsonResponse([
          {
            id: "b1",
            tenant_id: "t1",
            project_id: "p1",
            skill_id: skillLinux.id,
            skill: skillLinux,
          },
        ]);
      }
      return jsonResponse({ message: "unexpected " + method + " " + url }, 500);
    });

    const screen = await render(
      <QueryClientProvider client={createQueryClient()}>
        <ProjectCapabilityBindingsPanel
          apiOptions={{ baseUrl: "http://cp.local", fetcher }}
          projectId="p1"
        />
      </QueryClientProvider>,
    );

    await expect.element(screen.getByRole("heading", { name: "项目技能" })).toBeInTheDocument();
    await expect.element(screen.getByText("暂无项目技能绑定", { exact: true })).toBeInTheDocument();

    // open select via combobox role if available
    const triggers = screen.getByRole("combobox", { name: "从技能市场添加" });
    await userEvent.click(triggers);
    await userEvent.click(screen.getByText(/Linux 巡检/));
    await userEvent.click(screen.getByRole("button", { name: "添加到列表" }));

    await expect.element(screen.getByText("未保存", { exact: true })).toBeInTheDocument();
    await expect
      .element(screen.getByText(/该技能依赖 github-mcp/))
      .toBeInTheDocument();

    // no PUT yet
    expect(calls.some((c) => c.method === "PUT")).toBe(false);

    await userEvent.click(screen.getByRole("button", { name: "保存技能绑定" }));
    await vi.waitFor(() => {
      expect(calls.some((c) => c.method === "PUT" && c.url.includes("skill-bindings"))).toBe(true);
    });
    const put = calls.find((c) => c.method === "PUT" && c.url.includes("skill-bindings"));
    expect(JSON.parse(put!.body!)).toEqual({ items: [{ skill_id: "skill-linux" }] });
  });

  // 复检揪出的缺陷回归：**已保存**的绑定行也必须显示依赖闭包预览。
  // 首版只覆盖「刚从市场添加」的草稿行（deps 来自技能市场响应），而已保存行的
  // deps 来自 /skill-bindings 内嵌的 skill——后端当时没补全子项，返回 mcp_servers: []，
  // 于是本页最有价值的提示对已存绑定永远不显示，测试却是绿的。
  it("已保存的绑定行也显示依赖闭包预览", async () => {
    const fetcher = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      const method = (init?.method ?? "GET").toUpperCase();
      if (url.endsWith("/skill-bindings") && method === "GET") {
        return jsonResponse([
          {
            id: "b1",
            tenant_id: "t1",
            project_id: "p1",
            skill_id: "skill-linux",
            created_at: "2026-08-06T00:00:00Z",
            skill: skillLinux,
          },
        ]);
      }
      if (url.endsWith("/mcp-bindings") && method === "GET") return jsonResponse([]);
      if (url.includes("/api/v1/skills")) return jsonResponse([]);
      if (url.endsWith("/mcp-servers")) return jsonResponse([]);
      return jsonResponse({}, 404);
    });

    const screen = await render(
      <QueryClientProvider client={createQueryClient()}>
        <ProjectCapabilityBindingsPanel
          apiOptions={{ baseUrl: "http://cp.local", fetcher }}
          projectId="p1"
        />
      </QueryClientProvider>,
    );

    // 零交互，直接从已保存绑定渲染出来
    await expect.element(screen.getByText(/该技能依赖 github-mcp/)).toBeInTheDocument();
    await expect
      .element(screen.getByText(/若执行者已配齐所需环境变量/))
      .toBeInTheDocument();
  });

  it("MCP PUT 缺服务端错误时保留草稿 message", async () => {
    const fetcher = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      const method = (init?.method ?? "GET").toUpperCase();
      if (url.endsWith("/skill-bindings") && method === "GET") return jsonResponse([]);
      if (url.endsWith("/mcp-bindings") && method === "GET") return jsonResponse([]);
      if (url.includes("/api/v1/skills")) return jsonResponse([]);
      if (url.endsWith("/mcp-servers")) {
        return jsonResponse([
          {
            id: "mcp-1",
            tenant_id: "t1",
            name: "GitHub",
            server_key: "github-mcp",
            description: "",
            transport: "http",
            url: "https://example.com",
            auth_strategy: "none",
            required_env_vars: [],
            optional_env_vars: [],
            tool_allowlist: [],
            risk_level: "low",
            status: "active",
            project_bindings: [],
          },
        ]);
      }
      if (url.endsWith("/mcp-bindings") && method === "PUT") {
        return new Response(
          JSON.stringify({ message: "invalid: missing MCP dependency s2-orphan-mcp" }),
          { status: 400, headers: { "content-type": "application/json" } },
        );
      }
      return jsonResponse({}, 404);
    });

    const screen = await render(
      <QueryClientProvider client={createQueryClient()}>
        <ProjectCapabilityBindingsPanel
          apiOptions={{ baseUrl: "http://cp.local", fetcher }}
          projectId="p1"
        />
      </QueryClientProvider>,
    );

    await expect.element(screen.getByRole("heading", { name: "项目 MCP" })).toBeInTheDocument();
    await userEvent.click(screen.getByRole("combobox", { name: "注册表 MCP" }));
    await userEvent.click(screen.getByText(/GitHub/));
    await userEvent.click(screen.getByRole("button", { name: "添加到绑定列表" }));
    await expect.element(screen.getByText("未保存", { exact: true })).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "保存 MCP 绑定" }));
    await expect.element(screen.getByText(/s2-orphan-mcp/)).toBeInTheDocument();
    await expect.element(screen.getByText("未保存", { exact: true })).toBeInTheDocument();
  });
});
