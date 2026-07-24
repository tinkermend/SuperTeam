import { describe, expect, it, vi } from "vitest";
import { render } from "vitest-browser-react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { userEvent } from "vitest/browser";
import { SubmitDemandDialog } from "./submit-demand-dialog";
import {
  listScenarioTemplates,
  type ScenarioTemplate
} from "@/lib/api/scenario-templates";

vi.mock("@/lib/config/control-plane-url", () => ({
  resolveControlPlaneUrl: () => "http://control-plane.local"
}));
vi.mock("@/lib/api/scenario-templates", async (importOriginal) => {
  const original =
    await importOriginal<typeof import("@/lib/api/scenario-templates")>();
  return {
    ...original,
    listScenarioTemplates: vi.fn()
};
});

const softwareDelivery = {
  id: "00000000-0000-0000-0000-000000000401",
  tenant_id: "00000000-0000-0000-0000-000000000001",
  template_key: "software_delivery",
  name: "软件开发",
  description: "开发→审查→测试的软件交付场景",
  spec: {},
  status: "active",
  created_at: "2026-07-13T00:00:00Z",
  updated_at: "2026-07-13T00:00:00Z"
} satisfies ScenarioTemplate;

const draftTemplate = {
  ...softwareDelivery,
  id: "00000000-0000-0000-0000-000000000402",
  template_key: "draft_template",
  name: "草稿模板",
  status: "draft"
} satisfies ScenarioTemplate;

async function renderDialog(
  props: Partial<React.ComponentProps<typeof SubmitDemandDialog>> = {},
) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } }
});
  return await render(
    <QueryClientProvider client={queryClient}>
      <SubmitDemandDialog
        onOpenChange={vi.fn()}
        onSubmit={vi.fn()}
        open
        {...props}
      />
    </QueryClientProvider>,
  );
}

describe("SubmitDemandDialog", () => {
  it("offers an active scenario template select and defaults to unbound", async () => {
    vi.mocked(listScenarioTemplates).mockResolvedValue([
      softwareDelivery,
      draftTemplate,
    ]);
    const screen = await renderDialog();

    await expect
      .element(screen.getByRole("combobox", { name: "场景模板" }))
      .toBeVisible();
    await userEvent.click(screen.getByRole("combobox", { name: "场景模板" }));
    await expect
      .element(screen.getByRole("option", { name: "软件开发（software_delivery）" }))
      .toBeVisible();
    await expect
      .element(screen.getByRole("option", { name: "草稿模板（draft_template）" }))
      .not.toBeInTheDocument();
  });

  it("submits scenario_template_key when a template is selected", async () => {
    vi.mocked(listScenarioTemplates).mockResolvedValue([softwareDelivery]);
    const onSubmit = vi.fn();
    const screen = await renderDialog({ onSubmit });

    await userEvent.type(
      screen.getByPlaceholder("补充验收证据"),
      "验证发布任务",
    );
    await userEvent.click(screen.getByRole("combobox", { name: "场景模板" }));
    await userEvent.click(
      screen.getByRole("option", { name: "软件开发（software_delivery）" }),
    );
    await userEvent.click(screen.getByRole("button", { name: "提交" }));

    expect(onSubmit).toHaveBeenCalledWith(
      expect.objectContaining({
        scenario_template_key: "software_delivery",
        title: "验证发布任务"
}),
    );
  });

  it("omits scenario_template_key when left unbound", async () => {
    vi.mocked(listScenarioTemplates).mockResolvedValue([softwareDelivery]);
    const onSubmit = vi.fn();
    const screen = await renderDialog({ onSubmit });

    await userEvent.type(screen.getByPlaceholder("补充验收证据"), "常规需求");
    await userEvent.click(screen.getByRole("button", { name: "提交" }));

    expect(onSubmit).toHaveBeenCalledWith(
      expect.objectContaining({ title: "常规需求" }),
    );
    const call = onSubmit.mock.calls[0]?.[0];
    expect(call.scenario_template_key).toBeUndefined();
  });
});
