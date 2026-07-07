# 团队管理详情页改造 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 重构团队管理详情页：移除借调和审计Tab，拆分概览Tab为数字员工/人类成员两个独立区块，治理策略改用结构化审批表单，能力与知识页按钮风格统一为 V3 规范。

**Architecture:** 纯前端改造，不涉及后端 API 变动。7 个文件修改（4 modify + 2 delete），1 个 API 层清理。按功能边界拆为 5 个独立任务，每个任务可单独测试和提交。

**Tech Stack:** React, TypeScript, TanStack Query, TanStack Router, Lucide Icons, V3 design system (`@/components/superteam`)

## Global Constraints

- 所有按钮使用 `V3Button`（`@/components/superteam`），禁止使用 shadcn 原生 `Button`
- 表格使用 `V3Table`/`V3Th`/`V3Td`/`V3Tr`，容器使用 `WorkSurface`（`@/components/superteam`）
- 空状态使用 `V3EmptyState`，加载状态使用 `V3LoadingState`
- Web 测试命令：`corepack pnpm --filter ./apps/web run test`，禁止使用 `npx vitest run`
- 页面跳转使用 TanStack Router 的 `Link` 或 `navigate`，禁止 `window.location.href`

---

## File Structure

| 文件 | 动作 |
|------|------|
| `apps/web/src/features/teams/components/team-detail-layout.tsx` | Modify |
| `apps/web/src/features/teams/components/team-overview-tab.tsx` | Modify |
| `apps/web/src/features/teams/components/team-governance-tab.tsx` | Modify |
| `apps/web/src/features/teams/components/team-capabilities-tab.tsx` | Modify |
| `apps/web/src/features/teams/components/team-lending-tab.tsx` | **Delete** |
| `apps/web/src/features/teams/components/team-audit-tab.tsx` | **Delete** |
| `apps/web/src/lib/api/teams.ts` | Modify（删除借调函数和类型） |

---

### Task 1: 删除借调Tab、审计Tab 及借调 API 层

**Files:**
- Delete: `apps/web/src/features/teams/components/team-lending-tab.tsx`
- Delete: `apps/web/src/features/teams/components/team-audit-tab.tsx`
- Modify: `apps/web/src/features/teams/components/team-detail-layout.tsx`
- Modify: `apps/web/src/lib/api/teams.ts`

**Interfaces:**
- Produces: 干净的 3-Tab 布局（概览/能力与知识/治理策略），后续 Task 2-5 在此基础上修改

- [ ] **Step 1: 删除 team-lending-tab.tsx**

```bash
rm apps/web/src/features/teams/components/team-lending-tab.tsx
```

- [ ] **Step 2: 删除 team-audit-tab.tsx**

```bash
rm apps/web/src/features/teams/components/team-audit-tab.tsx
```

- [ ] **Step 3: 清理 teams.ts 中借调相关函数和类型**

打开 `apps/web/src/lib/api/teams.ts`，删除以下内容（从 `// ---- 团队借调（lending）----` 注释行到文件中最后一个借调函数）：

- 类型：`TeamLendingApprovalMode`、`TeamLendingRequestStatus`、`TeamLendingPolicy`、`UpsertTeamLendingPolicyInput`、`TeamLendingRequest`、`DecideTeamLendingRequestInput`
- 函数：`getTeamLendingPolicy`、`upsertTeamLendingPolicy`、`listTeamLendingRequests`、`approveTeamLendingRequest`、`rejectTeamLendingRequest`、`revokeTeamLendingRequest`
- `AllowedTeamAction` 联合类型中删除：`"team.lending.policy.read"`、`"team.lending.policy.edit"`、`"team.lending.request.read"`、`"team.lending.request.decide"`

- [ ] **Step 4: 修改 team-detail-layout.tsx — 删除借调/审计 Tab 及权限变量**

打开 `apps/web/src/features/teams/components/team-detail-layout.tsx`，做以下修改：

4a. 删除 import 行中的 `TeamLendingTab` 和 `TeamAuditTab`（如有）。

