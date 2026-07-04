# Project Runtime Placement Coordination Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make Project Runtime Placement a first-class configuration and diagnostic fact, split planning eligibility from dispatch readiness, and expose blocked coordination states in the real project/workflow UI.

**Architecture:** Control Plane owns project placement, readiness aggregation, coordination facts, and dispatch gating. Runtime Agent remains the execution layer and only supplies node/capability/connection state. Web reads project placement/readiness/blocking APIs and renders explicit next actions instead of showing endless planning.

**Tech Stack:** Go Control Plane with chi HTTP handlers, pgx/sqlc storage, Temporal project coordinator, TypeScript/React Web with TanStack Query/Router, existing OpenAPI contract, `corepack pnpm` verification.

---

## Spec

Implement the approved design in `docs/superpowers/specs/2026-07-04-project-runtime-placement-coordination-design.md`.

## File Structure

### Control Plane project domain

- Modify `apps/control-plane/internal/project/types.go`
  - Add project event types for runtime placement and coordination blocking.
  - Add request/response/readiness structs used by handler, service, tests, and Web contract.
- Modify `apps/control-plane/internal/project/repository.go`
  - Add repository methods for project placement get/upsert/release.
- Modify `apps/control-plane/internal/project/pg_repository.go`
  - Implement placement repository methods using sqlc query methods.
- Modify `apps/control-plane/internal/project/service.go`
  - Add placement service methods.
  - Add execution readiness aggregation.
  - Keep Control Plane policy and audit here.
- Modify `apps/control-plane/internal/project/handler.go`
  - Add HTTP endpoints for placement and readiness.
- Modify `apps/control-plane/internal/api/server.go`
  - Register the project placement/readiness routes under authenticated project routes.

### Storage and contract

- Modify `apps/control-plane/internal/storage/queries/project_runtime_affinity.sql`
  - Add `ReleaseProjectPlacement`.
  - Add optional list/query helpers only if needed by repository code.
- Regenerate sqlc outputs under `apps/control-plane/internal/storage/queries/`.
- Modify `contracts/control-plane/openapi.yaml`
  - Add runtime placement and readiness paths/schemas.
- Regenerate contract output if this repo's generator changes generated files.

### Coordination

- Modify `apps/control-plane/internal/workflow/projectcoordination/planning_profile.go`
  - Add planning readiness hint fields without making them hard failures.
- Modify `apps/control-plane/internal/workflow/projectcoordination/project_store.go`
  - Stop filtering Planner candidate pool by dispatch readiness.
  - Record structured blocked facts for no planning candidates and dispatch readiness failures.
- Modify `apps/control-plane/internal/workflow/projectcoordination/predispatch_gate.go`
  - Keep strict dispatch gate semantics.
  - Ensure reason codes cover missing placement/runtime/provider/workspace/slot/contract.
- Modify `apps/control-plane/internal/workflow/projectcoordination/workflow.go`
  - Project known workflow signal/planning failures into readable project events where this layer owns the error.

### Web API and UI

- Modify `apps/web/src/lib/api/projects.ts`
  - Add placement/readiness types and client functions.
- Modify `apps/web/src/lib/api/projects.test.ts`
  - Add URL encoding and payload tests for new client functions.
- Create `apps/web/src/features/projects/components/project-runtime-placement-panel.tsx`
  - Render placement, readiness, selectable Runtime nodes, and bind/release actions.
- Modify `apps/web/src/features/projects/components/project-operational-detail.tsx`
  - Include execution readiness status and placement panel entry point.
- Modify `apps/web/src/features/projects/index.tsx`
  - Query placement/readiness/runtime nodes and wire mutations.
- Modify `apps/web/src/features/workflows/index.tsx`
  - Surface coordination blocked facts from task graph/events/current blocker.
- Modify `apps/web/src/features/workflows/workflow-graph-adapter.ts`
  - Build a blocking node when task graph has a blocking fact but no executable nodes.

### Tests

- Modify `apps/control-plane/internal/project/service_test.go`
- Modify `apps/control-plane/internal/project/handler_test.go`
- Modify `apps/control-plane/internal/api/project_routes_test.go`
- Modify `apps/control-plane/internal/workflow/projectcoordination/project_store_test.go`
- Modify `apps/control-plane/internal/workflow/projectcoordination/predispatch_gate_test.go`
- Modify `apps/web/src/features/projects/index.test.tsx`
- Modify `apps/web/src/features/workflows/index.test.tsx`
- Modify `apps/web/src/features/workflows/workflow-graph-adapter.test.ts`

---

## Task 1: Storage, Domain Types, and Contract Skeleton

**Files:**
- Modify: `apps/control-plane/internal/storage/queries/project_runtime_affinity.sql`
- Modify generated: `apps/control-plane/internal/storage/queries/project_runtime_affinity.sql.go`
- Modify generated: `apps/control-plane/internal/storage/queries/querier.go`
- Modify: `apps/control-plane/internal/project/types.go`
- Modify: `apps/control-plane/internal/project/repository.go`
- Modify: `apps/control-plane/internal/project/pg_repository.go`
- Modify: `contracts/control-plane/openapi.yaml`

- [ ] **Step 1: Add a release query for active project placement**

Edit `apps/control-plane/internal/storage/queries/project_runtime_affinity.sql` and add this query after `GetActiveProjectPlacement`:

```sql
-- name: ReleaseProjectPlacement :one
UPDATE project_placements
SET
    placement_status = 'released',
    released_at = NOW(),
    updated_at = NOW()
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_id = sqlc.arg('project_id')::uuid
  AND placement_status = 'active'
RETURNING *;
```

- [ ] **Step 2: Regenerate sqlc outputs**

Run:

```bash
make -C apps/control-plane sqlc
```

Expected: generated query files compile and include `ReleaseProjectPlacement`.

