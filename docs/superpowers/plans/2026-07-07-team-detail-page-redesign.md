# 团队管理详情页改造 Implementation Plan
> 复核状态：与配对spec相同——CHANGELOG无明确记录；锚点抽查未发现team-detail-layout.tsx删除lending/audit tab明确证据

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 重构团队管理详情页：移除团队详情内的借调和审计 Tab，拆分概览 Tab 为数字员工/人类成员两个独立区块，治理策略改用结构化审批表单，能力与知识页按钮风格统一为 V3 规范。

**Architecture:** 纯前端页面改造，不修改后端、OpenAPI、迁移或 Control Plane 路由。仅清理 Web 团队详情页不再使用的团队借调 API client；必须保留项目侧 lending API client，因为后端仍提供 `/api/v1/projects/{projectId}/lending-requests`。每个任务结束时仓库必须处于可编译、可测试状态。

**Tech Stack:** React, TypeScript, TanStack Query, TanStack Router, Lucide Icons, V3 design system (`@/components/superteam`)

## Global Constraints

- 前端页面、布局或样式变更前必须阅读 `DESIGN.md`。
- 所有新/改操作按钮使用 `V3Button`（`@/components/superteam`），不要新增 shadcn 原生 `Button`。
- 表格使用 `V3Table`/`V3Th`/`V3Td`/`V3Tr`，密集数据容器使用 `WorkSurface`（`@/components/superteam`）。
- 空状态使用 `V3EmptyState`，加载状态使用 `V3LoadingState`。
- Web 测试命令使用 `corepack pnpm --filter ./apps/web run test -- <test-file>` 或 `corepack pnpm --filter ./apps/web run test`，禁止使用 `npx vitest run`。
- 类型检查命令使用 `corepack pnpm --filter ./apps/web run typecheck`，不要用 `grep`/`tail` 管道截断或掩盖退出码。
- 页面跳转使用 TanStack Router 的 `Link` 或 `navigate`，禁止 `window.location.href`。
- 不新增后端接口、不改 OpenAPI、不删除 Control Plane lending/audit 能力。
- 不要在 Header 放可点击但无行为的 no-op 按钮。

---

## File Structure

| 文件 | 动作 |
|------|------|
| `apps/web/src/features/teams/components/team-detail-layout.tsx` | Modify |
| `apps/web/src/features/teams/components/team-overview-tab.tsx` | Modify |
| `apps/web/src/features/teams/components/team-governance-tab.tsx` | Modify |
| `apps/web/src/features/teams/components/team-capabilities-tab.tsx` | Modify |
| `apps/web/src/features/teams/components/team-lending-tab.tsx` | Delete |
| `apps/web/src/features/teams/components/team-audit-tab.tsx` | Delete |
| `apps/web/src/features/teams/index.test.tsx` | Modify |
| `apps/web/src/lib/api/teams.ts` | Modify narrowly: remove only team-detail unused team-lending policy/decision client; keep project-side lending client |
| `docs/superpowers/plans/2026-07-07-team-detail-page-redesign.md` | Modify: this corrected plan |

---

### Task 1: 删除团队详情借调/审计 Tab，保留项目侧 lending API

**Files:**
- Delete: `apps/web/src/features/teams/components/team-lending-tab.tsx`
- Delete: `apps/web/src/features/teams/components/team-audit-tab.tsx`
- Modify: `apps/web/src/features/teams/components/team-detail-layout.tsx`
- Modify: `apps/web/src/lib/api/teams.ts`
- Modify: `apps/web/src/features/teams/index.test.tsx`

**Interfaces:**
- Consumes: `TeamDetailView` existing route-level test harness in `apps/web/src/features/teams/index.test.tsx`.
- Produces: 3-Tab team detail layout: `概览` / `能力与知识` / `治理策略`.
- Preserves: `CreateProjectLendingRequestInput`, `createProjectLendingRequest`, and `listProjectLendingRequests` in `apps/web/src/lib/api/teams.ts`.

- [ ] **Step 1: Add failing regression test for removed tabs**

In `apps/web/src/features/teams/index.test.tsx`, inside the existing `describe("TeamDetailView")` block, add this test before the action-button tests:

```tsx
  it("shows only overview, capabilities, and governance tabs on team detail", async () => {
    const screen = await renderWithQueryClient(
      <TeamDetailView
        apiBaseUrl="http://control-plane.local"
        fetcher={createTeamsFetcher()}
        teamId="team-1"
      />,
    );

    await expect
      .element(screen.getByRole("heading", { name: "运维团队" }))
      .toBeVisible();
    for (const tab of ["概览", "能力与知识", "治理策略"]) {
      await expect.element(screen.getByRole("tab", { name: tab })).toBeVisible();
    }
    await expect
      .element(screen.getByRole("tab", { name: "借调" }))
      .not.toBeInTheDocument();
    await expect
      .element(screen.getByRole("tab", { name: "审计记录" }))
      .not.toBeInTheDocument();
  });
```

- [ ] **Step 2: Run regression test and verify it fails**

Run:

```bash
corepack pnpm --filter ./apps/web run test -- src/features/teams/index.test.tsx
```

