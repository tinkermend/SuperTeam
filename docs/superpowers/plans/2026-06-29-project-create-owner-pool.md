# Project Create Owner Pool Implementation Plan
> 复核状态：已实现（2026-06-29完成）

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Rework project creation around a human-owner pool and remove task-launch reviewer selection while keeping primary-owner fallback behavior.

**Architecture:** Keep the existing `human_owner_user_id` root field as the primary owner and express additional human owners through `project_members` with `project_role = owner`. Treat selected source teams as create-time digital-employee filters only, submit the first selected source team as the compatibility `team_id`, and submit only selected digital employees as executor members. Task launch stops collecting reviewer input; the Control Plane derives reviewer preference from the primary owner when the client omits reviewer fields.

**Tech Stack:** React 19, TanStack Query, TypeScript, Vitest browser tests, Go Control Plane service tests, OpenAPI contract verification only when contract files change.

---

## Preconditions

- This repo currently has unrelated dirty files. Before editing, run `git status --short` and inspect target-file diffs with `git diff -- <path>`.
- Do not revert or overwrite user changes. If a target file already has edits, work with the current content.
- Code discovery should prefer codebase-memory MCP graph tools. In the current tool list those graph tools are not exposed, so use `rg` and direct file reads.
- Frontend visual changes already satisfy the requirement to read `DESIGN.md`; keep the existing v3 split-console style and do not introduce a new visual system.

## File Structure

- Modify `apps/control-plane/internal/project/service.go`
  - Change implicit demand reviewer selection so missing reviewer fields always resolve to the primary owner, even if reviewer members exist.
- Modify `apps/control-plane/internal/project/service_test.go`
  - Update demand reviewer tests to match primary-owner fallback semantics.
  - Keep explicit reviewer compatibility tests.
- Modify `apps/web/src/features/task-launches/components/task-launch-form.tsx`
  - Remove reviewer resolver, reviewer dropdown, reviewer state, and reviewer fields from submit payload.
- Modify `apps/web/src/features/task-launches/index.tsx`
  - Stop fetching project members only for reviewer selection.
- Modify `apps/web/src/features/task-launches/index.test.tsx`
  - Remove resolver tests, remove reviewer-selector assertions, and verify submit payload omits reviewer fields.
- Modify `apps/web/src/features/projects/components/create-project/create-project-draft.ts`
  - Replace leader/acceptance/reviewer draft fields with `ownerUsers` and `sourceTeamIds`.
  - Map additional human owners to `owner` project members.
- Modify `apps/web/src/features/projects/components/create-project/create-project-shell.tsx`
  - Change step list from five to four steps and wire human-owner and multi-source-team state.
- Create `apps/web/src/features/projects/components/create-project/project-human-owners-step.tsx`
  - Human owner pool selection component.
- Delete `apps/web/src/features/projects/components/create-project/project-human-roles-step.tsx`
  - Old role-heavy component is no longer used.
- Modify `apps/web/src/features/projects/components/create-project/project-basics-step.tsx`
  - Remove team selection and fixed owner display from the basics step.
- Modify `apps/web/src/features/projects/components/create-project/project-digital-employees-step.tsx`
  - Add source-team multi-select and merge digital employees from selected teams.
- Modify `apps/web/src/features/projects/components/create-project/project-policy-step.tsx`
  - Update policy copy to primary-owner/project-owner language.
- Modify `apps/web/src/features/projects/components/create-project/project-review-panel.tsx`
  - Show primary owner, additional owners, source teams, selected digital employees, and policy summary.
- Modify `apps/web/src/features/projects/index.test.tsx`
  - Update create-project tests to assert owner-pool payloads and absence of old role fields.
- Modify `CHANGELOG.md`
  - Add a timestamped entry after implementation is complete using `TZ=Asia/Shanghai date '+%Y-%m-%d %H:%M'`.

---

### Task 1: Backend Demand Reviewer Fallback

**Files:**
- Modify: `apps/control-plane/internal/project/service_test.go`
- Modify: `apps/control-plane/internal/project/service.go`

- [ ] **Step 1: Update the default reviewer test to expect owner fallback**

In `apps/control-plane/internal/project/service_test.go`, replace `TestSubmitDemandPersistsDefaultReviewerPreference` with this test:

```go
func TestSubmitDemandPersistsPrimaryOwnerFallbackWhenReviewerOmitted(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	ownerID := uuid.New()
	reviewerID := uuid.New()
	repo := newMemoryRepository()
	repo.projects[projectID] = Project{
		ID:               projectID,
		TenantID:         tenantID,
		Status:           ProjectStatusRunning,
		HumanOwnerUserID: ownerID,
	}
	repo.members[projectID] = []ProjectMember{
		{
			ID: uuid.New(), TenantID: tenantID, ProjectID: projectID,
			PrincipalType: PrincipalTypeHumanUser, PrincipalID: ownerID,
			ProjectRole: ProjectRoleOwner, Status: "active",
		},
		{
			ID: uuid.New(), TenantID: tenantID, ProjectID: projectID,
			PrincipalType: PrincipalTypeHumanUser, PrincipalID: reviewerID,
			ProjectRole: ProjectRoleReviewer, Status: "active",
		},
	}
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	demand, err := service.SubmitDemand(context.Background(), SubmitProjectDemandRequest{
		TenantID: tenantID, ProjectID: projectID, SubmittedByUserID: ownerID,
		Title: "审查 PR", Content: "统计 PR 并分派审查",
	})
	if err != nil {
		t.Fatalf("submit demand: %v", err)
	}

	if demand.ReviewerPreference == nil {
		t.Fatalf("expected reviewer preference on demand: %#v", demand)
	}
	if demand.ReviewerPreference.ReviewerUserID != ownerID {
		t.Fatalf("expected owner %s, got %#v", ownerID, demand.ReviewerPreference)
	}
	if demand.ReviewerPreference.SelectionReason != ReviewerSelectionProjectHumanOwnerFallback {
		t.Fatalf("unexpected reviewer reason: %#v", demand.ReviewerPreference)
	}
	if demand.SourceRefs["reviewer_user_id"] != ownerID.String() {
		t.Fatalf("expected owner persisted in source refs: %#v", demand.SourceRefs)
	}
}
```

- [ ] **Step 2: Update the multiple-reviewer test to expect owner fallback**

In `apps/control-plane/internal/project/service_test.go`, replace `TestSubmitDemandRequiresExplicitReviewerWhenMultipleReviewers` with this test:

```go
func TestSubmitDemandFallsBackToPrimaryOwnerWhenMultipleReviewersExist(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	ownerID := uuid.New()
	repo := newMemoryRepository()
	repo.projects[projectID] = Project{
		ID:               projectID,
		TenantID:         tenantID,
		Status:           ProjectStatusRunning,
		HumanOwnerUserID: ownerID,
	}
	repo.members[projectID] = []ProjectMember{{
		ID: uuid.New(), TenantID: tenantID, ProjectID: projectID,
		PrincipalType: PrincipalTypeHumanUser, PrincipalID: ownerID,
		ProjectRole: ProjectRoleOwner, Status: "active",
	}}
	for range 2 {
		repo.members[projectID] = append(repo.members[projectID], ProjectMember{
			ID: uuid.New(), TenantID: tenantID, ProjectID: projectID,
			PrincipalType: PrincipalTypeHumanUser, PrincipalID: uuid.New(),
			ProjectRole: ProjectRoleReviewer, Status: "active",
		})
	}
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	demand, err := service.SubmitDemand(context.Background(), SubmitProjectDemandRequest{
		TenantID: tenantID, ProjectID: projectID, SubmittedByUserID: ownerID,
		Title: "多审核人项目",
	})
	if err != nil {
		t.Fatalf("submit demand: %v", err)
	}
	if demand.ReviewerPreference == nil || demand.ReviewerPreference.ReviewerUserID != ownerID {
		t.Fatalf("expected owner fallback preference: %#v", demand.ReviewerPreference)
	}
	if demand.ReviewerPreference.SelectionReason != ReviewerSelectionProjectHumanOwnerFallback {
		t.Fatalf("expected owner fallback reason, got %#v", demand.ReviewerPreference)
	}
}
```

- [ ] **Step 3: Run the backend tests and confirm they fail before implementation**

Run:

```bash
go test ./apps/control-plane/internal/project -run 'TestSubmitDemand(PersistsPrimaryOwnerFallbackWhenReviewerOmitted|FallsBackToPrimaryOwnerWhenMultipleReviewersExist)' -count=1
```

Expected: fail because `selectReviewer` still chooses reviewer defaults or rejects multiple reviewers.

- [ ] **Step 4: Replace implicit reviewer selection with primary-owner fallback**

In `apps/control-plane/internal/project/service.go`, replace `selectReviewer` with:

```go
func selectReviewer(explicit *uuid.UUID, explicitReason ReviewerSelectionReason, project Project, members []ProjectMember) (ProjectMember, ReviewerSelectionReason, bool, error) {
	if explicit != nil {
		reason, err := normalizeReviewerSelectionReason(explicitReason)
		if err != nil {
			return ProjectMember{}, "", false, err
		}
		for _, member := range members {
			if member.PrincipalType == PrincipalTypeHumanUser && member.PrincipalID == *explicit && member.Status == "active" {
				return member, reason, false, nil
			}
		}
		return ProjectMember{}, "", false, ErrInvalidProjectMember
	}
	for _, member := range members {
		if member.PrincipalType == PrincipalTypeHumanUser && member.PrincipalID == project.HumanOwnerUserID && member.ProjectRole == ProjectRoleOwner && member.Status == "active" {
			return member, ReviewerSelectionProjectHumanOwnerFallback, true, nil
		}
	}
	return ProjectMember{}, "", false, ErrInvalidProjectMember
}
```

- [ ] **Step 5: Run the targeted backend reviewer tests**

Run:

```bash
go test ./apps/control-plane/internal/project -run 'TestSubmitDemand(PersistsPrimaryOwnerFallbackWhenReviewerOmitted|PersistsExplicitReviewerSelectionReason|RejectsInvalidReviewerSelectionReason|DiscardsSpoofedReviewerSourceRefs|FallsBackToHumanOwnerWhenNoReviewer|RequiresActiveHumanOwnerMemberForFallback|RejectsDigitalEmployeeReviewer|FallsBackToPrimaryOwnerWhenMultipleReviewersExist)' -count=1
```

Expected: pass.

- [ ] **Step 6: Commit backend fallback change**

Run:

```bash
git add apps/control-plane/internal/project/service.go apps/control-plane/internal/project/service_test.go
git commit -m "fix(control-plane): default demands to project owner"
```

---

### Task 2: Task Launch Without Reviewer Selection

**Files:**
- Modify: `apps/web/src/features/task-launches/components/task-launch-form.tsx`
- Modify: `apps/web/src/features/task-launches/index.tsx`
- Modify: `apps/web/src/features/task-launches/index.test.tsx`

- [ ] **Step 1: Update task-launch tests for reviewer-free submit**

In `apps/web/src/features/task-launches/index.test.tsx`:

1. Remove the import of `resolveDefaultReviewer` and `ReviewerDefaultResolution`.
2. Delete the `expectReviewerResolution` helper.
3. Delete the `describe("resolveDefaultReviewer", ...)` block.
4. Replace the first submit test with:

```tsx
it("submits demand without reviewer fields", async () => {
  mocks.navigate.mockClear();
  const fetcher = createTaskLaunchFetcher();
  await renderWithQueryClient(
    <TaskLaunchView apiBaseUrl="http://control-plane.local" fetcher={fetcher} />,
  );

  await typeInLabeledField("需求描述", "审查这个开源项目的 PR，并按数量分配数字员工");
  await clickButton("提交任务");

  expect(postBody(fetcher, "/api/v1/projects/project-1/demands")).toEqual({
    title: "审查这个开源项目的 PR，并按数量分配数字员工",
    content: "审查这个开源项目的 PR，并按数量分配数字员工",
    source_type: "manual",
    source_refs: {},
    attachments: [],
  });
  await vi.waitFor(() => {
    expect(mocks.navigate).toHaveBeenCalledWith({
      params: { demandId: "demand-1" },
      to: "/workflows/$demandId",
    });
  });
});
```

5. Replace the second-project submit assertion with:

```tsx
expect(postBody(fetcher, "/api/v1/projects/project-2/demands")).toMatchObject({
  content: "处理第二个项目的巡检问题",
  title: "处理第二个项目的巡检问题",
});
expect(postBody(fetcher, "/api/v1/projects/project-2/demands")).not.toHaveProperty("reviewer_user_id");
expect(postBody(fetcher, "/api/v1/projects/project-2/demands")).not.toHaveProperty("reviewer_selection_reason");
```

6. Delete the test named `requires explicit reviewer selection when the selected project has multiple active human reviewers`.
7. In the composer test, replace `expect(getByLabelText("审核人")).toBeTruthy();` with:

```tsx
expect(() => getByLabelText("审核人")).toThrow();
expect(queryByText("审核人")).toBeNull();
```

- [ ] **Step 2: Run the task-launch test and confirm it fails before implementation**

Run:

```bash
corepack pnpm --filter ./apps/web run test -- src/features/task-launches/index.test.tsx
```

Expected: fail because the reviewer selector still renders and submit payload still includes reviewer fields.

- [ ] **Step 3: Replace `TaskLaunchForm` props and submit behavior**

In `apps/web/src/features/task-launches/components/task-launch-form.tsx`:

1. Remove imports for `UserRoundCheck`, `ProjectMember`, and `ReviewerSelectionReason`.
2. Delete `ReviewerDefaultResolution`, `resolveDefaultReviewer`, `reviewerSortKey`, and `reviewerLabel`.
3. Replace `TaskLaunchFormProps` with:

```tsx
type TaskLaunchFormProps = {
  isSubmitting?: boolean;
  onProjectChange: (projectId: string) => void;
  onSubmit: (projectId: string, input: SubmitProjectDemandInput) => void;
  projects: Project[];
  selectedProjectId?: string;
};
```

4. Remove `members`, `isReviewerLoading`, `reviewerId`, `currentProjectMembers`, `reviewerDefault`, `selectedReviewerId`, `selectedReason`, `humanReviewers`, and the `useEffect` that clears reviewer state.
5. Replace `handleProjectChange` with:

```tsx
function handleProjectChange(nextProjectId: string) {
  setError("");
  onProjectChange(nextProjectId);
}
```

6. Replace the reviewer validation and submit payload in `handleSubmit` with:

```tsx
setError("");
onSubmit(projectId, {
  attachments: [],
  content: trimmedContent,
  source_refs: {},
  source_type: "manual" as ProjectDemandSourceType,
  title: resolvedTitle,
});
```

7. Replace the parameter-grid class with three columns:

```tsx
className="grid gap-3 md:grid-cols-2 xl:grid-cols-[minmax(15rem,1.1fr)_minmax(9rem,0.55fr)_minmax(10rem,0.65fr)] xl:items-end"
```

8. Delete the entire `LaunchSelect` block labeled `审核人`.
9. Change the copy `先固定项目、审核与风险边界，再交给协调线程拆解。` to:

```tsx
先固定项目、优先级与风险边界，再交给协调线程拆解。
```

10. Replace submit button disabled logic with:

```tsx
disabled={isSubmitting}
```

- [ ] **Step 4: Remove task-launch member fetching**

In `apps/web/src/features/task-launches/index.tsx`:

1. Remove `listProjectMembers` from the import list.
2. Delete the `membersQuery`, `hasSelectedProjectMembers`, and `isReviewerLoading` constants.
3. Replace the `<TaskLaunchForm />` props with:

```tsx
<TaskLaunchForm
  isSubmitting={submitMutation.isPending}
  onProjectChange={setSelectedProjectId}
  onSubmit={(projectId, input) => submitMutation.mutate({ input, projectId })}
  projects={projectsQuery.data ?? []}
  selectedProjectId={selectedProjectId}
/>
```

- [ ] **Step 5: Run the task-launch test**

Run:

```bash
corepack pnpm --filter ./apps/web run test -- src/features/task-launches/index.test.tsx
```

Expected: pass.

- [ ] **Step 6: Commit task-launch change**

Run:

```bash
git add apps/web/src/features/task-launches/components/task-launch-form.tsx apps/web/src/features/task-launches/index.tsx apps/web/src/features/task-launches/index.test.tsx
git commit -m "feat(web): remove task launch reviewer selection"
```

---

### Task 3: Create Project Draft And Payload Mapping

**Files:**
- Modify: `apps/web/src/features/projects/components/create-project/create-project-draft.ts`
- Modify: `apps/web/src/features/projects/index.test.tsx`

- [ ] **Step 1: Update create-project test expectations for owner-pool payload**

> NOTE: The snippet below is an early sketch. The authoritative, fixture-consistent rewrite of this main test is **Task 5 Step 3 (3c)** — it fixes the team names (`平台运营`/`风控审查`, not `平台工程`/`验收团队`) and the `team_id` assertion (`TEAM_REVIEW_ID`, where the employees actually live). When you reach Task 5, replace whatever you wrote here with 3c. The two must not diverge.

In `apps/web/src/features/projects/index.test.tsx`, rename the test `creates a project from the split console with roles, employees, and policies` to:

```tsx
it("creates a project from the split console with human owners, source teams, employees, and policies", async () => {
```

Inside that test, after filling project name and goal, replace the old role-selection steps with this sequence:

```tsx
await userEvent.click(screen.getByRole("button", { name: "下一步" }));
await userEvent.fill(screen.getByLabelText("搜索项目人类负责人"), "李娜");
await userEvent.click(screen.getByRole("button", { name: "选择 leader" }).first());

await userEvent.click(screen.getByRole("button", { name: "下一步" }));
await userEvent.click(screen.getByRole("button", { name: /平台工程/ }));
await userEvent.click(screen.getByRole("button", { name: /验收团队/ }));
await userEvent.click(screen.getByText("研发助手"));
await userEvent.click(screen.getByText("测试工程师"));

await userEvent.click(screen.getByRole("button", { name: "下一步" }));
await userEvent.click(screen.getByRole("button", { name: /高风险审批/ }));
await userEvent.click(screen.getByRole("button", { name: "创建项目", exact: true }));
```

Replace the body assertion with:

```tsx
const body = JSON.parse(String(postCall?.[1]?.body));
expect(body).toMatchObject({
  human_owner_user_id: CURRENT_USER_ID,
  name: "客户验收推进",
  team_id: TEAM_AUTHORIZED_ID,
  approval_policy: {
    budget_overrun_requires_owner_approval: true,
    high_risk_action_requires_confirmation: true,
    new_demand_requires_human_confirmation: true,
    preset: "highRisk",
  },
  evidence_policy: {
    acceptance_requires_evidence: true,
    preset: "highRisk",
  },
});
expect(body).not.toHaveProperty("leader_user_id");
expect(body).not.toHaveProperty("acceptance_user_id");
expect(body.members).toEqual(
  expect.arrayContaining([
    expect.objectContaining({
      principal_id: LEADER_USER_ID,
      principal_type: "human_user",
      project_role: "owner",
    }),
    expect.objectContaining({
      principal_id: EMPLOYEE_ASSISTANT_ID,
      principal_type: "digital_employee",
      project_role: "executor",
    }),
  ]),
);
expect(body.members).not.toEqual(
  expect.arrayContaining([
    expect.objectContaining({
      principal_type: "team",
    }),
    expect.objectContaining({
      project_role: "leader",
    }),
    expect.objectContaining({
      project_role: "acceptance",
    }),
    expect.objectContaining({
      project_role: "reviewer",
    }),
  ]),
);
```

- [ ] **Step 2: Run the create-project test and confirm it fails before implementation**

Run:

```bash
corepack pnpm --filter ./apps/web run test -- src/features/projects/index.test.tsx
```

Expected: fail because the draft still uses leader/acceptance/reviewer fields and the confirm step still exists.

- [ ] **Step 3: Replace draft shape and step list**

In `apps/web/src/features/projects/components/create-project/create-project-draft.ts`, replace the step and draft definitions with:

```ts
export type ProjectCreateStep = "basics" | "owners" | "digitalEmployees" | "policies";

export const projectCreateSteps: Array<{ id: ProjectCreateStep; label: string }> = [
  { id: "basics", label: "基础信息" },
  { id: "owners", label: "人类负责人" },
  { id: "digitalEmployees", label: "数字员工池" },
  { id: "policies", label: "策略预设" },
];

export type ProjectPolicyPreset = "standard" | "lightweight" | "highRisk";

export type ProjectCreateDraft = {
  description: string;
  goal: string;
  name: string;
  ownerUsers: UserSummary[];
  policyPreset: ProjectPolicyPreset;
  policyToggles: {
    auditLogEnabled: boolean;
    budgetOverrunNeedsOwnerApproval: boolean;
    highRiskActionNeedsConfirmation: boolean;
    newDemandNeedsHumanConfirmation: boolean;
    requireEvidenceBeforeAcceptance: boolean;
  };
  selectedDigitalEmployees: DigitalEmployee[];
  sourceTeamIds: string[];
};
```

