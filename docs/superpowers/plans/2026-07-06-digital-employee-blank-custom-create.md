# Digital Employee Blank Custom Create Implementation Plan
> 复核状态：与配对spec相同——CHANGELOG 2026-07-06 16:41记录数字员工创建页开放空白自定义路径；metadata.creation_mode已实现

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Open the `空白自定义` digital-employee creation path while reusing existing employee types, team governance, create-options, and the current four-step create wizard.

**Architecture:** This is a Web-only change. `apps/web/src/features/employees/create.tsx` gains an explicit creation mode, a blank-custom employee-type selector, and mode-aware summaries/submission metadata. `apps/web/src/features/employees/create.test.tsx` drives the implementation with focused browser tests over the existing fixture and fetch mock.

**Tech Stack:** React, TypeScript, TanStack Query, TanStack Router, shadcn/Radix primitives, Vitest Browser, `corepack pnpm --filter ./apps/web run test -- src/features/employees/create.test.tsx`.

---

## File Structure

- Modify: `apps/web/src/features/employees/create.test.tsx`
  - Replace the old assertion that `空白自定义` is disabled.
  - Add blank-custom helper navigation.
  - Add regression tests for blank-custom employee-type selection, empty capability defaults, submission body, confirm summary, and dirty path switching.
- Modify: `apps/web/src/features/employees/create.tsx`
  - Add `CreationMode` and `creation_mode` to `WizardDraft`.
  - Make `CreationPathPanel` controlled and open `空白自定义`.
  - Add `BlankCustomSelectionPanel` and `EmployeeTypeTableRow`.
  - Add `applyBlankTypeDefaults`.
  - Make selected summary, preflight copy, confirm copy, and submit body mode-aware.
- Do not modify:
  - `contracts/control-plane/openapi.yaml`
  - `apps/control-plane/**`
  - `apps/runtime-agent/**`
  - database migrations

## Spec Coverage Map

- Open `空白自定义` path: Task 1 failing entry test, Task 2 controlled `CreationPathPanel`.
- Require existing `employee_type`: Task 1 helper and selector test, Task 2 `BlankCustomSelectionPanel`.
- Do not inject template capabilities or policy overrides: Task 1 empty-capability test, Task 2 `applyBlankTypeDefaults`, Task 4 submission assertion.
- Reuse four-step wizard: Task 1 `enterBlankCustomConfiguration`, Task 3 mode-aware summary only, no separate wizard.
- Keep current create contract: Task 4 uses existing POST body with optional `metadata`; no OpenAPI/backend tasks exist.
- Preserve template path behavior: Task 5 template defaults regression.
- Verify locally and through real path: Task 5 focused web test and `git diff --check`, Task 6 browser/API smoke plus completion gate.

## Task 1: Add Failing Tests For Blank-Custom Entry And Selection

**Files:**
- Modify: `apps/web/src/features/employees/create.test.tsx`

- [ ] **Step 1: Replace the disabled blank-custom path test**

Replace the existing test named `marks unavailable creation paths and blank custom entry points as disabled` with:

```tsx
  it("opens the blank-custom employee type selector while keeping copy and clone disabled", async () => {
    const screen = await renderCreateEmployeeView();

    await expect.element(screen.getByRole("button", { name: /^从专业模板创建/ })).toBeEnabled();
    await expect.element(screen.getByRole("button", { name: /^空白自定义/ })).toBeEnabled();
    await expect.element(screen.getByRole("button", { name: /^从团队角色复制/ })).toBeDisabled();
    await expect.element(screen.getByRole("button", { name: /^从历史员工克隆/ })).toBeDisabled();

    await userEvent.click(screen.getByRole("button", { name: /^空白自定义/ }));

    await expect.element(screen.getByRole("heading", { name: "选择员工类型" })).toBeVisible();
    await expect.element(screen.getByText("员工类型用于后端治理校验；空白自定义不会自动注入模板推荐能力。")).toBeVisible();
    await expect.element(screen.getByRole("button", { name: "已选择数据库管理员类型" })).toBeVisible();
    expect(document.body.textContent).not.toContain("选择内置模板");
    expect(document.body.textContent).not.toContain("员工画像蓝图");
  });
```

- [ ] **Step 2: Run the focused test and confirm it fails**

Run:

```bash
corepack pnpm --filter ./apps/web run test -- src/features/employees/create.test.tsx -t "opens the blank-custom employee type selector"
```

Expected: FAIL because `空白自定义` is still disabled and `选择员工类型` does not exist.

- [ ] **Step 3: Add a helper for entering blank-custom configuration**

Add this helper after `enterConfiguration`:

```tsx
async function enterBlankCustomConfiguration(screen: Awaited<ReturnType<typeof renderCreateEmployeeView>>) {
  await expect.element(screen.getByRole("button", { name: /^空白自定义/ })).toBeEnabled();
  await userEvent.click(screen.getByRole("button", { name: /^空白自定义/ }));
  await expect.element(screen.getByRole("heading", { name: "选择员工类型" })).toBeVisible();
  await expect.element(screen.getByRole("button", { name: "进入配置预检" })).toBeEnabled();
  await userEvent.click(screen.getByRole("button", { name: "进入配置预检" }));
  await expect.element(screen.getByRole("heading", { name: "配置预检" })).toBeVisible();
  await userEvent.click(screen.getByRole("button", { name: /继续配置/ }));
  await expect.element(screen.getByRole("heading", { name: "员工画像蓝图" })).toBeVisible();
}
```

- [ ] **Step 4: Add a test that blank custom starts with empty capabilities**

Append this test near the existing capability and create-flow tests:

```tsx
  it("starts blank-custom configuration without template-injected capabilities", async () => {
    const screen = await renderCreateEmployeeView();

    await enterBlankCustomConfiguration(screen);

    await expect.element(screen.getByText("空白自定义草稿")).toBeVisible();
    await expect.element(screen.getByLabelText("员工类型")).toHaveValue("database_admin");
    await expect.element(screen.getByLabelText("角色")).toHaveValue("database_admin");

    await userEvent.click(screen.getByRole("button", { name: "下一步" }));

    await expect.element(screen.getByRole("checkbox", { name: "incident-diagnosis" })).not.toBeChecked();
    await expect.element(screen.getByRole("checkbox", { name: "sql-review" })).not.toBeChecked();
    await expect.element(screen.getByRole("checkbox", { name: "postgres" })).not.toBeChecked();
    await expect.element(screen.getByRole("checkbox", { name: "jira.search" })).not.toBeChecked();
  });
```

- [ ] **Step 5: Run the blank-custom tests and confirm they fail**

Run:

```bash
corepack pnpm --filter ./apps/web run test -- src/features/employees/create.test.tsx -t "blank-custom"
```

Expected: FAIL because the blank-custom path and empty capability defaults are not implemented.

## Task 2: Implement Creation Mode And Blank-Custom Selection

**Files:**
- Modify: `apps/web/src/features/employees/create.tsx`

- [ ] **Step 1: Add creation mode types and state**

Update the types near the top of `create.tsx`:

```tsx
const configSteps = ["身份", "能力", "治理", "执行器"] as const;
type StepName = (typeof configSteps)[number];
type CreateFlowStep = "template" | "preflight" | "configure" | "confirm";
type CreationMode = "template" | "blank_custom";

type WizardDraft = {
  creation_mode: CreationMode;
  capability_selection: {
    enabled_external_capabilities: string[];
    enabled_mcp_servers: string[];
    enabled_skills: string[];
  };
  context_policy_override: Record<string, unknown>;
  daily_token_limit: string;
  approval_policy_override: Record<string, unknown>;
  description: string;
  employee_type: string;
  avatar_asset_id: string;
  name: string;
  risk_level: string;
  role: string;
  runtime_binding: string;
  runtime_node_id: string;
  provider_type: string;
  team_id: string;
  environment_variables: EnvironmentVariableDraftRow[];
};
```

Update `emptyDraft`:

```tsx
const emptyDraft: WizardDraft = {
  creation_mode: "template",
  approval_policy_override: {},
  capability_selection: {
    enabled_external_capabilities: [],
    enabled_mcp_servers: [],
    enabled_skills: [],
  },
  context_policy_override: {},
  daily_token_limit: "",
  description: "",
  employee_type: "",
  avatar_asset_id: "",
  name: "",
  provider_type: "",
  risk_level: "medium",
  role: "",
  runtime_binding: "",
  runtime_node_id: "",
  team_id: "",
  environment_variables: [],
};
```

- [ ] **Step 2: Gate automatic template defaults to template mode**

Replace the current `useEffect` that auto-applies `firstPreferredEmployeeType` with:

```tsx
  useEffect(() => {
    if (draft.creation_mode !== "template") return;
    const optionsData = createOptions.data;
    const employeeTypes = optionsData?.employee_types ?? [];
    const firstType = firstPreferredEmployeeType(employeeTypes);
    if (!firstType) return;
    if (!draft.employee_type || !employeeTypes.some((item) => item.type === draft.employee_type)) {
      setDraft((current) => applyTypeDefaults(current, firstType));
    }
  }, [createOptions.data, draft.creation_mode, draft.employee_type]);
```

Replace the requested-template effect with:

```tsx
  useEffect(() => {
    if (draft.creation_mode !== "template") return;
    const optionsData = createOptions.data;
    if (!requestedTemplate || !optionsData || templateQueryHandled === requestedTemplate) return;
    const requestedType = findTemplateByType(optionsData, requestedTemplate);
    setTemplateQueryHandled(requestedTemplate);
    if (!requestedType) return;
    setDraft((current) => applyTypeDefaults(current, requestedType));
  }, [createOptions.data, draft.creation_mode, requestedTemplate, templateQueryHandled]);
```