Expected before implementation: FAIL because `借调` and `审计记录` tabs are still rendered.

- [ ] **Step 3: Delete team-specific Tab component files**

Run:

```bash
git rm apps/web/src/features/teams/components/team-lending-tab.tsx \
       apps/web/src/features/teams/components/team-audit-tab.tsx
```

- [ ] **Step 4: Clean `team-detail-layout.tsx` imports and permissions**

In `apps/web/src/features/teams/components/team-detail-layout.tsx`:

1. Change the Lucide import from:

```tsx
import { Archive, Plus, RotateCcw, ShieldCheck, Trash2, UserPlus, UsersRound } from "lucide-react";
```

to:

```tsx
import { Archive, Plus, RotateCcw, ShieldCheck, Trash2, UsersRound } from "lucide-react";
```

2. Delete these imports:

```tsx
import { TeamAuditTab } from "./team-audit-tab";
import { TeamLendingTab } from "./team-lending-tab";
```

3. Delete these variables:

```tsx
const canAddMember = isActive && overview.allowed_actions.includes("team.member.add");
const canEditLending = isActive && overview.allowed_actions.includes("team.lending.policy.edit");
const canDecideLending = isActive && overview.allowed_actions.includes("team.lending.request.decide");
```

- [ ] **Step 5: Remove Header Add Member button and keep governance header non-clickable**

In the Header button group of `team-detail-layout.tsx`, delete the entire disabled `添加成员` block.

Keep the `创建治理草案` button visually present but disabled, using V3 outline style:

```tsx
{canCreateGovernance ? (
  <V3Button disabled size="sm" variant="outline">
    <Plus data-icon="inline-start" />
    创建治理草案
  </V3Button>
) : null}
```

Do not add `onClick={() => {}}`.

- [ ] **Step 6: Remove lending and audit Tab triggers and contents**

In `team-detail-layout.tsx`, delete:

```tsx
the full `<TabsTrigger>` block whose `value` is `"lending"` and label is `借调`
the full `<TabsTrigger>` block whose `value` is `"audit"` and label is `审计记录`
```

Delete the two matching `TabsContent` blocks:

```tsx
the full `<TabsContent className="mt-0" value="lending">` block that renders `TeamLendingTab`
the full `<TabsContent className="mt-0" value="audit">` block that renders `TeamAuditTab`
```

- [ ] **Step 7: Clean only unused team-lending policy/decision API client**

In `apps/web/src/lib/api/teams.ts`:

1. In `AllowedTeamAction`, delete only:

```ts
  | "team.lending.policy.read"
  | "team.lending.policy.edit"
  | "team.lending.request.read"
  | "team.lending.request.decide"
```

2. Delete these team-detail-only types/functions:

```ts
export type TeamLendingPolicy;
export type UpsertTeamLendingPolicyInput;
export type DecideTeamLendingRequestInput;
export async function getTeamLendingPolicy;
export function upsertTeamLendingPolicy;
export async function listTeamLendingRequests;
export function approveTeamLendingRequest;
export function rejectTeamLendingRequest;
export function revokeTeamLendingRequest;
```

Delete the complete existing definitions for these symbols, from each `export type` / `export function` line through its closing `};` or function body.

3. Preserve these project-side lending definitions exactly, because they still match current backend routes:

```ts
export type TeamLendingApprovalMode = "auto" | "manual";
export type TeamLendingRequestStatus =
  | "pending"
  | "auto_approved"
  | "approved"
  | "rejected"
  | "revoked";

export type TeamLendingRequest = {
  id: string;
  tenant_id: string;
  team_id: string;
  project_id: string;
  status: TeamLendingRequestStatus;
  requested_by_user_id: string;
  request_reason: string;
  requested_budget?: string;
  requested_capability: Record<string, unknown>;
  granted_budget?: string;
  granted_capability: Record<string, unknown>;
  is_exception: boolean;
  decided_by_user_id?: string;
  decided_at?: string;
  decision_reason?: string;
  created_at?: string;
  updated_at?: string;
};

export type CreateProjectLendingRequestInput = {
  team_id: string;
  request_reason?: string;
  requested_budget?: string;
  requested_capability?: Record<string, unknown>;
};

export function createProjectLendingRequest(
  options: ApiClientOptions,
  projectId: string,
  input: CreateProjectLendingRequestInput,
): Promise<TeamLendingRequest>

export async function listProjectLendingRequests(
  options: ApiClientOptions,
  projectId: string,
  status?: TeamLendingRequestStatus,
): Promise<TeamLendingRequest[]>
```

- [ ] **Step 8: Remove obsolete audit-tab test expectations**

In `apps/web/src/features/teams/index.test.tsx`, delete the existing test named:

```tsx
it("renders the team audit tab with summary, authorization action, and before after detail", async () => { ... });
```

This test belongs to the removed team detail audit Tab. Do not delete API-level audit tests in `apps/web/src/lib/api/teams.test.ts`; only remove this UI Tab test.

- [ ] **Step 9: Run verification for Task 1**

Run these commands:

```bash
corepack pnpm --filter ./apps/web run test -- src/features/teams/index.test.tsx
corepack pnpm --filter ./apps/web run typecheck
rg -n "TeamLendingTab|TeamAuditTab|value=\"lending\"|value=\"audit\"|team\\.lending\\.policy|team\\.lending\\.request" apps/web/src/features/teams apps/web/src/lib/api/teams.ts
```

Expected:
- Targeted teams test passes.
- Typecheck passes.
- `rg` has no hits for deleted team-detail lending/audit imports, tabs, or allowed actions.

- [ ] **Step 10: Commit Task 1**

```bash
git add apps/web/src/features/teams/components/team-detail-layout.tsx \
        apps/web/src/lib/api/teams.ts \
        apps/web/src/features/teams/index.test.tsx
git commit -m "feat(teams): remove team detail lending and audit tabs"
```

---

### Task 2: 概览 Tab 拆分数字员工与人类成员区块

**Files:**
- Modify: `apps/web/src/features/teams/components/team-overview-tab.tsx`
- Modify: `apps/web/src/features/teams/index.test.tsx`

**Interfaces:**
- Consumes: `listDigitalEmployees(apiOptions, { team_id: teamId }): Promise<DigitalEmployee[]>` from `@/lib/api/employees`.
- Consumes: `EmployeeAvatar` from `@/features/employees/avatar`, `employeeAvatarAsset` from `@/features/employees/avatar-library`.
- Consumes: `UserIdentity`, `TeamRoleBadge`, `DirectTeamRole`, `DirectAddPanel`.
- Produces: `TeamOverviewTab` render with KPI cards, `DigitalEmployeesSection`, and `HumanMembersSection`.

- [ ] **Step 1: Add failing overview split regression test**

In `apps/web/src/features/teams/index.test.tsx`, inside the existing `describe("TeamDetailView")` block, add:

```tsx
  it("separates digital employees from human management members in overview", async () => {
    const screen = await renderWithQueryClient(
      <TeamDetailView
        apiBaseUrl="http://control-plane.local"
        fetcher={createTeamsFetcher()}
        teamId="team-1"
      />,
    );

    await expect.element(screen.getByText("数字员工")).toBeVisible();
    await expect.element(screen.getByText("人类管理成员")).toBeVisible();
    await expect.element(screen.getByText("数据库运维员工")).toBeVisible();
    await expect.element(screen.getByText("负责人甲", { exact: true })).toBeVisible();
    await expect
      .element(screen.getByText("团队成员与代理"))
      .not.toBeInTheDocument();
  });
```

- [ ] **Step 2: Run regression test and verify it fails**

Run:

```bash
corepack pnpm --filter ./apps/web run test -- src/features/teams/index.test.tsx
```

Expected before implementation: FAIL because overview still uses `团队成员与代理` mixed list and does not render `人类管理成员`.

- [ ] **Step 3: Update imports**

In `apps/web/src/features/teams/components/team-overview-tab.tsx`:

1. Add a type import:

```tsx
import type { ReactNode } from "react";
```

2. Keep `useEffect`, `useMemo`, and `useState` value imports.

3. Remove `SoftCard` from the `@/components/superteam` import.

- [ ] **Step 4: Delete mixed-list helper types and sorting**

Delete:

- the complete `type UnifiedMember` definition
- the complete `const roleOrder: Record<string, number>` definition
- the complete `const unifiedList: UnifiedMember[]` construction, including its `.sort` callback
- `const isLoadingList = membersQuery.isLoading || digitalEmployeesQuery.isLoading;`

Keep:

```tsx
const humanRoster = membersQuery.data ?? [];
const digitalRoster = digitalEmployeesQuery.data ?? [];
const existingUserIds = humanRoster.map((member) => member.user_id);
```

- [ ] **Step 5: Replace `TeamOverviewTab` return**

Replace the current return body with:

```tsx
  return (
    <div className="flex flex-col gap-6">
      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
        <V3MetricCard label="人类成员" value={member_count} icon={<Users />} meta="当前团队成员" />
        <V3MetricCard label="数字员工" value={digital_employee_count} icon={<Bot />} iconTone="info" meta="AI 代理执行引擎" />
        <V3MetricCard label="绑定能力" value={capability_count} icon={<Puzzle />} iconTone="artifact" meta="MCP 与外部工具" />
        <V3MetricCard
          label="待审批项"
          value={pending_item_count}
          icon={<TriangleAlert />}
          meta="需人类介入决策"
          iconTone={pending_item_count > 0 ? "warn" : "ok"}
          loud={pending_item_count > 0}
        />
      </div>

      <DigitalEmployeesSection
        employees={digitalRoster}
        isLoading={digitalEmployeesQuery.isLoading}
      />

      <HumanMembersSection
        addPanel={
          <DirectAddPanel
            apiBaseUrl={apiBaseUrl}
            canAdd={canAddMember}
            existingUserIds={existingUserIds}
            fetcher={fetcher}
            isPending={addMutation.isPending}
            onSubmit={(input) => addMutation.mutate(input)}
            resetToken={directAddResetToken}
          />
        }
        canAddMember={canAddMember}
        isLoading={membersQuery.isLoading}
        members={humanRoster}
        onRemove={(membershipId) => removeMutation.mutate(membershipId)}
        removing={removeMutation.isPending}
      />
    </div>
  );
```

