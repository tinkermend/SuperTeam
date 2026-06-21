# Dynamic Project Planning Phase 2 PlanRevision Decomposition Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add versioned `PlanRevision` as the durable planning object, route human review to a specific revision, and decompose only accepted revisions into a ProjectTask DAG with exact-once semantics.

**Architecture:** Keep planning orchestration in the Control Plane. Reuse the existing Phase 1 `RouteDecisionPlan` and planning-profile validation as the semantic planner output, persist a canonical `project_plan_revisions` record as the business fact, and move ProjectTask creation behind plan acceptance. Keep Runtime and Provider untouched in Phase 2.

**Tech Stack:** Go Control Plane, PostgreSQL migrations, sqlc, Temporal workflow/activity package `projectcoordination`, existing `project_tasks` graph writer, OpenAPI contract generation, React Query project console, Vitest.

---

## Execution Preflight

Use an isolated worktree for implementation because this phase touches migrations, generated sqlc, workflow tests, API contracts, and Web tests.

```bash
git status --short
git worktree add ../SuperTeam-dynamic-planning-phase-2 -b codex/dynamic-planning-phase-2
cd ../SuperTeam-dynamic-planning-phase-2
git status --short
```

Expected:

- root checkout may have unrelated user work
- implementation worktree should be clean
- branch should be `codex/dynamic-planning-phase-2`

Current code facts to preserve:

- `apps/control-plane/internal/workflow/projectcoordination/workflow.go` currently creates ProjectTasks before route review. Phase 2 must change this so `pending_review` revisions create no ProjectTasks.
- `apps/control-plane/internal/workflow/projectcoordination/project_store.go` already has `CreateProjectTasks` calling `repository.DecomposeAcceptedPlanRevision`, but it derives `AcceptedPlanRevisionID` from `RouteDecisionID`. Phase 2 must pass the persisted `PlanRevision.ID`.
- `apps/control-plane/internal/project/pg_repository.go` already replays tasks by `accepted_plan_revision_id`. Phase 2 should keep this graph replay behavior and add a real `project_plan_decomposition_claims` table around it.
- `apps/control-plane/internal/storage/migrations/024_project_task_attempts.sql` already adds `project_tasks.accepted_plan_revision_id`, `decomposition_claim_key`, and `uq_project_tasks_accepted_plan_decomposition`.

## File Structure

- Create `apps/control-plane/internal/project/plan_revision.go`: domain types, statuses, review actions, state-transition helpers, and request/result structs.
- Modify `apps/control-plane/internal/project/types.go`: add `PlanRevision`, `PlanDecompositionClaim`, and add plan contract fields to `ProjectTask` only if they are not already exposed through `PlannerMetadata`.
- Modify `apps/control-plane/internal/project/repository.go`: add repository methods for plan revision creation, listing, status transitions, review, and decomposition claims.
- Modify `apps/control-plane/internal/project/pg_repository.go`: implement plan revision and claim persistence in the existing project repository transaction style.
- Modify `apps/control-plane/internal/project/pg_repository_test.go`: add database-backed tests for revision uniqueness, state transitions, supersede behavior, and exact-once decomposition claims.
- Create `apps/control-plane/internal/storage/migrations/031_project_plan_revisions.sql`: create `project_plan_revisions` and `project_plan_decomposition_claims`.
- Modify `apps/control-plane/internal/storage/migrations/atlas.sum`: update through the repo migration workflow after adding migration `031`.
- Modify `apps/control-plane/internal/storage/migrations_test.go`: assert tenant-first indexes, status checks, uniqueness, and comments for the new migration.
- Modify `apps/control-plane/internal/storage/queries/project.sql`: add sqlc queries for plan revisions and decomposition claims.
- Regenerate `apps/control-plane/internal/storage/queries/*.go` with the repo generator.
- Create `apps/control-plane/internal/workflow/projectcoordination/plan_revision_payload.go`: convert `RouteDecisionPlan` into canonical `PlanRevisionPayload`, fingerprint payloads, validate payload-level plan rules, and convert accepted payload back to graph tasks.
- Create `apps/control-plane/internal/workflow/projectcoordination/plan_revision_payload_test.go`: cover canonical hash, duplicate keys, dependency cycles, high-risk review requirements, and final summary contract validation.
- Modify `apps/control-plane/internal/workflow/projectcoordination/types.go`: replace route-review inputs with plan-revision review and add `PlanRevisionID` to task decomposition input.
- Modify `apps/control-plane/internal/workflow/projectcoordination/activities.go`: add activity methods for persisting plan revisions, requesting plan review, accepting/rejecting/requesting changes, and decomposing accepted revision.
- Modify `apps/control-plane/internal/workflow/projectcoordination/project_store.go`: persist plan revisions, create approval/decision records for `plan_review`, and call repository decomposition with the real revision ID.
- Modify `apps/control-plane/internal/workflow/projectcoordination/project_store_test.go`: prove pending review creates no tasks, accepted revision creates tasks, retry replays tasks, and request changes supersedes the old revision.
- Modify `apps/control-plane/internal/workflow/projectcoordination/workflow.go`: move `CreateProjectTasks` behind accepted plan flow and replan on request changes.
- Modify `apps/control-plane/internal/workflow/projectcoordination/workflow_test.go`: update workflow tests for pending review, approval, rejection, request changes, and automatic acceptance.
- Modify `apps/control-plane/internal/project/service.go`: expose plan revision read methods and keep human decision resolution signaling the coordinator.
- Modify `apps/control-plane/internal/project/handler.go`: add list/get plan revision endpoints and response mapping.
- Modify `apps/control-plane/internal/api/server.go`: register project plan revision routes.
- Modify `apps/control-plane/internal/project/handler_test.go` and `apps/control-plane/internal/api/project_routes_test.go`: cover route registration and response shape.
- Modify `contracts/control-plane/openapi.yaml`: add plan revision schemas and routes.
- Modify generated client files produced by `corepack pnpm generate:control-plane`.
- Modify `apps/web/src/lib/api/projects.ts` and `apps/web/src/lib/api/projects.test.ts`: add plan revision API types and fetchers.
- Modify `apps/web/src/features/projects/index.tsx`: fetch plan revisions and invalidate them after plan review decisions.
- Modify `apps/web/src/features/projects/components/project-operational-detail.tsx`: show latest plan revision summary, DAG tasks, capabilities, risks, and pending review actions using the existing decision resolution path.
- Modify `apps/web/src/features/projects/index.test.tsx`: verify the plan review panel and action calls.

Do not modify Runtime Agent, Provider adapters, PreDispatchGate, TaskResultContract, or full graph editing in Phase 2.

### Task 1: Domain Status Model And Payload Canonicalization

**Files:**
- Create: `apps/control-plane/internal/project/plan_revision.go`
- Create: `apps/control-plane/internal/workflow/projectcoordination/plan_revision_payload.go`
- Create: `apps/control-plane/internal/workflow/projectcoordination/plan_revision_payload_test.go`
- Modify: `apps/control-plane/internal/project/types.go`

- [ ] **Step 1: Write failing payload tests**

Create `apps/control-plane/internal/workflow/projectcoordination/plan_revision_payload_test.go`:

```go
package projectcoordination

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestBuildPlanRevisionPayloadCanonicalFingerprintIsStable(t *testing.T) {
	employeeID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	plan := RouteDecisionPlan{
		Reason:              "检查数据异常并汇总结论",
		RequiresHumanReview: false,
		PlannerMetadata:     map[string]any{"planner_model": "gpt-test", "generated_at": "ignored"},
		Tasks: []PlannedTask{
			{
				Key:                       "inspect-db",
				Title:                     "检查数据库异常数据",
				Summary:                   "读取开发库并定位异常数据范围",
				TaskKind:                  "database_analysis",
				SelectedEmployeeID:        employeeID,
				EmployeeSelectionReason:   "具备 database.read 与 sql.analysis 能力",
				RequiredCapabilities:      []string{"database.read", "sql.analysis"},
				MatchedCapabilities:       []string{"sql.analysis", "database.read"},
				PermissionRequirements:    []string{"database.read:dev_database"},
				ToolRequirements:          []string{"mcp:postgres.readonly"},
				RuntimeRequirements:       []string{"provider:codex"},
				ExpectedOutputs:           []string{"异常范围", "证据 SQL", "风险说明"},
				VerificationRequirements:  []string{"只读 SQL 结果截图"},
				HandoffContract:           map[string]any{"acceptance_criteria": []any{"列出异常范围"}},
				PlanningProfileSnapshotHash: "profile-hash",
			},
		},
	}

	payload := BuildPlanRevisionPayload(plan)
	first, err := CanonicalPlanFingerprint(payload)
	require.NoError(t, err)
	second, err := CanonicalPlanFingerprint(payload)
	require.NoError(t, err)

	require.Len(t, first, 64)
	require.Equal(t, first, second)
	require.Equal(t, "检查数据异常并汇总结论", payload.Summary)
	require.Equal(t, []string{"conclusion", "evidence", "risks", "next_steps"}, payload.FinalSummaryContract.RequiredSections)
	require.Equal(t, "inspect-db", payload.Tasks[0].PlannedTaskKey)
	require.Equal(t, "具备 database.read 与 sql.analysis 能力", payload.Tasks[0].EmployeeSelectionReason)
}

func TestValidatePlanRevisionPayloadRejectsDuplicateAndDanglingDependencies(t *testing.T) {
	employeeID := uuid.New()
	payload := PlanRevisionPayload{
		Summary: "重复任务键",
		Tasks: []PlanRevisionTask{
			{
				PlannedTaskKey:          "inspect",
				Title:                   "检查",
				Objective:               "检查输入",
				TaskType:                "analysis",
				SelectedEmployeeID:      employeeID.String(),
				EmployeeSelectionReason: "具备分析能力",
				RequiredCapabilities:    []string{"codebase.analysis"},
				MatchedCapabilities:     []string{"codebase.analysis"},
				ExpectedOutputs:         []string{"结论"},
				AcceptanceCriteria:      []string{"结论可复核"},
			},
			{
				PlannedTaskKey:          "inspect",
				Title:                   "复查",
				Objective:               "复查输入",
				TaskType:                "analysis",
				SelectedEmployeeID:      employeeID.String(),
				EmployeeSelectionReason: "具备分析能力",
				RequiredCapabilities:    []string{"codebase.analysis"},
				MatchedCapabilities:     []string{"codebase.analysis"},
				ExpectedOutputs:         []string{"复查结论"},
				AcceptanceCriteria:      []string{"复查结论可复核"},
				DependsOn:               []string{"missing"},
			},
		},
		FinalSummaryContract: PlanRevisionFinalSummaryContract{RequiredSections: []string{"conclusion", "evidence", "risks", "next_steps"}},
	}

	result := ValidatePlanRevisionPayload(payload)

	require.Contains(t, result.Errors, "duplicate_planned_task_key:inspect")
	require.Contains(t, result.Errors, "unknown_dependency:inspect->missing")
	require.False(t, result.Acceptable)
}

func TestValidatePlanRevisionPayloadRequiresReviewForHighRiskTask(t *testing.T) {
	employeeID := uuid.New()
	payload := PlanRevisionPayload{
		Summary: "执行数据库迁移",
		Tasks: []PlanRevisionTask{
			{
				PlannedTaskKey:          "migrate-db",
				Title:                   "执行数据库迁移",
				Objective:               "创建新表",
				TaskType:                "database_migration",
				SelectedEmployeeID:      employeeID.String(),
				EmployeeSelectionReason: "具备迁移能力",
				RequiredCapabilities:    []string{"database.migration"},
				MatchedCapabilities:     []string{"database.migration"},
				ExpectedOutputs:         []string{"迁移结果"},
				AcceptanceCriteria:      []string{"迁移状态为 applied"},
				RiskLevel:               "high",
			},
		},
		FinalSummaryContract: PlanRevisionFinalSummaryContract{RequiredSections: []string{"conclusion", "evidence", "risks", "next_steps"}},
	}

	result := ValidatePlanRevisionPayload(payload)

	require.Empty(t, result.Errors)
	require.True(t, result.Acceptable)
	require.True(t, result.ReviewRequired)
	require.Contains(t, result.ReviewReasons, "high_risk_task:migrate-db")
}
```

- [ ] **Step 2: Run tests and verify they fail**

Run:

```bash
go test ./apps/control-plane/internal/workflow/projectcoordination -run 'TestBuildPlanRevisionPayload|TestValidatePlanRevisionPayload' -count=1
```

Expected: FAIL with undefined symbols `BuildPlanRevisionPayload`, `CanonicalPlanFingerprint`, `PlanRevisionPayload`, and `ValidatePlanRevisionPayload`.

- [ ] **Step 3: Add domain status model**

Create `apps/control-plane/internal/project/plan_revision.go`:

```go
package project

import (
	"time"

	"github.com/google/uuid"
)

const (
	PlanRevisionStatusDraft            = "draft"
	PlanRevisionStatusValidationFailed = "validation_failed"
	PlanRevisionStatusPendingReview    = "pending_review"
	PlanRevisionStatusAccepted         = "accepted"
	PlanRevisionStatusRejected         = "rejected"
	PlanRevisionStatusSuperseded       = "superseded"
	PlanRevisionStatusDecomposing      = "decomposing"
	PlanRevisionStatusDecomposed       = "decomposed"
)

const (
	PlanReviewDecisionAccept         = "approved"
	PlanReviewDecisionReject         = "rejected"
	PlanReviewDecisionRequestChanges = "request_changes"
	PlanReviewDecisionCancel         = "cancelled"
)

type PlanRevision struct {
	ID                       uuid.UUID
	TenantID                 uuid.UUID
	TeamID                   *uuid.UUID
	ProjectID                uuid.UUID
	DemandID                 uuid.UUID
	CoordinationJobID         *uuid.UUID
	RouteDecisionID           *uuid.UUID
	RevisionNumber            int32
	Status                   string
	Payload                  map[string]any
	PlannerProvider          *string
	PlannerModel             *string
	PlannerInputHash         *string
	PlanFingerprint          string
	ValidationErrors         []string
	ValidationWarnings       []string
	ReviewRequired           bool
	ReviewReason             *string
	AcceptedBy               *uuid.UUID
	AcceptedAt               *time.Time
	RejectedBy               *uuid.UUID
	RejectedAt               *time.Time
	RejectionReason          *string
	SupersededByRevisionID    *uuid.UUID
	DecompositionClaimID      *uuid.UUID
	CreatedTaskIDs           []uuid.UUID
	CreatedAt                time.Time
	UpdatedAt                time.Time
}

type PlanDecompositionClaim struct {
	ID                     uuid.UUID
	TenantID               uuid.UUID
	ProjectID              uuid.UUID
	DemandID               uuid.UUID
	AcceptedPlanRevisionID uuid.UUID
	PlanFingerprint        string
	Status                 string
	CreatedTaskIDs         []uuid.UUID
	Error                  map[string]any
	CreatedAt              time.Time
	UpdatedAt              time.Time
}

func CanTransitionPlanRevisionStatus(from, to string) bool {
	allowed := map[string]map[string]bool{
		PlanRevisionStatusDraft: {
			PlanRevisionStatusValidationFailed: true,
			PlanRevisionStatusPendingReview:    true,
			PlanRevisionStatusAccepted:         true,
		},
		PlanRevisionStatusValidationFailed: {
			PlanRevisionStatusSuperseded: true,
		},
		PlanRevisionStatusPendingReview: {
			PlanRevisionStatusAccepted:   true,
			PlanRevisionStatusRejected:   true,
			PlanRevisionStatusSuperseded: true,
		},
		PlanRevisionStatusAccepted: {
			PlanRevisionStatusDecomposing: true,
		},
		PlanRevisionStatusDecomposing: {
			PlanRevisionStatusDecomposed: true,
		},
		PlanRevisionStatusDecomposed: {
			// self-loop: the exact-once replay branch re-marks an already-decomposed
			// revision via MarkProjectPlanRevisionDecomposed; keep the domain guard
			// aligned with the SQL WHERE status IN ('decomposing','decomposed').
			PlanRevisionStatusDecomposed: true,
		},
	}
	return allowed[from][to]
}

func IsAcceptedPlanRevisionStatus(status string) bool {
	return status == PlanRevisionStatusAccepted ||
		status == PlanRevisionStatusDecomposing ||
		status == PlanRevisionStatusDecomposed
}

func IsMutablePlanRevisionStatus(status string) bool {
	return status == PlanRevisionStatusDraft ||
		status == PlanRevisionStatusValidationFailed ||
		status == PlanRevisionStatusPendingReview
}
```

