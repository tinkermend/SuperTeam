import type { ReactNode } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { describe, expect, it, vi } from "vitest";
import { render } from "vitest-browser-react";
import { Users } from "@/features/users";

vi.mock("@/components/layout/header", () => ({
  Header: ({ children }: { children: ReactNode }) => <header>{children}</header>,
}));

vi.mock("@/components/layout/main", () => ({
  Main: ({ children }: { children: ReactNode }) => <main>{children}</main>,
}));

vi.mock("@/components/search", () => ({
  Search: () => <button type="button">Search</button>,
}));

vi.mock("@/components/theme-switch", () => ({
  ThemeSwitch: () => <button type="button">Toggle theme</button>,
}));

function createQueryClient() {
  return new QueryClient({
    defaultOptions: {
      queries: {
        retry: false,
      },
    },
  });
}

function jsonResponse(body: unknown) {
  return new Response(JSON.stringify(body), {
    headers: { "content-type": "application/json" },
    status: 200,
  });
}

describe("Users", () => {
  it("renders user avatars from backend avatar config", async () => {
    const fetcher = vi.fn(async () =>
      jsonResponse({
        items: [
          {
            id: "user-1",
            username: "operator",
            status: "active",
            avatar: {
              provider: "dicebear",
              style: "adventurer",
              seed: "operator-avatar",
            },
          },
        ],
      }),
    );
    vi.stubGlobal("fetch", fetcher);

    const screen = await render(
      <QueryClientProvider client={createQueryClient()}>
        <Users />
      </QueryClientProvider>,
    );

    await expect.element(screen.getByText("operator")).toBeInTheDocument();
    const avatar = screen.getByAltText("operator 的头像");
    await expect.element(avatar).toBeInTheDocument();
    await expect.element(avatar).toHaveAttribute("src", expect.stringContaining("data:image/svg+xml"));
    expect(fetcher).toHaveBeenCalledWith(expect.stringContaining("/api/auth/users?limit=50&offset=0"), expect.any(Object));
  });
});
