# Project Management Main View Simplification Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Simplify the existing Web project-management default view into a project-owner main loop view, while preserving existing advanced project facts behind an explicit advanced section.

**Architecture:** This is Phase 1 from the approved spec and is intentionally Web-only. It reuses existing `ProjectOverview`, `ProjectDemand`, `PlanRevision`, `DecisionRequest`, `ProjectTaskGraph`, and governance data; no database migration, OpenAPI change, or new aggregation API is introduced in this plan. The primary UI changes live inside `ProjectOperationalDetail`, with focused helper functions and browser tests proving internal coordination objects no longer dominate the default project view.

**Tech Stack:** React 19, TypeScript, TanStack Query/Router, Radix Collapsible, lucide-react, SuperTeam v3 components, Vitest browser tests, `corepack pnpm --filter ./apps/web run test`.

---

## Source Spec

Implement the first independently shippable slice of:

- `docs/superpowers/specs/2026-06-27-project-management-simplified-main-loop-design.md`

This plan covers only Phase 1:

- Rename the product concepts shown in the project detail surface.
- Make the default project detail read like a project-owner workspace.
- Hide or fold advanced internal objects by default.
- Keep existing advanced capabilities reachable.
- Use existing API responses; do not change contracts or persistence.

This plan does not implement:

- Multiple project owner persistence.
- Multiple team owner persistence.
- `/api/v1/projects/{projectId}/workspace`.
- `/api/v1/project-demands/{demandId}/progress`.
- `/api/v1/workbench/actions`.
- `/api/v1/teams/{teamId}/service-pool`.
- Platform-management or team-service-pool pages.

## File Structure

Modify Web feature code:

- `apps/web/src/features/projects/components/project-operational-detail.tsx`
  Main implementation. Convert the default detail layout to project-owner language, add main-loop summary helpers, rename member panels, translate dispatch-gate details to business blockers, and move technical panels into an explicit advanced section.
- `apps/web/src/features/projects/index.tsx`
  Adjust page header copy and project list column labels so they no longer foreground coordination-thread implementation details.

Modify Web tests:

- `apps/web/src/features/projects/index.test.tsx`
  Update existing expectations and add regression coverage for the simplified main view, advanced-section hiding, business blocker translation, and renamed owner/service-pool concepts.

No backend files should change in this plan.

## Behavior Target

After implementation, opening `/projects` or `/projects/$projectId` should default to:

- Project goal and project-owner action buttons.
- Main-loop metrics: current project phase, pending owner actions, current demand, active execution.
- Plan confirmation panel.
- Current demand / current execution / latest result summary.
- Project owner group and project service pool.
- Recent project events.
- A clearly labeled advanced section that contains route decisions, coordination jobs, dispatch-gate technical data, execution trace, governance tabs, budget, archive, and full internal records.

The following technical labels must not appear in the default visible text unless the advanced section is expanded:

- `Pre-dispatch gate`
- `路由决策`
- `协调任务`
- `执行摘要`
- `转派请求`
- `人类角色`
- `数字员工池`
- `协调线程`

The following business labels should appear in the default visible text:

- `项目负责人组`
- `项目服务池`
- `当前需求`
- `当前执行`
- `最新结果`
- `待负责人处理`
- `高级项目事实`

## Task 1: Add Main-View Regression Tests

**Files:**

- Modify: `apps/web/src/features/projects/index.test.tsx`

- [ ] **Step 1: Update project overview fixture with display names and a second owner**

In `createProjectFetcher`, inside the `/api/v1/projects/project-1/overview` response, replace the current `human_roles` array with this array:

```ts
human_roles: [
  {
    display_name_snapshot: "负责人甲",
    id: "member-owner-1",
    principal_id: "human-owner-1",
    principal_type: "human_user",
    project_id: "project-1",
    project_role: "owner",
    settings: {},
    status: "active",
    tenant_id: "tenant-1",
  },
  {
    display_name_snapshot: "负责人乙",
    id: "member-owner-2",
    principal_id: "human-owner-2",
    principal_type: "human_user",
    project_id: "project-1",
    project_role: "owner",
    settings: {},
    status: "active",
    tenant_id: "tenant-1",
  },
],
```

Also replace the current `digital_employee_pool` array with this array:

```ts
digital_employee_pool: [
  {
    display_name_snapshot: "验收执行员工",
    id: "member-employee-1",
    principal_id: "de-1",
    principal_type: "digital_employee",
    project_id: "project-1",
    project_role: "executor",
    settings: { source_team_name: "平台运营" },
    status: "active",
    tenant_id: "tenant-1",
  },
],
```