- [ ] **Step 4: Add payload conversion and validation**

Create `apps/control-plane/internal/workflow/projectcoordination/plan_revision_payload.go`:

```go
package projectcoordination

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strings"

	"github.com/google/uuid"
)

type PlanRevisionPayload struct {
	Summary              string                           `json:"summary"`
	Assumptions          []string                         `json:"assumptions"`
	RiskAssessment       PlanRevisionRiskAssessment       `json:"risk_assessment"`
	HumanReview          PlanRevisionHumanReview          `json:"human_review"`
	Tasks                []PlanRevisionTask               `json:"tasks"`
	FinalSummaryContract PlanRevisionFinalSummaryContract `json:"final_summary_contract"`
}

type PlanRevisionRiskAssessment struct {
	Level   string   `json:"level"`
	Reasons []string `json:"reasons"`
}

type PlanRevisionHumanReview struct {
	Required bool     `json:"required"`
	Reasons  []string `json:"reasons"`
}

type PlanRevisionTask struct {
	PlannedTaskKey             string         `json:"planned_task_key"`
	Title                      string         `json:"title"`
	Objective                  string         `json:"objective"`
	TaskType                   string         `json:"task_type"`
	SelectedEmployeeID         string         `json:"selected_employee_id"`
	EmployeeSelectionReason    string         `json:"employee_selection_reason"`
	RequiredCapabilities       []string       `json:"required_capabilities"`
	MatchedCapabilities        []string       `json:"matched_capabilities"`
	MissingCapabilities        []string       `json:"missing_capabilities"`
	PermissionRequirements     []string       `json:"permission_requirements"`
	ToolRequirements           []string       `json:"tool_requirements"`
	RuntimeRequirements        []string       `json:"runtime_requirements"`
	InputContextRefs           []string       `json:"input_context_refs"`
	ExpectedOutputs            []string       `json:"expected_outputs"`
	AcceptanceCriteria         []string       `json:"acceptance_criteria"`
	VerificationRequirements   []string       `json:"verification_requirements"`
	DependsOn                  []string       `json:"depends_on"`
	RiskLevel                  string         `json:"risk_level"`
	HumanReviewRequired        bool           `json:"human_review_required"`
	SelectionScore             int            `json:"selection_score,omitempty"`
	PlanningProfileSnapshotHash string        `json:"planning_profile_snapshot_hash,omitempty"`
	HandoffContract            map[string]any `json:"handoff_contract,omitempty"`
}

type PlanRevisionFinalSummaryContract struct {
	RequiredSections []string `json:"required_sections"`
}

type PlanRevisionValidationResult struct {
	Acceptable      bool
	Errors          []string
	Warnings        []string
	ReviewRequired  bool
	ReviewReasons   []string
	PlanFingerprint string
}

func BuildPlanRevisionPayload(plan RouteDecisionPlan) PlanRevisionPayload {
	tasks := make([]PlanRevisionTask, 0, len(plan.Tasks))
	reviewReasons := []string{}
	for _, task := range plan.Tasks {
		acceptanceCriteria := stringsFromAnySlice(task.HandoffContract["acceptance_criteria"])
		if len(acceptanceCriteria) == 0 {
			acceptanceCriteria = append([]string{}, task.ExpectedOutputs...)
		}
		if task.RequiresHumanApproval {
			reviewReasons = append(reviewReasons, "task_requires_human_approval:"+task.Key)
		}
		tasks = append(tasks, PlanRevisionTask{
			PlannedTaskKey:              task.Key,
			Title:                       task.Title,
			Objective:                   firstNonEmpty(task.Summary, task.Title),
			TaskType:                    task.TaskKind,
			SelectedEmployeeID:          task.SelectedEmployeeID.String(),
			EmployeeSelectionReason:     task.EmployeeSelectionReason,
			RequiredCapabilities:        sortedStrings(task.RequiredCapabilities),
			MatchedCapabilities:         sortedStrings(task.MatchedCapabilities),
			MissingCapabilities:         sortedStrings(task.MissingCapabilities),
			PermissionRequirements:      sortedStrings(task.PermissionRequirements),
			ToolRequirements:            sortedStrings(task.ToolRequirements),
			RuntimeRequirements:         sortedStrings(task.RuntimeRequirements),
			InputContextRefs:            keysFromMap(task.InputRequirements),
			ExpectedOutputs:             append([]string{}, task.ExpectedOutputs...),
			AcceptanceCriteria:          acceptanceCriteria,
			VerificationRequirements:    append([]string{}, task.VerificationRequirements...),
			DependsOn:                   append([]string{}, task.BlockedByKeys...),
			RiskLevel:                   firstNonEmpty(task.RiskLevel, "low"),
			HumanReviewRequired:         task.RequiresHumanApproval,
			SelectionScore:              task.SelectionScore,
			PlanningProfileSnapshotHash: task.PlanningProfileSnapshotHash,
			HandoffContract:             cloneAnyMap(task.HandoffContract),
		})
	}
	if plan.RequiresHumanReview {
		reviewReasons = append(reviewReasons, "plan_requires_human_review")
	}
	return PlanRevisionPayload{
		Summary:     strings.TrimSpace(plan.Reason),
		Assumptions: []string{},
		RiskAssessment: PlanRevisionRiskAssessment{
			Level:   planRiskLevel(tasks),
			Reasons: planRiskReasons(tasks),
		},
		HumanReview: PlanRevisionHumanReview{
			Required: plan.RequiresHumanReview,
			Reasons:  reviewReasons,
		},
		Tasks: tasks,
		FinalSummaryContract: PlanRevisionFinalSummaryContract{
			RequiredSections: []string{"conclusion", "evidence", "risks", "next_steps"},
		},
	}
}

func CanonicalPlanFingerprint(payload PlanRevisionPayload) (string, error) {
	canonical := payload
	canonical.Tasks = append([]PlanRevisionTask{}, payload.Tasks...)
	sort.SliceStable(canonical.Tasks, func(i, j int) bool {
		return canonical.Tasks[i].PlannedTaskKey < canonical.Tasks[j].PlannedTaskKey
	})
	for i := range canonical.Tasks {
		canonical.Tasks[i].DependsOn = sortedStrings(canonical.Tasks[i].DependsOn)
		canonical.Tasks[i].RequiredCapabilities = sortedStrings(canonical.Tasks[i].RequiredCapabilities)
		canonical.Tasks[i].MatchedCapabilities = sortedStrings(canonical.Tasks[i].MatchedCapabilities)
		canonical.Tasks[i].MissingCapabilities = sortedStrings(canonical.Tasks[i].MissingCapabilities)
		canonical.Tasks[i].PermissionRequirements = sortedStrings(canonical.Tasks[i].PermissionRequirements)
		canonical.Tasks[i].ToolRequirements = sortedStrings(canonical.Tasks[i].ToolRequirements)
		canonical.Tasks[i].RuntimeRequirements = sortedStrings(canonical.Tasks[i].RuntimeRequirements)
		canonical.Tasks[i].InputContextRefs = sortedStrings(canonical.Tasks[i].InputContextRefs)
		canonical.Tasks[i].VerificationRequirements = sortedStrings(canonical.Tasks[i].VerificationRequirements)
	}
	bytes, err := json.Marshal(canonical)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(bytes)
	return hex.EncodeToString(sum[:]), nil
}

func ValidatePlanRevisionPayload(payload PlanRevisionPayload) PlanRevisionValidationResult {
	result := PlanRevisionValidationResult{Acceptable: true}
	if strings.TrimSpace(payload.Summary) == "" {
		result.Errors = append(result.Errors, "missing_summary")
	}
	if len(payload.Tasks) == 0 {
		result.Errors = append(result.Errors, "missing_tasks")
	}
	if len(payload.FinalSummaryContract.RequiredSections) == 0 {
		result.Errors = append(result.Errors, "missing_final_summary_contract")
	}
	seen := map[string]bool{}
	for _, task := range payload.Tasks {
		key := strings.TrimSpace(task.PlannedTaskKey)
		if key == "" {
			result.Errors = append(result.Errors, "missing_planned_task_key")
			continue
		}
		if key != task.PlannedTaskKey {
			result.Errors = append(result.Errors, "invalid_planned_task_key:"+task.PlannedTaskKey)
		}
		if seen[key] {
			result.Errors = append(result.Errors, "duplicate_planned_task_key:"+key)
		}
		seen[key] = true
		if strings.TrimSpace(task.Title) == "" {
			result.Errors = append(result.Errors, "missing_title:"+key)
		}
		if strings.TrimSpace(task.Objective) == "" {
			result.Errors = append(result.Errors, "missing_objective:"+key)
		}
		if _, err := uuid.Parse(task.SelectedEmployeeID); err != nil {
			result.Errors = append(result.Errors, "invalid_selected_employee_id:"+key)
		}
		if strings.TrimSpace(task.EmployeeSelectionReason) == "" {
			result.Errors = append(result.Errors, "missing_employee_selection_reason:"+key)
		}
		if len(task.ExpectedOutputs) == 0 {
			result.Errors = append(result.Errors, "missing_expected_outputs:"+key)
		}
		if len(task.AcceptanceCriteria) == 0 {
			result.Errors = append(result.Errors, "missing_acceptance_criteria:"+key)
		}
		if isHighRiskLevel(task.RiskLevel) && !task.HumanReviewRequired {
			result.ReviewRequired = true
			result.ReviewReasons = append(result.ReviewReasons, "high_risk_task:"+key)
		}
	}
	rootCount := 0
	for _, task := range payload.Tasks {
		if len(task.DependsOn) == 0 {
			rootCount++
		}
		for _, dependency := range task.DependsOn {
			if !seen[dependency] {
				result.Errors = append(result.Errors, "unknown_dependency:"+task.PlannedTaskKey+"->"+dependency)
			}
		}
	}
	if len(payload.Tasks) > 0 && rootCount == 0 {
		result.Errors = append(result.Errors, "missing_root_task")
	}
	if hasPlanRevisionCycle(payload.Tasks) {
		result.Errors = append(result.Errors, "dependency_cycle")
	}
	fingerprint, err := CanonicalPlanFingerprint(payload)
	if err != nil {
		result.Errors = append(result.Errors, "fingerprint_error:"+err.Error())
	}
	result.PlanFingerprint = fingerprint
	result.ReviewRequired = result.ReviewRequired || payload.HumanReview.Required
	result.ReviewReasons = append(result.ReviewReasons, payload.HumanReview.Reasons...)
	result.Acceptable = len(result.Errors) == 0
	return result
}

func isHighRiskLevel(level string) bool {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "high", "critical":
		return true
	default:
		return false
	}
}
```

Add helper functions in the same file:

```go
func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func sortedStrings(values []string) []string {
	out := append([]string{}, values...)
	sort.Strings(out)
	return out
}

func stringsFromAnySlice(value any) []string {
	raw, ok := value.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		if text, ok := item.(string); ok && strings.TrimSpace(text) != "" {
			out = append(out, strings.TrimSpace(text))
		}
	}
	return out
}

func keysFromMap(values map[string]any) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		if strings.TrimSpace(key) != "" {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	return keys
}

func planRiskLevel(tasks []PlanRevisionTask) string {
	for _, task := range tasks {
		if strings.EqualFold(task.RiskLevel, "critical") {
			return "critical"
		}
	}
	for _, task := range tasks {
		if strings.EqualFold(task.RiskLevel, "high") {
			return "high"
		}
	}
	for _, task := range tasks {
		if strings.EqualFold(task.RiskLevel, "medium") {
			return "medium"
		}
	}
	return "low"
}

func planRiskReasons(tasks []PlanRevisionTask) []string {
	reasons := []string{}
	for _, task := range tasks {
		if isHighRiskLevel(task.RiskLevel) {
			reasons = append(reasons, task.PlannedTaskKey+":"+task.RiskLevel)
		}
	}
	return reasons
}

func hasPlanRevisionCycle(tasks []PlanRevisionTask) bool {
	graph := map[string][]string{}
	for _, task := range tasks {
		graph[task.PlannedTaskKey] = append([]string{}, task.DependsOn...)
	}
	visiting := map[string]bool{}
	visited := map[string]bool{}
	var visit func(string) bool
	visit = func(key string) bool {
		if visiting[key] {
			return true
		}
		if visited[key] {
			return false
		}
		visiting[key] = true
		for _, dependency := range graph[key] {
			if visit(dependency) {
				return true
			}
		}
		visiting[key] = false
		visited[key] = true
		return false
	}
	for key := range graph {
		if visit(key) {
			return true
		}
	}
	return false
}
```

- [ ] **Step 5: Run tests and verify they pass**

Run:

```bash
go test ./apps/control-plane/internal/workflow/projectcoordination -run 'TestBuildPlanRevisionPayload|TestValidatePlanRevisionPayload' -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add apps/control-plane/internal/project/plan_revision.go apps/control-plane/internal/workflow/projectcoordination/plan_revision_payload.go apps/control-plane/internal/workflow/projectcoordination/plan_revision_payload_test.go apps/control-plane/internal/project/types.go
git commit -m "feat: add plan revision domain model"
```

### Task 2: Database Migration And sqlc Queries

**Files:**
- Create: `apps/control-plane/internal/storage/migrations/031_project_plan_revisions.sql`
- Modify: `apps/control-plane/internal/storage/migrations/atlas.sum`
- Modify: `apps/control-plane/internal/storage/migrations_test.go`
- Modify: `apps/control-plane/internal/storage/queries/project.sql`
- Generated: `apps/control-plane/internal/storage/queries/project.sql.go`
- Generated: `apps/control-plane/internal/storage/queries/querier.go`

- [ ] **Step 1: Write failing migration tests**

Append to `apps/control-plane/internal/storage/migrations_test.go`:

```go
func TestProjectPlanRevisionsMigrationHasTenantFirstIndexes(t *testing.T) {
	migration := readMigrationFile(t, "031_project_plan_revisions.sql")

	require.Contains(t, migration, "CREATE TABLE project_plan_revisions")
	require.Contains(t, migration, "CREATE TABLE project_plan_decomposition_claims")
	require.Contains(t, migration, "tenant_id UUID NOT NULL")
	require.Contains(t, migration, "CREATE UNIQUE INDEX uq_project_plan_revisions_revision_number")
	require.Contains(t, migration, "ON project_plan_revisions(tenant_id, project_id, demand_id, revision_number)")
	require.Contains(t, migration, "CREATE UNIQUE INDEX uq_project_plan_revisions_fingerprint")
	require.Contains(t, migration, "ON project_plan_revisions(tenant_id, project_id, demand_id, plan_fingerprint)")
	require.Contains(t, migration, "CREATE UNIQUE INDEX uq_project_plan_revisions_current_accepted")
	require.Contains(t, migration, "CREATE UNIQUE INDEX uq_project_plan_decomposition_claims_revision")
	require.Contains(t, migration, "ON project_plan_decomposition_claims(tenant_id, project_id, demand_id, accepted_plan_revision_id)")
	require.Contains(t, migration, "COMMENT ON TABLE project_plan_revisions")
	require.Contains(t, migration, "COMMENT ON COLUMN project_plan_revisions.payload")
}
```

- [ ] **Step 2: Run migration test and verify it fails**

Run:

```bash
go test ./apps/control-plane/internal/storage -run TestProjectPlanRevisionsMigrationHasTenantFirstIndexes -count=1
```

Expected: FAIL because migration `031_project_plan_revisions.sql` does not exist.

- [ ] **Step 3: Add migration**

Create `apps/control-plane/internal/storage/migrations/031_project_plan_revisions.sql`:

```sql
CREATE TABLE project_plan_revisions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    team_id UUID,
    project_id UUID NOT NULL,
    demand_id UUID NOT NULL,
    coordination_job_id UUID,
    route_decision_id UUID,
    revision_number INTEGER NOT NULL,
    status VARCHAR(32) NOT NULL,
    payload JSONB NOT NULL DEFAULT '{}'::jsonb,
    planner_provider VARCHAR(120),
    planner_model VARCHAR(180),
    planner_input_hash VARCHAR(128),
    plan_fingerprint VARCHAR(128) NOT NULL,
    validation_errors JSONB NOT NULL DEFAULT '[]'::jsonb,
    validation_warnings JSONB NOT NULL DEFAULT '[]'::jsonb,
    review_required BOOLEAN NOT NULL DEFAULT false,
    review_reason TEXT,
    accepted_by UUID,
    accepted_at TIMESTAMPTZ,
    rejected_by UUID,
    rejected_at TIMESTAMPTZ,
    rejection_reason TEXT,
    superseded_by_revision_id UUID,
    decomposition_claim_id UUID,
    created_task_ids UUID[] NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_project_plan_revisions_status CHECK (
        status IN (
            'draft',
            'validation_failed',
            'pending_review',
            'accepted',
            'rejected',
            'superseded',
            'decomposing',
            'decomposed'
        )
    )
);

CREATE UNIQUE INDEX uq_project_plan_revisions_revision_number
    ON project_plan_revisions(tenant_id, project_id, demand_id, revision_number);

CREATE UNIQUE INDEX uq_project_plan_revisions_fingerprint
    ON project_plan_revisions(tenant_id, project_id, demand_id, plan_fingerprint);

CREATE UNIQUE INDEX uq_project_plan_revisions_current_accepted
    ON project_plan_revisions(tenant_id, project_id, demand_id)
    WHERE status IN ('accepted', 'decomposing', 'decomposed');

CREATE INDEX idx_project_plan_revisions_project_status
    ON project_plan_revisions(tenant_id, project_id, status, created_at DESC);

CREATE INDEX idx_project_plan_revisions_demand_created
    ON project_plan_revisions(tenant_id, project_id, demand_id, created_at DESC);

CREATE TRIGGER update_project_plan_revisions_updated_at
    BEFORE UPDATE ON project_plan_revisions
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TABLE project_plan_decomposition_claims (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    project_id UUID NOT NULL,
    demand_id UUID NOT NULL,
    accepted_plan_revision_id UUID NOT NULL,
    plan_fingerprint VARCHAR(128) NOT NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'in_flight',
    created_task_ids UUID[] NOT NULL DEFAULT '{}',
    error JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_project_plan_decomposition_claims_status CHECK (
        status IN ('in_flight', 'completed', 'failed')
    )
);

CREATE UNIQUE INDEX uq_project_plan_decomposition_claims_revision
    ON project_plan_decomposition_claims(tenant_id, project_id, demand_id, accepted_plan_revision_id);

CREATE INDEX idx_project_plan_decomposition_claims_status
    ON project_plan_decomposition_claims(tenant_id, project_id, status, created_at DESC);

CREATE TRIGGER update_project_plan_decomposition_claims_updated_at
    BEFORE UPDATE ON project_plan_decomposition_claims
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

COMMENT ON TABLE project_plan_revisions IS '项目计划版本表，保存 planner 生成并经服务端校验的人类可审核计划版本。';
COMMENT ON COLUMN project_plan_revisions.id IS '计划版本ID。';
COMMENT ON COLUMN project_plan_revisions.tenant_id IS '租户ID，所有查询必须以租户隔离。';
COMMENT ON COLUMN project_plan_revisions.team_id IS '项目所属团队ID；历史项目可为空，由应用层从项目校验。';
COMMENT ON COLUMN project_plan_revisions.project_id IS '计划版本所属项目ID。';
COMMENT ON COLUMN project_plan_revisions.demand_id IS '计划版本关联的用户需求或触发事件ID。';
COMMENT ON COLUMN project_plan_revisions.coordination_job_id IS '生成该计划版本的项目协调作业ID。';
COMMENT ON COLUMN project_plan_revisions.route_decision_id IS '兼容现有路由决策读模型的关联决策ID。';
COMMENT ON COLUMN project_plan_revisions.revision_number IS '同一项目需求下从 1 开始递增的计划版本号。';
COMMENT ON COLUMN project_plan_revisions.status IS '计划版本状态：draft, validation_failed, pending_review, accepted, rejected, superseded, decomposing, decomposed。';
COMMENT ON COLUMN project_plan_revisions.payload IS '结构化 PlanRevisionPayload，不保存长 prompt 或原始模型全文。';
COMMENT ON COLUMN project_plan_revisions.planner_provider IS '生成计划的 planner provider。';
COMMENT ON COLUMN project_plan_revisions.planner_model IS '生成计划的 planner model。';
COMMENT ON COLUMN project_plan_revisions.planner_input_hash IS 'PlanningSnapshot 输入摘要 hash。';
COMMENT ON COLUMN project_plan_revisions.plan_fingerprint IS 'canonical payload hash，用于幂等与审计。';
COMMENT ON COLUMN project_plan_revisions.validation_errors IS '服务端校验 hard error 列表。';
COMMENT ON COLUMN project_plan_revisions.validation_warnings IS '服务端校验 warning 列表。';
COMMENT ON COLUMN project_plan_revisions.review_required IS '该版本是否需要人类 review。';
COMMENT ON COLUMN project_plan_revisions.review_reason IS '需要人类 review 的摘要原因。';
COMMENT ON COLUMN project_plan_revisions.accepted_by IS '接受该计划版本的人类用户ID；策略自动接受时为空。';
COMMENT ON COLUMN project_plan_revisions.accepted_at IS '计划版本被接受时间。';
COMMENT ON COLUMN project_plan_revisions.rejected_by IS '驳回该计划版本的人类用户ID。';
COMMENT ON COLUMN project_plan_revisions.rejected_at IS '计划版本被驳回时间。';
COMMENT ON COLUMN project_plan_revisions.rejection_reason IS '驳回或要求修改的原因。';
COMMENT ON COLUMN project_plan_revisions.superseded_by_revision_id IS '替代该版本的新计划版本ID。';
COMMENT ON COLUMN project_plan_revisions.decomposition_claim_id IS '分解该版本的幂等 claim ID。';
COMMENT ON COLUMN project_plan_revisions.created_task_ids IS '该计划版本分解后创建或重放的 ProjectTask ID。';
COMMENT ON COLUMN project_plan_revisions.created_at IS '计划版本创建时间。';
COMMENT ON COLUMN project_plan_revisions.updated_at IS '计划版本最近更新时间。';

COMMENT ON TABLE project_plan_decomposition_claims IS '计划版本分解幂等声明表，保证 accepted PlanRevision 精确一次转换为 ProjectTask DAG。';
COMMENT ON COLUMN project_plan_decomposition_claims.id IS '分解声明ID。';
COMMENT ON COLUMN project_plan_decomposition_claims.tenant_id IS '租户ID。';
COMMENT ON COLUMN project_plan_decomposition_claims.project_id IS '项目ID。';
COMMENT ON COLUMN project_plan_decomposition_claims.demand_id IS '需求ID。';
COMMENT ON COLUMN project_plan_decomposition_claims.accepted_plan_revision_id IS '被分解的 accepted PlanRevision ID。';
COMMENT ON COLUMN project_plan_decomposition_claims.plan_fingerprint IS '被分解计划的 canonical hash。';
COMMENT ON COLUMN project_plan_decomposition_claims.status IS '分解状态：in_flight, completed, failed。';
COMMENT ON COLUMN project_plan_decomposition_claims.created_task_ids IS '分解成功或恢复后对应的 ProjectTask ID。';
COMMENT ON COLUMN project_plan_decomposition_claims.error IS '分解失败时记录的结构化错误。';
COMMENT ON COLUMN project_plan_decomposition_claims.created_at IS '分解声明创建时间。';
COMMENT ON COLUMN project_plan_decomposition_claims.updated_at IS '分解声明最近更新时间。';
```

- [ ] **Step 4: Add sqlc queries**

Append to `apps/control-plane/internal/storage/queries/project.sql` after route-decision queries:

```sql
-- name: CreateProjectPlanRevision :one
INSERT INTO project_plan_revisions (
    tenant_id,
    team_id,
    project_id,
    demand_id,
    coordination_job_id,
    route_decision_id,
    revision_number,
    status,
    payload,
    planner_provider,
    planner_model,
    planner_input_hash,
    plan_fingerprint,
    validation_errors,
    validation_warnings,
    review_required,
    review_reason
) VALUES (
    sqlc.arg('tenant_id')::uuid,
    sqlc.narg('team_id')::uuid,
    sqlc.arg('project_id')::uuid,
    sqlc.arg('demand_id')::uuid,
    sqlc.narg('coordination_job_id')::uuid,
    sqlc.narg('route_decision_id')::uuid,
    sqlc.arg('revision_number')::integer,
    sqlc.arg('status')::varchar,
    COALESCE(sqlc.narg('payload')::jsonb, '{}'::jsonb),
    sqlc.narg('planner_provider')::varchar,
    sqlc.narg('planner_model')::varchar,
    sqlc.narg('planner_input_hash')::varchar,
    sqlc.arg('plan_fingerprint')::varchar,
    COALESCE(sqlc.narg('validation_errors')::jsonb, '[]'::jsonb),
    COALESCE(sqlc.narg('validation_warnings')::jsonb, '[]'::jsonb),
    COALESCE(sqlc.arg('review_required')::boolean, false),
    sqlc.narg('review_reason')::text
) RETURNING *;

-- name: GetProjectPlanRevision :one
SELECT * FROM project_plan_revisions
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_id = sqlc.arg('project_id')::uuid
  AND id = sqlc.arg('id')::uuid;

-- name: GetProjectPlanRevisionByFingerprint :one
SELECT * FROM project_plan_revisions
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_id = sqlc.arg('project_id')::uuid
  AND demand_id = sqlc.arg('demand_id')::uuid
  AND plan_fingerprint = sqlc.arg('plan_fingerprint')::varchar;

-- name: ListProjectPlanRevisions :many
SELECT * FROM project_plan_revisions
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_id = sqlc.arg('project_id')::uuid
  AND (sqlc.narg('demand_id')::uuid IS NULL OR demand_id = sqlc.narg('demand_id')::uuid)
ORDER BY demand_id ASC, revision_number DESC
LIMIT sqlc.arg('limit') OFFSET sqlc.arg('offset');

-- name: NextProjectPlanRevisionNumber :one
SELECT COALESCE(MAX(revision_number), 0)::integer + 1 AS revision_number
FROM project_plan_revisions
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_id = sqlc.arg('project_id')::uuid
  AND demand_id = sqlc.arg('demand_id')::uuid;

-- name: SupersedeOpenProjectPlanRevisions :exec
UPDATE project_plan_revisions
SET status = 'superseded',
    superseded_by_revision_id = sqlc.arg('superseded_by_revision_id')::uuid,
    rejection_reason = sqlc.narg('reason')::text,
    updated_at = NOW()
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_id = sqlc.arg('project_id')::uuid
  AND demand_id = sqlc.arg('demand_id')::uuid
  AND id <> sqlc.arg('superseded_by_revision_id')::uuid
  AND status IN ('draft', 'validation_failed', 'pending_review');

-- name: AcceptProjectPlanRevision :one
UPDATE project_plan_revisions
SET status = 'accepted',
    accepted_by = sqlc.narg('accepted_by')::uuid,
    accepted_at = NOW(),
    updated_at = NOW()
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_id = sqlc.arg('project_id')::uuid
  AND id = sqlc.arg('id')::uuid
  AND status IN ('draft', 'pending_review')
RETURNING *;

-- name: RejectProjectPlanRevision :one
UPDATE project_plan_revisions
SET status = 'rejected',
    rejected_by = sqlc.narg('rejected_by')::uuid,
    rejected_at = NOW(),
    rejection_reason = sqlc.narg('rejection_reason')::text,
    updated_at = NOW()
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_id = sqlc.arg('project_id')::uuid
  AND id = sqlc.arg('id')::uuid
  AND status = 'pending_review'
RETURNING *;

-- name: MarkProjectPlanRevisionDecomposing :one
UPDATE project_plan_revisions
SET status = 'decomposing',
    decomposition_claim_id = sqlc.arg('decomposition_claim_id')::uuid,
    updated_at = NOW()
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_id = sqlc.arg('project_id')::uuid
  AND id = sqlc.arg('id')::uuid
  AND status = 'accepted'
RETURNING *;

-- name: MarkProjectPlanRevisionDecomposed :one
UPDATE project_plan_revisions
SET status = 'decomposed',
    created_task_ids = sqlc.arg('created_task_ids')::uuid[],
    updated_at = NOW()
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_id = sqlc.arg('project_id')::uuid
  AND id = sqlc.arg('id')::uuid
  AND status IN ('decomposing', 'decomposed')
RETURNING *;

-- name: CreateProjectPlanDecompositionClaim :one
INSERT INTO project_plan_decomposition_claims (
    tenant_id,
    project_id,
    demand_id,
    accepted_plan_revision_id,
    plan_fingerprint,
    status
) VALUES (
    sqlc.arg('tenant_id')::uuid,
    sqlc.arg('project_id')::uuid,
    sqlc.arg('demand_id')::uuid,
    sqlc.arg('accepted_plan_revision_id')::uuid,
    sqlc.arg('plan_fingerprint')::varchar,
    'in_flight'
)
ON CONFLICT (tenant_id, project_id, demand_id, accepted_plan_revision_id)
DO UPDATE SET updated_at = project_plan_decomposition_claims.updated_at
RETURNING *;

-- name: CompleteProjectPlanDecompositionClaim :one
UPDATE project_plan_decomposition_claims
SET status = 'completed',
    created_task_ids = sqlc.arg('created_task_ids')::uuid[],
    error = '{}'::jsonb,
    updated_at = NOW()
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_id = sqlc.arg('project_id')::uuid
  AND id = sqlc.arg('id')::uuid
RETURNING *;

-- name: FailProjectPlanDecompositionClaim :one
UPDATE project_plan_decomposition_claims
SET status = 'failed',
    error = COALESCE(sqlc.narg('error')::jsonb, '{}'::jsonb),
    updated_at = NOW()
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_id = sqlc.arg('project_id')::uuid
  AND id = sqlc.arg('id')::uuid
RETURNING *;
```

