import { forwardRef, type AnchorHTMLAttributes, type ReactNode } from "react";
import { userEvent } from "vitest/browser";
import { describe, expect, it, vi } from "vitest";
import { render } from "vitest-browser-react";
import { AuthContext } from "@/features/auth/auth-context";
import { SidebarProvider } from "@/components/ui/sidebar";
import { Header } from "./header";
import "@/styles/index.css";

vi.mock("@/context/search-provider", () => ({
  useSearch: () => ({
    setOpen: vi.fn()
})
}));

vi.mock("@/context/theme-provider", () => ({
  useTheme: () => ({
    setTheme: vi.fn(),
    theme: "system"
})
}));

vi.mock("@/components/sign-out-dialog", () => ({
  SignOutDialog: () => null
}));

vi.mock("@tanstack/react-router", () => {
  type MockLinkProps = AnchorHTMLAttributes<HTMLAnchorElement> & {
    children: ReactNode;
    to: string;
  };
  const Link = forwardRef<HTMLAnchorElement, MockLinkProps>(
    ({ children, to, ...props }, ref) => (
      <a {...props} data-router-link="true" href={to} ref={ref}>
        {children}
      </a>
    ),
  );
  Link.displayName = "MockRouterLink";

  return { Link };
});

describe("Header", () => {
  it("uses the global v3 shell header controls", async () => {
    const screen = await render(
      <AuthContext
        value={{
          apiBaseUrl: "http://control-plane.local",
          isAuthenticated: true,
          isLoading: false,
          login: vi.fn(),
          logout: vi.fn(),
          refreshCurrentUser: vi.fn(),
          user: {
            display_name: "林 Anna",
            email: "anna@example.com",
            id: "user-1",
            status: "active",
            username: "anna",
            avatar: {
              provider: "dicebear",
              seed: "anna",
              style: "adventurer"
}
}
}}
      >
        <SidebarProvider>
          <Header>
            <button type="button">旧页面动作</button>
          </Header>
        </SidebarProvider>
      </AuthContext>,
    );

    const header = screen.getByRole("banner").element() as HTMLElement;
    const trigger = document.querySelector('[data-slot="sidebar-trigger"]');
    const headerStyle = getComputedStyle(header);
    const search = screen.getByRole("button", {
      name: /搜索任务、数字员工、能力、文档或快捷命令/
}).element() as HTMLElement;

    expect(header.dataset.slot).toBe("shell-header");
    expect(header.dataset.variant).toBe("global");
    expect(headerStyle.backgroundColor).toBe("rgba(0, 0, 0, 0)");
    expect(headerStyle.backdropFilter).toBe("none");
    expect(header.className).toContain("h-14");
    expect(trigger).toBeInstanceOf(HTMLElement);
    expect((trigger as HTMLElement).className).toContain("--shell-control");
    expect((trigger as HTMLElement).className).toContain("text-ink-2");
    expect(search.className).toContain("mx-auto");
    expect(search.className).toContain("rounded-full");
    expect(search.className).toContain("--shell-search");
    expect(search.className).toContain("--shell-search-border");
    expect(search.className).toContain("[box-shadow:var(--shell-search-shadow)]");
    expect(header.firstElementChild?.className).toContain(
      "grid-cols-[auto_minmax(0,1fr)_auto]",
    );
    expect(header.firstElementChild?.className).toContain(
      "lg:grid-cols-[minmax(14rem,1fr)_minmax(16rem,42rem)_minmax(14rem,1fr)]",
    );
    expect(screen.getByRole("button", { name: "切换主题" })).toBeTruthy();
    expect(screen.getByRole("button", { name: "通知" }).element().className).toContain(
      "shrink-0",
    );
    expect(
      screen.getByRole("button", { name: /用户信息：林 Anna/ }).element().className,
    ).toContain("shrink-0");
    expect(document.body.textContent).not.toContain("旧页面动作");
  });

  it("can replace the sidebar trigger with page-level header content", async () => {
    const screen = await render(
      <AuthContext
        value={{
          apiBaseUrl: "http://control-plane.local",
          isAuthenticated: true,
          isLoading: false,
          login: vi.fn(),
          logout: vi.fn(),
          refreshCurrentUser: vi.fn(),
          user: {
            display_name: "林 Anna",
            email: "anna@example.com",
            id: "user-1",
            status: "active",
            username: "anna",
            avatar: {
              provider: "dicebear",
              seed: "anna",
              style: "adventurer"
}
}
}}
      >
        <SidebarProvider>
          <Header showSidebarTrigger={false}>
            <div data-testid="page-title-slot">流程编排</div>
          </Header>
        </SidebarProvider>
      </AuthContext>,
    );

    const titleSlot = screen.getByTestId("page-title-slot").element();
    const search = screen.getByRole("button", {
      name: /搜索任务、数字员工、能力、文档或快捷命令/
}).element() as HTMLElement;

    expect(titleSlot).toBeInstanceOf(HTMLElement);
    expect(document.querySelector('[data-slot="sidebar-trigger"]')).toBeNull();
    expect(search.className).toContain("justify-self-center");
    expect(search.className).toContain("max-w-sm");
  });

  it("opens lightweight top-right controls without blocking neighboring clicks", async () => {
    const screen = await render(
      <AuthContext
        value={{
          apiBaseUrl: "http://control-plane.local",
          isAuthenticated: true,
          isLoading: false,
          login: vi.fn(),
          logout: vi.fn(),
          refreshCurrentUser: vi.fn(),
          user: {
            display_name: "林 Anna",
            email: "anna@example.com",
            id: "user-1",
            status: "active",
            username: "anna",
            avatar: {
              provider: "dicebear",
              seed: "anna",
              style: "adventurer"
}
}
}}
      >
        <SidebarProvider>
          <Header />
        </SidebarProvider>
      </AuthContext>,
    );

    await userEvent.click(screen.getByRole("button", { name: "切换主题" }));
    await expect.element(screen.getByRole("menuitem", { name: /跟随系统/ })).toBeVisible();
    expect(document.body.style.pointerEvents).not.toBe("none");

    await userEvent.click(screen.getByRole("button", { name: "通知" }));
    await expect.element(screen.getByText("暂无新通知")).toBeVisible();
    expect(document.body.style.pointerEvents).not.toBe("none");

    await userEvent.click(screen.getByRole("button", { name: /用户信息：林 Anna/ }));
    await expect.element(screen.getByRole("menuitem", { name: "账户" })).toBeVisible();
    expect(document.body.style.pointerEvents).not.toBe("none");
  });
});