4b. 删除以下变量声明：
```tsx
// 删除这两行
const canEditLending = isActive && overview.allowed_actions.includes("team.lending.policy.edit");
const canDecideLending = isActive && overview.allowed_actions.includes("team.lending.request.decide");
```

4c. 删除 `<TabsTrigger value="lending">借调</TabsTrigger>` 和 `<TabsTrigger value="audit">审计记录</TabsTrigger>`。

4d. 删除 `<TabsContent value="lending">...</TabsContent>` 和 `<TabsContent value="audit">...</TabsContent>` 整块。

4e. 删除 Header 区域的"添加成员"按钮（整个 `canAddMember` 条件渲染块）：
```tsx
// 删除这段
{canAddMember ? (
  <V3Button disabled size="sm" variant="outline">
    <UserPlus data-icon="inline-start" />
    添加成员
  </V3Button>
) : null}
```

4f. 将"创建治理草案"按钮的 `disabled` 属性移除（改为由 Tab 内部控制），并确认 variant 为 `"outline"`：
```tsx
{canCreateGovernance ? (
  <V3Button size="sm" variant="outline" onClick={() => {}}>
    <Plus data-icon="inline-start" />
    创建治理草案
  </V3Button>
) : null}
```

- [ ] **Step 5: 运行类型检查确认无报错**

```bash
corepack pnpm --filter ./apps/web run typecheck 2>&1 | tail -20
```

期望：无 lending/audit 相关 TypeScript 报错。

- [ ] **Step 6: 运行测试**

```bash
corepack pnpm --filter ./apps/web run test 2>&1 | tail -30
```

期望：pass（团队相关测试不涉及 lending/audit）。

- [ ] **Step 7: 提交**

```bash
git add apps/web/src/features/teams/components/team-detail-layout.tsx \
        apps/web/src/lib/api/teams.ts
git rm apps/web/src/features/teams/components/team-lending-tab.tsx \
       apps/web/src/features/teams/components/team-audit-tab.tsx
git commit -m "feat(teams): remove lending tab, audit tab and lending API layer"
```

---

### Task 2: 概览Tab — 数字员工区块

**Files:**
- Modify: `apps/web/src/features/teams/components/team-overview-tab.tsx`

**Interfaces:**
- Consumes: `listDigitalEmployees(apiOptions, { team_id: teamId }): Promise<DigitalEmployee[]>` from `@/lib/api/employees`
- Consumes: `EmployeeAvatar` from `@/features/employees/avatar`, `employeeAvatarAsset` from `@/features/employees/avatar-library`
- Produces: `DigitalEmployeesSection` 组件（供 Task 3 整合进页面布局）

- [ ] **Step 1: 删除混合列表逻辑**

在 `TeamOverviewTab` 函数中删除以下内容：

```tsx
// 删除 UnifiedMember 类型定义
type UnifiedMember = { ... }
// 删除 roleOrder 常量
const roleOrder: Record<string, number> = { ... }
// 删除 unifiedList 构建与排序逻辑
const unifiedList: UnifiedMember[] = [...].sort(...)
// 删除 isLoadingList
const isLoadingList = membersQuery.isLoading || digitalEmployeesQuery.isLoading;
```

保留 `membersQuery`、`digitalEmployeesQuery`、`humanRoster`、`digitalRoster`、`existingUserIds`、`addMutation`、`removeMutation` 等。

- [ ] **Step 2: 确认 DigitalEmployee 类型字段**

```bash
grep -nE "role|status|description" apps/web/src/lib/api/employees.ts | grep -E ":\s|role:|status:" | head -20
```

记录 `DigitalEmployee` 是否有 `role`、`status`、`description` 字段。若无 `role`，Step 3 中 `employee.role || "—"` 改为 `"—"`。

- [ ] **Step 3: 新增 DigitalEmployeesSection 组件**

在 `team-overview-tab.tsx` 末尾添加：

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
                        <p className="mt-1.5 truncate text-sm text-v3-ink-2">{employee.description}</p>
                      </div>
                    </div>
                  </V3Td>
                  <V3Td>
                    <StatusPill tone="info">{employee.role || "—"}</StatusPill>
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

