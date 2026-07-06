import type { ReactNode } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { describe, expect, it, vi } from "vitest";
import { render } from "vitest-browser-react";
import { userEvent } from "vitest/browser";

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

async function renderPage(fetcher: typeof fetch) {
  return await render(
    <QueryClientProvider client={queryClient()}>
      <RunOverviewView apiBaseUrl="http://control-plane.local" fetcher={fetcher} />
    </QueryClientProvider>,
  );
}

describe("RunOverviewView", () => {
  it("renders the runtime overview map from existing Control Plane read APIs", async () => {
    const { fetcher, requests } = createFetcher();
    const screen = await renderPage(fetcher);

    await expect.element(screen.getByRole("heading", { name: "运行总览" })).toBeVisible();
    await expect.element(screen.getByText("开发团队").first()).toBeVisible();
    await expect.element(screen.getByText("运维团队").first()).toBeVisible();
    await expect.element(screen.getByText("高秀英").first()).toBeVisible();
    await expect.element(screen.getByText("容量 3/10").first()).toBeVisible();
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
  });

  it("switches floors without refetching layout data", async () => {
    const { fetcher, requests } = createFetcher();
    const screen = await renderPage(fetcher);

    await expect.element(screen.getByRole("button", { name: "1层" })).toBeVisible();
    await userEvent.click(screen.getByRole("button", { name: "2层" }));
    await expect.element(screen.getByText("当前楼层：2层")).toBeVisible();
    expect(requests.filter((request) => request.pathname === "/api/v1/digital-employees/overview").length).toBe(1);
  });

  it("renders fixed ten-seat team workspaces and selectable employee avatars", async () => {
    const { fetcher } = createFetcher();
    const screen = await renderPage(fetcher);

    await expect.element(screen.getByLabelText("运行总览地图画布")).toBeVisible();
    expect(screen.container.querySelectorAll("[data-runtime-seat='team-dev']").length).toBe(10);
    expect(screen.container.querySelectorAll("[data-runtime-seat='team-ops']").length).toBe(10);

    await userEvent.click(screen.getByRole("button", { name: /高秀英/ }));
    await expect.element(screen.getByText("当前选择：高秀英")).toBeVisible();
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
});
