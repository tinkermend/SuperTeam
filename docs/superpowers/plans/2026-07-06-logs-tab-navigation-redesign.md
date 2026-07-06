# 日志管理 Tab 导航重构 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 将日志管理从 Sidebar 展开子菜单改为页内 Tab 导航，新增快速筛选 chips 和行详情 Sheet，提升日志查找效率。

**Architecture:** 新增 TanStack Router 父级布局路由 `logs/route.tsx`，提供统一的 ShellPageHeader 和 Tab 导航栏（使用 `useNavigate` 驱动切换）；三个子路由保留各自 URL，去掉重复的 ShellPageHeader，新增快速筛选 chips 和行详情 Sheet。Sidebar 把"日志管理"展开菜单替换为单条直链 `/logs`。

**Tech Stack:** TanStack Router (file-based routing, `useRouterState`, `useNavigate`), React Query, shadcn `Tabs` / `Sheet`, Vitest + vitest-browser-react

## Global Constraints

- Web 测试命令：`corepack pnpm --filter ./apps/web run test`，禁止使用 `npx vitest run`
- 内部页面跳转必须使用 TanStack Router 的 `Link` 或 `navigate`，不用 `<a href>`
- 样式遵循 v3 设计系统 token（`v3-ink`、`v3-card`、`v3-brand-soft` 等）
- 不引入新的依赖包

---

## File Map

| 操作 | 路径 | 职责 |
|------|------|------|
| 新建 | `apps/web/src/routes/_authenticated/logs/route.tsx` | 父布局：ShellPageHeader + Tab 导航 + Outlet |
| 新建 | `apps/web/src/routes/_authenticated/logs/index.tsx` | `/logs` 重定向到 `/logs/login` |
| 修改 | `apps/web/src/routes/_authenticated/logs/-shared.tsx` | 新增 `LogChips` 共用组件 |
| 修改 | `apps/web/src/routes/_authenticated/logs/login/index.tsx` | 移除 Header/Main，加 chips + Sheet |
| 修改 | `apps/web/src/routes/_authenticated/logs/operation/index.tsx` | 移除 Header/Main，加 chips + Sheet |
| 修改 | `apps/web/src/routes/_authenticated/logs/runtime/index.tsx` | 移除 Header/Main，加 chips + Sheet |
| 修改 | `apps/web/src/components/layout/data/sidebar-data.ts` | 展开菜单 → 单条直链 |
| 修改 | `apps/web/src/routes/_authenticated/logs/operation/-index.test.tsx` | 更新断言：移除 heading 检查，加 chips 检查 |

---

## Task 1: Sidebar 简化 + `/logs` 重定向

**Files:**
- Modify: `apps/web/src/components/layout/data/sidebar-data.ts`
- Create: `apps/web/src/routes/_authenticated/logs/index.tsx`

**Interfaces:**
- Produces: `/logs` URL 可访问并自动跳转 `/logs/login`

- [ ] **Step 1: 修改 sidebar-data.ts，将展开菜单改为单条直链**

打开 `apps/web/src/components/layout/data/sidebar-data.ts`，找到"日志管理"条目（约第 159 行），将整个 `items` 展开结构替换为单条直链：

```ts
{
  title: "日志管理",
  url: "/logs",
  icon: ScrollText,
  iconTone: "neutral",
},
```

同时移除顶部不再使用的 import：`ClipboardList`、`LogIn`、`ServerCog`（如果其他地方没用到）。

- [ ] **Step 2: 新建重定向路由 `logs/index.tsx`**

创建 `apps/web/src/routes/_authenticated/logs/index.tsx`：

```tsx
import { createFileRoute, redirect } from "@tanstack/react-router";

export const Route = createFileRoute("/_authenticated/logs/")({
  beforeLoad: () => {
    throw redirect({ to: "/logs/login" });
  },
});
```

- [ ] **Step 3: 验证重定向**

启动 dev server，浏览器访问 `http://localhost:5173/logs`，应自动跳转到 `/logs/login`。

- [ ] **Step 4: Commit**

```bash
git add apps/web/src/components/layout/data/sidebar-data.ts \
        apps/web/src/routes/_authenticated/logs/index.tsx
git commit -m "feat(web/logs): simplify sidebar to single link, add /logs redirect"
```

---

## Task 2: 父布局路由 `logs/route.tsx`

**Files:**
- Create: `apps/web/src/routes/_authenticated/logs/route.tsx`

**Interfaces:**
- Consumes: `ShellPageHeader`, `Main` from `@/components/layout`, `Tabs`/`TabsList`/`TabsTrigger` from `@/components/ui/tabs`, `Outlet` from `@tanstack/react-router`
- Produces: 所有 `/logs/*` 子路由共享的 PageHeader + Tab 导航条

- [ ] **Step 1: 创建父布局路由**

