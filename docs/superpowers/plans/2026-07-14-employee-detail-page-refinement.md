# 数字员工详情页优化 Implementation Plan

> 复核状态：已实施，现状与配对spec一致（同日spec见上，10任务全部完成并合并main，遗留三瑕疵07-14当日跟进修复）。

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 按 `docs/superpowers/specs/2026-07-14-employee-detail-page-refinement-design.md` 优化数字员工详情页:事件流语义化时间线、指标条精简+通道警示条、状态中文化、删除上下文包流程图、技能入口修正、生效上下文去重、抽屉格式化、重复代码收敛、底部快照卡改造。

**Architecture:** 全部改动在 `apps/web`(Console 层)。新增一个纯展示组件 `RunEventTimeline`(把 `DigitalEmployeeRunEvent[]` 折叠成语义时间线),其余是对既有组件的收敛与删减。不改契约、不改后端。

**Tech Stack:** React + TanStack Router/Query,vitest browser mode(`vitest-browser-react`),v3 设计 token(`SoftCard`/`StatusPill`/`V3MetricCard` 等 superteam 组件)。

## Global Constraints

- 所有用户可见文案用中文;状态文案统一走 `src/lib/status-labels.ts`。
- Web 内部跳转必须用 TanStack Router `Link`/`navigate`;禁止原生 `<a href>`(外链除外)。
- 测试命令:`corepack pnpm --filter @superteam/web test [文件路径]`(串行 wrapper,可传文件过滤);禁止 `npx vitest run` / `npx playwright install`。
- 样式只用已有 v3 token/组件(`text-v3-ink`、`bg-v3-card-soft`、`rounded-v3-inner` 等),改前已核对 DESIGN.md 相关约定。
- 每个任务收尾提交一次,commit message 用 conventional 风格 + `Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>`。
- 事件 payload 字段来源(不可凭空造字段):`session_started{session_id}`、`text_delta{text}`、`tool_started{tool_id,name,input_excerpt,input_truncated}`、`tool_completed{tool_id,is_error,output_excerpt,output_truncated}`、`turn_completed{summary}`(usage 不入 payload,防御性读取)、`turn_error{message}`;生命周期:`run_dispatched/run_completed/run_failed/run_cancelled/run_timed_out/run_reaped_stale`。

---

### Task 1: 共享 provider 标签 + 运行状态标签收敛

**Files:**
- Create: `apps/web/src/features/employees/provider-label.ts`
- Create: `apps/web/src/features/employees/provider-label.test.ts`
- Modify: `apps/web/src/features/employees/detail.tsx`(删除文件底部本地 `providerDisplayName`,改 import)
- Modify: `apps/web/src/features/employees/components/employee-detail-header.tsx`(同上)
- Modify: `apps/web/src/features/employees/components/effective-context-panel.tsx`(同上)
- Modify: `apps/web/src/features/employees/create.tsx`(删除本地 `providerLabels`/`providerLabel`,call site 改用共享函数)
- Modify: `apps/web/src/features/employees/index.tsx`(删除本地 `providerLabel`,call site 改用共享函数)
- Modify: `apps/web/src/features/employees/components/run-detail-drawer.tsx`(删除本地 `runStatusLabel`,改 import;Provider 值走 `providerDisplayName`)

**Interfaces:**
- Produces: `providerDisplayName(value: string): string`,from `@/features/employees/provider-label` — 后续任务(Task 3/4)依赖。
- Consumes: `runStatusLabel(status: string | undefined): string`,from `@/lib/status-labels`(已存在)。

**注意:** 本地版 `runStatusLabel` 把 `dispatching` 译为「调度中」,lib 版为「分派中」;统一采用 lib 版「分派中」。

- [ ] **Step 1: 写失败测试**

创建 `apps/web/src/features/employees/provider-label.test.ts`:

```ts
import { describe, expect, it } from "vitest";
import { providerDisplayName } from "./provider-label";

describe("providerDisplayName", () => {
  it("maps known provider ids across separator and case variants", () => {
    expect(providerDisplayName("claude-code")).toBe("Claude Code");
    expect(providerDisplayName("claude_code")).toBe("Claude Code");
    expect(providerDisplayName("claude")).toBe("Claude Code");
    expect(providerDisplayName("OpenCode")).toBe("OpenCode");
    expect(providerDisplayName("open-code")).toBe("OpenCode");
    expect(providerDisplayName(" codex ")).toBe("Codex");
  });

  it("falls back to the raw value for unknown providers", () => {
    expect(providerDisplayName("custom-provider")).toBe("custom-provider");
  });
});
```

- [ ] **Step 2: 跑测试确认失败**

Run: `corepack pnpm --filter @superteam/web test src/features/employees/provider-label.test.ts`
Expected: FAIL(模块不存在)。

- [ ] **Step 3: 实现共享函数**

创建 `apps/web/src/features/employees/provider-label.ts`:

```ts
const PROVIDER_LABELS: Record<string, string> = {
  claude: "Claude Code",
  claude_code: "Claude Code",
  codex: "Codex",
  open_code: "OpenCode",
  opencode: "OpenCode",
};

/** 统一 provider 展示名:大小写、`-`/`_` 分隔符变体均归一;未知值原样返回。 */
export function providerDisplayName(value: string): string {
  const normalized = value.trim().toLowerCase().replace(/-/g, "_");
  return PROVIDER_LABELS[normalized] ?? value;
}
```

- [ ] **Step 4: 跑测试确认通过**

Run: `corepack pnpm --filter @superteam/web test src/features/employees/provider-label.test.ts`
Expected: PASS。

- [ ] **Step 5: 迁移 5 处副本**

对以下文件:删除文件底部的本地 `providerDisplayName` / `providerLabel` 函数(create.tsx 还要删 `providerLabels` record;`canonicalProviderTypes` 保留),顶部加 import,call site 换名:

1. `detail.tsx`:`import { providerDisplayName } from "./provider-label";`(调用点:`EmployeeMetricsStrip` 的 `providerType` 参数)。
2. `employee-detail-header.tsx`:`import { providerDisplayName } from "../provider-label";`。
3. `effective-context-panel.tsx`:`import { providerDisplayName } from "../provider-label";`。
4. `create.tsx`:`import { providerDisplayName } from "./provider-label";`,所有 `providerLabel(` 调用改为 `providerDisplayName(`(用 `rg -n "providerLabel\(" apps/web/src/features/employees/create.tsx` 找全)。
5. `index.tsx`:同 create.tsx 方式处理(调用点在 `provider = providerLabel(execution.provider_type)`)。