- [ ] **Step 2: Add helper for collapsed advanced content assertions**

Add this helper near `fetchCalls`:

```ts
function pageText() {
  return document.body.textContent ?? "";
}
```

- [ ] **Step 3: Replace the selected-overview test expectations**

Find the test named `renders the project list and selected overview` and replace its body with:

```ts
const fetcher = createProjectFetcher();
const screen = await renderProjects(fetcher);

await expect
  .element(screen.getByRole("heading", { name: "客户接入验收" }))
  .toBeInTheDocument();
// `当前需求`/`当前执行`/`待负责人处理` each render in both a metric tile and
// a panel header, so these locators resolve to multiple elements; use
// `.first()` to avoid strict-locator multi-match failures.
await expect.element(screen.getByText("当前需求").first()).toBeInTheDocument();
await expect.element(screen.getByText("补充上线验收说明").first()).toBeInTheDocument();
await expect.element(screen.getByText("当前执行").first()).toBeInTheDocument();
await expect.element(screen.getByText("整理接入证据").first()).toBeInTheDocument();
await expect.element(screen.getByText("待负责人处理").first()).toBeInTheDocument();
await expect.element(screen.getByText("需要负责人确认")).toBeInTheDocument();
await expect.element(screen.getByText("项目负责人组")).toBeInTheDocument();
await expect.element(screen.getByText("负责人甲")).toBeInTheDocument();
await expect.element(screen.getByText("负责人乙")).toBeInTheDocument();
await expect.element(screen.getByText("项目服务池")).toBeInTheDocument();
await expect.element(screen.getByText("验收执行员工")).toBeInTheDocument();
await expect.element(screen.getByText("最新结果")).toBeInTheDocument();
await expect.element(screen.getByText("证据充分")).toBeInTheDocument();
await expect.element(screen.getByText("高级项目事实")).toBeInTheDocument();

expect(pageText()).not.toContain("Pre-dispatch gate");
expect(pageText()).not.toContain("路由决策");
expect(pageText()).not.toContain("协调任务");
expect(pageText()).not.toContain("人类角色");
expect(pageText()).not.toContain("数字员工池");
expect(pageText()).not.toContain("协调线程");

await userEvent.click(screen.getByRole("button", { name: "展开高级项目事实" }));

await expect.element(screen.getByText("路由决策")).toBeInTheDocument();
await expect.element(screen.getByText("协调任务")).toBeInTheDocument();
// Execution trace panel keeps its existing title `执行证据链`; this plan does
// not rename it.
await expect.element(screen.getByText("执行证据链")).toBeInTheDocument();
expect(pageText()).toContain("runtime.node_offline");
expect(pageText()).toContain("runtime.inspect、codebase.analysis");

await userEvent.click(
  screen.getByRole("button", { name: "要求修改计划版本 v2" }),
);

await vi.waitFor(() => {
  expect(
    fetchCalls(fetcher).some(([url, init]) => {
      return (
        String(url).endsWith(
          "/api/v1/projects/project-1/decisions/decision-plan-1/resolve",
        ) &&
        init?.method === "POST" &&
        JSON.parse(String(init.body)).decision === "request_changes"
      );
    }),
  ).toBe(true);
});
```

- [ ] **Step 4: Replace the dispatch-gate visible test**

Find the test named `shows the latest pre-dispatch gate status for a selected project task` and replace its first three visible assertions with:

```ts
await expect.element(screen.getByText("当前阻塞")).toBeInTheDocument();
await expect.element(screen.getByText("运行节点暂不可用，系统会稍后重试")).toBeInTheDocument();
expect(pageText()).not.toContain("Pre-dispatch gate");
expect(pageText()).not.toContain("runtime.node_offline");

await userEvent.click(screen.getByRole("button", { name: "展开高级项目事实" }));

await expect.element(screen.getByText("Dispatch gate 技术详情")).toBeInTheDocument();
await expect.element(screen.getByText("Retry later")).toBeInTheDocument();
await expect.element(screen.getByText("runtime.node_offline")).toBeInTheDocument();
```

Leave the existing `vi.waitFor` fetch-call assertions in that test unchanged.

- [ ] **Step 5: Update governance tab test to expand advanced first**

Find `renders governance tabs and archive preview metrics`. Insert this line immediately after `const screen = await renderProjects(fetcher, "project-1");`:

```ts
await userEvent.click(screen.getByRole("button", { name: "展开高级项目事实" }));
```

- [ ] **Step 6: Update evidence test to expand advanced first**

Find `creates and verifies project evidence from the governance tab`. Insert this line immediately after rendering:

```ts
await userEvent.click(screen.getByRole("button", { name: "展开高级项目事实" }));
```