- [ ] **Step 3: Add mode-aware reset and selection functions**

Replace `selectType`, `resetDraftForTeam`, `requestTemplateChange`, and `requestTeamChange` with this set:

```tsx
  function selectType(typeValue: string) {
    if (flowStep === "configure") {
      setDraftTouched(true);
    }
    const nextType = createOptions.data?.employee_types.find((item) => item.type === typeValue);
    if (!nextType) {
      updateDraft({ employee_type: typeValue });
      return;
    }
    setDraft((current) =>
      current.creation_mode === "blank_custom"
        ? applyBlankTypeDefaults(current, nextType)
        : applyTypeDefaults(current, nextType),
    );
  }

  function resetDraftForTeam(teamId: string, creationMode: CreationMode = draft.creation_mode) {
    setErrors({});
    setStepIndex(0);
    setDraftTouched(false);
    setDraft({ ...emptyDraft, creation_mode: creationMode, team_id: teamId });
  }

  function requestCreationModeChange(nextMode: CreationMode) {
    if (nextMode === draft.creation_mode) {
      return;
    }
    if (draftTouched && !window.confirm("更换创建路径会重置当前配置草稿，是否继续？")) {
      return;
    }
    setErrors({});
    setStepIndex(0);
    setDraftTouched(false);
    setFlowStep("template");
    if (nextMode === "blank_custom") {
      const employeeTypes = createOptions.data?.employee_types ?? [];
      const firstType = firstPreferredEmployeeType(employeeTypes);
      if (firstType) {
        setDraft(applyBlankTypeDefaults({ ...emptyDraft, creation_mode: "blank_custom", team_id: draft.team_id }, firstType));
        return;
      }
    }
    setDraft({ ...emptyDraft, creation_mode: nextMode, team_id: draft.team_id });
  }

  function requestTemplateChange() {
    if (draftTouched && !window.confirm("更换创建路径会重置当前配置草稿，是否继续？")) {
      return;
    }
    resetDraftForTeam(draft.team_id, draft.creation_mode);
    setFlowStep("template");
  }

  function requestTeamChange(nextTeamId: string) {
    if (nextTeamId === draft.team_id) {
      return;
    }
    if (draftTouched && !window.confirm("更换团队会重置当前配置草稿，是否继续？")) {
      return;
    }
    resetDraftForTeam(nextTeamId, draft.creation_mode);
  }
```

- [ ] **Step 3b: Confirm `FileText` and `ChevronRight` are imported**

Before writing `CreationPathPanel` and `BlankCustomSelectionPanel`, verify that `FileText` and `ChevronRight` appear in the lucide-react import line at the top of `create.tsx`. If either is missing, add it to the existing import. Do not introduce a second import block.

- [ ] **Step 4: Make the path panel controlled**

Replace `CreationPathPanel()` with:

```tsx
function CreationPathPanel({
  creationMode,
  onSelectMode,
}: {
  creationMode: CreationMode;
  onSelectMode: (mode: CreationMode) => void;
}) {
  const paths = [
    {
      title: "从专业模板创建",
      description: "按职责模板带出默认角色、能力建议和治理策略。",
      icon: Sparkles,
      mode: "template" as const,
      badge: "推荐",
      disabled: false,
    },
    {
      title: "空白自定义",
      description: "选择底层员工类型后，逐项手动配置职责、能力和执行器。",
      icon: FileText,
      mode: "blank_custom" as const,
      badge: "可用",
      disabled: false,
    },
    {
      title: "从团队角色复制",
      description: "复用团队内已验证的角色画像和能力边界。",
      icon: ClipboardCheck,
      mode: undefined,
      badge: "暂未开放",
      disabled: true,
    },
    {
      title: "从历史员工克隆",
      description: "基于已有员工配置生成新草稿，保留审计来源。",
      icon: GitBranch,
      mode: undefined,
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
          const active = path.mode === creationMode;
          return (
            <button
              aria-pressed={active}
              className={cn(
                "rounded-md border p-3 text-left transition",
                active
                  ? "border-primary/40 bg-primary/10 text-foreground shadow-xs"
                  : "border-border/70 bg-background/80 text-muted-foreground",
                path.disabled ? "cursor-not-allowed opacity-65" : "hover:border-primary/30 hover:bg-primary/5",
              )}
              disabled={path.disabled}
              key={path.title}
              onClick={() => {
                if (path.mode) onSelectMode(path.mode);
              }}
              type="button"
            >
              <span className="flex items-start gap-2">
                <span
                  className={cn(
                    "mt-0.5 flex size-8 shrink-0 items-center justify-center rounded-md border",
                    active ? "border-primary/30 bg-primary/15 text-primary" : "bg-muted text-muted-foreground",
                  )}
                >
                  <Icon className="size-4" />
                </span>
                <span className="min-w-0 flex-1">
                  <span className="flex items-center justify-between gap-2">
                    <span className="text-sm font-medium">{path.title}</span>
                    <Badge variant={active ? "default" : "secondary"}>{path.badge}</Badge>
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

- [ ] **Step 5: Add blank-custom defaults**

Add this helper near `applyTypeDefaults`:

```tsx
function applyBlankTypeDefaults(current: WizardDraft, typeOption: DigitalEmployeeTypeOption): WizardDraft {
  return {
    ...current,
    creation_mode: "blank_custom",
    approval_policy_override: {},
    capability_selection: {
      enabled_external_capabilities: [],
      enabled_mcp_servers: [],
      enabled_skills: [],
    },
    context_policy_override: {},
    employee_type: typeOption.type,
    risk_level: stringValue(typeOption.default_approval_policy?.min_risk_for_human) || "medium",
    role: typeOption.default_role || typeOption.type,
  };
}
```

- [ ] **Step 6: Add the blank-custom selection panel**

Add these components after `TemplateSelectionPanel`:

```tsx
function BlankCustomSelectionPanel({
  draft,
  options,
  selectedTeamName,
  selectedType,
  onEnterPreflight,
  onSelectType,
}: {
  draft: WizardDraft;
  options?: DigitalEmployeeCreateOptions;
  selectedTeamName?: string;
  selectedType?: DigitalEmployeeTypeOption;
  onEnterPreflight: () => void;
  onSelectType: (value: string) => void;
}) {
  const employeeTypes = useMemo(() => orderedEmployeeTypes(options?.employee_types ?? []), [options?.employee_types]);

  return (
    <section className="@container/template flex min-w-0 flex-col overflow-hidden rounded-md border bg-card/95 shadow-xs">
      <div className="border-b p-4">
        <div className="flex flex-col gap-2 sm:flex-row sm:items-start sm:justify-between">
          <div>
            <h2 className="text-base font-semibold">选择员工类型</h2>
            <p className="mt-1 text-sm text-muted-foreground">
              员工类型用于后端治理校验；空白自定义不会自动注入模板推荐能力。
            </p>
          </div>
          <Badge variant="secondary">{employeeTypes.length} 个类型</Badge>
        </div>
      </div>
      {employeeTypes.length === 0 ? (
        <div className="m-4 flex min-h-[420px] flex-1 items-center justify-center rounded-md border bg-muted/30 p-6 text-sm text-muted-foreground">
          当前团队治理配置未返回可用员工类型。
        </div>
      ) : (
        <div className="min-h-0 flex-1 p-4">
          <div className="h-full overflow-hidden rounded-md border bg-background" data-testid="blank-type-selection-table">
            <div className="h-full max-h-[min(680px,calc(100vh-360px))] overflow-auto">
              <table className="w-full min-w-[760px] border-collapse text-sm">
                <thead className="sticky top-0 z-10 border-b bg-muted text-xs font-medium text-muted-foreground">
                  <tr>
                    <th className="w-[38%] px-3 py-2 text-left">员工类型</th>
                    <th className="w-[22%] px-3 py-2 text-left">类型标识</th>
                    <th className="w-[18%] px-3 py-2 text-left">默认角色</th>
                    <th className="w-[14%] px-3 py-2 text-left">风险建议</th>
                    <th className="w-[8%] px-3 py-2 text-right">选择</th>
                  </tr>
                </thead>
                <tbody>
                  {employeeTypes.map((typeOption) => (
                    <EmployeeTypeTableRow
                      key={typeOption.type}
                      selected={typeOption.type === draft.employee_type}
                      typeOption={typeOption}
                      onSelect={() => onSelectType(typeOption.type)}
                    />
                  ))}
                </tbody>
              </table>
            </div>
          </div>
        </div>
      )}
      <div className="border-t bg-card/95 px-4 py-3">
        <div className="grid gap-3 lg:grid-cols-[minmax(0,1fr)_auto] lg:items-center">
          <div className="min-w-0">
            <div className="flex flex-wrap items-center gap-2 text-sm">
              <span className="font-medium text-foreground">空白草稿摘要</span>
              <Badge variant="secondary">团队 {selectedTeamName || "无（租户级）"}</Badge>
              <Badge variant="secondary">类型 {selectedType?.label ?? (draft.employee_type || "未选择")}</Badge>
              <Badge variant="secondary">能力手动配置</Badge>
            </div>
            <p className="mt-2 text-sm text-muted-foreground">
              选择类型后进入同一套配置向导，能力、治理覆盖和执行器由你逐项配置。
            </p>
          </div>
          <Button disabled={!draft.employee_type} onClick={onEnterPreflight} type="button">
            进入配置预检
            <ChevronRight data-icon="inline-end" />
          </Button>
        </div>
      </div>
    </section>
  );
}

