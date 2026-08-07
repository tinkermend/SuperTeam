# Project Create Split Console UI Implementation Plan
> 复核状态：已实现（create-project 拆分组件在 apps/web/src/features/projects/components/create-project/）

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the current raw create-project drawer with a production split-console create flow based on `docs/prototypes/project-create-concept-3-split-console.png`, borrowing the digital-employee list pattern from `docs/prototypes/project-create-concept-2-stepped-drawer.png`.

**Architecture:** Keep the backend contract unchanged and continue submitting `CreateProjectInput` to `POST /api/v1/projects`. Move create-project UI into focused draft, stepper, people-role, digital-employee-pool, policy-preset, and review components under `apps/web/src/features/projects/components/create-project/`, then keep `ProjectsView` as the data owner and mutation boundary. The flow creates only the project container; it must not submit a task demand, but the final review must explicitly state that the created project becomes selectable from task launch.

**Tech Stack:** React, TypeScript, TanStack Query, existing SuperTeam v3 components, shadcn/Radix primitives, Vitest browser tests via `corepack pnpm --filter ./apps/web run test`, real browser smoke through Chrome plug after implementation.

---

## Design Source

- Primary target: [project-create-concept-3-split-console.png](../../prototypes/project-create-concept-3-split-console.png)
- Borrowed local pattern: [project-create-concept-2-stepped-drawer.png](../../prototypes/project-create-concept-2-stepped-drawer.png)
- Design system: `DESIGN.md`, `docs/design-system/tokens.md`, `docs/design-system/surfaces.md`, `docs/design-system/forms.md`, `docs/design-system/overlays.md`, `docs/design-system/visual-language.md`

## Scope Boundaries

- Do not add or change Control Plane routes.
- Do not change `CreateProjectInput` field semantics in `apps/web/src/lib/api/projects.ts`.
- Do not submit `ProjectDemand` from create-project UI.
- Do not widen `human_owner_user_id`; it remains the current logged-in creator.
- Do not use raw UUID text inputs for leader or acceptance roles.
- 数字员工池为可选:创建项目容器不强制要求选择数字员工,可后续在项目配置中补充;`canSubmit` 不把数字员工数量作为门槛。
- Do not create a separate task-launch verification UI; final real smoke should create a project, then verify the created project appears in the existing `/task-launches` project selector.
- Code discovery normally prefers codebase-memory MCP graph tools. If `search_graph`, `trace_path`, and `get_code_snippet` are not available in the active tool list, fall back to `rg` and direct file reads and record that in the final verification notes.

## Current Facts To Preserve

- `CreateProjectInput` already supports `team_id`, `name`, `description`, `goal`, `human_owner_user_id`, `leader_user_id`, `acceptance_user_id`, `members`, `coordination_policy`, `approval_policy`, and `evidence_policy`.
- `ProjectMemberInput` supports `principal_type`, `principal_id`, `project_role`, `display_name_snapshot`, and optional `settings`.
- `ProjectMemberInput.project_role` can represent human roles such as `leader`, `acceptance`, `reviewer`, and digital employee `executor`.
- The backend auto-adds the owner member. The UI may include additional explicit members for leader, acceptance, reviewers, and executor pool.
- Current data sources:
  - current user: `getCurrentUser(apiOptions)`
  - authorized teams: `listUserProjectTeamScopes(apiOptions, currentUserId)`
  - users: `listUsers({ baseUrl, fetcher, status: "active", limit, offset, q })`
  - team digital employees: `listDigitalEmployees(apiOptions, { team_id })`
- Existing entry point is `CreateProjectDrawer` mounted from `ProjectsView` in `apps/web/src/features/projects/index.tsx`.

## File Structure

- Modify: `apps/web/src/features/projects/index.tsx`
  - Keep query and mutation ownership.
  - Replace `CreateProjectDrawer` usage with the new split-console component.
  - Pass `apiBaseUrl`, `fetcher`, `currentUser`, authorization state, teams, submit state, and errors.
- Delete or stop using: `apps/web/src/features/projects/components/create-project-drawer.tsx`
  - Only delete after tests are updated and no imports remain.
- Create: `apps/web/src/features/projects/components/create-project/create-project-draft.ts`
  - Draft types, empty draft factory, validation helpers, policy preset definitions, payload mapping helpers.
- Create: `apps/web/src/features/projects/components/create-project/create-project-shell.tsx`
  - Full-screen/split-console visual shell, step navigation, footer, cancel/submit behavior.
- Create: `apps/web/src/features/projects/components/create-project/project-basics-step.tsx`
  - Project name, goal, description, authorized team selection, read-only owner display.
- Create: `apps/web/src/features/projects/components/create-project/project-human-roles-step.tsx`
  - Leader, acceptance, and reviewer selection using active users.
- Create: `apps/web/src/features/projects/components/create-project/project-digital-employees-step.tsx`
  - Team-scoped digital employee pool search/filter/select list.
- Create: `apps/web/src/features/projects/components/create-project/project-policy-step.tsx`
  - Policy preset selector and toggles mapped to existing policy blobs.
- Create: `apps/web/src/features/projects/components/create-project/project-review-panel.tsx`
  - Right-side live review surface with project facts, human responsibility, digital employee pool, policy/audit, and after-create note.
- Create: `apps/web/src/features/projects/components/create-project/index.ts`
  - Public export for `CreateProjectShell`.
- Modify: `apps/web/src/features/projects/index.test.tsx`
  - Replace drawer-specific tests with split-console tests.
  - Keep authorization and submit regression coverage.
- Optional modify: `apps/web/src/components/superteam/user-search-select.tsx`
  - Only if the existing component cannot represent selected/clear state cleanly.
- Modify: `CHANGELOG.md`
  - Add one dated entry when implementation is complete.

---

### Task 1: Draft Model And Payload Mapping

**Files:**
- Create: `apps/web/src/features/projects/components/create-project/create-project-draft.ts`

- [ ] **Step 1: Write the draft helper file**

Create `apps/web/src/features/projects/components/create-project/create-project-draft.ts` with this content:

```ts
import type { UserSummary, UserProjectTeamScope } from "@/lib/api";
import type { DigitalEmployee } from "@/lib/api/employees";
import type { CreateProjectInput, ProjectMemberInput } from "@/lib/api/projects";

export type ProjectCreateStep = "basics" | "roles" | "digitalEmployees" | "policies" | "review";

export const projectCreateSteps: Array<{ id: ProjectCreateStep; label: string }> = [
  { id: "basics", label: "基础信息" },
  { id: "roles", label: "团队与人类角色" },
  { id: "digitalEmployees", label: "数字员工池" },
  { id: "policies", label: "策略预设" },
  { id: "review", label: "确认创建" },
];

export type ProjectPolicyPreset = "standard" | "lightweight" | "highRisk";

export type ProjectCreateDraft = {
  acceptanceUser?: UserSummary;
  description: string;
  goal: string;
  leaderUser?: UserSummary;
  name: string;
  policyPreset: ProjectPolicyPreset;
  policyToggles: {
    auditLogEnabled: boolean;
    budgetOverrunNeedsOwnerApproval: boolean;
    highRiskActionNeedsConfirmation: boolean;
    newDemandNeedsHumanConfirmation: boolean;
    requireEvidenceBeforeAcceptance: boolean;
  };
  reviewerUsers: UserSummary[];
  selectedDigitalEmployees: DigitalEmployee[];
  teamId: string;
};

export const emptyProjectCreateDraft: ProjectCreateDraft = {
  description: "",
  goal: "",
  name: "",
  policyPreset: "standard",
  policyToggles: {
    auditLogEnabled: true,
    budgetOverrunNeedsOwnerApproval: false,
    highRiskActionNeedsConfirmation: true,
    newDemandNeedsHumanConfirmation: true,
    requireEvidenceBeforeAcceptance: true,
  },
  reviewerUsers: [],
  selectedDigitalEmployees: [],
  teamId: "",
};

export function activeSelectableTeams(scopes: UserProjectTeamScope[] | undefined) {
  return (scopes ?? []).filter(
    (scope) => scope.status === "active" && !scope.revoked_at && scope.team.status === "active",
  );
}

export function selectedTeam(scopes: UserProjectTeamScope[] | undefined, teamId: string) {
  return activeSelectableTeams(scopes).find((scope) => scope.team_id === teamId);
}

export function applyPolicyPreset(
  draft: ProjectCreateDraft,
  preset: ProjectPolicyPreset,
): ProjectCreateDraft {
  const policyToggles =
    preset === "lightweight"
      ? {
          auditLogEnabled: true,
          budgetOverrunNeedsOwnerApproval: false,
          highRiskActionNeedsConfirmation: true,
          newDemandNeedsHumanConfirmation: false,
          requireEvidenceBeforeAcceptance: false,
        }
      : preset === "highRisk"
        ? {
            auditLogEnabled: true,
            budgetOverrunNeedsOwnerApproval: true,
            highRiskActionNeedsConfirmation: true,
            newDemandNeedsHumanConfirmation: true,
            requireEvidenceBeforeAcceptance: true,
          }
        : {
            auditLogEnabled: true,
            budgetOverrunNeedsOwnerApproval: false,
            highRiskActionNeedsConfirmation: true,
            newDemandNeedsHumanConfirmation: true,
            requireEvidenceBeforeAcceptance: true,
          };

  return { ...draft, policyPreset: preset, policyToggles };
}

export function projectCreateValidation(
  draft: ProjectCreateDraft,
  currentUserId: string | undefined,
  selectableTeams: UserProjectTeamScope[],
) {
  const authorizedTeamIds = new Set(selectableTeams.map((scope) => scope.team_id));
  return {
    basics: Boolean(draft.name.trim()) && Boolean(draft.goal.trim()) && Boolean(draft.teamId),
    currentUser: Boolean(currentUserId),
    digitalEmployees: draft.selectedDigitalEmployees.length > 0,
    policies: draft.policyToggles.auditLogEnabled,
    teamAuthorized: Boolean(draft.teamId) && authorizedTeamIds.has(draft.teamId),
  };
}

export function buildProjectCreateInput(
  draft: ProjectCreateDraft,
  currentUser: UserSummary,
): CreateProjectInput {
  const members: ProjectMemberInput[] = [];

  if (draft.leaderUser) {
    members.push({
      display_name_snapshot: draft.leaderUser.display_name ?? draft.leaderUser.username,
      principal_id: draft.leaderUser.id,
      principal_type: "human_user",
      project_role: "leader",
    });
  }

  if (draft.acceptanceUser) {
    members.push({
      display_name_snapshot: draft.acceptanceUser.display_name ?? draft.acceptanceUser.username,
      principal_id: draft.acceptanceUser.id,
      principal_type: "human_user",
      project_role: "acceptance",
    });
  }

  for (const reviewer of draft.reviewerUsers) {
    members.push({
      display_name_snapshot: reviewer.display_name ?? reviewer.username,
      principal_id: reviewer.id,
      principal_type: "human_user",
      project_role: "reviewer",
    });
  }

  for (const employee of draft.selectedDigitalEmployees) {
    members.push({
      display_name_snapshot: employee.name,
      principal_id: employee.id,
      principal_type: "digital_employee",
      project_role: "executor",
      settings: {
        role: employee.role,
        risk_level: employee.risk_level,
      },
    });
  }

  return {
    approval_policy: {
      budget_overrun_requires_owner_approval: draft.policyToggles.budgetOverrunNeedsOwnerApproval,
      high_risk_action_requires_confirmation: draft.policyToggles.highRiskActionNeedsConfirmation,
      new_demand_requires_human_confirmation: draft.policyToggles.newDemandNeedsHumanConfirmation,
      preset: draft.policyPreset,
    },
    coordination_policy: {
      audit_log_enabled: draft.policyToggles.auditLogEnabled,
      preset: draft.policyPreset,
    },
    description: draft.description.trim() || undefined,
    evidence_policy: {
      acceptance_requires_evidence: draft.policyToggles.requireEvidenceBeforeAcceptance,
      preset: draft.policyPreset,
    },
    goal: draft.goal.trim(),
    human_owner_user_id: currentUser.id,
    leader_user_id: draft.leaderUser?.id,
    acceptance_user_id: draft.acceptanceUser?.id,
    members,
    name: draft.name.trim(),
    team_id: draft.teamId,
  };
}
```

- [ ] **Step 2: Run TypeScript check for the new file**

Run:

```bash
corepack pnpm --filter ./apps/web exec tsc --noEmit --pretty false
```

Expected: this may fail because the new helper is not imported yet or because `ProjectRole` literal types differ. If it fails on `project_role`, inspect `ProjectRole` in `apps/web/src/lib/api/projects.ts` and update the literals to match the current union.

- [ ] **Step 3: Commit**

```bash
git add apps/web/src/features/projects/components/create-project/create-project-draft.ts
git commit -m "feat(web): add project create draft model"
```

---

### Task 2: Split Console Shell And Basic Step

**Files:**
- Create: `apps/web/src/features/projects/components/create-project/create-project-shell.tsx`
- Create: `apps/web/src/features/projects/components/create-project/project-basics-step.tsx`
- Create: `apps/web/src/features/projects/components/create-project/project-review-panel.tsx`
- Create: `apps/web/src/features/projects/components/create-project/index.ts`
- Modify: `apps/web/src/features/projects/index.tsx`

- [ ] **Step 1: Create the basics step**

Create `apps/web/src/features/projects/components/create-project/project-basics-step.tsx`:

```tsx
import type { UserSummary, UserProjectTeamScope } from "@/lib/api";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { UserIdentity } from "@/components/superteam/user-identity";
import type { ProjectCreateDraft } from "./create-project-draft";

type ProjectBasicsStepProps = {
  currentUser?: UserSummary;
  draft: ProjectCreateDraft;
  isAuthorizationLoading: boolean;
  onChange: (draft: ProjectCreateDraft) => void;
  selectableTeams: UserProjectTeamScope[];
};

export function ProjectBasicsStep({
  currentUser,
  draft,
  isAuthorizationLoading,
  onChange,
  selectableTeams,
}: ProjectBasicsStepProps) {
  return (
    <div className="grid gap-5">
      <div className="grid gap-2">
        <Label htmlFor="project-create-name">项目名称 *</Label>
        <Input
          id="project-create-name"
          maxLength={60}
          onChange={(event) => onChange({ ...draft, name: event.target.value })}
          placeholder="客户接入验收"
          value={draft.name}
        />
        <p className="text-xs text-v3-ink-3">建议使用清晰明确的业务闭环名称，2-60 个字符。</p>
      </div>

      <div className="grid gap-2">
        <Label htmlFor="project-create-goal">项目目标 *</Label>
        <Textarea
          id="project-create-goal"
          onChange={(event) => onChange({ ...draft, goal: event.target.value })}
          placeholder="描述项目背景、预期产出与成功标准，便于对齐与评估。"
          value={draft.goal}
        />
      </div>

      <div className="grid gap-2">
        <Label htmlFor="project-create-description">描述</Label>
        <Textarea
          id="project-create-description"
          onChange={(event) => onChange({ ...draft, description: event.target.value })}
          placeholder="可选：背景、边界、风险、验收说明。"
          value={draft.description}
        />
      </div>

      <div className="grid gap-2">
        <Label htmlFor="project-create-team">授权团队 *</Label>
        <select
          aria-label="授权团队"
          className="h-10 w-full rounded-xl border border-v3-line-strong bg-v3-card px-3 text-sm text-v3-ink outline-none transition focus-visible:border-v3-brand focus-visible:ring-4 focus-visible:ring-v3-brand/10 disabled:cursor-not-allowed disabled:opacity-60"
          disabled={isAuthorizationLoading || selectableTeams.length === 0}
          id="project-create-team"
          onChange={(event) => onChange({ ...draft, teamId: event.target.value, selectedDigitalEmployees: [] })}
          value={draft.teamId}
        >
          {selectableTeams.length === 0 ? <option value="">暂无可选团队</option> : null}
          {selectableTeams.map((scope) => (
            <option key={scope.id} value={scope.team_id}>
              {scope.team.name}
            </option>
          ))}
        </select>
        <p className="text-xs text-v3-ink-3">只展示当前用户被授权用于创建项目的团队。</p>
      </div>

      <div className="grid gap-2">
        <Label>固定负责人（人类）</Label>
        <div className="rounded-xl border border-v3-line bg-v3-card-soft px-3 py-2">
          {currentUser ? (
            <UserIdentity showSecondary user={currentUser} />
          ) : (
            <p className="text-sm text-v3-ink-3">正在加载当前用户...</p>
          )}
        </div>
        <p className="text-xs text-v3-ink-3">项目最终责任人固定为当前创建人，创建后可在项目配置中调整。</p>
      </div>
    </div>
  );
}
```