- [ ] **Step 7: Update acceptance/archive test to expand advanced first**

Find `submits project acceptance and creates an archive snapshot`. Insert this line immediately after rendering:

```ts
await userEvent.click(screen.getByRole("button", { name: "展开高级项目事实" }));
```

- [ ] **Step 8: Replace route-decision test expectations with advanced-section expectations**

Find `renders route decisions, transfer requests, and resolves pending human decisions`. Replace the assertions before the approval click with:

```ts
await expect.element(screen.getByText("待负责人处理")).toBeInTheDocument();
await expect.element(screen.getByText("需要负责人确认")).toBeInTheDocument();
expect(pageText()).not.toContain("路由决策");
expect(pageText()).not.toContain("转派请求");

await userEvent.click(screen.getByRole("button", { name: "展开高级项目事实" }));

await expect.element(screen.getByText("路由决策")).toBeInTheDocument();
await expect
  .element(screen.getByText("选择项目数字员工池中的 active executor"))
  .toBeInTheDocument();
await expect.element(screen.getByText("转派请求")).toBeInTheDocument();
```

Leave the existing approval click and `vi.waitFor` assertions unchanged.

- [ ] **Step 9: Run the focused test and confirm it fails**

Run:

```bash
corepack pnpm --filter ./apps/web run test -- src/features/projects/index.test.tsx
```

Expected: FAIL. Failures should mention missing simplified labels such as `当前需求`, `项目负责人组`, `高级项目事实`, or the still-visible technical labels.

- [ ] **Step 10: Commit the failing tests**

Commit only the test changes:

```bash
git add apps/web/src/features/projects/index.test.tsx
git commit -m "test(web): capture simplified project main view"
```

## Task 2: Simplify Project Header, Metrics, And Default Main Loop

**Files:**

- Modify: `apps/web/src/features/projects/index.tsx`
- Modify: `apps/web/src/features/projects/components/project-operational-detail.tsx`
- Test: `apps/web/src/features/projects/index.test.tsx`

- [ ] **Step 1: Update page header copy**

In `apps/web/src/features/projects/index.tsx`, replace the page header description:

```tsx
<p className="mt-1 text-sm text-v3-ink-2">
  项目事实、成员池、任务、事件和需求记录
</p>
```

with:

```tsx
<p className="mt-1 text-sm text-v3-ink-2">
  围绕项目负责人、服务池、计划确认、执行进展和最终结果推进闭环
</p>
```

- [ ] **Step 2: Update project list technical column labels**

In `ProjectsV3Table`, replace the `协调线程` column header with `推进状态`.

Replace the table cell that renders `project.coordination_status || "registered"` with:

```tsx
<span className="text-[13px] font-semibold text-v3-ink-2">
  {projectMainLoopLabel(project.status)}
</span>
```

Add this helper near `projectStatusLabel`:

```tsx
function projectMainLoopLabel(status: ProjectStatus | string) {
  if (status === "draft" || status === "configuring") return "待配置";
  if (status === "running") return "推进中";
  if (status === "paused") return "已暂停";
  if (status === "acceptance") return "待确认结果";
  if (status === "archived") return "已关闭";
  return "待推进";
}
```

- [ ] **Step 3: Introduce owner and service-pool derived values**

In `ProjectOperationalDetail`, after:

```tsx
const humanRoles = overview?.human_roles ?? [];
const digitalPool = overview?.digital_employee_pool ?? [];
```

add:

```tsx
const projectOwners = ownerMembers(humanRoles, project.human_owner_user_id);
const servicePool = digitalPool;
const latestDemand = demands[0];
const latestResult = executionSummaries[0];
const pendingOwnerDecisions = decisionRequests.filter(
  (decision) => decision.status_snapshot === "pending",
);
const businessBlocker = projectBusinessBlocker(dispatchGates ?? []);
```

- [ ] **Step 4: Replace hero action copy and remove audit/cost/inline archive prominence**

In the hero button group, keep `提交需求` and `配置`, but remove the visible `审计`, `成本`, and `归档` buttons from the default hero actions.

Replace the button group with:

```tsx
<div className="flex flex-wrap gap-2">
  <V3Button
    disabled={isArchived}
    type="button"
    onClick={onSubmitDemand}
  >
    <FileText data-icon="inline-start" />
    提交需求
  </V3Button>
  <V3Button asChild variant="outline">
    <Link
      params={{ projectId: project.id }}
      to="/projects/$projectId/config"
    >
      <Settings2 data-icon="inline-start" />
      配置项目
    </Link>
  </V3Button>
</div>
```

