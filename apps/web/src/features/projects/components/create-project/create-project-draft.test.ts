import { describe, expect, it } from "vitest";
import {
  buildProjectCreateInput,
  directoryNameHintFromGitURL,
  emptyProjectCreateDraft,
  projectCreateValidation,
  validateDisplayProjectName,
  validateProjectDirectoryName,
} from "./create-project-draft";
import type { UserProjectTeamScope, UserSummary } from "@/lib/api";

const currentUser: UserSummary = {
  id: "11111111-1111-1111-1111-111111111111",
  username: "admin",
  display_name: "管理员",
  email: "admin@example.com",
  status: "active",
  avatar: { provider: "dicebear", seed: "admin", style: "adventurer" },
};

describe("create-project-draft name/directory split", () => {
  it("allows Chinese display names and rejects Chinese directory names", () => {
    expect(validateDisplayProjectName("客户接入试点")).toBeNull();
    expect(validateProjectDirectoryName("客户接入")).not.toBeNull();
    expect(validateProjectDirectoryName("customer-onboarding")).toBeNull();
  });

  it("derives directory hint from git URL", () => {
    expect(directoryNameHintFromGitURL("https://github.com/acme/repo.git")).toBe("repo");
    expect(directoryNameHintFromGitURL("git@github.com:acme/SuperTeam.git")).toBe("SuperTeam");
  });

  it("requires directory for non-git and URL for git", () => {
    const teamId = "22222222-2222-2222-2222-222222222222";
    const teams: UserProjectTeamScope[] = [
      {
        id: "33333333-3333-3333-3333-333333333333",
        tenant_id: "44444444-4444-4444-4444-444444444444",
        user_id: currentUser.id,
        team_id: teamId,
        status: "active",
        created_at: "2026-06-04T02:28:13Z",
        updated_at: "2026-06-04T02:28:13Z",
        revoked_at: undefined,
        team: {
          id: teamId,
          name: "默认团队",
          status: "active",
          slug: "default",
          digital_employee_count: 0,
          governance_status: "active",
          human_owners: [],
          pending_draft_count: 0,
          risk_summary: "低风险",
        },
      },
    ];

    const nonGit = {
      ...emptyProjectCreateDraft,
      name: "客户接入",
      goal: "目标",
      sourceKind: "directory" as const,
      directoryName: "",
      sourceTeamIds: [teamId],
      runtimeNodeIds: ["33333333-3333-3333-3333-333333333333"],
    };
    expect(projectCreateValidation(nonGit, currentUser.id, teams).basics).toBe(false);

    const nonGitOk = { ...nonGit, directoryName: "customer-onboarding" };
    expect(projectCreateValidation(nonGitOk, currentUser.id, teams).basics).toBe(true);

    const gitMissingUrl = {
      ...emptyProjectCreateDraft,
      name: "客户接入",
      goal: "目标",
      sourceKind: "git" as const,
      repoUrl: "",
      sourceTeamIds: [teamId],
      runtimeNodeIds: ["33333333-3333-3333-3333-333333333333"],
    };
    expect(projectCreateValidation(gitMissingUrl, currentUser.id, teams).repoError).toBeTruthy();

    const gitOk = {
      ...gitMissingUrl,
      repoUrl: "https://github.com/acme/customer-onboarding.git",
    };
    expect(projectCreateValidation(gitOk, currentUser.id, teams).basics).toBe(true);

    const input = buildProjectCreateInput(gitOk, currentUser);
    expect(input.name).toBe("客户接入");
    expect(input.directory_name).toBeUndefined();
    expect(input.repo_binding?.url).toContain("customer-onboarding");
  });
});
