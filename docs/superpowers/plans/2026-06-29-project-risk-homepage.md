# Project Risk Homepage Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Convert `/projects` into a risk-first project management homepage using existing project APIs, current-page risk enrichment, selected-project context, and safe navigation-only actions.

**Architecture:** Keep `listProjects` as the base data source, enrich only the current page with existing project detail endpoints, and derive a frontend `ProjectRiskSummary` model that can later be replaced by a backend aggregation response. Split risk derivation, enrichment hook, queue UI, summary bar, and selected context into focused units so `ProjectsView` orchestrates state instead of owning all behavior.

**Tech Stack:** React 19, TypeScript, TanStack Query, TanStack Router, SuperTeam v3 components, Vitest browser tests through `corepack pnpm --filter ./apps/web run test`.

---

## Review Revisions (2026-06-29)

Plan review against the live codebase resolved the following before implementation:

- **R1 (was a compile error):** `ProjectTask` has no `updated_at` field (`apps/web/src/lib/api/projects.ts:183`; only `ProjectTaskGraphNode` adds it). Task-sourced risk reasons must not read `task.updated_at`. The risk model below omits `waitingSince` for task reasons.
- **R2 (was a compile error):** `ProjectEventType` is a closed union (`projects.ts:24-46`) with no `project.updated` or `runtime.*` members. Typed test fixtures must use real members. Untyped `jsonResponse` page fixtures may use any string at runtime.
- **R3 (was a false-positive logic bug):** Substring keyword matching on `event_type` (`runtime`/`dispatch`/`coordination`/`workflow`) matches routine events `coordination_job.created`, `workflow.signaled`, `project_task.dispatched`, flagging healthy projects. The backend has **no** runtime-failure event type. Decision: `runtime_or_coordination` risk is derived **only** from abnormal `coordination_status`; the event-keyword scan is removed. Real dispatch failures already surface through `project_task.failed` task status (`execution_failed`).
- **R4 (request fan-out):** The enrichment hook issues 3 requests per project per page (tasks, decisions, evidence; events dropped per R3). Page size stays at its existing default of 10 and `pageSizeOptions` is `[10, 20]` (≤60 concurrent requests). This is an accepted interim cost; backend `ProjectHomeOverview` aggregation is the real fix.
- **R5 (accepted limitation):** Risk ordering, filtering, and summary-bar counts are **current-page scoped**. A high-risk project on a later page does not float to the top, and counts can understate global totals. The summary bar and queue header are labelled "(当前页)" so the scope is explicit until backend aggregation lands.
- **R6 (pagination scope):** Risk chips filter **within the current page**; `V3Pagination` total/pageCount stay bound to the full server list. Documented in the queue header so filtered rows vs. page totals are not read as a mismatch.
- **R7 (keep existing UX):** Search (`q`) and status filtering are **retained**, not dropped. They are server-driven through the existing `listFilters` memo (`index.tsx:263-267`), so `ProjectRiskQueue` owns a toolbar with `V3ToolbarSearch` + a status `<select>` wired to `filters.q`/`filters.status` via `onFiltersChange`. The toolbar moves out of the deleted `ProjectsV3List` into the queue component (which imports `V3ToolbarSearch` and defines its own status options); `index.tsx` then no longer imports `V3ToolbarSearch` or `statusOptions`.

---

## Preconditions

- The workspace may contain unrelated dirty files. Before editing, run `git status --short` and inspect target-file diffs with `git diff -- <path>`.
- Do not revert or overwrite user changes. If a target file already has edits, work with the current content.
- Code discovery should prefer codebase-memory MCP graph tools. If the graph tools are not exposed in the tool list, use `rg` and direct file reads.
- Frontend layout work must follow `DESIGN.md`, `docs/design-system/actions.md`, and `docs/design-system/data-display.md`.
- Use TanStack Router `Link` or `navigate` for internal Web navigation.
- Do not add a backend API in this implementation. The backend `ProjectHomeOverview` aggregation is a second-stage design boundary only.
- Do not add homepage write actions for approval,补证,验收,负责人分派, or batch processing.

## File Structure

- Create `apps/web/src/features/projects/project-risk.ts`
  - Owns pure risk types, risk derivation, risk counts, filters, labels, tones, and sorting.
- Create `apps/web/src/features/projects/project-risk.test.ts`
  - Unit tests for the five risk classes, summary counts, filter matching, and sorting.
- Create `apps/web/src/features/projects/hooks/use-project-risk-signals.ts`
  - Enriches the current page of projects with existing APIs and returns `Map<projectId, ProjectRiskSummary>`.
- Create `apps/web/src/features/projects/components/project-risk-home.tsx`
  - Owns `ProjectHomeRiskSummaryBar`, `ProjectRiskQueue`, and `ProjectSelectedContextPanel`.
- Modify `apps/web/src/features/projects/index.tsx`
  - Imports the new risk model, hook, and components.
  - Replaces the current metric cards and `ProjectsV3List` table with the risk homepage layout.
  - Keeps `ProjectOperationalDetail` for `/projects/$projectId`; the `/projects` index shows the new right-side selected context instead of full operational detail.
- Modify `apps/web/src/features/projects/index.test.tsx`
  - Extends the fetcher fixture for risk signals.
  - Covers risk-first ordering, risk summary counts, enrichment failure degradation, selection context, and detail navigation.
- Modify `CHANGELOG.md`
  - Add one timestamped entry at implementation completion using `TZ=Asia/Shanghai date '+%Y-%m-%d %H:%M'`.

---

### Task 1: Risk Model and Unit Tests

**Files:**
- Create: `apps/web/src/features/projects/project-risk.ts`
- Create: `apps/web/src/features/projects/project-risk.test.ts`

- [ ] **Step 1: Write failing risk model tests**

Create `apps/web/src/features/projects/project-risk.test.ts` with:

```ts
import { describe, expect, it } from "vitest";
import type {
  Project,
  ProjectDecisionRequest,
  ProjectEvidenceRef,
  ProjectTask,
} from "@/lib/api/projects";
import {
  buildRiskCounts,
  deriveProjectRiskSummary,
  matchesProjectRiskFilter,
  sortProjectsByRisk,
  type ProjectRiskFilter,
} from "./project-risk";

function project(
  id: string,
  overrides: Partial<Project> = {},
): Project {
  return {
    approval_policy: {},
    coordination_policy: {},
    coordination_status: "registered",
    coordination_workflow_id: `project-coordinator:${id}`,
    evidence_policy: {},
    goal: `${id} goal`,
    human_owner_user_id: `owner-${id}`,
    id,
    name: id,
    status: "running",
    tenant_id: "tenant-1",
    updated_at: `2026-06-29T0${id.length}:00:00Z`,
    ...overrides,
  };
}

function task(
  projectId: string,
  overrides: Partial<ProjectTask> = {},
): ProjectTask {
  return {
    id: `${projectId}-task`,
    project_id: projectId,
    requires_human_approval: false,
    status: "running",
    tenant_id: "tenant-1",
    title: `${projectId} task`,
    ...overrides,
  };
}

function decision(
  projectId: string,
  overrides: Partial<ProjectDecisionRequest> = {},
): ProjectDecisionRequest {
  return {
    approval_request_id: `${projectId}-approval`,
    decision_type: "route_review",
    id: `${projectId}-decision`,
    project_id: projectId,
    status_snapshot: "pending",
    target_user_id: `owner-${projectId}`,
    tenant_id: "tenant-1",
    title_snapshot: "需要负责人确认",
    ...overrides,
  };
}

function evidence(
  projectId: string,
  overrides: Partial<ProjectEvidenceRef> = {},
): ProjectEvidenceRef {
  return {
    evidence_type: "acceptance_check",
    id: `${projectId}-evidence`,
    metadata: {},
    project_id: projectId,
    source_ref: "ticket://SUP-1",
    source_type: "ticket",
    submitted_by_id: "de-1",
    submitted_by_type: "digital_employee",
    summary: "证据摘要",
    tenant_id: "tenant-1",
    title: "验收证据",
    verification_status: "submitted",
    ...overrides,
  };
}

describe("project risk model", () => {
  it("marks pending human decisions as danger and requiring human", () => {
    const summary = deriveProjectRiskSummary({
      decisions: [
        decision("p1", {
          risk_level_snapshot: "high",
          summary_snapshot: "生产发布需要确认",
          title_snapshot: "生产发布审批",
        }),
      ],
      evidence: [],
      events: [],
      project: project("p1"),
      tasks: [],
    });

    expect(summary.level).toBe("danger");
    expect(summary.requiresHuman).toBe(true);
    expect(summary.primaryReason?.type).toBe("human_decision");
    expect(summary.primaryReason?.label).toBe("待人类决策");
    expect(summary.reasons.map((reason) => reason.type)).toContain("human_decision");
  });

  it("marks failed tasks as execution failures", () => {
    const summary = deriveProjectRiskSummary({
      decisions: [],
      evidence: [],
      events: [],
      project: project("p2"),
      tasks: [task("p2", { status: "failed", title: "回归测试失败" })],
    });

    expect(summary.level).toBe("danger");
    expect(summary.primaryReason?.type).toBe("execution_failed");
    expect(summary.primaryReason?.detail).toContain("回归测试失败");
  });

  it("marks rejected and submitted evidence as evidence-required risk", () => {
    const summary = deriveProjectRiskSummary({
      decisions: [],
      evidence: [
        evidence("p3", {
          title: "PII 范围说明",
          verification_status: "rejected",
        }),
      ],
      events: [],
      project: project("p3"),
      tasks: [],
    });

    expect(summary.level).toBe("warn");
    expect(summary.primaryReason?.type).toBe("evidence_required");
    expect(summary.primaryReason?.label).toBe("补证等待");
  });

  it("marks abnormal coordination status as runtime or coordination risk", () => {
    const summary = deriveProjectRiskSummary({
      decisions: [],
      evidence: [],
      events: [],
      project: project("p4", { coordination_status: "error" }),
      tasks: [],
    });

    expect(summary.level).toBe("danger");
    expect(summary.primaryReason?.type).toBe("runtime_or_coordination");
    expect(summary.primaryReason?.detail).toContain("error");
  });

  it("marks stale active projects as SLA waiting risk", () => {
    const summary = deriveProjectRiskSummary(
      {
        decisions: [],
        evidence: [],
        events: [],
        project: project("p5", { updated_at: "2026-06-29T00:00:00Z" }),
        tasks: [],
      },
      { now: new Date("2026-06-29T03:00:00Z") },
    );

    expect(summary.level).toBe("warn");
    expect(summary.primaryReason?.type).toBe("sla_waiting");
    expect(summary.waitingSince).toBe("2026-06-29T00:00:00Z");
  });

  it("sorts danger before warn before healthy while preserving stable fallback order", () => {
    const danger = deriveProjectRiskSummary({
      decisions: [],
      evidence: [],
      events: [],
      project: project("danger"),
      tasks: [task("danger", { status: "failed" })],
    });
    const warn = deriveProjectRiskSummary({
      decisions: [],
      evidence: [evidence("warn")],
      events: [],
      project: project("warn"),
      tasks: [],
    });
    const healthy = deriveProjectRiskSummary({
      decisions: [],
      evidence: [],
      events: [],
      project: project("healthy"),
      tasks: [],
    });

    const sorted = sortProjectsByRisk(
      [project("healthy"), project("warn"), project("danger")],
      new Map([
        ["danger", danger],
        ["warn", warn],
        ["healthy", healthy],
      ]),
    );

    expect(sorted.map((item) => item.id)).toEqual(["danger", "warn", "healthy"]);
  });

  it("builds counts from the same summaries used by the queue filters", () => {
    const summaries = new Map([
      [
        "p1",
        deriveProjectRiskSummary({
          decisions: [decision("p1")],
          evidence: [],
          events: [],
          project: project("p1"),
          tasks: [],
        }),
      ],
      [
        "p2",
        deriveProjectRiskSummary({
          decisions: [],
          evidence: [],
          events: [],
          project: project("p2"),
          tasks: [task("p2", { status: "failed" })],
        }),
      ],
      [
        "p3",
        deriveProjectRiskSummary({
          decisions: [],
          evidence: [],
          events: [],
          project: project("p3"),
          tasks: [],
        }),
      ],
    ]);

    expect(buildRiskCounts(summaries)).toEqual({
      all: 3,
      blocked: 2,
      evidenceRequired: 0,
      executionFailed: 1,
      humanDecision: 1,
      runtimeOrCoordination: 0,
      slaWaiting: 0,
    });

    const filter: ProjectRiskFilter = "human_decision";
    expect(matchesProjectRiskFilter(summaries.get("p1"), filter)).toBe(true);
    expect(matchesProjectRiskFilter(summaries.get("p2"), filter)).toBe(false);
  });
});
```

- [ ] **Step 2: Run the risk model tests and confirm they fail**

Run:

```bash
corepack pnpm --filter ./apps/web run test -- apps/web/src/features/projects/project-risk.test.ts
```

Expected: fail with a module resolution error for `./project-risk` because the model file does not exist yet.

- [ ] **Step 3: Implement the risk model**

Create `apps/web/src/features/projects/project-risk.ts` with:

```ts
import type {
  Project,
  ProjectDecisionRequest,
  ProjectEvidenceRef,
  ProjectEvent,
  ProjectTask,
} from "@/lib/api/projects";
import type { V3Tone } from "@/components/superteam";

export type ProjectRiskLevel = "none" | "info" | "warn" | "danger";

export type ProjectRiskReasonType =
  | "human_decision"
  | "execution_failed"
  | "evidence_required"
  | "sla_waiting"
  | "runtime_or_coordination";

export type ProjectRiskFilter =
  | "all"
  | "blocked"
  | ProjectRiskReasonType;

export type ProjectRiskReason = {
  type: ProjectRiskReasonType;
  level: Exclude<ProjectRiskLevel, "none">;
  label: string;
  detail?: string;
  waitingSince?: string;
  source: "project" | "tasks" | "decisions" | "evidence" | "events";
};

export type ProjectRiskSummary = {
  projectId: string;
  level: ProjectRiskLevel;
  primaryReason?: ProjectRiskReason;
  reasons: ProjectRiskReason[];
  requiresHuman: boolean;
  waitingSince?: string;
  isPending: boolean;
  isPartial: boolean;
  error?: string;
  deepLink?: {
    route: "/projects/$projectId";
    tab?: string;
    targetId?: string;
  };
};

export type ProjectRiskSummaryMap = Map<string, ProjectRiskSummary>;

export type ProjectRiskInput = {
  project: Project;
  tasks: ProjectTask[];
  decisions: ProjectDecisionRequest[];
  evidence: ProjectEvidenceRef[];
  events: ProjectEvent[];
};

export type ProjectRiskOptions = {
  error?: string;
  isPartial?: boolean;
  isPending?: boolean;
  now?: Date;
};

export type ProjectRiskCounts = {
  all: number;
  blocked: number;
  evidenceRequired: number;
  executionFailed: number;
  humanDecision: number;
  runtimeOrCoordination: number;
  slaWaiting: number;
};

const ACTIVE_DECISION_STATUSES = new Set(["pending", "waiting", "requested", "open"]);
const FAILED_TASK_STATUSES = new Set(["failed", "error", "blocked", "cancelled"]);
const WAITING_TASK_STATUSES = new Set(["waiting_human", "pending_human", "approval_required"]);
const HEALTHY_COORDINATION_STATUSES = new Set([
  "",
  "active",
  "idle",
  "ready",
  "registered",
  "running",
  "started",
]);
const EVIDENCE_REQUIRED_STATUSES = new Set(["rejected", "submitted"]);
const SLA_WAIT_HOURS = 2;

const LEVEL_SCORE: Record<ProjectRiskLevel, number> = {
  none: 0,
  info: 1,
  warn: 2,
  danger: 3,
};

const REASON_PRIORITY: Record<ProjectRiskReasonType, number> = {
  human_decision: 50,
  execution_failed: 45,
  runtime_or_coordination: 40,
  evidence_required: 30,
  sla_waiting: 20,
};

export const PROJECT_RISK_FILTERS: Array<{
  label: string;
  value: ProjectRiskFilter;
}> = [
  { label: "全部", value: "all" },
  { label: "有阻塞", value: "blocked" },
  { label: "待我处理", value: "human_decision" },
  { label: "失败", value: "execution_failed" },
  { label: "补证", value: "evidence_required" },
  { label: "SLA", value: "sla_waiting" },
  { label: "Runtime 异常", value: "runtime_or_coordination" },
];

export function deriveProjectRiskSummary(
  input: ProjectRiskInput,
  options: ProjectRiskOptions = {},
): ProjectRiskSummary {
  const reasons: ProjectRiskReason[] = [
    ...humanDecisionReasons(input),
    ...executionFailedReasons(input),
    ...evidenceRequiredReasons(input),
    ...runtimeOrCoordinationReasons(input),
    ...slaWaitingReasons(input, options.now ?? new Date()),
  ].sort(compareRiskReasons);
  const primaryReason = reasons[0];
  const level = primaryReason?.level ?? "none";
  const waitingSince =
    reasons
      .map((reason) => reason.waitingSince)
      .filter(Boolean)
      .sort()[0] ?? undefined;

  return {
    deepLink: { route: "/projects/$projectId" },
    error: options.error,
    isPartial: Boolean(options.isPartial || options.error),
    isPending: Boolean(options.isPending),
    level,
    primaryReason,
    projectId: input.project.id,
    reasons,
    requiresHuman: reasons.some((reason) => reason.type === "human_decision"),
    waitingSince,
  };
}

export function emptyProjectRiskSummary(
  project: Project,
  options: ProjectRiskOptions = {},
): ProjectRiskSummary {
  return deriveProjectRiskSummary(
    { decisions: [], evidence: [], events: [], project, tasks: [] },
    options,
  );
}

export function buildRiskCounts(summaries: ProjectRiskSummaryMap): ProjectRiskCounts {
  const counts: ProjectRiskCounts = {
    all: summaries.size,
    blocked: 0,
    evidenceRequired: 0,
    executionFailed: 0,
    humanDecision: 0,
    runtimeOrCoordination: 0,
    slaWaiting: 0,
  };

  for (const summary of summaries.values()) {
    if (summary.level === "danger" || summary.level === "warn") {
      counts.blocked += 1;
    }
    for (const reason of summary.reasons) {
      if (reason.type === "human_decision") counts.humanDecision += 1;
      if (reason.type === "execution_failed") counts.executionFailed += 1;
      if (reason.type === "evidence_required") counts.evidenceRequired += 1;
      if (reason.type === "sla_waiting") counts.slaWaiting += 1;
      if (reason.type === "runtime_or_coordination") counts.runtimeOrCoordination += 1;
    }
  }

  return counts;
}

export function matchesProjectRiskFilter(
  summary: ProjectRiskSummary | undefined,
  filter: ProjectRiskFilter,
): boolean {
  if (filter === "all") return true;
  if (!summary) return filter === "blocked";
  if (filter === "blocked") return summary.level === "danger" || summary.level === "warn";
  return summary.reasons.some((reason) => reason.type === filter);
}

export function sortProjectsByRisk(
  projects: Project[],
  summaries: ProjectRiskSummaryMap,
): Project[] {
  return [...projects].sort((left, right) => {
    const leftSummary = summaries.get(left.id);
    const rightSummary = summaries.get(right.id);
    const levelDelta =
      LEVEL_SCORE[rightSummary?.level ?? "none"] - LEVEL_SCORE[leftSummary?.level ?? "none"];
    if (levelDelta !== 0) return levelDelta;

    const leftHuman = leftSummary?.requiresHuman ? 1 : 0;
    const rightHuman = rightSummary?.requiresHuman ? 1 : 0;
    if (leftHuman !== rightHuman) return rightHuman - leftHuman;

    const priorityDelta =
      reasonPriority(rightSummary?.primaryReason) - reasonPriority(leftSummary?.primaryReason);
    if (priorityDelta !== 0) return priorityDelta;

    const waitDelta =
      waitingTime(rightSummary?.waitingSince) - waitingTime(leftSummary?.waitingSince);
    if (waitDelta !== 0) return waitDelta;

    return updatedTime(right) - updatedTime(left);
  });
}

export function projectRiskLevelTone(level: ProjectRiskLevel): V3Tone {
  if (level === "danger") return "danger";
  if (level === "warn") return "warn";
  if (level === "info") return "info";
  return "mute";
}

export function projectRiskLevelLabel(summary: ProjectRiskSummary | undefined): string {
  if (!summary) return "风险待确认";
  if (summary.isPending) return "识别中";
  if (summary.error) return "风险待确认";
  if (summary.primaryReason) return summary.primaryReason.label;
  return "暂无阻塞";
}

function humanDecisionReasons(input: ProjectRiskInput): ProjectRiskReason[] {
  const pendingDecisions = input.decisions.filter((decision) =>
    ACTIVE_DECISION_STATUSES.has(decision.status_snapshot),
  );
  const humanTasks = input.tasks.filter(
    (task) => task.requires_human_approval || WAITING_TASK_STATUSES.has(task.status),
  );
  const reasons: ProjectRiskReason[] = [];

  for (const decision of pendingDecisions) {
    reasons.push({
      detail: decision.summary_snapshot || decision.title_snapshot,
      label: "待人类决策",
      level: decision.risk_level_snapshot === "high" ? "danger" : "warn",
      source: "decisions",
      type: "human_decision",
      waitingSince: decision.created_at,
    });
  }

  for (const task of humanTasks) {
    reasons.push({
      detail: task.summary || task.title,
      label: "待人类决策",
      level: task.risk_level === "high" ? "danger" : "warn",
      source: "tasks",
      type: "human_decision",
    });
  }

  if (input.project.status === "acceptance") {
    reasons.push({
      detail: "项目进入验收阶段，需要负责人给出结论",
      label: "待人类决策",
      level: "warn",
      source: "project",
      type: "human_decision",
      waitingSince: input.project.updated_at,
    });
  }

  return reasons;
}

function executionFailedReasons(input: ProjectRiskInput): ProjectRiskReason[] {
  return input.tasks
    .filter((task) => FAILED_TASK_STATUSES.has(task.status))
    .map((task) => ({
      detail: task.summary || task.title,
      label: "执行失败",
      level: "danger" as const,
      source: "tasks" as const,
      type: "execution_failed" as const,
    }));
}

function evidenceRequiredReasons(input: ProjectRiskInput): ProjectRiskReason[] {
  return input.evidence
    .filter((item) => EVIDENCE_REQUIRED_STATUSES.has(item.verification_status))
    .map((item) => ({
      detail: item.summary || item.title,
      label: "补证等待",
      level: "warn" as const,
      source: "evidence" as const,
      type: "evidence_required" as const,
    }));
}

function runtimeOrCoordinationReasons(input: ProjectRiskInput): ProjectRiskReason[] {
  // R3: derive only from abnormal coordination_status. The backend has no
  // runtime-failure event type, and substring-matching event_type produced
  // false positives on routine events (coordination_job.created,
  // workflow.signaled, project_task.dispatched). Real dispatch failures
  // surface through project_task.failed task status (execution_failed).
  const coordinationStatus = input.project.coordination_status || "";
  if (HEALTHY_COORDINATION_STATUSES.has(coordinationStatus)) {
    return [];
  }
  return [
    {
      detail: `协调状态 ${coordinationStatus}`,
      label: "协调异常",
      level: "danger",
      source: "project",
      type: "runtime_or_coordination",
      waitingSince: input.project.updated_at,
    },
  ];
}

function slaWaitingReasons(input: ProjectRiskInput, now: Date): ProjectRiskReason[] {
  if (!input.project.updated_at || input.project.status !== "running") return [];
  const updated = new Date(input.project.updated_at);
  if (!Number.isFinite(updated.getTime())) return [];
  const hours = (now.getTime() - updated.getTime()) / 1000 / 60 / 60;
  if (hours < SLA_WAIT_HOURS) return [];
  return [
    {
      detail: `超过 ${SLA_WAIT_HOURS} 小时未推进`,
      label: "SLA / 等待超时",
      level: "warn",
      source: "project",
      type: "sla_waiting",
      waitingSince: input.project.updated_at,
    },
  ];
}

function compareRiskReasons(left: ProjectRiskReason, right: ProjectRiskReason): number {
  const levelDelta = LEVEL_SCORE[right.level] - LEVEL_SCORE[left.level];
  if (levelDelta !== 0) return levelDelta;
  return REASON_PRIORITY[right.type] - REASON_PRIORITY[left.type];
}

function reasonPriority(reason: ProjectRiskReason | undefined): number {
  return reason ? REASON_PRIORITY[reason.type] : 0;
}

function waitingTime(value: string | undefined): number {
  if (!value) return 0;
  const time = new Date(value).getTime();
  return Number.isFinite(time) ? Date.now() - time : 0;
}

function updatedTime(project: Project): number {
  const time = new Date(project.updated_at ?? project.created_at ?? "").getTime();
  return Number.isFinite(time) ? time : 0;
}
```

