import { describe, expect, it } from "vitest";
import { directTeamRoles, privilegedTeamRoles, teamRoleLabel } from "./team-role";

describe("team roles", () => {
  it("separates direct and privileged roles", () => {
    expect(directTeamRoles.map((role) => role.value)).toEqual(["member", "viewer"]);
    expect(privilegedTeamRoles.map((role) => role.value)).toEqual(["owner", "admin", "approver"]);
  });

  it("returns Chinese labels", () => {
    expect(teamRoleLabel("owner")).toBe("负责人");
    expect(teamRoleLabel("viewer")).toBe("只读观察者");
  });
});
