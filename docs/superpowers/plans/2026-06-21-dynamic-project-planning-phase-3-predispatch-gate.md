# Dynamic Project Planning Phase 3 PreDispatchGate Implementation Plan

> 复核状态：06-21动态项目编排v1设计落地

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a durable Control Plane `PreDispatchGate` that must pass before any accepted `ProjectTask` creates a Runtime run or queued attempt.

**Architecture:** Keep policy, eligibility, audit, human-action creation, and idempotency in the Control Plane. Persist every gate evaluation before Runtime run creation, link the passed gate result to the created ProjectTask attempt, and reuse the existing Runtime run starter only after the gate returns `passed`. Runtime Agent and Provider adapters remain execution-only and do not decide business policy.

**Tech Stack:** Go Control Plane, PostgreSQL migrations, sqlc, Temporal workflow/activity package `projectcoordination`, existing approval and inbox services, OpenAPI contract generation, React Query project console, Vitest.

---

## Revision Notes (review pass 2026-06-21)

This plan was patched against the live codebase to fix four blocking issues found during review. Implementers do not need to re-derive these; the code blocks below already reflect the fixes.

- **B1 — constructor arity.** `NewProjectStoreWithApprovalsInboxAndRunStarter` takes four arguments `(repository, approvals, inbox, runStarter)`. All test call sites now pass a fourth `runStarter` argument (Tasks 4 and 5), and the `app.go` wiring keeps the real four-argument constructor plus its existing `.With*` chain.
- **B2 — employee/runtime data source.** `DigitalEmployeeExecutionInstanceRecord` has no `ConcurrencySlots`/`CurrentLoad` (those live on `runtime.NodeRecord`), `RuntimeNodeID` is a value `uuid.UUID` (not a pointer), and the employee record field is `Name` (not `DisplayName`). The Task 7 adapter now sources load/slot/online facts from `runtime.NodeRecord` (`MaxSlots`, `CurrentLoad`, `Status`, heartbeat freshness) looked up via the runtime *repository*; the Task 7 test uses a `fakeRuntimeNodeReader`. This also makes the Task 9 Step 5 "Runtime unavailable → retry_later" smoke actually detectable.
- **B3 — handler conventions.** The Task 8 handler uses the real `*HTTPHandler` receiver, `consoleIdentity`/`projectIDFromRequest`/`taskIDFromRequest` helpers, `writeJSON`/`writeHandlerError`, and chi camelCase params `{projectId}`/`{taskId}` (registered next to the existing `/projects/{projectId}/plan-revisions` route). The handler test sets `middleware.TenantIDKey`/`UserIDKey` and a chi route context with `projectId`+`taskId`.
- **B4 — composite FK target.** The two composite FKs from `project_task_attempts` and `project_tasks` reference `project_task_dispatch_gate_results(tenant_id, project_task_id, id)`. PostgreSQL requires that referenced column set to be backed by a unique constraint/index, so migration `032` now also creates `uq_project_task_dispatch_gate_results_tenant_task_id`, asserted by the migration test.

Auxiliary additions the implementer must make alongside the patched blocks (not spelled out inline): a `fakeRuntimeNodeReader` test double and a `runtimeNodeHeartbeatTTL` const (e.g. `2 * time.Minute`) with a `"time"` import in `planning_profile_adapter.go`; a `dispatchGates []PreDispatchGateResult` field plus `ListPreDispatchGateResults` method on the `handlerTestService` fake in `handler_test.go`; and a `dispatchGateResponses([]PreDispatchGateResult) []dispatchGateResponse` helper in `handler.go`.

## Execution Preflight

Use an isolated worktree for implementation because this phase touches migrations, generated sqlc, workflow tests, app wiring, API contracts, Web tests, and live dispatch behavior.

```bash
git status --short
git worktree add ../SuperTeam-dynamic-planning-phase-3 -b codex/dynamic-planning-phase-3
cd ../SuperTeam-dynamic-planning-phase-3
git status --short
```

Expected:

- root checkout may contain unrelated Phase 2 work
- implementation worktree should be clean
- branch should be `codex/dynamic-planning-phase-3`

Read these docs before editing:

```bash
sed -n '1,260p' docs/superpowers/specs/2026-06-21-dynamic-project-planning-orchestration-v1-phase-3-predispatch-gate.md
sed -n '1,420p' docs/superpowers/specs/2026-06-21-dynamic-project-planning-orchestration-v1-design.md
sed -n '1,240p' DATABASE_DESIGN.md
sed -n '1,220p' DESIGN.md
```

Current code facts to preserve:

- `apps/control-plane/internal/workflow/projectcoordination/project_store.go` starts the Runtime run inside `DispatchProjectTask` before `QueueProjectTaskWithAttempt`. Phase 3 inserts the persisted gate before `StartProjectTaskRun`.
- `QueueProjectTaskWithAttempt` in `apps/control-plane/internal/project/pg_repository.go` is already idempotent by attempt idempotency key and only accepts `planned` or `waiting_human`.
- `MoveProjectTaskToWaitingHuman` in `apps/control-plane/internal/storage/queries/project.sql` only accepts `queued` or `running`; keep that Runtime writeback rule and add a separate pre-dispatch transition for `planned`.
- `project_task_attempts` already has active-attempt uniqueness for `queued`, `running`, and `waiting_human`. Do not create an attempt when the gate returns `waiting_human`, `blocked`, `retry_later`, or `replan_required`.
- Phase 2 migration `031_project_plan_revisions.sql` is already used. Phase 3 migration number is `032_project_task_dispatch_gates.sql`.

## Scope Check

This phase touches one coherent subsystem: safe ProjectTask dispatch from Control Plane to Runtime. The Web change is a read-only visibility slice for the persisted gate facts and should not expand into a new planning workbench.

Do not implement Phase 4 result-contract release logic in this plan. Do not move provider/session execution policy into Runtime Agent. Do not add a separate manual dispatch API unless product scope explicitly changes; if a manual dispatch endpoint is later added, it must call the same `DispatchProjectTask` path described here.

## File Structure

- Create `apps/control-plane/internal/project/predispatch_gate.go`: domain constants, input/result structs, pure gate evaluator, status-to-task transition helpers, idempotency token helpers, and sanitized blocker/check payload builders.
- Create `apps/control-plane/internal/project/predispatch_gate_test.go`: pure evaluator tests for `passed`, `waiting_human`, `blocked`, `retry_later`, `replan_required`, active-attempt blocking, dependency blocking, missing context, high-risk approval, and deterministic idempotency.
- Modify `apps/control-plane/internal/project/types.go`: add `ProjectEventTaskDispatchGateChecked`, `ProjectEventTaskDispatchGateBlocked`, `ProjectEventTaskDispatchGateWaitingHuman`, `ProjectEventTaskDispatchGateRetryLater`, and `ProjectEventTaskDispatchGateReplanRequired`; add `HumanWaitReasonRuntimeRecovery` and `HumanWaitReasonBudgetApproval`.
- Modify `apps/control-plane/internal/project/repository.go`: add `PreDispatchGateResult` structs and repository methods for recording, reading, linking attempts, linking human requests, and moving a planned task to waiting-human because of a gate.
- Create `apps/control-plane/internal/storage/migrations/032_project_task_dispatch_gates.sql`: create `project_task_dispatch_gate_results`, link `project_task_attempts.dispatch_gate_result_id`, link `project_tasks.latest_dispatch_gate_result_id`, and add `project_decision_requests.dispatch_gate_result_id`.
- Modify `apps/control-plane/internal/storage/migrations/atlas.sum`: update through the repo migration workflow after adding migration `032`.
- Modify `apps/control-plane/internal/storage/migrations_test.go`: assert UUID-first design, tenant-first indexes, status checks, comments, and no secret-bearing raw details in gate columns.
- Modify `apps/control-plane/internal/storage/queries/project.sql`: add sqlc queries for gate insert/read/list/link and planned-to-waiting-human transition.
- Regenerate `apps/control-plane/internal/storage/queries/*.go` using the repo generator.
- Modify `apps/control-plane/internal/project/pg_repository.go`: implement gate persistence and mapping in existing transaction style.
- Modify `apps/control-plane/internal/project/pg_repository_test.go`: database-backed tests for gate idempotency, attempt linkage, human request linkage, planned-to-waiting-human transition, and retry-later record updates.
- Modify `apps/control-plane/internal/project/service.go`: extend valid human-wait reasons and decision type mapping for gate-created requests.
- Modify `apps/control-plane/internal/project/service_test.go`: cover new human-wait reasons and fake repository methods used by service tests.
- Create `apps/control-plane/internal/workflow/projectcoordination/predispatch_gate.go`: load gate context from repository/adapters, call the pure evaluator, persist gate result, create human action requests, and return a dispatch decision to `ProjectStore`.
- Create `apps/control-plane/internal/workflow/projectcoordination/predispatch_gate_test.go`: fake-store tests for status mapping, sanitized checks, human request creation, and idempotent re-entry.
- Modify `apps/control-plane/internal/workflow/projectcoordination/types.go`: add `DispatchReason` to `DispatchProjectTaskInput` and define gate decision result types used by activities.
- Modify `apps/control-plane/internal/workflow/projectcoordination/activities.go`: keep `DispatchProjectTask` as the only activity entry that can create a run; no separate run-start activity.
- Modify `apps/control-plane/internal/workflow/projectcoordination/project_store.go`: call the gate before `StartProjectTaskRun`, link gate result to attempt, record project events, and preserve current already-bound-run replay behavior.
- Modify `apps/control-plane/internal/workflow/projectcoordination/project_store_test.go`: prove gate statuses prevent run creation, passed gates create attempts, duplicate dispatch replays, and human approval causes the workflow to run the gate again.
- Modify `apps/control-plane/internal/workflow/projectcoordination/workflow.go`: pass dispatch reason values for root-ready, dependency-unlocked, human-resolved, and retry paths.
- Modify `apps/control-plane/internal/workflow/projectcoordination/workflow_test.go`: update dispatch activity expectations with reasons and verify retry-later remains retryable while blocked/replan are non-retryable.
- Modify `apps/control-plane/internal/app/app.go`: wire gate adapters to employee execution profile, runtime node readiness, MCP capability bindings, and approvals/inbox.
- Modify `apps/control-plane/internal/project/handler.go`: add read-only gate-result response mapping.
- Modify `apps/control-plane/internal/api/server.go`: register read-only route `GET /api/v1/projects/:project_id/tasks/:task_id/dispatch-gates`.
- Modify `apps/control-plane/internal/project/handler_test.go` and `apps/control-plane/internal/api/project_routes_test.go`: cover gate-result route registration and response shape.
- Modify `contracts/control-plane/openapi.yaml`: add dispatch gate schemas and read-only route.
- Modify generated client files from `corepack pnpm generate:control-plane`.
- Modify `apps/web/src/lib/api/projects.ts` and `apps/web/src/lib/api/projects.test.ts`: add gate-result types and fetcher.
- Modify `apps/web/src/features/projects/components/project-operational-detail.tsx`: show latest gate status in the task detail/timeline area using existing restrained console styling.
- Modify `apps/web/src/features/projects/index.test.tsx`: verify gate status, blockers, retry time, and linked human decision request render without changing task dispatch actions.

### Task 1: Domain Model And Pure Gate Evaluator

**Files:**
- Create: `apps/control-plane/internal/project/predispatch_gate.go`
- Create: `apps/control-plane/internal/project/predispatch_gate_test.go`
- Modify: `apps/control-plane/internal/project/types.go`

- [ ] **Step 1: Write failing evaluator tests**

Create `apps/control-plane/internal/project/predispatch_gate_test.go`:

```go
package project

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestEvaluatePreDispatchGatePassesReadyTask(t *testing.T) {
	now := time.Date(2026, 6, 21, 9, 30, 0, 0, time.UTC)
	projectID := uuid.MustParse("00000000-0000-0000-0000-000000000101")
	taskID := uuid.MustParse("00000000-0000-0000-0000-000000000102")
	employeeID := uuid.MustParse("00000000-0000-0000-0000-000000000103")
	revisionID := uuid.MustParse("00000000-0000-0000-0000-000000000104")
	key := "inspect-db"

	result := EvaluatePreDispatchGate(PreDispatchGateInput{
		ProjectID:              projectID,
		ProjectTaskID:          taskID,
		AcceptedPlanRevisionID: &revisionID,
		PlannedTaskKey:         &key,
		SelectedEmployeeID:     employeeID,
		AttemptNo:              1,
		DispatchReason:         DispatchReasonDependencyUnlocked,
	}, PreDispatchGateSnapshot{
		Task: ProjectTask{
			ID:                        taskID,
			ProjectID:                 projectID,
			Status:                    ProjectTaskStatusPlanned,
			AssignedDigitalEmployeeID: &employeeID,
			AcceptedPlanRevisionID:    &revisionID,
			PlannedTaskKey:            &key,
			MaxAttempts:               int32Ptr(3),
			AttemptCount:              0,
		},
		Employee: PreDispatchEmployeeSnapshot{
			ID:                  employeeID,
			IsProjectExecutor:   true,
			Status:              "active",
			PolicyAllowed:       true,
			RequiredLoadSlots:   1,
			AvailableLoadSlots:  1,
			ProfileSnapshotHash: "profile-hash",
		},
		Capabilities: PreDispatchCapabilitySnapshot{
			Required: []string{"database.read", "sql.analysis"},
			Matched:  []string{"database.read", "sql.analysis"},
		},
		Runtime: PreDispatchRuntimeSnapshot{
			NodeOnline:              true,
			ProviderAvailable:       true,
			WorkspaceReady:          true,
			SlotAvailable:           true,
			ContractVersionAccepted: true,
		},
		Budget:  PreDispatchBudgetSnapshot{ProjectBudgetAllowed: true, TaskBudgetPresent: true},
		Context: PreDispatchContextSnapshot{RequiredRefsResolved: true, InjectionAllowed: true},
	}, now)

	require.Equal(t, PreDispatchGateStatusPassed, result.Status)
	require.Empty(t, result.Blockers)
	require.Nil(t, result.HumanActionRequest)
	require.Equal(t, "project-task:00000000-0000-0000-0000-000000000102:reason:dependency_unlocked:attempt:1:employee:00000000-0000-0000-0000-000000000103", result.IdempotencyKey)
	require.Len(t, result.DispatchToken, 64)
	require.Contains(t, result.CheckKeys(), "task.dispatchable")
	require.Contains(t, result.CheckKeys(), "runtime.ready")
}

func TestEvaluatePreDispatchGateBlocksActiveAttempt(t *testing.T) {
	now := time.Date(2026, 6, 21, 9, 31, 0, 0, time.UTC)
	taskID := uuid.New()
	employeeID := uuid.New()
	activeAttemptID := uuid.New()

	result := EvaluatePreDispatchGate(PreDispatchGateInput{
		ProjectID:          uuid.New(),
		ProjectTaskID:      taskID,
		SelectedEmployeeID: employeeID,
		AttemptNo:          2,
		DispatchReason:     DispatchReasonRetry,
	}, PreDispatchGateSnapshot{
		Task: ProjectTask{
			ID:                        taskID,
			Status:                    ProjectTaskStatusPlanned,
			AssignedDigitalEmployeeID: &employeeID,
			CurrentAttemptID:          &activeAttemptID,
			AttemptCount:              1,
			MaxAttempts:               int32Ptr(3),
		},
		ActiveAttempt: &PreDispatchAttemptSnapshot{ID: activeAttemptID, Status: ProjectTaskAttemptStatusRunning},
	}, now)

	require.Equal(t, PreDispatchGateStatusBlocked, result.Status)
	require.Equal(t, "task.active_attempt_exists", result.Blockers[0].Key)
	require.False(t, result.CreateRun)
}

func TestEvaluatePreDispatchGateWaitsForHumanWhenRiskApprovalMissing(t *testing.T) {
	now := time.Date(2026, 6, 21, 9, 32, 0, 0, time.UTC)
	taskID := uuid.New()
	employeeID := uuid.New()
	risk := "high"

	result := EvaluatePreDispatchGate(PreDispatchGateInput{
		ProjectID:          uuid.New(),
		ProjectTaskID:      taskID,
		SelectedEmployeeID: employeeID,
		AttemptNo:          1,
		DispatchReason:     DispatchReasonRootReady,
	}, PreDispatchGateSnapshot{
		Task: ProjectTask{
			ID:                        taskID,
			Status:                    ProjectTaskStatusPlanned,
			AssignedDigitalEmployeeID: &employeeID,
			RiskLevel:                 &risk,
			MaxAttempts:               int32Ptr(1),
		},
		Employee: PreDispatchEmployeeSnapshot{ID: employeeID, IsProjectExecutor: true, Status: "active", PolicyAllowed: true, AvailableLoadSlots: 1, RequiredLoadSlots: 1},
		Runtime:  PreDispatchRuntimeSnapshot{NodeOnline: true, ProviderAvailable: true, WorkspaceReady: true, SlotAvailable: true, ContractVersionAccepted: true},
		Budget:   PreDispatchBudgetSnapshot{ProjectBudgetAllowed: true, TaskBudgetPresent: true},
		Context:  PreDispatchContextSnapshot{RequiredRefsResolved: true, InjectionAllowed: true},
		Risk:     PreDispatchRiskSnapshot{HumanApprovalRequired: true, HumanApprovalGranted: false, Reason: "database.write"},
	}, now)

	require.Equal(t, PreDispatchGateStatusWaitingHuman, result.Status)
	require.NotNil(t, result.HumanActionRequest)
	require.Equal(t, PreDispatchHumanActionRiskApproval, result.HumanActionRequest.Type)
	require.Equal(t, HumanWaitReasonApprovalRequired, result.HumanActionRequest.WaitingReason)
	require.False(t, result.CreateRun)
}

func TestEvaluatePreDispatchGateReturnsRetryLaterForRuntimeSlot(t *testing.T) {
	now := time.Date(2026, 6, 21, 9, 33, 0, 0, time.UTC)
	taskID := uuid.New()
	employeeID := uuid.New()

	result := EvaluatePreDispatchGate(PreDispatchGateInput{
		ProjectID:          uuid.New(),
		ProjectTaskID:      taskID,
		SelectedEmployeeID: employeeID,
		AttemptNo:          1,
		DispatchReason:     DispatchReasonRootReady,
	}, PreDispatchGateSnapshot{
		Task:     ProjectTask{ID: taskID, Status: ProjectTaskStatusPlanned, AssignedDigitalEmployeeID: &employeeID, MaxAttempts: int32Ptr(3)},
		Employee: PreDispatchEmployeeSnapshot{ID: employeeID, IsProjectExecutor: true, Status: "active", PolicyAllowed: true, AvailableLoadSlots: 1, RequiredLoadSlots: 1},
		Runtime:  PreDispatchRuntimeSnapshot{NodeOnline: true, ProviderAvailable: true, WorkspaceReady: true, SlotAvailable: false, RetryAfter: now.Add(2 * time.Minute), ContractVersionAccepted: true},
		Budget:   PreDispatchBudgetSnapshot{ProjectBudgetAllowed: true, TaskBudgetPresent: true},
		Context:  PreDispatchContextSnapshot{RequiredRefsResolved: true, InjectionAllowed: true},
	}, now)

	require.Equal(t, PreDispatchGateStatusRetryLater, result.Status)
	require.NotNil(t, result.RetryAfter)
	require.Equal(t, now.Add(2*time.Minute), *result.RetryAfter)
	require.Equal(t, "runtime.slot_unavailable", result.Blockers[0].Key)
}

func TestEvaluatePreDispatchGateRequiresReplanForHardMissingCapability(t *testing.T) {
	now := time.Date(2026, 6, 21, 9, 34, 0, 0, time.UTC)
	taskID := uuid.New()
	employeeID := uuid.New()

	result := EvaluatePreDispatchGate(PreDispatchGateInput{
		ProjectID:          uuid.New(),
		ProjectTaskID:      taskID,
		SelectedEmployeeID: employeeID,
		AttemptNo:          1,
		DispatchReason:     DispatchReasonRootReady,
	}, PreDispatchGateSnapshot{
		Task:     ProjectTask{ID: taskID, Status: ProjectTaskStatusPlanned, AssignedDigitalEmployeeID: &employeeID, MaxAttempts: int32Ptr(3)},
		Employee: PreDispatchEmployeeSnapshot{ID: employeeID, IsProjectExecutor: true, Status: "active", PolicyAllowed: true, AvailableLoadSlots: 1, RequiredLoadSlots: 1},
		Capabilities: PreDispatchCapabilitySnapshot{
			Required:    []string{"database.write"},
			HardMissing: []string{"database.write"},
		},
		Runtime: PreDispatchRuntimeSnapshot{NodeOnline: true, ProviderAvailable: true, WorkspaceReady: true, SlotAvailable: true, ContractVersionAccepted: true},
		Budget:  PreDispatchBudgetSnapshot{ProjectBudgetAllowed: true, TaskBudgetPresent: true},
		Context: PreDispatchContextSnapshot{RequiredRefsResolved: true, InjectionAllowed: true},
	}, now)

	require.Equal(t, PreDispatchGateStatusReplanRequired, result.Status)
	require.Equal(t, "capability.hard_missing", result.Blockers[0].Key)
}

func int32Ptr(v int32) *int32 {
	return &v
}
```

