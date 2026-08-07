# Digital Employee Create Flow Refinement Implementation Plan

> 复核状态：数字员工创建流程未落地

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Refine the digital employee creation wizard so templates only seed role/capability/governance defaults, configuration shows one selected-template summary, unavailable creation paths are clearly disabled, and Runtime binding lists real bindable Runtime Provider options as the selectable list while still surfacing unavailable runtimes with their reason for diagnosability.

**Architecture:** This is a Web-only refinement over the existing `GET /api/v1/digital-employees/create-options` contract. The backend remains the source of truth for team policy, Runtime availability, Provider health, and final create validation; the frontend narrows what it displays and where decisions are made. Existing create wizard state stays in `apps/web/src/features/employees/create.tsx`; tests stay in `apps/web/src/features/employees/create.test.tsx`.

**Tech Stack:** React, TypeScript, TanStack Query, TanStack Router, shadcn/Radix UI primitives, Vitest Browser, `corepack pnpm --filter ./apps/web run test`.

---

## File Structure

Modify:

- `apps/web/src/features/employees/create.test.tsx`
  - Extend test fixtures to represent unavailable Runtime Provider options.
  - Add regression tests for disabled creation paths, template/provider decoupling, selected-template summary, dirty-change confirmation, Runtime option filtering, and no-bindable-runtime empty state.
- `apps/web/src/features/employees/create.tsx`
  - Disable non-template creation paths and blank custom entry points.
  - Remove Provider from template-facing UI.
  - Replace the configure-page full template list with a selected-template summary and change-template action.
  - Track whether the configure draft has been edited; warn before changing template, and reset the draft (preserving the selected team) when the change is confirmed so the warning copy is truthful.
  - Render `available=true` Runtime Provider options as the selectable Runtime list, and surface unavailable ones in a separate non-selectable hint that keeps their `disabled_reason`.
  - Keep create submission sourced from the selected available Runtime Provider option.

Do not modify:

- `contracts/control-plane/openapi.yaml`
- `apps/control-plane/**`
- `apps/runtime-agent/**`
- database migrations

## Task 1: Add Fixture Support And Failing UI Tests

**Files:**

- Modify: `apps/web/src/features/employees/create.test.tsx`

- [ ] **Step 1: Extend the create-options fixture parameters**

In `apps/web/src/features/employees/create.test.tsx`, replace the current `createOptionsFixture` signature and Runtime fixture construction with this version. Keep the rest of the returned object unchanged.

