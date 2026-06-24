import type { ReactNode } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { describe, expect, it, vi } from "vitest";
import { render } from "vitest-browser-react";
import { userEvent } from "vitest/browser";
import { SkillUploadView } from "@/features/skills/upload";
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
  useNavigate: () => vi.fn(),
}));

const uploadedSkill = {
  id: "skill-uploaded",
  tenant_id: "tenant-1",
  slug: "api-doc",
  name: "接口文档生成",
  description: "根据 OpenAPI 生成接口文档",
  version: "v0.1.0",
  source: "upload",
  risk_level: "medium",
  icon_key: "blocks",
  color_token: "teal",
  tags: ["API"],
  archive_object_ref: "s3://bucket/skills/api-doc.zip",
  archive_filename: "skill-api-doc.zip",
  archive_size_bytes: 2048,
  archive_checksum_sha256: "def456abc789",
  archive_file_count: 1,
  created_by: "user-1",
  created_by_name: "开发管理员",
  team_bindings: [],
  agent_bindings: [],
  runtime_dependencies: { tools: ["gh"], env: ["GH_TOKEN", "OPENAI_API_KEY"] },
} satisfies Skill;

function createQueryClient() {
  return new QueryClient({
    defaultOptions: { mutations: { retry: false }, queries: { retry: false } },
  });
}

function renderUploadView({
  fetcher = vi.fn(async () => jsonResponse(uploadedSkill, 201)),
  onUploaded = vi.fn(),
}: {
  fetcher?: typeof fetch;
  onUploaded?: (skill: Skill) => void;
} = {}) {
  return render(
    <QueryClientProvider client={createQueryClient()}>
      <SkillUploadView
        apiBaseUrl="http://control-plane.local"
        fetcher={fetcher}
        onUploaded={onUploaded}
      />
    </QueryClientProvider>,
  );
}

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), { headers: { "content-type": "application/json" }, status });
}

function stepTitleColor(label: string): string {
  const step = document.querySelector(`[data-release-step="${label}"]`);
  const title = step?.querySelector<HTMLElement>("[data-release-step-title]");
  return title ? getComputedStyle(title).color : "";
}

function stepDescriptionColor(label: string): string {
  const step = document.querySelector(`[data-release-step="${label}"]`);
  const description = step?.querySelector<HTMLElement>("[data-release-step-description]");
  return description ? getComputedStyle(description).color : "";
}

