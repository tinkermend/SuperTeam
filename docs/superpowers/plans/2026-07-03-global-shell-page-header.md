# Global Shell Page Header Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Move SuperTeam page titles, subtitles, icons, and page-level actions from main content headers into the global shell top bar through one reusable component style.

**Architecture:** Keep `Header` as the global shell control surface and add an explicit page-title mode. Use `ShellPageHeader` as the page-facing wrapper so feature pages do not know about `Header showSidebarTrigger={false}` or top-bar sizing. Keep `V3PageHeader` as the single visual page-header primitive, with `variant="page"` for legacy content placement and `variant="shell"` for the global top bar.

**Tech Stack:** React, TanStack Router, TypeScript, Tailwind classes backed by `--v3-*` tokens, Vitest browser tests through `corepack pnpm --filter ./apps/web run test`.

---

## Current State

- `apps/web/src/components/layout/header.tsx` now supports `showSidebarTrigger?: boolean`.
- `apps/web/src/components/superteam/v3-components.tsx` now supports `V3PageHeader variant="shell"`.
- `apps/web/src/components/layout/shell-page-header.tsx` now wraps `Header showSidebarTrigger={false}` and `V3PageHeader variant="shell"`.
- `apps/web/src/features/workflows/components/workflow-shell.tsx` is the first migrated vertical slice.
- Targeted tests are green for `src/components/layout/header.test.tsx` and `src/features/workflows/index.test.tsx`.

## File Structure

- `apps/web/src/components/layout/header.tsx` owns the global shell grid, search placement, top-right controls, and optional sidebar trigger.
- `apps/web/src/components/layout/shell-page-header.tsx` is the migration-safe page API for top-bar page headers.
- `apps/web/src/components/superteam/v3-components.tsx` owns `V3PageHeader` visual variants.
- `apps/web/src/features/**` pages should import `ShellPageHeader` instead of rendering `Header` + `Search` + `ThemeSwitch` + content `V3PageHeader`.
- `apps/web/src/**/*.test.tsx` mocks of `@/components/layout/header` must understand `showSidebarTrigger` when a test asserts shell header placement.

---

### Task 1: Lock The Shared Shell Header Contract

**Files:**
- Modify: `apps/web/src/components/layout/header.tsx`
- Modify: `apps/web/src/components/layout/header.test.tsx`
- Modify: `apps/web/src/components/superteam/v3-components.tsx`
- Create: `apps/web/src/components/layout/shell-page-header.tsx`

- [ ] **Step 1: Verify the red tests cover the contract**

Run:

```bash
corepack pnpm --filter ./apps/web run test -- src/components/layout/header.test.tsx
```

Expected before implementation: `can replace the sidebar trigger with page-level header content` fails because `showSidebarTrigger` is passed to the DOM and children are not rendered.

- [ ] **Step 2: Keep the public contract explicit**

`HeaderProps` must include:

```tsx
type HeaderProps = React.HTMLAttributes<HTMLElement> & {
  fixed?: boolean;
  showSidebarTrigger?: boolean;
  ref?: React.Ref<HTMLElement>;
};
```

`Header` must default `showSidebarTrigger` to `true`, render the existing `SidebarTrigger` in default mode, and render `children` in the left slot when `showSidebarTrigger={false}`.

- [ ] **Step 3: Keep default Header behavior stable**

Default mode must retain these classes so existing pages do not shift:

```tsx
"relative grid h-full grid-cols-[auto_minmax(0,1fr)_auto] items-center gap-3 px-4 py-2 sm:px-5 lg:grid-cols-[minmax(14rem,1fr)_minmax(16rem,42rem)_minmax(14rem,1fr)]"
```

Page-title mode must use a three-column layout:

```tsx
"relative grid h-full grid-cols-[minmax(0,1fr)_auto_auto] items-center gap-3 px-4 py-2 sm:px-5 md:grid-cols-[minmax(0,1fr)_minmax(12rem,28rem)_auto] lg:grid-cols-[minmax(0,1fr)_minmax(16rem,30rem)_auto]"
```

- [ ] **Step 4: Keep search placement deterministic**

Default mode search remains centered and wide:

```tsx
<Search className="mx-auto h-10 w-full max-w-2xl rounded-full border-[var(--v3-shell-search-border)] bg-[var(--v3-shell-search)] px-4 [box-shadow:var(--v3-shell-search-shadow)] backdrop-blur-xl sm:w-full md:w-full lg:w-full xl:w-full" />
```

Page-title mode search must contain:

```tsx
"max-w-sm justify-self-center md:w-64 lg:w-72 xl:w-80 max-md:size-10 max-md:flex-none max-md:justify-center max-md:px-0 max-md:[&>span]:sr-only max-md:[&_svg]:left-1/2 max-md:[&_svg]:-translate-x-1/2"
```

- [ ] **Step 5: Add the page-facing wrapper**

Create `ShellPageHeader`:

```tsx
import type { ComponentProps } from "react";
import { Header } from "@/components/layout/header";
import { V3PageHeader } from "@/components/superteam";

type ShellPageHeaderProps = Omit<ComponentProps<typeof V3PageHeader>, "variant">;

export function ShellPageHeader(props: ShellPageHeaderProps) {
  return (
    <Header showSidebarTrigger={false}>
      <V3PageHeader {...props} variant="shell" />
    </Header>
  );
}
```

- [ ] **Step 6: Verify**

Run:

```bash
corepack pnpm --filter ./apps/web run test -- src/components/layout/header.test.tsx
```

Expected: all `Header` tests pass.

---

### Task 2: Migrate Workflow Orchestration As The Vertical Slice

**Files:**
- Modify: `apps/web/src/features/workflows/components/workflow-shell.tsx`
- Modify: `apps/web/src/features/workflows/index.test.tsx`

- [ ] **Step 1: Keep the existing failing workflow test**

The behavior test should assert:

```tsx
const heading = screen.getByRole("heading", { name: "流程编排" }).element() as HTMLElement;
const shellHeader = document.body.querySelector('[data-slot="v3-shell-header"]');
const mainHeader = document.body.querySelector('main [data-slot="v3-page-header"]');
const sidebarTrigger = document.body.querySelector('[data-slot="sidebar-trigger"]');
const search = screen.getByRole("button", {
  name: /搜索任务、数字员工、能力、文档或快捷命令/,
}).element() as HTMLElement;

expect(shellHeader).toBeInstanceOf(HTMLElement);
expect(shellHeader?.contains(heading)).toBe(true);
expect(mainHeader).toBeNull();
expect(sidebarTrigger).toBeNull();
expect(search.className).toContain("justify-self-center");
expect(search.className).toContain("max-w-sm");
```

- [ ] **Step 2: Update the workflow test Header mock**

The mock must preserve the new shell contract:

```tsx
vi.mock("@/components/layout/header", () => ({
  Header: ({
    children,
    showSidebarTrigger = true,
  }: {
    children?: ReactNode;
    showSidebarTrigger?: boolean;
  }) => (
    <header data-slot="v3-shell-header">
      {showSidebarTrigger ? (
        <button data-slot="sidebar-trigger" type="button">
          切换侧栏
        </button>
      ) : (
        children
      )}
      <button className={showSidebarTrigger ? "" : "justify-self-center max-w-sm"} type="button">
        搜索任务、数字员工、能力、文档或快捷命令... ⌘ K
      </button>
    </header>
  ),
}));
```

- [ ] **Step 3: Replace the old workflow shell header**

`WorkflowShell` should render:

```tsx
<ShellPageHeader
  icon={<GitBranch />}
  iconTone="info"
  subtitle="查看需求触发的规划、执行、阻塞和结果状态"
  title="流程编排"
/>
<Main className="min-w-0 overflow-x-hidden">
  <div className="flex min-w-0 flex-col gap-5">{children}</div>
</Main>
```

Do not leave a second `V3PageHeader` inside `Main`.

- [ ] **Step 4: Verify**

Run:

```bash
corepack pnpm --filter ./apps/web run test -- src/components/layout/header.test.tsx src/features/workflows/index.test.tsx
```

Expected: 2 files pass.

---

### Task 3: Migrate Reusable Feature Shells

**Files:**
- Modify: `apps/web/src/features/inbox/components/inbox-shell.tsx`
- Modify: `apps/web/src/features/projects/components/project-management-shell.tsx`
- Modify: `apps/web/src/features/task-launches/components/task-launch-shell.tsx`

- [ ] **Step 1: Replace local Header imports**

Remove these imports when present:

```tsx
import { Header } from "@/components/layout/header";
import { Search } from "@/components/search";
import { ThemeSwitch } from "@/components/theme-switch";
```

Add:

```tsx
import { ShellPageHeader } from "@/components/layout/shell-page-header";
```

- [ ] **Step 2: Move each shell title into `ShellPageHeader`**

Use the same title, subtitle, icon, icon tone, and actions currently passed to content `V3PageHeader`.

Pattern:

```tsx
<ShellPageHeader
  icon={<ExistingIcon />}
  iconTone="brand"
  subtitle="现有副标题"
  title="现有标题"
  actions={existingActions}
/>
<Main className="min-w-0 overflow-x-hidden bg-v3-bg">
  <div className="flex min-w-0 flex-col gap-5">{children}</div>
</Main>
```

- [ ] **Step 3: Verify no duplicate page header remains in those shells**

Run:

```bash
rg -n "<V3PageHeader|<Header|<Search|<ThemeSwitch" apps/web/src/features/inbox/components/inbox-shell.tsx apps/web/src/features/projects/components/project-management-shell.tsx apps/web/src/features/task-launches/components/task-launch-shell.tsx
```

Expected: only `ShellPageHeader` remains for the page-level header.

---

### Task 4: Migrate Primary Console Pages

**Files:**
- Modify: `apps/web/src/features/dashboard/index.tsx`
- Modify: `apps/web/src/features/projects/index.tsx`
- Modify: `apps/web/src/features/employees/index.tsx`
- Modify: `apps/web/src/features/skills/index.tsx`
- Modify: `apps/web/src/features/teams/index.tsx`
- Modify: `apps/web/src/features/users/index.tsx`
- Modify: `apps/web/src/features/permissions/index.tsx`
- Modify: `apps/web/src/features/mcp/index.tsx`
- Modify: `apps/web/src/features/runtime/index.tsx`
- Modify: `apps/web/src/routes/_authenticated/costs/index.tsx`

- [ ] **Step 1: Replace page-level Header blocks**

For each page, replace:

```tsx
<Header>
  <Search />
  <ThemeSwitch />
</Header>
```

with:

```tsx
<ShellPageHeader
  icon={existingIcon}
  iconTone={existingIconTone}
  subtitle={existingSubtitle}
  title={existingTitle}
  actions={existingActions}
/>
```

- [ ] **Step 2: Remove the content-level `V3PageHeader`**

After migration, `Main` should start with metrics, filters, tabs, state surfaces, tables, or domain content:

```tsx
<Main className="min-w-0 overflow-x-hidden bg-v3-bg">
  <div className="flex min-w-0 flex-col gap-5">
    {pageBody}
  </div>
</Main>
```

- [ ] **Step 3: Preserve actions**

If a page has a create/upload/manage button in `V3PageHeader.actions`, pass it unchanged to `ShellPageHeader.actions`. Use existing `Link`, `V3Button`, and handlers. Do not move page actions into arbitrary content cards.

- [ ] **Step 4: Verify imports are clean**

Run:

```bash
rg -n "from \"@/components/layout/header\"|from '@/components/layout/header'|from \"@/components/search\"|from '@/components/search'|from \"@/components/theme-switch\"|from '@/components/theme-switch'" apps/web/src/features/dashboard apps/web/src/features/projects apps/web/src/features/employees apps/web/src/features/skills apps/web/src/features/teams apps/web/src/features/users apps/web/src/features/permissions apps/web/src/features/mcp apps/web/src/features/runtime apps/web/src/routes/_authenticated/costs
```

Expected: no imports remain for pages migrated in this task unless the file has a non-page-level reason.

---

### Task 5: Migrate Secondary Pages, Detail Pages, Logs, And Placeholders

**Files:**
- Modify: `apps/web/src/features/employees/detail.tsx`
- Modify: `apps/web/src/features/employees/create.tsx`
- Modify: `apps/web/src/features/employees/config.tsx`
- Modify: `apps/web/src/features/employees/templates.tsx`
- Modify: `apps/web/src/features/skills/detail.tsx`
- Modify: `apps/web/src/features/skills/upload.tsx`
- Modify: `apps/web/src/features/account/index.tsx`
- Modify: `apps/web/src/routes/_authenticated/logs/login/index.tsx`
- Modify: `apps/web/src/routes/_authenticated/logs/operation/index.tsx`
- Modify: `apps/web/src/routes/_authenticated/logs/runtime/index.tsx`
- Modify: `apps/web/src/features/shared/unimplemented-page.tsx`
- Modify: `apps/web/src/routes/_authenticated/errors/$error.tsx`

- [ ] **Step 1: Preserve back affordances**

If a page uses `V3PageHeader back={...}`, pass the same `back` element to `ShellPageHeader`. Do not silently convert back navigation into a non-clickable icon.

Pattern:

```tsx
<ShellPageHeader
  back={existingBack}
  subtitle={existingSubtitle}
  title={existingTitle}
  actions={existingActions}
/>
```

- [ ] **Step 2: Preserve loading and error top bars**