创建 `apps/web/src/routes/_authenticated/logs/route.tsx`：

```tsx
import { createFileRoute, Outlet, useNavigate, useRouterState } from "@tanstack/react-router";
import { ScrollText } from "lucide-react";
import { Main } from "@/components/layout/main";
import { ShellPageHeader } from "@/components/layout/shell-page-header";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";

export const Route = createFileRoute("/_authenticated/logs")({
  component: LogsLayout,
});

const tabItems = [
  { label: "登录日志", to: "/logs/login", value: "login" },
  { label: "操作日志", to: "/logs/operation", value: "operation" },
  { label: "平台事件", to: "/logs/runtime", value: "runtime" },
] as const;

function LogsLayout() {
  const navigate = useNavigate();
  const pathname = useRouterState({ select: (s) => s.location.pathname });
  const activeTab = tabItems.find((t) => pathname.startsWith(t.to))?.value ?? "login";

  return (
    <>
      <ShellPageHeader
        icon={<ScrollText />}
        iconTone="mute"
        title="日志管理"
        subtitle="登录审计、操作追溯与平台事件"
      />
      <Main className="min-w-0 overflow-x-hidden">
        <div className="mx-auto flex w-full max-w-6xl flex-col gap-4 text-v3-ink">
          <Tabs
            value={activeTab}
            onValueChange={(v) => {
              const tab = tabItems.find((t) => t.value === v);
              if (tab) navigate({ to: tab.to });
            }}
            className="gap-4"
          >
            <TabsList className="h-auto max-w-full flex-wrap justify-start gap-1 overflow-x-auto rounded-[14px] bg-v3-card p-1.5 text-v3-ink-2 shadow-v3">
              {tabItems.map((tab) => (
                <TabsTrigger
                  key={tab.value}
                  value={tab.value}
                  className="h-9 flex-none rounded-[10px] border-0 px-4 py-2 text-[13px] font-semibold text-v3-ink-2 shadow-none transition-colors hover:bg-v3-card-soft hover:text-v3-ink focus-visible:ring-v3-brand/60 focus-visible:ring-offset-v3-bg data-[state=active]:bg-v3-brand-soft data-[state=active]:text-v3-brand-deep data-[state=active]:shadow-none"
                >
                  {tab.label}
                </TabsTrigger>
              ))}
            </TabsList>
          </Tabs>
          <Outlet />
        </div>
      </Main>
    </>
  );
}
```

- [ ] **Step 2: 浏览器验证 Tab 导航**

访问 `/logs/login`，确认：
- 页面顶部出现"日志管理"标题
- 三个 tab 显示，"登录日志"处于 active 状态
- 点击"操作日志"tab，URL 变为 `/logs/operation`，active 状态正确跟随

- [ ] **Step 3: Commit**

```bash
git add apps/web/src/routes/_authenticated/logs/route.tsx
git commit -m "feat(web/logs): add parent layout route with tab navigation"
```

---

## Task 3: 共用 `LogChips` 组件

**Files:**
- Modify: `apps/web/src/routes/_authenticated/logs/-shared.tsx`

**Interfaces:**
- Produces: `LogChips` — `({ options, value, onValueChange })` → `JSX.Element`
  - `options: Array<{ label: string; value: string }>`
  - `value: string | undefined` (undefined = 全部)
  - `onValueChange: (value: string | undefined) => void`

- [ ] **Step 1: 在 `-shared.tsx` 末尾追加 `LogChips` 组件**

```tsx
export function LogChips({
  onValueChange,
  options,
  value,
}: {
  onValueChange: (value: string | undefined) => void;
  options: Array<{ label: string; value: string }>;
  value: string | undefined;
}) {
  const all = { label: "全部", value: "__all__" };
  const current = value ?? "__all__";

  return (
    <div className="flex flex-wrap gap-1.5 px-5 pt-4">
      {[all, ...options].map((opt) => (
        <button
          key={opt.value}
          type="button"
          onClick={() => onValueChange(opt.value === "__all__" ? undefined : opt.value)}
          className={[
            "rounded-full border px-3 py-1 text-xs font-medium transition-colors",
            current === opt.value
              ? "border-v3-brand bg-v3-brand-soft text-v3-brand-deep"
              : "border-v3-line bg-v3-card text-v3-ink-2 hover:border-v3-line-strong hover:text-v3-ink",
          ].join(" ")}
        >
          {opt.label}
        </button>
      ))}
    </div>
  );
}
```

- [ ] **Step 2: Commit**

```bash
git add apps/web/src/routes/_authenticated/logs/-shared.tsx
git commit -m "feat(web/logs): add shared LogChips component"
```

---

## Task 4: 重构登录日志子路由

**Files:**
- Modify: `apps/web/src/routes/_authenticated/logs/login/index.tsx`