- [ ] **Step 6: Add `DigitalEmployeesSection`**

Add this function below `TeamOverviewTab` and above `DirectAddPanel`:

```tsx
function DigitalEmployeesSection({
  employees,
  isLoading,
}: {
  employees: DigitalEmployee[];
  isLoading: boolean;
}) {
  return (
    <WorkSurface>
      <div className="flex flex-col gap-3 border-b border-v3-line px-5 py-4 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <h2 className="text-base font-bold text-v3-ink">数字员工</h2>
          <p className="mt-1 text-[13px] text-v3-ink-2">团队当前绑定的数字员工。</p>
        </div>
        <V3Button asChild size="sm" variant="outline">
          <Link to="/employees/new">
            <Bot data-icon="inline-start" className="mr-1" />
            新建数字员工
          </Link>
        </V3Button>
      </div>
      <div>
        {isLoading ? (
          <V3LoadingState label="加载数字员工" />
        ) : employees.length === 0 ? (
          <V3EmptyState
            title="团队暂无数字员工"
            action={
              <V3Button asChild size="sm" variant="outline">
                <Link to="/employees/new">
                  <Bot data-icon="inline-start" className="mr-1" />
                  新建第一个数字员工
                </Link>
              </V3Button>
            }
          />
        ) : (
          <V3Table>
            <thead>
              <tr>
                <V3Th>数字员工</V3Th>
                <V3Th>职能</V3Th>
                <V3Th>状态</V3Th>
                <V3Th className="text-right">操作</V3Th>
              </tr>
            </thead>
            <tbody>
              {employees.map((employee) => (
                <V3Tr key={employee.id}>
                  <V3Td>
                    <div className="flex min-w-0 items-center gap-3">
                      <EmployeeAvatar
                        asset={employeeAvatarAsset(employee)}
                        name={employee.name}
                        size="md"
                      />
                      <div className="min-w-0">
                        <p className="truncate font-medium leading-none text-v3-ink">{employee.name}</p>
                        <p className="mt-1.5 truncate text-sm text-v3-ink-2">{employee.description || "执行代理"}</p>
                      </div>
                    </div>
                  </V3Td>
                  <V3Td>
                    <StatusPill tone="info">{employee.role || "未设置"}</StatusPill>
                  </V3Td>
                  <V3Td>
                    <StatusPill tone={employee.status === "active" ? "ok" : "warn"}>
                      {employee.status}
                    </StatusPill>
                  </V3Td>
                  <V3Td className="text-right">
                    <V3Button asChild size="sm" variant="outline">
                      <Link to="/employees/$employeeId" params={{ employeeId: employee.id }}>
                        详情
                      </Link>
                    </V3Button>
                  </V3Td>
                </V3Tr>
              ))}
            </tbody>
          </V3Table>
        )}
      </div>
    </WorkSurface>
  );
}
```

- [ ] **Step 7: Add `HumanMembersSection`**

Add this function below `DigitalEmployeesSection`:

```tsx
function HumanMembersSection({
  addPanel,
  canAddMember,
  isLoading,
  members,
  onRemove,
  removing,
}: {
  addPanel: ReactNode;
  canAddMember: boolean;
  isLoading: boolean;
  members: TeamMember[];
  onRemove: (membershipId: string) => void;
  removing: boolean;
}) {
  return (
    <div className="grid gap-6 xl:grid-cols-[minmax(0,1fr)_320px]">
      <WorkSurface className="min-w-0">
        <div className="border-b border-v3-line px-5 py-4">
          <h2 className="text-base font-bold text-v3-ink">人类管理成员</h2>
          <p className="mt-1 text-[13px] text-v3-ink-2">团队的管理、审批与观察人员。</p>
        </div>
        <div>
          {isLoading ? (
            <V3LoadingState label="加载人类成员" />
          ) : members.length === 0 ? (
            <V3EmptyState title="暂无人类成员" />
          ) : (
            <V3Table>
              <thead>
                <tr>
                  <V3Th>成员</V3Th>
                  <V3Th>角色</V3Th>
                  <V3Th className="text-right">操作</V3Th>
                </tr>
              </thead>
              <tbody>
                {members.map((member) => (
                  <V3Tr key={member.membership_id}>
                    <V3Td>
                      <UserIdentity
                        showSecondary
                        user={{
                          id: member.user_id,
                          username: member.username,
                          display_name: member.display_name,
                          email: member.email,
                          avatar: member.avatar,
                          status: member.account_status || "active",
                        }}
                      />
                    </V3Td>
                    <V3Td>
                      <TeamRoleBadge role={member.role as DirectTeamRole} />
                    </V3Td>
                    <V3Td className="text-right">
                      {member.role === "owner" ? (
                        <span className="text-xs text-v3-ink-3">—</span>
                      ) : (
                        <V3Button
                          aria-label={`移除 ${member.display_name || member.username}`}
                          disabled={removing}
                          onClick={() => onRemove(member.membership_id)}
                          size="icon"
                          type="button"
                          variant="ghost"
                        >
                          <Trash2 className="size-4" />
                          <span className="sr-only">移除</span>
                        </V3Button>
                      )}
                    </V3Td>
                  </V3Tr>
                ))}
              </tbody>
            </V3Table>
          )}
        </div>
      </WorkSurface>
      {canAddMember ? <aside className="flex min-w-0 flex-col gap-4">{addPanel}</aside> : null}
    </div>
  );
}
```