- [ ] **Step 4: Run the risk model tests and confirm they pass**

Run:

```bash
corepack pnpm --filter ./apps/web run test -- apps/web/src/features/projects/project-risk.test.ts
```

Expected: pass.

- [ ] **Step 5: Commit the risk model**

Run:

```bash
git add apps/web/src/features/projects/project-risk.ts apps/web/src/features/projects/project-risk.test.ts
git commit -m "feat(web): add project risk model"
```

Expected: commit succeeds.

---

### Task 2: Current-Page Risk Enrichment Hook

**Files:**
- Create: `apps/web/src/features/projects/hooks/use-project-risk-signals.ts`
- Modify: `apps/web/src/features/projects/index.test.tsx`

- [ ] **Step 1: Add fixture controls for risk enrichment failures**

In `apps/web/src/features/projects/index.test.tsx`, extend the `createProjectFetcher` options type with:

```ts
    riskSignalFailureProjectId?: string;
```

Inside the fetcher, before the generic `url.pathname.endsWith("/tasks")` handler, add:

```ts
    if (
      options.riskSignalFailureProjectId &&
      url.pathname.startsWith(`/api/v1/projects/${options.riskSignalFailureProjectId}/`) &&
      ["/tasks", "/decisions", "/evidence", "/events"].some((suffix) =>
        url.pathname.endsWith(suffix),
      ) &&
      method === "GET"
    ) {
      return jsonResponse({ error: "risk signal load failed" }, 500);
    }
```

Update the project-2 generic handlers so `project-2` can produce non-risk data without relying on selected-project detail:

```ts
    if (url.pathname === "/api/v1/projects/project-2/tasks" && method === "GET") {
      return jsonResponse([
        {
          id: "task-project-2-failed",
          project_id: "project-2",
          requires_human_approval: false,
          status: "failed",
          tenant_id: "tenant-1",
          title: "巡检脚本失败",
        },
      ]);
    }
    if (url.pathname === "/api/v1/projects/project-2/decisions" && method === "GET") {
      return jsonResponse([]);
    }
    if (url.pathname === "/api/v1/projects/project-2/evidence" && method === "GET") {
      return jsonResponse([]);
    }
    if (url.pathname === "/api/v1/projects/project-2/events" && method === "GET") {
      return jsonResponse([
        {
          actor_id: "runtime",
          actor_type: "system",
          event_type: "project_task.failed",
          id: "event-project-2-failed",
          payload: {},
          project_id: "project-2",
          sequence_number: 3,
          summary: "巡检脚本失败",
          tenant_id: "tenant-1",
        },
      ]);
    }
```

- [ ] **Step 2: Add page tests that expect enrichment behavior**

Append these tests in the existing `describe("ProjectsView", ...)` block:

```ts
it("orders the project queue by current-page risk signals", async () => {
  const fetcher = createProjectFetcher();
  const screen = await renderProjects(fetcher);

  await expect.element(screen.getByText("项目队列")).toBeVisible();
  await expect.element(screen.getByText("巡检脚本失败")).toBeVisible();

  const queue = screen.getByTestId("project-risk-queue");
  const queueText = await queue.element().innerText();
  expect(queueText.indexOf("生产巡检整改")).toBeLessThan(
    queueText.indexOf("客户接入验收"),
  );
  expect(queueText).toContain("执行失败");
});

it("keeps the base project list usable when one project's risk enrichment fails", async () => {
  const fetcher = createProjectFetcher({ riskSignalFailureProjectId: "project-2" });
  const screen = await renderProjects(fetcher);

  await expect.element(screen.getByText("项目队列")).toBeVisible();
  await expect.element(screen.getByText("生产巡检整改")).toBeVisible();
  await expect.element(screen.getByText("风险待确认")).toBeVisible();
  await expect.element(screen.getByRole("link", { name: "详情" })).toBeVisible();
});
```

The file already exposes `renderProjects(fetcher, routeProjectId?)`; reuse that helper instead of creating a second render helper.

- [ ] **Step 3: Run the page tests and confirm they fail**

Run:

```bash
corepack pnpm --filter ./apps/web run test -- apps/web/src/features/projects/index.test.tsx
```

Expected: fail because the risk queue UI and risk enrichment hook do not exist yet.

- [ ] **Step 4: Implement the enrichment hook**

Create `apps/web/src/features/projects/hooks/use-project-risk-signals.ts` with:

```ts
import { useMemo } from "react";
import { keepPreviousData, useQueries } from "@tanstack/react-query";
import type { ApiClientOptions } from "@/lib/api/client";
import {
  listProjectDecisionRequests,
  listProjectEvidence,
  listProjectTasks,
  type Project,
  type ProjectDecisionRequest,
  type ProjectEvidenceRef,
  type ProjectTask,
} from "@/lib/api/projects";
import {
  deriveProjectRiskSummary,
  emptyProjectRiskSummary,
  type ProjectRiskSummaryMap,
} from "../project-risk";

// R3/R4: events no longer contribute to risk (runtime/coordination risk comes
// from coordination_status), so they are not fetched here — 3 requests/project.
type ProjectRiskSignalPayload = {
  projectId: string;
  tasks: ProjectTask[];
  decisions: ProjectDecisionRequest[];
  evidence: ProjectEvidenceRef[];
};

type UseProjectRiskSignalsInput = {
  apiOptions: ApiClientOptions;
  enabled?: boolean;
  projects: Project[];
};

export type UseProjectRiskSignalsResult = {
  isFetching: boolean;
  summaries: ProjectRiskSummaryMap;
};

export function useProjectRiskSignals({
  apiOptions,
  enabled = true,
  projects,
}: UseProjectRiskSignalsInput): UseProjectRiskSignalsResult {
  const queries = useQueries({
    queries: projects.map((project) => ({
      enabled,
      placeholderData: keepPreviousData,
      queryFn: async (): Promise<ProjectRiskSignalPayload> => {
        const [tasks, decisions, evidence] = await Promise.all([
          listProjectTasks(apiOptions, project.id, { limit: 20 }),
          listProjectDecisionRequests(apiOptions, project.id, { limit: 20 }),
          listProjectEvidence(apiOptions, project.id, { limit: 10 }),
        ]);
        return {
          decisions: decisions.filter((decision) => decision.project_id === project.id),
          evidence: evidence.filter((item) => item.project_id === project.id),
          projectId: project.id,
          tasks: tasks.filter((task) => task.project_id === project.id),
        };
      },
      queryKey: ["project-risk-signals", project.id],
      retry: false,
      staleTime: 15_000,
    })),
  });

  return useMemo(() => {
    const summaries: ProjectRiskSummaryMap = new Map();
    projects.forEach((project, index) => {
      const query = queries[index];
      const payload = query?.data as ProjectRiskSignalPayload | undefined;
      if (payload?.projectId === project.id) {
        summaries.set(
          project.id,
          deriveProjectRiskSummary({
            decisions: payload.decisions,
            evidence: payload.evidence,
            events: [],
            project,
            tasks: payload.tasks,
          }),
        );
        return;
      }

      summaries.set(
        project.id,
        emptyProjectRiskSummary(project, {
          error: query?.error instanceof Error ? query.error.message : undefined,
          isPartial: Boolean(query?.isError),
          isPending: Boolean(query?.isPending || query?.isFetching),
        }),
      );
    });

    return {
      isFetching: queries.some((query) => query.isFetching),
      summaries,
    };
  }, [projects, queries]);
}
```