The archive action will be moved into the advanced section in Task 4.

- [ ] **Step 5: Replace the top metric tiles**

Replace the four current `FactTile` calls (`当前阶段`, `待人工处理`, `证据策略`, `活跃任务`) with:

```tsx
<FactTile
  icon={<GitBranch />}
  label="当前阶段"
  value={projectPhaseLabel(currentPhase)}
/>
<FactTile
  icon={<UserRound />}
  label="待负责人处理"
  value={`${pendingOwnerDecisions.length} 项`}
/>
<FactTile
  icon={<FileText />}
  label="当前需求"
  value={latestDemand?.title ?? "暂无需求"}
/>
<FactTile
  icon={<ClipboardList />}
  label="当前执行"
  value={`${activeTasks.length} 个任务`}
/>
```

Add this helper near `projectStatusLabel`:

```tsx
function projectPhaseLabel(phase: string) {
  const labels: Record<string, string> = {
    acceptance: "待确认结果",
    archived: "已关闭",
    configuring: "配置中",
    draft: "待配置",
    paused: "已暂停",
    running: "执行中",
  };
  return labels[phase] ?? phase;
}
```

Removing the `证据策略` and `待人工处理` tiles leaves three now-unused symbols that will fail the
`noUnusedLocals` / unused-import typecheck. Delete all three:

- The `taskSummary` derived const (`const taskSummary = overview?.task_summary;`).
- The `evidencePolicyConfigured` derived const
  (`const evidencePolicyConfigured = Object.keys(project.evidence_policy ?? {}).length > 0;`).
- The `FileArchive` entry in the `lucide-react` import list (it was only used by the `证据策略` tile).

- [ ] **Step 6: Add default current-demand panel**

Immediately before the plan-version card, add:

```tsx
<SoftCard className="overflow-hidden">
  <PanelHeader
    icon={<FileText />}
    title="当前需求"
    meta={latestDemand ? demandStatusLabel(latestDemand.status) : "暂无需求"}
  />
  {latestDemand ? (
    <div className="grid gap-2 p-4">
      <p className="text-sm font-semibold text-v3-ink">{latestDemand.title}</p>
      <p className="line-clamp-3 text-sm leading-6 text-v3-ink-2">
        {latestDemand.content || "需求内容已记录，等待项目协调线程编排。"}
      </p>
    </div>
  ) : (
    <EmptyLine label="暂无提交到项目的需求" />
  )}
</SoftCard>
```

Add this helper near `projectPhaseLabel`:

```tsx
function demandStatusLabel(status: string) {
  const labels: Record<string, string> = {
    cancelled: "已取消",
    completed: "已完成",
    executing: "执行中",
    failed: "失败",
    planned: "已计划",
    planning_pending: "待计划",
    recorded: "已记录",
    submitted: "待计划",
  };
  return labels[status] ?? status;
}
```

- [ ] **Step 7: Rename plan card to business language**

Change the plan card `PanelHeader` title from `计划版本` to `计划确认`.

Change the plan card empty line from `暂无计划版本` to:

```tsx
<EmptyLine label="暂无计划，提交需求后由协调线程生成。" />
```

- [ ] **Step 8: Rename active task panel to current execution**

In the fallback task panel, change the title from `任务计划` to `当前执行`.

Change the empty line from `当前项目暂无活跃任务` to:

```tsx
<EmptyLine label="当前没有正在执行的数字员工任务" />
```

- [ ] **Step 9: Add latest-result panel**

Immediately after the current-execution panel and before the owner-decision panel, add:

```tsx
<SoftCard className="overflow-hidden">
  <PanelHeader
    icon={<FileCheck2 />}
    title="最新结果"
    meta={latestResult ? "已回写" : "暂无结果"}
  />
  {latestResult ? (
    <div className="grid gap-2 p-4">
      <p className="line-clamp-3 text-sm font-medium text-v3-ink">
        {latestResult.conclusion}
      </p>
      {latestResult.recommended_next_action ? (
        <p className="line-clamp-2 text-xs text-v3-ink-2">
          {latestResult.recommended_next_action}
        </p>
      ) : null}
      <RuntimeMeta label="执行员工" value={latestResult.digital_employee_id} />
    </div>
  ) : (
    <EmptyLine label="数字员工完成任务后会在这里回写结果" />
  )}
</SoftCard>
```

- [ ] **Step 10: Rename owner decisions panel**

Change the `人类决策队列` panel title to `待负责人处理`.

Change its empty line from `当前没有待处理的人类决策` to:

```tsx
<EmptyLine label="当前没有需要项目负责人处理的事项" />
```

- [ ] **Step 11: Add default business blocker panel**

