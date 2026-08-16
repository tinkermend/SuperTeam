import type { ReactNode } from "react";
import { describe, expect, it, vi } from "vitest";
import { render } from "vitest-browser-react";
import { userEvent } from "vitest/browser";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { ScenarioTemplatesPage } from "@/features/scenario-templates";
import {
  deleteScenarioTemplate,
  listScenarioTemplates,
  patchScenarioTemplate,
  type ScenarioTemplate
} from "@/lib/api/scenario-templates";

vi.mock("@tanstack/react-router", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@tanstack/react-router")>();
  return {
    ...actual,
    Link: ({ children, to }: { children: ReactNode; to: string }) => <a href={to}>{children}</a>,
    useNavigate: () => vi.fn()
  };
});

vi.mock("@/components/layout/main", () => ({
  Main: ({ children }: { children?: React.ReactNode }) => <main>{children}</main>
}));
vi.mock("@/components/layout/shell-page-header", () => ({
  ShellPageHeader: ({ title, subtitle }: { title: string; subtitle?: string }) => (
    <header>
      <h1>{title}</h1>
      {subtitle ? <p>{subtitle}</p> : null}
    </header>
  )
}));
vi.mock("@/lib/config/control-plane-url", () => ({
  resolveControlPlaneUrl: () => "http://control-plane.local"
}));
vi.mock("@/lib/api/scenario-templates", async (importOriginal) => {
  const original =
    await importOriginal<typeof import("@/lib/api/scenario-templates")>();
  return {
    ...original,
    deleteScenarioTemplate: vi.fn(),
    listScenarioTemplates: vi.fn(),
    patchScenarioTemplate: vi.fn()
  };
});

const softwareDelivery = {
  id: "00000000-0000-0000-0000-000000000401",
  tenant_id: "00000000-0000-0000-0000-000000000001",
  template_key: "software_delivery",
  name: "软件开发",
  description: "开发→审查→测试的软件交付场景",
  spec: {
    roles: [
      { key: "developer", title: "开发" },
      { key: "reviewer", title: "审查", independent_from: ["developer"] },
      { key: "tester", title: "测试" }
    ],
    skeleton: [
      { step: "develop", role: "developer", produces_defaults: [{ name: "branch_ref" }] },
      { step: "review", role: "reviewer", depends_on: ["develop"], produces_defaults: [{ name: "review_verdict" }] },
      { step: "test", role: "tester", depends_on: ["develop"], produces_defaults: [{ name: "test_report" }] },
      { step: "release", title: "发布", role: "developer", depends_on: ["review", "test"] }
    ],
    exits: [
      { deliverable: "review_verdict", label: "审查通过" },
      { deliverable: "test_report", label: "测试通过" },
      { deliverable: "release_record", label: "已发布" }
    ]
  },
  status: "active",
  active_version: 2,
  created_at: "2026-07-13T00:00:00Z",
  updated_at: "2026-07-13T00:00:00Z"
} satisfies ScenarioTemplate;

const genericTemplate = {
  id: "00000000-0000-0000-0000-000000000400",
  tenant_id: "00000000-0000-0000-0000-000000000001",
  template_key: "generic",
  name: "通用兜底",
  description: "",
  spec: { roles: [], skeleton: [] },
  status: "active",
  created_at: "2026-07-13T00:00:00Z",
  updated_at: "2026-07-13T00:00:00Z"
} satisfies ScenarioTemplate;

const disabledReview = {
  id: "00000000-0000-0000-0000-000000000403",
  tenant_id: "00000000-0000-0000-0000-000000000001",
  template_key: "ops_review",
  name: "运维评审",
  description: "停用中的评审骨架。",
  spec: {
    roles: [{ key: "reviewer", title: "评审" }],
    skeleton: [{ step: "review", role: "reviewer" }]
  },
  status: "disabled",
  created_at: "2026-07-01T00:00:00Z",
  updated_at: "2026-07-10T00:00:00Z"
} satisfies ScenarioTemplate;

async function renderPage() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } }
  });
  return await render(
    <QueryClientProvider client={queryClient}>
      <ScenarioTemplatesPage />
    </QueryClientProvider>,
  );
}

