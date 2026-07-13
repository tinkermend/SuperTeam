import { describe, expect, it, vi } from "vitest";
import { render } from "vitest-browser-react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { ScenarioTemplatesPage } from "@/features/scenario-templates";
import {
  listScenarioTemplates,
  type ScenarioTemplate,
} from "@/lib/api/scenario-templates";

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
}));
vi.mock("@/lib/config/control-plane-url", () => ({
  resolveControlPlaneUrl: () => "http://control-plane.local",
}));
vi.mock("@/lib/api/scenario-templates", async (importOriginal) => {
  const original =
    await importOriginal<typeof import("@/lib/api/scenario-templates")>();
  return {
    ...original,
    listScenarioTemplates: vi.fn(),
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
    default_acceptance_criteria: ["变更以 branch+commit 交付且通过独立审查"],
  },
  status: "active",
  created_at: "2026-07-13T00:00:00Z",
  updated_at: "2026-07-13T00:00:00Z",
} satisfies ScenarioTemplate;

async function renderPage() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
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
});
