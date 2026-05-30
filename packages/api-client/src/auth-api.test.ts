import { describe, expect, it, vi } from "vitest";
import { getCurrentUser, listLoginLogs, login, logout } from "./auth-api";

describe("auth api client", () => {
  it("posts login credentials with cookie credentials and parses the user", async () => {
    const fetcher = vi.fn(async () =>
      new Response(
        JSON.stringify({
          user: {
            id: "00000000-0000-0000-0000-000000000001",
            username: "admin",
            status: "active",
          },
        }),
        {
          status: 200,
          headers: {
            "content-type": "application/json",
          },
        },
      ),
    );

    await expect(
      login(
        {
          baseUrl: "http://control-plane.local/",
          fetcher,
        },
        {
          username: "admin",
          password: "admin",
        },
      ),
    ).resolves.toMatchObject({
      user: {
        username: "admin",
      },
    });

    expect(fetcher).toHaveBeenCalledWith("http://control-plane.local/api/auth/login", {
      body: JSON.stringify({ username: "admin", password: "admin" }),
      credentials: "include",
      headers: {
        accept: "application/json",
        "content-type": "application/json",
      },
      method: "POST",
    });
  });

  it("loads the current user with cookie credentials", async () => {
    const fetcher = vi.fn(async () =>
      new Response(
        JSON.stringify({
          user: {
            id: "00000000-0000-0000-0000-000000000001",
            username: "admin",
            status: "active",
          },
        }),
        {
          status: 200,
          headers: {
            "content-type": "application/json",
          },
        },
      ),
    );

    await expect(
      getCurrentUser({
        baseUrl: "http://control-plane.local/",
        fetcher,
      }),
    ).resolves.toMatchObject({
      user: {
        username: "admin",
      },
    });

    expect(fetcher).toHaveBeenCalledWith("http://control-plane.local/api/auth/me", {
      credentials: "include",
      headers: {
        accept: "application/json",
      },
      method: "GET",
    });
  });

  it("posts logout with cookie credentials", async () => {
    const fetcher = vi.fn(async () =>
      new Response(JSON.stringify({ message: "logout success" }), {
        status: 200,
        headers: {
          "content-type": "application/json",
        },
      }),
    );

    await expect(
      logout({
        baseUrl: "http://control-plane.local/",
        fetcher,
      }),
    ).resolves.toBeUndefined();

    expect(fetcher).toHaveBeenCalledWith("http://control-plane.local/api/auth/logout", {
      credentials: "include",
      headers: {
        accept: "application/json",
      },
      method: "POST",
    });
  });

  it("loads login logs with pagination and cookie credentials", async () => {
    const fetcher = vi.fn(async () =>
      new Response(
        JSON.stringify({
          items: [
            {
              id: 1,
              event_type: "login_succeeded",
              user_id: 1,
              username: "admin",
              session_id: "session-1",
              client_ip: "127.0.0.1",
              user_agent: "test-agent",
              result: "succeeded",
              failure_reason: null,
              created_at: "2026-05-31T02:00:00Z",
            },
          ],
        }),
        {
          status: 200,
          headers: {
            "content-type": "application/json",
          },
        },
      ),
    );

    await expect(
      listLoginLogs({
        baseUrl: "http://control-plane.local/",
        fetcher,
        limit: 10,
        offset: 5,
      }),
    ).resolves.toMatchObject({
      items: [
        {
          event_type: "login_succeeded",
          username: "admin",
          result: "succeeded",
        },
      ],
    });

    expect(fetcher).toHaveBeenCalledWith("http://control-plane.local/api/auth/login-logs?limit=10&offset=5", {
      credentials: "include",
      headers: {
        accept: "application/json",
      },
      method: "GET",
    });
  });

  it("throws a useful error when login is rejected", async () => {
    const fetcher = vi.fn(async () =>
      new Response(JSON.stringify({ error: "unauthorized" }), {
        status: 401,
        headers: {
          "content-type": "application/json",
        },
      }),
    );

    await expect(
      login(
        {
          baseUrl: "http://control-plane.local/",
          fetcher,
        },
        {
          username: "admin",
          password: "wrong",
        },
      ),
    ).rejects.toThrow("auth login request failed with status 401");
  });
});