```tsx
type RuntimeAvailabilityMode = "all" | "first-unavailable" | "none";

function createOptionsFixture({
  runtimeAvailability = "all",
  runtimeCount = 1,
  sameRuntimeNodeProviders = false,
}: {
  runtimeAvailability?: RuntimeAvailabilityMode;
  runtimeCount?: 1 | 2;
  sameRuntimeNodeProviders?: boolean;
} = {}) {
  const firstRuntimeAvailable = runtimeAvailability === "all";
  const secondRuntimeAvailable = runtimeAvailability !== "none";
  const firstRuntimeOption = {
    runtime_node_id: "33333333-3333-4333-8333-333333333333",
    node_id: "runtime-a",
    runtime_name: "客户侧执行机 A",
    provider_type: "codex",
    runtime_status: firstRuntimeAvailable ? "online" : "offline",
    provider_status: firstRuntimeAvailable ? "healthy" : "unhealthy",
    health_status: firstRuntimeAvailable ? "healthy" : "unhealthy",
    current_load: 0,
    max_slots: 2,
    agent_home_dir: "/Users/wangpei/.codex",
    agent_home_dir_available: firstRuntimeAvailable,
    available: firstRuntimeAvailable,
    disabled_reason: firstRuntimeAvailable ? undefined : "runtime_session_inactive",
  };
  const sameNodeProviderOption = {
    runtime_node_id: "33333333-3333-4333-8333-333333333333",
    node_id: "runtime-a",
    runtime_name: "客户侧执行机 A",
    provider_type: "claude_code",
    runtime_status: firstRuntimeAvailable ? "online" : "offline",
    provider_status: firstRuntimeAvailable ? "healthy" : "unhealthy",
    health_status: firstRuntimeAvailable ? "healthy" : "unhealthy",
    current_load: 0,
    max_slots: 2,
    agent_home_dir: "/Users/wangpei/.claude",
    agent_home_dir_available: firstRuntimeAvailable,
    available: firstRuntimeAvailable,
    disabled_reason: firstRuntimeAvailable ? undefined : "runtime_session_inactive",
  };
  const secondRuntimeOption = {
    runtime_node_id: "44444444-4444-4444-8444-444444444444",
    node_id: "runtime-b",
    runtime_name: "客户侧执行机 B",
    provider_type: "codex",
    runtime_status: secondRuntimeAvailable ? "online" : "offline",
    provider_status: secondRuntimeAvailable ? "healthy" : "unhealthy",
    health_status: secondRuntimeAvailable ? "healthy" : "unhealthy",
    current_load: 1,
    max_slots: 2,
    agent_home_dir: "/Users/wangpei/.codex",
    agent_home_dir_available: secondRuntimeAvailable,
    available: secondRuntimeAvailable,
    disabled_reason: secondRuntimeAvailable ? undefined : "runtime_session_inactive",
  };
  const runtimeProviderOptions = [
    firstRuntimeOption,
    ...(sameRuntimeNodeProviders ? [sameNodeProviderOption] : []),
    ...(!sameRuntimeNodeProviders && runtimeCount === 2 ? [secondRuntimeOption] : []),
  ];

  return {
    team_config: {
      id: "55555555-5555-4555-8555-555555555555",
      tenant_id: "22222222-2222-4222-8222-222222222222",
      team_id: team.id,
      revision_number: 3,
      status: "approved",
      allowed_employee_types: ["database_admin"],
      allowed_provider_types: sameRuntimeNodeProviders ? ["codex", "claude_code"] : ["codex"],
      allowed_skills: ["incident-diagnosis", "sql-review"],
      allowed_mcp_servers: ["postgres"],
      allowed_external_capabilities: ["jira.search"],
      capability_policy: { mode: "allow_list" },
      context_policy: { max_refs: 8 },
      approval_policy: { required: true },
      artifact_contract: { required: ["summary"] },
      internal_collaboration_policy: { handoff: "structured" },
      runtime_scope_policy: { allowed_nodes: ["runtime-a"] },
    },
    employee_types: [
      {
        type: "database_admin",
        label: "数据库管理员",
        description: "负责数据库变更、备份、性能诊断和恢复验证",
        default_role: "database_admin",
        recommended_skills: ["incident-diagnosis"],
        recommended_mcp_servers: ["postgres"],
        recommended_provider_types: ["codex"],
        default_capability_selection: {
          enabled_skills: ["sql-review"],
          enabled_mcp_servers: ["postgres"],
          enabled_external_capabilities: ["jira.search"],
        },
        default_context_policy_override: { max_refs: 8 },
        default_approval_policy: { min_risk_for_human: "high" },
        metadata: { title: "数据库管理员" },
      },
    ],
    capability_options: {
      provider_types: sameRuntimeNodeProviders ? ["codex", "claude_code"] : ["codex"],
      skills: ["incident-diagnosis", "sql-review"],
      mcp_servers: ["postgres"],
      external_capabilities: ["jira.search"],
    },
    runtime_provider_options: runtimeProviderOptions,
    creation_checks: [
      {
        key: "team_governance",
        label: "团队治理版本",
        status: "passed",
        message: "#3 approved",
      },
      {
        key: "employee_templates",
        label: "专业模板",
        status: "passed",
        message: "1 个可用模板",
      },
      {
        key: "runtime_provider",
        label: "Runtime 可用",
        status: runtimeProviderOptions.some((option) => option.available) ? "passed" : "blocked",
        message: `${runtimeProviderOptions.filter((option) => option.available).length} 个可用运行绑定`,
      },
    ],
    policy_defaults: {
      permission_policy: { mode: "least_privilege" },
      context_policy_override: { max_refs: 6 },
      approval_policy: { required: true },
      capability_selection: { source: "team_default" },
      runtime_selector: { strategy: "manual" },
      workspace_policy: { mode: "ephemeral" },
      session_policy: { mode: "reuse_latest" },
      metadata: { source: "team_config" },
    },
  };
}
```

- [ ] **Step 2: Pass the new fixture option through `createWizardFetcher`**

Update the `createWizardFetcher` parameter type and call to `createOptionsFixture`:

```tsx
function createWizardFetcher({
  expectedEnvironmentVariables,
  expectedProviderType = "codex",
  expectedRuntimeNodeId = "33333333-3333-4333-8333-333333333333",
  runtimeAvailability = "all",
  runtimeCount = 1,
  sameRuntimeNodeProviders = false,
}: {
  expectedEnvironmentVariables?: Array<{ name: string; value: string; sensitive: boolean }>;
  expectedProviderType?: string;
  expectedRuntimeNodeId?: string;
  runtimeAvailability?: RuntimeAvailabilityMode;
  runtimeCount?: 1 | 2;
  sameRuntimeNodeProviders?: boolean;
} = {}) {
  const fetcher = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = new URL(String(input));
    const method = init?.method ?? "GET";

    if (url.pathname === "/api/v1/teams" && method === "GET") {
      return jsonResponse([team]);
    }

    if (url.pathname === "/api/v1/digital-employees/create-options" && method === "GET") {
      expect(url.searchParams.get("team_id")).toBe(team.id);
      return jsonResponse(createOptionsFixture({ runtimeAvailability, runtimeCount, sameRuntimeNodeProviders }));
    }
```