describe("SkillUploadView", () => {
  it("renders a single page upload workspace without wizard, permission, or env checks", async () => {
    const screen = await renderUploadView();

    await expect.element(screen.getByRole("heading", { name: "上传技能" })).toBeVisible();
    await expect.element(screen.getByText("仅声明变量名，运行时由数字员工配置注入值。")).toBeVisible();

    const content = document.body.textContent ?? "";
    expect(content).not.toContain("上传包->元数据");
    expect(content).not.toContain("所需权限");
    expect(content).not.toContain("解析结果");
    expect(content).not.toContain("发布前检查");
    expect(content).not.toContain("缺失");
  });

  it("uses router navigation for links back to the skill market", async () => {
    const screen = await renderUploadView();

    for (const link of [
      screen.getByRole("link", { exact: true, name: "技能市场" }),
      screen.getByRole("link", { exact: true, name: "返回技能市场" }),
    ]) {
      await expect.element(link).toHaveAttribute("href", "/skills");
      await expect.element(link).toHaveAttribute("data-router-link", "true");
    }
  });

  it("derives the package display name from the zip filename and requires a Chinese skill name", async () => {
    const screen = await renderUploadView();
    const publishButton = screen.getByRole("button", { name: "发布到技能市场" });

    await userEvent.upload(screen.getByLabelText("技能 zip 包"), new File(["zip"], "skill-api-doc.zip", { type: "application/zip" }));

    expect(document.body.textContent).toContain("skill-api-doc");
    await expect.element(publishButton).toBeDisabled();

    await userEvent.fill(screen.getByLabelText("技能中文名称"), "接口文档生成");

    await expect.element(publishButton).not.toBeDisabled();
  });

  it("shows a compact package status band and ready publish summary", async () => {
    const screen = await renderUploadView();

    await userEvent.upload(screen.getByLabelText("技能 zip 包"), new File(["zip"], "skill-api-doc.zip", { type: "application/zip" }));
    await userEvent.fill(screen.getByLabelText("技能中文名称"), "接口文档生成");
    await userEvent.fill(screen.getByLabelText("技能描述"), "根据 OpenAPI/Swagger 规范自动生成接口文档与请求示例，支持多语言 SDK 示例输出。");
    await userEvent.fill(screen.getByLabelText("标签"), "文档生成,API,OpenAPI");
    await userEvent.fill(screen.getByLabelText("CLI 依赖"), "gh,node");
    await userEvent.fill(screen.getByLabelText("环境变量"), "GH_TOKEN,OPENAI_API_KEY");

    await expect.element(screen.getByText("ZIP 已选择")).toBeVisible();
    await expect.element(screen.getByText("包含 SKILL.md")).toBeVisible();
    await expect.element(screen.getByText("服务端发布校验")).toBeVisible();
    await expect.element(screen.getByText("发布链路")).toBeVisible();
    await expect.element(screen.getByText("上传文件")).toBeVisible();
    await expect.element(screen.getByText("解析信息")).toBeVisible();
    await expect.element(screen.getByText("完善资料")).toBeVisible();
    await expect.element(screen.getByText("校验依赖")).toBeVisible();
    await expect.element(screen.getByText("发布确认")).toBeVisible();
    await expect.element(screen.getByText("可发布")).toBeVisible();
    await expect.element(screen.getByText("元数据与依赖声明已就绪")).toBeVisible();
    await expect.element(screen.getByText("skill-api-doc.zip（3 B）")).toBeVisible();
    await expect.element(screen.getByText("4 项")).toBeVisible();
  });

  it("keeps dependency validation greyed out until dependencies are declared", async () => {
    const screen = await renderUploadView();

    await userEvent.upload(screen.getByLabelText("技能 zip 包"), new File(["zip"], "skill-api-doc.zip", { type: "application/zip" }));
    await userEvent.fill(screen.getByLabelText("技能中文名称"), "接口文档生成");

    const dependencyStep = document.querySelector('[data-release-step="校验依赖"]');
    const publishStep = document.querySelector('[data-release-step="发布确认"]');

    expect(dependencyStep).toHaveAttribute("aria-disabled", "true");
    expect(dependencyStep?.className).not.toContain("opacity-75");
    expect(stepTitleColor("校验依赖")).toBe(stepTitleColor("发布确认"));
    expect(stepDescriptionColor("校验依赖")).toBe(stepDescriptionColor("发布确认"));
    expect(publishStep).toHaveAttribute("aria-disabled", "false");

    await userEvent.fill(screen.getByLabelText("CLI 依赖"), "gh");

    expect(document.querySelector('[data-release-step="校验依赖"]')).toHaveAttribute("aria-disabled", "false");
  });

  it("submits the Chinese name and declaration-only dependencies without environment value checks", async () => {
    const captured: Record<string, FormDataEntryValue | null> = {};
    const onUploaded = vi.fn();
    const fetcher = vi.fn(async (_input: RequestInfo | URL, init?: RequestInit) => {
      expect(init?.body).toBeInstanceOf(FormData);
      const formData = init?.body as FormData;
      captured.name = formData.get("name");
      captured.description = formData.get("description");
      captured.runtime_tools = formData.get("runtime_tools");
      captured.runtime_env = formData.get("runtime_env");
      captured.package_name = formData.get("package_name");
      return jsonResponse(uploadedSkill, 201);
    });
    const screen = await renderUploadView({ fetcher, onUploaded });

    await userEvent.upload(screen.getByLabelText("技能 zip 包"), new File(["zip"], "skill-api-doc.zip", { type: "application/zip" }));
    await userEvent.fill(screen.getByLabelText("技能中文名称"), "接口文档生成");
    await userEvent.fill(screen.getByLabelText("技能描述"), "根据 OpenAPI 生成接口文档");
    await userEvent.fill(screen.getByLabelText("CLI 依赖"), "gh,node");
    await userEvent.fill(screen.getByLabelText("环境变量"), "GH_TOKEN,OPENAI_API_KEY");
    await userEvent.click(screen.getByRole("button", { name: "发布到技能市场" }));

    expect(fetcher).toHaveBeenCalledWith(
      "http://control-plane.local/api/v1/skills/uploads",
      expect.objectContaining({ method: "POST" }),
    );
    expect(captured.name).toBe("接口文档生成");
    expect(captured.description).toBe("根据 OpenAPI 生成接口文档");
    expect(captured.runtime_tools).toBe("gh,node");
    expect(captured.runtime_env).toBe("GH_TOKEN,OPENAI_API_KEY");
    expect(captured.package_name).toBeNull();
    expect(document.body.textContent).not.toContain("GH_TOKEN 需目标配置");
    expect(onUploaded).toHaveBeenCalledWith(uploadedSkill);
  });

  it("keeps dependency tokens intact when users type them character by character", async () => {
    const captured: Record<string, FormDataEntryValue | null> = {};
    const fetcher = vi.fn(async (_input: RequestInfo | URL, init?: RequestInit) => {
      const formData = init?.body as FormData;
      captured.runtime_tools = formData.get("runtime_tools");
      captured.runtime_env = formData.get("runtime_env");
      return jsonResponse(uploadedSkill, 201);
    });
    const screen = await renderUploadView({ fetcher });

    await userEvent.upload(screen.getByLabelText("技能 zip 包"), new File(["zip"], "skill-api-doc.zip", { type: "application/zip" }));
    await userEvent.fill(screen.getByLabelText("技能中文名称"), "接口文档生成");
    await userEvent.type(screen.getByLabelText("CLI 依赖"), "gh,node");
    await userEvent.type(screen.getByLabelText("环境变量"), "GH_TOKEN,OPENAI_API_KEY");
    await userEvent.click(screen.getByRole("button", { name: "发布到技能市场" }));

    await vi.waitFor(() => expect(fetcher).toHaveBeenCalled());
    expect(captured.runtime_tools).toBe("gh,node");
    expect(captured.runtime_env).toBe("GH_TOKEN,OPENAI_API_KEY");
  });
});
