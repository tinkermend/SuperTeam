# Digital Employee Create Redesign Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement the approved digital-employee create redesign so blank custom creation defines a custom identity directly, Provider type is required, team governance blockers are business-readable, and capabilities are split into team-inherited and employee extension sections.

**Architecture:** Keep Control Plane as the source of employee type, Provider, team governance, and capability policy validation. Keep Web as the orchestration surface: it gathers create-options, existing team skill/MCP bindings, visible skill/MCP registries, and submits only employee extension selections. Runtime online state remains an advisory create-page signal; project runtime readiness remains the execution fact source.

**Tech Stack:** Go Control Plane service/handler tests, React/TypeScript/TanStack Query create page, existing Web API clients, Vitest Browser, OpenAPI generation only if a response schema changes, repo scripts through `corepack pnpm`.

## Global Constraints

- Implement the approved spec `docs/superpowers/specs/2026-07-07-digital-employee-create-redesign.md`.
- `空白自定义` directly creates a custom digital employee identity; the UI must not ask the user to select an employee type.
- The internal employee type for new blank custom requests is `custom_agent`.
- User-visible `角色` copy in this flow becomes `职责定位`; it still maps to backend `role` and `role_profile.role`.
- User-visible `Provider 偏好` copy becomes `Provider 类型` or `执行器类型`.
- Provider type is required and limited to `codex`, `opencode`, and `claude-code`.
- Frontend submits `claude-code`; backend may normalize legacy `claude_code` to `claude-code`, but new responses and requests use `claude-code`.
- Team selection is optional; no team means tenant-level default governance.
- A selected team without active governance config returns or is handled as `team_governance_config_required`.
- Team-inherited capabilities are read-only and must not be submitted as employee extension capabilities.
- Employee extension capabilities must come from the logged-in user's visible capabilities and be filtered by team policy.
- Runtime online state does not block employee creation when a Provider type is selectable.
- Preserve unrelated dirty worktree changes; stage and commit only files touched by the task.
- Frontend layout/styling must follow `DESIGN.md` and existing `apps/web/src/components/superteam/` / `apps/web/src/components/ui/` patterns.
- Real end-to-end verification is required before claiming the feature is complete unless explicitly blocked by services, auth, DB, or external Provider/runtime state.

---

## File Structure

Modify:

- `apps/control-plane/internal/employee/employee_types.go`
  - Add internal `custom_agent` employee type definition and keep it in `DefaultEmployeeTypeDefinitions`.
- `apps/control-plane/internal/employee/service.go`
  - Normalize Provider aliases.
  - Preserve Provider required validation.
  - Keep team governance checks.
  - Ensure `custom_agent` works with type lookup, defaults, team allowlist, and initial config validation.
- `apps/control-plane/internal/employee/service_test.go`
  - Add unit tests for `custom_agent`, Provider normalization, missing Provider, unsupported Provider, team Provider policy, and missing team governance config.
- `apps/control-plane/internal/employee/handler.go`
  - Return structured JSON for `ErrEffectiveConfigRequired` with `code: "team_governance_config_required"` and HTTP 422.
- `apps/control-plane/internal/api/employee_routes_test.go`
  - Add route-level assertions for structured 422 and create-options JSON compatibility.
- `apps/web/src/lib/api/client.ts`
  - Preserve structured error code/detail on JSON error responses.
- `apps/web/src/lib/api/employees.ts`
  - Add typed error helper for create-options team governance blockers if needed by the page.
  - Keep create-options shape compatible with existing fields.
- `apps/web/src/lib/api/employees.test.ts`
  - Assert structured error code is available to callers.
- `apps/web/src/features/employees/create.tsx`
  - Remove blank-custom employee-type selection.
  - Set `employee_type: "custom_agent"` for blank custom.
  - Rename labels/copy.
  - Add team governance blocker UI.
  - Split capability configuration into team-inherited and employee-extension sections.
  - Make Provider type a required single-select using canonical values.
- `apps/web/src/features/employees/create.test.tsx`
  - Replace old blank-custom/type-selector expectations.
  - Add Provider, team governance, capability split, and submit-body regression coverage.

Possibly modify:

- `contracts/control-plane/openapi.yaml`
  - Only if the implementation chooses to document structured 422 response schemas in the contract during this plan.
- Generated files under `apps/control-plane/internal/api/gen/`
  - Only after `corepack pnpm generate:control-plane` if OpenAPI changes.

Do not modify:

- `apps/runtime-agent/**`
- Project runtime placement/readiness semantics.
- Digital employee direct Runtime binding or execution-instance create semantics.
- Existing employee detail effective capability source tags except for narrow copy/test alignment if a test proves drift.

## Interfaces

- Backend accepted Provider values:

```go
var supportedDigitalEmployeeProviderTypes = map[string]struct{}{
	"claude-code": {},
	"opencode":    {},
	"codex":       {},
}
```

- Backend Provider normalization contract:

```go
func normalizeProviderType(value string) string {
	normalized := strings.ToLower(strings.TrimSpace(value))
	if normalized == "claude_code" {
		return "claude-code"
	}
	return normalized
}
```

- Blank-custom submit body fragment:

```json
{
  "employee_type": "custom_agent",
  "metadata": { "creation_mode": "blank_custom" },
  "role_profile": {
    "employee_type": "custom_agent",
    "role": "用户填写的职责定位",
    "title": "自定义身份"
  }
}
```

- Structured team governance error response:

```json
{
  "code": "team_governance_config_required",
  "message": "employee effective config required: active team governance config is required"
}
```

- Web capability split source:
  - Inherited skills: `listTeamSkills(options, teamId)` from `apps/web/src/lib/api/skills.ts`.
  - Inherited MCP: `listTeamMcpBindings(options, teamId)` from `apps/web/src/lib/api/capabilities.ts`.
  - Extension candidates: `createOptions.data.capability_options.skills`, `mcp_servers`, and `external_capabilities`, minus inherited keys.
  - Team-less inherited capabilities: empty arrays.

---

### Task 1: Backend Custom Type And Provider Semantics

**Files:**
- Modify: `apps/control-plane/internal/employee/service_test.go`
- Modify: `apps/control-plane/internal/employee/employee_types.go`
- Modify: `apps/control-plane/internal/employee/service.go`
- Modify: `apps/web/src/features/employees/template-utils.ts`

**Interfaces:**
- Consumes: existing `EmployeeTypeDefinitionByType`, `normalizeCreateDigitalEmployeeRequest`, `CreateDigitalEmployee`, `GetCreateOptions`.
- Produces: `custom_agent` type and canonical Provider normalization for later handler/Web tasks.

- [ ] **Step 1: Add failing custom-agent definition test**

Add this test near `TestDefaultEmployeeTypeDefinitions...` in `apps/control-plane/internal/employee/service_test.go`:

```go
func TestCustomAgentEmployeeTypeDefinitionIsAvailableForBlankCustomCreate(t *testing.T) {
	definition, ok := EmployeeTypeDefinitionByType(" custom_agent ")
	require.True(t, ok)
	require.Equal(t, "custom_agent", definition.Type)
	require.Equal(t, "自定义数字员工", definition.Label)
	require.Empty(t, definition.DefaultRole)
	require.Empty(t, definition.RecommendedSkills)
	require.Empty(t, definition.RecommendedMCPServers)
	require.Empty(t, definition.DefaultCapabilitySelection)
	require.Contains(t, definition.Metadata, "creation_mode")
	require.Equal(t, "blank_custom", definition.Metadata["creation_mode"])
}
```