- [ ] **Step 6: 抽屉状态标签收敛**

`run-detail-drawer.tsx`:
- 删除本地 `runStatusLabel` 函数(文件底部 switch)。
- 顶部加:`import { runStatusLabel } from "@/lib/status-labels";` 与 `import { providerDisplayName } from "../provider-label";`。
- `<SummaryItem label="Provider" value={displayedRun.provider_type} />` 改为 `value={providerDisplayName(displayedRun.provider_type)}`。

- [ ] **Step 7: 全量员工特性测试回归**

Run: `corepack pnpm --filter @superteam/web test src/features/employees`
Expected: PASS(无行为断言依赖被删函数;若 create/index 测试断言标签文本,共享实现输出相同)。

- [ ] **Step 8: Commit**

```bash
git add -A apps/web/src/features/employees
git commit -m "refactor(web): consolidate provider display name and run status labels

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 2: RunEventTimeline 语义化时间线组件(TDD)

**Files:**
- Create: `apps/web/src/features/employees/components/run-event-timeline.tsx`
- Create: `apps/web/src/features/employees/components/run-event-timeline.test.tsx`

**Interfaces:**
- Produces: `RunEventTimeline({ events, limitReached }: { events: DigitalEmployeeRunEvent[]; limitReached?: boolean })` — Task 3 在抽屉中使用。
- Consumes: `DigitalEmployeeRunEvent` from `@/lib/api/employees`;`StatusPill`, `V3Tone` from `@/components/superteam`。

- [ ] **Step 1: 写失败测试**

创建 `apps/web/src/features/employees/components/run-event-timeline.test.tsx`:

```tsx
import { describe, expect, it } from "vitest";
import { render } from "vitest-browser-react";
import { RunEventTimeline } from "./run-event-timeline";
import type { DigitalEmployeeRunEvent } from "@/lib/api/employees";

function event(overrides: Partial<DigitalEmployeeRunEvent>): DigitalEmployeeRunEvent {
  return { event_type: "text_delta", sequence_number: 1, payload: {}, ...overrides };
}

describe("RunEventTimeline", () => {
  it("renders Chinese lifecycle markers and merges consecutive text deltas", async () => {
    const screen = await render(
      <RunEventTimeline
        events={[
          event({ event_type: "session_started", sequence_number: 1, payload: { session_id: "sess-123" } }),
          event({ event_type: "turn_started", sequence_number: 2 }),
          event({ event_type: "text_delta", sequence_number: 3, payload: { text: "正在分析" } }),
          event({ event_type: "text_delta", sequence_number: 4, payload: { text: "需求" } }),
          event({ event_type: "turn_completed", sequence_number: 5, payload: { summary: "分析完成" } }),
          event({ event_type: "run_completed", sequence_number: 2147483000 }),
        ]}
      />,
    );

    await expect.element(screen.getByText("会话已建立")).toBeVisible();
    await expect.element(screen.getByText("sess-123")).toBeVisible();
    await expect.element(screen.getByText("回合开始")).toBeVisible();
    await expect.element(screen.getByText("正在分析需求")).toBeVisible();
    await expect.element(screen.getByText("回合完成")).toBeVisible();
    await expect.element(screen.getByText("分析完成")).toBeVisible();
    await expect.element(screen.getByText("运行完成")).toBeVisible();
  });

  it("pairs tool started/completed into one row with expandable excerpts", async () => {
    const screen = await render(
      <RunEventTimeline
        events={[
          event({
            event_type: "tool_started",
            sequence_number: 1,
            payload: { tool_id: "t1", name: "Read", input_excerpt: "path=/tmp/a.txt" },
          }),
          event({
            event_type: "tool_completed",
            sequence_number: 2,
            payload: { tool_id: "t1", is_error: false, output_excerpt: "file contents", output_truncated: true },
          }),
        ]}
      />,
    );

    await expect.element(screen.getByText("Read")).toBeVisible();
    await expect.element(screen.getByText("成功")).toBeVisible();
    await expect.element(screen.getByText("内容已截断。")).toBeVisible();
    expect(screen.getByText("工具调用").elements().length).toBe(1);

    await screen.getByText("输入").click();
    await expect.element(screen.getByText("path=/tmp/a.txt")).toBeVisible();
    await screen.getByText("输出").click();
    await expect.element(screen.getByText("file contents")).toBeVisible();
  });

  it("renders an orphan failed tool_completed as a standalone row", async () => {
    const screen = await render(
      <RunEventTimeline
        events={[
          event({
            event_type: "tool_completed",
            sequence_number: 7,
            payload: { tool_id: "t9", is_error: true, output_excerpt: "boom" },
          }),
        ]}
      />,
    );

    await expect.element(screen.getByText("工具调用")).toBeVisible();
    await expect.element(screen.getByText("t9")).toBeVisible();
    await expect.element(screen.getByText("失败")).toBeVisible();
  });

  it("renders turn errors, unknown event types and the limit hint", async () => {
    const screen = await render(
      <RunEventTimeline
        events={[
          event({ event_type: "turn_error", sequence_number: 1, payload: { message: "provider crashed" } }),
          event({ event_type: "provider.stdout", sequence_number: 2, payload: { text: "raw" } }),
        ]}
        limitReached
      />,
    );

    await expect.element(screen.getByText("回合出错")).toBeVisible();
    await expect.element(screen.getByText("provider crashed")).toBeVisible();
    await expect.element(screen.getByText("provider.stdout")).toBeVisible();
    await expect.element(screen.getByText("仅显示前 2 条事件。")).toBeVisible();
  });
});
```

- [ ] **Step 2: 跑测试确认失败**

Run: `corepack pnpm --filter @superteam/web test src/features/employees/components/run-event-timeline.test.tsx`
Expected: FAIL(组件不存在)。

- [ ] **Step 3: 实现组件**

创建 `apps/web/src/features/employees/components/run-event-timeline.tsx`:

```tsx
import type { ReactNode } from "react";
import { StatusPill, type V3Tone } from "@/components/superteam";
import type { DigitalEmployeeRunEvent } from "@/lib/api/employees";

type RunEventTimelineProps = {
  events: DigitalEmployeeRunEvent[];
  /** 事件数达到查询上限(50)时由调用方置 true,底部渲染截断提示。 */
  limitReached?: boolean;
};

type MarkerItem = {
  kind: "marker";
  key: string;
  label: string;
  tone: V3Tone;
  detail?: string;
  mono?: boolean;
  events: DigitalEmployeeRunEvent[];
};

type TextItem = { kind: "text"; key: string; text: string; events: DigitalEmployeeRunEvent[] };

