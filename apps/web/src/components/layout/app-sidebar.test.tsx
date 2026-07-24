import type { ReactNode } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { render } from "vitest-browser-react";
import { ApiRequestError } from "@/lib/api/client";
import { AppSidebar } from "./app-sidebar";

const getInboxBadge = vi.fn();

vi.mock("@/lib/api/inbox", () => ({
  getInboxBadge: (...args: unknown[]) => getInboxBadge(...args),
}));

vi.mock("@/lib/config/control-plane-url", () => ({
  resolveControlPlaneUrl: () => "http://127.0.0.1:8080",
}));

vi.mock("@/features/inbox/use-inbox-stream-status", () => ({
  useInboxStreamStatus: () => ({ connection: "connected", lastSyncedAt: null }),
}));

vi.mock("@/features/inbox/inbox-stream-status", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/features/inbox/inbox-stream-status")>();
  return {
    ...actual,
    inboxBadgeRefetchInterval: () => false as const,
  };
});

vi.mock("@/components/ui/sidebar", () => ({
  Sidebar: ({ children }: { children?: ReactNode }) => <aside>{children}</aside>,
  SidebarContent: ({ children }: { children?: ReactNode }) => <div>{children}</div>,
  SidebarHeader: ({ children }: { children?: ReactNode }) => <div>{children}</div>,
  SidebarRail: () => null,
  useSidebar: () => ({ state: "expanded", isMobile: false, open: true, setOpen: () => {} }),
}));

vi.mock("@/context/layout-provider", () => ({
  useLayout: () => ({ collapsible: "icon", variant: "sidebar" }),
}));

vi.mock("./app-title", () => ({
  AppTitle: () => <div>title</div>,
}));

vi.mock("./nav-group", () => ({
  NavGroup: ({
    title,
    items,
  }: {
    title: string;
    items: Array<{ title: string; badge?: string }>;
  }) => (
    <div>
      <h2>{title}</h2>
      {items.map((item) => (
        <div key={item.title}>
          <span>{item.title}</span>
          {item.badge ? <span>{item.badge}</span> : null}
        </div>
      ))}
    </div>
  ),
}));

vi.mock("@tanstack/react-router", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@tanstack/react-router")>();
  return {
    ...actual,
    Link: ({
      children,
      ...props
    }: {
      children?: ReactNode;
      to?: string;
      className?: string;
    }) => (
      <a href={typeof props.to === "string" ? props.to : "#"} {...props}>
        {children}
      </a>
    ),
    useRouterState: () => ({ location: { pathname: "/inbox" } }),
  };
});

function createQueryClient() {
  return new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false },
    },
  });
}

describe("AppSidebar inbox badge", () => {
  beforeEach(() => {
    getInboxBadge.mockReset();
  });

  it("shows the open count when the badge request succeeds", async () => {
    getInboxBadge.mockResolvedValue({
      mine_open_count: 3,
      team_open_count: 0,
      high_risk_count: 1,
    });

    const screen = await render(
      <QueryClientProvider client={createQueryClient()}>
        <AppSidebar />
      </QueryClientProvider>,
    );

    await expect.element(screen.getByText("收件箱")).toBeVisible();
    await expect.element(screen.getByText("3")).toBeVisible();
  });

  it("does not paint a fake zero badge when the badge request returns 401", async () => {
    getInboxBadge.mockRejectedValue(
      new ApiRequestError("inbox badge", 401, "unauthorized"),
    );

    const screen = await render(
      <QueryClientProvider client={createQueryClient()}>
        <AppSidebar />
      </QueryClientProvider>,
    );

    await expect.element(screen.getByText("收件箱")).toBeVisible();
    // 旧实现 catch 后返回 mine_open_count:0，侧栏仍无数字；此处断言失败态不出现「0」徽标文案。
    expect(screen.getByText("0", { exact: true }).query()).toBeNull();
    await expect.poll(() => getInboxBadge.mock.calls.length).toBeGreaterThan(0);
  });
});
