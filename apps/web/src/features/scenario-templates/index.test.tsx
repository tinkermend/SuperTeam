import type { ReactNode } from "react";
import { describe, expect, it, vi } from "vitest";
import { render } from "vitest-browser-react";
import { userEvent } from "vitest/browser";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { ScenarioTemplatesPage } from "@/features/scenario-templates";
import { listRoleVocabulary } from "@/lib/api/casting";
import {
  deleteScenarioTemplate,
  listScenarioTemplateVersions,
  listScenarioTemplates,
  patchScenarioTemplate,
  type ScenarioTemplate,
  type ScenarioTemplateVersion
} from "@/lib/api/scenario-templates";

vi.mock("@tanstack/react-router", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@tanstack/react-router")>();
  return {
    ...actual,
    Link: ({ children, to }: { children: ReactNode; to: string }) => <a href={to}>{children}</a>
};
});

vi.mock("@/lib/api/casting", async (importOriginal) => {
  const original = await importOriginal<typeof import("@/lib/api/casting")>();
  return {
    ...original,
    listRoleVocabulary: vi.fn()
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
    listScenarioTemplateVersions: vi.fn(),
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
    ],
    skeleton: [
      { step: "develop", role: "developer" },
      { step: "review", role: "reviewer", depends_on: ["develop"] },
    ],
    default_acceptance_criteria: ["变更以 branch+commit 交付且通过独立审查"]
},
  status: "active",
  active_version: 2,
  created_at: "2026-07-13T00:00:00Z",
  updated_at: "2026-07-13T00:00:00Z"
} satisfies ScenarioTemplate;

const softwareDeliveryV2 = {
  id: "00000000-0000-0000-0000-000000000402",
  tenant_id: "00000000-0000-0000-0000-000000000001",
  template_key: "software_delivery_v2",
  name: "软件开发 v2",
  description: "带出口的软件交付场景",
  spec: {
    spec_version: 2,
    roles: [
      { key: "developer", title: "开发" },
      { key: "reviewer", title: "审查", independent_from: ["developer"] },
    ],
    skeleton: [
      { step: "develop", role: "developer" },
      { step: "review", role: "reviewer", depends_on: ["develop"] },
    ],
    exits: [
      { deliverable: "branch_ref", label: "交付分支（不合入）" },
      { deliverable: "review_verdict", label: "审查通过并合入" },
    ],
    default_acceptance_criteria: [
      { statement: "变更以 branch+commit 交付", applies_from_exit: "branch_ref" },
      { statement: "通过独立审查", applies_from_exit: "review_verdict" },
    ]
},
  status: "active",
  active_version: 2,
  created_at: "2026-07-14T00:00:00Z",
  updated_at: "2026-07-14T00:00:00Z"
} satisfies ScenarioTemplate;

const versions: ScenarioTemplateVersion[] = [
  {
    id: "ver-2",
    template_id: softwareDelivery.id,
    version: 2,
    spec: softwareDelivery.spec,
    created_at: "2026-07-14T00:00:00Z"
},
  {
    id: "ver-1",
    template_id: softwareDelivery.id,
    version: 1,
    spec: { roles: [] },
    created_at: "2026-07-13T00:00:00Z"
},
];

const roleVocabulary = [
  {
    id: "role-developer",
    tenant_id: "00000000-0000-0000-0000-000000000001",
    role_key: "developer",
    title: "开发",
    description: "",
    status: "active",
    created_at: "2026-07-13T00:00:00Z",
    updated_at: "2026-07-13T00:00:00Z"
},
  {
    id: "role-reviewer",
    tenant_id: "00000000-0000-0000-0000-000000000001",
    role_key: "reviewer",
    title: "审查",
    description: "",
    status: "active",
    created_at: "2026-07-13T00:00:00Z",
    updated_at: "2026-07-13T00:00:00Z"
},
];

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
  it("renders the registry rows", async () => {
    vi.mocked(listScenarioTemplates).mockResolvedValue([softwareDelivery]);
    vi.mocked(listRoleVocabulary).mockResolvedValue(roleVocabulary);
    const screen = await renderPage();

    await expect.element(screen.getByText("场景模板").first()).toBeVisible();
    await expect.element(screen.getByText("software_delivery")).toBeVisible();
    await expect.element(screen.getByText("软件开发")).toBeVisible();
    await expect.element(screen.getByText("2 步")).toBeVisible();
    await expect.element(screen.getByRole("link", { name: "编辑" })).toBeVisible();
  });

  it("shows the empty state and create link when the registry has no rows", async () => {
    vi.mocked(listScenarioTemplates).mockResolvedValue([]);
    const screen = await renderPage();

    await expect.element(screen.getByText("还没有场景模板")).toBeVisible();
    await expect.element(screen.getByRole("link", { name: "新建模板" })).toBeVisible();
  });

  it("toggles status through a confirm dialog", async () => {
    vi.mocked(listScenarioTemplates).mockResolvedValue([softwareDelivery]);
    vi.mocked(listScenarioTemplateVersions).mockResolvedValue([]);
    vi.mocked(patchScenarioTemplate).mockResolvedValue({
      ...softwareDelivery,
      status: "disabled"
});

    const user = userEvent.setup();
    const screen = await renderPage();

    await user.click(screen.getByText("software_delivery"));
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

  it("lists version history with the active version marked once expanded", async () => {
    vi.mocked(listScenarioTemplates).mockResolvedValue([softwareDelivery]);
    vi.mocked(listScenarioTemplateVersions).mockResolvedValue(versions);

    const user = userEvent.setup();
    const screen = await renderPage();

    await user.click(screen.getByText("software_delivery"));

    await vi.waitFor(() => {
      expect(listScenarioTemplateVersions).toHaveBeenCalledWith(
        expect.anything(),
        "software_delivery",
      );
    });

    await expect.element(screen.getByText("v2")).toBeVisible();
    await expect.element(screen.getByText("v1")).toBeVisible();
    await expect.element(screen.getByText("当前版本")).toBeVisible();
  });

  it("renders v2 object-type acceptance criteria with exit labels when expanded", async () => {
    vi.mocked(listScenarioTemplates).mockResolvedValue([softwareDeliveryV2]);
    vi.mocked(listScenarioTemplateVersions).mockResolvedValue([]);

    const user = userEvent.setup();
    const screen = await renderPage();

    await user.click(screen.getByText("software_delivery_v2"));

    // Criteria should be visible and properly formatted
    await expect
      .element(screen.getByText(/变更以 branch\+commit 交付（出口 ≥ 交付分支（不合入））/))
      .toBeVisible();
    await expect
      .element(screen.getByText(/通过独立审查（出口 ≥ 审查通过并合入）/))
      .toBeVisible();
  });

  it("deletes a template through a confirm dialog", async () => {
    vi.mocked(listScenarioTemplates).mockResolvedValue([softwareDelivery]);
    vi.mocked(listScenarioTemplateVersions).mockResolvedValue([]);
    vi.mocked(deleteScenarioTemplate).mockResolvedValue(undefined);

    const user = userEvent.setup();
    const screen = await renderPage();

    await user.click(screen.getByRole("button", { name: "删除" }));
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
