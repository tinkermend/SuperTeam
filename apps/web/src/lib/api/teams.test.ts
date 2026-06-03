import { describe, expect, it, vi } from "vitest";
import {
  addTeamMember,
  approveTeamGovernanceDraft,
  approveTeamMemberRoleRequest,
  archiveTeam,
  createTeam,
  createTeamConfigRevision,
  createTeamGovernanceDraft,
  createTeamMemberRoleRequest,
  disableTeam,
  getCurrentTeamConfigRevision,
  getCurrentTeamGovernance,
  getTeamOverview,
  listTeamAuditEvents,
  listTeamGovernanceDrafts,
  listTeamMemberRoleRequests,
  listTeamMembers,
  listTeamSummaries,
  listTeams,
  previewTeamGovernanceDiff,
  rejectTeamGovernanceDraft,
  rejectTeamMemberRoleRequest,
  removeTeamMember,
  restoreTeam,
  updateTeam,
  updateTeamGovernanceDraft,
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
    const fetcher = vi.fn(
      async () =>
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

    expect(fetcher).toHaveBeenCalledWith(
      "http://control-plane.local/api/v1/teams",
      {
        credentials: "include",
        headers: { accept: "application/json" },
        method: "GET",
      },
    );
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
    const fetcher = vi.fn(
      async () =>
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
        { governance_status: "draft_pending", q: "ops", status: "active" },
      ),
    ).resolves.toEqual(teams);

    expect(fetcher).toHaveBeenCalledWith(
      "http://control-plane.local/api/v1/teams?status=active&governance_status=draft_pending&q=ops",
      {
        credentials: "include",
        headers: { accept: "application/json" },
        method: "GET",
      },
    );
  });

  it("lists team summaries with weak pagination filters", async () => {
    const fetcher = vi.fn(
      async () =>
        new Response(JSON.stringify([]), {
          headers: { "content-type": "application/json" },
          status: 200,
        }),
    );

    await listTeamSummaries(
      { baseUrl: "http://control-plane.local", fetcher },
      { limit: 20, offset: 40, q: "ops", status: "active" },
    );

    expect(fetcher).toHaveBeenCalledWith(
      "http://control-plane.local/api/v1/teams?status=active&q=ops&limit=20&offset=40",
      {
        credentials: "include",
        headers: { accept: "application/json" },
        method: "GET",
      },
    );
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
    const fetcher = vi.fn(
      async () =>
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
    const fetcher = vi.fn(
      async () =>
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

    expect(fetcher).toHaveBeenCalledWith(
      "http://control-plane.local/api/v1/teams/team%201%2Fprimary",
      {
        body: JSON.stringify({
          slug: "ops",
          name: "运维团队",
          human_owner_user_id: "33333333-3333-4333-8333-333333333333",
        }),
        credentials: "include",
        headers: {
          accept: "application/json",
          "content-type": "application/json",
        },
        method: "PATCH",
      },
    );
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
    const fetcher = vi.fn(
      async () =>
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

    expect(fetcher).toHaveBeenCalledWith(
      `http://control-plane.local/api/v1/teams/team%201%2Fprimary${suffix}`,
      {
        body: JSON.stringify({}),
        credentials: "include",
        headers: {
          accept: "application/json",
          "content-type": "application/json",
        },
        method: "POST",
      },
    );
  });

  it("creates team with owner and initial members and parses overview", async () => {
    const overview = {
      team: {
        id: "11111111-1111-4111-8111-111111111111",
        tenant_id: "22222222-2222-4222-8222-222222222222",
        slug: "security",
        name: "安全团队",
        status: "active",
        human_owner_user_id: "33333333-3333-4333-8333-333333333333",
        human_owner: {
          user_id: "33333333-3333-4333-8333-333333333333",
          username: "owner",
          display_name: "负责人",
          email: "owner@example.com",
          status: "active",
        },
      },
      member_count: 3,
      digital_employee_count: 0,
      capability_count: 0,
      pending_draft_count: 0,
      pending_item_count: 0,
      allowed_actions: ["team.update"],
    };
    const fetcher = vi.fn(
      async () =>
        new Response(JSON.stringify(overview), {
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
          slug: "security",
          name: "安全团队",
          human_owner_user_id: "33333333-3333-4333-8333-333333333333",
          initial_members: [
            { user_id: "44444444-4444-4444-8444-444444444444", role: "member" },
            { user_id: "55555555-5555-4555-8555-555555555555", role: "viewer" },
          ],
        },
      ),
    ).resolves.toEqual(overview);

    expect(fetcher).toHaveBeenCalledWith(
      "http://control-plane.local/api/v1/teams",
      {
        body: JSON.stringify({
          slug: "security",
          name: "安全团队",
          human_owner_user_id: "33333333-3333-4333-8333-333333333333",
          initial_members: [
            { user_id: "44444444-4444-4444-8444-444444444444", role: "member" },
            { user_id: "55555555-5555-4555-8555-555555555555", role: "viewer" },
          ],
        }),
        credentials: "include",
        headers: {
          accept: "application/json",
          "content-type": "application/json",
        },
        method: "POST",
      },
    );
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
    const fetcher = vi.fn(
      async () =>
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
        headers: {
          accept: "application/json",
          "content-type": "application/json",
        },
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
    const fetcher = vi.fn(
      async () =>
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

  it("calls governance draft endpoints with encoded ids", async () => {
    const revision = {
      id: "44444444-4444-4444-8444-444444444444",
      tenant_id: "22222222-2222-4222-8222-222222222222",
      team_id: "11111111-1111-4111-8111-111111111111",
      revision_number: 2,
      constitution: { hard_rules: ["提交前必须审批"] },
      capability_policy: { skill_bindings: ["incident-diagnosis"] },
      context_policy: {},
      approval_policy: {},
      artifact_contract: {},
      internal_collaboration_policy: {},
      runtime_scope_policy: {},
      human_owner_user_id: "33333333-3333-4333-8333-333333333333",
      status: "draft",
    };
    const diff = {
      added_hard_rules: 1,
      changed_approval_rules: 1,
      changed_capabilities: 1,
      blocking_errors: [],
      warnings: [
        {
          field: "constitution.hard_rules",
          message: "新增硬性规则需要复核",
          severity: "warning",
        },
      ],
    };
    const fetcher = vi.fn(
      async (input: RequestInfo | URL, init?: RequestInit) => {
        const url = new URL(String(input));
        if (
          url.pathname.endsWith("/governance/drafts") &&
          (init?.method ?? "GET") === "GET"
        ) {
          return new Response(JSON.stringify([revision]), {
            headers: { "content-type": "application/json" },
            status: 200,
          });
        }
        if (
          url.pathname.endsWith("/diff") &&
          (init?.method ?? "GET") === "GET"
        ) {
          return new Response(JSON.stringify(diff), {
            headers: { "content-type": "application/json" },
            status: 200,
          });
        }
        return new Response(JSON.stringify(revision), {
          headers: { "content-type": "application/json" },
          status:
            init?.method === "POST" &&
            url.pathname.endsWith("/governance/drafts")
              ? 201
              : 200,
        });
      },
    ) as unknown as typeof fetch;
    const options = { baseUrl: "http://control-plane.local", fetcher };

    await expect(
      getCurrentTeamGovernance(options, "team 1/primary"),
    ).resolves.toEqual(revision);
    await expect(
      listTeamGovernanceDrafts(options, "team 1/primary"),
    ).resolves.toEqual([revision]);
    await expect(
      createTeamGovernanceDraft(options, "team 1/primary", {
        constitution: { hard_rules: ["提交前必须审批"] },
        human_owner_user_id: "33333333-3333-4333-8333-333333333333",
      }),
    ).resolves.toEqual(revision);
    await expect(
      updateTeamGovernanceDraft(options, "team 1/primary", "draft 1/primary", {
        capability_policy: { mcp_bindings: ["ops-mcp-server"] },
      }),
    ).resolves.toEqual(revision);
    await expect(
      approveTeamGovernanceDraft(options, "team 1/primary", "draft 1/primary"),
    ).resolves.toEqual(revision);
    await expect(
      rejectTeamGovernanceDraft(options, "team 1/primary", "draft 1/primary"),
    ).resolves.toEqual(revision);
    await expect(
      previewTeamGovernanceDiff(options, "team 1/primary", "draft 1/primary"),
    ).resolves.toEqual(diff);

    expect(fetcher).toHaveBeenCalledWith(
      "http://control-plane.local/api/v1/teams/team%201%2Fprimary/governance/current",
      {
        credentials: "include",
        headers: { accept: "application/json" },
        method: "GET",
      },
    );
    expect(fetcher).toHaveBeenCalledWith(
      "http://control-plane.local/api/v1/teams/team%201%2Fprimary/governance/drafts",
      {
        credentials: "include",
        headers: { accept: "application/json" },
        method: "GET",
      },
    );
    expect(fetcher).toHaveBeenCalledWith(
      "http://control-plane.local/api/v1/teams/team%201%2Fprimary/governance/drafts",
      expect.objectContaining({
        body: JSON.stringify({
          constitution: { hard_rules: ["提交前必须审批"] },
          human_owner_user_id: "33333333-3333-4333-8333-333333333333",
        }),
        credentials: "include",
        method: "POST",
      }),
    );
    expect(fetcher).toHaveBeenCalledWith(
      "http://control-plane.local/api/v1/teams/team%201%2Fprimary/governance/drafts/draft%201%2Fprimary",
      expect.objectContaining({
        body: JSON.stringify({
          capability_policy: { mcp_bindings: ["ops-mcp-server"] },
        }),
        credentials: "include",
        method: "PATCH",
      }),
    );
    expect(fetcher).toHaveBeenCalledWith(
      "http://control-plane.local/api/v1/teams/team%201%2Fprimary/governance/drafts/draft%201%2Fprimary/approve",
      expect.objectContaining({ credentials: "include", method: "POST" }),
    );
    expect(fetcher).toHaveBeenCalledWith(
      "http://control-plane.local/api/v1/teams/team%201%2Fprimary/governance/drafts/draft%201%2Fprimary/reject",
      expect.objectContaining({ credentials: "include", method: "POST" }),
    );
    expect(fetcher).toHaveBeenCalledWith(
      "http://control-plane.local/api/v1/teams/team%201%2Fprimary/governance/drafts/draft%201%2Fprimary/diff",
      {
        credentials: "include",
        headers: { accept: "application/json" },
        method: "GET",
      },
    );
  });

  it("manages team members with encoded team and member ids", async () => {
    const members = [
      {
        membership_id: "membership 1/primary",
        tenant_id: "22222222-2222-4222-8222-222222222222",
        team_id: "11111111-1111-4111-8111-111111111111",
        user_id: "33333333-3333-4333-8333-333333333333",
        username: "operator",
        display_name: "值班同学",
        email: "operator@example.com",
        account_status: "active",
        role: "member",
        membership_status: "active",
      },
    ];
    const fetcher = vi.fn(
      async (_input: RequestInfo | URL, init?: RequestInit) => {
        if (init?.method === "DELETE") {
          return new Response(null, { status: 204 });
        }
        return new Response(
          JSON.stringify(init?.method === "POST" ? members[0] : members),
          {
            headers: { "content-type": "application/json" },
            status: init?.method === "POST" ? 201 : 200,
          },
        );
      },
    ) as unknown as typeof fetch;

    await expect(
      listTeamMembers(
        { baseUrl: "http://control-plane.local", fetcher },
        "team 1/primary",
      ),
    ).resolves.toEqual(members);
    await expect(
      addTeamMember(
        {
          baseUrl: "http://control-plane.local",
          fetcher,
        },
        "team 1/primary",
        { role: "member", user_id: "33333333-3333-4333-8333-333333333333" },
      ),
    ).resolves.toEqual(members[0]);
    await expect(
      removeTeamMember(
        { baseUrl: "http://control-plane.local", fetcher },
        "team 1/primary",
        "membership 1/primary",
      ),
    ).resolves.toBeUndefined();

    expect(fetcher).toHaveBeenCalledWith(
      "http://control-plane.local/api/v1/teams/team%201%2Fprimary/members",
      {
        credentials: "include",
        headers: { accept: "application/json" },
        method: "GET",
      },
    );
    expect(fetcher).toHaveBeenCalledWith(
      "http://control-plane.local/api/v1/teams/team%201%2Fprimary/members",
      {
        body: JSON.stringify({
          role: "member",
          user_id: "33333333-3333-4333-8333-333333333333",
        }),
        credentials: "include",
        headers: {
          accept: "application/json",
          "content-type": "application/json",
        },
        method: "POST",
      },
    );
    expect(fetcher).toHaveBeenCalledWith(
      "http://control-plane.local/api/v1/teams/team%201%2Fprimary/members/membership%201%2Fprimary",
      {
        credentials: "include",
        headers: { accept: "application/json" },
        method: "DELETE",
      },
    );
  });

  it("lists team audit events with pagination filters", async () => {
    const events = [
      {
        id: "55555555-5555-4555-8555-555555555555",
        tenant_id: "22222222-2222-4222-8222-222222222222",
        event_type: "team_management",
        actor_type: "user",
        actor_id: "33333333-3333-4333-8333-333333333333",
        resource_type: "team",
        resource_id: "team-1",
        action: "team.create",
        details: { summary: "创建团队" },
        ip_address: "10.20.2.15",
        created_at: "2026-06-03T09:30:00Z",
      },
    ];
    const fetcher = vi.fn(
      async () =>
        new Response(JSON.stringify(events), {
          headers: { "content-type": "application/json" },
          status: 200,
        }),
    );

    await expect(
      listTeamAuditEvents(
        {
          baseUrl: "http://control-plane.local",
          fetcher,
        },
        "team-1",
        { limit: 20, offset: 0 },
      ),
    ).resolves.toEqual(events);

    expect(fetcher).toHaveBeenCalledWith(
      "http://control-plane.local/api/v1/teams/team-1/audit?limit=20&offset=0",
      {
        credentials: "include",
        headers: { accept: "application/json" },
        method: "GET",
      },
    );
  });

  it("manages privileged role requests", async () => {
    const roleRequest = {
      id: "request 1/primary",
      tenant_id: "22222222-2222-4222-8222-222222222222",
      team_id: "11111111-1111-4111-8111-111111111111",
      target_user_id: "33333333-3333-4333-8333-333333333333",
      requested_role: "admin",
      requested_by: "44444444-4444-4444-8444-444444444444",
      status: "pending",
      reason: "需要维护成员",
      decision_reason: "",
    };
    const fetcher = vi.fn(
      async (_input: RequestInfo | URL, init?: RequestInit) =>
        new Response(
          JSON.stringify(init?.method === "GET" ? [roleRequest] : roleRequest),
          {
            headers: { "content-type": "application/json" },
            status: init?.method === "POST" ? 201 : 200,
          },
        ),
    ) as unknown as typeof fetch;

    await expect(
      listTeamMemberRoleRequests(
        { baseUrl: "http://control-plane.local", fetcher },
        "team 1/primary",
        "pending",
      ),
    ).resolves.toEqual([roleRequest]);
    await expect(
      createTeamMemberRoleRequest(
        {
          baseUrl: "http://control-plane.local",
          fetcher,
        },
        "team 1/primary",
        {
          reason: "需要维护成员",
          requested_role: "admin",
          target_user_id: "33333333-3333-4333-8333-333333333333",
        },
      ),
    ).resolves.toEqual(roleRequest);
    await expect(
      approveTeamMemberRoleRequest(
        { baseUrl: "http://control-plane.local", fetcher },
        "team 1/primary",
        "request 1/primary",
        { decision_reason: "同意" },
      ),
    ).resolves.toEqual(roleRequest);
    await expect(
      rejectTeamMemberRoleRequest(
        { baseUrl: "http://control-plane.local", fetcher },
        "team 1/primary",
        "request 1/primary",
        { decision_reason: "权限过高" },
      ),
    ).resolves.toEqual(roleRequest);

    expect(fetcher).toHaveBeenCalledWith(
      "http://control-plane.local/api/v1/teams/team%201%2Fprimary/member-role-requests?status=pending",
      {
        credentials: "include",
        headers: { accept: "application/json" },
        method: "GET",
      },
    );
    expect(fetcher).toHaveBeenCalledWith(
      "http://control-plane.local/api/v1/teams/team%201%2Fprimary/member-role-requests",
      {
        body: JSON.stringify({
          reason: "需要维护成员",
          requested_role: "admin",
          target_user_id: "33333333-3333-4333-8333-333333333333",
        }),
        credentials: "include",
        headers: {
          accept: "application/json",
          "content-type": "application/json",
        },
        method: "POST",
      },
    );
    expect(fetcher).toHaveBeenCalledWith(
      "http://control-plane.local/api/v1/teams/team%201%2Fprimary/member-role-requests/request%201%2Fprimary/approve",
      {
        body: JSON.stringify({ decision_reason: "同意" }),
        credentials: "include",
        headers: {
          accept: "application/json",
          "content-type": "application/json",
        },
        method: "POST",
      },
    );
    expect(fetcher).toHaveBeenCalledWith(
      "http://control-plane.local/api/v1/teams/team%201%2Fprimary/member-role-requests/request%201%2Fprimary/reject",
      {
        body: JSON.stringify({ decision_reason: "权限过高" }),
        credentials: "include",
        headers: {
          accept: "application/json",
          "content-type": "application/json",
        },
        method: "POST",
      },
    );
  });
});