- [ ] **Step 5: Run the page tests and confirm they still fail on missing UI only**

Run:

```bash
corepack pnpm --filter ./apps/web run test -- apps/web/src/features/projects/index.test.tsx
```

Expected: fail because `ProjectRiskQueue` is not rendered yet; the hook file should compile.

- [ ] **Step 6: Commit the enrichment hook and fixture changes**

Run:

```bash
git add apps/web/src/features/projects/hooks/use-project-risk-signals.ts apps/web/src/features/projects/index.test.tsx
git commit -m "feat(web): enrich project risk signals"
```

Expected: commit succeeds.

---

### Task 3: Risk Homepage Components

**Files:**
- Create: `apps/web/src/features/projects/components/project-risk-home.tsx`

- [ ] **Step 1: Create the risk homepage component file**

Create `apps/web/src/features/projects/components/project-risk-home.tsx` with:

```tsx
import { Link } from "@tanstack/react-router";
import {
  AlertTriangle,
  Archive,
  CircleDot,
  Clock3,
  FileWarning,
  FolderKanban,
  PlayCircle,
  ShieldAlert,
  UserCheck,
} from "lucide-react";
import {
  IconTile,
  StatusPill,
  V3Button,
  V3Chip,
  V3EmptyState,
  V3Pagination,
  V3Table,
  V3Td,
  V3Th,
  V3Tr,
  V3ToolbarSearch,
  WorkSurface,
  type V3Tone,
} from "@/components/superteam";
import type { Project, ProjectEvent, ProjectStatus } from "@/lib/api/projects";
import { cn } from "@/lib/utils";
import {
  buildRiskCounts,
  matchesProjectRiskFilter,
  PROJECT_RISK_FILTERS,
  projectRiskLevelLabel,
  projectRiskLevelTone,
  sortProjectsByRisk,
  type ProjectRiskFilter,
  type ProjectRiskSummary,
  type ProjectRiskSummaryMap,
} from "../project-risk";

type ProjectRiskQueueProps = {
  activePage: number;
  filters: {
    q: string;
    risk: ProjectRiskFilter;
    status: "all" | ProjectStatus;
  };
  isFetching: boolean;
  onFiltersChange: (filters: ProjectRiskQueueProps["filters"]) => void;
  onPageChange: (page: number) => void;
  onPageSizeChange: (pageSize: number) => void;
  onSelectProject: (projectId: string) => void;
  pageCount: number;
  pageSize: number;
  projects: Project[];
  riskSummaries: ProjectRiskSummaryMap;
  selectedProjectId?: string;
  total: number;
};

type ProjectSelectedContextPanelProps = {
  isLoading?: boolean;
  project?: Project;
  recentEvents: ProjectEvent[];
  riskSummary?: ProjectRiskSummary;
};

export function ProjectHomeRiskSummaryBar({
  riskSummaries,
}: {
  riskSummaries: ProjectRiskSummaryMap;
}) {
  const counts = buildRiskCounts(riskSummaries);
  const items = [
    { icon: ShieldAlert, label: "阻塞项目", tone: "danger" as const, value: counts.blocked },
    { icon: UserCheck, label: "待人类决策", tone: "warn" as const, value: counts.humanDecision },
    { icon: AlertTriangle, label: "执行失败", tone: "danger" as const, value: counts.executionFailed },
    { icon: FileWarning, label: "补证等待", tone: "warn" as const, value: counts.evidenceRequired },
    { icon: Clock3, label: "SLA / 等待", tone: "warn" as const, value: counts.slaWaiting },
    {
      icon: PlayCircle,
      label: "Runtime / 协调",
      tone: "info" as const,
      value: counts.runtimeOrCoordination,
    },
  ];

  return (
    <section
      aria-label="项目风险汇总（当前页）"
      className="grid gap-3 rounded-v3-card border border-v3-line bg-v3-card p-4 shadow-v3 sm:grid-cols-2 xl:grid-cols-6"
    >
      {items.map((item) => {
        const Icon = item.icon;
        return (
          <div key={item.label} className="flex min-w-0 items-center gap-3">
            <IconTile tone={item.tone} size="sm">
              <Icon />
            </IconTile>
            <div className="min-w-0">
              <div className="text-[12px] font-semibold text-v3-ink-2">{item.label}</div>
              <div className="text-2xl font-extrabold tabular-nums text-v3-ink">{item.value}</div>
            </div>
          </div>
        );
      })}
    </section>
  );
}

export function ProjectRiskQueue(props: ProjectRiskQueueProps) {
  const sortedProjects = sortProjectsByRisk(props.projects, props.riskSummaries).filter(
    (project) => matchesProjectRiskFilter(props.riskSummaries.get(project.id), props.filters.risk),
  );
  const riskCounts = buildRiskCounts(props.riskSummaries);

  return (
    <section data-testid="project-risk-queue" className="min-w-0" aria-label="项目队列">
      <WorkSurface className="min-w-0">
        <div className="flex min-w-0 flex-col gap-3 border-b border-v3-line p-4 lg:flex-row lg:items-center lg:justify-between">
          <div className="min-w-0">
            <h2 className="text-base font-extrabold text-v3-ink">项目队列</h2>
            <div className="mt-1 flex flex-wrap items-center gap-2">
              <StatusPill tone={props.isFetching ? "info" : "mute"}>
                {props.isFetching ? "正在识别风险" : `${props.total} 个项目`}
              </StatusPill>
              <StatusPill tone="danger">{riskCounts.blocked} 个阻塞（当前页）</StatusPill>
            </div>
            <p className="mt-1 text-[11px] text-v3-ink-3">
              风险识别与排序基于当前页；筛选标签在当前页内生效，分页对应完整项目列表。
            </p>
          </div>
          <V3Button asChild variant="outline">
            <Link to="/projects/new">新建</Link>
          </V3Button>
        </div>

        <div className="flex min-w-0 flex-col gap-3 border-b border-v3-line p-4">
          <div className="flex min-w-0 flex-wrap items-center gap-2">
            <V3ToolbarSearch
              aria-label="搜索项目"
              placeholder="搜索项目名称或目标"
              value={props.filters.q}
              onChange={(event) =>
                props.onFiltersChange({ ...props.filters, q: event.target.value })
              }
            />
            <select
              aria-label="项目状态筛选"
              className="rounded-[10px] border border-v3-line bg-v3-card px-3 py-2 text-[13px] text-v3-ink"
              value={props.filters.status}
              onChange={(event) =>
                props.onFiltersChange({
                  ...props.filters,
                  status: event.target.value as ProjectRiskQueueProps["filters"]["status"],
                })
              }
            >
              {PROJECT_STATUS_FILTER_OPTIONS.map((option) => (
                <option key={option.value} value={option.value}>
                  {option.label}
                </option>
              ))}
            </select>
          </div>
          <div className="flex flex-wrap gap-2">
            {PROJECT_RISK_FILTERS.map((filter) => (
              <V3Chip
                key={filter.value}
                active={props.filters.risk === filter.value}
                count={riskFilterCount(filter.value, riskCounts)}
                type="button"
                onClick={() => props.onFiltersChange({ ...props.filters, risk: filter.value })}
              >
                {filter.label}
              </V3Chip>
            ))}
          </div>
        </div>

        <V3Table>
          <thead>
            <tr>
              <V3Th className="min-w-[260px]">项目</V3Th>
              <V3Th className="min-w-36">风险</V3Th>
              <V3Th className="min-w-32">状态</V3Th>
              <V3Th className="min-w-36">负责人</V3Th>
              <V3Th className="min-w-[240px]">处置落点</V3Th>
              <V3Th className="min-w-32 text-right">操作</V3Th>
            </tr>
          </thead>
          <tbody>
            {sortedProjects.map((project) => (
              <ProjectRiskQueueRow
                key={project.id}
                onSelectProject={props.onSelectProject}
                project={project}
                riskSummary={props.riskSummaries.get(project.id)}
                selected={project.id === props.selectedProjectId}
              />
            ))}
            {sortedProjects.length === 0 ? (
              <tr>
                <V3Td colSpan={6}>
                  <V3EmptyState
                    icon={<FolderKanban />}
                    title="没有符合筛选条件的项目"
                    description="调整风险筛选、搜索关键词或项目状态后重试。"
                  />
                </V3Td>
              </tr>
            ) : null}
          </tbody>
        </V3Table>
        <V3Pagination
          page={props.activePage}
          pageCount={props.pageCount}
          pageSize={props.pageSize}
          pageSizeOptions={[10, 20]}
          total={props.total}
          onPageChange={props.onPageChange}
          onPageSizeChange={props.onPageSizeChange}
        />
      </WorkSurface>
    </section>
  );
}

export function ProjectSelectedContextPanel({
  isLoading,
  project,
  recentEvents,
  riskSummary,
}: ProjectSelectedContextPanelProps) {
  return (
    <aside className="hidden min-w-0 2xl:block" aria-label="选中项目上下文">
      <WorkSurface className="p-5">
        {!project ? (
          <V3EmptyState title="选择一个项目" description="从项目队列中选择项目后查看上下文。" />
        ) : (
          <div className="flex min-w-0 flex-col gap-5">
            <div className="flex items-start justify-between gap-3">
              <div className="min-w-0">
                <h2 className="truncate text-lg font-extrabold text-v3-ink">{project.name}</h2>
                <p className="mt-1 font-mono text-xs text-v3-ink-3">
                  {project.coordination_workflow_id}
                </p>
              </div>
              <StatusPill tone={projectRiskLevelTone(riskSummary?.level ?? "none")}>
                {projectRiskLevelLabel(riskSummary)}
              </StatusPill>
            </div>

            <div className="rounded-v3-inner border border-v3-line bg-v3-card-soft p-4">
              <div className="text-xs font-bold text-v3-ink-3">处置落点</div>
              <div className="mt-2 text-sm font-semibold text-v3-ink">
                {riskSummary?.primaryReason?.detail ?? "暂无需要立即介入的阻塞"}
              </div>
              <div className="mt-3 flex flex-wrap gap-2">
                <V3Button asChild size="sm" variant="outline">
                  <Link params={{ projectId: project.id }} to="/projects/$projectId">
                    详情
                  </Link>
                </V3Button>
                <V3Button asChild size="sm" variant="ghost">
                  <Link search={{ projectId: project.id }} to="/task-launches">
                    发起任务
                  </Link>
                </V3Button>
              </div>
            </div>

            <div>
              <div className="mb-2 text-xs font-bold text-v3-ink-3">最近事件</div>
              <div className="space-y-2">
                {isLoading ? (
                  <StatusPill tone="info">上下文加载中</StatusPill>
                ) : recentEvents.length > 0 ? (
                  recentEvents.slice(0, 3).map((event) => (
                    <div
                      key={event.id}
                      className="rounded-v3-inner border border-v3-line bg-v3-card p-3"
                    >
                      <div className="text-[13px] font-semibold text-v3-ink">
                        {event.summary || event.event_type}
                      </div>
                      <div className="mt-1 font-mono text-[11px] text-v3-ink-3">
                        #{event.sequence_number}
                      </div>
                    </div>
                  ))
                ) : (
                  <div className="text-sm text-v3-ink-2">暂无最近事件</div>
                )}
              </div>
            </div>
          </div>
        )}
      </WorkSurface>
    </aside>
  );
}

function ProjectRiskQueueRow({
  onSelectProject,
  project,
  riskSummary,
  selected,
}: {
  onSelectProject: (projectId: string) => void;
  project: Project;
  riskSummary?: ProjectRiskSummary;
  selected: boolean;
}) {
  const tone = riskSummary?.level === "danger" ? "danger" : riskSummary?.level === "warn" ? "warn" : undefined;
  return (
    <V3Tr className={cn(selected && "[&>td]:bg-v3-brand-soft/60")} tone={tone}>
      <V3Td className="whitespace-normal">
        <button
          aria-current={selected ? "true" : undefined}
          aria-label={`选择项目 ${project.name}`}
          className="flex min-w-0 items-start gap-3 text-left"
          type="button"
          onClick={() => onSelectProject(project.id)}
        >
          <IconTile tone={projectStatusTone(project.status)} size="sm">
            {project.status === "archived" ? <Archive /> : <CircleDot />}
          </IconTile>
          <span className="min-w-0">
            <span className="block truncate font-bold text-v3-ink">{project.name}</span>
            <span className="mt-0.5 block font-mono text-[12px] text-v3-ink-3">
              {project.id}
            </span>
          </span>
        </button>
      </V3Td>
      <V3Td>
        <div className="flex flex-col gap-1">
          <StatusPill tone={projectRiskLevelTone(riskSummary?.level ?? "none")}>
            {projectRiskLevelLabel(riskSummary)}
          </StatusPill>
          {riskSummary?.primaryReason?.detail ? (
            <span className="line-clamp-1 text-xs text-v3-ink-3">
              {riskSummary.primaryReason.detail}
            </span>
          ) : null}
        </div>
      </V3Td>
      <V3Td>
        <StatusPill tone={projectStatusTone(project.status)}>
          {projectStatusLabel(project.status)}
        </StatusPill>
      </V3Td>
      <V3Td>
        <span className="font-mono text-[12px] text-v3-ink-2">
          {project.human_owner_user_id || "未设置"}
        </span>
      </V3Td>
      <V3Td className="whitespace-normal">
        <span className="line-clamp-2 text-[13px] leading-5 text-v3-ink-2">
          {riskSummary?.requiresHuman ? "human_owner 判断" : "进入详情查看项目上下文"}
        </span>
      </V3Td>
      <V3Td>
        <div className="flex justify-end gap-2">
          <V3Button
            aria-label={`选择项目 ${project.name}`}
            onClick={() => onSelectProject(project.id)}
            size="sm"
            type="button"
            variant={selected ? "primary" : "outline"}
          >
            选择
          </V3Button>
          <V3Button asChild size="sm" variant="ghost">
            <Link params={{ projectId: project.id }} to="/projects/$projectId">
              详情
            </Link>
          </V3Button>
        </div>
      </V3Td>
    </V3Tr>
  );
}

function riskFilterCount(filter: ProjectRiskFilter, counts: ReturnType<typeof buildRiskCounts>) {
  if (filter === "all") return counts.all;
  if (filter === "blocked") return counts.blocked;
  if (filter === "human_decision") return counts.humanDecision;
  if (filter === "execution_failed") return counts.executionFailed;
  if (filter === "evidence_required") return counts.evidenceRequired;
  if (filter === "sla_waiting") return counts.slaWaiting;
  if (filter === "runtime_or_coordination") return counts.runtimeOrCoordination;
  return 0;
}

const PROJECT_STATUS_FILTER_OPTIONS: Array<{
  label: string;
  value: "all" | ProjectStatus;
}> = [
  { label: "全部状态", value: "all" },
  { label: "运行中", value: "running" },
  { label: "配置中", value: "configuring" },
  { label: "草稿", value: "draft" },
  { label: "已暂停", value: "paused" },
  { label: "验收中", value: "acceptance" },
  { label: "已归档", value: "archived" },
];

function projectStatusLabel(status: ProjectStatus | string) {
  const labels: Record<string, string> = {
    acceptance: "验收中",
    archived: "已归档",
    configuring: "配置中",
    draft: "草稿",
    paused: "已暂停",
    running: "运行中",
  };
  return labels[status] ?? status;
}

function projectStatusTone(status: ProjectStatus | string): V3Tone {
  if (status === "running") return "ok";
  if (status === "archived") return "mute";
  if (status === "paused" || status === "acceptance") return "warn";
  if (status === "configuring" || status === "draft") return "info";
  return "mute";
}
```