type ToolItem = {
  kind: "tool";
  key: string;
  toolId: string;
  name?: string;
  status: "running" | "ok" | "error";
  inputExcerpt?: string;
  outputExcerpt?: string;
  truncated: boolean;
  events: DigitalEmployeeRunEvent[];
};

type ErrorItem = { kind: "error"; key: string; message: string; events: DigitalEmployeeRunEvent[] };

type UnknownItem = { kind: "unknown"; key: string; eventType: string; events: DigitalEmployeeRunEvent[] };

type TimelineItem = MarkerItem | TextItem | ToolItem | ErrorItem | UnknownItem;

const LIFECYCLE_MARKERS: Record<string, { label: string; tone: V3Tone }> = {
  run_cancelled: { label: "运行已取消", tone: "warn" },
  run_completed: { label: "运行完成", tone: "ok" },
  run_dispatched: { label: "命令已下发", tone: "info" },
  run_failed: { label: "运行失败", tone: "danger" },
  run_reaped_stale: { label: "运行已被回收", tone: "danger" },
  run_timed_out: { label: "运行已超时", tone: "danger" },
};

export function RunEventTimeline({ events, limitReached }: RunEventTimelineProps) {
  const items = buildTimeline(events);
  return (
    <div className="space-y-2">
      {items.map((item) => (
        <TimelineRow item={item} key={item.key} />
      ))}
      {limitReached ? <p className="text-xs text-v3-ink-3">仅显示前 {events.length} 条事件。</p> : null}
    </div>
  );
}

function stringField(payload: Record<string, unknown> | undefined, key: string): string | undefined {
  const value = payload?.[key];
  return typeof value === "string" ? value : undefined;
}

function buildTimeline(events: DigitalEmployeeRunEvent[]): TimelineItem[] {
  const sorted = [...events].sort((a, b) => a.sequence_number - b.sequence_number);
  const items: TimelineItem[] = [];
  // tool_completed 与最近一个同 tool_id 的 tool_started 合并为一行。
  const openTools = new Map<string, ToolItem>();

  for (const event of sorted) {
    const payload = event.payload ?? {};
    switch (event.event_type) {
      case "session_started": {
        items.push({
          kind: "marker",
          key: `event-${event.sequence_number}`,
          label: "会话已建立",
          tone: "info",
          detail: stringField(payload, "session_id"),
          mono: true,
          events: [event],
        });
        break;
      }
      case "turn_started": {
        items.push({
          kind: "marker",
          key: `event-${event.sequence_number}`,
          label: "回合开始",
          tone: "mute",
          events: [event],
        });
        break;
      }
      case "text_delta": {
        const text = stringField(payload, "text") ?? "";
        const last = items[items.length - 1];
        if (last?.kind === "text") {
          last.text += text;
          last.events.push(event);
        } else {
          items.push({ kind: "text", key: `event-${event.sequence_number}`, text, events: [event] });
        }
        break;
      }
      case "tool_started": {
        const toolId = stringField(payload, "tool_id") ?? `tool-${event.sequence_number}`;
        const item: ToolItem = {
          kind: "tool",
          key: `event-${event.sequence_number}`,
          toolId,
          name: stringField(payload, "name"),
          status: "running",
          inputExcerpt: stringField(payload, "input_excerpt"),
          truncated: payload.input_truncated === true,
          events: [event],
        };
        items.push(item);
        openTools.set(toolId, item);
        break;
      }
      case "tool_completed": {
        const toolId = stringField(payload, "tool_id") ?? `tool-${event.sequence_number}`;
        const status: ToolItem["status"] = payload.is_error === true ? "error" : "ok";
        const open = openTools.get(toolId);
        if (open) {
          open.status = status;
          open.outputExcerpt = stringField(payload, "output_excerpt");
          open.truncated = open.truncated || payload.output_truncated === true;
          open.events.push(event);
          openTools.delete(toolId);
        } else {
          // 前 50 条上限截断可能只留下 completed 一半,按独立工具行展示。
          items.push({
            kind: "tool",
            key: `event-${event.sequence_number}`,
            toolId,
            status,
            outputExcerpt: stringField(payload, "output_excerpt"),
            truncated: payload.output_truncated === true,
            events: [event],
          });
        }
        break;
      }
      case "turn_completed": {
        const summary = stringField(payload, "summary");
        // usage 目前不入 payload(runtime 侧丢弃),防御性读取以兼容未来回写。
        const usage = payload.usage;
        const totalTokens =
          usage && typeof usage === "object" && typeof (usage as Record<string, unknown>).total_tokens === "number"
            ? ((usage as Record<string, unknown>).total_tokens as number)
            : undefined;
        const detailParts = [
          summary,
          totalTokens !== undefined ? `${totalTokens.toLocaleString("zh-CN")} tokens` : undefined,
        ].filter((part): part is string => Boolean(part));
        items.push({
          kind: "marker",
          key: `event-${event.sequence_number}`,
          label: "回合完成",
          tone: "ok",
          detail: detailParts.length ? detailParts.join(" · ") : undefined,
          events: [event],
        });
        break;
      }
      case "turn_error": {
        items.push({
          kind: "error",
          key: `event-${event.sequence_number}`,
          message: stringField(payload, "message") ?? "回合执行出错",
          events: [event],
        });
        break;
      }
      default: {
        const marker = LIFECYCLE_MARKERS[event.event_type];
        if (marker) {
          items.push({
            kind: "marker",
            key: `event-${event.sequence_number}`,
            label: marker.label,
            tone: marker.tone,
            events: [event],
          });
        } else {
          items.push({
            kind: "unknown",
            key: `event-${event.sequence_number}`,
            eventType: event.event_type,
            events: [event],
          });
        }
      }
    }
  }
  return items;
}