Add `CircleDot` to the `lucide-react` import list in `project-operational-detail.tsx`; it is used by the panel below and is not currently imported.

Immediately after the owner decisions panel, add:

```tsx
{businessBlocker ? (
  <SoftCard className="overflow-hidden">
    <PanelHeader icon={<CircleDot />} title="当前阻塞" meta={businessBlocker.status} />
    <div className="grid gap-2 p-4">
      <p className="text-sm font-semibold text-v3-ink">{businessBlocker.title}</p>
      <p className="text-xs leading-5 text-v3-ink-2">{businessBlocker.description}</p>
    </div>
  </SoftCard>
) : null}
```

- [ ] **Step 12: Add business-blocker helper**

Add this helper near `dispatchGateTone`:

```tsx
function projectBusinessBlocker(gates: DispatchGateResult[]) {
  const latest = gates[0];
  if (!latest || latest.status === "passed") {
    return undefined;
  }
  const blockerKeys = latest.blockers.map((blocker) => blocker.key);
  if (blockerKeys.some((key) => key.includes("runtime"))) {
    return {
      description: "目标运行资源暂不可用。项目负责人无需处理，系统会等待平台资源恢复或稍后重试。",
      status: "等待平台处理",
      title: "运行节点暂不可用，系统会稍后重试",
    };
  }
  if (latest.status === "waiting_human") {
    return {
      description: "当前任务需要负责人确认后才能继续推进。",
      status: "待负责人处理",
      title: "需要负责人确认",
    };
  }
  if (latest.status === "replan_required") {
    return {
      description: "当前计划不再满足执行条件，需要重新编排后继续。",
      status: "需重新计划",
      title: "计划需要调整",
    };
  }
  return {
    description: "当前执行条件尚未满足，系统已保留阻塞原因。",
    status: "待处理",
    title: "执行条件未满足",
  };
}
```

- [ ] **Step 13: Run the focused test**

Run:

```bash
corepack pnpm --filter ./apps/web run test -- src/features/projects/index.test.tsx
```

Expected: Some tests may still fail because advanced panels have not been moved yet, but the new main-loop labels should now render.

- [ ] **Step 14: Commit main-loop header and default panels**

Commit only the changed Web files:

```bash
git add apps/web/src/features/projects/index.tsx apps/web/src/features/projects/components/project-operational-detail.tsx apps/web/src/features/projects/index.test.tsx
git commit -m "feat(web): simplify project owner main view"
```

## Task 3: Rename Owner Group And Service Pool Panels

**Files:**

- Modify: `apps/web/src/features/projects/components/project-operational-detail.tsx`
- Test: `apps/web/src/features/projects/index.test.tsx`

- [ ] **Step 1: Replace member panels in the default aside**

Find the two default `MemberPanel` usages:

```tsx
<MemberPanel
  icon={<UserRound />}
  members={humanRoles}
  title="人类角色"
/>
<MemberPanel
  icon={<Bot />}
  members={digitalPool}
  title="数字员工池"
/>
```

Replace them with:

```tsx
<MemberPanel
  emptyLabel="当前项目尚未设置项目负责人"
  icon={<UserRound />}
  members={projectOwners}
  title="项目负责人组"
/>
<MemberPanel
  emptyLabel="当前项目服务池为空"
  icon={<Bot />}
  members={servicePool}
  title="项目服务池"
/>
```

- [ ] **Step 2: Update `MemberPanel` props**

Replace the `MemberPanel` signature with:

```tsx
function MemberPanel({
  emptyLabel,
  icon,
  members,
  title,
}: {
  emptyLabel: string;
  icon: ReactNode;
  members: ProjectMember[];
  title: string;
}) {
```

Inside `MemberPanel`, replace:

```tsx
<EmptyLine label={`${title}为空`} />
```

with:

```tsx
<EmptyLine label={emptyLabel} />
```

- [ ] **Step 3: Replace member role rendering with business labels**

Inside `MemberPanel`, replace:

```tsx
{member.project_role} · {member.principal_type}
```

with:

```tsx
{projectMemberBusinessLabel(member)}
```

Add this helper near `formatIdList`:

```tsx
function projectMemberBusinessLabel(member: ProjectMember) {
  if (member.principal_type === "digital_employee") {
    const sourceTeam = stringFromUnknown(member.settings?.source_team_name);
    return sourceTeam ? `数字员工 · ${sourceTeam}` : "数字员工";
  }
  if (member.project_role === "owner") {
    return "项目负责人";
  }
  return "项目参与人";
}

function stringFromUnknown(value: unknown) {
  return typeof value === "string" && value.trim() ? value : "";
}
```

