import type { ReactNode } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { describe, expect, it, vi } from "vitest";
import { render } from "vitest-browser-react";
import { SkillDetailView } from "@/features/skills/detail";
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
  Link: ({ children, to, ...props }: { children: ReactNode; to: string }) => (
    <a {...props} data-router-link="true" href={to}>{children}</a>
  ),
}));

const skillFixture = {
  id: "skill-requirement",
  tenant_id: "tenant-1",
  slug: "requirement-clarifier",
  name: "需求澄清助手",
  description: "基于结构化提问流程，输出需求澄清记录",
  version: "1.3.0",
  source: "upload",
  risk_level: "medium",
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
  created_at: "2026-06-20T10:12:00Z",
  updated_at: "2026-06-22T16:30:00Z",
  team_bindings: [
    { team_id: "team-1", team_name: "平台工程" },
    { team_id: "team-2", team_name: "产品团队" },
    { team_id: "team-without-name", team_name: "" },
  ],
  agent_bindings: [
    { agent_id: "agent-1", agent_name: "需求澄清 Agent", team_name: "产品团队", status: "enabled" },
    { agent_id: "agent-2", agent_name: "项目协调 Agent", team_name: "平台工程", status: "enabled" },
    { agent_id: "agent-without-name", agent_name: "", team_id: "team-3", team_name: "", status: "enabled" },
  ],
  runtime_dependencies: { tools: ["gh", "kubectl"], env: ["GH_TOKEN"] },
} satisfies Skill;

function createQueryClient() {
  return new QueryClient({
    defaultOptions: { mutations: { retry: false }, queries: { retry: false } },
  });
}

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), { headers: { "content-type": "application/json" }, status });
}

function createSkillFetcher(skill: Skill = skillFixture) {
  return vi.fn(async (input: RequestInfo | URL) => {
    const url = new URL(String(input));
    if (url.pathname === `/api/v1/skills/${skill.id}`) {
      return jsonResponse(skill);
    }
    return jsonResponse({ error: `unhandled ${url.pathname}` }, 500);
  });
}

async function renderSkillDetail(fetcher = createSkillFetcher()) {
  return render(
    <QueryClientProvider client={createQueryClient()}>
      <SkillDetailView
        apiBaseUrl="http://control-plane.local"
        fetcher={fetcher}
        skillId={skillFixture.id}
      />
    </QueryClientProvider>,
  );
}

describe("SkillDetailView", () => {
  it("renders a skill detail as a marketplace decision page without unsupported actions", async () => {
    const screen = await renderSkillDetail();

    await expect.element(screen.getByRole("heading", { name: "需求澄清助手" })).toBeVisible();
    await expect.element(screen.getByText("技能档案")).toBeVisible();
    await expect.element(screen.getByText("基于结构化提问流程，输出需求澄清记录")).toBeVisible();
    await expect.element(screen.getByText(/^创建者声明$/)).toBeVisible();
    expect(document.body.textContent).toContain("中风险");
    await expect.element(screen.getByText("未记录单独风险说明")).toBeVisible();
    await expect.element(screen.getByText("有运行依赖")).toBeVisible();
    const heroLayout = document.querySelector('[data-testid="skill-detail-hero-layout"]');
    const identityPanel = document.querySelector('[data-testid="skill-detail-identity"]');
    const declarationPanel = document.querySelector('[data-testid="skill-detail-declaration"]');
    expect(heroLayout?.textContent).toContain("创建者声明");
    expect(identityPanel?.textContent).not.toContain("创建者声明");
    expect(declarationPanel?.textContent).toContain("创建者声明");
    expect(declarationPanel?.textContent).toContain("风险说明");
    const tagStrip = document.querySelector('[data-testid="skill-detail-tags"]');
    expect(tagStrip?.textContent).toContain("需求分析");
    expect(tagStrip?.textContent).toContain("沟通协作");
    expect(tagStrip?.textContent).toContain("交付");
    expect(Array.from(document.querySelectorAll("h3")).map((heading) => heading.textContent?.trim())).not.toContain(
      "标签",
    );

    const installActions = Array.from(document.querySelectorAll("button")).filter((button) =>
      button.textContent?.includes("安装到"),
    );
    expect(installActions).toHaveLength(2);
    expect(installActions.every((button) => button.disabled)).toBe(true);
    expect(document.body.textContent).toContain("即将支持");
    expect(document.body.textContent).not.toContain("验证依赖");
    expect(document.body.textContent).not.toContain("查看包内容");

    await expect.element(screen.getByRole("heading", { name: "安装范围" })).toBeVisible();
    expect(document.body.textContent).toContain("团队安装");
    expect(document.body.textContent).toContain("平台工程");
    expect(document.body.textContent).toContain("产品团队");
    expect(document.body.textContent).toContain("team-without-name");

    expect(document.body.textContent).toContain("数字员工安装");
    await expect.element(screen.getByText("需求澄清 Agent")).toBeVisible();
    await expect.element(screen.getByText("项目协调 Agent")).toBeVisible();
    expect(document.body.textContent).toContain("agent-without-name");
    await expect.element(screen.getByText("产品团队 / enabled")).toBeVisible();
    await expect.element(screen.getByText("team-3 / enabled")).toBeVisible();

    await expect.element(screen.getByRole("heading", { name: "上传与存储信息" })).toBeVisible();
    expect(document.querySelector('[data-testid="skill-upload-metadata"]')?.textContent).toContain(
      "requirement-clarifier",
    );
    expect(document.body.textContent).toContain("requirement-clarifier");
    expect(document.body.textContent).toContain("requirement-clarifier.zip");
    expect(document.body.textContent).toContain("4 KB");
    expect(document.body.textContent).toContain("3 个文件");
    expect(document.body.textContent).toContain("abc123def456abc123def456abc123def456abc123def456abc123def456abcd");
    expect(document.body.textContent).toContain("开发管理员");

    await expect.element(screen.getByRole("heading", { name: "运行要求" })).toBeVisible();
    expect(document.body.textContent).toContain("gh");
    expect(document.body.textContent).toContain("kubectl");
    expect(document.body.textContent).toContain("GH_TOKEN");

    const backLink = screen.getByRole("link", { name: "返回技能市场" });
    await expect.element(backLink).toHaveAttribute("href", "/skills");
    await expect.element(backLink).toHaveAttribute("data-router-link", "true");
  });
});
