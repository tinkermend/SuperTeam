# 团队管理详情页改造设计

**日期：** 2026-07-07  
**状态：** 待实现  
**范围：** `apps/web/src/features/teams/`

---

## 背景与问题

当前团队管理详情页存在以下问题：

1. 概览 Tab 的成员表格把平台管理员混入数字员工列表，语义错误
2. 数字员工行沿用人类角色列（owner/member 等），已与业务模型不符
3. 新建数字员工和"管理"按钮样式与 v3 设计规范不一致
4. 能力与知识 Tab 按钮样式不统一，MCP 绑定表单使用旧版 shadcn 组件
5. 治理策略 Tab 包含"原则"和"Runtime 范围"两个不必要字段；审批策略是裸 JSON textarea，可用性差
6. 借调功能无实际必要性，增加维护成本
7. 审计记录应集中到审计中心，不应挂在团队详情页

---

## 目标

- Tab 结构从 5 个缩减到 3 个，减少认知负担
- 概览 Tab 将人类成员与数字员工完全分离展示
- 治理策略表单结构化，移除无用字段
- 能力与知识页面按钮风格统一为 V3 规范
- 删除借调 Tab
- 删除审计记录 Tab（迁移至审计中心，作为后续独立任务）

---

## Tab 结构

| 改前 | 改后 |
|------|------|
| 概览 | 概览（结构调整） |
| 能力与知识 | 能力与知识（按钮统一） |
| 治理策略 | 治理策略（字段裁剪 + 表单结构化） |
| 借调 | **删除** |
| 审计记录 | **删除**（迁移审计中心，后续任务） |

---

## 详细设计

### 1. Header 区域

`TeamDetailLayout` Header 右侧操作按钮：

- **删除** "添加成员"按钮（下沉到概览 Tab 内部人类成员区块）
- **保留** "创建治理草案"按钮，统一改为 `V3Button size="sm" variant="outline"`
- 禁用 / 归档 / 恢复 / 删除按钮保持不变

---

### 2. 概览 Tab（`team-overview-tab.tsx`）

#### 整体布局（纵向堆叠）

```
KPI 指标卡（4列）
  人类成员数 / 数字员工数 / 绑定能力数 / 待审批项

数字员工区块（全宽，主区块）
  WorkSurface
    header: "数字员工" + "新建数字员工" 按钮
    V3Table: 头像+名称 / 职能标签 / 状态 / 操作

人类管理成员区块（次要区块）
  grid xl:grid-cols-[minmax(0,1fr)_320px]
    左：成员列表（姓名+邮箱 / 角色 / 移除）
    右：添加成员面板（仅 canAddMember 时显示）
```

#### 数字员工表格列

| 列 | 内容 | 实现 |
|----|------|------|
| 数字员工 | `EmployeeAvatar` + 名称 + 描述（截断） | 同当前 |
| 职能 | `StatusPill` 显示 `employee.role`（如 engineer / analyst） | 替换原"角色"列 |
| 状态 | `StatusPill` tone：active=ok, inactive=warn | 同当前 |
| 操作 | `V3Button size="sm" variant="outline"` 文字"详情"，Link 跳转员工详情 | 替换"管理" ghost 按钮 |

空状态：`V3EmptyState` + 引导"新建第一个数字员工"按钮。

"新建数字员工"按钮：`V3Button size="sm" variant="outline"` + `<Bot />` 图标，Link 到 `/employees/new`。

#### 人类管理成员列表列

| 列 | 内容 |
|----|------|
| 成员 | `UserIdentity`（头像 + 名称 + 邮箱） |
| 角色 | `TeamRoleBadge`（owner/admin/approver/member/viewer） |
| 操作 | `V3Button size="icon" variant="ghost"` + `<Trash2 />`，仅 owner 不可自移除 |

添加成员面板（`DirectAddPanel`）：容器从 `SoftCard` 换为 `WorkSurface`，内部表单风格对齐 v3。

#### 数据来源

- 数字员工：`listDigitalEmployees(apiOptions, { team_id: teamId })` — 仅显示该团队数字员工
- 人类成员：`listTeamMembers(apiOptions, teamId)` — 显示所有人类成员（前端不额外过滤，平台管理员如有则显示在人类成员区块，与数字员工区块隔离）

---

### 3. 治理策略 Tab（`team-governance-tab.tsx`）

#### 删除的字段

| 字段 | 动作 |
|------|------|
| `principles` | 删除 `PolicyTextArea` 组件及 `setPrincipales` state、`draftInput.constitution.principles` |
| `runtime_scope_policy` | 删除 `PolicyTextArea` 组件及 `setRuntimeText` state、`draftInput.runtime_scope_policy` |

#### 保留的字段

- `hard_rules`（团队宪法）：保持现有多行文本 `PolicyTextArea`
- `approval_policy`：**改为结构化表单**

