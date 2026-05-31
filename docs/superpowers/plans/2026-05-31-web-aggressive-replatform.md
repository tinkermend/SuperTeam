# Web Aggressive Replatform Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 将 SuperTeam Web 控制台从旧 Next.js + 前端 packages 结构重铺为基于 `shadcn-admin` 的 Vite 单应用，并保留真实 Control Plane 登录主链路。

**Architecture:** 先用测试锁住 Control Plane URL、API client 和认证行为，再机械替换 `apps/web` 为 `shadcn-admin` 壳，随后把 API client、认证状态、路由守卫和页面壳集中到 `apps/web/src`。最后删除不再服务 Web 的前端 packages，并同步 workspace、脚本、文档和验证门禁。

**Tech Stack:** Vite, React 19, TypeScript, TanStack Router, TanStack Query, shadcn/ui, Radix UI, Tailwind CSS 4, React Hook Form, Zod, Vitest, Go Control Plane.

**Spec:** `docs/superpowers/specs/2026-05-31-web-aggressive-replatform-design.md`

---

## File Structure

### Execution Isolation

- Use at execution time: `superpowers:using-git-worktrees`
  - Create an isolated branch such as `codex/web-aggressive-replatform`.
  - Keep the main workspace clean because this plan deletes and recreates large frontend directories.

### New Web App

- Replace directory: `apps/web/`
  - Source: `/Users/wangpei/src/github/Front/shadcn-admin`
  - Keep `shadcn-admin` layout, route, UI, hook, context, style, and test utility structure.
  - Remove sample business domains that do not belong to SuperTeam: Clerk routes, mock auth token store, apps, chats, sign-up, forgot-password, OTP, settings demo pages, sample dashboard analytics.
- Modify: `apps/web/package.json`
  - Rename package to `@superteam/web`.
  - Replace Next scripts with Vite scripts.
  - Remove workspace dependencies on `@superteam/*` frontend packages.
- Modify: `apps/web/.env.example`
  - Use `VITE_CONTROL_PLANE_URL=http://localhost:8080`.
- Modify: `apps/web/vite.config.ts`
  - Keep TanStack Router plugin, React plugin, Tailwind Vite plugin, `@` alias, and Vitest browser config.

### Web Config And API Boundary

- Create: `apps/web/src/lib/config/control-plane-url.ts`
  - Resolve Control Plane base URL from `import.meta.env.VITE_CONTROL_PLANE_URL`.
  - Preserve localhost host-alignment behavior from old `apps/web/src/control-plane-url.ts`.
- Create: `apps/web/src/lib/config/control-plane-url.test.ts`
  - Cover configured remote URL, configured local URL, derived local URL, and no-browser fallback.
- Create: `apps/web/src/lib/api/client.ts`
  - Shared `ApiRequestError`, `ApiClientOptions`, `buildApiUrl`, `parseJson`.
- Create: `apps/web/src/lib/api/auth.ts`
  - Move behavior from `packages/api-client/src/auth-api.ts`.
- Create: `apps/web/src/lib/api/health.ts`
  - Move behavior from `packages/api-client/src/health.ts`.
- Create: `apps/web/src/lib/api/tasks.ts`
  - Move behavior from `packages/api-client/src/tasks.ts`.
- Create: `apps/web/src/lib/api/runtime.ts`
  - Move behavior from `packages/api-client/src/runtime.ts`.
- Create: `apps/web/src/lib/api/index.ts`
  - Export API functions and types.
- Create tests beside API files:
  - `apps/web/src/lib/api/auth.test.ts`
  - `apps/web/src/lib/api/health.test.ts`
  - `apps/web/src/lib/api/tasks.test.ts`
  - `apps/web/src/lib/api/runtime.test.ts`

### Web Auth

- Create: `apps/web/src/features/auth/auth-context.tsx`
  - Owns auth context type and default error.
- Create: `apps/web/src/features/auth/auth-provider.tsx`
  - Loads `/api/auth/me`, handles 401 as logged-out state, provides login/logout actions.
- Create: `apps/web/src/features/auth/use-auth.ts`
  - Exposes auth context.
- Modify: `apps/web/src/features/auth/auth-layout.tsx`
  - Replace `Shadcn Admin` with `SuperTeam`.
- Replace: `apps/web/src/features/auth/sign-in/components/user-auth-form.tsx`
  - Rename semantics to username/password login.
  - Remove mock token, social login buttons, sign-up link, and forgot-password link.
- Replace test: `apps/web/src/features/auth/sign-in/components/user-auth-form.test.tsx`
  - Prove real login callback, validation, error display, and navigation to redirect/default path.
- Create: `apps/web/src/routes/(auth)/login.tsx`
  - Main SuperTeam login route.
- Delete or replace with redirect: `apps/web/src/routes/(auth)/sign-in.tsx`
  - If retained, it must only redirect to `/login`.
- Modify: `apps/web/src/main.tsx`
  - Remove Axios and `useAuthStore`.
  - Wrap router with `AuthProvider`.
  - Query 401 behavior navigates to `/login`.
- Modify: `apps/web/src/routes/_authenticated/route.tsx`
  - Enforce auth guard through authenticated layout.
- Modify: `apps/web/src/components/sign-out-dialog.tsx`
  - Call Control Plane logout and navigate to `/login`.
- Modify: `apps/web/src/components/layout/nav-user.tsx`
  - Display current user from `useAuth()`.

### SuperTeam Console Pages

- Modify: `apps/web/src/components/layout/data/sidebar-data.ts`
  - Replace demo nav with SuperTeam navigation.
- Modify: `apps/web/src/components/layout/app-sidebar.tsx`
  - Keep shadcn sidebar shell, use SuperTeam app identity, and feed current user into `NavUser`.
- Create: `apps/web/src/components/layout/app-title.tsx`
  - Static SuperTeam product identity for sidebar header.
- Create: `apps/web/src/features/dashboard/index.tsx`
  - Console entry with real current-user greeting and a small real API status panel.
- Create: `apps/web/src/features/users/index.tsx`
  - List users from real Control Plane API.
- Create: `apps/web/src/features/shared/unimplemented-page.tsx`
  - Common “not implemented yet” state for task, employee, workflow, capability, approval, audit, and runtime pages.
- Create route files:
  - `apps/web/src/routes/_authenticated/index.tsx`
  - `apps/web/src/routes/_authenticated/tasks/index.tsx`
  - `apps/web/src/routes/_authenticated/employees/index.tsx`
  - `apps/web/src/routes/_authenticated/workflows/index.tsx`
  - `apps/web/src/routes/_authenticated/capabilities/index.tsx`
  - `apps/web/src/routes/_authenticated/approvals/index.tsx`
  - `apps/web/src/routes/_authenticated/audit/index.tsx`
  - `apps/web/src/routes/_authenticated/runtime/index.tsx`
  - `apps/web/src/routes/_authenticated/users/index.tsx`

### Package And Verification Cleanup

- Delete directories:
  - `packages/ui`
  - `packages/views`
  - `packages/core`
  - `packages/api-client`
- Modify: `pnpm-workspace.yaml`
  - Remove `packages/*` from workspace packages.
- Modify: `package.json`
  - Remove old Next/Web root dev dependencies that are no longer used by any root script.
  - Keep root verification scripts aligned with the new Web package.
- Modify: `scripts/verify-foundation-contracts.mjs`
  - Read TypeScript API paths from `apps/web/src/lib/api/*.ts` instead of deleted `packages/api-client/src/*.ts`.
- Modify: `README.md`
  - Explain Web is now Vite + TanStack Router.
- Modify: `docs/development.md`
  - Update Web dev/test/build commands and env variable name.
- Modify: `CHANGELOG.md`
  - Add Simplified Chinese entry for Web aggressive replatform.

---

## Task 1: Create An Isolated Execution Workspace

**Files:**
- No repository files changed in this task.

- [ ] **Step 1: Start from a clean workspace**

Run:

```bash
git status --short --untracked-files=all
```

Expected: no output. If output appears, stop and inspect before continuing because this plan deletes large directories.

- [ ] **Step 2: Create an isolated branch or worktree**

