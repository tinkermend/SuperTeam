import type { AnchorHTMLAttributes, ReactNode } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { describe, expect, it, vi } from "vitest";
import { render } from "vitest-browser-react";
import { ProjectTaskDetailDialog } from "./project-task-detail-dialog";
import type { ProjectTask } from "@/lib/api/projects";

/**
 * 任务详情弹层的深链缺页回落（spec 2026-08-11 §6.3 第 2 条）。
 *
 * 承重理由：项目页只加载最近 20 条任务（`ORDER BY updated_at DESC LIMIT 20`），
 * 而"很久以前失败、现在才被关注到"的任务恰恰掉在窗口外。历史实现是
 * `if (!task) return null`——深链到窗外任务时**静默什么都不发生**。
 * 这几条断言就是在钉死"窗外也能打开"，以及三种失败态不许互相冒充。
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
const OUT_OF_WINDOW_TASK_ID = "99999999-9999-4999-8999-999999999999";

function task(overrides: Partial<ProjectTask> = {}): ProjectTask {
  return {
    id: OUT_OF_WINDOW_TASK_ID,
    project_id: PROJECT_ID,
    requires_human_approval: false,
    status: "failed",
    tenant_id: "22222222-2222-4222-8222-222222222222",
    title: "分页窗口外的失败任务",
    ...overrides,
  };
}

function renderDialog(options: {
  fetcher?: typeof fetch;
  tasks?: ProjectTask[];
  withApiOptions?: boolean;
}) {
  const client = new QueryClient({
    defaultOptions: { queries: { gcTime: 0, retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <ProjectTaskDetailDialog
        apiOptions={
          options.withApiOptions === false
            ? undefined
            : { baseUrl: "http://control-plane.local", fetcher: options.fetcher }
        }
        decisionRequests={[]}
        demands={[]}
        onOpenChange={() => {}}
        onResolveDecision={() => {}}
        projectId={PROJECT_ID}
        taskId={OUT_OF_WINDOW_TASK_ID}
        tasks={options.tasks ?? []}
      />
    </QueryClientProvider>,
  );
}

describe("ProjectTaskDetailDialog 深链缺页回落", () => {
  it("任务不在已加载列表时单查取回并渲染（历史缺陷：静默 return null）", async () => {
    const fetcher = vi.fn(
      async () =>
        new Response(JSON.stringify(task()), {
          headers: { "content-type": "application/json" },
          status: 200,
        }),
    );

    const screen = await renderDialog({ fetcher });

    await expect
      .element(screen.getByText("分页窗口外的失败任务"))
      .toBeInTheDocument();
    expect(fetcher).toHaveBeenCalledWith(
      `http://control-plane.local/api/v1/projects/${PROJECT_ID}/tasks/${OUT_OF_WINDOW_TASK_ID}`,
      expect.objectContaining({ method: "GET" }),
    );
  });

  it("任务已在列表窗口内时不发单查请求", async () => {
    const fetcher = vi.fn(async () => new Response("{}", { status: 200 }));

    const screen = await renderDialog({ fetcher, tasks: [task()] });

    await expect
      .element(screen.getByText("分页窗口外的失败任务"))
      .toBeInTheDocument();
    expect(fetcher).not.toHaveBeenCalled();
  });

  it("404 说「不存在」", async () => {
    const screen = await renderDialog({
      fetcher: vi.fn(async () => new Response("not found", { status: 404 })),
    });

    await expect
      .element(screen.getByTestId("project-task-detail-missing"))
      .toBeInTheDocument();
    await expect.element(screen.getByText(/任务不存在或已被清理/)).toBeInTheDocument();
  });

  it("500 说「加载失败」而不是冒充不存在", async () => {
    const screen = await renderDialog({
      fetcher: vi.fn(async () => new Response("boom", { status: 500 })),
    });

    await expect
      .element(screen.getByText(/加载任务失败/))
      .toBeInTheDocument();
  });

  it("无 apiOptions 时说「无法单查」而不是冒充不存在", async () => {
    const screen = await renderDialog({ withApiOptions: false });

    await expect
      .element(screen.getByText(/任务不在当前列表/))
      .toBeInTheDocument();
  });
});