Replace `emptyProjectCreateDraft` with:

```ts
export const emptyProjectCreateDraft: ProjectCreateDraft = {
  description: "",
  goal: "",
  name: "",
  ownerUsers: [],
  policyPreset: "standard",
  policyToggles: {
    auditLogEnabled: true,
    budgetOverrunNeedsOwnerApproval: false,
    highRiskActionNeedsConfirmation: true,
    newDemandNeedsHumanConfirmation: true,
    requireEvidenceBeforeAcceptance: true,
  },
  selectedDigitalEmployees: [],
  sourceTeamIds: [],
};
```

Replace `selectedTeam` with:

```ts
export function selectedSourceTeams(scopes: UserProjectTeamScope[] | undefined, sourceTeamIds: string[]) {
  const selectedIds = new Set(sourceTeamIds);
  return activeSelectableTeams(scopes).filter((scope) => selectedIds.has(scope.team_id));
}
```

- [ ] **Step 4: Replace validation and payload mapping**

In `create-project-draft.ts`, replace `projectCreateValidation` with:

```ts
export function projectCreateValidation(
  draft: ProjectCreateDraft,
  currentUserId: string | undefined,
  selectableTeams: UserProjectTeamScope[],
) {
  const authorizedTeamIds = new Set(activeSelectableTeams(selectableTeams).map((scope) => scope.team_id));
  const sourceTeamsAuthorized =
    draft.sourceTeamIds.length > 0 && draft.sourceTeamIds.every((teamId) => authorizedTeamIds.has(teamId));
  return {
    basics: Boolean(draft.name.trim()) && Boolean(draft.goal.trim()),
    currentUser: Boolean(currentUserId),
    digitalEmployees: true,
    policies: draft.policyToggles.auditLogEnabled,
    sourceTeams: sourceTeamsAuthorized,
    teamAuthorized: sourceTeamsAuthorized,
  };
}
```

In `buildProjectCreateInput`, delete leader, acceptance, and reviewer member creation. Insert owner member creation before digital employee members:

```ts
  for (const owner of draft.ownerUsers) {
    members.push({
      display_name_snapshot: owner.display_name ?? owner.username,
      principal_id: owner.id,
      principal_type: "human_user",
      project_role: "owner",
    });
  }
```

Replace the returned identity fields with:

```ts
    human_owner_user_id: currentUser.id,
    members,
    name: draft.name.trim(),
    team_id: draft.sourceTeamIds[0],
```

Do not include `leader_user_id` or `acceptance_user_id` in the returned object.

- [ ] **Step 5: Run focused TypeScript tests for create-project draft consumers**

Run:

```bash
corepack pnpm --filter ./apps/web run test -- src/features/projects/index.test.tsx
```

Expected: still fail until the UI components are updated, but TypeScript errors should identify the remaining old draft field references.

- [ ] **Step 6: Commit after UI tasks, not now**

Do not commit after this task because the UI will not compile until Tasks 4 and 5 update all draft consumers.

---

### Task 4: Create Project Human Owners And Source Teams UI

**Files:**
- Create: `apps/web/src/features/projects/components/create-project/project-human-owners-step.tsx`
- Delete: `apps/web/src/features/projects/components/create-project/project-human-roles-step.tsx`
- Modify: `apps/web/src/features/projects/components/create-project/project-basics-step.tsx`
- Modify: `apps/web/src/features/projects/components/create-project/project-digital-employees-step.tsx`
- Modify: `apps/web/src/features/projects/components/create-project/create-project-shell.tsx`

- [ ] **Step 1: Create the human owners step**

Create `apps/web/src/features/projects/components/create-project/project-human-owners-step.tsx`:

```tsx
import { X } from "lucide-react";
import type { UserSummary } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import { UserIdentity } from "@/components/superteam/user-identity";
import { UserSearchSelect } from "@/components/superteam/user-search-select";
import type { ProjectCreateDraft } from "./create-project-draft";

type ProjectHumanOwnersStepProps = {
  apiBaseUrl: string;
  currentUser?: UserSummary;
  draft: ProjectCreateDraft;
  fetcher?: typeof fetch;
  onChange: (draft: ProjectCreateDraft) => void;
};

export function ProjectHumanOwnersStep({
  apiBaseUrl,
  currentUser,
  draft,
  fetcher,
  onChange,
}: ProjectHumanOwnersStepProps) {
  const excludedUserIds = [
    currentUser?.id,
    ...draft.ownerUsers.map((user) => user.id),
  ].filter(Boolean) as string[];

  return (
    <div className="grid gap-6">
      <section className="grid gap-2">
        <Label>主负责人（当前创建人）</Label>
        <div className="rounded-xl border border-v3-line bg-v3-card-soft px-3 py-2">
          {currentUser ? <UserIdentity showSecondary user={currentUser} /> : <p className="text-sm text-v3-ink-3">正在加载当前用户...</p>}
        </div>
        <p className="text-xs text-v3-ink-3">主负责人用于默认审批、需求确认和最终验收；其他负责人作为项目 owner 成员参与管理。</p>
      </section>

      <section className="grid gap-3">
        <div>
          <Label>项目人类负责人</Label>
          <p className="mt-1 text-xs text-v3-ink-3">可选。额外负责人会以 owner 成员加入项目，不拆分 Leader、验收负责人或审核人。</p>
        </div>
        <UserSearchSelect
          apiBaseUrl={apiBaseUrl}
          excludedUserIds={excludedUserIds}
          fetcher={fetcher}
          inputLabel="搜索项目人类负责人"
          onSelect={(owner) => {
            if (draft.ownerUsers.some((user) => user.id === owner.id)) return;
            onChange({ ...draft, ownerUsers: [...draft.ownerUsers, owner] });
          }}
          placeholder="搜索后添加项目负责人"
        />
        {draft.ownerUsers.length > 0 ? (
          <ul className="grid gap-2">
            {draft.ownerUsers.map((owner) => (
              <li className="flex items-center justify-between gap-3 rounded-xl border border-v3-line bg-v3-card-soft px-3 py-2" key={owner.id}>
                <UserIdentity showSecondary user={owner} />
                <Button
                  aria-label={`移除项目负责人 ${owner.username}`}
                  className="size-8"
                  onClick={() => onChange({ ...draft, ownerUsers: draft.ownerUsers.filter((user) => user.id !== owner.id) })}
                  size="icon"
                  type="button"
                  variant="ghost"
                >
                  <X className="size-4" />
                </Button>
              </li>
            ))}
          </ul>
        ) : null}
      </section>
    </div>
  );
}
```

