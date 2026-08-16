import type { ReactNode } from "react";
import { describe, expect, it, vi } from "vitest";
import { render } from "vitest-browser-react";
import { userEvent } from "vitest/browser";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { ScenarioTemplateComposerPage } from "./composer-page";
import { ApiRequestError } from "@/lib/api/client";
import { listRoleVocabulary } from "@/lib/api/casting";
import {
  createScenarioTemplate,
  createScenarioTemplateVersion,
  getScenarioTemplate,
  listScenarioTemplates,
} from "@/lib/api/scenario-templates";

vi.mock("@tanstack/react-router", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@tanstack/react-router")>();
  return {
    ...actual,
    Link: ({ children, to }: { children: ReactNode; to: string }) => <a href={to}>{children}</a>,
    useNavigate: () => vi.fn(),
  };
});

vi.mock("@/lib/api/casting", async (importOriginal) => {
  const original = await importOriginal<typeof import("@/lib/api/casting")>();
  return { ...original, listRoleVocabulary: vi.fn() };
});

vi.mock("@/components/layout/main", () => ({
  Main: ({ children }: { children?: React.ReactNode }) => <main>{children}</main>,
}));
vi.mock("@/components/layout/shell-page-header", () => ({
  ShellPageHeader: ({ title, subtitle }: { title: string; subtitle?: string }) => (
    <header>
      <h1>{title}</h1>
      {subtitle ? <p>{subtitle}</p> : null}
    </header>
  ),
  ShellPageHeaderBack: () => <a href="/scenario-templates">返回</a>,
}));
vi.mock("@/lib/config/control-plane-url", () => ({
  resolveControlPlaneUrl: () => "http://control-plane.local",
}));
vi.mock("@/lib/api/scenario-templates", async (importOriginal) => {
  const original = await importOriginal<typeof import("@/lib/api/scenario-templates")>();
  return {
    ...original,
    createScenarioTemplate: vi.fn(),
    createScenarioTemplateVersion: vi.fn(),
    getScenarioTemplate: vi.fn(),
    listScenarioTemplates: vi.fn(),
    patchScenarioTemplate: vi.fn(),
  };
});

const roleVocabulary = [
  {
    id: "role-developer",
    tenant_id: "t",
    role_key: "developer",
    title: "开发",
    description: "",
    status: "active",
    created_at: "",
    updated_at: "",
  },
  {
    id: "role-reviewer",
    tenant_id: "t",
    role_key: "reviewer",
    title: "审查",
    description: "",
    status: "active",
    created_at: "",
    updated_at: "",
  },
];

async function renderCreate() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return render(
    <QueryClientProvider client={queryClient}>
      <ScenarioTemplateComposerPage mode="create" />
    </QueryClientProvider>,
  );
}

describe("ScenarioTemplateComposerPage", () => {
  it("creates a serial template from the full page", async () => {
    vi.mocked(listRoleVocabulary).mockResolvedValue(roleVocabulary);
    vi.mocked(listScenarioTemplates).mockResolvedValue([]);
    vi.mocked(createScenarioTemplate).mockResolvedValue({
      id: "new",
      tenant_id: "t",
      template_key: "ops_review",
      name: "运维评审",
      description: "运维变更评审场景",
      spec: {},
      status: "active",
      created_at: "",
      updated_at: "",
    });

    const user = userEvent.setup();
    const screen = await renderCreate();

    await expect.element(screen.getByText("新建模板").first()).toBeVisible();
    await user.fill(screen.getByRole("textbox", { name: "名称", exact: true }), "运维评审");
    await user.fill(screen.getByLabelText("内部标识"), "ops_review");
    await user.fill(screen.getByLabelText("描述"), "运维变更评审场景");
    await vi.waitFor(() => expect(listRoleVocabulary).toHaveBeenCalled());
    await user.selectOptions(screen.getByLabelText("节点 1 接收角色"), "developer");
    await user.click(screen.getByRole("button", { name: "保存" }));

    await vi.waitFor(() => {
      expect(createScenarioTemplate).toHaveBeenCalledWith(
        expect.anything(),
        expect.objectContaining({
          template_key: "ops_review",
          name: "运维评审",
          spec: expect.objectContaining({
            spec_version: 2,
            exits: expect.any(Array),
          }),
        }),
      );
    });
  });

  it("shows server 400 detail when create fails", async () => {
    vi.mocked(listRoleVocabulary).mockResolvedValue(roleVocabulary);
    vi.mocked(listScenarioTemplates).mockResolvedValue([]);
    vi.mocked(createScenarioTemplate).mockRejectedValue(
      new ApiRequestError(
        "create scenario template",
        400,
        "invalid input: unknown capability keys: bogus_capability",
      ),
    );

    const user = userEvent.setup();
    const screen = await renderCreate();
    await user.fill(screen.getByRole("textbox", { name: "名称", exact: true }), "运维评审");
    await user.fill(screen.getByLabelText("内部标识"), "ops_review");
    await vi.waitFor(() => expect(listRoleVocabulary).toHaveBeenCalled());
    await user.selectOptions(screen.getByLabelText("节点 1 接收角色"), "developer");
    await user.click(screen.getByRole("button", { name: "保存" }));

    await expect.element(screen.getByText(/unknown capability keys: bogus_capability/)).toBeVisible();
  });

  it("rejects a chinese template key before calling the api", async () => {
    vi.mocked(createScenarioTemplate).mockClear();
    vi.mocked(listRoleVocabulary).mockResolvedValue(roleVocabulary);
    vi.mocked(listScenarioTemplates).mockResolvedValue([]);

    const user = userEvent.setup();
    const screen = await renderCreate();
    await user.fill(screen.getByRole("textbox", { name: "名称", exact: true }), "运维评审");
    const keyField = screen.getByLabelText("内部标识");
    await user.clear(keyField);
    await user.fill(keyField, "中文标识");
    await vi.waitFor(() => expect(listRoleVocabulary).toHaveBeenCalled());
    await user.selectOptions(screen.getByLabelText("节点 1 接收角色"), "developer");
    await user.click(screen.getByRole("button", { name: "保存" }));

    await expect
      .element(screen.getByText("内部标识只能用英文字母、数字和下划线，且必须以字母开头"))
      .toBeVisible();
    expect(createScenarioTemplate).not.toHaveBeenCalled();
  });

  it("blocks spec save for a parallel seed template", async () => {
    vi.mocked(listRoleVocabulary).mockResolvedValue(roleVocabulary);
    vi.mocked(getScenarioTemplate).mockResolvedValue({
      id: "sd",
      tenant_id: "t",
      template_key: "software_delivery",
      name: "软件交付",
      description: "",
      spec: {
        skeleton: [
          { step: "develop", role: "developer" },
          { step: "review", role: "reviewer", depends_on: ["develop"] },
          { step: "test", role: "tester", depends_on: ["develop"] },
          { step: "release", role: "developer", depends_on: ["review", "test"] },
        ],
      },
      status: "active",
      created_at: "",
      updated_at: "",
    });

    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
    });
    const screen = await render(
      <QueryClientProvider client={queryClient}>
        <ScenarioTemplateComposerPage mode="edit" templateKey="software_delivery" />
      </QueryClientProvider>,
    );

    await expect.element(screen.getByText(/此模板含并行依赖/)).toBeVisible();
    expect(createScenarioTemplateVersion).not.toHaveBeenCalled();
  });
});
