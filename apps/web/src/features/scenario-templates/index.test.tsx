import { describe, expect, it, vi } from "vitest";
import { render } from "vitest-browser-react";
import { userEvent } from "vitest/browser";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { ScenarioTemplatesPage } from "@/features/scenario-templates";
import { ApiRequestError } from "@/lib/api/client";
import {
  createScenarioTemplate,
  createScenarioTemplateVersion,
  listScenarioTemplateVersions,
  listScenarioTemplates,
  patchScenarioTemplate,
  type ScenarioTemplate,
  type ScenarioTemplateVersion
} from "@/lib/api/scenario-templates";

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
    createScenarioTemplate: vi.fn(),
    createScenarioTemplateVersion: vi.fn(),
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
    const screen = await renderPage();

    await expect.element(screen.getByText("场景模板").first()).toBeVisible();
    await expect.element(screen.getByText("software_delivery")).toBeVisible();
    await expect.element(screen.getByText("软件开发")).toBeVisible();
    await expect.element(screen.getByText("2 步")).toBeVisible();
  });

  it("shows the empty state when the registry has no rows", async () => {
    vi.mocked(listScenarioTemplates).mockResolvedValue([]);
    const screen = await renderPage();

    await expect.element(screen.getByText("还没有场景模板")).toBeVisible();
  });

  it("creates a new template through the create dialog", async () => {
    vi.mocked(listScenarioTemplates).mockResolvedValue([]);
    vi.mocked(createScenarioTemplate).mockResolvedValue({
      ...softwareDelivery,
      id: "new-template",
      template_key: "ops_review"
});

    const user = userEvent.setup();
    const screen = await renderPage();

    await user.click(screen.getByRole("button", { name: "新建模板" }));

    await expect
      .element(screen.getByRole("dialog", { name: "新建场景模板" }))
      .toBeVisible();

    await user.fill(screen.getByLabelText(/^template_key/), "ops_review");
    await user.fill(screen.getByLabelText(/^名称/), "运维评审");
    await user.fill(screen.getByLabelText(/^描述/), "运维变更评审场景");
    await user.click(screen.getByRole("button", { name: "创建" }));

    await vi.waitFor(() => {
      expect(createScenarioTemplate).toHaveBeenCalledWith(
        expect.anything(),
        expect.objectContaining({
          template_key: "ops_review",
          name: "运维评审",
          description: "运维变更评审场景",
          spec: expect.objectContaining({ spec_version: 2 })
}),
      );
    });

    await vi.waitFor(async () => {
      await expect
        .element(screen.getByRole("dialog", { name: "新建场景模板" }))
        .not.toBeInTheDocument();
    });
  });

  it("shows the server 400 detail text when create fails", async () => {
    vi.mocked(listScenarioTemplates).mockResolvedValue([]);
    vi.mocked(createScenarioTemplate).mockRejectedValue(
      new ApiRequestError(
        "create scenario template",
        400,
        "invalid input: unknown capability keys: bogus_capability",
      ),
    );

    const user = userEvent.setup();
    const screen = await renderPage();

    await user.click(screen.getByRole("button", { name: "新建模板" }));
    await user.fill(screen.getByLabelText(/^template_key/), "ops_review");
    await user.fill(screen.getByLabelText(/^名称/), "运维评审");
    await user.click(screen.getByRole("button", { name: "创建" }));

    await expect
      .element(screen.getByText(/unknown capability keys: bogus_capability/))
      .toBeVisible();
  });

  it("prefills the version dialog with the current spec and submits a new version", async () => {
    vi.mocked(listScenarioTemplates).mockResolvedValue([softwareDelivery]);
    vi.mocked(listScenarioTemplateVersions).mockResolvedValue([]);
    vi.mocked(createScenarioTemplateVersion).mockResolvedValue({
      ...softwareDelivery,
      active_version: 3
});

    const user = userEvent.setup();
    const screen = await renderPage();

    await user.click(screen.getByText("software_delivery"));
    await user.click(screen.getByRole("button", { name: "升版" }));

    const dialog = screen.getByRole("dialog", { name: "升版 software_delivery" });
    await expect.element(dialog).toBeVisible();

    const textarea = screen.getByLabelText(/spec/i);
    await expect
      .element(textarea)
      .toHaveValue(JSON.stringify(softwareDelivery.spec, null, 2));

    await user.click(screen.getByRole("button", { name: "提交新版本" }));

    await vi.waitFor(() => {
      expect(createScenarioTemplateVersion).toHaveBeenCalledWith(
        expect.anything(),
        "software_delivery",
        expect.objectContaining({
          spec: expect.objectContaining({
            roles: expect.arrayContaining([
              expect.objectContaining({ key: "developer" }),
            ])
})
}),
      );
    });
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
});
