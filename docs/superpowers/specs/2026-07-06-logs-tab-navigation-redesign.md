# 日志管理 Tab 导航重构设计

> 复核状态：CHANGELOG无明确记录；锚点抽查未发现logs/route.tsx父布局路由明确证据

**日期：** 2026-07-06  
**范围：** 前端仅  
**涉及路径：** `apps/web/src/routes/_authenticated/logs/`、`apps/web/src/components/layout/data/sidebar-data.ts`

---

## 背景与目标

当前"日志管理"在 Sidebar 以展开子菜单形式呈现（登录日志 / 操作日志 / 平台事件），用户每次需要展开菜单才能切换日志类型，体验割裂。

目标：改为与"权限中心"相同的页内 tab 切换模式，让用户在同一页面内快速切换日志类型，并通过快速筛选 chips 和行详情 Sheet 缩短"找到目标日志"的路径。

---

## 路由结构

采用 **TanStack Router 父级布局路由方案（方案 A）**。

### 新增文件

```
apps/web/src/routes/_authenticated/logs/route.tsx   ← 新增父布局
```

### 保留文件（内容改造）

```
apps/web/src/routes/_authenticated/logs/login/index.tsx
apps/web/src/routes/_authenticated/logs/operation/index.tsx
apps/web/src/routes/_authenticated/logs/runtime/index.tsx
apps/web/src/routes/_authenticated/logs/-shared.tsx
```

### 路由行为

- 访问 `/logs` → 重定向到 `/logs/login`（第一个 tab）
- Tab 切换 = TanStack Router `<Link>` 导航，URL 随之变化
- 每个日志类型保持独立 URL，可书签/分享

---

## Sidebar 变更

`sidebar-data.ts` 中"日志管理"从展开子菜单改为单条直链：

```ts
// 改前（展开菜单）
{
  title: "日志管理",
  icon: ScrollText,
  items: [
    { title: "登录日志", url: "/logs/login", ... },
    { title: "操作日志", url: "/logs/operation", ... },
    { title: "平台事件", url: "/logs/runtime", ... },
  ],
}

// 改后（单条直链）
{
  title: "日志管理",
  url: "/logs",
  icon: ScrollText,
  iconTone: "neutral",
}
```

---

## 父布局 `route.tsx`

负责：
1. 渲染统一的 `ShellPageHeader`（标题"日志管理"，副标题"登录审计、操作追溯与平台事件"）
2. 渲染 Tab 导航栏（与权限中心样式一致）
3. 渲染 `<Outlet>`（子路由内容区）

Tab 配置：

| value | label | 子路由 |
|---|---|---|
| `login` | 登录日志 | `/logs/login` |
| `operation` | 操作日志 | `/logs/operation` |
| `runtime` | 平台事件 | `/logs/runtime` |

active tab 通过 `useRouterState` 匹配当前路径确定，不使用 Tabs 组件内部 state（因为切换是路由跳转，不是组件内 state 变化）。

Tab 样式复用权限中心的 `TabsList` / `TabsTrigger` className，但 `onClick` 改为 `navigate`，渲染用 `<div>` 代替 `TabsContent`（内容由 `<Outlet>` 提供）。

---

## 子路由改造

每个子路由（login/operation/runtime）：

1. **移除** 各自的 `<ShellPageHeader>`（父布局统一提供）
2. **移除** 外层 `<Main>` 包裹（父布局的 `<Outlet>` 区域不需要重复套）
3. **保留** 现有过滤栏、表格、分页组件逻辑不变
4. **新增** 快速筛选 chips（见下文）
5. **新增** 行点击 → Sheet 详情（见下文）

---

## 快速筛选 Chips

位于过滤栏上方，横向排列，单选（含"全部"选项），点击即生效（不需要回车/blur）。选中状态与过滤栏的对应 filter 字段双向同步（chips 和 select 过滤器互为镜像，不重复维护状态）。

### 登录日志 chips

| chip | 对应 filter 值 |
|---|---|
| 全部 | `event_type=undefined` |
| 登录失败 | `event_type=login_failed` |
| 登录成功 | `event_type=login_succeeded` |
| 登出成功 | `event_type=logout_succeeded` |

chips 选中时同步清空 `result` filter（避免冲突），反之也成立。

### 操作日志 chips

| chip | 对应 filter 值 |
|---|---|
| 全部 | `module=undefined` |
| authz | `module=authz` |
| users | `module=users` |
| teams | `module=teams` |
| projects | `module=projects` |
| skills | `module=skills` |

模块值为固定枚举，与后端 `operation_logs.module` 字段一致。

### 平台事件 chips

| chip | 对应 filter 值 |
|---|---|
| 全部 | `severity=undefined` |
| 错误 | `severity=error` |
| 预警 | `severity=warning` |
| 成功 | `severity=success` |
| 信息 | `severity=info` |

---

## 行详情 Sheet

### 触发

点击表格任意一行 → 右侧 Sheet 展开。再次点击同行或点击 Sheet 关闭按钮 → Sheet 收起。

使用 shadcn `Sheet`（`side="right"`），宽度 `w-[420px]` 或 `max-w-[45vw]`，主表格不缩窄（Sheet 覆盖在上方）。

### 登录日志 Sheet 内容

| 字段 | 来源 |
|---|---|
| 事件类型 | `record.event_type`（中文 label） |
| 结果 | `record.result`（StatusPill） |
| 用户名 | `record.username` |
| 来源 IP | `record.client_ip` |
| 失败原因 | `record.failure_reason`（无则"—"） |
| 时间 | `record.created_at`（格式化） |

### 操作日志 Sheet 内容

| 字段 | 来源 |
|---|---|
| 模块.动作 | `record.module` + `record.action` |
| 结果 | `record.result`（StatusPill） |
| 操作者 | `record.username` |
| 资源 | `record.resource_type` + `record.resource_id` |
| 来源 IP | `record.client_ip` |
| 时间 | `record.created_at` |

### 平台事件 Sheet 内容

| 字段 | 来源 |
|---|---|
| 级别 | `event.severity`（StatusPill + 中文 label） |
| 事件类型 | `event.event_type` |
| 来源 | `event.source` |
| 节点 ID | `event.node_id` |
| 标题 | `event.title` |
| 描述 | `event.description` |
| Payload | `event.payload`（JSON 格式化显示，`pre` 块） |

---

## 不在本次范围内

- Stats KPI 卡片（需要新增 stats 接口，后续独立迭代）
- 日志导出功能
- 日志保留策略配置

---

## 验证标准

1. 访问 `/logs` 自动重定向到 `/logs/login`
2. Tab 切换后 URL 正确变化（`/logs/login`、`/logs/operation`、`/logs/runtime`）
3. Sidebar 点击"日志管理"可跳转，三个子菜单条目消失
4. 每个 tab 页面不出现重复 PageHeader
5. 快速筛选 chips 与过滤栏双向同步，不产生冲突筛选
6. 点击表格行弹出 Sheet，内容字段准确，关闭正常
7. 现有过滤、分页功能无回归