- [ ] **Step 4: 运行类型检查**

```bash
corepack pnpm --filter ./apps/web run typecheck 2>&1 | grep -iE "error|team-overview" | head -20
```

期望：无新增错误（注：此时 TeamOverviewTab 的 return 尚未整合，可能有 unused 警告，Task 3 会解决）。

- [ ] **Step 5: 提交**

```bash
git add apps/web/src/features/teams/components/team-overview-tab.tsx
git commit -m "feat(teams/overview): add digital employees section with v3 table"
```

---

### Task 3: 概览Tab — 人类成员区块与页面布局整合

**Files:**
- Modify: `apps/web/src/features/teams/components/team-overview-tab.tsx`

**Interfaces:**
- Consumes: `DigitalEmployeesSection`（Task 2）、`DirectAddPanel`（现有）
- Consumes: `listTeamMembers`、`addTeamMember`、`removeTeamMember` from `@/lib/api/teams`
- Consumes: `UserIdentity` from `@/components/superteam/user-identity`, `TeamRoleBadge` + `type DirectTeamRole` from `@/components/superteam/team-role`
- Produces: 完整的概览 Tab（KPI + 数字员工区块 + 人类成员区块）

- [ ] **Step 1: 新增 HumanMembersSection 组件**

在 `team-overview-tab.tsx` 添加：

```tsx
function HumanMembersSection({
  addPanel,
  canAddMember,
  isLoading,
  members,
  onRemove,
  removing,
}: {
  addPanel: React.ReactNode;
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

- [ ] **Step 2: DirectAddPanel 容器换 WorkSurface**

在现有 `DirectAddPanel` 中，将外层 `<SoftCard className="p-5">` 替换为 `<WorkSurface className="p-5">`，对应闭合标签 `</SoftCard>` 改为 `</WorkSurface>`。删除 `SoftCard` 的 import（若无其他引用）。

- [ ] **Step 3: 重写 TeamOverviewTab 主体 return**

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

- [ ] **Step 4: 清理未使用 imports**

确认 `React` 已 import（`HumanMembersSection` 用到 `React.ReactNode`）。若文件未 import React，改用 `import type { ReactNode } from "react"` 并将类型改为 `ReactNode`。运行 lint：

```bash
corepack pnpm --filter ./apps/web run lint 2>&1 | grep -A2 "team-overview" | head -20
```

期望：无 unused import 报错。

- [ ] **Step 5: 运行测试**

```bash
corepack pnpm --filter ./apps/web run test 2>&1 | tail -30
```

期望：pass。

- [ ] **Step 6: 提交**

```bash
git add apps/web/src/features/teams/components/team-overview-tab.tsx
git commit -m "feat(teams/overview): split human members section, stack layout"
```

---

### Task 4: 治理策略Tab — 删除字段 + 审批策略结构化表单

**Files:**
- Modify: `apps/web/src/features/teams/components/team-governance-tab.tsx`

**Interfaces:**
- Consumes: `Switch` from `@/components/ui/switch`, `Select` 系列 from `@/components/ui/select`, `Input` from `@/components/ui/input`
- Consumes: `GovernanceDraftInput`、`TeamConfigRevision` from `@/lib/api/teams`
- Produces: 结构化审批策略表单，`approval_policy` 序列化为 `{ enabled, risk_threshold, required_actions, min_approvers }`

- [ ] **Step 1: 定义 ApprovalPolicy 类型与解析/序列化辅助函数**

在 `team-governance-tab.tsx` 添加：

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
        ? obj.min_approvers
        : 1,
  };
}

function serializeApprovalPolicy(form: ApprovalPolicyForm): Record<string, unknown> {
  return {
    enabled: form.enabled,
    risk_threshold: form.risk_threshold,
    required_actions: form.required_actions,
    min_approvers: form.min_approvers,
  };
}
```

- [ ] **Step 2: 替换 state — 删除 principles/runtime，改审批为结构化**

在 `TeamGovernanceTab` 中：