**Interfaces:**
- Consumes: `LogChips`, `LogFilterBar`, `LogSelectFilter`, `LogPagination`, `formatLogDateTime` from `../-shared`
- Consumes: `Sheet`, `SheetContent`, `SheetHeader`, `SheetTitle` from `@/components/ui/sheet`
- Consumes: `listLoginLogs`, `LoginLogRecord`, `LoginLogEventType`, `LoginLogResult` from `@/lib/api/auth`

- [ ] **Step 1: 移除 ShellPageHeader 和 Main 包裹，保留核心逻辑**

用以下内容完整替换 `login/index.tsx`：

```tsx
import { keepPreviousData, useQuery } from "@tanstack/react-query";
import { createFileRoute } from "@tanstack/react-router";
import { useState } from "react";
import {
  StatusPill,
  V3EmptyState,
  V3ErrorState,
  V3LoadingState,
  V3Table,
  V3Td,
  V3Th,
  V3Tr,
  WorkSurface,
} from "@/components/superteam";
import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet";
import {
  listLoginLogs,
  type LoginLogEventType,
  type LoginLogRecord,
  type LoginLogResult,
} from "@/lib/api/auth";
import { resolveControlPlaneUrl } from "@/lib/config/control-plane-url";
import {
  LOG_PAGE_SIZE,
  LogChips,
  LogFilterBar,
  LogPagination,
  LogSelectFilter,
  formatLogDateTime,
} from "../-shared";

export const Route = createFileRoute("/_authenticated/logs/login/")({
  component: LoginLogsRoute,
});

const eventTypeLabel: Record<LoginLogEventType, string> = {
  login_succeeded: "登录成功",
  login_failed: "登录失败",
  logout_succeeded: "登出成功",
};

const chipOptions = [
  { label: "登录失败", value: "login_failed" },
  { label: "登录成功", value: "login_succeeded" },
  { label: "登出成功", value: "logout_succeeded" },
];

const resultOptions = [
  { label: "成功", value: "succeeded" },
  { label: "失败", value: "failed" },
];

type LoginLogFilters = {
  event_type?: LoginLogEventType;
  result?: LoginLogResult;
};

function LoginLogsRoute() {
  const apiBaseUrl = resolveControlPlaneUrl();
  const [filters, setFilters] = useState<LoginLogFilters>({});
  const [offset, setOffset] = useState(0);
  const [selectedRecord, setSelectedRecord] = useState<LoginLogRecord | null>(null);

  const logsQuery = useQuery({
    queryKey: ["web-login-logs", filters, offset],
    queryFn: () =>
      listLoginLogs({ baseUrl: apiBaseUrl, limit: LOG_PAGE_SIZE, offset, ...filters }),
    placeholderData: keepPreviousData,
  });

  const updateFilter = <K extends keyof LoginLogFilters>(key: K, value: LoginLogFilters[K]) => {
    setOffset(0);
    setFilters((prev) => ({ ...prev, [key]: value }));
  };

  const records = logsQuery.data?.items ?? [];
  const hasFilter = Boolean(filters.event_type || filters.result);

  return (
    <>
      <WorkSurface>
        <LogChips
          options={chipOptions}
          value={filters.event_type}
          onValueChange={(v) => updateFilter("event_type", v as LoginLogEventType | undefined)}
        />
        <LogFilterBar>
          <LogSelectFilter
            id="login-log-event-type"
            label="事件类型"
            options={chipOptions}
            value={filters.event_type}
            onValueChange={(v) => updateFilter("event_type", v as LoginLogEventType | undefined)}
          />
          <LogSelectFilter
            id="login-log-result"
            label="结果"
            options={resultOptions}
            value={filters.result}
            onValueChange={(v) => updateFilter("result", v as LoginLogResult | undefined)}
          />
        </LogFilterBar>

        {logsQuery.isLoading && !logsQuery.data ? (
          <V3LoadingState label="正在加载登录日志…" />
        ) : logsQuery.isError ? (
          <V3ErrorState title="登录日志加载失败" description="请稍后重试，或确认当前账号仍有访问权限。" />
        ) : records.length === 0 ? (
          <V3EmptyState
            title={hasFilter ? "筛选后无登录日志" : "暂无登录日志"}
            description="账号登录后会显示在这里。"
          />
        ) : (
          <V3Table>
            <thead>
              <V3Tr>
                <V3Th className="min-w-[150px]">时间</V3Th>
                <V3Th>事件类型</V3Th>
                <V3Th>结果</V3Th>
                <V3Th>用户</V3Th>
                <V3Th>来源 IP</V3Th>
                <V3Th className="min-w-[180px]">失败原因</V3Th>
              </V3Tr>
            </thead>
            <tbody>
              {records.map((record: LoginLogRecord) => (
                <V3Tr
                  key={record.id}
                  className="cursor-pointer"
                  onClick={() => setSelectedRecord(record)}
                >
                  <V3Td className="whitespace-nowrap text-xs text-v3-ink-2 tabular-nums">
                    {formatLogDateTime(record.created_at)}
                  </V3Td>
                  <V3Td className="whitespace-nowrap text-sm">
                    {eventTypeLabel[record.event_type] ?? record.event_type}
                  </V3Td>
                  <V3Td>
                    <StatusPill tone={record.result === "succeeded" ? "ok" : "danger"}>
                      {record.result === "succeeded" ? "成功" : "失败"}
                    </StatusPill>
                  </V3Td>
                  <V3Td className="max-w-[200px] truncate text-sm">{record.username}</V3Td>
                  <V3Td className="whitespace-nowrap font-mono text-xs">{record.client_ip || "-"}</V3Td>
                  <V3Td className="max-w-[240px] truncate text-xs text-v3-ink-3">
                    {record.failure_reason || "-"}
                  </V3Td>
                </V3Tr>
              ))}
            </tbody>
          </V3Table>
        )}

        <LogPagination
          isFetching={logsQuery.isFetching}
          itemCount={records.length}
          offset={offset}
          onOffsetChange={setOffset}
          pageSize={LOG_PAGE_SIZE}
        />
      </WorkSurface>

      <Sheet open={selectedRecord !== null} onOpenChange={(open) => { if (!open) setSelectedRecord(null); }}>
        <SheetContent side="right" className="w-[420px] max-w-[45vw]">
          {selectedRecord && (
            <>
              <SheetHeader>
                <SheetTitle className="text-base font-semibold text-v3-ink">
                  {eventTypeLabel[selectedRecord.event_type] ?? selectedRecord.event_type}
                </SheetTitle>
              </SheetHeader>
              <div className="mt-4 flex flex-col gap-3 text-sm">
                <DetailRow label="结果">
                  <StatusPill tone={selectedRecord.result === "succeeded" ? "ok" : "danger"}>
                    {selectedRecord.result === "succeeded" ? "成功" : "失败"}
                  </StatusPill>
                </DetailRow>
                <DetailRow label="用户名">{selectedRecord.username || "—"}</DetailRow>
                <DetailRow label="来源 IP">{selectedRecord.client_ip || "—"}</DetailRow>
                <DetailRow label="失败原因">{selectedRecord.failure_reason || "—"}</DetailRow>
                <DetailRow label="时间">{formatLogDateTime(selectedRecord.created_at)}</DetailRow>
              </div>
            </>
          )}
        </SheetContent>
      </Sheet>
    </>
  );
}

function DetailRow({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="flex items-start gap-3">
      <span className="w-20 shrink-0 text-xs text-v3-ink-2">{label}</span>
      <span className="min-w-0 break-all text-v3-ink">{children}</span>
    </div>
  );
}
```