If the local environment does not expose the repo's sqlc target, inspect `apps/control-plane/Makefile` and use the exact repo command. Do not hand-write generated sqlc files except as a last-resort repair that is clearly reported.

- [ ] **Step 3: Add project placement and readiness domain types**

In `apps/control-plane/internal/project/types.go`, add event constants:

```go
ProjectEventRuntimePlacementUpdated ProjectEventType = "project.runtime_placement.updated"
ProjectEventRuntimePlacementReleased ProjectEventType = "project.runtime_placement.released"
ProjectEventCoordinationBlocked      ProjectEventType = "coordination.blocked"
ProjectEventWorkflowCoordinationFailed ProjectEventType = "workflow.coordination_failed"
ProjectEventTaskDispatchBlocked      ProjectEventType = "project_task.dispatch_blocked"
```

Add these types near other project request/response types:

```go
type ProjectRuntimePlacementStatus string

const (
	ProjectRuntimePlacementStatusMissing                    ProjectRuntimePlacementStatus = "missing"
	ProjectRuntimePlacementStatusReady                      ProjectRuntimePlacementStatus = "ready"
	ProjectRuntimePlacementStatusRuntimeOffline             ProjectRuntimePlacementStatus = "runtime_offline"
	ProjectRuntimePlacementStatusCommandChannelDisconnected ProjectRuntimePlacementStatus = "command_channel_disconnected"
	ProjectRuntimePlacementStatusProviderUnavailable        ProjectRuntimePlacementStatus = "provider_unavailable"
	ProjectRuntimePlacementStatusCapacityFull               ProjectRuntimePlacementStatus = "capacity_full"
	ProjectRuntimePlacementStatusWorkspacePending           ProjectRuntimePlacementStatus = "workspace_pending"
	ProjectRuntimePlacementStatusContractMismatch           ProjectRuntimePlacementStatus = "contract_mismatch"
)

type ProjectRuntimePlacement struct {
	ID              uuid.UUID  `json:"id"`
	TenantID        uuid.UUID  `json:"tenant_id,omitempty"`
	ProjectID       uuid.UUID  `json:"project_id"`
	RuntimeNodeID   uuid.UUID  `json:"runtime_node_id"`
	PlacementStatus string     `json:"placement_status"`
	PlacementReason string     `json:"placement_reason,omitempty"`
	AssignedAt      time.Time  `json:"assigned_at,omitempty"`
	ReleasedAt      *time.Time `json:"released_at,omitempty"`
	CreatedAt       time.Time  `json:"created_at,omitempty"`
	UpdatedAt       time.Time  `json:"updated_at,omitempty"`
}

type PutProjectRuntimePlacementRequest struct {
	TenantID             uuid.UUID
	ProjectID            uuid.UUID
	RuntimeNodeID        uuid.UUID
	ActorUserID          uuid.UUID
	Reason               string
	ExpectedProviderTypes []string
}

type GetProjectRuntimePlacementRequest struct {
	TenantID  uuid.UUID
	ProjectID uuid.UUID
}

type ReleaseProjectRuntimePlacementRequest struct {
	TenantID    uuid.UUID
	ProjectID   uuid.UUID
	ActorUserID uuid.UUID
	Reason      string
}

type ProjectRuntimePlacementReadiness struct {
	PlacementStatus         ProjectRuntimePlacementStatus `json:"placement_status"`
	RuntimeNodeID           *uuid.UUID                    `json:"runtime_node_id,omitempty"`
	RuntimeNodeName         string                        `json:"runtime_node_name,omitempty"`
	CommandChannelConnected bool                          `json:"command_channel_connected"`
	ProviderCapabilities    []string                      `json:"provider_capabilities,omitempty"`
	RequiredProviderTypes   []string                      `json:"required_provider_types,omitempty"`
	EmployeeReadiness       []ProjectEmployeeReadiness    `json:"employee_readiness,omitempty"`
	BlockingReasons         []ProjectReadinessReason      `json:"blocking_reasons,omitempty"`
	NextActions             []ProjectReadinessAction      `json:"next_actions,omitempty"`
}

type ProjectEmployeeReadiness struct {
	DigitalEmployeeID uuid.UUID `json:"digital_employee_id"`
	DisplayName       string    `json:"display_name,omitempty"`
	ProviderType      string    `json:"provider_type,omitempty"`
	CanPlan           bool      `json:"can_plan"`
	CanDispatch       bool      `json:"can_dispatch"`
	ReasonCode        string    `json:"reason_code,omitempty"`
	ReasonMessage     string    `json:"reason_message,omitempty"`
}

type ProjectReadinessReason struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type ProjectReadinessAction struct {
	Code  string `json:"code"`
	Label string `json:"label"`
}
```

- [ ] **Step 4: Extend project repository interface**

In `apps/control-plane/internal/project/repository.go`, add methods to `Repository`:

```go
GetActiveProjectPlacement(ctx context.Context, tenantID, projectID uuid.UUID) (ProjectRuntimePlacement, error)
UpsertProjectPlacement(ctx context.Context, req PutProjectRuntimePlacementRequest) (ProjectRuntimePlacement, error)
ReleaseProjectPlacement(ctx context.Context, req ReleaseProjectRuntimePlacementRequest) (ProjectRuntimePlacement, error)
```

- [ ] **Step 5: Implement pg repository mapping**

In `apps/control-plane/internal/project/pg_repository.go`, implement the three methods using `queries.GetActiveProjectPlacementParams`, `queries.UpsertProjectPlacementParams`, and generated `queries.ReleaseProjectPlacementParams`.

Map `pgtype.Text` reason with repo-local helpers already used in this file. If no helper exists for text, add focused helpers:

```go
func pgTextFromString(value string) pgtype.Text {
	value = strings.TrimSpace(value)
	return pgtype.Text{String: value, Valid: value != ""}
}

func stringFromPgText(value pgtype.Text) string {
	if !value.Valid {
		return ""
	}
	return value.String
}
```

Expected mapping: `ProjectPlacement.RuntimeNodeID` becomes `ProjectRuntimePlacement.RuntimeNodeID`; nullable `ReleasedAt` maps to `*time.Time`.

- [ ] **Step 6: Add OpenAPI paths and schemas**

In `contracts/control-plane/openapi.yaml`, add paths:

```yaml
  /api/v1/projects/{projectId}/runtime-placement:
    get:
      operationId: getProjectRuntimePlacement
      summary: Get active Runtime placement for a project
      parameters:
        - $ref: "#/components/parameters/ProjectId"
      responses:
        "200":
          description: Project Runtime placement
          content:
            application/json:
              schema:
                $ref: "#/components/schemas/ProjectRuntimePlacement"
        "404":
          $ref: "#/components/responses/Error"
    put:
      operationId: putProjectRuntimePlacement
      summary: Bind a project to an active Runtime placement
      parameters:
        - $ref: "#/components/parameters/ProjectId"
      requestBody:
        required: true
        content:
          application/json:
            schema:
              $ref: "#/components/schemas/PutProjectRuntimePlacementRequest"
      responses:
        "200":
          description: Updated Project Runtime placement
          content:
            application/json:
              schema:
                $ref: "#/components/schemas/ProjectRuntimePlacement"
    delete:
      operationId: releaseProjectRuntimePlacement
      summary: Release active Runtime placement for a project
      parameters:
        - $ref: "#/components/parameters/ProjectId"
      responses:
        "200":
          description: Released Project Runtime placement
          content:
            application/json:
              schema:
                $ref: "#/components/schemas/ProjectRuntimePlacement"
  /api/v1/projects/{projectId}/runtime-readiness:
    get:
      operationId: getProjectRuntimeReadiness
      summary: Get project Runtime execution readiness
      parameters:
        - $ref: "#/components/parameters/ProjectId"
      responses:
        "200":
          description: Project Runtime readiness
          content:
            application/json:
              schema:
                $ref: "#/components/schemas/ProjectRuntimePlacementReadiness"
```

Add schemas matching the Go JSON fields. Keep enum values exactly aligned with `ProjectRuntimePlacementStatus`.

- [ ] **Step 7: Run contract verification**

Run:

```bash
corepack pnpm verify:contracts
```

Expected: PASS.

- [ ] **Step 8: Commit Task 1**

Run:

```bash
git add apps/control-plane/internal/storage/queries/project_runtime_affinity.sql apps/control-plane/internal/storage/queries/project_runtime_affinity.sql.go apps/control-plane/internal/storage/queries/querier.go apps/control-plane/internal/project/types.go apps/control-plane/internal/project/repository.go apps/control-plane/internal/project/pg_repository.go contracts/control-plane/openapi.yaml
git diff --cached --check
git commit -m "feat(control-plane): define project runtime placement contract"
```

## Task 2: Project Placement Service and HTTP API

**Files:**
- Modify: `apps/control-plane/internal/project/service.go`
- Modify: `apps/control-plane/internal/project/handler.go`
- Modify: `apps/control-plane/internal/api/server.go`
- Test: `apps/control-plane/internal/project/service_test.go`
- Test: `apps/control-plane/internal/project/handler_test.go`
- Test: `apps/control-plane/internal/api/project_routes_test.go`

- [ ] **Step 1: Write service tests for missing placement readiness**

Add `TestGetProjectRuntimeReadinessReportsMissingPlacement` to `apps/control-plane/internal/project/service_test.go`.

Arrange:
- A project with one active digital employee member.
- No active project placement.

Assert:
- `PlacementStatus == ProjectRuntimePlacementStatusMissing`.
- `BlockingReasons` contains `runtime_placement_missing`.
- `NextActions` contains `bind_runtime`.
- Employee readiness has `CanPlan=true` and `CanDispatch=false`.

- [ ] **Step 2: Write service tests for active placement readiness**

Add `TestPutProjectRuntimePlacementRecordsPlacementEventAndReadiness` to `apps/control-plane/internal/project/service_test.go`.

Arrange:
- A project.
- A runtime node record with `Status=online`, supported provider `codex`, available slot, and command channel connected if the fake exposes connection state.
- A digital employee member requiring `codex`.

Act:
- Call `PutProjectRuntimePlacement`.
- Call `GetProjectRuntimeReadiness`.

Assert:
- Placement runtime node id is persisted.
- Event type `project.runtime_placement.updated` is appended.
- Readiness status is `ready`.
- Required provider includes `codex`.

- [ ] **Step 3: Implement service methods**

In `apps/control-plane/internal/project/service.go`, add:

```go
func (s *Service) GetProjectRuntimePlacement(ctx context.Context, req GetProjectRuntimePlacementRequest) (*ProjectRuntimePlacement, error)
func (s *Service) PutProjectRuntimePlacement(ctx context.Context, req PutProjectRuntimePlacementRequest) (*ProjectRuntimePlacement, error)
func (s *Service) ReleaseProjectRuntimePlacement(ctx context.Context, req ReleaseProjectRuntimePlacementRequest) (*ProjectRuntimePlacement, error)
func (s *Service) GetProjectRuntimeReadiness(ctx context.Context, tenantID, projectID uuid.UUID) (*ProjectRuntimePlacementReadiness, error)
```

Validation:
- Reject nil tenant/project/runtime ids for put.
- Verify project exists and is not archived.
- Trim reason.
- Preserve tenant boundary.

Events:
- Put writes `ProjectEventRuntimePlacementUpdated`.
- Release writes `ProjectEventRuntimePlacementReleased`.

