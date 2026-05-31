import "@testing-library/jest-dom/vitest";
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

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
  afterEach(() => {
    cleanup();
  });

  beforeEach(() => {
    replace.mockReset();
    authState = {
      isAuthenticated: false,
      isLoading: false,
    };
    pathname = "/users";
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

  it("renders the external capability placeholder without requiring login", () => {
    pathname = "/capabilities";

    render(
      <AuthGate>
        <main>外部能力占位页</main>
      </AuthGate>,
    );

    expect(replace).not.toHaveBeenCalled();
    expect(screen.getByText("外部能力占位页")).toBeInTheDocument();
  });

  it("renders undeveloped top-level placeholder pages without requiring login", () => {
    for (const publicPathname of ["/", "/workspace", "/tasks", "/employees", "/workflows", "/approvals", "/audit"]) {
      cleanup();
      replace.mockReset();
      pathname = publicPathname;

      render(
        <AuthGate>
          <main>{publicPathname} 占位页</main>
        </AuthGate>,
      );

      expect(replace).not.toHaveBeenCalled();
      expect(screen.getByText(`${publicPathname} 占位页`)).toBeInTheDocument();
    }
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
    pathname = "/users";

    render(
      <AuthGate>
        <main>控制台</main>
      </AuthGate>,
    );

    expect(replace).not.toHaveBeenCalled();
    expect(screen.getByText("控制台")).toBeInTheDocument();
  });
});