- [ ] **Step 2: 运行测试确认无回归**

```bash
corepack pnpm --filter ./apps/web run test -- --reporter=verbose 2>&1 | grep -E "login|PASS|FAIL"
```

（登录日志无专属测试文件，只需确认其他测试未破坏。）

- [ ] **Step 3: 浏览器验证**

访问 `/logs/login`：chips 显示在过滤栏上方，点击"登录失败"chip 触发筛选；点击表格行弹出 Sheet。

- [ ] **Step 4: Commit**

```bash
git add apps/web/src/routes/_authenticated/logs/login/index.tsx
git commit -m "feat(web/logs): refactor login log route with chips and detail sheet"
```

---

## Task 5: 重构操作日志子路由 + 更新测试

**Files:**
- Modify: `apps/web/src/routes/_authenticated/logs/operation/index.tsx`
- Modify: `apps/web/src/routes/_authenticated/logs/operation/-index.test.tsx`

**Interfaces:**
- Consumes: `LogChips`, `LogFilterBar`, `LogSelectFilter`, `LogTextFilter`, `LogPagination`, `formatLogDateTime` from `../-shared`
- Consumes: `Sheet`, `SheetContent`, `SheetHeader`, `SheetTitle` from `@/components/ui/sheet`
- Consumes: `listOperationLogs`, `OperationLogRecord`, `OperationLogResult` from `@/lib/api/auth`

- [ ] **Step 1: 替换 operation/index.tsx**

