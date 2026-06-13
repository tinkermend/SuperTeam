import { describe, expect, it, vi } from "vitest";
import {
  bindEmployeeSkill,
  bindTeamSkill,
  listEmployeeSkills,
  listSkills,
  listTeamSkills,
  updateSkillFile,
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
    source: "internal_market",
    risk_level: "low",
    status: "installed",
    icon_key: "stethoscope",
    color_token: "cyan",
    tags: ["诊断", "测试"],
    files: [],
    team_bindings: [],
    agent_bindings: [],
    ...overrides,
  };
}

describe("skills API", () => {
  it("lists skills with files and agent bindings", async () => {
    const skills = [
      {
        id: "skill-1",
        tenant_id: "tenant-1",
        slug: "diagnose",
        name: "diagnose",
        description: "诊断流程",
        version: "v1.0.0",
        source: "internal_market",
        risk_level: "low",
        status: "installed",
        icon_key: "stethoscope",
        color_token: "cyan",
        tags: ["诊断", "测试"],
        files: [{ path: "SKILL.md", file_type: "file", content: "# diagnose", size_bytes: 10 }],
        team_bindings: [{ team_id: "team-1", team_name: "平台工程" }],
        agent_bindings: [{ agent_id: "agent-1", agent_name: "需求澄清 Agent", team_name: "产品团队", status: "enabled" }],
      },
    ] satisfies Skill[];
    const fetcher = vi.fn(async () => new Response(JSON.stringify(skills), { headers: { "content-type": "application/json" } }));

    await expect(listSkills({ baseUrl: "http://control-plane.local", fetcher }, { status: "installed", q: "dia" })).resolves.toEqual(skills);

    expect(fetcher).toHaveBeenCalledWith(
      "http://control-plane.local/api/v1/skills?status=installed&q=dia",
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
      status: "installed",
      icon_key: "blocks",
      color_token: "teal",
      tags: ["诊断"],
      files: [],
      team_bindings: [],
      agent_bindings: [],
    } satisfies Skill;
    const fetcher = vi.fn(async (_input: RequestInfo | URL, init?: RequestInit) => {
      expect(init?.body).toBeInstanceOf(FormData);
      const formData = init?.body as FormData;
      expect(formData.get("name")).toBe("custom-diagnose");
      expect(formData.get("description")).toBe("自定义诊断");
      expect(formData.get("tags")).toBe("诊断,自动化");
      expect(formData.get("team_ids")).toBe("team-1,team-2");
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
          team_ids: ["team-1", "team-2"],
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

  it("updates one skill file with an encoded path", async () => {
    const file = { path: "references/check list.md", file_type: "file", content: "# updated", size_bytes: 9 };
    const fetcher = vi.fn(async () => new Response(JSON.stringify(file), { headers: { "content-type": "application/json" } }));

    await expect(
      updateSkillFile(
        { baseUrl: "http://control-plane.local", fetcher },
        "skill-1",
        "references/check list.md",
        "# updated",
      ),
    ).resolves.toEqual(file);

    expect(fetcher).toHaveBeenCalledWith(
      "http://control-plane.local/api/v1/skills/skill-1/files/references/check%20list.md",
      {
        body: JSON.stringify({ content: "# updated" }),
        credentials: "include",
        headers: { accept: "application/json", "content-type": "application/json" },
        method: "PUT",
      },
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