function EmployeeTypeTableRow({
  selected,
  typeOption,
  onSelect,
}: {
  selected: boolean;
  typeOption: DigitalEmployeeTypeOption;
  onSelect: () => void;
}) {
  const risk = templateRisk(typeOption);

  return (
    <tr
      className={cn(
        "border-b transition last:border-b-0 hover:bg-muted/30",
        selected ? "bg-primary/5 [box-shadow:inset_3px_0_0_var(--v3-brand)]" : "",
      )}
    >
      <td className="px-3 py-3 align-top">
        <div className="flex min-w-0 gap-3">
          <span
            className={cn(
              "mt-0.5 flex size-9 shrink-0 items-center justify-center rounded-md border",
              selected ? "border-primary/30 bg-primary/15 text-primary" : "bg-muted text-muted-foreground",
            )}
          >
            <FileText className="size-4" />
          </span>
          <div className="min-w-0">
            <div className="font-semibold">{typeOption.label}</div>
            <div className="mt-1 line-clamp-2 text-xs leading-5 text-muted-foreground">{typeOption.description}</div>
          </div>
        </div>
      </td>
      <td className="px-3 py-3 align-top">
        <div className="max-w-[220px] truncate rounded-md border bg-muted/30 px-2 py-1 font-mono text-xs">
          {typeOption.type}
        </div>
      </td>
      <td className="px-3 py-3 align-top">
        <div className="max-w-[180px] truncate rounded-md border bg-muted/30 px-2 py-1 font-mono text-xs">
          {typeOption.default_role || typeOption.type}
        </div>
      </td>
      <td className="px-3 py-3 align-top">
        <Badge
          className={cn(
            "font-mono",
            risk === "high" || risk === "critical" ? "bg-amber-100 text-amber-800" : "",
            risk === "low" || risk === "medium" ? "bg-emerald-100 text-emerald-800" : "",
          )}
          variant="secondary"
        >
          {risk}
        </Badge>
      </td>
      <td className="px-3 py-3 text-right align-top">
        <Button
          aria-label={`${selected ? "已选择" : "选择"}${typeOption.label}类型`}
          aria-pressed={selected}
          onClick={onSelect}
          size="sm"
          type="button"
          variant={selected ? "default" : "outline"}
        >
          {selected ? <Check data-icon="inline-start" /> : null}
          {selected ? "已选" : "选择"}
        </Button>
      </td>
    </tr>
  );
}
```

- [ ] **Step 7: Wire the panels into the template flow**

Replace the template-flow render branch with:

```tsx
        {flowStep === "template" ? (
          <div className="grid gap-4 xl:h-[calc(100vh-220px)] xl:min-h-[560px] xl:grid-cols-[260px_minmax(0,1fr)]">
            <CreationPathPanel creationMode={draft.creation_mode} onSelectMode={requestCreationModeChange} />

            {draft.creation_mode === "template" ? (
              <TemplateSelectionPanel
                draft={draft}
                options={createOptions.data}
                selectedTeamName={selectedTeam?.name}
                selectedType={selectedType}
                onEnterPreflight={enterPreflight}
                onSelectType={selectType}
              />
            ) : (
              <BlankCustomSelectionPanel
                draft={draft}
                options={createOptions.data}
                selectedTeamName={selectedTeam?.name}
                selectedType={selectedType}
                onEnterPreflight={enterPreflight}
                onSelectType={selectType}
              />
            )}
          </div>
        ) : null}
```

- [ ] **Step 8: Run tests added in Task 1**

Run:

```bash
corepack pnpm --filter ./apps/web run test -- src/features/employees/create.test.tsx -t "blank-custom|opens the blank-custom"
```

Expected: PASS for the blank-custom entry and empty capability tests. Some unrelated existing tests may fail because confirmation copy changed from `更换模板` to `更换创建路径`; those are handled in Task 3.

## Task 3: Make Summaries And Dirty-Change Copy Mode-Aware

**Files:**
- Modify: `apps/web/src/features/employees/create.test.tsx`
- Modify: `apps/web/src/features/employees/create.tsx`

- [ ] **Step 1: Update template-change dirty-path tests to expect creation-path copy**

In `create.test.tsx`, replace only the expectations in tests that cover the template-change or creation-path-change flow (e.g., "keeps edited configuration" / "resets the configuration" style tests that assert the `requestTemplateChange` confirm dialog). Replace:

```tsx
expect(confirm).toHaveBeenCalledWith("更换模板会重置当前配置草稿，是否继续？");
```

with:

```tsx
expect(confirm).toHaveBeenCalledWith("更换创建路径会重置当前配置草稿，是否继续？");
```

Do **not** touch team-change confirmation tests (tests that cover `requestTeamChange` / "更换团队" flow). Those tests must keep their original assertion:

```tsx
expect(confirm).toHaveBeenCalledWith("更换团队会重置当前配置草稿，是否继续？");
```

- [ ] **Step 2: Add a test for blank-custom confirm summary**

Append:

```tsx
  it("shows blank-custom source on the selected summary and confirm step", async () => {
    const screen = await renderCreateEmployeeView();

    await enterBlankCustomConfiguration(screen);
    await expect.element(screen.getByText("空白自定义草稿")).toBeVisible();
    await expect.element(screen.getByText(/底层类型：数据库管理员/)).toBeVisible();

    await userEvent.fill(screen.getByLabelText("名称"), "数据库管理员工");
    await userEvent.click(screen.getByRole("button", { name: "下一步" }));
    await userEvent.click(screen.getByRole("button", { name: "下一步" }));
    await userEvent.click(screen.getByRole("button", { name: "下一步" }));
    await enterConfirmCreation(screen);

    await expect.element(screen.getByText("空白自定义草稿")).toBeVisible();
    await expect.element(screen.getByText("数据库管理员")).toBeVisible();
  });