- [ ] **Step 2: Run the failing custom-agent test**

Run:

```bash
go test ./apps/control-plane/internal/employee -run TestCustomAgentEmployeeTypeDefinitionIsAvailableForBlankCustomCreate -count=1
```

Expected: FAIL with `ok` false because `custom_agent` is not registered.

- [ ] **Step 3: Add the custom-agent employee type**

Insert this as the first item in `defaultEmployeeTypeDefinitions` in `apps/control-plane/internal/employee/employee_types.go`:

```go
{
	Type:                  "custom_agent",
	Label:                 "自定义数字员工",
	Description:           "由用户直接定义职责定位、能力扩展、治理策略和执行器类型的自定义数字员工。",
	DefaultRole:           "",
	RecommendedSkills:     []string{},
	RecommendedMCPServers: []string{},
	RecommendedProviderTypes: []string{
		"codex",
		"opencode",
		"claude-code",
	},
	DefaultCapabilitySelection:   map[string]any{},
	DefaultContextPolicyOverride: map[string]any{},
	DefaultApprovalPolicy:        map[string]any{},
	Metadata: map[string]any{
		"creation_mode": "blank_custom",
		"system_type":   true,
	},
},
```

- [ ] **Step 4: Verify the custom-agent definition test passes**

Run:

```bash
go test ./apps/control-plane/internal/employee -run TestCustomAgentEmployeeTypeDefinitionIsAvailableForBlankCustomCreate -count=1
```

Expected: PASS.

- [ ] **Step 5: Add failing Provider normalization tests**

Add this test near `TestCreateDigitalEmployeeProviderTypeMustBeSupportedEvenWithoutTeamAllowlist`:

```go
func TestCreateDigitalEmployeeNormalizesProviderTypeAliases(t *testing.T) {
	svc, repo, _, req := newCreateDigitalEmployeeReadyFixture(t)
	req.ProviderType = " CLAUDE_CODE "
	teamConfigID := repo.currentTeamConfigByTeam[*req.TeamID]
	teamConfig := repo.teamConfigs[teamConfigID]
	teamConfig.CapabilityPolicy["allowed_provider_types"] = []any{"claude-code"}
	teamConfig.RuntimeScopePolicy = map[string]any{"allowed_provider_types": []any{"claude-code"}}
	repo.teamConfigs[teamConfigID] = teamConfig

	created, err := svc.CreateDigitalEmployee(context.Background(), req)

	require.NoError(t, err)
	require.Equal(t, "claude-code", created.ProviderType)
	require.Equal(t, "claude-code", repo.employees[created.ID].ProviderType)
}

func TestCreateDigitalEmployeeRejectsBlankProviderType(t *testing.T) {
	svc, repo, dispatcher, req := newCreateDigitalEmployeeReadyFixture(t)
	req.ProviderType = " "

	_, err := svc.CreateDigitalEmployee(context.Background(), req)

	require.ErrorIs(t, err, ErrInvalidInput)
	require.Contains(t, err.Error(), "provider_type is required")
	require.Empty(t, dispatcher.commands)
	require.Empty(t, repo.employees)
	require.Empty(t, repo.commandReceipts)
}
```

- [ ] **Step 6: Run the failing Provider normalization tests**

Run:

```bash
go test ./apps/control-plane/internal/employee -run 'TestCreateDigitalEmployee(NormalizesProviderTypeAliases|RejectsBlankProviderType)' -count=1
```

Expected: `RejectsBlankProviderType` passes or already passes; `NormalizesProviderTypeAliases` fails until `normalizeProviderType` lowercases and maps `claude_code`.

- [ ] **Step 7: Implement canonical Provider normalization**

Replace `normalizeProviderType` in `apps/control-plane/internal/employee/service.go` with:

```go
func normalizeProviderType(value string) string {
	normalized := strings.ToLower(strings.TrimSpace(value))
	if normalized == "claude_code" {
		return "claude-code"
	}
	return normalized
}
```

- [ ] **Step 8: Add create-options custom-agent allowlist tests**

Add these tests near existing create-options tests:

```go
func TestGetCreateOptionsIncludesCustomAgentForTeamLessCreate(t *testing.T) {
	svc, repo, tenantID, _ := newCreateOptionsTestService(t, map[string]any{}, map[string]any{})
	repo.currentTeamConfigByTeam = map[uuid.UUID]uuid.UUID{}

	options, err := svc.GetCreateOptions(context.Background(), CreateOptionsRequest{TenantID: tenantID})

	require.NoError(t, err)
	require.True(t, employeeTypeOptionExists(options.EmployeeTypes, "custom_agent"))
}

func TestGetCreateOptionsHonorsTeamCustomAgentAllowlist(t *testing.T) {
	svc, _, tenantID, teamID := newCreateOptionsTestService(t, map[string]any{
		"allowed_employee_types": []any{"custom_agent"},
		"allowed_provider_types": []any{"codex"},
	}, map[string]any{})

	options, err := svc.GetCreateOptions(context.Background(), CreateOptionsRequest{
		TenantID: tenantID,
		TeamID:   &teamID,
	})

	require.NoError(t, err)
	require.Len(t, options.EmployeeTypes, 1)
	require.Equal(t, "custom_agent", options.EmployeeTypes[0].Type)
}

func employeeTypeOptionExists(items []EmployeeTypeDefinition, employeeType string) bool {
	for _, item := range items {
		if item.Type == employeeType {
			return true
		}
	}
	return false
}
```

- [ ] **Step 9: Run the backend employee service test slice**

Run:

```bash
go test ./apps/control-plane/internal/employee -run 'Test(CustomAgentEmployeeTypeDefinitionIsAvailableForBlankCustomCreate|CreateDigitalEmployeeNormalizesProviderTypeAliases|CreateDigitalEmployeeRejectsBlankProviderType|GetCreateOptionsIncludesCustomAgentForTeamLessCreate|GetCreateOptionsHonorsTeamCustomAgentAllowlist|CreateDigitalEmployeeRejectsProviderOutsideTeamPolicyBeforeCreatingFacts|CreateDigitalEmployeeProviderTypeMustBeSupportedEvenWithoutTeamAllowlist)' -count=1
```

Expected: PASS.

- [ ] **Step 9.5: Filter system_type entries from orderedEmployeeTypes**

`custom_agent` is now in `defaultEmployeeTypeDefinitions` and will appear in `create-options` `employee_types`. Without a filter it would show up in `TemplateSelectionPanel` and `BlankCustomSelectionPanel`. The `DigitalEmployeeTypeOption` type already carries `metadata?: Record<string, unknown>` so no type change is required.

In `apps/web/src/features/employees/template-utils.ts`, replace:

```ts
export function orderedEmployeeTypes(employeeTypes: DigitalEmployeeTypeOption[]) {
  return [...employeeTypes].sort((left, right) => {
```

with:

```ts
export function orderedEmployeeTypes(employeeTypes: DigitalEmployeeTypeOption[]) {
  return [...employeeTypes]
    .filter((item) => !item.metadata?.["system_type"])
    .sort((left, right) => {
```

Leave the sort body and all other code in the function unchanged.

Run:

```bash
corepack pnpm --filter ./apps/web run test -- src/features/employees/template-utils.test.ts 2>/dev/null || true
corepack pnpm --filter ./apps/web run test -- src/features/employees/create.test.tsx -t "template" 2>/dev/null || true
```

If no dedicated `template-utils.test.ts` exists, the create-page tests in Step 10 of Task 4 will cover this path. No test needs to be added here; the filter is straightforward.

Expected: `custom_agent` does not appear in template or blank-custom type picker lists.

- [ ] **Step 10: Commit Task 1**

```bash
git add apps/control-plane/internal/employee/employee_types.go apps/control-plane/internal/employee/service.go apps/control-plane/internal/employee/service_test.go apps/web/src/features/employees/template-utils.ts
git diff --cached --check
git commit -m "feat(employee): add custom agent type and provider normalization"
```

Expected: commit contains only the four listed files.

---

### Task 2: Structured Team Governance Error

**Files:**
- Modify: `apps/control-plane/internal/api/employee_routes_test.go`
- Modify: `apps/control-plane/internal/employee/handler.go`
- Possibly modify: `contracts/control-plane/openapi.yaml`
- Possibly generate: `apps/control-plane/internal/api/gen/control_plane.gen.go`

**Interfaces:**
- Consumes: existing `ErrEffectiveConfigRequired`.
- Produces: JSON 422 response with `code: "team_governance_config_required"` for Web branching.

- [ ] **Step 1: Add failing route test for create-options 422 JSON**

Add this test near the existing create-options route tests in `apps/control-plane/internal/api/employee_routes_test.go`:

```go
func TestDigitalEmployeeCreateOptionsReturnsStructuredTeamGovernanceError(t *testing.T) {
	server, service, cookie := newTestServerWithEmployeeService(t)
	teamID := uuid.New()
	service.createOptionsErr = fmt.Errorf("%w: active team governance config is required", employee.ErrEffectiveConfigRequired)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/digital-employees/create-options?team_id="+teamID.String(), nil)
	req.AddCookie(cookie)
	resp := httptest.NewRecorder()

	server.ServeHTTP(resp, req)

	require.Equal(t, http.StatusUnprocessableEntity, resp.Code)
	require.Contains(t, resp.Header().Get("Content-Type"), "application/json")
	var body struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	require.Equal(t, "team_governance_config_required", body.Code)
	require.Contains(t, body.Message, "active team governance config is required")
}
```

If the route service fake uses a differently named error field, add this field and branch:

```go
type routeEmployeeService struct {
	createOptionsErr error
	// existing fields stay unchanged
}

func (s *routeEmployeeService) GetCreateOptions(ctx context.Context, req employee.CreateOptionsRequest) (*employee.CreateOptions, error) {
	s.createOptionsReq = req
	if s.createOptionsErr != nil {
		return nil, s.createOptionsErr
	}
	return s.createOptions, nil
}
```

- [ ] **Step 2: Run the failing route test**

Run:

```bash
go test ./apps/control-plane/internal/api -run TestDigitalEmployeeCreateOptionsReturnsStructuredTeamGovernanceError -count=1
```

Expected: FAIL because `writeHandlerError` currently uses `http.Error` text/plain for `ErrEffectiveConfigRequired`.

- [ ] **Step 3: Add structured error response type and writer**

In `apps/control-plane/internal/employee/handler.go`, add near the response structs:

```go
type errorResponse struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func writeJSONError(w http.ResponseWriter, status int, code string, message string) {
	writeJSON(w, status, errorResponse{
		Code:    code,
		Message: message,
	})
}
```

Replace the `ErrEffectiveConfigRequired` branch in `writeHandlerError` with:

```go
	case errors.Is(err, ErrEffectiveConfigRequired):
		writeJSONError(w, http.StatusUnprocessableEntity, "team_governance_config_required", err.Error())
```

- [ ] **Step 4: Run handler/API tests**

Run:

```bash
go test ./apps/control-plane/internal/api -run TestDigitalEmployeeCreateOptionsReturnsStructuredTeamGovernanceError -count=1
go test ./apps/control-plane/internal/employee ./apps/control-plane/internal/api
```

Expected: PASS. If an existing test asserts text/plain body for effective-config errors, update only that assertion to expect the JSON shape above.

- [ ] **Step 5: Decide whether OpenAPI changes are necessary**

Search for the create-options operation in `contracts/control-plane/openapi.yaml`:

```bash
rg -n "create-options|DigitalEmployeeCreateOptions|422|team_governance_config_required" contracts/control-plane/openapi.yaml
```

If the contract already uses generic error responses or does not document error bodies for this route, do not change the contract in this task. If the contract explicitly lists a 422 schema for create-options, update it with this schema:

```yaml
DigitalEmployeeCreateOptionsError:
  type: object
  required:
    - code
    - message
  properties:
    code:
      type: string
      enum:
        - team_governance_config_required
    message:
      type: string
```

Then reference it from the create-options 422 response.

- [ ] **Step 6: Regenerate only if OpenAPI changed**

If Step 5 changed `contracts/control-plane/openapi.yaml`, run:

```bash
corepack pnpm generate:control-plane
corepack pnpm verify:contracts
```

Expected: both commands PASS and generated files match the contract update.

- [ ] **Step 7: Commit Task 2**

If OpenAPI was not changed:

```bash
git add apps/control-plane/internal/employee/handler.go apps/control-plane/internal/api/employee_routes_test.go
git diff --cached --check
git commit -m "feat(employee): return structured create options governance errors"
```

If OpenAPI was changed, include the contract and generated files:

```bash
git add apps/control-plane/internal/employee/handler.go apps/control-plane/internal/api/employee_routes_test.go contracts/control-plane/openapi.yaml apps/control-plane/internal/api/gen/control_plane.gen.go
git diff --cached --check
git commit -m "feat(employee): return structured create options governance errors"
```

Expected: no unrelated Web or prototype files are staged.

---

### Task 3: Web API Error Code Preservation

**Files:**
- Modify: `apps/web/src/lib/api/client.ts`
- Modify: `apps/web/src/lib/api/employees.ts`
- Modify: `apps/web/src/lib/api/employees.test.ts`

**Interfaces:**
- Consumes: Task 2 JSON error body.
- Produces: Web can check `error.code === "team_governance_config_required"` instead of matching English strings.

- [ ] **Step 1: Add failing API-client test for structured create-options error**

Add this test in `apps/web/src/lib/api/employees.test.ts` near the create-options tests:

```ts
  it("preserves structured create-options team governance error codes", async () => {
    const fetcher = vi.fn(async () =>
      new Response(
        JSON.stringify({
          code: "team_governance_config_required",
          message: "employee effective config required: active team governance config is required",
        }),
        {
          status: 422,
          headers: { "content-type": "application/json" },
        },
      ),
    );

    await expect(
      getDigitalEmployeeCreateOptions(
        { baseUrl: "http://control-plane.local", fetcher: fetcher as typeof fetch },
        "team-1",
      ),
    ).rejects.toMatchObject({
      name: "ApiRequestError",
      status: 422,
      code: "team_governance_config_required",
      detail: "employee effective config required: active team governance config is required",
    });
  });
```