Keep the existing avatar and create POST branches below this snippet.

- [ ] **Step 3: Add failing tests for creation-path and template Provider behavior**

Append these tests inside `describe("CreateEmployeeView", () => { ... })`, after the existing command-center entry test:

```tsx
  it("marks unavailable creation paths and blank custom entry points as disabled", async () => {
    const screen = await renderCreateEmployeeView();

    await expect.element(screen.getByRole("button", { name: /^从专业模板创建/ })).toBeEnabled();
    await expect.element(screen.getByRole("button", { name: /^从团队角色复制/ })).toBeDisabled();
    await expect.element(screen.getByRole("button", { name: /^从历史员工克隆/ })).toBeDisabled();
    await expect.element(screen.getByRole("button", { name: /^空白自定义/ })).toBeDisabled();
    await expect.element(screen.getByRole("button", { name: /^选择空白自定义/ })).toBeDisabled();

    await expect.element(screen.getByRole("heading", { name: "选择专业类型" })).toBeVisible();
    expect(document.body.textContent).not.toContain("员工画像蓝图");
  });

  it("keeps template cards provider-neutral", async () => {
    const screen = await renderCreateEmployeeView();

    await expect.element(screen.getByText("数据库管理员")).toBeVisible();
    await expect.element(screen.getByText("技能")).toBeVisible();
    await expect.element(screen.getByText("MCP")).toBeVisible();
    await expect.element(screen.getByText("风险")).toBeVisible();
    expect(document.body.textContent).not.toContain("Provider");
    expect(document.body.textContent).not.toContain("推荐 Provider");
  });
```

- [ ] **Step 4: Add failing tests for configure-page template summary and dirty return confirmation**

Append these tests after `keeps template cards provider-neutral`:

```tsx
  it("shows only the selected template summary after entering configuration", async () => {
    const screen = await renderCreateEmployeeView();

    await enterConfiguration(screen);

    await expect.element(screen.getByText("已选模板")).toBeVisible();
    await expect.element(screen.getByRole("button", { name: /更换模板/ })).toBeVisible();
    expect(document.body.textContent).not.toContain("推荐起步画像");
    expect(document.body.textContent).not.toContain("从空白开始自定义");
    expect(document.body.textContent).not.toContain("模板只提供默认值和推荐能力");
  });

  it("keeps edited configuration when change-template confirmation is cancelled", async () => {
    const confirm = vi.spyOn(window, "confirm").mockReturnValue(false);
    const screen = await renderCreateEmployeeView();

    await enterConfiguration(screen);
    await userEvent.fill(screen.getByLabelText("名称"), "数据库管理员工");
    await userEvent.click(screen.getByRole("button", { name: /更换模板/ }));

    expect(confirm).toHaveBeenCalledWith("更换模板会重置当前配置草稿，是否继续？");
    await expect.element(screen.getByRole("heading", { name: "身份" })).toBeVisible();
    await expect.element(screen.getByLabelText("名称")).toHaveValue("数据库管理员工");

    confirm.mockRestore();
  });

  it("resets the configuration draft when change-template confirmation is accepted", async () => {
    const confirm = vi.spyOn(window, "confirm").mockReturnValue(true);
    const screen = await renderCreateEmployeeView();

    await enterConfiguration(screen);
    await userEvent.fill(screen.getByLabelText("名称"), "数据库管理员工");
    await userEvent.click(screen.getByRole("button", { name: /更换模板/ }));

    expect(confirm).toHaveBeenCalledWith("更换模板会重置当前配置草稿，是否继续？");
    await expect.element(screen.getByRole("heading", { name: "选择专业类型" })).toBeVisible();

    await enterConfiguration(screen);
    await expect.element(screen.getByLabelText("名称")).toHaveValue("");

    confirm.mockRestore();
  });
```

- [ ] **Step 5: Add failing tests for Runtime filtering and empty state**

Append these tests after the existing Runtime selection tests:

```tsx
  it("selects available runtimes and surfaces unavailable ones with their reason", async () => {
    const screen = await renderCreateEmployeeView(
      createWizardFetcher({
        expectedRuntimeNodeId: "44444444-4444-4444-8444-444444444444",
        runtimeAvailability: "first-unavailable",
        runtimeCount: 2,
      }),
    );

    await enterConfiguration(screen);
    await userEvent.fill(screen.getByLabelText("名称"), "数据库管理员工");
    await userEvent.click(screen.getByRole("button", { name: "下一步" }));
    await userEvent.click(screen.getByRole("button", { name: "下一步" }));
    await userEvent.click(screen.getByRole("button", { name: "下一步" }));

    // Only the available runtime is a selectable radio and is auto-bound.
    await expect.element(screen.getByRole("radio", { name: "客户侧执行机 B / codex" })).toBeChecked();
    expect(screen.queryByRole("radio", { name: "客户侧执行机 A / codex" })).toBeNull();
    // The unavailable runtime is still surfaced (with its reason) for diagnosability.
    await expect.element(screen.getByText("暂不可绑定的 Runtime")).toBeVisible();
    await expect.element(screen.getByText("runtime_session_inactive")).toBeVisible();
  });

  it("blocks creation when there are no bindable runtime provider options", async () => {
    const screen = await renderCreateEmployeeView(createWizardFetcher({ runtimeAvailability: "none" }));

    await enterConfiguration(screen);
    await userEvent.fill(screen.getByLabelText("名称"), "数据库管理员工");
    await userEvent.click(screen.getByRole("button", { name: "下一步" }));
    await userEvent.click(screen.getByRole("button", { name: "下一步" }));
    await userEvent.click(screen.getByRole("button", { name: "下一步" }));

    await expect
      .element(screen.getByText("当前团队没有可绑定的 Runtime Provider，请检查 Runtime 在线状态、Provider 健康状态或团队运行策略。"))
      .toBeVisible();
    await expect.element(screen.getByRole("button", { name: "创建数字员工" })).toBeDisabled();
    // No selectable runtime radio exists, but the unavailable runtime is still listed with its reason.
    expect(screen.queryByRole("radio", { name: "客户侧执行机 A / codex" })).toBeNull();
    await expect.element(screen.getByText("暂不可绑定的 Runtime")).toBeVisible();
  });
```

> Note on `queryByRole`: `vitest-browser-react` exposes synchronous `queryBy*` locators that return `null` when no element matches. If the installed version does not, assert non-selectability instead with `expect(screen.container.querySelector('[role="radio"][aria-label="客户侧执行机 A / codex"]')).toBeNull();` — the unavailable list renders plain text, not a radio, so no `radio` role carries that name.

- [ ] **Step 6: Run the focused test and verify it fails**

Run:

```bash
corepack pnpm --filter ./apps/web run test -- src/features/employees/create.test.tsx
```

Expected: FAIL. The strongest failing signals should include at least these messages:

```text
expect.element(...).toBeDisabled()
expected document body not to contain "Provider"
Unable to find an element with the text: 已选模板
Unable to find an element with the text: 暂不可绑定的 Runtime
Unable to find an element with the text: 当前团队没有可绑定的 Runtime Provider...
```

## Task 2: Disable Unimplemented Creation Paths And Remove Provider From Template UI

**Files:**

- Modify: `apps/web/src/features/employees/create.tsx`
- Test: `apps/web/src/features/employees/create.test.tsx`

- [ ] **Step 1: Replace `CreationPathPanel` with explicit disabled-state rendering**

Replace the full `CreationPathPanel` function with:

```tsx
function CreationPathPanel() {
  const paths = [
    {
      title: "从专业模板创建",
      description: "按职责模板带出默认角色、能力建议和治理策略。",
      icon: Sparkles,
      active: true,
      badge: "推荐",
      disabled: false,
    },
    {
      title: "从团队角色复制",
      description: "复用团队内已验证的角色画像和能力边界。",
      icon: ClipboardCheck,
      active: false,
      badge: "暂未开放",
      disabled: true,
    },
    {
      title: "从历史员工克隆",
      description: "基于已有员工配置生成新草稿，保留审计来源。",
      icon: GitBranch,
      active: false,
      badge: "暂未开放",
      disabled: true,
    },
    {
      title: "空白自定义",
      description: "从空白身份开始逐项配置职责、能力和运行绑定。",
      icon: FileText,
      active: false,
      badge: "暂未开放",
      disabled: true,
    },
  ];

  return (
    <aside className="rounded-md border bg-card/95 p-3 shadow-xs">
      <div className="mb-3 flex items-center gap-2 px-1">
        <IconTile tone="brand" size="sm">
          <Sparkles />
        </IconTile>
        <div>
          <h2 className="text-base font-semibold">创建路径</h2>
          <p className="text-xs text-muted-foreground">先选入口，再进入配置。</p>
        </div>
      </div>
      <div className="grid gap-2">
        {paths.map((path) => {
          const Icon = path.icon;
          return (
            <button
              aria-pressed={path.active}
              className={cn(
                "rounded-md border p-3 text-left transition",
                path.active
                  ? "border-primary/40 bg-primary/10 text-foreground shadow-xs"
                  : "border-border/70 bg-background/80 text-muted-foreground",
                path.disabled ? "cursor-not-allowed opacity-65" : "hover:border-primary/30 hover:bg-primary/5",
              )}
              disabled={path.disabled}
              key={path.title}
              type="button"
            >
              <span className="flex items-start gap-2">
                <span
                  className={cn(
                    "mt-0.5 flex size-8 shrink-0 items-center justify-center rounded-md border",
                    path.active ? "border-primary/30 bg-primary/15 text-primary" : "bg-muted text-muted-foreground",
                  )}
                >
                  <Icon className="size-4" />
                </span>
                <span className="min-w-0 flex-1">
                  <span className="flex items-center justify-between gap-2">
                    <span className="text-sm font-medium">{path.title}</span>
                    <Badge variant={path.active ? "default" : "secondary"}>{path.badge}</Badge>
                  </span>
                  <span className="mt-1 block text-xs leading-5">{path.description}</span>
                </span>
              </span>
            </button>
          );
        })}
      </div>
      <div className="mt-3 rounded-md border bg-muted/30 p-3 text-xs leading-5 text-muted-foreground">
        创建后进入 ready，不会自动执行任务；项目或任务调度可手动发起。
      </div>
    </aside>
  );
}
```

