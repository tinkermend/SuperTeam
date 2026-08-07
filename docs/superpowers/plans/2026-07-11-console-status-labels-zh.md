# Console Status Labels ZH Implementation Plan
> 复核状态：已实现（基于CHANGELOG证据）

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make team-management status UI show Simplified Chinese via a shared label module, and collapse duplicated project lifecycle label maps into the same source.

**Architecture:** Extend `apps/web/src/lib/status-labels.ts` as the single Console status-copy source: unambiguous codes live in `STATUS_LABELS` / `statusLabel()`; ambiguous codes (`active`, governance-only codes, employee `error`) are overridden by thin domain helpers (`teamStatusLabel`, `governanceStatusLabel`, `employeeStatusLabel`, `projectStatusLabel`). Team and project UI call those helpers instead of raw English enums or local maps. Tone stays local.

**Tech Stack:** React + TypeScript in `apps/web`; Vitest via `corepack pnpm --filter @superteam/web test`; no API/contract changes.

**Spec:** `docs/superpowers/specs/2026-07-11-console-status-labels-zh-design.md`

## Global Constraints

- UI-only: do not change API enum values or OpenAPI contracts.
- Single source: `apps/web/src/lib/status-labels.ts`.
- Shared default `active` = `"启用中"`; team/governance/employee/project lifecycle displays must use domain helpers, not the shared default.
- Do not unify `StatusPill` tone in this plan.
- Do not migrate `projectPhaseLabel` / `demandStatusLabel` into the shared module (keep local in `project-operational-detail.tsx`).
- Out of scope: employee detail header, runtime overview, skills, event narrative copy, i18n framework.
- Tests: `corepack pnpm --filter @superteam/web test -- <path>` (never `npx vitest run`).
- Gate before claiming done: `corepack pnpm verify:web` plus browser smoke of team detail + one project status surface.
- Commits only when the user explicitly asks (plan steps may stage a message; do not auto-commit).

---

## File Structure

- Create: `apps/web/src/lib/status-labels.test.ts` — unit coverage for shared + domain helpers
- Modify: `apps/web/src/lib/status-labels.ts` — add missing codes + domain helpers
- Modify: `apps/web/src/features/teams/components/team-detail-layout.tsx` — `teamStatusLabel`
- Modify: `apps/web/src/features/teams/components/team-overview-tab.tsx` — `employeeStatusLabel`
- Modify: `apps/web/src/features/teams/components/team-capabilities-tab.tsx` — `statusLabel` for binding
- Modify: `apps/web/src/features/teams/components/create-team-step-members.tsx` — `employeeStatusLabel` + local tone map only
- Modify: `apps/web/src/features/teams/components/create-team-digital-employees-step.tsx` — `employeeStatusLabel`
- Modify: `apps/web/src/features/teams/components/team-management-toolbar.tsx` — filter labels from helpers
- Modify: `apps/web/src/features/projects/components/project-config-page.tsx` — remove local map; use `projectStatusLabel`
- Modify: `apps/web/src/features/projects/components/project-risk-home.tsx` — remove local map; use `projectStatusLabel` for display + filter options
- Modify: `apps/web/src/features/projects/components/project-operational-detail.tsx` — remove local `projectStatusLabel`; import shared

---

### Task 1: Extend `status-labels` + unit tests

**Files:**
- Create: `apps/web/src/lib/status-labels.test.ts`
- Modify: `apps/web/src/lib/status-labels.ts`

**Interfaces:**
- Produces:
  - `statusLabel(status: string | undefined): string`
  - `teamStatusLabel(status: string | undefined): string`
  - `governanceStatusLabel(status: string | undefined): string`
  - `employeeStatusLabel(status: string | undefined): string`
  - `projectStatusLabel(status: string | undefined): string`
- Consumes: existing `STATUS_LABELS` / `statusLabel` behavior for non-overridden codes

- [ ] **Step 1: Write failing tests**

Create `apps/web/src/lib/status-labels.test.ts`:

