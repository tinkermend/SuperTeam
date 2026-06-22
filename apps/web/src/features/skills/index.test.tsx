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

vi.mock("@tanstack/react-router", () => ({
  Link: ({ children, to }: { children: ReactNode; to: string }) => (
    <a data-router-link="true" href={to}>{children}</a>
  ),
}));

const skillsFixture = [
  {
    id: "skill-requirement",
    tenant_id: "tenant-1",
    slug: "requirement-clarifier",
    name: "需求澄清助手",
    description: "基于结构化提问流程，输出需求澄清记录",
    version: "1.3.0",
    source: "upload",
    risk_level: "low",
    icon_key: "stethoscope",
    color_token: "cyan",
    tags: ["需求分析", "沟通协作", "交付"],
    archive_object_ref: "s3://bucket/skills/requirement-clarifier.zip",
    archive_filename: "requirement-clarifier.zip",
    archive_size_bytes: 4096,
    archive_checksum_sha256: "abc123def456abc123def456abc123def456abc123def456abc123def456abcd",
    archive_file_count: 3,
    created_by: "user-1",
    created_by_name: "开发管理员",
    team_bindings: [
      { team_id: "team-1", team_name: "平台工程" },
      { team_id: "team-2", team_name: "产品团队" },
    ],
    agent_bindings: [
      { agent_id: "agent-1", agent_name: "需求澄清 Agent", team_name: "产品团队", status: "enabled" },
      { agent_id: "agent-2", agent_name: "项目协调 Agent", team_name: "平台工程", status: "enabled" },
    ],
    runtime_dependencies: { tools: [], env: [] },
  },
  {
    id: "skill-api-doc",
    tenant_id: "tenant-1",
    slug: "api-doc-generator",
    name: "接口文档生成",
    description: "根据 OpenAPI 规范生成接口文档与示例",
    version: "1.2.1",
    source: "upload",
    risk_level: "medium",
    icon_key: "flask",
    color_token: "emerald",
    tags: ["文档生成", "API", "规范"],
    archive_object_ref: "s3://bucket/skills/api-doc.zip",
    archive_filename: "api-doc.zip",
    archive_size_bytes: 2048,
    archive_checksum_sha256: "def456abc123def456abc123def456abc123def456abc123def456abc123def4567",
    archive_file_count: 1,
    created_by: "user-1",
    created_by_name: "开发管理员",
    team_bindings: [],
    agent_bindings: [],
    runtime_dependencies: { tools: [], env: [] },
  },
  {
    id: "skill-incident-review",
    tenant_id: "tenant-1",
    slug: "incident-review",
    name: "生产事故复盘",
    description: "结构化复盘生产事故，输出根因与改进建议",
    version: "1.0.2",
    source: "upload",
    risk_level: "high",
    icon_key: "shield-check",
    color_token: "violet",
    tags: ["运维", "复盘", "高风险"],
    archive_object_ref: "s3://bucket/skills/incident-review.zip",
    archive_filename: "incident-review.zip",
    archive_size_bytes: 8192,
    archive_checksum_sha256: "fed456abc123def456abc123def456abc123def456abc123def456abc123def456",
    archive_file_count: 2,
    created_by: "user-1",
    created_by_name: "开发管理员",
    team_bindings: [
      { team_id: "team-1", team_name: "平台工程" },
      { team_id: "team-2", team_name: "安全治理" },
    ],
    agent_bindings: [
      { agent_id: "agent-3", agent_name: "运维复盘 Agent", team_name: "平台工程", status: "enabled" },
    ],
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
      const q = url.searchParams.get("q")?.trim() ?? "";
      const rows = q
        ? skillsFixture.filter((skill) => `${skill.name} ${skill.description} ${skill.tags.join(" ")}`.includes(q))
        : skillsFixture;
      return jsonResponse(rows);
    }
    if (url.pathname === "/api/v1/teams" && method === "GET") {
      return jsonResponse(teamsFixture);
    }
  if (url.pathname === "/api/v1/digital-employees" && method === "GET") {
      return jsonResponse([]);
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
  it("renders the skill market homepage with metric strip and action buttons", async () => {
    const screen = await renderSkillsView();

    await expect.element(screen.getByRole("heading", { name: "技能市场" })).toBeVisible();
    await expect.element(screen.getByText("发现、校验并安装技能到团队或数字员工")).toBeVisible();
    await expect.element(screen.getByText("可安装")).toBeVisible();
    await expect.element(screen.getByText("需审批")).toBeVisible();
    await expect.element(screen.getByText("团队绑定")).toBeVisible();
    await expect.element(screen.getByText("数字员工绑定")).toBeVisible();
    await expect.element(screen.getByRole("columnheader", { name: "技能" })).toBeVisible();
    await expect.element(screen.getByRole("columnheader", { name: "风险" })).toBeVisible();
    await expect.element(screen.getByRole("button", { name: "查看详情 需求澄清助手" })).toBeVisible();
    await expect.element(screen.getByRole("button", { name: "安装 接口文档生成" })).toBeVisible();
  });

  it("uses router navigation for the upload page without designing that flow here", async () => {
    const screen = await renderSkillsView();

    const uploadLink = screen.getByRole("link", { name: "上传技能" });
    await expect.element(uploadLink).toHaveAttribute("href", "/skills/upload");
    await expect.element(uploadLink).toHaveAttribute("data-router-link", "true");
    expect(document.body.querySelector('[role="dialog"]')).toBeNull();
  });

  it("filters the market by search text while keeping the table chrome visible", async () => {
    const fetcher = createSkillsFetcher();
    const screen = await renderSkillsView(fetcher);

    await userEvent.fill(screen.getByRole("searchbox", { name: "搜索技能" }), "文档");

    expect(fetcher).toHaveBeenCalledWith(
      "http://control-plane.local/api/v1/skills?q=%E6%96%87%E6%A1%A3",
      expect.objectContaining({ method: "GET" }),
    );
    await expect.element(screen.getByRole("columnheader", { name: "技能" })).toBeVisible();
    await expect.element(screen.getByText("接口文档生成")).toBeVisible();
  });
});