- [ ] **Step 4: Add owner fallback helper**

Add this helper near `ownerMembers`:

```tsx
function ownerMembers(members: ProjectMember[], fallbackOwnerID: string) {
  const owners = members.filter(
    (member) =>
      member.principal_type === "human_user" &&
      member.status === "active" &&
      (member.project_role === "owner" || member.principal_id === fallbackOwnerID),
  );
  if (owners.length > 0) {
    return owners;
  }
  if (!fallbackOwnerID) {
    return [];
  }
  return [
    {
      id: `owner-${fallbackOwnerID}`,
      principal_id: fallbackOwnerID,
      principal_type: "human_user" as const,
      project_id: "",
      project_role: "owner" as const,
      settings: {},
      status: "active",
      tenant_id: "",
    },
  ];
}
```

- [ ] **Step 5: Run the focused test**

Run:

```bash
corepack pnpm --filter ./apps/web run test -- src/features/projects/index.test.tsx
```

Expected: Owner/service-pool assertions pass. Advanced-section tests may still fail until Task 4 is complete.

- [ ] **Step 6: Commit member terminology changes**

```bash
git add apps/web/src/features/projects/components/project-operational-detail.tsx apps/web/src/features/projects/index.test.tsx
git commit -m "feat(web): rename project owners and service pool"
```

## Task 4: Move Technical Panels Into An Explicit Advanced Section

**Files:**

- Modify: `apps/web/src/features/projects/components/project-operational-detail.tsx`
- Test: `apps/web/src/features/projects/index.test.tsx`

- [ ] **Step 1: Import collapsible primitives, the `cn` helper, and required icons**

Add imports:

```tsx
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "@/components/ui/collapsible";
import { cn } from "@/lib/utils";
```

`cn` is used by the advanced-section trigger chevron and is not currently imported in this file, so it must be added or typecheck fails.

Add `ChevronDown` to the `lucide-react` import list (used by the advanced-section trigger; not currently imported). `CircleDot` is also required but is imported earlier, in Task 2 Step 11, because the default `当前阻塞` panel uses it before this task runs.

- [ ] **Step 2: Add local advanced-open state**

Change the React import at the top from:

```tsx
import type { ReactNode } from "react";
```

to:

```tsx
import { useState, type ReactNode } from "react";
```

Inside `ProjectOperationalDetail`, after the `if (!project)` block and before derived values, add:

```tsx
const [advancedOpen, setAdvancedOpen] = useState(false);
```

- [ ] **Step 3: Replace the default `DispatchGateSummary` call with an advanced-only call**

Remove this default call from the main section:

```tsx
<DispatchGateSummary
  gates={dispatchGates ?? []}
  taskTitle={dispatchGateTaskTitle}
/>
```

It will be rendered inside the advanced section.

- [ ] **Step 4: Move route decisions, execution trace, governance tabs, and events into advanced content**

Remove these panels from the default main section:

- `路由决策` `SoftCard`
- `ProjectExecutionTracePanel`
- `ProjectGovernanceTabs`

Keep `事件流` in the default main section because recent project events are still useful to project owners.

- [ ] **Step 5: Move technical aside panels into advanced content**

Remove these panels from the default aside:

- `协调任务`
- `执行摘要`
- `转派请求`
- `协调线程`
- `需求记录`

Do not remove the renamed `项目负责人组` and `项目服务池` panels.

- [ ] **Step 6: Add advanced section after the main grid**

Before the closing `</div>` of the root detail container, add:

```tsx
<Collapsible open={advancedOpen} onOpenChange={setAdvancedOpen}>
  <SoftCard className="overflow-hidden">
    <CollapsibleTrigger asChild>
      <button
        className="flex w-full items-center justify-between gap-3 border-b border-v3-line p-4 text-left"
        type="button"
      >
        <span className="flex min-w-0 items-center gap-2">
          <IconTile tone="brand" size="sm" className="size-8 rounded-[10px] [&_svg]:size-3.5">
            <GitBranch />
          </IconTile>
          <span className="min-w-0">
            <span className="block font-semibold text-v3-ink">高级项目事实</span>
            <span className="mt-0.5 block text-xs text-v3-ink-2">
              计划历史、任务图、执行记录、治理、预算、归档和内部协调事实
            </span>
          </span>
        </span>
        <span className="flex shrink-0 items-center gap-2 text-xs font-semibold text-v3-ink-2">
          {advancedOpen ? "收起" : "展开"}
          <ChevronDown
            className={cn("size-4 transition-transform", advancedOpen && "rotate-180")}
          />
        </span>
      </button>
    </CollapsibleTrigger>
    <CollapsibleContent>
      <div className="grid gap-4 p-4 xl:grid-cols-[minmax(0,1.3fr)_minmax(320px,0.7fr)]">
        <section className="grid min-w-0 gap-4">
          <DispatchGateSummary
            gates={dispatchGates ?? []}
            taskTitle={dispatchGateTaskTitle}
          />
          <AdvancedRouteDecisions
            routeDecisions={routeDecisions}
          />
          <ProjectExecutionTracePanel
            errorMessage={executionTraceErrorMessage}
            isError={executionTraceIsError}
            isLoading={executionTraceIsLoading}
            onRetry={onRetryExecutionTrace}
            trace={executionTrace}
          />
          <ProjectGovernanceTabs
            acceptance={acceptance}
            archivePreview={archivePreview}
            archiveSnapshots={archiveSnapshots}
            artifacts={artifacts}
            budgetLedger={budgetLedger}
            budgetSummary={budgetSummary}
            decisionRequestCount={decisionRequests.length}
            demandCount={demands.length}
            evidence={evidence}
            executionSummaryCount={executionSummaries.length}
            onCreateAcceptance={onCreateAcceptance}
            onCreateArchiveSnapshot={onCreateArchiveSnapshot}
            onCreateEvidence={onCreateEvidence}
            onPatchEvidence={onPatchEvidence}
            reports={reports}
            routeDecisionCount={routeDecisions.length}
            taskCount={tasks.length}
          />
        </section>
        <aside className="grid min-w-0 gap-4">
          <AdvancedCoordinationJobs coordinationJobs={coordinationJobs} />
          <AdvancedExecutionSummaries executionSummaries={executionSummaries} />
          <AdvancedTransferRequests transferRequests={transferRequests} />
          <AdvancedWorkflow project={project} overview={overview} />
          <AdvancedDemands demands={demands} />
          <V3Button
            disabled={isArchived}
            type="button"
            variant="outline"
            onClick={onArchiveProject}
          >
            <Archive data-icon="inline-start" />
            归档项目
          </V3Button>
        </aside>
      </div>
    </CollapsibleContent>
  </SoftCard>
</Collapsible>
```

- [ ] **Step 7: Add extracted advanced components**

Move the existing removed panel JSX into focused helper components below `DispatchGateSummary`:

```tsx
function AdvancedRouteDecisions({
  routeDecisions,
}: {
  routeDecisions: ProjectRouteDecision[];
}) {
  return (
    <SoftCard className="overflow-hidden">
      <PanelHeader icon={<GitBranch />} title="路由决策" meta={`${routeDecisions.length} 条`} />
      <div className="divide-y divide-v3-line">
        {routeDecisions.length === 0 ? (
          <EmptyLine label="暂无路由决策" />
        ) : (
          routeDecisions.slice(0, 5).map((decision) => (
            <div className="grid gap-2 p-4" key={decision.id}>
              <div className="flex items-start justify-between gap-3">
                <p className="min-w-0 line-clamp-2 text-sm font-medium">
                  {decision.reason}
                </p>
                {decision.requires_human_review ? (
                  <StatusPill tone="warn">需人工复核</StatusPill>
                ) : (
                  <StatusPill tone="ok">已规划</StatusPill>
                )}
              </div>
              <RuntimeMeta label="已选数字员工" value={formatIdList(decision.selected_digital_employee_ids)} />
              <RuntimeMeta label="候选数字员工" value={formatIdList(decision.candidate_digital_employee_ids)} />
            </div>
          ))
        )}
      </div>
    </SoftCard>
  );
}
```

Add equivalent components by moving existing JSX without behavior changes:

- `AdvancedCoordinationJobs({ coordinationJobs })`
- `AdvancedExecutionSummaries({ executionSummaries })`
- `AdvancedTransferRequests({ transferRequests })`
- `AdvancedWorkflow({ project, overview })`
- `AdvancedDemands({ demands })`

Keep titles exactly:

- `协调任务`
- `执行摘要`
- `转派请求`
- `协调线程`
- `需求记录`

- [ ] **Step 8: Rename dispatch-gate technical header**

In `DispatchGateSummary`, change:

```tsx
<h3 className="text-sm font-semibold text-v3-ink">Pre-dispatch gate</h3>
```

to:

```tsx
<h3 className="text-sm font-semibold text-v3-ink">Dispatch gate 技术详情</h3>
```

Keep the internal blocker key list unchanged because this component is now advanced-only.

- [ ] **Step 9: Run the focused test**

Run:

```bash
corepack pnpm --filter ./apps/web run test -- src/features/projects/index.test.tsx
```

