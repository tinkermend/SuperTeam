import { describe, expect, it } from "vitest";
import { render } from "vitest-browser-react";
import { userEvent } from "vitest/browser";
import { directTeamRoles, privilegedTeamRoles, teamRoleLabel, TeamRoleSelect } from "./team-role";

describe("team roles", () => {
  it("separates direct and privileged roles", () => {
    expect(directTeamRoles.map((role) => role.value)).toEqual(["member", "viewer"]);
    expect(privilegedTeamRoles.map((role) => role.value)).toEqual(["owner", "admin", "approver"]);
  });

  it("returns Chinese labels", () => {
    expect(teamRoleLabel("owner")).toBe("负责人");
    expect(teamRoleLabel("viewer")).toBe("只读观察者");
  });

  it("limits direct select to member and viewer roles", async () => {
    const screen = await render(<TeamRoleSelect mode="direct" onChange={() => undefined} value="member" />);

    await userEvent.click(screen.getByRole("combobox", { name: "团队角色" }));

    await expect.element(screen.getByRole("option", { name: "普通成员" })).toBeInTheDocument();
    await expect.element(screen.getByRole("option", { name: "只读观察者" })).toBeInTheDocument();
    await expect.element(screen.getByRole("option", { name: "负责人" })).not.toBeInTheDocument();
  });

  it("limits privileged select and emits onChange", async () => {
    let selected = "";
    const screen = await render(<TeamRoleSelect mode="privileged" onChange={(role) => (selected = role)} value="owner" />);

    await userEvent.click(screen.getByRole("combobox", { name: "团队角色" }));
    await expect.element(screen.getByRole("option", { name: "负责人" })).toBeInTheDocument();
    await expect.element(screen.getByRole("option", { name: "普通成员" })).not.toBeInTheDocument();
    await userEvent.click(screen.getByRole("option", { name: "审批人" }));

    expect(selected).toBe("approver");
  });
});