- [ ] **Step 2: Run type-facing tests and confirm missing integration only**

Run:

```bash
corepack pnpm --filter ./apps/web run test -- apps/web/src/features/projects/index.test.tsx
```

Expected: still fail because `ProjectsView` has not imported or rendered the new components.

- [ ] **Step 3: Commit the component file**

Run:

```bash
git add apps/web/src/features/projects/components/project-risk-home.tsx
git commit -m "feat(web): add project risk homepage components"
```

Expected: commit succeeds.

---

### Task 4: Wire the Risk Homepage Into ProjectsView

**Files:**
- Modify: `apps/web/src/features/projects/index.tsx`

- [ ] **Step 1: Update imports**

In `apps/web/src/features/projects/index.tsx`, add these imports:

```ts
import {
  ProjectHomeRiskSummaryBar,
  ProjectRiskQueue,
  ProjectSelectedContextPanel,
} from "./components/project-risk-home";
import { useProjectRiskSignals } from "./hooks/use-project-risk-signals";
import {
  buildRiskCounts,
  matchesProjectRiskFilter,
  sortProjectsByRisk,
  type ProjectRiskFilter,
} from "./project-risk";
```

Remove unused imports after replacing the old metric cards and list helpers. The imports expected to become unused are `Archive`, `CircleDot`, `ClipboardList`, `ListChecks`, `type LucideIcon`, `V3MetricCard`, `V3ToolbarSearch`, `V3Table`, `V3Td`, `V3Th`, `V3Tr`, and `cn`.