Readiness aggregation:
- Load project members.
- Compute required provider types from employee planning profile source or employee identity adapter. If the current service cannot access provider directly, use existing project member/profile reader used by planning; if unavailable, return `provider_type_missing` per employee and keep the project not ready.
- Load active placement; if absent return missing.
- Load Runtime node information from existing runtime service/repository dependency available to the app. If service currently lacks this dependency, add a narrow interface on project service construction:

```go
type ProjectRuntimeNodeReader interface {
	ListRuntimeNodesForTenant(ctx context.Context, params runtime.ListRuntimeNodesForTenantParams) ([]runtime.NodeRecord, error)
	ListRuntimeCapabilitiesForNode(ctx context.Context, tenantID uuid.UUID, nodeID string) ([]runtime.RuntimeCapability, error)
}
```

Prefer existing app wiring over duplicating SQL in project service.

- [ ] **Step 4: Add handler interface methods**

In `apps/control-plane/internal/project/handler.go`, extend `HandlerService`:

```go
GetProjectRuntimePlacement(ctx context.Context, req GetProjectRuntimePlacementRequest) (*ProjectRuntimePlacement, error)
PutProjectRuntimePlacement(ctx context.Context, req PutProjectRuntimePlacementRequest) (*ProjectRuntimePlacement, error)
ReleaseProjectRuntimePlacement(ctx context.Context, req ReleaseProjectRuntimePlacementRequest) (*ProjectRuntimePlacement, error)
GetProjectRuntimeReadiness(ctx context.Context, tenantID, projectID uuid.UUID) (*ProjectRuntimePlacementReadiness, error)
```

Add handlers:

```go
func (h *HTTPHandler) GetProjectRuntimePlacement(w http.ResponseWriter, r *http.Request)
func (h *HTTPHandler) PutProjectRuntimePlacement(w http.ResponseWriter, r *http.Request)
func (h *HTTPHandler) ReleaseProjectRuntimePlacement(w http.ResponseWriter, r *http.Request)
func (h *HTTPHandler) GetProjectRuntimeReadiness(w http.ResponseWriter, r *http.Request)
```

Follow existing handler patterns for tenant/current user extraction and JSON errors.

- [ ] **Step 5: Register routes**

In `apps/control-plane/internal/api/server.go`, inside authenticated project routes, add:

```go
r.Get("/projects/{projectId}/runtime-placement", s.projectHandler.GetProjectRuntimePlacement)
r.Put("/projects/{projectId}/runtime-placement", s.projectHandler.PutProjectRuntimePlacement)
r.Delete("/projects/{projectId}/runtime-placement", s.projectHandler.ReleaseProjectRuntimePlacement)
r.Get("/projects/{projectId}/runtime-readiness", s.projectHandler.GetProjectRuntimeReadiness)
```

- [ ] **Step 6: Write handler and route tests**

In `apps/control-plane/internal/project/handler_test.go`, add tests:
- `TestGetProjectRuntimePlacement`
- `TestPutProjectRuntimePlacement`
- `TestReleaseProjectRuntimePlacement`
- `TestGetProjectRuntimeReadiness`

In `apps/control-plane/internal/api/project_routes_test.go`, assert the four new routes are registered with auth.

- [ ] **Step 7: Run project tests**

Run:

```bash
go test ./apps/control-plane/internal/project ./apps/control-plane/internal/api -count=1
```

Expected: PASS.

- [ ] **Step 8: Commit Task 2**

Run:

```bash
git add apps/control-plane/internal/project/service.go apps/control-plane/internal/project/handler.go apps/control-plane/internal/api/server.go apps/control-plane/internal/project/service_test.go apps/control-plane/internal/project/handler_test.go apps/control-plane/internal/api/project_routes_test.go
git diff --cached --check
git commit -m "feat(control-plane): expose project runtime placement readiness"
```

## Task 3: Split Planning Eligibility From Dispatch Readiness

**Files:**
- Modify: `apps/control-plane/internal/workflow/projectcoordination/planning_profile.go`
- Modify: `apps/control-plane/internal/workflow/projectcoordination/project_store.go`
- Test: `apps/control-plane/internal/workflow/projectcoordination/planning_profile_test.go`
- Test: `apps/control-plane/internal/workflow/projectcoordination/project_store_test.go`

- [ ] **Step 1: Write failing snapshot test for non-dispatchable but plannable employees**

In `apps/control-plane/internal/workflow/projectcoordination/project_store_test.go`, add a test named `TestLoadProjectCoordinationSnapshotKeepsPlannableEmployeesWhenRuntimeNotReady`.

Arrange:
- Project has one active executor digital employee.
- Fake employee reader returns employee policy allowed but runtime snapshot with `NodeOnline=false` and `WorkspaceReady=false`.

Act:
- Call `LoadProjectCoordinationSnapshot`.

Assert:
- `DigitalEmployeePool` length is 1.
- Profile includes runtime warning/hint showing dispatch not ready.
- No hard failure removes the employee.

- [ ] **Step 2: Add readiness hint fields to planning profile**

In `planning_profile.go`, extend `PlanningRuntimeRequirements`:

```go
DispatchReadinessStatus string   `json:"dispatch_readiness_status,omitempty"`
DispatchBlockingReasons []string `json:"dispatch_blocking_reasons,omitempty"`
```

Change `buildPlanningRuntimeRequirements` so `runtimeReady=false` sets:

```go
DispatchReadinessStatus: "not_ready"
DispatchBlockingReasons: []string{"runtime_not_ready"}
```

but does not append to `HardFailures`.

- [ ] **Step 3: Stop filtering candidates by runtime readiness**

In `LoadProjectCoordinationSnapshot`, keep `readyEmployees := s.runtimeReadyEmployeeIDs(...)` for hints, but remove this candidate exclusion:

```go
if readyEmployees != nil && !readyEmployees[member.PrincipalID] {
	continue
}
```

Keep role/status/lending filters unchanged.

- [ ] **Step 4: Add explicit no-plannable employee blocked event**

If eligible candidates are empty after role/status/lending filters, append a project event:

```go
EventType: project.ProjectEventCoordinationBlocked
Summary: "项目没有可参与规划的数字员工"
Payload: map[string]any{
  "reason_code": "no_plannable_digital_employee",
  "demand_id": input.DemandID.String(),
}
```

Do not treat runtime readiness absence as this reason.

- [ ] **Step 5: Run coordination tests**

Run:

```bash
go test ./apps/control-plane/internal/workflow/projectcoordination -run 'TestLoadProjectCoordinationSnapshot|TestBuildDigitalEmployeePlanningProfile|TestPlanningProfile' -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit Task 3**

Run:

```bash
git add apps/control-plane/internal/workflow/projectcoordination/planning_profile.go apps/control-plane/internal/workflow/projectcoordination/project_store.go apps/control-plane/internal/workflow/projectcoordination/planning_profile_test.go apps/control-plane/internal/workflow/projectcoordination/project_store_test.go
git diff --cached --check
git commit -m "fix(coordination): keep plannable employees before dispatch readiness"
```

## Task 4: Dispatch Blocking Facts and Task Graph Visibility

**Files:**
- Modify: `apps/control-plane/internal/workflow/projectcoordination/predispatch_gate.go`
- Modify: `apps/control-plane/internal/workflow/projectcoordination/project_store.go`
- Modify: `apps/control-plane/internal/project/task_graph_types.go`
- Modify: `apps/control-plane/internal/project/service.go` or `pg_repository.go` if task graph aggregation needs blocking events
- Test: `apps/control-plane/internal/workflow/projectcoordination/predispatch_gate_test.go`
- Test: `apps/control-plane/internal/workflow/projectcoordination/project_store_test.go`
- Test: `apps/control-plane/internal/project/handler_test.go`

- [ ] **Step 1: Write pre-dispatch gate tests for missing placement**

Add `TestRunPreDispatchGateBlocksMissingProjectPlacement` in `predispatch_gate_test.go`.

Expected gate:
- `AllowRunStart=false`
- `Terminal=false`
- `Retryable=false`
- reason code `runtime_placement_missing`
- recommended action `bind_runtime`

- [ ] **Step 2: Normalize dispatch reason codes**

In `predispatch_gate.go`, ensure gate results can produce stable reason codes:

```go
runtime_placement_missing
runtime_node_offline
command_channel_disconnected
provider_unavailable
capacity_full
workspace_pending
contract_mismatch
```

Keep raw errors in diagnostic payload only.

- [ ] **Step 3: Record blocked dispatch fact**

In `ProjectStore.DispatchProjectTask`, when `!gate.AllowRunStart`, append `ProjectEventTaskDispatchBlocked` with payload:

```go
map[string]any{
  "project_task_id": task.ID.String(),
  "demand_id": task.DemandID.String(),
  "reason_code": gate.PrimaryReasonCode,
  "reason_message": gate.PrimaryReasonMessage,
  "recommended_action": gate.RecommendedAction,
  "dispatch_gate_result_id": gate.Gate.ID.String(),
}
```

If `gate.Retryable`, still return `ErrProjectTaskDispatchRetryLater` after recording the event.

- [ ] **Step 4: Create or expose a blocking task graph node**

Extend `ProjectTaskGraph` response to include a blocking fact when there are no task nodes but demand launch events contain `coordination.blocked`, `workflow.coordination_failed`, or `project_task.dispatch_blocked`.

Preferred shape:
- Add `blocking_facts []ProjectTaskGraphBlockingFact` to `ProjectTaskGraph`.
- Each fact includes `reason_code`, `message`, `resource_type`, `resource_id`, `recommended_action`, `created_at`.

Do not invent fake ProjectTask rows in the database just to draw the UI.

- [ ] **Step 5: Write task graph blocked fact test**

In `apps/control-plane/internal/project/handler_test.go`, add `TestGetProjectTaskGraphReturnsBlockingFactWhenCoordinationBlocked`.

Arrange:
- A demand.
- A project event `coordination.blocked` with reason `runtime_placement_missing`.
- No project tasks.

Assert:
- response `blocking_facts` has one item.
- nodes remains empty.
- reason code is stable.

- [ ] **Step 6: Run focused tests**

Run:

```bash
go test ./apps/control-plane/internal/workflow/projectcoordination ./apps/control-plane/internal/project -run 'TestRunPreDispatchGateBlocksMissingProjectPlacement|TestDispatchProjectTask|TestGetProjectTaskGraphReturnsBlockingFact' -count=1
```

Expected: PASS.

- [ ] **Step 7: Commit Task 4**

Run:

```bash
git add apps/control-plane/internal/workflow/projectcoordination/predispatch_gate.go apps/control-plane/internal/workflow/projectcoordination/project_store.go apps/control-plane/internal/project/task_graph_types.go apps/control-plane/internal/project/service.go apps/control-plane/internal/project/pg_repository.go apps/control-plane/internal/workflow/projectcoordination/predispatch_gate_test.go apps/control-plane/internal/workflow/projectcoordination/project_store_test.go apps/control-plane/internal/project/handler_test.go
git diff --cached --check
git commit -m "feat(coordination): persist project dispatch blocking facts"
```

## Task 5: Workflow Failure Projection

**Files:**
- Modify: `apps/control-plane/internal/project/service.go`
- Modify: `apps/control-plane/internal/workflow/projectcoordination/workflow.go`
- Test: `apps/control-plane/internal/project/service_test.go`
- Test: `apps/control-plane/internal/workflow/projectcoordination/workflow_test.go`

- [ ] **Step 1: Write service test for workflow signal failure projection**

Extend or add a test near `TestSubmitDemandRecordsRetryableWorkflowSignalFailure`.

Arrange:
- Coordinator signal returns an error equivalent to empty planner pool or known coordination blocked reason.

Assert:
- Existing `workflow.signal_failed` event is still recorded.
- New `workflow.coordination_failed` event is recorded with `reason_code`.
- Payload contains `recommended_action`.

- [ ] **Step 2: Implement known error projection helper**

In `project/service.go`, add:

```go
func workflowCoordinationFailurePayload(signalName string, err error, extra map[string]any) (summary string, payload map[string]any)
```

Map known text defensively:
- Contains `digital_employee_pool is empty` -> `no_plannable_digital_employee`
- Contains `runtime_placement_missing` -> `runtime_placement_missing`
- Contains `provider_unavailable` -> `provider_unavailable`

Default:
- `reason_code=workflow_signal_failed`
- recommended action: inspect coordination event payload.

- [ ] **Step 3: Append projected event when workflow signal fails**

Where `appendWorkflowSignalEvent(..., "failed", err, ...)` is called, also append `ProjectEventWorkflowCoordinationFailed` with the helper payload.

Do not suppress the original event.

- [ ] **Step 4: Add workflow-level test if coordinator owns a failure path**

If `workflow.go` records `workflow.signal_failed` directly, add a workflow test asserting it records a readable projected event. If the project service owns all failure recording for demand submission, document this in the test name and skip workflow.go changes.

- [ ] **Step 5: Run focused tests**

Run:

```bash
go test ./apps/control-plane/internal/project ./apps/control-plane/internal/workflow/projectcoordination -run 'TestSubmitDemandRecordsRetryableWorkflowSignalFailure|Test.*Workflow.*Coordination.*Failed|Test.*Signal.*Failed' -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit Task 5**

