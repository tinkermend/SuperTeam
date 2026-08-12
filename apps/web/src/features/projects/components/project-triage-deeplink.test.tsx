import type { AnchorHTMLAttributes, ReactNode } from "react";
import { describe, expect, it, vi } from "vitest";
import { render } from "vitest-browser-react";
import { userEvent } from "vitest/browser";
import { ProjectTriagePanel } from "./project-risk-home";
import type { Project } from "@/lib/api/projects";
import type {
  ProjectRiskReason,
  ProjectRiskSummary,
} from "../project-risk";

/**
 * 关注项精确下钻（spec 2026-08-11 §6.1）：右栏每条可行动项必须把 `sourceId` 写进
 * search，落到「那一条」而不是它所在的列表。
 *
 * 这是本轮整改的用户可见承重点——历史缺陷正是 id 被静默丢弃、以及 evidence 落到
 * 一个没有证据的 tab。断言直接看 Link 生成的 href（query 即 search）。
 */
vi.mock("@tanstack/react-router", () => {
  type MockLinkProps = AnchorHTMLAttributes<HTMLAnchorElement> & {
    children: ReactNode;
    params?: Record<string, string>;
    search?: Record<string, string>;
    to: string;
  };
  return {
    Link: ({ children, params, search, to, ...props }: MockLinkProps) => {
      let href = to;
      if (params) {
        for (const [key, value] of Object.entries(params)) {
          href = href.replace(`$${key}`, encodeURIComponent(value));
        }
      }
      const query = search ? `?${new URLSearchParams(search).toString()}` : "";
      return (
        <a {...props} data-router-link="true" href={`${href}${query}`}>
          {children}
        </a>
      );
    },
  };
});

const PROJECT_ID = "11111111-1111-4111-8111-111111111111";

function project(): Project {
  return {
    coordination_policy: {},
    coordination_status: "running",
    coordination_workflow_id: "wf-1",
    directory_name: "app",
    goal: "交付目标",
    human_owner_user_id: "user-1",
    id: PROJECT_ID,
    name: "批三 H1H4 全链路",
    status: "running",
    tenant_id: "22222222-2222-4222-8222-222222222222",
    workspace_ready_status: "ready",
  };
}

function summary(reasons: ProjectRiskReason[]): ProjectRiskSummary {
  return {
    level: "danger",
    project: project(),
    projectId: PROJECT_ID,
    reasons,
    requiresHuman: true,
    state: "ready",
  };
}

function hrefOf(container: HTMLElement, actionText: string): string {
  const link = [...container.querySelectorAll("a[data-router-link]")].find(
    (a) => (a.textContent ?? "").includes(actionText),
  );
  if (!link) throw new Error(`未找到动作链接：${actionText}`);
  return link.getAttribute("href") ?? "";
}

describe("ProjectTriagePanel 精确下钻", () => {
  it("失败任务带上 task=<id> 落到任务 tab（历史缺陷：id 被静默丢弃）", async () => {
    const { container } = await render(
      <ProjectTriagePanel
        project={project()}
        summary={summary([
          {
            id: "task:t-9:execution_failed",
            level: "danger",
            source: "tasks",
            sourceId: "t-9",
            title: "构建失败",
            type: "execution_failed",
          },
        ])}
      />,
    );

    const href = hrefOf(container, "查看失败任务");
    expect(href).toContain(`/projects/${PROJECT_ID}`);
    expect(href).toContain("tab=tasks");
    expect(href).toContain("task=t-9");
  });

  it("等人任务同样带 task=<id>", async () => {
    const { container } = await render(
      <ProjectTriagePanel
        project={project()}
        summary={summary([
          {
            id: "task:t-3:waiting_human",
            level: "danger",
            source: "tasks",
            sourceId: "t-3",
            title: "等待放行",
            type: "waiting_human",
          },
        ])}
      />,
    );

    const href = hrefOf(container, "查看等人任务");
    expect(href).toContain("tab=tasks");
    expect(href).toContain("task=t-3");
  });

  it("决策带 focus=<id> 落到审批 tab", async () => {
    const { container } = await render(
      <ProjectTriagePanel
        project={project()}
        summary={summary([
          {
            id: "decision:d-7",
            level: "danger",
            source: "decisions",
            sourceId: "d-7",
            title: "计划评审待决",
            type: "human_decision",
          },
        ])}
      />,
    );

    const href = hrefOf(container, "处理决策");
    expect(href).toContain("tab=approval");
    expect(href).toContain("focus=d-7");
  });

  it("证据落工作台治理区而非 assets（assets 下根本没有证据子 tab）", async () => {
    const { container, getByRole } = await render(
      <ProjectTriagePanel
        project={project()}
        summary={summary([
          {
            id: "evidence:e-2",
            level: "warn",
            source: "evidence",
            sourceId: "e-2",
            title: "接口回归录屏",
            type: "evidence_required",
          },
        ])}
      />,
    );

    // 证据不是可行动项，默认收在「其它信号」折叠区里，先展开。
    await userEvent.click(getByRole("button", { name: /其它信号/ }));

    const href = hrefOf(container, "查看证据核验");
    expect(href).toContain("tab=workbench");
    expect(href).toContain("governance=evidence");
    expect(href).toContain("evidence=e-2");
    expect(href).not.toContain("tab=assets");
  });

  it("sourceId 缺失时只落 tab，不产出空参数", async () => {
    const { container } = await render(
      <ProjectTriagePanel
        project={project()}
        summary={summary([
          {
            id: "task:unknown:execution_failed",
            level: "danger",
            source: "tasks",
            title: "无 id 的失败项",
            type: "execution_failed",
          },
        ])}
      />,
    );

    const href = hrefOf(container, "查看失败任务");
    expect(href).toContain("tab=tasks");
    expect(href).not.toContain("task=");
  });
});