function TimelineRow({ item }: { item: TimelineItem }) {
  switch (item.kind) {
    case "marker":
      return (
        <TimelineCard events={item.events}>
          <div className="flex flex-wrap items-center gap-2">
            <StatusPill tone={item.tone}>{item.label}</StatusPill>
            {item.detail ? (
              <span
                className={
                  item.mono ? "break-all font-mono text-xs text-v3-ink-2" : "text-xs text-v3-ink-2"
                }
              >
                {item.detail}
              </span>
            ) : null}
          </div>
        </TimelineCard>
      );
    case "text":
      return (
        <TimelineCard events={item.events}>
          <p className="whitespace-pre-wrap break-words text-sm leading-6 text-v3-ink">{item.text}</p>
        </TimelineCard>
      );
    case "tool":
      return (
        <TimelineCard events={item.events}>
          <div className="flex flex-wrap items-center gap-2">
            <span className="text-[11px] text-v3-ink-3">工具调用</span>
            <span className="font-mono text-xs text-v3-ink">{item.name ?? item.toolId}</span>
            <StatusPill tone={item.status === "error" ? "danger" : item.status === "ok" ? "ok" : "mute"}>
              {item.status === "error" ? "失败" : item.status === "ok" ? "成功" : "运行中"}
            </StatusPill>
          </div>
          {item.inputExcerpt ? <ExcerptBlock label="输入" value={item.inputExcerpt} /> : null}
          {item.outputExcerpt ? <ExcerptBlock label="输出" value={item.outputExcerpt} /> : null}
          {item.truncated ? <p className="mt-1 text-[11px] text-v3-ink-3">内容已截断。</p> : null}
        </TimelineCard>
      );
    case "error":
      return (
        <TimelineCard events={item.events}>
          <div className="rounded-v3-inner bg-v3-danger-soft p-2">
            <p className="text-xs font-medium text-v3-danger">回合出错</p>
            <p className="mt-1 whitespace-pre-wrap break-words text-xs text-v3-ink-2">{item.message}</p>
          </div>
        </TimelineCard>
      );
    case "unknown":
      return (
        <TimelineCard events={item.events}>
          <StatusPill tone="mute">{item.eventType}</StatusPill>
        </TimelineCard>
      );
  }
}

function TimelineCard({ children, events }: { children: ReactNode; events: DigitalEmployeeRunEvent[] }) {
  return (
    <div className="rounded-md border border-v3-line px-3 py-2">
      {children}
      <details className="mt-1">
        <summary className="cursor-pointer text-[11px] text-v3-ink-3">原始 JSON</summary>
        <pre className="mt-1 max-h-64 overflow-auto whitespace-pre-wrap break-words rounded-v3-inner bg-v3-card-soft p-2 font-mono text-xs text-v3-ink-2">
          {JSON.stringify(events.length === 1 ? events[0] : events, null, 2)}
        </pre>
      </details>
    </div>
  );
}

function ExcerptBlock({ label, value }: { label: string; value: string }) {
  return (
    <details className="mt-1 min-w-0">
      <summary className="cursor-pointer text-[11px] text-v3-ink-3">{label}</summary>
      <pre className="mt-1 max-h-64 overflow-auto whitespace-pre-wrap break-words rounded-v3-inner bg-v3-card-soft p-2 font-mono text-xs text-v3-ink-2">
        {value}
      </pre>
    </details>
  );
}
```

- [ ] **Step 4: 跑测试确认通过**

Run: `corepack pnpm --filter @superteam/web test src/features/employees/components/run-event-timeline.test.tsx`
Expected: PASS(4 个用例)。

- [ ] **Step 5: Commit**

```bash
git add apps/web/src/features/employees/components/run-event-timeline.tsx apps/web/src/features/employees/components/run-event-timeline.test.tsx
git commit -m "feat(web): add semantic run event timeline component

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 3: 抽屉接入时间线 + 时间/结果格式化

**Files:**
- Modify: `apps/web/src/features/employees/components/run-detail-drawer.tsx`
- Modify: `apps/web/src/features/employees/components/run-detail-drawer.test.tsx`
- Modify: `apps/web/src/features/employees/detail.test.tsx`(事件 fixture 与断言改为真实事件类型)

**Interfaces:**
- Consumes: `RunEventTimeline`(Task 2)、`formatDateTime` from `@/lib/format-time`、`runStatusLabel` / `providerDisplayName`(Task 1)。

- [ ] **Step 1: 更新测试(先红)**

`run-detail-drawer.test.tsx`:events fixture(`createFetcher` 内)从

```ts
return jsonResponse([{ event_type: "provider.stdout", sequence_number: 1, payload: { text: "正在执行" } }]);
```

改为

```ts
return jsonResponse([{ event_type: "text_delta", sequence_number: 1, payload: { text: "正在执行" } }]);
```

并在 `shows events and stops an active run` 用例中,`await expect.element(screen.getByText(/正在执行/)).toBeVisible();` 之后加:

```ts
await expect.element(screen.getByText("更新时间")).toBeVisible();
```

新增用例(放在 describe 末尾),验证结论提取与 JSON 折叠:

```ts
it("renders completed result conclusion as prose with raw JSON collapsed", async () => {
  const completedRun: DigitalEmployeeRunListItem = {
    ...runningRun,
    status: "completed",
    result: { summary: "已生成验收报告", detail: { files: 3 } },
  };
  const screen = await render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <RunDetailDrawer
        apiOptions={{ baseUrl: "http://control-plane.local", fetcher: createFetcher() }}
        employeeId={employeeId}
        onOpenChange={vi.fn()}
        onStopped={vi.fn()}
        open
        run={completedRun}
      />
    </QueryClientProvider>,
  );

  await expect.element(screen.getByText("已生成验收报告")).toBeVisible();
  await expect.element(screen.getByText("原始结果 JSON")).toBeVisible();
});
```

`detail.test.tsx`:
1. `createDetailFetcher` 默认 `events` fixture 的 `event_type: "provider.stdout"` 改为 `"text_delta"`(其余字段不变)。
2. 首个用例中删除 `await expect.element(screen.getByText("provider.stdout")).toBeVisible();` 这一行(保留 `正在分析需求` 断言)。
3. `switches event stream...` 用例的 `eventsByRunId` 两处 `provider.stdout` / `provider.stderr` 都改为 `text_delta`。

- [ ] **Step 2: 跑测试确认失败**

Run: `corepack pnpm --filter @superteam/web test src/features/employees/components/run-detail-drawer.test.tsx`
Expected: FAIL(新用例「原始结果 JSON」不存在;text_delta 事件当前渲染为三列行仍显示文本,旧断言可能过,但新增断言红)。

- [ ] **Step 3: 改造抽屉**

`run-detail-drawer.tsx`:

1. imports 增加:

```tsx
import { formatDateTime } from "@/lib/format-time";
import { RunEventTimeline } from "./run-event-timeline";
```

(Task 1 已加 `runStatusLabel` / `providerDisplayName` imports。)

2. 摘要区四项改为:

```tsx
<div className="grid gap-2 text-sm md:grid-cols-2">
  <SummaryItem label="命令" mono value={displayedRun.command_id} />
  <SummaryItem label="Provider" value={providerDisplayName(displayedRun.provider_type)} />
  <SummaryItem label="节点" mono value={displayedRun.node_id || displayedRun.runtime_node_id} />
  <SummaryItem label="更新时间" value={formatRunTimestamp(displayedRun)} />
</div>
```

