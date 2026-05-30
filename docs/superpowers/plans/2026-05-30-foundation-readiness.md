# Foundation Readiness Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 收口 SuperTeam 基础骨架，让 Web、Control Plane、Runtime Agent、contracts 和共享 packages 形成可维护、可扩展、可复用的功能开发基线。

**Architecture:** 先用脚本和测试锁住契约事实源，再同步文档和本地启动路径，最后补齐前端真实数据最小边界。实现只覆盖 foundation readiness，不进入登录、审批、Temporal、OpenFGA、完整业务页面或生产级 Provider 治理。

**Tech Stack:** Next.js + React + TypeScript + Vitest, Go + chi + pgx + sqlc, Rust + Tokio + reqwest, OpenAPI YAML, shell verification scripts.

**Spec:** `docs/superpowers/specs/2026-05-30-foundation-readiness-design.md`

---

## File Structure

### Contract Guard

- Create: `scripts/verify-foundation-contracts.mjs`
  - Reads OpenAPI paths from `contracts/control-plane/openapi.yaml`.
  - Reads registered route literals from `apps/control-plane/internal/api/server.go`.
  - Reads Control Plane paths used by `apps/runtime-agent/src/controlplane/client.rs`.
  - Reads Control Plane paths used by `packages/api-client/src/*.ts`.
  - Fails when any critical route/client path is missing from OpenAPI or when required OpenAPI paths are not registered by Go routes.
- Modify: `package.json`
  - Add `verify:contracts` and `verify:foundation` scripts.
- Test: run `pnpm verify:contracts`.

### Frontend API Boundary

- Modify: `packages/api-client/src/tasks.ts`
  - Add `getTask`, `updateTaskStatus`, `cancelTask`.
- Modify: `packages/api-client/src/tasks.test.ts`
  - Cover the new task methods and exact paths.
- Modify: `packages/api-client/src/runtime.ts`
  - Add `getRuntimeNode`.
- Modify: `packages/api-client/src/runtime.test.ts`
  - Cover the new runtime node method and exact path.
- Modify: `packages/api-client/src/index.ts`
  - Export new functions and input types.

### Core Summaries

- Modify: `packages/core/src/task-summary.ts`
  - Add stable status tone mapping for task summaries.
- Modify: `packages/core/src/task-summary.test.ts`
  - Cover status tones.
- Modify: `packages/core/src/runtime-node-summary.ts`
  - Add stable status tone mapping and capacity percentage for runtime summaries.
- Modify: `packages/core/src/runtime-node-summary.test.ts`
  - Cover status tone and capacity percentage.
- Modify: `packages/core/src/index.ts`
  - Export new summary fields through existing exports.

### Documentation And Changelog

- Modify: `README.md`
  - Make current baseline, local commands, and verification entrypoints match the repo.
- Modify: `docs/development.md`
  - Document Docker/testcontainers behavior and exact verification fallback.
- Modify: `docs/api.md`
  - Align endpoint descriptions with canonical `/api/v1/runtime/tasks/...` paths.
- Modify: `docs/NEXT_STEPS.md`
  - Replace broad next-step language with foundation readiness handoff.
- Modify: `CHANGELOG.md`
  - Record implementation batches as they land.

---

## Task 1: Add Contract Drift Guard

**Files:**
- Create: `scripts/verify-foundation-contracts.mjs`
- Modify: `package.json`
- Modify: `CHANGELOG.md`

- [ ] **Step 1: Write the failing contract guard command**

Modify the root `package.json` scripts so they include:

```json
{
  "scripts": {
    "verify:contracts": "node scripts/verify-foundation-contracts.mjs",
    "verify:foundation": "pnpm verify:contracts && pnpm -r --if-present test && pnpm -r --if-present typecheck && cargo test --manifest-path apps/runtime-agent/Cargo.toml"
  }
}
```

Preserve all existing scripts. Add only the two new script entries.

- [ ] **Step 2: Run the new script and verify it fails because the file is missing**

Run:

```bash
pnpm verify:contracts
```

Expected: FAIL with a Node module-not-found error for `scripts/verify-foundation-contracts.mjs`.

- [ ] **Step 3: Create the contract guard script**

Create `scripts/verify-foundation-contracts.mjs`:

```js
import { readFileSync } from "node:fs";
import { resolve } from "node:path";

const root = process.cwd();

function readText(path) {
  return readFileSync(resolve(root, path), "utf8");
}

function normalizePath(path) {
  return path.replace(/\{id\}/g, "{taskId}").replace(/\{nodeId\}/g, "{nodeId}");
}

function readOpenApiPaths() {
  const openapi = readText("contracts/control-plane/openapi.yaml");
  const matches = [...openapi.matchAll(/^  (\/[^:\n]+):$/gm)];
  return new Set(matches.map((match) => normalizePath(match[1])));
}

function readGoRoutes() {
  const server = readText("apps/control-plane/internal/api/server.go");
  const literals = [...server.matchAll(/r\.(?:Get|Post|Put|Patch|Delete)\("([^"]+)"/g)].map(
    (match) => match[1],
  );
  const routePrefixes = ["/api/v1/tasks", "/api/v1/runtime"];
  const fullPaths = new Set();

  for (const literal of literals) {
    if (literal === "/health") {
      fullPaths.add("/health");
      continue;
    }

    for (const prefix of routePrefixes) {
      const combined = `${prefix}${literal}`.replace(/\/$/, "");
      if (combined.includes("/api/v1/tasks/api/v1") || combined.includes("/api/v1/runtime/api/v1")) {
        continue;
      }
      if (
        (prefix.endsWith("/tasks") && literal.startsWith("/runtime")) ||
        (prefix.endsWith("/runtime") && literal === "/") ||
        (prefix.endsWith("/runtime") && !literal.startsWith("/nodes") && !literal.startsWith("/tasks") && literal !== "/register" && literal !== "/heartbeat")
      ) {
        continue;
      }
      fullPaths.add(normalizePath(combined));
    }
  }

  return fullPaths;
}

function readRustClientPaths() {
  const client = readText("apps/runtime-agent/src/controlplane/client.rs");
  const matches = [...client.matchAll(/\/api\/v1\/[A-Za-z0-9_{}?=&/.-]+/g)];
  return new Set(
    matches
      .map((match) => match[0].split("?")[0])
      .map((path) => path.replace(/\{\}/g, "{taskId}"))
      .map(normalizePath),
  );
}

function readTypeScriptClientPaths() {
  const files = [
    "packages/api-client/src/health.ts",
    "packages/api-client/src/tasks.ts",
    "packages/api-client/src/runtime.ts",
  ];
  const paths = new Set();

  for (const file of files) {
    const text = readText(file);
    for (const match of text.matchAll(/"((?:\/health|\/api\/v1)[^"]*)"/g)) {
      paths.add(normalizePath(match[1]));
    }
  }

  return paths;
}

const requiredOpenApiPaths = new Set([
  "/health",
  "/api/v1/tasks",
  "/api/v1/tasks/{taskId}",
  "/api/v1/tasks/{taskId}/status",
  "/api/v1/tasks/{taskId}/cancel",
  "/api/v1/runtime/register",
  "/api/v1/runtime/heartbeat",
  "/api/v1/runtime/tasks/claim",
  "/api/v1/runtime/tasks/{taskId}/events",
  "/api/v1/runtime/tasks/{taskId}/complete",
  "/api/v1/runtime/tasks/{taskId}/fail",
  "/api/v1/runtime/tasks/{taskId}/lease",
  "/api/v1/runtime/nodes",
  "/api/v1/runtime/nodes/{nodeId}",
]);

function assertSetContainsAll(label, actual, expected) {
  const missing = [...expected].filter((path) => !actual.has(path));
  if (missing.length > 0) {
    throw new Error(`${label} missing paths:\n${missing.map((path) => `- ${path}`).join("\n")}`);
  }
}

const openApiPaths = readOpenApiPaths();
const goRoutes = readGoRoutes();
const rustClientPaths = readRustClientPaths();
const tsClientPaths = readTypeScriptClientPaths();

assertSetContainsAll("Control Plane OpenAPI", openApiPaths, requiredOpenApiPaths);
assertSetContainsAll("Go route registration", goRoutes, requiredOpenApiPaths);
assertSetContainsAll("Rust Control Plane client", openApiPaths, rustClientPaths);
assertSetContainsAll("TypeScript api-client", openApiPaths, tsClientPaths);

console.log("foundation contract guard passed");
```

- [ ] **Step 4: Run the contract guard and inspect the failure**

Run:

```bash
pnpm verify:contracts
```

Expected first run after creating the script: it may FAIL if the route parser misses nested chi route groups. If it fails only because parser logic cannot infer existing routes, fix the parser in `scripts/verify-foundation-contracts.mjs`; do not change production routes for a parser bug.