#### 审批策略结构化表单

替换原 `PolicyTextArea`（JSON textarea），改为以下字段：

| 字段 | 组件 | 说明 |
|------|------|------|
| 启用审批策略 | `Checkbox` / `Switch` | `approval_policy.enabled: boolean` |
| 风险阈值 | `Select`：low / medium / high | `approval_policy.risk_threshold: string` |
| 必须审批的动作 | `Textarea`（每行一条） | `approval_policy.required_actions: string[]` |
| 最小审批人数 | `Input type="number"` min=1 | `approval_policy.min_approvers: number` |

序列化格式（写入 `approval_policy` JSON 字段，向后兼容）：

```json
{
  "enabled": true,
  "risk_threshold": "medium",
  "required_actions": ["deploy", "delete"],
  "min_approvers": 1
}
```

反序列化：从 `sourceRevision.approval_policy` 读取以上字段，缺失时使用默认值（`enabled: false`, `risk_threshold: "medium"`, `required_actions: []`, `min_approvers: 1`）。

#### 操作按钮

"保存草稿" / "提交负责人批准" / "驳回草稿" 从 shadcn 原生 `Button` 统一换为 `V3Button`，variant 和 size 保持语义对应（保存=默认，提交=outline，驳回=outline）。

---

### 4. 能力与知识 Tab（`team-capabilities-tab.tsx`）

#### 按钮统一规范

| 场景 | 改前 | 改后 |
|------|------|------|
| 安装技能 | `V3Button size="sm" variant="outline"` | 保持，确认一致 |
| 移除技能 | `V3Button size="sm" variant="ghost"` + Trash2 | 改为 `V3Button size="icon" variant="ghost"` + `<Trash2 />` |
| 绑定公共MCP | `V3Button className="w-full"` | 改为 `V3Button variant="outline" className="w-full"` |
| 移除MCP绑定 | `V3Button size="icon" variant="ghost"` | 保持 |

#### MCP 绑定表单布局

- 布局从 `grid md:grid-cols-2` 改为 `flex flex-col gap-3`（竖向堆叠）
- 凭据环境变量输入框：仅在选中某个 MCP 后才显示（`selectedServerId` 非空时渲染）
- 绑定按钮独占一行，全宽

#### 技能表格列

| 改前 | 改后 |
|------|------|
| 技能 / 版本 / 风险 / 操作（4列） | 技能（名称+描述+版本小字）/ 风险 / 操作（3列） |

版本信息降级为名称下方 `text-xs text-v3-ink-3` 显示，版本列删除。

---

### 5. 借调 Tab — 删除

**前端删除：**
- `apps/web/src/features/teams/components/team-lending-tab.tsx` — 整个文件删除
- `team-detail-layout.tsx`：
  - 删除 `canEditLending`、`canDecideLending` 变量
  - 删除 `lending` TabsTrigger + TabsContent

**API 层：**
`apps/web/src/lib/api/teams.ts` 中以下函数及类型前端层删除（后端 API 暂不改动）：
- `getTeamLendingPolicy`
- `upsertTeamLendingPolicy`
- `listTeamLendingRequests`
- `approveTeamLendingRequest`
- `rejectTeamLendingRequest`
- `revokeTeamLendingRequest`
- `TeamLendingPolicy`、`TeamLendingRequest`、`TeamLendingApprovalMode`、`TeamLendingRequestStatus` 类型

---

### 6. 审计记录 Tab — 删除（迁移后续）

**前端删除：**
- `apps/web/src/features/teams/components/team-audit-tab.tsx` — 整个文件删除
- `team-detail-layout.tsx`：删除 `audit` TabsTrigger + TabsContent

**后续任务（不在本次范围）：**
审计中心页面（`/audit` 路由）增加按 team 过滤功能，使用户可从审计中心查看特定团队的审计记录。

---

## 文件变更清单

| 文件 | 动作 |
|------|------|
| `team-detail-layout.tsx` | 修改：删除 lending/audit Tab、调整 Header 按钮、删除借调权限变量 |
| `team-overview-tab.tsx` | 修改：拆分为数字员工区块 + 人类成员区块，调整列定义和按钮样式 |
| `team-governance-tab.tsx` | 修改：删除 principles/runtime_scope 字段，审批策略改结构化表单，按钮换 V3Button |
| `team-capabilities-tab.tsx` | 修改：按钮统一 V3 规范，MCP 表单布局简化，技能表格列裁剪 |
| `team-lending-tab.tsx` | **删除** |
| `team-audit-tab.tsx` | **删除** |
| `apps/web/src/lib/api/teams.ts` | 修改：删除借调相关函数和类型 |

---

## 不在范围内

- 后端 API 改动（借调 API 后端保留，前端停止调用）
- 审计中心的 team 过滤功能实现（后续独立任务）
- 数字员工详情页本身的改造
- 团队创建流程（wizard）改造