- [ ] **Step 2: Run the failing Web API test**

Run:

```bash
corepack pnpm --filter ./apps/web run test -- src/lib/api/employees.test.ts -t "structured create-options team governance"
```

Expected: FAIL because `ApiRequestError` currently only exposes `status`.

- [ ] **Step 3: Extend `ApiRequestError` without breaking message strings**

Change `ApiRequestError` in `apps/web/src/lib/api/client.ts` to:

```ts
export class ApiRequestError extends Error {
  readonly status: number;
  readonly code?: string;
  readonly detail?: string;

  constructor(resource: string, status: number, detail?: string, code?: string) {
    super(`${resource} request failed with status ${status}${detail ? `: ${detail}` : ""}`);
    this.name = "ApiRequestError";
    this.status = status;
    this.code = code;
    this.detail = detail;
  }
}
```

Replace `readErrorDetail` with structured parsing:

```ts
type ParsedErrorDetail = {
  detail?: string;
  code?: string;
};

async function readErrorDetail(response: Response): Promise<ParsedErrorDetail> {
  const contentType = response.headers.get("content-type") ?? "";
  const body = await response.text();

  if (!body) {
    return {};
  }

  if (contentType.includes("application/json")) {
    try {
      const parsed = JSON.parse(body) as { code?: unknown; error?: unknown; message?: unknown };
      const detail =
        typeof parsed.error === "string" && parsed.error
          ? parsed.error
          : typeof parsed.message === "string" && parsed.message
            ? parsed.message
            : body;
      return {
        detail,
        code: typeof parsed.code === "string" && parsed.code ? parsed.code : undefined,
      };
    } catch {
      return { detail: body };
    }
  }

  return { detail: body };
}
```

Update `parseJson`:

```ts
export async function parseJson<T>(response: Response, resource: string): Promise<T> {
  if (!response.ok) {
    const errorDetail = await readErrorDetail(response);
    throw new ApiRequestError(resource, response.status, errorDetail.detail, errorDetail.code);
  }

  return (await response.json()) as T;
}
```

- [ ] **Step 4: Add a narrow helper for the create page**

In `apps/web/src/lib/api/employees.ts`, import `ApiRequestError` and add:

```ts
export function isTeamGovernanceConfigRequiredError(error: unknown): boolean {
  return error instanceof ApiRequestError && error.status === 422 && error.code === "team_governance_config_required";
}
```

- [ ] **Step 5: Run affected Web API tests**

Run:

```bash
corepack pnpm --filter ./apps/web run test -- src/lib/api/employees.test.ts src/lib/api/auth.test.ts src/lib/api/tasks.test.ts src/lib/api/runtime.test.ts src/lib/api/capabilities.test.ts
```

Expected: PASS. Existing tests that assert error message text keep passing because the message format remains unchanged.

- [ ] **Step 6: Commit Task 3**

```bash
git add apps/web/src/lib/api/client.ts apps/web/src/lib/api/employees.ts apps/web/src/lib/api/employees.test.ts
git diff --cached --check
git commit -m "feat(web): preserve structured API error codes"
```

Expected: commit contains only Web API client files/tests.

---

### Task 4: Web Blank Custom Identity And Required Provider Type

**Files:**
- Modify: `apps/web/src/features/employees/create.test.tsx`
- Modify: `apps/web/src/features/employees/create.tsx`

**Interfaces:**
- Consumes: Task 1 `custom_agent` and Provider canonical values.
- Produces: create wizard with no blank-custom employee-type selection and no `Provider 偏好` copy.

- [ ] **Step 1: Update fixture Provider values to canonical values**

In `apps/web/src/features/employees/create.test.tsx`, replace fixture values:

```tsx
provider_type: "claude_code"
```

with:

```tsx
provider_type: "claude-code"
```

Replace all `allowed_provider_types` and `capability_options.provider_types` occurrences of `claude_code` with `claude-code`.

Replace label expectations and clicks from:

```tsx
screen.getByLabelText("claude_code")
```

to:

```tsx
screen.getByLabelText("Claude Code")
```

- [ ] **Step 2: Replace old blank-custom selector tests with new product behavior**

Replace the test named `opens the blank-custom employee type selector while keeping copy and clone disabled` with:

```tsx
  it("opens blank custom as a custom identity without asking for employee type", async () => {
    const screen = await renderCreateEmployeeView();

    await expect.element(screen.getByRole("button", { name: /^从专业模板创建/ })).toBeEnabled();
    await expect.element(screen.getByRole("button", { name: /^空白自定义/ })).toBeEnabled();
    await expect.element(screen.getByRole("button", { name: /^从团队角色复制/ })).toBeDisabled();
    await expect.element(screen.getByRole("button", { name: /^从历史员工克隆/ })).toBeDisabled();

    await userEvent.click(screen.getByRole("button", { name: /^空白自定义/ }));

    await expect.element(screen.getByRole("heading", { name: "配置预检" })).toBeVisible();
    await expect.element(screen.getByText("自定义身份", { exact: true })).toBeVisible();
    expect(document.body.textContent).not.toContain("选择员工类型");
    expect(document.body.textContent).not.toContain("底层类型");
  });
```

Update `enterBlankCustomConfiguration` to:

```tsx
async function enterBlankCustomConfiguration(screen: Awaited<ReturnType<typeof renderCreateEmployeeView>>) {
  await expect.element(screen.getByRole("button", { name: /^空白自定义/ })).toBeEnabled();
  await userEvent.click(screen.getByRole("button", { name: /^空白自定义/ }));
  await expect.element(screen.getByRole("heading", { name: "配置预检" })).toBeVisible();
  await userEvent.click(screen.getByRole("button", { name: /继续配置/ }));
  await expect.element(screen.getByRole("heading", { name: "员工画像蓝图" })).toBeVisible();
}
```

- [ ] **Step 3: Add tests for renamed copy and required Provider**

Add these tests near Provider tests:

```tsx
  it("uses responsibility and provider type copy instead of legacy role and provider preference copy", async () => {
    const screen = await renderCreateEmployeeView(createWizardFetcher({ sameRuntimeNodeProviders: true }));

    await enterConfiguration(screen);
    await expect.element(screen.getByLabelText("职责定位")).toHaveValue("database_admin");
    expect(document.body.textContent).not.toContain("Provider 偏好");
    expect(document.body.textContent).not.toContain("角色");

    await userEvent.fill(screen.getByLabelText("名称"), "数据库管理员工");
    await userEvent.click(screen.getByRole("button", { name: "下一步" }));
    await userEvent.click(screen.getByRole("button", { name: "下一步" }));
    await userEvent.click(screen.getByRole("button", { name: "下一步" }));

    await expect.element(screen.getByRole("heading", { name: "Provider 类型" })).toBeVisible();
    await expect.element(screen.getByRole("button", { name: "进入确认创建" })).toBeDisabled();
    await userEvent.click(screen.getByLabelText("Claude Code"));
    await expect.element(screen.getByRole("button", { name: "进入确认创建" })).toBeEnabled();
  });
```

