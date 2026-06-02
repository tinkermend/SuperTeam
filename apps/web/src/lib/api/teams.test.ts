import { describe, expect, it, vi } from "vitest";
import {
  createTeam,
  createTeamConfigRevision,
  getCurrentTeamConfigRevision,
  listTeams,
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