- [ ] **Step 8: Convert `DirectAddPanel` shell to `WorkSurface`**

In `DirectAddPanel`, replace:

```tsx
<SoftCard className="p-5">
```

with:

```tsx
<WorkSurface className="p-5">
```

Replace the closing `</SoftCard>` with `</WorkSurface>`.

- [ ] **Step 9: Run verification for Task 2**

Run:

```bash
corepack pnpm --filter ./apps/web run test -- src/features/teams/index.test.tsx
corepack pnpm --filter ./apps/web run typecheck
rg -n "UnifiedMember|roleOrder|团队成员与代理|SoftCard" apps/web/src/features/teams/components/team-overview-tab.tsx
```

Expected:
- Targeted teams test passes.
- Typecheck passes.
- `rg` has no hits.

- [ ] **Step 10: Commit Task 2**

```bash
git add apps/web/src/features/teams/components/team-overview-tab.tsx \
        apps/web/src/features/teams/index.test.tsx
git commit -m "feat(teams/overview): split digital and human member sections"
```

---

### Task 3: 治理策略 Tab 改为结构化审批表单

**Files:**
- Modify: `apps/web/src/features/teams/components/team-governance-tab.tsx`
- Modify: `apps/web/src/features/teams/index.test.tsx`

**Interfaces:**
- Consumes: `Switch` from `@/components/ui/switch`, `Select` series from `@/components/ui/select`, `Input` from `@/components/ui/input`, `Textarea` from `@/components/ui/textarea`.
- Consumes: `GovernanceDraftInput`, `TeamConfigRevision` from `@/lib/api/teams`.
- Produces: UI-only structured approval editor; serialized `approval_policy` preserves unknown existing keys and overrides `enabled`, `risk_threshold`, `required_actions`, `min_approvers`.
- Preserves: existing `constitution.principles` and `runtime_scope_policy` values from `sourceRevision`; these fields are no longer editable in this Tab.

- [ ] **Step 1: Add failing governance form regression test**

In `apps/web/src/features/teams/index.test.tsx`, inside the existing `describe("TeamDetailView")` block, add:

```tsx
  it("uses a structured approval policy editor without principles or runtime fields", async () => {
    const screen = await renderWithQueryClient(
      <TeamDetailView
        apiBaseUrl="http://control-plane.local"
        fetcher={createTeamsFetcher()}
        teamId="team-1"
      />,
    );

    await userEvent.click(screen.getByRole("tab", { name: "治理策略" }));

    await expect.element(screen.getByText("审批策略")).toBeVisible();
    await expect.element(screen.getByLabelText("启用审批策略")).toBeVisible();
    await expect.element(screen.getByLabelText("风险阈值")).toBeVisible();
    await expect.element(screen.getByLabelText("必须审批的动作（每行一条）")).toBeVisible();
    await expect.element(screen.getByLabelText("最小审批人数")).toBeVisible();
    await expect.element(screen.getByLabelText("原则")).not.toBeInTheDocument();
    await expect.element(screen.getByLabelText("Runtime 范围")).not.toBeInTheDocument();
  });
```

- [ ] **Step 2: Run regression test and verify it fails**

Run:

```bash
corepack pnpm --filter ./apps/web run test -- src/features/teams/index.test.tsx
```

Expected before implementation: FAIL because approval policy is still a JSON textarea.

- [ ] **Step 3: Update imports**

In `apps/web/src/features/teams/components/team-governance-tab.tsx`:

1. Replace:

```tsx
import { Button } from "@/components/ui/button";
```

with:

```tsx
import { V3Button } from "@/components/superteam";
```

2. Add:

```tsx
import { Input } from "@/components/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Switch } from "@/components/ui/switch";
```

- [ ] **Step 4: Add approval policy form helpers**

Add below `TeamGovernanceTabProps`:

```tsx
type ApprovalPolicyForm = {
  enabled: boolean;
  risk_threshold: "low" | "medium" | "high";
  required_actions: string[];
  min_approvers: number;
};

const DEFAULT_APPROVAL_POLICY: ApprovalPolicyForm = {
  enabled: false,
  risk_threshold: "medium",
  required_actions: [],
  min_approvers: 1,
};

function parseApprovalPolicy(value: unknown): ApprovalPolicyForm {
  if (!value || typeof value !== "object" || Array.isArray(value)) {
    return { ...DEFAULT_APPROVAL_POLICY };
  }
  const obj = value as Record<string, unknown>;
  const threshold = obj.risk_threshold;
  return {
    enabled: typeof obj.enabled === "boolean" ? obj.enabled : false,
    risk_threshold:
      threshold === "low" || threshold === "medium" || threshold === "high"
        ? threshold
        : "medium",
    required_actions: Array.isArray(obj.required_actions)
      ? obj.required_actions.filter((item): item is string => typeof item === "string")
      : [],
    min_approvers:
      typeof obj.min_approvers === "number" && obj.min_approvers >= 1
        ? Math.floor(obj.min_approvers)
        : 1,
  };
}

function approvalPolicyObject(value: unknown): Record<string, unknown> {
  if (!value || typeof value !== "object" || Array.isArray(value)) {
    return {};
  }
  return value as Record<string, unknown>;
}

function serializeApprovalPolicy(
  form: ApprovalPolicyForm,
  sourcePolicy: unknown,
): Record<string, unknown> {
  return {
    ...approvalPolicyObject(sourcePolicy),
    enabled: form.enabled,
    risk_threshold: form.risk_threshold,
    required_actions: form.required_actions,
    min_approvers: form.min_approvers,
  };
}
```