3. `SummaryItem` 改为:

```tsx
function SummaryItem({ label, value, mono }: { label: string; value: string; mono?: boolean }) {
  return (
    <div className="min-w-0 rounded-md border border-v3-line bg-v3-card-soft px-3 py-2">
      <p className="text-xs text-v3-ink-3">{label}</p>
      <p className={mono ? "mt-1 truncate font-mono text-xs text-v3-ink" : "mt-1 truncate text-sm font-medium text-v3-ink"}>
        {value}
      </p>
    </div>
  );
}
```

4. 新增 helper(文件底部):

```tsx
function formatRunTimestamp(run: DigitalEmployeeRunListItem) {
  const value = run.updated_at ?? run.created_at;
  return value ? formatDateTime(value) : "-";
}

const RESULT_TEXT_KEYS = ["summary", "conclusion", "text", "message"] as const;

function extractResultText(result: unknown): string | undefined {
  if (!result || typeof result !== "object") return undefined;
  const record = result as Record<string, unknown>;
  for (const key of RESULT_TEXT_KEYS) {
    const value = record[key];
    if (typeof value === "string" && value.trim()) return value;
  }
  return undefined;
}
```

5. `ResultBlock` 改为:

```tsx
function ResultBlock({ run }: { run: DigitalEmployeeRunListItem }) {
  const conclusion = extractResultText(run.result);
  const rawJson = compactJson(run.result);
  return (
    <div>
      <p className="text-sm font-medium">结果</p>
      {conclusion ? (
        <p className="mt-2 whitespace-pre-wrap break-words rounded-md border border-v3-line bg-v3-card-soft p-3 text-sm leading-6 text-v3-ink">
          {conclusion}
        </p>
      ) : null}
      {rawJson && conclusion ? (
        <details className="mt-2">
          <summary className="cursor-pointer text-xs text-v3-ink-3">原始结果 JSON</summary>
          <pre className="mt-2 max-h-72 overflow-auto rounded-md border border-v3-line bg-v3-card-soft p-3 text-xs">
            {rawJson}
          </pre>
        </details>
      ) : null}
      {rawJson && !conclusion ? (
        <pre className="mt-2 max-h-72 overflow-auto rounded-md border border-v3-line bg-v3-card-soft p-3 text-xs">
          {rawJson}
        </pre>
      ) : null}
      {!rawJson && !conclusion ? <p className="mt-2 text-sm text-v3-ink-2">无结果数据</p> : null}
    </div>
  );
}
```

6. 事件流区块:`events.data?.length` 分支的 `<div className="space-y-2">…</div>`(map RunEventRow)整体替换为:

```tsx
<RunEventTimeline events={events.data} limitReached={events.data.length >= 50} />
```

7. 删除 `RunEventRow` 函数(`compactJson` 保留,FailureBlock/ResultBlock 仍用)。

- [ ] **Step 4: 跑测试确认通过**

Run: `corepack pnpm --filter @superteam/web test src/features/employees/components/run-detail-drawer.test.tsx src/features/employees/detail.test.tsx`
Expected: PASS。

- [ ] **Step 5: Commit**

```bash
git add apps/web/src/features/employees/components/run-detail-drawer.tsx apps/web/src/features/employees/components/run-detail-drawer.test.tsx apps/web/src/features/employees/detail.test.tsx
git commit -m "feat(web): semantic event timeline and readable result in run drawer

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 4: 指标条精简 + 通道断开警示条

**Files:**
- Modify: `apps/web/src/features/employees/components/employee-metrics-strip.tsx`
- Modify: `apps/web/src/features/employees/components/employee-metrics-strip.test.tsx`
- Modify: `apps/web/src/features/employees/detail.tsx`
- Modify: `apps/web/src/features/employees/detail.test.tsx`

**Interfaces:**
- Produces: `EmployeeMetricsStrip({ stats, providerType })` — props 收窄,`runtimeNodeLabel`/`commandChannelConnected`/`currentStatusLabel` 删除。
- Consumes: `detail.tsx` 已有派生量 `runtimeCommandChannelDisconnected`;`Alert`/`AlertTitle`/`AlertDescription`/`AlertTriangle`(detail.tsx 已 import)。

- [ ] **Step 1: 更新测试(先红)**

`employee-metrics-strip.test.tsx` 两个用例的 render 改为只传 `providerType` 与 `stats`,并断言删除的卡不在:

```tsx
describe("EmployeeMetricsStrip", () => {
  it("renders formatted stats without runtime/state cards", async () => {
    const screen = await render(<EmployeeMetricsStrip providerType="Claude Code" stats={stats} />);

    await expect.element(screen.getByText("76")).toBeVisible();
    await expect.element(screen.getByText("89.5%")).toBeVisible();
    await expect.element(screen.getByText("68")).toBeVisible();
    await expect.element(screen.getByText("29分0秒")).toBeVisible();
    await expect.element(screen.getByText(/P90 48分0秒/)).toBeVisible();
    await expect.element(screen.getByText(/较上周期/)).toBeVisible();
    expect(screen.getByText("Runtime 执行位置").query()).toBeNull();
    expect(screen.getByText("当前状态").query()).toBeNull();
  });

  it("shows placeholder dashes when stats are unavailable", async () => {
    const screen = await render(<EmployeeMetricsStrip providerType="Codex" stats={undefined} />);

    await expect.element(screen.getByText("成功率")).toBeVisible();
    expect(screen.getByText("--").elements().length).toBeGreaterThan(0);
  });
});
```

`detail.test.tsx` 新增用例(describe 末尾),并在 `renders final employee config sections...`(Task 7 会重写该用例,此处先不动)之外验证警示条:

```tsx
it("shows a page-level alert when the runtime command channel is disconnected", async () => {
  const fetcher = createDetailFetcher({
    run: runFixture({ status: "completed" }),
    runtimeOverview: {
      summary: {
        online_nodes: 1,
        total_nodes: 1,
        pending_enrollments: 0,
        active_provider_sessions: 0,
        blocked_events: 0,
      },
      pending_enrollments: [],
      nodes: [
        {
          runtime_node_id: executionInstance.runtime_node_id,
          node_id: "node-a",
          name: "node-a",
          supported_providers: ["codex"],
          max_slots: 3,
          current_load: 0,
          status: "online",
          command_channel_connected: false,
        },
      ],
      provider_capabilities: [],
      recent_events: [],
    },
  });
  const screen = await renderEmployeeDetail(fetcher);

  await expect.element(screen.getByText("Runtime 命令通道未连接")).toBeVisible();
  await expect.element(screen.getByText(/当前无法开始新任务/)).toBeVisible();
});

