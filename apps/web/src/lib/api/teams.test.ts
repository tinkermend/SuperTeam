import { describe, expect, it, vi } from "vitest";
import {
  archiveTeam,
  createTeam,
  createTeamConfigRevision,
  disableTeam,
  getCurrentTeamConfigRevision,
  getTeamOverview,
  listTeamSummaries,
  listTeams,
  restoreTeam,
  updateTeam,
} from "./teams";

describe("team API", () => {
  it("lists teams with cookie credentials", async () => {
    const teams = [
      {
        id: "11111111-1111-4111-8111-111111111111",
        tenant_id: "22222222-2222-4222-8222-222222222222",
        slug: "platform",
        name: "平台团队",
        status: "active",
        human_owner_user_id: "33333333-3333-4333-8333-333333333333",
      },
    ];
    const fetcher = vi.fn(async () =>
      new Response(JSON.stringify(teams), {
        headers: { "content-type": "application/json" },
        status: 200,
      }),
    );

    await expect(
      listTeams({
        baseUrl: "http://control-plane.local",
        fetcher,
      }),
    ).resolves.toEqual(teams);

    expect(fetcher).toHaveBeenCalledWith("http://control-plane.local/api/v1/teams", {
      credentials: "include",
      headers: { accept: "application/json" },
      method: "GET",
    });
  });

  it("lists team summaries with filters", async () => {
    const teams = [
      {
        id: "11111111-1111-4111-8111-111111111111",
        tenant_id: "22222222-2222-4222-8222-222222222222",
        slug: "ops",
        name: "运维团队",
        status: "active",
        member_count: 18,
        digital_employee_count: 6,
        capability_count: 12,
        governance_status: "active",
        current_revision: 7,
        pending_draft_count: 2,
        risk_summary: "生产写操作需审批",
      },
    ];
    const fetcher = vi.fn(async () =>
      new Response(JSON.stringify(teams), {
        headers: { "content-type": "application/json" },
        status: 200,
      }),
    );

    await expect(
      listTeamSummaries(
        {
          baseUrl: "http://control-plane.local",
          fetcher,
        },
        { q: "ops", status: "active" },
      ),
    ).resolves.toEqual(teams);

    expect(fetcher).toHaveBeenCalledWith("http://control-plane.local/api/v1/teams?status=active&q=ops", {
      credentials: "include",
      headers: { accept: "application/json" },
      method: "GET",
    });
  });

  it("gets team overview with encoded team id", async () => {
    const overview = {
      team: {
        id: "11111111-1111-4111-8111-111111111111",
        tenant_id: "22222222-2222-4222-8222-222222222222",
        slug: "ops",
        name: "运维团队",
        status: "active",
      },
      member_count: 18,
      digital_employee_count: 6,
      capability_count: 12,
      pending_draft_count: 2,
      pending_item_count: 3,
      allowed_actions: ["team.update", "team.disable"],
    };
    const fetcher = vi.fn(async () =>
      new Response(JSON.stringify(overview), {
        headers: { "content-type": "application/json" },
        status: 200,
      }),
    );

    await expect(
      getTeamOverview(
        {
          baseUrl: "http://control-plane.local",
          fetcher,
        },
        "team 1/primary",
      ),
    ).resolves.toEqual(overview);

    expect(fetcher).toHaveBeenCalledWith(
      "http://control-plane.local/api/v1/teams/team%201%2Fprimary/overview",
      {
        credentials: "include",
        headers: { accept: "application/json" },
        method: "GET",
      },
    );
  });

  it("updates team with PATCH and JSON body", async () => {
    const team = {
      id: "11111111-1111-4111-8111-111111111111",
      tenant_id: "22222222-2222-4222-8222-222222222222",
      slug: "ops",
      name: "运维团队",
      status: "active",
      human_owner_user_id: "33333333-3333-4333-8333-333333333333",
    };
    const fetcher = vi.fn(async () =>
      new Response(JSON.stringify(team), {
        headers: { "content-type": "application/json" },
        status: 200,
      }),
    );

    await expect(
      updateTeam(
        {
          baseUrl: "http://control-plane.local",
          fetcher,
        },
        "team 1/primary",
        {
          slug: "ops",
          name: "运维团队",
          human_owner_user_id: "33333333-3333-4333-8333-333333333333",
        },
      ),
    ).resolves.toEqual(team);

    expect(fetcher).toHaveBeenCalledWith("http://control-plane.local/api/v1/teams/team%201%2Fprimary", {
      body: JSON.stringify({
        slug: "ops",
        name: "运维团队",
        human_owner_user_id: "33333333-3333-4333-8333-333333333333",
      }),
      credentials: "include",
      headers: { accept: "application/json", "content-type": "application/json" },
      method: "PATCH",
    });
  });

  it.each([
    ["disables", disableTeam, "/disable"],
    ["archives", archiveTeam, "/archive"],
    ["restores", restoreTeam, "/restore"],
  ] as const)("%s team with POST", async (_label, action, suffix) => {
    const team = {
      id: "11111111-1111-4111-8111-111111111111",
      tenant_id: "22222222-2222-4222-8222-222222222222",
      slug: "ops",
      name: "运维团队",
      status: "active",
    };
    const fetcher = vi.fn(async () =>
      new Response(JSON.stringify(team), {
        headers: { "content-type": "application/json" },
        status: 200,
      }),
    );

    await expect(
      action(
        {
          baseUrl: "http://control-plane.local",
          fetcher,
        },
        "team 1/primary",
      ),
    ).resolves.toEqual(team);

    expect(fetcher).toHaveBeenCalledWith(`http://control-plane.local/api/v1/teams/team%201%2Fprimary${suffix}`, {
      body: JSON.stringify({}),
      credentials: "include",
      headers: { accept: "application/json", "content-type": "application/json" },
      method: "POST",
    });
  });

  it("creates team with human owner user id", async () => {
    const team = {
      id: "11111111-1111-4111-8111-111111111111",
      tenant_id: "22222222-2222-4222-8222-222222222222",
      slug: "platform",
      name: "平台团队",
      status: "active",
      human_owner_user_id: "33333333-3333-4333-8333-333333333333",
    };
    const fetcher = vi.fn(async () =>
      new Response(JSON.stringify(team), {
        headers: { "content-type": "application/json" },
        status: 201,
      }),
    );

    await expect(
      createTeam(
        {
          baseUrl: "http://control-plane.local",
          fetcher,
        },
        {
          slug: "platform",
          name: "平台团队",
          human_owner_user_id: "33333333-3333-4333-8333-333333333333",
        },
      ),
    ).resolves.toEqual(team);

    expect(fetcher).toHaveBeenCalledWith("http://control-plane.local/api/v1/teams", {
      body: JSON.stringify({
        slug: "platform",
        name: "平台团队",
        human_owner_user_id: "33333333-3333-4333-8333-333333333333",
      }),
      credentials: "include",
      headers: { accept: "application/json", "content-type": "application/json" },
      method: "POST",
    });
  });

  it("creates team config revision with encoded team id and JSON body", async () => {
    const revision = {
      id: "44444444-4444-4444-8444-444444444444",
      tenant_id: "22222222-2222-4222-8222-222222222222",
      team_id: "11111111-1111-4111-8111-111111111111",
      revision_number: 1,
      constitution: { principle: "review" },
      capability_policy: { allow: ["incident-diagnosis"] },
      context_policy: {},
      approval_policy: {},
      artifact_contract: {},
      internal_collaboration_policy: {},
      runtime_scope_policy: {},
      human_owner_user_id: "33333333-3333-4333-8333-333333333333",
      status: "active",
    };
    const fetcher = vi.fn(async () =>
      new Response(JSON.stringify(revision), {
        headers: { "content-type": "application/json" },
        status: 201,
      }),
    );

    await expect(
      createTeamConfigRevision(
        {
          baseUrl: "http://control-plane.local",
          fetcher,
        },
        "team 1/primary",
        {
          constitution: { principle: "review" },
          capability_policy: { allow: ["incident-diagnosis"] },
          human_owner_user_id: "33333333-3333-4333-8333-333333333333",
          status: "active",
        },
      ),
    ).resolves.toEqual(revision);

    expect(fetcher).toHaveBeenCalledWith(
      "http://control-plane.local/api/v1/teams/team%201%2Fprimary/config-revisions",
      {
        body: JSON.stringify({
          constitution: { principle: "review" },
          capability_policy: { allow: ["incident-diagnosis"] },
          human_owner_user_id: "33333333-3333-4333-8333-333333333333",
          status: "active",
        }),
        credentials: "include",
        headers: { accept: "application/json", "content-type": "application/json" },
        method: "POST",
      },
    );
  });

  it("gets current team config revision with encoded team id and cookie credentials", async () => {
    const revision = {
      id: "44444444-4444-4444-8444-444444444444",
      tenant_id: "22222222-2222-4222-8222-222222222222",
      team_id: "11111111-1111-4111-8111-111111111111",
      revision_number: 1,
      constitution: {},
      capability_policy: {},
      context_policy: {},
      approval_policy: {},
      artifact_contract: {},
      internal_collaboration_policy: {},
      runtime_scope_policy: {},
      human_owner_user_id: "33333333-3333-4333-8333-333333333333",
      status: "active",
    };
    const fetcher = vi.fn(async () =>
      new Response(JSON.stringify(revision), {
        headers: { "content-type": "application/json" },
        status: 200,
      }),
    );

    await expect(
      getCurrentTeamConfigRevision(
        {
          baseUrl: "http://control-plane.local",
          fetcher,
        },
        "team 1/primary",
      ),
    ).resolves.toEqual(revision);

    expect(fetcher).toHaveBeenCalledWith(
      "http://control-plane.local/api/v1/teams/team%201%2Fprimary/config-revisions/current",
      {
        credentials: "include",
        headers: { accept: "application/json" },
        method: "GET",
      },
    );
  });
});
