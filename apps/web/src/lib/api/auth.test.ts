import { describe, expect, it, vi } from "vitest";
import {
  ApiRequestError,
  createUser,
  getLoginCaptcha,
  getCurrentUser,
  listCurrentUserLoginLogs,
  listLoginLogs,
  listUserProjectTeamScopes,
  listUsers,
  login,
  logout,
  replaceUserProjectTeamScopes,
  resetUserPassword,
  updateCurrentUserPassword,
  updateCurrentUserProfile,
  updateUserStatus,
} from "./auth";

const userId = "22222222-2222-4222-8222-222222222222";
const operatorUserId = "22222222-2222-4222-8222-222222222223";
const loginLogId = "44444444-4444-4444-8444-444444444444";
const sessionId = "33333333-3333-4333-8333-333333333333";

function jsonResponse(body: unknown) {
  return new Response(JSON.stringify(body), {
    headers: {
      "content-type": "application/json",
    },
    status: 200,
  });
}

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
          captcha_id: "11111111-1111-4111-8111-111111111111",
          captcha_code: "A7K2",
        },
      ),
    ).resolves.toMatchObject({
      user: {
        username: "admin",
      },
    });

    expect(fetcher).toHaveBeenCalledWith("http://control-plane.local/api/auth/login", {
      body: JSON.stringify({
        username: "admin",
        password: "admin",
        captcha_id: "11111111-1111-4111-8111-111111111111",
        captcha_code: "A7K2",
      }),
      credentials: "include",
      headers: {
        accept: "application/json",
        "content-type": "application/json",
      },
      method: "POST",
    });
  });

  it("loads a login captcha challenge with cookie credentials", async () => {
    const fetcher = vi.fn(async () =>
      jsonResponse({
        enabled: true,
        captcha_id: "11111111-1111-4111-8111-111111111111",
        image_data_url: "data:image/png;base64,abc123",
        expires_at: "2026-06-30T08:00:00Z",
      }),
    );

    await expect(
      getLoginCaptcha({
        baseUrl: "http://control-plane.local/",
        fetcher,
      }),
    ).resolves.toEqual({
      enabled: true,
      captcha_id: "11111111-1111-4111-8111-111111111111",
      image_data_url: "data:image/png;base64,abc123",
      expires_at: "2026-06-30T08:00:00Z",
    });

    expect(fetcher).toHaveBeenCalledWith("http://control-plane.local/api/auth/captcha", {
      credentials: "include",
      headers: {
        accept: "application/json",
      },
      method: "GET",
    });
  });

  it("loads a disabled login captcha state", async () => {
    const fetcher = vi.fn(async () =>
      jsonResponse({
        enabled: false,
      }),
    );

    await expect(
      getLoginCaptcha({
        baseUrl: "http://control-plane.local/",
        fetcher,
      }),
    ).resolves.toEqual({
      enabled: false,
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

  it("loads current user login logs with pagination and cookie credentials", async () => {
    const fetcher = vi.fn(async () =>
      jsonResponse({
        items: [
          {
            id: loginLogId,
            event_type: "login_succeeded",
            user_id: operatorUserId,
            username: "operator",
            session_id: null,
            client_ip: "127.0.0.1",
            user_agent: "Chrome",
            result: "succeeded",
            created_at: "2026-06-12T08:00:00Z",
          },
        ],
      }),
    );

    const response = await listCurrentUserLoginLogs({
      baseUrl: "http://control-plane.local",
      fetcher,
      limit: 5,
      offset: 10,
    });

    expect(response.items).toHaveLength(1);
    expect(fetcher).toHaveBeenCalledWith(
      "http://control-plane.local/api/auth/account/login-logs?limit=5&offset=10",
      {
        credentials: "include",
        headers: {
          accept: "application/json",
        },
        method: "GET",
      },
    );
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
              avatar: {
                provider: "dicebear",
                style: "adventurer",
                seed: "admin-avatar",
              },
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
      items: [{ username: "admin", status: "active", avatar: { provider: "dicebear", style: "adventurer", seed: "admin-avatar" } }],
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

  it("manages users with cookie credentials", async () => {
    const fetcher = vi.fn(async () =>
      new Response(
        JSON.stringify({
          user: {
            id: operatorUserId,
            username: "operator",
            status: "active",
            avatar: {
              provider: "dicebear",
              style: "adventurer",
              seed: "operator-avatar",
            },
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

    await updateUserStatus({ baseUrl: "http://control-plane.local/", fetcher }, operatorUserId, "disabled");
    await resetUserPassword({ baseUrl: "http://control-plane.local/", fetcher }, operatorUserId, "new-secret");

    expect(fetcher).toHaveBeenNthCalledWith(1, `http://control-plane.local/api/auth/users/${operatorUserId}/status`, {
      body: JSON.stringify({ status: "disabled" }),
      credentials: "include",
      headers: {
        accept: "application/json",
        "content-type": "application/json",
      },
      method: "PATCH",
    });
    expect(fetcher).toHaveBeenNthCalledWith(2, `http://control-plane.local/api/auth/users/${operatorUserId}/reset-password`, {
      body: JSON.stringify({ password: "new-secret" }),
      credentials: "include",
      headers: {
        accept: "application/json",
        "content-type": "application/json",
      },
      method: "POST",
    });
  });

  it("creates users with human avatar config and selectable teams unchanged", async () => {
    const teamId = "55555555-5555-4555-8555-555555555555";
    const avatar = {
      provider: "dicebear" as const,
      style: "adventurer" as const,
      seed: "user:zhoumin",
    };
    const fetcher = vi.fn(async () =>
      jsonResponse({
        user: {
          id: operatorUserId,
          username: "operator",
          display_name: "值班负责人",
          email: null,
          avatar_asset_id: null,
          status: "active",
          avatar,
        },
      }),
    );

    await expect(
      createUser(
        { baseUrl: "http://control-plane.local", fetcher },
        {
          username: "operator",
          display_name: "值班负责人",
          password: "secret",
          avatar,
          tenant_role: "member",
          selectable_team_ids: [teamId],
        },
      ),
    ).resolves.toMatchObject({
      user: {
        avatar,
        avatar_asset_id: null,
        display_name: "值班负责人",
        email: null,
      },
    });

    expect(fetcher).toHaveBeenCalledWith("http://control-plane.local/api/auth/users", {
      body: JSON.stringify({
        username: "operator",
        display_name: "值班负责人",
        password: "secret",
        avatar,
        tenant_role: "member",
        selectable_team_ids: [teamId],
      }),
      credentials: "include",
      headers: {
        accept: "application/json",
        "content-type": "application/json",
      },
      method: "POST",
    });
  });

  it("lists user project team scopes with cookie credentials", async () => {
    const scopedUserID = "user/id with space";
    const teamId = "55555555-5555-4555-8555-555555555555";
    const fetcher = vi.fn(async () =>
      jsonResponse({
        items: [
          {
            id: "66666666-6666-4666-8666-666666666666",
            tenant_id: "77777777-7777-4777-8777-777777777777",
            user_id: userId,
            team_id: teamId,
            status: "active",
            granted_by_user_id: null,
            revoked_at: null,
            created_at: "2026-06-17T02:00:00Z",
            updated_at: "2026-06-17T02:30:00Z",
            team: {
              digital_employee_count: 3,
              governance_status: "ready",
              human_owners: [
                {
                  id: operatorUserId,
                  username: "operator",
                  display_name: null,
                  email: null,
                  status: "active",
                  avatar: {
                    provider: "dicebear",
                    style: "adventurer",
                    seed: "operator-avatar",
                  },
                  avatar_asset_id: null,
                },
              ],
              id: teamId,
              name: "交付团队",
              pending_draft_count: 1,
              risk_summary: "low",
              slug: "delivery",
              status: "active",
            },
          },
        ],
      }),
    );

    await expect(
      listUserProjectTeamScopes(
        {
          baseUrl: "http://control-plane.local/",
          fetcher,
        },
        scopedUserID,
      ),
    ).resolves.toMatchObject({
      items: [
        {
          id: "66666666-6666-4666-8666-666666666666",
          tenant_id: "77777777-7777-4777-8777-777777777777",
          user_id: userId,
          team_id: teamId,
          status: "active",
          granted_by_user_id: null,
          revoked_at: null,
          created_at: "2026-06-17T02:00:00Z",
          updated_at: "2026-06-17T02:30:00Z",
          team: {
            digital_employee_count: 3,
            governance_status: "ready",
            human_owners: [
              {
                id: operatorUserId,
                username: "operator",
                display_name: null,
                email: null,
                status: "active",
                avatar: {
                  provider: "dicebear",
                  style: "adventurer",
                  seed: "operator-avatar",
                },
                avatar_asset_id: null,
              },
            ],
            id: teamId,
            name: "交付团队",
            pending_draft_count: 1,
            risk_summary: "low",
            slug: "delivery",
            status: "active",
          },
        },
      ],
    });

    expect(fetcher).toHaveBeenCalledWith(
      "http://control-plane.local/api/auth/users/user%2Fid%20with%20space/project-team-scopes",
      {
        credentials: "include",
        headers: {
          accept: "application/json",
        },
        method: "GET",
      },
    );
  });

  it("replaces user project team scopes with cookie credentials", async () => {
    const scopedUserID = "user/id with space";
    const teamId = "55555555-5555-4555-8555-555555555555";
    const fetcher = vi.fn(async () =>
      jsonResponse({
        items: [
          {
            id: "66666666-6666-4666-8666-666666666666",
            tenant_id: "77777777-7777-4777-8777-777777777777",
            user_id: userId,
            team_id: teamId,
            status: "active",
            granted_by_user_id: null,
            revoked_at: null,
            created_at: "2026-06-17T02:00:00Z",
            updated_at: "2026-06-17T02:30:00Z",
            team: {
              digital_employee_count: 3,
              governance_status: "ready",
              human_owners: [
                {
                  id: operatorUserId,
                  username: "operator",
                  display_name: null,
                  email: null,
                  status: "active",
                  avatar: {
                    provider: "dicebear",
                    style: "adventurer",
                    seed: "operator-avatar",
                  },
                  avatar_asset_id: null,
                },
              ],
              id: teamId,
              name: "交付团队",
              pending_draft_count: 1,
              risk_summary: "low",
              slug: "delivery",
              status: "active",
            },
          },
        ],
      }),
    );

    await expect(
      replaceUserProjectTeamScopes(
        {
          baseUrl: "http://control-plane.local/",
          fetcher,
        },
        scopedUserID,
        {
          team_ids: [teamId],
        },
      ),
    ).resolves.toMatchObject({
      items: [
        {
          granted_by_user_id: null,
          revoked_at: null,
          team_id: teamId,
          team: {
            name: "交付团队",
          },
        },
      ],
    });

    expect(fetcher).toHaveBeenCalledWith(
      "http://control-plane.local/api/auth/users/user%2Fid%20with%20space/project-team-scopes",
      {
        body: JSON.stringify({ team_ids: [teamId] }),
        credentials: "include",
        headers: {
          accept: "application/json",
          "content-type": "application/json",
        },
        method: "PUT",
      },
    );
  });

  it("updates current user profile and password with cookie credentials", async () => {
    const fetcher = vi.fn(async () =>
      jsonResponse({
        user: {
          id: operatorUserId,
          username: "operator",
          display_name: "值班负责人",
          email: "operator@example.com",
          status: "active",
          avatar: {
            provider: "dicebear",
            style: "adventurer",
            seed: "operator-v2",
          },
        },
      }),
    );

    await updateCurrentUserProfile(
      { baseUrl: "http://control-plane.local", fetcher },
      {
        avatar: {
          provider: "dicebear",
          seed: "operator-v2",
          style: "adventurer",
        },
        display_name: "值班负责人",
        email: "operator@example.com",
      },
    );
    await updateCurrentUserPassword(
      { baseUrl: "http://control-plane.local", fetcher },
      {
        current_password: "old-secret",
        password: "new-secret",
      },
    );

    expect(fetcher).toHaveBeenNthCalledWith(1, "http://control-plane.local/api/auth/account/profile", {
      body: JSON.stringify({
        avatar: {
          provider: "dicebear",
          seed: "operator-v2",
          style: "adventurer",
        },
        display_name: "值班负责人",
        email: "operator@example.com",
      }),
      credentials: "include",
      headers: {
        accept: "application/json",
        "content-type": "application/json",
      },
      method: "PATCH",
    });
    expect(fetcher).toHaveBeenNthCalledWith(2, "http://control-plane.local/api/auth/account/password", {
      body: JSON.stringify({
        current_password: "old-secret",
        password: "new-secret",
      }),
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
          captcha_id: "11111111-1111-4111-8111-111111111111",
          captcha_code: "A7K2",
        },
      ),
    ).rejects.toThrow("auth login request failed with status 401");

    expect(fetcher).toHaveBeenCalledWith("http://control-plane.local/api/auth/login", {
      body: JSON.stringify({
        username: "admin",
        password: "wrong",
        captcha_id: "11111111-1111-4111-8111-111111111111",
        captcha_code: "A7K2",
      }),
      credentials: "include",
      headers: {
        accept: "application/json",
        "content-type": "application/json",
      },
      method: "POST",
    });
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