- [ ] **Step 5: Regenerate storage code**

Run:

```bash
corepack pnpm generate:control-plane
```

Expected: PASS and generated sqlc files include `ProjectPlanRevision`, `ProjectPlanDecompositionClaim`, and query methods.

- [ ] **Step 6: Run migration and sqlc tests**

Run:

```bash
go test ./apps/control-plane/internal/storage -run TestProjectPlanRevisionsMigrationHasTenantFirstIndexes -count=1
go test ./apps/control-plane/internal/storage/queries -count=1
```

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add apps/control-plane/internal/storage/migrations/031_project_plan_revisions.sql apps/control-plane/internal/storage/migrations/atlas.sum apps/control-plane/internal/storage/migrations_test.go apps/control-plane/internal/storage/queries/project.sql apps/control-plane/internal/storage/queries/project.sql.go apps/control-plane/internal/storage/queries/querier.go
git commit -m "feat: persist project plan revisions"
```

### Task 3: Repository State Transitions And Exact-Once Claim

**Files:**
- Modify: `apps/control-plane/internal/project/repository.go`
- Modify: `apps/control-plane/internal/project/types.go`
- Modify: `apps/control-plane/internal/project/pg_repository.go`
- Modify: `apps/control-plane/internal/project/pg_repository_test.go`

**Pre-existing implementation (extend, do not rebuild):** `DecomposeAcceptedPlanRevision` already exists at `apps/control-plane/internal/project/pg_repository.go:1008` (replay-by-`accepted_plan_revision_id`, conflict detection via `acceptedPlanGraphComplete`, graph write via `createProjectTaskGraphWithQueries`) with three passing tests — `TestDecomposeAcceptedPlanRevisionIsIdempotent`, `TestDecomposeAcceptedPlanRevisionReplaysAcrossCoordinationJobs`, `TestDecomposeAcceptedPlanRevisionRejectsChangedPayload` — all built on the `createDecomposeAcceptedPlanRevisionFixtureRequest` helper at `pg_repository_test.go:2393`. `DecomposeAcceptedPlanRevisionRequest` already exists at `types.go:516` and matches the shape below except for `PlanFingerprint`. This task therefore **extends** the existing method to (a) require a real `project_plan_revisions` row in an accepted state with a matching fingerprint, and (b) record the decomposition in the new `project_plan_decomposition_claims` table. The existing fixture and the three existing tests must be updated in the same change — the new `q.GetProjectPlanRevision(...)` preamble will otherwise fail them with "revision not found".

- [ ] **Step 1: Update existing decomposition tests/fixture and add new tests**

Append to `apps/control-plane/internal/project/pg_repository_test.go`:

```go
func TestProjectPlanRevisionCreateAcceptAndList(t *testing.T) {
	repo, tenantID := newProjectRepositoryTestStore(t)
	projectID := createProjectFixture(t, repo, tenantID)
	demandID := createDemandFixture(t, repo, tenantID, projectID)
	jobID := createCoordinationJobFixture(t, repo, tenantID, projectID)
	routeID := createRouteDecisionFixture(t, repo, tenantID, projectID, demandID, jobID)

	created, err := repo.CreatePlanRevision(context.Background(), CreatePlanRevisionRequest{
		TenantID:          tenantID,
		ProjectID:         projectID,
		DemandID:          demandID,
		CoordinationJobID: &jobID,
		RouteDecisionID:   &routeID,
		Status:            PlanRevisionStatusPendingReview,
		Payload:           map[string]any{"summary": "计划"},
		PlanFingerprint:   "fingerprint-1",
		ReviewRequired:    true,
		ReviewReason:      strPtr("high risk"),
	})
	require.NoError(t, err)
	require.Equal(t, int32(1), created.RevisionNumber)
	require.Equal(t, PlanRevisionStatusPendingReview, created.Status)

	acceptedBy := uuid.New()
	accepted, err := repo.AcceptPlanRevision(context.Background(), AcceptPlanRevisionRequest{
		TenantID:   tenantID,
		ProjectID:  projectID,
		RevisionID: created.ID,
		AcceptedBy: &acceptedBy,
	})
	require.NoError(t, err)
	require.Equal(t, PlanRevisionStatusAccepted, accepted.Status)
	require.NotNil(t, accepted.AcceptedAt)

	revisions, err := repo.ListPlanRevisions(context.Background(), ListPlanRevisionsRequest{
		TenantID:  tenantID,
		ProjectID: projectID,
		DemandID:  &demandID,
		Limit:     20,
		Offset:    0,
	})
	require.NoError(t, err)
	require.Len(t, revisions, 1)
	require.Equal(t, created.ID, revisions[0].ID)
}

func TestProjectPlanRevisionSupersedesOpenRevisions(t *testing.T) {
	repo, tenantID := newProjectRepositoryTestStore(t)
	projectID := createProjectFixture(t, repo, tenantID)
	demandID := createDemandFixture(t, repo, tenantID, projectID)

	first, err := repo.CreatePlanRevision(context.Background(), CreatePlanRevisionRequest{
		TenantID:        tenantID,
		ProjectID:       projectID,
		DemandID:        demandID,
		Status:          PlanRevisionStatusPendingReview,
		Payload:         map[string]any{"summary": "旧计划"},
		PlanFingerprint: "fingerprint-old",
		ReviewRequired:  true,
	})
	require.NoError(t, err)

	second, err := repo.CreatePlanRevision(context.Background(), CreatePlanRevisionRequest{
		TenantID:               tenantID,
		ProjectID:              projectID,
		DemandID:               demandID,
		Status:                 PlanRevisionStatusPendingReview,
		Payload:                map[string]any{"summary": "新计划"},
		PlanFingerprint:        "fingerprint-new",
		ReviewRequired:         true,
		SupersedeOpenRevisions: true,
		SupersedeReason:        strPtr("human requested changes"),
	})
	require.NoError(t, err)
	require.Equal(t, int32(2), second.RevisionNumber)

	stale, err := repo.GetPlanRevision(context.Background(), tenantID, projectID, first.ID)
	require.NoError(t, err)
	require.Equal(t, PlanRevisionStatusSuperseded, stale.Status)
	require.Equal(t, second.ID, *stale.SupersededByRevisionID)
}

func TestDecomposeAcceptedPlanRevisionRequiresAcceptedRevisionAndCompletesClaim(t *testing.T) {
	repo, tenantID := newProjectRepositoryTestStore(t)
	projectID := createProjectFixture(t, repo, tenantID)
	demandID := createDemandFixture(t, repo, tenantID, projectID)
	jobID := createCoordinationJobFixture(t, repo, tenantID, projectID)
	routeID := createRouteDecisionFixture(t, repo, tenantID, projectID, demandID, jobID)
	employeeID := uuid.New()

	revision, err := repo.CreatePlanRevision(context.Background(), CreatePlanRevisionRequest{
		TenantID:        tenantID,
		ProjectID:       projectID,
		DemandID:        demandID,
		Status:          PlanRevisionStatusAccepted,
		Payload:         map[string]any{"summary": "accepted"},
		PlanFingerprint: "fingerprint-accepted",
	})
	require.NoError(t, err)

	req := DecomposeAcceptedPlanRevisionRequest{
		TenantID:               tenantID,
		ProjectID:              projectID,
		DemandID:               demandID,
		CoordinationJobID:      jobID,
		RouteDecisionID:        routeID,
		AcceptedPlanRevisionID: revision.ID,
		PlanFingerprint:        revision.PlanFingerprint,
		DecompositionClaimKey:  revision.ID.String(),
		Tasks: []ProjectTaskGraphCreateTask{
			{
				Key:                       "inspect",
				Title:                     "检查",
				Summary:                   "检查输入",
				Status:                    ProjectTaskStatusPlanned,
				AssignedDigitalEmployeeID: employeeID,
				ExpectedOutputs:           []any{"结论"},
				InputRequirements:         map[string]any{"context": "demand"},
				HandoffContract:           map[string]any{"acceptance_criteria": []any{"结论可复核"}},
				PlannerMetadata:           map[string]any{"accepted_plan_revision_id": revision.ID.String()},
			},
		},
	}

	first, err := repo.DecomposeAcceptedPlanRevision(context.Background(), req)
	require.NoError(t, err)
	require.False(t, first.Replayed)
	require.Len(t, first.Tasks, 1)

	second, err := repo.DecomposeAcceptedPlanRevision(context.Background(), req)
	require.NoError(t, err)
	require.True(t, second.Replayed)
	require.Equal(t, first.Tasks[0].ID, second.Tasks[0].ID)

	stored, err := repo.GetPlanRevision(context.Background(), tenantID, projectID, revision.ID)
	require.NoError(t, err)
	require.Equal(t, PlanRevisionStatusDecomposed, stored.Status)
	require.Equal(t, []uuid.UUID{first.Tasks[0].ID}, stored.CreatedTaskIDs)
	require.NotNil(t, stored.DecompositionClaimID)
}
```

Then update the existing fixture and the three existing decomposition tests so they create a real accepted revision row before calling `DecomposeAcceptedPlanRevision`, and pass `PlanFingerprint` on the request:

- `createDecomposeAcceptedPlanRevisionFixtureRequest` (`pg_repository_test.go:2393`): set `PlanFingerprint` on the returned request and insert a matching `project_plan_revisions` row (status `accepted`) via the new `CreatePlanRevision` repository method before returning.
- `TestDecomposeAcceptedPlanRevisionIsIdempotent`, `TestDecomposeAcceptedPlanRevisionReplaysAcrossCoordinationJobs`: keep their existing assertions; with the fixture providing the accepted revision row they must still pass against the rewritten method.
- `TestDecomposeAcceptedPlanRevisionRejectsChangedPayload`: re-assert that the conflict is now raised on fingerprint mismatch against the persisted revision row (the new `revision.PlanFingerprint != req.PlanFingerprint` check).

- [ ] **Step 2: Run repository tests and verify they fail**

Run:

```bash
go test ./apps/control-plane/internal/project -run 'TestProjectPlanRevision|TestDecomposeAcceptedPlanRevisionRequiresAcceptedRevisionAndCompletesClaim' -count=1
```

Expected: FAIL with undefined repository methods and request types.

- [ ] **Step 3: Add repository request types and interface methods**

Add to `apps/control-plane/internal/project/repository.go` interface near route decision methods:

```go
	CreatePlanRevision(ctx context.Context, req CreatePlanRevisionRequest) (PlanRevision, error)
	GetPlanRevision(ctx context.Context, tenantID, projectID, revisionID uuid.UUID) (PlanRevision, error)
	ListPlanRevisions(ctx context.Context, req ListPlanRevisionsRequest) ([]PlanRevision, error)
	AcceptPlanRevision(ctx context.Context, req AcceptPlanRevisionRequest) (PlanRevision, error)
	RejectPlanRevision(ctx context.Context, req RejectPlanRevisionRequest) (PlanRevision, error)
```

Add request types in `apps/control-plane/internal/project/repository.go` below `CreateRouteDecisionRequest`:

```go
type CreatePlanRevisionRequest struct {
	TenantID               uuid.UUID
	TeamID                 *uuid.UUID
	ProjectID              uuid.UUID
	DemandID               uuid.UUID
	CoordinationJobID       *uuid.UUID
	RouteDecisionID         *uuid.UUID
	Status                 string
	Payload                map[string]any
	PlannerProvider        *string
	PlannerModel           *string
	PlannerInputHash       *string
	PlanFingerprint        string
	ValidationErrors       []string
	ValidationWarnings     []string
	ReviewRequired         bool
	ReviewReason           *string
	SupersedeOpenRevisions bool
	SupersedeReason        *string
}

type ListPlanRevisionsRequest struct {
	TenantID  uuid.UUID
	ProjectID uuid.UUID
	DemandID  *uuid.UUID
	Limit     int32
	Offset    int32
}

type AcceptPlanRevisionRequest struct {
	TenantID   uuid.UUID
	ProjectID  uuid.UUID
	RevisionID uuid.UUID
	AcceptedBy *uuid.UUID
}