- [ ] **Step 2: Disable the blank-custom text action in `TemplateSelectionPanel`**

Replace the `没有合适的模板？` block in `TemplateSelectionPanel` with:

```tsx
      <div className="mt-3 text-sm text-muted-foreground">
        没有合适的模板？
        <button
          className="ml-2 cursor-not-allowed font-medium text-muted-foreground"
          disabled
          type="button"
        >
          选择空白自定义（暂未开放）
        </button>
      </div>
```

- [ ] **Step 3: Remove Provider from `TemplateCard`**

In `TemplateCard`, delete this line:

```tsx
  const providerLabel = typeOption.recommended_provider_types?.join(" / ") || "按团队策略";
```

Replace the metrics block with:

```tsx
      <span className="mt-auto grid grid-cols-2 gap-x-3 gap-y-2 border-t pt-3 text-xs @[640px]/template:grid-cols-4 @[980px]/template:grid-cols-2">
        <MetricPill label="技能" value={String(typeOption.recommended_skills?.length ?? 0)} />
        <MetricPill label="MCP" value={String(typeOption.recommended_mcp_servers?.length ?? 0)} />
        <MetricPill label="风险" value={risk} tone={risk === "high" || risk === "critical" ? "warning" : "success"} />
        <MetricPill label="默认角色" value={typeOption.default_role || typeOption.type} />
      </span>
```

- [ ] **Step 4: Remove Provider from the selection-stage readiness summary**

In `CreationReadinessPanel`, remove this line:

```tsx
          <InlineSummary label="推荐 Provider" value={selectedType?.recommended_provider_types?.join(" / ") || "按团队策略"} />
```

- [ ] **Step 5: Run the focused tests**

Run:

```bash
corepack pnpm --filter ./apps/web run test -- src/features/employees/create.test.tsx
```

Expected: The creation-path and provider-neutral tests pass. The selected-template summary and Runtime filtering tests still fail until later tasks.

- [ ] **Step 6: Commit this narrow UI cleanup**

Run:

```bash
git add apps/web/src/features/employees/create.tsx apps/web/src/features/employees/create.test.tsx
git commit -m "fix(web): clarify digital employee template entry states"
```

Expected: commit succeeds and includes only `create.tsx` and `create.test.tsx`.

## Task 3: Replace Configure-Page Template List With Selected Template Summary

**Files:**

- Modify: `apps/web/src/features/employees/create.tsx`
- Test: `apps/web/src/features/employees/create.test.tsx`

- [ ] **Step 1: Track configure-draft edits and add change-template handling**

Inside `CreateEmployeeView`, add this state after `workbenchMode`:

```tsx
  const [draftTouched, setDraftTouched] = useState(false);
```

Replace `updateDraft`, `selectType`, and `enterConfiguration` with:

```tsx
  function updateDraft(patch: Partial<WizardDraft>) {
    if (workbenchMode === "configure") {
      setDraftTouched(true);
    }
    setDraft((current) => ({ ...current, ...patch }));
  }

  function selectType(typeValue: string) {
    if (workbenchMode === "configure") {
      setDraftTouched(true);
    }
    const nextType = createOptions.data?.employee_types.find((item) => item.type === typeValue);
    if (!nextType) {
      updateDraft({ employee_type: typeValue });
      return;
    }
    setDraft((current) => applyTypeDefaults(current, nextType));
  }

  function enterConfiguration() {
    setWorkbenchMode("configure");
    setStepIndex(0);
    setDraftTouched(false);
  }
```