If a page returns a loading or error branch before data exists, it should still render a meaningful `ShellPageHeader` with the stable page title.

Example:

```tsx
if (query.isLoading) {
  return (
    <>
      <ShellPageHeader icon={<Users />} iconTone="brand" title="数字员工" subtitle="正在加载员工详情" />
      <Main className="min-w-0 overflow-x-hidden bg-v3-bg">
        <V3LoadingState />
      </Main>
    </>
  );
}
```

- [ ] **Step 3: Verify no main content duplicate remains**

Run:

```bash
rg -n "main .*v3-page-header|<V3PageHeader" apps/web/src/features/employees apps/web/src/features/skills apps/web/src/features/account apps/web/src/routes/_authenticated/logs apps/web/src/features/shared apps/web/src/routes/_authenticated/errors
```

Expected: remaining `V3PageHeader` usage should be only inside `ShellPageHeader` or files intentionally not migrated yet.

---

### Task 6: Update Tests And Test Mocks

**Files:**
- Modify: `apps/web/src/**/*.test.tsx` only where assertions or mocks depend on page header placement.

- [ ] **Step 1: Find stale Header mocks**

Run:

```bash
rg -n "vi.mock\\(\"@/components/layout/header\"|vi.mock\\('@/components/layout/header'" apps/web/src --glob '*.test.tsx'
```

- [ ] **Step 2: Update only mocks that need shell placement behavior**

Use this mock only when the test asserts page heading position or sidebar-trigger absence:

```tsx
vi.mock("@/components/layout/header", () => ({
  Header: ({
    children,
    showSidebarTrigger = true,
  }: {
    children?: ReactNode;
    showSidebarTrigger?: boolean;
  }) => (
    <header data-slot="v3-shell-header">
      {showSidebarTrigger ? <button data-slot="sidebar-trigger" type="button">切换侧栏</button> : children}
      <button className={showSidebarTrigger ? "" : "justify-self-center max-w-sm"} type="button">
        搜索任务、数字员工、能力、文档或快捷命令... ⌘ K
      </button>
    </header>
  ),
}));
```

For tests that only need a shell wrapper, keep a simpler mock:

```tsx
vi.mock("@/components/layout/header", () => ({
  Header: ({ children }: { children?: ReactNode }) => <header>{children}</header>,
}));
```

- [ ] **Step 3: Add at least one representative page regression per migrated batch**

For each batch, add one assertion equivalent to:

```tsx
expect(document.body.querySelector('[data-slot="v3-shell-header"]')?.textContent).toContain("页面标题");
expect(document.body.querySelector("main [data-slot='v3-page-header']")).toBeNull();
expect(document.body.querySelector('[data-slot="sidebar-trigger"]')).toBeNull();
```

---

### Task 7: Verification And Browser Smoke

**Files:**
- No production edits unless verification exposes a defect.

- [ ] **Step 1: Static cleanup**

Run:

```bash
git diff --check
```

Expected: no whitespace errors.

- [ ] **Step 2: Targeted Web tests**

Run:

```bash
corepack pnpm --filter ./apps/web run test -- src/components/layout/header.test.tsx src/features/workflows/index.test.tsx
```

Expected: all targeted tests pass.

- [ ] **Step 3: Broader Web test pass**

Run:

```bash
corepack pnpm --filter ./apps/web run test
```

Expected: pass. If this is too slow or blocked, record the exact failing file and command output.

- [ ] **Step 4: Check live services**

Run:

```bash
scripts/dev-services.sh status
```

If Web or Control Plane is stale after code changes, restart only the needed service:

```bash
scripts/dev-services.sh restart web
```

- [ ] **Step 5: Browser smoke with real app**

Using the Codex browser or Chrome plug, open these routes:

```text
http://127.0.0.1:3000/workflows
http://127.0.0.1:3000/projects
http://127.0.0.1:3000/employees
http://127.0.0.1:3000/skills
```

For each route, verify:

- Page title is inside `[data-slot="v3-shell-header"]`.
- `main [data-slot="v3-page-header"]` is absent.
- `[data-slot="sidebar-trigger"]` is absent on migrated pages.
- Search is visible, right-shifted, and no wider than the title/search/top-right controls can support.
- No desktop or mobile horizontal overflow.
- Loading, empty, error, and permission-denied states still render under `Main`.

- [ ] **Step 6: Completion gate**

Before claiming the full migration is complete, use `.codex/skills/superteam-completion-check/SKILL.md` and record which checks are real end-to-end evidence versus unit/component evidence.