```tsx
import { keepPreviousData, useQuery } from "@tanstack/react-query";
import { createFileRoute } from "@tanstack/react-router";
import { useState } from "react";
import {
  StatusPill,
  V3EmptyState,
  V3ErrorState,
  V3LoadingState,
  V3Table,
  V3Td,
  V3Th,
  V3Tr,
  WorkSurface,
} from "@/components/superteam";
import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet";
import {
  listOperationLogs,
  type OperationLogRecord,
  type OperationLogResult,
} from "@/lib/api/auth";
import { resolveControlPlaneUrl } from "@/lib/config/control-plane-url";
import {
  LOG_PAGE_SIZE,
  LogChips,
  LogFilterBar,
  LogPagination,
  LogSelectFilter,
  LogTextFilter,
  formatLogDateTime,
} from "../-shared";

export const Route = createFileRoute("/_authenticated/logs/operation/")({
  component: OperationLogsRoute,
});

const chipOptions = [
  { label: "authz", value: "authz" },
  { label: "users", value: "users" },
  { label: "teams", value: "teams" },
  { label: "projects", value: "projects" },
  { label: "skills", value: "skills" },
];

const resultOptions = [
  { label: "成功", value: "succeeded" },
  { label: "失败", value: "failed" },
];

type OperationLogFilters = {
  module?: string;
  action?: string;
  result?: OperationLogResult;
};

function OperationLogsRoute() {
  const apiBaseUrl = resolveControlPlaneUrl();
  const [filters, setFilters] = useState<OperationLogFilters>({});
  const [offset, setOffset] = useState(0);
  const [selectedRecord, setSelectedRecord] = useState<OperationLogRecord | null>(null);

  const logsQuery = useQuery({
    queryKey: ["web-operation-logs", filters, offset],
    queryFn: () =>
      listOperationLogs({ baseUrl: apiBaseUrl, limit: LOG_PAGE_SIZE, offset, ...filters }),
    placeholderData: keepPreviousData,
  });

  const updateFilter = <K extends keyof OperationLogFilters>(key: K, value: OperationLogFilters[K]) => {
    setOffset(0);
    setFilters((prev) => ({ ...prev, [key]: value }));
  };

  const records = logsQuery.data?.items ?? [];
  const hasFilter = Boolean(filters.module || filters.action || filters.result);

  return (
    <>
      <WorkSurface>
        <LogChips
          options={chipOptions}
          value={filters.module}
          onValueChange={(v) => updateFilter("module", v)}
        />
        <LogFilterBar>
          <LogTextFilter
            id="operation-log-module"
            label="模块"
            placeholder="如 authz、users"
            value={filters.module}
            onCommit={(v) => updateFilter("module", v)}
          />
          <LogTextFilter
            id="operation-log-action"
            label="动作"
            placeholder="如 user.create"
            value={filters.action}
            onCommit={(v) => updateFilter("action", v)}
          />
          <LogSelectFilter
            id="operation-log-result"
            label="结果"
            options={resultOptions}
            value={filters.result}
            onValueChange={(v) => updateFilter("result", v as OperationLogResult | undefined)}
          />
        </LogFilterBar>

        {logsQuery.isLoading && !logsQuery.data ? (
          <V3LoadingState label="正在加载操作日志…" />
        ) : logsQuery.isError ? (
          <V3ErrorState title="操作日志加载失败" description="请稍后重试，或确认当前账号仍有访问权限。" />
        ) : records.length === 0 ? (
          <V3EmptyState
            title={hasFilter ? "筛选后无操作日志" : "暂无操作日志"}
            description="控制台管理操作产生后会显示在这里。"
          />
        ) : (
          <V3Table>
            <thead>
              <V3Tr>
                <V3Th className="min-w-[150px]">时间</V3Th>
                <V3Th>模块</V3Th>
                <V3Th>动作</V3Th>
                <V3Th>结果</V3Th>
                <V3Th>用户</V3Th>
                <V3Th className="min-w-[180px]">资源</V3Th>
                <V3Th>来源 IP</V3Th>
              </V3Tr>
            </thead>
            <tbody>
              {records.map((record: OperationLogRecord) => (
                <V3Tr
                  key={record.id}
                  className="cursor-pointer"
                  onClick={() => setSelectedRecord(record)}
                >
                  <V3Td className="whitespace-nowrap text-xs text-v3-ink-2 tabular-nums">
                    {formatLogDateTime(record.created_at)}
                  </V3Td>
                  <V3Td className="whitespace-nowrap text-sm">{record.module}</V3Td>
                  <V3Td><StatusPill tone="mute">{record.action}</StatusPill></V3Td>
                  <V3Td>
                    <StatusPill tone={record.result === "succeeded" ? "ok" : "danger"}>
                      {record.result === "succeeded" ? "成功" : "失败"}
                    </StatusPill>
                  </V3Td>
                  <V3Td className="max-w-[160px] truncate text-sm">{record.username || "-"}</V3Td>
                  <V3Td className="max-w-[220px] truncate font-mono text-xs text-v3-ink-3">
                    {record.resource_type ? `${record.resource_type}:${record.resource_id || "-"}` : "-"}
                  </V3Td>
                  <V3Td className="whitespace-nowrap font-mono text-xs">{record.client_ip || "-"}</V3Td>
                </V3Tr>
              ))}
            </tbody>
          </V3Table>
        )}

        <LogPagination
          isFetching={logsQuery.isFetching}
          itemCount={records.length}
          offset={offset}
          onOffsetChange={setOffset}
          pageSize={LOG_PAGE_SIZE}
        />
      </WorkSurface>

      <Sheet open={selectedRecord !== null} onOpenChange={(open) => { if (!open) setSelectedRecord(null); }}>
        <SheetContent side="right" className="w-[420px] max-w-[45vw]">
          {selectedRecord && (
            <>
              <SheetHeader>
                <SheetTitle className="text-base font-semibold text-v3-ink">
                  {selectedRecord.module} · {selectedRecord.action}
                </SheetTitle>
              </SheetHeader>
              <div className="mt-4 flex flex-col gap-3 text-sm">
                <DetailRow label="结果">
                  <StatusPill tone={selectedRecord.result === "succeeded" ? "ok" : "danger"}>
                    {selectedRecord.result === "succeeded" ? "成功" : "失败"}
                  </StatusPill>
                </DetailRow>
                <DetailRow label="操作者">{selectedRecord.username || "—"}</DetailRow>
                <DetailRow label="资源">
                  {selectedRecord.resource_type
                    ? `${selectedRecord.resource_type}:${selectedRecord.resource_id || "-"}`
                    : "—"}
                </DetailRow>
                <DetailRow label="来源 IP">{selectedRecord.client_ip || "—"}</DetailRow>
                <DetailRow label="时间">{formatLogDateTime(selectedRecord.created_at)}</DetailRow>
              </div>
            </>
          )}
        </SheetContent>
      </Sheet>
    </>
  );
}

function DetailRow({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="flex items-start gap-3">
      <span className="w-20 shrink-0 text-xs text-v3-ink-2">{label}</span>
      <span className="min-w-0 break-all text-v3-ink">{children}</span>
    </div>
  );
}
```