- [ ] **Step 2: Create the review panel**

Create `apps/web/src/features/projects/components/create-project/project-review-panel.tsx`:

```tsx
import type { ReactNode } from "react";
import { CheckCircle2, CircleDot, Network } from "lucide-react";
import type { UserSummary, UserProjectTeamScope } from "@/lib/api";
import { StatusPill, WorkSurface } from "@/components/superteam";
import type { ProjectCreateDraft } from "./create-project-draft";

type ProjectReviewPanelProps = {
  currentUser?: UserSummary;
  draft: ProjectCreateDraft;
  selectableTeams: UserProjectTeamScope[];
};

export function ProjectReviewPanel({ currentUser, draft, selectableTeams }: ProjectReviewPanelProps) {
  const team = selectableTeams.find((scope) => scope.team_id === draft.teamId);
  const requiredPassed = [
    Boolean(draft.name.trim()) && Boolean(draft.goal.trim()),
    Boolean(team),
    draft.policyToggles.auditLogEnabled,
  ].filter(Boolean).length;

  return (
    <aside className="grid content-start gap-4">
      <WorkSurface className="p-6">
        <div className="mb-5 flex items-start justify-between gap-4">
          <div>
            <h3 className="text-lg font-semibold text-v3-ink">创建前审阅</h3>
            <p className="mt-1 text-sm text-v3-ink-2">以下为将要创建的项目对象预览。</p>
          </div>
          <StatusPill tone="warn">待创建</StatusPill>
        </div>

        <ReviewSection title="项目事实">
          <ReviewRow label="项目名称" value={draft.name || "未填写"} />
          <ReviewRow label="所属团队" value={team?.team.name ?? "未选择"} />
          <ReviewRow label="目标" value={draft.goal || "未填写"} />
        </ReviewSection>

        <ReviewSection title="人类责任">
          <ReviewRow label="固定负责人" value={currentUser?.display_name ?? currentUser?.username ?? "未加载"} />
          <ReviewRow label="项目负责人" value={draft.leaderUser?.display_name ?? draft.leaderUser?.username ?? "未选择"} />
          <ReviewRow label="验收负责人" value={draft.acceptanceUser?.display_name ?? draft.acceptanceUser?.username ?? "未选择"} />
          <ReviewRow label="审核人" value={`${draft.reviewerUsers.length} 位已选`} />
        </ReviewSection>

        <ReviewSection title="数字员工池">
          <ReviewRow label="执行员工" value={`${draft.selectedDigitalEmployees.length} 位已选`} />
        </ReviewSection>

        <ReviewSection title="策略与审计">
          <ReviewRow label="策略预设" value={policyPresetLabel(draft.policyPreset)} />
          <ReviewRow label="审计日志" value={draft.policyToggles.auditLogEnabled ? "自动开启" : "未开启"} />
          <ReviewRow label="证据要求" value={draft.policyToggles.requireEvidenceBeforeAcceptance ? "验收前必须补齐" : "轻量要求"} />
        </ReviewSection>

        <div className="mt-5 rounded-xl border border-v3-info/20 bg-v3-info-soft px-3 py-3 text-sm text-v3-info">
          <div className="flex gap-2">
            <Network className="mt-0.5 size-4 shrink-0" />
            <span>创建完成后，系统会注册项目协调线程，并可在任务发起中选择该项目提交需求。</span>
          </div>
        </div>
      </WorkSurface>

      <div className="rounded-v3-card border border-v3-line bg-v3-card p-5 shadow-sm">
        <div className="flex items-center gap-2 text-sm font-semibold text-v3-ink">
          <CheckCircle2 className="size-4 text-v3-ok" />
          必备项 {requiredPassed} / 3 已就绪
        </div>
        <div className="mt-3 grid gap-2 text-sm text-v3-ink-2">
          <CheckLine checked={Boolean(draft.name.trim()) && Boolean(draft.goal.trim())} label="基础信息已填写" />
          <CheckLine checked={Boolean(team)} label="团队授权有效" />
          <CheckLine checked={draft.policyToggles.auditLogEnabled} label="审计策略已开启" />
          <CheckLine checked={draft.selectedDigitalEmployees.length > 0} label="数字员工池已选择（可选）" />
        </div>
      </div>
    </aside>
  );
}

function ReviewSection({ children, title }: { children: ReactNode; title: string }) {
  return (
    <section className="border-t border-v3-line py-4 first:border-t-0 first:pt-0">
      <h4 className="mb-3 text-sm font-semibold text-v3-ink">{title}</h4>
      <div className="grid gap-2">{children}</div>
    </section>
  );
}

function ReviewRow({ label, value }: { label: string; value: string }) {
  return (
    <div className="grid grid-cols-[7rem_1fr] gap-3 text-sm">
      <span className="text-v3-ink-3">{label}</span>
      <span className="min-w-0 truncate font-medium text-v3-ink">{value}</span>
    </div>
  );
}

function CheckLine({ checked, label }: { checked: boolean; label: string }) {
  const Icon = checked ? CheckCircle2 : CircleDot;
  return (
    <div className="flex items-center gap-2">
      <Icon className={checked ? "size-4 text-v3-ok" : "size-4 text-v3-ink-3"} />
      <span>{label}</span>
    </div>
  );
}

function policyPresetLabel(preset: ProjectCreateDraft["policyPreset"]) {
  if (preset === "lightweight") return "轻量协作";
  if (preset === "highRisk") return "高风险审批";
  return "标准治理";
}
```

- [ ] **Step 3: Create the split-console shell**

Create `apps/web/src/features/projects/components/create-project/create-project-shell.tsx`:

```tsx
import { useEffect, useMemo, useState } from "react";
import { ArrowLeft, ArrowRight, Check, X } from "lucide-react";
import type { UserSummary, UserProjectTeamScope } from "@/lib/api";
import type { CreateProjectInput } from "@/lib/api/projects";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";
import {
  activeSelectableTeams,
  buildProjectCreateInput,
  emptyProjectCreateDraft,
  projectCreateSteps,
  projectCreateValidation,
  type ProjectCreateDraft,
  type ProjectCreateStep,
} from "./create-project-draft";
import { ProjectBasicsStep } from "./project-basics-step";
import { ProjectReviewPanel } from "./project-review-panel";

type CreateProjectShellProps = {
  availableTeams?: UserProjectTeamScope[];
  currentUser?: UserSummary;
  currentUserError?: string;
  isCurrentUserLoading?: boolean;
  isSubmitting?: boolean;
  isTeamsLoading?: boolean;
  onCancel: () => void;
  onSubmit: (input: CreateProjectInput) => void;
  submitError?: string;
  teamsError?: string;
};

export function CreateProjectShell({
  availableTeams,
  currentUser,
  currentUserError,
  isCurrentUserLoading,
  isSubmitting,
  isTeamsLoading,
  onCancel,
  onSubmit,
  submitError,
  teamsError,
}: CreateProjectShellProps) {
  const selectableTeams = useMemo(() => activeSelectableTeams(availableTeams), [availableTeams]);
  const [draft, setDraft] = useState<ProjectCreateDraft>(emptyProjectCreateDraft);
  const [activeStep, setActiveStep] = useState<ProjectCreateStep>("basics");
  const [localError, setLocalError] = useState("");
  const validation = projectCreateValidation(draft, currentUser?.id, selectableTeams);
  const activeIndex = projectCreateSteps.findIndex((step) => step.id === activeStep);
  const isAuthorizationLoading = Boolean(isCurrentUserLoading || isTeamsLoading);
  const authorizationError = currentUserError || teamsError;

  useEffect(() => {
    setDraft((current) => {
      if (current.teamId && selectableTeams.some((scope) => scope.team_id === current.teamId)) {
        return current;
      }
      return { ...current, teamId: selectableTeams[0]?.team_id ?? "", selectedDigitalEmployees: [] };
    });
  }, [selectableTeams]);

  useEffect(() => {
    if (typeof window === "undefined") return;
    function onKey(event: KeyboardEvent) {
      if (event.key === "Escape") onCancel();
    }
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [onCancel]);

  function goNext() {
    if (activeIndex < projectCreateSteps.length - 1) {
      setActiveStep(projectCreateSteps[activeIndex + 1].id);
    }
  }

  function goBack() {
    if (activeIndex > 0) {
      setActiveStep(projectCreateSteps[activeIndex - 1].id);
    }
  }

  function submit() {
    if (!currentUser) {
      setLocalError("当前用户未加载，无法创建项目");
      return;
    }
    if (!validation.basics || !validation.teamAuthorized) {
      setLocalError("请补齐项目名称、目标和授权团队");
      setActiveStep("basics");
      return;
    }
    setLocalError("");
    onSubmit(buildProjectCreateInput(draft, currentUser));
  }

  const canSubmit =
    validation.basics &&
    validation.currentUser &&
    validation.teamAuthorized &&
    !authorizationError &&
    !isAuthorizationLoading;

  return (
    <div
      aria-labelledby="project-create-title"
      aria-modal="true"
      className="fixed inset-0 z-50 overflow-hidden bg-[var(--v3-shell-background)]"
      role="dialog"
    >
      <div className="flex h-full flex-col">
        <header className="border-b border-v3-line bg-v3-card/90 px-8 py-5 backdrop-blur">
          <div className="flex items-start justify-between gap-4">
            <div>
              <p className="text-sm font-medium text-v3-brand">项目管理 / 新建项目</p>
              <h2 className="mt-2 text-3xl font-semibold tracking-tight text-v3-ink" id="project-create-title">新建项目</h2>
              <p className="mt-2 text-sm text-v3-ink-2">建立项目事实容器，配置负责人、团队、数字员工池与策略预设。</p>
            </div>
            <Button aria-label="关闭新建项目" className="size-10 rounded-xl" onClick={onCancel} size="icon" type="button" variant="ghost">
              <X className="size-5" />
            </Button>
          </div>
          <nav aria-label="新建项目步骤" className="mt-8 flex items-center gap-3">
            {projectCreateSteps.map((step, index) => {
              const active = step.id === activeStep;
              const done = index < activeIndex;
              return (
                <button
                  className={cn(
                    "flex min-w-0 items-center gap-2 rounded-xl px-3 py-2 text-sm font-medium transition",
                    active ? "bg-v3-brand-soft text-v3-brand" : "text-v3-ink-2 hover:bg-v3-card-soft",
                  )}
                  key={step.id}
                  onClick={() => setActiveStep(step.id)}
                  type="button"
                >
                  <span
                    className={cn(
                      "grid size-7 shrink-0 place-items-center rounded-full border text-xs",
                      active && "border-v3-brand bg-v3-brand text-white",
                      done && "border-v3-ok bg-v3-ok text-white",
                      !active && !done && "border-v3-line-strong bg-v3-card text-v3-ink-2",
                    )}
                  >
                    {done ? <Check className="size-4" /> : index + 1}
                  </span>
                  <span className="truncate">{step.label}</span>
                </button>
              );
            })}
          </nav>
        </header>

        <main className="grid min-h-0 flex-1 grid-cols-[minmax(0,1fr)_420px] gap-6 overflow-y-auto px-8 py-6">
          <section className="min-w-0 rounded-v3-card border border-v3-line bg-v3-card p-8 shadow-v3">
            {authorizationError ? (
              <div className="mb-5 rounded-xl border border-v3-danger/20 bg-v3-danger-soft px-3 py-2 text-sm text-v3-danger">
                {currentUserError ? "加载当前用户失败" : "加载可选团队失败"}
              </div>
            ) : null}
            {activeStep === "basics" ? (
              <ProjectBasicsStep
                currentUser={currentUser}
                draft={draft}
                isAuthorizationLoading={isAuthorizationLoading}
                onChange={setDraft}
                selectableTeams={selectableTeams}
              />
            ) : (
              <div className="rounded-xl border border-dashed border-v3-line-strong bg-v3-card-soft p-8 text-sm text-v3-ink-2">
                {projectCreateSteps.find((step) => step.id === activeStep)?.label} 步骤将在后续任务中接入。
              </div>
            )}
          </section>

          <ProjectReviewPanel currentUser={currentUser} draft={draft} selectableTeams={selectableTeams} />
        </main>

        <footer className="flex items-center justify-between border-t border-v3-line bg-v3-card px-8 py-4">
          <Button onClick={onCancel} type="button" variant="ghost">
            返回项目列表
          </Button>
          <div className="flex items-center gap-3">
            {(localError || submitError) ? <p className="text-sm text-v3-danger">{localError || submitError}</p> : null}
            <Button disabled={activeIndex === 0} onClick={goBack} type="button" variant="outline">
              <ArrowLeft className="mr-2 size-4" />
              上一步
            </Button>
            {activeStep === "review" ? (
              <Button disabled={isSubmitting || !canSubmit} onClick={submit} type="button">
                确认创建
              </Button>
            ) : (
              <Button onClick={goNext} type="button">
                下一步
                <ArrowRight className="ml-2 size-4" />
              </Button>
            )}
          </div>
        </footer>
      </div>
    </div>
  );
}
```

- [ ] **Step 4: Export the component**

Create `apps/web/src/features/projects/components/create-project/index.ts`:

```ts
export { CreateProjectShell } from "./create-project-shell";
```

- [ ] **Step 5: Mount it from `ProjectsView`**

In `apps/web/src/features/projects/index.tsx`, replace:

```ts
import { CreateProjectDrawer } from "./components/create-project-drawer";
```

with:

```ts
import { CreateProjectShell } from "./components/create-project";
```

Change the current user assignment from:

```ts
const currentUserId = currentUserQuery.data?.user.id;
```

to:

```ts
const currentUser = currentUserQuery.data?.user;
const currentUserId = currentUser?.id;
```

Replace the `<CreateProjectDrawer ... />` block with:

```tsx
{createOpen ? (
  <CreateProjectShell
    availableTeams={availableProjectTeamScopes}
    currentUser={currentUser}
    currentUserError={currentUserQuery.error?.message}
    isCurrentUserLoading={currentUserQuery.isFetching}
    isSubmitting={createMutation.isPending}
    isTeamsLoading={projectTeamScopesQuery.isFetching}
    submitError={createMutation.error?.message}
    teamsError={projectTeamScopesQuery.error?.message}
    onCancel={() => setCreateOpen(false)}
    onSubmit={(input) => createMutation.mutate(input)}
  />
) : null}
```

- [ ] **Step 6: Run targeted web tests**

Run:

```bash
corepack pnpm --filter ./apps/web run test -- src/features/projects/index.test.tsx
```

Expected: several create-project tests fail because button names and labels changed. Keep failures as the red phase for Task 5.

- [ ] **Step 7: Commit**

```bash
git add apps/web/src/features/projects/index.tsx apps/web/src/features/projects/components/create-project
git commit -m "feat(web): introduce split project create shell"
```

---

### Task 3: Human Roles Step

**Files:**
- Create: `apps/web/src/features/projects/components/create-project/project-human-roles-step.tsx`
- Modify: `apps/web/src/features/projects/components/create-project/create-project-shell.tsx`

- [ ] **Step 1: Create the human roles component**

Create `apps/web/src/features/projects/components/create-project/project-human-roles-step.tsx`:

```tsx
import { X } from "lucide-react";
import type { UserSummary } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import { UserIdentity } from "@/components/superteam/user-identity";
import { UserSearchSelect } from "@/components/superteam/user-search-select";
import type { ProjectCreateDraft } from "./create-project-draft";

type ProjectHumanRolesStepProps = {
  apiBaseUrl: string;
  currentUser?: UserSummary;
  draft: ProjectCreateDraft;
  fetcher?: typeof fetch;
  onChange: (draft: ProjectCreateDraft) => void;
};

export function ProjectHumanRolesStep({
  apiBaseUrl,
  currentUser,
  draft,
  fetcher,
  onChange,
}: ProjectHumanRolesStepProps) {
  const excludedUserIds = [
    currentUser?.id,
    draft.leaderUser?.id,
    draft.acceptanceUser?.id,
    ...draft.reviewerUsers.map((user) => user.id),
  ].filter(Boolean) as string[];

  return (
    <div className="grid gap-6">
      <section className="grid gap-2">
        <Label>固定负责人（当前创建人）</Label>
        <div className="rounded-xl border border-v3-line bg-v3-card-soft px-3 py-2">
          {currentUser ? <UserIdentity showSecondary user={currentUser} /> : <p className="text-sm text-v3-ink-3">正在加载当前用户...</p>}
        </div>
      </section>

      <section className="grid gap-2">
        <Label>项目负责人（Leader）</Label>
        <UserSearchSelect
          apiBaseUrl={apiBaseUrl}
          excludedUserIds={excludedUserIds.filter((id) => id !== draft.leaderUser?.id)}
          fetcher={fetcher}
          inputLabel="搜索项目负责人"
          onSelect={(leaderUser) => onChange({ ...draft, leaderUser })}
          placeholder="搜索人类用户作为项目负责人"
          value={draft.leaderUser}
        />
      </section>

      <section className="grid gap-2">
        <Label>验收负责人</Label>
        <UserSearchSelect
          apiBaseUrl={apiBaseUrl}
          excludedUserIds={excludedUserIds.filter((id) => id !== draft.acceptanceUser?.id)}
          fetcher={fetcher}
          inputLabel="搜索验收负责人"
          onSelect={(acceptanceUser) => onChange({ ...draft, acceptanceUser })}
          placeholder="搜索人类用户作为验收负责人"
          value={draft.acceptanceUser}
        />
      </section>

      <section className="grid gap-3">
        <div>
          <Label>审核人</Label>
          <p className="mt-1 text-xs text-v3-ink-3">可选。用于后续审批、补证或风险确认，不替代固定负责人。</p>
        </div>
        <UserSearchSelect
          apiBaseUrl={apiBaseUrl}
          excludedUserIds={excludedUserIds}
          fetcher={fetcher}
          inputLabel="搜索审核人"
          onSelect={(reviewer) => {
            if (draft.reviewerUsers.some((user) => user.id === reviewer.id)) return;
            onChange({ ...draft, reviewerUsers: [...draft.reviewerUsers, reviewer] });
          }}
          placeholder="搜索后添加审核人"
        />
        {draft.reviewerUsers.length > 0 ? (
          <ul className="grid gap-2">
            {draft.reviewerUsers.map((reviewer) => (
              <li className="flex items-center justify-between gap-3 rounded-xl border border-v3-line bg-v3-card-soft px-3 py-2" key={reviewer.id}>
                <UserIdentity showSecondary user={reviewer} />
                <Button
                  aria-label={`移除审核人 ${reviewer.username}`}
                  className="size-8"
                  onClick={() => onChange({ ...draft, reviewerUsers: draft.reviewerUsers.filter((user) => user.id !== reviewer.id) })}
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

- [ ] **Step 2: Pass `apiBaseUrl` and `fetcher` into shell**

Modify `CreateProjectShellProps` in `create-project-shell.tsx`:

```ts
type CreateProjectShellProps = {
  apiBaseUrl: string;
  availableTeams?: UserProjectTeamScope[];
  currentUser?: UserSummary;
  currentUserError?: string;
  fetcher?: typeof fetch;
  isCurrentUserLoading?: boolean;
  isSubmitting?: boolean;
  isTeamsLoading?: boolean;
  onCancel: () => void;
  onSubmit: (input: CreateProjectInput) => void;
  submitError?: string;
  teamsError?: string;
};
```

Add `apiBaseUrl` and `fetcher` to the destructuring.

- [ ] **Step 3: Render the roles step**

In `create-project-shell.tsx`, import:

```ts
import { ProjectHumanRolesStep } from "./project-human-roles-step";
```

Replace the placeholder branch for `activeStep === "roles"` with:

```tsx
{activeStep === "basics" ? (
  <ProjectBasicsStep
    currentUser={currentUser}
    draft={draft}
    isAuthorizationLoading={isAuthorizationLoading}
    onChange={setDraft}
    selectableTeams={selectableTeams}
  />
) : activeStep === "roles" ? (
  <ProjectHumanRolesStep
    apiBaseUrl={apiBaseUrl}
    currentUser={currentUser}
    draft={draft}
    fetcher={fetcher}
    onChange={setDraft}
  />
) : (
  <div className="rounded-xl border border-dashed border-v3-line-strong bg-v3-card-soft p-8 text-sm text-v3-ink-2">
    {projectCreateSteps.find((step) => step.id === activeStep)?.label} 步骤将在后续任务中接入。
  </div>
)}
```

- [ ] **Step 4: Pass props from `ProjectsView`**

In `apps/web/src/features/projects/index.tsx`, update `CreateProjectShell` usage:

```tsx
<CreateProjectShell
  apiBaseUrl={apiBaseUrl}
  availableTeams={availableProjectTeamScopes}
  currentUser={currentUser}
  currentUserError={currentUserQuery.error?.message}
  fetcher={fetcher}
  isCurrentUserLoading={currentUserQuery.isFetching}
  isSubmitting={createMutation.isPending}
  isTeamsLoading={projectTeamScopesQuery.isFetching}
  submitError={createMutation.error?.message}
  teamsError={projectTeamScopesQuery.error?.message}
  onCancel={() => setCreateOpen(false)}
  onSubmit={(input) => createMutation.mutate(input)}
/>
```

- [ ] **Step 5: Run targeted tests**

Run:

```bash
corepack pnpm --filter ./apps/web run test -- src/components/superteam/user-search-select.test.tsx src/features/projects/index.test.tsx
```

Expected: `user-search-select` should still pass. `projects/index.test.tsx` may still fail until Task 5 updates fetch fixtures and expectations.

- [ ] **Step 6: Commit**

```bash
git add apps/web/src/features/projects/components/create-project/project-human-roles-step.tsx apps/web/src/features/projects/components/create-project/create-project-shell.tsx apps/web/src/features/projects/index.tsx
git commit -m "feat(web): add project human role selection"
```

---

### Task 4: Digital Employee Pool And Policy Preset Steps

**Files:**
- Create: `apps/web/src/features/projects/components/create-project/project-digital-employees-step.tsx`
- Create: `apps/web/src/features/projects/components/create-project/project-policy-step.tsx`
- Modify: `apps/web/src/features/projects/components/create-project/create-project-shell.tsx`

- [ ] **Step 1: Create the digital employee pool step**

Create `apps/web/src/features/projects/components/create-project/project-digital-employees-step.tsx`:

```tsx
import type { ReactNode } from "react";
import { useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Check, Search, SlidersHorizontal } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { StatusPill, WorkSurface } from "@/components/superteam";
import { listDigitalEmployees, type DigitalEmployee } from "@/lib/api/employees";
import { cn } from "@/lib/utils";
import type { ProjectCreateDraft } from "./create-project-draft";

type ProjectDigitalEmployeesStepProps = {
  apiBaseUrl: string;
  draft: ProjectCreateDraft;
  fetcher?: typeof fetch;
  onChange: (draft: ProjectCreateDraft) => void;
};

type FilterMode = "all" | "schedulable" | "needsConfig";

