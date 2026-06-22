import type { ReactNode } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { describe, expect, it, vi } from "vitest";
import { render } from "vitest-browser-react";
import { userEvent } from "vitest/browser";
import { SkillsView } from "@/features/skills";
import type { Skill } from "@/lib/api/skills";

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

const skillsFixture = [
  {
    id: "skill-diagnose",
    tenant_id: "tenant-1",
    slug: "diagnose",
    name: "diagnose",
    description: "系统化诊断流程",
    version: "v1.0.0",
    source: "upload",
    risk_level: "low",
    icon_key: "stethoscope",
    color_token: "cyan",
    tags: ["诊断", "测试", "自动化"],
    archive_object_ref: "s3://bucket/skills/diagnose.zip",
    archive_filename: "diagnose.zip",
    archive_size_bytes: 4096,
    archive_checksum_sha256: "abc123def456abc123def456abc123def456abc123def456abc123def456abcd",
    archive_file_count: 3,
    created_by: "user-1",
    created_by_name: "开发管理员",
    team_bindings: [{ team_id: "team-1", team_name: "平台工程" }],
    agent_bindings: [
      { agent_id: "agent-1", agent_name: "需求澄清 Agent", team_name: "产品团队", status: "enabled" },
    ],
    runtime_dependencies: { tools: [], env: [] },
  },
  {
    id: "skill-tdd",
    tenant_id: "tenant-1",
    slug: "tdd",
    name: "tdd",
    description: "测试优先流程",
    version: "v1.0.0",
    source: "upload",
    risk_level: "medium",
    icon_key: "flask",
    color_token: "emerald",
    tags: ["测试"],
    archive_object_ref: "s3://bucket/skills/tdd.zip",
    archive_filename: "tdd.zip",
    archive_size_bytes: 2048,
    archive_checksum_sha256: "def456abc123def456abc123def456abc123def456abc123def456abc123def4567",
    archive_file_count: 1,
    created_by: "user-1",
    created_by_name: "开发管理员",
    team_bindings: [],
    agent_bindings: [],
    runtime_dependencies: { tools: [], env: [] },
  },
] satisfies Skill[];

const teamsFixture = [
  { id: "team-1", tenant_id: "tenant-1", slug: "platform", name: "平台工程", status: "active", member_count: 12, digital_employee_count: 3, capability_count: 8, governance_status: "active", pending_draft_count: 0, risk_summary: "常规" },
  { id: "team-2", tenant_id: "tenant-1", slug: "security", name: "安全治理", status: "active", member_count: 5, digital_employee_count: 2, capability_count: 4, governance_status: "active", pending_draft_count: 0, risk_summary: "高风险需审批" },
];

function createQueryClient() {
  return new QueryClient({
    defaultOptions: { mutations: { retry: false }, queries: { retry: false } },
  });
}

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), { headers: { "content-type": "application/json" }, status });
}

function createSkillsFetcher() {
  return vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = new URL(String(input));
    const method = init?.method ?? "GET";
    if (url.pathname === "/api/v1/skills" && method === "GET") {
      return jsonResponse(skillsFixture);
    }
    if (url.pathname === "/api/v1/teams" && method === "GET") {
      return jsonResponse(teamsFixture);
    }
  if (url.pathname === "/api/v1/digital-employees" && method === "GET") {
      return jsonResponse([]);
    }
    if (url.pathname.startsWith("/api/v1/skills/skill-diagnose") && method === "DELETE") {
      return new Response(null, { status: 204 });
    }
    return jsonResponse({ error: `unhandled ${method} ${url.pathname}` }, 500);
  });
}

async function renderSkillsView(fetcher = createSkillsFetcher()) {
  return render(
    <QueryClientProvider client={createQueryClient()}>
      <SkillsView apiBaseUrl="http://control-plane.local" fetcher={fetcher} />
    </QueryClientProvider>,
  );
}

describe("SkillsView", () => {
  it("renders skill list with uploader info and archive metadata", async () => {
    const screen = await renderSkillsView();

    await expect.element(screen.getByRole("heading", { name: "技能管理" })).toBeVisible();
    await expect.element(screen.getByText("开发管理员")).toBeVisible();
    await expect.element(screen.getByText("1 个团队")).toBeVisible();
  });

  it("links to the upload workspace instead of opening an upload dialog", async () => {
    const screen = await renderSkillsView();

    await expect.element(screen.getByRole("link", { name: "上传技能" })).toHaveAttribute("href", "/skills/upload");
    expect(document.body.querySelector('[role="dialog"]')).toBeNull();
  });

  it("deletes a skill after confirmation", async () => {
    const fetcher = createSkillsFetcher();
    const screen = await renderSkillsView(fetcher);

    await userEvent.click(screen.getByRole("button", { name: "删除技能 diagnose" }));
    await expect.element(screen.getByRole("dialog", { name: "删除技能" })).toBeVisible();
    await userEvent.click(screen.getByRole("button", { name: "确认删除" }));

    expect(fetcher).toHaveBeenCalledWith(
      "http://control-plane.local/api/v1/skills/skill-diagnose",
      expect.objectContaining({ method: "DELETE" }),
    );
  });
});