type RejectPlanRevisionRequest struct {
	TenantID        uuid.UUID
	ProjectID       uuid.UUID
	RevisionID      uuid.UUID
	RejectedBy      *uuid.UUID
	RejectionReason *string
}
```

Add the `PlanFingerprint` field to the **existing** `DecomposeAcceptedPlanRevisionRequest` in `apps/control-plane/internal/project/types.go` (the other fields already match at `types.go:516`; do not duplicate the struct):

```go
type DecomposeAcceptedPlanRevisionRequest struct {
	TenantID               uuid.UUID
	ProjectID              uuid.UUID
	DemandID               uuid.UUID
	CoordinationJobID      uuid.UUID
	RouteDecisionID        uuid.UUID
	AcceptedPlanRevisionID uuid.UUID
	PlanFingerprint        string
	DecompositionClaimKey  string
	Tasks                  []ProjectTaskGraphCreateTask
}
```

- [ ] **Step 4: Implement repository mapping and state transitions**

In `apps/control-plane/internal/project/pg_repository.go`, add methods:

```go
func (r *PgRepository) CreatePlanRevision(ctx context.Context, req CreatePlanRevisionRequest) (PlanRevision, error) {
	if req.TenantID == uuid.Nil || req.ProjectID == uuid.Nil || req.DemandID == uuid.Nil || strings.TrimSpace(req.PlanFingerprint) == "" {
		return PlanRevision{}, ErrInvalidProject
	}
	return withProjectQueries(ctx, r, "project plan revision create", func(q *queries.Queries) (PlanRevision, error) {
		if req.SupersedeOpenRevisions {
			revisionNumber, err := q.NextProjectPlanRevisionNumber(ctx, queries.NextProjectPlanRevisionNumberParams{
				TenantID: req.TenantID,
				ProjectID: req.ProjectID,
				DemandID: req.DemandID,
			})
			if err != nil {
				return PlanRevision{}, err
			}
			payload := jsonbObject(req.Payload)
			errorsJSON := jsonbStringArray(req.ValidationErrors)
			warningsJSON := jsonbStringArray(req.ValidationWarnings)
			row, err := q.CreateProjectPlanRevision(ctx, queries.CreateProjectPlanRevisionParams{
				TenantID:           req.TenantID,
				TeamID:             nullUUID(req.TeamID),
				ProjectID:          req.ProjectID,
				DemandID:           req.DemandID,
				CoordinationJobID:  nullUUID(req.CoordinationJobID),
				RouteDecisionID:    nullUUID(req.RouteDecisionID),
				RevisionNumber:     revisionNumber,
				Status:             req.Status,
				Payload:            payload,
				PlannerProvider:    textOrNull(req.PlannerProvider),
				PlannerModel:       textOrNull(req.PlannerModel),
				PlannerInputHash:   textOrNull(req.PlannerInputHash),
				PlanFingerprint:    req.PlanFingerprint,
				ValidationErrors:   errorsJSON,
				ValidationWarnings: warningsJSON,
				ReviewRequired:     req.ReviewRequired,
				ReviewReason:       textOrNull(req.ReviewReason),
			})
			if err != nil {
				return PlanRevision{}, err
			}
			created, err := planRevisionFromRecord(row)
			if err != nil {
				return PlanRevision{}, err
			}
			if err := q.SupersedeOpenProjectPlanRevisions(ctx, queries.SupersedeOpenProjectPlanRevisionsParams{
				TenantID:                req.TenantID,
				ProjectID:               req.ProjectID,
				DemandID:                req.DemandID,
				SupersededByRevisionID:  created.ID,
				Reason:                  textOrNull(req.SupersedeReason),
			}); err != nil {
				return PlanRevision{}, err
			}
			return created, nil
		}
		revisionNumber, err := q.NextProjectPlanRevisionNumber(ctx, queries.NextProjectPlanRevisionNumberParams{
			TenantID: req.TenantID,
			ProjectID: req.ProjectID,
			DemandID: req.DemandID,
		})
		if err != nil {
			return PlanRevision{}, err
		}
		row, err := q.CreateProjectPlanRevision(ctx, queries.CreateProjectPlanRevisionParams{
			TenantID:           req.TenantID,
			TeamID:             nullUUID(req.TeamID),
			ProjectID:          req.ProjectID,
			DemandID:           req.DemandID,
			CoordinationJobID:  nullUUID(req.CoordinationJobID),
			RouteDecisionID:    nullUUID(req.RouteDecisionID),
			RevisionNumber:     revisionNumber,
			Status:             req.Status,
			Payload:            jsonbObject(req.Payload),
			PlannerProvider:    textOrNull(req.PlannerProvider),
			PlannerModel:       textOrNull(req.PlannerModel),
			PlannerInputHash:   textOrNull(req.PlannerInputHash),
			PlanFingerprint:    req.PlanFingerprint,
			ValidationErrors:   jsonbStringArray(req.ValidationErrors),
			ValidationWarnings: jsonbStringArray(req.ValidationWarnings),
			ReviewRequired:     req.ReviewRequired,
			ReviewReason:       textOrNull(req.ReviewReason),
		})
		if isPGUniqueConstraint(err) {
			existing, existingErr := q.GetProjectPlanRevisionByFingerprint(ctx, queries.GetProjectPlanRevisionByFingerprintParams{
				TenantID:        req.TenantID,
				ProjectID:       req.ProjectID,
				DemandID:        req.DemandID,
				PlanFingerprint: req.PlanFingerprint,
			})
			if existingErr == nil {
				return planRevisionFromRecord(existing)
			}
		}
		if err != nil {
			return PlanRevision{}, err
		}
		return planRevisionFromRecord(row)
	})
}
```

Add `GetPlanRevision`, `ListPlanRevisions`, `AcceptPlanRevision`, and `RejectPlanRevision` using the generated queries. Convert `pgx.ErrNoRows` to `ErrProjectNotFound`, and convert empty UPDATE result to `ErrProjectConflict`.

- [ ] **Step 5: Extend existing decomposition with the claim table**

Rewrite the body of the **existing** `DecomposeAcceptedPlanRevision` at `pg_repository.go:1008` so it loads and validates the `PlanRevision`, creates or replays the claim, writes tasks with the real revision ID, completes the claim, and marks the revision `decomposed`. Reuse the method's current internals (`listProjectTasksByAcceptedPlanRevisionWithQueries`, `acceptedPlanGraphComplete`, `graphResultFromExisting`, `createProjectTaskGraphWithQueries`, `projectTaskGraphWriteOptions`); only the preamble (revision load + fingerprint check + claim row) and tail (claim complete + mark decomposed) are new. The replay branch re-marks an already-`decomposed` revision via `MarkProjectPlanRevisionDecomposed`; the SQL `WHERE status IN ('decomposing','decomposed')` permits this, and `CanTransitionPlanRevisionStatus` was updated in Task 1 Step 3 to allow `decomposed→decomposed` so the domain guard and SQL agree.

Use this flow in `apps/control-plane/internal/project/pg_repository.go`:

```go
func (r *PgRepository) DecomposeAcceptedPlanRevision(ctx context.Context, req DecomposeAcceptedPlanRevisionRequest) (DecomposeAcceptedPlanRevisionResult, error) {
	req.DecompositionClaimKey = strings.TrimSpace(req.DecompositionClaimKey)
	req.PlanFingerprint = strings.TrimSpace(req.PlanFingerprint)
	if err := validateDecomposeAcceptedPlanRevisionRequest(req); err != nil {
		return DecomposeAcceptedPlanRevisionResult{}, err
	}
	graphReq := graphRequestFromAcceptedPlanRevisionRequest(req)
	return withProjectQueries(ctx, r, "accepted plan revision decompose", func(q *queries.Queries) (DecomposeAcceptedPlanRevisionResult, error) {
		revisionRow, err := q.GetProjectPlanRevision(ctx, queries.GetProjectPlanRevisionParams{
			TenantID:  req.TenantID,
			ProjectID: req.ProjectID,
			ID:        req.AcceptedPlanRevisionID,
		})
		if err != nil {
			return DecomposeAcceptedPlanRevisionResult{}, projectRepositoryError(err)
		}
		revision, err := planRevisionFromRecord(revisionRow)
		if err != nil {
			return DecomposeAcceptedPlanRevisionResult{}, err
		}
		if revision.DemandID != req.DemandID || !IsAcceptedPlanRevisionStatus(revision.Status) {
			return DecomposeAcceptedPlanRevisionResult{}, ErrProjectConflict
		}
		if revision.PlanFingerprint != req.PlanFingerprint {
			return DecomposeAcceptedPlanRevisionResult{}, ErrProjectConflict
		}
		claimRow, err := q.CreateProjectPlanDecompositionClaim(ctx, queries.CreateProjectPlanDecompositionClaimParams{
			TenantID:               req.TenantID,
			ProjectID:              req.ProjectID,
			DemandID:               req.DemandID,
			AcceptedPlanRevisionID: req.AcceptedPlanRevisionID,
			PlanFingerprint:        req.PlanFingerprint,
		})
		if err != nil {
			return DecomposeAcceptedPlanRevisionResult{}, err
		}
		claim, err := planDecompositionClaimFromRecord(claimRow)
		if err != nil {
			return DecomposeAcceptedPlanRevisionResult{}, err
		}
		existing, err := r.listProjectTasksByAcceptedPlanRevisionWithQueries(ctx, q, req.TenantID, req.ProjectID, req.DemandID, req.AcceptedPlanRevisionID)
		if err != nil {
			return DecomposeAcceptedPlanRevisionResult{}, err
		}
		if len(existing) > 0 {
			originReq, complete, err := r.acceptedPlanGraphComplete(ctx, q, req, existing)
			if err != nil {
				return DecomposeAcceptedPlanRevisionResult{}, err
			}
			if !complete {
				return DecomposeAcceptedPlanRevisionResult{}, ErrProjectConflict
			}
			graph, err := r.graphResultFromExisting(ctx, q, originReq, existing)
			if err != nil {
				return DecomposeAcceptedPlanRevisionResult{}, err
			}
			taskIDs := projectTaskIDs(existing)
			if _, err := q.CompleteProjectPlanDecompositionClaim(ctx, queries.CompleteProjectPlanDecompositionClaimParams{
				TenantID:       req.TenantID,
				ProjectID:      req.ProjectID,
				ID:             claim.ID,
				CreatedTaskIds: taskIDs,
			}); err != nil {
				return DecomposeAcceptedPlanRevisionResult{}, err
			}
			if _, err := q.MarkProjectPlanRevisionDecomposed(ctx, queries.MarkProjectPlanRevisionDecomposedParams{
				TenantID:       req.TenantID,
				ProjectID:      req.ProjectID,
				ID:             req.AcceptedPlanRevisionID,
				CreatedTaskIds: taskIDs,
			}); err != nil {
				return DecomposeAcceptedPlanRevisionResult{}, err
			}
			return DecomposeAcceptedPlanRevisionResult{Tasks: existing, Dependencies: graph.Dependencies, Replayed: true}, nil
		}
		if revision.Status == PlanRevisionStatusAccepted {
			if _, err := q.MarkProjectPlanRevisionDecomposing(ctx, queries.MarkProjectPlanRevisionDecomposingParams{
				TenantID:             req.TenantID,
				ProjectID:            req.ProjectID,
				ID:                   req.AcceptedPlanRevisionID,
				DecompositionClaimID: claim.ID,
			}); err != nil {
				return DecomposeAcceptedPlanRevisionResult{}, err
			}
		}
		created, err := r.createProjectTaskGraphWithQueries(ctx, q, graphReq, projectTaskGraphWriteOptions{
			AcceptedPlanRevisionID: &req.AcceptedPlanRevisionID,
			DecompositionClaimKey:  &req.DecompositionClaimKey,
		})
		if err != nil {
			_, _ = q.FailProjectPlanDecompositionClaim(ctx, queries.FailProjectPlanDecompositionClaimParams{
				TenantID:  req.TenantID,
				ProjectID: req.ProjectID,
				ID:        claim.ID,
				Error:     jsonbObject(map[string]any{"message": err.Error()}),
			})
			return DecomposeAcceptedPlanRevisionResult{}, err
		}
		taskIDs := projectTaskIDs(created.Tasks)
		if _, err := q.CompleteProjectPlanDecompositionClaim(ctx, queries.CompleteProjectPlanDecompositionClaimParams{
			TenantID:       req.TenantID,
			ProjectID:      req.ProjectID,
			ID:             claim.ID,
			CreatedTaskIds: taskIDs,
		}); err != nil {
			return DecomposeAcceptedPlanRevisionResult{}, err
		}
		if _, err := q.MarkProjectPlanRevisionDecomposed(ctx, queries.MarkProjectPlanRevisionDecomposedParams{
			TenantID:       req.TenantID,
			ProjectID:      req.ProjectID,
			ID:             req.AcceptedPlanRevisionID,
			CreatedTaskIds: taskIDs,
		}); err != nil {
			return DecomposeAcceptedPlanRevisionResult{}, err
		}
		return DecomposeAcceptedPlanRevisionResult{Tasks: created.Tasks, Dependencies: created.Graph.Dependencies, Replayed: false}, nil
	})
}
```

Add `projectTaskIDs`, `planRevisionFromRecord`, and `planDecompositionClaimFromRecord` helpers in the same file. Use existing JSON helper patterns such as `jsonbObject`, `jsonbArray`, and record conversion helpers already used by `RouteDecision`, `ProjectTask`, and `DecisionRequest`.

- [ ] **Step 6: Run repository tests**

Run:

```bash
go test ./apps/control-plane/internal/project -run 'TestProjectPlanRevision|TestDecomposeAcceptedPlanRevision' -count=1
```

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add apps/control-plane/internal/project/repository.go apps/control-plane/internal/project/types.go apps/control-plane/internal/project/pg_repository.go apps/control-plane/internal/project/pg_repository_test.go
git commit -m "feat: add plan revision repository state"
```

### Task 4: ProjectStore Activities And Workflow Acceptance Order

**Files:**
- Modify: `apps/control-plane/internal/workflow/projectcoordination/types.go`
- Modify: `apps/control-plane/internal/workflow/projectcoordination/activities.go`
- Modify: `apps/control-plane/internal/workflow/projectcoordination/project_store.go`
- Modify: `apps/control-plane/internal/workflow/projectcoordination/project_store_test.go`
- Modify: `apps/control-plane/internal/workflow/projectcoordination/workflow.go`
- Modify: `apps/control-plane/internal/workflow/projectcoordination/workflow_test.go`

**Compile-order constraint (important):** The new `ActivityStore` interface in Step 4 removes `CreateProjectTasks` and `RequestRouteDecisionReview`, but `workflow.go:159` / `workflow.go:181` still call them and `project_store.go:359` / `project_store.go:979` still implement them (plus helpers `acceptedPlanRevisionIDForRouteDecision` `:1367`, `acceptedPlanRevisionDecompositionClaimKey` `:1383`, `routeReviewTargetUserID` `:1202`, `routeReviewContext` `:1628`). Until Steps 3–7 are all applied the package will not compile. Treat Steps 3–7 as a **single compileable change**: do the interface edit, the `ProjectStore` method replacement, the `workflow.go` rewrite, the deletion of the old store methods/helpers, and the test-double/test updates together, and commit only once `go build ./apps/control-plane/internal/workflow/projectcoordination` passes. Use `-run` to exercise individual new methods during development, but do not treat intermediate steps as green checkpoints.

- [ ] **Step 1: Write failing ProjectStore tests**

In `apps/control-plane/internal/workflow/projectcoordination/project_store_test.go`, add:

```go
func TestProjectStorePersistsPendingPlanRevisionWithoutCreatingTasks(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	demandID := uuid.New()
	jobID := uuid.New()
	routeID := uuid.New()
	employeeID := uuid.New()
	repo := newProjectStoreMemoryRepository()
	store := NewProjectStore(repo)

	result, err := store.PersistPlanRevision(context.Background(), PersistPlanRevisionInput{
		TenantID:          tenantID,
		ProjectID:         projectID,
		DemandID:          demandID,
		CoordinationJobID: jobID,
		RouteDecisionID:   routeID,
		Decision: RouteDecisionPlan{
			Reason:              "需要人工复核计划",
			RequiresHumanReview: true,
			Tasks: []PlannedTask{
				{
					Key:                       "inspect",
					Title:                     "检查",
					Summary:                   "检查输入",
					TaskKind:                  "analysis",
					SelectedEmployeeID:        employeeID,
					EmployeeSelectionReason:   "具备分析能力",
					RequiredCapabilities:      []string{"codebase.analysis"},
					MatchedCapabilities:       []string{"codebase.analysis"},
					ExpectedOutputs:           []string{"结论"},
					HandoffContract:           map[string]any{"acceptance_criteria": []any{"结论可复核"}},
				},
			},
		},
	})

	require.NoError(t, err)
	require.Equal(t, project.PlanRevisionStatusPendingReview, result.Status)
	require.NotEqual(t, uuid.Nil, result.ID)
	require.Empty(t, repo.decomposeAcceptedPlanRevisionRequests)
}

func TestProjectStoreDecomposesOnlyAcceptedPlanRevision(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	demandID := uuid.New()
	jobID := uuid.New()
	routeID := uuid.New()
	revisionID := uuid.New()
	employeeID := uuid.New()
	repo := newProjectStoreMemoryRepository()
	repo.planRevisions = append(repo.planRevisions, project.PlanRevision{
		ID:              revisionID,
		TenantID:        tenantID,
		ProjectID:       projectID,
		DemandID:        demandID,
		Status:          project.PlanRevisionStatusAccepted,
		Payload:         map[string]any{"summary": "accepted"},
		PlanFingerprint: "fingerprint",
	})
	store := NewProjectStore(repo)

	tasks, err := store.DecomposeAcceptedPlanRevision(context.Background(), DecomposeAcceptedPlanRevisionInput{
		TenantID:          tenantID,
		ProjectID:         projectID,
		DemandID:          demandID,
		CoordinationJobID: jobID,
		RouteDecisionID:   routeID,
		PlanRevisionID:    revisionID,
		PlanFingerprint:   "fingerprint",
		Payload: PlanRevisionPayload{
			Summary: "accepted",
			Tasks: []PlanRevisionTask{
				{
					PlannedTaskKey:          "inspect",
					Title:                   "检查",
					Objective:               "检查输入",
					TaskType:                "analysis",
					SelectedEmployeeID:      employeeID.String(),
					EmployeeSelectionReason: "具备分析能力",
					ExpectedOutputs:         []string{"结论"},
					AcceptanceCriteria:      []string{"结论可复核"},
				},
			},
			FinalSummaryContract: PlanRevisionFinalSummaryContract{RequiredSections: []string{"conclusion", "evidence", "risks", "next_steps"}},
		},
	})

	require.NoError(t, err)
	require.Len(t, tasks, 1)
	require.Len(t, repo.decomposeAcceptedPlanRevisionRequests, 1)
	require.Equal(t, revisionID, repo.decomposeAcceptedPlanRevisionRequests[0].AcceptedPlanRevisionID)
}
```

- [ ] **Step 2: Run ProjectStore tests and verify they fail**

Run:

```bash
go test ./apps/control-plane/internal/workflow/projectcoordination -run 'TestProjectStorePersistsPendingPlanRevision|TestProjectStoreDecomposesOnlyAcceptedPlanRevision' -count=1
```

Expected: FAIL with undefined `PersistPlanRevision`, `DecomposeAcceptedPlanRevisionInput`, and memory repository fields.

- [ ] **Step 3: Replace route-review activity types with plan revision types**

In `apps/control-plane/internal/workflow/projectcoordination/types.go`, add:

```go
type PersistPlanRevisionInput struct {
	TenantID          uuid.UUID
	ProjectID         uuid.UUID
	DemandID          uuid.UUID
	CoordinationJobID uuid.UUID
	RouteDecisionID   uuid.UUID
	Decision          RouteDecisionPlan
	SupersedeOpen     bool
	SupersedeReason   *string
}

type PlanRevisionResult struct {
	ID              uuid.UUID
	Status          string
	RevisionNumber  int32
	PlanFingerprint string
	Payload         PlanRevisionPayload
	ReviewRequired bool
	CreatedEventID uuid.UUID
}

type RequestPlanRevisionReviewInput struct {
	TenantID          uuid.UUID
	ProjectID         uuid.UUID
	CoordinationJobID uuid.UUID
	DemandID          uuid.UUID
	PlanRevisionID    uuid.UUID
	PlanFingerprint   string
	Payload           PlanRevisionPayload
	CreatedEventID    uuid.UUID
}

type ResolvePlanRevisionReviewInput struct {
	TenantID          uuid.UUID
	ProjectID         uuid.UUID
	DemandID          uuid.UUID
	CoordinationJobID uuid.UUID
	PlanRevisionID    uuid.UUID
	DecisionRequestID uuid.UUID
	Decision          string
	Payload           map[string]any
	ActorUserID       uuid.UUID
}

type DecomposeAcceptedPlanRevisionInput struct {
	TenantID          uuid.UUID
	ProjectID         uuid.UUID
	DemandID          uuid.UUID
	CoordinationJobID uuid.UUID
	RouteDecisionID   uuid.UUID
	PlanRevisionID    uuid.UUID
	PlanFingerprint   string
	Payload           PlanRevisionPayload
}
```

Keep `RequestRouteDecisionReviewInput` only if existing tests need compatibility; new workflow paths should use `RequestPlanRevisionReviewInput`.

- [ ] **Step 4: Add activity methods**

Update `ActivityStore` and `Activities` in `apps/control-plane/internal/workflow/projectcoordination/activities.go`:

```go
type ActivityStore interface {
	LoadProjectCoordinationSnapshot(ctx context.Context, input LoadSnapshotInput) (CoordinationSnapshot, error)
	CreateCoordinationJob(ctx context.Context, input CreateCoordinationJobInput) (CoordinationJobResult, error)
	PersistRouteDecision(ctx context.Context, input PersistRouteDecisionInput) (RouteDecisionResult, error)
	PersistPlanRevision(ctx context.Context, input PersistPlanRevisionInput) (PlanRevisionResult, error)
	RequestPlanRevisionReview(ctx context.Context, input RequestPlanRevisionReviewInput) (DecisionRequestResult, error)
	ResolvePlanRevisionReview(ctx context.Context, input ResolvePlanRevisionReviewInput) (PlanRevisionResult, error)
	DecomposeAcceptedPlanRevision(ctx context.Context, input DecomposeAcceptedPlanRevisionInput) ([]ProjectTaskResult, error)
	ListDispatchableTasks(ctx context.Context, input ListDispatchableTasksInput) ([]uuid.UUID, error)
	ResolveReadyDownstream(ctx context.Context, input ResolveReadyDownstreamInput) ([]uuid.UUID, error)
	IsProjectAcceptanceReady(ctx context.Context, input IsProjectAcceptanceReadyInput) (bool, error)
	RequestProjectAcceptanceReview(ctx context.Context, input RequestProjectAcceptanceReviewInput) (DecisionRequestResult, error)
	ApplyProjectAcceptanceDecision(ctx context.Context, input ApplyProjectAcceptanceDecisionInput) error
	HoldDownstreamForFailure(ctx context.Context, input HoldDownstreamForFailureInput) (DecisionRequestResult, error)
	ApplyFailureRecoveryDecision(ctx context.Context, input ApplyFailureRecoveryDecisionInput) (ApplyFailureRecoveryDecisionResult, error)
	AppendProjectEvent(ctx context.Context, input AppendProjectEventInput) (ProjectEventResult, error)
	DispatchProjectTask(ctx context.Context, input DispatchProjectTaskInput) error
	FinishCoordinationJob(ctx context.Context, input FinishCoordinationJobInput) error
}
```

Add forwarding methods:

```go
func (a *Activities) PersistPlanRevision(ctx context.Context, input PersistPlanRevisionInput) (PlanRevisionResult, error) {
	if a.store == nil {
		return PlanRevisionResult{}, ErrActivityStoreRequired
	}
	return a.store.PersistPlanRevision(ctx, input)
}

func (a *Activities) RequestPlanRevisionReview(ctx context.Context, input RequestPlanRevisionReviewInput) (DecisionRequestResult, error) {
	if a.store == nil {
		return DecisionRequestResult{}, ErrActivityStoreRequired
	}
	return a.store.RequestPlanRevisionReview(ctx, input)
}

func (a *Activities) ResolvePlanRevisionReview(ctx context.Context, input ResolvePlanRevisionReviewInput) (PlanRevisionResult, error) {
	if a.store == nil {
		return PlanRevisionResult{}, ErrActivityStoreRequired
	}
	return a.store.ResolvePlanRevisionReview(ctx, input)
}

func (a *Activities) DecomposeAcceptedPlanRevision(ctx context.Context, input DecomposeAcceptedPlanRevisionInput) ([]ProjectTaskResult, error) {
	if a.store == nil {
		return nil, ErrActivityStoreRequired
	}
	return a.store.DecomposeAcceptedPlanRevision(ctx, input)
}
```

In the same step, remove the now-orphaned members so the package compiles:

- Drop `CreateProjectTasks` and `RequestRouteDecisionReview` from the `ActivityStore` interface and delete their `(*Activities)` forwarding methods (`activities.go:74`, `activities.go:130`).
- Delete `(*ProjectStore).CreateProjectTasks` (`project_store.go:359`) and `(*ProjectStore).RequestRouteDecisionReview` (`project_store.go:979`).
- Delete the helpers that only served them once no caller remains: `acceptedPlanRevisionIDForRouteDecision` (`:1367`), `acceptedPlanRevisionDecompositionClaimKey` (`:1383`), `routeReviewTargetUserID` (`:1202`), `routeReviewContext` (`:1628`).
- Update `project_store_test.go` and the `newProjectStoreMemoryRepository` test double to call `PersistPlanRevision` / `RequestPlanRevisionReview` / `ResolvePlanRevisionReview` / `DecomposeAcceptedPlanRevision` instead of the removed methods. Keep `RequestRouteDecisionReviewInput` / `CreateProjectTasksInput` types only if still referenced; otherwise delete them.

- [ ] **Step 5: Implement ProjectStore plan methods**

In `apps/control-plane/internal/workflow/projectcoordination/project_store.go`, add `PersistPlanRevision`:

```go
func (s *ProjectStore) PersistPlanRevision(ctx context.Context, input PersistPlanRevisionInput) (PlanRevisionResult, error) {
	if s.repository == nil {
		return PlanRevisionResult{}, ErrActivityStoreRequired
	}
	payload := BuildPlanRevisionPayload(input.Decision)
	validation := ValidatePlanRevisionPayload(payload)
	status := project.PlanRevisionStatusAccepted
	if !validation.Acceptable {
		status = project.PlanRevisionStatusValidationFailed
	} else if validation.ReviewRequired || input.Decision.RequiresHumanReview {
		status = project.PlanRevisionStatusPendingReview
	}
	event, err := s.repository.AppendProjectEvent(ctx, coordinatorEvent(input.TenantID, input.ProjectID, project.ProjectEventCoordinationJobUpdated, input.CoordinationJobID.String(), "计划版本已生成", map[string]any{
		"demand_id":        input.DemandID.String(),
		"plan_fingerprint": validation.PlanFingerprint,
		"status":           status,
	}))
	if err != nil {
		return PlanRevisionResult{}, err
	}
	projectRecord, err := s.repository.GetProject(ctx, input.TenantID, input.ProjectID)
	if err != nil {
		return PlanRevisionResult{}, err
	}
	revision, err := s.repository.CreatePlanRevision(ctx, project.CreatePlanRevisionRequest{
		TenantID:               input.TenantID,
		TeamID:                 projectRecord.TeamID,
		ProjectID:              input.ProjectID,
		DemandID:               input.DemandID,
		CoordinationJobID:       &input.CoordinationJobID,
		RouteDecisionID:         &input.RouteDecisionID,
		Status:                 status,
		Payload:                planRevisionPayloadMap(payload),
		PlanFingerprint:        validation.PlanFingerprint,
		ValidationErrors:       validation.Errors,
		ValidationWarnings:     validation.Warnings,
		ReviewRequired:         validation.ReviewRequired || input.Decision.RequiresHumanReview,
		ReviewReason:           stringPtr(strings.Join(validation.ReviewReasons, "; ")),
		SupersedeOpenRevisions: input.SupersedeOpen,
		SupersedeReason:        input.SupersedeReason,
	})
	if err != nil {
		return PlanRevisionResult{}, err
	}
	return PlanRevisionResult{
		ID:              revision.ID,
		Status:          revision.Status,
		RevisionNumber:  revision.RevisionNumber,
		PlanFingerprint: revision.PlanFingerprint,
		Payload:         payload,
		ReviewRequired:  revision.ReviewRequired,
		CreatedEventID:  event.ID,
	}, nil
}
```

Add `DecomposeAcceptedPlanRevision`:

```go
func (s *ProjectStore) DecomposeAcceptedPlanRevision(ctx context.Context, input DecomposeAcceptedPlanRevisionInput) ([]ProjectTaskResult, error) {
	if s.repository == nil {
		return nil, ErrActivityStoreRequired
	}
	graphTasks := make([]project.ProjectTaskGraphCreateTask, 0, len(input.Payload.Tasks))
	for _, plannedTask := range input.Payload.Tasks {
		employeeID, err := uuid.Parse(plannedTask.SelectedEmployeeID)
		if err != nil {
			return nil, project.ErrInvalidProject
		}
		metadata := map[string]any{
			"accepted_plan_revision_id": input.PlanRevisionID.String(),
			"plan_fingerprint":          input.PlanFingerprint,
			"employee_selection": map[string]any{
				"reason":                         plannedTask.EmployeeSelectionReason,
				"required_capabilities":          plannedTask.RequiredCapabilities,
				"matched_capabilities":           plannedTask.MatchedCapabilities,
				"missing_capabilities":           plannedTask.MissingCapabilities,
				"selection_score":                plannedTask.SelectionScore,
				"planning_profile_snapshot_hash": plannedTask.PlanningProfileSnapshotHash,
			},
			"acceptance_criteria": plannedTask.AcceptanceCriteria,
		}
		graphTasks = append(graphTasks, project.ProjectTaskGraphCreateTask{
			Key:                       plannedTask.PlannedTaskKey,
			Title:                     plannedTask.Title,
			Summary:                   plannedTask.Objective,
			Status:                    project.ProjectTaskStatusPlanned,
			AssignedDigitalEmployeeID: employeeID,
			TaskKind:                  plannedTask.TaskType,
			RiskLevel:                 plannedTask.RiskLevel,
			RequiresHumanApproval:     plannedTask.HumanReviewRequired,
			ExpectedOutputs:           stringsToAny(plannedTask.ExpectedOutputs),
			InputRequirements:         map[string]any{"input_context_refs": plannedTask.InputContextRefs},
			HandoffContract: map[string]any{
				"acceptance_criteria":       plannedTask.AcceptanceCriteria,
				"verification_requirements": plannedTask.VerificationRequirements,
			},
			PlannerMetadata: metadata,
			BlockedByKeys:   plannedTask.DependsOn,
		})
	}
	decomposition, err := s.repository.DecomposeAcceptedPlanRevision(ctx, project.DecomposeAcceptedPlanRevisionRequest{
		TenantID:               input.TenantID,
		ProjectID:              input.ProjectID,
		DemandID:               input.DemandID,
		CoordinationJobID:      input.CoordinationJobID,
		RouteDecisionID:        input.RouteDecisionID,
		AcceptedPlanRevisionID: input.PlanRevisionID,
		PlanFingerprint:        input.PlanFingerprint,
		DecompositionClaimKey:  input.PlanRevisionID.String(),
		Tasks:                  graphTasks,
	})
	if err != nil {
		return nil, err
	}
	results := make([]ProjectTaskResult, 0, len(decomposition.Tasks))
	for _, task := range decomposition.Tasks {
		results = append(results, ProjectTaskResult{ID: task.ID})
	}
	return results, nil
}
```

Implement `RequestPlanRevisionReview` by copying `RequestRouteDecisionReview` and changing:

- `ResourceType: "project_plan_revision"`
- `ResourceID: input.PlanRevisionID`
- `DecisionType: "plan_review"`
- `Title: "确认项目计划版本"`
- `Options: []any{"approved", "rejected", "request_changes", "cancelled"}`
- context payload includes `plan_revision_id`, `plan_fingerprint`, `tasks`, `risk_assessment`, `human_review`

Implement `ResolvePlanRevisionReview`:

```go
func (s *ProjectStore) ResolvePlanRevisionReview(ctx context.Context, input ResolvePlanRevisionReviewInput) (PlanRevisionResult, error) {
	switch input.Decision {
	case project.PlanReviewDecisionAccept:
		revision, err := s.repository.AcceptPlanRevision(ctx, project.AcceptPlanRevisionRequest{
			TenantID:   input.TenantID,
			ProjectID:  input.ProjectID,
			RevisionID: input.PlanRevisionID,
			AcceptedBy: nilUUIDToNil(input.ActorUserID),
		})
		if err != nil {
			return PlanRevisionResult{}, err
		}
		return planRevisionResultFromDomain(revision), nil
	case project.PlanReviewDecisionReject, project.PlanReviewDecisionCancel:
		reason := stringFromMap(input.Payload, "reason")
		revision, err := s.repository.RejectPlanRevision(ctx, project.RejectPlanRevisionRequest{
			TenantID:        input.TenantID,
			ProjectID:       input.ProjectID,
			RevisionID:      input.PlanRevisionID,
			RejectedBy:      nilUUIDToNil(input.ActorUserID),
			RejectionReason: &reason,
		})
		if err != nil {
			return PlanRevisionResult{}, err
		}
		return planRevisionResultFromDomain(revision), nil
	case project.PlanReviewDecisionRequestChanges:
		reason := stringFromMap(input.Payload, "reason")
		revision, err := s.repository.RejectPlanRevision(ctx, project.RejectPlanRevisionRequest{
			TenantID:        input.TenantID,
			ProjectID:       input.ProjectID,
			RevisionID:      input.PlanRevisionID,
			RejectedBy:      nilUUIDToNil(input.ActorUserID),
			RejectionReason: &reason,
		})
		if err != nil {
			return PlanRevisionResult{}, err
		}
		return planRevisionResultFromDomain(revision), nil
	default:
		return PlanRevisionResult{}, project.ErrInvalidProject
	}
}
```

- [ ] **Step 6: Rewrite workflow order**

In `apps/control-plane/internal/workflow/projectcoordination/workflow.go`:

- Change `pendingRouteDecisionReview` to `pendingPlanRevisionReview` with `DemandID`, `RouteDecisionID`, `PlanRevisionID`, `PlanFingerprint`, and `Payload`.
- In `handleDemandSubmitted`, call `PersistRouteDecision`, then `PersistPlanRevision`.
- If `PlanRevisionResult.Status == accepted`, call `DecomposeAcceptedPlanRevision`, then dispatch root tasks.
- If `pending_review`, call `RequestPlanRevisionReview` and return pending without creating ProjectTasks.
- If `validation_failed`, finish coordination job with `validation_failed`.
- In human approval, first call `ResolvePlanRevisionReview`; on approved, decompose and dispatch. On `request_changes`, rerun planning with `SupersedeOpen: true`.

Use this helper shape:

```go
func decomposeAndDispatchAcceptedPlan(ctx workflow.Context, input ProjectCoordinatorInput, pending pendingPlanRevisionReview) error {
	var tasks []ProjectTaskResult
	if err := workflow.ExecuteActivity(ctx, (*Activities).DecomposeAcceptedPlanRevision, DecomposeAcceptedPlanRevisionInput{
		TenantID:          input.TenantID,
		ProjectID:         pending.ProjectID,
		DemandID:          pending.DemandID,
		CoordinationJobID: pending.CoordinationJobID,
		RouteDecisionID:   pending.RouteDecisionID,
		PlanRevisionID:    pending.PlanRevisionID,
		PlanFingerprint:   pending.PlanFingerprint,
		Payload:           pending.Payload,
	}).Get(ctx, &tasks); err != nil {
		return err
	}
	readyTaskIDs, err := listDispatchableTasks(ctx, input.TenantID, pending.ProjectID, pending.CoordinationJobID)
	if err != nil {
		return err
	}
	if err := dispatchProjectTasks(ctx, input.TenantID, pending.ProjectID, readyTaskIDs); err != nil {
		return err
	}
	outputEventIDs := append([]uuid.UUID{}, pending.OutputEventIDs...)
	return finishCoordinationJob(ctx, input.TenantID, pending.CoordinationJobID, "completed", outputEventIDs)
}
```

**Human-decision routing for `plan_review`:** The existing human-decision resolution path (Web `resolveDecisionMutation` → Control Plane decision-resolution endpoint → coordinator signal) must dispatch on `DecisionType` so that `plan_review` reaches `ResolvePlanRevisionReview` while legacy `route_review` keeps its current handler. This is the same chain a prior session found broken (`approve` failed while `reject` succeeded); wire it explicitly here and exercise it end-to-end in Task 7 Step 4b.

- [ ] **Step 7: Run ProjectStore and workflow tests**

Run:

```bash
go test ./apps/control-plane/internal/workflow/projectcoordination -run 'TestProjectStore.*PlanRevision|TestProjectCoordinator.*PlanRevision|TestProjectCoordinatorPausesDispatchWhenRouteRequiresHumanReview|TestProjectCoordinatorDispatchesOnlyRootTasks' -count=1
```

Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add apps/control-plane/internal/workflow/projectcoordination/types.go apps/control-plane/internal/workflow/projectcoordination/activities.go apps/control-plane/internal/workflow/projectcoordination/project_store.go apps/control-plane/internal/workflow/projectcoordination/project_store_test.go apps/control-plane/internal/workflow/projectcoordination/workflow.go apps/control-plane/internal/workflow/projectcoordination/workflow_test.go
git commit -m "feat: gate task decomposition on accepted plans"
```

### Task 5: Control Plane API And OpenAPI Contract

**Files:**
- Modify: `apps/control-plane/internal/project/service.go`
- Modify: `apps/control-plane/internal/project/handler.go`
- Modify: `apps/control-plane/internal/api/server.go`
- Modify: `apps/control-plane/internal/project/handler_test.go`
- Modify: `apps/control-plane/internal/api/project_routes_test.go`
- Modify: `contracts/control-plane/openapi.yaml`
- Generated: OpenAPI generated files from `corepack pnpm generate:control-plane`

- [ ] **Step 1: Write failing handler tests**

In `apps/control-plane/internal/project/handler_test.go`, add:

```go
func TestProjectHandlerListsPlanRevisions(t *testing.T) {
	projectID := uuid.New()
	revisionID := uuid.New()
	tenantID := uuid.New()
	service := &handlerTestService{
		planRevisions: []PlanRevision{
			{
				ID:              revisionID,
				TenantID:        tenantID,
				ProjectID:       projectID,
				DemandID:        uuid.New(),
				RevisionNumber:  1,
				Status:          PlanRevisionStatusPendingReview,
				Payload:         map[string]any{"summary": "计划"},
				PlanFingerprint: "fingerprint",
				ReviewRequired:  true,
				CreatedAt:       time.Now().UTC(),
				UpdatedAt:       time.Now().UTC(),
			},
		},
	}
	handler := NewHandler(service)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+projectID.String()+"/plan-revisions?limit=10", nil)
	req = req.WithContext(context.WithValue(req.Context(), middleware.TenantIDKey, tenantID))
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, uuid.New()))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("projectId", projectID.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	resp := httptest.NewRecorder()
	handler.ListPlanRevisions(resp, req)

	require.Equal(t, http.StatusOK, resp.Code)
	require.Contains(t, resp.Body.String(), revisionID.String())
	require.Contains(t, resp.Body.String(), "pending_review")
	require.Contains(t, resp.Body.String(), "fingerprint")
}
```

In `apps/control-plane/internal/api/project_routes_test.go`, add a route assertion for:

```go
req := httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+service.projectID.String()+"/plan-revisions?limit=7", nil)
```

Expected handler method: `ListPlanRevisions`.

- [ ] **Step 2: Run handler tests and verify they fail**

Run:

```bash
go test ./apps/control-plane/internal/project -run TestProjectHandlerListsPlanRevisions -count=1
go test ./apps/control-plane/internal/api -run TestProjectRoutesUseConsoleAuthAndProjectService -count=1
```

Expected: FAIL because handler and route methods do not exist.

- [ ] **Step 3: Add service and handler methods**

Add to `project.HandlerService`:

```go
	ListPlanRevisions(ctx context.Context, req ListPlanRevisionsRequest) ([]PlanRevision, error)
	GetPlanRevision(ctx context.Context, tenantID, projectID, revisionID uuid.UUID) (*PlanRevision, error)
```

Add to `apps/control-plane/internal/project/service.go`:

```go
func (s *Service) ListPlanRevisions(ctx context.Context, req ListPlanRevisionsRequest) ([]PlanRevision, error) {
	if req.TenantID == uuid.Nil || req.ProjectID == uuid.Nil {
		return nil, ErrInvalidProject
	}
	if req.Limit <= 0 {
		req.Limit = 20
	}
	return s.repository.ListPlanRevisions(ctx, req)
}

func (s *Service) GetPlanRevision(ctx context.Context, tenantID, projectID, revisionID uuid.UUID) (*PlanRevision, error) {
	if tenantID == uuid.Nil || projectID == uuid.Nil || revisionID == uuid.Nil {
		return nil, ErrInvalidProject
	}
	revision, err := s.repository.GetPlanRevision(ctx, tenantID, projectID, revisionID)
	if err != nil {
		return nil, err
	}
	return &revision, nil
}
```

Add response structs and handlers in `apps/control-plane/internal/project/handler.go`:

```go
type planRevisionResponse struct {
	ID                       string         `json:"id"`
	TenantID                 string         `json:"tenant_id"`
	TeamID                   *string        `json:"team_id,omitempty"`
	ProjectID                string         `json:"project_id"`
	DemandID                 string         `json:"demand_id"`
	CoordinationJobID         *string        `json:"coordination_job_id,omitempty"`
	RouteDecisionID           *string        `json:"route_decision_id,omitempty"`
	RevisionNumber            int32          `json:"revision_number"`
	Status                   string         `json:"status"`
	Payload                  map[string]any `json:"payload"`
	PlanFingerprint          string         `json:"plan_fingerprint"`
	ValidationErrors         []string       `json:"validation_errors"`
	ValidationWarnings       []string       `json:"validation_warnings"`
	ReviewRequired           bool           `json:"review_required"`
	ReviewReason             *string        `json:"review_reason,omitempty"`
	SupersededByRevisionID    *string        `json:"superseded_by_revision_id,omitempty"`
	DecompositionClaimID      *string        `json:"decomposition_claim_id,omitempty"`
	CreatedTaskIDs           []string       `json:"created_task_ids"`
	CreatedAt                time.Time      `json:"created_at"`
	UpdatedAt                time.Time      `json:"updated_at"`
}

func (h *HTTPHandler) ListPlanRevisions(w http.ResponseWriter, r *http.Request) {
	tenantID, _, ok := consoleIdentity(w, r)
	if !ok {
		return
	}
	projectID, ok := projectIDFromRoute(w, r)
	if !ok {
		return
	}
	service, ok := h.serviceFromRequest(w)
	if !ok {
		return
	}
	limit, offset, ok := paginationFromRequest(w, r)
	if !ok {
		return
	}
	var demandID *uuid.UUID
	if raw := strings.TrimSpace(r.URL.Query().Get("demand_id")); raw != "" {
		parsed, err := uuid.Parse(raw)
		if err != nil {
			writeHandlerError(w, ErrInvalidProject)
			return
		}
		demandID = &parsed
	}
	revisions, err := service.ListPlanRevisions(r.Context(), ListPlanRevisionsRequest{
		TenantID:  tenantID,
		ProjectID: projectID,
		DemandID:  demandID,
		Limit:     limit,
		Offset:    offset,
	})
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, planRevisionResponses(revisions))
}
```

Register routes in `apps/control-plane/internal/api/server.go` inside the project route group:

```go
r.Get("/projects/{projectId}/plan-revisions", s.projectHandler.ListPlanRevisions)
r.Get("/projects/{projectId}/plan-revisions/{revisionId}", s.projectHandler.GetPlanRevision)
```

- [ ] **Step 4: Update OpenAPI**

Add paths in `contracts/control-plane/openapi.yaml`:

```yaml
  /api/v1/projects/{projectId}/plan-revisions:
    get:
      operationId: listProjectPlanRevisions
      summary: List project plan revisions
      parameters:
        - $ref: "#/components/parameters/ProjectId"
        - name: demand_id
          in: query
          required: false
          schema:
            type: string
            format: uuid
        - $ref: "#/components/parameters/Limit"
        - $ref: "#/components/parameters/Offset"
      responses:
        "200":
          description: Project plan revision list
          content:
            application/json:
              schema:
                type: array
                items:
                  $ref: "#/components/schemas/ProjectPlanRevision"
  /api/v1/projects/{projectId}/plan-revisions/{revisionId}:
    get:
      operationId: getProjectPlanRevision
      summary: Get project plan revision
      parameters:
        - $ref: "#/components/parameters/ProjectId"
        - name: revisionId
          in: path
          required: true
          schema:
            type: string
            format: uuid
      responses:
        "200":
          description: Project plan revision
          content:
            application/json:
              schema:
                $ref: "#/components/schemas/ProjectPlanRevision"
```

Add schema:

```yaml
    ProjectPlanRevision:
      type: object
      required:
        - id
        - tenant_id
        - project_id
        - demand_id
        - revision_number
        - status
        - payload
        - plan_fingerprint
        - validation_errors
        - validation_warnings
        - review_required
        - created_task_ids
        - created_at
        - updated_at
      properties:
        id:
          type: string
          format: uuid
        tenant_id:
          type: string
          format: uuid
        team_id:
          type: string
          format: uuid
        project_id:
          type: string
          format: uuid
        demand_id:
          type: string
          format: uuid
        coordination_job_id:
          type: string
          format: uuid
        route_decision_id:
          type: string
          format: uuid
        revision_number:
          type: integer
          format: int32
        status:
          type: string
        payload:
          type: object
          additionalProperties: true
        plan_fingerprint:
          type: string
        validation_errors:
          type: array
          items:
            type: string
        validation_warnings:
          type: array
          items:
            type: string
        review_required:
          type: boolean
        review_reason:
          type: string
        superseded_by_revision_id:
          type: string
          format: uuid
        decomposition_claim_id:
          type: string
          format: uuid
        created_task_ids:
          type: array
          items:
            type: string
            format: uuid
        created_at:
          type: string
          format: date-time
        updated_at:
          type: string
          format: date-time
