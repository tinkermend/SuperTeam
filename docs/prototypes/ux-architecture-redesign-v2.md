# SuperTeam 用户体验架构 · 精炼 v2（基于现状对账）

> 交付方: ArchitectUX（用户体验架构师）
> 基线: 2026-07-06 初版方案 `ux-architecture-redesign.md`
> 本版: 对账当前代码状态（2026-07-06 晚），收敛落地范围
> 状态: 导航重组 + 审批中心 **已落地**；剩余断点集中在「页间跳转 + 面包屑」（第一期）与「项目中枢 + 审批闭环」（第二期）

---

## 0. 现状对账（关键变化）

初版方案诊断的 12 处断点 + 导航 3 组，经代码核查，已有两项实质性落地：

| 原诊断项 | 当前状态 | 证据 |
| --- | --- | --- |
| 导航 3 组 21 项 → 需重组为 4 组 | ✅ **已落地** | `apps/web/src/components/layout/data/sidebar-data.ts` 现为 4 组（工作台 / 协作对象 / 流程能力 / 治理平台），运行总览已在工作台，审批中心已入导航 |
| `/approvals` 占位 `UnimplementedPage` | ✅ **已落地** | `routes/_authenticated/approvals/index.tsx` → `ApprovalsCenterPage`，复用 `listInboxItems` + `InboxActionDialog`，是 inbox 审批项的聚合视图 |
| 全局面包屑 | ❌ 未做 | `shell-page-header.tsx` 仅有 `ShellPageHeaderBack`，无面包屑组件 |
| 项目详情 Member 死链 / 任务行无跳转 | ❌ 未做 | `projects/index.tsx` 无 `V3Tabs` / 跨链接 / `ExternalLink` 包裹 |
| run-overview 零外链 / 异常无处置 | ❌ 未做 | `run-overview/index.tsx` 无 `Link` / `useNavigate` / `去处理` |
| 员工详情 参与项目 / 产出工件 / 死按钮 | ❌ 未做 | `employees/detail.tsx` 仅返回链，无跨区；`QueueRow` 无 onClick |
| 项目详情七 Tab 中枢 | ❌ 未做 | 仍为纵向堆叠 + 折叠 |
| inbox 审批带上下文 + deep-link + 回流 | ❌ 未做 | `InboxActionDialog` 仍仅 `action.label` + 评论框 |

**结论**：信息架构骨架（导航 + 审批中心）已就位。**体验断点收敛到「页内/页间跳转」与「项目中枢」两层** —— 恰好是初版第一期 + 第二期。因此本版把落地路线从「三期」修订为「两期」。

---

## 1. 五条设计原则（不变，仍是架构地基）

1. **项目即工作中枢** —— 项目是业务事实聚合容器（AGENTS.md 已定义），详情页升级为完整工作面。
2. **上下文贯穿** —— 全局面包屑 + deep-link 锚点 + 动作后回流，杜绝单向流失。
3. **人类决策带上下文** —— 审批不是孤立动作，dialog 必须携带发起人 / 原因 / 风险 / 证据链。
4. **可观测即处置** —— 异常标注旁边就是处置入口，不让人在页面间搬运上下文。
5. **空状态即引导** —— 每个空状态都指向下一步，消灭死胡同。

---

## 2. 目标信息架构（已实现部分高亮）

### 2.1 导航（已落地，仅补清理）
四组已成事实。仅剩清理项：
- `/tasks`、`/task-launches/$demandId`（重定向到 `/workflows/$demandId`）确认是否保留路由。
- 命令中心（Cmd+K）需覆盖孤岛页（若有），保证每个一级路由可达。

### 2.2 项目详情中枢（待建，这是体验主轴）
顶层七 Tab：**概览 / 任务 / 工件 / 审批 / 预算 / 验收 / 配置**。
- 默认落地「概览」（现左主列内容）。
- 治理类（工件 / 审批 / 预算 / 验收）从折叠区提升到顶层 Tab。
- 每个 Tab 内的行 / 卡都带可点击跳转。

### 2.3 全链路面包屑（待建）
在 `shell-page-header.tsx` 新增 `ShellPageBreadcrumb`，所有详情页使用：

```
项目 › {项目名} › 需求 › {需求ID} › 任务 › {任务} › 运行
```

每级可点回上级；最深层不可点。

---

## 3. 剩余断点清单（对账后，12 → 实际开放约 9）

### A. 项目详情信息孤岛（4 处，全开放）
- [ ] 无顶层 Tab → 引入 `V3Tabs` 七 Tab
- [ ] 当前需求卡无跳转 → `Link` `/workflows/$demandId`
- [ ] 当前执行任务行无跳转 → `Link` 运行记录 + 员工
- [ ] Member 面板 `ExternalLink` 未包裹 `Link` → 包 `/employees/$employeeId`

