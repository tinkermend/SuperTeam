import type { ReactNode } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { describe, expect, it, vi } from "vitest";
import { render } from "vitest-browser-react";
import { userEvent } from "vitest/browser";
import { McpManagementPage } from "@/features/mcp";
import {
  createMcpServerDefinition,
  listMcpServerDefinitions,
  type McpServerDefinition,
} from "@/lib/api/capabilities";

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

vi.mock("@/lib/config/control-plane-url", () => ({
  resolveControlPlaneUrl: () => "http://control-plane.local",
}));

vi.mock("@/lib/api/capabilities", () => ({
  listMcpServerDefinitions: vi.fn(),
  createMcpServerDefinition: vi.fn(),
  deleteMcpServerDefinition: vi.fn(),
}));

const githubDefinition = {
  id: "mcp-github",
  tenant_id: "tenant-1",
  name: "GitHub MCP",
  server_key: "github",
  description: "",
  transport: "streamable_http",
  url: "https://api.githubcopilot.com/mcp/",
  auth_strategy: "bearer_env",
  required_env_vars: ["GITHUB_TOKEN"],
  optional_env_vars: [],
  tool_allowlist: [],
  risk_level: "medium",
  status: "active",
} satisfies McpServerDefinition;

async function withClient(ui: ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>{ui}</QueryClientProvider>,
  );
}

describe("MCP management page", () => {
  it("renders the heading and an existing registry entry with required env vars", async () => {
    vi.mocked(listMcpServerDefinitions).mockResolvedValue([githubDefinition]);

    const screen = await withClient(<McpManagementPage />);

    await expect.element(screen.getByText("MCP 管理")).toBeVisible();
    await expect.element(screen.getByText("GitHub MCP")).toBeVisible();
    await expect
      .element(screen.getByText("https://api.githubcopilot.com/mcp/"))
      .toBeVisible();
    await expect.element(screen.getByText("GITHUB_TOKEN")).toBeVisible();
  });

  it("submits a new MCP definition with server_key, transport, url and required env vars", async () => {
    vi.mocked(listMcpServerDefinitions).mockResolvedValue([]);
    vi.mocked(createMcpServerDefinition).mockResolvedValue({
      ...githubDefinition,
      id: "mcp-new",
    } satisfies McpServerDefinition);

    const user = userEvent.setup();
    const screen = await withClient(<McpManagementPage />);

    await user.click(screen.getByRole("button", { name: "注册 MCP" }));

    await user.fill(screen.getByLabelText("名称"), "GitHub MCP");
    await user.fill(
      screen.getByLabelText("server_key（[A-Za-z0-9_-]）"),
      "github",
    );
    await user.fill(screen.getByLabelText("URL"), "https://api.githubcopilot.com/mcp/");
    await user.fill(
      screen.getByLabelText("必需环境变量输入"),
      "GITHUB_TOKEN",
    );
    await user.keyboard("{Enter}");
    await user.click(screen.getByRole("button", { name: "创建" }));

    await vi.waitFor(() => {
      expect(createMcpServerDefinition).toHaveBeenCalledWith(
        expect.anything(),
        expect.objectContaining({
          name: "GitHub MCP",
          server_key: "github",
          transport: "streamable_http",
          url: "https://api.githubcopilot.com/mcp/",
          auth_strategy: "none",
          required_env_vars: ["GITHUB_TOKEN"],
        }),
      );
    });
  });

  it("shows the empty state when no MCP is registered", async () => {
    vi.mocked(listMcpServerDefinitions).mockResolvedValue([]);

    const screen = await withClient(<McpManagementPage />);

    await expect.element(screen.getByText("还没有注册 MCP")).toBeVisible();
  });
});
