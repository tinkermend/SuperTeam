import { forwardRef, type AnchorHTMLAttributes, type ReactNode } from "react";
import { ArrowLeft, GitBranch } from "lucide-react";
import { describe, expect, it, vi } from "vitest";
import { render } from "vitest-browser-react";
import { AuthContext } from "@/features/auth/auth-context";
import { SidebarProvider } from "@/components/ui/sidebar";
import { ShellPageHeader, ShellPageHeaderBack } from "./shell-page-header";
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

function renderInShell(children: ReactNode) {
  return render(
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
      <SidebarProvider>{children}</SidebarProvider>
    </AuthContext>,
  );
}

describe("ShellPageHeader", () => {
  it("renders the page header inside the global shell header", async () => {
    const screen = await renderInShell(
      <ShellPageHeader
        icon={<GitBranch />}
        iconTone="info"
        title="流程编排"
        subtitle="查看需求触发的规划、执行、阻塞和结果状态"
      />,
    );

    const shellHeader = document.body.querySelector('[data-slot="shell-header"]');
    const pageHeader = document.body.querySelector('[data-slot="page-header"]');
    const heading = screen.getByRole("heading", { name: "流程编排" }).element();
    const search = screen.getByRole("button", {
      name: /搜索任务、数字员工、能力、文档或快捷命令/
}).element() as HTMLElement;

    expect(shellHeader).toBeInstanceOf(HTMLElement);
    expect(pageHeader).toBeInstanceOf(HTMLElement);
    expect(pageHeader?.getAttribute("data-variant")).toBe("shell");
    expect(shellHeader?.contains(heading)).toBe(true);
    expect(document.querySelector('[data-slot="sidebar-trigger"]')).toBeNull();
    expect(search.className).toContain("justify-self-center");
    expect(search.className).toContain("max-w-sm");
  });

  it("uses a shared icon-only back link", async () => {
    const screen = await renderInShell(
      <ShellPageHeader
        back={<ShellPageHeaderBack ariaLabel="返回项目管理" to="/projects" />}
        title="项目详情"
      />,
    );

    const backLink = screen.getByRole("link", { name: "返回项目管理" }).element() as HTMLElement;

    expect(backLink.getAttribute("href")).toBe("/projects");
    expect(backLink.getAttribute("data-router-link")).toBe("true");
    expect(backLink.className).toContain("h-10");
    expect(backLink.className).toContain("w-10");
    expect(backLink.className).toContain("border-line");
    expect(backLink.querySelector("svg")).toBeInstanceOf(SVGElement);
  });

  it("allows replacing the default back icon while keeping the shared control style", async () => {
    const screen = await renderInShell(
      <ShellPageHeader
        back={
          <ShellPageHeaderBack ariaLabel="返回流程编排" to="/workflows">
            <ArrowLeft data-testid="custom-back-icon" />
          </ShellPageHeaderBack>
        }
        title="流程实例"
      />,
    );

    const backLink = screen.getByRole("link", { name: "返回流程编排" }).element();

    expect(backLink.querySelector('[data-testid="custom-back-icon"]')).toBeInstanceOf(
      SVGElement,
    );
  });
});
