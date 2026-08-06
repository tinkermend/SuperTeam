import { forwardRef, type AnchorHTMLAttributes, type ReactNode } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { describe, expect, it, vi } from "vitest";
import { render } from "vitest-browser-react";
import { ProjectConfigView } from "@/features/projects/components/project-config-page";
import type { ProjectConfig, ProjectConfigRevision } from "@/lib/api/projects";

vi.mock("@/components/layout/header", () => ({
  Header: ({ children }: { children: ReactNode }) => <header>{children}</header>
}));
vi.mock("@/components/layout/main", () => ({
  Main: ({ children }: { children: ReactNode }) => <main>{children}</main>
}));
vi.mock("@/components/search", () => ({ Search: () => null }));
vi.mock("@/components/theme-switch", () => ({ ThemeSwitch: () => null }));
vi.mock("@tanstack/react-router", () => {
  type MockLinkProps = AnchorHTMLAttributes<HTMLAnchorElement> & {
    children: ReactNode;
    params?: Record<string, string>;
    to: string;
  };
  const Link = forwardRef<HTMLAnchorElement, MockLinkProps>(
    ({ children, params, to, ...props }, ref) => (
      <a
        {...props}
        href={params?.projectId ? to.replace("$projectId", params.projectId) : to}
        ref={ref}
      >
        {children}
      </a>
    )
  );
  Link.displayName = "MockLink";
  return { Link };
});

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    headers: { "content-type": "application/json" },
    status
  });
}

function makeConfig(status: "running" | "archived" = "running"): ProjectConfig {
  return {
    project: {
      coordination_policy: {},
      coordination_status: "registered",
      coordination_workflow_id: "wf",
      description: "d",
      directory_name: "dir",
      goal: "g",
      human_owner_user_id: "h1",
      human_owner_user_ids: ["h1"],
      id: "project-1",
      name: "能力绑定验收项目",
      status,
      tenant_id: "t1",
      workspace_ready_status: "ready"
    },
    members: [
      {
        display_name_snapshot: "负责人",
        id: "m1",
        principal_id: "h1",
        principal_type: "human_user",
        project_id: "project-1",
        project_role: "owner",
        settings: {},
        status: "active",
        tenant_id: "t1"
      }
    ]
  } as ProjectConfig;
}

function makeFetcher() {
  return vi.fn(async (input: RequestInfo | URL) => {
    const url = String(input);
    if (url.includes("/config-revisions")) return jsonResponse([] as ProjectConfigRevision[]);
    if (url.includes("/config")) return jsonResponse(makeConfig());
    if (url.includes("/digital-employees") && !url.includes("skills")) return jsonResponse([]);
    if (url.includes("/users")) return jsonResponse([]);
    if (url.includes("/skill-bindings")) return jsonResponse([]);
    if (url.includes("/mcp-bindings")) return jsonResponse([]);
    if (url.includes("/api/v1/skills")) return jsonResponse([]);
    if (url.includes("/mcp-servers")) return jsonResponse([]);
    return jsonResponse({}, 404);
  });
}

describe("ProjectConfig capabilities tab", () => {
  it("U1: 能力绑定 tab 展示两区与说明条", async () => {
    const screen = await render(
      <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
        <ProjectConfigView
          apiBaseUrl="http://cp.local"
          fetcher={makeFetcher()}
          initialTab="capabilities"
          projectId="project-1"
        />
      </QueryClientProvider>
    );
    await expect.element(screen.getByRole("tab", { name: "能力绑定" })).toBeInTheDocument();
    await expect.element(screen.getByRole("heading", { name: "项目技能" })).toBeInTheDocument();
    await expect.element(screen.getByRole("heading", { name: "项目 MCP" })).toBeInTheDocument();
    await expect.element(screen.getByText("场地供给", { exact: true })).toBeInTheDocument();
  });

  it("U6: 归档项目禁用能力绑定操作", async () => {
    const fetcher = vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input);
      if (url.includes("/config-revisions")) return jsonResponse([]);
      if (url.includes("/config")) return jsonResponse(makeConfig("archived"));
      if (url.includes("/digital-employees")) return jsonResponse([]);
      if (url.includes("/users")) return jsonResponse([]);
      if (url.includes("/skill-bindings") || url.includes("/mcp-bindings")) return jsonResponse([]);
      if (url.includes("/api/v1/skills") || url.includes("/mcp-servers")) return jsonResponse([]);
      return jsonResponse({}, 404);
    });
    const screen = await render(
      <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
        <ProjectConfigView
          apiBaseUrl="http://cp.local"
          fetcher={fetcher}
          initialTab="capabilities"
          projectId="project-1"
        />
      </QueryClientProvider>
    );
    await expect.element(screen.getByRole("heading", { name: "项目技能" })).toBeInTheDocument();
    const saveSkill = screen.getByRole("button", { name: "保存技能绑定" });
    await expect.element(saveSkill).toBeDisabled();
    const saveMcp = screen.getByRole("button", { name: "保存 MCP 绑定" });
    await expect.element(saveMcp).toBeDisabled();
  });
});