export function ProjectDigitalEmployeesStep({
  apiBaseUrl,
  draft,
  fetcher,
  onChange,
}: ProjectDigitalEmployeesStepProps) {
  const [query, setQuery] = useState("");
  const [filterMode, setFilterMode] = useState<FilterMode>("all");
  const employeesQuery = useQuery({
    enabled: Boolean(draft.teamId),
    queryKey: ["project-create", "digital-employees", draft.teamId],
    queryFn: () => listDigitalEmployees({ baseUrl: apiBaseUrl, fetcher }, { team_id: draft.teamId }),
  });
  const selectedIds = useMemo(() => new Set(draft.selectedDigitalEmployees.map((employee) => employee.id)), [draft.selectedDigitalEmployees]);
  const employees = (employeesQuery.data ?? []).filter((employee) => {
    const textMatch = `${employee.name} ${employee.role}`.toLowerCase().includes(query.trim().toLowerCase());
    const modeMatch =
      filterMode === "all" ||
      (filterMode === "schedulable" && employee.status === "active") ||
      (filterMode === "needsConfig" && employee.status !== "active");
    return textMatch && modeMatch;
  });

  function toggleEmployee(employee: DigitalEmployee) {
    if (selectedIds.has(employee.id)) {
      onChange({
        ...draft,
        selectedDigitalEmployees: draft.selectedDigitalEmployees.filter((item) => item.id !== employee.id),
      });
      return;
    }
    onChange({ ...draft, selectedDigitalEmployees: [...draft.selectedDigitalEmployees, employee] });
  }

  return (
    <div className="grid gap-5">
      <div>
        <h3 className="text-xl font-semibold text-v3-ink">选择数字员工池</h3>
        <p className="mt-1 text-sm text-v3-ink-2">仅从项目数字员工池中选取执行员工；人类负责人不归入数字员工。</p>
      </div>

      <div className="flex flex-wrap items-center gap-3">
        <div className="relative min-w-[260px] flex-1">
          <Search className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-v3-ink-3" />
          <Input
            aria-label="搜索数字员工"
            className="pl-9"
            onChange={(event) => setQuery(event.target.value)}
            placeholder="搜索数字员工"
            type="search"
            value={query}
          />
        </div>
        <FilterButton active={filterMode === "all"} onClick={() => setFilterMode("all")}>全部</FilterButton>
        <FilterButton active={filterMode === "schedulable"} onClick={() => setFilterMode("schedulable")}>可调度</FilterButton>
        <FilterButton active={filterMode === "needsConfig"} onClick={() => setFilterMode("needsConfig")}>需配置</FilterButton>
        <Button aria-label="筛选设置" className="size-10 rounded-xl" size="icon" type="button" variant="outline">
          <SlidersHorizontal className="size-4" />
        </Button>
      </div>

      <WorkSurface>
        <div className="grid grid-cols-[3rem_minmax(12rem,1fr)_1fr_7rem] border-b border-v3-line bg-v3-card-soft px-4 py-3 text-xs font-semibold text-v3-ink-2">
          <span />
          <span>数字员工</span>
          <span>能力标签</span>
          <span>状态</span>
        </div>
        {employeesQuery.isLoading ? (
          <p className="px-4 py-6 text-sm text-v3-ink-2">正在加载数字员工...</p>
        ) : employeesQuery.isError ? (
          <p className="px-4 py-6 text-sm text-v3-danger">数字员工加载失败</p>
        ) : employees.length === 0 ? (
          <p className="px-4 py-6 text-sm text-v3-ink-2">{draft.teamId ? "暂无匹配数字员工" : "请先选择授权团队"}</p>
        ) : (
          employees.map((employee) => {
            const selected = selectedIds.has(employee.id);
            return (
              <div
                aria-pressed={selected}
                className="grid w-full cursor-pointer grid-cols-[3rem_minmax(12rem,1fr)_1fr_7rem] items-center border-b border-v3-line px-4 py-3 text-left last:border-b-0 hover:bg-v3-card-soft focus-visible:outline-none focus-visible:ring-4 focus-visible:ring-v3-brand/10"
                key={employee.id}
                onClick={() => toggleEmployee(employee)}
                onKeyDown={(event) => {
                  if (event.key === "Enter" || event.key === " ") {
                    event.preventDefault();
                    toggleEmployee(employee);
                  }
                }}
                role="button"
                tabIndex={0}
              >
                <span
                  aria-hidden="true"
                  className={cn(
                    "grid size-5 place-items-center rounded-md border",
                    selected ? "border-v3-brand bg-v3-brand-soft text-v3-ok" : "border-v3-line-strong bg-v3-card text-v3-ink-3",
                  )}
                >
                  {selected ? <Check className="size-3.5" /> : null}
                </span>
                <div className="min-w-0">
                  <p className="truncate text-sm font-semibold text-v3-ink">{employee.name}</p>
                  <p className="truncate text-xs text-v3-ink-3">{employee.role}</p>
                </div>
                <div className="flex min-w-0 flex-wrap gap-1.5">
                  {employeeCapabilities(employee).map((capability) => (
                    <span className="rounded-lg bg-v3-mute-soft px-2 py-1 text-xs font-medium text-v3-mute" key={capability}>
                      {capability}
                    </span>
                  ))}
                </div>
                <StatusPill tone={employee.status === "active" ? "ok" : "warn"}>
                  {employee.status === "active" ? "可调度" : "待配置"}
                </StatusPill>
              </div>
            );
          })
        )}
      </WorkSurface>

      <div className="rounded-xl border border-v3-warn/20 bg-v3-warn-soft px-3 py-3 text-sm text-v3-warn">
        只选择可参与该项目的执行员工。创建项目不会立即发起任务，也不会自动调度数字员工。
      </div>
    </div>
  );
}

function FilterButton({
  active,
  children,
  onClick,
}: {
  active: boolean;
  children: ReactNode;
  onClick: () => void;
}) {
  return (
    <Button className={cn("rounded-xl", active && "border-v3-brand bg-v3-brand-soft text-v3-brand")} onClick={onClick} type="button" variant="outline">
      {children}
    </Button>
  );
}

function employeeCapabilities(employee: DigitalEmployee) {
  const metadata = employee.metadata ?? {};
  const label = typeof metadata.effective_config_label === "string" ? metadata.effective_config_label : undefined;
  return [employee.employee_type, label, employee.risk_level].filter(Boolean).slice(0, 3);
}
```

- [ ] **Step 2: Create the policy preset step**

Create `apps/web/src/features/projects/components/create-project/project-policy-step.tsx`:

```tsx
import type { ReactNode } from "react";
import { ShieldCheck, Sparkles, TriangleAlert } from "lucide-react";
import { Switch } from "@/components/ui/switch";
import { StatusPill } from "@/components/superteam";
import { cn } from "@/lib/utils";
import { applyPolicyPreset, type ProjectCreateDraft, type ProjectPolicyPreset } from "./create-project-draft";

type ProjectPolicyStepProps = {
  draft: ProjectCreateDraft;
  onChange: (draft: ProjectCreateDraft) => void;
};

const presets: Array<{
  description: string;
  icon: ReactNode;
  id: ProjectPolicyPreset;
  label: string;
}> = [
  { description: "适合多数项目，保留人工确认、证据和审计边界。", icon: <ShieldCheck className="size-4" />, id: "standard", label: "标准治理" },
  { description: "降低前置确认，适合低风险协作和试运行。", icon: <Sparkles className="size-4" />, id: "lightweight", label: "轻量协作" },
  { description: "强化审批、证据和预算阈值，适合高风险项目。", icon: <TriangleAlert className="size-4" />, id: "highRisk", label: "高风险审批" },
];

export function ProjectPolicyStep({ draft, onChange }: ProjectPolicyStepProps) {
  return (
    <div className="grid gap-6">
      <div>
        <h3 className="text-xl font-semibold text-v3-ink">策略预设</h3>
        <p className="mt-1 text-sm text-v3-ink-2">定义创建后的审批、预算、证据与验收边界。</p>
      </div>

      <div className="grid gap-3 sm:grid-cols-3">
        {presets.map((preset) => {
          const active = draft.policyPreset === preset.id;
          return (
            <button
              className={cn(
                "rounded-v3-inner border bg-v3-card p-4 text-left transition hover:border-v3-brand/50",
                active ? "border-v3-brand ring-4 ring-v3-brand/10" : "border-v3-line",
              )}
              key={preset.id}
              onClick={() => onChange(applyPolicyPreset(draft, preset.id))}
              type="button"
            >
              <div className="flex items-center gap-2 text-sm font-semibold text-v3-ink">
                <span className={cn("grid size-8 place-items-center rounded-xl", active ? "bg-v3-brand-soft text-v3-brand" : "bg-v3-mute-soft text-v3-mute")}>
                  {preset.icon}
                </span>
                {preset.label}
              </div>
              <p className="mt-3 text-xs leading-5 text-v3-ink-2">{preset.description}</p>
            </button>
          );
        })}
      </div>

      <div className="divide-y divide-v3-line rounded-v3-inner border border-v3-line bg-v3-card">
        <PolicyToggle
          checked={draft.policyToggles.newDemandNeedsHumanConfirmation}
          description="任何新需求在执行前需由负责人或审批人确认。"
          label="新需求需要人工确认"
          onCheckedChange={(checked) => onChange({ ...draft, policyToggles: { ...draft.policyToggles, newDemandNeedsHumanConfirmation: checked } })}
        />
        <PolicyToggle
          checked={draft.policyToggles.highRiskActionNeedsConfirmation}
          description="涉及数据删除、权限变更、外部调用等高风险动作需暂停确认。"
          label="高风险动作暂停等待确认"
          onCheckedChange={(checked) => onChange({ ...draft, policyToggles: { ...draft.policyToggles, highRiskActionNeedsConfirmation: checked } })}
        />
        <PolicyToggle
          checked={draft.policyToggles.requireEvidenceBeforeAcceptance}
          description="验收前必须补齐产出、测试、日志或审计证据。"
          label="验收前必须补齐证据"
          onCheckedChange={(checked) => onChange({ ...draft, policyToggles: { ...draft.policyToggles, requireEvidenceBeforeAcceptance: checked } })}
        />
        <PolicyToggle
          checked={draft.policyToggles.budgetOverrunNeedsOwnerApproval}
          description="实际消耗超过预算阈值时，需要负责人审批后继续。"
          label="预算超限需负责人审批"
          onCheckedChange={(checked) => onChange({ ...draft, policyToggles: { ...draft.policyToggles, budgetOverrunNeedsOwnerApproval: checked } })}
        />
      </div>

      <div className="flex flex-wrap gap-2">
        <StatusPill tone="info">审批策略</StatusPill>
        <StatusPill tone="artifact">工作契约</StatusPill>
        <StatusPill tone="warn">证据要求</StatusPill>
        <StatusPill tone="ok">审计默认开启</StatusPill>
      </div>
    </div>
  );
}