- [ ] **Step 5: Replace local state and sync**

In `TeamGovernanceTab`, replace:

```tsx
const [principlesText, setPrinciplesText] = useState(() => arrayText(sourceRevision?.constitution.principles));
const [approvalText, setApprovalText] = useState(() => jsonText(sourceRevision?.approval_policy));
const [runtimeText, setRuntimeText] = useState(() => jsonText(sourceRevision?.runtime_scope_policy));
```

with:

```tsx
const [approvalPolicy, setApprovalPolicy] = useState<ApprovalPolicyForm>(() =>
  parseApprovalPolicy(sourceRevision?.approval_policy),
);
```

Update the `useEffect` body to:

```tsx
    setHardRulesText(arrayText(sourceRevision.constitution.hard_rules));
    setApprovalPolicy(parseApprovalPolicy(sourceRevision.approval_policy));
```

- [ ] **Step 6: Update `draftInput`**

Replace the current `draftInput` with:

```tsx
  const draftInput = useMemo<GovernanceDraftInput>(
    () => ({
      approval_policy: serializeApprovalPolicy(approvalPolicy, sourceRevision?.approval_policy),
      artifact_contract: sourceRevision?.artifact_contract ?? {},
      capability_policy: sourceRevision?.capability_policy ?? {},
      constitution: {
        ...(sourceRevision?.constitution ?? {}),
        hard_rules: lineList(hardRulesText),
      },
      context_policy: sourceRevision?.context_policy ?? {},
      human_owner_user_ids: sourceRevision?.human_owner_user_ids,
      internal_collaboration_policy: sourceRevision?.internal_collaboration_policy ?? {},
      runtime_scope_policy: sourceRevision?.runtime_scope_policy ?? {},
    }),
    [approvalPolicy, hardRulesText, sourceRevision],
  );
```

- [ ] **Step 7: Replace editor JSX**

In the first card's `CardContent`, keep only `PolicyTextArea` for `团队宪法` and add `ApprovalPolicyEditor`:

```tsx
        <CardContent className="flex flex-col gap-4">
          <PolicyTextArea
            description="每行一条负责人必须确认的硬性规则。"
            disabled={!canEdit}
            label="团队宪法"
            onChange={setHardRulesText}
            value={hardRulesText}
          />
          <ApprovalPolicyEditor
            disabled={!canEdit}
            onChange={setApprovalPolicy}
            value={approvalPolicy}
          />
        </CardContent>
```

- [ ] **Step 8: Add `ApprovalPolicyEditor`**

Add this component below `PolicyTextArea`:

```tsx
function ApprovalPolicyEditor({
  disabled,
  onChange,
  value,
}: {
  disabled: boolean;
  onChange: (value: ApprovalPolicyForm) => void;
  value: ApprovalPolicyForm;
}) {
  return (
    <div className="flex flex-col gap-4 rounded-md border border-v3-line bg-v3-card-soft p-4">
      <div className="flex items-center gap-2">
        <ShieldCheck className="size-4" />
        <Label>审批策略</Label>
      </div>

      <label className="flex items-center gap-2 text-sm text-v3-ink">
        <Switch
          checked={value.enabled}
          disabled={disabled}
          onCheckedChange={(checked) => onChange({ ...value, enabled: checked })}
        />
        启用审批策略
      </label>

      <div className="grid gap-2">
        <Label htmlFor="approval-risk-threshold">风险阈值触发审批</Label>
        <Select
          disabled={disabled || !value.enabled}
          onValueChange={(next) =>
            onChange({ ...value, risk_threshold: next as ApprovalPolicyForm["risk_threshold"] })
          }
          value={value.risk_threshold}
        >
          <SelectTrigger aria-label="风险阈值" id="approval-risk-threshold">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="low">low - 低风险即触发</SelectItem>
            <SelectItem value="medium">medium - 中风险及以上触发</SelectItem>
            <SelectItem value="high">high - 仅高风险触发</SelectItem>
          </SelectContent>
        </Select>
      </div>

      <div className="grid gap-2">
        <Label htmlFor="approval-required-actions">必须审批的动作（每行一条）</Label>
        <Textarea
          aria-label="必须审批的动作（每行一条）"
          disabled={disabled || !value.enabled}
          id="approval-required-actions"
          onChange={(event) =>
            onChange({ ...value, required_actions: lineList(event.target.value) })
          }
          placeholder={"deploy\ndelete"}
          rows={3}
          value={value.required_actions.join("\n")}
        />
      </div>

      <div className="grid gap-2">
        <Label htmlFor="approval-min-approvers">最小审批人数</Label>
        <Input
          disabled={disabled || !value.enabled}
          id="approval-min-approvers"
          min={1}
          onChange={(event) =>
            onChange({ ...value, min_approvers: Math.max(1, Math.floor(Number(event.target.value) || 1)) })
          }
          type="number"
          value={value.min_approvers}
        />
      </div>
    </div>
  );
}
```

