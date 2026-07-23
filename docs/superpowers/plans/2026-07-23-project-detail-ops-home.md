# 2026-07-23 项目详情运营首屏落地 Implementation Plan

> **For agentic workers:** Execute task-by-task. Checkboxes track progress.

**Goal:** 将项目详情「概览」Tab 重构为运营指挥首屏（对齐 `docs/prototypes/project-detail-ops-home.html`），回答「这个项目怎么了」，并与收件箱 / 任务中枢 / 运行总览划清边界。

**Architecture:** Web-only 首版。复用既有 `ProjectOverview`、`ProjectTask`、`ProjectEvent`、`ProjectDecisionRequest`、`digital_employee_pool`、Runtime placement panel；不新增聚合 API / 迁移。新布局抽成独立组件挂入 `ProjectOperationalDetail` overview，避免继续膨胀单文件。计划图 / 需求长卷 / 计划确认下沉到任务或审批相关入口（概览仅导流或折叠）。

**Tech Stack:** React 19、TanStack Router/Query、v3 Soft-Flat、Vitest browser tests。

**原型:** `docs/prototypes/project-detail-ops-home.html`

---

## 产品边界（已拍板）

| 模块 | 一句话 |
|---|---|
| 项目详情首屏 | 这个项目怎么了（运行态指挥台） |
| 收件箱 | 我该处理什么（人视角队列） |
| 任务中枢 | 向项目投递需求（Plan/Loop/对话） |
| 运行总览 | 团队/楼层运行态观察；**项目详情不链过去**（无稳定项目锚点） |

### 首屏结构

```
头卡（目标/状态/负责人/阻塞·执行事实行）
  主 CTA：提交需求 → /task-launches?project=&mode=plan
  次 CTA：配置；去掉「在运行总览查看」
左主：运行脉搏（周，时间+Plan/Loop/对话）→ 执行中与阻塞（≤3）→ 员工∥Runtime
右辅：本项目阻塞 → 事件流（执行+审批）→ 项目脉搏摘要
空态：min-height + 占位，栅格不塌宽
跳出：查看全部任务→tasks Tab；全部待办→inbox；审计→/audit；配置类→config
```

### 数据映射（无新 API）

| UI | 数据源 |
|---|---|
| 运行脉搏 | `tasks` / `overview.active_tasks` + demand `coordination_mode`（任务级若无 mode，从归属 demand 或事件推断；chat 对话若不进任务流则脉搏可不画或仅事件） |
| 执行中与阻塞 | active / failed / blocked 任务切片 |
| 数字员工 | `overview.digital_employee_pool` + 当前任务交叉 |
| Runtime | 现有 `runtimePlacementPanel` 摘要化展示（就绪折叠） |
| 本项目阻塞 | `decisionRequests` pending（含待他人） |
| 事件流 | `events` / `recent_events` 过滤执行类 + decision/approval 类 |
| 项目脉搏 | budget summary / risk hooks / plan revision / artifacts count（已有则挂，无则 —） |

---

## 文件结构

新建：

- `apps/web/src/features/projects/components/project-ops-home.tsx` — 运营首屏布局与空态
- `apps/web/src/features/projects/components/project-ops-home.test.tsx` — 有数据/空态/跳出
- `apps/web/src/features/projects/lib/project-ops-home.ts` — 脉搏周聚合、事件过滤、任务切片 helpers

修改：

- `apps/web/src/features/projects/components/project-operational-detail.tsx` — overview 换挂 `ProjectOpsHome`；头卡 CTA；去掉运行总览链
- `apps/web/src/features/projects/index.tsx` — `onSubmitDemand` 改为 navigate 任务中枢（或保留 dialog 由 props 切换）；深链参数
- `apps/web/src/features/task-launches/index.tsx` + route search — 接受 `?project=` `?mode=plan|loop|chat`
- `apps/web/src/features/projects/components/project-runtime-placement-panel.tsx` — 确保摘要态适合首屏（若已满足则少改）
- `apps/web/src/features/projects/components/project-operational-detail.test.tsx` — 对齐新文案/结构
- `CHANGELOG.md` — 完成后记一条

不改：Control Plane 契约、迁移、运行总览项目透镜逻辑。

---

## 任务列表

### Task 1: Plan 入库 + helpers

- [x] 写入本 plan
- [x] 实现 `project-ops-home.ts`

### Task 2: 任务中枢深链

- [x] route/search validate：`project?`，`mode?`
- [x] `TaskLaunchView` 初始化选中项目与 mode
- [x] 项目详情头卡 CTA → `/task-launches?project=&mode=plan`
- [x] 去掉头卡「在运行总览查看」

### Task 3: `ProjectOpsHome` UI

- [x] 左：脉搏 / 执行阻塞 / 员工∥Runtime
- [x] 右：阻塞 / 事件流 / 项目脉搏
- [x] 空态 min-height；「查看全部」切 tab / Link

### Task 4: 挂入 overview

- [x] overview 换挂 `ProjectOpsHome`
- [x] 计划确认移入审批 Tab；计划图进高级
- [x] 更新 operational-detail 测试

### Task 5: 验证

- [x] 定向 Vitest PASS（19）
- [x] 浏览器视觉核对（进行中）
- [x] CHANGELOG
---

## 非目标（本切片）

- 真实「周负荷」百分比条
- 运行总览 `?team=` 深链
- 新后端脉搏聚合 API
- 把审批完整表单塞进右栏（轻量处理可保留现有 resolve；复杂仍审批 Tab）

---

## 验收标准

1. `/projects/$id` 概览视觉对齐原型信息架构（非像素逐拷）
2. 提交需求进入任务中枢且 mode=plan、项目预选
3. 无「在运行总览查看」主出口
4. 空列表卡片不塌宽
5. 事件流可见执行类与 decision 类事件（有数据时）
6. 脉搏条目展示时间与模式（有数据时）
7. 「查看全部任务」切到任务 Tab
