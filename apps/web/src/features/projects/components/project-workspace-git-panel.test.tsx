import { describe, expect, it, vi } from "vitest";
import { render } from "vitest-browser-react";
import { userEvent } from "vitest/browser";
import { ProjectWorkspaceGitBadge } from "./project-workspace-git-badge";
import { ProjectWorkspaceGitPanel } from "./project-workspace-git-panel";
import type { ProjectWorkspaceGitStatus } from "@/lib/api/projects";

const dirtyStatus: ProjectWorkspaceGitStatus = {
  applicable: true,
  is_git_repo: true,
  is_clean: false,
  head_commit: "abcdef0123456789",
  repo_state: "ok",
  uncommitted_count: 2,
  uncommitted_entries: [
    { path: "src/main.go", category: "modified" },
    { path: "notes.md", category: "untracked" },
  ],
  sampled_at: new Date(Date.now() - 5 * 60_000).toISOString(),
};

describe("ProjectWorkspaceGitBadge", () => {
  it("renders dirty lean badge without probing", async () => {
    const screen = await render(<ProjectWorkspaceGitBadge status={dirtyStatus} />);
    await expect.element(screen.getByText("工作区脏")).toBeInTheDocument();
  });

  it("renders not applicable", async () => {
    const screen = await render(
      <ProjectWorkspaceGitBadge
        status={{ applicable: false, is_git_repo: false, uncommitted_count: 0 }}
      />,
    );
    await expect.element(screen.getByText("非 git")).toBeInTheDocument();
  });
});

describe("ProjectWorkspaceGitPanel", () => {
  it("shows short head hash and expands dirty list", async () => {
    const onRefresh = vi.fn();
    const screen = await render(
      <ProjectWorkspaceGitPanel
        pending={false}
        status={dirtyStatus}
        onRefresh={onRefresh}
      />,
    );
    await expect.element(screen.getByText(/abcdef0/)).toBeInTheDocument();
    await expect.element(screen.getByText("工作区脏")).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: /展开未提交清单/ }));
    await expect.element(screen.getByText("src/main.go")).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "刷新现场" }));
    expect(onRefresh).toHaveBeenCalled();
  });
});