Add this function after `enterConfiguration`:

```tsx
  function requestTemplateChange() {
    if (draftTouched && !window.confirm("更换模板会重置当前配置草稿，是否继续？")) {
      return;
    }
    setErrors({});
    setStepIndex(0);
    setDraftTouched(false);
    setDraft((current) => ({ ...emptyDraft, team_id: current.team_id }));
    setWorkbenchMode("select");
  }
```

Resetting to `emptyDraft` while preserving `team_id` makes the confirm copy truthful: the configure draft (name, role, description, avatar, capabilities, env vars) is cleared. The existing `useEffect` hooks then re-seed the preferred employee type, first active avatar, and single-runtime auto-binding for the still-selected team, so re-entering configuration starts from clean defaults rather than a stale draft.

- [ ] **Step 2: Remove the configure-page `BlueprintSidebar` column**

Replace the configure-mode wrapper:

```tsx
          <div className="grid gap-4 xl:grid-cols-[260px_minmax(0,1fr)_340px]">
            <BlueprintSidebar
              draft={draft}
              options={createOptions.data}
              selectedType={selectedType}
              onSelectType={selectType}
            />

          <section className="min-w-0 rounded-md border bg-card/95 shadow-xs">
```

with:

```tsx
          <div className="grid gap-4 xl:grid-cols-[minmax(0,1fr)_340px]">
          <section className="min-w-0 rounded-md border bg-card/95 shadow-xs">
```

- [ ] **Step 3: Replace `TemplateOverview` with `SelectedTemplateSummary`**

In the configure-mode form body, replace:

```tsx
              <TemplateOverview
                draft={draft}
                options={createOptions.data}
                selectedType={selectedType}
                onSelectType={selectType}
              />
```

with:

```tsx
              <SelectedTemplateSummary
                draft={draft}
                selectedType={selectedType}
                onChangeTemplate={requestTemplateChange}
              />
```

- [ ] **Step 4: Delete `BlueprintSidebar` and `TemplateOverview`**

Remove the full `BlueprintSidebar` function and the full `TemplateOverview` function from `apps/web/src/features/employees/create.tsx`.

- [ ] **Step 5: Add `SelectedTemplateSummary`**

Add this function where `TemplateOverview` was removed:

```tsx
function SelectedTemplateSummary({
  draft,
  selectedType,
  onChangeTemplate,
}: {
  draft: WizardDraft;
  selectedType?: DigitalEmployeeTypeOption;
  onChangeTemplate: () => void;
}) {
  return (
    <section className="rounded-md border bg-background p-4">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div className="min-w-0">
          <div className="text-xs font-medium text-muted-foreground">已选模板</div>
          <h2 className="mt-1 truncate text-base font-semibold">
            {selectedType?.label ?? draft.employee_type || "未选择模板"}
          </h2>
          <p className="mt-1 line-clamp-2 text-sm leading-6 text-muted-foreground">
            {selectedType?.description ?? "模板只作为初始草稿来源，运行绑定在最后一步选择。"}
          </p>
          <div className="mt-2 flex flex-wrap gap-1.5">
            <Badge variant="secondary">默认角色 {selectedType?.default_role || draft.role || "未生成"}</Badge>
            <Badge variant="secondary">技能 {selectedType?.recommended_skills?.length ?? 0}</Badge>
            <Badge variant="secondary">MCP {selectedType?.recommended_mcp_servers?.length ?? 0}</Badge>
          </div>
        </div>
        <Button onClick={onChangeTemplate} type="button" variant="outline">
          <ArrowLeft data-icon="inline-start" />
          更换模板
        </Button>
      </div>
    </section>
  );
}
```

- [ ] **Step 6: Run the focused tests**

Run:

```bash
corepack pnpm --filter ./apps/web run test -- src/features/employees/create.test.tsx
```

Expected: configure-page summary and dirty-confirmation tests pass. Runtime filtering tests still fail until Task 4.

- [ ] **Step 7: Commit this configure-page summary change**

Run:

```bash
git add apps/web/src/features/employees/create.tsx apps/web/src/features/employees/create.test.tsx
git commit -m "fix(web): summarize selected employee template during configuration"
```

Expected: commit succeeds and includes only `create.tsx` and `create.test.tsx`.

## Task 4: Filter Runtime Binding To Real Available Options

**Files:**

- Modify: `apps/web/src/features/employees/create.tsx`
- Test: `apps/web/src/features/employees/create.test.tsx`