- [ ] **Step 2: Remove the old human roles file**

Run:

```bash
git rm apps/web/src/features/projects/components/create-project/project-human-roles-step.tsx
```

Expected: the file is staged for deletion. If local user changes exist in that file, stop and preserve them before removing.

- [ ] **Step 3: Remove team and owner fields from basics step**

In `apps/web/src/features/projects/components/create-project/project-basics-step.tsx`:

1. Remove imports for `UserProjectTeamScope` and `UserIdentity`.
2. Replace `ProjectBasicsStepProps` with:

```tsx
type ProjectBasicsStepProps = {
  draft: ProjectCreateDraft;
  onChange: (draft: ProjectCreateDraft) => void;
};
```

3. Replace the function signature with:

```tsx
export function ProjectBasicsStep({ draft, onChange }: ProjectBasicsStepProps) {
```

4. Delete the `授权团队 *` section and the `固定负责人（人类）` section.

- [ ] **Step 4: Add source-team multi-select and merged employees**

In `apps/web/src/features/projects/components/create-project/project-digital-employees-step.tsx`:

1. Replace `useQuery` import with:

```tsx
import { useQueries } from "@tanstack/react-query";
```

2. Add `UserProjectTeamScope` to imports:

```tsx
import type { UserProjectTeamScope } from "@/lib/api";
```

3. Add `selectableTeams` to props:

```tsx
type ProjectDigitalEmployeesStepProps = {
  apiBaseUrl: string;
  draft: ProjectCreateDraft;
  fetcher?: typeof fetch;
  onChange: (draft: ProjectCreateDraft) => void;
  selectableTeams: UserProjectTeamScope[];
};
```

4. Add `selectableTeams` to the function arguments.
5. Replace the single `employeesQuery` with:

```tsx
  const employeeQueries = useQueries({
    queries: draft.sourceTeamIds.map((teamId) => ({
      enabled: Boolean(teamId),
      queryKey: ["project-create", "digital-employees", teamId],
      queryFn: () => listDigitalEmployees({ baseUrl: apiBaseUrl, fetcher }, { team_id: teamId }),
    })),
  });
  const employeesQueryIsLoading = employeeQueries.some((queryItem) => queryItem.isLoading);
  const employeesQueryIsError = employeeQueries.some((queryItem) => queryItem.isError);
  const employeesById = new Map<string, DigitalEmployee>();
  for (const queryItem of employeeQueries) {
    for (const employee of queryItem.data ?? []) {
      employeesById.set(employee.id, employee);
    }
  }
```

6. Replace `const employees = (employeesQuery.data ?? []).filter(...` with:

```tsx
  const employees = Array.from(employeesById.values()).filter((employee) => {
    const textMatch = `${employee.name} ${employee.role}`.toLowerCase().includes(query.trim().toLowerCase());
    const modeMatch =
      filterMode === "all" ||
      (filterMode === "schedulable" && employee.status === "active") ||
      (filterMode === "needsConfig" && employee.status !== "active");
    return textMatch && modeMatch;
  });
```

7. Add this helper inside the component:

```tsx
  function toggleSourceTeam(teamId: string) {
    const selected = draft.sourceTeamIds.includes(teamId);
    const sourceTeamIds = selected
      ? draft.sourceTeamIds.filter((item) => item !== teamId)
      : [...draft.sourceTeamIds, teamId];
    const allowedTeamIds = new Set(sourceTeamIds);
    onChange({
      ...draft,
      sourceTeamIds,
      selectedDigitalEmployees: draft.selectedDigitalEmployees.filter((employee) => {
        const employeeTeamId = typeof employee.team_id === "string" ? employee.team_id : undefined;
        return !employeeTeamId || allowedTeamIds.has(employeeTeamId);
      }),
    });
  }
```

8. Insert this source-team section above the search box:

```tsx
      <section className="grid gap-3">
        <div>
          <h4 className="text-sm font-semibold text-v3-ink">数字员工来源团队</h4>
          <p className="mt-1 text-xs text-v3-ink-3">来源团队只用于筛选候选数字员工，不会作为项目团队成员提交。</p>
        </div>
        <div className="flex flex-wrap gap-2">
          {selectableTeams.map((scope) => {
            const active = draft.sourceTeamIds.includes(scope.team_id);
            return (
              <Button
                className={cn("rounded-xl", active && "border-v3-brand bg-v3-brand-soft text-v3-brand")}
                key={scope.id}
                onClick={() => toggleSourceTeam(scope.team_id)}
                type="button"
                variant="outline"
              >
                {scope.team.name}
              </Button>
            );
          })}
        </div>
      </section>
```

9. Replace `employeesQuery.isLoading` with `employeesQueryIsLoading`.
10. Replace `employeesQuery.isError` with `employeesQueryIsError`.
11. Replace empty-state copy with:

```tsx
{draft.sourceTeamIds.length > 0 ? "暂无匹配数字员工" : "请先选择数字员工来源团队"}
```

- [ ] **Step 5: Wire shell to new step names and defaults**

In `apps/web/src/features/projects/components/create-project/create-project-shell.tsx`:

1. Replace import `ProjectHumanRolesStep` with:

```tsx
import { ProjectHumanOwnersStep } from "./project-human-owners-step";
```

2. Replace the first `useEffect` body with:

```tsx
  useEffect(() => {
    setDraft((current) => {
      const selectableTeamIds = new Set(selectableTeams.map((scope) => scope.team_id));
      const sourceTeamIds = current.sourceTeamIds.filter((teamId) => selectableTeamIds.has(teamId));
      if (sourceTeamIds.length > 0) {
        return { ...current, sourceTeamIds };
      }
      return { ...current, sourceTeamIds: selectableTeams[0]?.team_id ? [selectableTeams[0].team_id] : [], selectedDigitalEmployees: [] };
    });
  }, [selectableTeams]);
```

3. Replace the whole validation guard in `submit()` (do not just swap copy — the old guard forces `setActiveStep("basics")`, but source-team selection now lives in the `digitalEmployees` step):

```tsx
  if (!validation.basics) {
    setLocalError("请补齐项目名称和目标");
    setActiveStep("basics");
    return;
  }
  if (!validation.teamAuthorized) {
    setLocalError("请选择已授权的数字员工来源团队");
    setActiveStep("digitalEmployees");
    return;
  }
```

`canSubmit` needs no change — it still reads `validation.teamAuthorized`, which the new validation keeps as a key.

4. Replace `activeStep === "roles"` branch with:

```tsx
            ) : activeStep === "owners" ? (
              <ProjectHumanOwnersStep
                apiBaseUrl={apiBaseUrl}
                currentUser={currentUser}
                draft={draft}
                fetcher={fetcher}
                onChange={setDraft}
              />
```