### B. 人类决策旅程断裂（初版 4，其中 `/approvals` 已解决 → 剩 3）
- [x] `/approvals` 占位 → ✅ 已落地 `ApprovalsCenterPage`
- [ ] `InboxActionDialog` 缺上下文（发起人 / 原因 / 风险 / 证据）
- [ ] `resolveInboxHref` 无锚点 → `?tab=审批&focus=`
- [ ] 审批后不回流 → 提交后 toast 回流

### C. 运行可观测处置分离（4 处，全开放）
- [ ] run-overview 零外链 → 员工 / 节点 `Link`
- [ ] 异常无处置入口 → 「去处理」按钮
- [ ] workflow 与项目无互链 → 顶部面包屑回链
- [ ] 员工详情死按钮 → 参与项目 / 产出工件 / `QueueRow` onClick

---

## 4. 五条核心旅程闭环（待闭合）

- **A. 项目内部跳转**（补 Link）：概览 → 需求(workflow) / 任务(运行+员工) / 运行总览。
- **B. 审批闭环**（inbox 上下文 + deep-link + 回流）：`/approvals` 已提供聚合视图，但 inbox → 项目 的上下文携带与回流仍缺。
- **C. 员工工作台**（参与项目 + 产出工件 + 修复死按钮）。
- **D. 运行可观测双向链接**（异常旁即处置）。
- **E. 空状态引导**（消灭死胡同）。

---

## 5. 落地路线（修订为两期，因导航 / 审批中心已落地）

### 第一期（高杠杆，低风险）：跳转 + 面包屑 + 项目中枢骨架
目标：让对象活起来，可观测入口直达处置。
1. 面包屑组件 `ShellPageBreadcrumb`（`shell-page-header.tsx`）+ 各详情页接入
2. 项目详情：Member `Link` + 当前需求 `Link` + 当前执行 `Link` + 顶部「在运行总览查看」
3. run-overview：选中员工 / 节点 `Link` + 异常「去处理」
4. workflow 详情：顶部项目面包屑 + 异常 banner `Link`
5. 员工详情：参与项目区 + 产出工件区 + `QueueRow` / 死按钮修复
6. 空状态 `V3EmptyState` + `action` 引导（首批关键页）

### 第二期（核心价值，中风险）：项目七 Tab 中枢 + 审批闭环
1. 项目详情 `V3Tabs` 七 Tab（治理提升）
2. 审批 Tab 聚合 + 项目内直接审批
3. `InboxActionDialog` 上下文增强
4. `resolveInboxHref` deep-link + `focus` 定位高亮
5. 审批后回流 toast

---

## 6. 开发者交接（精确文件清单）

| 文件 | 改动 | 期 |
| --- | --- | --- |
| `components/layout/shell-page-header.tsx` | 新增 `ShellPageBreadcrumb` | 1 |
| `features/projects/index.tsx` + 详情组件 | Member / 需求 / 执行 `Link`；七 Tab | 1+2 |
| `features/run-overview/index.tsx` | 员工 / 节点 `Link` + 去处理 | 1 |
| `features/workflows/index.tsx` | 面包屑回链 + banner `Link` | 1 |
| `features/employees/detail.tsx` | 参与项目 / 产出工件 / 死按钮 | 1 |
| `features/inbox/*` | dialog 上下文 + deep-link + 回流 | 2 |
| `components/superteam/` | 沉淀 `V3Breadcrumb`、`V3EmptyState(action)` | 1 |

**设计约束**（遵循 AGENTS.md / DESIGN.md）：
- 全部遵循 `DESIGN.md` v3 Soft-Flat 设计语言
- Tab 用 `V3Tabs` + `V3TabList` + `V3Tab`
- 跳转用 TanStack Router `Link` / `navigate`，不用原生 `<a>`
- 空状态用 `V3EmptyState`，带 `action`
- 面包屑为新增组件，沉淀到 `apps/web/src/components/superteam/`

**验证纪律**：每期收尾用 `$superteam-completion-check` skill；真实端到端验证走通完整旅程；Web 测试用 `corepack pnpm --filter ./apps/web run test`。

---

## 7. 建议下一步

从**第一期**起步：风险最低、收益最高、不依赖第二期 Tab 重构。建议先落地「面包屑 + 项目 / Member / run-overview 关键跳转 + 员工详情跨区」，即日可测。第二期（项目中枢 + 审批闭环）是核心价值，单独排期。

---

**下一步**：请评审本精炼方案。确认后建议立即启动第一期（补全跳转 + 面包屑）——它风险最低、收益最高，且能立即消除当前最刺眼的「对象孤岛」体验。