```ts
import { describe, expect, it } from "vitest";
import {
  employeeStatusLabel,
  governanceStatusLabel,
  projectStatusLabel,
  statusLabel,
  teamStatusLabel,
} from "./status-labels";

describe("statusLabel", () => {
  it("maps shared codes and defaults active to 启用中", () => {
    expect(statusLabel("active")).toBe("启用中");
    expect(statusLabel("archived")).toBe("已归档");
    expect(statusLabel("paused")).toBe("已暂停");
    expect(statusLabel("configuring")).toBe("配置中");
    expect(statusLabel("acceptance")).toBe("验收中");
    expect(statusLabel("error")).toBe("异常");
    expect(statusLabel("  READY ")).toBe("就绪");
    expect(statusLabel(undefined)).toBe("未知");
    expect(statusLabel("totally_unknown")).toBe("totally_unknown");
  });
});

describe("domain overrides", () => {
  it("teamStatusLabel overrides active", () => {
    expect(teamStatusLabel("active")).toBe("活跃");
    expect(teamStatusLabel("disabled")).toBe("已禁用");
    expect(teamStatusLabel("archived")).toBe("已归档");
  });

  it("governanceStatusLabel overrides active and governance-only codes", () => {
    expect(governanceStatusLabel("active")).toBe("已生效");
    expect(governanceStatusLabel("not_configured")).toBe("未配置");
    expect(governanceStatusLabel("draft_pending")).toBe("草案待批准");
    expect(governanceStatusLabel("needs_update")).toBe("需更新");
  });

  it("employeeStatusLabel overrides active and error", () => {
    expect(employeeStatusLabel("active")).toBe("运行中");
    expect(employeeStatusLabel("error")).toBe("异常");
    expect(employeeStatusLabel("draft")).toBe("草稿");
    expect(employeeStatusLabel("ready")).toBe("就绪");
    expect(employeeStatusLabel("disabled")).toBe("已禁用");
  });

  it("projectStatusLabel covers lifecycle codes", () => {
    expect(projectStatusLabel("draft")).toBe("草稿");
    expect(projectStatusLabel("configuring")).toBe("配置中");
    expect(projectStatusLabel("running")).toBe("运行中");
    expect(projectStatusLabel("paused")).toBe("已暂停");
    expect(projectStatusLabel("acceptance")).toBe("验收中");
    expect(projectStatusLabel("archived")).toBe("已归档");
  });
});
```

- [ ] **Step 2: Run tests — expect FAIL**

```bash
corepack pnpm --filter @superteam/web test -- src/lib/status-labels.test.ts
```

Expected: FAIL — missing exports and/or missing shared codes (`archived`, `paused`, `configuring`, `acceptance`, `error`).

- [ ] **Step 3: Implement helpers**

In `apps/web/src/lib/status-labels.ts`:

1. Add to `STATUS_LABELS` (alphabetically with neighbors):

```ts
  acceptance: "验收中",
  archived: "已归档",
  configuring: "配置中",
  error: "异常",
  paused: "已暂停",
```

(`active` stays `"启用中"`; `draft` / `disabled` / `ready` / `running` already exist.)

2. Add helpers after existing exports:

```ts
function labelWithOverrides(
  status: string | undefined,
  overrides: Record<string, string>,
): string {
  if (!status) {
    return "未知";
  }
  const normalized = status.trim().toLowerCase();
  return overrides[normalized] ?? statusLabel(normalized);
}

export function teamStatusLabel(status: string | undefined): string {
  return labelWithOverrides(status, {
    active: "活跃",
    archived: "已归档",
    disabled: "已禁用",
  });
}

export function governanceStatusLabel(status: string | undefined): string {
  return labelWithOverrides(status, {
    active: "已生效",
    draft_pending: "草案待批准",
    needs_update: "需更新",
    not_configured: "未配置",
  });
}

export function employeeStatusLabel(status: string | undefined): string {
  return labelWithOverrides(status, {
    active: "运行中",
    error: "异常",
  });
}

export function projectStatusLabel(status: string | undefined): string {
  return statusLabel(status);
}
```

- [ ] **Step 4: Run tests — expect PASS**

```bash
corepack pnpm --filter @superteam/web test -- src/lib/status-labels.test.ts
```

Expected: PASS.

- [ ] **Step 5: Commit (only if user asked)**

```bash
git add apps/web/src/lib/status-labels.ts apps/web/src/lib/status-labels.test.ts
git commit -m "$(cat <<'EOF'
feat(web): add domain status label helpers for zh display

EOF
)"
```

---

### Task 2: Wire team management UI to helpers