- [ ] **Step 9: Update diff badge, buttons, and dead helpers**

1. Replace the approval diff badge with:

```tsx
<Badge variant="outline">{diff.data?.changed_approval_rules ? "有变更" : approvalPolicy.enabled ? "已启用" : "未启用"}</Badge>
```

2. Replace the three bottom `Button` elements with `V3Button`:

```tsx
<V3Button disabled={!canEdit || saveMutation.isPending} onClick={() => saveMutation.mutate()}>
  <Save data-icon="inline-start" />
  保存草稿
</V3Button>
<V3Button
  disabled={!canApprove || approveMutation.isPending || !draftID}
  onClick={() => approveMutation.mutate()}
  variant="outline"
>
  <Send data-icon="inline-start" />
  提交负责人批准
</V3Button>
<V3Button
  disabled={!canApprove || rejectMutation.isPending || !draftID}
  onClick={() => rejectMutation.mutate()}
  variant="outline"
>
  <XCircle data-icon="inline-start" />
  驳回草稿
</V3Button>
```

3. Delete unused `jsonText` and `parseObjectText` helper functions.

- [ ] **Step 10: Run verification for Task 3**

Run:

```bash
corepack pnpm --filter ./apps/web run test -- src/features/teams/index.test.tsx
corepack pnpm --filter ./apps/web run typecheck
rg -n "principlesText|approvalText|runtimeText|jsonText|parseObjectText|<Button" apps/web/src/features/teams/components/team-governance-tab.tsx
```

Expected:
- Targeted teams test passes.
- Typecheck passes.
- `rg` has no hits.

- [ ] **Step 11: Commit Task 3**

```bash
git add apps/web/src/features/teams/components/team-governance-tab.tsx \
        apps/web/src/features/teams/index.test.tsx
git commit -m "feat(teams/governance): add structured approval editor"
```

---

### Task 4: 能力与知识 Tab 统一按钮与简化 MCP 表单

**Files:**
- Modify: `apps/web/src/features/teams/components/team-capabilities-tab.tsx`
- Modify: `apps/web/src/features/teams/index.test.tsx`

**Interfaces:**
- Consumes: existing `V3Button`, `V3Table`, `StatusPill`, `WorkSurface`.
- Produces: public MCP form with vertical layout, conditional credential field, outline bind button, skill version folded into skill identity cell, installed skill removal as icon ghost button.

- [ ] **Step 1: Add failing capabilities interaction regression test**

In `apps/web/src/features/teams/index.test.tsx`, inside the existing `describe("TeamDetailView")` block, add:

```tsx
  it("shows MCP credential input only after selecting a registry MCP", async () => {
    const fetcher = createTeamsFetcher({
      extraRoutes: {
        "GET /api/v1/mcp-servers": [
          {
            id: "mcp-github",
            tenant_id: "tenant-1",
            name: "GitHub MCP",
            server_key: "github",
            description: "",
            transport: "streamable_http",
            url: "https://api.githubcopilot.com/mcp/",
            auth_strategy: "bearer_env",
            required_env_vars: ["GITHUB_TOKEN"],
            optional_env_vars: [],
            tool_allowlist: [],
            risk_level: "medium",
            status: "active",
          },
        ],
        "GET /api/v1/teams/team-1/mcp-bindings": [],
        "GET /api/v1/skills": [],
        "GET /api/v1/teams/team-1/skills": [],
      },
    });
    const screen = await renderWithQueryClient(
      <TeamDetailView
        apiBaseUrl="http://control-plane.local"
        fetcher={fetcher}
        teamId="team-1"
      />,
    );

    await userEvent.click(screen.getByRole("tab", { name: "能力与知识" }));

    await expect
      .element(screen.getByRole("textbox", { name: "凭据环境变量（可选）" }))
      .not.toBeInTheDocument();
    await userEvent.click(screen.getByRole("combobox", { name: "注册表 MCP" }));
    await userEvent.click(screen.getByRole("option", { name: "GitHub MCP（github）" }));
    await expect
      .element(screen.getByRole("textbox", { name: "凭据环境变量（可选）" }))
      .toBeVisible();
  });
```

- [ ] **Step 2: Run regression test and verify it fails**

Run:

```bash
corepack pnpm --filter ./apps/web run test -- src/features/teams/index.test.tsx
```

Expected before implementation: FAIL because credential input is always visible.

- [ ] **Step 3: Make MCP form vertical**

In `apps/web/src/features/teams/components/team-capabilities-tab.tsx`, replace:

```tsx
<div className="grid gap-3 md:grid-cols-[minmax(0,1fr)_minmax(0,1fr)]">
```

with:

```tsx
<div className="flex flex-col gap-3">
```

- [ ] **Step 4: Conditionally render credential input**

Wrap the credential input block with:

```tsx
{selectedServerId ? (
  <div className="min-w-0 space-y-2">
    <Label htmlFor="team-mcp-credential-env">凭据环境变量（可选）</Label>
    <Input
      disabled={!canEdit || createMcpMutation.isPending}
      id="team-mcp-credential-env"
      onChange={(event) => setCredentialEnvVar(event.target.value)}
      placeholder="例如 GITHUB_TOKEN"
      value={credentialEnvVar}
    />
  </div>
) : null}
```

- [ ] **Step 5: Make bind button outline and remove wrapper**

Delete the wrapper:

```tsx
<div className="flex min-w-0 items-end">
  <V3Button className="w-full" disabled={!canCreateMcp} onClick={() => createMcpMutation.mutate()} type="button">
    <Plus data-icon="inline-start" />
    绑定公共 MCP
  </V3Button>
</div>
```

Place this button directly in the form stack:

```tsx
<V3Button
  className="w-full"
  disabled={!canCreateMcp}
  onClick={() => createMcpMutation.mutate()}
  type="button"
  variant="outline"
>
  <Plus data-icon="inline-start" />
  绑定公共 MCP
</V3Button>
```

- [ ] **Step 6: Remove skill version column**

In `SkillTable`, remove the version header:

```tsx
<V3Th>版本</V3Th>
```

In `SkillRow`, remove the version cell:

```tsx
<V3Td>
  <StatusPill tone="mute" showDot={false}>
    {skill.version}
  </StatusPill>
</V3Td>
```

- [ ] **Step 7: Fold version into skill identity cell**

Replace the skill identity text block with:

```tsx
<div className="min-w-0">
  <p className="truncate text-sm font-medium text-v3-ink">{skill.name}</p>
  <p className="truncate text-xs text-v3-ink-2">{skill.description}</p>
  <p className="mt-0.5 truncate text-xs text-v3-ink-3">v{skill.version}</p>
</div>
```

- [ ] **Step 8: Convert installed remove action to icon ghost button**

Replace the `SkillRow` operation cell with:

```tsx
<V3Td className="text-right">
  {variant === "installed" ? (
    <V3Button
      aria-label={actionLabel}
      disabled={!canEdit || pending}
      onClick={onAction}
      size="icon"
      type="button"
      variant="ghost"
    >
      <Trash2 className="size-4" />
      <span className="sr-only">{actionLabel}</span>
    </V3Button>
  ) : (
    <V3Button disabled={!canEdit || pending} onClick={onAction} size="sm" type="button" variant="outline">
      <Plus data-icon="inline-start" />
      {actionLabel}
    </V3Button>
  )}
</V3Td>
```

- [ ] **Step 9: Run verification for Task 4**

Run:

```bash
corepack pnpm --filter ./apps/web run test -- src/features/teams/index.test.tsx
corepack pnpm --filter ./apps/web run typecheck
rg -n "<V3Th>版本</V3Th>|items-end|md:grid-cols\\[minmax\\(0,1fr\\)_minmax\\(0,1fr\\)\\]" apps/web/src/features/teams/components/team-capabilities-tab.tsx
```

Expected:
- Targeted teams test passes.
- Typecheck passes.
- `rg` has no hits.

- [ ] **Step 10: Commit Task 4**

```bash
git add apps/web/src/features/teams/components/team-capabilities-tab.tsx \
        apps/web/src/features/teams/index.test.tsx
git commit -m "feat(teams/capabilities): simplify mcp form and skill actions"
```

---

## 收尾验证（全部任务完成后）

- [ ] **Full local verification**

Run:

```bash
corepack pnpm --filter ./apps/web run test -- src/features/teams/index.test.tsx
corepack pnpm --filter ./apps/web run typecheck
corepack pnpm --filter ./apps/web run test
git diff --check
```

- [ ] **真实端到端验证（项目宪法强制要求）**

Run:

```bash
scripts/dev-services.sh status
scripts/dev-services.sh restart web
scripts/dev-services.sh status
```

Then use Codex Chrome plug / browser automation to open the running team detail page and verify:

1. Tab 只剩 3 个：`概览` / `能力与知识` / `治理策略`，无 `借调`、无 `审计记录`。
2. 概览页数字员工区块与人类成员区块分离，数字员工列为头像+名称/职能/状态/详情。
3. 治理策略页无 `原则`、无 `Runtime 范围`，审批策略为结构化表单（开关/阈值/动作/人数）。
4. 能力与知识页按钮统一，MCP 凭据输入框选中 MCP 后才出现。

If services are down, auth is missing, or the route cannot be opened against the current running Web + Control Plane, mark the feature blocked and do not claim real-chain completion.

- [ ] **完成前检查**

Read and run project skill `$superteam-completion-check` (`.codex/skills/superteam-completion-check/SKILL.md`) before final status, commit finish, merge, branch cleanup, or PR.