```

- [ ] **Step 3: Run the summary tests and confirm they fail**

Run:

```bash
corepack pnpm --filter ./apps/web run test -- src/features/employees/create.test.tsx -t "blank-custom source|change-template confirmation|team-change confirmation"
```

Expected: FAIL because `SelectedTemplateSummary`, `PreflightStep`, and `ConfirmCreationStep` still use template-only labels.

- [ ] **Step 4: Make `SelectedTemplateSummary` mode-aware**

Replace `SelectedTemplateSummary` with:

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
  const blankCustom = draft.creation_mode === "blank_custom";

  return (
    <section className="rounded-md border bg-background p-4">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <p className="text-xs font-medium uppercase tracking-wide text-muted-foreground">
            {blankCustom ? "空白自定义草稿" : "已选模板"}
          </p>
          <h2 className="mt-1 text-lg font-semibold">
            {blankCustom
              ? `底层类型：${selectedType?.label ?? draft.employee_type || "未选择类型"}`
              : selectedType?.label ?? (draft.employee_type || "未选择模板")}
          </h2>
          <p className="mt-1 text-sm text-muted-foreground">
            {blankCustom
              ? "能力、上下文覆盖、审批覆盖和执行器由你手动配置。"
              : selectedType?.description ?? "模板只作为初始草稿来源，Provider 在最后一步选择。"}
          </p>
        </div>
        <Button onClick={onChangeTemplate} type="button" variant="outline">
          <ArrowLeft data-icon="inline-start" />
          更换创建路径
        </Button>
      </div>
      <div className="mt-3 flex flex-wrap gap-1.5">
        {blankCustom ? (
          <>
            <Badge variant="secondary">默认角色 {selectedType?.default_role || draft.role || "未生成"}</Badge>
            <Badge variant="secondary">能力手动配置</Badge>
            <Badge variant="secondary">治理覆盖 0</Badge>
          </>
        ) : (
          <>
            <Badge variant="secondary">默认角色 {selectedType?.default_role || draft.role || "未生成"}</Badge>
            <Badge variant="secondary">技能 {selectedType?.recommended_skills?.length ?? 0}</Badge>
            <Badge variant="secondary">MCP {selectedType?.recommended_mcp_servers?.length ?? 0}</Badge>
          </>
        )}
      </div>
    </section>
  );
}
```

- [ ] **Step 5: Make preflight and confirm copy mode-aware**

In `PreflightStep`, change the draft summary labels to:

```tsx
            <InlineSummary label="创建路径" value={draft.creation_mode === "blank_custom" ? "空白自定义" : "专业模板"} />
            <InlineSummary
              label={draft.creation_mode === "blank_custom" ? "底层类型" : "专业模板"}
              value={selectedType?.label ?? (draft.employee_type || "未选择")}
            />
```

Keep the existing `归属团队` line before these and the existing role/risk/capability lines after these.

In `ConfirmCreationStep`, replace the first two summary rows under `身份与模板` with:

```tsx
            <InlineSummary label="归属团队" value={selectedTeamName || "无（租户级）"} />
            <InlineSummary label="创建路径" value={draft.creation_mode === "blank_custom" ? "空白自定义草稿" : "专业模板"} />
            <InlineSummary
              label={draft.creation_mode === "blank_custom" ? "底层类型" : "专业模板"}
              value={selectedType?.label ?? (draft.employee_type || "未选择")}
            />
```

- [ ] **Step 6: Run the focused summary tests**

Run:

```bash
corepack pnpm --filter ./apps/web run test -- src/features/employees/create.test.tsx -t "blank-custom source|keeps edited configuration|resets the configuration"
```

Expected: PASS.

## Task 4: Submit Blank-Custom Payload Without Template Defaults

**Files:**
- Modify: `apps/web/src/features/employees/create.test.tsx`
- Modify: `apps/web/src/features/employees/create.tsx`

- [ ] **Step 1: Allow the fetch mock to assert a blank-custom create body**

In `create.test.tsx`, add this type after `type FetchMockCall`:

```tsx
type ExpectedCreateBody = Record<string, unknown>;
```

Update `createWizardFetcher` parameters:

```tsx
function createWizardFetcher({
  expectedCreateBody,
  expectedEnvironmentVariables,
  expectedProviderType = "codex",
  expectedRuntimeNodeId = "33333333-3333-4333-8333-333333333333",
  expectedRuntimeNodeIdSubmitted = false,
  expectedTeamId,
  runtimeAvailability = "all",
  runtimeCount = 1,
  sameRuntimeNodeProviders = false,
  includePolicyExcludedProvider = false,
  includeFrontendTemplate = false,
  teams = [team],
}: {
  expectedCreateBody?: ExpectedCreateBody;
  expectedEnvironmentVariables?: Array<{ name: string; value: string; sensitive: boolean }>;
  expectedProviderType?: string;
  expectedRuntimeNodeId?: string;
  expectedRuntimeNodeIdSubmitted?: boolean;
  expectedTeamId?: string | undefined;
  runtimeAvailability?: RuntimeAvailabilityMode;
  runtimeCount?: 1 | 2;
  sameRuntimeNodeProviders?: boolean;
  includePolicyExcludedProvider?: boolean;
  includeFrontendTemplate?: boolean;
  teams?: Array<typeof team>;
} = {}) {
```

Inside the POST branch, replace the current exact `expect(bodyWithoutBudgetPolicy).toEqual(...)` block with:

```tsx
      const defaultExpectedBody = {
        ...(expectedTeamId ? { team_id: expectedTeamId } : {}),
        employee_type: "database_admin",
        name: "数据库管理员工",
        role: "database_admin",
        description: "负责生产数据库变更和恢复验证",
        risk_level: "high",
        avatar_asset_id: avatarAsset.id,
        role_profile: {
          employee_type: "database_admin",
          role: "database_admin",
          title: "数据库管理员",
        },
        capability_selection: {
          enabled_skills: ["sql-review"],
          enabled_mcp_servers: ["postgres"],
          enabled_external_capabilities: ["jira.search"],
        },
        context_policy_override: { max_refs: 8 },
        approval_policy_override: { min_risk_for_human: "high" },
        output_contract_addendum: {},
        ...(expectedRuntimeNodeIdSubmitted ? { runtime_node_id: expectedRuntimeNodeId } : {}),
        provider_type: expectedProviderType,
        session_policy: { mode: "reuse_latest" },
        workspace_policy: {},
        environment_variables: expectedEnvironmentVariables ?? [],
      };
      expect(bodyWithoutBudgetPolicy).toEqual(expectedCreateBody ?? defaultExpectedBody);
```

- [ ] **Step 2: Add a blank-custom submission test**

Append:

```tsx
  it("submits blank-custom creation without template-injected capabilities or policy overrides", async () => {
    const fetcher = createWizardFetcher({
      expectedCreateBody: {
        employee_type: "database_admin",
        name: "数据库管理员工",
        role: "database_admin",
        description: "负责生产数据库变更和恢复验证",
        risk_level: "high",
        avatar_asset_id: avatarAsset.id,
        role_profile: {
          employee_type: "database_admin",
          role: "database_admin",
          title: "数据库管理员",
        },
        capability_selection: {
          enabled_skills: [],
          enabled_mcp_servers: [],
          enabled_external_capabilities: [],
        },
        context_policy_override: {},
        approval_policy_override: {},
        output_contract_addendum: {},
        provider_type: "codex",
        session_policy: { mode: "reuse_latest" },
        workspace_policy: {},
        environment_variables: [],
        metadata: { creation_mode: "blank_custom" },
      },
    });
    const screen = await renderCreateEmployeeView(fetcher);

    await enterBlankCustomConfiguration(screen);
    await userEvent.fill(screen.getByLabelText("名称"), "数据库管理员工");
    await userEvent.fill(screen.getByLabelText("描述"), "负责生产数据库变更和恢复验证");
    await userEvent.click(screen.getByRole("button", { name: "下一步" }));
    await userEvent.click(screen.getByRole("button", { name: "下一步" }));
    await userEvent.click(screen.getByRole("button", { name: "下一步" }));
    await enterConfirmCreation(screen);
    await userEvent.click(screen.getByRole("button", { name: "确认创建" }));

    const createCall = findCreateEmployeePost(fetcher);
    expect(createCall).toBeTruthy();
  });
```

- [ ] **Step 3: Run the submission test and confirm it fails**

Run:

```bash
corepack pnpm --filter ./apps/web run test -- src/features/employees/create.test.tsx -t "submits blank-custom creation"
```

Expected: FAIL because the create mutation does not include blank-custom metadata and still sends template defaults if blank mode is not fully wired.

- [ ] **Step 4: Make the create mutation mode-aware**

In `CreateEmployeeView`, add this local constant near `selectedType`:

```tsx
  const blankCustom = draft.creation_mode === "blank_custom";
```

In the create mutation body, replace `role_profile` and add `metadata`:

```tsx
          role_profile: {
            employee_type: draft.employee_type,
            role: draft.role.trim(),
            title: selectedType?.label ?? draft.employee_type,
          },
          ...(blankCustom ? { metadata: { creation_mode: "blank_custom" } } : {}),
```