- [ ] **Step 5: Make the guard pass**

Adjust only `scripts/verify-foundation-contracts.mjs` until this command passes:

```bash
pnpm verify:contracts
```

Expected: PASS and output:

```text
foundation contract guard passed
```

- [ ] **Step 6: Record the change**

Add this entry under `CHANGELOG.md` → `[Unreleased]` → `Added`:

```markdown
#### Foundation 契约漂移检查 (2026-05-30)

- 新增 `pnpm verify:contracts`，检查 Control Plane OpenAPI、Go route、Rust Control Plane client 和 TypeScript api-client 的关键路径一致性。
- 新增 `pnpm verify:foundation`，聚合契约检查、TypeScript 测试、TypeScript 类型检查和 Runtime Agent Rust 测试。
```

- [ ] **Step 7: Verify and commit**

Run:

```bash
pnpm verify:contracts
```

Expected: PASS.

Commit:

```bash
git add package.json scripts/verify-foundation-contracts.mjs CHANGELOG.md
git commit -m "test: add foundation contract guard"
```

---

## Task 2: Complete Minimal API Client Coverage

**Files:**
- Modify: `packages/api-client/src/tasks.ts`
- Modify: `packages/api-client/src/tasks.test.ts`
- Modify: `packages/api-client/src/runtime.ts`
- Modify: `packages/api-client/src/runtime.test.ts`
- Modify: `packages/api-client/src/index.ts`
- Modify: `CHANGELOG.md`

- [ ] **Step 1: Add failing tests for task detail, status, and cancel clients**

Append these tests to `packages/api-client/src/tasks.test.ts`:

```ts
describe("getTask", () => {
  it("calls the task detail endpoint and parses JSON", async () => {
    const task: TaskResponse = {
      id: 7,
      title: "Inspect foundation",
      status: "running",
      provider_type: "codex",
      priority: 5,
      params: {},
    };
    const fetcher = vi.fn(async () => new Response(JSON.stringify(task), { status: 200 }));

    await expect(
      getTask({ baseUrl: "http://control-plane.local/", fetcher }, 7),
    ).resolves.toEqual(task);

    expect(fetcher).toHaveBeenCalledWith("http://control-plane.local/api/v1/tasks/7", {
      headers: {
        accept: "application/json",
      },
      method: "GET",
    });
  });
});

describe("updateTaskStatus", () => {
  it("puts the new status to the task status endpoint", async () => {
    const task: TaskResponse = {
      id: 8,
      title: "Complete foundation",
      status: "completed",
      provider_type: "codex",
      priority: 5,
      params: {},
    };
    const fetcher = vi.fn(async () => new Response(JSON.stringify(task), { status: 200 }));

    await expect(
      updateTaskStatus(
        { baseUrl: "http://control-plane.local", fetcher },
        8,
        { status: "completed" },
      ),
    ).resolves.toEqual(task);

    expect(fetcher).toHaveBeenCalledWith("http://control-plane.local/api/v1/tasks/8/status", {
      body: JSON.stringify({ status: "completed" }),
      headers: {
        accept: "application/json",
        "content-type": "application/json",
      },
      method: "PUT",
    });
  });
});

describe("cancelTask", () => {
  it("posts to the task cancel endpoint and parses JSON", async () => {
    const task: TaskResponse = {
      id: 9,
      title: "Cancel foundation task",
      status: "cancelled",
      provider_type: "codex",
      priority: 5,
      params: {},
    };
    const fetcher = vi.fn(async () => new Response(JSON.stringify(task), { status: 200 }));

    await expect(
      cancelTask({ baseUrl: "http://control-plane.local/", fetcher }, 9),
    ).resolves.toEqual(task);

    expect(fetcher).toHaveBeenCalledWith("http://control-plane.local/api/v1/tasks/9/cancel", {
      headers: {
        accept: "application/json",
      },
      method: "POST",
    });
  });
});
```

Update the import at the top of the same file to:

```ts
import { cancelTask, createTask, getTask, listTasks, updateTaskStatus } from "./tasks";
```

- [ ] **Step 2: Run tests and verify they fail**

Run:

```bash
pnpm --filter @superteam/api-client test -- tasks.test.ts
```

Expected: FAIL because `getTask`, `updateTaskStatus`, and `cancelTask` are not exported from `./tasks`.

- [ ] **Step 3: Implement the task client methods**

Modify `packages/api-client/src/tasks.ts`:

```ts
export type UpdateTaskStatusInput = {
  status: TaskStatus;
};

export async function getTask(options: ApiClientOptions, taskId: number): Promise<TaskResponse> {
  const fetcher = options.fetcher ?? fetch;
  const response = await fetcher(buildApiUrl(options.baseUrl, `/api/v1/tasks/${taskId}`), {
    headers: {
      accept: "application/json",
    },
    method: "GET",
  });

  return parseJson<TaskResponse>(response, "task");
}

export async function updateTaskStatus(
  options: ApiClientOptions,
  taskId: number,
  input: UpdateTaskStatusInput,
): Promise<TaskResponse> {
  const fetcher = options.fetcher ?? fetch;
  const response = await fetcher(buildApiUrl(options.baseUrl, `/api/v1/tasks/${taskId}/status`), {
    body: JSON.stringify(input),
    headers: {
      accept: "application/json",
      "content-type": "application/json",
    },
    method: "PUT",
  });

  return parseJson<TaskResponse>(response, "task status");
}

export async function cancelTask(
  options: ApiClientOptions,
  taskId: number,
): Promise<TaskResponse> {
  const fetcher = options.fetcher ?? fetch;
  const response = await fetcher(buildApiUrl(options.baseUrl, `/api/v1/tasks/${taskId}/cancel`), {
    headers: {
      accept: "application/json",
    },
    method: "POST",
  });

  return parseJson<TaskResponse>(response, "task cancel");
}
```

Place the `UpdateTaskStatusInput` type near the other exported input types. Place the functions after `createTask`.

- [ ] **Step 4: Export the task methods**

Modify `packages/api-client/src/index.ts`:

```ts
export type { CreateTaskInput, TaskResponse, UpdateTaskStatusInput } from "./tasks";
export { cancelTask, createTask, getTask, listTasks, updateTaskStatus } from "./tasks";
```

Preserve the existing namespace export:

```ts
export * as tasks from "./tasks";
```

- [ ] **Step 5: Verify task client tests pass**

Run:

```bash
pnpm --filter @superteam/api-client test -- tasks.test.ts
```

Expected: PASS.

- [ ] **Step 6: Add failing tests for runtime node detail client**

Append this test to `packages/api-client/src/runtime.test.ts` and update the import to include `getRuntimeNode`:

```ts
import { getRuntimeNode, listRuntimeNodes } from "./runtime";
```

```ts
describe("getRuntimeNode", () => {
  it("calls the runtime node detail endpoint and parses JSON", async () => {
    const node: RuntimeNodeResponse = {
      node_id: "node-1",
      name: "developer-machine",
      supported_providers: ["codex"],
      status: "online",
      current_load: 0,
      max_slots: 2,
    };
    const fetcher = vi.fn(async () => new Response(JSON.stringify(node), { status: 200 }));

    await expect(
      getRuntimeNode({ baseUrl: "http://control-plane.local/", fetcher }, "node-1"),
    ).resolves.toEqual(node);

    expect(fetcher).toHaveBeenCalledWith(
      "http://control-plane.local/api/v1/runtime/nodes/node-1",
      {
        headers: {
          accept: "application/json",
        },
        method: "GET",
      },
    );
  });
});
```

- [ ] **Step 7: Run runtime client tests and verify they fail**

Run:

```bash
pnpm --filter @superteam/api-client test -- runtime.test.ts
```

Expected: FAIL because `getRuntimeNode` is not exported from `./runtime`.

- [ ] **Step 8: Implement the runtime node detail client**

Append this function to `packages/api-client/src/runtime.ts`:

```ts
export async function getRuntimeNode(
  options: ApiClientOptions,
  nodeId: string,
): Promise<RuntimeNodeResponse> {
  const fetcher = options.fetcher ?? fetch;
  const response = await fetcher(
    buildApiUrl(options.baseUrl, `/api/v1/runtime/nodes/${encodeURIComponent(nodeId)}`),
    {
      headers: {
        accept: "application/json",
      },
      method: "GET",
    },
  );

  return parseJson<RuntimeNodeResponse>(response, "runtime node");
}
```

- [ ] **Step 9: Export the runtime node detail client**

Modify `packages/api-client/src/index.ts`:

```ts
export { getRuntimeNode, listRuntimeNodes } from "./runtime";
```

Preserve the existing namespace export:

```ts
export * as runtime from "./runtime";
```

- [ ] **Step 10: Verify api-client package**

Run:

```bash
pnpm --filter @superteam/api-client test
pnpm --filter @superteam/api-client typecheck
```

Expected: PASS.

- [ ] **Step 11: Record the change**

Add this entry under `CHANGELOG.md` → `[Unreleased]` → `Added`:

```markdown
#### API Client 最小任务与 Runtime 覆盖 (2026-05-30)

- 为 `packages/api-client` 补齐任务详情、任务状态更新、任务取消和 Runtime 节点详情的最小 client 方法。
- 通过 Vitest 锁定这些方法使用的 Control Plane canonical path。
```

- [ ] **Step 12: Verify and commit**

Run:

```bash
pnpm --filter @superteam/api-client test
pnpm --filter @superteam/api-client typecheck
pnpm verify:contracts
```

Expected: PASS.

Commit:

```bash
git add packages/api-client/src/tasks.ts packages/api-client/src/tasks.test.ts packages/api-client/src/runtime.ts packages/api-client/src/runtime.test.ts packages/api-client/src/index.ts CHANGELOG.md
git commit -m "feat(api-client): cover foundation task and runtime endpoints"
```

---

## Task 3: Strengthen Core Summary Helpers

**Files:**
- Modify: `packages/core/src/task-summary.ts`
- Modify: `packages/core/src/task-summary.test.ts`
- Modify: `packages/core/src/runtime-node-summary.ts`
- Modify: `packages/core/src/runtime-node-summary.test.ts`
- Modify: `packages/core/src/index.ts`
- Modify: `CHANGELOG.md`

- [ ] **Step 1: Add failing tests for task summary tones**

Modify the first test in `packages/core/src/task-summary.test.ts` so the expected summary includes a tone:

```ts
expect(
  summarizeTask({
    id: 42,
    title: "Analyze requirements",
    status: "pending",
    provider_type: "codex",
  }),
).toEqual({
  id: "42",
  title: "Analyze requirements",
  status: "pending",
  providerLabel: "codex",
  statusTone: "warning",
});
```

Append this test:

```ts
it("maps known task statuses to stable tones", () => {
  expect(summarizeTask({ id: 1, title: "a", status: "completed" }).statusTone).toBe("success");
  expect(summarizeTask({ id: 2, title: "b", status: "failed" }).statusTone).toBe("danger");
  expect(summarizeTask({ id: 3, title: "c", status: "cancelled" }).statusTone).toBe("neutral");
  expect(summarizeTask({ id: 4, title: "d", status: "running" }).statusTone).toBe("info");
  expect(summarizeTask({ id: 5, title: "e", status: "claimed" }).statusTone).toBe("info");
  expect(summarizeTask({ id: 6, title: "f", status: "unknown-status" }).statusTone).toBe("neutral");
});
```

- [ ] **Step 2: Run task summary tests and verify they fail**

Run:

```bash
pnpm --filter @superteam/core test -- task-summary.test.ts
```

Expected: FAIL because `statusTone` does not exist.

- [ ] **Step 3: Implement task status tones**

Modify `packages/core/src/task-summary.ts`:

```ts
export type SummaryTone = "danger" | "info" | "neutral" | "success" | "warning";
```

Add `statusTone` to `TaskSummary`:

```ts
statusTone: SummaryTone;
```

Add this helper:

```ts
function taskStatusTone(status: string): SummaryTone {
  switch (status) {
    case "pending":
      return "warning";
    case "claimed":
    case "running":
      return "info";
    case "completed":
      return "success";
    case "failed":
      return "danger";
    case "cancelled":
      return "neutral";
    default:
      return "neutral";
  }
}
```

Update `summarizeTask`:

```ts
export function summarizeTask(raw: TaskSummaryInput): TaskSummary {
  return {
    id: String(raw.id),
    title: raw.title,
    status: raw.status,
    statusTone: taskStatusTone(raw.status),
    providerLabel: toLabel(raw.provider_type ?? raw.providerType, "unknown"),
  };
}
```

- [ ] **Step 4: Verify task summary tests pass**

Run:

```bash
pnpm --filter @superteam/core test -- task-summary.test.ts
```

Expected: PASS.

- [ ] **Step 5: Add failing tests for runtime summary tone and load percentage**

Modify expected runtime summaries in `packages/core/src/runtime-node-summary.test.ts` to include `statusTone` and `loadPercent`:

```ts
expect(
  summarizeRuntimeNode({
    node_id: "node-1",
    name: "builder",
    status: "online",
    current_load: 2,
    max_slots: 8,
  }),
).toEqual({
  id: "node-1",
  name: "builder",
  status: "online",
  statusTone: "success",
  loadLabel: "2/8",
  loadPercent: 25,
});
```

Append this test:

```ts
it("maps offline runtime nodes and avoids division by zero", () => {
  expect(
    summarizeRuntimeNode({
      node_id: "node-offline",
      name: "offline",
      status: "offline",
      current_load: 0,
      max_slots: 0,
    }),
  ).toEqual({
    id: "node-offline",
    name: "offline",
    status: "offline",
    statusTone: "neutral",
    loadLabel: "0/0",
    loadPercent: 0,
  });
});
```

- [ ] **Step 6: Run runtime summary tests and verify they fail**

Run:

```bash
pnpm --filter @superteam/core test -- runtime-node-summary.test.ts
```

Expected: FAIL because `statusTone` and `loadPercent` do not exist.

- [ ] **Step 7: Implement runtime summary fields**

Modify `packages/core/src/runtime-node-summary.ts`:

```ts
import type { SummaryTone } from "./task-summary";
```

Add fields to `RuntimeNodeSummary`:

```ts
statusTone: SummaryTone;
loadPercent: number;
```

Add helpers:

```ts
function runtimeStatusTone(status: string): SummaryTone {
  return status === "online" ? "success" : "neutral";
}

function loadPercent(currentLoad: number, maxSlots: number): number {
  if (maxSlots <= 0) {
    return 0;
  }
  return Math.min(100, Math.round((currentLoad / maxSlots) * 100));
}
```

Update `summarizeRuntimeNode`:

```ts
export function summarizeRuntimeNode(raw: RuntimeNodeSummaryInput): RuntimeNodeSummary {
  const currentLoad = toCount(raw.current_load ?? raw.currentLoad);
  const maxSlots = toCount(raw.max_slots ?? raw.maxSlots);

  return {
    id: String(raw.node_id ?? raw.nodeId),
    name: raw.name,
    status: raw.status,
    statusTone: runtimeStatusTone(raw.status),
    loadLabel: `${currentLoad}/${maxSlots}`,
    loadPercent: loadPercent(currentLoad, maxSlots),
  };
}
```

- [ ] **Step 8: Export the shared tone type**

Modify `packages/core/src/index.ts`:

```ts
export type { SummaryTone, TaskSummary, TaskSummaryInput } from "./task-summary";
```

- [ ] **Step 9: Verify core package**

Run:

```bash
pnpm --filter @superteam/core test
pnpm --filter @superteam/core typecheck
```

Expected: PASS.

- [ ] **Step 10: Record the change**

Add this entry under `CHANGELOG.md` → `[Unreleased]` → `Added`:

```markdown
#### Core Summary 状态映射 (2026-05-30)

- 为任务和 Runtime 节点 summary helper 增加稳定状态 tone，供后续 Web 页面复用。
- 为 Runtime 节点 summary 增加负载百分比，避免每个页面重复计算槽位占用。
```

- [ ] **Step 11: Verify and commit**

Run:

```bash
pnpm --filter @superteam/core test
pnpm --filter @superteam/core typecheck
pnpm -r --if-present typecheck
```

Expected: PASS.

Commit:

```bash
git add packages/core/src/task-summary.ts packages/core/src/task-summary.test.ts packages/core/src/runtime-node-summary.ts packages/core/src/runtime-node-summary.test.ts packages/core/src/index.ts CHANGELOG.md
git commit -m "feat(core): add reusable foundation summaries"
```

---

## Task 4: Document The Foundation Startup And Verification Path

**Files:**
- Modify: `README.md`
- Modify: `docs/development.md`
- Modify: `docs/api.md`
- Modify: `docs/NEXT_STEPS.md`
- Modify: `CHANGELOG.md`

- [ ] **Step 1: Inspect current docs before editing**

Run:

```bash
sed -n '1,220p' README.md
sed -n '1,260p' docs/development.md
sed -n '1,260p' docs/api.md
sed -n '1,220p' docs/NEXT_STEPS.md
```

Expected: identify references that imply stale foundation gaps or omit `pnpm verify:contracts` / `pnpm verify:foundation`.

- [ ] **Step 2: Update README foundation commands**

Modify `README.md` so the local command block includes:

```bash
pnpm install
pnpm verify:contracts
pnpm -r --if-present test
pnpm -r --if-present typecheck
go test ./apps/control-plane/...
cargo test --manifest-path apps/runtime-agent/Cargo.toml
pnpm verify:foundation
pnpm dev:control-plane
pnpm dev:web
cargo run --manifest-path apps/runtime-agent/Cargo.toml -- --config apps/runtime-agent/config.toml
```

Add a short note below the command block:

```markdown
`pnpm verify:foundation` intentionally excludes the full Go test suite because `apps/control-plane/internal/storage/queries` uses testcontainers and requires a working Docker provider. Run `go test ./apps/control-plane/...` when Docker/testcontainers is available; if it fails with `rootless Docker not found`, first verify non-Docker Go packages and then fix the local container runtime.
```

- [ ] **Step 3: Update development verification section**

In `docs/development.md`, add a subsection under the testing section:

```markdown
### Foundation 验证

```bash
pnpm verify:contracts
pnpm -r --if-present test
pnpm -r --if-present typecheck
cargo test --manifest-path apps/runtime-agent/Cargo.toml
pnpm verify:foundation
```

`pnpm verify:foundation` 覆盖契约漂移、前端测试、前端类型检查和 Runtime Agent Rust 测试。完整 Go 验证仍使用：

```bash
go test ./apps/control-plane/...
```

如果完整 Go 验证在 `github.com/superteam/control-plane/internal/storage/queries` 失败，并出现 `rootless Docker not found, failed to create Docker provider`，说明当前机器没有可用的 testcontainers Docker provider。此时先运行以下命令确认非 Docker Go 包基线：

```bash
go test \
  ./apps/control-plane/internal/api \
  ./apps/control-plane/internal/api/handlers \
  ./apps/control-plane/internal/app \
  ./apps/control-plane/internal/approval \
  ./apps/control-plane/internal/artifact \
  ./apps/control-plane/internal/audit \
  ./apps/control-plane/internal/auth \
  ./apps/control-plane/internal/config \
  ./apps/control-plane/internal/runtime \
  ./apps/control-plane/internal/storage \
  ./apps/control-plane/internal/task \
  ./apps/control-plane/internal/workflow
```

完整通过标准仍然是 Docker/testcontainers 可用后 `go test ./apps/control-plane/...` 通过。
```

Ensure nested code fences render correctly in the final markdown by using normal triple backticks in the file.

- [ ] **Step 4: Align API documentation with canonical runtime task paths**

In `docs/api.md`, ensure Runtime task lifecycle paths are documented as:

```markdown
- `POST /api/v1/runtime/tasks/claim`
- `POST /api/v1/runtime/tasks/{taskId}/events`
- `POST /api/v1/runtime/tasks/{taskId}/complete`
- `POST /api/v1/runtime/tasks/{taskId}/fail`
- `POST /api/v1/runtime/tasks/{taskId}/lease`
```

If the document references old `/api/v1/runtime/claim` or `/api/v1/tasks/{taskId}/events` Runtime write paths, replace them with the canonical paths above.

- [ ] **Step 5: Update next steps**

Modify `docs/NEXT_STEPS.md` so the first next step is:

```markdown
### 1. Foundation Readiness 收口

- 保持 `pnpm verify:contracts`、`pnpm verify:foundation`、Rust 测试和 Go 测试入口可用。
- 在进入任务中心、审批中心、数字员工管理等功能前，先确保 README、开发文档、API 文档和实际代码状态一致。
- 将 Web 新页面的数据访问收敛到 `packages/api-client` 和 `packages/core`，避免继续扩展 mock-only 页面结构。
```

Keep remaining next-step sections, but ensure they are clearly after foundation readiness.

- [ ] **Step 6: Record the documentation change**

Add this entry under `CHANGELOG.md` → `[Unreleased]` → `Changed`:

```markdown
#### Foundation Readiness 文档收口 (2026-05-30)

- 同步 README、开发指南、API 文档和下一步指引，明确底座阶段的启动、验证、契约守护和 testcontainers 环境边界。
```

- [ ] **Step 7: Verify documentation references**

Run:

```bash
rg -n "/api/v1/runtime/claim|/api/v1/tasks/\\{taskId\\}/events|verify:foundation|verify:contracts|rootless Docker" README.md docs/development.md docs/api.md docs/NEXT_STEPS.md
```

Expected:
- No match for `/api/v1/runtime/claim`.
- No Runtime write-path documentation pointing to `/api/v1/tasks/{taskId}/events`.
- Matches for `verify:foundation`, `verify:contracts`, and `rootless Docker`.