Run:

```bash
git add apps/control-plane/internal/project/service.go apps/control-plane/internal/project/service_test.go apps/control-plane/internal/workflow/projectcoordination/workflow.go apps/control-plane/internal/workflow/projectcoordination/workflow_test.go
git diff --cached --check
git commit -m "feat(project): expose workflow coordination failure reasons"
```

## Task 6: Web API Client for Placement and Readiness

**Files:**
- Modify: `apps/web/src/lib/api/projects.ts`
- Test: `apps/web/src/lib/api/projects.test.ts`

- [ ] **Step 1: Add API client tests**

In `apps/web/src/lib/api/projects.test.ts`, add tests:
- `getProjectRuntimePlacement encodes project id`
- `putProjectRuntimePlacement posts runtime node and reason`
- `releaseProjectRuntimePlacement deletes active placement`
- `getProjectRuntimeReadiness encodes project id`

Assert exact URLs:

```text
/api/v1/projects/project%201%2Fprimary/runtime-placement
/api/v1/projects/project%201%2Fprimary/runtime-readiness
```

- [ ] **Step 2: Add TypeScript types**

In `apps/web/src/lib/api/projects.ts`, add:

```ts
export type ProjectRuntimePlacementStatus =
  | "missing"
  | "ready"
  | "runtime_offline"
  | "command_channel_disconnected"
  | "provider_unavailable"
  | "capacity_full"
  | "workspace_pending"
  | "contract_mismatch";

export type ProjectRuntimePlacement = {
  id: string;
  project_id: string;
  runtime_node_id: string;
  placement_status: string;
  placement_reason?: string;
  assigned_at?: string;
  released_at?: string;
  created_at?: string;
  updated_at?: string;
};

export type ProjectRuntimePlacementReadiness = {
  placement_status: ProjectRuntimePlacementStatus;
  runtime_node_id?: string;
  runtime_node_name?: string;
  command_channel_connected: boolean;
  provider_capabilities?: string[];
  required_provider_types?: string[];
  employee_readiness?: ProjectEmployeeReadiness[];
  blocking_reasons?: ProjectReadinessReason[];
  next_actions?: ProjectReadinessAction[];
};
```

Add the supporting employee/reason/action types.

- [ ] **Step 3: Add client functions**

In `apps/web/src/lib/api/projects.ts`, add:

```ts
export function getProjectRuntimePlacement(options: ApiClientOptions, projectId: string): Promise<ProjectRuntimePlacement> {
  return getJson<ProjectRuntimePlacement>(options, projectPath(projectId, "/runtime-placement"), "project runtime placement");
}

export function putProjectRuntimePlacement(
  options: ApiClientOptions,
  projectId: string,
  input: { runtime_node_id: string; reason?: string; expected_provider_types?: string[] },
): Promise<ProjectRuntimePlacement> {
  return putJson<ProjectRuntimePlacement>(options, projectPath(projectId, "/runtime-placement"), input, "project runtime placement");
}

export function releaseProjectRuntimePlacement(options: ApiClientOptions, projectId: string): Promise<ProjectRuntimePlacement> {
  return deleteJson<ProjectRuntimePlacement>(options, projectPath(projectId, "/runtime-placement"), "project runtime placement");
}

export function getProjectRuntimeReadiness(options: ApiClientOptions, projectId: string): Promise<ProjectRuntimePlacementReadiness> {
  return getJson<ProjectRuntimePlacementReadiness>(options, projectPath(projectId, "/runtime-readiness"), "project runtime readiness");
}
```

If `deleteJson` does not exist in `apps/web/src/lib/api/client.ts`, add it there with the same error handling pattern as `putJson`.

- [ ] **Step 4: Run API client tests**

Run:

```bash
corepack pnpm --filter ./apps/web run test -- src/lib/api/projects.test.ts
```

Expected: PASS.

- [ ] **Step 5: Commit Task 6**

Run:

```bash
git add apps/web/src/lib/api/projects.ts apps/web/src/lib/api/projects.test.ts apps/web/src/lib/api/client.ts
git diff --cached --check
git commit -m "feat(web): add project runtime placement API client"
```

## Task 7: Project Runtime Placement UI

**Files:**
- Create: `apps/web/src/features/projects/components/project-runtime-placement-panel.tsx`
- Modify: `apps/web/src/features/projects/components/project-operational-detail.tsx`
- Modify: `apps/web/src/features/projects/index.tsx`
- Test: `apps/web/src/features/projects/index.test.tsx`

- [ ] **Step 1: Read design guidance**

Run:

```bash
sed -n '1,220p' DESIGN.md
```

Use the existing SuperTeam design system. Keep this as a work surface panel, not a marketing card.

- [ ] **Step 2: Write UI test for missing placement**

In `apps/web/src/features/projects/index.test.tsx`, add or extend a project detail test:
- Mock `GET /runtime-readiness` with `placement_status: "missing"`.
- Mock `GET /runtime/nodes` with one online `local-dev-node` supporting `codex`.
- Assert the page shows missing placement reason and a bind action.

- [ ] **Step 3: Write UI test for binding runtime**

In the same test file:
- User selects `local-dev-node`.
- Clicks bind action.
- Assert `PUT /api/v1/projects/project-1/runtime-placement` body includes `runtime_node_id`.
- Assert queries for readiness/task graph are invalidated/refetched.

- [ ] **Step 4: Create placement panel**

Create `project-runtime-placement-panel.tsx`.

Props:

```ts
type ProjectRuntimePlacementPanelProps = {
  readiness?: ProjectRuntimePlacementReadiness;
  runtimeNodes: RuntimeNodeResponse[];
  selectedRuntimeNodeId: string;
  onSelectedRuntimeNodeIdChange: (nodeId: string) => void;
  onBindRuntime: () => void;
  onReleaseRuntime: () => void;
  isBinding: boolean;
  isReleasing: boolean;
};
```

Render:
- Status label from readiness.
- Blocking reasons list.
- Runtime select using online nodes first.
- Provider capabilities summary.
- Button with a link/plug icon or server icon from lucide-react if already used in the app.

- [ ] **Step 5: Wire project detail**

In `project-operational-detail.tsx`, add a slot prop for runtime placement panel:

```ts
runtimePlacementPanel?: ReactNode;
```

Render it near task launch/execution readiness, before task graph.

- [ ] **Step 6: Wire queries and mutations**

In `apps/web/src/features/projects/index.tsx`:
- Query `getProjectRuntimeReadiness` for selected project.
- Query `listRuntimeNodes` for placement selection.
- Mutation `putProjectRuntimePlacement`.
- Mutation `releaseProjectRuntimePlacement`.
- Invalidate:
  - `["project-runtime-readiness", projectId]`
  - `["project-task-graph", projectId]`
  - `["project-events", projectId]`
  - `["project-overview", projectId]`

- [ ] **Step 7: Run project UI tests**

Run:

```bash
corepack pnpm --filter ./apps/web run test -- src/features/projects/index.test.tsx
```

Expected: PASS.

- [ ] **Step 8: Commit Task 7**

Run:

```bash
git add apps/web/src/features/projects/components/project-runtime-placement-panel.tsx apps/web/src/features/projects/components/project-operational-detail.tsx apps/web/src/features/projects/index.tsx apps/web/src/features/projects/index.test.tsx
git diff --cached --check
git commit -m "feat(web): show project runtime placement readiness"
```

## Task 8: Workflow and Task Graph Blocking UI

**Files:**
- Modify: `apps/web/src/lib/api/projects.ts`
- Modify: `apps/web/src/features/workflows/workflow-graph-adapter.ts`
- Modify: `apps/web/src/features/workflows/components/workflow-task-node.tsx`
- Modify: `apps/web/src/features/workflows/index.tsx`
- Test: `apps/web/src/features/workflows/workflow-graph-adapter.test.ts`
- Test: `apps/web/src/features/workflows/index.test.tsx`

- [ ] **Step 1: Extend task graph client type**

In `apps/web/src/lib/api/projects.ts`, extend `ProjectTaskGraph` with:

```ts
blocking_facts?: ProjectTaskGraphBlockingFact[];
```

Add:

```ts
export type ProjectTaskGraphBlockingFact = {
  reason_code: string;
  message: string;
  resource_type?: string;
  resource_id?: string;
  recommended_action?: string;
  created_at?: string;
};
```

- [ ] **Step 2: Write graph adapter test**

In `workflow-graph-adapter.test.ts`, add a test:
- Input graph has no nodes.
- Input graph has `blocking_facts` with `runtime_placement_missing`.
- Assert adapter returns one node with blocked status and readable title.

- [ ] **Step 3: Add blocked graph node adapter logic**

In `workflow-graph-adapter.ts`, when graph nodes are empty and `blocking_facts` is non-empty, return a synthetic UI node:

```ts
id: `blocking-${fact.reason_code}`
type: "task"
data.status: "blocked"
data.title: fact.message
data.subtitle: fact.recommended_action
```

This is a UI node only. It must not be posted back as a ProjectTask.

- [ ] **Step 4: Surface blocked state in workflow page**

In `apps/web/src/features/workflows/index.tsx`, display a concise blocking banner when selected task graph has blocking facts:

```text
协调已阻塞：<message>
下一步：<recommended_action>
```

Keep existing graph visible below.

- [ ] **Step 5: Update workflow node rendering if needed**

If `WorkflowTaskNode` lacks blocked styling for this node shape, add the minimal blocked tone using existing status tone conventions.

- [ ] **Step 6: Run workflow tests**

Run:

```bash
corepack pnpm --filter ./apps/web run test -- src/features/workflows/workflow-graph-adapter.test.ts src/features/workflows/index.test.tsx src/features/workflows/components/workflow-task-node.test.tsx
```

Expected: PASS.

- [ ] **Step 7: Commit Task 8**

Run:

```bash
git add apps/web/src/lib/api/projects.ts apps/web/src/features/workflows/workflow-graph-adapter.ts apps/web/src/features/workflows/components/workflow-task-node.tsx apps/web/src/features/workflows/index.tsx apps/web/src/features/workflows/workflow-graph-adapter.test.ts apps/web/src/features/workflows/index.test.tsx apps/web/src/features/workflows/components/workflow-task-node.test.tsx
git diff --cached --check
git commit -m "feat(web): show workflow coordination blockers"
```

## Task 9: Integrated Verification Gates

**Files:**
- Modify as needed only if previous tasks expose test failures.
- Add `CHANGELOG.md` entry after feature is implemented.

- [ ] **Step 1: Run backend focused tests**

Run:

```bash
go test ./apps/control-plane/internal/project ./apps/control-plane/internal/api ./apps/control-plane/internal/workflow/projectcoordination -count=1
```

Expected: PASS.

- [ ] **Step 2: Run Web focused tests**

Run:

```bash
corepack pnpm --filter ./apps/web run test -- src/lib/api/projects.test.ts src/features/projects/index.test.tsx src/features/workflows/workflow-graph-adapter.test.ts src/features/workflows/index.test.tsx
```

Expected: PASS.

- [ ] **Step 3: Run contract verification**

Run:

```bash
corepack pnpm verify:contracts
```

Expected: PASS.

- [ ] **Step 4: Run migration validation if sql changed**

Run:

```bash
make -C apps/control-plane migrate-validate
```

Expected: PASS.

If Docker or the dev database is unavailable, report the exact blocker. Do not claim database validation passed.

- [ ] **Step 5: Add changelog entry**

Run:

```bash
TZ=Asia/Shanghai date '+%Y-%m-%d %H:%M'
```

Add an entry to `CHANGELOG.md`:

```markdown
- YYYY-MM-DD HH:MM: 将项目 Runtime Placement 做成一等配置与诊断事实，拆分规划资格和派发就绪，并在项目与 Workflow 页面展示协调阻塞原因。
```

- [ ] **Step 6: Commit Task 9**

Run:

```bash
git add CHANGELOG.md
git diff --cached --check
git commit -m "chore: record project runtime placement coordination rollout"
```

## Task 10: Real End-to-End Smoke

**Files:**
- No code edits unless the smoke exposes a bug. If a bug is exposed, fix it in a separate scoped commit with failing evidence first.

- [ ] **Step 1: Confirm services**

Run:

```bash
scripts/dev-services.sh status
```

Expected:
- Temporal healthy.
- Control Plane healthy.
- Web healthy.
- Runtime Agent running.

- [ ] **Step 2: Restart changed services**

Run:

```bash
scripts/dev-services.sh restart control-plane
scripts/dev-services.sh restart web
scripts/dev-services.sh restart runtime-agent
```

Expected: all three services restart successfully and status is healthy after restart.

- [ ] **Step 3: Confirm runtime provider readiness**

Use authenticated curl or Web API evidence:

```bash
curl -sS http://127.0.0.1:8081/health
```

Then verify `/api/v1/runtime/overview` with the repo's dev auth cookie/session. Expected:
- `local-dev-node` exists.
- `command_channel_connected=true`.
- `supported_providers` includes `codex`.

- [ ] **Step 4: Run Chrome E2E**

Using Chrome plug against `http://127.0.0.1:3000`:

1. Log in with local dev credentials.
2. Create teams:
   - `主机运维团队 <timestamp>`
   - `网络运维团队 <timestamp>`
   - `故障分析团队 <timestamp>`
3. Create employees:
   - `主机运维数字员工 <timestamp>` with provider `codex`
   - `网络运维数字员工 <timestamp>` with provider `codex`
   - `故障分析数字员工 <timestamp>` with provider `codex`
4. Create project:
   - `系统健康分析项目 <timestamp>`
5. Add the three employees as executor pool.
6. Bind project Runtime placement to `local-dev-node`.
7. Submit demand:
   - `深入分析当前操作系统整体健康运行状态`
8. Open Workflow and Project detail.

Expected:
- Project readiness shows `ready` before demand submission.
- Route decision is created.
- Task graph has real task nodes or a visible blocking node with reason.
- If dispatch proceeds, ProjectTask attempt/run records project id, demand id, employee id, provider `codex`, and runtime node id.
- Runtime/provider emits real execution events or an explicit provider dependency blocker.

- [ ] **Step 5: Verify via API**

Use authenticated API calls for the created project/demand:

```bash
GET /api/v1/projects/{project_id}/runtime-placement
GET /api/v1/projects/{project_id}/runtime-readiness
GET /api/v1/projects/{project_id}/route-decisions
GET /api/v1/projects/{project_id}/task-graph?demand_id={demand_id}
GET /api/v1/projects/{project_id}/events?limit=50
GET /api/v1/runtime/overview
```

Expected:
- placement references `local-dev-node` runtime node id.
- readiness is `ready`, or shows an accurate blocker.
- route decisions are not empty if planning succeeded.
- task graph is not blank without explanation.
- events contain readable coordination/dispatch facts.

- [ ] **Step 6: Completion gate**

Run:

```bash
git diff --check
```

Then apply `$superteam-completion-check` before final reporting.

If real provider credentials, provider API availability, safe workspace, auth session, migration, or service startup blocks the smoke, final status must be:

```text
阻塞：<exact dependency>; 尚不能声明 ProjectTask execution path 完成。
```

Do not replace this smoke with unit tests when claiming the cross-layer feature is usable.
