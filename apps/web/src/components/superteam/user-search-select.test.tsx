import type { ReactNode } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { describe, expect, it, vi } from "vitest";
import { render } from "vitest-browser-react";
import { userEvent } from "vitest/browser";
import type { UserSummary } from "@/lib/api";
import { UserSearchSelect } from "./user-search-select";

function createQueryClient() {
  return new QueryClient({
    defaultOptions: {
      queries: {
        retry: false,
      },
    },
  });
}

async function renderWithQueryClient(children: ReactNode) {
  return await render(<QueryClientProvider client={createQueryClient()}>{children}</QueryClientProvider>);
}

function jsonResponse(body: unknown) {
  return new Response(JSON.stringify(body), {
    headers: { "content-type": "application/json" },
    status: 200,
  });
}

const selectedUser: UserSummary = {
  avatar: { provider: "dicebear", seed: "selected-user", style: "adventurer" },
  id: "user-current",
  status: "active",
  username: "selected",
};

describe("UserSearchSelect", () => {
  it("searches active users and selects a visible result", async () => {
    const onSelect = vi.fn();
    const visibleUser: UserSummary = {
      avatar: { provider: "dicebear", seed: "visible-user", style: "adventurer" },
      id: "user-visible",
      status: "active",
      username: "zhoumin",
    };
    const excludedUser: UserSummary = {
      avatar: { provider: "dicebear", seed: "excluded-user", style: "adventurer" },
      id: "user-excluded",
      status: "active",
      username: "excluded",
    };
    const fetcher = vi.fn(async () => jsonResponse({ items: [visibleUser, excludedUser] }));

    const screen = await renderWithQueryClient(
      <UserSearchSelect
        apiBaseUrl="https://control-plane.example"
        excludedUserIds={["user-excluded"]}
        fetcher={fetcher}
        onSelect={onSelect}
        value={selectedUser}
      />,
    );

    await expect.element(screen.getByText("selected")).toBeInTheDocument();
    await userEvent.fill(screen.getByLabelText("搜索用户"), "zhou");

    await expect.element(screen.getByText("zhoumin")).toBeInTheDocument();
    await expect.element(screen.getByText("excluded")).not.toBeInTheDocument();
    expect(fetcher).toHaveBeenLastCalledWith(
      "https://control-plane.example/api/auth/users?q=zhou&status=active&limit=20&offset=0",
      expect.any(Object),
    );

    await userEvent.click(screen.getByRole("button", { name: "选择 zhoumin" }));

    expect(onSelect).toHaveBeenCalledWith(visibleUser);
  });
});
