import type { ReactNode } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, describe, expect, it, vi } from "vitest";
import { render } from "vitest-browser-react";
import { userEvent } from "vitest/browser";

let routerSearch: Record<string, string | undefined> = {};

vi.mock("@tanstack/react-router", () => ({
  Link: ({
    children,
    params,
    search,
    to,
  }: {
    children: ReactNode;
    params?: Record<string, string>;
    search?: Record<string, string | undefined>;
    to: string;
  }) => {
    let href = to;
    if (params) {
      for (const [key, value] of Object.entries(params)) {
        href = href.replace(`$${key}`, encodeURIComponent(value));
      }
    }
    const query = search
      ? `?${new URLSearchParams(Object.entries(search).filter((entry): entry is [string, string] => Boolean(entry[1]))).toString()}`
      : "";
    return <a href={`${href}${query}`}>{children}</a>;
  },
  useSearch: () => routerSearch,
}));

vi.mock("@/components/layout/main", () => ({
  Main: ({ children }: { children: ReactNode }) => <main data-testid="run-overview-main">{children}</main>,
}));

vi.mock("@/components/layout/shell-page-header", () => ({
  ShellPageHeader: ({
    actions,
    subtitle,
    title,
  }: {
    actions?: ReactNode;
    subtitle?: ReactNode;
    title: ReactNode;
  }) => (
    <header data-testid="run-overview-shell-header">
      <h1>{title}</h1>
      {subtitle ? <p>{subtitle}</p> : null}
      {actions ? <div data-testid="run-overview-shell-header-actions">{actions}</div> : null}
    </header>
  ),
}));

import { RunOverviewView } from "@/features/run-overview";
import { digitalEmployeeOverviewFixture, teamListFixture } from "./runtime-overview-fixtures";

function jsonResponse(body: unknown) {
  return new Response(JSON.stringify(body), {
    headers: { "content-type": "application/json" },
    status: 200,
  });
}

function createFetcher() {
  const requests: Array<{ pathname: string; search: string }> = [];
  const fetcher = vi.fn(async (input: RequestInfo | URL) => {
    const url = new URL(String(input));
    requests.push({ pathname: url.pathname, search: url.search });
    if (url.pathname === "/api/v1/digital-employees/overview") {
      return jsonResponse(digitalEmployeeOverviewFixture);
    }
    if (url.pathname === "/api/v1/teams") {
      return jsonResponse(teamListFixture);
    }
    return new Response(JSON.stringify({ error: `unhandled ${url.pathname}` }), { status: 404 });
  }) as unknown as typeof fetch;
  return { fetcher, requests };
}

function queryClient() {
  return new QueryClient({
    defaultOptions: {
      queries: { retry: false },
    },
  });
}

async function renderPage(fetcher: typeof fetch, search: Record<string, string | undefined> = {}) {
  routerSearch = search;
  return await render(
    <QueryClientProvider client={queryClient()}>
      <RunOverviewView apiBaseUrl="http://control-plane.local" fetcher={fetcher} />
    </QueryClientProvider>,
  );
}