- [ ] **Step 2: Run tests and verify they fail**

Run:

```bash
go test ./apps/control-plane/internal/project -run 'TestEvaluatePreDispatchGate' -count=1
```

Expected: FAIL with undefined symbols `EvaluatePreDispatchGate`, `PreDispatchGateInput`, `PreDispatchGateStatusPassed`, and related gate types.

- [ ] **Step 3: Add event and wait-reason constants**

Modify `apps/control-plane/internal/project/types.go` near the existing `ProjectEventType` constants:

```go
	ProjectEventTaskDispatchGateChecked       ProjectEventType = "project_task.dispatch_gate.checked"
	ProjectEventTaskDispatchGateBlocked       ProjectEventType = "project_task.dispatch_gate.blocked"
	ProjectEventTaskDispatchGateWaitingHuman  ProjectEventType = "project_task.dispatch_gate.waiting_human"
	ProjectEventTaskDispatchGateRetryLater    ProjectEventType = "project_task.dispatch_gate.retry_later"
	ProjectEventTaskDispatchGateReplanRequired ProjectEventType = "project_task.dispatch_gate.replan_required"
```

Modify the human wait reason constants:

```go
	HumanWaitReasonRuntimeRecovery = "runtime_recovery"
	HumanWaitReasonBudgetApproval  = "budget_approval"
```

Modify `validHumanWaitReason` in `apps/control-plane/internal/project/service.go`:

```go
	case HumanWaitReasonRuntimeRecovery,
		HumanWaitReasonBudgetApproval:
		return true
```

Modify `projectTaskHumanWaitDecisionType`:

```go
	case HumanWaitReasonRuntimeRecovery:
		return "project_task_runtime_recovery"
	case HumanWaitReasonBudgetApproval:
		return "project_task_budget_approval"
```

- [ ] **Step 4: Add the pure evaluator**

Create `apps/control-plane/internal/project/predispatch_gate.go`:

```go
package project

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	PreDispatchGateStatusPassed         = "passed"
	PreDispatchGateStatusWaitingHuman   = "waiting_human"
	PreDispatchGateStatusBlocked        = "blocked"
	PreDispatchGateStatusRetryLater     = "retry_later"
	PreDispatchGateStatusReplanRequired = "replan_required"
)

const (
	DispatchReasonRootReady          = "root_ready"
	DispatchReasonDependencyUnlocked = "dependency_unlocked"
	DispatchReasonHumanResolved      = "human_resolved"
	DispatchReasonRetry              = "retry"
	DispatchReasonManual             = "manual"
)

const (
	PreDispatchHumanActionPermissionApproval = "permission_approval"
	PreDispatchHumanActionRiskApproval       = "risk_approval"
	PreDispatchHumanActionMissingContext     = "missing_context"
	PreDispatchHumanActionToolAuthorization  = "tool_authorization"
	PreDispatchHumanActionRuntimeRecovery    = "runtime_recovery"
	PreDispatchHumanActionBudgetApproval     = "budget_approval"
	PreDispatchHumanActionReplanDecision     = "replan_decision"
)

type PreDispatchGateInput struct {
	ProjectID              uuid.UUID
	ProjectTaskID          uuid.UUID
	AcceptedPlanRevisionID *uuid.UUID
	PlannedTaskKey         *string
	SelectedEmployeeID     uuid.UUID
	AttemptNo              int32
	DispatchReason         string
}

type PreDispatchGateSnapshot struct {
	Task         ProjectTask
	ActiveAttempt *PreDispatchAttemptSnapshot
	Dependencies []PreDispatchDependencySnapshot
	Employee     PreDispatchEmployeeSnapshot
	Capabilities PreDispatchCapabilitySnapshot
	Tools        PreDispatchToolSnapshot
	Runtime      PreDispatchRuntimeSnapshot
	Budget       PreDispatchBudgetSnapshot
	Risk         PreDispatchRiskSnapshot
	Context      PreDispatchContextSnapshot
}

type PreDispatchAttemptSnapshot struct {
	ID     uuid.UUID
	Status string
}

type PreDispatchDependencySnapshot struct {
	TaskID             uuid.UUID
	Status             string
	AcceptanceSatisfied bool
	ResultVersion      string
}

type PreDispatchEmployeeSnapshot struct {
	ID                  uuid.UUID
	IsProjectExecutor   bool
	Status              string
	PolicyAllowed       bool
	RequiredLoadSlots   int32
	AvailableLoadSlots  int32
	ProfileSnapshotHash string
}

type PreDispatchCapabilitySnapshot struct {
	Required    []string
	Matched     []string
	HardMissing []string
	Unknown     []string
}

type PreDispatchToolSnapshot struct {
	MissingBindings       []string
	ExpiredAuthorizations []string
	RetryableUnavailable []string
}

type PreDispatchRuntimeSnapshot struct {
	NodeOnline              bool
	ProviderAvailable       bool
	WorkspaceReady          bool
	SlotAvailable           bool
	ContractVersionAccepted bool
	RetryAfter              time.Time
}

type PreDispatchBudgetSnapshot struct {
	ProjectBudgetAllowed bool
	TaskBudgetPresent    bool
	NeedsApproval        bool
	ApprovalGranted      bool
}

type PreDispatchRiskSnapshot struct {
	HumanApprovalRequired bool
	HumanApprovalGranted  bool
	Reason                string
}

type PreDispatchContextSnapshot struct {
	RequiredRefsResolved bool
	InjectionAllowed     bool
	MissingRefs          []string
}

type PreDispatchGateEvaluation struct {
	Status             string
	CheckedAt          time.Time
	Checks             []PreDispatchGateCheck
	Blockers           []PreDispatchGateBlocker
	HumanActionRequest *PreDispatchHumanActionRequest
	RetryAfter         *time.Time
	IdempotencyKey     string
	DispatchToken      string
	CreateRun          bool
}

type PreDispatchGateCheck struct {
	Key     string
	Status  string
	Details map[string]any
}

type PreDispatchGateBlocker struct {
	Key       string
	Severity  string
	Retryable bool
	Details   map[string]any
}

type PreDispatchHumanActionRequest struct {
	Type          string
	WaitingReason string
	DecisionType  string
	Title         string
	Summary       string
	RiskLevel     string
	Options       []any
	Context       map[string]any
}

func EvaluatePreDispatchGate(input PreDispatchGateInput, snapshot PreDispatchGateSnapshot, now time.Time) PreDispatchGateEvaluation {
	input.DispatchReason = normalizeDispatchReason(input.DispatchReason)
	result := PreDispatchGateEvaluation{
		Status:         PreDispatchGateStatusPassed,
		CheckedAt:      now,
		IdempotencyKey: PreDispatchGateIdempotencyKey(input),
		DispatchToken:  PreDispatchGateDispatchToken(input),
		CreateRun:      true,
	}

	addCheck := func(key, status string, details map[string]any) {
		result.Checks = append(result.Checks, PreDispatchGateCheck{Key: key, Status: status, Details: sanitizeGateDetails(details)})
	}
	addBlocker := func(key, severity string, retryable bool, details map[string]any) {
		result.Blockers = append(result.Blockers, PreDispatchGateBlocker{Key: key, Severity: severity, Retryable: retryable, Details: sanitizeGateDetails(details)})
	}

	task := snapshot.Task
	if task.ID != input.ProjectTaskID || task.ProjectID != input.ProjectID {
		addCheck("task.identity", "failed", map[string]any{"reason": "task_project_mismatch"})
		addBlocker("task.identity_mismatch", "hard", false, nil)
		result.Status = PreDispatchGateStatusBlocked
	} else {
		addCheck("task.identity", "passed", nil)
	}

	if task.Status != ProjectTaskStatusPlanned && task.Status != ProjectTaskStatusWaitingHuman {
		addCheck("task.dispatchable", "failed", map[string]any{"status": task.Status})
		addBlocker("task.status_not_dispatchable", "hard", false, map[string]any{"status": task.Status})
		result.Status = PreDispatchGateStatusBlocked
	} else {
		addCheck("task.dispatchable", "passed", map[string]any{"status": task.Status})
	}

	if snapshot.ActiveAttempt != nil && activeAttemptStatus(snapshot.ActiveAttempt.Status) {
		addCheck("task.active_attempt", "failed", map[string]any{"attempt_id": snapshot.ActiveAttempt.ID.String(), "status": snapshot.ActiveAttempt.Status})
		addBlocker("task.active_attempt_exists", "hard", false, map[string]any{"attempt_id": snapshot.ActiveAttempt.ID.String()})
		result.Status = PreDispatchGateStatusBlocked
	} else {
		addCheck("task.active_attempt", "passed", nil)
	}

	if task.MaxAttempts != nil && input.AttemptNo > *task.MaxAttempts {
		addCheck("task.retry_policy", "failed", map[string]any{"attempt_no": input.AttemptNo, "max_attempts": *task.MaxAttempts})
		addBlocker("task.retry_exhausted", "hard", false, nil)
		result.Status = PreDispatchGateStatusBlocked
	} else {
		addCheck("task.retry_policy", "passed", map[string]any{"attempt_no": input.AttemptNo})
	}

	dependencyFailed := false
	for _, dep := range snapshot.Dependencies {
		if dep.Status != ProjectTaskStatusCompleted || !dep.AcceptanceSatisfied {
			dependencyFailed = true
			addBlocker("dependency.not_ready", "hard", false, map[string]any{"project_task_id": dep.TaskID.String(), "status": dep.Status, "acceptance_satisfied": dep.AcceptanceSatisfied})
		}
	}
	if dependencyFailed {
		addCheck("dependency.ready", "failed", nil)
		result.Status = PreDispatchGateStatusBlocked
	} else {
		addCheck("dependency.ready", "passed", map[string]any{"dependency_count": len(snapshot.Dependencies)})
	}

	if task.AssignedDigitalEmployeeID == nil || *task.AssignedDigitalEmployeeID != input.SelectedEmployeeID {
		addCheck("employee.selected", "failed", nil)
		addBlocker("employee.selection_changed", "hard", false, nil)
		result.Status = PreDispatchGateStatusReplanRequired
	} else if !snapshot.Employee.IsProjectExecutor || snapshot.Employee.Status != "active" || !snapshot.Employee.PolicyAllowed {
		addCheck("employee.dispatchable", "failed", map[string]any{"status": snapshot.Employee.Status, "project_executor": snapshot.Employee.IsProjectExecutor, "policy_allowed": snapshot.Employee.PolicyAllowed})
		addBlocker("employee.not_dispatchable", "hard", false, nil)
		result.Status = PreDispatchGateStatusReplanRequired
	} else if snapshot.Employee.AvailableLoadSlots < snapshot.Employee.RequiredLoadSlots {
		addCheck("employee.load", "failed", map[string]any{"available": snapshot.Employee.AvailableLoadSlots, "required": snapshot.Employee.RequiredLoadSlots})
		addBlocker("employee.slot_unavailable", "transient", true, nil)
		result.Status = PreDispatchGateStatusRetryLater
	} else {
		addCheck("employee.dispatchable", "passed", map[string]any{"profile_snapshot_hash": snapshot.Employee.ProfileSnapshotHash})
	}

	if len(snapshot.Capabilities.HardMissing) > 0 {
		addCheck("capability.match", "failed", map[string]any{"hard_missing": append([]string(nil), snapshot.Capabilities.HardMissing...)})
		addBlocker("capability.hard_missing", "hard", false, map[string]any{"hard_missing": append([]string(nil), snapshot.Capabilities.HardMissing...)})
		result.Status = PreDispatchGateStatusReplanRequired
	} else {
		addCheck("capability.match", "passed", map[string]any{"required": append([]string(nil), snapshot.Capabilities.Required...), "matched": append([]string(nil), snapshot.Capabilities.Matched...)})
	}

	if len(snapshot.Tools.ExpiredAuthorizations) > 0 {
		result.HumanActionRequest = humanGateRequest(PreDispatchHumanActionToolAuthorization, HumanWaitReasonPermissionRequired, "project_task_permission", "工具授权已失效", "需要人类重新授权后才能分派任务", "medium")
		addCheck("tool.authorization", "failed", map[string]any{"expired": append([]string(nil), snapshot.Tools.ExpiredAuthorizations...)})
		addBlocker("tool.authorization_expired", "human", false, nil)
		result.Status = PreDispatchGateStatusWaitingHuman
	} else if len(snapshot.Tools.MissingBindings) > 0 {
		result.HumanActionRequest = humanGateRequest(PreDispatchHumanActionToolAuthorization, HumanWaitReasonPermissionRequired, "project_task_permission", "任务缺少工具绑定", "需要补齐 MCP 或外部能力绑定后才能分派任务", "medium")
		addCheck("tool.binding", "failed", map[string]any{"missing": append([]string(nil), snapshot.Tools.MissingBindings...)})
		addBlocker("tool.binding_missing", "human", false, nil)
		result.Status = PreDispatchGateStatusWaitingHuman
	} else if len(snapshot.Tools.RetryableUnavailable) > 0 {
		addCheck("tool.available", "failed", map[string]any{"retryable_unavailable": append([]string(nil), snapshot.Tools.RetryableUnavailable...)})
		addBlocker("tool.retryable_unavailable", "transient", true, nil)
		result.Status = PreDispatchGateStatusRetryLater
	} else {
		addCheck("tool.available", "passed", nil)
	}

	if !snapshot.Runtime.NodeOnline {
		addCheck("runtime.ready", "failed", map[string]any{"node_online": false})
		addBlocker("runtime.node_offline", "transient", true, nil)
		result.Status = PreDispatchGateStatusRetryLater
	} else if !snapshot.Runtime.ProviderAvailable {
		addCheck("runtime.ready", "failed", map[string]any{"provider_available": false})
		addBlocker("runtime.provider_unavailable", "transient", true, nil)
		result.Status = PreDispatchGateStatusRetryLater
	} else if !snapshot.Runtime.WorkspaceReady {
		result.HumanActionRequest = humanGateRequest(PreDispatchHumanActionRuntimeRecovery, HumanWaitReasonRuntimeRecovery, "project_task_runtime_recovery", "执行工作区未就绪", "需要恢复 Runtime 工作区后才能分派任务", "medium")
		addCheck("runtime.workspace", "failed", nil)
		addBlocker("runtime.workspace_not_ready", "human", false, nil)
		result.Status = PreDispatchGateStatusWaitingHuman
	} else if !snapshot.Runtime.SlotAvailable {
		addCheck("runtime.ready", "failed", map[string]any{"slot_available": false})
		addBlocker("runtime.slot_unavailable", "transient", true, nil)
		result.Status = PreDispatchGateStatusRetryLater
	} else if !snapshot.Runtime.ContractVersionAccepted {
		addCheck("runtime.contract", "failed", nil)
		addBlocker("runtime.contract_version_unsupported", "hard", false, nil)
		result.Status = PreDispatchGateStatusReplanRequired
	} else {
		addCheck("runtime.ready", "passed", nil)
	}

	if !snapshot.Budget.TaskBudgetPresent {
		result.HumanActionRequest = humanGateRequest(PreDispatchHumanActionBudgetApproval, HumanWaitReasonBudgetApproval, "project_task_budget_approval", "任务预算缺失", "需要确认任务预算和超时策略后才能分派任务", "medium")
		addCheck("budget.ready", "failed", map[string]any{"task_budget_present": false})
		addBlocker("budget.task_budget_missing", "human", false, nil)
		result.Status = PreDispatchGateStatusWaitingHuman
	} else if !snapshot.Budget.ProjectBudgetAllowed || (snapshot.Budget.NeedsApproval && !snapshot.Budget.ApprovalGranted) {
		result.HumanActionRequest = humanGateRequest(PreDispatchHumanActionBudgetApproval, HumanWaitReasonBudgetApproval, "project_task_budget_approval", "项目预算需要确认", "需要人类确认预算后才能分派任务", "medium")
		addCheck("budget.ready", "failed", map[string]any{"project_budget_allowed": snapshot.Budget.ProjectBudgetAllowed})
		addBlocker("budget.approval_required", "human", false, nil)
		result.Status = PreDispatchGateStatusWaitingHuman
	} else {
		addCheck("budget.ready", "passed", nil)
	}

	if snapshot.Risk.HumanApprovalRequired && !snapshot.Risk.HumanApprovalGranted {
		result.HumanActionRequest = humanGateRequest(PreDispatchHumanActionRiskApproval, HumanWaitReasonApprovalRequired, "project_task_approval", "高风险动作需要确认", "需要人类确认风险后才能分派任务", "high")
		if snapshot.Risk.Reason != "" {
			result.HumanActionRequest.Context["risk_reason"] = snapshot.Risk.Reason
		}
		addCheck("risk.approval", "failed", map[string]any{"reason": snapshot.Risk.Reason})
		addBlocker("risk.approval_required", "human", false, nil)
		result.Status = PreDispatchGateStatusWaitingHuman
	} else {
		addCheck("risk.approval", "passed", nil)
	}

	if !snapshot.Context.RequiredRefsResolved {
		result.HumanActionRequest = humanGateRequest(PreDispatchHumanActionMissingContext, HumanWaitReasonMissingContext, "project_task_missing_context", "任务缺少必要上下文", "需要补充上下文后才能分派任务", "medium")
		result.HumanActionRequest.Context["missing_refs"] = append([]string(nil), snapshot.Context.MissingRefs...)
		addCheck("context.ready", "failed", map[string]any{"missing_refs": append([]string(nil), snapshot.Context.MissingRefs...)})
		addBlocker("context.missing_required_refs", "human", false, nil)
		result.Status = PreDispatchGateStatusWaitingHuman
	} else if !snapshot.Context.InjectionAllowed {
		addCheck("context.policy", "failed", nil)
		addBlocker("context.injection_denied", "hard", false, nil)
		result.Status = PreDispatchGateStatusBlocked
	} else {
		addCheck("context.ready", "passed", nil)
	}

	if result.Status != PreDispatchGateStatusPassed {
		result.CreateRun = false
	}
	if result.Status == PreDispatchGateStatusRetryLater && !snapshot.Runtime.RetryAfter.IsZero() {
		retryAfter := snapshot.Runtime.RetryAfter
		result.RetryAfter = &retryAfter
	}
	sort.SliceStable(result.Checks, func(i, j int) bool { return result.Checks[i].Key < result.Checks[j].Key })
	return result
}

func (r PreDispatchGateEvaluation) CheckKeys() []string {
	keys := make([]string, 0, len(r.Checks))
	for _, check := range r.Checks {
		keys = append(keys, check.Key)
	}
	return keys
}

func PreDispatchGateIdempotencyKey(input PreDispatchGateInput) string {
	reason := normalizeDispatchReason(input.DispatchReason)
	return fmt.Sprintf("project-task:%s:reason:%s:attempt:%d:employee:%s", input.ProjectTaskID, reason, input.AttemptNo, input.SelectedEmployeeID)
}

func PreDispatchGateDispatchToken(input PreDispatchGateInput) string {
	sum := sha256.Sum256([]byte(PreDispatchGateIdempotencyKey(input)))
	return hex.EncodeToString(sum[:])
}

func normalizeDispatchReason(reason string) string {
	reason = strings.TrimSpace(reason)
	switch reason {
	case DispatchReasonRootReady, DispatchReasonDependencyUnlocked, DispatchReasonHumanResolved, DispatchReasonRetry, DispatchReasonManual:
		return reason
	default:
		return DispatchReasonRootReady
	}
}

func activeAttemptStatus(status string) bool {
	return status == ProjectTaskAttemptStatusQueued || status == ProjectTaskAttemptStatusRunning || status == ProjectTaskAttemptStatusWaitingHuman
}

func sanitizeGateDetails(details map[string]any) map[string]any {
	if len(details) == 0 {
		return map[string]any{}
	}
	clean := make(map[string]any, len(details))
	for key, value := range details {
		lower := strings.ToLower(key)
		if strings.Contains(lower, "secret") || strings.Contains(lower, "token") || strings.Contains(lower, "password") || strings.Contains(lower, "connection_string") {
			clean[key] = "[redacted]"
			continue
		}
		clean[key] = value
	}
	return clean
}

func humanGateRequest(actionType, waitingReason, decisionType, title, summary, riskLevel string) *PreDispatchHumanActionRequest {
	return &PreDispatchHumanActionRequest{
		Type:          actionType,
		WaitingReason: waitingReason,
		DecisionType:  decisionType,
		Title:         title,
		Summary:       summary,
		RiskLevel:     riskLevel,
		Options:       []any{"approved", "rejected", "needs_more_evidence", "cancelled"},
		Context:       map[string]any{"source": "predispatch_gate"},
	}
}
```