- [ ] **Step 2: Extend the UI filter state**

Replace `UiProjectListFilters` with:

```ts
type UiProjectListFilters = {
  q: string;
  risk: ProjectRiskFilter;
  status: "all" | ProjectStatus;
};
```

Replace the `useState` initializer with:

```ts
  const [filters, setFilters] = useState<UiProjectListFilters>({
    q: "",
    risk: "all",
    status: "all",
  });
```

- [ ] **Step 3: Build risk summaries for the current page**

After `pagedProjects` is computed, add:

```ts
  const currentPageRiskSignals = useProjectRiskSignals({
    apiOptions,
    projects: pagedProjects,
  });
  const riskSortedPagedProjects = useMemo(
    () =>
      sortProjectsByRisk(pagedProjects, currentPageRiskSignals.summaries).filter((project) =>
        matchesProjectRiskFilter(currentPageRiskSignals.summaries.get(project.id), filters.risk),
      ),
    [currentPageRiskSignals.summaries, filters.risk, pagedProjects],
  );
```

Keep `projectListPageCount` for server-list pagination. The first implementation should not add a second page-count calculation after risk filtering; an empty filtered current page should render the queue empty state.

- [ ] **Step 4: Reset page on risk filter changes**

Replace the current page reset effect with:

```ts
  useEffect(() => {
    setProjectListPage(1);
  }, [filters.q, filters.risk, filters.status]);
```

- [ ] **Step 5: Preserve selection when risk filtering changes**

After the existing selection effect, add:

```ts
  useEffect(() => {
    if (routeProjectId || riskSortedPagedProjects.length === 0) {
      return;
    }
    if (!selectedProjectId || !riskSortedPagedProjects.some((project) => project.id === selectedProjectId)) {
      setSelectedProjectId(riskSortedPagedProjects[0].id);
    }
  }, [riskSortedPagedProjects, routeProjectId, selectedProjectId]);
```

- [ ] **Step 6: Replace the old metric and list block**

In the render branch that currently starts with:

```tsx
              <section
                aria-label="项目管理指标"
                className="grid gap-4 sm:grid-cols-2 xl:grid-cols-4"
              >
```

replace the metric section and `ProjectsV3List` usage with:

```tsx
              <ProjectHomeRiskSummaryBar riskSummaries={currentPageRiskSignals.summaries} />

              <div className="grid min-w-0 items-start gap-5 2xl:grid-cols-[minmax(760px,1.15fr)_minmax(320px,0.85fr)]">
                <ProjectRiskQueue
                  activePage={activeProjectListPage}
                  filters={filters}
                  isFetching={projectsQuery.isFetching || currentPageRiskSignals.isFetching}
                  onFiltersChange={setFilters}
                  onPageChange={setProjectListPage}
                  onPageSizeChange={(size) => {
                    setProjectListPageSize(size);
                    setProjectListPage(1);
                  }}
                  onSelectProject={setSelectedProjectId}
                  pageCount={projectListPageCount}
                  pageSize={projectListPageSize}
                  projects={riskSortedPagedProjects}
                  riskSummaries={currentPageRiskSignals.summaries}
                  selectedProjectId={effectiveProjectId}
                  total={projects.length}
                />
                {routeProjectId ? (
                  <ProjectOperationalDetail
                    acceptance={projectAcceptance}
                    archivePreview={projectArchivePreview}
                    archiveSnapshots={projectArchiveSnapshots}
                    artifacts={projectArtifacts}
                    budgetLedger={projectBudgetLedger}
                    budgetSummary={projectBudgetSummary}
                    coordinationJobs={projectCoordinationJobs}
                    decisionRequests={projectDecisionRequests}
                    demands={projectDemands}
                    dispatchGateTaskTitle={dispatchGateTask?.title}
                    dispatchGates={projectDispatchGates}
                    evidence={projectEvidence}
                    events={projectEvents}
                    executionTrace={projectExecutionTrace}
                    executionTraceErrorMessage={projectExecutionTraceErrorMessage}
                    executionTraceIsError={projectExecutionTraceIsError}
                    executionTraceIsLoading={projectExecutionTraceIsLoading}
                    executionSummaries={projectExecutionSummaries}
                    isArchived={isArchived}
                    onArchiveProject={() => {
                      if (effectiveProjectId) {
                        archiveMutation.mutate(effectiveProjectId);
                      }
                    }}
                    onCreateAcceptance={(input) => {
                      if (effectiveProjectId) {
                        createAcceptanceMutation.mutate(input);
                      }
                    }}
                    onCreateArchiveSnapshot={(input) => {
                      if (effectiveProjectId) {
                        createArchiveSnapshotMutation.mutate(input);
                      }
                    }}
                    onCreateEvidence={(input) => {
                      if (effectiveProjectId) {
                        createEvidenceMutation.mutate(input);
                      }
                    }}
                    onPatchEvidence={(evidenceId, verificationStatus) => {
                      if (effectiveProjectId) {
                        patchEvidenceMutation.mutate({ evidenceId, verificationStatus });
                      }
                    }}
                    onRetryExecutionTrace={() => {
                      void executionTraceQuery.refetch();
                    }}
                    onResolveDecision={(decisionId, decision) => {
                      if (effectiveProjectId) {
                        resolveDecisionMutation.mutate({ decisionId, decision });
                      }
                    }}
                    onSubmitDemand={() => setDemandOpen(true)}
                    overview={overview}
                    project={displayedProject}
                    reports={projectReports}
                    planRevisions={projectPlanRevisions}
                    routeDecisions={projectRouteDecisions}
                    taskGraph={taskGraphQuery.data}
                    tasks={projectTasks}
                    transferRequests={projectTransferRequests}
                  />
                ) : (
                  <ProjectSelectedContextPanel
                    isLoading={eventsQuery.isFetching}
                    project={displayedProject}
                    recentEvents={projectEvents}
                    riskSummary={
                      effectiveProjectId
                        ? currentPageRiskSignals.summaries.get(effectiveProjectId)
                        : undefined
                    }
                  />
                )}
              </div>
```

