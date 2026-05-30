import "@testing-library/jest-dom/vitest";
import { render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { AuthGate } from "./auth-gate";

const replace = vi.fn();
let authState = {
  isAuthenticated: false,
  isLoading: false,
};
let pathname = "/";

vi.mock("@superteam/core/auth", () => ({
  useAuth: () => authState,
}));

vi.mock("next/navigation", () => ({
  usePathname: () => pathname,
  useRouter: () => ({
    replace,
  }),
}));

describe("AuthGate", () => {
  beforeEach(() => {
    replace.mockReset();
    authState = {
      isAuthenticated: false,
      isLoading: false,
    };
    pathname = "/";
  });

  it("redirects unauthenticated users away from protected pages", () => {
    render(
      <AuthGate>
        <main>控制台</main>
      </AuthGate>,
    );

    expect(replace).toHaveBeenCalledWith("/login");
    expect(screen.queryByText("控制台")).not.toBeInTheDocument();
  });

  it("renders the login page when unauthenticated users are already on /login", () => {
    pathname = "/login";

    render(
      <AuthGate>
        <main>登录页</main>
      </AuthGate>,
    );

    expect(replace).not.toHaveBeenCalled();
    expect(screen.getByText("登录页")).toBeInTheDocument();
  });

  it("redirects authenticated users away from the login page", () => {
    pathname = "/login";
    authState = {
      isAuthenticated: true,
      isLoading: false,
    };

    render(
      <AuthGate>
        <main>登录页</main>
      </AuthGate>,
    );

    expect(replace).toHaveBeenCalledWith("/");
  });

  it("renders protected content when the current user is authenticated", () => {
    authState = {
      isAuthenticated: true,
      isLoading: false,
    };

    render(
      <AuthGate>
        <main>控制台</main>
      </AuthGate>,
    );

    expect(replace).not.toHaveBeenCalled();
    expect(screen.getByText("控制台")).toBeInTheDocument();
  });
});