Add a submit test for blank custom:

```tsx
  it("submits blank custom as internal custom_agent with canonical provider type", async () => {
    const fetcher = createWizardFetcher({
      sameRuntimeNodeProviders: true,
      expectedProviderType: "claude-code",
      expectedCreateBody: {
        employee_type: "custom_agent",
        name: "自定义员工",
        role: "负责跨系统问题诊断",
        description: "直接定义身份",
        risk_level: "medium",
        avatar_asset_id: avatarAsset.id,
        role_profile: {
          employee_type: "custom_agent",
          role: "负责跨系统问题诊断",
          title: "自定义身份",
        },
        capability_selection: {
          enabled_skills: [],
          enabled_mcp_servers: [],
          enabled_external_capabilities: [],
        },
        context_policy_override: {},
        approval_policy_override: {},
        output_contract_addendum: {},
        provider_type: "claude-code",
        session_policy: { mode: "reuse_latest" },
        workspace_policy: {},
        environment_variables: [],
        metadata: { creation_mode: "blank_custom" },
      },
    });
    const screen = await renderCreateEmployeeView(fetcher);

    await enterBlankCustomConfiguration(screen);
    await userEvent.fill(screen.getByLabelText("名称"), "自定义员工");
    await userEvent.fill(screen.getByLabelText("职责定位"), "负责跨系统问题诊断");
    await userEvent.fill(screen.getByLabelText("描述"), "直接定义身份");
    await userEvent.click(screen.getByRole("button", { name: "下一步" }));
    await userEvent.click(screen.getByRole("button", { name: "下一步" }));
    await userEvent.click(screen.getByRole("button", { name: "下一步" }));
    await userEvent.click(screen.getByLabelText("Claude Code"));
    await enterConfirmCreation(screen);
    await userEvent.click(screen.getByRole("button", { name: "确认创建" }));

    const createCall = findCreateEmployeePost(fetcher);
    expect(createCall).toBeTruthy();
    const body = JSON.parse(String(createCall?.[1]?.body));
    expect(body.employee_type).toBe("custom_agent");
    expect(body.provider_type).toBe("claude-code");
    expect(body.metadata).toEqual({ creation_mode: "blank_custom" });
  });
```

- [ ] **Step 4: Run the failing Web create tests**

Run:

```bash
corepack pnpm --filter ./apps/web run test -- src/features/employees/create.test.tsx -t "blank custom|provider type|canonical provider"
```

Expected: FAIL because current UI still uses employee-type selection, `角色`, `Provider 偏好`, and `claude_code`.

- [ ] **Step 5: Update draft defaults and config step names**

In `apps/web/src/features/employees/create.tsx`, add:

```tsx
const BLANK_CUSTOM_EMPLOYEE_TYPE = "custom_agent";
const BLANK_CUSTOM_TITLE = "自定义身份";
```

**5a — Rename the step constant.** Replace:

```tsx
const configSteps = ["身份", "能力", "治理", "Provider 偏好"] as const;
```

with:

```tsx
const configSteps = ["身份", "能力", "治理", "Provider 类型"] as const;
```

**5b — Fix the render guard.** The step component is conditionally rendered with a string equality check against `currentStep`. Find the block:

```tsx
{!teams.isLoading && !createOptions.isLoading && currentStep === "Provider 偏好" ? (
```

and replace `"Provider 偏好"` with `"Provider 类型"`:

```tsx
{!teams.isLoading && !createOptions.isLoading && currentStep === "Provider 类型" ? (
```

If this guard is missing the render component will silently disappear when the user reaches the Provider step.

**5c — Fix validateStep call sites.** There are two call sites that pass the old step name as a string argument. Find and replace both:

```tsx
// call site 1 — inside the "next step" handler (validates before advancing)
const nextErrors = validateStep("Provider 偏好", draft);
```

→

```tsx
const nextErrors = validateStep("Provider 类型", draft);
```

```tsx
// call site 2 — inside validateStep itself
if (step === "Provider 偏好" && !draft.provider_type) {
  errors.runtime = "请选择 Provider 类型";
}
```

→

```tsx
if (step === "Provider 类型" && !draft.provider_type) {
  errors.runtime = "请选择 Provider 类型";
}
```

After these three targeted replacements, do a final scan for any remaining occurrences:

```bash
rg "Provider 偏好" apps/web/src/features/employees/create.tsx
```

Replace every remaining occurrence that is user-visible copy with `Provider 类型` or `执行器类型` per the spec. Non-copy occurrences (string literals used as step keys or condition values) must also be updated so they match the renamed constant.

- [ ] **Step 6: Make blank custom skip employee type selection**

Replace the blank-custom branch in `selectCreationMode` with:

```tsx
const selectCreationMode = (nextMode: CreationMode) => {
  const nextDraft = resetDraftForMode(nextMode, draft.team_id);
  if (nextMode === "blank_custom") {
    setDraft(applyBlankCustomDefaults(nextDraft));
    setFlowStep("preflight");
    return;
  }
  setDraft(nextDraft);
  setFlowStep("template");
};
```

Add:

```tsx
function applyBlankCustomDefaults(current: WizardDraft): WizardDraft {
  return {
    ...current,
    creation_mode: "blank_custom",
    employee_type: BLANK_CUSTOM_EMPLOYEE_TYPE,
    role: "",
    risk_level: "medium",
    capability_selection: {
      enabled_external_capabilities: [],
      enabled_mcp_servers: [],
      enabled_skills: [],
    },
    context_policy_override: {},
    approval_policy_override: {},
  };
}
```

Remove `BlankCustomSelectionPanel` rendering and any `applyBlankTypeDefaults` calls.

- [ ] **Step 7: Hide employee type for blank custom in identity and summaries**

In `IdentityStep`, render the employee type field only for template mode:

```tsx
{!isBlankCustom ? (
  <Field label="员工类型" error={errors.employee_type}>
    <Input id={fieldIds["员工类型"]} value={draft.employee_type} disabled />
  </Field>
) : null}
```

Change the role field label:

```tsx
<Field label="职责定位" error={errors.role}>
  <Input
    id={fieldIds["职责定位"]}
    value={draft.role}
    onChange={(event) => onDraftChange({ ...draft, role: event.target.value })}
    placeholder="例如：负责跨系统问题诊断与交付验证"
  />
</Field>
```

Change `fieldIds` to:

```tsx
const fieldIds = {
  名称: "employee-name",
  职责定位: "employee-role",
  描述: "employee-description",
  员工类型: "employee-type",
  归属团队: "employee-team",
  风险等级: "employee-risk",
} as const;
```

Change validation text:

```tsx
if (!draft.role.trim()) errors.role = "职责定位不能为空";
```

In summaries and confirm cards, use:

```tsx
const roleTitle = draft.creation_mode === "blank_custom" ? BLANK_CUSTOM_TITLE : selectedType?.label ?? draft.employee_type;
```

For blank custom, do not render `底层类型` or `员工类型`; render `创建路径: 自定义身份`.

- [ ] **Step 8: Update submit payload title**

In the create mutation body, change `role_profile.title` to:

```tsx
title: blankCustom ? BLANK_CUSTOM_TITLE : selectedType?.label ?? draft.employee_type,
```

Keep:

```tsx
employee_type: draft.employee_type,
role: draft.role.trim(),
...(blankCustom ? { metadata: { creation_mode: "blank_custom" } } : {}),
```

- [ ] **Step 9: Canonical Provider display labels**

Add:

```tsx
const providerLabels: Record<string, string> = {
  codex: "Codex",
  opencode: "OpenCode",
  "claude-code": "Claude Code",
};

function providerLabel(providerType: string): string {
  return providerLabels[providerType] ?? providerType;
}
```

Use `providerLabel(option.provider_type)` for radio labels and summaries while storing/submitting the raw canonical value.

Replace all user-visible copy containing `Provider 偏好` with `Provider 类型`.

Replace the no-runtime warning copy with:

```tsx
"当前没有在线 Runtime 节点支持该 Provider；创建后运行前需要可用 Runtime。"
```

- [ ] **Step 10: Run full create-page test**

Run:

```bash
corepack pnpm --filter ./apps/web run test -- src/features/employees/create.test.tsx
```

Expected: PASS after updating affected expectations.

- [ ] **Step 11: Commit Task 4**

```bash
git add apps/web/src/features/employees/create.tsx apps/web/src/features/employees/create.test.tsx
git diff --cached --check
git commit -m "feat(web): redesign blank custom employee create flow"
```

Expected: commit contains only the create page and its test.

---

### Task 5: Web Team Governance Blocker And Capability Split

**Files:**
- Modify: `apps/web/src/features/employees/create.test.tsx`
- Modify: `apps/web/src/features/employees/create.tsx`

**Interfaces:**
- Consumes: Task 3 `isTeamGovernanceConfigRequiredError`.
- Consumes: existing `listTeamSkills`, `listSkills`, `listTeamMcpBindings`.
- Produces: business blocker for team governance and read-only inherited vs selectable extension capability UI.

- [ ] **Step 1: Extend create-page fixture for team capability APIs**

In `apps/web/src/features/employees/create.test.tsx`, update `createWizardFetcher` so it handles:

```tsx
if (url.pathname === `/api/v1/teams/${team.id}/skills` && method === "GET") {
  return jsonResponse([
    {
      id: "skill-team-sql-review",
      tenant_id: "22222222-2222-4222-8222-222222222222",
      slug: "sql-review",
      name: "SQL Review",
      description: "SQL review",
      version: "1.0.0",
      source: "internal",
      risk_level: "medium",
      icon_key: "database",
      color_token: "blue",
      tags: [],
      archive_object_ref: "skills/sql-review.zip",
      archive_filename: "sql-review.zip",
      archive_size_bytes: 1,
      archive_checksum_sha256: "checksum",
      archive_file_count: 1,
      created_by: "user-1",
      created_by_name: "Admin",
      team_bindings: [],
      agent_bindings: [],
    },
  ]);
}

if (url.pathname === `/api/v1/teams/${team.id}/mcp-bindings` && method === "GET") {
  return jsonResponse([
    {
      id: "binding-postgres",
      tenant_id: "22222222-2222-4222-8222-222222222222",
      team_id: team.id,
      mcp_server_id: "mcp-postgres",
      server_key: "postgres",
      server_name: "Postgres",
      status: "active",
      source_scope: "team",
    },
  ]);
}
```

Ensure team-less paths do not require these API calls.

- [ ] **Step 2: Add failing test for structured team governance blocker**

Add this test:

```tsx
  it("shows a business blocker when selected team lacks active governance config", async () => {
    const fetcher = createWizardFetcher({ teams: [team, secondTeam], teamGovernanceMissingForTeamId: secondTeam.id });
    const screen = await renderCreateEmployeeView(fetcher);

    await enterConfiguration(screen);
    await userEvent.selectOptions(screen.getByLabelText("归属团队"), secondTeam.id);

    await expect
      .element(screen.getByText("该团队尚未启用治理配置，不能在此团队下创建数字员工。"))
      .toBeVisible();
    await expect.element(screen.getByRole("button", { name: "先不归属团队创建" })).toBeVisible();
    await expect.element(screen.getByRole("link", { name: "前往团队治理配置" })).toBeVisible();
    expect(document.body.textContent).not.toContain("employee effective config required");

    await userEvent.click(screen.getByRole("button", { name: "先不归属团队创建" }));
    await expect.element(screen.getByLabelText("归属团队")).toHaveValue("");
  });
```

Update the fetch fixture so selected `teamGovernanceMissingForTeamId` returns:

```tsx
return jsonResponse(
  {
    code: "team_governance_config_required",
    message: "employee effective config required: active team governance config is required",
  },
  { status: 422 },
);
```

- [ ] **Step 3: Add failing capability split test**

Add this test:

```tsx
  it("separates team-inherited capabilities from employee extension selections", async () => {
    const fetcher = createWizardFetcher({ expectedTeamId: team.id });
    const screen = await renderCreateEmployeeView(fetcher);

    await enterConfiguration(screen);
    await userEvent.selectOptions(screen.getByLabelText("归属团队"), team.id);
    await userEvent.fill(screen.getByLabelText("名称"), "数据库管理员工");
    await userEvent.fill(screen.getByLabelText("职责定位"), "负责数据库变更");
    await userEvent.click(screen.getByRole("button", { name: "下一步" }));

    await expect.element(screen.getByRole("heading", { name: "团队继承能力" })).toBeVisible();
    await expect.element(screen.getByText("SQL Review")).toBeVisible();
    await expect.element(screen.getByText("Postgres")).toBeVisible();
    await expect.element(screen.getAllByText("团队继承")[0]).toBeVisible();
    await expect.element(screen.getByRole("heading", { name: "员工扩展能力" })).toBeVisible();
    expect(screen.getByRole("checkbox", { name: "sql-review" }).query()).toBeNull();
    expect(screen.getByRole("checkbox", { name: "postgres" }).query()).toBeNull();

    await userEvent.click(screen.getByRole("checkbox", { name: "incident-diagnosis" }));
    await userEvent.click(screen.getByRole("checkbox", { name: "jira.search" }));
    await userEvent.click(screen.getByRole("button", { name: "下一步" }));
    await userEvent.click(screen.getByRole("button", { name: "下一步" }));
    await userEvent.click(screen.getByLabelText("Codex"));
    await enterConfirmCreation(screen);
    await userEvent.click(screen.getByRole("button", { name: "确认创建" }));

    const createCall = findCreateEmployeePost(fetcher);
    expect(createCall).toBeTruthy();
    const body = JSON.parse(String(createCall?.[1]?.body));
    expect(body.capability_selection).toEqual({
      enabled_skills: ["incident-diagnosis"],
      enabled_mcp_servers: [],
      enabled_external_capabilities: ["jira.search"],
    });
  });
```

- [ ] **Step 4: Run the failing team/capability tests**

Run:

```bash
corepack pnpm --filter ./apps/web run test -- src/features/employees/create.test.tsx -t "governance config|team-inherited capabilities"
```

Expected: FAIL until create page catches structured 422 and renders capability split.