Expected: PASS for `apps/web/src/features/projects/index.test.tsx`.

- [ ] **Step 10: Commit advanced section changes**

```bash
git add apps/web/src/features/projects/components/project-operational-detail.tsx apps/web/src/features/projects/index.test.tsx
git commit -m "feat(web): move project internals to advanced section"
```

## Task 5: Final Verification And Browser Smoke

**Files:**

- Read: `DESIGN.md`
- Read: `.codex/skills/superteam-completion-check/SKILL.md`
- Verify: `apps/web/src/features/projects/index.tsx`
- Verify: `apps/web/src/features/projects/components/project-operational-detail.tsx`
- Verify: `apps/web/src/features/projects/index.test.tsx`

- [ ] **Step 1: Run targeted Web tests**

Run:

```bash
corepack pnpm --filter ./apps/web run test -- src/features/projects/index.test.tsx
```

Expected: PASS.

- [ ] **Step 2: Run Web typecheck**

Run:

```bash
corepack pnpm --filter ./apps/web run typecheck
```

Expected: PASS.

- [ ] **Step 3: Run whitespace check**

Run:

```bash
git diff --check
```

Expected: no output and exit code 0.

- [ ] **Step 4: Check running services**

Run:

```bash
scripts/dev-services.sh status
```

Expected: Web and Control Plane are running. If Web or Control Plane is not running, start or restart only the required service with:

```bash
scripts/dev-services.sh start
```

or:

```bash
scripts/dev-services.sh restart web
scripts/dev-services.sh restart control-plane
```

- [ ] **Step 5: Browser smoke the real project page**

Use the browser or Chrome plugin. Open:

```text
http://127.0.0.1:3000/projects
```

If authentication is required, use the existing local login flow. Verify:

- The project detail shows `项目负责人组`.
- The project detail shows `项目服务池`.
- The project detail shows `当前需求`, `当前执行`, `待负责人处理`, and `最新结果`.
- `Pre-dispatch gate`, `路由决策`, `协调任务`, `执行摘要`, `转派请求`, and `协调线程` are not visible before expanding `高级项目事实`.
- Clicking `展开高级项目事实` reveals route decisions, execution record, governance tabs, and technical dispatch-gate detail.
- There is no horizontal overflow or overlapping text at the current desktop viewport.

- [ ] **Step 6: Commit verification fixes if needed**

If verification required small fixes, commit them:

```bash
git add apps/web/src/features/projects/index.tsx apps/web/src/features/projects/components/project-operational-detail.tsx apps/web/src/features/projects/index.test.tsx
git commit -m "fix(web): polish simplified project main view"
```

If no fixes were needed, do not create an empty commit.

- [ ] **Step 7: Final report**

Report the concrete verification performed using this shape:

```text
真实链路验证：opened /projects against running Web and Control Plane; verified default project view hides internal coordination details until Advanced is expanded.
```

If browser smoke is blocked, use:

```text
阻塞：Web/Control Plane/browser auth unavailable;尚不能声明完成。
```

Do not claim the feature is complete from tests alone if the browser smoke could not run.

## Self-Review Checklist

- Spec coverage:
  - Phase 1 main-view simplification: Tasks 1-4.
  - Existing advanced capabilities reachable: Task 4.
  - No backend/API/persistence changes: File Structure and all tasks.
  - Business labels for project owners and service pool: Task 3.
  - Technical blocker translation: Task 2 and Task 4.
  - Real verification expectation: Task 5.
- Placeholder scan:
  - This plan contains no `TBD`, `TODO`, or open-ended “handle edge cases” steps.
- Type consistency:
  - `ProjectOperationalDetail` already receives all advanced data through existing props.
  - New helpers use existing imported `ProjectMember`, `ProjectStatus`, `DispatchGateResult`, `ProjectRouteDecision`, and component-local data.
  - No new API type is introduced.
  - New imports required and accounted for: `cn` (`@/lib/utils`), `Collapsible*` (`@/components/ui/collapsible`), and lucide icons `ChevronDown` (Task 4) and `CircleDot` (Task 2 Step 11).
  - `noUnusedLocals` compliance: Task 2 Step 5 deletes the now-unused `taskSummary`, `evidencePolicyConfigured`, and the `FileArchive` import after removing the `证据策略`/`待人工处理` tiles.
- Test-locator consistency:
  - `当前需求`, `当前执行`, `待负责人处理`, and the demand title render in both a metric tile and a panel, so Task 1 Step 3 uses `.first()` on those locators to avoid strict multi-match failures.
  - The execution trace panel keeps its existing `执行证据链` title; tests assert that label, not a renamed one.
