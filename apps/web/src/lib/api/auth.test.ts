import { describe, expect, it, vi } from "vitest";
import {
  ApiRequestError,
  createUser,
  getCurrentUser,
  listLoginLogs,
  listUsers,
  login,
  logout,
  resetUserPassword,
  updateUserStatus,
} from "./auth";

const userId = "22222222-2222-4222-8222-222222222222";
const operatorUserId = "22222222-2222-4222-8222-222222222223";
const loginLogId = "44444444-4444-4444-8444-444444444444";
const sessionId = "33333333-3333-4333-8333-333333333333";

describe("auth api client", () => {
  it("posts login credentials with cookie credentials and parses the user", async () => {
    const fetcher = vi.fn(async () =>
      new Response(
        JSON.stringify({
          user: {
            id: userId,
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
            id: userId,
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
              id: loginLogId,
              event_type: "login_succeeded",
              user_id: userId,
              username: "admin",
              session_id: sessionId,
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

  it("loads users with filters and cookie credentials", async () => {
    const fetcher = vi.fn(async () =>
      new Response(
        JSON.stringify({
          items: [
            {
              id: userId,
              username: "admin",
              status: "active",
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
      listUsers({
        baseUrl: "http://control-plane.local/",
        fetcher,
        limit: 20,
        offset: 0,
        status: "active",
      }),
    ).resolves.toMatchObject({
      items: [{ username: "admin", status: "active" }],
    });

    expect(fetcher).toHaveBeenCalledWith("http://control-plane.local/api/auth/users?status=active&limit=20&offset=0", {
      credentials: "include",
      headers: {
        accept: "application/json",
      },
      method: "GET",
    });
  });

  it("lists users with search query and active filter", async () => {
    const fetcher = vi.fn(async () =>
      new Response(JSON.stringify({ items: [] }), {
        headers: { "content-type": "application/json" },
        status: 200,
      }),
    );

    await listUsers({
      baseUrl: "http://control-plane.local",
      fetcher,
      limit: 20,
      offset: 0,
      q: "owner",
      status: "active",
    });

    expect(fetcher).toHaveBeenCalledWith(
      "http://control-plane.local/api/auth/users?q=owner&status=active&limit=20&offset=0",
      {
        credentials: "include",
        headers: { accept: "application/json" },
        method: "GET",
      },
    );
  });

  it("creates and manages users with cookie credentials", async () => {
    const fetcher = vi.fn(async () =>
      new Response(
        JSON.stringify({
          user: {
            id: operatorUserId,
            username: "operator",
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

    await createUser({ baseUrl: "http://control-plane.local/", fetcher }, { username: "operator", password: "secret" });
    await updateUserStatus({ baseUrl: "http://control-plane.local/", fetcher }, operatorUserId, "disabled");
    await resetUserPassword({ baseUrl: "http://control-plane.local/", fetcher }, operatorUserId, "new-secret");

    expect(fetcher).toHaveBeenNthCalledWith(1, "http://control-plane.local/api/auth/users", {
      body: JSON.stringify({ username: "operator", password: "secret" }),
      credentials: "include",
      headers: {
        accept: "application/json",
        "content-type": "application/json",
      },
      method: "POST",
    });
    expect(fetcher).toHaveBeenNthCalledWith(2, `http://control-plane.local/api/auth/users/${operatorUserId}/status`, {
      body: JSON.stringify({ status: "disabled" }),
      credentials: "include",
      headers: {
        accept: "application/json",
        "content-type": "application/json",
      },
      method: "PATCH",
    });
    expect(fetcher).toHaveBeenNthCalledWith(3, `http://control-plane.local/api/auth/users/${operatorUserId}/reset-password`, {
      body: JSON.stringify({ password: "new-secret" }),
      credentials: "include",
      headers: {
        accept: "application/json",
        "content-type": "application/json",
      },
      method: "POST",
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

  it("exposes the response status on rejected auth requests", async () => {
    const fetcher = vi.fn(async () => new Response(JSON.stringify({ error: "unauthorized" }), { status: 401 }));

    const promise =
      getCurrentUser({
        baseUrl: "http://control-plane.local/",
        fetcher,
      });

    await expect(promise).rejects.toMatchObject({
      name: "ApiRequestError",
      status: 401,
    });

    try {
      await promise;
    } catch (error) {
      expect(error).toBeInstanceOf(ApiRequestError);
    }
  });
});