- [ ] **Step 5: Run evaluator tests and commit**

Run:

```bash
go test ./apps/control-plane/internal/project -run 'TestEvaluatePreDispatchGate' -count=1
```

Expected: PASS.

Commit:

```bash
git add apps/control-plane/internal/project/predispatch_gate.go apps/control-plane/internal/project/predispatch_gate_test.go apps/control-plane/internal/project/types.go apps/control-plane/internal/project/service.go
git commit -m "feat: add pre-dispatch gate evaluator"
```

### Task 2: Gate Persistence Migration And Queries

**Files:**
- Create: `apps/control-plane/internal/storage/migrations/032_project_task_dispatch_gates.sql`
- Modify: `apps/control-plane/internal/storage/migrations/atlas.sum`
- Modify: `apps/control-plane/internal/storage/migrations_test.go`
- Modify: `apps/control-plane/internal/storage/queries/project.sql`
- Regenerate: `apps/control-plane/internal/storage/queries/*.go`

- [ ] **Step 1: Write failing migration tests**

Append to `apps/control-plane/internal/storage/migrations_test.go`:

```go
func TestProjectTaskDispatchGateMigration(t *testing.T) {
	body, err := os.ReadFile("migrations/032_project_task_dispatch_gates.sql")
	if err != nil {
		t.Fatalf("read dispatch gate migration: %v", err)
	}
	sql := string(body)

	for _, expected := range []string{
		"CREATE TABLE project_task_dispatch_gate_results",
		"id UUID PRIMARY KEY DEFAULT gen_random_uuid()",
		"tenant_id UUID NOT NULL",
		"project_id UUID NOT NULL",
		"project_task_id UUID NOT NULL",
		"accepted_plan_revision_id UUID",
		"selected_employee_id UUID NOT NULL",
		"attempt_no INTEGER NOT NULL",
		"idempotency_key VARCHAR(255) NOT NULL",
		"dispatch_token VARCHAR(128) NOT NULL",
		"status VARCHAR(32) NOT NULL",
		"checks JSONB NOT NULL DEFAULT '[]'::jsonb",
		"blockers JSONB NOT NULL DEFAULT '[]'::jsonb",
		"human_action_request JSONB NOT NULL DEFAULT '{}'::jsonb",
		"retry_after TIMESTAMPTZ",
		"dispatch_token VARCHAR(128) NOT NULL",
		"attempt_id UUID",
		"decision_request_id UUID",
		"created_event_id UUID",
		"CREATE UNIQUE INDEX uq_project_task_dispatch_gate_results_key",
		"ON project_task_dispatch_gate_results(tenant_id, project_task_id, idempotency_key)",
		"CREATE UNIQUE INDEX uq_project_task_dispatch_gate_results_tenant_task_id",
		"ON project_task_dispatch_gate_results(tenant_id, project_task_id, id)",
		"CREATE INDEX idx_project_task_dispatch_gate_results_task_created",
		"ALTER TABLE project_task_attempts",
		"ADD COLUMN dispatch_gate_result_id UUID",
		"ALTER TABLE project_tasks",
		"ADD COLUMN latest_dispatch_gate_result_id UUID",
		"ALTER TABLE project_decision_requests",
		"ADD COLUMN dispatch_gate_result_id UUID",
		"COMMENT ON TABLE project_task_dispatch_gate_results IS",
		"COMMENT ON COLUMN project_task_dispatch_gate_results.blockers IS",
	} {
		if !strings.Contains(sql, expected) {
			t.Fatalf("expected dispatch gate migration to contain %q", expected)
		}
	}

	for _, forbidden := range []string{
		"BIGSERIAL",
		"CREATE TYPE",
		"connection_string",
		"secret_value",
		"raw_log",
		"provider_stdout",
	} {
		if strings.Contains(sql, forbidden) {
			t.Fatalf("dispatch gate migration must avoid %q", forbidden)
		}
	}
}
```

- [ ] **Step 2: Run migration test and verify it fails**

Run:

```bash
go test ./apps/control-plane/internal/storage -run TestProjectTaskDispatchGateMigration -count=1
```

Expected: FAIL because migration `032_project_task_dispatch_gates.sql` does not exist.

- [ ] **Step 3: Add migration**

Create `apps/control-plane/internal/storage/migrations/032_project_task_dispatch_gates.sql`:

```sql
CREATE TABLE project_task_dispatch_gate_results (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    project_id UUID NOT NULL,
    project_task_id UUID NOT NULL,
    accepted_plan_revision_id UUID,
    planned_task_key VARCHAR(100),
    selected_employee_id UUID NOT NULL,
    attempt_no INTEGER NOT NULL,
    dispatch_reason VARCHAR(80) NOT NULL,
    idempotency_key VARCHAR(255) NOT NULL,
    dispatch_token VARCHAR(128) NOT NULL,
    status VARCHAR(32) NOT NULL,
    checked_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    checks JSONB NOT NULL DEFAULT '[]'::jsonb,
    blockers JSONB NOT NULL DEFAULT '[]'::jsonb,
    human_action_request JSONB NOT NULL DEFAULT '{}'::jsonb,
    retry_after TIMESTAMPTZ,
    attempt_id UUID,
    decision_request_id UUID,
    created_event_id UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT fk_project_task_dispatch_gate_results_task
        FOREIGN KEY (tenant_id, project_task_id) REFERENCES project_tasks(tenant_id, id) ON DELETE CASCADE,
    CONSTRAINT chk_project_task_dispatch_gate_results_status CHECK (
        status IN ('passed', 'waiting_human', 'blocked', 'retry_later', 'replan_required')
    )
);

CREATE UNIQUE INDEX uq_project_task_dispatch_gate_results_key
    ON project_task_dispatch_gate_results(tenant_id, project_task_id, idempotency_key);

-- Required target for the composite FKs added below from project_task_attempts and
-- project_tasks. PostgreSQL requires the referenced column set to be backed by a unique
-- constraint or unique index; the primary key on (id) alone is not sufficient for a
-- three-column reference.
CREATE UNIQUE INDEX uq_project_task_dispatch_gate_results_tenant_task_id
    ON project_task_dispatch_gate_results(tenant_id, project_task_id, id);

CREATE INDEX idx_project_task_dispatch_gate_results_task_created
    ON project_task_dispatch_gate_results(tenant_id, project_id, project_task_id, created_at DESC);

CREATE INDEX idx_project_task_dispatch_gate_results_status
    ON project_task_dispatch_gate_results(tenant_id, project_id, status, created_at DESC);

CREATE INDEX idx_project_task_dispatch_gate_results_decision
    ON project_task_dispatch_gate_results(tenant_id, decision_request_id)
    WHERE decision_request_id IS NOT NULL;

ALTER TABLE project_task_attempts
    ADD COLUMN dispatch_gate_result_id UUID;

ALTER TABLE project_task_attempts
    ADD CONSTRAINT fk_project_task_attempts_dispatch_gate
    FOREIGN KEY (tenant_id, project_task_id, dispatch_gate_result_id)
    REFERENCES project_task_dispatch_gate_results(tenant_id, project_task_id, id);

CREATE INDEX idx_project_task_attempts_dispatch_gate
    ON project_task_attempts(tenant_id, dispatch_gate_result_id)
    WHERE dispatch_gate_result_id IS NOT NULL;

ALTER TABLE project_tasks
    ADD COLUMN latest_dispatch_gate_result_id UUID;

ALTER TABLE project_tasks
    ADD CONSTRAINT fk_project_tasks_latest_dispatch_gate
    FOREIGN KEY (tenant_id, id, latest_dispatch_gate_result_id)
    REFERENCES project_task_dispatch_gate_results(tenant_id, project_task_id, id);

CREATE INDEX idx_project_tasks_latest_dispatch_gate
    ON project_tasks(tenant_id, latest_dispatch_gate_result_id)
    WHERE latest_dispatch_gate_result_id IS NOT NULL;

ALTER TABLE project_decision_requests
    ADD COLUMN dispatch_gate_result_id UUID;

CREATE INDEX idx_project_decision_requests_dispatch_gate
    ON project_decision_requests(tenant_id, dispatch_gate_result_id)
    WHERE dispatch_gate_result_id IS NOT NULL;

CREATE TRIGGER update_project_task_dispatch_gate_results_updated_at
    BEFORE UPDATE ON project_task_dispatch_gate_results
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

COMMENT ON TABLE project_task_dispatch_gate_results IS 'ProjectTask 分派 Runtime 前的 Control Plane gate 结果，保存可审计的检查摘要、阻塞原因和后续 attempt 或人类请求引用。';
COMMENT ON COLUMN project_task_dispatch_gate_results.id IS 'Gate 结果ID。';
COMMENT ON COLUMN project_task_dispatch_gate_results.tenant_id IS '租户ID。';
COMMENT ON COLUMN project_task_dispatch_gate_results.project_id IS '项目ID。';
COMMENT ON COLUMN project_task_dispatch_gate_results.project_task_id IS '被检查的 ProjectTask ID。';
COMMENT ON COLUMN project_task_dispatch_gate_results.accepted_plan_revision_id IS '生成任务的 accepted PlanRevision ID。';
COMMENT ON COLUMN project_task_dispatch_gate_results.planned_task_key IS 'PlanRevision payload 内稳定任务键。';
COMMENT ON COLUMN project_task_dispatch_gate_results.selected_employee_id IS 'Gate 检查时的被选数字员工ID。';
COMMENT ON COLUMN project_task_dispatch_gate_results.attempt_no IS '本次 gate 预期创建的执行尝试序号。';
COMMENT ON COLUMN project_task_dispatch_gate_results.dispatch_reason IS '触发 gate 的原因，例如 root_ready、dependency_unlocked、human_resolved、retry。';
COMMENT ON COLUMN project_task_dispatch_gate_results.idempotency_key IS 'Gate 幂等键，同一任务同一分派原因和尝试序号只保留一个结果。';
COMMENT ON COLUMN project_task_dispatch_gate_results.dispatch_token IS '通过 gate 后交给 dispatch 的稳定 token，不包含密钥。';
COMMENT ON COLUMN project_task_dispatch_gate_results.status IS 'Gate 状态：passed, waiting_human, blocked, retry_later, replan_required。';
COMMENT ON COLUMN project_task_dispatch_gate_results.checked_at IS 'Gate 检查时间。';
COMMENT ON COLUMN project_task_dispatch_gate_results.checks IS '检查项摘要 JSON 数组，禁止保存密钥、完整连接串、敏感 SQL 或完整日志。';
COMMENT ON COLUMN project_task_dispatch_gate_results.blockers IS '阻塞原因 JSON 数组，禁止保存密钥、完整连接串、敏感 SQL 或完整日志。';
COMMENT ON COLUMN project_task_dispatch_gate_results.human_action_request IS '需要人类动作时的请求摘要，不作为审批事实源。';
COMMENT ON COLUMN project_task_dispatch_gate_results.retry_after IS '暂态不满足时建议下次 gate 时间。';
COMMENT ON COLUMN project_task_dispatch_gate_results.attempt_id IS 'Gate 通过后创建的 ProjectTaskAttempt ID。';
COMMENT ON COLUMN project_task_dispatch_gate_results.decision_request_id IS 'Gate 创建的人类决策请求投影ID。';
COMMENT ON COLUMN project_task_dispatch_gate_results.created_event_id IS '记录该 gate 结果时产生的项目事件ID。';
COMMENT ON COLUMN project_task_dispatch_gate_results.created_at IS 'Gate 结果创建时间。';
COMMENT ON COLUMN project_task_dispatch_gate_results.updated_at IS 'Gate 结果最近更新时间。';
COMMENT ON COLUMN project_task_attempts.dispatch_gate_result_id IS '创建该尝试前通过的 gate 结果ID。';
COMMENT ON COLUMN project_tasks.latest_dispatch_gate_result_id IS '该任务最近一次 gate 结果ID。';
COMMENT ON COLUMN project_decision_requests.dispatch_gate_result_id IS '该人类决策由哪个 dispatch gate 结果创建。';
```

- [ ] **Step 4: Add sqlc queries**

Append to `apps/control-plane/internal/storage/queries/project.sql`:

```sql
-- name: CreateProjectTaskDispatchGateResult :one
INSERT INTO project_task_dispatch_gate_results (
    id,
    tenant_id,
    project_id,
    project_task_id,
    accepted_plan_revision_id,
    planned_task_key,
    selected_employee_id,
    attempt_no,
    dispatch_reason,
    idempotency_key,
    dispatch_token,
    status,
    checked_at,
    checks,
    blockers,
    human_action_request,
    retry_after,
    created_event_id
) VALUES (
    sqlc.arg('id')::uuid,
    sqlc.arg('tenant_id')::uuid,
    sqlc.arg('project_id')::uuid,
    sqlc.arg('project_task_id')::uuid,
    sqlc.narg('accepted_plan_revision_id')::uuid,
    sqlc.narg('planned_task_key')::varchar,
    sqlc.arg('selected_employee_id')::uuid,
    sqlc.arg('attempt_no')::integer,
    sqlc.arg('dispatch_reason')::varchar,
    sqlc.arg('idempotency_key')::varchar,
    sqlc.arg('dispatch_token')::varchar,
    sqlc.arg('status')::varchar,
    sqlc.arg('checked_at')::timestamptz,
    sqlc.arg('checks')::jsonb,
    sqlc.arg('blockers')::jsonb,
    sqlc.arg('human_action_request')::jsonb,
    sqlc.narg('retry_after')::timestamptz,
    sqlc.narg('created_event_id')::uuid
)
ON CONFLICT (tenant_id, project_task_id, idempotency_key)
DO UPDATE SET
    status = EXCLUDED.status,
    checked_at = EXCLUDED.checked_at,
    checks = EXCLUDED.checks,
    blockers = EXCLUDED.blockers,
    human_action_request = EXCLUDED.human_action_request,
    retry_after = EXCLUDED.retry_after,
    created_event_id = COALESCE(project_task_dispatch_gate_results.created_event_id, EXCLUDED.created_event_id),
    updated_at = NOW()
RETURNING *;

-- name: GetProjectTaskDispatchGateResult :one
SELECT * FROM project_task_dispatch_gate_results
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_id = sqlc.arg('project_id')::uuid
  AND id = sqlc.arg('id')::uuid;

-- name: GetProjectTaskDispatchGateResultByKey :one
SELECT * FROM project_task_dispatch_gate_results
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_task_id = sqlc.arg('project_task_id')::uuid
  AND idempotency_key = sqlc.arg('idempotency_key')::varchar;

-- name: ListProjectTaskDispatchGateResults :many
SELECT * FROM project_task_dispatch_gate_results
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_id = sqlc.arg('project_id')::uuid
  AND project_task_id = sqlc.arg('project_task_id')::uuid
ORDER BY created_at DESC
LIMIT sqlc.arg('limit')::integer
OFFSET sqlc.arg('offset')::integer;

-- name: LinkProjectTaskDispatchGateAttempt :one
UPDATE project_task_dispatch_gate_results
SET attempt_id = sqlc.arg('attempt_id')::uuid,
    updated_at = NOW()
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_id = sqlc.arg('project_id')::uuid
  AND project_task_id = sqlc.arg('project_task_id')::uuid
  AND id = sqlc.arg('id')::uuid
RETURNING *;

-- name: LinkProjectTaskDispatchGateDecisionRequest :one
UPDATE project_task_dispatch_gate_results
SET decision_request_id = sqlc.arg('decision_request_id')::uuid,
    updated_at = NOW()
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_id = sqlc.arg('project_id')::uuid
  AND project_task_id = sqlc.arg('project_task_id')::uuid
  AND id = sqlc.arg('id')::uuid
RETURNING *;

-- name: MarkProjectTaskLatestDispatchGate :one
UPDATE project_tasks
SET latest_dispatch_gate_result_id = sqlc.arg('latest_dispatch_gate_result_id')::uuid,
    updated_at = NOW()
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_id = sqlc.arg('project_id')::uuid
  AND id = sqlc.arg('id')::uuid
RETURNING *;

-- name: MovePlannedProjectTaskToWaitingHumanForGate :one
UPDATE project_tasks
SET status = 'waiting_human',
    waiting_reason = sqlc.arg('waiting_reason')::varchar,
    waiting_request_id = sqlc.narg('waiting_request_id')::uuid,
    latest_dispatch_gate_result_id = sqlc.arg('latest_dispatch_gate_result_id')::uuid,
    latest_event_id = COALESCE(sqlc.narg('latest_event_id')::uuid, latest_event_id),
    status_changed_at = NOW(),
    updated_at = NOW()
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_id = sqlc.arg('project_id')::uuid
  AND id = sqlc.arg('id')::uuid
  AND status IN ('planned', 'waiting_human')
RETURNING *;

-- name: SetProjectDecisionRequestDispatchGate :one
UPDATE project_decision_requests
SET dispatch_gate_result_id = sqlc.arg('dispatch_gate_result_id')::uuid,
    updated_at = NOW()
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_id = sqlc.arg('project_id')::uuid
  AND id = sqlc.arg('id')::uuid
RETURNING *;
```

- [ ] **Step 5: Regenerate sqlc and atlas checksum**

Run:

```bash
corepack pnpm --filter ./apps/control-plane run generate:sqlc
corepack pnpm --filter ./apps/control-plane run migrate:hash
```

Expected:

- generated query files update
- `apps/control-plane/internal/storage/migrations/atlas.sum` includes migration `032_project_task_dispatch_gates.sql`

- [ ] **Step 6: Run migration tests and commit**

Run:

```bash
go test ./apps/control-plane/internal/storage -run 'TestProjectTaskDispatchGateMigration|TestMigrationsApply' -count=1
```

Expected: PASS.

Commit:

```bash
git add apps/control-plane/internal/storage/migrations/032_project_task_dispatch_gates.sql apps/control-plane/internal/storage/migrations/atlas.sum apps/control-plane/internal/storage/migrations_test.go apps/control-plane/internal/storage/queries/project.sql apps/control-plane/internal/storage/queries
git commit -m "feat: persist project task dispatch gates"
```

### Task 3: Repository Persistence And Task Transition

**Files:**
- Modify: `apps/control-plane/internal/project/repository.go`
- Modify: `apps/control-plane/internal/project/pg_repository.go`
- Modify: `apps/control-plane/internal/project/pg_repository_test.go`
- Modify: `apps/control-plane/internal/project/service_test.go`

- [ ] **Step 1: Write failing repository tests**

Append to `apps/control-plane/internal/project/pg_repository_test.go`:

```go
func TestRecordPreDispatchGateResultIsIdempotent(t *testing.T) {
	repo := newTestProjectRepository(t)
	tenantID := uuid.New()
	projectID := uuid.New()
	taskID := uuid.New()
	employeeID := uuid.New()
	seedProjectTaskForAttempt(t, repo, tenantID, projectID, taskID, employeeID, ProjectTaskStatusPlanned)
	now := time.Date(2026, 6, 21, 10, 0, 0, 0, time.UTC)

	req := RecordPreDispatchGateResultRequest{
		TenantID:           tenantID,
		ProjectID:          projectID,
		ProjectTaskID:      taskID,
		SelectedEmployeeID: employeeID,
		AttemptNo:          1,
		DispatchReason:     DispatchReasonRootReady,
		IdempotencyKey:     "project-task:" + taskID.String() + ":reason:root_ready:attempt:1:employee:" + employeeID.String(),
		DispatchToken:      "dispatch-token",
		Status:             PreDispatchGateStatusRetryLater,
		CheckedAt:          now,
		Checks:             []PreDispatchGateCheck{{Key: "runtime.ready", Status: "failed", Details: map[string]any{"node_online": false}}},
		Blockers:           []PreDispatchGateBlocker{{Key: "runtime.node_offline", Severity: "transient", Retryable: true}},
		RetryAfter:         timePtr(now.Add(time.Minute)),
	}

	first, err := repo.RecordPreDispatchGateResult(context.Background(), req)
	require.NoError(t, err)
	require.Equal(t, PreDispatchGateStatusRetryLater, first.Status)
	require.NotEqual(t, uuid.Nil, first.ID)

	req.Status = PreDispatchGateStatusPassed
	req.RetryAfter = nil
	req.Checks = []PreDispatchGateCheck{{Key: "runtime.ready", Status: "passed"}}
	req.Blockers = nil
	second, err := repo.RecordPreDispatchGateResult(context.Background(), req)
	require.NoError(t, err)
	require.Equal(t, first.ID, second.ID)
	require.Equal(t, PreDispatchGateStatusPassed, second.Status)
	require.Nil(t, second.RetryAfter)

	results, err := repo.ListPreDispatchGateResults(context.Background(), ListPreDispatchGateResultsRequest{
		TenantID:      tenantID,
		ProjectID:     projectID,
		ProjectTaskID: taskID,
		Limit:         10,
	})
	require.NoError(t, err)
	require.Len(t, results, 1)
}

func TestMoveProjectTaskToWaitingHumanForPreDispatchGate(t *testing.T) {
	repo := newTestProjectRepository(t)
	tenantID := uuid.New()
	projectID := uuid.New()
	taskID := uuid.New()
	employeeID := uuid.New()
	ownerID := uuid.New()
	seedProjectWithOwner(t, repo, tenantID, projectID, ownerID)
	seedProjectTaskForAttempt(t, repo, tenantID, projectID, taskID, employeeID, ProjectTaskStatusPlanned)
	gate := seedPreDispatchGateResult(t, repo, tenantID, projectID, taskID, employeeID, PreDispatchGateStatusWaitingHuman)
	decisionID := uuid.New()
	eventID := uuid.New()

	task, err := repo.MoveProjectTaskToWaitingHumanForPreDispatchGate(context.Background(), MoveProjectTaskToWaitingHumanForPreDispatchGateRequest{
		TenantID:       tenantID,
		ProjectID:      projectID,
		ProjectTaskID:  taskID,
		GateResultID:   gate.ID,
		WaitingReason:  HumanWaitReasonPermissionRequired,
		DecisionRequestID: &decisionID,
		EventID:        &eventID,
	})
	require.NoError(t, err)
	require.Equal(t, ProjectTaskStatusWaitingHuman, task.Status)
	require.NotNil(t, task.WaitingRequestID)
	require.Equal(t, decisionID, *task.WaitingRequestID)
	require.NotNil(t, task.LatestDispatchGateResultID)
	require.Equal(t, gate.ID, *task.LatestDispatchGateResultID)
}

func TestLinkPreDispatchGateResultToAttempt(t *testing.T) {
	repo := newTestProjectRepository(t)
	tenantID := uuid.New()
	projectID := uuid.New()
	taskID := uuid.New()
	employeeID := uuid.New()
	seedProjectTaskForAttempt(t, repo, tenantID, projectID, taskID, employeeID, ProjectTaskStatusPlanned)
	gate := seedPreDispatchGateResult(t, repo, tenantID, projectID, taskID, employeeID, PreDispatchGateStatusPassed)

	queue, err := repo.QueueProjectTaskWithAttempt(context.Background(), QueueProjectTaskRequest{
		TenantID:             tenantID,
		ProjectID:            projectID,
		ProjectTaskID:        taskID,
		DigitalEmployeeID:    employeeID,
		IdempotencyKey:       "project-task:" + taskID.String(),
		LeaseToken:           "lease-token",
		DispatchGateResultID: &gate.ID,
		ExecutionContextPacket: map[string]any{
			"dispatch_gate_result_id": gate.ID.String(),
		},
		ExecutionContextPacketVersion: "v1",
	})
	require.NoError(t, err)
	require.NotNil(t, queue.Attempt.DispatchGateResultID)
	require.Equal(t, gate.ID, *queue.Attempt.DispatchGateResultID)

	linked, err := repo.GetPreDispatchGateResult(context.Background(), tenantID, projectID, gate.ID)
	require.NoError(t, err)
	require.NotNil(t, linked.AttemptID)
	require.Equal(t, queue.Attempt.ID, *linked.AttemptID)
}
```

- [ ] **Step 2: Run tests and verify they fail**

Run:

```bash
go test ./apps/control-plane/internal/project -run 'TestRecordPreDispatchGateResult|TestMoveProjectTaskToWaitingHumanForPreDispatchGate|TestLinkPreDispatchGateResultToAttempt' -count=1
```

Expected: FAIL with undefined repository request/result types and missing `DispatchGateResultID` on `QueueProjectTaskRequest` / `ProjectTaskAttempt`.

- [ ] **Step 3: Add repository types**

Modify `apps/control-plane/internal/project/types.go`:

```go
	LatestDispatchGateResultID *uuid.UUID
```

Add to `ProjectTaskAttempt`:

```go
	DispatchGateResultID *uuid.UUID
```

Add to `QueueProjectTaskRequest`:

```go
	DispatchGateResultID *uuid.UUID
```

Modify `apps/control-plane/internal/project/repository.go`:

```go
	RecordPreDispatchGateResult(ctx context.Context, req RecordPreDispatchGateResultRequest) (PreDispatchGateResult, error)
	GetPreDispatchGateResult(ctx context.Context, tenantID, projectID, gateResultID uuid.UUID) (PreDispatchGateResult, error)
	GetPreDispatchGateResultByKey(ctx context.Context, tenantID, projectTaskID uuid.UUID, idempotencyKey string) (PreDispatchGateResult, error)
	ListPreDispatchGateResults(ctx context.Context, req ListPreDispatchGateResultsRequest) ([]PreDispatchGateResult, error)
	LinkPreDispatchGateAttempt(ctx context.Context, req LinkPreDispatchGateAttemptRequest) (PreDispatchGateResult, error)
	LinkPreDispatchGateDecisionRequest(ctx context.Context, req LinkPreDispatchGateDecisionRequest) (PreDispatchGateResult, error)
	MoveProjectTaskToWaitingHumanForPreDispatchGate(ctx context.Context, req MoveProjectTaskToWaitingHumanForPreDispatchGateRequest) (ProjectTask, error)
```

Add request/result structs in the same file:

```go
type PreDispatchGateResult struct {
	ID                     uuid.UUID
	TenantID               uuid.UUID
	ProjectID              uuid.UUID
	ProjectTaskID          uuid.UUID
	AcceptedPlanRevisionID *uuid.UUID
	PlannedTaskKey         *string
	SelectedEmployeeID     uuid.UUID
	AttemptNo              int32
	DispatchReason         string
	IdempotencyKey         string
	DispatchToken          string
	Status                 string
	CheckedAt              time.Time
	Checks                 []PreDispatchGateCheck
	Blockers               []PreDispatchGateBlocker
	HumanActionRequest     map[string]any
	RetryAfter             *time.Time
	AttemptID              *uuid.UUID
	DecisionRequestID      *uuid.UUID
	CreatedEventID         *uuid.UUID
	CreatedAt              time.Time
	UpdatedAt              time.Time
}

type RecordPreDispatchGateResultRequest struct {
	TenantID               uuid.UUID
	ProjectID              uuid.UUID
	ProjectTaskID          uuid.UUID
	AcceptedPlanRevisionID *uuid.UUID
	PlannedTaskKey         *string
	SelectedEmployeeID     uuid.UUID
	AttemptNo              int32
	DispatchReason         string
	IdempotencyKey         string
	DispatchToken          string
	Status                 string
	CheckedAt              time.Time
	Checks                 []PreDispatchGateCheck
	Blockers               []PreDispatchGateBlocker
	HumanActionRequest     map[string]any
	RetryAfter             *time.Time
	CreatedEventID         *uuid.UUID
}

type ListPreDispatchGateResultsRequest struct {
	TenantID      uuid.UUID
	ProjectID     uuid.UUID
	ProjectTaskID uuid.UUID
	Limit         int32
	Offset        int32
}

type LinkPreDispatchGateAttemptRequest struct {
	TenantID      uuid.UUID
	ProjectID     uuid.UUID
	ProjectTaskID uuid.UUID
	GateResultID  uuid.UUID
	AttemptID     uuid.UUID
}

type LinkPreDispatchGateDecisionRequest struct {
	TenantID          uuid.UUID
	ProjectID         uuid.UUID
	ProjectTaskID     uuid.UUID
	GateResultID      uuid.UUID
	DecisionRequestID uuid.UUID
}

type MoveProjectTaskToWaitingHumanForPreDispatchGateRequest struct {
	TenantID          uuid.UUID
	ProjectID         uuid.UUID
	ProjectTaskID     uuid.UUID
	GateResultID      uuid.UUID
	WaitingReason     string
	DecisionRequestID *uuid.UUID
	EventID           *uuid.UUID
}
```

- [ ] **Step 4: Implement pg repository mapping**

Add helper functions to `apps/control-plane/internal/project/pg_repository.go`:

```go
func preDispatchGateResultFromRecord(row queries.ProjectTaskDispatchGateResult) (PreDispatchGateResult, error) {
	var checks []PreDispatchGateCheck
	if len(row.Checks) > 0 {
		if err := json.Unmarshal(row.Checks, &checks); err != nil {
			return PreDispatchGateResult{}, err
		}
	}
	var blockers []PreDispatchGateBlocker
	if len(row.Blockers) > 0 {
		if err := json.Unmarshal(row.Blockers, &blockers); err != nil {
			return PreDispatchGateResult{}, err
		}
	}
	humanAction := map[string]any{}
	if len(row.HumanActionRequest) > 0 {
		if err := json.Unmarshal(row.HumanActionRequest, &humanAction); err != nil {
			return PreDispatchGateResult{}, err
		}
	}
	return PreDispatchGateResult{
		ID:                     row.ID,
		TenantID:               row.TenantID,
		ProjectID:              row.ProjectID,
		ProjectTaskID:          row.ProjectTaskID,
		AcceptedPlanRevisionID: ptrUUID(row.AcceptedPlanRevisionID),
		PlannedTaskKey:         ptrText(row.PlannedTaskKey),
		SelectedEmployeeID:     row.SelectedEmployeeID,
		AttemptNo:              row.AttemptNo,
		DispatchReason:         row.DispatchReason,
		IdempotencyKey:         row.IdempotencyKey,
		DispatchToken:          row.DispatchToken,
		Status:                 row.Status,
		CheckedAt:              row.CheckedAt,
		Checks:                 checks,
		Blockers:               blockers,
		HumanActionRequest:     humanAction,
		RetryAfter:             ptrTime(row.RetryAfter),
		AttemptID:              ptrUUID(row.AttemptID),
		DecisionRequestID:      ptrUUID(row.DecisionRequestID),
		CreatedEventID:         ptrUUID(row.CreatedEventID),
		CreatedAt:              row.CreatedAt,
		UpdatedAt:              row.UpdatedAt,
	}, nil
}
```

Add repository methods:

```go
func (r *PgRepository) RecordPreDispatchGateResult(ctx context.Context, req RecordPreDispatchGateResultRequest) (PreDispatchGateResult, error) {
	checks, err := marshalJSON(req.Checks, "checks")
	if err != nil {
		return PreDispatchGateResult{}, err
	}
	blockers, err := marshalJSON(req.Blockers, "blockers")
	if err != nil {
		return PreDispatchGateResult{}, err
	}
	humanAction, err := jsonbObject(req.HumanActionRequest, "human_action_request")
	if err != nil {
		return PreDispatchGateResult{}, err
	}
	row, err := r.q.CreateProjectTaskDispatchGateResult(ctx, queries.CreateProjectTaskDispatchGateResultParams{
		ID:                     uuid.New(),
		TenantID:               req.TenantID,
		ProjectID:              req.ProjectID,
		ProjectTaskID:          req.ProjectTaskID,
		AcceptedPlanRevisionID: nullUUID(req.AcceptedPlanRevisionID),
		PlannedTaskKey:         textPtr(req.PlannedTaskKey),
		SelectedEmployeeID:     req.SelectedEmployeeID,
		AttemptNo:              req.AttemptNo,
		DispatchReason:         req.DispatchReason,
		IdempotencyKey:         req.IdempotencyKey,
		DispatchToken:          req.DispatchToken,
		Status:                 req.Status,
		CheckedAt:              req.CheckedAt,
		Checks:                 checks,
		Blockers:               blockers,
		HumanActionRequest:     humanAction,
		RetryAfter:             timestamptzPtr(req.RetryAfter),
		CreatedEventID:         nullUUID(req.CreatedEventID),
	})
	if err != nil {
		return PreDispatchGateResult{}, projectRepositoryError(err)
	}
	if _, markErr := r.q.MarkProjectTaskLatestDispatchGate(ctx, queries.MarkProjectTaskLatestDispatchGateParams{
		LatestDispatchGateResultID: row.ID,
		TenantID:                   req.TenantID,
		ProjectID:                  req.ProjectID,
		ID:                         req.ProjectTaskID,
	}); markErr != nil {
		return PreDispatchGateResult{}, projectRepositoryError(markErr)
	}
	return preDispatchGateResultFromRecord(row)
}

func (r *PgRepository) GetPreDispatchGateResult(ctx context.Context, tenantID, projectID, gateResultID uuid.UUID) (PreDispatchGateResult, error) {
	row, err := r.q.GetProjectTaskDispatchGateResult(ctx, queries.GetProjectTaskDispatchGateResultParams{TenantID: tenantID, ProjectID: projectID, ID: gateResultID})
	if err != nil {
		return PreDispatchGateResult{}, projectRepositoryError(err)
	}
	return preDispatchGateResultFromRecord(row)
}

func (r *PgRepository) ListPreDispatchGateResults(ctx context.Context, req ListPreDispatchGateResultsRequest) ([]PreDispatchGateResult, error) {
	limit := req.Limit
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	rows, err := r.q.ListProjectTaskDispatchGateResults(ctx, queries.ListProjectTaskDispatchGateResultsParams{
		TenantID:      req.TenantID,
		ProjectID:     req.ProjectID,
		ProjectTaskID: req.ProjectTaskID,
		Limit:         limit,
		Offset:        req.Offset,
	})
	if err != nil {
		return nil, projectRepositoryError(err)
	}
	results := make([]PreDispatchGateResult, 0, len(rows))
	for _, row := range rows {
		result, err := preDispatchGateResultFromRecord(row)
		if err != nil {
			return nil, err
		}
		results = append(results, result)
	}
	return results, nil
}
```

Add the link and transition methods:

```go
func (r *PgRepository) LinkPreDispatchGateAttempt(ctx context.Context, req LinkPreDispatchGateAttemptRequest) (PreDispatchGateResult, error) {
	row, err := r.q.LinkProjectTaskDispatchGateAttempt(ctx, queries.LinkProjectTaskDispatchGateAttemptParams{
		AttemptID:     req.AttemptID,
		TenantID:      req.TenantID,
		ProjectID:     req.ProjectID,
		ProjectTaskID: req.ProjectTaskID,
		ID:            req.GateResultID,
	})
	if err != nil {
		return PreDispatchGateResult{}, projectRepositoryError(err)
	}
	return preDispatchGateResultFromRecord(row)
}

func (r *PgRepository) LinkPreDispatchGateDecisionRequest(ctx context.Context, req LinkPreDispatchGateDecisionRequest) (PreDispatchGateResult, error) {
	row, err := r.q.LinkProjectTaskDispatchGateDecisionRequest(ctx, queries.LinkProjectTaskDispatchGateDecisionRequestParams{
		DecisionRequestID: req.DecisionRequestID,
		TenantID:          req.TenantID,
		ProjectID:         req.ProjectID,
		ProjectTaskID:     req.ProjectTaskID,
		ID:                req.GateResultID,
	})
	if err != nil {
		return PreDispatchGateResult{}, projectRepositoryError(err)
	}
	if _, err := r.q.SetProjectDecisionRequestDispatchGate(ctx, queries.SetProjectDecisionRequestDispatchGateParams{
		DispatchGateResultID: req.GateResultID,
		TenantID:             req.TenantID,
		ProjectID:            req.ProjectID,
		ID:                   req.DecisionRequestID,
	}); err != nil {
		return PreDispatchGateResult{}, projectRepositoryError(err)
	}
	return preDispatchGateResultFromRecord(row)
}

func (r *PgRepository) MoveProjectTaskToWaitingHumanForPreDispatchGate(ctx context.Context, req MoveProjectTaskToWaitingHumanForPreDispatchGateRequest) (ProjectTask, error) {
	row, err := r.q.MovePlannedProjectTaskToWaitingHumanForGate(ctx, queries.MovePlannedProjectTaskToWaitingHumanForGateParams{
		WaitingReason:              req.WaitingReason,
		WaitingRequestID:           nullUUID(req.DecisionRequestID),
		LatestDispatchGateResultID: req.GateResultID,
		LatestEventID:              nullUUID(req.EventID),
		TenantID:                   req.TenantID,
		ProjectID:                  req.ProjectID,
		ID:                         req.ProjectTaskID,
	})
	if err != nil {
		return ProjectTask{}, projectRepositoryError(err)
	}
	return taskFromRecord(row)
}
```

Update `QueueProjectTaskWithAttempt` so `CreateProjectTaskAttempt` receives `DispatchGateResultID`, `QueueProjectTask` marks `latest_dispatch_gate_result_id`, and `LinkPreDispatchGateAttempt` is called in the same transaction after the attempt exists.

- [ ] **Step 5: Update service test fake repository**

Modify the in-memory repository in `apps/control-plane/internal/project/service_test.go` with methods that store gate results in a slice. Use the same idempotency rule as the real repository:

```go
func (r *memoryRepository) RecordPreDispatchGateResult(ctx context.Context, req RecordPreDispatchGateResultRequest) (PreDispatchGateResult, error) {
	for i, result := range r.dispatchGateResults {
		if result.TenantID == req.TenantID && result.ProjectTaskID == req.ProjectTaskID && result.IdempotencyKey == req.IdempotencyKey {
			r.dispatchGateResults[i].Status = req.Status
			r.dispatchGateResults[i].Checks = req.Checks
			r.dispatchGateResults[i].Blockers = req.Blockers
			r.dispatchGateResults[i].HumanActionRequest = req.HumanActionRequest
			r.dispatchGateResults[i].RetryAfter = req.RetryAfter
			return r.dispatchGateResults[i], nil
		}
	}
	result := PreDispatchGateResult{
		ID:                     uuid.New(),
		TenantID:               req.TenantID,
		ProjectID:              req.ProjectID,
		ProjectTaskID:          req.ProjectTaskID,
		AcceptedPlanRevisionID: req.AcceptedPlanRevisionID,
		PlannedTaskKey:         req.PlannedTaskKey,
		SelectedEmployeeID:     req.SelectedEmployeeID,
		AttemptNo:              req.AttemptNo,
		DispatchReason:         req.DispatchReason,
		IdempotencyKey:         req.IdempotencyKey,
		DispatchToken:          req.DispatchToken,
		Status:                 req.Status,
		CheckedAt:              req.CheckedAt,
		Checks:                 req.Checks,
		Blockers:               req.Blockers,
		HumanActionRequest:     req.HumanActionRequest,
		RetryAfter:             req.RetryAfter,
	}
	r.dispatchGateResults = append(r.dispatchGateResults, result)
	return result, nil
}
```

Add the other gate methods with direct in-memory lookup, link, and task update behavior. Return `ErrProjectNotFound` when the requested gate or task is missing.

- [ ] **Step 6: Run repository tests and commit**

Run:

```bash
go test ./apps/control-plane/internal/project -run 'TestRecordPreDispatchGateResult|TestMoveProjectTaskToWaitingHumanForPreDispatchGate|TestLinkPreDispatchGateResultToAttempt|TestQueueProjectTask' -count=1
```

Expected: PASS.

Commit:

```bash
git add apps/control-plane/internal/project/repository.go apps/control-plane/internal/project/types.go apps/control-plane/internal/project/pg_repository.go apps/control-plane/internal/project/pg_repository_test.go apps/control-plane/internal/project/service_test.go
git commit -m "feat: store dispatch gate outcomes"
```

### Task 4: Gate Context Loader And Human Action Creation

**Files:**
- Create: `apps/control-plane/internal/workflow/projectcoordination/predispatch_gate.go`
- Create: `apps/control-plane/internal/workflow/projectcoordination/predispatch_gate_test.go`
- Modify: `apps/control-plane/internal/workflow/projectcoordination/project_store.go`

- [ ] **Step 1: Write failing gate store tests**

Create `apps/control-plane/internal/workflow/projectcoordination/predispatch_gate_test.go`:

```go
package projectcoordination

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/singe/superteam/apps/control-plane/internal/project"
)

func TestProjectStoreRunPreDispatchGatePersistsPassedResult(t *testing.T) {
	ctx := context.Background()
	tenantID := uuid.New()
	projectID := uuid.New()
	taskID := uuid.New()
	employeeID := uuid.New()
	repo := newPreDispatchGateMemoryRepository(tenantID, projectID, taskID, employeeID)
	store := NewProjectStoreWithApprovalsInboxAndRunStarter(repo, nil, nil, &projectTaskRunStarterFake{}).WithClock(func() time.Time {
		return time.Date(2026, 6, 21, 11, 0, 0, 0, time.UTC)
	})

	decision, err := store.RunPreDispatchGate(ctx, DispatchProjectTaskInput{
		TenantID:       tenantID,
		ProjectID:      projectID,
		TaskID:         taskID,
		DispatchReason: project.DispatchReasonRootReady,
	})
	require.NoError(t, err)
	require.Equal(t, project.PreDispatchGateStatusPassed, decision.Gate.Status)
	require.True(t, decision.AllowRunStart)
	require.Empty(t, repo.createdDecisionRequests)
	require.Len(t, repo.dispatchGateResults, 1)
}

func TestProjectStoreRunPreDispatchGateCreatesHumanRequestOnce(t *testing.T) {
	ctx := context.Background()
	tenantID := uuid.New()
	projectID := uuid.New()
	taskID := uuid.New()
	employeeID := uuid.New()
	repo := newPreDispatchGateMemoryRepository(tenantID, projectID, taskID, employeeID)
	repo.snapshot.Risk = project.PreDispatchRiskSnapshot{HumanApprovalRequired: true, HumanApprovalGranted: false, Reason: "database.write"}
	store := NewProjectStoreWithApprovalsInboxAndRunStarter(repo, &recordingApprovalClient{}, &recordingInbox{}, &projectTaskRunStarterFake{}).WithClock(func() time.Time {
		return time.Date(2026, 6, 21, 11, 1, 0, 0, time.UTC)
	})

	first, err := store.RunPreDispatchGate(ctx, DispatchProjectTaskInput{TenantID: tenantID, ProjectID: projectID, TaskID: taskID, DispatchReason: project.DispatchReasonRootReady})
	require.NoError(t, err)
	second, err := store.RunPreDispatchGate(ctx, DispatchProjectTaskInput{TenantID: tenantID, ProjectID: projectID, TaskID: taskID, DispatchReason: project.DispatchReasonRootReady})
	require.NoError(t, err)

	require.False(t, first.AllowRunStart)
	require.False(t, second.AllowRunStart)
	require.Equal(t, project.PreDispatchGateStatusWaitingHuman, first.Gate.Status)
	require.Len(t, repo.createdDecisionRequests, 1)
	require.NotNil(t, repo.tasks[taskID].WaitingRequestID)
	require.Equal(t, repo.createdDecisionRequests[0].ID, *repo.tasks[taskID].WaitingRequestID)
}

func TestProjectStoreRunPreDispatchGateDoesNotCreateRunOnRetryLater(t *testing.T) {
	ctx := context.Background()
	tenantID := uuid.New()
	projectID := uuid.New()
	taskID := uuid.New()
	employeeID := uuid.New()
	repo := newPreDispatchGateMemoryRepository(tenantID, projectID, taskID, employeeID)
	retryAt := time.Date(2026, 6, 21, 11, 5, 0, 0, time.UTC)
	repo.snapshot.Runtime = project.PreDispatchRuntimeSnapshot{
		NodeOnline:              true,
		ProviderAvailable:       true,
		WorkspaceReady:          true,
		SlotAvailable:           false,
		ContractVersionAccepted: true,
		RetryAfter:              retryAt,
	}
	store := NewProjectStoreWithApprovalsInboxAndRunStarter(repo, nil, nil)

	decision, err := store.RunPreDispatchGate(ctx, DispatchProjectTaskInput{TenantID: tenantID, ProjectID: projectID, TaskID: taskID, DispatchReason: project.DispatchReasonRootReady})
	require.NoError(t, err)
	require.Equal(t, project.PreDispatchGateStatusRetryLater, decision.Gate.Status)
	require.False(t, decision.AllowRunStart)
	require.NotNil(t, decision.Gate.RetryAfter)
	require.Equal(t, retryAt, *decision.Gate.RetryAfter)
}
```

- [ ] **Step 2: Run tests and verify they fail**

Run:

```bash
go test ./apps/control-plane/internal/workflow/projectcoordination -run 'TestProjectStoreRunPreDispatchGate' -count=1
```

Expected: FAIL with undefined `RunPreDispatchGate`, `WithClock`, and test fake types.

- [ ] **Step 3: Add gate decision types and clock injection**

Modify `apps/control-plane/internal/workflow/projectcoordination/project_store.go`:

```go
type clockFunc func() time.Time

type PreDispatchGateDecision struct {
	Gate          project.PreDispatchGateResult
	AllowRunStart bool
	Retryable     bool
	Terminal      bool
}

func (s *ProjectStore) WithClock(clock clockFunc) *ProjectStore {
	s.clock = clock
	return s
}

func (s *ProjectStore) now() time.Time {
	if s.clock != nil {
		return s.clock()
	}
	return time.Now().UTC()
}
```

Add `clock clockFunc` to `ProjectStore`.

- [ ] **Step 4: Add context loader and gate runner**

Create `apps/control-plane/internal/workflow/projectcoordination/predispatch_gate.go`:

```go
package projectcoordination

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"github.com/singe/superteam/apps/control-plane/internal/approval"
	"github.com/singe/superteam/apps/control-plane/internal/project"
)

func (s *ProjectStore) RunPreDispatchGate(ctx context.Context, input DispatchProjectTaskInput) (PreDispatchGateDecision, error) {
	if s.repository == nil {
		return PreDispatchGateDecision{}, ErrActivityStoreRequired
	}
	task, err := s.repository.GetProjectTask(ctx, input.TenantID, input.TaskID)
	if err != nil {
		return PreDispatchGateDecision{}, err
	}
	if task.ProjectID != input.ProjectID {
		return PreDispatchGateDecision{}, project.ErrProjectNotFound
	}
	if task.AssignedDigitalEmployeeID == nil {
		return s.recordEvaluatedGate(ctx, input, task, project.EvaluatePreDispatchGate(project.PreDispatchGateInput{
			ProjectID:          input.ProjectID,
			ProjectTaskID:      input.TaskID,
			SelectedEmployeeID: uuid.Nil,
			AttemptNo:          task.AttemptCount + 1,
			DispatchReason:     input.DispatchReason,
		}, project.PreDispatchGateSnapshot{Task: task}, s.now()))
	}
	snapshot, err := s.loadPreDispatchGateSnapshot(ctx, input, task)
	if err != nil {
		return PreDispatchGateDecision{}, err
	}
	evaluation := project.EvaluatePreDispatchGate(project.PreDispatchGateInput{
		ProjectID:              input.ProjectID,
		ProjectTaskID:          input.TaskID,
		AcceptedPlanRevisionID: task.AcceptedPlanRevisionID,
		PlannedTaskKey:         task.PlannedTaskKey,
		SelectedEmployeeID:     *task.AssignedDigitalEmployeeID,
		AttemptNo:              task.AttemptCount + 1,
		DispatchReason:         input.DispatchReason,
	}, snapshot, s.now())
	return s.recordEvaluatedGate(ctx, input, task, evaluation)
}

func (s *ProjectStore) loadPreDispatchGateSnapshot(ctx context.Context, input DispatchProjectTaskInput, task project.ProjectTask) (project.PreDispatchGateSnapshot, error) {
	snapshot := project.PreDispatchGateSnapshot{
		Task: task,
		Employee: project.PreDispatchEmployeeSnapshot{
			ID:                 *task.AssignedDigitalEmployeeID,
			IsProjectExecutor:  true,
			Status:             "active",
			PolicyAllowed:      true,
			RequiredLoadSlots:  1,
			AvailableLoadSlots: 1,
		},
		Runtime: project.PreDispatchRuntimeSnapshot{
			NodeOnline:              true,
			ProviderAvailable:       true,
			WorkspaceReady:          true,
			SlotAvailable:           true,
			ContractVersionAccepted: true,
		},
		Budget:  project.PreDispatchBudgetSnapshot{ProjectBudgetAllowed: true, TaskBudgetPresent: true},
		Context: project.PreDispatchContextSnapshot{RequiredRefsResolved: true, InjectionAllowed: true},
	}
	if attempt, err := s.repository.GetCurrentProjectTaskAttempt(ctx, input.TenantID, task.ID); err == nil {
		snapshot.ActiveAttempt = &project.PreDispatchAttemptSnapshot{ID: attempt.ID, Status: attempt.Status}
	} else if !errors.Is(err, project.ErrProjectNotFound) {
		return snapshot, err
	}
	if task.RequiresHumanApproval {
		snapshot.Risk = project.PreDispatchRiskSnapshot{HumanApprovalRequired: true, HumanApprovalGranted: false, Reason: "task.requires_human_approval"}
	}
	return snapshot, nil
}

func (s *ProjectStore) recordEvaluatedGate(ctx context.Context, input DispatchProjectTaskInput, task project.ProjectTask, evaluation project.PreDispatchGateEvaluation) (PreDispatchGateDecision, error) {
	eventType := gateEventType(evaluation.Status)
	event, err := s.repository.AppendProjectEvent(ctx, coordinatorEvent(input.TenantID, input.ProjectID, eventType, input.TaskID.String(), gateEventSummary(evaluation.Status), map[string]any{
		"project_task_id": input.TaskID.String(),
		"status":          evaluation.Status,
		"dispatch_reason": input.DispatchReason,
		"blockers":        evaluation.Blockers,
		"retry_after":     evaluation.RetryAfter,
	}))
	if err != nil {
		return PreDispatchGateDecision{}, err
	}
	gate, err := s.repository.RecordPreDispatchGateResult(ctx, project.RecordPreDispatchGateResultRequest{
		TenantID:               input.TenantID,
		ProjectID:              input.ProjectID,
		ProjectTaskID:          input.TaskID,
		AcceptedPlanRevisionID: task.AcceptedPlanRevisionID,
		PlannedTaskKey:         task.PlannedTaskKey,
		SelectedEmployeeID:     selectedEmployeeID(task),
		AttemptNo:              task.AttemptCount + 1,
		DispatchReason:         input.DispatchReason,
		IdempotencyKey:         evaluation.IdempotencyKey,
		DispatchToken:          evaluation.DispatchToken,
		Status:                 evaluation.Status,
		CheckedAt:              evaluation.CheckedAt,
		Checks:                 evaluation.Checks,
		Blockers:               evaluation.Blockers,
		HumanActionRequest:     humanActionMap(evaluation.HumanActionRequest),
		RetryAfter:             evaluation.RetryAfter,
		CreatedEventID:         &event.ID,
	})
	if err != nil {
		return PreDispatchGateDecision{}, err
	}
	if evaluation.Status == project.PreDispatchGateStatusWaitingHuman && evaluation.HumanActionRequest != nil {
		if _, err := s.createGateHumanAction(ctx, input, task, gate, *evaluation.HumanActionRequest); err != nil {
			return PreDispatchGateDecision{}, err
		}
	}
	return PreDispatchGateDecision{
		Gate:          gate,
		AllowRunStart: evaluation.Status == project.PreDispatchGateStatusPassed,
		Retryable:     evaluation.Status == project.PreDispatchGateStatusRetryLater,
		Terminal:      evaluation.Status == project.PreDispatchGateStatusBlocked || evaluation.Status == project.PreDispatchGateStatusReplanRequired,
	}, nil
}

func (s *ProjectStore) createGateHumanAction(ctx context.Context, input DispatchProjectTaskInput, task project.ProjectTask, gate project.PreDispatchGateResult, action project.PreDispatchHumanActionRequest) (project.DecisionRequest, error) {
	projectRecord, err := s.repository.GetProject(ctx, input.TenantID, input.ProjectID)
	if err != nil {
		return project.DecisionRequest{}, err
	}
	approvalRequestID := uuid.Nil
	if s.approvals != nil {
		approvalRequest, err := s.approvals.CreateRequest(ctx, approval.CreateRequestInput{
			TenantID:       input.TenantID,
			ResourceType:   "project_task_dispatch_gate",
			ResourceID:     gate.ID,
			RequesterType:  "project_coordinator",
			TargetUserID:   projectRecord.HumanOwnerUserID,
			DecisionType:   action.DecisionType,
			Title:          action.Title,
			Summary:        action.Summary,
			RiskLevel:      action.RiskLevel,
			Options:        action.Options,
			ContextPayload:  action.Context,
		})
		if err != nil {
			return project.DecisionRequest{}, err
		}
		approvalRequestID = approvalRequest.ID
	}
	event, err := s.repository.AppendProjectEvent(ctx, coordinatorEvent(input.TenantID, input.ProjectID, project.ProjectEventDecisionRequested, gate.ID.String(), action.Title, map[string]any{
		"dispatch_gate_result_id": gate.ID.String(),
		"project_task_id":         task.ID.String(),
		"decision_type":           action.DecisionType,
	}))
	if err != nil {
		return project.DecisionRequest{}, err
	}
	decision, err := s.repository.CreateDecisionRequest(ctx, project.CreateDecisionRequestRequest{
		TenantID:          input.TenantID,
		ProjectID:         input.ProjectID,
		ApprovalRequestID: approvalRequestID,
		CoordinationJobID: task.CoordinationJobID,
		ProjectTaskID:     &task.ID,
		TargetUserID:      projectRecord.HumanOwnerUserID,
		DecisionType:      action.DecisionType,
		TitleSnapshot:     action.Title,
		SummarySnapshot:   action.Summary,
		RiskLevelSnapshot: action.RiskLevel,
		StatusSnapshot:    "pending",
		CreatedEventID:    &event.ID,
	})
	if err != nil {
		return project.DecisionRequest{}, err
	}
	if _, err := s.repository.LinkPreDispatchGateDecisionRequest(ctx, project.LinkPreDispatchGateDecisionRequest{
		TenantID:          input.TenantID,
		ProjectID:         input.ProjectID,
		ProjectTaskID:     task.ID,
		GateResultID:      gate.ID,
		DecisionRequestID: decision.ID,
	}); err != nil {
		return project.DecisionRequest{}, err
	}
	_, err = s.repository.MoveProjectTaskToWaitingHumanForPreDispatchGate(ctx, project.MoveProjectTaskToWaitingHumanForPreDispatchGateRequest{
		TenantID:          input.TenantID,
		ProjectID:         input.ProjectID,
		ProjectTaskID:     task.ID,
		GateResultID:      gate.ID,
		WaitingReason:     action.WaitingReason,
		DecisionRequestID: &decision.ID,
		EventID:           &event.ID,
	})
	if err != nil {
		return project.DecisionRequest{}, err
	}
	if s.inbox != nil {
		if err := s.inbox.UpsertProjectDecisionRequest(ctx, decision); err != nil {
			return project.DecisionRequest{}, err
		}
	}
	return decision, nil
}
```

Add helper functions `gateEventType`, `gateEventSummary`, `selectedEmployeeID`, and `humanActionMap` in the same file with direct status mappings to the event constants from Task 1.

- [ ] **Step 5: Add realistic adapters incrementally**

Extend `loadPreDispatchGateSnapshot` to load real facts in this order:

1. `ListProjectTaskDependencies` / dependency readiness for upstream completion.
2. `ListProjectMembers` to confirm `PrincipalTypeDigitalEmployee`, `ProjectRoleExecutor`, and active member status.
3. `task.PlannerMetadata`, `task.InputRequirements`, and `task.HandoffContract` to extract required capabilities, permission requirements, tool requirements, runtime requirements, context refs, and budget metadata.
4. Injected adapter interfaces for employee execution instance, runtime node/capabilities, and MCP bindings.

Use these small interfaces in `predispatch_gate.go`:

```go
type GateEmployeeRuntimeReader interface {
	GetEmployeeRuntimeSnapshot(ctx context.Context, tenantID, employeeID uuid.UUID) (project.PreDispatchEmployeeSnapshot, project.PreDispatchRuntimeSnapshot, error)
}

type GateCapabilityReader interface {
	GetEmployeeCapabilitySnapshot(ctx context.Context, tenantID, employeeID uuid.UUID, task project.ProjectTask) (project.PreDispatchCapabilitySnapshot, project.PreDispatchToolSnapshot, error)
}
```

Add `WithPreDispatchGateReaders(employeeReader GateEmployeeRuntimeReader, capabilityReader GateCapabilityReader) *ProjectStore` to keep app wiring out of pure workflow code.

- [ ] **Step 6: Run tests and commit**

Run:

```bash
go test ./apps/control-plane/internal/workflow/projectcoordination -run 'TestProjectStoreRunPreDispatchGate' -count=1
```

Expected: PASS.

Commit:

```bash
git add apps/control-plane/internal/workflow/projectcoordination/predispatch_gate.go apps/control-plane/internal/workflow/projectcoordination/predispatch_gate_test.go apps/control-plane/internal/workflow/projectcoordination/project_store.go
git commit -m "feat: evaluate dispatch gates in project store"
```

### Task 5: Dispatch Integration And Idempotency

**Files:**
- Modify: `apps/control-plane/internal/workflow/projectcoordination/types.go`
- Modify: `apps/control-plane/internal/workflow/projectcoordination/activities.go`
- Modify: `apps/control-plane/internal/workflow/projectcoordination/project_store.go`
- Modify: `apps/control-plane/internal/workflow/projectcoordination/project_store_test.go`

- [ ] **Step 1: Write failing dispatch integration tests**

Append to `apps/control-plane/internal/workflow/projectcoordination/project_store_test.go`:

```go
func TestProjectStoreDispatchProjectTaskRunsGateBeforeRunStart(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	demandID := uuid.New()
	taskID := uuid.New()
	employeeID := uuid.New()
	runID := uuid.New()
	runtimeTaskID := uuid.New()
	runtimeNodeID := uuid.New()
	repo := newProjectStoreMemoryRepository()
	repo.projects[projectID] = project.Project{ID: projectID, TenantID: tenantID, HumanOwnerUserID: uuid.New()}
	repo.demands[demandID] = project.ProjectDemand{ID: demandID, TenantID: tenantID, ProjectID: projectID, Title: "Analyze data"}
	repo.tasks[taskID] = project.ProjectTask{
		ID:                        taskID,
		TenantID:                  tenantID,
		ProjectID:                 projectID,
		DemandID:                  &demandID,
		Title:                     "Inspect DB",
		Status:                    project.ProjectTaskStatusPlanned,
		AssignedDigitalEmployeeID: &employeeID,
		MaxAttempts:               int32Ptr(3),
	}
	starter := &projectTaskRunStarterFake{result: StartProjectTaskRunResult{RunID: runID, RuntimeTaskID: runtimeTaskID, RuntimeNodeID: runtimeNodeID, NodeID: "runtime-dev"}}
	store := NewProjectStoreWithApprovalsInboxAndRunStarter(repo, nil, nil, starter)

	err := store.DispatchProjectTask(context.Background(), DispatchProjectTaskInput{
		TenantID:       tenantID,
		ProjectID:      projectID,
		TaskID:         taskID,
		DispatchReason: project.DispatchReasonRootReady,
	})
	require.NoError(t, err)

	require.Len(t, repo.dispatchGateResults, 1)
	require.Equal(t, project.PreDispatchGateStatusPassed, repo.dispatchGateResults[0].Status)
	require.Len(t, starter.requests, 1)
	require.Equal(t, repo.dispatchGateResults[0].ID.String(), starter.requests[0].Metadata["dispatch_gate_result_id"])
	require.NotNil(t, repo.projectTaskAttempts[0].DispatchGateResultID)
	require.Equal(t, repo.dispatchGateResults[0].ID, *repo.projectTaskAttempts[0].DispatchGateResultID)
}

func TestProjectStoreDispatchProjectTaskWaitingHumanGateDoesNotStartRun(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	taskID := uuid.New()
	employeeID := uuid.New()
	repo := newProjectStoreMemoryRepositoryWithTask(tenantID, projectID, taskID, employeeID, project.ProjectTaskStatusPlanned)
	task := repo.tasks[taskID]
	task.RequiresHumanApproval = true
	repo.tasks[taskID] = task
	starter := &projectTaskRunStarterFake{}
	store := NewProjectStoreWithApprovalsInboxAndRunStarter(repo, &approvalClientFake{}, nil, starter)

	err := store.DispatchProjectTask(context.Background(), DispatchProjectTaskInput{
		TenantID:       tenantID,
		ProjectID:      projectID,
		TaskID:         taskID,
		DispatchReason: project.DispatchReasonRootReady,
	})
	require.NoError(t, err)

	require.Len(t, starter.requests, 0)
	require.Len(t, repo.dispatchGateResults, 1)
	require.Equal(t, project.PreDispatchGateStatusWaitingHuman, repo.dispatchGateResults[0].Status)
	require.Equal(t, project.ProjectTaskStatusWaitingHuman, repo.tasks[taskID].Status)
}

func TestProjectStoreDispatchProjectTaskRetryLaterGateIsRetryable(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	taskID := uuid.New()
	employeeID := uuid.New()
	repo := newProjectStoreMemoryRepositoryWithTask(tenantID, projectID, taskID, employeeID, project.ProjectTaskStatusPlanned)
	repo.forceRuntimeSlotUnavailable = true
	store := NewProjectStoreWithApprovalsInboxAndRunStarter(repo, nil, nil, &projectTaskRunStarterFake{})

	err := store.DispatchProjectTask(context.Background(), DispatchProjectTaskInput{
		TenantID:       tenantID,
		ProjectID:      projectID,
		TaskID:         taskID,
		DispatchReason: project.DispatchReasonRootReady,
	})

	var dispatchErr *ProjectTaskDispatchError
	require.ErrorAs(t, err, &dispatchErr)
	require.True(t, dispatchErrorRetryable(err))
	require.Len(t, repo.dispatchGateResults, 1)
	require.Equal(t, project.PreDispatchGateStatusRetryLater, repo.dispatchGateResults[0].Status)
}
```

- [ ] **Step 2: Run tests and verify they fail**

Run:

```bash
go test ./apps/control-plane/internal/workflow/projectcoordination -run 'TestProjectStoreDispatchProjectTask.*Gate' -count=1
```

Expected: FAIL because `DispatchProjectTask` does not call the gate and does not pass gate IDs into run metadata or attempts.

- [ ] **Step 3: Extend dispatch input**

Modify `apps/control-plane/internal/workflow/projectcoordination/types.go`:

```go
type DispatchProjectTaskInput struct {
	TenantID       uuid.UUID
	ProjectID      uuid.UUID
	TaskID         uuid.UUID
	DispatchReason string
}
```

Modify `Activities.DispatchProjectTask` to default empty `DispatchReason` to `project.DispatchReasonRootReady` before delegating.

- [ ] **Step 4: Insert gate into dispatch path**

Modify `DispatchProjectTask` in `apps/control-plane/internal/workflow/projectcoordination/project_store.go` after task validation and before `GetProject` / `GetProjectDemand`:

```go
gateDecision, err := s.RunPreDispatchGate(ctx, input)
if err != nil {
	return err
}
if !gateDecision.AllowRunStart {
	if gateDecision.Retryable {
		return &ProjectTaskDispatchError{FailureRecorded: true, Err: ErrProjectTaskDispatchRetryLater}
	}
	if gateDecision.Terminal {
		return &ProjectTaskDispatchError{FailureRecorded: true, Err: project.ErrInvalidProject}
	}
	return nil
}
```

Add sentinel:

```go
var ErrProjectTaskDispatchRetryLater = errors.New("project task dispatch gate retry later")
```

Update `dispatchErrorRetryable`:

```go
if errors.Is(err, ErrProjectTaskDispatchRetryLater) {
	return true
}
```

Add gate metadata to `StartProjectTaskRunRequest.Metadata`:

```go
"dispatch_gate_result_id": gateDecision.Gate.ID.String(),
"dispatch_gate_token":     gateDecision.Gate.DispatchToken,
"dispatch_reason":         gateDecision.Gate.DispatchReason,
```

Add gate ID to `QueueProjectTaskWithAttempt`:

```go
DispatchGateResultID: &gateDecision.Gate.ID,
```

Add gate ID to `ExecutionContextPacket`:

```go
"dispatch_gate_result_id": gateDecision.Gate.ID.String(),
"dispatch_gate_token":     gateDecision.Gate.DispatchToken,
"dispatch_reason":         gateDecision.Gate.DispatchReason,
```

After `QueueProjectTaskWithAttempt` succeeds, call `LinkPreDispatchGateAttempt` only if the repository did not already link it during queueing:

```go
if _, err := s.repository.LinkPreDispatchGateAttempt(ctx, project.LinkPreDispatchGateAttemptRequest{
	TenantID:      input.TenantID,
	ProjectID:     input.ProjectID,
	ProjectTaskID: input.TaskID,
	GateResultID:  gateDecision.Gate.ID,
	AttemptID:     queued.Attempt.ID,
}); err != nil {
	return s.recordDispatchFailure(ctx, input.TenantID, input.ProjectID, task, err)
}
```

Keep the existing already-bound-run branch at the top of `DispatchProjectTask` unchanged. That branch is the replay path for an already-created run and must not create a new gate result.

- [ ] **Step 5: Run dispatch tests and commit**

Run:

```bash
go test ./apps/control-plane/internal/workflow/projectcoordination -run 'TestProjectStoreDispatchProjectTask.*Gate|TestProjectStoreDispatchProjectTaskStartsRunAndQueuesTask|TestProjectStoreDispatchProjectTaskQueuedAttemptEventIsIdempotentOnRetry|TestActivitiesDispatchProjectTask' -count=1
```

Expected: PASS.

Commit:

```bash
git add apps/control-plane/internal/workflow/projectcoordination/types.go apps/control-plane/internal/workflow/projectcoordination/activities.go apps/control-plane/internal/workflow/projectcoordination/project_store.go apps/control-plane/internal/workflow/projectcoordination/project_store_test.go
git commit -m "feat: gate project task dispatch before runtime run"
```

### Task 6: Workflow Dispatch Reasons And Human Resume

**Files:**
- Modify: `apps/control-plane/internal/workflow/projectcoordination/workflow.go`
- Modify: `apps/control-plane/internal/workflow/projectcoordination/workflow_test.go`

- [ ] **Step 1: Write failing workflow tests**

Modify `apps/control-plane/internal/workflow/projectcoordination/workflow_test.go` to assert dispatch reasons:

```go
func TestProjectCoordinatorDispatchesRootTasksWithRootReadyReason(t *testing.T) {
	env, store := newProjectCoordinatorWorkflowTestEnv(t)
	rootTaskID := uuid.MustParse("00000000-0000-0000-0000-000000000301")
	store.dispatchableTaskIDs = []uuid.UUID{rootTaskID}

	env.ExecuteWorkflow(ProjectCoordinatorWorkflow, ProjectCoordinatorInput{
		TenantID:   store.tenantID,
		ProjectID:  store.projectID,
		WorkflowID: "project-coordinator:" + store.projectID.String(),
	})

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
	require.Len(t, store.dispatchInputs, 1)
	require.Equal(t, project.DispatchReasonRootReady, store.dispatchInputs[0].DispatchReason)
}

func TestProjectCoordinatorDispatchesDownstreamWithDependencyUnlockedReason(t *testing.T) {
	env, store := newProjectCoordinatorWorkflowTestEnv(t)
	completedTaskID := uuid.MustParse("00000000-0000-0000-0000-000000000302")
	downstreamTaskID := uuid.MustParse("00000000-0000-0000-0000-000000000303")
	store.downstreamReadyTaskIDs = []uuid.UUID{downstreamTaskID}

	env.SignalWorkflow(SignalEmployeeTaskCompleted, EmployeeTaskCompleted{
		ProjectTaskID:      completedTaskID,
		ExecutionSummaryID: uuid.New(),
		CompletedEventID:   uuid.New(),
	})
	env.ExecuteWorkflow(ProjectCoordinatorWorkflow, ProjectCoordinatorInput{
		TenantID:   store.tenantID,
		ProjectID:  store.projectID,
		WorkflowID: "project-coordinator:" + store.projectID.String(),
	})

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
	require.Len(t, store.dispatchInputs, 1)
	require.Equal(t, project.DispatchReasonDependencyUnlocked, store.dispatchInputs[0].DispatchReason)
}

func TestProjectCoordinatorDispatchesHumanResolvedTaskThroughGate(t *testing.T) {
	env, store := newProjectCoordinatorWorkflowTestEnv(t)
	taskID := uuid.MustParse("00000000-0000-0000-0000-000000000304")
	store.humanResolvedReadyTaskIDs = []uuid.UUID{taskID}

	env.SignalWorkflow(SignalHumanDecisionSubmitted, HumanDecisionSubmitted{
		DecisionRequestID: uuid.New(),
		Decision:          "approved",
		Payload:           map[string]any{"project_task_id": taskID.String()},
		ResolvedEventID:   uuid.New(),
	})
	env.ExecuteWorkflow(ProjectCoordinatorWorkflow, ProjectCoordinatorInput{
		TenantID:   store.tenantID,
		ProjectID:  store.projectID,
		WorkflowID: "project-coordinator:" + store.projectID.String(),
	})

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
	require.Len(t, store.dispatchInputs, 1)
	require.Equal(t, project.DispatchReasonHumanResolved, store.dispatchInputs[0].DispatchReason)
}
```

- [ ] **Step 2: Run tests and verify they fail**

Run:

```bash
go test ./apps/control-plane/internal/workflow/projectcoordination -run 'TestProjectCoordinatorDispatches.*Reason|TestProjectCoordinatorDispatchesHumanResolvedTaskThroughGate' -count=1
```

Expected: FAIL because workflow dispatch calls do not set reason values.

- [ ] **Step 3: Pass reasons through dispatch helper**

Modify `dispatchProjectTasks` in `workflow.go`:

```go
func dispatchProjectTasks(ctx workflow.Context, tenantID, projectID uuid.UUID, taskIDs []uuid.UUID, reason string) error {
	for _, taskID := range taskIDs {
		if err := workflow.ExecuteActivity(ctx, (*Activities).DispatchProjectTask, DispatchProjectTaskInput{
			TenantID:       tenantID,
			ProjectID:      projectID,
			TaskID:         taskID,
			DispatchReason: reason,
		}).Get(ctx, nil); err != nil {
			workflow.GetLogger(ctx).Warn("project task dispatch failed", "project_task_id", taskID.String(), "error", err)
			return err
		}
	}
	return nil
}
```

Update callers:

```go
return dispatchProjectTasks(ctx, input.TenantID, pending.ProjectID, readyTaskIDs, project.DispatchReasonRootReady)
```

For downstream completion:

```go
return dispatchProjectTasks(ctx, input.TenantID, input.ProjectID, readyTaskIDs, project.DispatchReasonDependencyUnlocked)
```

For human decision resolution:

```go
return dispatchProjectTasks(ctx, input.TenantID, input.ProjectID, readyTaskIDs, project.DispatchReasonHumanResolved)
```

For failure recovery retry:

```go
return dispatchProjectTasks(ctx, input.TenantID, input.ProjectID, recovered.ReadyTaskIDs, project.DispatchReasonRetry)
```

- [ ] **Step 4: Run workflow tests and commit**

Run:

```bash
go test ./apps/control-plane/internal/workflow/projectcoordination -run 'TestProjectCoordinator|TestActivitiesDispatchProjectTask' -count=1
```

Expected: PASS.

Commit:

```bash
git add apps/control-plane/internal/workflow/projectcoordination/workflow.go apps/control-plane/internal/workflow/projectcoordination/workflow_test.go
git commit -m "feat: carry dispatch reasons through coordinator workflow"
```

### Task 7: App Wiring For Employee Runtime And Capability Checks

**Files:**
- Modify: `apps/control-plane/internal/app/app.go`
- Modify: `apps/control-plane/internal/app/planning_profile_adapter.go`
- Modify: `apps/control-plane/internal/app/planning_profile_adapter_test.go`
- Modify: `apps/control-plane/internal/workflow/projectcoordination/predispatch_gate.go`
- Modify: `apps/control-plane/internal/workflow/projectcoordination/predispatch_gate_test.go`

- [ ] **Step 1: Write failing adapter tests**

Append to `apps/control-plane/internal/app/planning_profile_adapter_test.go`:

```go
func TestPreDispatchGateAdapterMapsEmployeeRuntimeFacts(t *testing.T) {
	tenantID := uuid.New()
	employeeID := uuid.New()
	nodeID := uuid.New()
	reader := &fakePlanningProfileEmployeeReader{
		employees: map[uuid.UUID]employee.DigitalEmployeeRecord{
			employeeID: {ID: employeeID, TenantID: tenantID, Status: employee.DigitalEmployeeStatusActive, Name: "Data Analyst"},
		},
		executionInstances: map[uuid.UUID]employee.DigitalEmployeeExecutionInstanceRecord{
			employeeID: {
				ID:                uuid.New(),
				TenantID:          tenantID,
				DigitalEmployeeID: employeeID,
				RuntimeNodeID:     nodeID,
				ProviderType:      "codex",
				Status:            employee.ExecutionInstanceStatusReady,
			},
		},
	}
	runtimeReader := &fakeRuntimeNodeReader{
		nodes: map[string]runtime.NodeRecord{
			nodeID.String(): {ID: nodeID, TenantID: tenantID, NodeID: nodeID.String(), Status: "online", MaxSlots: 2, CurrentLoad: 1},
		},
	}
	adapter := newPreDispatchGateAdapter(reader, nil, runtimeReader)

	employeeSnapshot, runtimeSnapshot, err := adapter.GetEmployeeRuntimeSnapshot(context.Background(), tenantID, employeeID)
	require.NoError(t, err)
	require.Equal(t, employeeID, employeeSnapshot.ID)
	require.Equal(t, "active", employeeSnapshot.Status)
	require.Equal(t, int32(1), employeeSnapshot.AvailableLoadSlots)
	require.True(t, runtimeSnapshot.NodeOnline)
	require.True(t, runtimeSnapshot.ProviderAvailable)
	require.True(t, runtimeSnapshot.SlotAvailable)
}

func TestPreDispatchGateAdapterReportsMissingMCPBinding(t *testing.T) {
	tenantID := uuid.New()
	employeeID := uuid.New()
	task := project.ProjectTask{
		ID:                        uuid.New(),
		TenantID:                  tenantID,
		AssignedDigitalEmployeeID: &employeeID,
		InputRequirements: map[string]any{
			"tool_requirements": []any{"mcp:postgres.readonly"},
		},
	}
	adapter := newPreDispatchGateAdapter(nil, &fakeCapabilityReader{effectiveServers: []capability.MCPServer{}}, nil)

	_, tools, err := adapter.GetEmployeeCapabilitySnapshot(context.Background(), tenantID, employeeID, task)
	require.NoError(t, err)
	require.Equal(t, []string{"mcp:postgres.readonly"}, tools.MissingBindings)
}
```

- [ ] **Step 2: Run tests and verify they fail**

Run:

```bash
go test ./apps/control-plane/internal/app -run 'TestPreDispatchGateAdapter' -count=1
```

Expected: FAIL with undefined `newPreDispatchGateAdapter`.

- [ ] **Step 3: Add gate adapter**

Add to `apps/control-plane/internal/app/planning_profile_adapter.go`:

```go
type preDispatchGateAdapter struct {
	employeeReader digitalEmployeePlanningProfileReader
	capabilityReader interface {
		ListEffectiveMCPServers(ctx context.Context, req capability.EmployeeScopedRequest) ([]capability.MCPServer, error)
	}
	runtimeReader interface {
		GetNode(ctx context.Context, nodeID string) (runtime.NodeRecord, error)
	}
}

func newPreDispatchGateAdapter(employeeReader digitalEmployeePlanningProfileReader, capabilityReader interface {
	ListEffectiveMCPServers(ctx context.Context, req capability.EmployeeScopedRequest) ([]capability.MCPServer, error)
}, runtimeReader interface {
	GetNode(ctx context.Context, nodeID string) (runtime.NodeRecord, error)
}) *preDispatchGateAdapter {
	return &preDispatchGateAdapter{employeeReader: employeeReader, capabilityReader: capabilityReader, runtimeReader: runtimeReader}
}

func (a *preDispatchGateAdapter) GetEmployeeRuntimeSnapshot(ctx context.Context, tenantID, employeeID uuid.UUID) (project.PreDispatchEmployeeSnapshot, project.PreDispatchRuntimeSnapshot, error) {
	employeeRecord, err := a.employeeReader.GetDigitalEmployee(ctx, tenantID, employeeID)
	if err != nil {
		return project.PreDispatchEmployeeSnapshot{ID: employeeID, Status: "unknown"}, project.PreDispatchRuntimeSnapshot{}, nil
	}
	policyAllowed := employeeRecord.Status == employee.DigitalEmployeeStatusActive
	instance, err := a.employeeReader.GetDigitalEmployeeExecutionInstanceByEmployeeID(ctx, tenantID, employeeID)
	if err != nil || instance.RuntimeNodeID == uuid.Nil {
		// No bound execution instance: the employee is not dispatchable on any runtime.
		return project.PreDispatchEmployeeSnapshot{
			ID:                 employeeID,
			IsProjectExecutor:  true,
			Status:             employeeRecord.Status,
			PolicyAllowed:      policyAllowed,
			RequiredLoadSlots:  1,
			AvailableLoadSlots: 0,
		}, project.PreDispatchRuntimeSnapshot{}, nil
	}
	// Load, slots, and node liveness come from the runtime node, NOT the employee
	// execution instance (which carries no concurrency/load facts).
	node, err := a.runtimeReader.GetNode(ctx, instance.RuntimeNodeID.String())
	if err != nil {
		return project.PreDispatchEmployeeSnapshot{
			ID:                 employeeID,
			IsProjectExecutor:  true,
			Status:             employeeRecord.Status,
			PolicyAllowed:      policyAllowed,
			RequiredLoadSlots:  1,
			AvailableLoadSlots: 0,
		}, project.PreDispatchRuntimeSnapshot{}, nil
	}
	available := node.MaxSlots - node.CurrentLoad
	if available < 0 {
		available = 0
	}
	nodeOnline := node.Status == "online" && isRuntimeNodeHeartbeatFresh(node)
	runtimeSnapshot := project.PreDispatchRuntimeSnapshot{
		NodeOnline:              nodeOnline,
		ProviderAvailable:       instance.ProviderType != "" && nodeOnline,
		WorkspaceReady:          instance.Status == employee.ExecutionInstanceStatusReady || instance.Status == employee.ExecutionInstanceStatusActive,
		SlotAvailable:           available > 0,
		ContractVersionAccepted: true,
	}
	return project.PreDispatchEmployeeSnapshot{
		ID:                 employeeID,
		IsProjectExecutor:  true,
		Status:             employeeRecord.Status,
		PolicyAllowed:      policyAllowed,
		RequiredLoadSlots:  1,
		AvailableLoadSlots: available,
	}, runtimeSnapshot, nil
}

// isRuntimeNodeHeartbeatFresh treats a stopped Runtime Agent as offline even before any
// sweeper flips node.Status, by checking the last heartbeat against the dispatch TTL.
func isRuntimeNodeHeartbeatFresh(node runtime.NodeRecord) bool {
	if !node.LastHeartbeatAt.Valid {
		return false
	}
	return time.Since(node.LastHeartbeatAt.Time) <= runtimeNodeHeartbeatTTL
}
```

Add capability snapshot logic that reads required capabilities and tools from `task.InputRequirements`:

```go
func (a *preDispatchGateAdapter) GetEmployeeCapabilitySnapshot(ctx context.Context, tenantID, employeeID uuid.UUID, task project.ProjectTask) (project.PreDispatchCapabilitySnapshot, project.PreDispatchToolSnapshot, error) {
	required := stringSliceFromAny(task.InputRequirements["required_capabilities"])
	matched := append([]string(nil), required...)
	toolRequirements := stringSliceFromAny(task.InputRequirements["tool_requirements"])
	if len(toolRequirements) == 0 {
		return project.PreDispatchCapabilitySnapshot{Required: required, Matched: matched}, project.PreDispatchToolSnapshot{}, nil
	}
	servers, err := a.capabilityReader.ListEffectiveMCPServers(ctx, capability.EmployeeScopedRequest{TenantID: tenantID, DigitalEmployeeID: employeeID})
	if err != nil {
		return project.PreDispatchCapabilitySnapshot{Required: required, Matched: matched}, project.PreDispatchToolSnapshot{RetryableUnavailable: toolRequirements}, nil
	}
	available := map[string]struct{}{}
	for _, server := range servers {
		available["mcp:"+server.Name] = struct{}{}
	}
	missing := []string{}
	for _, requiredTool := range toolRequirements {
		if _, ok := available[requiredTool]; !ok {
			missing = append(missing, requiredTool)
		}
	}
	return project.PreDispatchCapabilitySnapshot{Required: required, Matched: matched}, project.PreDispatchToolSnapshot{MissingBindings: missing}, nil
}
```

- [ ] **Step 4: Wire adapter into app**

Modify `apps/control-plane/internal/app/app.go` where `ProjectStore` is constructed:

```go
gateAdapter := newPreDispatchGateAdapter(employeeRepository, capabilityRepository, runtimeRepository)
coordinationStore = coordinationStore.WithPreDispatchGateReaders(gateAdapter, gateAdapter)
```