- [ ] **Step 1: Render only available Runtime Provider options**

In `RuntimeStep`, replace:

```tsx
  const runtimeOptions = options?.runtime_provider_options ?? [];
```

with:

```tsx
  const runtimeOptions = options?.runtime_provider_options ?? [];
  const availableRuntimeOptions = runtimeOptions.filter((option) => option.available);
  const unavailableRuntimeOptions = runtimeOptions.filter((option) => !option.available);
```

Replace the `RadioGroup` block and empty-state paragraph:

```tsx
      <RadioGroup onValueChange={onSelectRuntime} value={draft.runtime_binding}>
        <div className="grid gap-3">
          {runtimeOptions.map((option) => (
            <RuntimeOption
              key={runtimeBinding(option)}
              onSelectRuntime={onSelectRuntime}
              option={option}
            />
          ))}
        </div>
      </RadioGroup>
      {runtimeOptions.length === 0 ? <p className="text-sm text-muted-foreground">暂无可用 Runtime Provider。</p> : null}
```

with:

```tsx
      <RadioGroup onValueChange={onSelectRuntime} value={draft.runtime_binding}>
        <div className="grid gap-3">
          {availableRuntimeOptions.map((option) => (
            <RuntimeOption
              key={runtimeBinding(option)}
              onSelectRuntime={onSelectRuntime}
              option={option}
            />
          ))}
        </div>
      </RadioGroup>
      {availableRuntimeOptions.length === 0 ? (
        <p className="rounded-md border border-dashed bg-muted/30 p-3 text-sm text-muted-foreground">
          当前团队没有可绑定的 Runtime Provider，请检查 Runtime 在线状态、Provider 健康状态或团队运行策略。
        </p>
      ) : null}
      {unavailableRuntimeOptions.length > 0 ? <UnavailableRuntimeList options={unavailableRuntimeOptions} /> : null}
```

- [ ] **Step 2: Make `RuntimeOption` assume it receives only available options**

Replace the full `RuntimeOption` function with:

```tsx
function RuntimeOption({
  onSelectRuntime,
  option,
}: {
  onSelectRuntime: (runtimeBindingValue: string) => void;
  option: DigitalEmployeeRuntimeProviderOption;
}) {
  const label = `${option.runtime_name} / ${option.provider_type}`;
  const binding = runtimeBinding(option);

  return (
    <label
      className="flex cursor-pointer items-start gap-3 rounded-md border p-3 text-sm"
      onClick={(event) => {
        event.preventDefault();
        onSelectRuntime(binding);
      }}
    >
      <RadioGroupItem value={binding} />
      <span className="min-w-0 flex-1">
        <span className="block font-medium">{label}</span>
        <span className="mt-1 block text-muted-foreground">
          {option.node_id} · {option.runtime_status} · {option.provider_status} · {option.current_load}/{option.max_slots}
        </span>
      </span>
    </label>
  );
}
```

Add this `UnavailableRuntimeList` function directly after `RuntimeOption`. It renders unavailable runtimes as plain text (not radios or labels), so they stay visible for diagnosability without being selectable:

```tsx
function UnavailableRuntimeList({ options }: { options: DigitalEmployeeRuntimeProviderOption[] }) {
  return (
    <section className="rounded-md border border-dashed bg-muted/20 p-3">
      <h3 className="text-xs font-medium text-muted-foreground">暂不可绑定的 Runtime</h3>
      <div className="mt-2 grid gap-2">
        {options.map((option) => (
          <div className="rounded-md border bg-background/60 p-3 text-sm opacity-80" key={runtimeBinding(option)}>
            <span className="block font-medium text-muted-foreground">
              {option.runtime_name} / {option.provider_type}
            </span>
            <span className="mt-1 block text-xs text-muted-foreground">
              {option.node_id} · {option.runtime_status} · {option.provider_status}
            </span>
            {option.disabled_reason ? (
              <span className="mt-1 block text-xs text-destructive">{option.disabled_reason}</span>
            ) : null}
          </div>
        ))}
      </div>
    </section>
  );
}
```

- [ ] **Step 3: Prevent submission from resolving unavailable Runtime options**

Replace `findRuntimeOption` with:

```tsx
function findRuntimeOption(options: DigitalEmployeeCreateOptions | undefined, runtimeBindingValue: string) {
  return options?.runtime_provider_options.find((option) => option.available && runtimeBinding(option) === runtimeBindingValue);
}
```

- [ ] **Step 4: Run the focused tests**

Run:

```bash
corepack pnpm --filter ./apps/web run test -- src/features/employees/create.test.tsx
```

Expected: PASS for all tests in `src/features/employees/create.test.tsx`.

- [ ] **Step 5: Commit the Runtime filtering change**

