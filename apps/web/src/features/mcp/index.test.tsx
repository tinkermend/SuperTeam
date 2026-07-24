import type { ReactNode } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { describe, expect, it, vi } from "vitest";
import { render } from "vitest-browser-react";
import { userEvent } from "vitest/browser";
import { McpManagementPage } from "@/features/mcp";
import {
  createMcpServerDefinition,
  deleteMcpServerDefinition,
  listMcpServerDefinitions,
  listMcpServerDependentSkills,
  type McpServerDefinition
} from "@/lib/api/capabilities";

vi.mock("@/components/layout/header", () => ({
  Header: ({ children }: { children: ReactNode }) => <header>{children}</header>
}));

vi.mock("@/components/layout/main", () => ({
  Main: ({ children }: { children: ReactNode }) => <main>{children}</main>
}));

vi.mock("@/components/search", () => ({
  Search: () => <button type="button">Search</button>
}));

vi.mock("@/components/theme-switch", () => ({
  ThemeSwitch: () => <button type="button">Toggle theme</button>
}));

vi.mock("@/lib/config/control-plane-url", () => ({
  resolveControlPlaneUrl: () => "http://control-plane.local"
}));

vi.mock("@/lib/api/capabilities", () => ({
  listMcpServerDefinitions: vi.fn(),
  createMcpServerDefinition: vi.fn(),
  deleteMcpServerDefinition: vi.fn(),
  listMcpServerDependentSkills: vi.fn()
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
  status: "active"
} satisfies McpServerDefinition;

async function withClient(ui: ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } }
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

  it("submits a new MCP definition through the register dialog", async () => {
    vi.mocked(listMcpServerDefinitions).mockResolvedValue([]);
    vi.mocked(createMcpServerDefinition).mockResolvedValue({
      ...githubDefinition,
      id: "mcp-new"
} satisfies McpServerDefinition);

    const user = userEvent.setup();
    const screen = await withClient(<McpManagementPage />);

    await user.click(screen.getByRole("button", { name: "注册 MCP" }));

    await expect
      .element(screen.getByRole("dialog", { name: "注册新 MCP" }))
      .toBeVisible();

    await user.fill(screen.getByLabelText(/^名称/), "GitHub MCP");
    await user.fill(screen.getByLabelText(/^server_key/), "github");
    await user.fill(screen.getByLabelText(/^URL/), "https://api.githubcopilot.com/mcp/");
    await user.fill(
      screen.getByLabelText(/^描述/),
      "GitHub 代码托管能力",
    );
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
          description: "GitHub 代码托管能力",
          transport: "streamable_http",
          url: "https://api.githubcopilot.com/mcp/",
          auth_strategy: "none",
          required_env_vars: ["GITHUB_TOKEN"]
}),
      );
    });

    await vi.waitFor(async () => {
      await expect
        .element(screen.getByRole("dialog", { name: "注册新 MCP" }))
        .not.toBeInTheDocument();
    });
  });

  it("shows the empty state when no MCP is registered", async () => {
    vi.mocked(listMcpServerDefinitions).mockResolvedValue([]);

    const screen = await withClient(<McpManagementPage />);

    await expect.element(screen.getByText("还没有注册 MCP")).toBeVisible();
  });

  it("delete asks for confirmation and blocks it while skills still depend on the definition", async () => {
    vi.mocked(listMcpServerDefinitions).mockResolvedValue([githubDefinition]);
    vi.mocked(listMcpServerDependentSkills).mockResolvedValue([
      { skill_id: "skill-1", slug: "deploy-helper", name: "部署助手" },
    ]);

    const user = userEvent.setup();
    const screen = await withClient(<McpManagementPage />);

    await user.click(screen.getByRole("button", { name: "删除 GitHub MCP" }));

    await expect.element(screen.getByText(/deploy-helper/)).toBeVisible();
    await expect
      .element(screen.getByRole("button", { name: "删除" }))
      .toBeDisabled();

    expect(deleteMcpServerDefinition).not.toHaveBeenCalled();
  });

  it("deletes the definition once confirmed when no skill depends on it", async () => {
    vi.mocked(listMcpServerDefinitions).mockResolvedValue([githubDefinition]);
    vi.mocked(listMcpServerDependentSkills).mockResolvedValue([]);
    vi.mocked(deleteMcpServerDefinition).mockResolvedValue(undefined);

    const user = userEvent.setup();
    const screen = await withClient(<McpManagementPage />);

    await user.click(screen.getByRole("button", { name: "删除 GitHub MCP" }));

    await expect.element(screen.getByText(/不可撤销/)).toBeVisible();
    await user.click(screen.getByRole("button", { name: "删除" }));

    await vi.waitFor(() => {
      expect(deleteMcpServerDefinition).toHaveBeenCalledWith(
        expect.anything(),
        githubDefinition.id,
      );
    });
  });
});