- [ ] **Step 2: 更新 operation/-index.test.tsx**

现有测试断言 `getByRole("heading", { name: "操作日志" })` 会失败（heading 已移至父布局）。更新为：

```tsx
describe("OperationLogsRoute", () => {
  it("renders operation logs from the control plane with v3 surfaces", async () => {
    const screen = await renderRoute();

    await expect.element(screen.getByText("user.create")).toBeVisible();
    await expect.element(screen.getByText("users")).toBeVisible();

    expect(document.body.querySelector('[data-slot="v3-work-surface"]')).not.toBeNull();
    expect(document.body.querySelector('[data-slot="v3-table"]')).not.toBeNull();
  });

  it("renders module quick-filter chips", async () => {
    const screen = await renderRoute();

    await expect.element(screen.getByRole("button", { name: "全部" })).toBeVisible();
    await expect.element(screen.getByRole("button", { name: "users" })).toBeVisible();
    await expect.element(screen.getByRole("button", { name: "authz" })).toBeVisible();
  });
});
```

- [ ] **Step 3: 运行测试**

```bash
corepack pnpm --filter ./apps/web run test -- --reporter=verbose 2>&1 | grep -E "operation|PASS|FAIL"
```

期望：所有 operation log 测试通过。

- [ ] **Step 4: 浏览器验证**

访问 `/logs/operation`：chips 显示，点击"authz"筛选，点击行弹出 Sheet。

- [ ] **Step 5: Commit**

```bash
git add apps/web/src/routes/_authenticated/logs/operation/index.tsx \
        apps/web/src/routes/_authenticated/logs/operation/-index.test.tsx
git commit -m "feat(web/logs): refactor operation log route with chips and detail sheet"
```

---

## Task 6: 重构平台事件子路由

**Files:**
- Modify: `apps/web/src/routes/_authenticated/logs/runtime/index.tsx`

**Interfaces:**
- Consumes: `LogChips`, `LogFilterBar`, `LogSelectFilter`, `LogTextFilter`, `LogPagination`, `formatLogDateTime` from `../-shared`
- Consumes: `Sheet`, `SheetContent`, `SheetHeader`, `SheetTitle` from `@/components/ui/sheet`
- Consumes: `listRuntimeEvents`, `RuntimeEvent`, `RuntimeEventSeverity` from `@/lib/api/runtime`

- [ ] **Step 1: 替换 runtime/index.tsx 完整内容**