Run:

```bash
git add apps/web/src/features/employees/create.tsx apps/web/src/features/employees/create.test.tsx
git commit -m "fix(web): show only bindable runtime provider options"
```

Expected: commit succeeds and includes only `create.tsx` and `create.test.tsx`.

## Task 5: Final Verification And Real-Chain Smoke

**Files:**

- Modify if required by project policy: `CHANGELOG.md`
- Inspect: `apps/web/src/features/employees/create.tsx`
- Inspect: `apps/web/src/features/employees/create.test.tsx`

- [ ] **Step 1: Run the Web test gate**

Run:

```bash
corepack pnpm --filter ./apps/web run test -- src/features/employees/create.test.tsx
```

Expected: PASS. Record the final passing test count in the implementation handoff.

- [ ] **Step 2: Run the whitespace gate**

Run:

```bash
git diff --check
```

Expected: no output and exit code 0.

- [ ] **Step 3: Inspect local service status before real-chain smoke**

Run:

```bash
scripts/dev-services.sh status
```

Expected: report whether Web and Control Plane are running. If either service is down, start or restart only the missing service with the repo script before browser/API smoke:

```bash
scripts/dev-services.sh start
```

If the script reports an unmanaged process on the required port, inspect it and do not kill it without user approval.

- [ ] **Step 4: Verify the create-options route returns real Runtime provider data**

Use the same authenticated browser session or repo-local API smoke method already used for Web flows in this workspace. The evidence must show a real response from:

```http
GET /api/v1/digital-employees/create-options?team_id={realTeamId}
```

Expected: HTTP 200 with `runtime_provider_options` present. The response may contain zero available candidates; if so, record the exact `creation_checks` message and treat the UI empty state as the expected real-chain result.

- [ ] **Step 5: Browser-smoke the visible creation flow**

Open the running Web app and navigate to the digital employee creation page. Verify these user-visible facts against the real running page:

- `空白自定义` is disabled and marked `暂未开放`.
- Template cards do not show a Provider metric.
- Entering configuration shows `已选模板` and `更换模板`.
- The full template list is not shown in configuration mode.
- The Runtime step lists only currently bindable Runtime Provider options as selectable radios; any unavailable runtimes appear under `暂不可绑定的 Runtime` with their reason, and the empty state text shows when none are bindable.

Expected: the running page reflects the current checkout, not stale compiled assets. If stale behavior appears, restart `web` through `scripts/dev-services.sh restart web` and repeat this step.

- [ ] **Step 6: Add a changelog entry only if implementation policy requires it**

If this UI fix is being delivered as completed feature work in this repo, add one timestamped entry to `CHANGELOG.md` using:

```bash
TZ=Asia/Shanghai date '+%Y-%m-%d %H:%M'
```

Entry text:

```markdown
- YYYY-MM-DD HH:MM 修正数字员工创建向导：模板不再展示 Provider 决策，配置页改为已选模板摘要，运行步骤只展示真实可绑定 Runtime Provider。
```

Replace `YYYY-MM-DD HH:MM` with the command output.

- [ ] **Step 7: Final status check**

Run:

```bash
git status --short
```

Expected: only the intended implementation files are modified or staged. Unrelated pre-existing worktree changes must remain untouched.

- [ ] **Step 8: Commit final verification or changelog changes**

If Task 5 added `CHANGELOG.md`, commit it with the implementation files not already committed:

```bash
git add CHANGELOG.md
git commit -m "docs: record digital employee create flow refinement"
```

If no files changed in Task 5, do not create an empty commit.

## Self-Review Checklist

- Spec coverage:
  - Disabled blank custom and other undeveloped paths: Task 1 tests, Task 2 implementation.
  - Template no longer decides Provider: Task 1 tests, Task 2 implementation.
  - Configure page selected-template summary: Task 1 tests, Task 3 implementation.
  - Change-template confirm truthfully resets the draft: Task 1 cancel + accept tests, Task 3 `requestTemplateChange` reset.
  - Runtime lists bindable options and surfaces unavailable ones with reasons: Task 1 tests, Task 4 implementation.
  - No backend contract changes: file structure and all task file lists exclude backend and contracts.
  - Verification boundary: Task 5 separates local tests from real-chain smoke.
- Marker scan:
  - No forbidden planning markers or open-ended test instructions.
  - Each code-changing step includes concrete code or exact replacement instructions.
- Type consistency:
  - `RuntimeAvailabilityMode`, `runtimeAvailability`, `availableRuntimeOptions`, `unavailableRuntimeOptions`, `UnavailableRuntimeList`, `SelectedTemplateSummary`, `requestTemplateChange`, and `draftTouched` use the same names across tests and implementation.