5. Add `selectableTeams={selectableTeams}` to `ProjectDigitalEmployeesStep`.
6. Delete the `activeStep === "review"` branch in `<main>`. Keep the always-rendered `<ProjectReviewPanel … />` below it — that is the persistent side panel, not the deleted wizard step.
7. In the footer, keep it a two-way conditional but gate the submit branch on the **last step** (do not make the submit button unconditional — a literal "replace the conditional" removes the `下一步` button and breaks step navigation). Replace the existing `activeStep === "review" ? (…) : (…)` block with:

```tsx
            {activeIndex === projectCreateSteps.length - 1 ? (
              <Button disabled={isSubmitting || !canSubmit} onClick={submit} type="button">
                创建项目
              </Button>
            ) : (
              <Button onClick={goNext} type="button">
                下一步
                <ArrowRight className="ml-2 size-4" />
              </Button>
            )}
```

- [ ] **Step 6: Run the create-project test**

Run:

```bash
corepack pnpm --filter ./apps/web run test -- src/features/projects/index.test.tsx
```

Expected: remaining failures point to review panel/policy copy and test fixture labels, then Task 5 completes the UI.

---

### Task 5: Review Panel, Policy Copy, And Create Tests

**Files:**
- Modify: `apps/web/src/features/projects/components/create-project/project-policy-step.tsx`
- Modify: `apps/web/src/features/projects/components/create-project/project-review-panel.tsx`
- Modify: `apps/web/src/features/projects/index.test.tsx`

- [ ] **Step 1: Update policy copy**

In `apps/web/src/features/projects/components/create-project/project-policy-step.tsx`, replace the four toggle descriptions with:

```tsx
description="任何新需求在执行前需由主负责人确认。"
```

```tsx
description="涉及数据删除、权限变更、外部调用等高风险动作需暂停并等待主负责人确认。"
```

```tsx
description="最终验收前必须补齐产出、测试、日志或审计证据。"
```

```tsx
description="实际消耗超过预算阈值时，需要主负责人审批后继续。"
```

- [ ] **Step 2: Update review panel data model**

In `apps/web/src/features/projects/components/create-project/project-review-panel.tsx`:

1. Replace the `team` constant with:

```tsx
  const sourceTeams = selectableTeams.filter((scope) => draft.sourceTeamIds.includes(scope.team_id));
```

2. Replace `requiredPassed` with:

```tsx
  const requiredPassed = [
    Boolean(draft.name.trim()) && Boolean(draft.goal.trim()),
    sourceTeams.length > 0,
    draft.policyToggles.auditLogEnabled,
  ].filter(Boolean).length;
```

3. Replace `ReviewRow label="所属团队"` with:

```tsx
          <ReviewRow label="来源团队" value={sourceTeams.length > 0 ? `${sourceTeams.length} 个已选` : "未选择"} />
```

4. Replace the `人类责任` section with:

```tsx
        <ReviewSection title="人类负责人">
          <ReviewRow label="主负责人" value={currentUser?.display_name ?? currentUser?.username ?? "未加载"} />
          <ReviewRow label="额外负责人" value={`${draft.ownerUsers.length} 位已选`} />
        </ReviewSection>
```

5. Replace `Boolean(team)` in `CheckLine` with `sourceTeams.length > 0`.
6. Replace `团队授权有效` with `来源团队已选择`.

- [ ] **Step 3: Rewrite the create-project test blocks for the new step flow**

A generic find/replace is not enough: several `it()` blocks drive the old basics-step team `<select>` (`授权团队`), the `固定负责人（人类）` display, and the `确认创建` button — all removed/moved by this plan. Rewrite each block below.

Navigation model after this change: **basics(1) → owners(2) → digitalEmployees(3) → policies(4)**; the `创建项目` submit button appears only on step 4. Fixture facts to stay consistent with: `平台运营 = TEAM_AUTHORIZED_ID`, `风控审查 = TEAM_REVIEW_ID`; digital employees `研发助手 (EMPLOYEE_ASSISTANT_ID)` and `测试工程师 (EMPLOYEE_QA_ID)` both have `team_id = TEAM_REVIEW_ID`; the user-search result button is `选择 {username}` (so 李娜 → `选择 leader`); the digital-employees fetcher ignores `team_id` and returns both employees regardless of which source team is selected.

**3a. `"fetches current user and authorized project teams on the create route"`** — teams are now buttons in step 3, not `<option>`s on basics. Keep the fetch-call assertions; move the UI check to step 3:

```tsx
  it("fetches current user and authorized project teams on the create route", async () => {
    const fetcher = createProjectFetcher();
    const screen = await renderProjectCreate(fetcher);
    await userEvent.fill(screen.getByLabelText("项目名称 *"), "授权检查");
    await userEvent.fill(screen.getByLabelText("项目目标 *"), "确认来源团队可见");

    await userEvent.click(screen.getByRole("button", { name: "下一步" })); // → owners
    await userEvent.click(screen.getByRole("button", { name: "下一步" })); // → digitalEmployees

    await expect.element(screen.getByRole("button", { name: "平台运营" })).toBeInTheDocument();
    await expect.element(screen.getByRole("button", { name: "风控审查" })).toBeInTheDocument();
    await expect.element(screen.getByRole("button", { name: "已撤销团队" })).not.toBeInTheDocument();
    await expect.element(screen.getByRole("button", { name: "停用团队" })).not.toBeInTheDocument();

    await vi.waitFor(() => {
      expect(fetchCalls(fetcher).some(([url, init]) =>
        String(url).endsWith("/api/auth/me") && init?.method === "GET")).toBe(true);
      expect(fetchCalls(fetcher).some(([url, init]) =>
        String(url).endsWith(`/api/auth/users/${CURRENT_USER_ID}/project-team-scopes`) && init?.method === "GET")).toBe(true);
    });
  });
```

**3b. `"refetches cached authorization and blocks stale team submit while scopes refresh"`** — the stale-scopes guard now shows as a disabled `创建项目` button (no `授权团队`, no `固定负责人`). Replace the body inside `try { … }`:

```tsx
    const screen = await renderProjectCreate(fetcher, queryClient);
    await userEvent.fill(screen.getByLabelText("项目名称 *"), "缓存授权项目");
    await userEvent.fill(screen.getByLabelText("项目目标 *"), "等待授权范围刷新后才能提交");

    try {
      await userEvent.click(screen.getByRole("button", { name: "下一步" })); // owners
      await userEvent.click(screen.getByRole("button", { name: "下一步" })); // digitalEmployees
      await userEvent.click(screen.getByRole("button", { name: "下一步" })); // policies
      await expect.element(screen.getByRole("button", { name: "创建项目", exact: true })).toBeDisabled();
      expect(fetchCalls(fetcher).some(([url, init]) =>
        String(url).endsWith(`/api/auth/users/${CURRENT_USER_ID}/project-team-scopes`) && init?.method === "GET")).toBe(true);
      expect(fetchCalls(fetcher).some(([url, init]) =>
        String(url).endsWith("/api/v1/projects") && init?.method === "POST")).toBe(false);
    } finally {
      deferred.resolve(jsonResponse(userProjectTeamScopesResponse()));
    }
```