Use `superpowers:using-git-worktrees` at execution time. If the skill creates a worktree, run all later commands from that worktree. If the skill chooses an in-place branch, use:

```bash
git switch -c codex/web-aggressive-replatform
```

Expected: branch name is `codex/web-aggressive-replatform`.

- [ ] **Step 3: Record the approved spec path**

Run:

```bash
test -f docs/superpowers/specs/2026-05-31-web-aggressive-replatform-design.md
```

Expected: command exits with status 0.

- [ ] **Step 4: Commit checkpoint**

No commit is required because no files changed.

## Task 2: Lock Control Plane URL Resolution Before Replatforming

**Files:**
- Create after scaffold replacement: `apps/web/src/lib/config/control-plane-url.ts`
- Create after scaffold replacement: `apps/web/src/lib/config/control-plane-url.test.ts`

- [ ] **Step 1: Preserve the old behavior as target code**

Use this exact implementation later when `apps/web/src/lib/config/control-plane-url.ts` is created:

```ts
const DEFAULT_CONTROL_PLANE_PORT = "8080";
const DEFAULT_CONTROL_PLANE_URL = `http://localhost:${DEFAULT_CONTROL_PLANE_PORT}`;

export function resolveControlPlaneUrl(configuredUrl = import.meta.env.VITE_CONTROL_PLANE_URL?.trim()) {
  if (typeof window === "undefined") {
    return configuredUrl || DEFAULT_CONTROL_PLANE_URL;
  }

  if (configuredUrl) {
    return resolveBrowserControlPlaneUrl(configuredUrl);
  }

  return `${window.location.protocol}//${window.location.hostname}:${DEFAULT_CONTROL_PLANE_PORT}`;
}

function resolveBrowserControlPlaneUrl(configuredUrl: string) {
  let parsedUrl: URL;
  try {
    parsedUrl = new URL(configuredUrl);
  } catch {
    return configuredUrl;
  }

  if (isLocalHost(parsedUrl.hostname) && isLocalHost(window.location.hostname)) {
    parsedUrl.hostname = window.location.hostname;
    return trimTrailingSlash(parsedUrl.toString());
  }

  return trimTrailingSlash(configuredUrl);
}

function isLocalHost(hostname: string) {
  const normalizedHostname = hostname.replace(/^\[/, "").replace(/\]$/, "");
  return normalizedHostname === "localhost" || normalizedHostname === "127.0.0.1" || normalizedHostname === "::1";
}

function trimTrailingSlash(url: string) {
  return url.replace(/\/+$/, "");
}
```

- [ ] **Step 2: Preserve the target test code**

Use this exact test later when `apps/web/src/lib/config/control-plane-url.test.ts` is created:

```ts
import { afterEach, describe, expect, it, vi } from "vitest";
import { resolveControlPlaneUrl } from "./control-plane-url";

describe("resolveControlPlaneUrl", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
    vi.unstubAllEnvs();
  });

  it("uses the configured remote control plane URL when provided", () => {
    vi.stubEnv("VITE_CONTROL_PLANE_URL", "http://control-plane.local:8080");
    vi.stubGlobal("window", {
      location: {
        hostname: "127.0.0.1",
        protocol: "http:",
      },
    });

    expect(resolveControlPlaneUrl()).toBe("http://control-plane.local:8080");
  });

  it("keeps a configured local API URL on the current browser host", () => {
    vi.stubEnv("VITE_CONTROL_PLANE_URL", "http://localhost:8080");
    vi.stubGlobal("window", {
      location: {
        hostname: "127.0.0.1",
        protocol: "http:",
      },
    });

    expect(resolveControlPlaneUrl()).toBe("http://127.0.0.1:8080");
  });

  it("derives the local API URL from the current browser host", () => {
    vi.stubEnv("VITE_CONTROL_PLANE_URL", "");
    vi.stubGlobal("window", {
      location: {
        hostname: "127.0.0.1",
        protocol: "http:",
      },
    });

    expect(resolveControlPlaneUrl()).toBe("http://127.0.0.1:8080");
  });

  it("falls back to localhost when no browser location is available", () => {
    vi.stubEnv("VITE_CONTROL_PLANE_URL", "");
    vi.stubGlobal("window", undefined);

    expect(resolveControlPlaneUrl()).toBe("http://localhost:8080");
  });
});
```

- [ ] **Step 3: Do not run tests yet**

The files do not exist until the scaffold is copied in Task 3. This task is a behavior snapshot so the implementation agent does not have to rediscover the old Next-specific code after deletion.

- [ ] **Step 4: Commit checkpoint**

No commit is required because no files changed.

## Task 3: Replace `apps/web` With The shadcn-admin Vite Scaffold

**Files:**
- Replace directory: `apps/web/`
- Modify generated by copy: `apps/web/package.json`
- Modify generated by copy: `apps/web/.env.example`
- Modify generated by copy: `apps/web/index.html`

- [ ] **Step 1: Remove old Web app directory**

Run:

```bash
rm -rf apps/web
mkdir -p apps/web
```

Expected: `apps/web` exists and is empty.

- [ ] **Step 2: Copy scaffold without upstream repository metadata**

Run:

```bash
rsync -a \
  --exclude '.git' \
  --exclude 'node_modules' \
  --exclude 'pnpm-lock.yaml' \
  --exclude '.github' \
  --exclude '.vscode' \
  --exclude 'netlify.toml' \
  /Users/wangpei/src/github/Front/shadcn-admin/ apps/web/
```

Expected: `apps/web/src/main.tsx`, `apps/web/vite.config.ts`, and `apps/web/components.json` exist.

- [ ] **Step 3: Replace `apps/web/package.json`**

Set `apps/web/package.json` to this content:

```json
{
  "name": "@superteam/web",
  "private": true,
  "version": "0.0.0",
  "type": "module",
  "scripts": {
    "dev": "vite --host 127.0.0.1 --port 3000",
    "build": "tsc -b && vite build",
    "preview": "vite preview --host 127.0.0.1 --port 3000",
    "test": "vitest run --browser.headless",
    "test:watch": "vitest --browser.headless",
    "test:browser:install": "playwright install chromium",
    "typecheck": "tsc -b --pretty false"
  },
  "dependencies": {
    "@hookform/resolvers": "^5.2.2",
    "@radix-ui/react-alert-dialog": "^1.1.15",
    "@radix-ui/react-avatar": "^1.1.11",
    "@radix-ui/react-checkbox": "^1.3.3",
    "@radix-ui/react-collapsible": "^1.1.12",
    "@radix-ui/react-dialog": "^1.1.15",
    "@radix-ui/react-direction": "^1.1.1",
    "@radix-ui/react-dropdown-menu": "^2.1.16",
    "@radix-ui/react-icons": "^1.3.2",
    "@radix-ui/react-label": "^2.1.8",
    "@radix-ui/react-popover": "^1.1.15",
    "@radix-ui/react-radio-group": "^1.3.8",
    "@radix-ui/react-scroll-area": "^1.2.10",
    "@radix-ui/react-select": "^2.2.6",
    "@radix-ui/react-separator": "^1.1.8",
    "@radix-ui/react-slot": "^1.2.4",
    "@radix-ui/react-switch": "^1.2.6",
    "@radix-ui/react-tabs": "^1.1.13",
    "@radix-ui/react-tooltip": "^1.2.8",
    "@tailwindcss/vite": "^4.2.2",
    "@tanstack/react-query": "^5.99.0",
    "@tanstack/react-router": "^1.168.22",
    "@tanstack/react-table": "^8.21.3",
    "class-variance-authority": "^0.7.1",
    "clsx": "^2.1.1",
    "cmdk": "1.1.1",
    "date-fns": "^4.1.0",
    "input-otp": "^1.4.2",
    "lucide-react": "^1.8.0",
    "react": "19.2.6",
    "react-day-picker": "9.14.0",
    "react-dom": "19.2.6",
    "react-hook-form": "^7.72.1",
    "react-top-loading-bar": "^3.0.2",
    "recharts": "^3.8.1",
    "sonner": "^2.0.7",
    "tailwind-merge": "^3.5.0",
    "tailwindcss": "^4.2.2",
    "tw-animate-css": "^1.4.0",
    "zod": "^4.3.6"
  },
  "devDependencies": {
    "@tanstack/eslint-plugin-query": "^5.99.0",
    "@tanstack/react-query-devtools": "^5.99.0",
    "@tanstack/react-router-devtools": "^1.166.13",
    "@tanstack/router-plugin": "^1.167.22",
    "@types/node": "^25.6.0",
    "@types/react": "^19.2.15",
    "@types/react-dom": "^19.2.3",
    "@vitejs/plugin-react": "^6.0.1",
    "@vitest/browser-playwright": "^4.1.4",
    "@vitest/coverage-v8": "^4.1.4",
    "@vitest/ui": "^4.1.4",
    "playwright": "1.59.1",
    "typescript": "6.0.3",
    "vite": "^8.0.8",
    "vitest": "^4.1.7",
    "vitest-browser-react": "^2.2.0"
  }
}
```

- [ ] **Step 4: Replace Web env example**

Set `apps/web/.env.example` to:

```dotenv
# Web Console 环境变量配置
# 复制此文件为 .env.local 并填入实际值