- [ ] **Step 5: Add team capability queries to create page**

In `apps/web/src/features/employees/create.tsx`, import:

```tsx
import { listSkills, listTeamSkills } from "@/lib/api/skills";
import { listTeamMcpBindings } from "@/lib/api/capabilities";
import { isTeamGovernanceConfigRequiredError } from "@/lib/api/employees";
```

Add queries:

```tsx
const visibleSkills = useQuery({
  queryKey: ["skills", "employee-create-visible"],
  queryFn: () => listSkills({ baseUrl: apiBaseUrl, fetcher }),
});

const teamSkills = useQuery({
  enabled: Boolean(draft.team_id) && !isTeamGovernanceConfigRequiredError(createOptions.error),
  queryKey: ["team-skills", draft.team_id],
  queryFn: () => listTeamSkills({ baseUrl: apiBaseUrl, fetcher }, draft.team_id),
});

const teamMcpBindings = useQuery({
  enabled: Boolean(draft.team_id) && !isTeamGovernanceConfigRequiredError(createOptions.error),
  queryKey: ["team-mcp-bindings", draft.team_id],
  queryFn: () => listTeamMcpBindings({ baseUrl: apiBaseUrl, fetcher }, draft.team_id),
});
```

If the skill registry query creates excessive traffic, keep it cached with the query key above and use `staleTime: 30_000`.

- [ ] **Step 6: Render the team governance blocker**

Add a computed flag:

```tsx
const teamGovernanceBlocked = isTeamGovernanceConfigRequiredError(createOptions.error);
```

When `teamGovernanceBlocked` is true, render this near the team field and before step navigation:

```tsx
<Alert variant="destructive">
  <AlertTitle>团队治理未启用</AlertTitle>
  <AlertDescription>
    该团队尚未启用治理配置，不能在此团队下创建数字员工。
  </AlertDescription>
  <div className="mt-3 flex flex-wrap gap-2">
    <Button
      type="button"
      variant="secondary"
      onClick={() => {
        setDraft((current) => ({
          ...current,
          team_id: "",
          capability_selection: {
            ...current.capability_selection,
            enabled_skills: [],
            enabled_mcp_servers: [],
          },
        }));
      }}
    >
      先不归属团队创建
    </Button>
    <Button asChild type="button" variant="outline">
      <Link to="/teams/$teamId" params={{ teamId: draft.team_id }}>
        前往团队治理配置
      </Link>
    </Button>
  </div>
</Alert>
```

The current router has `/_authenticated/teams/$teamId`, exposed to callers as `to="/teams/$teamId"`.

Block next-step progression while this flag is true:

```tsx
if (teamGovernanceBlocked) {
  return { ...nextErrors, team_id: "该团队尚未启用治理配置" };
}
```

- [ ] **Step 7: Build inherited and extension capability sets**

Add helpers:

```tsx
function skillKey(skill: { slug?: string; name?: string }): string {
  return skill.slug || skill.name || "";
}

function mcpBindingKey(binding: { server_key?: string; server_name?: string }): string {
  return binding.server_key || binding.server_name || "";
}

function withoutInherited(values: string[], inherited: Set<string>): string[] {
  return values.filter((value) => !inherited.has(value));
}
```

In `CapabilityStep`, derive:

```tsx
const inheritedSkillKeys = new Set((teamSkills.data ?? []).map(skillKey).filter(Boolean));
const inheritedMcpKeys = new Set((teamMcpBindings.data ?? []).map(mcpBindingKey).filter(Boolean));
const extensionSkills = withoutInherited(capabilityOptions?.skills ?? [], inheritedSkillKeys);
const extensionMcpServers = withoutInherited(capabilityOptions?.mcp_servers ?? [], inheritedMcpKeys);
```

For visible skills, display names can come from the registry:

```tsx
const visibleSkillLabelBySlug = new Map((visibleSkills.data ?? []).map((skill) => [skill.slug, skill.name]));
```

- [ ] **Step 8: Render read-only inherited and editable extension sections**

Replace the current flat capability checkbox rendering with:

```tsx
<section>
  <h2 className="text-lg font-semibold">团队继承能力</h2>
  <p className="mt-1 text-sm text-muted-foreground">由归属团队绑定而来，只读展示，不作为员工扩展能力提交。</p>
  <div className="mt-3 grid gap-2">
    {(teamSkills.data ?? []).map((skill) => (
      <CapabilityReadOnlyRow key={`skill-${skill.id}`} label={skill.name || skill.slug} kind="技能" />
    ))}
    {(teamMcpBindings.data ?? []).map((binding) => (
      <CapabilityReadOnlyRow key={`mcp-${binding.id}`} label={binding.server_name || binding.server_key || binding.mcp_server_id} kind="MCP" />
    ))}
    {!draft.team_id ? <p className="text-sm text-muted-foreground">未选择团队，当前没有团队继承能力。</p> : null}
  </div>
</section>

<section className="mt-6">
  <h2 className="text-lg font-semibold">员工扩展能力</h2>
  <p className="mt-1 text-sm text-muted-foreground">从当前账号可见且团队策略允许的能力中为该员工追加。</p>
  <CapabilityCheckboxGroup
    title="技能"
    checkedValues={draft.capability_selection.enabled_skills}
    values={extensionSkills}
    labelForValue={(value) => visibleSkillLabelBySlug.get(value) ?? value}
    onToggle={(value) => toggle("enabled_skills", value)}
  />
  <CapabilityCheckboxGroup
    title="MCP"
    checkedValues={draft.capability_selection.enabled_mcp_servers}
    values={extensionMcpServers}
    onToggle={(value) => toggle("enabled_mcp_servers", value)}
  />
  <CapabilityCheckboxGroup
    title="外部能力"
    checkedValues={draft.capability_selection.enabled_external_capabilities}
    values={capabilityOptions?.external_capabilities ?? []}
    onToggle={(value) => toggle("enabled_external_capabilities", value)}
  />
</section>
```

Add:

```tsx
function CapabilityReadOnlyRow({ label, kind }: { label: string; kind: string }) {
  return (
    <div className="flex items-center justify-between rounded-md border bg-background px-3 py-2 text-sm">
      <span>{label}</span>
      <div className="flex items-center gap-2">
        <Badge variant="outline">{kind}</Badge>
        <Badge variant="secondary">团队继承</Badge>
      </div>
    </div>
  );
}
```

- [ ] **Step 9: Prune extension selections when team inherited values arrive**

Add an effect:

```tsx
useEffect(() => {
  const inheritedSkillKeys = new Set((teamSkills.data ?? []).map(skillKey).filter(Boolean));
  const inheritedMcpKeys = new Set((teamMcpBindings.data ?? []).map(mcpBindingKey).filter(Boolean));
  if (inheritedSkillKeys.size === 0 && inheritedMcpKeys.size === 0) return;

  setDraft((current) => ({
    ...current,
    capability_selection: {
      ...current.capability_selection,
      enabled_skills: current.capability_selection.enabled_skills.filter((value) => !inheritedSkillKeys.has(value)),
      enabled_mcp_servers: current.capability_selection.enabled_mcp_servers.filter((value) => !inheritedMcpKeys.has(value)),
    },
  }));
}, [teamSkills.data, teamMcpBindings.data]);
```

