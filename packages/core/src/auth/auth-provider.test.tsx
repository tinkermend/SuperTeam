// @vitest-environment jsdom
import "@testing-library/jest-dom/vitest";
import { act, render, screen, waitFor } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import { AuthProvider } from "./auth-provider";
import { useAuth } from "./use-auth";

function AuthProbe() {
  const { isAuthenticated, isLoading, user } = useAuth();

  return (
    <div>
      <span data-testid="loading">{String(isLoading)}</span>
      <span data-testid="authenticated">{String(isAuthenticated)}</span>
      <span data-testid="username">{user?.username ?? "none"}</span>
    </div>
  );
}

describe("AuthProvider", () => {
  it("clears the current user when a session check receives unauthorized", async () => {
    const fetcher = vi
      .fn()
      .mockResolvedValueOnce(
        new Response(
          JSON.stringify({
            user: {
              id: 1,
              status: "active",
              username: "admin",
            },
          }),
          { headers: { "content-type": "application/json" }, status: 200 },
        ),
      )
      .mockResolvedValueOnce(new Response(JSON.stringify({ error: "unauthorized" }), { status: 401 }));

    render(
      <AuthProvider apiBaseUrl="http://localhost:8080" fetcher={fetcher}>
        <AuthProbe />
      </AuthProvider>,
    );

    await screen.findByText("admin");

    await act(async () => {
      window.dispatchEvent(new Event("focus"));
    });

    await waitFor(() => {
      expect(screen.getByTestId("authenticated")).toHaveTextContent("false");
    });
    expect(screen.getByTestId("username")).toHaveTextContent("none");
  });
});