function PolicyToggle({
  checked,
  description,
  label,
  onCheckedChange,
}: {
  checked: boolean;
  description: string;
  label: string;
  onCheckedChange: (checked: boolean) => void;
}) {
  return (
    <div className="flex items-center justify-between gap-4 px-4 py-3">
      <div>
        <p className="text-sm font-semibold text-v3-ink">{label}</p>
        <p className="mt-1 text-xs text-v3-ink-3">{description}</p>
      </div>
      <Switch checked={checked} onCheckedChange={onCheckedChange} />
    </div>
  );
}
```

- [ ] **Step 3: Render both steps in shell**

In `create-project-shell.tsx`, import:

```ts
import { ProjectDigitalEmployeesStep } from "./project-digital-employees-step";
import { ProjectPolicyStep } from "./project-policy-step";
```

Extend the conditional branch:

```tsx
) : activeStep === "digitalEmployees" ? (
  <ProjectDigitalEmployeesStep
    apiBaseUrl={apiBaseUrl}
    draft={draft}
    fetcher={fetcher}
    onChange={setDraft}
  />
) : activeStep === "policies" ? (
  <ProjectPolicyStep draft={draft} onChange={setDraft} />
) : activeStep === "review" ? (
  <div className="grid gap-4">
    <h3 className="text-xl font-semibold text-v3-ink">确认创建</h3>
    <p className="text-sm text-v3-ink-2">请在右侧审阅项目事实、人类责任、数字员工池和策略配置。确认后只创建项目容器，不发起任务。</p>
  </div>
) : (
```

- [ ] **Step 4: Run targeted tests**

Run:

```bash
corepack pnpm --filter ./apps/web run test -- src/features/projects/index.test.tsx
```

Expected: failures should now be limited to tests still expecting old drawer labels or old raw UUID fields.

- [ ] **Step 5: Commit**

```bash
git add apps/web/src/features/projects/components/create-project
git commit -m "feat(web): add project employee pool and policy steps"
```

---

### Task 5: Update Project Tests And Fetch Fixtures

**Files:**
- Modify: `apps/web/src/features/projects/index.test.tsx`

- [ ] **Step 1: Add users and digital employees fixtures**

Near constants in `apps/web/src/features/projects/index.test.tsx`, add:

```ts
const LEADER_USER_ID = "leader-user-1";
const ACCEPTANCE_USER_ID = "acceptance-user-1";
const REVIEWER_USER_ID = "reviewer-user-1";
const EMPLOYEE_ASSISTANT_ID = "employee-assistant-1";
const EMPLOYEE_QA_ID = "employee-qa-1";
```

Add helper functions below `userProjectTeamScopesResponse()`:

```ts
function usersResponse() {
  return {
    items: [
      {
        avatar: { provider: "dicebear", seed: "leader", style: "adventurer" },
        avatar_asset_id: null,
        display_name: "李娜",
        email: "leader@example.com",
        id: LEADER_USER_ID,
        status: "active",
        username: "leader",
      },
      {
        avatar: { provider: "dicebear", seed: "acceptance", style: "adventurer" },
        avatar_asset_id: null,
        display_name: "王磊",
        email: "acceptance@example.com",
        id: ACCEPTANCE_USER_ID,
        status: "active",
        username: "acceptance",
      },
      {
        avatar: { provider: "dicebear", seed: "reviewer", style: "adventurer" },
        avatar_asset_id: null,
        display_name: "赵明",
        email: "reviewer@example.com",
        id: REVIEWER_USER_ID,
        status: "active",
        username: "reviewer",
      },
    ],
  };
}

function digitalEmployeesResponse() {
  return [
    {
      approval_policy: {},
      context_policy: {},
      created_at: "2026-06-01T00:00:00Z",
      employee_type: "delivery",
      id: EMPLOYEE_ASSISTANT_ID,
      metadata: { effective_config_label: "代码实现" },
      name: "研发助手",
      owner_user_id: CURRENT_USER_ID,
      permission_policy: {},
      risk_level: "medium",
      role: "rd-assistant",
      status: "active",
      team_id: TEAM_REVIEW_ID,
      tenant_id: "tenant-1",
    },
    {
      approval_policy: {},
      context_policy: {},
      created_at: "2026-06-01T00:00:00Z",
      employee_type: "qa",
      id: EMPLOYEE_QA_ID,
      metadata: { effective_config_label: "自动化测试" },
      name: "测试工程师",
      owner_user_id: CURRENT_USER_ID,
      permission_policy: {},
      risk_level: "low",
      role: "qa-engineer",
      status: "active",
      team_id: TEAM_REVIEW_ID,
      tenant_id: "tenant-1",
    },
  ];
}
```

- [ ] **Step 2: Extend fake fetcher**

Inside `createProjectFetcher`, before `/api/v1/projects`, add:

```ts
if (url.pathname === "/api/auth/users" && method === "GET") {
  return jsonResponse(usersResponse());
}

if (url.pathname === "/api/v1/digital-employees" && method === "GET") {
  return jsonResponse(digitalEmployeesResponse());
}
```

- [ ] **Step 3: Replace old create submit test**

Replace the old test named `"creates a project with a selected authorized team and the current user as owner"` with:

```tsx
it("creates a project from the split console with roles, employees, and policies", async () => {
  const fetcher = createProjectFetcher();
  const screen = await renderProjects(fetcher);

  await userEvent.click(screen.getByRole("button", { name: "新建项目" }));
  await userEvent.fill(screen.getByLabelText("项目名称 *"), "客户验收推进");
  await userEvent.fill(screen.getByLabelText("项目目标 *"), "完成客户验收闭环");
  await userEvent.selectOptions(screen.getByLabelText("授权团队"), TEAM_REVIEW_ID);

  await userEvent.click(screen.getByRole("button", { name: "下一步" }));
  await userEvent.fill(screen.getByLabelText("搜索项目负责人"), "李娜");
  await userEvent.click(screen.getByRole("button", { name: "选择 leader" }));
  await userEvent.fill(screen.getByLabelText("搜索验收负责人"), "王磊");
  await userEvent.click(screen.getByRole("button", { name: "选择 acceptance" }));

  await userEvent.click(screen.getByRole("button", { name: "下一步" }));
  await userEvent.click(screen.getByText("研发助手"));
  await userEvent.click(screen.getByText("测试工程师"));

  await userEvent.click(screen.getByRole("button", { name: "下一步" }));
  await userEvent.click(screen.getByRole("button", { name: /高风险审批/ }));

  await userEvent.click(screen.getByRole("button", { name: "下一步" }));
  await userEvent.click(screen.getByRole("button", { name: "确认创建" }));

  await vi.waitFor(() => {
    const postCall = fetchCalls(fetcher).find(([url, init]) => {
      return String(url).endsWith("/api/v1/projects") && init?.method === "POST";
    });
    expect(postCall).toBeTruthy();
    expect(JSON.parse(String(postCall?.[1]?.body))).toMatchObject({
      acceptance_user_id: ACCEPTANCE_USER_ID,
      human_owner_user_id: CURRENT_USER_ID,
      leader_user_id: LEADER_USER_ID,
      name: "客户验收推进",
      team_id: TEAM_REVIEW_ID,
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
    const body = JSON.parse(String(postCall?.[1]?.body));
    expect(body.members).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          principal_id: LEADER_USER_ID,
          principal_type: "human_user",
          project_role: "leader",
        }),
        expect.objectContaining({
          principal_id: ACCEPTANCE_USER_ID,
          principal_type: "human_user",
          project_role: "acceptance",
        }),
        expect.objectContaining({
          principal_id: EMPLOYEE_ASSISTANT_ID,
          principal_type: "digital_employee",
          project_role: "executor",
        }),
      ]),
    );
  });
});
```

`UserSearchSelect` 的选择按钮 accessible name 是 `选择 ${username}`(已核验 `apps/web/src/components/superteam/user-search-select.tsx`),因此 fixture 里 username 取 `leader`/`acceptance`,匹配 `选择 leader` / `选择 acceptance`。

- [ ] **Step 4: Update loading/error/unauthorized tests**

Replace old assertions that look for `创建项目` submit button with `确认创建` only when the flow reaches review. For initial disabled authorization tests, assert visible state instead:

```tsx
await expect.element(screen.getByText("正在加载当前用户...")).toBeInTheDocument();
await expect.element(screen.getByText("固定负责人（人类）")).toBeInTheDocument();
```

For no selectable teams:

```tsx
await expect.element(screen.getByText("暂无可选团队")).toBeInTheDocument();
await userEvent.click(screen.getByRole("button", { name: "下一步" }));
await userEvent.click(screen.getByRole("button", { name: "下一步" }));
await userEvent.click(screen.getByRole("button", { name: "下一步" }));
await userEvent.click(screen.getByRole("button", { name: "下一步" }));
await expect.element(screen.getByRole("button", { name: "确认创建" })).toBeDisabled();
```

- [ ] **Step 5: Run targeted tests until green**

Run:

```bash
corepack pnpm --filter ./apps/web run test -- src/features/projects/index.test.tsx
```

Expected: PASS.

- [ ] **Step 6: Run navigation/design adjacent tests**

Run:

```bash
corepack pnpm --filter ./apps/web run test -- src/navigation-rules.test.ts src/components/superteam/user-search-select.test.tsx src/components/superteam/v3-components.test.tsx
```

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add apps/web/src/features/projects/index.test.tsx
git commit -m "test(web): cover split project creation flow"
```