- [ ] **Step 10: Run full create-page tests**

Run:

```bash
corepack pnpm --filter ./apps/web run test -- src/features/employees/create.test.tsx
```

Expected: PASS.

- [ ] **Step 11: Commit Task 5**

```bash
git add apps/web/src/features/employees/create.tsx apps/web/src/features/employees/create.test.tsx
git diff --cached --check
git commit -m "feat(web): split inherited and extension capabilities on employee create"
```

Expected: commit contains only the create page and its test.

---

### Task 6: Cross-Layer Verification And Real Smoke

**Files:**
- No required source edits.
- Modify only tests or docs if verification exposes a real mismatch.

**Interfaces:**
- Consumes: Tasks 1-5.
- Produces: evidence that the implementation works through unit tests and a real create flow, or a precise blocker report.

- [ ] **Step 1: Run backend tests**

Run:

```bash
go test ./apps/control-plane/internal/employee ./apps/control-plane/internal/api
```

Expected: PASS.

- [ ] **Step 2: Run Web tests**

Run:

```bash
corepack pnpm --filter ./apps/web run test -- src/features/employees/create.test.tsx src/lib/api/employees.test.ts
```

Expected: PASS.

- [ ] **Step 3: Run contract verification if OpenAPI changed**

If `git diff --name-only HEAD~5..HEAD` or current branch diff includes `contracts/control-plane/openapi.yaml`, run:

```bash
corepack pnpm generate:control-plane
corepack pnpm verify:contracts
```

Expected: PASS with no unexpected generated drift.

- [ ] **Step 4: Run typecheck for Web API/page typing**

Run:

```bash
corepack pnpm --filter ./apps/web run typecheck
```

Expected: PASS. If the repo does not define this script in `apps/web/package.json`, record the missing script and rely on the focused Vitest/browser tests plus final smoke.

- [ ] **Step 5: Run diff hygiene**

Run:

```bash
git diff --check
```

Expected: no whitespace errors.

- [ ] **Step 6: Restart live services on the current code**

Run:

```bash
scripts/dev-services.sh status
scripts/dev-services.sh restart control-plane web
scripts/dev-services.sh status
```

Expected: Control Plane and Web are running from this workspace. Do not restart Runtime unless the smoke needs a runtime-state advisory check.

- [ ] **Step 7: Real API smoke for team-less blank custom create**

Use the existing dev login seed if available:

```bash
curl -i -c /tmp/superteam-cookies.txt \
  -H 'content-type: application/json' \
  -d '{"username":"admin","password":"admin"}' \
  http://localhost:3000/api/auth/login
```

Then call create-options without a team:

```bash
curl -s -b /tmp/superteam-cookies.txt \
  http://localhost:3000/api/v1/digital-employees/create-options | jq '.employee_types[] | select(.type=="custom_agent")'
```

Expected: a `custom_agent` option is present.

Create a smoke employee:

```bash
curl -s -b /tmp/superteam-cookies.txt \
  -H 'content-type: application/json' \
  -d '{
    "employee_type":"custom_agent",
    "name":"烟测自定义数字员工",
    "avatar_asset_id":"engineer-m-01",
    "role":"负责创建流程烟测",
    "description":"Codex smoke created custom agent",
    "risk_level":"medium",
    "metadata":{"creation_mode":"blank_custom"},
    "role_profile":{"employee_type":"custom_agent","role":"负责创建流程烟测","title":"自定义身份"},
    "capability_selection":{"enabled_skills":[],"enabled_mcp_servers":[],"enabled_external_capabilities":[]},
    "context_policy_override":{},
    "approval_policy_override":{},
    "budget_policy":{},
    "output_contract_addendum":{},
    "provider_type":"codex",
    "session_policy":{"mode":"reuse_latest"},
    "workspace_policy":{},
    "environment_variables":[]
  }' \
  http://localhost:3000/api/v1/digital-employees | jq '{id, employee_type, provider_type, role, status}'
```

Expected: HTTP 201 and JSON includes `employee_type: "custom_agent"`, `provider_type: "codex"`, `status: "ready"` or the current create-success status.

- [ ] **Step 8: Real API smoke for structured missing-governance error**

Find or create a test team without active governance config only if this is safe in the dev database. If a known team ID exists, run:

```bash
curl -i -b /tmp/superteam-cookies.txt \
  "http://localhost:3000/api/v1/digital-employees/create-options?team_id=<team-without-active-config>"
```

Expected: HTTP 422 and JSON body includes `code: "team_governance_config_required"`.

If no such team exists and creating one would pollute shared state, record this smoke as blocked with the reason `no safe team without active governance config in current dev DB`.

- [ ] **Step 9: Browser smoke**

Use the browser/Chrome plugin per project instructions:

1. Open `http://localhost:3000/employees/new`.
2. Log in if redirected.
3. Click `空白自定义`.
4. Confirm the first configuration screen shows `自定义身份`, not `选择员工类型`.
5. Fill `名称` and `职责定位`.
6. Leave team empty.
7. Go to `能力`, confirm `团队继承能力` shows no inherited team capabilities.
8. Go to `Provider 类型`, select `Codex` or another available canonical Provider.
9. Confirm and create.
10. Verify navigation to the employee detail page and visible identity/provider data.

Expected: flow completes through real Web and Control Plane.

- [ ] **Step 10: Run completion gate**

Use the project completion skill before final reporting:

```bash
# Read and follow .codex/skills/superteam-completion-check/SKILL.md
```

Expected: final report distinguishes real-chain evidence from unit/component evidence and lists any blocked smoke items.

- [ ] **Step 11: Final commit if verification required fixes**

If verification uncovered fixes after Task 5, commit them separately:

```bash
git add <only-fixed-files>
git diff --cached --check
git commit -m "fix(employee): align create redesign verification findings"
```

Expected: no unrelated dirty files staged.

---

## Self-Review

Spec coverage:

- Blank custom no longer asks for employee type: Task 4.
- Internal `custom_agent`: Task 1 and Task 4.
- `角色` to `职责定位`: Task 4.
- Provider required and not preference: Task 1 and Task 4.
- Provider values `codex`, `opencode`, `claude-code`: Task 1 and Task 4.
- Team optional and no-team default governance: Task 1 plus existing path, verified in Task 6.
- Team missing governance business blocker: Task 2, Task 3, Task 5.
- Capability split inherited vs extension: Task 5.
- Inherited capabilities not submitted: Task 5 test.
- Details effective source tags: existing employee detail code already uses `团队继承` and personal/employee source; Task 6 catches regressions without expanding this plan.
- Runtime online not create blocker: Task 4 preserves Provider-only creation and Task 6 smoke verifies no direct Runtime binding.

Placeholder scan:

- No unresolved placeholder markers or unspecified test instructions are intentionally present.
- Where the route path for team governance settings may vary, the plan instructs discovery in existing router files and keeps the required label/behavior fixed.

Type consistency:

- Backend Provider spelling is `claude-code`.
- Web display label is `Claude Code`.
- Blank custom internal employee type is `custom_agent`.
- Structured error code is `team_governance_config_required`.
- Web field label is `职责定位`; submit field remains `role`.
