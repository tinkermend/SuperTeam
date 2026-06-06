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

vi.mock("@monaco-editor/react", () => ({
  default: ({ value, onChange }: { value?: string; onChange?: (value?: string) => void }) => (
    <textarea
      aria-label="技能文件编辑器"
      value={value ?? ""}
      onChange={(event) => onChange?.(event.currentTarget.value)}
    />
  ),
}));

const skillsFixture = [
  {
    id: "skill-diagnose",
    tenant_id: "tenant-1",
    slug: "diagnose",
    name: "diagnose",
    description: "系统化诊断流程",
    version: "v1.0.0",
    source: "internal_market",
    risk_level: "low",
    status: "installed",
    icon_key: "stethoscope",
    color_token: "cyan",
    tags: ["诊断", "测试", "自动化"],
    files: [
      { path: "SKILL.md", file_type: "file", content: "# diagnose\n\n## 工作流", size_bytes: 20 },
      { path: "scripts/reproduce.sh", file_type: "file", content: "#!/usr/bin/env bash\n", size_bytes: 19 },
      { path: "references/checklist.md", file_type: "file", content: "- 复现\n- 证据", size_bytes: 14 },
    ],
    team_bindings: [{ team_id: "team-1", team_name: "平台工程" }],
    agent_bindings: [
      { agent_id: "agent-1", agent_name: "需求澄清 Agent", team_name: "产品团队", status: "enabled" },
      { agent_id: "agent-2", agent_name: "后端执行 Agent", team_name: "后端团队", status: "enabled" },
    ],
  },
  {
    id: "skill-tdd",
    tenant_id: "tenant-1",
    slug: "tdd",
    name: "tdd",
    description: "测试优先流程",
    version: "v1.0.0",
    source: "internal_market",
    risk_level: "medium",
    status: "installed",
    icon_key: "flask",
    color_token: "emerald",
    tags: ["测试"],
    files: [{ path: "SKILL.md", file_type: "file", content: "# tdd", size_bytes: 5 }],
    team_bindings: [],
    agent_bindings: [],
  },
] satisfies Skill[];

const teamsFixture = [
  {
    id: "team-1",
    tenant_id: "tenant-1",
    slug: "platform",
    name: "平台工程",
    status: "active",
    member_count: 12,
    digital_employee_count: 3,
    capability_count: 8,
    governance_status: "active",
    pending_draft_count: 0,
    risk_summary: "常规",
  },
  {
    id: "team-2",
    tenant_id: "tenant-1",
    slug: "security",
    name: "安全治理",
    status: "active",
    member_count: 5,
    digital_employee_count: 2,
    capability_count: 4,
    governance_status: "active",
    pending_draft_count: 0,
    risk_summary: "高风险需审批",
  },
];

function createQueryClient() {
  return new QueryClient({
    defaultOptions: {
      mutations: { retry: false },
      queries: { retry: false },
    },
  });
}

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    headers: { "content-type": "application/json" },
    status,
  });
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
    if (url.pathname === "/api/v1/skills/skill-diagnose/files/SKILL.md" && method === "PUT") {
      expect(JSON.parse(String(init?.body))).toEqual({ content: "# diagnose\n\n已更新" });
      return jsonResponse({ path: "SKILL.md", file_type: "file", content: "# diagnose\n\n已更新", size_bytes: 18 });
    }
    if (url.pathname === "/api/v1/skills/uploads" && method === "POST") {
      const formData = init?.body as FormData;
      expect(formData.get("name")).toBe("custom-audit");
      expect(formData.get("description")).toBe("自定义审计技能");
      expect(formData.get("tags")).toBe("审计,脚本");
      expect(formData.get("team_ids")).toBe("team-1,team-2");
      expect(formData.get("file")).toBeInstanceOf(File);
      return jsonResponse({
        ...skillsFixture[0],
        id: "skill-custom-audit",
        slug: "custom-audit",
        name: "custom-audit",
        description: "自定义审计技能",
        tags: ["审计", "脚本"],
      }, 201);
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
  it("renders installed skill tree, Monaco editor, file save and agent bindings", async () => {
    const fetcher = createSkillsFetcher();
    const screen = await renderSkillsView(fetcher);

    await expect.element(screen.getByRole("heading", { name: "技能管理" })).toBeVisible();
    await expect.element(screen.getByRole("tab", { name: "已安装技能" })).toBeVisible();
    await expect.element(screen.getByRole("button", { name: "diagnose" })).toBeVisible();
    await expect.element(screen.getByText("已绑定 2 个 Agent")).toBeVisible();
    await expect.element(screen.getByRole("button", { name: "scripts" })).toBeVisible();

    await userEvent.click(screen.getByRole("button", { name: "scripts" }));
    await expect.element(screen.getByRole("button", { name: "reproduce.sh" })).toBeVisible();
    await userEvent.click(screen.getByRole("button", { name: "SKILL.md" }));
    await expect.element(screen.getByLabelText("技能文件编辑器")).toHaveValue("# diagnose\n\n## 工作流");

    await userEvent.fill(screen.getByLabelText("技能文件编辑器"), "# diagnose\n\n已更新");
    await userEvent.click(screen.getByRole("button", { name: "保存" }));

    expect(fetcher).toHaveBeenCalledWith(
      "http://control-plane.local/api/v1/skills/skill-diagnose/files/SKILL.md",
      expect.objectContaining({ method: "PUT" }),
    );
    await expect.element(screen.getByText("需求澄清 Agent")).toBeVisible();
    await expect.element(screen.getByText("后端执行 Agent")).toBeVisible();
  });

  it("renders marketplace cards without ratings and uses uploaded tags as labels", async () => {
    const screen = await renderSkillsView();

    await userEvent.click(screen.getByRole("tab", { name: "技能市场" }));

    await expect.element(screen.getByText("系统化诊断流程")).toBeVisible();
    await expect.element(screen.getByLabelText("diagnose 图标")).toBeVisible();
    await expect.element(screen.getByText("诊断", { exact: true }).last()).toBeVisible();
    await expect.element(screen.getByText("测试", { exact: true }).last()).toBeVisible();
    await expect.element(screen.getByText("自动化", { exact: true }).last()).toBeVisible();
    await expect.element(screen.getByText("诊断与调试")).not.toBeInTheDocument();
    await expect.element(screen.getByText("4.9")).not.toBeInTheDocument();
    await expect.element(screen.getByText("评分")).not.toBeInTheDocument();
  });

  it("uploads a zip from a dialog with description tags and multiple teams", async () => {
    const fetcher = createSkillsFetcher();
    const screen = await renderSkillsView(fetcher);

    await userEvent.click(screen.getByRole("button", { name: "上传技能" }));
    await expect.element(screen.getByRole("dialog", { name: "上传技能" })).toBeVisible();
    await userEvent.fill(screen.getByLabelText("技能名称"), "custom-audit");
    await userEvent.fill(screen.getByLabelText("技能描述"), "自定义审计技能");
    await userEvent.fill(screen.getByLabelText("标签"), "审计,脚本");
    await userEvent.click(screen.getByRole("checkbox", { name: "平台工程" }));
    await userEvent.click(screen.getByRole("checkbox", { name: "安全治理" }));
    await userEvent.upload(screen.getByLabelText("技能 zip 包"), new File(["zip"], "custom-audit.zip", { type: "application/zip" }));
    await userEvent.click(screen.getByRole("button", { name: "上传并安装" }));

    expect(fetcher).toHaveBeenCalledWith(
      "http://control-plane.local/api/v1/skills/uploads",
      expect.objectContaining({ method: "POST" }),
    );
  });
});