---

### Task 6: Remove Old Drawer And Polish Responsive Layout

**Files:**
- Delete: `apps/web/src/features/projects/components/create-project-drawer.tsx`
- Modify: `apps/web/src/features/projects/components/create-project/*.tsx`
- Modify: `apps/web/src/features/projects/index.tsx`

- [ ] **Step 1: Confirm old drawer is unused**

Run:

```bash
rg -n "CreateProjectDrawer|create-project-drawer" apps/web/src
```

Expected: no results except the file itself. If imports remain, remove them before deletion.

- [ ] **Step 2: Delete old drawer**

Use `apply_patch`:

```diff
*** Begin Patch
*** Delete File: apps/web/src/features/projects/components/create-project-drawer.tsx
*** End Patch
```

- [ ] **Step 3: Add responsive classes**

In `create-project-shell.tsx`, change the main grid class from:

```tsx
<main className="grid min-h-0 flex-1 grid-cols-[minmax(0,1fr)_420px] gap-6 overflow-y-auto px-8 py-6">
```

to:

```tsx
<main className="grid min-h-0 flex-1 grid-cols-1 gap-6 overflow-y-auto px-4 py-5 lg:grid-cols-[minmax(0,1fr)_420px] lg:px-8 lg:py-6">
```

Change the header padding from:

```tsx
<header className="border-b border-v3-line bg-v3-card/90 px-8 py-5 backdrop-blur">
```

to:

```tsx
<header className="border-b border-v3-line bg-v3-card/90 px-4 py-5 backdrop-blur lg:px-8">
```

Change the footer class from:

```tsx
<footer className="flex items-center justify-between border-t border-v3-line bg-v3-card px-8 py-4">
```

to:

```tsx
<footer className="flex flex-col gap-3 border-t border-v3-line bg-v3-card px-4 py-4 sm:flex-row sm:items-center sm:justify-between lg:px-8">
```

- [ ] **Step 4: Run full web verification**

Run:

```bash
corepack pnpm --filter ./apps/web run test
corepack pnpm --filter ./apps/web run typecheck
corepack pnpm --filter ./apps/web run build
git diff --check
```

Expected: all pass.

- [ ] **Step 5: Commit**

```bash
git add apps/web/src/features/projects apps/web/src/features/projects/components/create-project-drawer.tsx
git commit -m "refactor(web): remove legacy project create drawer"
```

---

### Task 7: Changelog And Real-Chain Verification

**Files:**
- Modify: `CHANGELOG.md`

- [ ] **Step 1: Add changelog entry**

Run:

```bash
TZ=Asia/Shanghai date '+%Y-%m-%d %H:%M'
```

Add one entry near the top of `CHANGELOG.md` using the returned timestamp:

```md
- YYYY-MM-DD HH:MM: 项目管理新建项目改为分屏配置台，支持授权团队、人类角色、数字员工池、策略预设和创建前审阅。
```

- [ ] **Step 2: Check services**

Run:

```bash
scripts/dev-services.sh status
```

Expected: Web and Control Plane are running. If either is stopped or stale, restart only the needed service:

```bash
scripts/dev-services.sh restart web
scripts/dev-services.sh restart control-plane
```

- [ ] **Step 3: Real browser smoke**

Use Chrome plug/browser automation, not Playwright install. Verify:

1. Open running Web URL from `scripts/dev-services.sh status`.
2. Log in if needed.
3. Navigate to 项目管理.
4. Click 新建项目.
5. Fill project name and goal.
6. Select an authorized team.
7. Select leader and acceptance users through search.
8. 选择至少一位可调度数字员工(用于验证数字员工步骤;创建本身不强制要求选择数字员工)。
9. Choose 标准治理 or 高风险审批.
10. Confirm creation.
11. Verify the project appears selected or visible in project management.
12. Navigate to `/task-launches`.
13. Verify the newly created project is available in the project selector.

Record the exact URL, selected team, created project name, and final page evidence in the final response.

- [ ] **Step 4: Final common checks**

Run:

```bash
corepack pnpm --filter ./apps/web run test
corepack pnpm --filter ./apps/web run typecheck
corepack pnpm --filter ./apps/web run build
git diff --check
```

Expected: all pass.

- [ ] **Step 5: Commit**

```bash
git add CHANGELOG.md
git commit -m "chore: document project create split console"
```

---

## Self-Review Notes

- Spec coverage: plan covers the selected split-console layout, the borrowed drawer-style digital employee list, owner fixed to current user, human role search, team authorization, policy blobs, final review, and task-launch separation.
- No backend work is planned because current `CreateProjectInput` already supports the required fields.
- Real-chain verification is required before claiming this feature usable because this is visible Web UI that changes the real project creation payload and must prove downstream task-launch selection still works.
- If the implementation discovers backend validation rejects one of the policy blob keys, keep the UI and adjust only the mapped JSON key names in `buildProjectCreateInput`; do not add a new backend contract for this UI pass unless the user explicitly expands scope.
- 后端已核验(`apps/control-plane/internal/project/service.go` 的 `ensureOwnerMember` 只自动补 owner 成员;`leader_user_id`/`acceptance_user_id` 仅作为 `projects` 标量列存储,不落成员行;唯一索引 `uq_project_members_principal_role` 建在 `(tenant_id, project_id, principal_type, principal_id, project_role)` 上)。因此 `buildProjectCreateInput` 同时下发顶层 `leader_user_id`/`acceptance_user_id` 与对应 `members[]` 条目是安全且必要的(否则人类 leader/acceptance 不会进项目花名册);只需避免在 `members[]` 里下发完全相同的 `(principal_type, principal_id, project_role)` 三元组。
- 已移除 `coordination_policy.project_create_ui` 这一没有消费方的键,避免与"不改契约语义"的边界冲突;三个 policy blob 在前端均为 `Record<string, unknown>`,保留 `preset` 作为可读标记是安全的。
- 全屏 overlay 用 `bg-[var(--v3-shell-background)]`(该 token 未进 Tailwind `@theme`,必须走任意值语法,直接写 `bg-v3-shell-background` 不会生成任何 CSS),并补上 `role="dialog"` / `aria-modal="true"` / `aria-labelledby` 与 Esc 关闭;完整的焦点陷阱如需补强,建议后续用 Radix `DialogContent` 包裹。
- 数字员工池为可选:`canSubmit` 不把数字员工数量作为门槛,审阅面板的"必备项"计数只覆盖基础信息、团队授权与审计策略三项,数字员工以"可选"勾展示;真实 smoke 仍建议选至少一位以验证数字员工步骤与 `members[]` 下发。