```

- [ ] **Step 5: Generate contracts and run tests**

Run:

```bash
corepack pnpm generate:control-plane
corepack pnpm verify:contracts
go test ./apps/control-plane/internal/project -run 'TestProjectHandlerListsPlanRevisions|TestProjectHandlerGetsPlanRevision' -count=1
go test ./apps/control-plane/internal/api -run TestProjectRoutesUseConsoleAuthAndProjectService -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add apps/control-plane/internal/project/service.go apps/control-plane/internal/project/handler.go apps/control-plane/internal/project/handler_test.go apps/control-plane/internal/api/server.go apps/control-plane/internal/api/project_routes_test.go contracts/control-plane/openapi.yaml
git add apps/control-plane/internal/storage/queries
git commit -m "feat: expose plan revision read model"
```

### Task 6: Web Plan Review Read Model

**Files:**
- Read before editing: `DESIGN.md`
- Modify: `apps/web/src/lib/api/projects.ts`
- Modify: `apps/web/src/lib/api/projects.test.ts`
- Modify: `apps/web/src/features/projects/index.tsx`
- Modify: `apps/web/src/features/projects/components/project-operational-detail.tsx`
- Modify: `apps/web/src/features/projects/index.test.tsx`

- [ ] **Step 1: Read the frontend design guide**

Run:

```bash
sed -n '1,220p' DESIGN.md
```

Expected: read the local visual and UX rules before touching `apps/web`.

- [ ] **Step 2: Write failing API tests**

In `apps/web/src/lib/api/projects.test.ts`, add:

```ts
it("lists project plan revisions", async () => {
  const fetcher = createJsonFetcher([
    {
      url: "http://control-plane.local/api/v1/projects/project%201%2Fprimary/plan-revisions?limit=10",
      response: [
        {
          id: "revision-1",
          tenant_id: "tenant-1",
          project_id: "project 1/primary",
          demand_id: "demand-1",
          revision_number: 1,
          status: "pending_review",
          payload: { summary: "检查计划", tasks: [] },
          plan_fingerprint: "fingerprint",
          validation_errors: [],
          validation_warnings: [],
          review_required: true,
          created_task_ids: [],
          created_at: "2026-06-21T10:00:00Z",
          updated_at: "2026-06-21T10:00:00Z",
        },
      ],
    },
  ]);

  const result = await listProjectPlanRevisions(
    { baseUrl: "http://control-plane.local", fetcher },
    "project 1/primary",
    { limit: 10 },
  );

  expect(result[0]?.id).toBe("revision-1");
  expect(result[0]?.status).toBe("pending_review");
});
```

- [ ] **Step 3: Run API test and verify it fails**

Run:

```bash
corepack pnpm --filter ./apps/web run test -- src/lib/api/projects.test.ts
```

Expected: FAIL with `listProjectPlanRevisions is not defined`.

- [ ] **Step 4: Add Web API types and fetcher**

In `apps/web/src/lib/api/projects.ts`, add:

```ts
export type ProjectPlanRevisionPayloadTask = {
  planned_task_key: string;
  title: string;
  objective: string;
  task_type: string;
  selected_employee_id: string;
  employee_selection_reason: string;
  required_capabilities: string[];
  matched_capabilities: string[];
  missing_capabilities: string[];
  permission_requirements: string[];
  tool_requirements: string[];
  runtime_requirements: string[];
  input_context_refs: string[];
  expected_outputs: string[];
  acceptance_criteria: string[];
  verification_requirements: string[];
  depends_on: string[];
  risk_level: string;
  human_review_required: boolean;
};

export type ProjectPlanRevisionPayload = {
  summary?: string;
  assumptions?: string[];
  risk_assessment?: { level?: string; reasons?: string[] };
  human_review?: { required?: boolean; reasons?: string[] };
  tasks?: ProjectPlanRevisionPayloadTask[];
  final_summary_contract?: { required_sections?: string[] };
};

export type ProjectPlanRevision = {
  id: string;
  tenant_id: string;
  team_id?: string;
  project_id: string;
  demand_id: string;
  coordination_job_id?: string;
  route_decision_id?: string;
  revision_number: number;
  status: string;
  payload: ProjectPlanRevisionPayload;
  plan_fingerprint: string;
  validation_errors: string[];
  validation_warnings: string[];
  review_required: boolean;
  review_reason?: string;
  superseded_by_revision_id?: string;
  decomposition_claim_id?: string;
  created_task_ids: string[];
  created_at?: string;
  updated_at?: string;
};

export function listProjectPlanRevisions(
  options: ApiClientOptions,
  projectId: string,
  filters: PaginationFilters & { demandId?: string } = {},
): Promise<ProjectPlanRevision[]> {
  const demandQuery = filters.demandId
    ? `&demand_id=${encodeURIComponent(filters.demandId)}`
    : "";
  return getJson<ProjectPlanRevision[]>(
    options,
    projectPath(projectId, `/plan-revisions${paginationQuery(filters)}${demandQuery}`),
    "project plan revisions",
  );
}
```

- [ ] **Step 5: Add project page query and invalidation**

In `apps/web/src/features/projects/index.tsx`:

- import `listProjectPlanRevisions`
- add query:

```tsx
const planRevisionsQuery = useQuery({
  enabled: Boolean(effectiveProjectId),
  queryKey: ["project-plan-revisions", effectiveProjectId],
  queryFn: () =>
    listProjectPlanRevisions(apiOptions, effectiveProjectId as string, { limit: 20 }),
  placeholderData: keepPreviousData,
});
```

- add this invalidation to `resolveDecisionMutation.onSuccess`:

```tsx
queryClient.invalidateQueries({ queryKey: ["project-plan-revisions", projectId] }),
```

- pass `planRevisions={planRevisionsQuery.data ?? []}` into `ProjectOperationalDetail`.

- [ ] **Step 6: Add compact plan revision panel**

In `apps/web/src/features/projects/components/project-operational-detail.tsx`, extend props:

```tsx
import type { ProjectPlanRevision } from "../../../lib/api/projects";

type ProjectOperationalDetailProps = {
  planRevisions: ProjectPlanRevision[];
};
```

Add panel near "路由决策":

```tsx
<LiquidCard className="rounded-xl">
  <PanelHeader
    icon={<GitBranch />}
    title="计划版本"
    meta={`${planRevisions.length} 版`}
  />
  <div className="divide-y">
    {planRevisions.length === 0 ? (
      <EmptyLine label="暂无计划版本" />
    ) : (
      planRevisions.slice(0, 5).map((revision) => {
        const tasks = revision.payload.tasks ?? [];
        return (
          <div className="grid gap-3 p-4" key={revision.id}>
            <div className="flex items-start justify-between gap-3">
              <div className="min-w-0">
                <p className="text-sm font-medium">Revision {revision.revision_number}</p>
                <p className="line-clamp-2 text-xs text-muted-foreground">
                  {revision.payload.summary || "未提供计划摘要"}
                </p>
              </div>
              <StatusBadge tone={revision.status === "decomposed" ? "success" : "warning"}>
                {revision.status}
              </StatusBadge>
            </div>
            <div className="grid gap-2">
              {tasks.slice(0, 4).map((task) => (
                <div className="rounded-md border p-3" key={task.planned_task_key}>
                  <div className="flex items-start justify-between gap-3">
                    <p className="min-w-0 text-sm font-medium">{task.title}</p>
                    <span className="shrink-0 text-xs text-muted-foreground">
                      {task.risk_level || "low"}
                    </span>
                  </div>
                  <RuntimeMeta label="数字员工" value={task.selected_employee_id} />
                  <RuntimeMeta label="选择理由" value={task.employee_selection_reason} />
                  <RuntimeMeta
                    label="能力匹配"
                    value={`${task.matched_capabilities.length}/${task.required_capabilities.length}`}
                  />
                </div>
              ))}
            </div>
          </div>
        );
      })
    )}
  </div>
</LiquidCard>
```

Keep action buttons in the existing "人工决策" panel. Change labels for `decision_type === "plan_review"` so pending plan review offers:

- `批准` -> `approved`
- `要求修改` -> `request_changes`
- `驳回` -> `rejected`

- [ ] **Step 7: Run Web tests**

Run:

```bash
corepack pnpm --filter ./apps/web run test -- src/lib/api/projects.test.ts src/features/projects/index.test.tsx
```

Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add apps/web/src/lib/api/projects.ts apps/web/src/lib/api/projects.test.ts apps/web/src/features/projects/index.tsx apps/web/src/features/projects/components/project-operational-detail.tsx apps/web/src/features/projects/index.test.tsx
git commit -m "feat: show plan revisions in project console"
```

### Task 7: End-To-End Control Plane Verification

**Files:**
- Modify: `CHANGELOG.md`

- [ ] **Step 1: Run targeted backend tests**

Run:

```bash
go test ./apps/control-plane/internal/workflow/projectcoordination -run 'TestPlanRevision|TestProjectStore.*PlanRevision|TestProjectCoordinator.*PlanRevision|TestProjectCoordinatorPausesDispatchWhenRouteRequiresHumanReview|TestProjectCoordinatorDispatchesOnlyRootTasks' -count=1
go test ./apps/control-plane/internal/project -run 'TestProjectPlanRevision|TestDecomposeAcceptedPlanRevision|TestProjectHandlerListsPlanRevisions' -count=1
go test ./apps/control-plane/internal/api -run TestProjectRoutesUseConsoleAuthAndProjectService -count=1
go test ./apps/control-plane/internal/storage -run TestProjectPlanRevisionsMigrationHasTenantFirstIndexes -count=1
```

Expected: PASS.

- [ ] **Step 2: Run contract and Web tests**

Run:

```bash
corepack pnpm verify:contracts
corepack pnpm --filter ./apps/web run test -- src/lib/api/projects.test.ts src/features/projects/index.test.tsx
```

Expected: PASS.

- [ ] **Step 3: Run migration status against the intended development database**

Run:

```bash
scripts/dev-services.sh status
DATABASE_URL="${DATABASE_URL}" make -C apps/control-plane migrate-status
```

Expected:

- dev-services reports current service status
- migration status command can see migration `031_project_plan_revisions.sql`

If `DATABASE_URL` is not set in the shell, read the project-approved local env file used by `scripts/dev-services.sh` and run the same command with that database URL. Do not point this migration check at an unconfirmed database.

- [ ] **Step 4: Run real Control Plane smoke**

If Control Plane is running:

```bash
scripts/dev-services.sh restart control-plane
curl -i "$CONTROL_PLANE_URL/api/v1/projects/$PROJECT_ID/plan-revisions?limit=5" \
  -H "Cookie: $SUPERTEAM_CONSOLE_COOKIE"
```

Expected:

- HTTP status is non-5xx
- authenticated local session returns `200` with JSON array, or `404` only if the chosen project does not exist
- response body contains plan revision fields after a demand has generated a plan

If auth cookie or a local project ID is unavailable, mark real-chain verification blocked and keep local tests as supporting evidence only.

- [ ] **Step 4b: Verify the real plan → approve → decompose closed loop**

This is the default completion gate per `CLAUDE.md` (real end-to-end, not just a list endpoint). With Control Plane, Temporal, and Web running the current code, drive the full golden path once and assert the loop actually closes:

1. Submit a real demand on an existing project (Web or `curl`) and wait for the coordinator to produce a `pending_review` PlanRevision.
2. `GET /api/v1/projects/{projectId}/plan-revisions` returns the revision with `status: pending_review`, and **no** ProjectTasks exist yet for that demand.
3. Resolve the `plan_review` human decision with `approved` (Web 决策队列 or the decision-resolution endpoint).
4. The revision transitions `accepted` → `decomposing` → `decomposed`, the `project_plan_decomposition_claims` row is `completed`, and the ProjectTask DAG is created with root tasks dispatched. (Runtime/Provider execution is **out of scope** for Phase 2 — stop at task dispatch.)

```bash
scripts/dev-services.sh status
scripts/dev-services.sh restart control-plane temporal web
# 1. submit demand — adapt auth to the local console session
curl -i "$CONTROL_PLANE_URL/api/v1/projects/$PROJECT_ID/demands" -H "Cookie: $SUPERTEAM_CONSOLE_COOKIE" ...
# 2. poll until a pending_review revision appears
curl -s "$CONTROL_PLANE_URL/api/v1/projects/$PROJECT_ID/plan-revisions?limit=5" -H "Cookie: $SUPERTEAM_CONSOLE_COOKIE"
# 3. approve the plan_review decision via the same endpoint the Web console uses
# 4. assert revision becomes decomposed and project_tasks exist
```

Expected: revision reaches `decomposed`, `created_task_ids` is non-empty, and at least one root ProjectTask is dispatchable. If the `plan_review` approve action fails (the symptom recorded in session memory), mark the task **blocked** with the missing dependency — do not declare Phase 2 complete on curl-list evidence alone.

- [ ] **Step 5: Add changelog entry**

Run:

```bash
TZ=Asia/Shanghai date '+%Y-%m-%d %H:%M'
```

Add to `CHANGELOG.md` under the latest section:

```markdown
- <timestamp> 动态项目规划 Phase 2：新增 PlanRevision 计划版本事实源、人工 review 绑定具体 revision、accepted revision 精确一次分解 ProjectTask DAG，并在项目控制台展示计划版本读模型。
```

Use the exact timestamp printed by the command.

- [ ] **Step 6: Run final hygiene**

Run:

```bash
gofmt -w apps/control-plane/internal/project apps/control-plane/internal/workflow/projectcoordination
corepack pnpm verify:contracts
git diff --check
```

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add CHANGELOG.md
git add apps/control-plane/internal/project apps/control-plane/internal/workflow/projectcoordination apps/control-plane/internal/api apps/control-plane/internal/storage contracts/control-plane apps/web/src/lib/api apps/web/src/features/projects
git commit -m "feat: implement plan revision decomposition"
```

## Self-Review

Spec coverage:

- Versioned `PlanRevision`: Tasks 1, 2, 3, 5, 6.
- Status machine: Tasks 1 and 3.
- Human accept/reject/request changes/cancel: Tasks 4, 5, 6.
- Accepted revision exact-once decomposition: Tasks 2, 3, 4.
- ProjectTask contract metadata and dependencies: Task 4 reuses `ProjectTaskGraphCreateTask` and keeps `accepted_plan_revision_id`, `planned_task_key`, employee-selection metadata, outputs, acceptance criteria, verification requirements, and `BlockedByKeys`.
- Supersede old draft/pending revision: Tasks 2, 3, 4.
- No partial plan accept and no graph editor: File Structure and Task 6 keep whole-plan review only.
- Phase 2 no Runtime execution requirement: Task 7 verifies Control Plane persistence and API path; Runtime smoke is not part of Phase 2.

Placeholder scan:

- The plan uses concrete filenames, test names, commands, status strings, route names, SQL objects, and code snippets.
- No step leaves a behavior unspecified for a later design pass.

Type consistency:

- PlanRevision IDs are `uuid.UUID` in Go and `format: uuid` in OpenAPI/Web.
- Review action strings are `approved`, `rejected`, `request_changes`, and `cancelled`.
- Decomposition uses `PlanRevisionID` in workflow input and `AcceptedPlanRevisionID` at repository/task persistence boundaries.

## Execution Handoff

Plan complete and saved to `docs/superpowers/plans/2026-06-21-dynamic-project-planning-phase-2-plan-revision-decomposition.md`. Two execution options:

1. Subagent-Driven (recommended) - dispatch a fresh subagent per task, review between tasks, fast iteration.
2. Inline Execution - execute tasks in this session using executing-plans, batch execution with checkpoints.