it("hides the channel alert when the command channel is connected", async () => {
  const screen = await renderEmployeeDetail();

  await expect.element(screen.getByRole("heading", { level: 2, name: "需求分析员工" })).toBeVisible();
  expect(screen.getByText("Runtime 命令通道未连接").query()).toBeNull();
});
```

注意:详情页头部 heading 是 level 2(`<h2>`);若现有用例用了 level 1,以现有断言为准不改。

- [ ] **Step 2: 跑测试确认失败**

Run: `corepack pnpm --filter @superteam/web test src/features/employees/components/employee-metrics-strip.test.tsx src/features/employees/detail.test.tsx`
Expected: FAIL(props 不匹配 / 警示条不存在)。

- [ ] **Step 3: 实现**

`employee-metrics-strip.tsx` 整文件替换为:

```tsx
import { Activity, CheckCircle2, Clock, Gauge, Hand, Server, XCircle } from "lucide-react";
import { V3MetricCard } from "@/components/superteam";
import type { DigitalEmployeeRunStats } from "@/lib/api/employees";

type EmployeeMetricsStripProps = {
  stats: DigitalEmployeeRunStats | undefined;
  providerType: string;
};

export function EmployeeMetricsStrip({ stats, providerType }: EmployeeMetricsStripProps) {
  const trend = formatTrend(stats);

  return (
    <section className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
      <V3MetricCard icon={<Server />} iconTone="brand" label="Provider" value={providerType} />
      <V3MetricCard icon={<Activity />} iconTone="mute" label="累计执行" value={stats ? stats.total_count : "--"} />
      <V3MetricCard icon={<Activity />} iconTone="info" label="近7天" meta={trend} value={stats ? stats.last_7d_count : "--"} />
      <V3MetricCard
        icon={<Gauge />}
        iconTone="ok"
        label="成功率"
        value={stats && stats.success_rate !== null ? formatPercent(stats.success_rate) : "--"}
      />
      <V3MetricCard
        icon={<Clock />}
        iconTone="brand"
        label="平均耗时"
        meta={stats?.p90_duration_sec != null ? `P90 ${formatDuration(stats.p90_duration_sec)}` : undefined}
        value={stats?.avg_duration_sec != null ? formatDuration(stats.avg_duration_sec) : "--"}
      />
      <V3MetricCard icon={<CheckCircle2 />} iconTone="ok" label="成功" value={stats ? stats.succeeded_count : "--"} />
      <V3MetricCard icon={<XCircle />} iconTone="danger" label="失败" value={stats ? stats.failed_count : "--"} />
      <V3MetricCard icon={<Hand />} iconTone="warn" label="人工停止" value={stats ? stats.cancelled_count : "--"} />
    </section>
  );
}

function formatPercent(ratio: number): string {
  return `${(ratio * 100).toFixed(1)}%`;
}

function formatDuration(seconds: number): string {
  const totalSeconds = Math.round(seconds);
  const minutes = Math.floor(totalSeconds / 60);
  const remainSeconds = totalSeconds % 60;
  return `${minutes}分${remainSeconds}秒`;
}

function formatTrend(stats: DigitalEmployeeRunStats | undefined): string | undefined {
  if (!stats) {
    return undefined;
  }
  if (stats.prev_7d_count === 0) {
    return `较上周期 +${stats.last_7d_count}`;
  }
  const change = ((stats.last_7d_count - stats.prev_7d_count) / stats.prev_7d_count) * 100;
  const arrow = change >= 0 ? "↑" : "↓";
  return `较上周期 ${arrow}${Math.abs(change).toFixed(0)}%`;
}
```

`detail.tsx`:

1. `EmployeeMetricsStrip` 调用改为:

```tsx
<EmployeeMetricsStrip providerType={providerDisplayName(employee.data.provider_type)} stats={runStats.data} />
```

2. 在 `<EmployeeDetailHeader …/>` 之前(同一 `flex flex-col gap-4` 容器内首位)插入:

```tsx
{runtimeCommandChannelDisconnected ? (
  <Alert className="border-v3-danger/30 bg-v3-danger-soft text-v3-danger" variant="destructive">
    <AlertTriangle className="size-4" />
    <AlertTitle>Runtime 命令通道未连接</AlertTitle>
    <AlertDescription>当前无法开始新任务，请检查 Runtime Agent 连接状态后重试。</AlertDescription>
  </Alert>
) : null}
```

(文案刻意与开始任务抽屉里的禁用原因「Runtime 命令通道未连接，暂不能开始任务」不同,避免测试重复匹配。)

- [ ] **Step 4: 跑测试确认通过**

Run: `corepack pnpm --filter @superteam/web test src/features/employees/components/employee-metrics-strip.test.tsx src/features/employees/detail.test.tsx`
Expected: PASS。

- [ ] **Step 5: Commit**

```bash
git add apps/web/src/features/employees/components/employee-metrics-strip.tsx apps/web/src/features/employees/components/employee-metrics-strip.test.tsx apps/web/src/features/employees/detail.tsx apps/web/src/features/employees/detail.test.tsx
git commit -m "feat(web): trim employee metrics strip and add channel-disconnected alert

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 5: 状态中文化 + 生效上下文去重 + 技能入口 + 运行表状态/时间

**Files:**
- Modify: `apps/web/src/features/employees/components/employee-detail-header.tsx`
- Modify: `apps/web/src/features/employees/components/employee-detail-header.test.tsx`
- Modify: `apps/web/src/features/employees/components/effective-context-panel.tsx`
- Modify: `apps/web/src/features/employees/components/effective-context-panel.test.tsx`
- Modify: `apps/web/src/features/employees/components/employee-run-history-table.tsx`
- Modify: `apps/web/src/features/employees/components/employee-run-history-table.test.tsx`
- Modify: `apps/web/src/features/employees/detail.test.tsx`(`getByText("dispatching")` → `"分派中"`)

**Interfaces:**
- Consumes: `employeeStatusLabel`, `runStatusLabel` from `@/lib/status-labels`;`formatDateTime` from `@/lib/format-time`。

- [ ] **Step 1: 更新测试(先红)**

1. `employee-detail-header.test.tsx`:`await expect.element(screen.getByText("active")).toBeVisible();` 改为 `await expect.element(screen.getByText("运行中")).toBeVisible();`。
2. `effective-context-panel.test.tsx`:现有用例 `onManageCapabilities: vi.fn()` 提出为变量并新增断言。用例改为:

```tsx
it("renders skill/mcp counts, project constitution, env vars and persona memory status", async () => {
  const onManageCapabilities = vi.fn();
  const screen = await render(
    <EffectiveContextPanel
      employee={employee}
      employeeId="employee-1"
      envVars={{
        isLoading: false,
        isError: false,
        configuredCount: 1,
        totalCount: 2,
        missingNames: ["REDIS_URL"],
      }}
      executionInstance={undefined}
      mcp={{ isLoading: false, isError: false, personalCount: 0, inheritedCount: 1, totalCount: 1 }}
      onManageCapabilities={onManageCapabilities}
      skills={{ isLoading: false, isError: false, personalCount: 1, inheritedCount: 2, totalCount: 3 }}
    />,
  );

  await expect.element(screen.getByText("个人技能 1")).toBeVisible();
  await expect.element(screen.getByText("团队继承技能 2")).toBeVisible();
  await expect.element(screen.getByText("生效总数 3")).toBeVisible();
  await expect.element(screen.getByText("人格记忆：已配置")).toBeVisible();
  await expect.element(screen.getByText("已配置 1")).toBeVisible();
  await expect.element(screen.getByText("REDIS_URL")).toBeVisible();

  // 与头部重复的「状态」行已移除
  expect(screen.getByText("状态", { exact: true }).query()).toBeNull();

  // 技能入口打开管理抽屉而不是跳全局技能页
  await screen.getByRole("button", { name: "管理" }).click();
  expect(onManageCapabilities).toHaveBeenCalledTimes(1);
});
```

3. `employee-run-history-table.test.tsx` 首用例:`await expect.element(screen.getByText("已完成")).toBeVisible();` 改为按作用域断言(行内状态 pill 与过滤 chip 都会是「已完成」):

```tsx
await expect
  .element(screen.getByRole("row", { name: /数据库迁移脚本校验/ }).getByText("已完成"))
  .toBeVisible();
await expect.element(screen.getByRole("button", { name: "已完成" })).toBeVisible();
```

4. `detail.test.tsx`:`await expect.element(screen.getByText("dispatching")).toBeVisible();` 改为 `await expect.element(screen.getByText("分派中")).toBeVisible();`。

- [ ] **Step 2: 跑测试确认失败**

Run: `corepack pnpm --filter @superteam/web test src/features/employees`
Expected: FAIL(中文标签/按钮/去重断言未实现)。

- [ ] **Step 3: 实现**

1. `employee-detail-header.tsx`:加 `import { employeeStatusLabel } from "@/lib/status-labels";`,StatusPill 内容改为:

```tsx
<StatusPill tone={statusTone[employee.status] ?? "mute"}>{employeeStatusLabel(employee.status)}</StatusPill>
```

2. `effective-context-panel.tsx`:
   - 删除基本信息里的 `<InfoItem label="状态" value={employee.status} />`。
   - 技能区的 `<Link className="text-xs text-v3-brand" to="/skills">查看全部</Link>` 替换为:

```tsx
<V3Button onClick={onManageCapabilities} size="sm" variant="ghost">
  管理
</V3Button>
```

   - MCP 的 `/mcp` Link 与「编辑」Link 保持不变(`Link` import 仍需要)。

3. `employee-run-history-table.tsx`:
   - imports 加:`import { runStatusLabel } from "@/lib/status-labels";` 和 `import { formatDateTime } from "@/lib/format-time";`。
   - 行状态 pill:`<StatusPill tone={runStatusTone[item.status]}>{runStatusLabel(item.status)}</StatusPill>`。
   - 时间列:`{item.updated_at ?? item.created_at ? formatDateTime((item.updated_at ?? item.created_at)!) : "-"}` — 为避免非空断言,用文件底部 helper:

```tsx
function formatRowTime(item: DigitalEmployeeRunListItem) {
  const value = item.updated_at ?? item.created_at;
  return value ? formatDateTime(value) : "-";
}
```

时间列改为 `{formatRowTime(item)}`(`DigitalEmployeeRunListItem` 类型该文件已引用,若无则从 `@/lib/api/employees` 引入 type)。

- [ ] **Step 4: 跑测试确认通过**

Run: `corepack pnpm --filter @superteam/web test src/features/employees`
Expected: PASS。

- [ ] **Step 5: Commit**

```bash
git add apps/web/src/features/employees
git commit -m "feat(web): Chinese employee/run status labels, dedupe context panel, fix skills entry

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 6: 删除上下文包流程图

**Files:**
- Delete: `apps/web/src/features/employees/components/context-injection-chain.tsx`
- Delete: `apps/web/src/features/employees/components/context-injection-chain.test.tsx`
- Modify: `apps/web/src/features/employees/detail.tsx`

- [ ] **Step 1: 移除引用与文件**

`detail.tsx`:删除 `import { ContextInjectionChain } from "./components/context-injection-chain";` 与 `<ContextInjectionChain …/>` 整个 JSX 块(`envConfiguredCount`/`inheritedSkillCount`/`mcpQuery`/`personalSkillCount` 等派生量仍被 `EffectiveContextPanel` 使用,保留)。

```bash
git rm apps/web/src/features/employees/components/context-injection-chain.tsx apps/web/src/features/employees/components/context-injection-chain.test.tsx
```

- [ ] **Step 2: 确认无残留引用并回归**

Run: `rg -n "ContextInjectionChain|context-injection-chain" apps/web/src`
Expected: 无输出。

Run: `corepack pnpm --filter @superteam/web test src/features/employees/detail.test.tsx`
Expected: PASS。

- [ ] **Step 3: Commit**

```bash
git add -A apps/web/src/features/employees
git commit -m "feat(web): remove context injection chain diagram from employee detail

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 7: 底部快照卡改造

**Files:**
- Modify: `apps/web/src/features/employees/detail.tsx`
- Modify: `apps/web/src/features/employees/detail.test.tsx`

**Interfaces:**
- Consumes: `BudgetPolicy` type from `@/lib/api/employees`(`{ daily_token_limit?: number | null }`)。

- [ ] **Step 1: 更新测试(先红)**

`detail.test.tsx` 的 `renders final employee config sections without legacy config copy` 用例整体替换为:

```tsx
it("renders persona memory and budget policy without raw JSON snapshot cards", async () => {
  const screen = await renderEmployeeDetail();

  await expect.element(screen.getByText("人格记忆.md")).toBeVisible();
  await expect.element(screen.getByText("# 人格画像\n证据优先")).toBeVisible();
  await expect.element(screen.getByText("预算策略")).toBeVisible();
  await expect.element(screen.getByText("每日 Token 上限")).toBeVisible();
  await expect.element(screen.getByText("12,000")).toBeVisible();
  expect(screen.getByText("能力绑定").query()).toBeNull();
  expect(screen.getByText("运行与缓存状态").query()).toBeNull();
  expect(screen.getByText("角色配置").query()).toBeNull();
  expect(screen.getByText("能力与策略").query()).toBeNull();
});
```

(fixture `budget_policy.daily_token_limit` 为 `12000`,`toLocaleString("zh-CN")` 输出 `12,000`。)

- [ ] **Step 2: 跑测试确认失败**

Run: `corepack pnpm --filter @superteam/web test src/features/employees/detail.test.tsx`
Expected: FAIL(仍渲染四张 JSON 卡)。

- [ ] **Step 3: 实现**

`detail.tsx`:

1. `EmployeeConfigSnapshotSection` 及其下方的 `ConfigSnapshotCard`/`formatConfigSnapshotJson`/`hasRuntimeState` 整体替换为:

```tsx
function EmployeeConfigSnapshotSection({ employee }: { employee: DigitalEmployee }) {
  const personaMemory = employee.persona_memory_markdown?.trim();

  return (
    <section className="grid gap-4 lg:grid-cols-2">
      <SoftCard className="p-4">
        <div className="text-sm font-semibold text-v3-ink">人格记忆.md</div>
        {personaMemory ? (
          <p className="mt-3 whitespace-pre-wrap break-words rounded-[14px] border border-v3-line bg-v3-card-soft p-3 text-sm leading-6 text-v3-ink">
            {personaMemory}
          </p>
        ) : (
          <p className="mt-3 text-sm text-v3-ink-3">未设置</p>
        )}
      </SoftCard>
      <SoftCard className="p-4">
        <div className="text-sm font-semibold text-v3-ink">预算策略</div>
        <BudgetPolicyContent budgetPolicy={employee.budget_policy} />
      </SoftCard>
    </section>
  );
}

function BudgetPolicyContent({ budgetPolicy }: { budgetPolicy?: BudgetPolicy }) {
  const limit = budgetPolicy?.daily_token_limit;
  const extraEntries = Object.entries(budgetPolicy ?? {}).filter(([key]) => key !== "daily_token_limit");

  return (
    <div className="mt-3 space-y-2 text-sm">
      <div className="flex items-center justify-between gap-3 rounded-[14px] border border-v3-line bg-v3-card-soft px-3 py-2">
        <span className="text-v3-ink-2">每日 Token 上限</span>
        <span className="font-medium tabular-nums text-v3-ink">
          {typeof limit === "number" ? limit.toLocaleString("zh-CN") : "未设置"}
        </span>
      </div>
      {extraEntries.map(([key, value]) => (
        <div
          className="flex items-center justify-between gap-3 rounded-[14px] border border-v3-line bg-v3-card-soft px-3 py-2"
          key={key}
        >
          <span className="text-v3-ink-2">{key}</span>
          <span className="break-all font-mono text-xs text-v3-ink">{JSON.stringify(value)}</span>
        </div>
      ))}
    </div>
  );
}
```

2. 调用点 `<EmployeeConfigSnapshotSection employee={employee.data} executionInstance={instance.data} />` 改为 `<EmployeeConfigSnapshotSection employee={employee.data} />`。
3. imports:`BudgetPolicy` type 加入 `@/lib/api/employees` import;`DigitalEmployeeExecutionInstance` type 若仅剩快照区使用则从 import 中删除(先 `rg -n "DigitalEmployeeExecutionInstance" apps/web/src/features/employees/detail.tsx` 确认)。

- [ ] **Step 4: 跑测试确认通过**

Run: `corepack pnpm --filter @superteam/web test src/features/employees/detail.test.tsx`
Expected: PASS。

- [ ] **Step 5: Commit**

```bash
git add apps/web/src/features/employees/detail.tsx apps/web/src/features/employees/detail.test.tsx
git commit -m "feat(web): readable persona/budget snapshot cards, drop JSON dump cards

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 8: 分层门禁 + 端到端真实验证

**Files:** 无新改动(只验证;发现缺陷则回到对应任务修复)。

- [ ] **Step 1: Web 全量门禁**

Run: `corepack pnpm --filter @superteam/web test`
Expected: 全部 PASS。

Run: `corepack pnpm verify:web`
Expected: PASS(lint/typecheck/test 全绿)。

- [ ] **Step 2: 启动真实服务**

Run: `scripts/dev-services.sh status`;若未全部运行,`scripts/dev-services.sh start`(control-plane 启动会自动跑 Atlas 迁移)。确认 Temporal、Control Plane、Web、Runtime Agent 均在运行,且 Web 已加载当前代码(dev server 热更新;必要时 `scripts/dev-services.sh restart web`)。

- [ ] **Step 3: 浏览器真实链路验证(playwright MCP)**

以 dev-services 输出的 Web 地址(默认 `http://localhost:5173`)打开:

1. `/employees` → 进入任一有历史运行的数字员工详情页。
2. 核对:头部状态 pill 为中文(如「就绪」);指标条 8 卡且无「Runtime 执行位置」「当前状态」;页面无「下次任务会注入的上下文包」区块;生效上下文无「状态」行;底部仅「人格记忆.md」「预算策略」两卡且为可读排版。
3. 生效上下文技能区点「管理」→ 右侧打开「管理技能与 MCP」抽屉;MCP「查看全部」仍跳 `/mcp`。
4. 点开一条真实历史运行 → 抽屉事件流为语义时间线:中文节点、正文合并、工具行可折叠展开、原始 JSON 折叠可用;「更新时间」为 `MM/DD HH:mm` 本地格式;完成态运行「结果」为正文 + 原始 JSON 折叠。
5. `scripts/dev-services.sh stop runtime-agent` → 刷新详情页 → 顶部出现「Runtime 命令通道未连接」警示条;`scripts/dev-services.sh start runtime-agent` → 恢复后警示条消失。
6. 若该环境无历史运行数据,从详情页「开始任务」发起一次真实任务,等待事件流真实产生后核对时间线渲染。

任何一步无法执行(服务起不来、无可用 Provider、无真实数据且无法造)→ 按 CLAUDE.md 标记为阻塞并停止,不得以未验证状态宣称完成。

- [ ] **Step 4: 收尾**

Run: `git status`(确认无未提交残留)。
使用项目内 skill `$superteam-completion-check` 走收尾清单。