```tsx
import { keepPreviousData, useQuery } from "@tanstack/react-query";
import { createFileRoute } from "@tanstack/react-router";
import { useState } from "react";
import {
  StatusPill,
  V3EmptyState,
  V3ErrorState,
  V3LoadingState,
  V3Table,
  V3Td,
  V3Th,
  V3Tr,
  type V3Tone,
  WorkSurface,
} from "@/components/superteam";
import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet";
import {
  listRuntimeEvents,
  type RuntimeEvent,
  type RuntimeEventSeverity,
} from "@/lib/api/runtime";
import { resolveControlPlaneUrl } from "@/lib/config/control-plane-url";
import {
  LOG_PAGE_SIZE,
  LogChips,
  LogFilterBar,
  LogPagination,
  LogSelectFilter,
  LogTextFilter,
  formatLogDateTime,
} from "../-shared";

export const Route = createFileRoute("/_authenticated/logs/runtime/")({
  component: RuntimeEventLogsRoute,
});

const severityLabel: Record<RuntimeEventSeverity, string> = {
  error: "错误",
  info: "信息",
  success: "成功",
  warning: "预警",
};

const severityTone: Record<RuntimeEventSeverity, V3Tone> = {
  error: "danger",
  info: "info",
  success: "ok",
  warning: "warn",
};

const chipOptions = [
  { label: "错误", value: "error" },
  { label: "预警", value: "warning" },
  { label: "成功", value: "success" },
  { label: "信息", value: "info" },
];

const severityOptions = chipOptions;

type RuntimeEventFilters = {
  severity?: RuntimeEventSeverity;
  event_type?: string;
  node_id?: string;
};

function RuntimeEventLogsRoute() {
  const apiBaseUrl = resolveControlPlaneUrl();
  const [filters, setFilters] = useState<RuntimeEventFilters>({});
  const [offset, setOffset] = useState(0);
  const [selectedEvent, setSelectedEvent] = useState<RuntimeEvent | null>(null);

  const eventsQuery = useQuery({
    queryKey: ["web-runtime-event-logs", filters, offset],
    queryFn: () =>
      listRuntimeEvents({ baseUrl: apiBaseUrl, limit: LOG_PAGE_SIZE, offset, ...filters }),
    placeholderData: keepPreviousData,
  });

  const updateFilter = <K extends keyof RuntimeEventFilters>(key: K, value: RuntimeEventFilters[K]) => {
    setOffset(0);
    setFilters((prev) => ({ ...prev, [key]: value }));
  };

  const events = eventsQuery.data?.items ?? [];
  const hasFilter = Boolean(filters.severity || filters.event_type || filters.node_id);

  return (
    <>
      <WorkSurface>
        <LogChips
          options={chipOptions}
          value={filters.severity}
          onValueChange={(v) => updateFilter("severity", v as RuntimeEventSeverity | undefined)}
        />
        <LogFilterBar>
          <LogSelectFilter
            id="runtime-log-severity"
            label="级别"
            options={severityOptions}
            value={filters.severity}
            onValueChange={(v) => updateFilter("severity", v as RuntimeEventSeverity | undefined)}
          />
          <LogTextFilter
            id="runtime-log-event-type"
            label="事件类型"
            placeholder="如 node_offline"
            value={filters.event_type}
            onCommit={(v) => updateFilter("event_type", v)}
          />
          <LogTextFilter
            id="runtime-log-node"
            label="节点"
            placeholder="node_id"
            value={filters.node_id}
            onCommit={(v) => updateFilter("node_id", v)}
          />
        </LogFilterBar>

        {eventsQuery.isLoading && !eventsQuery.data ? (
          <V3LoadingState label="正在加载平台事件…" />
        ) : eventsQuery.isError ? (
          <V3ErrorState title="平台事件加载失败" description="请稍后重试，或确认当前账号仍有访问权限。" />
        ) : events.length === 0 ? (
          <V3EmptyState
            title={hasFilter ? "筛选后无平台事件" : "暂无平台事件"}
            description="平台或 Runtime 事件产生后会显示在这里。"
          />
        ) : (
          <V3Table>
            <thead>
              <V3Tr>
                <V3Th className="min-w-[150px]">时间</V3Th>
                <V3Th>级别</V3Th>
                <V3Th>事件类型</V3Th>
                <V3Th>节点</V3Th>
                <V3Th className="min-w-[260px]">标题</V3Th>
              </V3Tr>
            </thead>
            <tbody>
              {events.map((event: RuntimeEvent) => (
                <V3Tr
                  key={event.id}
                  className="cursor-pointer"
                  onClick={() => setSelectedEvent(event)}
                >
                  <V3Td className="whitespace-nowrap text-xs text-v3-ink-2 tabular-nums">
                    {formatLogDateTime(event.created_at)}
                  </V3Td>
                  <V3Td>
                    <StatusPill tone={severityTone[event.severity] ?? "mute"}>
                      {severityLabel[event.severity] ?? event.severity}
                    </StatusPill>
                  </V3Td>
                  <V3Td className="whitespace-nowrap text-sm">{event.event_type}</V3Td>
                  <V3Td className="max-w-[160px] truncate font-mono text-xs">{event.node_id || "-"}</V3Td>
                  <V3Td className="max-w-[320px] truncate text-sm">
                    {event.title}
                    {event.description ? (
                      <span className="block truncate text-xs text-v3-ink-3">{event.description}</span>
                    ) : null}
                  </V3Td>
                </V3Tr>
              ))}
            </tbody>
          </V3Table>
        )}

        <LogPagination
          isFetching={eventsQuery.isFetching}
          itemCount={events.length}
          offset={offset}
          onOffsetChange={setOffset}
          pageSize={LOG_PAGE_SIZE}
        />
      </WorkSurface>

      <Sheet open={selectedEvent !== null} onOpenChange={(open) => { if (!open) setSelectedEvent(null); }}>
        <SheetContent side="right" className="w-[420px] max-w-[45vw] overflow-y-auto">
          {selectedEvent && (
            <>
              <SheetHeader>
                <SheetTitle className="text-base font-semibold text-v3-ink">{selectedEvent.title}</SheetTitle>
              </SheetHeader>
              <div className="mt-4 flex flex-col gap-3 text-sm">
                <DetailRow label="级别">
                  <StatusPill tone={severityTone[selectedEvent.severity] ?? "mute"}>
                    {severityLabel[selectedEvent.severity] ?? selectedEvent.severity}
                  </StatusPill>
                </DetailRow>
                <DetailRow label="事件类型">{selectedEvent.event_type}</DetailRow>
                <DetailRow label="来源">{selectedEvent.source}</DetailRow>
                <DetailRow label="节点 ID">{selectedEvent.node_id || "—"}</DetailRow>
                <DetailRow label="描述">{selectedEvent.description || "—"}</DetailRow>
                <DetailRow label="时间">{formatLogDateTime(selectedEvent.created_at)}</DetailRow>
                {selectedEvent.payload && (
                  <div className="mt-2">
                    <div className="text-xs text-v3-ink-2 mb-1">Payload</div>
                    <pre className="rounded-md bg-v3-card-soft p-2 text-xs text-v3-ink overflow-x-auto">
                      {JSON.stringify(selectedEvent.payload, null, 2)}
                    </pre>
                  </div>
                )}
              </div>
            </>
          )}
        </SheetContent>
      </Sheet>
    </>
  );
}

function DetailRow({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="flex items-start gap-3">
      <span className="w-20 shrink-0 text-xs text-v3-ink-2">{label}</span>
      <span className="min-w-0 break-all text-v3-ink">{children}</span>
    </div>
  );
}
```