# Control Plane API 地址（浏览器访问）
VITE_CONTROL_PLANE_URL=http://localhost:8080
```

- [ ] **Step 5: Run install to refresh dependency graph**

Run:

```bash
pnpm install
```

Expected: completes without package resolution errors.

- [ ] **Step 6: Run the copied app build and capture expected failures**

Run:

```bash
pnpm --filter @superteam/web typecheck
```

Expected: this may fail because copied `shadcn-admin` still contains sample auth and routes. Record the first TypeScript error for reference, then continue to Task 4.

- [ ] **Step 7: Commit scaffold replacement**

Run:

```bash
git add apps/web package.json pnpm-lock.yaml
git commit -m "feat(web): replace app with shadcn admin scaffold"
```

Expected: commit succeeds. If `pnpm-lock.yaml` did not change, omit it from `git add`.

## Task 4: Add Vite Control Plane Config Module

**Files:**
- Create: `apps/web/src/lib/config/control-plane-url.ts`
- Create: `apps/web/src/lib/config/control-plane-url.test.ts`
- Modify: `apps/web/src/vite-env.d.ts`

- [ ] **Step 1: Create failing config test**

Create `apps/web/src/lib/config/control-plane-url.test.ts` using the exact test code from Task 2 Step 2.

- [ ] **Step 2: Run the focused test and verify it fails**

Run:

```bash
pnpm --filter @superteam/web test -- src/lib/config/control-plane-url.test.ts
```

Expected: FAIL because `./control-plane-url` does not exist.

- [ ] **Step 3: Create config implementation**

Create `apps/web/src/lib/config/control-plane-url.ts` using the exact implementation from Task 2 Step 1.

- [ ] **Step 4: Add Vite env typing**

Ensure `apps/web/src/vite-env.d.ts` includes:

```ts
/// <reference types="vite/client" />

interface ImportMetaEnv {
  readonly VITE_CONTROL_PLANE_URL?: string;
}

interface ImportMeta {
  readonly env: ImportMetaEnv;
}
```

- [ ] **Step 5: Run focused config test**

Run:

```bash
pnpm --filter @superteam/web test -- src/lib/config/control-plane-url.test.ts
```

Expected: PASS.

- [ ] **Step 6: Commit config module**

Run:

```bash
git add apps/web/src/lib/config/control-plane-url.ts apps/web/src/lib/config/control-plane-url.test.ts apps/web/src/vite-env.d.ts apps/web/.env.example
git commit -m "feat(web): add control plane url config"
```

## Task 5: Internalize The TypeScript API Client

**Files:**
- Create: `apps/web/src/lib/api/client.ts`
- Create: `apps/web/src/lib/api/auth.ts`
- Create: `apps/web/src/lib/api/auth.test.ts`
- Create: `apps/web/src/lib/api/health.ts`
- Create: `apps/web/src/lib/api/health.test.ts`
- Create: `apps/web/src/lib/api/tasks.ts`
- Create: `apps/web/src/lib/api/tasks.test.ts`
- Create: `apps/web/src/lib/api/runtime.ts`
- Create: `apps/web/src/lib/api/runtime.test.ts`
- Create: `apps/web/src/lib/api/index.ts`

- [ ] **Step 1: Create shared API client primitives**

Create `apps/web/src/lib/api/client.ts`:

```ts
export type ApiClientOptions = {
  baseUrl: string;
  fetcher?: typeof fetch;
};

export class ApiRequestError extends Error {
  readonly status: number;

  constructor(resource: string, status: number) {
    super(`${resource} request failed with status ${status}`);
    this.name = "ApiRequestError";
    this.status = status;
  }
}

export function buildApiUrl(baseUrl: string, path: string): string {
  return `${baseUrl.replace(/\/+$/, "")}${path}`;
}

export async function parseJson<T>(response: Response, resource: string): Promise<T> {
  if (!response.ok) {
    throw new ApiRequestError(resource, response.status);
  }

  return (await response.json()) as T;
}
```

- [ ] **Step 2: Move auth API client and update imports**

Copy the behavior from `packages/api-client/src/auth-api.ts` into `apps/web/src/lib/api/auth.ts`.

Use these import changes at the top:

```ts
import type { ApiClientOptions } from "./client";
import { ApiRequestError, buildApiUrl, parseJson } from "./client";
```

Keep these exported names:

```ts
export type UserSummary = {
  id: number;
  status: "active" | "disabled";
  username: string;
};

export type LoginRequest = {
  password: string;
  username: string;
};

export type LoginResponse = {
  user: UserSummary;
};