The conditional keeps full operational detail on `/projects/$projectId`, while `/projects` gets the lightweight right-side context.

- [ ] **Step 7: Delete old local list helpers**

Remove these now-unused local functions and constants from `apps/web/src/features/projects/index.tsx`:

```ts
ProjectsV3List
ProjectsV3Table
ProjectsV3TableRow
buildProjectStats
projectMainLoopLabel
projectStatusTone
projectStatusLabel
statusOptions          // R7: the status <select> moved into ProjectRiskQueue
```

Keep `queryErrorMessage`, `selectDispatchGateTask`, and `dispatchGateCandidateStatus`. The `q`/`status` server filtering (`listFilters` memo) stays — only the toolbar UI moved into the queue component.

- [ ] **Step 8: Run the project tests**

Run:

```bash
corepack pnpm --filter ./apps/web run test -- apps/web/src/features/projects/index.test.tsx apps/web/src/features/projects/project-risk.test.ts
```

Expected: pass. If `renderProjects` or role queries need minor adjustment because the mocked `Link` creates multiple `详情` links, scope assertions to `getByTestId("project-risk-queue")`.

- [ ] **Step 9: Commit the page wiring**

Run:

```bash
git add apps/web/src/features/projects/index.tsx apps/web/src/features/projects/index.test.tsx
git commit -m "feat(web): wire risk-first projects homepage"
```

Expected: commit succeeds.

---

### Task 5: Complete Page-Level Tests and UX Details

**Files:**
- Modify: `apps/web/src/features/projects/index.test.tsx`
- Modify: `apps/web/src/features/projects/components/project-risk-home.tsx`
- Modify: `apps/web/src/features/projects/index.tsx`

- [ ] **Step 1: Add filter interaction test**

Append this test in `apps/web/src/features/projects/index.test.tsx`:

```ts
it("filters the queue by risk category without changing the base project list request", async () => {
  const fetcher = createProjectFetcher();
  const screen = await renderProjects(fetcher);

  await expect.element(screen.getByText("项目队列")).toBeVisible();
  await userEvent.click(screen.getByRole("button", { name: /失败/ }));

  const queueText = await screen.getByTestId("project-risk-queue").element().innerText();
  expect(queueText).toContain("生产巡检整改");
  expect(queueText).not.toContain("客户接入验收");

  const projectListCalls = fetcher.mock.calls.filter(([input]) => {
    const url = new URL(String(input));
    return url.pathname === "/api/v1/projects";
  });
  expect(projectListCalls.length).toBeGreaterThan(0);
});
```

- [ ] **Step 2: Add selected context test**

Append this test:

```ts
it("shows a lightweight selected-project context on the projects index", async () => {
  const fetcher = createProjectFetcher();
  const screen = await renderProjects(fetcher);

  await expect.element(screen.getByLabelText("选中项目上下文")).toBeVisible();
  await expect.element(screen.getByText("客户接入验收")).toBeVisible();
  await expect.element(screen.getByText("project-coordinator:project-1")).toBeVisible();
  await expect.element(screen.getByRole("link", { name: "发起任务" })).toHaveAttribute(
    "href",
    "/task-launches?projectId=project-1",
  );
});
```

- [ ] **Step 3: Add detail-route preservation test**

Append this test:

```ts
it("keeps the full operational detail on project detail routes", async () => {
  const fetcher = createProjectFetcher();
  const screen = await renderProjects(fetcher, "project-1");

  await expect.element(screen.getByText("客户接入验收")).toBeVisible();
  await expect.element(screen.getByText("整理接入证据")).toBeVisible();
  expect(screen.queryByLabelText("选中项目上下文")).toBeNull();
});
```

- [ ] **Step 4: Run page tests and fix exact accessible-name mismatches**

Run:

```bash
corepack pnpm --filter ./apps/web run test -- apps/web/src/features/projects/index.test.tsx
```

Expected: pass after adjusting only accessible-name scoping. Do not remove assertions about risk order, degradation, selected context, or detail route preservation.

- [ ] **Step 5: Commit test and UX refinements**

Run:

```bash
git add apps/web/src/features/projects/index.test.tsx apps/web/src/features/projects/components/project-risk-home.tsx apps/web/src/features/projects/index.tsx
git commit -m "test(web): cover project risk homepage"
```

Expected: commit succeeds.

---

### Task 6: Changelog, Full Web Test, and Real UI Verification

**Files:**
- Modify: `CHANGELOG.md`

- [ ] **Step 1: Add a timestamped changelog entry**

Run:

```bash
TZ=Asia/Shanghai date '+%Y-%m-%d %H:%M'
```

Add an entry near the top of `CHANGELOG.md` using the returned timestamp:

```md
- YYYY-MM-DD HH:MM 项目管理首页改为风险优先队列，基于当前页项目补强任务、决策、证据和协调状态风险信号，并保留统一详情跳转。
```

Replace `YYYY-MM-DD HH:MM` with the exact command output.

- [ ] **Step 2: Run full Web tests**

Run:

```bash
corepack pnpm --filter ./apps/web run test
```

Expected: pass.

- [ ] **Step 3: Check services before real UI verification**

Run:

```bash
scripts/dev-services.sh status
```

Expected: Web and Control Plane are running. If Web is not running, start or restart only the Web service with:

```bash
scripts/dev-services.sh restart web
```

If Control Plane is not running, start or restart only the Control Plane service with:

```bash
scripts/dev-services.sh restart control-plane
```

- [ ] **Step 4: Verify `/projects` in a real browser**

Use the browser or Chrome plugin against the running Web URL. Verify these visible states:

- `/projects` loads without a stuck loading state.
- The title is `项目管理`.
- The risk summary bar is visible.
- The table heading is `项目队列`.
- At least one project row is visible when the database has projects.
- Clicking `详情` navigates to `/projects/$projectId` through TanStack Router.
- On the `/projects` index, the right-side panel is the lightweight selected-project context on wide screens.
- On `/projects/$projectId`, the existing full operational detail still appears.

- [ ] **Step 5: Run diff hygiene**

Run:

```bash
git diff --check
```

Expected: no output.

- [ ] **Step 6: Commit changelog and verification adjustments**

Run:

```bash
git add CHANGELOG.md apps/web/src/features/projects apps/web/src/features/projects/index.tsx
git commit -m "docs: record project risk homepage"
```

Expected: commit succeeds if there are staged changes. If only `CHANGELOG.md` changed in this task, stage and commit only `CHANGELOG.md`.

---

## Self-Review Checklist

- Spec coverage:
  - Risk-first homepage UI is covered by Tasks 3-4.
  - Current-page light enrichment is covered by Task 2.
  - Selected-project desktop context is covered by Tasks 3-5.
  - Unified `/projects/$projectId` detail navigation is covered by Tasks 3-5.
  - No homepage write actions are introduced in any task.
  - Backend aggregation remains outside this implementation.
- Placeholder scan:
  - Checked for placeholder language and unspecified implementation steps.
- Type consistency:
  - Risk types are defined in `project-risk.ts`.
  - Hook result uses `ProjectRiskSummaryMap`.
  - UI components consume `ProjectRiskSummaryMap` and `ProjectRiskFilter`.
  - Page state adds `risk: ProjectRiskFilter`.