describe("RunOverviewView", () => {
  afterEach(() => {
    routerSearch = {};
  });

  it("renders the runtime overview map from existing Control Plane read APIs", async () => {
    const { fetcher, requests } = createFetcher();
    const screen = await renderPage(fetcher);

    await expect.element(screen.getByRole("heading", { name: "运行总览" })).toBeVisible();
    await expect.element(screen.getByText("开发团队").first()).toBeVisible();
    await expect.element(screen.getByText("运维团队").first()).toBeVisible();
    await expect.element(screen.getByText("高秀英").first()).toBeVisible();
    await expect.element(screen.getByText("工位 3/3").first()).toBeVisible();
    expect(requests.some((request) => request.pathname === "/api/v1/digital-employees/overview")).toBe(true);
    expect(requests.some((request) => request.pathname === "/api/v1/teams")).toBe(true);
  });

  it("keeps shell header outside the page content and page actions inside the map toolbar", async () => {
    const { fetcher } = createFetcher();
    const screen = await renderPage(fetcher);

    await expect.element(screen.getByRole("heading", { name: "运行总览" })).toBeVisible();
    const main = screen.container.querySelector<HTMLElement>("[data-testid='run-overview-main']");
    const shellHeader = screen.container.querySelector<HTMLElement>("[data-testid='run-overview-shell-header']");
    const shellHeaderActions = screen.container.querySelector<HTMLElement>("[data-testid='run-overview-shell-header-actions']");
    expect(main).not.toBeNull();
    expect(shellHeader).not.toBeNull();
    expect(shellHeaderActions).toBeNull();
    expect(main?.contains(shellHeader)).toBe(false);
    await expect.element(screen.getByRole("button", { name: "1层" })).toBeVisible();
    expect(main?.querySelector("[data-runtime-overview-toolbar]")).not.toBeNull();
    expect(main?.querySelector("button[aria-label='刷新运行总览']")).not.toBeNull();
    await expect.element(screen.getByRole("button", { name: "拖动画布" })).not.toBeInTheDocument();
    await expect.element(screen.getByRole("button", { name: "缩小" })).not.toBeInTheDocument();
    await expect.element(screen.getByRole("button", { name: "放大" })).not.toBeInTheDocument();
    await expect.element(screen.getByRole("button", { name: "适应视图" })).not.toBeInTheDocument();
  });

  it("switches floors without refetching layout data", async () => {
    const { fetcher, requests } = createFetcher();
    const screen = await renderPage(fetcher);

    await expect.element(screen.getByRole("button", { name: "1层" })).toBeVisible();
    await userEvent.click(screen.getByRole("button", { name: "2层" }));
    await expect.element(screen.getByText("当前楼层：2层")).toBeVisible();
    expect(requests.filter((request) => request.pathname === "/api/v1/digital-employees/overview").length).toBe(1);
  });

  it("renders office-zone capacity workspaces and selectable employee avatars", async () => {
    const { fetcher } = createFetcher();
    const screen = await renderPage(fetcher);

    await expect.element(screen.getByLabelText("运行总览地图画布")).toBeVisible();
    expect(screen.container.querySelectorAll("[data-runtime-seat='team-dev']").length).toBe(3);
    expect(screen.container.querySelectorAll("[data-runtime-seat='team-ops']").length).toBe(4);
    expect(screen.container.textContent).not.toMatch(/\+\d+\s*空闲/);
    expect(screen.container.querySelectorAll("[data-runtime-team-callout='team-dev']").length).toBe(1);
    expect(screen.container.querySelectorAll("button[data-employee-id] > span.absolute.right-1.bottom-1.size-3.rounded-full.border-2.border-white").length).toBe(0);
    const calloutLink = screen.container.querySelector("[data-runtime-team-callout-link='team-dev']");
    expect(calloutLink).not.toBeNull();
    expect(calloutLink?.getAttribute("stroke")).toBe("#6482A3");

    await userEvent.click(screen.getByRole("button", { name: /高秀英/ }));
    await expect.element(screen.getByText("当前选择：高秀英")).not.toBeInTheDocument();
    await expect.element(screen.getByRole("heading", { name: "高秀英" })).toBeVisible();
    expect(screen.container.querySelectorAll("[data-runtime-team-selection-frame]").length).toBe(0);
  });

  it("keeps the office map unframed and feathered into the page background", async () => {
    const { fetcher } = createFetcher();
    const screen = await renderPage(fetcher);

    await expect.element(screen.getByLabelText("运行总览地图画布")).toBeVisible();
    const mapStage = screen.container.querySelector<HTMLElement>("[aria-label='运行总览地图画布']");
    const mapScene = screen.container.querySelector<HTMLElement>("[data-runtime-map-scene]");
    const mapFeather = screen.container.querySelector<HTMLElement>("[data-runtime-map-feather]");
    const mapBackground = screen.container.querySelector<HTMLElement>("[data-runtime-map-background]");
    expect(mapStage).not.toBeNull();
    expect(mapScene).not.toBeNull();
    expect(mapFeather).not.toBeNull();
    expect(mapBackground).not.toBeNull();
    expect(mapStage?.className).not.toContain("border-[var(--v3-shell-glass-border)]");
    expect(mapStage?.className).not.toContain("bg-[var(--v3-shell-glass)]");
    expect(mapStage?.className).not.toContain("p-1");
    expect(mapStage?.className).not.toContain("shadow-v3");
    expect(mapScene?.style.clipPath).not.toContain("polygon(");
    expect(
      `${mapScene?.style.maskImage ?? ""} ${mapScene?.style.getPropertyValue("-webkit-mask-image")}`,
    ).toContain("linear-gradient");
    expect(
      `${mapFeather?.style.maskImage ?? ""} ${mapFeather?.style.getPropertyValue("-webkit-mask-image")}`,
    ).toContain("linear-gradient");
    expect(mapBackground?.className).toContain("mix-blend-multiply");

    await userEvent.click(screen.getByRole("button", { name: "2层" }));
    expect(screen.container.querySelector<HTMLElement>("[data-runtime-map-scene]")?.style.clipPath).not.toContain("polygon(");
    expect(screen.container.querySelector<HTMLElement>("[data-runtime-map-feather]")?.style.maskImage).toContain("linear-gradient");
    await userEvent.click(screen.getByRole("button", { name: "3层" }));
    expect(screen.container.querySelector<HTMLElement>("[data-runtime-map-scene]")?.style.clipPath).not.toContain("polygon(");
    expect(screen.container.querySelector<HTMLElement>("[data-runtime-map-feather]")?.style.maskImage).toContain("linear-gradient");
  });

  it("renders connector guide lines without center node circles", async () => {
    const { fetcher } = createFetcher();
    const screen = await renderPage(fetcher);

    await expect.element(screen.getByLabelText("运行总览地图画布")).toBeVisible();
    expect(screen.container.querySelectorAll("[aria-label='运行总览地图画布'] svg circle").length).toBe(0);
  });

  it("shows selected employee details without a table view fallback", async () => {
    const { fetcher } = createFetcher();
    const screen = await renderPage(fetcher);

    await userEvent.click(screen.getByRole("button", { name: /高秀英/ }));
    await expect.element(screen.getByRole("heading", { name: "高秀英" })).toBeVisible();
    await expect.element(screen.getByText("排查线上告警并生成修复计划")).toBeVisible();
    await expect.element(screen.getByRole("button", { name: "表格视图" })).not.toBeInTheDocument();
    await expect.element(screen.getByRole("table", { name: "运行总览表格" })).not.toBeInTheDocument();
  });

  it("only shows daily cumulative usage in the selected employee consumption panel", async () => {
    const { fetcher } = createFetcher();
    const screen = await renderPage(fetcher, { employee: "emp-dev-1" });

    await expect.element(screen.getByText("消耗情况")).toBeVisible();
    await expect.element(screen.getByText("今日累计")).toBeVisible();
    await expect.element(screen.getByText("本任务消耗")).not.toBeInTheDocument();
  });

  it("deep-links to employees and runtime nodes from the selected employee panel", async () => {
    const { fetcher } = createFetcher();
    const screen = await renderPage(fetcher, { employee: "emp-dev-1" });

    await expect.element(screen.getByText("当前选择：陆一鸣")).not.toBeInTheDocument();
    await expect.element(screen.getByRole("heading", { name: "陆一鸣" })).toBeVisible();
    await expect.element(screen.getByRole("link", { name: "查看员工详情" })).toHaveAttribute(
      "href",
      "/employees/emp-dev-1",
    );
    await expect.element(screen.getByRole("link", { name: "查看 Runtime 节点" })).toHaveAttribute(
      "href",
      "/runtime?node=local-dev-node",
    );
  });

  it("lets employee query changes override the previous local selection", async () => {
    const { fetcher } = createFetcher();
    const screen = await renderPage(fetcher, { employee: "emp-dev-1" });

    await expect.element(screen.getByText("当前选择：陆一鸣")).not.toBeInTheDocument();
    await expect.element(screen.getByRole("heading", { name: "陆一鸣" })).toBeVisible();

    routerSearch = { employee: "emp-ops-1" };
    await screen.rerender(
      <QueryClientProvider client={queryClient()}>
        <RunOverviewView apiBaseUrl="http://control-plane.local" fetcher={fetcher} />
      </QueryClientProvider>,
    );

    await expect.element(screen.getByText("当前选择：高秀英")).not.toBeInTheDocument();
    await expect.element(screen.getByRole("heading", { name: "高秀英" })).toBeVisible();
  });
});