- [ ] **Step 8: Commit**

```bash
git add README.md docs/development.md docs/api.md docs/NEXT_STEPS.md CHANGELOG.md
git commit -m "docs: align foundation startup and verification"
```

---

## Task 5: Run Foundation Verification And Capture Boundaries

**Files:**
- Modify: `CHANGELOG.md` only if verification reveals a documented boundary needs clarification.

- [ ] **Step 1: Run contract guard**

Run:

```bash
pnpm verify:contracts
```

Expected: PASS with `foundation contract guard passed`.

- [ ] **Step 2: Run TypeScript tests**

Run:

```bash
pnpm -r --if-present test
```

Expected: PASS across `packages/ui`, `packages/api-client`, `packages/core`, `packages/views`, and `apps/web`.

- [ ] **Step 3: Run TypeScript typecheck**

Run:

```bash
pnpm -r --if-present typecheck
```

Expected: PASS.

- [ ] **Step 4: Run Runtime Agent tests**

Run:

```bash
cargo test --manifest-path apps/runtime-agent/Cargo.toml
```

Expected: PASS.

- [ ] **Step 5: Run non-Docker Go package baseline**

Run:

```bash
go test \
  ./apps/control-plane/internal/api \
  ./apps/control-plane/internal/api/handlers \
  ./apps/control-plane/internal/app \
  ./apps/control-plane/internal/approval \
  ./apps/control-plane/internal/artifact \
  ./apps/control-plane/internal/audit \
  ./apps/control-plane/internal/auth \
  ./apps/control-plane/internal/config \
  ./apps/control-plane/internal/runtime \
  ./apps/control-plane/internal/storage \
  ./apps/control-plane/internal/task \
  ./apps/control-plane/internal/workflow
```

Expected: PASS.

- [ ] **Step 6: Run full Go test and classify result**

Run:

```bash
go test ./apps/control-plane/...
```

Expected in a fully provisioned environment: PASS.

If it fails with:

```text
rootless Docker not found, failed to create Docker provider
```

classify it as an environment prerequisite failure for `internal/storage/queries`, not as a code regression. Do not mark full Go verification as passed in that case.

- [ ] **Step 7: Run aggregate foundation check**

Run:

```bash
pnpm verify:foundation
```

Expected: PASS. This command intentionally covers only checks that do not require Docker/testcontainers.

- [ ] **Step 8: Inspect working tree**

Run:

```bash
git status --short
```

Expected: clean after all committed tasks, or only intentional documentation clarification changes.

- [ ] **Step 9: Final completion audit**

Check each requirement from `docs/superpowers/specs/2026-05-30-foundation-readiness-design.md`:

```bash
rg -n "verify:contracts|verify:foundation|rootless Docker|/api/v1/runtime/tasks/claim|packages/api-client|packages/core" README.md docs/development.md docs/api.md docs/NEXT_STEPS.md CHANGELOG.md
pnpm verify:contracts
pnpm -r --if-present test
pnpm -r --if-present typecheck
cargo test --manifest-path apps/runtime-agent/Cargo.toml
```

Expected: docs contain the required boundaries and commands pass. Full Go completion remains conditional on Docker/testcontainers availability unless that environment has been fixed.

---

## Self-Review

Spec coverage:

- Engineering startup foundation: Task 4 documents the startup and verification path.
- Contract consistency foundation: Task 1 adds `verify:contracts`; Tasks 2 and 4 align api-client/docs with canonical paths.
- Minimal backend execution loop: Task 5 requires current e2e and route verification evidence; this plan does not add new product behavior because `apps/control-plane/internal/api/e2e_test.go` already covers the foundation lifecycle.
- Frontend real-data boundary: Tasks 2 and 3 add minimal api-client/core coverage without creating feature pages.
- Documentation and changelog: Tasks 1-4 update `CHANGELOG.md`; Task 4 updates README and docs.
- Non-goals: no task implements login, OpenFGA, Temporal, business pages, CI/CD, or production Provider governance.

Placeholder scan:

- This plan contains no placeholder markers and no step that asks for generic tests without concrete code or commands.

Type consistency:

- `TaskStatus`, `TaskResponse`, `RuntimeNodeResponse`, and `ApiClientOptions` match existing `packages/api-client` types.
- `SummaryTone` is introduced in `task-summary.ts` and reused in `runtime-node-summary.ts`.
- Contract paths use canonical `/api/v1/runtime/tasks/...` for Runtime writes.