**3c. Main create test** — this supersedes the version in Task 3 Step 1 (which referenced non-existent teams `平台工程`/`验收团队` and asserted `team_id: TEAM_AUTHORIZED_ID` while the employees live under `TEAM_REVIEW_ID`). Select `风控审查` **first** so `team_id === TEAM_REVIEW_ID`:

```tsx
  it("creates a project from the split console with human owners, source teams, employees, and policies", async () => {
    const fetcher = createProjectFetcher();
    const screen = await renderProjectCreate(fetcher);
    await userEvent.fill(screen.getByLabelText("项目名称 *"), "客户验收推进");
    await userEvent.fill(screen.getByLabelText("项目目标 *"), "完成客户验收闭环");

    await userEvent.click(screen.getByRole("button", { name: "下一步" })); // → owners
    await userEvent.fill(screen.getByLabelText("搜索项目人类负责人"), "李娜");
    await userEvent.click(screen.getByRole("button", { name: "选择 leader" }).first());

    await userEvent.click(screen.getByRole("button", { name: "下一步" })); // → digitalEmployees
    await userEvent.click(screen.getByRole("button", { name: "风控审查" })); // first → team_id
    await userEvent.click(screen.getByRole("button", { name: "平台运营" }));
    await userEvent.click(screen.getByText("研发助手"));
    await userEvent.click(screen.getByText("测试工程师"));

    await userEvent.click(screen.getByRole("button", { name: "下一步" })); // → policies
    await userEvent.click(screen.getByRole("button", { name: /高风险审批/ }));
    await userEvent.click(screen.getByRole("button", { name: "创建项目", exact: true }));

    await vi.waitFor(() => {
      const postCall = fetchCalls(fetcher).find(([url, init]) =>
        String(url).endsWith("/api/v1/projects") && init?.method === "POST");
      expect(postCall).toBeTruthy();
      const body = JSON.parse(String(postCall?.[1]?.body));
      expect(body).toMatchObject({
        human_owner_user_id: CURRENT_USER_ID,
        name: "客户验收推进",
        team_id: TEAM_REVIEW_ID,
        approval_policy: {
          budget_overrun_requires_owner_approval: true,
          high_risk_action_requires_confirmation: true,
          new_demand_requires_human_confirmation: true,
          preset: "highRisk",
        },
        evidence_policy: { acceptance_requires_evidence: true, preset: "highRisk" },
      });
      expect(body).not.toHaveProperty("leader_user_id");
      expect(body).not.toHaveProperty("acceptance_user_id");
      expect(body.members).toEqual(expect.arrayContaining([
        expect.objectContaining({ principal_id: LEADER_USER_ID, principal_type: "human_user", project_role: "owner" }),
        expect.objectContaining({ principal_id: EMPLOYEE_ASSISTANT_ID, principal_type: "digital_employee", project_role: "executor" }),
      ]));
      expect(body.members).not.toEqual(expect.arrayContaining([
        expect.objectContaining({ principal_type: "team" }),
        expect.objectContaining({ project_role: "leader" }),
        expect.objectContaining({ project_role: "acceptance" }),
        expect.objectContaining({ project_role: "reviewer" }),
      ]));
    });
  });
```

**3d. `"does not submit a project when the selected team is not authorized"`** — the old DOM-injection of a fake `<option>` is obsolete: source teams are buttons derived only from authorized scopes, so an unauthorized team is unreachable by construction. Re-point the test at that guarantee and rename it:

```tsx
  it("does not offer unauthorized teams as digital-employee source teams", async () => {
    const fetcher = createProjectFetcher();
    const screen = await renderProjectCreate(fetcher);
    await userEvent.fill(screen.getByLabelText("项目名称 *"), "越权团队项目");
    await userEvent.fill(screen.getByLabelText("项目目标 *"), "尝试提交未授权团队");

    await userEvent.click(screen.getByRole("button", { name: "下一步" })); // owners
    await userEvent.click(screen.getByRole("button", { name: "下一步" })); // digitalEmployees

    await expect.element(screen.getByRole("button", { name: "平台运营" })).toBeInTheDocument();
    await expect.element(screen.getByRole("button", { name: "未授权团队" })).not.toBeInTheDocument();

    // With no source team selected, creation stays disabled and nothing is posted.
    await userEvent.click(screen.getByRole("button", { name: "下一步" })); // policies
    await expect.element(screen.getByRole("button", { name: "创建项目", exact: true })).toBeDisabled();
    expect(fetchCalls(fetcher).some(([url, init]) =>
      String(url).endsWith("/api/v1/projects") && init?.method === "POST")).toBe(false);
  });
```

> Server-side authorization is still covered by the Go `validateProjectTeamScopes` test. Note the design asymmetry: only `sourceTeamIds[0]` is submitted as `team_id`, so the backend authorizes only the first source team; the frontend gates the rest.

**3e. `"shows an empty state and disables project creation when no teams are selectable"`**:

```tsx
  it("shows an empty state and disables project creation when no teams are selectable", async () => {
    const fetcher = createProjectFetcher({ projectTeamScopesStatus: "empty" });
    const screen = await renderProjectCreate(fetcher);
    await userEvent.fill(screen.getByLabelText("项目名称 *"), "客户验收推进");
    await userEvent.fill(screen.getByLabelText("项目目标 *"), "完成客户验收闭环");

    await userEvent.click(screen.getByRole("button", { name: "下一步" })); // owners
    await userEvent.click(screen.getByRole("button", { name: "下一步" })); // digitalEmployees
    await expect.element(screen.getByText("暂无可选团队")).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "下一步" })); // policies
    await expect.element(screen.getByRole("button", { name: "创建项目", exact: true })).toBeDisabled();
  });
```