describe("ScenarioTemplatesPage", () => {
  it("renders chain-in-table rows without version history", async () => {
    vi.mocked(listScenarioTemplates).mockResolvedValue([softwareDelivery, genericTemplate]);
    const screen = await renderPage();

    await expect.element(screen.getByText("场景模板").first()).toBeVisible();
    await expect.element(screen.getByText("software_delivery")).toBeVisible();
    await expect.element(screen.getByRole("link", { name: "软件开发" })).toBeVisible();
    await expect.element(screen.getByText("∥")).toBeVisible();
    await expect.element(screen.getByText("发布", { exact: true })).toBeVisible();
    await expect.element(screen.getByText("无骨架 · generic 行为")).toBeVisible();
    await expect.element(screen.getByText("已发布")).toBeVisible();
    await expect.element(screen.getByText("版本历史")).not.toBeInTheDocument();
    await expect.element(screen.getByText("当前版本")).not.toBeInTheDocument();
    await expect.element(screen.getByRole("link", { name: "编辑" }).first()).toBeVisible();
  });

  it("shows the empty state and create link when the registry has no rows", async () => {
    vi.mocked(listScenarioTemplates).mockResolvedValue([]);
    const screen = await renderPage();

    await expect.element(screen.getByText("还没有场景模板")).toBeVisible();
    await expect.element(screen.getByRole("link", { name: "新建模板" }).first()).toBeVisible();
  });

  it("filters by status and keeps full-set counts", async () => {
    vi.mocked(listScenarioTemplates).mockResolvedValue([softwareDelivery, disabledReview]);
    const user = userEvent.setup();
    const screen = await renderPage();

    await expect.element(screen.getByText("软件开发")).toBeVisible();
    await expect.element(screen.getByText("运维评审")).toBeVisible();
    await expect.element(screen.getByRole("button", { name: /已禁用/ })).toBeVisible();

    await user.click(screen.getByRole("button", { name: /已禁用/ }));
    await expect.element(screen.getByText("运维评审")).toBeVisible();
    await expect.element(screen.getByText("软件开发")).not.toBeInTheDocument();
    await expect.element(screen.getByRole("button", { name: /启用中/ })).toBeVisible();
  });

  it("filters by name search", async () => {
    vi.mocked(listScenarioTemplates).mockResolvedValue([
      softwareDelivery,
      disabledReview,
      genericTemplate,
    ]);
    const user = userEvent.setup();
    const screen = await renderPage();

    await user.type(screen.getByRole("searchbox", { name: "搜索模板名称" }), "软件");
    await expect.element(screen.getByText("软件开发")).toBeVisible();
    await expect.element(screen.getByText("运维评审")).not.toBeInTheDocument();
    await expect.element(screen.getByText("通用兜底")).not.toBeInTheDocument();
  });

  it("pages the filtered list", async () => {
    const many = Array.from({ length: 11 }, (_, index) => ({
      ...genericTemplate,
      id: `00000000-0000-0000-0000000004${String(index).padStart(2, "0")}`,
      template_key: `tpl_${index}`,
      name: `模板 ${index + 1}`,
    })) satisfies ScenarioTemplate[];
    vi.mocked(listScenarioTemplates).mockResolvedValue(many);
    const user = userEvent.setup();
    const screen = await renderPage();

    await expect.element(screen.getByText("模板 1", { exact: true })).toBeVisible();
    await expect.element(screen.getByText("模板 11", { exact: true })).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "下一页" }));
    await expect.element(screen.getByText("模板 11", { exact: true })).toBeVisible();
    await expect.element(screen.getByText("模板 1", { exact: true })).not.toBeInTheDocument();
  });

  it("shows a no-match empty state instead of the create empty state", async () => {
    vi.mocked(listScenarioTemplates).mockResolvedValue([softwareDelivery]);
    const user = userEvent.setup();
    const screen = await renderPage();

    await user.click(screen.getByRole("button", { name: /已禁用/ }));
    await expect.element(screen.getByText("没有符合筛选的模板")).toBeVisible();
    await expect.element(screen.getByText("还没有场景模板")).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "查看全部" }));
    await expect.element(screen.getByText("软件开发")).toBeVisible();
  });

  it("toggles status through a confirm dialog", async () => {
    vi.mocked(listScenarioTemplates).mockResolvedValue([softwareDelivery]);
    vi.mocked(patchScenarioTemplate).mockResolvedValue({
      ...softwareDelivery,
      status: "disabled"
    });

    const user = userEvent.setup();
    const screen = await renderPage();

    await user.click(screen.getByRole("button", { name: "停用" }));
    await expect.element(screen.getByText(/generic/)).toBeVisible();
    await user.click(screen.getByRole("button", { name: "确认停用" }));

    await vi.waitFor(() => {
      expect(patchScenarioTemplate).toHaveBeenCalledWith(
        expect.anything(),
        "software_delivery",
        expect.objectContaining({ status: "disabled" }),
      );
    });
  });

  it("does not render expanded acceptance criteria", async () => {
    vi.mocked(listScenarioTemplates).mockResolvedValue([
      {
        ...softwareDelivery,
        spec: {
          ...softwareDelivery.spec,
          default_acceptance_criteria: [
            { statement: "变更以 branch+commit 交付", applies_from_exit: "branch_ref" }
          ]
        }
      }
    ]);
    const screen = await renderPage();
    await expect.element(screen.getByText(/出口 ≥/)).not.toBeInTheDocument();
  });

  it("deletes a template through the overflow menu and confirm dialog", async () => {
    vi.mocked(listScenarioTemplates).mockResolvedValue([softwareDelivery]);
    vi.mocked(deleteScenarioTemplate).mockResolvedValue(undefined);

    const user = userEvent.setup();
    const screen = await renderPage();

    await user.click(screen.getByRole("button", { name: "更多操作" }));
    await user.click(screen.getByRole("menuitem", { name: "删除" }));
    await expect.element(screen.getByText(/删除后列表和任务发起不再出现/)).toBeVisible();
    await user.click(screen.getByRole("button", { name: "确认删除" }));

    await vi.waitFor(() => {
      expect(deleteScenarioTemplate).toHaveBeenCalledWith(
        expect.anything(),
        "software_delivery",
      );
    });
  });
});