Build the adapter from the real reader dependencies (`employeeRepository`, `capabilityRepository`, `runtimeRepository` — pass the runtime **repository**, whose `GetNode` returns `runtime.NodeRecord`; the runtime *service*'s `GetNode` returns `*Node`). `capabilityRepository` is constructed later in `NewContainerWithConfig` than the `coordinationStore` chain, so do not pass gate readers into the constructor; instead append them with the chained builder after `capabilityRepository` exists. Leave the existing four-argument constructor (`projectRepository, approvalService, decisionProjector, projectTaskRunStarterAdapter{runService: runService}`) and its `.WithDigitalEmployeeReadiness(...)` / `.WithLendingGatekeeper(...)` / `.WithDigitalEmployeePlanningProfiles(...)` chain unchanged, and append `.WithPreDispatchGateReaders(gateAdapter, gateAdapter)` once `capabilityRepository` is available.

- [ ] **Step 5: Run adapter/app tests and commit**

Run:

```bash
go test ./apps/control-plane/internal/app -run 'TestPreDispatchGateAdapter|TestDigitalEmployeePlanningProfileAdapter' -count=1
go test ./apps/control-plane/internal/workflow/projectcoordination -run 'TestProjectStoreRunPreDispatchGate' -count=1
```

Expected: PASS.

Commit:

```bash
git add apps/control-plane/internal/app/app.go apps/control-plane/internal/app/planning_profile_adapter.go apps/control-plane/internal/app/planning_profile_adapter_test.go apps/control-plane/internal/workflow/projectcoordination/predispatch_gate.go apps/control-plane/internal/workflow/projectcoordination/predispatch_gate_test.go
git commit -m "feat: wire gate runtime and capability checks"
```

### Task 8: Read-Only API And Web Visibility

**Files:**
- Modify: `apps/control-plane/internal/project/handler.go`
- Modify: `apps/control-plane/internal/project/handler_test.go`
- Modify: `apps/control-plane/internal/api/server.go`
- Modify: `apps/control-plane/internal/api/project_routes_test.go`
- Modify: `contracts/control-plane/openapi.yaml`
- Regenerate: generated control-plane client files
- Modify: `apps/web/src/lib/api/projects.ts`
- Modify: `apps/web/src/lib/api/projects.test.ts`
- Modify: `apps/web/src/features/projects/components/project-operational-detail.tsx`
- Modify: `apps/web/src/features/projects/index.test.tsx`

- [ ] **Step 1: Write failing handler test**

Append to `apps/control-plane/internal/project/handler_test.go`:

```go
func TestHandlerListProjectTaskDispatchGates(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	taskID := uuid.New()
	gateID := uuid.New()
	retryAfter := time.Date(2026, 6, 21, 12, 2, 0, 0, time.UTC)
	service := &handlerTestService{
		dispatchGates: []PreDispatchGateResult{
			{
				ID:                 gateID,
				TenantID:           tenantID,
				ProjectID:          projectID,
				ProjectTaskID:      taskID,
				SelectedEmployeeID: uuid.New(),
				AttemptNo:          1,
				DispatchReason:     DispatchReasonRootReady,
				Status:             PreDispatchGateStatusRetryLater,
				CheckedAt:          time.Date(2026, 6, 21, 12, 0, 0, 0, time.UTC),
				Blockers:           []PreDispatchGateBlocker{{Key: "runtime.node_offline", Severity: "transient", Retryable: true}},
				RetryAfter:         &retryAfter,
			},
		},
	}
	handler := NewHandler(service)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+projectID.String()+"/tasks/"+taskID.String()+"/dispatch-gates", nil)
	req = req.WithContext(context.WithValue(req.Context(), middleware.TenantIDKey, tenantID))
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, uuid.New()))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("projectId", projectID.String())
	rctx.URLParams.Add("taskId", taskID.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rr := httptest.NewRecorder()

	handler.ListProjectTaskDispatchGates(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &body))
	items := body["items"].([]any)
	require.Len(t, items, 1)
	item := items[0].(map[string]any)
	require.Equal(t, gateID.String(), item["id"])
	require.Equal(t, PreDispatchGateStatusRetryLater, item["status"])
	require.Equal(t, "runtime.node_offline", item["blockers"].([]any)[0].(map[string]any)["key"])
}
```

- [ ] **Step 2: Run test and verify it fails**

Run:

```bash
go test ./apps/control-plane/internal/project -run TestHandlerListProjectTaskDispatchGates -count=1
```

Expected: FAIL because handler method and service fake field do not exist.

- [ ] **Step 3: Add handler mapping**

Add handler service method:

```go
ListPreDispatchGateResults(ctx context.Context, req ListPreDispatchGateResultsRequest) ([]PreDispatchGateResult, error)
```

Add response structs in `handler.go`:

```go
type dispatchGateResponse struct {
	ID                 string                    `json:"id"`
	ProjectTaskID      string                    `json:"project_task_id"`
	AcceptedPlanRevisionID *string              `json:"accepted_plan_revision_id,omitempty"`
	PlannedTaskKey     *string                   `json:"planned_task_key,omitempty"`
	SelectedEmployeeID string                    `json:"selected_employee_id"`
	AttemptNo          int32                     `json:"attempt_no"`
	DispatchReason     string                    `json:"dispatch_reason"`
	Status             string                    `json:"status"`
	CheckedAt          time.Time                 `json:"checked_at"`
	Checks             []PreDispatchGateCheck    `json:"checks"`
	Blockers           []PreDispatchGateBlocker  `json:"blockers"`
	HumanActionRequest map[string]any            `json:"human_action_request"`
	RetryAfter         *time.Time                `json:"retry_after,omitempty"`
	AttemptID          *string                   `json:"attempt_id,omitempty"`
	DecisionRequestID  *string                   `json:"decision_request_id,omitempty"`
}
```

Add handler:

```go
func (h *HTTPHandler) ListProjectTaskDispatchGates(w http.ResponseWriter, r *http.Request) {
	tenantID, _, ok := consoleIdentity(w, r)
	if !ok {
		return
	}
	projectID, ok := projectIDFromRequest(w, r)
	if !ok {
		return
	}
	taskID, ok := taskIDFromRequest(w, r)
	if !ok {
		return
	}
	service, ok := h.serviceFromRequest(w)
	if !ok {
		return
	}
	results, err := service.ListPreDispatchGateResults(r.Context(), ListPreDispatchGateResultsRequest{
		TenantID:      tenantID,
		ProjectID:     projectID,
		ProjectTaskID: taskID,
		Limit:         50,
	})
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": dispatchGateResponses(results)})
}
```

Register in `apps/control-plane/internal/api/server.go`, next to the existing `/projects/{projectId}/plan-revisions` registration inside the `/api/v1` chi route group. chi path params are camelCase (`{projectId}`, `{taskId}`) to match `projectIDFromRequest` / `taskIDFromRequest`:

```go
r.Get("/projects/{projectId}/tasks/{taskId}/dispatch-gates", s.projectHandler.ListProjectTaskDispatchGates)
```

- [ ] **Step 4: Update OpenAPI and generated client**

Add schemas to `contracts/control-plane/openapi.yaml`:

```yaml
DispatchGateResult:
  type: object
  required:
    - id
    - project_task_id
    - selected_employee_id
    - attempt_no
    - dispatch_reason
    - status
    - checked_at
    - checks
    - blockers
    - human_action_request
  properties:
    id:
      type: string
      format: uuid
    project_task_id:
      type: string
      format: uuid
    accepted_plan_revision_id:
      type: string
      format: uuid
      nullable: true
    planned_task_key:
      type: string
      nullable: true
    selected_employee_id:
      type: string
      format: uuid
    attempt_no:
      type: integer
      format: int32
    dispatch_reason:
      type: string
    status:
      type: string
      enum: [passed, waiting_human, blocked, retry_later, replan_required]
    checked_at:
      type: string
      format: date-time
    checks:
      type: array
      items:
        $ref: '#/components/schemas/DispatchGateCheck'
    blockers:
      type: array
      items:
        $ref: '#/components/schemas/DispatchGateBlocker'
    human_action_request:
      type: object
      additionalProperties: true
    retry_after:
      type: string
      format: date-time
      nullable: true
    attempt_id:
      type: string
      format: uuid
      nullable: true
    decision_request_id:
      type: string
      format: uuid
      nullable: true
DispatchGateCheck:
  type: object
  required: [key, status, details]
  properties:
    key:
      type: string
    status:
      type: string
    details:
      type: object
      additionalProperties: true
DispatchGateBlocker:
  type: object
  required: [key, severity, retryable, details]
  properties:
    key:
      type: string
    severity:
      type: string
    retryable:
      type: boolean
    details:
      type: object
      additionalProperties: true
```

Add path:

```yaml
/api/v1/projects/{project_id}/tasks/{task_id}/dispatch-gates:
  get:
    operationId: listProjectTaskDispatchGates
    parameters:
      - name: project_id
        in: path
        required: true
        schema:
          type: string
          format: uuid
      - name: task_id
        in: path
        required: true
        schema:
          type: string
          format: uuid
    responses:
      '200':
        description: Project task dispatch gate results
        content:
          application/json:
            schema:
              type: object
              required: [items]
              properties:
                items:
                  type: array
                  items:
                    $ref: '#/components/schemas/DispatchGateResult'
```

Run:

```bash
corepack pnpm generate:control-plane
```

Expected: generated client files update without schema errors.

- [ ] **Step 5: Add Web API tests and fetcher**

Modify `apps/web/src/lib/api/projects.test.ts`:

```ts
it("lists project task dispatch gates", async () => {
  const fetchMock = vi.fn().mockResolvedValue(
    new Response(
      JSON.stringify({
        items: [
          {
            id: "00000000-0000-0000-0000-000000000401",
            project_task_id: "00000000-0000-0000-0000-000000000402",
            selected_employee_id: "00000000-0000-0000-0000-000000000403",
            attempt_no: 1,
            dispatch_reason: "root_ready",
            status: "retry_later",
            checked_at: "2026-06-21T12:00:00Z",
            checks: [],
            blockers: [{ key: "runtime.node_offline", severity: "transient", retryable: true, details: {} }],
            human_action_request: {},
            retry_after: "2026-06-21T12:02:00Z",
          },
        ],
      }),
      { status: 200, headers: { "Content-Type": "application/json" } },
    ),
  );
  const api = createProjectsApi({ fetch: fetchMock, baseUrl: "http://control-plane.test" });

  const result = await api.listProjectTaskDispatchGates("project-1", "task-1");

  expect(fetchMock).toHaveBeenCalledWith(
    "http://control-plane.test/api/v1/projects/project-1/tasks/task-1/dispatch-gates",
    expect.objectContaining({ method: "GET" }),
  );
  expect(result.items[0].status).toBe("retry_later");
  expect(result.items[0].blockers[0].key).toBe("runtime.node_offline");
});
```

Modify `apps/web/src/lib/api/projects.ts`:

```ts
export type DispatchGateStatus = "passed" | "waiting_human" | "blocked" | "retry_later" | "replan_required";

export interface DispatchGateCheck {
  key: string;
  status: string;
  details: Record<string, unknown>;
}

export interface DispatchGateBlocker {
  key: string;
  severity: string;
  retryable: boolean;
  details: Record<string, unknown>;
}

export interface DispatchGateResult {
  id: string;
  project_task_id: string;
  accepted_plan_revision_id?: string | null;
  planned_task_key?: string | null;
  selected_employee_id: string;
  attempt_no: number;
  dispatch_reason: string;
  status: DispatchGateStatus;
  checked_at: string;
  checks: DispatchGateCheck[];
  blockers: DispatchGateBlocker[];
  human_action_request: Record<string, unknown>;
  retry_after?: string | null;
  attempt_id?: string | null;
  decision_request_id?: string | null;
}

export interface DispatchGateListResponse {
  items: DispatchGateResult[];
}
```

Add fetcher:

```ts
async listProjectTaskDispatchGates(projectId: string, taskId: string): Promise<DispatchGateListResponse> {
  return requestJson<DispatchGateListResponse>(
    `${baseUrl}/api/v1/projects/${encodeURIComponent(projectId)}/tasks/${encodeURIComponent(taskId)}/dispatch-gates`,
    { method: "GET" },
  );
}
```

- [ ] **Step 6: Add Web display test and component slice**

Modify `apps/web/src/features/projects/index.test.tsx`:

```tsx
it("shows the latest pre-dispatch gate status for a selected project task", async () => {
  server.use(
    http.get("*/api/v1/projects/:projectId/tasks/:taskId/dispatch-gates", () =>
      HttpResponse.json({
        items: [
          {
            id: "00000000-0000-0000-0000-000000000501",
            project_task_id: "00000000-0000-0000-0000-000000000502",
            selected_employee_id: "00000000-0000-0000-0000-000000000503",
            attempt_no: 1,
            dispatch_reason: "root_ready",
            status: "retry_later",
            checked_at: "2026-06-21T12:00:00Z",
            checks: [],
            blockers: [{ key: "runtime.node_offline", severity: "transient", retryable: true, details: {} }],
            human_action_request: {},
            retry_after: "2026-06-21T12:02:00Z",
          },
        ],
      }),
    ),
  );

  render(<ProjectsPage />);

  expect(await screen.findByText("Pre-dispatch gate")).toBeInTheDocument();
  expect(await screen.findByText("Retry later")).toBeInTheDocument();
  expect(await screen.findByText("runtime.node_offline")).toBeInTheDocument();
});
```

Modify `apps/web/src/features/projects/components/project-operational-detail.tsx` by adding a compact unframed section near the task timeline:

```tsx
function DispatchGateSummary({ gates }: { gates: DispatchGateResult[] }) {
  const latest = gates[0];
  if (!latest) {
    return null;
  }
  const statusLabel: Record<DispatchGateStatus, string> = {
    passed: "Passed",
    waiting_human: "Waiting human",
    blocked: "Blocked",
    retry_later: "Retry later",
    replan_required: "Replan required",
  };
  return (
    <section className="border-t border-border pt-4">
      <div className="flex items-center justify-between gap-3">
        <h3 className="text-sm font-medium text-foreground">Pre-dispatch gate</h3>
        <span className="rounded border border-border px-2 py-0.5 text-xs text-muted-foreground">
          {statusLabel[latest.status]}
        </span>
      </div>
      {latest.blockers.length > 0 ? (
        <ul className="mt-3 space-y-2">
          {latest.blockers.map((blocker) => (
            <li key={`${latest.id}-${blocker.key}`} className="text-sm text-muted-foreground">
              <span className="font-mono text-xs text-foreground">{blocker.key}</span>
              <span className="ml-2">{blocker.retryable ? "retryable" : blocker.severity}</span>
            </li>
          ))}
        </ul>
      ) : null}
      {latest.retry_after ? (
        <p className="mt-2 text-xs text-muted-foreground">Retry after {formatDateTime(latest.retry_after)}</p>
      ) : null}
    </section>
  );
}
```

Use an existing React Query pattern in the component to fetch gates for the selected task. Keep the layout unframed; do not nest this section inside another card.

- [ ] **Step 7: Run API/Web tests and commit**

Run:

```bash
go test ./apps/control-plane/internal/project -run 'TestHandlerListProjectTaskDispatchGates' -count=1
go test ./apps/control-plane/internal/api -run 'TestProject.*DispatchGate' -count=1
corepack pnpm --filter ./apps/web run test -- projects
```

Expected: PASS.

Commit:

```bash
git add apps/control-plane/internal/project/handler.go apps/control-plane/internal/project/handler_test.go apps/control-plane/internal/api/server.go apps/control-plane/internal/api/project_routes_test.go contracts/control-plane/openapi.yaml apps/web/src/lib/api/projects.ts apps/web/src/lib/api/projects.test.ts apps/web/src/features/projects/components/project-operational-detail.tsx apps/web/src/features/projects/index.test.tsx
git add apps/web/src/lib/api/generated apps/control-plane/internal/api/generated
git commit -m "feat: expose dispatch gate status in console"
```

### Task 9: Full Verification And Live Smoke

**Files:**
- No code files expected.
- Use service scripts and real APIs for verification.

- [ ] **Step 1: Run backend unit and integration tests**

Run:

```bash
go test ./apps/control-plane/internal/project ./apps/control-plane/internal/workflow/projectcoordination ./apps/control-plane/internal/app ./apps/control-plane/internal/api ./apps/control-plane/internal/storage -count=1
```

Expected: PASS.

- [ ] **Step 2: Run Web tests through repo script**

Run:

```bash
corepack pnpm --filter ./apps/web run test
```

Expected: PASS.

- [ ] **Step 3: Run generation/contract checks**

Run:

```bash
corepack pnpm generate:control-plane
git diff --check
```

Expected:

- generation command exits 0
- `git diff --check` prints no whitespace errors

- [ ] **Step 4: Start real local stack**

Run:

```bash
scripts/dev-services.sh status
scripts/dev-services.sh restart control-plane
scripts/dev-services.sh restart web
scripts/dev-services.sh status
```

Expected:

- Temporal, Control Plane, Web, and Runtime Agent status are visible
- restarted services point at current worktree paths

- [ ] **Step 5: Smoke retry-later gate with Runtime unavailable**

Stop only the Runtime Agent:

```bash
scripts/dev-services.sh stop runtime-agent
scripts/dev-services.sh status
```

Trigger a real accepted-plan ProjectTask dispatch through the coordinator using an existing dev project demand fixture or the seeded test project used by local smoke scripts. Then read the latest gate result:

```bash
curl -sS -H "Authorization: Bearer $SUPERTEAM_DEV_TOKEN" \
  "http://127.0.0.1:8080/api/v1/projects/$PROJECT_ID/tasks/$PROJECT_TASK_ID/dispatch-gates" | jq .
```

Expected JSON:

```json
{
  "items": [
    {
      "status": "retry_later",
      "blockers": [
        {
          "key": "runtime.node_offline",
          "retryable": true
        }
      ],
      "attempt_id": null
    }
  ]
}
```

Also verify no Runtime run was created for the blocked task:

```bash
psql "$SUPERTEAM_DATABASE_URL" -c "select count(*) from project_task_attempts where tenant_id = '$TENANT_ID' and project_task_id = '$PROJECT_TASK_ID';"
```

Expected: count is `0` for the retry-later gate that happened before run creation.

- [ ] **Step 6: Smoke passed gate with Runtime available**

Restart Runtime Agent:

```bash
scripts/dev-services.sh restart runtime-agent
scripts/dev-services.sh status
```

Trigger the same dispatch path after the runtime heartbeat is visible. Read the gate results:

```bash
curl -sS -H "Authorization: Bearer $SUPERTEAM_DEV_TOKEN" \
  "http://127.0.0.1:8080/api/v1/projects/$PROJECT_ID/tasks/$PROJECT_TASK_ID/dispatch-gates" | jq '.items[0] | {status, attempt_id, selected_employee_id, dispatch_reason}'
```

Expected JSON:

```json
{
  "status": "passed",
  "attempt_id": "non-empty-uuid",
  "selected_employee_id": "non-empty-uuid",
  "dispatch_reason": "root_ready"
}
```

Verify the attempt is linked:

```bash
psql "$SUPERTEAM_DATABASE_URL" -c "select pta.id, pta.dispatch_gate_result_id, pt.latest_dispatch_gate_result_id from project_task_attempts pta join project_tasks pt on pt.tenant_id = pta.tenant_id and pt.id = pta.project_task_id where pta.tenant_id = '$TENANT_ID' and pta.project_task_id = '$PROJECT_TASK_ID' order by pta.created_at desc limit 1;"
```

Expected: `dispatch_gate_result_id` and `latest_dispatch_gate_result_id` are the same non-null UUID.

- [ ] **Step 7: Browser smoke Web visibility**

Use the Codex Chrome plugin for browser verification. Open the project detail page and inspect the task detail section.

Expected:

- "Pre-dispatch gate" is visible for the task
- latest status matches API result
- blockers are visible for retry-later / waiting-human results
- no overlapping text at desktop and mobile widths

- [ ] **Step 8: Run project completion check**

Before claiming completion, use the project skill:

```bash
sed -n '1,220p' .codex/skills/superteam-completion-check/SKILL.md
```

Follow its checklist. Record the exact verification commands and whether the real Runtime/Provider path was fully tested. If Runtime or Provider is not available, report the task as blocked for real-chain completion and do not call it fully done.

- [ ] **Step 9: Final commit**

Commit verification-only fixes if any:

```bash
git status --short
git add .
git commit -m "test: verify pre-dispatch gate integration"
```

Expected:

- commit only contains intentional Phase 3 files
- no unrelated dirty root-worktree files are included

## Spec Coverage Self-Review

- Gate trigger before Runtime run: Tasks 4 and 5.
- State, dependency, employee, capability, permission/tool, Runtime, budget, risk, context checks: Tasks 1, 4, and 7.
- Durable gate result and audit facts: Tasks 2 and 3.
- Human interaction request creation and idempotent re-entry: Tasks 4 and 6.
- Gate and dispatch idempotency: Tasks 2, 3, and 5.
- Boundary between Control Plane and Runtime: Tasks 5 and 9 keep Runtime untouched.
- Events, timeline, API, and Web visibility: Tasks 2, 4, and 8.
- Unit, integration, and real smoke verification: Task 9.

No Phase 4 result-contract release logic is included. No Runtime Agent policy decision is included.