删除：
```tsx
const [principlesText, setPrinciplesText] = useState(() => arrayText(sourceRevision?.constitution.principles));
const [approvalText, setApprovalText] = useState(() => jsonText(sourceRevision?.approval_policy));
const [runtimeText, setRuntimeText] = useState(() => jsonText(sourceRevision?.runtime_scope_policy));
```

替换为：
```tsx
const [approvalPolicy, setApprovalPolicy] = useState<ApprovalPolicyForm>(() =>
  parseApprovalPolicy(sourceRevision?.approval_policy),
);
```

保留 `hardRulesText` state。

- [ ] **Step 3: 更新 useEffect 同步逻辑**

```tsx
  useEffect(() => {
    if (!sourceRevision) {
      return;
    }
    setHardRulesText(arrayText(sourceRevision.constitution.hard_rules));
    setApprovalPolicy(parseApprovalPolicy(sourceRevision.approval_policy));
  }, [sourceRevision]);
```

- [ ] **Step 4: 更新 draftInput**

```tsx
  const draftInput = useMemo<GovernanceDraftInput>(
    () => ({
      approval_policy: serializeApprovalPolicy(approvalPolicy),
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

注意：`principles` 从 constitution 中删除（不再覆盖，保留 sourceRevision 原值即通过 spread 继承）；`runtime_scope_policy` 保留 sourceRevision 原值不再编辑。

- [ ] **Step 5: 替换编辑区 JSX**

将 `<CardContent className="flex flex-col gap-4">` 内的 4 个 PolicyTextArea 替换为：

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

- [ ] **Step 6: 新增 ApprovalPolicyEditor 组件**

在文件末尾添加：

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
    <div className="flex flex-col gap-4 rounded-md border p-4">
      <div className="flex items-center gap-2">
        <ShieldCheck />
        <Label>审批策略</Label>
      </div>

      <label className="flex items-center gap-2 text-sm">
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
            <SelectItem value="low">low · 低风险即触发</SelectItem>
            <SelectItem value="medium">medium · 中风险及以上触发</SelectItem>
            <SelectItem value="high">high · 仅高风险触发</SelectItem>
          </SelectContent>
        </Select>
      </div>

      <div className="grid gap-2">
        <Label htmlFor="approval-required-actions">必须审批的动作（每行一条）</Label>
        <Textarea
          disabled={disabled || !value.enabled}
          id="approval-required-actions"
          onChange={(event) =>
            onChange({ ...value, required_actions: lineList(event.target.value) })
          }
          placeholder="deploy&#10;delete"
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
            onChange({ ...value, min_approvers: Math.max(1, Number(event.target.value) || 1) })
          }
          type="number"
          value={value.min_approvers}
        />
      </div>
    </div>
  );
}
```

- [ ] **Step 7: 更新 imports 与操作按钮**

7a. 在 import 中增加：
```tsx
import { Switch } from "@/components/ui/switch";
import { Input } from "@/components/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { V3Button } from "@/components/superteam";
```

7b. 删除不再使用的 `jsonText`、`parseObjectText` 辅助函数（若无其他引用）；保留 `arrayText`、`lineList`。

7c. 将底部 3 个 shadcn `Button`（保存草稿/提交负责人批准/驳回草稿）替换为 `V3Button`，size 保持默认，variant：保存=默认（不填），提交=`outline`，驳回=`outline`。删除 shadcn `Button` 的 import。

7d. 更新变更 diff 区中审批策略行的判断，将 `approvalText.trim()` 改为 `approvalPolicy.enabled`：
```tsx
<Badge variant="outline">{diff.data?.changed_approval_rules ? "有变更" : approvalPolicy.enabled ? "已启用" : "未启用"}</Badge>
```

- [ ] **Step 8: 运行类型检查与测试**

```bash
corepack pnpm --filter ./apps/web run typecheck 2>&1 | grep -iE "error|governance" | head -20
corepack pnpm --filter ./apps/web run test 2>&1 | tail -30
```

期望：无错误，测试 pass。

- [ ] **Step 9: 提交**

```bash
git add apps/web/src/features/teams/components/team-governance-tab.tsx
git commit -m "feat(teams/governance): drop principles/runtime fields, structured approval form"
```