export type CurrentUserResponse = {
  user: UserSummary;
};
```

All auth and user-management requests must include:

```ts
credentials: "include",
```

- [ ] **Step 3: Move health API client and update imports**

Copy `packages/api-client/src/health.ts` into `apps/web/src/lib/api/health.ts`, then replace the local `ApiClientOptions` definition with:

```ts
import type { ApiClientOptions } from "./client";
```

Keep exported type:

```ts
export type HealthResponse = {
  status: "ok";
  service: "control-plane";
};
```

- [ ] **Step 4: Move task API client and include cookies**

Copy `packages/api-client/src/tasks.ts` into `apps/web/src/lib/api/tasks.ts`, then replace the import with:

```ts
import type { ApiClientOptions } from "./client";
import { buildApiUrl, parseJson } from "./client";
```

For every task request, include:

```ts
credentials: "include",
```

Expected endpoint paths remain:

```text
/api/v1/tasks
/api/v1/tasks/{taskId}
/api/v1/tasks/{taskId}/status
/api/v1/tasks/{taskId}/cancel
```

- [ ] **Step 5: Move runtime API client and include cookies**

Copy `packages/api-client/src/runtime.ts` into `apps/web/src/lib/api/runtime.ts`, then replace the import with:

```ts
import type { ApiClientOptions } from "./client";
import { buildApiUrl, parseJson } from "./client";
```

For every runtime node request, include:

```ts
credentials: "include",
```

Expected endpoint paths remain:

```text
/api/v1/runtime/nodes
/api/v1/runtime/nodes/{nodeId}
```

- [ ] **Step 6: Add API barrel exports**

Create `apps/web/src/lib/api/index.ts`:

```ts
export * from "./auth";
export * from "./client";
export * from "./health";
export * from "./runtime";
export * from "./tasks";
```

- [ ] **Step 7: Move and update API tests**

Copy the existing tests from `packages/api-client/src/*.test.ts` into matching `apps/web/src/lib/api/*.test.ts`.

Update import paths:

```ts
import { getHealth } from "./health";
import { listTasks } from "./tasks";
import { listRuntimeNodes } from "./runtime";
import { login } from "./auth";
```

Update task and runtime expectations to include `credentials: "include"` in the expected fetch options.

- [ ] **Step 8: Run API client tests**

Run:

```bash
pnpm --filter @superteam/web test -- src/lib/api
```

Expected: PASS.

- [ ] **Step 9: Commit API client internalization**

Run:

```bash
git add apps/web/src/lib/api
git commit -m "feat(web): internalize control plane api client"
```

## Task 6: Replace Mock Auth With Real Cookie Session Auth

**Files:**
- Create: `apps/web/src/features/auth/auth-context.tsx`
- Create: `apps/web/src/features/auth/auth-provider.tsx`
- Create: `apps/web/src/features/auth/auth-provider.test.tsx`
- Create: `apps/web/src/features/auth/use-auth.ts`
- Modify: `apps/web/src/features/auth/auth-layout.tsx`
- Modify: `apps/web/src/features/auth/sign-in/components/user-auth-form.tsx`
- Modify: `apps/web/src/features/auth/sign-in/components/user-auth-form.test.tsx`
- Delete: `apps/web/src/stores/auth-store.ts`
- Delete: `apps/web/src/stores/auth-store.test.ts`

- [ ] **Step 1: Create auth context**

Create `apps/web/src/features/auth/auth-context.tsx`:

```tsx
import { createContext } from "react";
import type { UserSummary } from "@/lib/api";

export type AuthContextValue = {
  isAuthenticated: boolean;
  isLoading: boolean;
  login: (credentials: { password: string; username: string }) => Promise<void>;
  logout: () => Promise<void>;
  refreshCurrentUser: (options?: { showLoading?: boolean }) => Promise<void>;
  user: UserSummary | null;
};

export const AuthContext = createContext<AuthContextValue | null>(null);
```

- [ ] **Step 2: Create auth hook**

Create `apps/web/src/features/auth/use-auth.ts`:

```ts
import { useContext } from "react";
import { AuthContext } from "./auth-context";

export function useAuth() {
  const value = useContext(AuthContext);
  if (!value) {
    throw new Error("useAuth must be used within AuthProvider");
  }
  return value;
}
```

- [ ] **Step 3: Create auth provider**

Create `apps/web/src/features/auth/auth-provider.tsx`:

```tsx
import type { ReactNode } from "react";
import { useCallback, useEffect, useMemo, useState } from "react";
import type { ApiClientOptions, UserSummary } from "@/lib/api";
import { ApiRequestError, getCurrentUser, login as loginRequest, logout as logoutRequest } from "@/lib/api";
import { AuthContext } from "./auth-context";

type AuthProviderProps = {
  apiBaseUrl: string;
  children: ReactNode;
  fetcher?: ApiClientOptions["fetcher"];
};

export function AuthProvider({ apiBaseUrl, children, fetcher }: AuthProviderProps) {
  const [user, setUser] = useState<UserSummary | null>(null);
  const [isLoading, setIsLoading] = useState(true);

  const refreshCurrentUser = useCallback(
    async (options?: { showLoading?: boolean }) => {
      if (options?.showLoading ?? true) {
        setIsLoading(true);
      }

      try {
        const response = await getCurrentUser({ baseUrl: apiBaseUrl, fetcher });
        setUser(response.user);
      } catch (error) {
        if (error instanceof ApiRequestError && error.status === 401) {
          setUser(null);
          return;
        }
        throw error;
      } finally {
        if (options?.showLoading ?? true) {
          setIsLoading(false);
        }
      }
    },
    [apiBaseUrl, fetcher],
  );

  useEffect(() => {
    let isMounted = true;

    async function loadInitialUser() {
      try {
        const response = await getCurrentUser({ baseUrl: apiBaseUrl, fetcher });
        if (isMounted) {
          setUser(response.user);
        }
      } catch {
        if (isMounted) {
          setUser(null);
        }
      } finally {
        if (isMounted) {
          setIsLoading(false);
        }
      }
    }

    void loadInitialUser();

    return () => {
      isMounted = false;
    };
  }, [apiBaseUrl, fetcher]);

  useEffect(() => {
    function refreshSession() {
      void refreshCurrentUser({ showLoading: false });
    }

    window.addEventListener("focus", refreshSession);
    return () => window.removeEventListener("focus", refreshSession);
  }, [refreshCurrentUser]);

  const login = useCallback(
    async (credentials: { password: string; username: string }) => {
      const response = await loginRequest({ baseUrl: apiBaseUrl, fetcher }, credentials);
      setUser(response.user);
    },
    [apiBaseUrl, fetcher],
  );

  const logout = useCallback(async () => {
    try {
      await logoutRequest({ baseUrl: apiBaseUrl, fetcher });
    } finally {
      setUser(null);
    }
  }, [apiBaseUrl, fetcher]);

  const value = useMemo(
    () => ({
      isAuthenticated: Boolean(user),
      isLoading,
      login,
      logout,
      refreshCurrentUser,
      user,
    }),
    [isLoading, login, logout, refreshCurrentUser, user],
  );

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}
```

- [ ] **Step 4: Add auth provider test**

Create `apps/web/src/features/auth/auth-provider.test.tsx`:

```tsx
import { act, render, screen, waitFor } from "vitest-browser-react";
import { describe, expect, it, vi } from "vitest";
import { AuthProvider } from "./auth-provider";
import { useAuth } from "./use-auth";

function AuthProbe() {
  const { isAuthenticated, isLoading, user } = useAuth();

  return (
    <div>
      <span data-testid="loading">{String(isLoading)}</span>
      <span data-testid="authenticated">{String(isAuthenticated)}</span>
      <span data-testid="username">{user?.username ?? "none"}</span>
    </div>
  );
}

describe("AuthProvider", () => {
  it("clears the current user when a focused session refresh receives unauthorized", async () => {
    const fetcher = vi
      .fn()
      .mockResolvedValueOnce(
        new Response(
          JSON.stringify({
            user: {
              id: 1,
              status: "active",
              username: "admin",
            },
          }),
          { headers: { "content-type": "application/json" }, status: 200 },
        ),
      )
      .mockResolvedValueOnce(new Response(JSON.stringify({ error: "unauthorized" }), { status: 401 }));

    await render(
      <AuthProvider apiBaseUrl="http://localhost:8080" fetcher={fetcher}>
        <AuthProbe />
      </AuthProvider>,
    );

    await expect.element(screen.getByText("admin")).toBeInTheDocument();

    await act(async () => {
      window.dispatchEvent(new Event("focus"));
    });

    await waitFor(() => {
      expect(screen.getByTestId("authenticated").element().textContent).toBe("false");
    });
    expect(screen.getByTestId("username").element().textContent).toBe("none");
  });
});
```

- [ ] **Step 5: Replace auth layout branding**

In `apps/web/src/features/auth/auth-layout.tsx`, replace product text with:

```tsx
<h1 className="text-xl font-medium">SuperTeam</h1>
```

Keep the surrounding layout from `shadcn-admin`.

- [ ] **Step 6: Replace login form behavior**

In `apps/web/src/features/auth/sign-in/components/user-auth-form.tsx`:

- Replace `email` field with `username`.
- Remove `useAuthStore`, `sleep`, mock user, mock token, social login buttons, sign-up link, and forgot-password link.
- Use `const { login } = useAuth()`.
- On submit call `await login({ username: data.username, password: data.password })`.
- On success navigate to `redirectTo || "/"`.
- On failure show form-level text `用户名或密码不正确`.

The schema must be:

```ts
const formSchema = z.object({
  username: z.string().min(1, "请输入用户名。"),
  password: z.string().min(1, "请输入密码。"),
});
```

- [ ] **Step 7: Replace login form tests**

Update `apps/web/src/features/auth/sign-in/components/user-auth-form.test.tsx` so it verifies:

```ts
const login = vi.fn();
const navigate = vi.fn();

vi.mock("@/features/auth/use-auth", () => ({
  useAuth: () => ({ login }),
}));
```

Test cases:

- Empty submit shows `请输入用户名。` and `请输入密码。`.
- Successful submit calls `login({ username: "admin", password: "admin" })`.
- Successful submit navigates to `{ to: "/", replace: true }`.
- Failed submit renders `用户名或密码不正确`.

- [ ] **Step 8: Delete mock auth store**

Run:

```bash
rm -f apps/web/src/stores/auth-store.ts apps/web/src/stores/auth-store.test.ts
```

- [ ] **Step 9: Run auth tests**

Run:

```bash
pnpm --filter @superteam/web test -- src/features/auth
```

Expected: PASS.

- [ ] **Step 10: Commit real auth foundation**

Run:

```bash
git add apps/web/src/features/auth apps/web/src/stores
git commit -m "feat(web): replace mock auth with control plane session auth"
```

## Task 7: Wire Auth Provider Into The Router And Route Guard

**Files:**
- Modify: `apps/web/src/main.tsx`
- Modify: `apps/web/src/routes/__root.tsx`
- Modify: `apps/web/src/routes/(auth)/login.tsx`
- Modify or delete: `apps/web/src/routes/(auth)/sign-in.tsx`
- Modify: `apps/web/src/routes/_authenticated/route.tsx`
- Modify: `apps/web/src/components/sign-out-dialog.tsx`
- Modify: `apps/web/src/components/sign-out-dialog.test.tsx`

- [ ] **Step 1: Update `main.tsx`**

Remove imports for `AxiosError`, `useAuthStore`, and `handleServerError`.

Add imports:

```ts
import { AuthProvider } from "@/features/auth/auth-provider";
import { ApiRequestError } from "@/lib/api";
import { resolveControlPlaneUrl } from "@/lib/config/control-plane-url";
```

In QueryClient query `retry`, use:

```ts
retry: (failureCount, error) => {
  if (error instanceof ApiRequestError && [401, 403].includes(error.status)) {
    return false;
  }
  return failureCount < 2;
},
```

In `queryCache.onError`, use:

```ts
if (error instanceof ApiRequestError && error.status === 401) {
  const redirect = router.history.location.href;
  void router.navigate({ to: "/login", search: { redirect } });
}
if (error instanceof ApiRequestError && error.status === 500 && import.meta.env.PROD) {
  void router.navigate({ to: "/500" });
}
```

Wrap `RouterProvider` with:

```tsx
<AuthProvider apiBaseUrl={resolveControlPlaneUrl()}>
  <RouterProvider router={router} />
</AuthProvider>
```

- [ ] **Step 2: Create `/login` route**

Create `apps/web/src/routes/(auth)/login.tsx`:

```tsx
import { createFileRoute } from "@tanstack/react-router";
import { SignIn } from "@/features/auth/sign-in";

export const Route = createFileRoute("/(auth)/login")({
  validateSearch: (search: Record<string, unknown>) => ({
    redirect: typeof search.redirect === "string" ? search.redirect : undefined,
  }),
  component: SignIn,
});
```

- [ ] **Step 3: Update sign-in feature search source**

In `apps/web/src/features/auth/sign-in/index.tsx`, replace:

```ts
const { redirect } = useSearch({ from: "/(auth)/sign-in" });
```

with:

```ts
const { redirect } = useSearch({ from: "/(auth)/login" });
```

Remove sign-up, terms, and privacy copy from the rendered card. The card description should say:

```tsx
<CardDescription>使用 Control Plane 账号登录 SuperTeam 控制台。</CardDescription>
```

- [ ] **Step 4: Replace `/sign-in` route with redirect or delete it**

Preferred: delete `apps/web/src/routes/(auth)/sign-in.tsx`.

If route generation fails because existing links still reference `/sign-in`, create this temporary redirect file instead:

```tsx
import { createFileRoute, redirect } from "@tanstack/react-router";

export const Route = createFileRoute("/(auth)/sign-in")({
  beforeLoad: () => {
    throw redirect({ to: "/login" });
  },
});
```

- [ ] **Step 5: Guard authenticated layout**

In `apps/web/src/routes/_authenticated/route.tsx`, keep the route component as `AuthenticatedLayout`.

In `apps/web/src/components/layout/authenticated-layout.tsx`, add:

```tsx
import { Navigate, Outlet, useLocation } from "@tanstack/react-router";
import { useAuth } from "@/features/auth/use-auth";
```

At the top of the component body:

```tsx
const { isAuthenticated, isLoading } = useAuth();
const location = useLocation();

if (isLoading) {
  return <div className="flex h-svh items-center justify-center text-sm text-muted-foreground">加载中...</div>;
}

if (!isAuthenticated) {
  return <Navigate to="/login" search={{ redirect: location.href }} replace />;
}
```

Then keep the existing sidebar layout.

- [ ] **Step 6: Update sign out dialog**

In `apps/web/src/components/sign-out-dialog.tsx`, replace `useAuthStore` with:

```ts
import { useAuth } from "@/features/auth/use-auth";
```

Use:

```ts
const { logout } = useAuth();

const handleSignOut = async () => {
  await logout();
  const currentPath = location.href;
  navigate({
    to: "/login",
    search: { redirect: currentPath },
    replace: true,
  });
};
```

- [ ] **Step 7: Update sign out test**

In `apps/web/src/components/sign-out-dialog.test.tsx`, mock `useAuth`:

```ts
const logout = vi.fn();

vi.mock("@/features/auth/use-auth", () => ({
  useAuth: () => ({ logout }),
}));
```

Expected assertion:

```ts
expect(logout).toHaveBeenCalledOnce();
expect(navigate).toHaveBeenCalledWith({
  to: "/login",
  search: { redirect: MOCK_HREF },
  replace: true,
});
```

- [ ] **Step 8: Run router/auth tests**

Run:

```bash
pnpm --filter @superteam/web test -- src/components/sign-out-dialog.test.tsx src/features/auth
pnpm --filter @superteam/web typecheck
```

Expected: PASS.

- [ ] **Step 9: Commit router auth wiring**

Run:

```bash
git add apps/web/src/main.tsx apps/web/src/routes apps/web/src/components/sign-out-dialog.tsx apps/web/src/components/sign-out-dialog.test.tsx apps/web/src/components/layout/authenticated-layout.tsx apps/web/src/features/auth
git commit -m "feat(web): wire session auth into router"
```

## Task 8: Replace Demo Navigation With SuperTeam Console Navigation

**Files:**
- Create: `apps/web/src/components/layout/app-title.tsx`
- Modify: `apps/web/src/components/layout/app-sidebar.tsx`
- Modify: `apps/web/src/components/layout/data/sidebar-data.ts`
- Modify: `apps/web/src/components/layout/types.ts`
- Modify: `apps/web/src/components/layout/nav-user.tsx`

- [ ] **Step 1: Create app title**

Create `apps/web/src/components/layout/app-title.tsx`:

```tsx
import { Command } from "lucide-react";
import {
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
} from "@/components/ui/sidebar";

export function AppTitle() {
  return (
    <SidebarMenu>
      <SidebarMenuItem>
        <SidebarMenuButton size="lg" asChild>
          <a href="/">
            <div className="flex aspect-square size-8 items-center justify-center rounded-lg bg-sidebar-primary text-sidebar-primary-foreground">
              <Command />
            </div>
            <div className="grid flex-1 text-start text-sm leading-tight">
              <span className="truncate font-semibold">SuperTeam</span>
              <span className="truncate text-xs">数字员工控制平面</span>
            </div>
          </a>
        </SidebarMenuButton>
      </SidebarMenuItem>
    </SidebarMenu>
  );
}
```

- [ ] **Step 2: Replace sidebar static data**

Set `apps/web/src/components/layout/data/sidebar-data.ts` to:

```ts
import {
  Bot,
  ClipboardList,
  FileClock,
  GitBranch,
  LayoutDashboard,
  Puzzle,
  Server,
  ShieldCheck,
  Users,
} from "lucide-react";
import { type SidebarData } from "../types";

export const sidebarData: SidebarData = {
  navGroups: [
    {
      title: "控制台",
      items: [
        {
          title: "工作台",
          url: "/",
          icon: LayoutDashboard,
        },
        {
          title: "任务中心",
          url: "/tasks",
          icon: ClipboardList,
        },
        {
          title: "数字员工",
          url: "/employees",
          icon: Bot,
        },
        {
          title: "流程编排",
          url: "/workflows",
          icon: GitBranch,
        },
        {
          title: "外部能力",
          url: "/capabilities",
          icon: Puzzle,
        },
        {
          title: "审批中心",
          url: "/approvals",
          icon: ShieldCheck,
        },
        {
          title: "Runtime 节点",
          url: "/runtime",
          icon: Server,
        },
        {
          title: "用户管理",
          url: "/users",
          icon: Users,
        },
        {
          title: "审计日志",
          url: "/audit",
          icon: FileClock,
        },
      ],
    },
  ],
};
```

- [ ] **Step 3: Update layout types**

In `apps/web/src/components/layout/types.ts`, remove `User`, `Team`, and `teams` from `SidebarData`. Keep:

```ts
type SidebarData = {
  navGroups: NavGroup[];
};
```

- [ ] **Step 4: Use AppTitle in sidebar**

In `apps/web/src/components/layout/app-sidebar.tsx`, remove `TeamSwitcher` import and use:

```tsx
import { AppTitle } from "./app-title";
```

In `SidebarHeader`, render:

```tsx
<AppTitle />
```

In `SidebarFooter`, render:

```tsx
<NavUser />
```

- [ ] **Step 5: Make NavUser read auth state**

Change `apps/web/src/components/layout/nav-user.tsx` so `NavUser` takes no props and reads:

```ts
import { useAuth } from "@/features/auth/use-auth";
```

Inside component:

```ts
const { user } = useAuth();
const displayName = user?.username ?? "未登录";
const displayEmail = user?.status === "active" ? "active" : "disabled";
const fallback = displayName.slice(0, 2).toUpperCase();
```

Remove menu items for upgrade, billing, and notifications. Keep only account label and destructive sign-out item.

- [ ] **Step 6: Run typecheck**

Run:

```bash
pnpm --filter @superteam/web typecheck
```

Expected: PASS or only failures from remaining demo routes removed in Task 9.

- [ ] **Step 7: Commit navigation replacement**

Run:

```bash
git add apps/web/src/components/layout
git commit -m "feat(web): add superteam console navigation"
```

## Task 9: Replace Demo Routes With SuperTeam Pages

**Files:**
- Create: `apps/web/src/features/shared/unimplemented-page.tsx`
- Replace: `apps/web/src/features/dashboard/index.tsx`
- Replace: `apps/web/src/features/users/index.tsx`
- Create route files under `apps/web/src/routes/_authenticated/`
- Delete demo feature directories and routes that are not SuperTeam domains.

- [ ] **Step 1: Create shared unimplemented page**

Create `apps/web/src/features/shared/unimplemented-page.tsx`:

```tsx
import type { LucideIcon } from "lucide-react";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Header } from "@/components/layout/header";
import { Main } from "@/components/layout/main";
import { Search } from "@/components/search";
import { ThemeSwitch } from "@/components/theme-switch";

type UnimplementedPageProps = {
  description: string;
  icon: LucideIcon;
  title: string;
};

export function UnimplementedPage({ description, icon: Icon, title }: UnimplementedPageProps) {
  return (
    <>
      <Header>
        <Search />
        <ThemeSwitch />
      </Header>
      <Main>
        <div className="mb-4 flex items-center gap-3">
          <div className="flex size-10 items-center justify-center rounded-md border bg-muted">
            <Icon />
          </div>
          <div>
            <h1 className="text-2xl font-bold tracking-tight">{title}</h1>
            <p className="text-sm text-muted-foreground">{description}</p>
          </div>
        </div>
        <Card>
          <CardHeader>
            <CardTitle>功能建设中</CardTitle>
            <CardDescription>当前页面只保留导航入口，不使用 mock 数据冒充真实业务能力。</CardDescription>
          </CardHeader>
          <CardContent className="text-sm text-muted-foreground">
            后续实现会从 Control Plane API 获取真实数据，并按任务、审批、审计和工件边界逐步接入。
          </CardContent>
        </Card>
      </Main>
    </>
  );
}
```

- [ ] **Step 2: Replace dashboard with SuperTeam entry**

Set `apps/web/src/features/dashboard/index.tsx` to:

```tsx
import { useQuery } from "@tanstack/react-query";
import { Activity, ShieldCheck, Users } from "lucide-react";
import { useAuth } from "@/features/auth/use-auth";
import { getHealth, listLoginLogs } from "@/lib/api";
import { resolveControlPlaneUrl } from "@/lib/config/control-plane-url";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Header } from "@/components/layout/header";
import { Main } from "@/components/layout/main";
import { Search } from "@/components/search";
import { ThemeSwitch } from "@/components/theme-switch";

const apiBaseUrl = resolveControlPlaneUrl();

export function Dashboard() {
  const { user } = useAuth();
  const healthQuery = useQuery({
    queryKey: ["control-plane-health"],
    queryFn: () => getHealth({ baseUrl: apiBaseUrl }),
  });
  const loginLogsQuery = useQuery({
    queryKey: ["login-logs", 5],
    queryFn: () => listLoginLogs({ baseUrl: apiBaseUrl, limit: 5, offset: 0 }),
  });

  return (
    <>
      <Header>
        <Search />
        <ThemeSwitch />
      </Header>
      <Main>
        <div className="mb-4">
          <h1 className="text-2xl font-bold tracking-tight">工作台</h1>
          <p className="text-sm text-muted-foreground">欢迎回来，{user?.username ?? "用户"}。</p>
        </div>
        <div className="grid gap-4 md:grid-cols-3">
          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2 text-base">
                <Activity /> Control Plane
              </CardTitle>
              <CardDescription>后端健康状态</CardDescription>
            </CardHeader>
            <CardContent>
              <Badge variant={healthQuery.data?.status === "ok" ? "default" : "secondary"}>
                {healthQuery.isLoading ? "检查中" : healthQuery.data?.status ?? "不可用"}
              </Badge>
            </CardContent>
          </Card>
          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2 text-base">
                <Users /> 当前用户
              </CardTitle>
              <CardDescription>来自 /api/auth/me</CardDescription>
            </CardHeader>
            <CardContent className="text-sm">
              {user?.username ?? "未登录"}
            </CardContent>
          </Card>
          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2 text-base">
                <ShieldCheck /> 登录日志
              </CardTitle>
              <CardDescription>来自 /api/auth/login-logs</CardDescription>
            </CardHeader>
            <CardContent className="text-sm">
              {loginLogsQuery.isLoading ? "加载中" : `${loginLogsQuery.data?.items.length ?? 0} 条最近记录`}
            </CardContent>
          </Card>
        </div>
      </Main>
    </>
  );
}
```

- [ ] **Step 3: Replace users page with real list**

Set `apps/web/src/features/users/index.tsx` to:

```tsx
import { useQuery } from "@tanstack/react-query";
import { listUsers } from "@/lib/api";
import { resolveControlPlaneUrl } from "@/lib/config/control-plane-url";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Header } from "@/components/layout/header";
import { Main } from "@/components/layout/main";
import { Search } from "@/components/search";
import { ThemeSwitch } from "@/components/theme-switch";

const apiBaseUrl = resolveControlPlaneUrl();

export function Users() {
  const usersQuery = useQuery({
    queryKey: ["users"],
    queryFn: () => listUsers({ baseUrl: apiBaseUrl, limit: 50, offset: 0 }),
  });

  return (
    <>
      <Header>
        <Search />
        <ThemeSwitch />
      </Header>
      <Main>
        <div className="mb-4">
          <h1 className="text-2xl font-bold tracking-tight">用户管理</h1>
          <p className="text-sm text-muted-foreground">展示 Control Plane 返回的真实用户列表。</p>
        </div>
        <Card>
          <CardHeader>
            <CardTitle>用户列表</CardTitle>
            <CardDescription>当前阶段只读展示，写操作后续按权限和审计要求接入。</CardDescription>
          </CardHeader>
          <CardContent>
            {usersQuery.isLoading ? (
              <p className="text-sm text-muted-foreground">加载中...</p>
            ) : usersQuery.isError ? (
              <p className="text-sm text-destructive">用户列表加载失败。</p>
            ) : (
              <div className="divide-y rounded-md border">
                {(usersQuery.data?.items ?? []).map((user) => (
                  <div key={user.id} className="flex items-center justify-between p-3 text-sm">
                    <span className="font-medium">{user.username}</span>
                    <Badge variant={user.status === "active" ? "default" : "secondary"}>{user.status}</Badge>
                  </div>
                ))}
              </div>
            )}
          </CardContent>
        </Card>
      </Main>
    </>
  );
}
```

- [ ] **Step 4: Create route files**

Keep `apps/web/src/routes/_authenticated/index.tsx`:

```tsx
import { createFileRoute } from "@tanstack/react-router";
import { Dashboard } from "@/features/dashboard";

export const Route = createFileRoute("/_authenticated/")({
  component: Dashboard,
});
```

Create `apps/web/src/routes/_authenticated/users/index.tsx`:

```tsx
import { createFileRoute } from "@tanstack/react-router";
import { Users } from "@/features/users";

export const Route = createFileRoute("/_authenticated/users/")({
  component: Users,
});
```

For the other route files, use this pattern with the listed icon/title/description:

```tsx
import { createFileRoute } from "@tanstack/react-router";
import { ClipboardList } from "lucide-react";
import { UnimplementedPage } from "@/features/shared/unimplemented-page";

export const Route = createFileRoute("/_authenticated/tasks/")({
  component: () => (
    <UnimplementedPage
      icon={ClipboardList}
      title="任务中心"
      description="围绕任务输入、输出、上下文、风险和验收状态组织执行链路。"
    />
  ),
});
```

Use these route-specific values:

```text
tasks: ClipboardList, 任务中心, 围绕任务输入、输出、上下文、风险和验收状态组织执行链路。
employees: Bot, 数字员工, 管理数字员工定义、技能绑定、权限边界和上下文策略。
workflows: GitBranch, 流程编排, 编排需求分析、人类确认、Runtime 执行、工件回传和验收流程。
capabilities: Puzzle, 外部能力, 注册和审计 Dify、Deephub、HTTP 服务和企业内部系统能力。
approvals: ShieldCheck, 审批中心, 承载高风险动作、需求歧义和上线发布等人类决策。
runtime: Server, Runtime 节点, 查看 Runtime Agent 节点、Provider 支持能力、负载和心跳状态。
audit: FileClock, 审计日志, 查询平台关键操作、登录事件、风险和执行结果。
```

- [ ] **Step 5: Delete non-SuperTeam demo routes and features**

Run:

```bash
rm -rf \
  apps/web/src/routes/clerk \
  apps/web/src/routes/_authenticated/apps \
  apps/web/src/routes/_authenticated/chats \
  apps/web/src/routes/_authenticated/help-center \
  apps/web/src/routes/_authenticated/settings \
  apps/web/src/routes/\(auth\)/sign-up.tsx \
  apps/web/src/routes/\(auth\)/forgot-password.tsx \
  apps/web/src/routes/\(auth\)/otp.tsx \
  apps/web/src/routes/\(auth\)/sign-in-2.tsx \
  apps/web/src/features/apps \
  apps/web/src/features/chats \
  apps/web/src/features/settings \
  apps/web/src/features/auth/forgot-password \
  apps/web/src/features/auth/otp \
  apps/web/src/features/auth/sign-up \
  apps/web/src/assets/clerk-logo.tsx \
  apps/web/src/assets/clerk-full-logo.tsx \
  apps/web/src/assets/logo.tsx \
  apps/web/src/assets/brand-icons
```

Keep `apps/web/src/routes/(errors)` and `apps/web/src/features/errors`.

Why these extras: `sign-up.tsx`, `forgot-password.tsx`, `otp.tsx`, `sign-in-2.tsx` are scaffold auth routes that would be auto-registered by TanStack Router file-system generator. Their feature directories (`forgot-password/`, `otp/`, `sign-up/`) will have broken imports after Task 6 deletes the mock `auth-store.ts`. `logo.tsx` and `brand-icons/` are no longer imported after Task 6 Step 5 removes the `<Logo>` component and social login buttons from the auth layout and form.

- [ ] **Step 6: Run route generation through typecheck**

Run:

```bash
pnpm --filter @superteam/web typecheck
```

Expected: PASS. If TanStack route tree generation changes `apps/web/src/routeTree.gen.ts`, include it in the commit.

- [ ] **Step 7: Run Web tests**

Run:

```bash
pnpm --filter @superteam/web test
```

Expected: PASS.

- [ ] **Step 8: Commit SuperTeam pages**

Run:

```bash
git add apps/web/src
git commit -m "feat(web): replace demo pages with superteam console"
```

## Task 10: Remove Frontend Workspace Packages

**Files:**
- Delete: `packages/ui`
- Delete: `packages/views`
- Delete: `packages/core`
- Delete: `packages/api-client`
- Modify: `pnpm-workspace.yaml`
- Modify: `package.json`
- Modify: `scripts/verify-foundation-contracts.mjs`

- [ ] **Step 1: Confirm Web has no workspace package imports**

Run:

```bash
rg '@superteam/(ui|views|core|api-client)|packages/' apps/web package.json pnpm-workspace.yaml scripts
```

Expected: matches still exist in root scripts, workspace config, and contract guard before this task. There must be no matches inside `apps/web/src`.

- [ ] **Step 2: Delete frontend packages**

Run:

```bash
rm -rf packages/ui packages/views packages/core packages/api-client
```

- [ ] **Step 3: Update workspace packages**

Set `pnpm-workspace.yaml` to:

```yaml
packages:
  - "apps/*"
```

- [ ] **Step 4: Update root `package.json` scripts**

In root `package.json`, set the scripts block to include:

```json
{
  "dev:web": "pnpm --filter @superteam/web dev",
  "build:web": "pnpm --filter @superteam/web build",
  "dev:control-plane": "go run ./apps/control-plane/cmd/control-plane",
  "dev:runtime-agent": "cargo run --manifest-path apps/runtime-agent/Cargo.toml -- --config apps/runtime-agent/config.example.toml --once",
  "generate:control-plane": "cd apps/control-plane && go generate ./internal/api",
  "verify:contracts": "node scripts/verify-foundation-contracts.mjs",
  "verify:foundation": "pnpm verify:contracts && pnpm test:ts && pnpm typecheck && pnpm test:go && pnpm test:rust",
  "verify:web": "pnpm --filter @superteam/web test && pnpm --filter @superteam/web typecheck && pnpm --filter @superteam/web build",
  "verify:control-plane": "pnpm verify:contracts && pnpm test:go",
  "verify:runtime-agent": "pnpm verify:contracts && pnpm test:rust",
  "verify:db": "pnpm test:go",
  "test": "pnpm test:ts && pnpm test:go && pnpm test:rust",
  "test:ts": "pnpm -r --if-present test",
  "test:go": "go test ./apps/control-plane/...",
  "test:rust": "cargo test --manifest-path apps/runtime-agent/Cargo.toml",
  "typecheck": "pnpm -r --if-present typecheck"
}
```

Set root `devDependencies` to this minimal object, because Web-specific dependencies now live in `apps/web/package.json`:

```json
{
  "devDependencies": {}
}
```

If `pnpm install` later proves a root script needs a Node package, add only that package back with a comment in the commit message explaining which script uses it.

- [ ] **Step 5: Update contract guard TypeScript file list**

In `scripts/verify-foundation-contracts.mjs`, replace:

```js
const files = [
  "packages/api-client/src/health.ts",
  "packages/api-client/src/tasks.ts",
  "packages/api-client/src/runtime.ts",
];
```

with:

```js
const files = [
  "apps/web/src/lib/api/health.ts",
  "apps/web/src/lib/api/tasks.ts",
  "apps/web/src/lib/api/runtime.ts",
];
```

- [ ] **Step 6: Refresh lockfile**

Run:

```bash
pnpm install
```

Expected: completes and removes deleted workspace packages from lockfile.

- [ ] **Step 7: Verify no package references remain**

Run:

```bash
rg '@superteam/(ui|views|core|api-client)|packages/' apps package.json pnpm-workspace.yaml scripts docs README.md CHANGELOG.md
```

Expected: no matches that describe active frontend package dependencies. Historical changelog entries may remain; do not edit old changelog history just to remove old names.

- [ ] **Step 8: Run core checks**

Run:

```bash
pnpm verify:contracts
pnpm --filter @superteam/web typecheck
pnpm --filter @superteam/web test
```

Expected: PASS.

- [ ] **Step 9: Commit package removal**

Run:

```bash
git add -A packages pnpm-workspace.yaml package.json pnpm-lock.yaml scripts/verify-foundation-contracts.mjs
git commit -m "refactor(web): remove frontend workspace packages"
```

## Task 11: Update Documentation And Changelog

**Files:**
- Modify: `README.md`
- Modify: `docs/development.md`
- Modify: `CHANGELOG.md`

- [ ] **Step 1: Update README Web wording**

In `README.md`, replace the first paragraph with:

```md
SuperTeam 是企业级数字员工控制平面。第一阶段优先建立外部 Web 控制台、Control Plane、Runtime Agent 和契约基线。Web 控制台当前使用 Vite + TanStack Router + shadcn/ui，后端仍由 Go Control Plane 提供 API。
```

If README contains old Next.js commands, replace them with:

```bash
pnpm dev:web
pnpm build:web
pnpm verify:web
```

- [ ] **Step 2: Update development docs**

In `docs/development.md`, make the Web section say:

```md
### Web 控制台

Web 控制台位于 `apps/web`，使用 Vite、React、TanStack Router、TanStack Query 和 shadcn/ui。

本地启动：

```bash
pnpm dev:web
```

浏览器环境变量使用 `VITE_CONTROL_PLANE_URL` 指向 Control Plane，例如：

```dotenv
VITE_CONTROL_PLANE_URL=http://localhost:8080
```

验证：

```bash
pnpm verify:web
```
```

- [ ] **Step 3: Add changelog entry**

Under `CHANGELOG.md` → `[Unreleased]` → `### Changed`, add:

```md
- Web 控制台从旧 Next.js + 前端 workspace packages 结构激进重铺为 Vite + TanStack Router + shadcn-admin 单应用结构；前端 API client、认证状态、页面和 UI 组件集中到 `apps/web/src`，后端 Control Plane API 契约保持不变。
```

Under `CHANGELOG.md` → `[Unreleased]` → `### Added`, add:

```md
#### Web Vite 控制台重铺 (2026-05-31)

- 新 Web 壳接入 `shadcn-admin` 的侧边栏、顶部栏、主题、命令面板和响应式布局。
- 保留真实 Control Plane 登录、当前用户、退出登录和路由保护主链路，继续使用 cookie session 与 `credentials: "include"`。
- 新增 Vite 环境变量 `VITE_CONTROL_PLANE_URL`，保留本地 `localhost` / `127.0.0.1` host 对齐策略。
- 删除不再服务 Web 的 `packages/ui`、`packages/views`、`packages/core` 和 `packages/api-client` 前端拆分。
```

- [ ] **Step 4: Run docs grep**

Run:

```bash
rg 'Next.js|NEXT_PUBLIC_CONTROL_PLANE_URL|@superteam/(ui|views|core|api-client)' README.md docs/development.md docs/superpowers/specs/2026-05-31-web-aggressive-replatform-design.md
```

Expected: only historical/spec context matches remain. Active development docs must not tell developers to use Next.js or `NEXT_PUBLIC_CONTROL_PLANE_URL`.

- [ ] **Step 5: Commit docs update**

Run:

```bash
git add README.md docs/development.md CHANGELOG.md
git commit -m "docs: update web replatform guidance"
```

## Task 12: Full Verification And Browser Smoke

**Files:**
- No planned source edits unless verification finds defects.

- [ ] **Step 1: Run Web browser dependency install if needed**

Run:

```bash
pnpm --filter @superteam/web test:browser:install
```

Expected: Playwright Chromium is installed or already present.

- [ ] **Step 2: Run Web verification**

Run:

```bash
pnpm verify:web
```

Expected: Web tests, typecheck, and build pass.

- [ ] **Step 3: Run contract verification**

Run:

```bash
pnpm verify:contracts
```

Expected: `foundation contract guard passed`.

- [ ] **Step 4: Run backend tests**

Run:

```bash
go test ./apps/control-plane/...
```

Expected: PASS or documented skips for explicitly gated database/Redis integration tests.

- [ ] **Step 5: Run runtime tests**

Run:

```bash
cargo test --manifest-path apps/runtime-agent/Cargo.toml
```

Expected: PASS.

- [ ] **Step 6: Start Control Plane**

In one terminal:

```bash
go run ./apps/control-plane/cmd/control-plane
```

Expected: Control Plane listens on `127.0.0.1:8080` or the configured address.

- [ ] **Step 7: Start Web**

In another terminal:

```bash
pnpm dev:web
```

Expected: Vite serves Web at `http://127.0.0.1:3000`.

- [ ] **Step 8: Browser smoke test**

Open `http://127.0.0.1:3000` in the in-app browser and verify:

```text
1. Unauthenticated visit redirects or renders /login.
2. Wrong credentials show 用户名或密码不正确.
3. admin / admin logs in successfully.
4. Refresh keeps the session by calling /api/auth/me.
5. Dashboard shows Control Plane health or a clear backend error state.
6. User menu shows current username.
7. Sign out returns to /login.
8. Visiting /users after sign out is blocked.
```

- [ ] **Step 9: Stop dev servers**

Stop the Control Plane and Vite dev server with `Ctrl-C`. Ensure no long-running sessions remain.

- [ ] **Step 10: Commit verification fixes**

If verification required fixes, commit them:

```bash
git add -A
git commit -m "fix(web): pass replatform verification"
```

If no fixes were needed, skip this commit.

## Task 13: Final Review Before Merge

**Files:**
- No planned source edits unless review finds defects.

- [ ] **Step 1: Inspect full diff**

Run:

```bash
git diff --stat main...HEAD
git diff --name-status main...HEAD
```

Expected: diff includes Web replacement, frontend package deletion, script/docs updates, and changelog.

- [ ] **Step 2: Check no ignored sensitive doc was staged**

Run:

```bash
git status --short --untracked-files=all
git check-ignore -v doc/private-notes.md || true
```

Expected: `doc/private-notes.md` is ignored by `.gitignore`; no sensitive `doc/` files are staged.

- [ ] **Step 3: Check old frontend packages are gone**

Run:

```bash
test ! -d packages/ui
test ! -d packages/views
test ! -d packages/core
test ! -d packages/api-client
```

Expected: all commands exit with status 0.

- [ ] **Step 4: Check Web does not import deleted packages**

Run:

```bash
rg '@superteam/(ui|views|core|api-client)' apps/web
```

Expected: no output.

- [ ] **Step 5: Final verification bundle**

Run:

```bash
pnpm verify:contracts
pnpm verify:web
go test ./apps/control-plane/...
cargo test --manifest-path apps/runtime-agent/Cargo.toml
```

Expected: all pass, except explicitly documented environment-gated database/Redis tests may skip.

- [ ] **Step 6: Final commit if needed**

If any final review changes were made:

```bash
git add -A
git commit -m "chore(web): finalize replatform cleanup"
```

If no changes were made, skip this commit.

---

## Self-Review Checklist

- Spec coverage:
  - Web becomes Vite + TanStack Router + shadcn-admin: Tasks 3, 7, 8, 9.
  - Control Plane API contract remains unchanged: Tasks 5, 10, 12.
  - API client, auth, UI, pages move into `apps/web/src`: Tasks 5, 6, 8, 9.
  - `packages/*` frontend packages are removed: Task 10.
  - Real login/current user/logout/route guard are preserved: Tasks 5, 6, 7, 12.
  - At least one logged-in page calls real Control Plane API: Task 9 dashboard and users page.
  - Docs and CHANGELOG update: Task 11.
  - Verification and browser smoke: Task 12 and Task 13.
- Placeholder scan:
  - No unresolved placeholder markers or vague “fill later” steps.
  - Every code-writing step names exact files and provides either exact code or exact source file to copy plus required edits.
- Type consistency:
  - Auth route is `/login`.
  - Env var is `VITE_CONTROL_PLANE_URL`.
  - API imports come from `@/lib/api`.
  - Auth hook imports come from `@/features/auth/use-auth`.