Keep the existing `capability_selection`, `context_policy_override`, and `approval_policy_override` fields sourced from `draft`; the blank-custom behavior comes from `applyBlankTypeDefaults`.

- [ ] **Step 5: Run the submission test**

Run:

```bash
corepack pnpm --filter ./apps/web run test -- src/features/employees/create.test.tsx -t "submits blank-custom creation"
```

Expected: PASS.

## Task 5: Preserve Template Path Behavior And Run Full Focused Suite

**Files:**
- Modify: `apps/web/src/features/employees/create.test.tsx`
- Modify: `apps/web/src/features/employees/create.tsx`

- [ ] **Step 1: Add a regression test proving template defaults still apply**

Append:

```tsx
  it("keeps template creation seeded with template capability and policy defaults", async () => {
    const fetcher = createWizardFetcher();
    const screen = await renderCreateEmployeeView(fetcher);

    await enterConfiguration(screen);
    await userEvent.fill(screen.getByLabelText("名称"), "数据库管理员工");
    await userEvent.fill(screen.getByLabelText("描述"), "负责生产数据库变更和恢复验证");
    await userEvent.click(screen.getByRole("button", { name: "下一步" }));

    await expect.element(screen.getByRole("checkbox", { name: "sql-review" })).toBeChecked();
    await expect.element(screen.getByRole("checkbox", { name: "postgres" })).toBeChecked();
    await expect.element(screen.getByRole("checkbox", { name: "jira.search" })).toBeChecked();

    await userEvent.click(screen.getByRole("button", { name: "下一步" }));
    await userEvent.click(screen.getByRole("button", { name: "下一步" }));
    await enterConfirmCreation(screen);
    await userEvent.click(screen.getByRole("button", { name: "确认创建" }));

    const createCall = findCreateEmployeePost(fetcher);
    expect(createCall).toBeTruthy();
    const body = JSON.parse(String(createCall?.[1]?.body));
    expect(body.capability_selection).toEqual({
      enabled_skills: ["sql-review"],
      enabled_mcp_servers: ["postgres"],
      enabled_external_capabilities: ["jira.search"],
    });
    expect(body.context_policy_override).toEqual({ max_refs: 8 });
    expect(body.approval_policy_override).toEqual({ min_risk_for_human: "high" });
    expect(body.metadata).toBeUndefined();
  });
```

- [ ] **Step 2: Run the full create-flow test file**

Run:

```bash
corepack pnpm --filter ./apps/web run test -- src/features/employees/create.test.tsx
```

Expected: PASS. If a test fails due to accessible-name changes, update the test to use the final Chinese label that a user sees. Do not loosen tests to generic text searches when a role-based query is possible.

- [ ] **Step 3: Run whitespace validation**

Run:

```bash
git diff --check
```

Expected: no output and exit code 0.

## Task 6: Real Browser/API Smoke After Implementation

**Files:**
- No planned file edits.

- [ ] **Step 1: Check local service status**

Run:

```bash
scripts/dev-services.sh status
```

Expected: status output for Web and Control Plane. If either is stopped, start or restart only the missing service:

```bash
scripts/dev-services.sh start control-plane
scripts/dev-services.sh start web
```

- [ ] **Step 2: Confirm Control Plane create-options responds through a real authenticated path**

Use the existing dev login seed if no browser session exists:

```bash
curl -i -sS \
  -c /tmp/superteam-codex-cookie.txt \
  -H 'content-type: application/json' \
  -d '{"username":"admin","password":"admin"}' \
  http://127.0.0.1:8081/api/auth/login
```

Expected: HTTP 200 or 204 with a session cookie.

Then run:

```bash
curl -sS \
  -b /tmp/superteam-codex-cookie.txt \
  'http://127.0.0.1:8081/api/v1/digital-employees/create-options' | jq '.employee_types | length'
```

Expected: a number greater than 0.

- [ ] **Step 3: Use the browser to exercise `/employees/new`**

Use the Codex Chrome/browser plugin as required by project rules. Open the running Web URL, log in if needed, then verify:

- `空白自定义` is enabled.
- Clicking it shows `选择员工类型`.
- Selecting the default type allows entering `配置预检`.
- The configuration summary shows `空白自定义草稿`.
- Capability checkboxes are initially unchecked.
- Provider selection reflects real current create-options.

If a real Provider is available, complete creation and verify the app navigates to the new employee detail page. If no Provider is available, record the visible `creation_checks` or Provider explanation and do not claim full create success.

- [ ] **Step 4: Final completion gate**

Before claiming complete, run:

```bash
corepack pnpm --filter ./apps/web run test -- src/features/employees/create.test.tsx
git diff --check
```

Then use `$superteam-completion-check` for the project completion gate. The final report must distinguish:

- Component/browser test evidence
- Real API/browser smoke evidence
- Any blocker such as no available Runtime Provider