---

### Task 5: 能力与知识Tab — 按钮统一 + MCP 表单简化 + 技能列裁剪

**Files:**
- Modify: `apps/web/src/features/teams/components/team-capabilities-tab.tsx`

**Interfaces:**
- Consumes: 现有 `V3Button`、`V3Table` 等组件（无新增依赖）
- Produces: 统一 V3 按钮风格的能力页

- [ ] **Step 1: MCP 绑定表单布局改为竖向堆叠**

将 MCP 绑定表单的外层 `<div className="grid gap-3 md:grid-cols-[minmax(0,1fr)_minmax(0,1fr)]">` 改为 `<div className="flex flex-col gap-3">`。

- [ ] **Step 2: 凭据环境变量输入框条件渲染**

将凭据环境变量输入框整块用 `{selectedServerId ? (...) : null}` 包裹，仅在选中 MCP 后显示：

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

- [ ] **Step 3: 绑定按钮改 outline + 全宽独占一行**

```tsx
<V3Button variant="outline" className="w-full" disabled={!canCreateMcp} onClick={() => createMcpMutation.mutate()} type="button">
  <Plus data-icon="inline-start" />
  绑定公共 MCP
</V3Button>
```

删除原来包裹按钮的 `<div className="flex min-w-0 items-end">`。

- [ ] **Step 4: 技能表格删除版本列**

在 `SkillTable` 的 thead 中删除 `<V3Th>版本</V3Th>`；在 `SkillRow` 中删除对应的版本 `<V3Td>` 整块。

- [ ] **Step 5: 技能名称行加入版本小字**

在 `SkillRow` 的名称区块中，将描述行下方追加版本：

```tsx
          <div className="min-w-0">
            <p className="truncate text-sm font-medium text-v3-ink">{skill.name}</p>
            <p className="truncate text-xs text-v3-ink-2">{skill.description}</p>
            <p className="mt-0.5 truncate text-xs text-v3-ink-3">v{skill.version}</p>
          </div>
```

- [ ] **Step 6: 移除技能按钮改 icon ghost**

在 `SkillRow` 的操作列，将 installed variant 的移除按钮改为图标按钮。把操作列 `<V3Td>` 改为：

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
          </V3Button>
        ) : (
          <V3Button disabled={!canEdit || pending} onClick={onAction} size="sm" type="button" variant="outline">
            <Plus data-icon="inline-start" />
            {actionLabel}
          </V3Button>
        )}
      </V3Td>
```

- [ ] **Step 7: 运行类型检查与测试**

```bash
corepack pnpm --filter ./apps/web run typecheck 2>&1 | grep -iE "error|capabilities" | head -20
corepack pnpm --filter ./apps/web run test 2>&1 | tail -30
```

期望：无错误，测试 pass。

- [ ] **Step 8: 提交**

```bash
git add apps/web/src/features/teams/components/team-capabilities-tab.tsx
git commit -m "feat(teams/capabilities): unify v3 buttons, simplify mcp form, trim skill columns"
```

---

## 收尾验证（全部任务完成后）

- [ ] **真实端到端验证（项目宪法强制要求）**

按 CLAUDE.md 规则，前端变更收尾必须走真实链路验证：

```bash
scripts/dev-services.sh status   # 确认 Web + Control Plane 运行中当前代码
scripts/dev-services.sh restart web
```

然后通过 Chrome plug（codex chrome plug）打开团队详情页，验证：
1. Tab 只剩 3 个（概览/能力与知识/治理策略），无借调、无审计记录
2. 概览页数字员工区块与人类成员区块分离，数字员工列为 头像+名称/职能/状态/详情
3. 治理策略页无"原则"、无"Runtime 范围"，审批策略为结构化表单（开关/阈值/动作/人数）
4. 能力与知识页按钮统一，MCP 凭据输入框选中后才出现

若服务未启动或认证缺失导致无法验证，标记为阻塞并说明缺失依赖，不得声明完成。

- [ ] **完成前检查**

运行项目 skill `$superteam-completion-check`（`.codex/skills/superteam-completion-check/SKILL.md`）。