- [ ] **Step 2: 浏览器验证**

访问 `/logs/runtime`：chips 显示，点击"错误"筛选，点击行弹出 Sheet（含 payload JSON）。

- [ ] **Step 3: Commit**

```bash
git add apps/web/src/routes/_authenticated/logs/runtime/index.tsx
git commit -m "feat(web/logs): refactor runtime event route with chips and detail sheet"
```

---

## Self-Review

**Spec coverage check:**

| Spec 要求 | 对应任务 | 状态 |
|----------|---------|------|
| Sidebar 展开菜单 → 单条直链 | Task 1 | ✓ |
| `/logs` 重定向到 `/logs/login` | Task 1 | ✓ |
| 父布局路由 route.tsx (Header + Tabs + Outlet) | Task 2 | ✓ |
| 子路由移除 Header/Main | Tasks 4-6 | ✓ |
| 快速筛选 chips | Task 3 + Tasks 4-6 | ✓ |
| 行详情 Sheet | Tasks 4-6 | ✓ |
| 登录日志 chips (event_type) | Task 4 | ✓ |
| 操作日志 chips (module) | Task 5 | ✓ |
| 平台事件 chips (severity) | Task 6 | ✓ |
| 现有过滤/分页功能保留 | Tasks 4-6 | ✓ |
| 测试更新 | Task 5 | ✓ |

**Placeholder scan:** 无 TBD / TODO

**Type consistency:**
- `LogChips` 接口在 Task 3 定义，Tasks 4-6 引用一致
- Sheet 组件来自 `@/components/ui/sheet`，所有任务一致
- filter 字段名（event_type, module, severity）匹配 API 类型

**Task decomposition:**
- Task 1: Sidebar + redirect (独立可测)
- Task 2: 父布局 (可单独浏览器验证)
- Task 3: 共用组件 (被 Tasks 4-6 依赖)
- Tasks 4-6: 三个子路由并行可做，互不依赖

---

## Execution Handoff

Plan complete and saved to `docs/superpowers/plans/2026-07-06-logs-tab-navigation-redesign.md`.

**Two execution options:**

**1. Subagent-Driven (recommended)** — I dispatch a fresh subagent per task, review between tasks, fast iteration

**2. Inline Execution** — Execute tasks in this session using executing-plans, batch execution with checkpoints

**Which approach?**