**Files:**
- Modify: `apps/web/src/features/teams/components/team-detail-layout.tsx`
- Modify: `apps/web/src/features/teams/components/team-overview-tab.tsx`
- Modify: `apps/web/src/features/teams/components/team-capabilities-tab.tsx`
- Modify: `apps/web/src/features/teams/components/create-team-step-members.tsx`
- Modify: `apps/web/src/features/teams/components/create-team-digital-employees-step.tsx`
- Modify: `apps/web/src/features/teams/components/team-management-toolbar.tsx`
- Test: `apps/web/src/features/teams/index.test.tsx` (and create-team tests if they assert English status)

**Interfaces:**
- Consumes: `teamStatusLabel`, `governanceStatusLabel`, `employeeStatusLabel`, `statusLabel` from Task 1

- [ ] **Step 1: Update `team-detail-layout.tsx`**

Import `teamStatusLabel`. Replace local label map; keep tone map:

```tsx
import { teamStatusLabel } from "@/lib/status-labels";

function TeamStatusPill({ status }: { status: TeamStatus }) {
  const tone: Record<TeamStatus, "ok" | "mute" | "warn"> = {
    active: "ok",
    archived: "mute",
    disabled: "warn",
  };

  return (
    <StatusPill tone={tone[status]}>
      {teamStatusLabel(status)}
    </StatusPill>
  );
}
```

- [ ] **Step 2: Update overview + capabilities**

In `team-overview-tab.tsx`:

```tsx
import { employeeStatusLabel } from "@/lib/status-labels";
// ...
<StatusPill tone={employee.status === "active" ? "ok" : "warn"}>
  {employeeStatusLabel(employee.status)}
</StatusPill>
```

In `team-capabilities-tab.tsx`:

```tsx
import { statusLabel } from "@/lib/status-labels";
// ...
<StatusPill tone={binding.status === "active" ? "ok" : "warn"}>
  {statusLabel(binding.status)}
</StatusPill>
```

- [ ] **Step 3: Update create-team employee status displays**

In `create-team-step-members.tsx`, replace `EMPLOYEE_STATUS_PRESENTATION` with tone-only map + helper:

```tsx
import { employeeStatusLabel } from "@/lib/status-labels";
import type { V3Tone } from "@/components/superteam/v3-components";

const EMPLOYEE_STATUS_TONE: Record<DigitalEmployeeStatus, V3Tone> = {
  draft: "mute",
  ready: "ok",
  active: "info",
  disabled: "mute",
  error: "danger",
};

// in render:
const tone = EMPLOYEE_STATUS_TONE[employee.status] ?? "mute";
// ...
<StatusPill className="shrink-0" tone={tone}>
  {employeeStatusLabel(employee.status)}
</StatusPill>
```

In `create-team-digital-employees-step.tsx`:

```tsx
import { employeeStatusLabel } from "@/lib/status-labels";
// ...
状态: {employeeStatusLabel(employee.status)}
```

- [ ] **Step 4: Update toolbar filter labels**

In `team-management-toolbar.tsx`:

```tsx
import { governanceStatusLabel, teamStatusLabel } from "@/lib/status-labels";
```

Replace hard-coded `SelectItem` children:

```tsx
<SelectItem value="active">{teamStatusLabel("active")}</SelectItem>
<SelectItem value="disabled">{teamStatusLabel("disabled")}</SelectItem>
<SelectItem value="archived">{teamStatusLabel("archived")}</SelectItem>
```

```tsx
<SelectItem value="not_configured">{governanceStatusLabel("not_configured")}</SelectItem>
<SelectItem value="draft_pending">{governanceStatusLabel("draft_pending")}</SelectItem>
<SelectItem value="active">{governanceStatusLabel("active")}</SelectItem>
<SelectItem value="needs_update">{governanceStatusLabel("needs_update")}</SelectItem>
```

Keep `"全部状态"` / `"全部治理"` as literal UI chrome (not status codes).

- [ ] **Step 5: Run team tests**

```bash
corepack pnpm --filter @superteam/web test -- src/features/teams
```

Expected: PASS. If any assertion still expects English `active` / `draft` as visible status text, update to Chinese labels from helpers.

- [ ] **Step 6: Commit (only if user asked)**

```bash
git add apps/web/src/features/teams
git commit -m "$(cat <<'EOF'
fix(web): show Chinese status labels in team management

EOF
)"
```

