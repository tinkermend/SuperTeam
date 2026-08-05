import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { describe, expect, it, vi } from "vitest";
import { render } from "vitest-browser-react";

import { PlaybookCastingPanel } from "./playbook-casting-panel";

/**
 * 剧本编制面板的产品判据（spec 2026-08-04 §5.2 / §6.2）：
 * - 剧本一律可选，用「可达最深收口 + 缺什么」表达，**不置灰**
 * - 「暂不可跑」也要显示，不能隐藏——隐藏会让人以为平台没有这个剧本
 * - 能力是**提示**不是门禁：不满足的候选标 ⚠ 但仍可选
 */

const PROJECT_ID = "project-1";

function jsonResponse(payload: unknown, status = 200): Response {
  return new Response(JSON.stringify(payload), {
    headers: { "Content-Type": "application/json" },
    status
});
}

const templates = [
  { name: "软件开发", status: "active", template_key: "software_delivery" },
  { name: "故障排查", status: "active", template_key: "incident_response" },
  { name: "已停用的", status: "disabled", template_key: "retired_one" },
];

const readiness = [
  {
    deepest_exit: { deliverable: "branch_ref", label: "交付分支" },
    missing_roles_for_any: [],
    next_exit_needs_roles: ["reviewer"],
    runnable: true,
    scenario_template_key: "software_delivery",
    template_name: "软件开发"
},
  {
    deepest_exit: null,
    missing_roles_for_any: ["collector", "analyst"],
    next_exit_needs_roles: [],
    runnable: false,
    scenario_template_key: "ops_analysis",
    template_name: "运维分析"
},
];

/** 一个满足能力、一个不满足——用于验证 ⚠ 仍可选。 */
const candidates = [
  {
    capability_fit: "matched",
    digital_employee_id: "emp-fit",
    matched_capabilities: ["code_implementation"],
    missing_capabilities: [],
    name: "开发-A",
    team_name: "默认团队"
},
  {
    capability_fit: "missing",
    digital_employee_id: "emp-unfit",
    matched_capabilities: [],
    missing_capabilities: ["code_implementation"],
    name: "外包-Z",
    team_name: "外包组"
},
];

function stubFetcher(): typeof fetch {
  return async (input: RequestInfo | URL) => {
    const url = String(input);
    if (url.includes("/playbook-readiness")) return jsonResponse(readiness);
    if (url.includes("/role-candidates")) return jsonResponse(candidates);
    if (url.includes("/castings")) return jsonResponse([]);
    if (url.includes("/scenario-templates")) return jsonResponse(templates);
    return jsonResponse({ detail: "not found" }, 404);
  };
}

async function renderPanel(props: Partial<React.ComponentProps<typeof PlaybookCastingPanel>> = {}) {
  vi.stubGlobal("fetch", stubFetcher());
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } }
});
  return await render(
    <QueryClientProvider client={queryClient}>
      <PlaybookCastingPanel projectId={PROJECT_ID} {...props} />
    </QueryClientProvider>,
  );
}

describe("PlaybookCastingPanel", () => {
  it("以「可达最深收口 + 再往深需要什么」表达可跑性，而不是可选/不可选", async () => {
    const screen = await renderPanel();

    await expect
      .element(screen.getByText("可走到「交付分支」 · 再往深需要：reviewer"))
      .toBeVisible();
  });

  it("暂不可跑的剧本仍然显示并点名缺什么角色（不隐藏）", async () => {
    const screen = await renderPanel();

    // 隐藏会让人以为平台没有这个剧本；必须显示且说清缺口。
    await expect.element(screen.getByText("运维分析")).toBeVisible();
    await expect.element(screen.getByText("暂不可跑 · 缺：collector、analyst")).toBeVisible();
  });

  it("锁定剧本时不再显示剧本选择器，直接进入角色编制", async () => {
    const screen = await renderPanel({ lockedTemplateKey: "software_delivery" });

    // 发起对话框里剧本已选定，不该再让人改。
    expect(screen.container.textContent).not.toContain("场景模板");
  });

  it("说明文案讲清编制的两条产品规则：自动入池、能力仅提示", async () => {
    const screen = await renderPanel();

    await expect
      .element(
        screen.getByText("为每个角色指定一人；选人自动加入项目成员池。能力仅作提示，⚠ 仍可选。"),
      )
      .toBeVisible();
  });
});