> The `"暂无可选团队"` empty-state string lives in the basics-step select today; it must move into the source-team section of `project-digital-employees-step.tsx`. Either keep that exact string there or change the assertion to the new `"请先选择数字员工来源团队"` copy — make the component and the test agree (Task 4 Step 4 #11 currently introduces the second string).

**3f. The four loading/error gating tests** (`…current user is loading`, `…current user fails to load`, `…selectable teams are loading`, `…selectable teams fail to load`) all assert `固定负责人（人类）` (removed) and two assert a disabled `授权团队` select (removed). Rewrite to the surviving signals — the auth banner and the owners-step loading text — plus a disabled `创建项目`:

```tsx
  it("keeps project creation disabled while the current user is loading", async () => {
    const deferred = makeDeferred<Response>();
    const fetcher = createProjectFetcher({ currentUserDeferred: deferred, currentUserStatus: "loading" });
    const screen = await renderProjectCreate(fetcher);
    try {
      await userEvent.click(screen.getByRole("button", { name: "下一步" })); // owners
      await expect.element(screen.getByText("正在加载当前用户...")).toBeInTheDocument();
    } finally {
      deferred.resolve(jsonResponse({ user: { /* …unchanged current-user payload… */ } }));
    }
  });

  it("keeps project creation disabled when the current user fails to load", async () => {
    const fetcher = createProjectFetcher({ currentUserStatus: "error" });
    const screen = await renderProjectCreate(fetcher);
    await expect.element(screen.getByText("加载当前用户失败")).toBeInTheDocument();
  });

  it("keeps project creation disabled while selectable teams are loading", async () => {
    const deferred = makeDeferred<Response>();
    const fetcher = createProjectFetcher({ projectTeamScopesDeferred: deferred, projectTeamScopesStatus: "loading" });
    const screen = await renderProjectCreate(fetcher);
    try {
      await userEvent.fill(screen.getByLabelText("项目名称 *"), "等待团队");
      await userEvent.fill(screen.getByLabelText("项目目标 *"), "团队加载中");
      await userEvent.click(screen.getByRole("button", { name: "下一步" })); // owners
      await userEvent.click(screen.getByRole("button", { name: "下一步" })); // digitalEmployees
      await userEvent.click(screen.getByRole("button", { name: "下一步" })); // policies
      await expect.element(screen.getByRole("button", { name: "创建项目", exact: true })).toBeDisabled();
    } finally {
      deferred.resolve(jsonResponse(userProjectTeamScopesResponse()));
    }
  });

  it("keeps project creation disabled when selectable teams fail to load", async () => {
    const fetcher = createProjectFetcher({ projectTeamScopesStatus: "error" });
    const screen = await renderProjectCreate(fetcher);
    await expect.element(screen.getByText("加载可选团队失败")).toBeInTheDocument();
  });
```

**3g.** Remove the now-stale module-level fixtures only referenced by deleted assertions if they become unused (`ACCEPTANCE_USER_ID`, `REVIEWER_USER_ID`, `TEAM_UNAUTHORIZED_ID`) — the linter/TS `noUnusedLocals` will flag them. Keep any still used by other tests.

- [ ] **Step 4: Run project create tests**

Run:

```bash
corepack pnpm --filter ./apps/web run test -- src/features/projects/index.test.tsx
```

Expected: pass.

- [ ] **Step 5: Commit create-project UI change**

Run:

```bash
git add apps/web/src/features/projects/components/create-project/create-project-draft.ts apps/web/src/features/projects/components/create-project/create-project-shell.tsx apps/web/src/features/projects/components/create-project/project-basics-step.tsx apps/web/src/features/projects/components/create-project/project-digital-employees-step.tsx apps/web/src/features/projects/components/create-project/project-human-owners-step.tsx apps/web/src/features/projects/components/create-project/project-human-roles-step.tsx apps/web/src/features/projects/components/create-project/project-policy-step.tsx apps/web/src/features/projects/components/create-project/project-review-panel.tsx apps/web/src/features/projects/index.test.tsx
git commit -m "feat(web): create projects with owner pool"
```

---

### Task 6: Contract And Generated-Code Check

**Files:**
- Inspect: `contracts/control-plane/openapi.yaml`
- Inspect: `apps/web/src/lib/api/projects.ts`
- Optional Modify: generated files only if contract verification shows drift

- [ ] **Step 1: Confirm reviewer fields are optional in OpenAPI**

Run:

```bash
sed -n '6739,6764p' contracts/control-plane/openapi.yaml
```

Expected output includes:

```yaml
    SubmitProjectDemandRequest:
      type: object
      required:
        - title
```

Reviewer fields must not appear under `required`.

- [ ] **Step 2: Confirm frontend type already has optional reviewer fields**

Run:

```bash
rg -n "type SubmitProjectDemandInput|reviewer_user_id\\?|reviewer_selection_reason\\?" apps/web/src/lib/api/projects.ts
```

Expected: both reviewer fields use `?`.

- [ ] **Step 3: Run contract verification**

Run:

```bash
corepack pnpm verify:contracts
```

Expected: pass. If it fails because generated code is stale, run:

```bash
corepack pnpm generate:control-plane
corepack pnpm verify:contracts
```

- [ ] **Step 4: Commit generated contract changes only if files changed**

Run:

```bash
git status --short contracts apps/control-plane apps/web/src/lib/api/projects.ts
```

If generated files changed, commit them:

```bash
git add contracts apps/control-plane apps/web/src/lib/api/projects.ts
git commit -m "chore: refresh project demand contract outputs"
```

If no files changed, do not commit.

---

### Task 7: Verification, Changelog, And Real Smoke

**Files:**
- Modify: `CHANGELOG.md`
- Read: `.codex/skills/superteam-completion-check/SKILL.md`

- [ ] **Step 1: Run the targeted backend tests**

Run:

```bash
go test ./apps/control-plane/internal/project -run 'TestSubmitDemand|TestCreateProject' -count=1
```

Expected: pass.

- [ ] **Step 2: Run the targeted Web tests**

Run:

```bash
corepack pnpm --filter ./apps/web run test -- src/features/projects/index.test.tsx src/features/task-launches/index.test.tsx
```

Expected: pass.

- [ ] **Step 3: Run project hygiene checks**

Run:

```bash
git diff --check
```

Expected: no output.

- [ ] **Step 4: Add changelog entry**

Run:

```bash
TZ=Asia/Shanghai date '+%Y-%m-%d %H:%M'
```

Add this entry near the top of `CHANGELOG.md`, replacing `<timestamp>` with the command output:

```md
- <timestamp> 项目创建流程收敛为负责人池模型：新建项目使用主负责人 + 额外 owner 成员，多来源团队只用于筛选数字员工；任务发起不再要求选择审核人，后端默认派给主负责人。
```

- [ ] **Step 5: Commit changelog and final test-only fixes**

Run:

```bash
git add CHANGELOG.md
git commit -m "docs: record project owner pool flow"
```

If there are no changelog changes because the implementation branch policy skips changelog for this slice, do not create an empty commit.

- [ ] **Step 6: Use the project completion skill**

Read and follow:

```bash
sed -n '1,260p' .codex/skills/superteam-completion-check/SKILL.md
```

Classify the change as cross-layer Web + Control Plane behavior. Unit and component tests are supporting evidence only; final completion needs a real chain smoke unless the human explicitly narrows scope.

- [ ] **Step 7: Run services and real browser/API smoke**

Run:

```bash
scripts/dev-services.sh status
scripts/dev-services.sh restart control-plane
scripts/dev-services.sh restart web
```

Use the Chrome plug/browser automation against the running Web:

1. Open `/projects/new`.
2. Create a project with current user as primary owner, at least one additional human owner, at least two source teams, and at least one selected digital employee.
3. Confirm the project detail shows the human owner pool and digital employee pool.
4. Open `/task-launches`.
5. Select the newly created project.
6. Submit a demand without any reviewer selector.
7. Confirm the demand exists and its reviewer metadata points to `human_owner_user_id`.
8. Confirm no task was created during project creation itself.

- [ ] **Step 8: Final status**

Run:

```bash
git status --short
```

Expected: only unrelated pre-existing dirty files remain. If task-owned files are dirty, inspect and either commit them or explain why they must remain uncommitted.