---

### Task 3: Collapse project lifecycle local maps

**Files:**
- Modify: `apps/web/src/features/projects/components/project-config-page.tsx`
- Modify: `apps/web/src/features/projects/components/project-risk-home.tsx`
- Modify: `apps/web/src/features/projects/components/project-operational-detail.tsx`

**Interfaces:**
- Consumes: `projectStatusLabel` from Task 1
- Leaves local: `projectPhaseLabel`, `demandStatusLabel`, `projectStatusTone` (tone only)

- [ ] **Step 1: Fix `project-config-page.tsx`**

Remove the local `function statusLabel(...)` map (lines ~83–93).

Change import:

```tsx
import { projectStatusLabel, statusLabel as genericStatusLabel, taskStatusLabel } from "@/lib/status-labels";
```

Where project lifecycle is shown:

```tsx
{projectStatusLabel(config.project.status)}
```

Keep any existing `genericStatusLabel` / `taskStatusLabel` usages unchanged.

- [ ] **Step 2: Fix `project-risk-home.tsx`**

Delete local `function projectStatusLabel(...)`.

Import:

```tsx
import { projectStatusLabel } from "@/lib/status-labels";
```

Build filter options from the helper (keep `all` local):

```tsx
const PROJECT_STATUS_FILTERS: Array<{ label: string; value: ProjectStatus | "all" }> = [
  { label: "全部状态", value: "all" },
  { label: projectStatusLabel("running"), value: "running" },
  { label: projectStatusLabel("configuring"), value: "configuring" },
  { label: projectStatusLabel("draft"), value: "draft" },
  { label: projectStatusLabel("paused"), value: "paused" },
  { label: projectStatusLabel("acceptance"), value: "acceptance" },
  { label: projectStatusLabel("archived"), value: "archived" },
];
```

Keep `projectStatusTone` local. Any display that called the old local function now uses the imported one.

- [ ] **Step 3: Fix `project-operational-detail.tsx`**

Delete local `function projectStatusLabel(...)`.

Extend the existing `@/lib/status-labels` import:

```tsx
import {
  // ...existing
  projectStatusLabel,
  statusLabel,
} from "@/lib/status-labels";
```

Keep local `projectPhaseLabel` and `demandStatusLabel` unchanged.

- [ ] **Step 4: Run project-related tests**

```bash
corepack pnpm --filter @superteam/web test -- src/features/projects
```

Expected: PASS. Fix any broken imports or duplicate-name shadowing.

- [ ] **Step 5: Commit (only if user asked)**

```bash
git add apps/web/src/features/projects/components/project-config-page.tsx \
  apps/web/src/features/projects/components/project-risk-home.tsx \
  apps/web/src/features/projects/components/project-operational-detail.tsx
git commit -m "$(cat <<'EOF'
refactor(web): route project lifecycle labels through status-labels

EOF
)"
```

---

### Task 4: Verification gate

**Files:** none new (verification only)

- [ ] **Step 1: Typecheck + web verify**

```bash
corepack pnpm verify:web
```

Expected: tests, typecheck, and build all pass.

- [ ] **Step 2: Browser smoke (required for completion)**

With Web + Control Plane running (`scripts/dev-services.sh status`):

1. Open team detail → overview: digital employee status pills are Chinese (e.g. 运行中), not `active`.
2. Open team capabilities: MCP binding status shows 启用中 (or other mapped Chinese), not raw English.
3. Team header status pill: 活跃 / 已禁用 / 已归档.
4. Team list filters: 活跃 / 已生效 etc. still readable Chinese.
5. Open a project list/detail/config surface: lifecycle status still Chinese and unchanged in meaning.

If services are down, mark **blocked** with missing dependency; do not claim done.

- [ ] **Step 3: Completion check**

Run `$superteam-completion-check` before any “done” claim.

---

## Spec coverage (self-review)

| Spec item | Task |
|---|---|
| Shared module + default `active`=启用中 | Task 1 |
| Domain helpers team/governance/employee/project | Task 1 |
| Team UI wiring + toolbar | Task 2 |
| Project local map collapse + filter同源 | Task 3 |
| Tone not unified; phase/demand left alone; out-of-scope surfaces | Constraints + Task 3 notes |
| verify:web + browser smoke | Task 4 |

No placeholders. Helper names consistent across tasks.
