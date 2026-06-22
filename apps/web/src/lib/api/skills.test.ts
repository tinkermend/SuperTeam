import { describe, expect, it, vi } from "vitest";
import {
  bindEmployeeSkill,
  bindTeamSkill,
  deleteSkill,
  listEmployeeSkills,
  listSkills,
  listTeamSkills,
  unbindEmployeeSkill,
  unbindTeamSkill,
  uploadSkill,
  type Skill,
} from "./skills";

function makeSkill(overrides: Partial<Skill> = {}): Skill {
  return {
    id: "skill-1",
    tenant_id: "tenant-1",
    slug: "diagnose",
    name: "diagnose",
    description: "诊断流程",
    version: "v1.0.0",
    source: "upload",
    risk_level: "low",
    icon_key: "stethoscope",
    color_token: "cyan",
    tags: ["诊断", "测试"],
    archive_object_ref: "s3://bucket/skills/diagnose.zip",
    archive_filename: "diagnose.zip",
    archive_size_bytes: 1024,
    archive_checksum_sha256: "abc123def456",
    archive_file_count: 2,
    created_by: "user-1",
    created_by_name: "开发管理员",
    team_bindings: [],
    agent_bindings: [],
    ...overrides,
  };
}

describe("skills API", () => {
  it("lists skills with archive metadata and agent bindings", async () => {
    const skills = [
      {
        id: "skill-1",
        tenant_id: "tenant-1",
        slug: "diagnose",
        name: "diagnose",
        description: "诊断流程",
        version: "v1.0.0",
        source: "upload",
        risk_level: "low",
        icon_key: "stethoscope",
        color_token: "cyan",
        tags: ["诊断", "测试"],
        archive_object_ref: "s3://bucket/skills/diagnose.zip",
        archive_filename: "diagnose.zip",
        archive_size_bytes: 1024,
        archive_checksum_sha256: "abc123def456",
        archive_file_count: 2,
        created_by: "user-1",
        created_by_name: "开发管理员",
        team_bindings: [{ team_id: "team-1", team_name: "平台工程" }],
        agent_bindings: [{ agent_id: "agent-1", agent_name: "需求澄清 Agent", team_name: "产品团队", status: "enabled" }],
      },
    ] satisfies Skill[];
    const fetcher = vi.fn(async () => new Response(JSON.stringify(skills), { headers: { "content-type": "application/json" } }));

    await expect(listSkills({ baseUrl: "http://control-plane.local", fetcher }, { q: "dia" })).resolves.toEqual(skills);

    expect(fetcher).toHaveBeenCalledWith(
      "http://control-plane.local/api/v1/skills?q=dia",
      {
        credentials: "include",
        headers: { accept: "application/json" },
        method: "GET",
      },
    );
  });

  it("uploads a zip with description tags and team bindings", async () => {
    const skill = {
      id: "skill-uploaded",
      tenant_id: "tenant-1",
      slug: "custom-diagnose",
      name: "custom-diagnose",
      description: "自定义诊断",
      version: "v0.1.0",
      source: "upload",
      risk_level: "medium",
      icon_key: "blocks",
      color_token: "teal",
      tags: ["诊断"],
      archive_object_ref: "s3://bucket/skills/custom-diagnose.zip",
      archive_filename: "skill.zip",
      archive_size_bytes: 2048,
      archive_checksum_sha256: "def456abc789",
      archive_file_count: 1,
      created_by: "user-1",
      created_by_name: "开发管理员",
      team_bindings: [],
      agent_bindings: [],
    } satisfies Skill;
    const fetcher = vi.fn(async (_input: RequestInfo | URL, init?: RequestInit) => {
      expect(init?.body).toBeInstanceOf(FormData);
      const formData = init?.body as FormData;
      expect(formData.get("name")).toBe("custom-diagnose");
      expect(formData.get("description")).toBe("自定义诊断");
      expect(formData.get("tags")).toBe("诊断,自动化");
      expect(formData.get("risk_level")).toBe("medium");
      expect(formData.get("file")).toBeInstanceOf(File);
      return new Response(JSON.stringify(skill), { headers: { "content-type": "application/json" }, status: 201 });
    });

    await expect(
      uploadSkill(
        { baseUrl: "http://control-plane.local", fetcher },
        {
          description: "自定义诊断",
          file: new File(["zip"], "skill.zip", { type: "application/zip" }),
          name: "custom-diagnose",
          tags: ["诊断", "自动化"],
          risk_level: "medium",
        },
      ),
    ).resolves.toEqual(skill);

    expect(fetcher).toHaveBeenCalledWith(
      "http://control-plane.local/api/v1/skills/uploads",
      expect.objectContaining({
        credentials: "include",
        method: "POST",
      }),
    );
  });

  it("omits blank optional upload fields so the backend can derive SKILL.md metadata", async () => {
    const skill = makeSkill({
      archive_filename: "release-review.zip",
      description: "检查发布计划、回滚策略和验收证据。",
      name: "Release Review",
      slug: "release-review",
    });
    const fetcher = vi.fn(async (_input: RequestInfo | URL, init?: RequestInit) => {
      expect(init?.body).toBeInstanceOf(FormData);
      const formData = init?.body as FormData;
      expect(formData.has("name")).toBe(false);
      expect(formData.has("description")).toBe(false);
      expect(formData.has("runtime_tools")).toBe(false);
      expect(formData.has("runtime_env")).toBe(false);
      expect(formData.get("file")).toBeInstanceOf(File);
      return new Response(JSON.stringify(skill), { headers: { "content-type": "application/json" }, status: 201 });
    });

    await expect(
      uploadSkill(
        { baseUrl: "http://control-plane.local", fetcher },
        {
          description: " ",
          file: new File(["zip"], "release-review.zip", { type: "application/zip" }),
          name: " ",
          runtime_dependencies: { env: [], tools: [] },
          tags: [],
        },
      ),
    ).resolves.toEqual(skill);
  });

  it("deletes a skill by id", async () => {
    const fetcher = vi.fn(async () => new Response(null, { status: 204 }));
    await expect(deleteSkill({ baseUrl: "http://control-plane.local", fetcher }, "skill 1/ops")).resolves.toBeUndefined();
    expect(fetcher).toHaveBeenCalledWith(
      "http://control-plane.local/api/v1/skills/skill%201%2Fops",
      expect.objectContaining({ credentials: "include", method: "DELETE" }),
    );
  });

  it("lists binds and unbinds team skills with encoded path segments", async () => {
    const skill = makeSkill({ id: "skill 1/ops" });
    const fetcher = vi.fn(async (_input: RequestInfo | URL, init?: RequestInit) => {
      if (init?.method === "DELETE") {
        return new Response(null, { status: 204 });
      }
      if (init?.method === "POST") {
        expect(JSON.parse(String(init.body))).toEqual({ skill_id: "skill 1/ops" });
        return new Response(JSON.stringify(skill), {
          headers: { "content-type": "application/json" },
          status: 201,
        });
      }
      return new Response(JSON.stringify([skill]), {
        headers: { "content-type": "application/json" },
      });
    });

    await expect(
      listTeamSkills(
        { baseUrl: "http://control-plane.local", fetcher },
        "team 1/ops",
      ),
    ).resolves.toEqual([skill]);
    await expect(
      bindTeamSkill(
        { baseUrl: "http://control-plane.local", fetcher },
        "team 1/ops",
        "skill 1/ops",
      ),
    ).resolves.toEqual(skill);
    await expect(
      unbindTeamSkill(
        { baseUrl: "http://control-plane.local", fetcher },
        "team 1/ops",
        "skill 1/ops",
      ),
    ).resolves.toBeUndefined();

    expect(fetcher).toHaveBeenNthCalledWith(
      1,
      "http://control-plane.local/api/v1/teams/team%201%2Fops/skills",
      {
        credentials: "include",
        headers: { accept: "application/json" },
        method: "GET",
      },
    );
    expect(fetcher).toHaveBeenNthCalledWith(
      2,
      "http://control-plane.local/api/v1/teams/team%201%2Fops/skills",
      {
        body: JSON.stringify({ skill_id: "skill 1/ops" }),
        credentials: "include",
        headers: { accept: "application/json", "content-type": "application/json" },
        method: "POST",
      },
    );
    expect(fetcher).toHaveBeenNthCalledWith(
      3,
      "http://control-plane.local/api/v1/teams/team%201%2Fops/skills/skill%201%2Fops",
      {
        credentials: "include",
        method: "DELETE",
      },
    );
  });

  it("lists binds and unbinds employee skills including inherited read only shape", async () => {
    const skill = makeSkill({ id: "skill 1/ops" });
    const effectiveSkill = {
      skill,
      source_scope: "team",
      inherited: true,
      read_only: true,
    } as const;
    const fetcher = vi.fn(async (_input: RequestInfo | URL, init?: RequestInit) => {
      if (init?.method === "DELETE") {
        return new Response(null, { status: 204 });
      }
      if (init?.method === "POST") {
        expect(JSON.parse(String(init.body))).toEqual({ skill_id: "skill 1/ops" });
        return new Response(JSON.stringify(skill), {
          headers: { "content-type": "application/json" },
          status: 201,
        });
      }
      return new Response(JSON.stringify([effectiveSkill]), {
        headers: { "content-type": "application/json" },
      });
    });

    await expect(
      listEmployeeSkills(
        { baseUrl: "http://control-plane.local", fetcher },
        "employee 1/primary",
      ),
    ).resolves.toEqual([effectiveSkill]);
    await expect(
      bindEmployeeSkill(
        { baseUrl: "http://control-plane.local", fetcher },
        "employee 1/primary",
        "skill 1/ops",
      ),
    ).resolves.toEqual(skill);
    await expect(
      unbindEmployeeSkill(
        { baseUrl: "http://control-plane.local", fetcher },
        "employee 1/primary",
        "skill 1/ops",
      ),
    ).resolves.toBeUndefined();

    expect(fetcher).toHaveBeenNthCalledWith(
      1,
      "http://control-plane.local/api/v1/digital-employees/employee%201%2Fprimary/skills",
      {
        credentials: "include",
        headers: { accept: "application/json" },
        method: "GET",
      },
    );
    expect(fetcher).toHaveBeenNthCalledWith(
      2,
      "http://control-plane.local/api/v1/digital-employees/employee%201%2Fprimary/skills",
      {
        body: JSON.stringify({ skill_id: "skill 1/ops" }),
        credentials: "include",
        headers: { accept: "application/json", "content-type": "application/json" },
        method: "POST",
      },
    );
    expect(fetcher).toHaveBeenNthCalledWith(
      3,
      "http://control-plane.local/api/v1/digital-employees/employee%201%2Fprimary/skills/skill%201%2Fops",
      {
        credentials: "include",
        method: "DELETE",
      },
    );
  });
});
